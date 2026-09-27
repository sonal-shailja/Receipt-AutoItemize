package transaction_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	"github.com/sonalshailja/receipt-autoitemize/internal/entities/receipt"
	"github.com/sonalshailja/receipt-autoitemize/internal/entities/transaction"
	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
	"github.com/sonalshailja/receipt-autoitemize/internal/testutils"
)

// seedTransaction inserts the receipt and OCR result a transaction's foreign
// keys need, then the transaction itself.
func seedTransaction(t *testing.T, db *database.DB) *transaction.Transaction {
	t.Helper()
	ctx := context.Background()
	receipts := receipt.NewRepo(db)

	rec := &receipt.Receipt{FileName: "a.txt", ContentType: "text/plain", FilePath: "/tmp/a", SizeBytes: 1, SHA256: "h"}
	require.NoError(t, receipts.Create(ctx, rec))
	result := &receipt.OCRResult{ReceiptID: rec.ID, Provider: "stub", Status: receipt.OCRStatusSucceeded}
	require.NoError(t, receipts.CreateOCRResult(ctx, result))

	total := int64(1785)
	txn := &transaction.Transaction{
		ReceiptID: rec.ID, OCRResultID: result.ID, Merchant: "Cafe Mitte", GrandTotalMinor: &total,
		ItemizeStatus: reconcile.StatusComplete,
	}
	require.NoError(t, transaction.NewRepo(db).Create(ctx, txn))
	return txn
}

func TestRepo_ReplaceLineItemsSoftDeletesPreviousRows(t *testing.T) {
	ctx := context.Background()
	db := testutils.NewDB(t)
	repo := transaction.NewRepo(db)
	txn := seedTransaction(t, db)

	require.NoError(t, repo.ReplaceLineItems(ctx, txn.ID, []transaction.LineItem{
		{Description: "Espresso", AmountMinor: 350, Source: transaction.SourceAuto},
		{Description: "Sandwlch", AmountMinor: 890, Source: transaction.SourceAuto},
	}))
	require.NoError(t, repo.ReplaceLineItems(ctx, txn.ID, []transaction.LineItem{
		{Description: "Sandwich", AmountMinor: 1240, Source: transaction.SourceUser},
	}))

	got, err := repo.FindByID(ctx, txn.ID)
	require.NoError(t, err)
	require.Len(t, got.LineItems, 1)
	assert.Equal(t, "Sandwich", got.LineItems[0].Description)
	assert.Equal(t, 1, got.LineItems[0].Position)

	var all, deleted int64
	require.NoError(t, db.Instance(ctx).Table("line_items").Where("transaction_id = ?", txn.ID).Count(&all).Error)
	require.NoError(t, db.Instance(ctx).Table("line_items").
		Where("transaction_id = ? AND deleted_at IS NOT NULL", txn.ID).Count(&deleted).Error)
	assert.Equal(t, int64(3), all, "earlier rows are kept")
	assert.Equal(t, int64(2), deleted)
}

func TestRepo_ReplaceTaxesOrdersByPosition(t *testing.T) {
	ctx := context.Background()
	db := testutils.NewDB(t)
	repo := transaction.NewRepo(db)
	txn := seedTransaction(t, db)

	require.NoError(t, repo.ReplaceTaxes(ctx, txn.ID, []transaction.Tax{
		{Name: "VAT", RateBps: 1900, AmountMinor: 190},
		{Name: "VAT", RateBps: 700, AmountMinor: 35},
	}))

	got, err := repo.FindByID(ctx, txn.ID)
	require.NoError(t, err)
	require.Len(t, got.Taxes, 2)
	assert.Equal(t, int64(1900), got.Taxes[0].RateBps)
	assert.Equal(t, int64(700), got.Taxes[1].RateBps)
}

func TestRepo_FindByReceiptIDAndUpdate(t *testing.T) {
	ctx := context.Background()
	db := testutils.NewDB(t)
	repo := transaction.NewRepo(db)
	txn := seedTransaction(t, db)

	none, err := repo.FindByReceiptID(ctx, "rcpt_other")
	require.NoError(t, err)
	assert.Nil(t, none)

	found, err := repo.FindByReceiptID(ctx, txn.ReceiptID)
	require.NoError(t, err)
	require.NotNil(t, found)

	found.ItemizeStatus = reconcile.StatusNeedsReview
	found.ItemizeReason = reconcile.ReasonTotalMismatch
	require.NoError(t, repo.Update(ctx, found))

	got, err := repo.FindByID(ctx, txn.ID)
	require.NoError(t, err)
	assert.Equal(t, reconcile.StatusNeedsReview, got.ItemizeStatus)
	assert.Equal(t, reconcile.ReasonTotalMismatch, got.ItemizeReason)
}

func TestRepo_UpdateItemizeStatusLeavesHeaderAlone(t *testing.T) {
	ctx := context.Background()
	db := testutils.NewDB(t)
	repo := transaction.NewRepo(db)
	txn := seedTransaction(t, db)

	require.NoError(t, repo.UpdateItemizeStatus(ctx, txn.ID, reconcile.StatusNeedsReview, reconcile.ReasonTotalMismatch))

	got, err := repo.FindByID(ctx, txn.ID)
	require.NoError(t, err)
	assert.Equal(t, reconcile.StatusNeedsReview, got.ItemizeStatus)
	assert.Equal(t, reconcile.ReasonTotalMismatch, got.ItemizeReason)
	assert.Equal(t, int64(1785), *got.GrandTotalMinor)
	assert.Equal(t, txn.OCRResultID, got.OCRResultID)
}

func TestRepo_FindByIDNotFound(t *testing.T) {
	_, err := transaction.NewRepo(testutils.NewDB(t)).FindByID(context.Background(), "txn_missing")
	assert.Equal(t, apperrors.KindNotFound, apperrors.KindOf(err))
}

func TestTransaction_ReconcileUsesStoredTotalAndTaxes(t *testing.T) {
	total := int64(1785)
	txn := &transaction.Transaction{
		GrandTotalMinor: &total,
		Taxes:           []transaction.Tax{{AmountMinor: 285}},
	}
	assert.True(t, txn.Reconcile([]int64{610, 890}).Reconciled())
	assert.False(t, txn.Reconcile([]int64{350}).Reconciled())
}

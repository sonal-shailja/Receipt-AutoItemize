package receipt

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	receiptentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/receipt"
	transactionentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/transaction"
	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/ocr"
	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
	"github.com/sonalshailja/receipt-autoitemize/internal/testutils"
	"github.com/sonalshailja/receipt-autoitemize/pkg/filestore"
	"github.com/sonalshailja/receipt-autoitemize/pkg/money"
)

const fixturesDir = "../../../fixtures/task-a"

type harness struct {
	svc *Service
	db  *database.DB
}

func newHarness(t *testing.T) harness {
	t.Helper()
	db := testutils.NewDB(t)
	files, err := filestore.NewLocal(t.TempDir())
	require.NoError(t, err)
	svc := NewService(db, receiptentity.NewRepo(db), transactionentity.NewRepo(db), ocr.NewStub(fixturesDir), files)
	return harness{svc: svc, db: db}
}

func (h harness) upload(t *testing.T, name, contentType string) *receiptentity.Receipt {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(fixturesDir, name))
	if err != nil {
		body = []byte("\x89PNG not really an image")
	}
	rec, err := h.svc.Upload(context.Background(), UploadInput{FileName: name, ContentType: contentType, Body: bytes.NewReader(body)})
	require.NoError(t, err)
	return rec
}

func (h harness) process(t *testing.T, fixture string) *transactionentity.Transaction {
	t.Helper()
	txn, err := h.svc.Process(context.Background(), h.upload(t, fixture, "text/plain").ID)
	require.NoError(t, err)
	return txn
}

func (h harness) count(t *testing.T, table, where string, args ...any) int64 {
	t.Helper()
	var n int64
	require.NoError(t, h.db.Instance(context.Background()).Table(table).Where(where, args...).Count(&n).Error)
	return n
}

type goldReceipt struct {
	Merchant   string  `json:"merchant"`
	Date       string  `json:"date"`
	Currency   string  `json:"currency"`
	GrandTotal float64 `json:"grand_total"`
	Taxes      []struct {
		Name   string  `json:"name"`
		Rate   float64 `json:"rate"`
		Amount float64 `json:"amount"`
	} `json:"taxes"`
	LineItems []struct {
		Description string  `json:"description"`
		Amount      float64 `json:"amount"`
	} `json:"line_items"`
	ItemizeStatus string `json:"itemize_status"`
}

func TestProcess_MatchesGold(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(fixturesDir, "gold.json"))
	require.NoError(t, err)
	var gold map[string]goldReceipt
	require.NoError(t, json.Unmarshal(raw, &gold))
	require.Len(t, gold, 3)

	for name, want := range gold {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			txn := h.process(t, name+".txt")

			assert.Equal(t, want.Merchant, txn.Merchant)
			assert.Equal(t, want.Date, txn.Date)
			assert.Equal(t, want.Currency, txn.Currency)
			require.NotNil(t, txn.GrandTotalMinor)
			assert.InDelta(t, want.GrandTotal, money.ToFloat(*txn.GrandTotalMinor), 0.001)
			assert.Equal(t, want.ItemizeStatus, string(txn.ItemizeStatus))

			require.Len(t, txn.Taxes, len(want.Taxes))
			for i, tax := range want.Taxes {
				assert.Equal(t, tax.Name, txn.Taxes[i].Name)
				assert.InDelta(t, tax.Rate, money.RateToFraction(txn.Taxes[i].RateBps), 0.0001)
				assert.InDelta(t, tax.Amount, money.ToFloat(txn.Taxes[i].AmountMinor), 0.001)
			}

			require.Len(t, txn.LineItems, len(want.LineItems))
			for i, item := range want.LineItems {
				assert.Equal(t, item.Description, txn.LineItems[i].Description)
				assert.InDelta(t, item.Amount, money.ToFloat(txn.LineItems[i].AmountMinor), 0.001)
				assert.Equal(t, transactionentity.SourceAuto, txn.LineItems[i].Source)
			}
		})
	}
}

func TestProcess_MismatchKeepsTotalAndAddsNoBalancingLine(t *testing.T) {
	txn := newHarness(t).process(t, "receipt-mismatch.txt")

	assert.Equal(t, int64(1850), *txn.GrandTotalMinor)
	assert.Len(t, txn.LineItems, 2)
	assert.Equal(t, reconcile.ReasonTotalMismatch, txn.ItemizeReason)
}

func TestProcess_ReprocessUpdatesTheSameTransaction(t *testing.T) {
	h := newHarness(t)
	rec := h.upload(t, "receipt-clean.txt", "text/plain")

	first, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)
	second, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)

	assert.Equal(t, first.ID, second.ID)
	assert.NotEqual(t, first.OCRResultID, second.OCRResultID, "each run records a new OCR result")
	assert.Equal(t, int64(1), h.count(t, "transactions", "receipt_id = ?", rec.ID))
	assert.Equal(t, int64(2), h.count(t, "ocr_results", "receipt_id = ?", rec.ID))
	assert.Len(t, second.LineItems, 3)
	assert.Equal(t, int64(6), h.count(t, "line_items", "transaction_id = ?", first.ID), "earlier items are soft-deleted, not removed")
}

func TestProcess_ImageUploadUsesFixtureText(t *testing.T) {
	h := newHarness(t)
	rec := h.upload(t, "receipt-clean.png", "image/png")

	txn, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)
	assert.Equal(t, reconcile.StatusComplete, txn.ItemizeStatus)
}

func TestProcess_OCRFailureIsRecordedAndReturns422(t *testing.T) {
	h := newHarness(t)
	rec := h.upload(t, "holiday.png", "image/png")

	_, err := h.svc.Process(context.Background(), rec.ID)

	assert.Equal(t, apperrors.KindUnprocessable, apperrors.KindOf(err))
	assert.Equal(t, int64(1), h.count(t, "ocr_results", "receipt_id = ? AND status = ?", rec.ID, "FAILED"))
	assert.Equal(t, int64(0), h.count(t, "transactions", "receipt_id = ?", rec.ID))
}

func TestProcess_UnknownReceipt(t *testing.T) {
	_, err := newHarness(t).svc.Process(context.Background(), "rcpt_missing")
	assert.Equal(t, apperrors.KindNotFound, apperrors.KindOf(err))
}

func TestUpdateItems_MergeReconcilesAndSoftDeletes(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")

	got, err := h.svc.UpdateItems(context.Background(), txn.ID, []ItemInput{
		{Description: "Drinks", AmountMinor: 610},
		{Description: "Sandwich", AmountMinor: 890},
	})
	require.NoError(t, err)

	assert.Equal(t, reconcile.StatusComplete, got.ItemizeStatus)
	require.Len(t, got.LineItems, 2)
	assert.Equal(t, "Drinks", got.LineItems[0].Description)
	assert.Equal(t, transactionentity.SourceUser, got.LineItems[0].Source)
	assert.Equal(t, int64(3), h.count(t, "line_items", "transaction_id = ? AND deleted_at IS NOT NULL", txn.ID))
}

func TestUpdateItems_MismatchIs409AndWritesNothing(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")

	_, err := h.svc.UpdateItems(context.Background(), txn.ID, []ItemInput{{Description: "Espresso", AmountMinor: 350}})

	var domain *apperrors.Error
	require.ErrorAs(t, err, &domain)
	assert.Equal(t, apperrors.KindConflict, domain.Kind)
	assert.Equal(t, "items_total_mismatch", domain.Code)
	assert.Equal(t, map[string]string{
		"expected_total": "17.85", "items_total": "3.50", "tax_total": "2.85",
		"difference": "11.50", "reason": "total_mismatch",
	}, domain.Details)

	after, err := h.svc.Get(context.Background(), txn.ID)
	require.NoError(t, err)
	assert.Len(t, after.LineItems, 3)
	assert.Equal(t, int64(0), h.count(t, "line_items", "transaction_id = ? AND deleted_at IS NOT NULL", txn.ID))
}

func TestUpdateItems_UserCanResolveAMismatchedReceipt(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-mismatch.txt")

	got, err := h.svc.UpdateItems(context.Background(), txn.ID, []ItemInput{
		{Description: "Water", AmountMinor: 400},
		{Description: "Snacks", AmountMinor: 600},
		{Description: "Minibar", AmountMinor: 660},
	})
	require.NoError(t, err)
	assert.Equal(t, reconcile.StatusComplete, got.ItemizeStatus)
	assert.Equal(t, int64(1850), *got.GrandTotalMinor, "the total is never changed")
}

func TestUpdateItems_Validation(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")
	zero := int32(0)

	cases := map[string][]ItemInput{
		"empty list":        nil,
		"blank description": {{Description: "  ", AmountMinor: 1785}},
		"negative amount":   {{Description: "Refund", AmountMinor: -1}},
		"zero quantity":     {{Description: "Espresso", AmountMinor: 1500, Quantity: &zero}},
	}
	for name, items := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.svc.UpdateItems(context.Background(), txn.ID, items)
			assert.Equal(t, apperrors.KindInvalidArgument, apperrors.KindOf(err))
		})
	}
}

func TestItemize_ReplacesItemsOnlyFromStoredOCR(t *testing.T) {
	h := newHarness(t)
	txn := h.process(t, "receipt-clean.txt")
	_, err := h.svc.UpdateItems(context.Background(), txn.ID, []ItemInput{{Description: "Everything", AmountMinor: 1500}})
	require.NoError(t, err)

	got, err := h.svc.Itemize(context.Background(), txn.ID)
	require.NoError(t, err)

	assert.Equal(t, txn.ID, got.ID)
	assert.Equal(t, txn.OCRResultID, got.OCRResultID)
	require.Len(t, got.LineItems, 3)
	assert.Equal(t, transactionentity.SourceAuto, got.LineItems[0].Source)
	assert.Equal(t, txn.Taxes[0].ID, got.Taxes[0].ID, "taxes are untouched")
	assert.Equal(t, int64(1), h.count(t, "transactions", "receipt_id = ?", txn.ReceiptID))
}

func TestUpload_RejectsUnsupportedTypeAndEmptyFile(t *testing.T) {
	h := newHarness(t)

	_, err := h.svc.Upload(context.Background(), UploadInput{
		FileName: "tool.exe", ContentType: "application/octet-stream", Body: bytes.NewReader([]byte("MZ")),
	})
	assert.Equal(t, apperrors.KindInvalidArgument, apperrors.KindOf(err))

	_, err = h.svc.Upload(context.Background(), UploadInput{
		FileName: "empty.txt", ContentType: "text/plain", Body: bytes.NewReader(nil),
	})
	assert.Equal(t, apperrors.KindInvalidArgument, apperrors.KindOf(err))
}

func (h harness) uploadText(t *testing.T, name, text string) *receiptentity.Receipt {
	t.Helper()
	rec, err := h.svc.Upload(context.Background(), UploadInput{FileName: name, ContentType: "text/plain", Body: strings.NewReader(text)})
	require.NoError(t, err)
	return rec
}

func TestProcess_NoTotalFailsAndPatchCannotReconcile(t *testing.T) {
	h := newHarness(t)
	rec := h.uploadText(t, "no-total.txt", "MERCHANT: Kiosk\nEspresso 3.50\n")

	txn, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)
	assert.Equal(t, reconcile.StatusFailed, txn.ItemizeStatus)
	assert.Equal(t, reconcile.ReasonTotalNotFound, txn.ItemizeReason)
	assert.Nil(t, txn.GrandTotalMinor)
	assert.Len(t, txn.LineItems, 1, "items are still proposed for the user to review")

	_, err = h.svc.UpdateItems(context.Background(), txn.ID, []ItemInput{{Description: "Espresso", AmountMinor: 350}})
	var domain *apperrors.Error
	require.ErrorAs(t, err, &domain)
	assert.Equal(t, apperrors.KindConflict, domain.Kind)
	assert.Equal(t, "total_not_found", domain.Details["reason"])
}

func TestItemize_ReconcilesAgainstLatestProcess(t *testing.T) {
	h := newHarness(t)
	rec := h.upload(t, "receipt-clean.txt", "text/plain")
	first, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)
	second, err := h.svc.Process(context.Background(), rec.ID)
	require.NoError(t, err)

	got, err := h.svc.Itemize(context.Background(), first.ID)
	require.NoError(t, err)
	assert.Equal(t, second.OCRResultID, got.OCRResultID, "itemize never writes back a stale OCR result id")
	assert.Equal(t, int64(1785), *got.GrandTotalMinor)
}

// failingReceipts fails every insert, to exercise the upload cleanup path.
type failingReceipts struct{ receiptentity.Repository }

func (failingReceipts) Create(context.Context, *receiptentity.Receipt) error {
	return apperrors.New(apperrors.KindInternal, "receipt_create_failed", "boom")
}

func TestUpload_RemovesFileWhenInsertFails(t *testing.T) {
	db := testutils.NewDB(t)
	dir := t.TempDir()
	files, err := filestore.NewLocal(dir)
	require.NoError(t, err)
	svc := NewService(db, failingReceipts{}, transactionentity.NewRepo(db), ocr.NewStub(fixturesDir), files)

	_, err = svc.Upload(context.Background(), UploadInput{FileName: "a.txt", ContentType: "text/plain", Body: strings.NewReader("TOTAL 1.00")})
	require.Error(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries, "no orphan file is left behind")
}

func TestGet_UnknownTransaction(t *testing.T) {
	_, err := newHarness(t).svc.Get(context.Background(), "txn_missing")
	assert.Equal(t, apperrors.KindNotFound, apperrors.KindOf(err))
}

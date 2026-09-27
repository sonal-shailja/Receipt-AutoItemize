package receipt_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperrors "github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/entities/receipt"
	"github.com/sonalshailja/receipt-autoitemize/internal/testutils"
)

func TestRepo_ReceiptAndOCRResultRoundTrip(t *testing.T) {
	ctx := context.Background()
	repo := receipt.NewRepo(testutils.NewDB(t))

	rec := &receipt.Receipt{FileName: "a.txt", ContentType: "text/plain", FilePath: "/tmp/a", SizeBytes: 3, SHA256: "abc"}
	require.NoError(t, repo.Create(ctx, rec))
	assert.Contains(t, rec.ID, "rcpt_")

	got, err := repo.FindByID(ctx, rec.ID)
	require.NoError(t, err)
	assert.Equal(t, "a.txt", got.FileName)

	text := "TOTAL 1.00"
	result := &receipt.OCRResult{ReceiptID: rec.ID, Provider: "stub", Status: receipt.OCRStatusSucceeded, RawText: &text}
	require.NoError(t, repo.CreateOCRResult(ctx, result))
	assert.Contains(t, result.ID, "ocr_")

	gotResult, err := repo.FindOCRResult(ctx, result.ID)
	require.NoError(t, err)
	assert.Equal(t, text, *gotResult.RawText)
}

func TestRepo_NotFound(t *testing.T) {
	ctx := context.Background()
	repo := receipt.NewRepo(testutils.NewDB(t))

	_, err := repo.FindByID(ctx, "rcpt_missing")
	assert.Equal(t, apperrors.KindNotFound, apperrors.KindOf(err))

	_, err = repo.FindOCRResult(ctx, "ocr_missing")
	assert.Equal(t, apperrors.KindNotFound, apperrors.KindOf(err))
}

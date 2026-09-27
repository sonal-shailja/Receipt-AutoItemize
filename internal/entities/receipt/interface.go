package receipt

import "context"

// Repository persists receipts and their OCR results. There are no update or
// delete methods: both are insert-only evidence.
type Repository interface {
	Create(ctx context.Context, receipt *Receipt) error
	FindByID(ctx context.Context, receiptID string) (*Receipt, error)
	CreateOCRResult(ctx context.Context, result *OCRResult) error
	FindOCRResult(ctx context.Context, resultID string) (*OCRResult, error)
}

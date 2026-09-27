package receipt

import (
	"context"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	"github.com/sonalshailja/receipt-autoitemize/internal/errors"
)

// Repo is the gorm-backed Repository.
type Repo struct {
	db *database.DB
}

// NewRepo builds a Repo.
func NewRepo(db *database.DB) *Repo {
	return &Repo{db: db}
}

var _ Repository = (*Repo)(nil)

// Create persists a new receipt.
func (r *Repo) Create(ctx context.Context, receipt *Receipt) error {
	if err := r.db.Instance(ctx).Create(receipt).Error; err != nil {
		return errors.New(errors.KindInternal, "receipt_create_failed", "could not save receipt").Wrap(err)
	}
	return nil
}

// FindByID returns one receipt.
func (r *Repo) FindByID(ctx context.Context, receiptID string) (*Receipt, error) {
	var receipt Receipt
	if err := r.db.Instance(ctx).Where("id = ?", receiptID).First(&receipt).Error; err != nil {
		if database.IsNotFound(err) {
			return nil, errors.Newf(errors.KindNotFound, "receipt_not_found", "no receipt with id %s", receiptID)
		}
		return nil, errors.New(errors.KindInternal, "receipt_fetch_failed", "could not fetch receipt").Wrap(err)
	}
	return &receipt, nil
}

// CreateOCRResult persists one OCR attempt.
func (r *Repo) CreateOCRResult(ctx context.Context, result *OCRResult) error {
	if err := r.db.Instance(ctx).Create(result).Error; err != nil {
		return errors.New(errors.KindInternal, "ocr_result_create_failed", "could not save OCR result").Wrap(err)
	}
	return nil
}

// FindOCRResult returns one OCR attempt.
func (r *Repo) FindOCRResult(ctx context.Context, resultID string) (*OCRResult, error) {
	var result OCRResult
	if err := r.db.Instance(ctx).Where("id = ?", resultID).First(&result).Error; err != nil {
		if database.IsNotFound(err) {
			return nil, errors.Newf(errors.KindNotFound, "ocr_result_not_found", "no OCR result with id %s", resultID)
		}
		return nil, errors.New(errors.KindInternal, "ocr_result_fetch_failed", "could not fetch OCR result").Wrap(err)
	}
	return &result, nil
}

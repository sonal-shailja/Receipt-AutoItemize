// Package receipt holds the uploaded document and every OCR attempt made on it.
// Both are evidence of what the paper said: rows are inserted, never updated.
package receipt

import (
	"gorm.io/gorm"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	"github.com/sonalshailja/receipt-autoitemize/pkg/id"
)

const (
	// IDPrefix namespaces receipt identifiers.
	IDPrefix = "rcpt"
	// OCRResultIDPrefix namespaces OCR result identifiers.
	OCRResultIDPrefix = "ocr"
)

// OCRStatus is the outcome of one OCR attempt.
type OCRStatus string

const (
	OCRStatusSucceeded OCRStatus = "SUCCEEDED"
	OCRStatusFailed    OCRStatus = "FAILED"
)

// Receipt is one uploaded file.
type Receipt struct {
	database.Model

	FileName    string `gorm:"column:file_name"`
	ContentType string `gorm:"column:content_type"`
	FilePath    string `gorm:"column:file_path"`
	SizeBytes   int64  `gorm:"column:size_bytes"`
	SHA256      string `gorm:"column:sha256"`
}

// TableName returns the physical table.
func (*Receipt) TableName() string { return "receipts" }

// BeforeCreate assigns an id unless the caller chose one.
func (r *Receipt) BeforeCreate(tx *gorm.DB) error {
	if r.ID == "" {
		r.ID = id.New(IDPrefix)
	}
	return r.Model.BeforeCreate(tx)
}

// OCRResult is one attempt at reading a receipt. RawText is the source every
// later re-itemize parses.
type OCRResult struct {
	database.Model

	ReceiptID string    `gorm:"column:receipt_id"`
	Provider  string    `gorm:"column:provider"`
	Status    OCRStatus `gorm:"column:status"`
	RawText   *string   `gorm:"column:raw_text"`
	Error     *string   `gorm:"column:error"`
}

// TableName returns the physical table.
func (*OCRResult) TableName() string { return "ocr_results" }

// BeforeCreate assigns an id.
func (o *OCRResult) BeforeCreate(tx *gorm.DB) error {
	if o.ID == "" {
		o.ID = id.New(OCRResultIDPrefix)
	}
	return o.Model.BeforeCreate(tx)
}

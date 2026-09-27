// Package receipt implements the receipt flow: upload, OCR + extraction into a
// transaction, re-itemize from stored OCR, and the user override of line items.
package receipt

import (
	"context"
	"fmt"
	"io"
	"mime"
	"path/filepath"
	"strings"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	receiptentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/receipt"
	transactionentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/transaction"
	"github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/extraction"
	"github.com/sonalshailja/receipt-autoitemize/internal/ocr"
	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
	"github.com/sonalshailja/receipt-autoitemize/pkg/filestore"
	"github.com/sonalshailja/receipt-autoitemize/pkg/id"
	"github.com/sonalshailja/receipt-autoitemize/pkg/money"
)

// TxRunner runs fn inside one database transaction.
type TxRunner interface {
	WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Service orchestrates the pure parts (OCR, extraction, reconciliation) over
// the repositories.
type Service struct {
	db       TxRunner
	receipts receiptentity.Repository
	txns     transactionentity.Repository
	ocr      ocr.Provider
	files    filestore.Store
}

// NewService builds a Service.
func NewService(
	db TxRunner,
	receipts receiptentity.Repository,
	txns transactionentity.Repository,
	provider ocr.Provider,
	files filestore.Store,
) *Service {
	return &Service{db: db, receipts: receipts, txns: txns, ocr: provider, files: files}
}

// UploadInput is one uploaded file.
type UploadInput struct {
	FileName    string
	ContentType string
	Body        io.Reader
}

// ItemInput is one line item from the user override.
type ItemInput struct {
	Description    string
	AmountMinor    int64
	Quantity       *int32
	TaxAmountMinor *int64
}

// Upload stores the file and records the receipt.
func (s *Service) Upload(ctx context.Context, in UploadInput) (*receiptentity.Receipt, error) {
	name := filepath.Base(strings.TrimSpace(in.FileName))
	if name == "" || name == "." || name == string(filepath.Separator) {
		return nil, errors.New(errors.KindInvalidArgument, "file_name_required", "the upload needs a file name")
	}

	contentType := normalizeContentType(in.ContentType)
	if !supportedContentType(contentType) {
		return nil, errors.Newf(errors.KindInvalidArgument, "unsupported_file_type",
			"%q is not supported; upload an image, a PDF or OCR text", contentType)
	}

	receiptID := id.New(receiptentity.IDPrefix)
	stored, err := s.files.Save(ctx, receiptID+filepath.Ext(name), in.Body)
	if err != nil {
		return nil, errors.New(errors.KindInternal, "file_store_failed", "could not store the file").Wrap(err)
	}
	if stored.SizeBytes == 0 {
		_ = s.files.Remove(ctx, stored.Path)
		return nil, errors.New(errors.KindInvalidArgument, "empty_file", "the uploaded file is empty")
	}

	rec := &receiptentity.Receipt{
		Model:       database.Model{ID: receiptID},
		FileName:    name,
		ContentType: contentType,
		FilePath:    stored.Path,
		SizeBytes:   stored.SizeBytes,
		SHA256:      stored.SHA256,
	}
	if err := s.receipts.Create(ctx, rec); err != nil {
		// The file write is outside the database transaction, so undo it by
		// hand rather than leave a file no row points at.
		_ = s.files.Remove(ctx, stored.Path)
		return nil, err
	}
	return rec, nil
}

// Process runs OCR, records the result, and creates or updates the receipt's
// one transaction with its taxes and auto-itemized line items.
func (s *Service) Process(ctx context.Context, receiptID string) (*transactionentity.Transaction, error) {
	rec, err := s.receipts.FindByID(ctx, receiptID)
	if err != nil {
		return nil, err
	}

	text, ocrErr := s.ocr.Extract(ctx, ocr.File{Name: rec.FileName, ContentType: rec.ContentType, Path: rec.FilePath})
	if ocrErr != nil {
		// The failed attempt is kept as evidence; any existing transaction is
		// left exactly as it was.
		message := ocrErr.Error()
		failed := &receiptentity.OCRResult{
			ReceiptID: rec.ID, Provider: s.ocr.Name(), Status: receiptentity.OCRStatusFailed, Error: &message,
		}
		if err := s.receipts.CreateOCRResult(ctx, failed); err != nil {
			return nil, err
		}
		return nil, errors.New(errors.KindUnprocessable, "ocr_failed", "could not read text from the receipt").
			Wrap(ocrErr).
			WithDetails(map[string]string{"ocr_result_id": failed.ID})
	}

	parsed := extraction.Parse(text)

	var transactionID string
	err = s.db.WithTransaction(ctx, func(ctx context.Context) error {
		result := &receiptentity.OCRResult{
			ReceiptID: rec.ID, Provider: s.ocr.Name(), Status: receiptentity.OCRStatusSucceeded, RawText: &text,
		}
		if err := s.receipts.CreateOCRResult(ctx, result); err != nil {
			return err
		}

		txn, err := s.txns.FindByReceiptID(ctx, rec.ID)
		if err != nil {
			return err
		}
		isNew := txn == nil
		if isNew {
			txn = &transactionentity.Transaction{ReceiptID: rec.ID}
		}

		txn.OCRResultID = result.ID
		txn.Merchant = parsed.Merchant
		txn.Date = parsed.Date
		txn.Currency = parsed.Currency
		txn.SubtotalMinor = parsed.Subtotal
		txn.GrandTotalMinor = parsed.GrandTotal
		txn.DiscountMinor = parsed.Discount
		txn.Taxes = taxRows(parsed.Taxes)
		items := autoItems(parsed.Items)
		txn.SetItemizeResult(txn.Reconcile(transactionentity.AmountsOf(items)))

		if isNew {
			err = s.txns.Create(ctx, txn)
		} else {
			err = s.txns.Update(ctx, txn)
		}
		if err != nil {
			return err
		}
		if err := s.txns.ReplaceTaxes(ctx, txn.ID, txn.Taxes); err != nil {
			return err
		}
		if err := s.txns.ReplaceLineItems(ctx, txn.ID, items); err != nil {
			return err
		}

		transactionID = txn.ID
		return nil
	})
	if err != nil {
		return nil, err
	}

	return s.txns.FindByID(ctx, transactionID)
}

// Get returns a transaction with its live taxes and line items.
func (s *Service) Get(ctx context.Context, transactionID string) (*transactionentity.Transaction, error) {
	return s.txns.FindByID(ctx, transactionID)
}

// Itemize re-runs auto-itemize over the raw text of the OCR result the
// transaction was built from. Only the line items and the status change; user
// edits are replaced by design and remain as soft-deleted rows.
//
// The read happens inside the transaction: with SQLite's single connection
// that serialises it against a concurrent Process, so the items are always
// parsed from, and reconciled against, the current OCR result and taxes.
func (s *Service) Itemize(ctx context.Context, transactionID string) (*transactionentity.Transaction, error) {
	err := s.db.WithTransaction(ctx, func(ctx context.Context) error {
		txn, err := s.txns.FindByID(ctx, transactionID)
		if err != nil {
			return err
		}
		result, err := s.receipts.FindOCRResult(ctx, txn.OCRResultID)
		if err != nil {
			return err
		}
		if result.RawText == nil {
			return errors.New(errors.KindInternal, "ocr_text_missing", "the transaction's OCR result has no text")
		}

		items := autoItems(extraction.Parse(*result.RawText).Items)
		verdict := txn.Reconcile(transactionentity.AmountsOf(items))

		if err := s.txns.ReplaceLineItems(ctx, txn.ID, items); err != nil {
			return err
		}
		return s.txns.UpdateItemizeStatus(ctx, txn.ID, verdict.Status, verdict.Reason)
	})
	if err != nil {
		return nil, err
	}
	return s.txns.FindByID(ctx, transactionID)
}

// UpdateItems replaces the line items with the user's full list. The list must
// reconcile with the stored total and taxes; otherwise nothing is written and
// the caller gets a conflict carrying the numbers. The read is inside the
// transaction for the same reason as in Itemize.
func (s *Service) UpdateItems(ctx context.Context, transactionID string, inputs []ItemInput) (*transactionentity.Transaction, error) {
	items, err := userItems(inputs)
	if err != nil {
		return nil, err
	}

	err = s.db.WithTransaction(ctx, func(ctx context.Context) error {
		txn, err := s.txns.FindByID(ctx, transactionID)
		if err != nil {
			return err
		}
		verdict := txn.Reconcile(transactionentity.AmountsOf(items))
		if !verdict.Reconciled() {
			return mismatchError(verdict)
		}
		if err := s.txns.ReplaceLineItems(ctx, txn.ID, items); err != nil {
			return err
		}
		return s.txns.UpdateItemizeStatus(ctx, txn.ID, verdict.Status, verdict.Reason)
	})
	if err != nil {
		return nil, err
	}
	return s.txns.FindByID(ctx, transactionID)
}

func userItems(inputs []ItemInput) ([]transactionentity.LineItem, error) {
	if len(inputs) == 0 {
		return nil, errors.New(errors.KindInvalidArgument, "items_required", "send the full list of line items")
	}

	items := make([]transactionentity.LineItem, 0, len(inputs))
	for i, in := range inputs {
		description := strings.TrimSpace(in.Description)
		switch {
		case description == "":
			return nil, errors.Newf(errors.KindInvalidArgument, "item_description_required", "items[%d].description is required", i)
		case in.AmountMinor < 0:
			return nil, errors.Newf(errors.KindInvalidArgument, "item_amount_negative", "items[%d].amount must not be negative", i)
		case in.Quantity != nil && *in.Quantity <= 0:
			return nil, errors.Newf(errors.KindInvalidArgument, "item_quantity_invalid", "items[%d].quantity must be positive", i)
		}
		items = append(items, transactionentity.LineItem{
			Description:    description,
			AmountMinor:    in.AmountMinor,
			Quantity:       in.Quantity,
			TaxAmountMinor: in.TaxAmountMinor,
			Source:         transactionentity.SourceUser,
		})
	}
	return items, nil
}

func mismatchError(result reconcile.Result) error {
	message := "the transaction has no grand total to reconcile line items against"
	if result.Reason != reconcile.ReasonTotalNotFound {
		message = fmt.Sprintf("line items plus taxes minus discount total %s but the receipt total is %s",
			money.Format(result.ItemsTotal+result.TaxTotal-result.DiscountTotal), money.Format(result.ExpectedTotal))
	}
	return errors.New(errors.KindConflict, "items_total_mismatch", message).WithDetails(map[string]string{
		"expected_total": money.Format(result.ExpectedTotal),
		"items_total":    money.Format(result.ItemsTotal),
		"tax_total":      money.Format(result.TaxTotal),
		"discount_total": money.Format(result.DiscountTotal),
		"difference":     money.Format(result.Difference),
		"reason":         string(result.Reason),
	})
}

func taxRows(taxes []extraction.Tax) []transactionentity.Tax {
	rows := make([]transactionentity.Tax, 0, len(taxes))
	for _, tax := range taxes {
		rows = append(rows, transactionentity.Tax{
			Name: tax.Name, RateBps: tax.RateBps, AmountMinor: tax.Amount, Inclusive: tax.Inclusive,
		})
	}
	return rows
}

func autoItems(items []extraction.Item) []transactionentity.LineItem {
	rows := make([]transactionentity.LineItem, 0, len(items))
	for _, item := range items {
		rows = append(rows, transactionentity.LineItem{
			Description: item.Description, AmountMinor: item.Amount, Source: transactionentity.SourceAuto,
		})
	}
	return rows
}

func normalizeContentType(value string) string {
	mediaType, _, err := mime.ParseMediaType(value)
	if err != nil {
		return strings.ToLower(strings.TrimSpace(value))
	}
	return mediaType
}

func supportedContentType(contentType string) bool {
	return contentType == "text/plain" || contentType == "application/pdf" || strings.HasPrefix(contentType, "image/")
}

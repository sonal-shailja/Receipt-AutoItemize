package transaction

import (
	"context"

	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
)

// Repository persists transactions with their taxes and line items.
type Repository interface {
	// Create inserts the header only; use ReplaceTaxes and ReplaceLineItems
	// for the children.
	Create(ctx context.Context, txn *Transaction) error
	// Update writes every header column. Only Process uses it: it owns the
	// header fields extracted from OCR.
	Update(ctx context.Context, txn *Transaction) error
	// UpdateItemizeStatus writes the status and reason only, so itemize and the
	// user override can never write back a stale total or OCR result id.
	UpdateItemizeStatus(ctx context.Context, transactionID string, status reconcile.Status, reason reconcile.Reason) error
	// FindByID returns the header with its live taxes and items, by position.
	FindByID(ctx context.Context, transactionID string) (*Transaction, error)
	// FindByReceiptID returns the receipt's transaction, or nil if it has none.
	FindByReceiptID(ctx context.Context, receiptID string) (*Transaction, error)
	// ReplaceTaxes soft-deletes the live taxes and inserts taxes in order.
	ReplaceTaxes(ctx context.Context, transactionID string, taxes []Tax) error
	// ReplaceLineItems soft-deletes the live items and inserts items in order.
	ReplaceLineItems(ctx context.Context, transactionID string, items []LineItem) error
}

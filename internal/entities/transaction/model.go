// Package transaction is the expense record interpreted from a receipt: the
// header, its taxes and its line items. Taxes and items are replaced as a set
// and never hard-deleted, so every earlier version stays queryable.
package transaction

import (
	"gorm.io/gorm"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
	"github.com/sonalshailja/receipt-autoitemize/pkg/id"
)

const (
	IDPrefix         = "txn"
	TaxIDPrefix      = "tax"
	LineItemIDPrefix = "li"
)

// Source records who produced a line item.
type Source string

const (
	// SourceAuto is an item proposed by auto-itemize.
	SourceAuto Source = "AUTO"
	// SourceUser is an item set by the user override.
	SourceUser Source = "USER"
)

// Transaction is the header. Taxes and LineItems hold live rows only.
type Transaction struct {
	database.Model

	ReceiptID       string           `gorm:"column:receipt_id"`
	OCRResultID     string           `gorm:"column:ocr_result_id"`
	Merchant        string           `gorm:"column:merchant"`
	Date            string           `gorm:"column:txn_date"`
	Currency        string           `gorm:"column:currency"`
	SubtotalMinor   *int64           `gorm:"column:subtotal_minor"`
	GrandTotalMinor *int64           `gorm:"column:grand_total_minor"`
	ItemizeStatus   reconcile.Status `gorm:"column:itemize_status"`
	ItemizeReason   reconcile.Reason `gorm:"column:itemize_reason"`

	Taxes     []Tax      `gorm:"foreignKey:TransactionID"`
	LineItems []LineItem `gorm:"foreignKey:TransactionID"`
}

// TableName returns the physical table.
func (*Transaction) TableName() string { return "transactions" }

// EntityName returns the logical name used in error messages.
func (*Transaction) EntityName() string { return "transaction" }

// BeforeCreate assigns an id.
func (t *Transaction) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = id.New(IDPrefix)
	}
	return t.Model.BeforeCreate(tx)
}

// Reconcile checks item amounts against this transaction's stored total and
// taxes.
func (t *Transaction) Reconcile(itemAmounts []int64) reconcile.Result {
	taxes := make([]reconcile.Tax, 0, len(t.Taxes))
	for _, tax := range t.Taxes {
		taxes = append(taxes, reconcile.Tax{Amount: tax.AmountMinor, Inclusive: tax.Inclusive})
	}
	return reconcile.Check(reconcile.Input{
		GrandTotal:  t.GrandTotalMinor,
		Taxes:       taxes,
		ItemAmounts: itemAmounts,
	})
}

// SetItemizeResult records a reconciliation verdict on the header.
func (t *Transaction) SetItemizeResult(result reconcile.Result) {
	t.ItemizeStatus = result.Status
	t.ItemizeReason = result.Reason
}

// Tax is one tax line as printed on the receipt.
type Tax struct {
	database.SoftDeletableModel

	TransactionID string `gorm:"column:transaction_id"`
	Position      int    `gorm:"column:position"`
	Name          string `gorm:"column:name"`
	RateBps       int64  `gorm:"column:rate_bps"`
	AmountMinor   int64  `gorm:"column:amount_minor"`
	Inclusive     bool   `gorm:"column:inclusive"`
}

// TableName returns the physical table.
func (*Tax) TableName() string { return "taxes" }

// EntityName returns the logical name used in error messages.
func (*Tax) EntityName() string { return "tax" }

// BeforeCreate assigns an id.
func (t *Tax) BeforeCreate(tx *gorm.DB) error {
	if t.ID == "" {
		t.ID = id.New(TaxIDPrefix)
	}
	return t.SoftDeletableModel.BeforeCreate(tx)
}

// LineItem is one item of the transaction.
type LineItem struct {
	database.SoftDeletableModel

	TransactionID string `gorm:"column:transaction_id"`
	Position      int    `gorm:"column:position"`
	Description   string `gorm:"column:description"`
	AmountMinor   int64  `gorm:"column:amount_minor"`
	Quantity      *int32 `gorm:"column:quantity"`
	// TaxAmountMinor is set only when supplied; it is never derived from the
	// receipt-level tax.
	TaxAmountMinor *int64 `gorm:"column:tax_amount_minor"`
	Source         Source `gorm:"column:source"`
}

// TableName returns the physical table.
func (*LineItem) TableName() string { return "line_items" }

// EntityName returns the logical name used in error messages.
func (*LineItem) EntityName() string { return "line item" }

// BeforeCreate assigns an id.
func (l *LineItem) BeforeCreate(tx *gorm.DB) error {
	if l.ID == "" {
		l.ID = id.New(LineItemIDPrefix)
	}
	return l.SoftDeletableModel.BeforeCreate(tx)
}

// AmountsOf returns the amounts of items, in order.
func AmountsOf(items []LineItem) []int64 {
	amounts := make([]int64, 0, len(items))
	for _, item := range items {
		amounts = append(amounts, item.AmountMinor)
	}
	return amounts
}

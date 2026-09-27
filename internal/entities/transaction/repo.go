package transaction

import (
	"context"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	"github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/reconcile"
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

// liveByPosition scopes a preload to rows that are not soft-deleted.
func liveByPosition(db *gorm.DB) *gorm.DB {
	return db.Where("deleted_at IS NULL").Order("position")
}

// Create inserts the header.
func (r *Repo) Create(ctx context.Context, txn *Transaction) error {
	if err := r.db.Instance(ctx).Omit(clause.Associations).Create(txn).Error; err != nil {
		return errors.New(errors.KindInternal, "transaction_create_failed", "could not create transaction").Wrap(err)
	}
	return nil
}

// Update writes the header columns.
func (r *Repo) Update(ctx context.Context, txn *Transaction) error {
	if err := r.db.Instance(ctx).Omit(clause.Associations).Save(txn).Error; err != nil {
		return errors.New(errors.KindInternal, "transaction_update_failed", "could not update transaction").Wrap(err)
	}
	return nil
}

// UpdateItemizeStatus writes the itemize status and reason only.
func (r *Repo) UpdateItemizeStatus(ctx context.Context, transactionID string, status reconcile.Status, reason reconcile.Reason) error {
	err := r.db.Instance(ctx).Model(&Transaction{}).Where("id = ?", transactionID).Updates(map[string]any{
		"itemize_status": status,
		"itemize_reason": reason,
		"updated_at":     time.Now().UTC().UnixMilli(),
	}).Error
	if err != nil {
		return errors.New(errors.KindInternal, "transaction_update_failed", "could not update itemize status").Wrap(err)
	}
	return nil
}

// FindByID returns the header with live taxes and items.
func (r *Repo) FindByID(ctx context.Context, transactionID string) (*Transaction, error) {
	var txn Transaction
	err := r.db.Instance(ctx).
		Preload("Taxes", liveByPosition).
		Preload("LineItems", liveByPosition).
		Where("id = ?", transactionID).
		First(&txn).Error
	if err != nil {
		if database.IsNotFound(err) {
			return nil, errors.Newf(errors.KindNotFound, "transaction_not_found", "no transaction with id %s", transactionID)
		}
		return nil, errors.New(errors.KindInternal, "transaction_fetch_failed", "could not fetch transaction").Wrap(err)
	}
	return &txn, nil
}

// FindByReceiptID returns the receipt's transaction header, or nil.
func (r *Repo) FindByReceiptID(ctx context.Context, receiptID string) (*Transaction, error) {
	var rows []Transaction
	if err := r.db.Instance(ctx).Where("receipt_id = ?", receiptID).Limit(1).Find(&rows).Error; err != nil {
		return nil, errors.New(errors.KindInternal, "transaction_fetch_failed", "could not fetch transaction").Wrap(err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return &rows[0], nil
}

// ReplaceTaxes soft-deletes live taxes and inserts the new set.
func (r *Repo) ReplaceTaxes(ctx context.Context, transactionID string, taxes []Tax) error {
	return r.db.WithTransaction(ctx, func(ctx context.Context) error {
		if err := r.softDeleteLive(ctx, &Tax{}, transactionID); err != nil {
			return err
		}
		if len(taxes) == 0 {
			return nil
		}
		for i := range taxes {
			taxes[i].TransactionID = transactionID
			taxes[i].Position = i + 1
		}
		if err := r.db.Instance(ctx).Create(&taxes).Error; err != nil {
			return errors.New(errors.KindInternal, "taxes_create_failed", "could not save taxes").Wrap(err)
		}
		return nil
	})
}

// ReplaceLineItems soft-deletes live items and inserts the new set.
func (r *Repo) ReplaceLineItems(ctx context.Context, transactionID string, items []LineItem) error {
	return r.db.WithTransaction(ctx, func(ctx context.Context) error {
		if err := r.softDeleteLive(ctx, &LineItem{}, transactionID); err != nil {
			return err
		}
		if len(items) == 0 {
			return nil
		}
		for i := range items {
			items[i].TransactionID = transactionID
			items[i].Position = i + 1
		}
		if err := r.db.Instance(ctx).Create(&items).Error; err != nil {
			return errors.New(errors.KindInternal, "line_items_create_failed", "could not save line items").Wrap(err)
		}
		return nil
	})
}

func (r *Repo) softDeleteLive(ctx context.Context, model any, transactionID string) error {
	now := time.Now().UTC().UnixMilli()
	err := r.db.Instance(ctx).Model(model).
		Where("transaction_id = ? AND deleted_at IS NULL", transactionID).
		Updates(map[string]any{"deleted_at": now, "updated_at": now}).Error
	if err != nil {
		return errors.New(errors.KindInternal, "soft_delete_failed", "could not replace rows").Wrap(err)
	}
	return nil
}

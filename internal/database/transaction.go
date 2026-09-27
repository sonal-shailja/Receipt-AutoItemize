package database

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

type txKey struct{}

func transactionFromContext(ctx context.Context) (*gorm.DB, bool) {
	tx, ok := ctx.Value(txKey{}).(*gorm.DB)
	return tx, ok && tx != nil
}

// InTransaction reports whether ctx carries an open transaction.
func InTransaction(ctx context.Context) bool {
	_, ok := transactionFromContext(ctx)
	return ok
}

// WithTransaction runs fn inside a transaction, committing when it returns nil
// and rolling back otherwise. The transaction rides on the context passed to
// fn, so repository calls made with it join automatically; a nested call runs
// on the existing transaction instead of opening a second.
func (d *DB) WithTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if InTransaction(ctx) {
		return fn(ctx)
	}
	return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(context.WithValue(ctx, txKey{}, tx))
	})
}

// IsNotFound reports whether err is gorm's no-rows error.
func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

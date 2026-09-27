package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/sonalshailja/receipt-autoitemize/internal/testutils"
)

func TestMigrate_CreatesTablesAndIsIdempotent(t *testing.T) {
	db := testutils.NewDB(t)
	ctx := context.Background()

	require.NoError(t, db.Migrate(ctx), "second run is a no-op")

	for _, table := range []string{"receipts", "ocr_results", "transactions", "taxes", "line_items"} {
		var n int64
		require.NoError(t, db.Instance(ctx).Raw(
			"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", table).Scan(&n).Error)
		assert.Equal(t, int64(1), n, table)
	}
}

func TestWithTransaction_RollsBackOnError(t *testing.T) {
	db := testutils.NewDB(t)
	ctx := context.Background()

	err := db.WithTransaction(ctx, func(ctx context.Context) error {
		require.NoError(t, db.Instance(ctx).Exec(
			`INSERT INTO receipts VALUES ('rcpt_x','a.txt','text/plain','/tmp/a',1,'h',1,1)`).Error)
		return errors.New("abort")
	})
	require.Error(t, err)

	var n int64
	require.NoError(t, db.Instance(ctx).Raw("SELECT count(*) FROM receipts").Scan(&n).Error)
	assert.Zero(t, n)
}

func TestForeignKeysAreEnforced(t *testing.T) {
	db := testutils.NewDB(t)
	err := db.Instance(context.Background()).Exec(
		`INSERT INTO ocr_results VALUES ('ocr_x','rcpt_missing','stub','SUCCEEDED',NULL,NULL,1,1)`).Error
	assert.Error(t, err)
}

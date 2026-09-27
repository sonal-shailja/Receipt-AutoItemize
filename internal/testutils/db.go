// Package testutils builds real, migrated SQLite databases for tests. Each test
// gets its own file under t.TempDir(), so tests never share state.
package testutils

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/sonalshailja/receipt-autoitemize/internal/config"
	"github.com/sonalshailja/receipt-autoitemize/internal/database"
)

// NewDB returns a migrated database that is closed when the test ends.
func NewDB(t testing.TB) *database.DB {
	t.Helper()
	ctx := context.Background()

	db, err := database.Connect(ctx, config.DatabaseConfig{Path: filepath.Join(t.TempDir(), "test.db")})
	require.NoError(t, err)
	require.NoError(t, db.Migrate(ctx))
	t.Cleanup(func() { _ = db.Close() })

	return db
}

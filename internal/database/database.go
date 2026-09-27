// Package database owns the SQLite connection, the embedded schema migrations
// and the transaction plumbing.
package database

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/pressly/goose/v3"
	goosedb "github.com/pressly/goose/v3/database"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/sonalshailja/receipt-autoitemize/internal/config"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// DB wraps a gorm connection. Queries go through Instance(ctx) so an active
// transaction on the context is picked up automatically.
type DB struct {
	db *gorm.DB
}

// Connect opens (creating if needed) the SQLite file and verifies it.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*DB, error) {
	if dir := filepath.Dir(cfg.Path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return nil, fmt.Errorf("database: creating %s: %w", dir, err)
		}
	}

	level := gormlogger.Warn
	if cfg.Debug {
		level = gormlogger.Info
	}

	gormDB, err := gorm.Open(sqlite.Open(dsn(cfg.Path)), &gorm.Config{
		Logger: gormlogger.New(log.New(os.Stdout, "", log.LstdFlags), gormlogger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  level,
			IgnoreRecordNotFoundError: true,
		}),
		SkipDefaultTransaction: true,
		NowFunc:                func() time.Time { return time.Now().UTC() },
	})
	if err != nil {
		return nil, fmt.Errorf("database: opening %s: %w", cfg.Path, err)
	}

	pool, err := gormDB.DB()
	if err != nil {
		return nil, fmt.Errorf("database: retrieving pool: %w", err)
	}
	// SQLite allows one writer at a time. A single connection serialises every
	// statement in-process, so concurrent requests queue instead of failing
	// with SQLITE_BUSY. Everything inside a transaction goes through
	// Instance(ctx), so it never asks for a second connection.
	pool.SetMaxOpenConns(1)

	if err := pool.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}

	slog.Info("database connected", "path", cfg.Path)
	return &DB{db: gormDB}, nil
}

// dsn turns on foreign keys, which SQLite leaves off by default, and WAL so
// readers do not block the writer.
func dsn(path string) string {
	return path + "?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)"
}

// Migrate applies every embedded migration that has not run yet.
func (d *DB) Migrate(ctx context.Context) error {
	pool, err := d.db.DB()
	if err != nil {
		return fmt.Errorf("database: retrieving pool: %w", err)
	}
	fsys, err := fs.Sub(migrationFiles, "migrations")
	if err != nil {
		return fmt.Errorf("database: opening migrations: %w", err)
	}
	provider, err := goose.NewProvider(goosedb.DialectSQLite3, pool, fsys)
	if err != nil {
		return fmt.Errorf("database: building migrator: %w", err)
	}
	if _, err := provider.Up(ctx); err != nil {
		return fmt.Errorf("database: applying migrations: %w", err)
	}
	return nil
}

// Instance returns the transaction on ctx if there is one, and the pool
// otherwise.
func (d *DB) Instance(ctx context.Context) *gorm.DB {
	if tx, ok := transactionFromContext(ctx); ok {
		return tx.WithContext(ctx)
	}
	return d.db.WithContext(ctx)
}

// Ping reports whether the database is reachable.
func (d *DB) Ping(ctx context.Context) error {
	pool, err := d.db.DB()
	if err != nil {
		return err
	}
	return pool.PingContext(ctx)
}

// Close releases the connection.
func (d *DB) Close() error {
	pool, err := d.db.DB()
	if err != nil {
		return err
	}
	return pool.Close()
}

// SQL exposes the underlying *sql.DB.
func (d *DB) SQL() (*sql.DB, error) {
	return d.db.DB()
}

// Package boot wires the process together: configuration, database, services
// and the two listeners. All dependency construction happens here.
package boot

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sonalshailja/receipt-autoitemize/internal/config"
	"github.com/sonalshailja/receipt-autoitemize/internal/database"
	receiptentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/receipt"
	transactionentity "github.com/sonalshailja/receipt-autoitemize/internal/entities/transaction"
	"github.com/sonalshailja/receipt-autoitemize/internal/logger"
	"github.com/sonalshailja/receipt-autoitemize/internal/ocr"
	healthsvc "github.com/sonalshailja/receipt-autoitemize/internal/services/health"
	receiptsvc "github.com/sonalshailja/receipt-autoitemize/internal/services/receipt"
	"github.com/sonalshailja/receipt-autoitemize/pkg/filestore"
)

// Dependencies is the fully constructed object graph.
type Dependencies struct {
	Config *config.Config
	Logger *slog.Logger
	DB     *database.DB

	ReceiptService *receiptsvc.Service

	// gRPC handlers, one per service.
	Health  *healthsvc.Handler
	Receipt *receiptsvc.Handler
}

// New builds every dependency in order, migrating the database on the way.
func New(ctx context.Context, env string) (*Dependencies, error) {
	cfg, err := config.Get(env)
	if err != nil {
		return nil, fmt.Errorf("boot: loading config: %w", err)
	}

	appLogger := logger.New(cfg.Log.Level, cfg.Log.Format).With("service", cfg.App.Name, "env", cfg.App.Env)
	logger.SetDefault(appLogger)

	db, err := database.Connect(ctx, cfg.Database)
	if err != nil {
		return nil, fmt.Errorf("boot: connecting to database: %w", err)
	}
	deps := &Dependencies{Config: cfg, Logger: appLogger, DB: db}

	if err := db.Migrate(ctx); err != nil {
		_ = deps.Close()
		return nil, fmt.Errorf("boot: migrating database: %w", err)
	}

	provider, err := newOCRProvider(cfg.OCR)
	if err != nil {
		_ = deps.Close()
		return nil, err
	}

	files, err := filestore.NewLocal(cfg.Storage.UploadDir)
	if err != nil {
		_ = deps.Close()
		return nil, fmt.Errorf("boot: preparing upload dir: %w", err)
	}

	deps.ReceiptService = receiptsvc.NewService(
		db, receiptentity.NewRepo(db), transactionentity.NewRepo(db), provider, files,
	)
	deps.Health = healthsvc.NewHandler(db)
	deps.Receipt = receiptsvc.NewHandler(deps.ReceiptService)

	appLogger.Info("dependencies ready", "ocr_provider", provider.Name(), "upload_dir", cfg.Storage.UploadDir)
	return deps, nil
}

func newOCRProvider(cfg config.OCRConfig) (ocr.Provider, error) {
	switch cfg.Provider {
	case ocr.StubName, "":
		return ocr.NewStub(cfg.FixturesDir), nil
	default:
		return nil, fmt.Errorf("boot: unknown ocr provider %q", cfg.Provider)
	}
}

// Close releases everything New acquired.
func (d *Dependencies) Close() error {
	if d.DB != nil {
		return d.DB.Close()
	}
	return nil
}

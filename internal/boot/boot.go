package boot

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
)

// EnvVar names the environment variable selecting the config file.
const EnvVar = "APP_ENV"

// Env returns the active environment, defaulting to dev.
func Env() string {
	if env := os.Getenv(EnvVar); env != "" {
		return env
	}
	return "dev"
}

// Initialize builds the dependency graph, starts the servers, and blocks until
// SIGINT or SIGTERM triggers a graceful shutdown.
func Initialize(ctx context.Context) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := New(ctx, Env())
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := deps.Close(); closeErr != nil {
			deps.Logger.Error("failed to release dependencies", "error", closeErr)
		}
	}()

	server, err := NewServer(ctx, deps)
	if err != nil {
		return fmt.Errorf("boot: constructing server: %w", err)
	}

	deps.Logger.Info("service starting",
		"grpc", deps.Config.Server.GRPC,
		"http", deps.Config.Server.HTTP,
	)

	if err := server.Run(ctx); err != nil {
		return fmt.Errorf("boot: running server: %w", err)
	}

	deps.Logger.Info("service stopped")
	return nil
}

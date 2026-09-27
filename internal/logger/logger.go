// Package logger provides a structured logger built on log/slog. The logger is
// carried on the request context so every layer logs the same request_id.
package logger

import (
	"context"
	"log/slog"
	"os"
	"strings"
)

// ctxKey is unexported so nothing outside this package can collide with it.
type ctxKey struct{}

var fallback = slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

// New builds a logger for the given level ("debug", "info", "warn", "error")
// and format ("json" or "text").
func New(level, format string) *slog.Logger {
	opts := &slog.HandlerOptions{Level: parseLevel(level)}

	var handler slog.Handler
	if strings.EqualFold(format, "json") {
		handler = slog.NewJSONHandler(os.Stdout, opts)
	} else {
		handler = slog.NewTextHandler(os.Stdout, opts)
	}

	return slog.New(handler)
}

// WithContext stores a logger on the context.
func WithContext(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, ctxKey{}, l)
}

// FromContext returns the logger carried by ctx, or a usable default. It never
// returns nil, so callers can log unconditionally.
func FromContext(ctx context.Context) *slog.Logger {
	if l, ok := ctx.Value(ctxKey{}).(*slog.Logger); ok && l != nil {
		return l
	}
	return fallback
}

// SetDefault installs l as the package fallback and as slog's global default.
func SetDefault(l *slog.Logger) {
	fallback = l
	slog.SetDefault(l)
}

func parseLevel(level string) slog.Level {
	switch strings.ToLower(level) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

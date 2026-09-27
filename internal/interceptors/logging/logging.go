// Package logging logs one line per RPC and puts a request-scoped logger on
// the context.
package logging

import (
	"context"
	"log/slog"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/sonalshailja/receipt-autoitemize/internal/errors"
	"github.com/sonalshailja/receipt-autoitemize/internal/interceptors/requestid"
	"github.com/sonalshailja/receipt-autoitemize/internal/logger"
)

// UnaryServerInterceptor logs the outcome and duration of each call, and
// installs a logger pre-tagged with the request id so that downstream code logs
// correlated lines without passing a logger around.
func UnaryServerInterceptor(base *slog.Logger) grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		start := time.Now()

		requestLogger := base.With(
			"request_id", requestid.FromContext(ctx),
			"method", info.FullMethod,
		)
		ctx = logger.WithContext(ctx, requestLogger)

		resp, err := handler(ctx, req)

		attrs := []any{
			"duration_ms", time.Since(start).Milliseconds(),
			"code", status.Code(err).String(),
		}

		switch {
		case err == nil:
			requestLogger.InfoContext(ctx, "rpc completed", attrs...)

		case errors.KindOf(err) == errors.KindInternal:
			// Internal failures carry the full cause, because this line is the
			// only place the detail survives — the caller gets a generic
			// message so no stack trace or SQL fragment leaks out.
			requestLogger.ErrorContext(ctx, "rpc failed", append(attrs, "error", err.Error())...)

		default:
			// Expected outcomes: a validation failure or a booking conflict is
			// the system working, not breaking. Logging these at error level is
			// how an alert channel becomes noise everyone learns to ignore.
			requestLogger.WarnContext(ctx, "rpc rejected", append(attrs, "error", err.Error())...)
		}

		return resp, err
	}
}

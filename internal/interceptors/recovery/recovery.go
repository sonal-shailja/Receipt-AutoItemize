// Package recovery converts a panic in a handler into an Internal error,
// rather than letting it kill the process.
package recovery

import (
	"context"
	"runtime/debug"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/sonalshailja/receipt-autoitemize/internal/logger"
)

// UnaryServerInterceptor recovers panics and logs the stack. The stack is
// never returned to the caller.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		info *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (resp any, err error) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.FromContext(ctx).ErrorContext(ctx, "panic recovered",
					"method", info.FullMethod,
					"panic", recovered,
					"stack", string(debug.Stack()),
				)
				// Named return values are what let a deferred function replace
				// the handler's outcome after the panic unwound it.
				resp = nil
				err = status.Error(codes.Internal, "internal error")
			}
		}()

		return handler(ctx, req)
	}
}

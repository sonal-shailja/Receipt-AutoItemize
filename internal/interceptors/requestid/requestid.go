// Package requestid attaches a correlation id to every request, stamps it on
// log lines, and echoes it back in a response header.
package requestid

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/sonalshailja/receipt-autoitemize/pkg/id"
)

// HeaderKey is the metadata and HTTP header carrying the id.
const HeaderKey = "x-request-id"

type ctxKey struct{}

// FromContext returns the request id on ctx, or "" if absent.
func FromContext(ctx context.Context) string {
	if value, ok := ctx.Value(ctxKey{}).(string); ok {
		return value
	}
	return ""
}

// WithContext stores a request id on ctx.
func WithContext(ctx context.Context, requestID string) context.Context {
	return context.WithValue(ctx, ctxKey{}, requestID)
}

// UnaryServerInterceptor reuses an inbound x-request-id when present so a
// trace spans service boundaries, and generates one otherwise.
func UnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(
		ctx context.Context,
		req any,
		_ *grpc.UnaryServerInfo,
		handler grpc.UnaryHandler,
	) (any, error) {
		requestID := fromIncomingMetadata(ctx)
		if requestID == "" {
			requestID = id.New("req")
		}

		ctx = WithContext(ctx, requestID)

		// Echo it back so a client can log the id of a call that failed.
		_ = grpc.SetHeader(ctx, metadata.Pairs(HeaderKey, requestID))

		return handler(ctx, req)
	}
}

func fromIncomingMetadata(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	if values := md.Get(HeaderKey); len(values) > 0 {
		return values[0]
	}
	return ""
}

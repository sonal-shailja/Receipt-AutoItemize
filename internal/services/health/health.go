// Package health implements the health probe.
package health

import (
	"context"

	healthv1 "github.com/sonalshailja/receipt-autoitemize/rpc/common/health/v1"
)

const (
	statusServing    = "SERVING"
	statusNotServing = "NOT_SERVING"
)

// Pinger is a dependency that can be probed.
type Pinger interface {
	Ping(ctx context.Context) error
}

// Handler serves the health rpc.
type Handler struct {
	healthv1.UnimplementedHealthServiceServer

	db Pinger
}

// NewHandler builds a Handler.
func NewHandler(db Pinger) *Handler {
	return &Handler{db: db}
}

// Check reports whether the database answers. A failed probe is a
// NOT_SERVING status, not an error, so the caller sees which dependency is down.
func (h *Handler) Check(ctx context.Context, _ *healthv1.CheckRequest) (*healthv1.CheckResponse, error) {
	check := &healthv1.DependencyCheck{Name: "database", Status: statusServing}
	if err := h.db.Ping(ctx); err != nil {
		check.Status = statusNotServing
		check.Error = err.Error()
	}
	return &healthv1.CheckResponse{Status: check.GetStatus(), Dependencies: []*healthv1.DependencyCheck{check}}, nil
}

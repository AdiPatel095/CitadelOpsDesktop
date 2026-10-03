package Intent

import (
	"context"
	"time"
)

// RejectionRefresh identifies the committed response to the refresh scheduled
// after a rejection. A successful response without fresh target data is unknown.
type RejectionRefresh struct {
	StartedAt            time.Time
	ObservedAt           time.Time
	SessionGeneration    uint64
	ConnectionGeneration uint64
}

type rejectionRefreshContextKey struct{}

func RejectionRefreshFromContext(ctx context.Context) (RejectionRefresh, bool) {
	refresh, ok := ctx.Value(rejectionRefreshContextKey{}).(RejectionRefresh)
	return refresh, ok
}

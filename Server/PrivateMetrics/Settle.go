package PrivateMetrics

import (
	"sync"
	"time"
)

// Settle is the window after something that a handover (or a stale dashboard)
// waits on: a new placement epoch, a session transition, or a configuration
// apply. While it is active both publishers upload every evaluation without
// content gating, because the backend's handover readiness needs an actual
// checkpoint upload and an actual metrics upload, each no older than two
// minutes, at the same moment. One Settle is shared by the runtime's metrics and
// checkpoint publishers, so a trigger seen by either restarts the window for
// both. It is safe for concurrent use.
type Settle struct {
	mu     sync.Mutex
	window time.Duration
	until  time.Time
}

// NewSettle returns a settle window of the given length (defaultSettleWindow
// when window is not positive).
func NewSettle(window time.Duration) *Settle {
	if window <= 0 {
		window = defaultSettleWindow
	}
	return &Settle{window: window}
}

// Restart begins (or extends) the window at the given time.
func (settle *Settle) Restart(now time.Time) {
	if settle == nil {
		return
	}
	settle.mu.Lock()
	if until := now.Add(settle.window); until.After(settle.until) {
		settle.until = until
	}
	settle.mu.Unlock()
}

// Active reports whether the window is open at the given time.
func (settle *Settle) Active(now time.Time) bool {
	if settle == nil {
		return false
	}
	settle.mu.Lock()
	defer settle.mu.Unlock()
	return now.Before(settle.until)
}

package PrivateMetrics

import (
	"sync"
	"time"

	"CitadelDesktop/Server/State"
)

type settleKey struct{ epoch, connection, generation uint64 }

func settleTrigger(epoch uint64, view State.GameState) settleKey {
	if epoch == 0 || view.Session.LoginFailure != nil || sampleGate(view) != nil {
		return settleKey{}
	}
	return settleKey{epoch, view.Session.ConnectionGeneration, view.Session.Generation}
}

// Settle is the three-minute window opened when a placed runtime first passes
// the sample gate. Both publishers upload every evaluation while it is active.
// A successful checkpoint also requests one sample so both publications stay
// fresh together even after the window. It is safe for concurrent use.
type Settle struct {
	mu              sync.Mutex
	window          time.Duration
	until           time.Time
	sampleRequested bool
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

// RequestSample pairs the next ready metrics evaluation with a checkpoint upload.
func (settle *Settle) RequestSample() {
	if settle == nil {
		return
	}
	settle.mu.Lock()
	settle.sampleRequested = true
	settle.mu.Unlock()
}

// TakeSampleRequest consumes a pending request once.
func (settle *Settle) TakeSampleRequest() bool {
	if settle == nil {
		return false
	}
	settle.mu.Lock()
	defer settle.mu.Unlock()
	requested := settle.sampleRequested
	settle.sampleRequested = false
	return requested
}

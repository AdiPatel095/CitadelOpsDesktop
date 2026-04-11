package GameParser

import (
	"sync/atomic"
	"time"
)

// lastJAAProcessedNs is bumped after each inbound **jaa** frame is fully handled in MessageRouter
// (focus + buildings + troops). Callers that SendTroopFocus can wait for a newer value to observe fresh GameState.
var lastJAAProcessedNs int64

func markJAAProcessed() {
	atomic.StoreInt64(&lastJAAProcessedNs, time.Now().UnixNano())
}

func awaitNextJAAAfter(prevNs int64, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if atomic.LoadInt64(&lastJAAProcessedNs) > prevNs {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}

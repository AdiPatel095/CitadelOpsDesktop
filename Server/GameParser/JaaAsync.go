package GameParser

import (
	"sync/atomic"
	"time"
)

// lastJAAProcessedNs is bumped after each inbound **jaa** frame is fully handled in MessageRouter
// (focus + buildings + troops). Callers that SendCastleFocus can wait for a newer value to observe fresh GameState.
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

// lastJCAErrorNs is bumped when the game pushes a **jca** error frame. A castle-focus that SUCCEEDS is
// answered with **jaa**; a focus the server REJECTS is answered with **jca** (e.g. code 6) and leaves the
// optimistic jaa/CastleFocus pointing at a stale castle/kingdom. Focus-await uses this to fail instead of
// letting callers send sdi/cds under a focus the server never applied.
var lastJCAErrorNs int64

func markJCAError() {
	atomic.StoreInt64(&lastJCAErrorNs, time.Now().UnixNano())
}

// jcaErrorAfter waits up to `wait` for a jca focus-rejection recorded after sinceNs; true if one arrived.
// On the common success path no rejection comes, so it blocks for the full `wait` (the focus settle window).
func jcaErrorAfter(sinceNs int64, wait time.Duration) bool {
	deadline := time.Now().Add(wait)
	for {
		if atomic.LoadInt64(&lastJCAErrorNs) > sinceNs {
			return true
		}
		if !time.Now().Before(deadline) {
			return false
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// JAAProcessedSnapshot returns the monotonic token bumped after each inbound **jaa** is handled.
func JAAProcessedSnapshot() int64 {
	return atomic.LoadInt64(&lastJAAProcessedNs)
}

// AwaitJAAProcessedAfter waits until jaa handling advances past prevNs or timeout elapses.
func AwaitJAAProcessedAfter(prevNs int64, timeout time.Duration) bool {
	return awaitNextJAAAfter(prevNs, timeout)
}

package App

// The commander launch-hold registry breaks the CRA 256 churn observed live:
// a burst of attack launches plans faster than the ~5s movement refresh, so
// every volley in the burst saw the same stale movement set, re-selected the
// same lowest-ID commander, and the game rejected the repeats with
// "response code 256 for CRA: commander already assigned to an active
// movement" — a tight rejected-request pattern that reads as bot traffic and
// precedes temporary suspensions.
//
// Selecting a commander for a launch now holds it briefly (a few movement
// refreshes). By the time the hold lapses, the real movement is visible in
// state and the ordinary availability check owns the answer. Holds are
// in-memory per account runtime: after a restart the first movement snapshot
// arrives before any lane plans, so nothing needs to persist.

import (
	"sync"
	"time"

	"CitadelDesktop/Server/State"
)

// craCommanderLaunchHold covers three movement refreshes with slack: long
// enough that the launched movement is always visible before the hold lapses,
// short enough to never idle a genuinely free commander.
const craCommanderLaunchHold = 15 * time.Second

type commanderLaunchHolds struct {
	mu    sync.Mutex
	until map[State.CommanderID]time.Time
}

func newCommanderLaunchHolds() *commanderLaunchHolds {
	return &commanderLaunchHolds{until: map[State.CommanderID]time.Time{}}
}

func (holds *commanderLaunchHolds) HoldCommanders(ids []State.CommanderID, until time.Time) {
	if holds == nil || len(ids) == 0 || until.IsZero() {
		return
	}
	holds.mu.Lock()
	defer holds.mu.Unlock()
	for _, id := range ids {
		if current, exists := holds.until[id]; !exists || until.After(current) {
			holds.until[id] = until
		}
	}
}

func (holds *commanderLaunchHolds) CommanderHeldAt(id State.CommanderID, now time.Time) bool {
	if holds == nil {
		return false
	}
	holds.mu.Lock()
	defer holds.mu.Unlock()
	until, exists := holds.until[id]
	if !exists {
		return false
	}
	if !now.Before(until) {
		// Expired: prune on the way out so the map stays small.
		delete(holds.until, id)
		return false
	}
	return true
}

package State

import "time"

// ProductionQueueNeedsRefresh reports when a cached queue cannot safely drive
// a production or alliance-help command. It deliberately does not expire a
// valid long-running queue by age alone; the active completion time supplies
// the authoritative point at which its slot state must be read again.
func ProductionQueueNeedsRefresh(state *GameState, queue ProductionQueue, now time.Time) bool {
	if queue.LineID != 2 && queue.Capacity > 0 && len(queue.Slots) == 0 {
		// Pre-slot snapshots must be refreshed, including after persistence.
		return true
	}
	if queue.ObservedAt.IsZero() {
		return true
	}
	if !now.IsZero() && queue.ObservedAt.After(now) {
		return true
	}
	if state.Session.Generation > 0 && !state.Session.ChangedAt.IsZero() &&
		queue.ObservedAt.Before(state.Session.ChangedAt) {
		return true
	}
	return queue.Active != nil && queue.Active.CompletesAt != nil &&
		!now.IsZero() && !now.Before(*queue.Active.CompletesAt)
}

// ProductionQueuePredatesCastleSnapshot detects a JAA/JCA castle-context
// snapshot that committed without a matching production-line snapshot. Both
// reducers use the same frame timestamp when the line is present, so an older
// queue is not authoritative for the castle context that is now committed.
func ProductionQueuePredatesCastleSnapshot(castle CastleState, queue ProductionQueue) bool {
	return !castle.ContextSnapshotObservedAt.IsZero() &&
		queue.ObservedAt.Before(castle.ContextSnapshotObservedAt)
}

// ProductionQueueFreeSlots counts empty slots whose observed entitlement is
// still usable at the supplied planning or dispatch time. Compact hospital
// queues retain their separate capacity representation.
func ProductionQueueFreeSlots(queue ProductionQueue, at time.Time) int {
	if queue.LineID == 2 {
		return max(0, queue.Capacity-len(queue.Queued))
	}
	free := 0
	for _, slot := range queue.Slots {
		if !slot.Occupied && (slot.Permanent || !slot.ExpiresAt.IsZero() && at.Before(slot.ExpiresAt)) {
			free++
		}
	}
	return free
}

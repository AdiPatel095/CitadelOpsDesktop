package State

// KhanDefenseLaunchesForOccurrence returns the number of authoritative Khan
// retaliation movements observed for one event occurrence. Unlike the local
// taunt dispatch cursor, this count advances only after the game accepts a
// taunt and publishes its incoming movement.
func (state GameState) KhanDefenseLaunchesForOccurrence(eventID int64, occurrence EventOccurrence) int64 {
	activity, found := state.LookupEventActivity(eventID)
	if !found || occurrence.EndsAt.IsZero() ||
		!SameEventOccurrence(activity.OccurrenceEndsAt, occurrence.EndsAt) {
		return 0
	}
	return max(int64(0), activity.KhanDefense.Launches)
}

// FullRageTauntDue reports whether the authoritative full rage bar belongs to
// a fill that has not already dispatched a retaliation in this event occurrence.
func (state KhanState) FullRageTauntDue(occurrence EventOccurrence) bool {
	if state.PlayerRageCap <= 0 || state.PlayerRage < state.PlayerRageCap || state.RageObservedAt.IsZero() {
		return false
	}
	return !state.TauntCursorIncludes(state.PlayerTotalRage, occurrence)
}

// TauntCursorIncludes compares total rage only within the same Khan event.
// Legacy cursors did not persist an occurrence end, so ObservedFrom provides a
// safe migration boundary without treating a same-event rage correction as a
// new fill.
func (state KhanState) TauntCursorIncludes(totalRage int64, occurrence EventOccurrence) bool {
	if state.LastTauntTriggeredAt.IsZero() {
		return false
	}
	if !state.LastTauntTriggeredEventEndsAt.IsZero() && !occurrence.EndsAt.IsZero() {
		if !SameEventOccurrence(state.LastTauntTriggeredEventEndsAt, occurrence.EndsAt) {
			return false
		}
	} else if state.LastTauntTriggeredEventEndsAt.IsZero() && !occurrence.ObservedFrom.IsZero() &&
		state.LastTauntTriggeredAt.Before(occurrence.ObservedFrom) {
		return false
	}
	return state.LastTauntTriggeredRage >= totalRage
}

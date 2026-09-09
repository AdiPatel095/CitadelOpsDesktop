package State

import (
	"testing"
	"time"
)

func TestRecordEventAttackLaunchForOccurrencePreservesPublishedEventActivity(t *testing.T) {
	now := time.Date(2026, time.September, 9, 12, 0, 0, 0, time.UTC)
	endsAt := now.Add(time.Hour)
	initialLaunch := MovementID(101)
	initialAttack := EventAttackRecord{
		MovementID: initialLaunch,
		Kind:       EventActivityInvasion,
		LaunchedAt: now.Add(-time.Minute),
	}
	launchIDs := make([]MovementID, 1, 4)
	launchIDs[0] = initialLaunch
	pendingAttacks := make([]EventAttackRecord, 1, 4)
	pendingAttacks[0] = initialAttack

	activity := EventActivityState{
		EventID:          77,
		OccurrenceEndsAt: endsAt,
		ObservedFrom:     now.Add(-time.Hour),
		Invasion:         EventCombatTotals{Launches: 1},
		LaunchIDs:        launchIDs,
		PendingAttacks:   pendingAttacks,
	}
	store := NewStore(NewGameState())
	if _, err := store.ApplyComponents(Components(ComponentEventScores), func(state *GameState) ([]string, bool, error) {
		state.SetEventActivity(77, activity)
		return []string{"event-scores"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	before, found := store.ReadOnlyView().LookupEventActivity(77)
	if !found || cap(before.LaunchIDs) < 2 || cap(before.PendingAttacks) < 2 {
		t.Fatalf("published activity does not retain spare capacity: %#v", before)
	}

	newAttack := EventAttackRecord{
		MovementID: 102,
		Kind:       EventActivityInvasion,
		LaunchedAt: now,
	}
	if _, err := store.ApplyComponents(Components(ComponentEventScores), func(state *GameState) ([]string, bool, error) {
		changed := RecordEventAttackLaunchForOccurrence(state, 77, endsAt, newAttack)
		return []string{"event-scores"}, changed, nil
	}); err != nil {
		t.Fatal(err)
	}

	if got := before.LaunchIDs[:2][1]; got != 0 {
		t.Fatalf("prior generation launch backing array mutated to %d", got)
	}
	if got := before.PendingAttacks[:2][1]; got != (EventAttackRecord{}) {
		t.Fatalf("prior generation pending backing array mutated to %#v", got)
	}
	after, found := store.ReadOnlyView().LookupEventActivity(77)
	if !found || len(after.LaunchIDs) != 2 || after.LaunchIDs[1] != newAttack.MovementID ||
		len(after.PendingAttacks) != 2 || after.PendingAttacks[1] != newAttack || after.Invasion.Launches != 2 {
		t.Fatalf("updated activity = %#v", after)
	}
}

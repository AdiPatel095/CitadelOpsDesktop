package State

import (
	"testing"
	"time"
)

func TestKhanTauntCursorIsScopedToEventOccurrence(t *testing.T) {
	currentStart := time.Date(2026, 8, 29, 8, 0, 0, 0, time.UTC)
	currentEnd := time.Date(2026, 9, 1, 7, 30, 0, 0, time.UTC)
	previousEnd := time.Date(2026, 8, 25, 7, 30, 0, 0, time.UTC)
	occurrence := EventOccurrence{EndsAt: currentEnd, ObservedFrom: currentStart}
	base := KhanState{
		PlayerRage: 1_500, PlayerRageCap: 1_500, PlayerTotalRage: 10_080,
		RageObservedAt: currentStart.Add(time.Hour), LastTauntTriggeredAt: currentStart.Add(time.Minute),
		LastTauntTriggeredRage: 10_080, LastTauntTriggeredEventEndsAt: currentEnd,
	}

	if base.FullRageTauntDue(occurrence) {
		t.Fatal("same occurrence and total reopened the taunt cursor")
	}
	lowerCorrection := base
	lowerCorrection.PlayerTotalRage = 9_000
	if lowerCorrection.FullRageTauntDue(occurrence) {
		t.Fatal("same-occurrence lower rage correction reopened the taunt cursor")
	}
	higherFill := base
	higherFill.PlayerTotalRage = 11_580
	if !higherFill.FullRageTauntDue(occurrence) {
		t.Fatal("higher total rage in the same occurrence did not reopen the taunt cursor")
	}
	newOccurrence := base
	newOccurrence.LastTauntTriggeredEventEndsAt = previousEnd
	if !newOccurrence.FullRageTauntDue(occurrence) {
		t.Fatal("equal total rage in a new occurrence remained deduplicated")
	}
	newOccurrence.PlayerTotalRage = 9_000
	if !newOccurrence.FullRageTauntDue(occurrence) {
		t.Fatal("lower total rage in a new occurrence remained deduplicated")
	}
	legacyPriorOccurrence := base
	legacyPriorOccurrence.LastTauntTriggeredEventEndsAt = time.Time{}
	legacyPriorOccurrence.LastTauntTriggeredAt = currentStart.Add(-24 * time.Hour)
	if !legacyPriorOccurrence.FullRageTauntDue(occurrence) {
		t.Fatal("legacy prior-occurrence cursor was not migrated by the authoritative boundary")
	}
	legacyCurrentOccurrence := base
	legacyCurrentOccurrence.LastTauntTriggeredEventEndsAt = time.Time{}
	legacyCurrentOccurrence.PlayerTotalRage = 9_000
	if legacyCurrentOccurrence.FullRageTauntDue(occurrence) {
		t.Fatal("legacy same-occurrence lower correction reopened the taunt cursor")
	}
}

func TestKhanDefenseLaunchesAreScopedToEventOccurrence(t *testing.T) {
	currentEnd := time.Date(2026, 9, 1, 7, 30, 0, 0, time.UTC)
	previousEnd := time.Date(2026, 8, 25, 7, 30, 0, 0, time.UTC)
	gameState := NewGameState()
	gameState.SetEventActivity(72, EventActivityState{
		EventID: 72, OccurrenceEndsAt: currentEnd,
		KhanDefense: EventCombatTotals{Launches: 4},
	})

	if got := gameState.KhanDefenseLaunchesForOccurrence(72, EventOccurrence{EndsAt: currentEnd}); got != 4 {
		t.Fatalf("current occurrence launches = %d, want 4", got)
	}
	if got := gameState.KhanDefenseLaunchesForOccurrence(72, EventOccurrence{EndsAt: previousEnd}); got != 0 {
		t.Fatalf("previous occurrence launches leaked into current count: %d", got)
	}
}

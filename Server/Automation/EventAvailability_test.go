package Automation

import (
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestLimitedEventGateUsesOpeningGraceBeforeSoftLock(t *testing.T) {
	opening := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC) // 10:00 CEST.
	state := State.NewGameState()
	state.EventScores.Inventory = State.EventInventoryState{
		ObservedAt:    opening,
		ActiveByEvent: map[int64]State.EventAvailability{},
	}

	decision, locked := limitedEventGate(state, opening.Add(4*time.Second), []int64{72, 80}, "Nomad or Samurai event")
	if !locked || decision.Status != "opening-check" || !decision.NextCheckAt.Equal(opening.Add(5*time.Minute)) {
		t.Fatalf("opening decision = %#v locked=%t", decision, locked)
	}

	decision, locked = limitedEventGate(state, opening.Add(5*time.Minute), []int64{72, 80}, "Nomad or Samurai event")
	if !locked || decision.Status != "soft-locked" ||
		!decision.NextCheckAt.Equal(opening.Add(24*time.Hour)) || !strings.Contains(decision.Detail, "not active") {
		t.Fatalf("settled decision = %#v locked=%t", decision, locked)
	}
}

func TestLimitedEventGateReopensImmediatelyFromAuthoritativeInventory(t *testing.T) {
	now := time.Date(2026, 8, 14, 8, 3, 53, 0, time.UTC)
	state := State.NewGameState()
	state.EventScores.Inventory = State.EventInventoryState{
		ObservedAt: now,
		ActiveByEvent: map[int64]State.EventAvailability{
			80: {EventID: 80, EndsAt: now.Add(20 * time.Hour)},
		},
	}
	if decision, locked := limitedEventGate(state, now, []int64{72, 80}, "Nomad or Samurai event"); locked {
		t.Fatalf("active event remained locked: %#v", decision)
	}
}

func TestLimitedEventOpeningTracksBerlinDaylightSaving(t *testing.T) {
	summer := time.Date(2026, 8, 14, 7, 0, 0, 0, time.UTC)
	winter := time.Date(2026, 12, 14, 8, 0, 0, 0, time.UTC)
	if got, want := limitedEventOpeningAfter(summer), time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("summer opening = %s, want %s", got, want)
	}
	if got, want := limitedEventOpeningAfter(winter), time.Date(2026, 12, 14, 9, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Fatalf("winter opening = %s, want %s", got, want)
	}
}

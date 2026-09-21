package Automation

import (
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
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

func TestLimitedEventDescriptorUsesWholeIdentityTemplates(t *testing.T) {
	opening := time.Date(2026, 8, 14, 8, 0, 0, 0, time.UTC)
	for _, family := range []struct {
		ids   []int64
		label string
	}{
		{[]int64{nomadEventID, samuraiEventID}, "Nomad or Samurai event"},
		{[]int64{bloodcrowEventID, foreignLordsEventID}, "Foreign Lords or Bloodcrow event"},
		{[]int64{autoKhanEventID}, "Nomad Khan event"},
		{[]int64{GameData.BerimondEventID}, "Battle for Berimond"},
	} {
		for _, phase := range []struct{ now, observed time.Time }{
			{opening.Add(time.Second), opening},
			{opening.Add(6 * time.Minute), opening},
			{opening.Add(6 * time.Minute), opening.Add(-time.Hour)},
		} {
			state := State.NewGameState()
			state.EventScores.Inventory.ObservedAt = phase.observed
			decision, locked := limitedEventGate(state, phase.now, family.ids, family.label)
			if !locked || decision.DetailDescriptor == nil || decision.DetailDescriptor.Fallback != decision.Detail || len(decision.DetailDescriptor.Params) != 0 {
				t.Fatalf("lost whole event message: %+v", decision)
			}
		}
	}
	for _, ids := range [][]int64{nil, {999}, {nomadEventID, samuraiEventID, 999}, {nomadEventID, nomadEventID}} {
		if limitedEventDescriptor(ids, "inactive") != nil {
			t.Fatalf("unknown event family inferred: %v", ids)
		}
	}
	if limitedEventDescriptor([]int64{autoKhanEventID}, "unknown") != nil {
		t.Fatal("unknown phase inferred")
	}
}

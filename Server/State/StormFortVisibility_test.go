package State

import (
	"testing"
	"time"
)

// CIT-24: the official Storm fort visibility flag survives every copy site.
func TestStormFortHiddenFlagSurvivesProjectionFactsAndTracking(t *testing.T) {
	now := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	hidden := MapObservation{
		KingdomID: 4, X: 106, Y: 107, TypeID: MapTypeStormFort, StormIsleID: 10,
		StormCooldownRemaining: 120, StormHidden: true, ObservedAt: now,
	}
	projected, retained := projectMapObservation(hidden)
	if !retained || !projected.StormHidden || projected.StormCooldownRemaining != 120 {
		t.Fatalf("projection = %#v retained=%t", projected, retained)
	}
	if roundTrip := worldMapFact(hidden).observation(); !roundTrip.StormHidden || roundTrip != projected {
		t.Fatalf("world fact round trip = %#v, want %#v", roundTrip, projected)
	}
	visible := hidden
	visible.StormHidden = false
	if roundTrip := worldMapFact(visible).observation(); roundTrip.StormHidden {
		t.Fatalf("visible fort became hidden: %#v", roundTrip)
	}

	state := NewGameState()
	older := visible
	older.ObservedAt = now.Add(-time.Minute)
	state.Storm.Map.Targets["106:107"] = older
	if !state.RefreshStormTargetObservation(hidden) || !state.Storm.Map.Targets["106:107"].StormHidden {
		t.Fatalf("tracked Storm target = %#v", state.Storm.Map.Targets["106:107"])
	}
}

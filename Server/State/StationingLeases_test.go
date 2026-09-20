package State

import (
	"testing"
	"time"
)

func TestKhanAutoStationYieldUsesOnlyAuthoritativeTrackedMovements(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	future := now.Add(time.Minute)
	past := now.Add(-time.Minute)
	zero := time.Time{}

	tests := []struct {
		name      string
		operation StationingOperation
		movements []MovementState
		want      bool
	}{
		{
			name: "unrelated same route is not station owned",
			operation: StationingOperation{
				Purpose: "autoStation", SourceCastleID: 1, TargetCastleID: 2,
				MovementID: 10,
			},
			movements: []MovementState{
				{ID: 10, Direction: 0, SourceCastleID: 1, TargetCastleID: 2, ArrivesAt: &past},
				{ID: 11, Direction: 0, SourceCastleID: 1, TargetCastleID: 2, ArrivesAt: &future},
			},
		},
		{
			name: "absent tracked id does not claim same route",
			operation: StationingOperation{
				Purpose: "autoStation", SourceCastleID: 1, TargetCastleID: 2,
				MovementID: 10,
			},
			movements: []MovementState{
				{ID: 11, Direction: 0, SourceCastleID: 1, TargetCastleID: 2, ArrivesAt: &future},
			},
		},
		{
			name: "manual same route without tracked ids is not station owned",
			operation: StationingOperation{
				Purpose: "autoStation", SourceCastleID: 1, TargetCastleID: 2,
			},
			movements: []MovementState{
				{ID: 11, Direction: 0, SourceCastleID: 1, TargetCastleID: 2, ArrivesAt: &future},
			},
		},
		{
			name:      "tracked outbound remains active",
			operation: StationingOperation{Purpose: "autoStation", MovementID: 10},
			movements: []MovementState{{ID: 10, Direction: 0, ArrivesAt: &future}},
			want:      true,
		},
		{
			name: "batch checks later active id",
			operation: StationingOperation{
				Purpose: "autoStation", MovementIDs: []MovementID{10, 11},
			},
			movements: []MovementState{
				{ID: 10, Direction: 0, ArrivesAt: &past},
				{ID: 11, Direction: 0, ArrivesAt: &future},
			},
			want: true,
		},
		{
			name: "nonempty batch excludes stale scalar",
			operation: StationingOperation{
				Purpose: "autoStation", MovementID: 10, MovementIDs: []MovementID{11},
			},
			movements: []MovementState{
				{ID: 10, Direction: 0, ArrivesAt: &future},
				{ID: 11, Direction: 0, ArrivesAt: &past},
			},
		},
		{
			name:      "tracked return remains active",
			operation: StationingOperation{Purpose: "autoStation", MovementIDs: []MovementID{10}},
			movements: []MovementState{{ID: 10, Direction: 1, ReturnsAt: &future}},
			want:      true,
		},
		{
			name:      "missing completion is conservative",
			operation: StationingOperation{Purpose: "autoStation", MovementID: 10},
			movements: []MovementState{{ID: 10, Direction: 0}},
			want:      true,
		},
		{
			name:      "zero completion is conservative",
			operation: StationingOperation{Purpose: "autoStation", MovementID: 10},
			movements: []MovementState{{ID: 10, Direction: 1, ReturnsAt: &zero}},
			want:      true,
		},
		{
			name:      "invalid direction is conservative",
			operation: StationingOperation{Purpose: "autoStation", MovementID: 10},
			movements: []MovementState{{ID: 10, Direction: 2, ArrivesAt: &past, ReturnsAt: &past}},
			want:      true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := NewGameState()
			state.Stationing["autoStation:1"] = test.operation
			for _, movement := range test.movements {
				state.Movements[movement.ID] = movement
			}
			if got := KhanAutoStationYieldActiveAt(state, now); got != test.want {
				t.Fatalf("KhanAutoStationYieldActiveAt() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestKhanAutoStationYieldPreservesSafeAfterGrace(t *testing.T) {
	now := time.Date(2026, 9, 19, 12, 0, 0, 0, time.UTC)
	safeAfter := now
	state := NewGameState()
	state.Stationing["autoStation:1"] = StationingOperation{
		Purpose: "autoStation", MovementID: 10, SafeAfter: &safeAfter,
	}

	if !KhanAutoStationYieldActiveAt(state, now.Add(5*time.Second-time.Nanosecond)) {
		t.Fatal("Auto Khan did not yield inside the SafeAfter grace window")
	}
	if KhanAutoStationYieldActiveAt(state, now.Add(5*time.Second)) {
		t.Fatal("Auto Khan yielded at the expired SafeAfter grace boundary")
	}
}

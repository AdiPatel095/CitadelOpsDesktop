package App

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestCommanderLaunchHoldsExpireAndExtend(t *testing.T) {
	holds := newCommanderLaunchHolds()
	base := time.Date(2026, 8, 17, 17, 0, 0, 0, time.UTC)

	holds.HoldCommanders([]State.CommanderID{7}, base.Add(15*time.Second))
	if !holds.CommanderHeldAt(7, base) {
		t.Fatal("commander should be held immediately after selection")
	}
	if holds.CommanderHeldAt(8, base) {
		t.Fatal("unrelated commander must not be held")
	}
	if holds.CommanderHeldAt(7, base.Add(15*time.Second)) {
		t.Fatal("hold must lapse at its deadline")
	}
	// Extensions only ever push the deadline out, never pull it in.
	holds.HoldCommanders([]State.CommanderID{9}, base.Add(30*time.Second))
	holds.HoldCommanders([]State.CommanderID{9}, base.Add(10*time.Second))
	if !holds.CommanderHeldAt(9, base.Add(20*time.Second)) {
		t.Fatal("a shorter later hold must not shorten an existing one")
	}
}

func TestCRASelectionSkipsHeldCommandersAndRegistersHolds(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders = map[State.CommanderID]State.CommanderState{
		1: {ID: 1, Available: true},
		2: {ID: 2, Available: true},
	}
	holds := newCommanderLaunchHolds()

	first, err := resolveCRACommanders(gameState,
		&craCommanderSelectionRequest{Candidates: []State.CommanderID{1, 2}, Count: 1, Strategy: "lowest_id"},
		craCommanderSelectionOptions{Holds: holds, DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Selected) != 1 || first.Selected[0] != 1 {
		t.Fatalf("first selection = %v, want commander 1", first.Selected)
	}

	// Selection must NOT register holds itself: plan validation re-runs the
	// same selection before dispatch, and self-registration made the re-plan
	// fail permanently stale on single-commander configurations. Repeating
	// the selection therefore keeps yielding the same commander.
	second, err := resolveCRACommanders(gameState,
		&craCommanderSelectionRequest{Candidates: []State.CommanderID{1, 2}, Count: 1, Strategy: "lowest_id"},
		craCommanderSelectionOptions{Holds: holds, DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Selected) != 1 || second.Selected[0] != 1 {
		t.Fatalf("second selection = %v, want commander 1 again (no self-registration)", second.Selected)
	}

	// An EXTERNALLY registered hold (the future dispatch-layer owner) is
	// still honored by selection.
	holds.HoldCommanders([]State.CommanderID{1}, time.Now().Add(15*time.Second))
	third, err := resolveCRACommanders(gameState,
		&craCommanderSelectionRequest{Candidates: []State.CommanderID{1, 2}, Count: 1, Strategy: "lowest_id"},
		craCommanderSelectionOptions{Holds: holds, DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(third.Selected) != 1 || third.Selected[0] != 2 {
		t.Fatalf("third selection = %v, want commander 2 while 1 is externally held", third.Selected)
	}
}

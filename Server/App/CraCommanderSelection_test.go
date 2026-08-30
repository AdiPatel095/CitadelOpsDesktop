package App

import (
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestResolveCRACommandersPreservesCandidateOrderAndSkipsBusy(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Commanders[9] = State.CommanderState{ID: 9, Available: true}
	selection := &craCommanderSelectionRequest{
		Candidates: []State.CommanderID{7, 5, 9},
		Count:      2,
		Strategy:   "first_available",
	}
	resolution, err := resolveCRACommanders(gameState, selection, craCommanderSelectionOptions{
		DefaultCount:     1,
		RequireAvailable: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolution.Selected, []State.CommanderID{5, 9}) {
		t.Fatalf("selected = %#v", resolution.Selected)
	}
	if !reflect.DeepEqual(resolution.Candidates, []State.CommanderID{7, 5, 9}) {
		t.Fatalf("candidates = %#v", resolution.Candidates)
	}
}

func TestResolveCRACommandersRejectsActiveMovementWhenRosterSaysAvailable(t *testing.T) {
	now := time.Now().UTC()
	arrivesAt := now.Add(10 * time.Minute)
	commanderID := State.CommanderID(7)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = State.CastleState{ID: 100}
	gameState.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: true}
	gameState.Commanders[9] = State.CommanderState{ID: 9, Available: true}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
		CommanderID: &commanderID, ArrivesAt: &arrivesAt,
	}

	resolution, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{
		Candidates: []State.CommanderID{7, 9}, Count: 1, Strategy: "lowest_id",
	}, craCommanderSelectionOptions{DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolution.Selected, []State.CommanderID{9}) {
		t.Fatalf("selected = %#v, want inactive commander 9", resolution.Selected)
	}
}

func TestResolveCRACommandersSupportsDeterministicIDStrategies(t *testing.T) {
	gameState := State.NewGameState()
	for _, id := range []State.CommanderID{9, 5, 7} {
		gameState.Commanders[id] = State.CommanderState{ID: id, Available: true}
	}
	tests := []struct {
		name     string
		strategy string
		want     []State.CommanderID
	}{
		{name: "lowest", strategy: "lowest_id", want: []State.CommanderID{5, 7}},
		{name: "highest", strategy: "highest_id", want: []State.CommanderID{9, 7}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resolution, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{
				Candidates: []State.CommanderID{9, 5, 7}, Count: 2, Strategy: test.strategy,
			}, craCommanderSelectionOptions{DefaultCount: 1, RequireAvailable: true})
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(resolution.Selected, test.want) {
				t.Fatalf("selected = %#v, want %#v", resolution.Selected, test.want)
			}
		})
	}
}

func TestResolveCRACommandersRejectsInvalidSelection(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	if _, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{
		Candidates: []State.CommanderID{5}, Count: 1, Strategy: "random",
	}, craCommanderSelectionOptions{DefaultCount: 1}); err == nil {
		t.Fatal("expected invalid strategy error")
	}
	if _, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{
		Candidates: []State.CommanderID{6}, Count: 1,
	}, craCommanderSelectionOptions{DefaultCount: 1}); err == nil {
		t.Fatal("expected unknown commander error")
	}
}

func TestResolveCRACommandersAcceptsWireIDZero(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	resolution, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{
		Candidates: []State.CommanderID{0}, Count: 1,
	}, craCommanderSelectionOptions{DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(resolution.Selected, []State.CommanderID{0}) {
		t.Fatalf("selected = %#v", resolution.Selected)
	}
}

package App

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestMovementClockReleasesReturnedCommander(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = State.CastleState{ID: 100}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	commanderID := State.CommanderID(7)
	returnsAt := time.Now().UTC().Add(40 * time.Millisecond)
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, TargetCastleID: 100,
		CommanderID: &commanderID, ReturnsAt: &returnsAt,
	}
	application := &Application{State: State.NewStore(gameState)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go application.runMovementClock(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := application.State.Snapshot()
		if _, exists := snapshot.Movements[50]; !exists && snapshot.Commanders[7].Available {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("movement clock did not release the returned commander")
}

func TestMovementClockWaitsForGameReportedStationReturn(t *testing.T) {
	now := time.Now().UTC()
	arrivedAt := now.Add(-time.Hour)
	gameState := State.NewGameState()
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, WaitSeconds: 6 * 3600, ArrivesAt: &arrivedAt,
	}

	if next := nextMovementCompletion(gameState); !next.IsZero() {
		t.Fatalf("station wait scheduled a locally predicted completion at %s", next)
	}
}

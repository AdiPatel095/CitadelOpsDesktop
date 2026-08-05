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

func TestMovementClockReturnsObservedSurvivorsToCastle(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	commanderID := State.CommanderID(7)
	returnsAt := time.Now().UTC().Add(40 * time.Millisecond)
	castle := State.CastleState{
		ID: 100,
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{10: 20},
			Traveling: map[State.UnitID]int64{10: 48},
			Hospital:  map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{},
			Total: map[State.UnitID]int64{10: 68},
		},
		UnitsObservedAt: returnsAt.Add(-time.Minute),
	}
	gameState.Castles[100] = castle
	gameState.Commanders[commanderID] = State.CommanderState{ID: commanderID, Available: false}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, TargetCastleID: 100,
		CommanderID: &commanderID, ReturnsAt: &returnsAt, Units: map[State.UnitID]int64{10: 48},
	}
	application := &Application{State: State.NewStore(gameState)}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	go application.runMovementClock(ctx)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		snapshot := application.State.Snapshot()
		if _, exists := snapshot.Movements[50]; !exists && snapshot.Commanders[commanderID].Available &&
			snapshot.Castles[100].Units.Stationed[10] == 68 && snapshot.Castles[100].Units.Traveling[10] == 0 {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("movement clock did not return observed survivors to the castle")
}

func TestMovementClockUsesProjectedStationReturn(t *testing.T) {
	now := time.Now().UTC()
	arrivedAt := now.Add(-time.Hour)
	gameState := State.NewGameState()
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, TravelSeconds: 600, WaitSeconds: 6 * 3600, ArrivesAt: &arrivedAt,
	}

	want := arrivedAt.Add(6*time.Hour + 10*time.Minute)
	if next := nextMovementCompletion(gameState); !next.Equal(want) {
		t.Fatalf("station movement completion = %s, want return %s", next, want)
	}
}

func TestMovementClockUsesMarketBarrowReturnLease(t *testing.T) {
	now := time.Now().UTC()
	arrivesAt := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	gameState.Castles[100] = State.CastleState{ID: 100}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
		MarketBarrows: 10, TravelSeconds: 60, ArrivesAt: &arrivesAt,
	}

	want := arrivesAt.Add(time.Minute)
	if next := nextMovementCompletion(gameState); !next.Equal(want) {
		t.Fatalf("market movement completion = %s, want return %s", next, want)
	}
}

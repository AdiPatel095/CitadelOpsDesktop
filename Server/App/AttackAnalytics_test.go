package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestCaptureAttackFeatureLaunchAllowsCommanderZero(t *testing.T) {
	now := time.Date(2026, time.July, 21, 19, 18, 0, 0, time.UTC)
	arrivesAt := now.Add(5 * time.Minute)
	commanderID := State.CommanderID(0)
	state := State.NewGameState()
	state.Movements[123] = State.MovementState{
		ID: 123, Direction: 0, SourceCastleID: 1928, CommanderID: &commanderID,
		KingdomID: 4, TargetX: 605, TargetY: 689, ArrivesAt: &arrivesAt, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, err := json.Marshal(attackFeatureCaptureRequest{
		FeatureID: State.AttackFeatureAutoStorm, SourceCastleID: 1928, CommanderID: commanderID,
		KingdomID: 4, TargetTypeID: 25, TargetX: 605, TargetY: 689,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := application.captureAttackFeatureLaunch(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	snapshot := application.State.Snapshot()
	if len(snapshot.AttackAnalytics.PendingAttacks) != 1 || snapshot.AttackAnalytics.PendingAttacks[0].MovementID != 123 {
		t.Fatalf("captured attacks = %#v", snapshot.AttackAnalytics.PendingAttacks)
	}
	if len(snapshot.AttackAnalytics.RecentAutoStormLaunches) != 1 ||
		snapshot.AttackAnalytics.RecentAutoStormLaunches[0].MovementID != 123 {
		t.Fatalf("recent Auto Storm launch history = %#v", snapshot.AttackAnalytics.RecentAutoStormLaunches)
	}
}

func TestCaptureAutoBeriAttackFeatureLaunch(t *testing.T) {
	now := time.Date(2026, time.July, 29, 11, 36, 0, 0, time.UTC)
	arrivesAt := now.Add(5 * time.Minute)
	commanderID := State.CommanderID(0)
	state := State.NewGameState()
	state.Movements[456] = State.MovementState{
		ID: 456, Direction: 0, SourceCastleID: 900, CommanderID: &commanderID,
		KingdomID: 10, TargetX: 1438, TargetY: 82, ArrivesAt: &arrivesAt, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, err := json.Marshal(attackFeatureCaptureRequest{
		FeatureID: State.AttackFeatureAutoBeriWorld, SourceCastleID: 900, CommanderID: commanderID,
		KingdomID: 10, TargetTypeID: 17, TargetX: 1438, TargetY: 82,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.captureAttackFeatureLaunch(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	pending := application.State.Snapshot().AttackAnalytics.PendingAttacks
	if len(pending) != 1 || pending[0].MovementID != 456 ||
		pending[0].FeatureID != State.AttackFeatureAutoBeriWorld {
		t.Fatalf("captured Auto Beri attacks = %#v", pending)
	}
}

func TestCaptureAutoTowerLaunchAdvancesCastleCursorCountOnlyOnce(t *testing.T) {
	now := time.Date(2026, time.July, 23, 13, 0, 0, 0, time.UTC)
	arrivesAt := now.Add(5 * time.Minute)
	commanderID := State.CommanderID(7)
	state := State.NewGameState()
	state.Movements[321] = State.MovementState{
		ID: 321, Direction: 0, SourceCastleID: 100, CommanderID: &commanderID,
		KingdomID: 0, TargetX: 101, TargetY: 102, ArrivesAt: &arrivesAt, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(state)}
	arguments, err := json.Marshal(attackFeatureCaptureRequest{
		FeatureID: State.AttackFeatureAutoTowers, SourceCastleID: 100, CommanderID: commanderID,
		KingdomID: 0, TargetTypeID: 2, TargetX: 101, TargetY: 102,
	})
	if err != nil {
		t.Fatal(err)
	}

	if err := application.captureAttackFeatureLaunch(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	if err := application.captureAttackFeatureLaunch(context.Background(), arguments); err != nil {
		t.Fatal(err)
	}
	snapshot := application.State.Snapshot()
	if got := snapshot.TowerQueue.ConfirmedLaunchesByCastle[100]; got != 1 {
		t.Fatalf("confirmed tower launches = %d, want 1", got)
	}
}

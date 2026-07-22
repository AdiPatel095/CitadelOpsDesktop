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
}

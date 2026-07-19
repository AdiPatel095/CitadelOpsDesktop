package App

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestCaptureKhanLaunchRecordsAndBlocksOvertakingArrival(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	previousArrival := now.Add(2 * time.Minute)
	currentArrival := now.Add(time.Minute)
	commanderID := State.CommanderID(2)
	gameState := State.NewGameState()
	gameState.Khan = State.KhanState{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0, TargetX: 210, TargetY: 942,
		AttacksLaunched: 1,
		Launches:        []State.KhanLaunchState{{CommanderID: 1, MovementID: 10, ArrivesAt: previousArrival}},
		Taunts:          map[State.MovementID]State.KhanTauntState{},
	}
	gameState.Movements[11] = State.MovementState{
		ID: 11, Direction: 0, SourceCastleID: 2, KingdomID: 0, TargetX: 210, TargetY: 942,
		CommanderID: &commanderID, ArrivesAt: &currentArrival, ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(khanLaunchCapture{
		RunID: "run", SourceCastleID: 2, MainCastleID: 1, KingdomID: 0,
		TargetX: 210, TargetY: 942, CommanderID: commanderID,
	})
	err := application.captureKhanLaunch(t.Context(), arguments)
	if err == nil || !strings.Contains(err.Error(), "unsafe Khan chain arrival order") {
		t.Fatalf("capture error = %v", err)
	}
	khan := application.State.Snapshot().Khan
	if khan.SafetyError == "" || khan.AttacksLaunched != 2 || len(khan.Launches) != 2 {
		t.Fatalf("captured Khan state = %#v", khan)
	}
}

func TestPlanKhanPointLimitProtectionRecallsKhanLaunchAndOpensGates(t *testing.T) {
	now := time.Now().UTC()
	arrival := now.Add(time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 158
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 0, SlotType: 1}
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 0, OwnerPlayerID: 158, SourceCastleID: 1, ArrivesAt: &arrival,
	}
	gameState.Khan.Launches = []State.KhanLaunchState{{MovementID: 50}}
	gameState.EventScores.ByEvent[khanEventID] = State.ScalableEventScore{EventID: khanEventID, PlayerScore: 10_000}
	arguments := json.RawMessage(`{"castleId":1,"pointThreshold":10000}`)
	plan, err := planKhanPointLimitProtection(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "mcm" || plan.Steps[1].Opcode != "mos" {
		t.Fatalf("point-limit steps = %#v", plan.Steps)
	}
}

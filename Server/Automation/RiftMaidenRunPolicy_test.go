package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestRiftMaidenRunPolicyCapsRoundAtExactRemainingGoal(t *testing.T) {
	now := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Rift.MaidenRun = &State.RiftMaidenRunState{
		ID: "run", Status: "running", RequestedAttacks: 50, AttacksLaunched: 48,
		UnitID: 216, HorseTravelBoostID: -1, CommanderIDs: []State.CommanderID{5, 6, 7},
		SourceCastleID: 1, SourceX: 7, SourceY: 8, KingdomID: 0, TargetX: 10, TargetY: 20,
	}
	gameState.Castles[1] = State.CastleState{
		ID: 1, X: 7, Y: 8, KingdomID: 0,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 99}},
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: 43},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Commanders[6] = State.CommanderState{ID: 6, Available: true}
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	policy := NewRiftMaidenRunPolicy()
	decision, err := policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "rift.maiden_wave.launch" {
		t.Fatalf("round decision = %#v err=%v", decision, err)
	}
	var request struct {
		RunID              string `json:"runId"`
		CommanderSelection struct {
			Count int `json:"count"`
		} `json:"commanderSelection"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.RunID != "run" || request.CommanderSelection.Count != 2 {
		t.Fatalf("round request = %#v", request)
	}
	if !policyEnabled(policy, map[string]bool{}, gameState) {
		t.Fatal("active on-demand Rift run was disabled by automation.enabled")
	}
	gameState.Rift.MaidenRun.Status = "completed"
	if policyEnabled(policy, map[string]bool{"rift_maiden_run": true}, gameState) {
		t.Fatal("completed on-demand Rift run remained active")
	}
}

func TestRiftMaidenRunPolicyWaitsForCommanderReturn(t *testing.T) {
	now := time.Date(2026, 8, 5, 14, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Rift.MaidenRun = &State.RiftMaidenRunState{
		ID: "run", Status: "running", RequestedAttacks: 4, AttacksLaunched: 3,
		UnitID: 216, CommanderIDs: []State.CommanderID{5}, SourceCastleID: 1,
		SourceX: 7, SourceY: 8, KingdomID: 0, TargetX: 10, TargetY: 20,
	}
	gameState.Castles[1] = State.CastleState{
		ID: 1, X: 7, Y: 8, KingdomID: 0,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 33}},
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"10:20": {KingdomID: 0, X: 10, Y: 20, TypeID: 43},
	}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: false}
	decision, err := NewRiftMaidenRunPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "waiting" || decision.NextCheckAt.IsZero() {
		t.Fatalf("waiting decision = %#v err=%v", decision, err)
	}
}

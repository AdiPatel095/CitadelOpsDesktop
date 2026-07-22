package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanTroopsStationRejectsAutomationDuringPurchasedProtectionMode(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ProtectionMode = State.PlayerProtectionModeState{
		ModeState: 1, RemainingSec: 3600, ObservedAt: time.Now().UTC(),
	}
	for _, purpose := range []string{"autoBird", "autoStation"} {
		arguments, _ := json.Marshal(map[string]any{"purpose": purpose})
		_, err := planTroopsStation(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
		if err == nil || !strings.Contains(err.Error(), "stationing is disabled while Protection Mode") {
			t.Fatalf("%s protection guard error = %v", purpose, err)
		}
	}
}

func TestPlanTroopsStationRejectsTools(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[1928] = State.CastleState{
		ID: 1928, KingdomID: 4, X: 576, Y: 669,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{735: 472_910}},
	}
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 1829, KingdomID: 4, X: 696, Y: 583, SlotType: 12,
	}}
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":735,"toolCategory":"Premium","slotTypes":"1,2,9"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	arguments := json.RawMessage(`{
		"sourceCastleId":1928,"targetCastleId":1829,"delayHours":11,
		"purpose":"autoBird","units":[{"unitId":735,"amount":472910}]
	}`)
	_, err = planTroopsStation(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err == nil || !strings.Contains(err.Error(), "tool, not a stationable troop") {
		t.Fatalf("tool station error = %v", err)
	}
}

func TestPlanTroopsStationSkipsAnActiveTrackedAutomationMovement(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Stationing["autoBird:10"] = State.StationingOperation{
		ID: "autoBird:10", Purpose: "autoBird", SourceCastleID: 10,
		TargetCastleID: 20, UpdatedAt: now,
	}
	plan, err := planTroopsStation(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":6,
		"purpose":"autoBird","trackingId":"autoBird:10","units":[{"unitId":489,"amount":1}]
	}`))
	if err != nil || len(plan.Steps) != 0 || !strings.Contains(plan.Summary, "already active") {
		t.Fatalf("active tracked station plan = %#v err=%v", plan, err)
	}
}

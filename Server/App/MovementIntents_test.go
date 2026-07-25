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

func TestTrackStationMovementSetsSuccessCooldownThroughConfiguredDelay(t *testing.T) {
	gameState := State.NewGameState()
	application := &Application{State: State.NewStore(gameState)}
	startedAt := time.Now().UTC()
	err := application.trackStationMovement(t.Context(), json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":6,
		"purpose":"autoBird","trackingId":"autoBird:10","units":[{"unitId":489,"amount":8274}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	operation := application.State.Snapshot().Stationing["autoBird:10"]
	if operation.SuccessCooldownUntil == nil ||
		operation.SuccessCooldownUntil.Before(startedAt.Add(6*time.Hour)) ||
		operation.SuccessCooldownUntil.After(time.Now().UTC().Add(6*time.Hour+time.Second)) {
		t.Fatalf("AutoBird success cooldown = %v", operation.SuccessCooldownUntil)
	}
	if !operation.ActiveAt(nil, startedAt.Add(6*time.Minute)) {
		t.Fatal("successful AutoBird dispatch became eligible for a duplicate retry")
	}
}

func TestPlanTroopsStationRefreshesFocusedSourceAndDefersManifest(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 20, Y: 30, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 100}},
	}
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 40, Y: 50, SlotType: 1,
	}}
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	plan, err := planTroopsStation(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":1,
		"purpose":"autoStation","trackingId":"autoStation:10","units":[{"unitId":489,"amount":100}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "jca" ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		plan.Steps[2].Resolver != "troops.station.build" {
		t.Fatalf("station plan steps = %#v", plan.Steps)
	}
}

func TestResolveTroopsStationClampsAutomationToRefreshedUnits(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{215: 67_644, 216: 39_237, 489: 92}},
	}
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 342, Y: 604, SlotType: 1,
	}}
	step, err := resolveTroopsStationStep(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":1,"purpose":"autoStation",
		"units":[{"unitId":215,"amount":68180},{"unitId":216,"amount":39237},{"unitId":489,"amount":92}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Units [][2]int64 `json:"A"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	want := [][2]int64{{215, 67_644}, {216, 39_237}, {489, 92}}
	if len(payload.Units) != len(want) {
		t.Fatalf("resolved units = %#v", payload.Units)
	}
	for index := range want {
		if payload.Units[index] != want[index] {
			t.Fatalf("resolved units = %#v, want %#v", payload.Units, want)
		}
	}
}

func TestResolveTroopsStationRejectsStaleManualAmount(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, Units: State.CastleUnits{Stationed: map[State.UnitID]int64{489: 90}},
	}
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 40, Y: 50, SlotType: 1,
	}}
	_, err := resolveTroopsStationStep(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":1,"units":[{"unitId":489,"amount":100}]
	}`))
	if err == nil || !strings.Contains(err.Error(), "now has 90 stationed unit 489") {
		t.Fatalf("manual stale amount error = %v", err)
	}
}

func TestTrackStationMovementUsesResolvedManifest(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Alliance.Holdings = []State.AllianceHolding{{
		CastleID: 20, KingdomID: 0, X: 40, Y: 50, SlotType: 1,
	}}
	gameState.Movements[30] = State.MovementState{
		ID: 30, Direction: 0, SourceCastleID: 10, TargetCastleID: 20, TargetX: 40, TargetY: 50,
		Units:      map[State.UnitID]int64{215: 67_644, 216: 39_237, 489: 92},
		ObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	err := application.trackStationMovement(t.Context(), json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"delayHours":1,
		"purpose":"autoStation","trackingId":"autoStation:10",
		"units":[{"unitId":215,"amount":68180},{"unitId":216,"amount":39237},{"unitId":489,"amount":92}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	operation := application.State.Snapshot().Stationing["autoStation:10"]
	if operation.MovementID != 30 || operation.Units[215] != 67_644 {
		t.Fatalf("tracked station operation = %#v", operation)
	}
}

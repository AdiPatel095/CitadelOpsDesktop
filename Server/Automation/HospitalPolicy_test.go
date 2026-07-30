package Automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestHospitalPolicyQueuesBaseStackWhenHospitalIsEmpty(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	decision, err := NewHospitalPolicy().Evaluate(context.Background(), hospitalPolicySnapshot(t, now))
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "hospital.heal" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if amount := hospitalIntentAmount(t, decision); amount != hospitalBaseStackAmount {
		t.Fatalf("heal amount = %d, want %d", amount, hospitalBaseStackAmount)
	}
}

func TestHospitalPolicyUsesSubscriptionStackBonus(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	snapshot := hospitalPolicySnapshot(t, now)
	snapshot.State.Subscriptions[1] = State.SubscriptionState{TypeID: 1, RemainingSec: 60}
	decision, err := NewHospitalPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "hospital.heal" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if amount := hospitalIntentAmount(t, decision); amount != hospitalMaximumStackAmount {
		t.Fatalf("heal amount = %d, want %d", amount, hospitalMaximumStackAmount)
	}
}

func TestHospitalPolicyUsesOfficialHospitalSlotsWhenQueueCapacityIsMissing(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	snapshot := hospitalPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	castle.Production[hospitalLineID] = State.ProductionQueue{LineID: hospitalLineID, ObservedAt: now}
	snapshot.State.Castles[77] = castle
	decision, err := NewHospitalPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "hospital.heal" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestHospitalPolicyRequestsAllianceHelpAfterFillingTheQueue(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	snapshot := hospitalPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[hospitalLineID]
	queue.Active = &State.QueueItem{ProductionID: 101, AllianceHelpAvailable: true}
	queue.Queued = []State.QueueItem{{ProductionID: 102}, {ProductionID: 103}, {ProductionID: 104}, {ProductionID: 105}}
	castle.Production[hospitalLineID] = queue
	snapshot.State.Castles[77] = castle
	decision, err := NewHospitalPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if productionID := allianceHelpProductionID(t, decision); productionID != 101 {
		t.Fatalf("production id = %d, want 101", productionID)
	}
}

func TestHospitalPolicyRequestsAllianceHelpWhenNoWoundedUnitsRemain(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	snapshot := hospitalPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	castle.Units.Hospital = map[State.UnitID]int64{}
	queue := castle.Production[hospitalLineID]
	queue.Active = &State.QueueItem{ProductionID: 101, AllianceHelpAvailable: true}
	queue.Queued = []State.QueueItem{{ProductionID: 102}, {ProductionID: 103}, {ProductionID: 104}, {ProductionID: 105}}
	castle.Production[hospitalLineID] = queue
	snapshot.State.Castles[77] = castle
	decision, err := NewHospitalPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "alliance.help.request" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	if productionID := allianceHelpProductionID(t, decision); productionID != 101 {
		t.Fatalf("production id = %d, want 101", productionID)
	}
}

func TestHospitalPolicyWaitsAtAllianceHelpRequestLimit(t *testing.T) {
	now := time.Date(2026, 7, 12, 17, 0, 0, 0, time.UTC)
	snapshot := hospitalPolicySnapshot(t, now)
	castle := snapshot.State.Castles[77]
	queue := castle.Production[hospitalLineID]
	queue.Active = &State.QueueItem{ProductionID: 101, AllianceHelpRequested: true}
	queue.Queued = []State.QueueItem{
		{ProductionID: 102, AllianceHelpRequested: true},
		{ProductionID: 103, AllianceHelpRequested: true},
		{ProductionID: 104},
		{ProductionID: 105},
	}
	castle.Production[hospitalLineID] = queue
	snapshot.State.Castles[77] = castle

	decision, err := NewHospitalPolicy().Evaluate(t.Context(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "3 outstanding hospital alliance-help requests") {
		t.Fatalf("alliance-help capacity decision = %#v", decision)
	}
}

func hospitalPolicySnapshot(t *testing.T, now time.Time) Snapshot {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1940,"hospitalSlots":"5"}],
		"units":[{"wodID":215}],
		"subscriptionsBuffs":[{"subscriptionTypeID":"1","effects":"189&40"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatalf("decode game data: %v", err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID:    77,
		Units: State.CastleUnits{Hospital: map[State.UnitID]int64{215: 100}},
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 1940, Level: 11},
		},
		Production: map[int]State.ProductionQueue{
			hospitalLineID: {LineID: hospitalLineID, Capacity: 5, ObservedAt: now},
		},
	}
	return Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoHospital": json.RawMessage(`{"checkIntervalSec":60}`),
		}},
		GameData: gameData,
		Now:      now,
	}
}

func hospitalIntentAmount(t *testing.T, decision Decision) int64 {
	t.Helper()
	var arguments struct {
		Amount int64 `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatalf("decode intent arguments: %v", err)
	}
	return arguments.Amount
}

func allianceHelpProductionID(t *testing.T, decision Decision) int64 {
	t.Helper()
	var arguments struct {
		ProductionID int64 `json:"productionId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatalf("decode intent arguments: %v", err)
	}
	return arguments.ProductionID
}

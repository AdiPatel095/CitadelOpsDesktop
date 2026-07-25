package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestCraftingPolicyRepairsUnavailableRecipeLevel(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":6,"JSONKey":"C","name":"coal"}],
		"currencies":[],
		"craftingRecipes":[
			{"craftingRecipeId":102,"queueTypeId":1,"recipeGroupID":4,"researchGroupID":9,"level":2,"type":"Short","costC":1000},
			{"craftingRecipeId":103,"queueTypeId":1,"recipeGroupID":4,"researchGroupID":9,"level":3,"type":"Short","costC":2000}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	castle := State.CastleState{
		ID: 20, Name: "Main", KingdomID: 0, SlotType: 1,
		Resources: map[State.ResourceID]State.ResourceBalance{6: {Amount: 5_000}},
		Crafting: State.CraftingState{
			Buildings: map[State.BuildingInstanceID]State.CraftingBuilding{
				200: {
					CastleID: 20, InstanceID: 200, DefinitionID: 1, QueueTypeID: 1,
					Active: []State.CraftingQueueItem{}, Queued: []State.CraftingQueueItem{}, ObservedAt: now,
				},
			},
			EnabledRecipeIDs:       []int64{102},
			EnabledRecipeGroupIDs:  []int64{4},
			OutputBoostByQueueType: map[int]float64{},
		},
	}
	gameState.Castles[castle.ID] = castle
	rawSettings := json.RawMessage(`{
		"castles":{"20":{"buildings":{"1":{
			"enabled":true,"steps":[{"recipeID":103,"repeat":1}],"cursor":0
		}}}}
	}`)
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": rawSettings,
		}},
		GameData: store,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.start" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var start struct {
		RecipeID int64 `json:"recipeId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &start); err != nil {
		t.Fatal(err)
	}
	if start.RecipeID != 102 {
		t.Fatalf("resolved recipe = %d, want 102", start.RecipeID)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
		t.Fatalf("missing repair follow-up: %+v", decision)
	}
	var followUp struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(decision.FollowUp.Arguments, &followUp); err != nil {
		t.Fatal(err)
	}
	var updated craftingSettings
	if err := json.Unmarshal(followUp.Value, &updated); err != nil {
		t.Fatal(err)
	}
	if got := updated.Castles["20"].Buildings["1"].Steps[0].RecipeID; got != 102 {
		t.Fatalf("persisted recipe = %d, want 102", got)
	}
}

func TestCraftingPolicyQueuesAffordableFallbackWithoutBreakingCycle(t *testing.T) {
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":6,"JSONKey":"C","name":"coal"}],
		"currencies":[],
		"craftingRecipes":[
			{"craftingRecipeId":100,"queueTypeId":1,"recipeGroupID":1,"level":1,"type":"Short","costC":1000},
			{"craftingRecipeId":101,"queueTypeId":1,"recipeGroupID":2,"level":1,"type":"Short","costC":100}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 7, 22, 18, 30, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[20] = State.CastleState{
		ID: 20, Name: "Main", KingdomID: 0, SlotType: 1,
		Resources: map[State.ResourceID]State.ResourceBalance{6: {Amount: 500}},
		Crafting: State.CraftingState{
			Buildings: map[State.BuildingInstanceID]State.CraftingBuilding{
				200: {
					CastleID: 20, InstanceID: 200, DefinitionID: 1, QueueTypeID: 1,
					Active: []State.CraftingQueueItem{}, Queued: []State.CraftingQueueItem{}, ObservedAt: now,
				},
			},
			EnabledRecipeIDs:       []int64{100, 101},
			EnabledRecipeGroupIDs:  []int64{1, 2},
			OutputBoostByQueueType: map[int]float64{},
		},
	}
	rawSettings := json.RawMessage(`{
		"castles":{"20":{"buildings":{"1":{
			"enabled":true,"steps":[{"recipeID":100,"repeat":1},{"recipeID":101,"repeat":1}],"cursor":0
		}}}}
	}`)

	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": rawSettings,
		}},
		GameData: store,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.start" {
		t.Fatalf("fallback crafting decision = %+v", decision)
	}
	var start struct {
		RecipeID int64 `json:"recipeId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &start); err != nil {
		t.Fatal(err)
	}
	if start.RecipeID != 101 {
		t.Fatalf("queued recipe = %d, want affordable fallback 101", start.RecipeID)
	}
	if !strings.Contains(decision.Detail, "earlier cycle recipe 100 waits") {
		t.Fatalf("fallback detail = %q", decision.Detail)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
		t.Fatalf("fallback did not advance the cycle: %+v", decision)
	}
	var followUp struct {
		Value json.RawMessage `json:"value"`
	}
	if err := json.Unmarshal(decision.FollowUp.Arguments, &followUp); err != nil {
		t.Fatal(err)
	}
	var updated craftingSettings
	if err := json.Unmarshal(followUp.Value, &updated); err != nil {
		t.Fatal(err)
	}
	if cursor := updated.Castles["20"].Buildings["1"].Cursor; cursor != 0 {
		t.Fatalf("fallback cursor = %d, want top-priority recipe retried next", cursor)
	}

	castle := gameState.Castles[20]
	castle.Resources[6] = State.ResourceBalance{Amount: 2_000}
	gameState.Castles[20] = castle
	decision, err = NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": rawSettings,
		}},
		GameData: store,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.start" {
		t.Fatalf("priority crafting decision = %+v", decision)
	}
	if err := json.Unmarshal(decision.Request.Arguments, &start); err != nil {
		t.Fatal(err)
	}
	if start.RecipeID != 100 {
		t.Fatalf("queued recipe = %d, want top-priority affordable recipe 100", start.RecipeID)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "config.update" {
		t.Fatalf("priority recipe did not advance the cycle: %+v", decision)
	}
	followUp.Value = nil
	updated = craftingSettings{}
	if err := json.Unmarshal(decision.FollowUp.Arguments, &followUp); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(followUp.Value, &updated); err != nil {
		t.Fatal(err)
	}
	if cursor := updated.Castles["20"].Buildings["1"].Cursor; cursor != 1 {
		t.Fatalf("priority cursor = %d, want next lower recipe for alternation", cursor)
	}
}

func TestCraftingPolicyPrefersLaterCraftableCastleOverEarlierResourceWait(t *testing.T) {
	now := time.Date(2026, 7, 25, 13, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	waiting := craftingLogisticsCastle(10, 1, 10, 10)
	ready := craftingLogisticsCastle(20, 2, 20, 20)
	capacity := float64(100_000)
	waiting.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	ready.Resources[6] = State.ResourceBalance{Amount: 20_000, Capacity: &capacity}
	waiting.Crafting.Buildings[100] = State.CraftingBuilding{
		CastleID: waiting.ID, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
	}
	ready.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: ready.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
	}
	gameState.Castles[waiting.ID] = waiting
	gameState.Castles[ready.ID] = ready
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{
		KingdomID: waiting.KingdomID, RemainingSec: 3_600,
	}}
	gameState.KingdomTransport.Unlocks[waiting.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: waiting.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"autoKingdomTransport":true,
			"castles":{
				"10":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}},
				"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}
			}
		}`),
	}}

	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.start" {
		t.Fatalf("earlier resource wait blocked a later craftable castle: %+v", decision)
	}
	var arguments struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.CastleID != ready.ID {
		t.Fatalf("crafting start castle = %d, want %d", arguments.CastleID, ready.ID)
	}
}

func TestCraftingPolicyRefreshesStaleSnapshotBeforeUsingQueues(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	staleAt := now.Add(-6 * time.Minute)
	code := 0
	gameState := State.NewGameState()
	gameState.Castles[20] = State.CastleState{
		ID: 20, KingdomID: 0, SlotType: 1,
		Crafting: State.CraftingState{Buildings: map[State.BuildingInstanceID]State.CraftingBuilding{
			200: {CastleID: 20, InstanceID: 200, QueueTypeID: 1, ObservedAt: staleAt},
		}},
	}
	gameState.Observations["crin"] = State.ProtocolObservation{
		Opcode: "crin", LastDirection: "inbound", LastCode: &code, LastSeenAt: staleAt,
	}
	settings := json.RawMessage(`{"checkIntervalSec":300,"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":6}]}}}}}`)

	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": settings,
		}},
		Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.refresh" {
		t.Fatalf("stale crafting decision = %+v", decision)
	}
	if !decision.ReevaluateOnSuccess {
		t.Fatal("crafting refresh did not request response-driven continuation")
	}
}

func TestCraftingPolicyRefreshesSnapshotFromPreviousSession(t *testing.T) {
	now := time.Date(2026, 7, 19, 12, 0, 0, 0, time.UTC)
	observedAt := now.Add(-time.Minute)
	code := 0
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now
	gameState.Castles[20] = State.CastleState{
		ID: 20, KingdomID: 0, SlotType: 1,
		Crafting: State.CraftingState{Buildings: map[State.BuildingInstanceID]State.CraftingBuilding{
			200: {CastleID: 20, InstanceID: 200, QueueTypeID: 1, ObservedAt: observedAt},
		}},
	}
	gameState.Observations["crin"] = State.ProtocolObservation{
		Opcode: "crin", LastDirection: "inbound", LastCode: &code, LastSeenAt: observedAt,
	}
	settings := json.RawMessage(`{"checkIntervalSec":300,"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":6}]}}}}}`)

	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoSceatResources": settings,
		}},
		Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.refresh" {
		t.Fatalf("previous-session crafting decision = %+v", decision)
	}
}

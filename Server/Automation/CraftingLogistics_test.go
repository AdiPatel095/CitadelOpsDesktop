package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestCraftingPolicyWakesWhenTowerLootChangesResources(t *testing.T) {
	for _, domain := range NewCraftingLogisticsPolicy().WakeDomains() {
		if domain == "resources" {
			return
		}
	}
	t.Fatal("crafting policy does not wake when returned tower loot changes castle resources")
}

func TestCraftingLogisticsPolicyRunsAsIndependentAutoSceatLane(t *testing.T) {
	policy := NewCraftingLogisticsPolicy()
	if policy.ID() == NewCraftingPolicy().ID() {
		t.Fatal("crafting and logistics share one coordinator runtime")
	}
	if policy.EnabledKey() != NewCraftingPolicy().EnabledKey() {
		t.Fatalf("logistics enabled key = %q", policy.EnabledKey())
	}
	if policyScheduleKey(policy) != NewCraftingPolicy().ID() || policyActorID(policy) != NewCraftingPolicy().ID() {
		t.Fatalf("logistics feature controls = schedule %q actor %q", policyScheduleKey(policy), policyActorID(policy))
	}
}

func TestCraftingPolicyWaitsForMarketBarrowReturnBeforeLogisticsRefresh(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	returnsAt := now.Add(10 * time.Minute)
	gameState := State.NewGameState()
	gameState.Player.ID = 1
	source := craftingLogisticsCastle(10, 0, 10, 10)
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	gameState.Castles[source.ID] = source
	gameState.Castles[20] = craftingLogisticsCastle(20, 0, 20, 20)
	gameState.Market.ObservedAt = now.Add(-10 * time.Minute)
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	gameState.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: source.ID,
		MarketBarrows: 100, ReturnsAt: &returnsAt,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{"checkIntervalSec":300,"autoKingdomTransport":true}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Request != nil || !decision.NextCheckAt.Equal(returnsAt.Add(time.Second)) {
		t.Fatalf("market lease decision = %+v", decision)
	}
}

func TestCraftingPolicyShipsMissingResourceAcrossKingdoms(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameData := craftingLogisticsGameData(t)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 50_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, SlotCount: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{}, Queued: []State.CraftingQueueItem{},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[1] = State.KingdomTransportUnlock{KingdomID: 1, Unlocked: true}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,"useStormBuffer":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}],"cursor":0}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: gameData, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var arguments struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.SourceCastleID != 10 || arguments.TargetCastleID != 20 || arguments.ResourceID != 6 || arguments.Amount != 10_000 {
		t.Fatalf("unexpected shipment arguments: %+v", arguments)
	}
}

func TestCraftingPolicyShipsMissingResourceWithinKingdomBelowKingdomMinimum(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 0, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 50_000, Capacity: &capacity}
	source.Buildings[100] = State.Building{InstanceID: 100, DefinitionID: 137}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Castles[source.ID] = State.MarketCastleState{
		CastleID: source.ID, KingdomID: source.KingdomID, AvailableBarrows: 100,
	}
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":50000,"sourceReservePercent":10,
			"minimumCoinReserve":9999999,"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" ||
		string(decision.Request.Arguments) != `{"amount":9000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20}` {
		t.Fatalf("below-minimum market shortfall decision = %+v", decision)
	}
}

func TestCraftingPolicyDrainsGreenOutpostLootWhileQueueIsFull(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	source.Name = "Green Outpost"
	source.SlotType = 4
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("full queue did not request a proactive loot-drain shipment: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"amount":20000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20,"workflowOwner":"autoSceatRes"}`; got != want {
		t.Fatalf("loot-drain shipment = %s, want %s", got, want)
	}
	if !decision.ReevaluateOnSuccess {
		t.Fatal("loot-drain shipment did not request response-driven continuation")
	}
}

func TestCraftingPolicyMainCastleDonatesSurplusAfterPreservingOwnRefill(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	source.Name = "Green Main Castle"
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 30_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	source.Crafting.Buildings[100] = State.CraftingBuilding{
		CastleID: source.ID, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,
			"castles":{
				"10":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}},
				"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}
			}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"amount":12000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20,"workflowOwner":"autoSceatRes"}`; got != want {
		t.Fatalf("main-castle surplus shipment = %s, want %s", got, want)
	}
}

func TestCraftingPolicyLootDrainPrefersSovereignResourceKingdom(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	ordinarySource := craftingLogisticsCastle(10, 0, 10, 10)
	towerLootSource := craftingLogisticsCastle(30, 2, 30, 30)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	capacity := float64(100_000)
	ordinarySource.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	towerLootSource.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[ordinarySource.ID] = ordinarySource
	gameState.Castles[towerLootSource.ID] = towerLootSource
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"amount":20000,"resourceId":6,"sourceCastleId":30,"targetCastleId":20,"workflowOwner":"autoSceatRes"}`; got != want {
		t.Fatalf("preferred-source shipment = %s, want %s", got, want)
	}
}

func TestCraftingPolicyLootDrainFallsBackWhenPreferredSourceIsUnavailable(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	fallbackSource := craftingLogisticsCastle(10, 0, 10, 10)
	reservedPreferredSource := craftingLogisticsCastle(30, 2, 30, 30)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	capacity := float64(100_000)
	fallbackSource.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	reservedPreferredSource.Resources[6] = State.ResourceBalance{Amount: 10_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[fallbackSource.ID] = fallbackSource
	gameState.Castles[reservedPreferredSource.ID] = reservedPreferredSource
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"amount":20000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20,"workflowOwner":"autoSceatRes"}`; got != want {
		t.Fatalf("fallback-source shipment = %s, want %s", got, want)
	}
}

func TestCraftingPolicyLootDrainUsesGreenOutpostThroughMarketplace(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 0, 20, 20)
	source.Name = "Green Outpost"
	source.SlotType = 4
	target.Name = "Main Castle"
	target.SlotType = 1
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	source.Buildings[100] = State.Building{InstanceID: 100, DefinitionID: 137}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Castles[source.ID] = State.MarketCastleState{
		CastleID: source.ID, KingdomID: source.KingdomID, AvailableBarrows: 100,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":50000,"sourceReservePercent":10,
			"minimumCoinReserve":9999999,"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"amount":10100,"resourceId":6,"sourceCastleId":10,"targetCastleId":20}`; got != want {
		t.Fatalf("green-outpost drain shipment = %s, want %s", got, want)
	}
}

func TestCraftingPolicyLootDrainUsesOwnedCapitalAndMetropolisSources(t *testing.T) {
	for _, test := range []struct {
		name     string
		slotType int
	}{
		{name: "capital", slotType: 3},
		{name: "alternate capital", slotType: 6},
		{name: "metropolis", slotType: 5},
		{name: "trading metropolis", slotType: 22},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
			gameState := State.NewGameState()
			source := craftingLogisticsCastle(10, 2, 10, 10)
			target := craftingLogisticsCastle(20, 1, 20, 20)
			source.Name = test.name
			source.SlotType = test.slotType
			capacity := float64(100_000)
			source.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
			target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
			target.Crafting.Buildings[200] = State.CraftingBuilding{
				CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
				Active: []State.CraftingQueueItem{{RecipeID: 100}},
				Queued: []State.CraftingQueueItem{{RecipeID: 100}},
			}
			gameState.Castles[source.ID] = source
			gameState.Castles[target.ID] = target
			gameState.KingdomTransport.ObservedAt = now
			gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
				KingdomID: target.KingdomID, Unlocked: true,
			}
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				"automation.autoSceatResources": json.RawMessage(`{
					"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
					"autoKingdomTransport":true,
					"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
				}`),
			}}

			decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Request == nil || decision.Request.Name != "resource.ship" {
				t.Fatalf("owned %s did not contribute to the crafting buffer: %+v", test.name, decision)
			}
			if got, want := string(decision.Request.Arguments), `{"amount":20000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20,"workflowOwner":"autoSceatRes"}`; got != want {
				t.Fatalf("owned-%s drain shipment = %s, want %s", test.name, got, want)
			}
		})
	}
}

func TestCraftingPolicyStorageNodesNeverCreateQueuesOrRefillDemand(t *testing.T) {
	for _, test := range []struct {
		name      string
		kingdomID State.KingdomID
		slotType  int
	}{
		{name: "green outpost", kingdomID: 0, slotType: 4},
		{name: "capital", kingdomID: 0, slotType: 3},
		{name: "alternate capital", kingdomID: 2, slotType: 6},
		{name: "metropolis", kingdomID: 1, slotType: 5},
		{name: "trading metropolis", kingdomID: 3, slotType: 22},
		{name: "storm castle", kingdomID: 4, slotType: 12},
	} {
		t.Run(test.name, func(t *testing.T) {
			now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
			gameState := State.NewGameState()
			storage := craftingLogisticsCastle(10, test.kingdomID, 10, 10)
			storage.Name = test.name
			storage.SlotType = test.slotType
			capacity := float64(100_000)
			storage.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
			storage.Crafting.Buildings[100] = State.CraftingBuilding{
				CastleID: storage.ID, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
			}
			gameState.Castles[storage.ID] = storage
			raw := json.RawMessage(`{
				"checkIntervalSec":300,
				"castles":{"10":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
			}`)
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				"automation.autoSceatResources": raw,
			}}
			snapshot := Snapshot{
				State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
			}

			decision, err := NewCraftingPolicy().Evaluate(t.Context(), snapshot)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Request != nil {
				t.Fatalf("storage node %s opened a crafting queue: %+v", test.name, decision)
			}
			var settings craftingSettings
			if err := json.Unmarshal(raw, &settings); err != nil {
				t.Fatal(err)
			}
			if demands := craftingRefillDemands(settings, snapshot); len(demands) != 0 {
				t.Fatalf("storage node %s created refill demand: %#v", test.name, demands)
			}
		})
	}
}

func TestCraftingPolicyLootDrainWaitsForPendingKingdomShipment(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 80_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{
		KingdomID: target.KingdomID, RemainingSec: 60,
	}}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"autoKingdomTransport":true,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("pending kingdom shipment did not block loot drain: %+v", decision)
	}
}

func TestCraftingPolicyRedistributesKhanLootAsOneCapacityFillingShipment(t *testing.T) {
	now := time.Date(2026, 7, 25, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 1, 20, 20)
	source.Name = "Green Main Castle"
	target.Name = "Configured Kingdom Castle"
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	source.Resources[7] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	source.Resources[8] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 20_000, Capacity: &capacity}
	target.Resources[7] = State.ResourceBalance{Amount: 30_000, Capacity: &capacity}
	target.Resources[8] = State.ResourceBalance{Amount: 40_000, Capacity: &capacity}
	source.Crafting.Buildings[100] = State.CraftingBuilding{
		CastleID: source.ID, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Player.Currencies[50] = 2
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"checkIntervalSec":300,"minimumShipmentSize":10000,"sourceReservePercent":10,
			"overflowThresholdPercent":90,"autoKingdomTransport":true,
			"useKingdomTimeSkips":true,"allowedTimeSkips":["MS5"],
			"castles":{
				"10":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}},
				"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}
			}
		}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingMultiResourceLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("covered refill buffers blocked Khan-loot redistribution: %+v", decision)
	}
	if got, want := string(decision.Request.Arguments), `{"goods":[{"resourceId":6,"amount":80000},{"resourceId":7,"amount":70000},{"resourceId":8,"amount":60000}],"minimumRemaining":0,"sourceCastleId":10,"targetCastleId":20,"timeSkipId":"MS5","workflowOwner":"autoSceatRes"}`; got != want {
		t.Fatalf("Khan-loot redistribution = %s, want %s", got, want)
	}
	if !decision.ReevaluateOnSuccess {
		t.Fatal("Khan-loot redistribution did not request response-driven continuation")
	}

	source.Resources[6] = State.ResourceBalance{Amount: 20_000, Capacity: &capacity}
	source.Resources[7] = State.ResourceBalance{Amount: 30_000, Capacity: &capacity}
	source.Resources[8] = State.ResourceBalance{Amount: 40_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	target.Resources[7] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	target.Resources[8] = State.ResourceBalance{Amount: 100_000, Capacity: &capacity}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	next, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingMultiResourceLogisticsGameData(t), Now: now.Add(time.Second),
	})
	if err != nil {
		t.Fatal(err)
	}
	if next.Request != nil && next.Request.Name == "resource.ship" {
		t.Fatalf("configured destination became a return donor: %+v", next)
	}
}

func TestCraftingPolicyDoesNotRequireMarketForSingleCastleKingdoms(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	mainCastle := craftingLogisticsCastle(10, 0, 10, 10)
	mainCastle.Buildings[100] = State.Building{InstanceID: 100, DefinitionID: 137}
	dungeonCastle := craftingLogisticsCastle(20, 3, 20, 20)
	gameState.Castles[mainCastle.ID] = mainCastle
	gameState.Castles[dungeonCastle.ID] = dungeonCastle
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{"autoKingdomTransport":true}`),
	}}

	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("single-castle kingdoms unexpectedly requested logistics refresh: %#v", decision)
	}
}

func TestCraftingPolicyUsesSmallestCoveringKingdomTimeSkip(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{KingdomID: 1, RemainingSec: 350}}
	gameState.KingdomTransport.ResourceWorkflows[1] = State.KingdomResourceTransportWorkflow{
		Owner: autoSceatTransportOwner, KingdomID: 1, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now,
	}
	gameState.Player.Currencies[50] = 2
	gameState.Player.Currencies[51] = 2
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"useKingdomTimeSkips":true,
			"allowedTimeSkips":["MS5","MS3"],
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}
	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.skip" || string(decision.Request.Arguments) != `{"minimumRemaining":0,"targetKingdomId":1,"timeSkipId":"MS3"}` {
		t.Fatalf("unexpected time-skip decision: %+v", decision)
	}
}

func TestCraftingPolicyRentsConfiguredSlotOnlyWhenNextRecipeIsAffordable(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	castle := craftingLogisticsCastle(20, 0, 20, 20)
	capacity := float64(100_000)
	castle.Resources[6] = State.ResourceBalance{Amount: 20_000, Capacity: &capacity}
	castle.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: castle.ID, InstanceID: 200, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 100}}, Queued: []State.CraftingQueueItem{{RecipeID: 100}},
		ActiveSlotRentals: []int{}, QueueSlotRentals: []int{},
	}
	gameState.Castles[castle.ID] = castle
	gameState.Player.Resources[1] = 6_000_000
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"minimumCoinReserve":100000,"castles":{"20":{"buildings":{"1":{
				"enabled":true,"steps":[{"recipeID":100,"repeat":1}],"autoRentQueueSlots":1
			}}}}
		}`),
	}}
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.rent_slot" || string(decision.Request.Arguments) != `{"buildingInstanceId":200,"castleId":20,"slot":1,"slotType":"queue"}` {
		t.Fatalf("unexpected rental decision: %+v", decision)
	}
}

func TestCraftingPolicyMovesSameKingdomOverflowBelowKingdomMinimum(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 0, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 95_000, Capacity: &capacity}
	source.Buildings[100] = State.Building{InstanceID: 100, DefinitionID: 137}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Player.Resources[1] = 1_000_000
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Castles[source.ID] = State.MarketCastleState{
		CastleID: source.ID, KingdomID: source.KingdomID, AvailableBarrows: 100,
	}
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"minimumShipmentSize":50000,"overflowThresholdPercent":90
		}`),
	}}
	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" || string(decision.Request.Arguments) != `{"amount":5000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20}` {
		t.Fatalf("unexpected market overflow decision: %+v", decision)
	}
	if !decision.ReevaluateOnSuccess {
		t.Fatal("market overflow shipment did not request response-driven continuation")
	}
}

func TestCraftingPolicyRequiresMarketplaceForSameKingdomShipment(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 0, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 50_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	target.Crafting.Buildings[200] = State.CraftingBuilding{
		CastleID: target.ID, InstanceID: 200, QueueTypeID: 1, SlotCount: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{}, Queued: []State.CraftingQueueItem{},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Castles[source.ID] = State.MarketCastleState{
		CastleID: source.ID, KingdomID: source.KingdomID, AvailableBarrows: 100,
	}
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"minimumShipmentSize":1000,
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}
	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("expected no market shipment without a marketplace, got %+v", decision)
	}
}

func TestCraftingPolicyRequiresMarketplaceForOverflowShipment(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	target := craftingLogisticsCastle(20, 0, 20, 20)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 95_000, Capacity: &capacity}
	target.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Player.Resources[1] = 1_000_000
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.Market.Castles[source.ID] = State.MarketCastleState{
		CastleID: source.ID, KingdomID: source.KingdomID, AvailableBarrows: 100,
	}
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"minimumShipmentSize":1000,"overflowThresholdPercent":90
		}`),
	}}
	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("expected no overflow shipment without a marketplace, got %+v", decision)
	}
}

func TestCraftingPolicyMovesBlockedOverflowToStorm(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := craftingLogisticsCastle(10, 0, 10, 10)
	storm := craftingLogisticsCastle(40, 4, 40, 40)
	capacity := float64(100_000)
	source.Resources[6] = State.ResourceBalance{Amount: 95_000, Capacity: &capacity}
	storm.Resources[6] = State.ResourceBalance{Amount: 0, Capacity: &capacity}
	gameState.Castles[source.ID] = source
	gameState.Castles[storm.ID] = storm
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"useStormBuffer":true,"minimumShipmentSize":1000,"overflowThresholdPercent":90
		}`),
	}}
	decision, err := NewCraftingLogisticsPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" || string(decision.Request.Arguments) != `{"amount":5000,"resourceId":6,"sourceCastleId":10,"targetCastleId":40,"workflowOwner":"autoSceatRes"}` {
		t.Fatalf("unexpected Storm overflow decision: %+v", decision)
	}
}

func TestCraftingPolicyRubySkipsOneCraftWhenOverflowCannotMove(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	remaining := 50
	gameState := State.NewGameState()
	main := craftingLogisticsCastle(10, 0, 10, 10)
	main.SlotType = 1
	capacity := float64(100_000)
	main.Resources[6] = State.ResourceBalance{Amount: 95_000, Capacity: &capacity}
	main.Crafting.Buildings[100] = State.CraftingBuilding{
		CastleID: main.ID, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
		Active: []State.CraftingQueueItem{{RecipeID: 101, RemainingSec: &remaining}},
		Queued: []State.CraftingQueueItem{{RecipeID: 100}},
	}
	gameState.Castles[main.ID] = main
	gameState.Player.Resources[2] = 100
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"useRubyOverflowSkip":true,
			"minimumShipmentSize":1000,"overflowThresholdPercent":90,
			"castles":{"10":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "crafting.skip" || string(decision.Request.Arguments) != `{"buildingInstanceId":100,"castleId":10,"slot":0}` {
		t.Fatalf("unexpected ruby overflow decision: %+v", decision)
	}
}

func craftingLogisticsGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1},{"wodID":137,"name":"Market","marketCarriages":5}],"units":[{"wodID":1}],
		"constructionItems":[{"constructionItemID":1}],
		"resources":[
			{"resourceID":1,"JSONKey":"C1","name":"currency1"},
			{"resourceID":2,"JSONKey":"C2","name":"currency2"},
			{"resourceID":6,"JSONKey":"C","name":"coal"}
		],
		"currencies":[
			{"currencyID":50,"JSONKey":"MS5","Name":"hourSkip"},
			{"currencyID":51,"JSONKey":"MS3","Name":"tenMinuteSkip"}
		],
		"craftingRecipes":[
			{"craftingRecipeId":100,"queueTypeId":1,"type":"Green","costC":9000,"craftingDuration":100,"skipCostC2":20},
			{"craftingRecipeId":101,"queueTypeId":1,"type":"Green","costC":1000,"craftingDuration":100,"skipCostC2":20}
		],
		"levelBoosters":[{"boosterType":11,"level":0,"boostPercentage":0}],
		"effects":[{"effectID":90,"name":"marketCarriageCapacityBoost"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func craftingMultiResourceLogisticsGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1},{"wodID":137,"name":"Market","marketCarriages":5}],"units":[{"wodID":1}],
		"constructionItems":[{"constructionItemID":1}],
		"resources":[
			{"resourceID":1,"JSONKey":"C1","name":"currency1"},
			{"resourceID":2,"JSONKey":"C2","name":"currency2"},
			{"resourceID":6,"JSONKey":"C","name":"coal"},
			{"resourceID":7,"JSONKey":"O","name":"oil"},
			{"resourceID":8,"JSONKey":"G","name":"glass"}
		],
		"currencies":[
			{"currencyID":50,"JSONKey":"MS5","Name":"hourSkip"},
			{"currencyID":51,"JSONKey":"MS3","Name":"tenMinuteSkip"}
		],
		"craftingRecipes":[
			{"craftingRecipeId":100,"queueTypeId":1,"type":"Green","costC":9000,"costO":8000,"costG":7000,"craftingDuration":100,"skipCostC2":20}
		],
		"levelBoosters":[{"boosterType":11,"level":0,"boostPercentage":0}],
		"effects":[{"effectID":90,"name":"marketCarriageCapacityBoost"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func craftingLogisticsCastle(id State.CastleID, kingdom State.KingdomID, x int, y int) State.CastleState {
	slotType := 12
	if kingdom == 0 {
		slotType = 1
	}
	return State.CastleState{
		ID: id, KingdomID: kingdom, SlotType: slotType, X: x, Y: y,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Buildings: map[State.BuildingInstanceID]State.Building{},
		Crafting: State.CraftingState{
			Buildings:              map[State.BuildingInstanceID]State.CraftingBuilding{},
			OutputBoostByQueueType: map[int]float64{},
		},
	}
}

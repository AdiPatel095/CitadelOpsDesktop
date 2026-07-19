package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

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

	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: gameData, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.ship" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var arguments struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetKingdom  State.KingdomID  `json:"targetKingdomId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.SourceCastleID != 10 || arguments.TargetKingdom != 1 || arguments.ResourceID != 6 || arguments.Amount != 10_000 {
		t.Fatalf("unexpected shipment arguments: %+v", arguments)
	}
}

func TestCraftingPolicyUsesSmallestCoveringKingdomTimeSkip(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Market.ObservedAt = now
	gameState.Market.CaravanLevelLoaded = true
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{KingdomID: 1, RemainingSec: 350}}
	gameState.Player.Currencies[50] = 2
	gameState.Player.Currencies[51] = 2
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoSceatResources": json.RawMessage(`{
			"autoKingdomTransport":true,"useKingdomTimeSkips":true,
			"allowedTimeSkips":["MS5","MS3"],
			"castles":{"20":{"buildings":{"1":{"enabled":true,"steps":[{"recipeID":100,"repeat":1}]}}}}
		}`),
	}}
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.skip" || string(decision.Request.Arguments) != `{"targetKingdomId":1,"timeSkipId":"MS3"}` {
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

func TestCraftingPolicyMovesSameKingdomOverflowIntoFreeStorage(t *testing.T) {
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
			"autoKingdomTransport":true,"minimumShipmentSize":1000,"overflowThresholdPercent":90
		}`),
	}}
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.market.ship" || string(decision.Request.Arguments) != `{"amount":5000,"resourceId":6,"sourceCastleId":10,"targetCastleId":20}` {
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
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
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
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
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
	decision, err := NewCraftingPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: craftingLogisticsGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.ship" || string(decision.Request.Arguments) != `{"amount":5000,"resourceId":6,"sourceCastleId":10,"targetKingdomId":4}` {
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

func craftingLogisticsCastle(id State.CastleID, kingdom State.KingdomID, x int, y int) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, X: x, Y: y,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Buildings: map[State.BuildingInstanceID]State.Building{},
		Crafting: State.CraftingState{
			Buildings:              map[State.BuildingInstanceID]State.CraftingBuilding{},
			OutputBoostByQueueType: map[int]float64{},
		},
	}
}

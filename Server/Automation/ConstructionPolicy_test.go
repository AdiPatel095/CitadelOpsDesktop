package Automation

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestConstructionPolicyUpgradesDueOfficialTier(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	remaining := 200
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining, Level: 1}}
	gameState.Castles[10] = castle
	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.upgrade" || string(decision.Request.Arguments) != `{"buildingInstanceId":100,"castleId":10,"constructionItemId":101,"offerCode":2000,"slot":0}` {
		t.Fatalf("unexpected upgrade decision: %+v", decision)
	}
}

func TestConstructionPolicyWaitsUntilFiveMinutesRemainBeforeUpgrade(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	remaining := 301
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining, Level: 1}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("unexpected early upgrade decision: %+v", decision)
	}
	if want := now.Add(time.Second); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("next check = %v, want %v", decision.NextCheckAt, want)
	}
}

func TestConstructionPolicyEquipsLowestAvailableConfiguredTier(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	gameState.Inventory.ConstructionItems[102] = 1
	gameState.Inventory.ConstructionItems[103] = 1
	gameState.Inventory.ConstructionItems[104] = 1
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.constructionItems": json.RawMessage(`{"targets":{"10":[{"id":101,"minLevel":2,"amount":4}]}}`),
	}}

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration,
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.equip" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var request struct {
		ConstructionItemID State.ConstructionItemID `json:"constructionItemId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.ConstructionItemID != 102 {
		t.Fatalf("construction item = %d, want lowest configured tier 102", request.ConstructionItemID)
	}
}

func TestConstructionPolicyPurchasesMissingTierFromLiveOfficialOffer(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.ReplaceInventoryConstructionOffers(map[State.PackageID]int64{500: 1}, now, 10, 0)
	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.purchase" || string(decision.Request.Arguments) != `{"amount":1,"castleId":10,"productId":500}` {
		t.Fatalf("unexpected purchase decision: %+v", decision)
	}
	if decision.FollowUp == nil || decision.FollowUp.Name != "construction.inventory.refresh" {
		t.Fatalf("purchase inventory follow-up = %+v", decision.FollowUp)
	}
}

func TestConstructionPolicyPurchasesOfficialTrivialTierOutsideLiveOffers(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.ReplaceInventoryConstructionOffers(map[State.PackageID]int64{}, now, 10, 0)

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.purchase" || string(decision.Request.Arguments) != `{"amount":1,"castleId":10,"productId":501}` {
		t.Fatalf("unexpected trivial purchase decision: %+v", decision)
	}
}

func TestConstructionPolicyWaitsWhenInventoryIsFull(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[201] = State.ConstructionItemInventoryLimit
	gameState.ReplaceInventoryConstructionOffers(map[State.PackageID]int64{}, now, 10, 0)

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, fmt.Sprintf("inventory is full (%d/%d)", State.ConstructionItemInventoryLimit, State.ConstructionItemInventoryLimit)) {
		t.Fatalf("full inventory decision = %+v", decision)
	}
	if want := now.Add(constructionCheckInterval); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("next check = %v, want %v", decision.NextCheckAt, want)
	}
}

func TestConstructionPolicyPreservesSelectedVariantWhenGroupIDIsReused(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	gameState.Inventory.ConstructionItems[301] = 1
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.constructionItems": json.RawMessage(`{"targets":{"10":[{"id":101,"amount":4}]}}`),
	}}
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":200,"constructionItemGroupIDs":"1"}],
		"units":[{"wodID":1}],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":1,"name":"AnniversaryDwelling","duration":3600,"effects":"10&5+0","level":1,"slotTypeID":1},
			{"constructionItemID":102,"constructionItemGroupID":1,"name":"AnniversaryDwelling","duration":3600,"effects":"10&10+0","level":2,"slotTypeID":1},
			{"constructionItemID":301,"constructionItemGroupID":1,"name":"BlackFridayDwelling","duration":3600,"effects":"10&20+0","level":4,"slotTypeID":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: store, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.equip" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
	var request struct {
		ConstructionItemID State.ConstructionItemID `json:"constructionItemId"`
		Slot               int                      `json:"slot"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.ConstructionItemID != 101 {
		t.Fatalf("construction item = %d, want selected variant tier 101", request.ConstructionItemID)
	}
	if request.Slot != 1 {
		t.Fatalf("construction slot = %d, want catalog slot 1", request.Slot)
	}
}

func TestConstructionPolicyUsesFreeTargetSlotWhenDifferentSlotIsOccupied(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 201, Slot: 0}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.equip" {
		t.Fatalf("unexpected decision: %+v", decision)
	}
}

func TestConstructionPolicyUpgradesTowardFloorOneTierAtATime(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	remaining := 200
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.constructionItems": json.RawMessage(`{"targets":{"10":[{"id":101,"minLevel":3,"amount":4}]}}`),
	}}
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":200,"constructionItemGroupIDs":"1"}],
		"units":[{"wodID":1}],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":1,"slotTypeID":0},
			{"constructionItemID":102,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":2,"slotTypeID":0},
			{"constructionItemID":103,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":3,"slotTypeID":0},
			{"constructionItemID":104,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":4,"slotTypeID":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: store, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.upgrade" || !strings.Contains(string(decision.Request.Arguments), `"offerCode":2000`) {
		t.Fatalf("unexpected stepwise upgrade decision: %+v", decision)
	}
}

func TestConstructionPolicyWaitsForOccupiedTemporarySlotBeforeBuying(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.ReplaceInventoryConstructionOffers(map[State.PackageID]int64{500: 1}, now, 10, 0)
	remaining := 600
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 301, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || !strings.Contains(decision.Detail, "occupied construction slot") {
		t.Fatalf("unexpected occupied-slot decision: %+v", decision)
	}
	if want := now.Add(constructionCheckInterval); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("next check = %v, want %v", decision.NextCheckAt, want)
	}
}

func TestConstructionPolicyUsesElapsedSlotTimeForUpgrade(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	remaining := 600
	castle := gameState.Castles[10]
	castle.ConstructionSlotsObservedAt = now.Add(-6 * time.Minute)
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 101, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.upgrade" {
		t.Fatalf("unexpected elapsed-time decision: %+v", decision)
	}
}

func TestConstructionPolicyWaitsForAuthoritativeRemovalOfElapsedTemporaryItem(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	remaining := 120
	castle := gameState.Castles[10]
	castle.ConstructionSlotsObservedAt = now.Add(-3 * time.Minute)
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 102, Slot: 0, RemainingSec: &remaining}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || !strings.Contains(decision.Detail, "occupied construction slot") {
		t.Fatalf("unexpected elapsed-item decision: %+v", decision)
	}
	if want := now.Add(constructionCheckInterval); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("next check = %v, want %v", decision.NextCheckAt, want)
	}
}

func TestConstructionPolicyTreatsEveryAttachedTargetSlotAsOccupied(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	zero := 0
	tests := []struct {
		name      string
		itemID    State.ConstructionItemID
		remaining *int
	}{
		{name: "permanent missing timer", itemID: 202},
		{name: "permanent zero timer", itemID: 202, remaining: &zero},
		{name: "temporary zero timer", itemID: 301, remaining: &zero},
		{name: "unknown definition", itemID: 999999, remaining: &zero},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState := constructionPolicyState(now)
			gameState.Inventory.ConstructionItems[101] = 1
			castle := gameState.Castles[10]
			castle.ConstructionSlots[100] = []State.ConstructionSlot{{
				DefinitionID: test.itemID, Slot: 0, RemainingSec: test.remaining,
			}}
			gameState.Castles[10] = castle

			decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: constructionPolicyConfiguration(),
				GameData: constructionPolicyGameData(t), Now: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Request != nil || !strings.Contains(decision.Detail, "refreshed slot snapshot must confirm removal") {
				t.Fatalf("attached-slot decision = %+v", decision)
			}
		})
	}
}

func TestConstructionPolicyUsesAnotherGenuinelyFreeCompatibleBuilding(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	zero := 0
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	castle := gameState.Castles[10]
	castle.Buildings[101] = State.Building{InstanceID: 101, DefinitionID: 200}
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 301, Slot: 0, RemainingSec: &zero}}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "construction.equip" ||
		!strings.Contains(string(decision.Request.Arguments), `"buildingInstanceId":101`) {
		t.Fatalf("alternate-host decision = %+v", decision)
	}
}

func TestConstructionPolicyEquipsAfterRefreshedSnapshotConfirmsVacancy(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	zero := 0
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionItems[101] = 1
	castle := gameState.Castles[10]
	castle.ConstructionSlots[100] = []State.ConstructionSlot{{DefinitionID: 301, Slot: 0, RemainingSec: &zero}}
	gameState.Castles[10] = castle

	blocked, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil || blocked.Request != nil {
		t.Fatalf("pre-refresh decision = %+v, err = %v", blocked, err)
	}

	castle = gameState.Castles[10]
	castle.ConstructionSlots[100] = nil
	castle.ConstructionSlotsObservedAt = now.Add(time.Minute)
	gameState.Castles[10] = castle
	ready, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now.Add(time.Minute),
	})
	if err != nil {
		t.Fatal(err)
	}
	if ready.Request == nil || ready.Request.Name != "construction.equip" {
		t.Fatalf("post-refresh decision = %+v", ready)
	}
}

func TestConstructionPolicyCapturedExpiredItemsNeverDispatchRPC(t *testing.T) {
	now := time.Date(2026, 9, 16, 5, 32, 59, 0, time.UTC)
	store := capturedConstructionPolicyGameData(t)
	tests := []struct {
		name       string
		buildingID State.BuildingInstanceID
		targetID   State.ConstructionItemID
		slots      []State.ConstructionSlot
	}{
		{
			name: "pirate market 30403 blocks 30401", buildingID: 4094, targetID: 30401,
			slots: []State.ConstructionSlot{{DefinitionID: 214, Slot: 0}, {DefinitionID: 30403, Slot: 0, RemainingSec: intPointerForConstructionTest(0)}},
		},
		{
			name: "pirates woodcutter 30482 blocks 30481", buildingID: 6875, targetID: 30481,
			slots: []State.ConstructionSlot{{DefinitionID: 30482, Slot: 0, RemainingSec: intPointerForConstructionTest(0)}},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Inventory.ConstructionItemsObservedAt = now
			gameState.Inventory.ConstructionItems[test.targetID] = 1
			gameState.Castles[10] = State.CastleState{
				ID: 10, KingdomID: 0, SlotType: 1, Name: "Main",
				Buildings: map[State.BuildingInstanceID]State.Building{
					test.buildingID: {InstanceID: test.buildingID, DefinitionID: 200},
				},
				ConstructionSlots:           map[State.BuildingInstanceID][]State.ConstructionSlot{test.buildingID: test.slots},
				ConstructionSlotsObservedAt: now,
			}
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				"automation.constructionItems": json.RawMessage(fmt.Sprintf(`{"targets":{"10":[{"id":%d,"amount":4}]}}`, test.targetID)),
			}}
			decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, Configuration: configuration, GameData: store, Now: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Request != nil {
				t.Fatalf("captured occupied state produced request: %+v", decision)
			}
		})
	}
}

func intPointerForConstructionTest(value int) *int { return &value }

func capturedConstructionPolicyGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":200,"constructionItemGroupIDs":"1"}],
		"units":[{"wodID":1}],
		"constructionItems":[
			{"constructionItemID":214,"constructionItemGroupID":1,"name":"marketCarriages","level":1,"slotTypeID":1},
			{"constructionItemID":30401,"constructionItemGroupID":1,"name":"pirateMarket","duration":345600,"level":2,"slotTypeID":0},
			{"constructionItemID":30402,"constructionItemGroupID":1,"name":"pirateMarket","duration":345600,"level":3,"slotTypeID":0},
			{"constructionItemID":30403,"constructionItemGroupID":1,"name":"pirateMarket","duration":345600,"level":4,"slotTypeID":0},
			{"constructionItemID":30481,"constructionItemGroupID":1,"name":"piratesWoodcutter","duration":345600,"level":2,"slotTypeID":0},
			{"constructionItemID":30482,"constructionItemGroupID":1,"name":"piratesWoodcutter","duration":345600,"level":3,"slotTypeID":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestConstructionPolicyRefreshesUnobservedSlots(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	castle := gameState.Castles[10]
	castle.ConstructionSlotsObservedAt = time.Time{}
	gameState.Castles[10] = castle

	decision, err := NewConstructionPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: constructionPolicyConfiguration(),
		GameData: constructionPolicyGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("unexpected refresh decision: %+v", decision)
	}
}

func constructionPolicyState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	castle := State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 20,
		Resources:                   map[State.ResourceID]State.ResourceBalance{},
		Buildings:                   map[State.BuildingInstanceID]State.Building{100: {InstanceID: 100, DefinitionID: 200}},
		ConstructionSlots:           map[State.BuildingInstanceID][]State.ConstructionSlot{},
		ConstructionSlotsObservedAt: now,
	}
	gameState.Castles[10] = castle
	gameState.Inventory.ConstructionItemsObservedAt = now
	return gameState
}

func constructionPolicyConfiguration() Configuration.Snapshot {
	return Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.constructionItems": json.RawMessage(`{"targets":{"10":[{"id":101,"amount":2}]}}`),
	}}
}

func constructionPolicyGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":200,"constructionItemGroupIDs":"1"}],
		"units":[{"wodID":1}],
		"constructionItems":[
			{"constructionItemID":101,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":1,"slotTypeID":0},
			{"constructionItemID":102,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":2,"slotTypeID":0},
			{"constructionItemID":103,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":3,"slotTypeID":0},
			{"constructionItemID":104,"constructionItemGroupID":1,"name":"Target","duration":3600,"level":4,"slotTypeID":0},
			{"constructionItemID":201,"constructionItemGroupID":1,"name":"Permanent","level":1,"slotTypeID":1},
			{"constructionItemID":202,"constructionItemGroupID":1,"name":"PermanentTargetSlot","level":1,"slotTypeID":0},
			{"constructionItemID":301,"constructionItemGroupID":1,"name":"OtherTemporary","duration":3600,"level":1,"slotTypeID":0}
		],
		"packages":[
			{"packageID":500,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":1},
			{"packageID":501,"packageType":"constructionItem","comment2":"Central Silver Shop - keep like this ","constructionItemID":101,"constructionItemAmount":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

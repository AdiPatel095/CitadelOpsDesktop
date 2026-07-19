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

func TestConstructionPolicyPurchasesMissingTierFromLiveOfficialOffer(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := constructionPolicyState(now)
	gameState.Inventory.ConstructionOffersObservedAt = now
	gameState.Inventory.ConstructionOffers[500] = 1
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
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.ConstructionItemID != 101 {
		t.Fatalf("construction item = %d, want selected variant tier 101", request.ConstructionItemID)
	}
}

func TestConstructionPolicyWaitsForHostOccupiedByPermanentItem(t *testing.T) {
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
	if decision.Request != nil || !strings.Contains(decision.Detail, "occupied construction slot") {
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
	gameState.Inventory.ConstructionOffersObservedAt = now
	gameState.Inventory.ConstructionOffers[500] = 1
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
	if want := now.Add(600 * time.Second); !decision.NextCheckAt.Equal(want) {
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

func TestConstructionPolicyRenewsExpiredTemporaryItem(t *testing.T) {
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
	if decision.Request == nil || decision.Request.Name != "construction.equip" {
		t.Fatalf("unexpected renewal decision: %+v", decision)
	}
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
			{"constructionItemID":201,"constructionItemGroupID":1,"name":"Permanent","level":1,"slotTypeID":1},
			{"constructionItemID":301,"constructionItemGroupID":1,"name":"OtherTemporary","duration":3600,"level":1,"slotTypeID":0}
		],
		"packages":[{"packageID":500,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

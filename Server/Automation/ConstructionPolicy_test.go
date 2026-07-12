package Automation

import (
	"encoding/json"
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
	if decision.Request == nil || decision.Request.Name != "construction.upgrade" || string(decision.Request.Arguments) != `{"buildingInstanceId":100,"castleId":10,"offerCode":2000,"slot":0}` {
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
}

func constructionPolicyState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	castle := State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main", X: 10, Y: 20,
		Resources:         map[State.ResourceID]State.ResourceBalance{},
		Buildings:         map[State.BuildingInstanceID]State.Building{100: {InstanceID: 100, DefinitionID: 200}},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{},
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
			{"constructionItemID":101,"constructionItemGroupID":1,"level":1,"slotTypeID":1},
			{"constructionItemID":102,"constructionItemGroupID":1,"level":2,"slotTypeID":1}
		],
		"packages":[{"packageID":500,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

package App

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanCraftingStartRejectsInsufficientOfficialRecipeCost(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"resources":[
			{"resourceID":8,"name":"glass","JSONKey":"G"},
			{"resourceID":10,"name":"iron","JSONKey":"I"}
		],
		"craftingRecipes":[{
			"craftingRecipeId":110,"queueTypeId":2,
			"costGlass":21200,"costIron":19200
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	castle := resourceIntentCastle(10, 0, 10, 20)
	castle.Resources[8] = State.ResourceBalance{Amount: 492004}
	castle.Resources[10] = State.ResourceBalance{Amount: 16168}
	castle.Crafting.Buildings = map[State.BuildingInstanceID]State.CraftingBuilding{}
	castle.Crafting.Buildings[100] = State.CraftingBuilding{
		CastleID: 10, InstanceID: 100, QueueTypeID: 2,
	}
	gameState.Castles[10] = castle
	arguments := json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"recipeId":110}`)
	_, err = planCraftingStart(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err == nil || !strings.Contains(err.Error(), "needs 19200 I; 16168 are observed") {
		t.Fatalf("crafting affordability error = %v", err)
	}

	castle.Resources[10] = State.ResourceBalance{Amount: 19200}
	gameState.Castles[10] = castle
	plan, err := planCraftingStart(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if !craftingContainsString(plan.Claims, "account-resources") {
		t.Fatalf("crafting claims = %#v", plan.Claims)
	}
}

func TestPlanCraftingStartRejectsStorageNodeWithStaleBuildingState(t *testing.T) {
	gameState := State.NewGameState()
	outpost := resourceIntentCastle(10, 0, 10, 20)
	outpost.SlotType = 4
	outpost.Crafting.Buildings = map[State.BuildingInstanceID]State.CraftingBuilding{
		100: {CastleID: 10, InstanceID: 100, QueueTypeID: 1},
	}
	gameState.Castles[outpost.ID] = outpost

	_, err := planCraftingStart(t.Context(), Intent.PlanningContext{State: gameState}, json.RawMessage(
		`{"castleId":10,"buildingInstanceId":100,"recipeId":110}`,
	))
	if err == nil || !strings.Contains(err.Error(), "storage node, not a crafting castle") {
		t.Fatalf("storage-node crafting error = %v", err)
	}
}

func craftingContainsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

func TestPlanCraftingSlotRentalBuildsObservedNextSlot(t *testing.T) {
	gameState := State.NewGameState()
	castle := resourceIntentCastle(10, 0, 10, 20)
	castle.Crafting.Buildings = map[State.BuildingInstanceID]State.CraftingBuilding{
		100: {
			CastleID: 10, InstanceID: 100, QueueTypeID: 1,
			QueueSlotRentals: []int{600}, ActiveSlotRentals: []int{},
		},
	}
	gameState.Castles[10] = castle
	gameState.Player.Resources[1] = 10_000_000
	plan, err := planCraftingSlotRental(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: resourceIntentGameData(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"slotType":"queue","slot":2}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Opcode != "crun" || string(plan.Steps[0].Command.Payload) != `{"KID":0,"AID":10,"OID":100,"S":[2],"ST":"queue"}` {
		t.Fatalf("unexpected rental plan: %+v payload=%s", plan, plan.Steps[0].Command.Payload)
	}
}

func TestPlanCraftingSkipUsesCurrentOfficialRubyPrice(t *testing.T) {
	now := time.Now().UTC()
	remaining := 50
	gameState := State.NewGameState()
	castle := resourceIntentCastle(10, 0, 10, 20)
	castle.Crafting.Buildings = map[State.BuildingInstanceID]State.CraftingBuilding{
		100: {
			CastleID: 10, InstanceID: 100, QueueTypeID: 1, ObservedAt: now,
			Active: []State.CraftingQueueItem{{RecipeID: 100, RemainingSec: &remaining}},
		},
	}
	gameState.Castles[10] = castle
	gameState.Player.Resources[2] = 100
	plan, err := planCraftingSkip(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: constructionPolicyGameDataForApp(t),
	}, json.RawMessage(`{"castleId":10,"buildingInstanceId":100,"slot":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Opcode != "crsk" || string(plan.Steps[0].Command.Payload) != `{"KID":0,"AID":10,"OID":100,"S":0,"ST":"production","PC2":10}` {
		t.Fatalf("unexpected crafting skip: %+v payload=%s", plan, plan.Steps[0].Command.Payload)
	}
}

func TestPlanConstructionPurchaseUsesLiveOfficialOffer(t *testing.T) {
	gameData := constructionPolicyGameDataForApp(t)
	gameState := State.NewGameState()
	castle := resourceIntentCastle(10, 0, 10, 20)
	gameState.Castles[10] = castle
	gameState.Inventory.ConstructionItemsObservedAt = time.Now().UTC()
	gameState.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	gameState.Inventory.ConstructionOffers[500] = 1
	plan, err := planConstructionPurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"castleId":10,"productId":500,"amount":1
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "jaa" || plan.Steps[1].Opcode != "aec" || plan.Steps[2].Opcode != "gbc" {
		t.Fatalf("construction purchase context = %#v", plan.Steps)
	}
	if plan.Steps[1].ResumePolicy != Intent.ResumeRebuild || plan.Steps[2].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("construction shop context is not resumable: %#v", plan.Steps[1:3])
	}
	purchase := plan.Steps[len(plan.Steps)-1]
	if purchase.Opcode != "sbp" || string(purchase.Command.Payload) != `{"PID":500,"BT":0,"TID":116,"AMT":1,"KID":0,"AID":10,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}` {
		t.Fatalf("unexpected construction purchase: %+v payload=%s", plan, purchase.Command.Payload)
	}
	if plan.Summary != "Buy 1 x Elven Keep (level 1) from castle 10" {
		t.Fatalf("construction purchase summary = %q", plan.Summary)
	}
}

func TestPlanConstructionPurchaseAllowsOfficialTrivialProductOutsideLiveOffers(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = resourceIntentCastle(10, 0, 10, 20)
	gameState.Inventory.ConstructionItemsObservedAt = time.Now().UTC()
	gameState.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	plan, err := planConstructionPurchase(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: constructionPolicyGameDataForApp(t),
	}, json.RawMessage(`{"castleId":10,"productId":501,"amount":1}`))
	if err != nil {
		t.Fatal(err)
	}
	purchase := plan.Steps[len(plan.Steps)-1]
	if purchase.Opcode != "sbp" || string(purchase.Command.Payload) != `{"PID":501,"BT":0,"TID":116,"AMT":1,"KID":0,"AID":10,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}` {
		t.Fatalf("unexpected trivial construction purchase: %+v payload=%s", plan, purchase.Command.Payload)
	}
}

func TestPlanConstructionPurchaseRejectsFullInventory(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[10] = resourceIntentCastle(10, 0, 10, 20)
	gameState.Inventory.ConstructionItemsObservedAt = time.Now().UTC()
	gameState.Inventory.ConstructionItems[101] = State.ConstructionItemInventoryLimit
	gameState.Inventory.ConstructionOffersObservedAt = time.Now().UTC()
	gameState.Inventory.ConstructionOffers[500] = 1

	_, err := planConstructionPurchase(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: constructionPolicyGameDataForApp(t),
	}, json.RawMessage(`{"castleId":10,"productId":500,"amount":1}`))
	if err == nil || !strings.Contains(err.Error(), "inventory is full (1000/1000)") {
		t.Fatalf("full construction-item inventory error = %v", err)
	}
}

func constructionPolicyGameDataForApp(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":1}],
		"resources":[
			{"resourceID":1,"JSONKey":"C1","name":"currency1"},
			{"resourceID":2,"JSONKey":"C2","name":"currency2"}
		],
		"craftingRecipes":[{"craftingRecipeId":100,"queueTypeId":1,"craftingDuration":100,"skipCostC2":20}],
		"constructionItems":[{"constructionItemID":101,"name":"elvenKeep","level":1}],
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

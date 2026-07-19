package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

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

func TestPlanConstructionPurchaseUsesOnlyLiveOfficialOffer(t *testing.T) {
	gameData := constructionPolicyGameDataForApp(t)
	gameState := State.NewGameState()
	castle := resourceIntentCastle(10, 0, 10, 20)
	gameState.Castles[10] = castle
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
		"packages":[{"packageID":500,"packageType":"constructionItem","constructionItemID":101,"constructionItemAmount":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

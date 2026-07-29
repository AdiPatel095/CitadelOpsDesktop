package App

import (
	"encoding/json"
	"errors"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanBeriToolPurchaseUsesCapturedArmorerWireShape(t *testing.T) {
	gameData := beriToolIntentGameData(t)
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.Resources[1] = 100_000
	gameState.KingdomTransport.Unlocks[beriKingdomID] = State.KingdomTransportUnlock{
		KingdomID: beriKingdomID, Unlocked: true, Created: true,
	}
	gameState.Castles[989] = State.CastleState{
		ID: 989, KingdomID: beriKingdomID, Focused: true, UnitsObservedAt: time.Now().UTC(),
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
			GameData.BerimondScalingLadderToolID: 1,
		}},
	}
	arguments := json.RawMessage(`{"castleId":989,"packageId":28,"toolId":614,"amount":9,"minimum":10}`)
	plan, err := planBeriToolPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 ||
		plan.Steps[0].Opcode != "jca" || plan.Steps[0].AwaitOpcode != "jaa" ||
		plan.Steps[1].Opcode != "gbc" ||
		plan.Steps[2].Action != "beri.tools.purchase.guard" ||
		plan.Steps[2].ResumePolicy != Intent.ResumeRebuild ||
		plan.Steps[3].Opcode != "sbp" ||
		plan.Steps[4].Opcode != "jca" || plan.Steps[4].AwaitOpcode != "jaa" {
		t.Fatalf("unexpected Berimond tool plan: %#v", plan.Steps)
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"CID":989,"KID":10}` {
		t.Fatalf("gbc payload = %s", got)
	}
	if got := string(plan.Steps[3].Command.Payload); got != `{"PID":28,"BT":0,"TID":27,"AMT":9,"KID":10,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}` {
		t.Fatalf("sbp payload = %s", got)
	}
	for _, claim := range []string{
		"shop", "shop:table:27", "account-resources", "castle-focus",
		"castle:989", "attack-inventory:989", "unit:614",
	} {
		if !slices.Contains(plan.Claims, claim) {
			t.Fatalf("Berimond tool purchase is missing claim %q: %#v", claim, plan.Claims)
		}
	}

	castle := gameState.Castles[989]
	castle.Units.Stationed[GameData.BerimondScalingLadderToolID] = 2
	gameState.Castles[castle.ID] = castle
	_, _, _, err = beriToolPurchaseContext(
		Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("changed shortage did not stale the captured purchase: %v", err)
	}
}

func TestBeriToolPurchasePassesProductionResourceAdmission(t *testing.T) {
	gameData := beriToolIntentGameData(t)
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.Resources[1] = 100_000
	gameState.Castles[989] = State.CastleState{
		ID: 989, KingdomID: beriKingdomID, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
			GameData.BerimondScalingLadderToolID: 1,
		}},
	}
	registry := Intent.NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Intent.Definition{
		Name: "beri.tools.purchase", Effect: Intent.EffectWrite, Planner: planBeriToolPurchase,
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(
		registry, State.NewStore(gameState), beriIntentGameDataProvider{store: gameData}, nil, nil,
	)
	receipt := engine.Submit(t.Context(), Intent.Request{
		Name: "beri.tools.purchase", DryRun: true,
		Arguments: json.RawMessage(`{"castleId":989,"packageId":28,"toolId":614,"amount":9,"minimum":10}`),
	})
	if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
		t.Fatalf("Berimond tool resource admission failed: %#v", receipt)
	}
	for _, resource := range receipt.Plan.Resources {
		if resource.Capability == "legacy" {
			t.Fatalf("Berimond tool purchase retained a legacy resource: %#v", receipt.Plan.Resources)
		}
	}
}

func TestBeriToolPurchaseRequiresGameCappedBatch(t *testing.T) {
	gameData := beriToolIntentGameData(t)
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.Resources[1] = 1_000_000
	gameState.Castles[989] = State.CastleState{
		ID: 989, KingdomID: beriKingdomID, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{
			GameData.BerimondScalingLadderToolID: 1,
		}},
	}
	input := Intent.PlanningContext{State: gameState, GameData: gameData}
	valid := json.RawMessage(`{"castleId":989,"packageId":28,"toolId":614,"amount":1000,"minimum":2501}`)
	if _, _, _, err := beriToolPurchaseContext(input, valid); err != nil {
		t.Fatalf("game-capped batch was rejected: %v", err)
	}
	overCap := json.RawMessage(`{"castleId":989,"packageId":28,"toolId":614,"amount":1001,"minimum":2501}`)
	if _, _, _, err := beriToolPurchaseContext(input, overCap); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("over-cap batch did not stale: %v", err)
	}
}

func beriToolIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":611,"typ":"Attack"},
			{"wodID":614,"typ":"Attack"},
			{"wodID":620,"typ":"Attack"}
		],
		"packages":[
			{"packageID":28,"packageType":"tool","comment1":"Scaling ladder","comment2":"Armorer","packagePriceC1":220,"unitID":614,"unitAmount":1,"minLevel":4},
			{"packageID":32,"packageType":"tool","comment1":"Battering ram","comment2":"Armorer","packagePriceC1":430,"unitID":611,"unitAmount":1,"minLevel":4},
			{"packageID":36,"packageType":"tool","comment1":"Mantlet","comment2":"Armorer","packagePriceC1":810,"unitID":620,"unitAmount":1,"minLevel":17}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

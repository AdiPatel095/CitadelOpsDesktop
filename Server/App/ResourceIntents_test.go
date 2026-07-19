package App

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"CitadelDesktop/Server/Telemetry"
)

func TestResourceShipmentPlannersUseOfficialWireKeys(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	source := resourceIntentCastle(10, 0, 100, 200)
	target := resourceIntentCastle(20, 0, 110, 215)
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	source.Resources[3] = State.ResourceBalance{Amount: 50_000}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.Castles[source.ID] = State.MarketCastleState{CastleID: source.ID, AvailableBarrows: 10}
	gameState.Market.ObservedAt = time.Now().UTC()

	marketPlan, err := planMarketResourceShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"resourceId":3,"amount":12000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var marketPayload struct {
		Goods [][]json.RawMessage `json:"G"`
		X     int                 `json:"TX"`
		Y     int                 `json:"TY"`
	}
	if err := json.Unmarshal(marketPlan.Steps[0].Command.Payload, &marketPayload); err != nil {
		t.Fatal(err)
	}
	if string(marketPayload.Goods[0][0]) != `"W"` || marketPayload.X != target.X || marketPayload.Y != target.Y {
		t.Fatalf("unexpected market payload: %s", marketPlan.Steps[0].Command.Payload)
	}

	target.KingdomID = 1
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[1] = State.KingdomTransportUnlock{KingdomID: 1, Unlocked: true}
	kingdomPlan, err := planKingdomResourceShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetKingdomId":1,"resourceId":3,"amount":15000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var kingdomPayload struct {
		Goods [][]json.RawMessage `json:"G"`
	}
	if err := json.Unmarshal(kingdomPlan.Steps[1].Command.Payload, &kingdomPayload); err != nil {
		t.Fatal(err)
	}
	if kingdomPlan.Steps[0].Opcode != "kpi" || kingdomPlan.Steps[0].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("kingdom context step = %#v", kingdomPlan.Steps[0])
	}
	if string(kingdomPayload.Goods[0][0]) != `"W"` || kingdomPlan.Steps[1].Opcode != "kgt" {
		t.Fatalf("unexpected kingdom payload: %s", kingdomPlan.Steps[1].Command.Payload)
	}
}

func TestMarketShipmentPlannerRejectsBarrowsWithoutMarketplace(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	source := resourceIntentCastle(10, 0, 100, 200)
	target := resourceIntentCastle(20, 0, 110, 215)
	source.Resources[3] = State.ResourceBalance{Amount: 50_000}
	gameState.Castles[source.ID] = source
	gameState.Castles[target.ID] = target
	gameState.Market.Castles[source.ID] = State.MarketCastleState{CastleID: source.ID, AvailableBarrows: 10}
	gameState.Market.ObservedAt = time.Now().UTC()

	_, err := planMarketResourceShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"resourceId":3,"amount":12000
	}`))
	if err == nil || !strings.Contains(err.Error(), "no Marketplace building") {
		t.Fatalf("market shipment error = %v", err)
	}
}

func TestKingdomSkipPlannerRequiresObservedInventory(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{KingdomID: 1, RemainingSec: 3600}}
	gameState.Player.Currencies[50] = 2
	plan, err := planKingdomResourceSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"targetKingdomId":1,"timeSkipId":"ms5"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Steps[0].Opcode != "msk" || string(plan.Steps[0].Command.Payload) != `{"KID":"1","MST":"MS5","TT":"2"}` {
		t.Fatalf("unexpected skip plan: %+v payload=%s", plan, plan.Steps[0].Command.Payload)
	}
}

func TestKingdomShipmentPlanNamesProvidedDonorAndTargetCastles(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 0, 100, 200)
	donor.Name = "Donor Castle"
	donor.Resources[3] = State.ResourceBalance{Amount: 50_000}
	target := resourceIntentCastle(20, 1, 110, 215)
	target.Name = "Target Castle"
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{KingdomID: target.KingdomID, Unlocked: true}

	plan, err := planKingdomResourceShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"targetKingdomId":1,"resourceId":3,"amount":15000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Ship 15000 W from Donor Castle to Target Castle by kingdom transport" {
		t.Fatalf("kingdom shipment summary = %q", plan.Summary)
	}
}

func TestKingdomShipmentCombinesMultipleResourceGoods(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 0, 100, 200)
	donor.Resources[3] = State.ResourceBalance{Amount: 50_000}
	donor.Resources[4] = State.ResourceBalance{Amount: 50_000}
	target := resourceIntentCastle(20, 4, 110, 215)
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	plan, err := planKingdomResourceShipment(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"targetKingdomId":4,
		"goods":[{"resourceId":4,"amount":5124},{"resourceId":3,"amount":5124}]
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"SCID":10,"SKID":0,"TKID":4,"G":[["W",5124],["S",5124]]}` {
		t.Fatalf("multi-good kingdom payload = %s", got)
	}
}

func TestAutoFoodBalanceReceiptLogsActualDonorAndTargetCastles(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 0, 100, 200)
	donor.Name = "Donor Castle"
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	donor.Resources[3] = State.ResourceBalance{Amount: 50_000}
	target := resourceIntentCastle(20, 0, 110, 215)
	target.Name = "Target Castle"
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.Market.Castles[donor.ID] = State.MarketCastleState{CastleID: donor.ID, AvailableBarrows: 10}
	gameState.Market.ObservedAt = time.Now().UTC()

	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{
		Name: "resource.market.ship", Effect: Intent.EffectLaunch, Planner: planMarketResourceShipment,
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, State.NewStore(gameState), resourceIntentGameDataProvider{store: gameData}, nil, nil)
	receipt := engine.Submit(t.Context(), Intent.Request{
		Name: "resource.market.ship", Actor: "automation:autoFoodBalance", DryRun: true,
		Arguments: json.RawMessage(`{"sourceCastleId":10,"targetCastleId":20,"resourceId":3,"amount":12000}`),
	})
	if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
		t.Fatalf("food-balance receipt = %#v", receipt)
	}

	telemetry := Telemetry.NewStore(100)
	application := &Application{Telemetry: telemetry}
	application.recordIntentLog(receipt)
	foodBalanceLog := strings.Join(telemetry.Tail(Telemetry.ChannelAutoFoodBalance, 10), "\n")
	if !strings.Contains(foodBalanceLog, "Donor Castle") || !strings.Contains(foodBalanceLog, "Target Castle") {
		t.Fatalf("Auto Food Balance log = %q, want actual shipment donor and target", foodBalanceLog)
	}
}

func resourceIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":137,"name":"Market","marketCarriages":5}],"units":[{"wodID":1}],
		"constructionItems":[],"levelBoosters":[],"effects":[],
		"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":3,"JSONKey":"W"},{"resourceID":4,"JSONKey":"S"}],
		"currencies":[{"currencyID":50,"JSONKey":"MS5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

type resourceIntentGameDataProvider struct{ store *GameData.Store }

func (provider resourceIntentGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

func resourceIntentCastle(id State.CastleID, kingdom State.KingdomID, x int, y int) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, X: x, Y: y,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Buildings: map[State.BuildingInstanceID]State.Building{},
	}
}

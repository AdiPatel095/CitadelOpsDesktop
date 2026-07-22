package App

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
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
	if err := json.Unmarshal(kingdomPlan.Steps[3].Command.Payload, &kingdomPayload); err != nil {
		t.Fatal(err)
	}
	if kingdomPlan.Steps[0].Opcode != "kpi" || kingdomPlan.Steps[0].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("kingdom context step = %#v", kingdomPlan.Steps[0])
	}
	if kingdomPlan.Steps[1].Opcode != "grc" || kingdomPlan.Steps[1].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("kingdom donor refresh = %#v", kingdomPlan.Steps[1])
	}
	if kingdomPlan.Steps[2].Action != "kingdom.transport.verify_available" || kingdomPlan.Steps[2].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("kingdom transport guard = %#v", kingdomPlan.Steps[2])
	}
	if string(kingdomPayload.Goods[0][0]) != `"W"` || kingdomPlan.Steps[3].Opcode != "kgt" ||
		kingdomPlan.Steps[4].Action != "resources.kingdom.consume_source" {
		t.Fatalf("unexpected kingdom shipment steps: %#v", kingdomPlan.Steps)
	}
}

func TestResourceLogisticsRefreshSkipsMarketForSingleCastleKingdoms(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	marketCastle := resourceIntentCastle(10, 0, 100, 200)
	marketCastle.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	dungeonCastle := resourceIntentCastle(20, 3, 110, 215)
	dungeonCastle.Focused = true
	gameState.Castles[marketCastle.ID] = marketCastle
	gameState.Castles[dungeonCastle.ID] = dungeonCastle

	plan, err := planResourceLogisticsRefresh(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "kpi" {
		t.Fatalf("single-castle kingdom refresh steps = %#v", plan.Steps)
	}
	for _, claim := range plan.Claims {
		if claim == "castle-focus" {
			t.Fatalf("single-castle kingdom refresh claimed castle focus: %#v", plan.Claims)
		}
	}
}

func TestResourceLogisticsRefreshUsesEligibleMarketCastleAndRestoresFocus(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	marketCastle := resourceIntentCastle(10, 0, 100, 200)
	marketCastle.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	peerCastle := resourceIntentCastle(11, 0, 105, 205)
	dungeonCastle := resourceIntentCastle(20, 3, 110, 215)
	dungeonCastle.Focused = true
	gameState.Castles[marketCastle.ID] = marketCastle
	gameState.Castles[peerCastle.ID] = peerCastle
	gameState.Castles[dungeonCastle.ID] = dungeonCastle

	plan, err := planResourceLogisticsRefresh(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	wantOpcodes := []string{"kpi", "jaa", "boi", "cmi", "jaa"}
	if len(plan.Steps) != len(wantOpcodes) {
		t.Fatalf("market refresh steps = %#v", plan.Steps)
	}
	for index, opcode := range wantOpcodes {
		if plan.Steps[index].Opcode != opcode {
			t.Fatalf("market refresh step %d opcode = %q, want %q", index, plan.Steps[index].Opcode, opcode)
		}
	}
	var focusPayloads []struct {
		X         int             `json:"PX"`
		Y         int             `json:"PY"`
		KingdomID State.KingdomID `json:"KID"`
	}
	for _, index := range []int{1, 4} {
		var payload struct {
			X         int             `json:"PX"`
			Y         int             `json:"PY"`
			KingdomID State.KingdomID `json:"KID"`
		}
		if err := json.Unmarshal(plan.Steps[index].Command.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		focusPayloads = append(focusPayloads, payload)
	}
	if focusPayloads[0].X != marketCastle.X || focusPayloads[0].Y != marketCastle.Y || focusPayloads[0].KingdomID != marketCastle.KingdomID {
		t.Fatalf("market focus payload = %#v", focusPayloads[0])
	}
	if focusPayloads[1].X != dungeonCastle.X || focusPayloads[1].Y != dungeonCastle.Y || focusPayloads[1].KingdomID != dungeonCastle.KingdomID {
		t.Fatalf("restore focus payload = %#v", focusPayloads[1])
	}
}

func TestResourceShipmentPlannerSelectsTransportFromCastleKingdoms(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	source := resourceIntentCastle(10, 0, 100, 200)
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	source.Resources[3] = State.ResourceBalance{Amount: 50_000}
	sameKingdomTarget := resourceIntentCastle(20, 0, 110, 215)
	crossKingdomTarget := resourceIntentCastle(30, 3, 120, 225)
	gameState.Castles[source.ID] = source
	gameState.Castles[sameKingdomTarget.ID] = sameKingdomTarget
	gameState.Castles[crossKingdomTarget.ID] = crossKingdomTarget
	gameState.Market.Castles[source.ID] = State.MarketCastleState{CastleID: source.ID, AvailableBarrows: 10}
	gameState.Market.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[crossKingdomTarget.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: crossKingdomTarget.KingdomID, Unlocked: true,
	}
	input := Intent.PlanningContext{State: gameState, GameData: gameData}

	marketPlan, err := planResourceShipment(t.Context(), input, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"resourceId":3,"amount":12000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(marketPlan.Steps) != 1 || marketPlan.Steps[0].Opcode != "crm" {
		t.Fatalf("same-kingdom shipment plan = %#v", marketPlan)
	}

	kingdomPlan, err := planResourceShipment(t.Context(), input, json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":30,"resourceId":3,"amount":15000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(kingdomPlan.Steps) != 5 || kingdomPlan.Steps[0].Opcode != "kpi" || kingdomPlan.Steps[1].Opcode != "grc" ||
		kingdomPlan.Steps[2].Action != "kingdom.transport.verify_available" || kingdomPlan.Steps[3].Opcode != "kgt" ||
		kingdomPlan.Steps[4].Action != "resources.kingdom.consume_source" {
		t.Fatalf("cross-kingdom shipment plan = %#v", kingdomPlan)
	}
	var payload struct {
		TargetKingdom State.KingdomID `json:"TKID"`
	}
	if err := json.Unmarshal(kingdomPlan.Steps[3].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.TargetKingdom != crossKingdomTarget.KingdomID {
		t.Fatalf("cross-kingdom target = %d, want %d", payload.TargetKingdom, crossKingdomTarget.KingdomID)
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
	if plan.Summary != "Apply a 1-hour time skip to kingdom 1 resource transport" {
		t.Fatalf("resource time-skip summary = %q", plan.Summary)
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

func TestKingdomShipmentRejectsSettlingTransportBeforeAndAfterRefresh(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 0, 100, 200)
	donor.Resources[3] = State.ResourceBalance{Amount: 50_000}
	target := resourceIntentCastle(20, 4, 110, 215)
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{
		KingdomID: target.KingdomID, RemainingSec: -1,
	}}
	arguments := json.RawMessage(`{
		"sourceCastleId":10,"targetCastleId":20,"targetKingdomId":4,"resourceId":3,"amount":15000
	}`)

	if _, err := planKingdomResourceShipment(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	); err == nil || !strings.Contains(err.Error(), "settling") {
		t.Fatalf("settling shipment planning error = %v", err)
	}

	application := &Application{State: State.NewStore(gameState)}
	guardArguments, _ := json.Marshal(kingdomTransportAvailabilityGuard{
		TargetKingdomID: target.KingdomID, TransportKind: "resource",
	})
	if err := application.verifyKingdomTransportAvailable(t.Context(), guardArguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("post-refresh transport guard error = %v", err)
	}
}

func TestKingdomShipmentGuardAndConfirmedConsumptionUseFreshDonorBalance(t *testing.T) {
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 1, 100, 200)
	donor.Resources[3] = State.ResourceBalance{Amount: 14_999}
	gameState.Castles[donor.ID] = donor
	application := &Application{State: State.NewStore(gameState)}
	goods := []kingdomResourceShipmentGood{{ResourceID: 3, Amount: 15_000}}
	guardArguments, _ := json.Marshal(kingdomTransportAvailabilityGuard{
		TargetKingdomID: 2, TransportKind: "resource", SourceCastleID: donor.ID, Goods: goods,
	})
	if err := application.verifyKingdomTransportAvailable(t.Context(), guardArguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("stale donor guard error = %v", err)
	}

	_, err := application.State.Apply(func(state *State.GameState) ([]string, bool, error) {
		updated := state.Castles[donor.ID]
		balance := updated.Resources[3]
		balance.Amount = 50_000
		updated.Resources[3] = balance
		state.Castles[donor.ID] = updated
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyKingdomTransportAvailable(t.Context(), guardArguments); err != nil {
		t.Fatalf("fresh donor guard error = %v", err)
	}
	consumeArguments, _ := json.Marshal(kingdomResourceShipmentRequest{SourceCastleID: donor.ID, Goods: goods})
	if err := application.consumeKingdomResourceSource(t.Context(), consumeArguments); err != nil {
		t.Fatal(err)
	}
	if got := application.State.Snapshot().Castles[donor.ID].Resources[3].Amount; got != 35_000 {
		t.Fatalf("confirmed donor balance = %.0f, want 35000", got)
	}
}

func TestAutomationKingdomShipmentRecordsAndSettlesItsWorkflow(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	donor := resourceIntentCastle(10, 0, 100, 200)
	target := resourceIntentCastle(20, 4, 110, 215)
	donor.Resources[3] = State.ResourceBalance{Amount: 50_000}
	gameState.Castles[donor.ID] = donor
	gameState.Castles[target.ID] = target
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(kingdomResourceShipmentRequest{
		SourceCastleID: donor.ID, TargetCastleID: target.ID, TargetKingdomID: target.KingdomID,
		Goods: []kingdomResourceShipmentGood{{ResourceID: 3, Amount: 15_000}}, WorkflowOwner: "autoFoodBalance",
	})
	ctx := Outbound.WithMetadata(t.Context(), Outbound.Metadata{
		Actor: "automation:autoFoodBalance", SubmittedAt: now,
	})
	if err := application.consumeKingdomResourceSource(ctx, arguments); err != nil {
		t.Fatal(err)
	}
	workflow, exists := application.State.Snapshot().KingdomTransport.ResourceWorkflows[target.KingdomID]
	if !exists || workflow.Owner != "autoFoodBalance" || workflow.SourceCastleID != donor.ID || workflow.TargetCastleID != target.ID {
		t.Fatalf("recorded kingdom resource workflow = %#v exists=%t", workflow, exists)
	}

	settlementArguments := json.RawMessage(`{"owner":"autoFoodBalance","targetKingdomId":4}`)
	plan, err := planKingdomResourceSettlement(t.Context(), Intent.PlanningContext{
		State: application.State.Snapshot(), GameData: resourceIntentGameData(t),
	}, settlementArguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Opcode != "grc" || string(plan.Steps[0].Command.Payload) != `{"AID":20,"KID":4}` ||
		plan.Steps[1].Action != "resources.kingdom.complete_workflow" {
		t.Fatalf("settlement plan = %#v", plan.Steps)
	}
	if err := application.completeKingdomResourceWorkflow(ctx, settlementArguments); err != nil {
		t.Fatal(err)
	}
	if _, exists := application.State.Snapshot().KingdomTransport.ResourceWorkflows[target.KingdomID]; exists {
		t.Fatal("completed kingdom resource workflow remained in state")
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
	if got := string(plan.Steps[3].Command.Payload); got != `{"SCID":10,"SKID":0,"TKID":4,"G":[["W",5124],["S",5124]]}` {
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
		Name: "resource.ship", Effect: Intent.EffectLaunch, Planner: planResourceShipment,
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, State.NewStore(gameState), resourceIntentGameDataProvider{store: gameData}, nil, nil)
	receipt := engine.Submit(t.Context(), Intent.Request{
		Name: "resource.ship", Actor: "automation:autoFoodBalance", DryRun: true,
		Arguments: json.RawMessage(`{"sourceCastleId":10,"targetCastleId":20,"resourceId":3,"amount":12000}`),
	})
	if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
		t.Fatalf("food-balance receipt = %#v", receipt)
	}
	receipt.Status = Intent.StatusSucceeded

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
		"currencies":[{"currencyID":50,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"50","MinutesSkipValue":"60"}]
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
	slotType := 12
	if kingdom == 0 {
		slotType = 1
	}
	return State.CastleState{
		ID: id, KingdomID: kingdom, SlotType: slotType, X: x, Y: y,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Buildings: map[State.BuildingInstanceID]State.Building{},
	}
}

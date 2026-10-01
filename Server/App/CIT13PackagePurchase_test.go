package App

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func cit13DirewolfCatalog(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":277,"type":"Elitetinoswolves","name":"Eventunit","comment1":"Nomad Shop"}],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}],
		"currencies":[{"currencyID":37,"JSONKey":"KT","Name":"KhanTablet"}],
		"events":[{"eventID":94,"packageIDs":"3857","kIDs":"0","areaTypes":"1"}],
		"packages":[
			{"packageID":3857,"packageType":"soldier","unitID":277,"unitAmount":100,"stock":50,"sortOrder":33,"costKhanTablet":2340,"comment1":"Nomad EDS Shop (2023) - Khan Tablets"}
		],"feasts":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func cit13DirewolfState(now time.Time) State.GameState {
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Focused = true
	gameState.Castles[10] = castle
	gameState.Player.Currencies[37] = 1_000_000
	gameState.EventScores.ShopByPackage[3857] = State.EventShopRoute{EventID: 94, RemainingSec: 3600, ObservedAt: now}
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	return gameState
}

// Captured manual purchase (purchase-sbp-wire.log, 2026-09-16, K0 main castle):
// PID:3857,BT:0,TID:<table>,AMT:1,KID:0,AID:-1 → code 0. Official shop dialogs
// send KID = activeKingdomID and AID = -1 for unit rewards; the planner matches.
func TestDirewolfPackagePayloadMatchesOfficialKingdomRule(t *testing.T) {
	now := time.Now().UTC()
	gameState := cit13DirewolfState(now)
	plan, err := planAutoBuyerPackagePurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: cit13DirewolfCatalog(t)}, json.RawMessage(`{
		"sourceCastleId":10,"shopId":"nomad","packageId":3857,"amount":1,"targetPurchasesPerReset":50,
		"minimumBalanceReserve":0,"allowRubyPackages":false,"maximumRubySpendPerReset":0,
		"minimumRubyReserve":0,"expectedPurchasedBefore":0,"expectedBalanceBefore":1000000
	}`))
	if err != nil {
		t.Fatal(err)
	}
	var purchase Intent.Step
	for _, step := range plan.Steps {
		if step.Opcode == "sbp" {
			purchase = step
		}
	}
	var payload struct {
		ProductID int64 `json:"PID"`
		Amount    int64 `json:"AMT"`
		KingdomID int64 `json:"KID"`
		CastleID  int64 `json:"AID"`
	}
	if err := json.Unmarshal(purchase.Command.Payload, &payload); err != nil ||
		payload.ProductID != 3857 || payload.Amount != 1 || payload.KingdomID != 0 || payload.CastleID != -1 {
		t.Fatalf("Direwolf SBP payload = %s err=%v", purchase.Command.Payload, err)
	}
	if !purchase.ResponseProjectionFailureIndeterminate || purchase.FinalDispatchAction != "auto_buyer.package.guard" {
		t.Fatalf("paid purchase step can be treated as definitively failed without a reply: %#v", purchase)
	}
}

func TestEventBackedSBPGuardRequiresSettledSessionKingdom(t *testing.T) {
	now := time.Now().UTC()
	gameState := cit13DirewolfState(now)
	castle := gameState.Castles[10]
	request := eventBackedSBPRequest{PackageID: 3857, TableID: 94, Amount: 1, Stock: 50, MaxBuyPerClick: 1000}
	settled := State.ProtocolContextState{
		SessionGeneration: 1, ConnectionGeneration: 1, FocusedCastleID: 10, FocusedKingdomID: 0,
		FocusSubcontext: State.FocusSubcontextCastle, FocusEpoch: 4, FocusChangedAt: now.Add(-time.Second),
	}
	input := Intent.PlanningContext{State: gameState, GameData: cit13DirewolfCatalog(t), ProtocolContext: settled}
	if _, err := validateEventBackedSBP(input, castle, request, now, true); err != nil {
		t.Fatalf("settled K0 context rejected: %v", err)
	}
	for name, mutate := range map[string]func(*State.ProtocolContextState){
		"session kingdom differs":            func(context *State.ProtocolContextState) { context.FocusedKingdomID = 1 },
		"focus changed after gbc counters":   func(context *State.ProtocolContextState) { context.FocusChangedAt = now.Add(time.Second) },
		"world-map read sent after focusing": func(context *State.ProtocolContextState) { context.MapReadSentAt = now },
	} {
		candidate := input
		mutate(&candidate.ProtocolContext)
		if _, err := validateEventBackedSBP(candidate, castle, request, now, true); !errors.Is(err, Intent.ErrPlanStale) {
			t.Errorf("%s: guard = %v, want stale", name, err)
		}
	}
	// A later committed context reply settles an unanswered read.
	answered := input
	answered.ProtocolContext.MapReadSentAt = now
	answered.ProtocolContext.MapReadSettledAt = now.Add(time.Millisecond)
	if _, err := validateEventBackedSBP(answered, castle, request, now, true); err != nil {
		t.Fatalf("settled map read rejected: %v", err)
	}
	// A read sent before the committed focus change has already settled.
	settledRead := input
	settledRead.ProtocolContext.MapReadSentAt = now.Add(-2 * time.Second)
	if _, err := validateEventBackedSBP(settledRead, castle, request, now, true); err != nil {
		t.Fatalf("settled earlier map read rejected: %v", err)
	}
	if !strings.Contains(castle.Name, "Main") {
		t.Fatalf("fixture source changed: %#v", castle)
	}
}

// CIT-13 SBP/175: the rejection locks the lane, the verify step never runs,
// counters and balance stay unchanged and the dispatch is recorded so the
// policy must read counters again before any further purchase.
func TestPackagePurchase175IsNeverCountedAsPurchase(t *testing.T) {
	now := time.Now().UTC()
	gameState := cit13DirewolfState(now)
	gameState.Inventory.ConstructionOffers[3857] = 7
	stateStore := State.NewStore(&gameState)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, nil, registry)
	sender := &craRejectingSender{equipmentSaleEngineSender: &equipmentSaleEngineSender{pipeline: pipeline}, opcode: "sbp", code: 175}
	intents := Intent.NewRegistry()
	payload := json.RawMessage(`{"PID":3857,"BT":0,"TID":94,"AMT":50,"KID":0,"AID":-1,"PC2":-1,"BA":0,"PWR":0,"_PO":-1}`)
	if err := intents.Register(Intent.Definition{Name: "package.test", Effect: Intent.EffectWrite,
		Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
			purchase := shopCommandStep("Purchase Direwolves", "sbp", payload, 0)
			purchase.ResponseProjectionFailureIndeterminate = true
			return Intent.Plan{Steps: []Intent.Step{purchase, {Name: "Verify", Action: "auto_buyer.package.verify"}}}, nil
		}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(intents, stateStore, nil, sender, pipeline)
	verified := 0
	if err := engine.RegisterAction("auto_buyer.package.verify", func(context.Context, json.RawMessage) error { verified++; return nil }); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Intent.Request{ID: "sbp-175", Name: "package.test", Actor: "automation:autoFortress", AutomationLane: "autoFortress:supply"})
	if receipt.Status != Intent.StatusFailed || receipt.Failure == nil || receipt.Failure.SafetyLock == nil ||
		receipt.Failure.SafetyLock.Opcode != "sbp" || receipt.Failure.SafetyLock.Code != 175 || verified != 0 {
		t.Fatalf("SBP 175 receipt = %s verified=%d failure=%#v", receipt.Status, verified, receipt.Failure)
	}
	view := stateStore.ReadOnlyView()
	if view.Inventory.ConstructionOffers[3857] != 7 || view.Player.Currencies[37] != 1_000_000 {
		t.Fatalf("SBP 175 changed counters or balance: offers=%d balance=%v", view.Inventory.ConstructionOffers[3857], view.Player.Currencies[37])
	}
	if dispatch := view.Inventory.LastPackagePurchaseDispatch; dispatch.PackageID != 3857 || dispatch.Amount != 50 ||
		State.PackageCountersAfterLastPurchase(&view, now) {
		t.Fatalf("dispatched purchase was not recorded against older counters: %#v", dispatch)
	}
	meaning := GameData.ResolveResponseCode(nil, "sbp", 175)
	if !strings.Contains(meaning.Recovery, "Re-enter the destination castle") {
		t.Fatalf("SBP 175 recovery = %q", meaning.Recovery)
	}
}

// Review r4135728774: an unanswered GAA must not block purchases forever. A
// later committed context reply (the plan's own same-castle JCA, answered as
// JAA) settles it without a focus-epoch change; without any reply it stays
// in flight.
func TestSBPGuardSettlesUnansweredMapReadOnLaterContextReply(t *testing.T) {
	now := time.Now().UTC()
	gameState := cit13DirewolfState(now)
	gameState.Player.ID = 42
	stateStore := State.NewStore(&gameState)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, nil, registry)
	code := 0
	enterCastle := func(at time.Time) {
		t.Helper()
		if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "jaa", ResponseCode: &code, ReceivedAt: at,
			Payload: json.RawMessage(`{"KID":0,"gca":{"A":[1,100,100,10,42,0,0,0,0,0,"Main"]},"gui":{"I":[],"TU":[],"HI":[],"SHI":[]}}`),
		}); err != nil {
			t.Fatal(err)
		}
	}
	guard := func() error {
		view := stateStore.PlanningView()
		_, err := validateEventBackedSBP(Intent.PlanningContext{
			State: view.State, GameData: cit13DirewolfCatalog(t), ProtocolContext: view.ProtocolContext,
		}, view.State.Castles[10], eventBackedSBPRequest{PackageID: 3857, TableID: 94, Amount: 1, Stock: 50, MaxBuyPerClick: 1000}, time.Now().UTC(), true)
		return err
	}
	enterCastle(time.Now().UTC())
	// Counters read after the committed focus.
	if _, err := stateStore.ApplyComponents(State.Components(State.ComponentInventory), func(state *State.GameState) ([]string, bool, error) {
		state.Inventory.ConstructionOffersObservedAt = time.Now().UTC().Add(time.Millisecond)
		state.MutableInventoryConstructionOffers()
		return []string{"inventory"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := guard(); err != nil {
		t.Fatalf("settled castle context rejected: %v", err)
	}
	epoch := stateStore.ProtocolContext().FocusEpoch

	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Direction: Protocol.DirectionOutbound, Opcode: "gaa", ReceivedAt: time.Now().UTC(),
		Payload: json.RawMessage(`{"KID":1,"AX1":0,"AY1":0,"AX2":10,"AY2":10}`),
	}); err != nil {
		t.Fatal(err)
	}
	if err := guard(); !errors.Is(err, Intent.ErrPlanStale) || !strings.Contains(err.Error(), "in-flight world-map read") {
		t.Fatalf("unanswered GAA did not hold the purchase: %v", err)
	}
	if err := guard(); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("GAA settled although nothing committed: %v", err)
	}

	enterCastle(time.Now().UTC().Add(time.Millisecond)) // same-castle JCA reply
	if got := stateStore.ProtocolContext().FocusEpoch; got != epoch {
		t.Fatalf("same-castle re-entry changed the focus epoch %d → %d", epoch, got)
	}
	if err := guard(); err != nil {
		t.Fatalf("committed same-castle reply did not settle the map read: %v", err)
	}
}

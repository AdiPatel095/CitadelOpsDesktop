package App

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestPlanAutoBuyerPackagePurchaseRefreshesGuardsAndVerifiesCounter(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC()
	gameState := autoBuyerIntentTestState(now)
	gameState.Player.Currencies[70] = 100
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	gameState.EventScores.ShopByPackage[102] = State.EventShopRoute{EventID: 88, RemainingSec: 3600, ObservedAt: now}
	arguments := json.RawMessage(`{
		"sourceCastleId":10,"shopId":"rift","packageId":102,"amount":1,"targetPurchasesPerReset":1,
		"minimumBalanceReserve":50,"allowRubyPackages":false,"maximumRubySpendPerReset":0,
		"minimumRubyReserve":0,"expectedPurchasedBefore":0,"expectedBalanceBefore":100
	}`)
	plan, err := planAutoBuyerPackagePurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "gbc" || plan.Steps[1].Action != "auto_buyer.package.guard" ||
		plan.Steps[2].Opcode != "sbp" || plan.Steps[3].Opcode != "gbc" || plan.Steps[4].Action != "auto_buyer.package.verify" {
		t.Fatalf("package steps = %#v", plan.Steps)
	}
	var payload struct {
		ProductID int64 `json:"PID"`
		TableID   int64 `json:"TID"`
		Amount    int64 `json:"AMT"`
		BuyAll    int64 `json:"BA"`
		Premium   int64 `json:"PC2"`
	}
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &payload); err != nil || payload.ProductID != 102 ||
		payload.TableID != GameData.AutoBuyerMasterBlacksmithTableID || payload.Amount != 1 || payload.BuyAll != 0 || payload.Premium != -1 {
		t.Fatalf("SBP payload = %#v err=%v", payload, err)
	}

	gameState.Inventory.ConstructionOffers[102] = 1
	_, _, _, err = autoBuyerPackagePurchaseContext(Intent.PlanningContext{State: gameState, GameData: gameData}, arguments, now, true)
	if err == nil {
		t.Fatal("fresh guard accepted a purchase after the server counter changed")
	}
}

func TestPlanAutoBuyerSpecialistPurchaseIsSingleSevenDayRenewal(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	gameState.Player.Resources[2] = 5000
	gameState.Market.BoostersObservedAt = now
	expiresAt := now.Add(13 * 24 * time.Hour)
	gameState.Market.Boosters[0] = State.MarketBoosterState{ID: 0, ExpiresAt: expiresAt, ContinuousPurchaseCount: 1}
	arguments, _ := json.Marshal(autoBuyerSpecialistPurchaseRequest{
		SpecialistID: 0, MinimumDays: 14, MaximumRubyCostPerPurchase: 625, MinimumRubyReserve: 1000,
		ExpectedExpiresAtUnix: expiresAt.Unix(), ExpectedPurchaseCount: 1, ExpectedRubyBalance: 5000, HistoryRefreshSec: 900,
	})
	plan, err := planAutoBuyerSpecialistPurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "boi" || plan.Steps[1].Action != "auto_buyer.specialist.guard" ||
		plan.Steps[2].Opcode != "ovs" || plan.Steps[3].Opcode != "boi" || plan.Steps[4].Action != "auto_buyer.specialist.verify" {
		t.Fatalf("specialist steps = %#v", plan.Steps)
	}
	var payload struct {
		Type     int `json:"T"`
		Position int `json:"PO"`
	}
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &payload); err != nil || payload.Type != 0 || payload.Position != -1 {
		t.Fatalf("overseer payload = %#v err=%v", payload, err)
	}
}

func TestPlanAutoBuyerBoostersRefreshUsesOfficialFeastContextOrder(t *testing.T) {
	plan, err := planAutoBuyerBoostersRefresh(
		t.Context(), Intent.PlanningContext{}, json.RawMessage(`{"feastContext":true}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "fce" || plan.Steps[1].Opcode != "dcl" ||
		plan.Steps[2].Opcode != "boi" || plan.Steps[3].Action != "auto_buyer.feast.refresh.verify" {
		t.Fatalf("feast context refresh steps = %#v", plan.Steps)
	}
	for index, step := range plan.Steps[:3] {
		if step.ResponseBarrier != Intent.ResponseBarrierCommitted || step.ResumePolicy != Intent.ResumeRebuild {
			t.Fatalf("feast context refresh step %d = %#v", index, step)
		}
	}
	if plan.Steps[3].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("feast refresh verifier = %#v", plan.Steps[3])
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"CD":1}` {
		t.Fatalf("DCL payload = %s", got)
	}
	var request autoBuyerBoostersRefreshRequest
	if err := json.Unmarshal(plan.Steps[3].ActionArguments, &request); err != nil || request.FeastRefreshAfter.IsZero() {
		t.Fatalf("feast refresh boundary = %#v err=%v", request, err)
	}

	specialistsOnly, err := planAutoBuyerBoostersRefresh(t.Context(), Intent.PlanningContext{}, json.RawMessage(`{}`))
	if err != nil || len(specialistsOnly.Steps) != 1 || specialistsOnly.Steps[0].Opcode != "boi" {
		t.Fatalf("specialist refresh plan = %#v err=%v", specialistsOnly, err)
	}
}

func TestVerifyAutoBuyerFeastRefreshRejectsOmittedBOIFeast(t *testing.T) {
	cutoff := time.Now().UTC().Truncate(time.Second)
	request, _ := json.Marshal(autoBuyerBoostersRefreshRequest{FeastContext: true, FeastRefreshAfter: cutoff})
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = cutoff.Add(-time.Minute)
	gameState.Market.Feast = State.MarketFeastState{ObservedAt: cutoff.Add(-time.Second)}
	gameState.Market.FeastCostReductionObservedAt = cutoff

	err := verifyAutoBuyerFeastRefreshContext(Intent.PlanningContext{State: gameState}, request)
	if err == nil || !strings.Contains(err.Error(), "omitted feast status") {
		t.Fatalf("omitted BOI feast error = %v", err)
	}

	gameState.Market.Feast.ObservedAt = cutoff
	if err := verifyAutoBuyerFeastRefreshContext(Intent.PlanningContext{State: gameState}, request); err != nil {
		t.Fatalf("authoritative feast refresh rejected: %v", err)
	}
}

func TestPlanAutoBuyerFeastRefreshesResourcesAndTimer(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	gameState.Market.BoostersObservedAt = now
	gameState.Market.FeastCostReductionPercent = 25
	gameState.Market.FeastCostReductionObservedAt = now
	expectedCost := int64(60000)
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedActiveFeastID: 0, ExpectedExpiresAtUnix: 0, ExpectedEffectiveCost: &expectedCost, HistoryRefreshSec: 900,
	})
	plan, err := planAutoBuyerFeastPurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Resolver != "auto_buyer.feast.purchase.build" ||
		plan.Steps[0].AwaitOpcode != "bfs" || plan.Steps[0].CommandDependencies == nil ||
		plan.Steps[0].CommandDependencies.Opcode != "bfs" || plan.Steps[1].Opcode != "boi" ||
		plan.Steps[2].Opcode != "dcl" || plan.Steps[3].Action != "auto_buyer.feast.verify" {
		t.Fatalf("feast steps = %#v", plan.Steps)
	}
	if plan.Steps[0].ResumePolicy == Intent.ResumeRebuild ||
		plan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted || !plan.Steps[0].CaptureResponse {
		t.Fatalf("deferred BFS step = %#v", plan.Steps[0])
	}
	if plan.Steps[1].ResumePolicy != Intent.ResumeRebuild || plan.Steps[2].ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("post-purchase refresh steps must rebuild on resume: %#v", plan.Steps[1:3])
	}
	if plan.Summary != "Start or extend Food feast for 60000 Food" {
		t.Fatalf("discounted feast summary = %q", plan.Summary)
	}
	dependencies, err := (&Application{}).resolveAutoBuyerFeastCommandDependencies(
		t.Context(), Intent.PlanningContext{}, Intent.Step{Payload: plan.Steps[0].CommandDependencies.Payload},
	)
	if err != nil || len(dependencies.Steps) != 3 || dependencies.Steps[0].Opcode != "boi" ||
		dependencies.Steps[1].Opcode != "fce" || dependencies.Steps[2].Opcode != "dcl" {
		t.Fatalf("BFS dependencies = %#v err=%v", dependencies, err)
	}
	var resolved autoBuyerFeastPurchaseRequest
	if err := json.Unmarshal(plan.Steps[0].ResolverArguments, &resolved); err != nil ||
		resolved.ExpectedBalanceBefore == nil || *resolved.ExpectedBalanceBefore != 120000 ||
		resolved.ExpectedEffectiveCost == nil || *resolved.ExpectedEffectiveCost != 60000 || resolved.FeastRefreshAfter.IsZero() {
		t.Fatalf("resolved feast request = %#v err=%v", resolved, err)
	}
	gameState.Market.Feast.ObservedAt = resolved.FeastRefreshAfter
	gameState.Market.FeastCostReductionObservedAt = resolved.FeastRefreshAfter
	castle = gameState.Castles[10]
	castle.FoodBalanceObservedAt = resolved.FeastRefreshAfter
	gameState.Castles[10] = castle
	concrete, err := resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[0].ResolverArguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		FeastID  int64           `json:"T"`
		CastleID State.CastleID  `json:"CID"`
		Kingdom  State.KingdomID `json:"KID"`
		Position int             `json:"PO"`
		Power    int             `json:"PWR"`
	}
	if err := json.Unmarshal(concrete.Command.Payload, &payload); err != nil || payload.FeastID != 0 ||
		payload.CastleID != 10 || payload.Kingdom != 0 || payload.Position != -1 || payload.Power != 0 ||
		concrete.ResponseBarrier != Intent.ResponseBarrierCommitted ||
		concrete.PreDispatchAction != "auto_buyer.feast.purchase.arm" ||
		concrete.DefinitiveSendFailureAction != "auto_buyer.feast.purchase.disarm" ||
		!concrete.ResponseProjectionFailureIndeterminate {
		t.Fatalf("BFS payload = %#v err=%v", payload, err)
	}
}

func TestAutoBuyerFeastPurchaseArmsDurableMarkerBeforeBFSDispatch(t *testing.T) {
	application, engine, sender, arguments := newAutoBuyerFeastIntegrationHarness(t, false)
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "feast-arm-integration", Name: "autoBuyer.feast.purchase",
		Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments,
	})
	if receipt.Status != Intent.StatusIndeterminate ||
		!strings.Contains(receipt.Error, "simulated missing BFS acknowledgement") {
		t.Fatalf("indeterminate BFS receipt = %#v", receipt)
	}
	if sender.markerErr != nil {
		t.Fatalf("marker before BFS dispatch: %v", sender.markerErr)
	}
	if !sender.markerObservedBeforeBFS {
		t.Fatal("BFS reached the sender before its durable marker was observed")
	}
	if got := strings.Join(sender.opcodes, ","); got != "boi,fce,dcl,bfs" {
		t.Fatalf("feast command order = %s", got)
	}
	if sender.bfsMetadata.OperationID != "feast-arm-integration" {
		t.Fatalf("BFS operation ID = %q", sender.bfsMetadata.OperationID)
	}

	assertAutoBuyerFeastPendingMarker(t, application.State.ReadOnlyView().Market, sender.bfsMetadata)
	persisted, err := State.LoadSnapshot(application.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	assertAutoBuyerFeastPendingMarker(t, persisted.Market, sender.bfsMetadata)
}

func TestAutoBuyerFeastPurchaseRunsFullRefreshBFSAndVerificationPath(t *testing.T) {
	application, engine, sender, arguments := newAutoBuyerFeastIntegrationHarness(t, true)
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "feast-success-integration", Name: "autoBuyer.feast.purchase",
		Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments,
	})
	if receipt.Status != Intent.StatusSucceeded {
		t.Fatalf("successful BFS receipt = %#v", receipt)
	}
	if sender.markerErr != nil {
		t.Fatalf("marker before BFS dispatch: %v", sender.markerErr)
	}
	if !sender.markerObservedBeforeBFS {
		t.Fatal("BFS reached the sender before its durable marker was observed")
	}
	if got := strings.Join(sender.opcodes, ","); got != "boi,fce,dcl,bfs,boi,dcl" {
		t.Fatalf("feast command order = %s", got)
	}
	if sender.bfsMetadata.OperationID != "feast-success-integration" {
		t.Fatalf("BFS operation ID = %q", sender.bfsMetadata.OperationID)
	}
	market := application.State.ReadOnlyView().Market
	if market.FeastPurchasePending || market.Feast.ID != 0 || market.FeastLastPurchaseAt.IsZero() ||
		!market.Feast.ActiveAt(time.Now().UTC()) {
		t.Fatalf("verified feast state = %+v", market)
	}
	castle := application.State.ReadOnlyView().Castles[10]
	if castle.Resources[5].Amount != 60000 || castle.FoodBalanceObservedAt.IsZero() ||
		castle.FoodBalanceObservedAt.Before(market.FeastLastPurchaseAt) {
		t.Fatalf("verified post-purchase food state = %+v", castle)
	}
}

func newAutoBuyerFeastIntegrationHarness(
	t *testing.T,
	completePurchase bool,
) (*Application, *Intent.Engine, *autoBuyerFeastArmIntegrationSender, json.RawMessage) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second).Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionPercent = 25

	stateStore := State.NewStore(gameState)
	gameData := autoBuyerIntentTestManager(t)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, gameData, registry)
	application := &Application{
		DataDir: t.TempDir(), State: stateStore, GameData: gameData, Ingest: pipeline,
	}
	sender := &autoBuyerFeastArmIntegrationSender{
		application: application, pipeline: pipeline, completePurchase: completePurchase,
	}
	engine := Intent.NewEngine(Intent.NewRegistry(), stateStore, gameData, sender, pipeline)
	application.Intents = engine
	if err := application.registerAutoBuyerIntents(); err != nil {
		t.Fatal(err)
	}

	arguments, err := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		AttemptAfter: now, HistoryRefreshSec: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application, engine, sender, arguments
}

type autoBuyerFeastArmIntegrationSender struct {
	application             *Application
	pipeline                *Ingest.Pipeline
	opcodes                 []string
	bfsMetadata             Outbound.Metadata
	markerObservedBeforeBFS bool
	markerErr               error
	completePurchase        bool
	purchaseCompleted       bool
	lastResponseAt          time.Time
}

func (*autoBuyerFeastArmIntegrationSender) Ready() bool               { return true }
func (*autoBuyerFeastArmIntegrationSender) Namespace() string         { return "EmpireEx_21" }
func (*autoBuyerFeastArmIntegrationSender) CorrelatesResponses() bool { return true }

func (sender *autoBuyerFeastArmIntegrationSender) Send(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, command.Opcode)
	metadata := Outbound.MetadataFromContext(ctx)
	if command.Opcode == "bfs" {
		sender.bfsMetadata = metadata
		persisted, loadErr := State.LoadSnapshot(sender.application.DataDir)
		if loadErr != nil {
			sender.markerErr = loadErr
		} else if markerErr := validateAutoBuyerFeastPendingMarker(persisted.Market, metadata); markerErr != nil {
			sender.markerErr = markerErr
		} else {
			sender.markerObservedBeforeBFS = true
		}
		if !sender.completePurchase {
			return Outbound.MarkIndeterminate(fmt.Errorf("simulated missing BFS acknowledgement"))
		}
		sender.purchaseCompleted = true
	}

	code := 0
	receivedAt := time.Now().UTC()
	if !receivedAt.After(sender.lastResponseAt) {
		receivedAt = sender.lastResponseAt.Add(time.Microsecond)
	}
	sender.lastResponseAt = receivedAt
	response := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Namespace: command.Namespace, Opcode: command.Opcode,
		ResponseCode: &code, ReceivedAt: receivedAt, ResponseToken: metadata.ResponseToken,
		CausationOperationID: metadata.OperationID,
	}
	switch command.Opcode {
	case "boi":
		if sender.purchaseCompleted {
			response.Payload = json.RawMessage(`{"BO":[],"bfs":{"T":0,"RT":21600}}`)
		} else {
			response.Payload = json.RawMessage(`{"BO":[],"bfs":{"T":-1,"RT":0}}`)
		}
	case "fce":
		response.Payload = json.RawMessage(`{"FRM":25}`)
	case "dcl":
		food := 120000
		if sender.purchaseCompleted {
			food = 60000
		}
		response.Payload, err = json.Marshal(map[string]any{
			"C": []any{map[string]any{"KID": 0, "AI": []any{map[string]any{"AID": 10, "F": food}}}},
		})
	case "bfs":
		response.Payload = json.RawMessage(`{"T":0,"RT":21600}`)
	default:
		return fmt.Errorf("unexpected feast command %q", command.Opcode)
	}
	if err != nil {
		return err
	}
	_, err = sender.pipeline.HandleFrame(ctx, response)
	return err
}

func assertAutoBuyerFeastPendingMarker(t *testing.T, market State.MarketState, metadata Outbound.Metadata) {
	t.Helper()
	if err := validateAutoBuyerFeastPendingMarker(market, metadata); err != nil {
		t.Fatal(err)
	}
}

func validateAutoBuyerFeastPendingMarker(market State.MarketState, metadata Outbound.Metadata) error {
	if !market.FeastPurchasePending || market.FeastPurchaseExpectedID != 0 || market.FeastPurchasePendingSince.IsZero() {
		return fmt.Errorf("pending feast marker = %+v", market)
	}
	wantExpiry := market.FeastPurchasePendingSince.Add(6 * time.Hour)
	if !market.FeastPurchaseExpectedExpiresAt.Equal(wantExpiry) {
		return fmt.Errorf("pending feast expiry = %s, want %s", market.FeastPurchaseExpectedExpiresAt, wantExpiry)
	}
	if metadata.OperationID == "" || metadata.ResponseToken == "" {
		return fmt.Errorf("BFS correlation metadata = %+v", metadata)
	}
	if market.FeastPurchaseOperationID != metadata.OperationID ||
		market.FeastPurchaseResponseToken != metadata.ResponseToken {
		return fmt.Errorf("pending feast correlation = operation %q token %q, want operation %q token %q",
			market.FeastPurchaseOperationID, market.FeastPurchaseResponseToken,
			metadata.OperationID, metadata.ResponseToken,
		)
	}
	return nil
}

func TestAutoBuyerFeastReconciliationOnlySucceedsAfterPendingLatchClears(t *testing.T) {
	attemptedAt := time.Now().UTC().Add(-time.Minute)
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, SourceCastleID: 10, AttemptAfter: attemptedAt,
	})
	plan, err := planAutoBuyerFeastReconcile(t.Context(), Intent.PlanningContext{}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 ||
		plan.Steps[0].Action != "auto_buyer.feast.reconcile.mark" ||
		plan.Steps[1].Opcode != "boi" ||
		plan.Steps[2].Opcode != "dcl" ||
		plan.Steps[3].Action != "auto_buyer.feast.reconcile.verify" {
		t.Fatalf("feast reconciliation steps = %#v", plan.Steps)
	}

	gameState := State.NewGameState()
	gameState.Market.FeastPurchasePending = true
	gameState.Market.FeastPurchaseExpectedID = 0
	application := &Application{State: State.NewStore(gameState)}
	if err := application.verifyAutoBuyerFeastReconciliation(t.Context(), arguments); err == nil ||
		!strings.Contains(err.Error(), "authoritative feast snapshot") {
		t.Fatalf("pending reconciliation error = %v", err)
	}

	gameState.Market.FeastPurchasePending = false
	application.State = State.NewStore(gameState)
	if err := application.verifyAutoBuyerFeastReconciliation(t.Context(), arguments); err != nil {
		t.Fatalf("cleared reconciliation rejected: %v", err)
	}
}

func TestAutoBuyerFeastDefinitiveFailureOnlyDisarmsMatchingDispatch(t *testing.T) {
	arguments := json.RawMessage(`{"feastId":0}`)
	newApplication := func() *Application {
		gameState := State.NewGameState()
		gameState.Market.FeastPurchasePending = true
		gameState.Market.FeastPurchaseExpectedID = 0
		gameState.Market.FeastPurchaseOperationID = "expected-operation"
		gameState.Market.FeastPurchaseResponseToken = "expected-token"
		return &Application{State: State.NewStore(gameState)}
	}
	testCases := []struct {
		name        string
		metadata    Outbound.Metadata
		wantPending bool
	}{
		{
			name:        "other operation",
			metadata:    Outbound.Metadata{OperationID: "other-operation", ResponseToken: "expected-token"},
			wantPending: true,
		},
		{
			name:        "other response",
			metadata:    Outbound.Metadata{OperationID: "expected-operation", ResponseToken: "other-token"},
			wantPending: true,
		},
		{
			name:     "matching dispatch",
			metadata: Outbound.Metadata{OperationID: "expected-operation", ResponseToken: "expected-token"},
		},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			application := newApplication()
			ctx := Outbound.WithMetadata(t.Context(), testCase.metadata)
			if err := application.disarmAutoBuyerFeastPurchase(ctx, arguments); err != nil {
				t.Fatal(err)
			}
			if got := application.State.ReadOnlyView().Market.FeastPurchasePending; got != testCase.wantPending {
				t.Fatalf("pending latch = %t, want %t", got, testCase.wantPending)
			}
		})
	}
}

func TestResolveAutoBuyerFeastRequiresImmediateDCLFoodAuthority(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Add(-time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	plan, err := planAutoBuyerFeastPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"historyRefreshSec":900}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var request autoBuyerFeastPurchaseRequest
	if err := json.Unmarshal(plan.Steps[0].ResolverArguments, &request); err != nil {
		t.Fatal(err)
	}
	gameState.Market.Feast.ObservedAt = request.FeastRefreshAfter
	gameState.Market.FeastCostReductionObservedAt = request.FeastRefreshAfter
	_, err = resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[0].ResolverArguments,
	)
	if err == nil || !strings.Contains(err.Error(), "food balance was not refreshed") {
		t.Fatalf("stale DCL food authority error = %v", err)
	}

	castle = gameState.Castles[10]
	castle.FoodBalanceObservedAt = request.FeastRefreshAfter
	gameState.Castles[10] = castle
	if _, err := resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[0].ResolverArguments,
	); err != nil {
		t.Fatalf("fresh DCL food authority rejected: %v", err)
	}
}

func TestPlanAutoBuyerFeastFailsClosedOnStaleCostReduction(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	gameState.Market.FeastCostReductionObservedAt = now.Add(-901 * time.Second)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedActiveFeastID: 0, HistoryRefreshSec: 900,
	})
	if _, err := planAutoBuyerFeastPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	); err == nil || !strings.Contains(err.Error(), "feast cost reduction is stale") {
		t.Fatalf("stale FCE plan error = %v", err)
	}
}

func TestPlanAutoBuyerFeastBindsPolicyBalanceAndEffectiveCost(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionPercent = 25

	staleBalance := int64(119999)
	expectedCost := int64(60000)
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedBalanceBefore: &staleBalance, ExpectedEffectiveCost: &expectedCost,
		HistoryRefreshSec: 900,
	})
	plan, err := planAutoBuyerFeastPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatalf("fresh food balance should replace policy snapshot: %v", err)
	}
	var resolved autoBuyerFeastPurchaseRequest
	if err := json.Unmarshal(plan.Steps[0].ResolverArguments, &resolved); err != nil {
		t.Fatal(err)
	}
	if resolved.ExpectedBalanceBefore == nil || *resolved.ExpectedBalanceBefore != 120000 {
		t.Fatalf("bound feast balance = %v, want 120000", resolved.ExpectedBalanceBefore)
	}

	currentBalance := int64(120000)
	staleCost := int64(60001)
	arguments, _ = json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedBalanceBefore: &currentBalance, ExpectedEffectiveCost: &staleCost,
		HistoryRefreshSec: 900,
	})
	_, err = planAutoBuyerFeastPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err == nil || !strings.Contains(err.Error(), "effective feast cost changed") {
		t.Fatalf("bound feast plan error = %v, want effective cost change", err)
	}
}

func TestResolveAutoBuyerFeastRejectsBOIThatOmitsFeastState(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Add(-time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionPercent = 25
	plan, err := planAutoBuyerFeastPurchase(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"historyRefreshSec":900}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	var request autoBuyerFeastPurchaseRequest
	if err := json.Unmarshal(plan.Steps[0].ResolverArguments, &request); err != nil {
		t.Fatal(err)
	}
	// FCE and the broad BOI clock advanced, but BOI omitted bfs and therefore
	// preserved the older feast-specific authority timestamp.
	gameState.Market.FeastCostReductionObservedAt = request.FeastRefreshAfter
	gameState.Market.BoostersObservedAt = request.FeastRefreshAfter
	_, err = resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[0].ResolverArguments,
	)
	if err == nil || !strings.Contains(err.Error(), "feast timer was not refreshed") {
		t.Fatalf("omitted BFS preflight error = %v", err)
	}
}

func TestVerifyAutoBuyerRubyFeastChecksActualDebitAgainstCeiling(t *testing.T) {
	expectedBalance := int64(1000)
	expectedCost := int64(250)
	request := autoBuyerFeastPurchaseRequest{
		AllowRubies: true, MaximumRubyCostPerPurchase: 250, MinimumRubyReserve: 500,
		ExpectedBalanceBefore: &expectedBalance, ExpectedEffectiveCost: &expectedCost,
	}
	feast := GameData.AutoBuyerFeast{
		Name:  "Ruby feast",
		Price: GameData.AutoBuyerPrice{Name: "Rubies", Premium: true},
	}
	if err := verifyAutoBuyerFeastBalance(request, feast, 740); err == nil ||
		!strings.Contains(err.Error(), "ruby ceiling") {
		t.Fatalf("ruby debit ceiling error = %v", err)
	}
	if err := verifyAutoBuyerFeastBalance(request, feast, 750); err != nil {
		t.Fatalf("bounded ruby debit rejected: %v", err)
	}
}

func TestVerifyAutoBuyerFeastUsesDispatchBoundCostAfterDelayedResume(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	dispatchedAt := time.Now().UTC().Add(-20 * time.Minute).Truncate(time.Second)
	purchasedAt := dispatchedAt.Add(time.Second)
	verifiedAt := dispatchedAt.Add(20 * time.Minute)
	gameState := autoBuyerIntentTestState(dispatchedAt)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 140000}
	castle.FoodBalanceObservedAt = verifiedAt
	gameState.Castles[10] = castle
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: int(purchasedAt.Add(21600*time.Second).Sub(verifiedAt) / time.Second),
		ObservedAt: verifiedAt, ExpiresAt: purchasedAt.Add(21600 * time.Second),
	}
	gameState.Market.FeastLastPurchaseAt = purchasedAt
	gameState.Market.FeastCostReductionObservedAt = dispatchedAt.Add(-time.Hour)
	expectedBalance, expectedCost := int64(200000), int64(60000)
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedBalanceBefore: &expectedBalance, ExpectedEffectiveCost: &expectedCost,
		FeastRefreshAfter: dispatchedAt, HistoryRefreshSec: 900,
	})
	if err := verifyAutoBuyerFeastPurchaseContext(
		Intent.PlanningContext{State: gameState, GameData: gameData}, arguments, verifiedAt,
	); err != nil {
		t.Fatalf("delayed feast verification rejected its dispatch-bound quote: %v", err)
	}
}

func autoBuyerIntentTestState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now
	gameState.Player.Level = 70
	gameState.Player.LegendLevel = 950
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main", Resources: map[State.ResourceID]State.ResourceBalance{},
	}
	gameState.Market.BoostersObservedAt = now
	gameState.Market.Feast = State.MarketFeastState{ObservedAt: now}
	gameState.Market.FeastCostReductionObservedAt = now
	return gameState
}

const autoBuyerIntentTestCatalog = `{
	"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
	"resources":[{"resourceID":1,"JSONKey":"C1","name":"Coins"},{"resourceID":2,"JSONKey":"C2","name":"Rubies"},{"resourceID":5,"JSONKey":"F","name":"Food"}],
	"currencies":[{"currencyID":36,"JSONKey":"STO","Name":"SilverToken"},{"currencyID":70,"JSONKey":"RCO","Name":"RiftCoin"}],
	"packages":[
		{"packageID":100,"comment1":"Central Silver Shop","stock":5,"costSilverToken":10},
		{"packageID":101,"comment1":"Master Blacksmith Ruby","stock":2,"packagePriceC2":150},
		{"packageID":102,"comment1":"ARE Blacksmith - Rift Coin Package","stock":1,"costRiftCoin":25}
	],
	"feasts":[{"feastID":0,"comment":"Food feast","duration":21600,"productionBoost":80,"costFood":80000},{"feastID":1,"comment":"Ruby feast","duration":21600,"productionBoost":120,"costC2":250}]
}`

func autoBuyerIntentTestStore(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(autoBuyerIntentTestCatalog), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func autoBuyerIntentTestManager(t *testing.T) *GameData.Manager {
	t.Helper()
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vtest.json"), []byte(autoBuyerIntentTestCatalog), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{
		CacheDir: cacheDir, VersionURL: "offline://items-version",
	})
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return manager
}

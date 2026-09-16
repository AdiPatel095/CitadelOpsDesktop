package App

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Configuration"
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
		ExpectedExpiresAtUnix: expiresAt.Unix(), ExpectedRubyBalance: 5000, HistoryRefreshSec: 900,
	})
	plan, err := planAutoBuyerSpecialistPurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "boi" || plan.Steps[1].Resolver != "auto_buyer.specialist.purchase.build" ||
		plan.Steps[2].Opcode != "boi" || plan.Steps[3].Action != "auto_buyer.specialist.verify" {
		t.Fatalf("specialist steps = %#v", plan.Steps)
	}
	resolvedStep, err := resolveAutoBuyerSpecialistPurchaseStep(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[1].ResolverArguments)
	if err != nil || resolvedStep.Opcode != "ovs" || resolvedStep.PreDispatchAction != "auto_buyer.specialist.arm" || resolvedStep.FinalDispatchAction != "auto_buyer.specialist.dispatch" {
		t.Fatalf("resolved specialist step = %#v err=%v", resolvedStep, err)
	}
	var payload struct {
		Type     int `json:"T"`
		Position int `json:"PO"`
	}
	if err := json.Unmarshal(resolvedStep.Command.Payload, &payload); err != nil || payload.Type != 0 || payload.Position != -1 {
		t.Fatalf("overseer payload = %#v err=%v", payload, err)
	}
}

func TestAutoBuyerSpecialistCommandMatrixAndFreshRubyAuthority(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Millisecond)
	expected := map[int]struct {
		opcode   string
		resource int
	}{0: {"ovs", 0}, 1: {"ovs", 1}, 2: {"ovs", 2}, 3: {"ovs", 3}, 4: {"ovs", 4}, 5: {"ovs", 5}, 6: {"bms", 0}, 8: {"btx", 0}, 10: {"bis", 0}}
	for id, want := range expected {
		t.Run(fmt.Sprintf("specialist-%d", id), func(t *testing.T) {
			state := autoBuyerIntentTestState(now)
			state.Player.Resources[2] = 20_000
			request := autoBuyerSpecialistPurchaseRequest{SpecialistID: id, MinimumDays: 14, MaximumRubyCostPerPurchase: 625, MinimumRubyReserve: 1000, ExpectedRubyBalance: 20_000, ExpectedRubyObservedAt: now, ExpectedSessionGeneration: 1, HistoryRefreshSec: 900}
			specialist, _ := GameData.AutoBuyerSpecialistByID(id)
			request.MaximumRubyCostPerPurchase = specialist.ValidatedMaximumRubyCost
			arguments, _ := json.Marshal(request)
			step, err := resolveAutoBuyerSpecialistPurchaseStep(t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, arguments)
			if err != nil || step.Opcode != want.opcode || string(step.Command.Payload) == "" || !step.CaptureResponse || step.ResponseBarrier != Intent.ResponseBarrierCommitted {
				t.Fatalf("step = %#v err=%v", step, err)
			}
			var payload struct {
				Type     *int `json:"T"`
				Position int  `json:"PO"`
			}
			if json.Unmarshal(step.Command.Payload, &payload) != nil || payload.Position != -1 {
				t.Fatalf("payload = %s", step.Command.Payload)
			}
			if want.opcode == "ovs" && (payload.Type == nil || *payload.Type != want.resource) {
				t.Fatalf("OVS payload = %s", step.Command.Payload)
			}
			if want.opcode != "ovs" && payload.Type != nil {
				t.Fatalf("specialist payload has T: %s", step.Command.Payload)
			}
		})
	}
	state := autoBuyerIntentTestState(now)
	state.Player.Resources[2] = 20_000
	delete(state.Player.ResourceObservations, 2)
	request := autoBuyerSpecialistPurchaseRequest{SpecialistID: 0, MinimumDays: 14, MaximumRubyCostPerPurchase: 625, ExpectedRubyBalance: 20_000, HistoryRefreshSec: 900}
	arguments, _ := json.Marshal(request)
	if _, _, err := autoBuyerSpecialistPurchaseContext(Intent.PlanningContext{State: state, GameData: gameData}, arguments, now); err == nil || !strings.Contains(err.Error(), "fresh current-session ruby") {
		t.Fatalf("missing ruby authority error = %v", err)
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

func TestAutoBuyerBoostersRefreshExecutesWithEnforcedResourceDeclarations(t *testing.T) {
	t.Run("specialists", func(t *testing.T) {
		_, engine, sender := newAutoBuyerSpecialistIntegrationHarness(t)
		defer sender.router.Close()
		receipt := engine.Submit(t.Context(), Intent.Request{
			ID: "buyer-specialists-refresh", Name: "autoBuyer.boosters.refresh",
			Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: json.RawMessage(`{}`),
		})
		if receipt.Status != Intent.StatusSucceeded || len(receipt.Exchanges) != 1 || receipt.Exchanges[0].Command.Opcode != "boi" {
			t.Fatalf("specialist refresh receipt = %#v", receipt)
		}
	})

	t.Run("feast", func(t *testing.T) {
		_, engine, sender, _ := newAutoBuyerFeastIntegrationHarness(t, false)
		receipt := engine.Submit(t.Context(), Intent.Request{
			ID: "buyer-feast-refresh", Name: "autoBuyer.boosters.refresh",
			Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: json.RawMessage(`{"feastContext":true}`),
		})
		if receipt.Status != Intent.StatusSucceeded || strings.Join(sender.opcodes, ",") != "fce,dcl,boi" {
			t.Fatalf("feast refresh receipt = %#v opcodes=%v", receipt, sender.opcodes)
		}
	})
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
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
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
		t.Context(), Intent.PlanningContext{State: gameState}, Intent.Step{Payload: plan.Steps[0].CommandDependencies.Payload},
	)
	if err != nil || len(dependencies.Steps) != 4 || dependencies.Steps[0].Opcode != "jaa" ||
		dependencies.Steps[1].Opcode != "boi" || dependencies.Steps[2].Opcode != "fce" ||
		dependencies.Steps[3].Opcode != "dcl" {
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
	castle.FoodEconomyObservedAt = resolved.FeastRefreshAfter
	castle.ContextSnapshotObservedAt = resolved.FeastRefreshAfter
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

func TestResolveAutoBuyerFeastRoutesNumericOwnedOuterKingdomSource(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
	gameState := autoBuyerIntentTestState(now)
	mainCastle := gameState.Castles[10]
	mainCastle.Resources[5] = autoBuyerIntentFoodBalance(120000)
	gameState.Castles[10] = mainCastle
	outerCastle := mainCastle
	outerCastle.ID = 20
	outerCastle.KingdomID = 2
	outerCastle.Name = "Fire Peaks"
	outerCastle.Resources = map[State.ResourceID]State.ResourceBalance{5: autoBuyerIntentFoodBalance(200000)}
	gameState.Castles[20] = outerCastle
	gameState.Market.FeastCostReductionPercent = 25
	expectedBalance, expectedCost := int64(200000), int64(60000)
	arguments, err := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 20, ExpectedSourceKingdomID: 2,
		MinimumFoodReserve: 30000, ExpectedBalanceBefore: &expectedBalance, ExpectedEffectiveCost: &expectedCost,
		AttemptAfter: now, FeastRefreshAfter: now, HistoryRefreshSec: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	step, err := resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if got := string(payload["CID"]); got != "20" {
		t.Fatalf("BFS CID = %s, want numeric 20", got)
	}
	if got := string(payload["KID"]); got != "2" {
		t.Fatalf("BFS KID = %s, want numeric 2", got)
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
	if got := strings.Join(sender.opcodes, ","); got != "jca,boi,fce,dcl,bfs" {
		t.Fatalf("feast command order = %s", got)
	}
	if sender.bfsMetadata.OperationID != "feast-arm-integration" {
		t.Fatalf("BFS operation ID = %q", sender.bfsMetadata.OperationID)
	}

	assertAutoBuyerFeastPendingMarker(t, application.State.ReadOnlyView().Market, sender.bfsMetadata)
	armed := application.State.ReadOnlyView()
	if evidence := armed.Market.LatestFeastPurchase; !evidence.FoodBeforeKnown || evidence.FoodBefore != 130000 ||
		!evidence.FoodBeforeObservedAt.Equal(armed.Castles[10].FoodBalanceObservedAt) {
		t.Fatalf("atomic pre-dispatch food evidence = %+v, castle observed at %s", evidence, armed.Castles[10].FoodBalanceObservedAt)
	}
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
	if got := strings.Join(sender.opcodes, ","); got != "jca,boi,fce,dcl,bfs,boi,dcl" {
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

func TestAutoBuyerSpecialistsExecuteThroughQueuedRouterAndReconcileAllMappings(t *testing.T) {
	application, engine, sender := newAutoBuyerSpecialistIntegrationHarness(t)
	defer sender.router.Close()
	ids := []int{0, 1, 2, 3, 4, 5, 6, 8, 10}
	for _, id := range ids {
		for purchase := 0; purchase < 2; purchase++ {
			state := application.State.ReadOnlyView()
			specialist, _ := GameData.AutoBuyerSpecialistByID(id)
			ruby := state.Player.ResourceObservations[2]
			arguments, _ := json.Marshal(autoBuyerSpecialistPurchaseRequest{SpecialistID: id, MinimumDays: 14, MaximumRubyCostPerPurchase: specialist.ValidatedMaximumRubyCost, MinimumRubyReserve: 1000, ExpectedExpiresAtUnix: autoBuyerIntentUnix(state.Market.Boosters[id].ExpiresAt), ExpectedRubyBalance: int64(state.Player.Resources[2]), ExpectedRubyObservedAt: ruby.ObservedAt, ExpectedSessionGeneration: 1, HistoryRefreshSec: 900})
			receipt := engine.Submit(t.Context(), Intent.Request{ID: fmt.Sprintf("specialist-%d-%d", id, purchase), Name: "autoBuyer.specialist.purchase", Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments})
			if receipt.Status != Intent.StatusSucceeded || len(receipt.Exchanges) == 0 || len(receipt.Evidence) == 0 {
				t.Fatalf("specialist %d purchase %d receipt = %#v", id, purchase, receipt)
			}
			if !bytes.Contains(receipt.Evidence[len(receipt.Evidence)-1].Data, []byte(`"specialistId":`+strconv.Itoa(id))) {
				t.Fatalf("specialist %d durable operation evidence = %s", id, receipt.Evidence[len(receipt.Evidence)-1].Data)
			}
			foundRawResult := false
			for _, exchange := range receipt.Exchanges {
				if exchange.Response != nil && strings.EqualFold(exchange.Response.Opcode, specialist.Opcode) &&
					bytes.Contains(exchange.Response.Payload, []byte(`"gcu"`)) && bytes.Contains(exchange.Response.Payload, []byte(`"boi"`)) {
					foundRawResult = true
				}
			}
			if !foundRawResult {
				t.Fatalf("specialist %d operation receipt omitted raw request/result: %#v", id, receipt.Exchanges)
			}
			market := application.State.ReadOnlyView().Market
			if market.SpecialistPurchasePending || !market.LatestSpecialistPurchase.ActivationConfirmed || market.LatestSpecialistPurchase.SpecialistID != id {
				t.Fatalf("specialist %d evidence = %#v", id, market.LatestSpecialistPurchase)
			}
			if market.LatestSpecialistPurchase.DebitVerification != "command-local-observed" ||
				!market.LatestSpecialistPurchase.RubyAfterObservedAt.Before(market.LatestSpecialistPurchase.TimerAfterObservedAt) {
				t.Fatalf("specialist %d ack/post-refresh evidence ordering = %#v", id, market.LatestSpecialistPurchase)
			}
		}
	}
	if sender.sentByID[0] != 2 || sender.sentByID[10] != 2 {
		t.Fatalf("specialist sends = %#v", sender.sentByID)
	}
	persisted, err := State.LoadSnapshot(application.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if persisted.Market.SpecialistPurchasePending || !persisted.Market.LatestSpecialistPurchase.ActivationConfirmed {
		t.Fatalf("persisted specialist evidence = %#v", persisted.Market)
	}
}

func TestAutoBuyerSpecialistRestartRecoveryRequiresCorrelatedActivationEvidence(t *testing.T) {
	now := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	for _, testCase := range []struct {
		name          string
		correlatedAck bool
		wantPending   bool
		wantOutcome   string
	}{
		{name: "captured correlated ack", correlatedAck: true, wantOutcome: "confirmed"},
		{name: "uncorrelated later timer", wantPending: true, wantOutcome: "unresolved"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state := autoBuyerIntentTestState(now)
			state.Player.Resources[2] = 99_375
			state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now.Add(5 * time.Second), ConnectionGeneration: 1}
			state.Market.SpecialistPurchasePending = true
			state.Market.SpecialistPurchasePendingSince = now
			state.Market.SpecialistPurchaseExpectedID = 0
			state.Market.SpecialistPurchasePreviousExpiry = time.Time{}
			state.Market.SpecialistPurchaseMaximumExpiry = now.Add(7 * 24 * time.Hour)
			state.Market.SpecialistPurchaseOperationID = "specialist-restart"
			state.Market.SpecialistPurchaseResponseToken = "specialist-restart/2"
			state.Market.SpecialistPurchaseRubyResourceID = 2
			state.Market.LatestSpecialistPurchase = State.SpecialistPurchaseEvidence{Outcome: "purchasing", SpecialistID: 0, Opcode: "ovs", AttemptedAt: now, UpdatedAt: now, ValidatedMaximumCost: 625, ConfiguredRubyCeiling: 625, MinimumRubyReserve: 1000, RubyBefore: 100_000, RubyBeforeKnown: true, RubyBeforeObservedAt: now.Add(-time.Second), DebitVerification: "unresolved"}
			expiry := now.Add(7 * 24 * time.Hour)
			if testCase.correlatedAck {
				state.Market.SpecialistPurchaseResponseConfirmedAt = now.Add(5 * time.Second)
				state.Market.SpecialistPurchaseResponseExpiresAt = expiry
				state.Market.SpecialistPurchaseResponseRuby = 99_375
				state.Market.SpecialistPurchaseResponseRubyAt = now.Add(5 * time.Second)
			}
			state.Market.Boosters = map[int]State.MarketBoosterState{0: {ID: 0, RemainingSec: 7 * 24 * 60 * 60, ExpiresAt: expiry}}
			state.Market.BoostersObservedAt = now.Add(10 * time.Second)
			state.Market.BoostersObservedGeneration = 1
			dataDir := t.TempDir()
			if err := State.SaveSnapshot(dataDir, state); err != nil {
				t.Fatal(err)
			}
			loaded, err := State.LoadSnapshot(dataDir)
			if err != nil {
				t.Fatal(err)
			}
			store := State.NewStore(loaded)
			application := &Application{DataDir: dataDir, State: store, GameData: autoBuyerIntentTestManager(t)}
			if err := application.reconcileAutoBuyerSpecialistPurchase(t.Context(), json.RawMessage(`{"specialistId":0}`)); err != nil {
				t.Fatal(err)
			}
			if !application.State.ReadOnlyView().Market.SpecialistPurchasePending {
				t.Fatal("restart trusted persisted booster authority before a new-session BOI")
			}
			_, err = store.ApplyComponents(State.Components(State.ComponentSession, State.ComponentMarket), func(current *State.GameState) ([]string, bool, error) {
				current.Session.ConnectionGeneration = 2
				current.Session.LoggedIn = true
				current.Session.SocketReady = true
				current.Session.ChangedAt = now.Add(6 * time.Second)
				current.Market.BoostersObservedAt = now.Add(10 * time.Second)
				current.Market.BoostersObservedGeneration = 2
				return []string{"session", "boosters"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := application.reconcileAutoBuyerSpecialistPurchase(t.Context(), json.RawMessage(`{"specialistId":0}`)); err != nil {
				t.Fatal(err)
			}
			market := application.State.ReadOnlyView().Market
			if market.SpecialistPurchasePending != testCase.wantPending || market.LatestSpecialistPurchase.Outcome != testCase.wantOutcome {
				t.Fatalf("restart reconciliation = %+v", market)
			}
			if testCase.correlatedAck && market.LatestSpecialistPurchase.DebitVerification != "command-local-observed" {
				t.Fatalf("restart command-local evidence = %+v", market.LatestSpecialistPurchase)
			}
		})
	}
}

func TestAutoBuyerSpecialistFinalGuardRejectsEveryQueuedEligibilityChange(t *testing.T) {
	testCases := []struct {
		name   string
		mutate func(*testing.T, *Application)
	}{
		{name: "master disabled", mutate: func(t *testing.T, application *Application) {
			if _, err := application.Configuration.Update("automation.enabled", json.RawMessage(`{"auto_buyer":false}`)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "Bot Lock enabled", mutate: func(t *testing.T, application *Application) {
			if _, err := application.Configuration.Update("scheduler", json.RawMessage(`{"botLocked":true}`)); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "lane safety lock enabled", mutate: func(t *testing.T, application *Application) {
			_, err := application.State.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
				automation := state.Automations["autoBuyer"]
				automation.SafetyLock = State.AutomationSafetyLock{OperationID: "prior-rejection", Opcode: "ovs", Code: 269, Until: time.Now().UTC().Add(time.Hour)}
				state.Automations["autoBuyer"] = automation
				return []string{"automations"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "session changed", mutate: func(t *testing.T, application *Application) {
			_, err := application.State.ApplyComponents(State.Components(State.ComponentSession), func(state *State.GameState) ([]string, bool, error) {
				state.Session.ConnectionGeneration++
				state.Session.SocketReady = false
				return []string{"session"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "ceiling changed", mutate: func(t *testing.T, application *Application) {
			raw := application.Configuration.Snapshot().Sections["automation.autoBuyer"]
			changed := bytes.Replace(raw, []byte(`"maximumRubyCostPerPurchase":625`), []byte(`"maximumRubyCostPerPurchase":624`), 1)
			if bytes.Equal(raw, changed) {
				t.Fatal("specialist ceiling fixture was not found")
			}
			if _, err := application.Configuration.Update("automation.autoBuyer", changed); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "timer became covered", mutate: func(t *testing.T, application *Application) {
			_, err := application.State.ApplyComponents(State.Components(State.ComponentMarket), func(state *State.GameState) ([]string, bool, error) {
				state.Market.Boosters[0] = State.MarketBoosterState{ID: 0, RemainingSec: 15 * 24 * 60 * 60, ExpiresAt: time.Now().UTC().Add(15 * 24 * time.Hour)}
				state.Market.BoostersObservedAt = time.Now().UTC()
				return []string{"boosters"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "ruby sample changed", mutate: func(t *testing.T, application *Application) {
			_, err := application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
				state.Player.Resources[2]--
				state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: state.Session.ConnectionGeneration}
				return []string{"resources"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			application, _, sender := newAutoBuyerSpecialistIntegrationHarness(t)
			defer sender.router.Close()
			state := application.State.ReadOnlyView()
			specialist, _ := GameData.AutoBuyerSpecialistByID(0)
			ruby := state.Player.ResourceObservations[2]
			initial, _ := json.Marshal(autoBuyerSpecialistPurchaseRequest{SpecialistID: 0, MinimumDays: 14, MaximumRubyCostPerPurchase: specialist.ValidatedMaximumRubyCost, MinimumRubyReserve: 1000, ExpectedExpiresAtUnix: autoBuyerIntentUnix(state.Market.Boosters[0].ExpiresAt), ExpectedRubyBalance: int64(state.Player.Resources[2]), ExpectedRubyObservedAt: ruby.ObservedAt, ExpectedSessionGeneration: 1, HistoryRefreshSec: 900})
			step, err := resolveAutoBuyerSpecialistPurchaseStep(t.Context(), Intent.PlanningContext{State: state, GameData: mustAutoBuyerGameData(t, application)}, initial)
			if err != nil {
				t.Fatal(err)
			}
			metadata := Outbound.Metadata{OperationID: "queued-specialist", ResponseToken: "queued-specialist/2"}
			ctx := Outbound.WithMetadata(t.Context(), metadata)
			if err := application.armAutoBuyerSpecialistPurchase(ctx, step.PreDispatchArguments); err != nil {
				t.Fatal(err)
			}
			testCase.mutate(t, application)
			if err := application.guardAutoBuyerSpecialistPurchase(ctx, step.FinalDispatchArguments); err == nil {
				t.Fatal("queued specialist command remained eligible after final-boundary mutation")
			}
			if err := application.disarmAutoBuyerSpecialistPurchase(ctx, step.DefinitiveSendFailureArguments); err != nil {
				t.Fatal(err)
			}
			market := application.State.ReadOnlyView().Market
			if market.SpecialistPurchasePending || market.LatestSpecialistPurchase.Outcome != "not-sent" {
				t.Fatalf("matching final-guard compensation = %+v", market)
			}
		})
	}
}

func mustAutoBuyerGameData(t *testing.T, application *Application) *GameData.Store {
	t.Helper()
	store, ready := application.GameData.Current()
	if !ready || store == nil {
		t.Fatal("test game data unavailable")
	}
	return store
}

type autoBuyerSpecialistIntegrationSender struct {
	router   *Outbound.Router
	pipeline *Ingest.Pipeline
	expires  map[int]time.Time
	rubies   int64
	sentByID map[int]int
	debit    int64
	omitGCU  bool
	reject   bool
}

func (sender *autoBuyerSpecialistIntegrationSender) Ready() bool                  { return true }
func (sender *autoBuyerSpecialistIntegrationSender) Namespace() string            { return "EmpireEx_21" }
func (sender *autoBuyerSpecialistIntegrationSender) CorrelatesResponses() bool    { return true }
func (sender *autoBuyerSpecialistIntegrationSender) ConnectionGeneration() uint64 { return 1 }
func (sender *autoBuyerSpecialistIntegrationSender) Send(ctx context.Context, payload []byte) error {
	return sender.router.Send(ctx, payload)
}

func newAutoBuyerSpecialistIntegrationHarness(t *testing.T) (*Application, *Intent.Engine, *autoBuyerSpecialistIntegrationSender) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
	state := autoBuyerIntentTestState(now)
	state.Player.Resources[2] = 100_000
	state.Automations["autoBuyer"] = State.AutomationState{ID: "autoBuyer", Enabled: true}
	store := State.NewStore(state)
	_, _ = store.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
		current.Player.Resources[2] = 100_000
		current.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 1}
		return []string{"resources"}, true, nil
	})
	gameData := autoBuyerIntentTestManager(t)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, gameData, registry)
	dataDir := t.TempDir()
	settings := `{"version":1,"checkIntervalSec":1800,"historyRefreshSec":900,"minimumRubyReserve":1000,"packages":[],"specialists":[{"enabled":true,"id":0,"minimumDays":14,"maximumRubyCostPerPurchase":625},{"enabled":true,"id":1,"minimumDays":14,"maximumRubyCostPerPurchase":625},{"enabled":true,"id":2,"minimumDays":14,"maximumRubyCostPerPurchase":625},{"enabled":true,"id":3,"minimumDays":14,"maximumRubyCostPerPurchase":625},{"enabled":true,"id":4,"minimumDays":14,"maximumRubyCostPerPurchase":625},{"enabled":true,"id":5,"minimumDays":14,"maximumRubyCostPerPurchase":4900},{"enabled":true,"id":6,"minimumDays":14,"maximumRubyCostPerPurchase":990},{"enabled":true,"id":8,"minimumDays":14,"maximumRubyCostPerPurchase":750},{"enabled":true,"id":10,"minimumDays":14,"maximumRubyCostPerPurchase":990}],"feast":{"enabled":false}}`
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{"automation.autoBuyer": json.RawMessage(settings), "automation.enabled": json.RawMessage(`{"auto_buyer":true}`), "scheduler": json.RawMessage(`{"botLocked":false}`)})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{DataDir: dataDir, State: store, GameData: gameData, Configuration: configuration, Ingest: pipeline}
	sender := &autoBuyerSpecialistIntegrationSender{pipeline: pipeline, expires: map[int]time.Time{}, rubies: 100_000, sentByID: map[int]int{}}
	sender.router = Outbound.NewRouter(t.Context(), Outbound.Config{Ready: func() bool { return true }, Send: func(ctx context.Context, payload []byte) error { return sender.dispatch(ctx, payload) }})
	intentRegistry := Intent.NewRegistry()
	intentRegistry.EnforceResourceDeclarations()
	engine := Intent.NewEngine(intentRegistry, store, gameData, sender, pipeline)
	application.Intents = engine
	if err := application.registerAutoBuyerIntents(); err != nil {
		t.Fatal(err)
	}
	return application, engine, sender
}

func (sender *autoBuyerSpecialistIntegrationSender) dispatch(ctx context.Context, payload []byte) error {
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	metadata := Outbound.MetadataFromContext(ctx)
	receivedAt := time.Now().UTC()
	code := 0
	response := Protocol.Frame{Direction: Protocol.DirectionInbound, Namespace: command.Namespace, Opcode: command.Opcode, ResponseCode: &code, ReceivedAt: receivedAt, ResponseToken: metadata.ResponseToken, CausationOperationID: metadata.OperationID}
	rows := func() []map[string]any {
		result := []map[string]any{}
		for id, expiry := range sender.expires {
			remaining := int64(expiry.Sub(receivedAt) / time.Second)
			if remaining < 0 {
				remaining = 0
			}
			result = append(result, map[string]any{"ID": id, "RT": remaining, "PC": 0})
		}
		return result
	}
	switch command.Opcode {
	case "boi":
		response.Payload, _ = json.Marshal(map[string]any{"BO": rows()})
	case "ovs", "bms", "btx", "bis":
		id := 0
		if command.Opcode == "ovs" {
			var body struct {
				Type int `json:"T"`
			}
			if json.Unmarshal(command.Payload, &body) != nil {
				return fmt.Errorf("bad ovs")
			}
			id = body.Type
		} else if command.Opcode == "bms" {
			id = 6
		} else if command.Opcode == "btx" {
			id = 8
		} else {
			id = 10
		}
		if sender.reject {
			code = 269
			response.ResponseCode = &code
			response.Payload = json.RawMessage(`{}`)
			break
		}
		specialist, _ := GameData.AutoBuyerSpecialistByID(id)
		baseline := receivedAt
		if sender.expires[id].After(baseline) {
			baseline = sender.expires[id]
		}
		sender.expires[id] = baseline.Add(time.Duration(specialist.DurationSec) * time.Second)
		debit := specialist.ValidatedMaximumRubyCost
		if sender.debit != 0 {
			debit = sender.debit
		}
		sender.rubies -= debit
		sender.sentByID[id]++
		body := map[string]any{"boi": map[string]any{"BO": rows()}}
		if !sender.omitGCU {
			body["gcu"] = map[string]any{"C2": sender.rubies}
		}
		response.Payload, _ = json.Marshal(body)
	default:
		return fmt.Errorf("unexpected specialist opcode %s", command.Opcode)
	}
	_, err = sender.pipeline.HandleFrame(ctx, response)
	return err
}

func TestAutoBuyerSpecialistRejectedAndIncompleteResponsesKeepPerOperationEvidence(t *testing.T) {
	for _, testCase := range []struct {
		name        string
		configure   func(*autoBuyerSpecialistIntegrationSender)
		wantPending bool
		wantOutcome string
	}{
		{name: "definitive rejection", configure: func(sender *autoBuyerSpecialistIntegrationSender) { sender.reject = true }, wantOutcome: "rejected"},
		{name: "accepted without command-local C2", configure: func(sender *autoBuyerSpecialistIntegrationSender) { sender.omitGCU = true }, wantPending: true, wantOutcome: "unresolved"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			application, engine, sender := newAutoBuyerSpecialistIntegrationHarness(t)
			defer sender.router.Close()
			testCase.configure(sender)
			state := application.State.ReadOnlyView()
			specialist, _ := GameData.AutoBuyerSpecialistByID(0)
			ruby := state.Player.ResourceObservations[2]
			arguments, _ := json.Marshal(autoBuyerSpecialistPurchaseRequest{SpecialistID: 0, MinimumDays: 14, MaximumRubyCostPerPurchase: specialist.ValidatedMaximumRubyCost, MinimumRubyReserve: 1000, ExpectedExpiresAtUnix: autoBuyerIntentUnix(state.Market.Boosters[0].ExpiresAt), ExpectedRubyBalance: int64(state.Player.Resources[2]), ExpectedRubyObservedAt: ruby.ObservedAt, ExpectedSessionGeneration: 1, HistoryRefreshSec: 900})
			receipt := engine.Submit(t.Context(), Intent.Request{ID: "specialist-incomplete-" + strings.ReplaceAll(testCase.name, " ", "-"), Name: "autoBuyer.specialist.purchase", Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments})
			if receipt.Status == Intent.StatusSucceeded || len(receipt.Exchanges) == 0 || len(receipt.Evidence) == 0 {
				t.Fatalf("incomplete operation receipt = %#v", receipt)
			}
			market := application.State.ReadOnlyView().Market
			if market.SpecialistPurchasePending != testCase.wantPending || market.LatestSpecialistPurchase.Outcome != testCase.wantOutcome {
				t.Fatalf("incomplete specialist state = %+v", market)
			}
			latestEvidence := receipt.Evidence[len(receipt.Evidence)-1].Data
			if !bytes.Contains(latestEvidence, []byte(`"originOperationId":"`+receipt.ID+`"`)) {
				t.Fatalf("attributed incomplete evidence = %s", latestEvidence)
			}
			if testCase.wantPending && !bytes.Contains(latestEvidence, []byte(`"outcome":"`+testCase.wantOutcome+`"`)) {
				t.Fatalf("unresolved operation evidence = %s", latestEvidence)
			}
			if !testCase.wantPending && (receipt.Failure == nil || receipt.Failure.GameCode == nil || *receipt.Failure.GameCode != 269) {
				t.Fatalf("rejected operation result evidence = %#v", receipt.Failure)
			}
		})
	}
}

func TestAutoBuyerSpecialistUnverifiedSpendingStaysLatchedAndCannotDispatchAgain(t *testing.T) {
	for _, testCase := range []struct {
		name             string
		debit            int64
		wantVerification string
	}{
		{name: "debit exceeds official and saved ceiling", debit: 1000, wantVerification: "discrepancy"},
		{name: "negative debit is unattributed", debit: -100, wantVerification: "observed-unattributed"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			application, engine, sender := newAutoBuyerSpecialistIntegrationHarness(t)
			defer sender.router.Close()
			sender.debit = testCase.debit
			state := application.State.ReadOnlyView()
			specialist, _ := GameData.AutoBuyerSpecialistByID(0)
			ruby := state.Player.ResourceObservations[2]
			arguments, _ := json.Marshal(autoBuyerSpecialistPurchaseRequest{SpecialistID: 0, MinimumDays: 14, MaximumRubyCostPerPurchase: specialist.ValidatedMaximumRubyCost, MinimumRubyReserve: 1000, ExpectedExpiresAtUnix: autoBuyerIntentUnix(state.Market.Boosters[0].ExpiresAt), ExpectedRubyBalance: int64(state.Player.Resources[2]), ExpectedRubyObservedAt: ruby.ObservedAt, ExpectedSessionGeneration: 1, HistoryRefreshSec: 900})
			receipt := engine.Submit(t.Context(), Intent.Request{ID: "specialist-spending-unresolved", Name: "autoBuyer.specialist.purchase", Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments})
			if receipt.Status == Intent.StatusSucceeded {
				t.Fatalf("unverified spend receipt = %#v", receipt)
			}
			market := application.State.ReadOnlyView().Market
			if !market.SpecialistPurchasePending || !market.LatestSpecialistPurchase.ActivationConfirmed ||
				market.LatestSpecialistPurchase.Outcome != "activation-confirmed-spend-unresolved" ||
				market.LatestSpecialistPurchase.DebitVerification != testCase.wantVerification {
				t.Fatalf("unverified spending evidence = %+v", market.LatestSpecialistPurchase)
			}

			gameData, ready := application.GameData.Current()
			if !ready {
				t.Fatal("game data unavailable")
			}
			decision, err := Automation.NewAutoBuyerPolicy().Evaluate(t.Context(), Automation.Snapshot{
				State:         application.State.ReadOnlyView(),
				GameData:      gameData,
				Configuration: application.Configuration.Snapshot(),
				Now:           market.SpecialistPurchasePendingSince.Add(31 * time.Second),
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.specialist.reconcile" {
				t.Fatalf("next policy after unresolved spend = %#v err=%v", decision, err)
			}

			second := engine.Submit(t.Context(), Intent.Request{ID: "specialist-second-dispatch", Name: "autoBuyer.specialist.purchase", Actor: "automation:autoBuyer", AutomationLane: "autoBuyer", Arguments: arguments})
			if second.Status == Intent.StatusSucceeded || sender.sentByID[0] != 1 {
				t.Fatalf("second purchase was not blocked: receipt=%#v sends=%#v", second, sender.sentByID)
			}
		})
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
	castle.Focused = true
	food := castle.Resources[5]
	food.Amount = 120000
	castle.Resources[5] = food
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionPercent = 25
	gameState.Automations["autoBuyer"] = State.AutomationState{ID: "autoBuyer", Enabled: true}

	stateStore := State.NewStore(gameState)
	gameData := autoBuyerIntentTestManager(t)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, gameData, registry)
	dataDir := t.TempDir()
	configuration, err := Configuration.Open(dataDir, map[string]json.RawMessage{
		"automation.autoBuyer": json.RawMessage(`{
			"version":1,"checkIntervalSec":1800,"historyRefreshSec":900,"minimumRubyReserve":0,
			"packages":[],"specialists":[],
			"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":0,
				"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
		}`),
		"automation.enabled": json.RawMessage(`{"auto_buyer":true}`),
		"scheduler":          json.RawMessage(`{"botLocked":false}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{
		DataDir: dataDir, State: stateStore, GameData: gameData, Configuration: configuration, Ingest: pipeline,
	}
	sender := &autoBuyerFeastArmIntegrationSender{
		application: application, pipeline: pipeline, completePurchase: completePurchase,
	}
	intentRegistry := Intent.NewRegistry()
	intentRegistry.EnforceResourceDeclarations()
	engine := Intent.NewEngine(intentRegistry, stateStore, gameData, sender, pipeline)
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
	case "jca":
		response.Opcode = "jaa"
		response.Payload = json.RawMessage(`{"KID":0,"gca":{"A":[1,0,0,10,0,0,0,0,0,0,"Main"]}}`)
	case "boi":
		if sender.purchaseCompleted {
			response.Payload = json.RawMessage(`{"BO":[],"bfs":{"T":0,"RT":21600}}`)
		} else {
			response.Payload = json.RawMessage(`{"BO":[],"bfs":{"T":-1,"RT":0}}`)
		}
	case "fce":
		response.Payload = json.RawMessage(`{"FRM":25}`)
	case "dcl":
		food := 130000
		if sender.purchaseCompleted {
			food = 60000
		}
		response.Payload, err = json.Marshal(map[string]any{
			"C": []any{map[string]any{"KID": 0, "AI": []any{map[string]any{
				"AID": 10, "F": food, "gpa": map[string]any{"DF": 1000, "DFC": 100},
			}}}},
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

func TestAutoBuyerFeastReconciliationRequiresTimerProgressAndFreshChargedCastle(t *testing.T) {
	attemptedAt := time.Now().UTC().Add(-time.Minute)
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, SourceCastleID: 10, ExpectedSourceKingdomID: 0,
		AttemptAfter: attemptedAt, HistoryRefreshSec: 900,
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

	gameState := autoBuyerIntentTestState(attemptedAt)
	gameState.Market.FeastPurchasePending = true
	gameState.Market.FeastPurchaseExpectedID = 0
	gameState.Market.FeastPurchasePendingSince = attemptedAt
	gameState.Market.FeastPurchasePreviousExpiresAt = attemptedAt.Add(time.Hour)
	gameState.Market.LatestFeastPurchase = State.FeastPurchaseEvidence{
		Outcome: "uncertain", FeastID: 0, ChargedCastleID: 10, ChargedKingdomID: 0, AttemptedAt: attemptedAt,
	}
	application := &Application{State: State.NewStore(gameState), GameData: autoBuyerIntentTestManager(t)}
	if err := application.verifyAutoBuyerFeastReconciliation(t.Context(), arguments); err == nil ||
		!strings.Contains(err.Error(), "authoritative increased timer") {
		t.Fatalf("pending reconciliation error = %v", err)
	}

	jitterObservedAt := attemptedAt.Add(900 * time.Millisecond)
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: 60 * 60, ExpiresAt: jitterObservedAt.Add(time.Hour), ObservedAt: jitterObservedAt,
	}
	castle := gameState.Castles[10]
	castle.FoodBalanceObservedAt = jitterObservedAt
	gameState.Castles[10] = castle
	application.State = State.NewStore(gameState)
	if err := application.verifyAutoBuyerFeastReconciliation(t.Context(), arguments); err == nil ||
		!strings.Contains(err.Error(), "authoritative increased timer") || !application.State.ReadOnlyView().Market.FeastPurchasePending {
		t.Fatalf("fractional timer jitter reconciliation error = %v market=%+v", err, application.State.ReadOnlyView().Market)
	}

	refreshedAt := time.Now().UTC()
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: 6 * 60 * 60, ExpiresAt: refreshedAt.Add(6 * time.Hour), ObservedAt: refreshedAt,
	}
	castle = gameState.Castles[10]
	castle.FoodBalanceObservedAt = refreshedAt
	castle.Resources[5] = autoBuyerIntentFoodBalance(60000)
	gameState.Castles[10] = castle
	application.State = State.NewStore(gameState)
	if err := application.verifyAutoBuyerFeastReconciliation(t.Context(), arguments); err != nil {
		t.Fatalf("lost-response reconciliation rejected: %v", err)
	}
	market := application.State.ReadOnlyView().Market
	if market.FeastPurchasePending || market.LatestFeastPurchase.Outcome != "reconciled-unverified" ||
		market.LatestFeastPurchase.ActivationConfirmed || !market.LatestFeastPurchase.FoodAfterKnown {
		t.Fatalf("reconciled evidence = %+v", market)
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
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
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
	castle.FoodEconomyObservedAt = request.FeastRefreshAfter
	castle.ContextSnapshotObservedAt = request.FeastRefreshAfter
	gameState.Castles[10] = castle
	if _, err := resolveAutoBuyerFeastPurchaseStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, plan.Steps[0].ResolverArguments,
	); err != nil {
		t.Fatalf("fresh DCL food authority rejected: %v", err)
	}
}

func TestAutoBuyerFeastPreflightAcceptsFractionalBOITimerSamplingDrift(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	observedAt := time.Date(2026, time.September, 15, 14, 0, 0, 900000000, time.UTC)
	gameState := autoBuyerIntentTestState(observedAt)
	castle := gameState.Castles[10]
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionPercent = 25
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: 6 * 60 * 60, ObservedAt: observedAt, ExpiresAt: observedAt.Add(6 * time.Hour),
	}
	expectedExpiry := gameState.Market.Feast.ExpiresAt.Add(-100 * time.Millisecond)
	expectedBalance, expectedCost := int64(120000), int64(60000)
	arguments, err := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, ExpectedSourceKingdomID: 0,
		MinimumFoodReserve: 30000, ExpectedActiveFeastID: 0, ExpectedExpiresAtUnix: expectedExpiry.Unix(),
		ExpectedExpiresAt: expectedExpiry, ExpectedBalanceBefore: &expectedBalance, ExpectedEffectiveCost: &expectedCost,
		AttemptAfter: observedAt.Add(-time.Second), FeastRefreshAfter: observedAt, HistoryRefreshSec: 900,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := autoBuyerFeastPurchaseContext(
		Intent.PlanningContext{State: gameState, GameData: gameData}, arguments, observedAt.Add(100*time.Millisecond), true,
	); err != nil {
		t.Fatalf("fractional BOI sampling drift rejected: %v", err)
	}
}

func TestPlanAutoBuyerFeastFailsClosedOnStaleCostReduction(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	gameState.Market.FeastCostReductionObservedAt = now.Add(-901 * time.Second)
	castle := gameState.Castles[10]
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
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
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
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
	castle.Resources[5] = autoBuyerIntentFoodBalance(120000)
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
	castle.Resources[5] = autoBuyerIntentFoodBalance(140000)
	castle.FoodBalanceObservedAt = verifiedAt
	gameState.Castles[10] = castle
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: int(purchasedAt.Add(21600*time.Second).Sub(verifiedAt) / time.Second),
		ObservedAt: verifiedAt, ExpiresAt: purchasedAt.Add(21600 * time.Second),
	}
	gameState.Market.FeastLastPurchaseAt = purchasedAt
	gameState.Market.LatestFeastPurchase = State.FeastPurchaseEvidence{
		Outcome: "confirmed", FeastID: 0, ChargedCastleID: 10, ChargedKingdomID: 0,
		ActivationConfirmed: true, ActivationConfirmedAt: verifiedAt,
	}
	richerCastle := gameState.Castles[10]
	richerCastle.ID = 20
	richerCastle.KingdomID = 2
	richerCastle.Name = "Fire Peaks"
	richerCastle.Resources = map[State.ResourceID]State.ResourceBalance{5: autoBuyerIntentFoodBalance(500000)}
	gameState.Castles[20] = richerCastle
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

func TestValidateAutoBuyerFeastDispatchFailsClosedAtFinalBoundary(t *testing.T) {
	now := time.Now().UTC().Add(-time.Second).Truncate(time.Millisecond)
	expectedBalance, expectedCost := int64(120000), int64(60000)
	request := autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, ExpectedSourceKingdomID: 0,
		MinimumFoodReserve: 30000, ExpectedBalanceBefore: &expectedBalance, ExpectedEffectiveCost: &expectedCost,
		AttemptAfter: now, FeastRefreshAfter: now, HistoryRefreshSec: 900,
	}
	arguments, err := json.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name      string
		mutate    func(*State.GameState)
		settings  string
		enabled   string
		scheduler string
		wantError string
	}{
		{name: "current state"},
		{name: "saved automation master disabled", enabled: `{"auto_buyer":false}`, wantError: "disabled before feast dispatch"},
		{name: "saved timed automation master expired", enabled: fmt.Sprintf(
			`{"auto_buyer":{"enabled":true,"expiresAt":%q}}`, now.Add(-time.Second).Format(time.RFC3339Nano),
		), wantError: "disabled before feast dispatch"},
		{name: "scheduler Bot Lock active", scheduler: `{"botLocked":true}`, wantError: "scheduler Bot Lock is active"},
		{name: "lane safety lock active", mutate: func(state *State.GameState) {
			state.Automations["autoBuyer"] = State.AutomationState{ID: "autoBuyer", Enabled: true, SafetyLock: State.AutomationSafetyLock{
				OperationID: "rejected-operation", Opcode: "bfs", Code: 99, ObservedAt: now,
			}}
		}, wantError: "lane safety lock is active"},
		{name: "session unavailable", mutate: func(state *State.GameState) {
			state.Session.SocketReady = false
		}, wantError: "game session is unavailable"},
		{name: "saved goal changed", settings: `{
			"version":1,"checkIntervalSec":1800,"historyRefreshSec":900,"minimumRubyReserve":0,
			"packages":[],"specialists":[],
			"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":13,"sourceCastleId":0,
				"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
		}`, wantError: "saved feast settings changed before dispatch"},
		{name: "automatic source changed", mutate: func(state *State.GameState) {
			castle := state.Castles[10]
			castle.ID = 20
			castle.KingdomID = 2
			castle.Name = "Fire Peaks"
			castle.Resources = map[State.ResourceID]State.ResourceBalance{5: autoBuyerIntentFoodBalance(200000)}
			state.Castles[20] = castle
		}, wantError: "automatic feast source changed to castle 20"},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := autoBuyerIntentTestState(now)
			castle := gameState.Castles[10]
			castle.Resources[5] = autoBuyerIntentFoodBalance(float64(expectedBalance))
			gameState.Castles[10] = castle
			gameState.Market.FeastCostReductionPercent = 25
			gameState.Automations["autoBuyer"] = State.AutomationState{ID: "autoBuyer", Enabled: true}
			if testCase.mutate != nil {
				testCase.mutate(&gameState)
			}
			settings := testCase.settings
			if settings == "" {
				settings = `{
					"version":1,"checkIntervalSec":1800,"historyRefreshSec":900,"minimumRubyReserve":0,
					"packages":[],"specialists":[],
					"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":0,
						"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
				}`
			}
			enabled := testCase.enabled
			if enabled == "" {
				enabled = `{"auto_buyer":true}`
			}
			scheduler := testCase.scheduler
			if scheduler == "" {
				scheduler = `{"botLocked":false}`
			}
			configuration, openErr := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
				"automation.autoBuyer": json.RawMessage(settings),
				"automation.enabled":   json.RawMessage(enabled),
				"scheduler":            json.RawMessage(scheduler),
			})
			if openErr != nil {
				t.Fatal(openErr)
			}
			application := &Application{
				State: State.NewStore(gameState), GameData: autoBuyerIntentTestManager(t), Configuration: configuration,
			}
			dispatchErr := application.validateAutoBuyerFeastDispatch(arguments, now.Add(time.Second))
			if testCase.wantError == "" {
				if dispatchErr != nil {
					t.Fatalf("current guarded dispatch rejected: %v", dispatchErr)
				}
				return
			}
			if dispatchErr == nil || !strings.Contains(dispatchErr.Error(), testCase.wantError) {
				t.Fatalf("dispatch error = %v, want %q", dispatchErr, testCase.wantError)
			}
		})
	}
}

func autoBuyerIntentTestState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Session.ChangedAt = now.Add(-time.Minute)
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	gameState.Session.Generation = 1
	gameState.Session.BaselineGeneration = 1
	gameState.Session.ConnectionGeneration = 1
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 1}
	gameState.Player.Level = 70
	gameState.Player.LegendLevel = 950
	production, consumption := 100.0, 10.0
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main",
		ContextSnapshotObservedAt: now, FoodBalanceObservedAt: now, FoodEconomyObservedAt: now,
		Resources: map[State.ResourceID]State.ResourceBalance{
			5: {ProductionPerHour: &production, ConsumptionPerHour: &consumption},
		},
	}
	gameState.Market.BoostersObservedAt = now
	gameState.Market.BoostersObservedGeneration = 1
	gameState.Market.Feast = State.MarketFeastState{ObservedAt: now}
	gameState.Market.FeastCostReductionObservedAt = now
	return gameState
}

func autoBuyerIntentFoodBalance(amount float64) State.ResourceBalance {
	production, consumption := 100.0, 10.0
	return State.ResourceBalance{Amount: amount, ProductionPerHour: &production, ConsumptionPerHour: &consumption}
}

const autoBuyerIntentTestCatalog = `{
	"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
	"resources":[
		{"resourceID":1,"JSONKey":"C1","name":"Coins"},{"resourceID":2,"JSONKey":"C2","name":"Rubies"},
		{"resourceID":5,"JSONKey":"F","name":"Food"},{"resourceID":11,"JSONKey":"HONEY","name":"Honey"},
		{"resourceID":12,"JSONKey":"MEAD","name":"Mead"},{"resourceID":13,"JSONKey":"BEEF","name":"Beef"}
	],
	"currencies":[{"currencyID":36,"JSONKey":"STO","Name":"SilverToken"},{"currencyID":70,"JSONKey":"RCO","Name":"RiftCoin"}],
	"packages":[
		{"packageID":100,"comment1":"Central Silver Shop","stock":5,"costSilverToken":10},
		{"packageID":101,"comment1":"Master Blacksmith Ruby","stock":2,"packagePriceC2":150},
		{"packageID":102,"comment1":"ARE Blacksmith - Rift Coin Package","stock":1,"costRiftCoin":25}
	],
	"feasts":[
		{"feastID":0,"comment":"Food feast","duration":21600,"productionBoost":80,"costFood":80000},
		{"feastID":1,"comment":"Ruby feast","duration":21600,"productionBoost":120,"costC2":250},
		{"feastID":8,"comment":"King's feast","duration":21600,"productionBoost":400,"costFood":150000}
	]
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

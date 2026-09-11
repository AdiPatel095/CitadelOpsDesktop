package Automation

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestAutoBuyerWakeDomainsHonorLongConfiguredCadence(t *testing.T) {
	got := NewAutoBuyerPolicy().WakeDomains()
	want := []string{"boosters", "market", "construction-offers", "events", "event-scores"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wake domains = %v, want %v", got, want)
	}
}

func TestAutoBuyerSpecialistRequiresFourteenDayFloorAndRenewsOneWeekAtATime(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Resources[2] = 5000
	gameState.Market.BoostersObservedAt = now
	gameState.Market.Feast = State.MarketFeastState{}
	gameState.Market.Boosters[0] = State.MarketBoosterState{
		ID: 0, RemainingSec: 13 * 24 * 60 * 60, ExpiresAt: now.Add(13 * 24 * time.Hour), ContinuousPurchaseCount: 1,
	}
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"minimumRubyReserve":1000,
		"packages":[],"specialists":[{"enabled":true,"id":0,"minimumDays":14,"maximumRubyCostPerPurchase":625}],
		"feast":{"enabled":false}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.specialist.purchase" {
		t.Fatalf("specialist decision = %#v err=%v", decision, err)
	}
	var request struct {
		SpecialistID          int   `json:"specialistId"`
		MinimumDays           int   `json:"minimumDays"`
		ExpectedRubyBalance   int64 `json:"expectedRubyBalance"`
		ExpectedPurchaseCount int   `json:"expectedPurchaseCount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || request.SpecialistID != 0 ||
		request.MinimumDays != 14 || request.ExpectedRubyBalance != 5000 || request.ExpectedPurchaseCount != 1 {
		t.Fatalf("specialist request = %#v err=%v", request, err)
	}

	gameState.Market.Boosters[0] = State.MarketBoosterState{ID: 0, ExpiresAt: now.Add(15 * 24 * time.Hour)}
	decision, err = NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("satisfied floor decision = %#v err=%v", decision, err)
	}

	settings = json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,
		"packages":[],"specialists":[{"enabled":true,"id":0,"minimumDays":7,"maximumRubyCostPerPurchase":625}],
		"feast":{"enabled":false}
	}`)
	decision, err = NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("unsafe floor decision = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerEventPackageWaitsForRouteAndUsesResetCounter(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Currencies[70] = 100
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[{"enabled":true,"shopId":"rift","packageId":102,"targetPurchasesPerReset":1,"minimumBalanceReserve":50}],
		"specialists":[],"feast":{"enabled":false}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || decision.Status != "idle" ||
		decision.Metrics["ignoredUnavailableEventShops"] != 1 {
		t.Fatalf("inactive event decision = %#v err=%v", decision, err)
	}
	gameState.EventScores.ShopByPackage[102] = State.EventShopRoute{EventID: 88, RemainingSec: 3600, ObservedAt: now}
	decision, err = NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.package.purchase" {
		t.Fatalf("active event decision = %#v err=%v", decision, err)
	}
	gameState.Inventory.ConstructionOffers[102] = 1
	decision, err = NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("completed reset goal = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerIgnoresUnavailableEventShopAndContinuesOtherShopGoals(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Currencies[70] = 100
	gameState.Player.Currencies[36] = 100
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":10,
		"packages":[
			{"enabled":true,"shopId":"rift","packageId":102,"targetPurchasesPerReset":1,"minimumBalanceReserve":50},
			{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":1,"minimumBalanceReserve":0}
		],
		"specialists":[],"feast":{"enabled":false}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.package.purchase" ||
		decision.Metrics["ignoredUnavailableEventShops"] != 1 {
		t.Fatalf("mixed-shop decision = %#v err=%v", decision, err)
	}
	var request struct {
		ShopID    string          `json:"shopId"`
		PackageID State.PackageID `json:"packageId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil ||
		request.ShopID != GameData.AutoBuyerShopMasterBlacksmith || request.PackageID != 100 {
		t.Fatalf("mixed-shop request = %#v err=%v", request, err)
	}
}

func TestAutoBuyerPackageHonorsPerResetLimitBelowStock(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Currencies[36] = 100
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":2,"minimumBalanceReserve":0}],
		"specialists":[],"feast":{"enabled":false}
	}`)
	evaluate := func() Decision {
		t.Helper()
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: now,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}

	decision := evaluate()
	if decision.Request == nil || decision.Request.Name != "autoBuyer.package.purchase" {
		t.Fatalf("limited package decision = %#v", decision)
	}
	var request struct {
		Amount                  int64 `json:"amount"`
		TargetPurchasesPerReset int64 `json:"targetPurchasesPerReset"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil ||
		request.Amount != 2 || request.TargetPurchasesPerReset != 2 {
		t.Fatalf("limited package request = %#v err=%v", request, err)
	}

	gameState.Inventory.ConstructionOffers[100] = 2
	decision = evaluate()
	if decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("completed lower purchase limit decision = %#v", decision)
	}
}

func TestAutoBuyerEnforcesHourlyHistoryAndThirtyMinuteChecks(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Currencies[36] = 100
	gameState.ReplaceInventoryConstructionOffers(
		map[State.PackageID]int64{100: 2}, now.Add(-30*time.Minute), 10, 0,
	)
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":2,"minimumBalanceReserve":0}],
		"specialists":[],"feast":{"enabled":false}
	}`)
	evaluate := func(at time.Time) Decision {
		t.Helper()
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: at,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	decision := evaluate(now)
	if decision.Request != nil || decision.Status != "idle" || !decision.NextCheckAt.Equal(now.Add(30*time.Minute)) {
		t.Fatalf("low-frequency idle decision = %#v", decision)
	}
	decision = evaluate(now.Add(31 * time.Minute))
	if decision.Request == nil || decision.Request.Name != "autoBuyer.package.history" {
		t.Fatalf("hourly history refresh decision = %#v", decision)
	}
}

func TestAutoBuyerFeastPreservesFoodReserve(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 100000}
	gameState.Castles[10] = castle
	gameState.Market.BoostersObservedAt = now
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[],"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("food reserve decision = %#v err=%v", decision, err)
	}
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	decision, err = NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.feast.purchase" {
		t.Fatalf("feast purchase decision = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerFoodFeastRequiresFreshReductionAndUsesDiscountedCost(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 90000}
	gameState.Castles[10] = castle
	gameState.Market.FeastCostReductionObservedAt = time.Time{}
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[],"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
	}`)
	evaluate := func(at time.Time) Decision {
		t.Helper()
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: at,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}

	decision := evaluate(now)
	if decision.Request == nil || decision.Request.Name != "autoBuyer.boosters.refresh" {
		t.Fatalf("stale FCE decision = %#v", decision)
	}
	if decision.ReevaluateOnStale || !decision.ReevaluateOnSuccess ||
		!decision.NextCheckAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("stale feast refresh retry policy = %#v", decision)
	}
	var refreshRequest struct {
		FeastContext bool `json:"feastContext"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &refreshRequest); err != nil || !refreshRequest.FeastContext {
		t.Fatalf("stale FCE refresh request = %#v err=%v", refreshRequest, err)
	}

	gameState.Market.FeastCostReductionPercent = 25
	gameState.Market.FeastCostReductionObservedAt = now
	decision = evaluate(now)
	if decision.Request == nil || decision.Request.Name != "autoBuyer.feast.purchase" ||
		decision.ReevaluateOnSuccess || !decision.NextCheckAt.Equal(now.Add(30*time.Second)) ||
		decision.Metrics["feastEffectiveCost"] != 60000 {
		t.Fatalf("discounted feast decision = %#v", decision)
	}
	var purchaseRequest struct {
		ExpectedEffectiveCost int64 `json:"expectedEffectiveCost"`
		ExpectedBalanceBefore int64 `json:"expectedBalanceBefore"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &purchaseRequest); err != nil ||
		purchaseRequest.ExpectedEffectiveCost != 60000 || purchaseRequest.ExpectedBalanceBefore != 90000 {
		t.Fatalf("discounted feast request = %#v err=%v", purchaseRequest, err)
	}
}

func TestAutoBuyerUsesFeastSpecificFreshness(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":10,
		"packages":[],"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000}
	}`)

	for _, testCase := range []struct {
		name       string
		mutate     func(*State.GameState)
		wantDetail string
	}{
		{
			name: "fresh booster clock cannot mask stale feast",
			mutate: func(state *State.GameState) {
				state.Market.BoostersObservedAt = now
				state.Market.Feast.ObservedAt = now.Add(-time.Hour)
			},
			wantDetail: "Refresh specialist and feast context",
		},
		{
			name: "pre-session feast is stale",
			mutate: func(state *State.GameState) {
				state.Session.ChangedAt = now
				state.Market.Feast.ObservedAt = now.Add(-time.Second)
			},
			wantDetail: "Refresh specialist and feast context",
		},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := autoBuyerPolicyTestState(now)
			castle := gameState.Castles[10]
			castle.Resources[5] = State.ResourceBalance{Amount: 120000}
			gameState.Castles[10] = castle
			testCase.mutate(&gameState)
			decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
				State: gameState, GameData: gameData, Now: now,
				Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.boosters.refresh" ||
				decision.Detail != testCase.wantDetail || decision.ReevaluateOnStale ||
				!decision.NextCheckAt.Equal(now.Add(30*time.Second)) {
				t.Fatalf("feast freshness decision = %#v err=%v", decision, err)
			}
		})
	}
}

func TestAutoBuyerUnreconciledFeastAcknowledgementCanOnlyRefresh(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 31, 0, time.UTC)
	acknowledgedAt := now.Add(-31 * time.Second)
	gameState := autoBuyerPolicyTestState(acknowledgedAt.Add(-time.Minute))
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 200000}
	gameState.Castles[10] = castle
	gameState.Market.FeastPurchasePending = true
	gameState.Market.FeastPurchaseExpectedID = 0
	gameState.Market.FeastPurchasePendingSince = acknowledgedAt.Add(-time.Second)
	gameState.Market.FeastPurchaseExpectedExpiresAt = acknowledgedAt.Add(6 * time.Hour)
	store := State.NewStore(gameState)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, nil, registry)
	code := 0
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: acknowledgedAt, Payload: json.RawMessage(`{"unexpected":true}`),
	}); err == nil {
		t.Fatal("unprojectable acknowledged BFS was accepted")
	}
	if _, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: acknowledgedAt.Add(time.Second), Payload: json.RawMessage(`{"BO":[]}`),
	}); err != nil {
		t.Fatalf("BOI without feast state = %v", err)
	}
	gameState = store.Snapshot()
	if !gameState.Market.FeastPurchasePending || !gameState.Market.FeastLastPurchaseAt.IsZero() {
		t.Fatalf("unreconciled feast state = %+v", gameState.Market)
	}
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":60,"historyRefreshSec":900,"sourceCastleId":10,
		"packages":[],"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request != nil || !strings.Contains(decision.Detail, "awaiting authoritative game reconciliation") {
		t.Fatalf("unreconciled feast acknowledgement decision = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerEligibleFeastPrecedesUnrelatedPackageHistoryRefresh(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Currencies[36] = 100
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":10,
		"packages":[{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":1,"minimumBalanceReserve":0}],
		"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.feast.purchase" {
		t.Fatalf("feast/package-history priority = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerEligibleSpecialistPrecedesUnrelatedPackageHistoryRefresh(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Resources[2] = 5000
	gameState.Player.Currencies[36] = 100
	gameState.Market.Boosters[0] = State.MarketBoosterState{
		ID: 0, ExpiresAt: now.Add(13 * 24 * time.Hour), ContinuousPurchaseCount: 1,
	}
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":10,"minimumRubyReserve":1000,
		"packages":[{"enabled":true,"shopId":"master-blacksmith","packageId":100,"targetPurchasesPerReset":1,"minimumBalanceReserve":0}],
		"specialists":[{"enabled":true,"id":0,"minimumDays":14,"maximumRubyCostPerPurchase":625}],
		"feast":{"enabled":false}
	}`)
	decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "autoBuyer.specialist.purchase" {
		t.Fatalf("specialist/package-history priority = %#v err=%v", decision, err)
	}
}

func TestAutoBuyerPacesRepeatedFeastPurchases(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 200000}
	gameState.Castles[10] = castle
	gameState.Market.Feast = State.MarketFeastState{
		ID: 0, RemainingSec: 6 * 60 * 60, ExpiresAt: now.Add(6 * time.Hour), ObservedAt: now,
	}
	gameState.Market.FeastLastPurchaseAt = now
	settings := json.RawMessage(`{
		"version":1,"checkIntervalSec":1800,"historyRefreshSec":3600,"sourceCastleId":10,
		"packages":[],"specialists":[],
		"feast":{"enabled":true,"feastId":0,"minimumRemainingHours":12,"sourceCastleId":10,"minimumFoodReserve":30000,"allowRubies":false,"maximumRubyCostPerPurchase":0}
	}`)
	evaluate := func(at time.Time) Decision {
		t.Helper()
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: at,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	decision := evaluate(now)
	if decision.Request != nil || decision.Status != "waiting" ||
		!decision.NextCheckAt.Equal(now.Add(30*time.Second)) {
		t.Fatalf("paced feast decision = %#v", decision)
	}
	gameState.Market.Feast.ObservedAt = now.Add(25 * time.Second)
	gameState.Market.Feast.ExpiresAt = gameState.Market.Feast.ObservedAt.Add(6 * time.Hour)
	decision = evaluate(now.Add(31 * time.Second))
	if decision.Request == nil || decision.Request.Name != "autoBuyer.feast.purchase" ||
		decision.ReevaluateOnSuccess || !decision.NextCheckAt.Equal(now.Add(61*time.Second)) {
		t.Fatalf("post-pacing feast decision = %#v", decision)
	}
}

func TestAutoBuyerRubyPackageRequiresOptInCeilingAndReserve(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Resources[2] = 500
	gameState.Inventory.ConstructionOffersCastleID = 10
	gameState.Inventory.ConstructionOffersKingdomID = 0
	gameState.Inventory.ConstructionOffersObservedAt = now
	evaluate := func(allow bool, ceiling, reserve int64) Decision {
		t.Helper()
		settings, _ := json.Marshal(autoBuyerSettings{
			Version: 1, CheckIntervalSec: 60, HistoryRefreshSec: 900, SourceCastleID: 10,
			AllowRubyPackages: allow, MinimumRubyReserve: reserve,
			Packages: []autoBuyerPackageRule{{
				Enabled: true, ShopID: GameData.AutoBuyerShopMasterBlacksmith, PackageID: 101,
				TargetPurchasesPerReset: 1, MaximumRubySpendPerReset: ceiling,
			}},
		})
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: now,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	if decision := evaluate(false, 150, 0); decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("ruby opt-in decision = %#v", decision)
	}
	if decision := evaluate(true, 149, 0); decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("ruby ceiling decision = %#v", decision)
	}
	if decision := evaluate(true, 150, 400); decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("ruby reserve decision = %#v", decision)
	}
	if decision := evaluate(true, 150, 300); decision.Request == nil || decision.Request.Name != "autoBuyer.package.purchase" {
		t.Fatalf("permitted ruby purchase decision = %#v", decision)
	}
}

func TestAutoBuyerRubyFeastRequiresExplicitPermissionAndCeiling(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Resources[2] = 1000
	gameState.Market.BoostersObservedAt = now
	evaluate := func(allow bool, ceiling int64) Decision {
		t.Helper()
		settings, _ := json.Marshal(autoBuyerSettings{
			Version: 1, CheckIntervalSec: 60, HistoryRefreshSec: 900, SourceCastleID: 10, MinimumRubyReserve: 500,
			Feast: autoBuyerFeastSettings{
				Enabled: true, FeastID: 1, MinimumRemainingHours: 12, SourceCastleID: 10,
				AllowRubies: allow, MaximumRubyCostPerPurchase: ceiling,
			},
		})
		decision, err := NewAutoBuyerPolicy().Evaluate(t.Context(), Snapshot{
			State: gameState, GameData: gameData, Now: now,
			Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoBuyerSection: settings}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return decision
	}
	if decision := evaluate(false, 250); decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("ruby feast opt-in decision = %#v", decision)
	}
	if decision := evaluate(true, 249); decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("ruby feast ceiling decision = %#v", decision)
	}
	if decision := evaluate(true, 250); decision.Request != nil || decision.Status != "waiting" ||
		decision.Detail != GameData.AutoBuyerRubyFeastUnsupportedReason {
		t.Fatalf("unverifiable ruby feast decision = %#v", decision)
	}
}

func autoBuyerPolicyTestState(now time.Time) State.GameState {
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

func autoBuyerPolicyTestStore(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
		"resources":[{"resourceID":1,"JSONKey":"C1","name":"Coins"},{"resourceID":2,"JSONKey":"C2","name":"Rubies"},{"resourceID":5,"JSONKey":"F","name":"Food"}],
		"currencies":[{"currencyID":36,"JSONKey":"STO","Name":"SilverToken"},{"currencyID":70,"JSONKey":"RCO","Name":"RiftCoin"}],
		"packages":[
			{"packageID":100,"comment1":"Central Silver Shop","stock":5,"costSilverToken":10},
			{"packageID":101,"comment1":"Master Blacksmith Ruby","stock":2,"packagePriceC2":150},
			{"packageID":102,"comment1":"ARE Blacksmith - Rift Coin Package","stock":1,"costRiftCoin":25}
		],
		"feasts":[{"feastID":0,"comment":"Food feast","duration":21600,"productionBoost":80,"costFood":80000},{"feastID":1,"comment":"Ruby feast","duration":21600,"productionBoost":120,"costC2":250}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

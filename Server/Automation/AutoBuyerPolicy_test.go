package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoBuyerSpecialistRequiresFourteenDayFloorAndRenewsOneWeekAtATime(t *testing.T) {
	gameData := autoBuyerPolicyTestStore(t)
	now := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	gameState := autoBuyerPolicyTestState(now)
	gameState.Player.Resources[2] = 5000
	gameState.Market.BoostersObservedAt = now
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
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
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
	if decision := evaluate(true, 250); decision.Request == nil || decision.Request.Name != "autoBuyer.feast.purchase" {
		t.Fatalf("permitted ruby feast decision = %#v", decision)
	}
}

func autoBuyerPolicyTestState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.LegendLevel = 950
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main", Resources: map[State.ResourceID]State.ResourceBalance{},
	}
	gameState.Market.BoostersObservedAt = now
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

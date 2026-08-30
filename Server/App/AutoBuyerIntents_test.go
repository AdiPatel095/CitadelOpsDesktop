package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
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

func TestPlanAutoBuyerFeastRefreshesResourcesAndTimer(t *testing.T) {
	gameData := autoBuyerIntentTestStore(t)
	now := time.Now().UTC().Truncate(time.Second)
	gameState := autoBuyerIntentTestState(now)
	castle := gameState.Castles[10]
	castle.Resources[5] = State.ResourceBalance{Amount: 120000}
	gameState.Castles[10] = castle
	gameState.Market.BoostersObservedAt = now
	arguments, _ := json.Marshal(autoBuyerFeastPurchaseRequest{
		FeastID: 0, MinimumRemainingHours: 12, SourceCastleID: 10, MinimumFoodReserve: 30000,
		ExpectedActiveFeastID: 0, ExpectedExpiresAtUnix: 0, ExpectedBalanceBefore: 120000, HistoryRefreshSec: 900,
	})
	plan, err := planAutoBuyerFeastPurchase(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 7 || plan.Steps[0].Opcode != "grc" || plan.Steps[1].Opcode != "boi" ||
		plan.Steps[2].Action != "auto_buyer.feast.guard" || plan.Steps[3].Opcode != "bfs" ||
		plan.Steps[4].Opcode != "grc" || plan.Steps[5].Opcode != "boi" || plan.Steps[6].Action != "auto_buyer.feast.verify" {
		t.Fatalf("feast steps = %#v", plan.Steps)
	}
	var payload struct {
		FeastID  int64           `json:"T"`
		CastleID State.CastleID  `json:"CID"`
		Kingdom  State.KingdomID `json:"KID"`
		Position int             `json:"PO"`
	}
	if err := json.Unmarshal(plan.Steps[3].Command.Payload, &payload); err != nil || payload.FeastID != 0 ||
		payload.CastleID != 10 || payload.Kingdom != 0 || payload.Position != -1 {
		t.Fatalf("BFS payload = %#v err=%v", payload, err)
	}
}

func autoBuyerIntentTestState(now time.Time) State.GameState {
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.Player.LegendLevel = 950
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, SlotType: 1, Name: "Main", Resources: map[State.ResourceID]State.ResourceBalance{},
	}
	gameState.Market.BoostersObservedAt = now
	return gameState
}

func autoBuyerIntentTestStore(t *testing.T) *GameData.Store {
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

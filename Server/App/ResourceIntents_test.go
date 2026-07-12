package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestResourceShipmentPlannersUseOfficialWireKeys(t *testing.T) {
	gameData := resourceIntentGameData(t)
	gameState := State.NewGameState()
	source := resourceIntentCastle(10, 0, 100, 200)
	target := resourceIntentCastle(20, 0, 110, 215)
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
	if err := json.Unmarshal(kingdomPlan.Steps[0].Command.Payload, &kingdomPayload); err != nil {
		t.Fatal(err)
	}
	if string(kingdomPayload.Goods[0][0]) != `"W"` || kingdomPlan.Steps[0].Opcode != "kgt" {
		t.Fatalf("unexpected kingdom payload: %s", kingdomPlan.Steps[0].Command.Payload)
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

func resourceIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":3,"JSONKey":"W"}],
		"currencies":[{"currencyID":50,"JSONKey":"MS5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func resourceIntentCastle(id State.CastleID, kingdom State.KingdomID, x int, y int) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, X: x, Y: y,
		Resources: map[State.ResourceID]State.ResourceBalance{},
		Buildings: map[State.BuildingInstanceID]State.Building{},
	}
}

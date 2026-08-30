package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestRuntimeInventoryAndQueueableReducers(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	stormCastle := newCastleState(200)
	stormCastle.KingdomID = 4
	gameState.Castles[200] = stormCastle
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	code := 0

	_, changed, err := reduceQueueableProduction(t.Context(), Protocol.Frame{
		Opcode: "gpc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"A":[{"AID":100,"U":{"U":[10,20,20,999]}}]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("queueable production: changed=%t err=%v", changed, err)
	}
	queueable := gameState.Castles[100].QueueableProduction
	if len(queueable[0]) != 1 || queueable[0][0].Collection != "units" || queueable[0][0].ID != 10 {
		t.Fatalf("unexpected queueable units: %#v", queueable[0])
	}
	if len(queueable[1]) != 1 || queueable[1][0].Collection != "tools" || queueable[1][0].ID != 20 {
		t.Fatalf("unexpected queueable tools: %#v", queueable[1])
	}

	_, changed, err = reduceStorageInventory(t.Context(), Protocol.Frame{
		Opcode: "sin", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`[{"SID":1,"RD":[[2944,3],[2944,2]]},{"SID":2,"RD":[[2944,7]]}]`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("storage inventory: changed=%t err=%v", changed, err)
	}
	if gameState.Inventory.Items["storage:1"][2944] != 5 || gameState.Inventory.Items["storage:2"][2944] != 7 {
		t.Fatalf("storage namespaces collided: %#v", gameState.Inventory.Items)
	}

	_, changed, err = reduceConstructionOffersCommand(t.Context(), Protocol.Frame{
		Opcode: "gbc", Direction: Protocol.DirectionOutbound, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"CID":100,"KID":0}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("construction offer context: changed=%t err=%v", changed, err)
	}
	_, changed, err = reduceConstructionOffers(t.Context(), Protocol.Frame{
		Opcode: "gbc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"PL":[{"PID":4743,"AMT":5},{"PID":4741,"AMT":1}]}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Inventory.ConstructionOffers[4743] != 5 {
		t.Fatalf("construction offers: changed=%t offers=%#v err=%v", changed, gameState.Inventory.ConstructionOffers, err)
	}
	_, changed, err = reduceConstructionOffersCommand(t.Context(), Protocol.Frame{
		Opcode: "gbc", Direction: Protocol.DirectionOutbound, ReceivedAt: observedAt.Add(time.Minute),
		Payload: json.RawMessage(`{"CID":200,"KID":4}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("Storm construction offer context: changed=%t err=%v", changed, err)
	}
	_, changed, err = reduceConstructionOffers(t.Context(), Protocol.Frame{
		Opcode: "gbc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Minute),
		Payload: json.RawMessage(`{"PL":[{"PID":245,"AMT":12}]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("Storm construction offers: changed=%t err=%v", changed, err)
	}
	greatEmpireOffers, _, found := gameState.ConstructionOffersFor(100, 0)
	if !found || greatEmpireOffers[4743] != 5 {
		t.Fatalf("Great Empire construction offers were overwritten: %#v found=%t", greatEmpireOffers, found)
	}
	stormOffers, _, found := gameState.ConstructionOffersFor(200, 4)
	if !found || stormOffers[245] != 12 {
		t.Fatalf("Storm construction offers = %#v found=%t", stormOffers, found)
	}
}

func TestKingdomTransportReducerPreservesAutomationWorkflowThroughSettlement(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	gameState.KingdomTransport.ResourceWorkflows[4] = State.KingdomResourceTransportWorkflow{
		Owner: "autoFoodBalance", KingdomID: 4, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now,
	}
	code := 0
	_, _, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"UL":[{"KID":4,"U":1}],"RT":[{"KID":4,"G":[["F",900]],"RS":60}]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if workflow, exists := gameState.KingdomTransport.ResourceWorkflows[4]; !exists || workflow.Owner != "autoFoodBalance" {
		t.Fatalf("pending transport lost workflow: %#v exists=%t", workflow, exists)
	}
	_, _, err = reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Minute),
		Payload: json.RawMessage(`{"UL":[{"KID":4,"U":1}]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if workflow, exists := gameState.KingdomTransport.ResourceWorkflows[4]; !exists || workflow.Owner != "autoFoodBalance" {
		t.Fatalf("settled transport lost workflow before destination refresh: %#v exists=%t", workflow, exists)
	}
}

func TestKingdomTransportReducerClearsPendingFromSuccessfulMSKSnapshot(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	gameState.KingdomTransport.Pending = []State.KingdomResourceTransport{{
		KingdomID: 2, RemainingSec: 1_440,
	}}
	gameState.KingdomTransport.ResourceWorkflows[2] = State.KingdomResourceTransportWorkflow{
		Owner: "autoSceatRes", KingdomID: 2, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now,
	}
	code := 0
	_, changed, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "msk", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Second),
		Payload: json.RawMessage(`{"kpi":{"UL":[{"KID":2,"U":1}]}}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("successful MSK reduction: changed=%t err=%v", changed, err)
	}
	if len(gameState.KingdomTransport.Pending) != 0 {
		t.Fatalf("completed MSK left pending transports: %#v", gameState.KingdomTransport.Pending)
	}
	if workflow, exists := gameState.KingdomTransport.ResourceWorkflows[2]; !exists || workflow.Owner != "autoSceatRes" {
		t.Fatalf("successful MSK lost settlement workflow: %#v exists=%t", workflow, exists)
	}
}

func TestRuntimeNestedResponseReducers(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	gameState.Castles[100] = castle
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	code := 0

	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	provider := staticGameDataProvider{store: gameData}
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, provider, registry)
	_, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "bup", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"gcu":{"C1":900},"sce":[["STP",12]],"grc":{"AID":100,"W":777},
			"spl":{"LID":1,"PS":{"WID":20,"TUA":5,"RCT":60},"QS":[{"P":{"WID":20,"TUA":10}},{"SI":{"RUT":50,"VIP":1}}]}
		}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := store.Snapshot()
	if snapshot.Player.Resources[1] != 900 || snapshot.Player.Currencies[2] != 12 {
		t.Fatalf("nested account resources not applied: %#v %#v", snapshot.Player.Resources, snapshot.Player.Currencies)
	}
	if snapshot.Castles[100].Resources[3].Amount != 777 {
		t.Fatalf("nested castle resource not applied: %#v", snapshot.Castles[100].Resources)
	}
	queue := snapshot.Castles[100].Production[1]
	if queue.Active == nil || queue.Active.Definition.ID != 20 || queue.Capacity != 2 || len(queue.Queued) != 1 {
		t.Fatalf("nested production queue not applied: %#v", queue)
	}

	_, err = pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Opcode: "ssi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Second),
		Payload: json.RawMessage(`{"gaa":{"KID":0,"AI":[[1,12,34,100,42,70,"Castle"]]}}`),
	})
	if err != nil || snapshotMapName(store.Snapshot(), 0, "12:34") != "Castle" {
		t.Fatalf("nested map snapshot not applied: err=%v map=%#v", err, store.Snapshot().Map)
	}
}

func TestRuntimeTransportAndSubscriptionReducers(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	code := 0

	_, changed, err := reduceMarketInfo(t.Context(), Protocol.Frame{
		Opcode: "cmi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"C":[{"CID":100,"KID":0,"TC":12,"AC":9,"W":456,"AE":[[7,[2.5],"event"]]}]}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Market.Castles[100].AvailableBarrows != 9 || gameState.Castles[100].Resources[3].Amount != 456 {
		t.Fatalf("market info: changed=%t market=%#v castle=%#v err=%v", changed, gameState.Market, gameState.Castles[100], err)
	}

	_, changed, err = reduceMarketBooster(t.Context(), Protocol.Frame{
		Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"BO":[{"ID":11,"L":21,"RT":2147483647},{"ID":24,"B":400,"RT":10702,"PC":2}],"bfs":{"T":3,"RT":7200}}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Market.CaravanLevel != 21 {
		t.Fatalf("market booster: changed=%t market=%#v err=%v", changed, gameState.Market, err)
	}
	gallantry := gameState.Market.Boosters[24]
	if gallantry.BonusPercent != 400 || gallantry.RemainingSec != 10702 || gallantry.ContinuousPurchaseCount != 2 ||
		!gallantry.ExpiresAt.Equal(observedAt.Add(10702*time.Second)) || !gallantry.ActiveAt(observedAt) {
		t.Fatalf("gallantry booster = %#v", gallantry)
	}
	if caravan := gameState.Market.Boosters[11]; !caravan.Permanent || !caravan.ActiveAt(observedAt) {
		t.Fatalf("permanent caravan booster = %#v", caravan)
	}
	if !gameState.Market.BoostersObservedAt.Equal(observedAt) {
		t.Fatalf("booster observation time = %s", gameState.Market.BoostersObservedAt)
	}
	if feast := gameState.Market.Feast; feast.ID != 3 || feast.RemainingSec != 7200 ||
		!feast.ExpiresAt.Equal(observedAt.Add(7200*time.Second)) || !feast.ActiveAt(observedAt) {
		t.Fatalf("market feast = %#v", feast)
	}

	_, changed, err = reduceMarketFeast(t.Context(), Protocol.Frame{
		Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt.Add(time.Minute),
		Payload: json.RawMessage(`{"bfs":{"T":3,"RT":14400}}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Market.Feast.RemainingSec != 14400 ||
		!gameState.Market.Feast.ExpiresAt.Equal(observedAt.Add(time.Minute+14400*time.Second)) {
		t.Fatalf("feast response: changed=%t feast=%#v err=%v", changed, gameState.Market.Feast, err)
	}

	_, changed, err = reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kgt", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"kpi":{"UL":[{"KID":1,"U":1,"C":1,"SL":4}],"RT":[{"KID":1,"RS":3600,"G":[["W",35249]]}],"UT":[{"KID":4,"RS":1800,"I":[[10,25]]}]}}`),
	}, &gameState, gameData)
	if err != nil || !changed || !gameState.KingdomTransport.Unlocks[1].Unlocked || gameState.KingdomTransport.Pending[0].Goods[0].ResourceID != 3 ||
		gameState.KingdomTransport.PendingUnits[0].KingdomID != 4 || gameState.KingdomTransport.PendingUnits[0].Units[0].UnitID != 10 {
		t.Fatalf("kingdom transport: changed=%t state=%#v err=%v", changed, gameState.KingdomTransport, err)
	}

	_, changed, err = reduceSubscriptions(t.Context(), Protocol.Frame{
		Opcode: "sie", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"SP":[{"STID":1,"RS":50,"RSGP":100}]}`),
	}, &gameState, gameData)
	if err != nil || !changed || gameState.Subscriptions[1].GracePeriodSec != 100 {
		t.Fatalf("subscriptions: changed=%t state=%#v err=%v", changed, gameState.Subscriptions, err)
	}

	_, changed, err = reduceSubscriptions(t.Context(), Protocol.Frame{
		Opcode: "upc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"R":[["C2",1520000]]}`),
	}, &gameState, gameData)
	if err != nil || changed || gameState.Subscriptions[1].GracePeriodSec != 100 {
		t.Fatalf("unrelated upc changed subscriptions: changed=%t state=%#v err=%v", changed, gameState.Subscriptions, err)
	}
}

func TestBeriCapacityReducerKeepsUnitIdentity(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	_, changed, err := reduceBeriCapacity(t.Context(), Protocol.Frame{
		Opcode: "fuc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"SCID":100,"A":[[10,25],[20,7]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Beri capacity: changed=%t err=%v", changed, err)
	}
	if gameState.Beri.AvailableTroops != 25 || gameState.Beri.TroopsByUnit[10] != 25 || gameState.Beri.ParsedSourceID != 100 {
		t.Fatalf("unexpected Beri state: %#v", gameState.Beri)
	}
}

type staticGameDataProvider struct{ store *GameData.Store }

func (provider staticGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

func runtimeTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":1}],
		"units":[{"wodID":10},{"wodID":20,"slotTypes":"1,2"}],
		"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":3,"JSONKey":"W"}],
		"currencies":[{"currencyID":2,"JSONKey":"STP"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func snapshotMapName(state State.GameState, kingdom State.KingdomID, key string) string {
	return state.Map[kingdom][key].Name
}

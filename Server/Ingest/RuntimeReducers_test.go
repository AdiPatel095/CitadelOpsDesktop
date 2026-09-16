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

func TestKingdomTroopWorkflowRequiresCurrentSessionContinuity(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Now().UTC().Add(-time.Minute)
	gameState.Session.ConnectionGeneration = 8
	gameState.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: "autoFortress", Status: "armed", KingdomID: 2,
		Units:   []State.KingdomTransportUnit{{UnitID: 277, Amount: 100}},
		ArmedAt: now, SessionGeneration: 7,
	}
	code := 0
	_, changed, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kut", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Second),
		Payload: json.RawMessage(`{"kpi":{"UT":[{"KID":2,"RS":3600,"I":[[277,100]]}]}}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("transport reduction: changed=%t err=%v", changed, err)
	}
	if got := gameState.KingdomTransport.TroopWorkflows[2].Status; got != "ownership_uncertain" {
		t.Fatalf("lookalike transport was adopted across sessions: status=%q", got)
	}
	_, _, err = reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(2 * time.Second),
		Payload: json.RawMessage(`{"UL":[{"KID":2,"U":1}]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if got := gameState.KingdomTransport.TroopWorkflows[2].Status; got != "ownership_uncertain" {
		t.Fatalf("completed ambiguous transport lost durable ambiguity: status=%q", got)
	}
}

func TestKingdomTroopWorkflowRejectsExactManualReplacementWithResetTimer(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Now().UTC().Add(-time.Minute)
	gameState.Session.ConnectionGeneration = 8
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: "autoFortress", Status: "pending", KingdomID: 2,
		Units:   []State.KingdomTransportUnit{{UnitID: 277, Amount: 100}},
		ArmedAt: now.Add(-time.Minute), LaunchedAt: now.Add(-time.Minute), TransportObservedAt: now,
		RemainingSec: 100, SessionGeneration: 8,
	}
	code := 0
	_, _, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(20 * time.Second),
		Payload: json.RawMessage(`{"UL":[{"KID":2,"U":1}],"UT":[{"KID":2,"RS":100,"I":[[277,100]]}]}`),
	}, &gameState, gameData)
	if err != nil {
		t.Fatal(err)
	}
	if got := gameState.KingdomTransport.TroopWorkflows[2].Status; got != "awaiting_destination_refresh" {
		t.Fatalf("exact lookalike with reset timer was adopted: status=%q", got)
	}
}

func TestKingdomTroopWorkflowPreservesSkipMarkerWhenTransportCompletes(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Now().UTC().Add(-time.Minute)
	gameState.Session.ConnectionGeneration = 8
	gameState.KingdomTransport.TroopWorkflows[2] = State.KingdomTroopTransportWorkflow{
		ID: "owned", Owner: "autoFortress", Status: "pending", KingdomID: 2,
		Units: []State.KingdomTransportUnit{{UnitID: 277, Amount: 100}}, ArmedAt: now.Add(-time.Minute),
		TransportObservedAt: now, RemainingSec: 3600, SessionGeneration: 8,
		SkipCurrencyID: 1005, SkipWireKey: "MS5", SkipBalanceBefore: 2, SkipRemainingBefore: 3600,
		SkipDurationSec: 3600, SkipRequestedAt: now.Add(time.Second),
	}
	code := 0
	_, changed, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "msk", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(2 * time.Second),
		Payload: json.RawMessage(`{"kpi":{"UL":[{"KID":2,"U":1}]}}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("completed skip reduction: changed=%t err=%v", changed, err)
	}
	workflow := gameState.KingdomTransport.TroopWorkflows[2]
	if workflow.SkipRequestedAt.IsZero() || workflow.Status != "pending" || workflow.RemainingSec != 0 {
		t.Fatalf("completed transport discarded unresolved skip marker: %#v", workflow)
	}
}

func TestKingdomTransportRejectsMalformedAndOlderSnapshots(t *testing.T) {
	gameData := runtimeTestGameData(t)
	gameState := State.NewGameState()
	now := time.Now().UTC().Add(-time.Minute)
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 2, RemainingSec: 60, Units: []State.KingdomTransportUnit{{UnitID: 277, Amount: 100}},
	}}
	code := 0
	_, changed, err := reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(time.Second),
		Payload: json.RawMessage(`{"UL":[{"KID":2,"U":1}],"UT":null}`),
	}, &gameState, gameData)
	if err == nil || changed || len(gameState.KingdomTransport.PendingUnits) != 1 {
		t.Fatalf("malformed snapshot replaced state: changed=%t pending=%#v err=%v", changed, gameState.KingdomTransport.PendingUnits, err)
	}
	_, changed, err = reduceKingdomTransport(t.Context(), Protocol.Frame{
		Opcode: "kpi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: now.Add(-time.Second),
		Payload: json.RawMessage(`{"UL":[{"KID":2,"U":1}]}`),
	}, &gameState, gameData)
	if err != nil || changed || len(gameState.KingdomTransport.PendingUnits) != 1 {
		t.Fatalf("older snapshot replaced state: changed=%t pending=%#v err=%v", changed, gameState.KingdomTransport.PendingUnits, err)
	}
}

func TestCurrencyAuthorityCannotFollowInvalidReplacement(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ConnectionGeneration = 4
	gameData := runtimeTestGameData(t)
	now := time.Now().UTC()
	changed, err := applyPlayerCurrencies(json.RawMessage(`[["STP",5]]`), &gameState, gameData, now, true)
	if err != nil || !changed || gameState.Player.CurrencyObservations[2].ConnectionGeneration != 4 {
		t.Fatalf("authoritative currency observation: changed=%t observation=%#v err=%v", changed, gameState.Player.CurrencyObservations[2], err)
	}
	changed, err = applyPlayerCurrencies(json.RawMessage(`[["STP",4]]`), &gameState, gameData, now.Add(2*time.Minute), true)
	if err != nil || !changed || gameState.Player.Currencies[2] != 4 {
		t.Fatalf("future replacement: changed=%t balance=%v err=%v", changed, gameState.Player.Currencies[2], err)
	}
	if observation := gameState.Player.CurrencyObservations[2]; !observation.ObservedAt.IsZero() {
		t.Fatalf("future replacement retained stale authority: %#v", observation)
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

func TestMarketBoosterPreservesFeastWhenBFSOmitted(t *testing.T) {
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	existingFeast := State.MarketFeastState{
		ID:           3,
		RemainingSec: 14400,
		ExpiresAt:    observedAt.Add(4 * time.Hour),
		ObservedAt:   observedAt,
	}
	code := 0

	for _, testCase := range []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "top-level", payload: json.RawMessage(`{"BO":[]}`)},
		{name: "nested", payload: json.RawMessage(`{"boi":{"BO":[]}}`)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.Feast = existingFeast
			receivedAt := observedAt.Add(time.Minute)

			_, changed, err := reduceMarketBooster(t.Context(), Protocol.Frame{
				Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: receivedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("market booster without bfs: changed=%t err=%v", changed, err)
			}
			if gameState.Market.Feast != existingFeast {
				t.Fatalf("market feast was cleared: got=%#v want=%#v", gameState.Market.Feast, existingFeast)
			}
			if !gameState.Market.BoostersObservedAt.Equal(receivedAt) {
				t.Fatalf("booster observation time = %s, want %s", gameState.Market.BoostersObservedAt, receivedAt)
			}
		})
	}
}

func TestMarketBoosterRequiresCompleteCoherentAuthoritativeArray(t *testing.T) {
	base := time.Date(2026, 9, 15, 12, 0, 0, 250_000_000, time.UTC)
	code := 0
	for _, testCase := range []struct {
		name    string
		payload json.RawMessage
	}{
		{"missing", json.RawMessage(`{}`)}, {"null", json.RawMessage(`{"BO":null}`)},
		{"malformed-id", json.RawMessage(`{"BO":[{"ID":null,"RT":0}]}`)},
		{"fractional-duration", json.RawMessage(`{"BO":[{"ID":0,"RT":1.5}]}`)},
		{"duplicate", json.RawMessage(`{"BO":[{"ID":0,"RT":1},{"ID":0,"RT":2}]}`)},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state := State.NewGameState()
			state.Session.ConnectionGeneration = 7
			state.Market.BoostersObservedAt = base
			state.Market.BoostersObservedGeneration = 7
			state.Market.Boosters[0] = State.MarketBoosterState{ID: 0, RemainingSec: 3600, ExpiresAt: base.Add(time.Hour)}
			_, _, err := reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(time.Second), Payload: testCase.payload}, &state, nil)
			if (testCase.name == "missing" || testCase.name == "null") && err != nil {
				t.Fatalf("optional BO error = %v", err)
			}
			if testCase.name != "missing" && testCase.name != "null" && err == nil {
				t.Fatal("malformed authoritative BO accepted")
			}
			if state.Market.Boosters[0].RemainingSec != 3600 || !state.Market.BoostersObservedAt.Equal(base) {
				t.Fatalf("invalid BO changed authority: %#v", state.Market)
			}
		})
	}
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 7
	state.Market.Boosters[0] = State.MarketBoosterState{ID: 0, RemainingSec: 3600, ExpiresAt: base.Add(time.Hour)}
	_, changed, err := reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base, Payload: json.RawMessage(`{"BO":[]}`)}, &state, nil)
	if err != nil || !changed || len(state.Market.Boosters) != 0 || state.Market.BoostersObservedGeneration != 7 {
		t.Fatalf("explicit empty BO = %#v err=%v", state.Market, err)
	}
	state.Market.Boosters[0] = State.MarketBoosterState{ID: 0, RemainingSec: 3600, ExpiresAt: base.Add(time.Hour)}
	state.Market.BoostersObservedAt = base
	_, _, _ = reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", Direction: Protocol.DirectionInbound, ReceivedAt: base.Add(time.Second), Payload: json.RawMessage(`{"BO":[]}`)}, &state, nil)
	if len(state.Market.Boosters) != 1 || !state.Market.BoostersObservedAt.Equal(base) {
		t.Fatalf("missing result code changed BO authority: %#v", state.Market)
	}
	_, changed, err = reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(2 * time.Second), Payload: json.RawMessage(`{"BO":[{"ID":99,"RT":2147483647,"L":1},{"ID":0,"RT":4000000000,"PC":0}]}`)}, &state, nil)
	if err != nil || !changed || !state.Market.Boosters[99].Permanent || state.Market.Boosters[0].RemainingSec != 4_000_000_000 || state.Market.Boosters[0].ContinuousPurchaseCount != 0 {
		t.Fatalf("ordered unrelated/permanent/large BO rows = %#v err=%v", state.Market.Boosters, err)
	}
}

func TestGlobalRubyObservationRequiresExplicitCurrentC2(t *testing.T) {
	gameData, decodeErr := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"resources":[{"resourceID":1,"JSONKey":"C1"},{"resourceID":2,"JSONKey":"C2"}],"currencies":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 9
	base := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	code := 0
	_, _, err := reduceGlobalResources(t.Context(), Protocol.Frame{Opcode: "gcu", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base, Payload: json.RawMessage(`{"C2":0}`)}, &state, gameData)
	if err != nil || state.Player.Resources[2] != 0 || !state.Player.ResourceObservations[2].ObservedAt.Equal(base) {
		t.Fatalf("zero ruby authority = %#v err=%v", state.Player, err)
	}
	_, _, _ = reduceGlobalResources(t.Context(), Protocol.Frame{Opcode: "gcu", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(time.Second), Payload: json.RawMessage(`{"C1":50}`)}, &state, gameData)
	if !state.Player.ResourceObservations[2].ObservedAt.Equal(base) {
		t.Fatal("missing C2 refreshed ruby authority")
	}
	_, _, _ = reduceGlobalResources(t.Context(), Protocol.Frame{Opcode: "gcu", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(2 * time.Second), Payload: json.RawMessage(`{"C2":null}`)}, &state, gameData)
	if !state.Player.ResourceObservations[2].ObservedAt.Equal(base) {
		t.Fatal("malformed C2 refreshed ruby authority")
	}
	_, _, _ = reduceGlobalResources(t.Context(), Protocol.Frame{Opcode: "gcu", Direction: Protocol.DirectionInbound, ReceivedAt: base.Add(3 * time.Second), Payload: json.RawMessage(`{"C2":500}`)}, &state, gameData)
	if state.Player.Resources[2] != 0 || !state.Player.ResourceObservations[2].ObservedAt.Equal(base) {
		t.Fatalf("missing result code changed ruby amount under old authority: %#v", state.Player)
	}
	for _, invalid := range []json.RawMessage{json.RawMessage(`{"C2":-1}`), json.RawMessage(`{"C2":1.5}`), json.RawMessage(`{"C2":"NaN"}`), json.RawMessage(`{"C2":9223372036854775808}`)} {
		_, _, _ = reduceGlobalResources(t.Context(), Protocol.Frame{Opcode: "gcu", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(4 * time.Second), Payload: invalid}, &state, gameData)
		if state.Player.Resources[2] != 0 || !state.Player.ResourceObservations[2].ObservedAt.Equal(base) {
			t.Fatalf("invalid C2 %s changed ruby authority: %#v", invalid, state.Player)
		}
	}
}

func TestSpecialistResponseKeepsRubyAndBoosterAuthorityAtomic(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"resources":[{"resourceID":2,"JSONKey":"C2"},{"resourceID":5,"JSONKey":"F"}],"currencies":[]}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 3
	base := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	state.Player.Resources[2] = 1000
	state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: base, ConnectionGeneration: 3}
	state.Market.Boosters = map[int]State.MarketBoosterState{0: {ID: 0, RemainingSec: 3600, ExpiresAt: base.Add(time.Hour)}}
	state.Market.BoostersObservedAt = base
	state.Market.BoostersObservedGeneration = 3
	state.Castles[100] = newCastleState(100)
	castle := state.Castles[100]
	castle.Focused = true
	state.Castles[100] = castle
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	store := State.NewStore(state)
	_, err = store.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
		current.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: base, ConnectionGeneration: 3}
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, staticGameDataProvider{store: gameData}, registry)
	code := 0
	_, err = pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "ovs", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(time.Second), Payload: json.RawMessage(`{"gcu":{"C2":900},"boi":{"BO":[{"ID":null,"RT":604800}]}}`)})
	if err == nil {
		t.Fatal("malformed specialist BO response was accepted")
	}
	afterError := store.ReadOnlyView()
	if afterError.Player.Resources[2] != 1000 || !afterError.Player.ResourceObservations[2].ObservedAt.Equal(base) || afterError.Market.Boosters[0].RemainingSec != 3600 || !afterError.Market.BoostersObservedAt.Equal(base) {
		t.Fatalf("ingest error partially refreshed authority: player=%+v market=%+v", afterError.Player, afterError.Market)
	}
	_, err = pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "ovs", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(2 * time.Second), Payload: json.RawMessage(`{"gcu":{"C2":900}}`)})
	if err != nil {
		t.Fatal(err)
	}
	afterOptional := store.ReadOnlyView()
	if afterOptional.Player.Resources[2] != 900 || !afterOptional.Player.ResourceObservations[2].ObservedAt.Equal(base.Add(2*time.Second)) || afterOptional.Market.Boosters[0].RemainingSec != 3600 || !afterOptional.Market.BoostersObservedAt.Equal(base) {
		t.Fatalf("optional missing BO did not preserve independent authority: player=%+v market=%+v", afterOptional.Player, afterOptional.Market)
	}
	_, err = pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "btx", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(3 * time.Second), Payload: json.RawMessage(`{"gcu":{"C2":800},"boi":{"BO":[{"ID":8,"RT":604800,"PC":0}]},"txi":{"TX":{"RT":1}}}`)})
	if err != nil {
		t.Fatal(err)
	}
	afterTax := store.ReadOnlyView()
	if afterTax.Market.Boosters[8].RemainingSec != 604800 || afterTax.Market.Boosters[8].ExpiresAt.Sub(base.Add(3*time.Second)) != 7*24*time.Hour {
		t.Fatalf("tax-cycle RT replaced tax specialist timer: %+v", afterTax.Market.Boosters[8])
	}
	_, err = pipeline.HandleFrame(t.Context(), Protocol.Frame{Opcode: "bis", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: base.Add(4 * time.Second), Payload: json.RawMessage(`{"gcu":{"C2":700},"gpa":{"DF":500,"DFC":120},"boi":{"BO":[{"ID":10,"RT":604800,"PC":0},{"ID":8,"RT":604799,"PC":0}]}}`)})
	if err != nil {
		t.Fatal(err)
	}
	afterDrill := store.ReadOnlyView()
	food := afterDrill.Castles[100].Resources[5]
	if afterDrill.Market.Boosters[10].RemainingSec != 604800 || food.ProductionPerHour == nil || *food.ProductionPerHour != 50 || food.ConsumptionPerHour == nil || *food.ConsumptionPerHour != 12 {
		t.Fatalf("drill response lost nested BO/GPA: booster=%+v food=%+v", afterDrill.Market.Boosters[10], food)
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

package App

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCoinDispatchGateBlocksAuditedOpcodesAndAllowsAffordableCommands(t *testing.T) {
	store := coinGateGameData(t)
	tests := []struct {
		opcode  string
		payload string
	}{
		{"bup", `{"LID":0,"WID":1,"AMT":10}`},
		{"hru", `{"U":1,"A":10}`},
		{"cds", `{"SID":10,"TX":50,"TY":0,"HBW":-1,"A":[[1,10]]}`},
		{"cra", `{"SX":0,"SY":0,"TX":50,"TY":0,"LID":3,"HBW":-1,"A":[{"L":{"U":[[1,10]]}}]}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.opcode, func(t *testing.T) {
			low := coinGateInput(store, 1)
			step := Intent.Step{Opcode: testCase.opcode, Payload: json.RawMessage(testCase.payload)}
			err := newCoinDispatchGate().Validate(coinGateContext("low-"+testCase.opcode), low, step)
			if !errors.Is(err, Intent.ErrCoinUnavailable) {
				t.Fatalf("low-balance %s dispatch error = %v", testCase.opcode, err)
			}
			high := coinGateInput(store, 1_000_000)
			if err := newCoinDispatchGate().Validate(coinGateContext("high-"+testCase.opcode), high, step); err != nil {
				t.Fatalf("affordable %s dispatch error = %v", testCase.opcode, err)
			}
		})
	}
}

func TestEngineCoinGatePreventsAuditedOpcodesFromReachingTransport(t *testing.T) {
	store := coinGateGameData(t)
	tests := []struct{ opcode, payload string }{
		{"bup", `{"LID":0,"WID":1,"AMT":10}`},
		{"hru", `{"U":1,"A":10}`},
		{"cds", `{"SID":10,"TX":50,"TY":0,"HBW":-1,"A":[[1,10]]}`},
		{"cra", `{"SX":0,"SY":0,"TX":50,"TY":0,"LID":3,"HBW":-1,"A":[{"L":{"U":[[1,10]]}}]}`},
	}
	for _, testCase := range tests {
		t.Run(testCase.opcode, func(t *testing.T) {
			gameState := coinGateInput(store, 1).State
			coinGateReadySession(&gameState)
			stateStore := State.NewStore(gameState)
			coinGatePrimeStore(t, stateStore, 1)
			sender := &coinGateEngineSender{}
			registry := Intent.NewRegistry()
			if err := registry.Register(Intent.Definition{Name: "test.coin." + testCase.opcode, Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
				return Intent.Plan{Steps: []Intent.Step{{Opcode: testCase.opcode, Payload: json.RawMessage(testCase.payload)}}}, nil
			}}); err != nil {
				t.Fatal(err)
			}
			engine := Intent.NewEngine(registry, stateStore, coinGateStoreProvider{store}, sender, nil)
			engine.SetFinalDispatchProvider(newCoinDispatchGate())
			receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.coin." + testCase.opcode})
			if receipt.Status != Intent.StatusFailed || sender.sentCount() != 0 || !strings.Contains(receipt.DiagnosticError(), Intent.ErrCoinUnavailable.Error()) {
				t.Fatalf("short %s dispatch reached transport: receipt=%#v sends=%d", testCase.opcode, receipt, sender.sentCount())
			}
		})
	}
}

func TestEngineCoinGateRefreshesUncorrelatedDebitsWithinHeldClaims(t *testing.T) {
	store := coinGateGameData(t)
	gameState := coinGateInput(store, 120).State
	coinGateReadySession(&gameState)
	stateStore := State.NewStore(gameState)
	coinGatePrimeStore(t, stateStore, 120)
	observer := newCoinGateEngineObserver()
	sender := &coinGateEngineSender{state: stateStore, observer: observer}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "test.coin.batch", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		step := Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":10}`), AwaitOpcode: "bup", SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted}
		return Intent.Plan{Claims: []string{"account-resources"}, Steps: []Intent.Step{step, step}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, stateStore, coinGateStoreProvider{store}, sender, observer)
	engine.SetFinalDispatchProvider(newCoinDispatchGate())
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.coin.batch"})
	if receipt.Status != Intent.StatusSucceeded {
		t.Fatalf("funded batch failed after uncorrelated updates: %#v", receipt)
	}
	if got := sender.sentOpcodes(); !slices.Equal(got, []string{"bup", "gbd", "bup", "gbd"}) {
		t.Fatalf("batch/refresh opcodes = %v", got)
	}
}

func TestEngineCoinGateRechecksConsumedEquipmentRetryWithFreshResponseToken(t *testing.T) {
	store := coinGateGameData(t)
	gameState := coinGateInput(store, 1_000).State
	coinGateReadySession(&gameState)
	gameState.Inventory.Equipment[1] = State.EquipmentInstance{ID: 1, Level: 0}
	stateStore := State.NewStore(gameState)
	coinGatePrimeStore(t, stateStore, 1_000)
	observer := newCoinGateEngineObserver()
	sender := &coinGateEngineSender{state: stateStore, observer: observer, retryEquipment227: true}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "test.coin.retry", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{Steps: []Intent.Step{{
			Opcode: "eqe", Payload: json.RawMessage(`{"C2":0,"EID":1}`), AwaitOpcode: "eqe", SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted,
			ResponseRetry: &Intent.ResponseRetryPolicy{Codes: []int{227}, GuardAction: "test.retry.guard", DelayMillis: 1},
		}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, stateStore, coinGateStoreProvider{store}, sender, observer)
	engine.SetFinalDispatchProvider(newCoinDispatchGate())
	if err := engine.RegisterAction("test.retry.guard", func(context.Context, json.RawMessage) error { return nil }); err != nil {
		t.Fatal(err)
	}
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.coin.retry"})
	if receipt.Status != Intent.StatusSucceeded {
		t.Fatalf("consumed retry did not recover: %#v", receipt)
	}
	tokens := sender.tokensFor("eqe")
	if len(tokens) != 2 || tokens[0] == "" || tokens[1] == "" || tokens[0] == tokens[1] {
		t.Fatalf("equipment retry response tokens = %v", tokens)
	}
}

func TestEngineCoinGateDoesNotReplayConfirmedSpendWhenRefreshTimesOut(t *testing.T) {
	store := coinGateGameData(t)
	gameState := coinGateInput(store, 100).State
	coinGateReadySession(&gameState)
	stateStore := State.NewStore(gameState)
	coinGatePrimeStore(t, stateStore, 100)
	observer := newCoinGateEngineObserver()
	sender := &coinGateEngineSender{state: stateStore, observer: observer, dropGBD: true}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "test.coin.refresh_timeout", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{Steps: []Intent.Step{{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":10}`), AwaitOpcode: "bup", SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(registry, stateStore, coinGateStoreProvider{store}, sender, observer)
	engine.SetFinalDispatchProvider(newCoinDispatchGate())
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	receipt := engine.Submit(ctx, Intent.Request{Name: "test.coin.refresh_timeout"})
	if receipt.Status != Intent.StatusSucceeded || !slices.Equal(receipt.CompletedStepIndexes, []int{0}) {
		t.Fatalf("confirmed spend became replayable after refresh timeout: %#v", receipt)
	}
	if got := sender.sentOpcodes(); !slices.Equal(got, []string{"bup", "gbd"}) {
		t.Fatalf("timeout send sequence = %v", got)
	}
}

func TestEngineCoinGateMarksMalformedPostSendResponseIndeterminate(t *testing.T) {
	store := coinGateGameData(t)
	gameState := coinGateInput(store, 100).State
	coinGateReadySession(&gameState)
	stateStore := State.NewStore(gameState)
	coinGatePrimeStore(t, stateStore, 100)
	observer := newCoinGateEngineObserver()
	sender := &coinGateEngineSender{state: stateStore, observer: observer, missingResponseCode: true}
	registry := Intent.NewRegistry()
	if err := registry.Register(Intent.Definition{Name: "test.coin.malformed_response", Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
		return Intent.Plan{Steps: []Intent.Step{{
			Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":10}`), AwaitOpcode: "bup",
			SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted,
			ResponseProjectionFailureIndeterminate: true,
		}}}, nil
	}}); err != nil {
		t.Fatal(err)
	}
	gate := newCoinDispatchGate()
	engine := Intent.NewEngine(registry, stateStore, coinGateStoreProvider{store}, sender, observer)
	engine.SetFinalDispatchProvider(gate)
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.coin.malformed_response"})
	if receipt.Status != Intent.StatusIndeterminate || len(gate.pending) != 1 {
		t.Fatalf("malformed response did not retain an uncertain debit: receipt=%#v pending=%#v", receipt, gate.pending)
	}
	for _, debit := range gate.pending {
		if debit.indeterminateAt.IsZero() {
			t.Fatalf("malformed response debit has no reconciliation timestamp: %#v", debit)
		}
	}

	refreshed := coinGateInput(store, 50)
	refreshed.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{
		ObservedAt: time.Now().UTC().Add(time.Second), ConnectionGeneration: 1,
	}
	if err := gate.Validate(coinGateContext("after-malformed-refresh"), refreshed, Intent.Step{
		Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":1}`),
	}); err != nil {
		t.Fatal(err)
	}
	if len(gate.pending) != 1 {
		t.Fatalf("post-response authoritative refresh did not replace the uncertain debit: %#v", gate.pending)
	}
}

func TestCoinDispatchGateRechecksIdempotentReservationAndSerializesBudget(t *testing.T) {
	store := coinGateGameData(t)
	gate := newCoinDispatchGate()
	step := Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":12}`)}
	first := coinGateInput(store, 100)
	if err := gate.Validate(coinGateContext("first"), first, step); err != nil {
		t.Fatal(err)
	}
	if err := gate.Validate(coinGateContext("second"), first, step); !errors.Is(err, Intent.ErrCoinUnavailable) {
		t.Fatalf("concurrent reservation error = %v", err)
	}

	// The same router key may be validated more than once. Only reservation
	// creation is idempotent; a later authoritative balance must be rechecked.
	lower := coinGateInput(store, 50)
	lower.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{
		ObservedAt: first.State.Player.ResourceObservations[1].ObservedAt.Add(time.Second), ConnectionGeneration: 1,
	}
	if err := gate.Validate(coinGateContext("first"), lower, step); !errors.Is(err, Intent.ErrCoinUnavailable) {
		t.Fatalf("revalidation after balance drop error = %v", err)
	}
}

func TestCoinDispatchGateRequiresCorrelatedCompletionOrPostCompletionRefresh(t *testing.T) {
	store := coinGateGameData(t)
	gate := newCoinDispatchGate()
	step := Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":12}`)}
	ctx := coinGateContext("spend")
	initial := coinGateInput(store, 100)
	if err := gate.Validate(ctx, initial, step); err != nil {
		t.Fatal(err)
	}
	incomeAt := initial.State.Player.ResourceObservations[1].ObservedAt.Add(time.Second)
	unrelated := coinGateInput(store, 160)
	unrelated.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: incomeAt, ConnectionGeneration: 1}
	gate.Completed(ctx, unrelated, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: incomeAt.Add(time.Second)}})
	if len(gate.pending) != 1 {
		t.Fatalf("unrelated income released pending debit: %#v", gate.pending)
	}
	refresh := unrelated
	refreshAt := time.Now().UTC().Add(time.Second)
	refresh.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: refreshAt, ConnectionGeneration: 1}
	if err := gate.Validate(coinGateContext("after-refresh"), refresh, Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":1}`)}); err != nil {
		t.Fatal(err)
	}
	if _, exists := gate.pending[coinDispatchKey(ctx, step)]; exists {
		t.Fatalf("post-completion authoritative refresh did not reconcile: %#v", gate.pending)
	}
}

func TestCoinDispatchGateCorrelatedCompletionAdvancesWatermark(t *testing.T) {
	store := coinGateGameData(t)
	gate := newCoinDispatchGate()
	step := Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":12}`)}
	ctx := coinGateContext("spend-watermark")
	initial := coinGateInput(store, 100)
	if err := gate.Validate(ctx, initial, step); err != nil {
		t.Fatal(err)
	}
	committedAt := initial.State.Player.ResourceObservations[1].ObservedAt.Add(time.Second)
	committed := coinGateInput(store, 40)
	committed.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: committedAt, ConnectionGeneration: 1}
	gate.Completed(ctx, committed, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: committedAt}})
	if err := gate.Validate(coinGateContext("old-snapshot"), initial, step); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("old pre-completion balance restored budget: %v", err)
	}
}

func TestCoinDispatchGateRejectsOlderSnapshotAfterNewerValidation(t *testing.T) {
	store := coinGateGameData(t)
	gate := newCoinDispatchGate()
	newer := coinGateInput(store, 50)
	newerAt := time.Now().UTC()
	newer.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: newerAt, ConnectionGeneration: 1}
	step := Intent.Step{Opcode: "bup", Payload: json.RawMessage(`{"LID":0,"WID":1,"AMT":1}`)}
	if err := gate.Validate(coinGateContext("newer"), newer, step); err != nil {
		t.Fatal(err)
	}
	older := coinGateInput(store, 1_000_000)
	older.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: newerAt.Add(-time.Second), ConnectionGeneration: 1}
	err := gate.Validate(coinGateContext("older"), older, step)
	if !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("older high-balance snapshot error = %v", err)
	}
}

func TestCoinDispatchGateConsumedRetryReleasesCorrelatedDebitAndUsesFreshToken(t *testing.T) {
	store := coinGateGameData(t)
	gate := newCoinDispatchGate()
	step := Intent.Step{CoinCost: &Intent.CoinCostRequirement{Amount: 10, Source: "test roll"}}
	firstContext := Outbound.WithMetadata(context.Background(), Outbound.Metadata{OperationID: "upgrade", ResponseToken: "upgrade/1"})
	initial := coinGateInput(store, 100)
	if err := gate.Validate(firstContext, initial, step); err != nil {
		t.Fatal(err)
	}
	responseAt := time.Now().UTC()
	afterRoll := coinGateInput(store, 90)
	afterRoll.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: responseAt, ConnectionGeneration: 1}
	code := 227
	gate.Completed(firstContext, afterRoll, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: responseAt, ResponseCode: &code}})
	if len(gate.pending) != 0 {
		t.Fatalf("correlated consumed retry retained debit: %#v", gate.pending)
	}
	secondContext := Outbound.WithMetadata(context.Background(), Outbound.Metadata{OperationID: "upgrade", ResponseToken: "upgrade/2"})
	if err := gate.Validate(secondContext, afterRoll, step); err != nil {
		t.Fatal(err)
	}
	if len(gate.pending) != 1 {
		t.Fatalf("fresh retry token did not create one reservation: %#v", gate.pending)
	}
}

func TestProductionCoinCostAcceptsOfficialNonCoinItemAndRejectsMalformedCost(t *testing.T) {
	store := coinGateGameData(t)
	if cost, err := productionCoinCost(store, json.RawMessage(`{"LID":0,"WID":2,"AMT":50}`)); err != nil || cost.amount != 0 {
		t.Fatalf("official non-coin unit cost = %+v, %v", cost, err)
	}
	if _, err := productionCoinCost(store, json.RawMessage(`{"LID":0,"WID":3,"AMT":1}`)); err == nil {
		t.Fatal("malformed costC1 was accepted")
	}
	if _, err := productionCoinCost(store, json.RawMessage(`{"LID":0,"WID":999,"AMT":1}`)); err == nil {
		t.Fatal("missing official item was accepted")
	}
}

func TestShopPackageCoinCostAcceptsOfficialFreePackageAndRejectsUnknownPackage(t *testing.T) {
	store := coinGateGameData(t)
	if cost, err := shopPackageCoinCost(store, json.RawMessage(`{"PID":29,"AMT":3}`)); err != nil || cost.amount != 0 {
		t.Fatalf("official free package cost = %+v, %v", cost, err)
	}
	if _, err := shopPackageCoinCost(store, json.RawMessage(`{"PID":999,"AMT":1}`)); err == nil {
		t.Fatal("missing official package was accepted")
	}
	if _, err := shopPackageCoinCost(store, json.RawMessage(`{"PID":28,"AMT":1,"BA":1}`)); err == nil {
		t.Fatal("coin-priced package buy-all was accepted without authoritative total pricing")
	}
}

func TestResolveCoinCostCoversSupportedSpendingOpcodes(t *testing.T) {
	input := coinGateInput(coinGateGameData(t), 1_000_000)
	input.State.Inventory.Equipment[1] = State.EquipmentInstance{ID: 1, Level: 0}
	input.State.Inventory.Gems[10] = State.GemInstance{ID: 10, Level: 8}
	input.State.Market.CaravanLevelLoaded = true
	input.State.Market.CaravanLevel = 1
	input.State.Market.ObservedAt = time.Now().UTC()
	input.State.Market.Castles[10] = State.MarketCastleState{CastleID: 10, AvailableBarrows: 100}
	tests := []struct {
		opcode  string
		payload string
		fixed   *Intent.CoinCostRequirement
	}{
		{"bup", `{"LID":0,"WID":1,"AMT":2}`, nil},
		{"hru", `{"U":1,"A":2}`, nil},
		{"cds", `{"SID":10,"TX":50,"TY":0,"HBW":-1,"A":[[1,10]]}`, nil},
		{"cra", `{"SX":0,"SY":0,"TX":50,"TY":0,"LID":3,"HBW":-1,"A":[{"L":{"U":[[1,10]]}}]}`, nil},
		{"csm", `{"SID":10,"TX":50,"TY":0,"SC":1}`, nil},
		{"crm", `{"SID":10,"TX":50,"TY":0,"HBW":-1,"G":[["W",100]]}`, nil},
		{"crun", `{}`, &Intent.CoinCostRequirement{Amount: 500_000, Reserve: 10, Source: "official rental"}},
		{"crst", `{}`, &Intent.CoinCostRequirement{Amount: 100, Reserve: 10, Source: "official recipe"}},
		{"sbp", `{"PID":28,"AMT":1}`, &Intent.CoinCostRequirement{Amount: 220, Source: "official package"}},
		{"ere", `{"RIID":1,"EQ":1,"C2":0}`, nil},
		{"eqe", `{"EID":1,"C2":0}`, nil},
		{"bge", `{"GID":100,"RGEM":0,"M":0}`, nil},
		{"kut", `{"TKID":2,"A":[[1,2],[4,1]]}`, nil},
	}
	for _, testCase := range tests {
		t.Run(testCase.opcode, func(t *testing.T) {
			cost, covered, err := resolveCoinCost(input, Intent.Step{Opcode: testCase.opcode, Payload: json.RawMessage(testCase.payload), CoinCost: testCase.fixed})
			if err != nil || !covered || cost.amount <= 0 {
				t.Fatalf("cost = %+v covered=%t err=%v", cost, covered, err)
			}
		})
	}
}

func TestAttackCoinCostUsesConnectionGenerationRatherThanSessionStatusTimestamp(t *testing.T) {
	input := coinGateInput(coinGateGameData(t), 1_000)
	input.State.DailyAttacks.ObservedAt = time.Now().UTC().Add(-time.Minute)
	input.State.Session.ChangedAt = time.Now().UTC() // readiness/status can advance after baseline GAI
	payload := json.RawMessage(`{"SX":0,"SY":0,"TX":50,"TY":0,"LID":3,"HBW":-1,"A":[{"L":{"U":[[1,10]]}}]}`)
	if _, err := attackCoinCost(input, payload); err != nil {
		t.Fatalf("same-connection ready transition blocked attack cost: %v", err)
	}
	input.State.DailyAttacks.ConnectionGeneration = 0
	if _, err := attackCoinCost(input, payload); err == nil {
		t.Fatal("stale persisted daily-attack state was accepted")
	}
}

func coinGateContext(operationID string) context.Context {
	return Outbound.WithMetadata(context.Background(), Outbound.Metadata{OperationID: operationID})
}

func coinGateInput(store *GameData.Store, coins float64) Intent.PlanningContext {
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 1
	state.Player.Resources[1] = coins
	state.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC().Add(-time.Minute), ConnectionGeneration: 1}
	state.Castles[10] = State.CastleState{ID: 10, X: 0, Y: 0}
	state.DailyAttacks = State.DailyAttackState{Count: 0, ServerThreshold: 10, ObservedAt: time.Now().UTC(), ConnectionGeneration: 1}
	return Intent.PlanningContext{State: state, GameData: store}
}

func coinGateReadySession(state *State.GameState) {
	now := time.Now().UTC()
	state.Session = State.SessionState{Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1, LoggedIn: true, SocketReady: true, ChangedAt: now.Add(-time.Minute)}
}

func coinGatePrimeStore(t *testing.T, store *State.Store, coins float64) {
	t.Helper()
	_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
		current.Player.Resources[1] = coins
		current.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC().Add(-time.Second), ConnectionGeneration: 1}
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

type coinGateStoreProvider struct{ store *GameData.Store }

func (provider coinGateStoreProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

type coinGateEngineObserver struct {
	mu       sync.Mutex
	watchers map[string]chan Protocol.CommittedFrame
}

func newCoinGateEngineObserver() *coinGateEngineObserver {
	return &coinGateEngineObserver{watchers: map[string]chan Protocol.CommittedFrame{}}
}

func (observer *coinGateEngineObserver) Watch(opcode string, _ uint64) (<-chan Protocol.CommittedFrame, func()) {
	return observer.watch(opcode)
}

func (observer *coinGateEngineObserver) WatchResponse(_ string, _ uint64, responseToken string) (<-chan Protocol.CommittedFrame, func()) {
	return observer.watch(responseToken)
}

func (observer *coinGateEngineObserver) watch(key string) (<-chan Protocol.CommittedFrame, func()) {
	channel := make(chan Protocol.CommittedFrame, 1)
	observer.mu.Lock()
	observer.watchers[key] = channel
	observer.mu.Unlock()
	return channel, func() {}
}

func (observer *coinGateEngineObserver) emit(opcode string, frame Protocol.CommittedFrame) {
	observer.mu.Lock()
	channel := observer.watchers[opcode]
	observer.mu.Unlock()
	channel <- frame
}

type coinGateEngineSender struct {
	mu                  sync.Mutex
	state               *State.Store
	observer            *coinGateEngineObserver
	opcodes             []string
	tokens              []string
	retryEquipment227   bool
	equipmentAttempts   int
	dropGBD             bool
	missingResponseCode bool
}

func (*coinGateEngineSender) Ready() bool               { return true }
func (*coinGateEngineSender) Namespace() string         { return "EmpireEx_21" }
func (*coinGateEngineSender) CorrelatesResponses() bool { return true }
func (sender *coinGateEngineSender) Send(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	frame, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.mu.Lock()
	sender.opcodes = append(sender.opcodes, frame.Opcode)
	responseToken := Outbound.MetadataFromContext(ctx).ResponseToken
	sender.tokens = append(sender.tokens, responseToken)
	if frame.Opcode == "eqe" {
		sender.equipmentAttempts++
	}
	equipmentAttempt := sender.equipmentAttempts
	sender.mu.Unlock()
	if sender.observer == nil {
		return nil
	}
	if frame.Opcode == "gbd" && sender.dropGBD {
		return nil
	}
	responseAt := time.Now().UTC().Add(time.Millisecond)
	if sender.state != nil {
		_, err = sender.state.ApplyComponents(State.Components(State.ComponentPlayer), func(current *State.GameState) ([]string, bool, error) {
			if frame.Opcode == "bup" {
				current.Player.Resources[1] -= 50
			} else if frame.Opcode == "eqe" {
				current.Player.Resources[1] -= 170
			}
			current.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: responseAt, ConnectionGeneration: 1}
			return []string{"resources"}, true, nil
		})
		if err != nil {
			return err
		}
	}
	code := 0
	if frame.Opcode == "eqe" && sender.retryEquipment227 && equipmentAttempt == 1 {
		code = 227
	}
	ackAt := responseAt
	if frame.Opcode == "bup" {
		ackAt = responseAt.Add(time.Millisecond) // distinct C1 update before acknowledgement
	}
	emitKey := frame.Opcode
	if responseToken != "" {
		emitKey = responseToken
	}
	var responseCode *int
	if !sender.missingResponseCode {
		responseCode = &code
	}
	sender.observer.emit(emitKey, Protocol.CommittedFrame{Frame: Protocol.Frame{Opcode: frame.Opcode, ResponseCode: responseCode, ReceivedAt: ackAt, ResponseToken: responseToken}})
	return nil
}

func (sender *coinGateEngineSender) tokensFor(opcode string) []string {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	result := []string{}
	for index, candidate := range sender.opcodes {
		if candidate == opcode {
			result = append(result, sender.tokens[index])
		}
	}
	return result
}

func (sender *coinGateEngineSender) sentCount() int {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return len(sender.opcodes)
}

func (sender *coinGateEngineSender) sentOpcodes() []string {
	sender.mu.Lock()
	defer sender.mu.Unlock()
	return append([]string(nil), sender.opcodes...)
}

func coinGateGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"constructionItems":[],"levelBoosters":[],"effects":[],
		"resources":[{"resourceID":1,"JSONKey":"C1"}],
		"buildings":[{"wodID":137,"marketCarriages":5}],
		"units":[
			{"wodID":1,"costC1":5,"healingCostC1":7},
			{"wodID":2,"costDragonGlassArrows":2},
			{"wodID":3,"costC1":"broken"},
			{"wodID":4,"slotTypes":"1","costC1":2}
		],
		"horses":[],"currencies":[],
		"gems":[{"gemID":100,"gemLevelID":1}],
		"gemlevels":[{"gemLevelID":1,"insertCostC1":1000}],
		"relicEnchanters":[{"level":1,"c1Cost":100000}],
		"kingdoms":[{"kID":2,"unitTravelTaxRate":30}],
		"packages":[{"packageID":28,"packagePriceC1":220},{"packageID":29}]
	}`), GameData.SourceMetadata{ItemVersion: "coin-gate-test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

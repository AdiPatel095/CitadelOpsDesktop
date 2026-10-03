package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"reflect"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/RiftTemplates"
	"CitadelDesktop/Server/State"
)

// SCE raw frames exercise the actual applyPlayerCurrencies decoder, not a
// manufactured authority marker. IDs/coordinates in captured fixtures are synthetic.
func ticketReplayInput(t *testing.T, rows string) (Intent.PlanningContext, *State.Store) {
	t.Helper()
	data, err := GameData.DecodeStore([]byte(`{"versionInfo":[],"buildings":[],"units":[],"currencies":[{"currencyID":22,"JSONKey":"PTT"}]}`), GameData.SourceMetadata{ItemVersion: "travel-ticket-replay"})
	if err != nil {
		t.Fatal(err)
	}
	initial := coinGateInput(data, 1000000).State
	coinGateReadySession(&initial)
	store := State.NewStore(&initial)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(store, coinGateStoreProvider{data}, registry)
	if rows != "" {
		code := 0
		_, err = pipeline.HandleFrame(t.Context(), Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "sce", ResponseCode: &code, Payload: json.RawMessage(rows), ReceivedAt: time.Now().UTC()})
		if err != nil {
			t.Fatal(err)
		}
	}
	input := Intent.PlanningContext{State: store.Snapshot(), GameData: data}
	return input, store
}

func TestTravelTicketGateReservationsAndFailures(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",2]]`)
	gate := newTravelTicketDispatchGate()
	step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"PTT":1}`)}
	for _, token := range []string{"one", "one", "two"} {
		if err := gate.Validate(coinGateContext(token), input, step); err != nil {
			t.Fatal(err)
		}
	}
	if err := gate.Validate(coinGateContext("three"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("third send error=%v", err)
	}
	gate.DefinitiveFailure(coinGateContext("one"), step)
	if err := gate.Validate(coinGateContext("three"), input, step); err != nil {
		t.Fatal(err)
	}
	gate.Indeterminate(coinGateContext("two"), step)
	if err := gate.Validate(coinGateContext("four"), input, step); err != nil {
		t.Fatal(err)
	}
	input.State.Session.ConnectionGeneration++
	if err := gate.Validate(coinGateContext("stale"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("stale session error=%v", err)
	}
	step.Payload = json.RawMessage(`{"PTT":0}`)
	if err := gate.Validate(coinGateContext("free"), input, step); err != nil {
		t.Fatal(err)
	}
}

func TestTravelTicketGateConcurrentMovements(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",2]]`)
	gate := newTravelTicketDispatchGate()
	step := Intent.Step{Opcode: "cds", Payload: json.RawMessage(`{"PTT":1}`)}
	var group sync.WaitGroup
	results := make(chan error, 3)
	for i := 0; i < 3; i++ {
		group.Add(1)
		go func(i int) { defer group.Done(); results <- gate.Validate(coinGateContext(fmt.Sprint(i)), input, step) }(i)
	}
	group.Wait()
	close(results)
	allowed := 0
	for err := range results {
		if err == nil {
			allowed++
		} else if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
			t.Fatal(err)
		}
	}
	if allowed != 2 {
		t.Fatalf("allowed %d movements", allowed)
	}
}

func TestTravelTicketGateSettlesOnlyAfterCompletionAndRejectsOlderWatermarks(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",1]]`)
	gate := newTravelTicketDispatchGate()
	step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"PTT":1}`)}
	ctx := coinGateContext("one")
	if err := gate.Validate(ctx, input, step); err != nil {
		t.Fatal(err)
	}
	old := input.State.Player.CurrencyObservations[22]
	fresh := old
	fresh.ObservedAt = time.Now().UTC()
	input.State.Player.CurrencyObservations[22] = fresh
	// A refresh before acknowledgement cannot give this ticket to another move.
	if err := gate.Validate(coinGateContext("two"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("early refresh released debit: %v", err)
	}
	if !gate.Completed(ctx, input, step, Protocol.CommittedFrame{Frame: Protocol.Frame{ReceivedAt: old.ObservedAt}}) {
		t.Fatal("uncorrelated completion did not request refresh")
	}
	after := fresh
	after.ObservedAt = time.Now().UTC().Add(time.Millisecond)
	input.State.Player.CurrencyObservations[22] = after
	if err := gate.Validate(coinGateContext("two"), input, step); err != nil {
		t.Fatal(err)
	}
	input.State.Player.CurrencyObservations[22] = old
	if err := gate.Validate(coinGateContext("older"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("old snapshot error=%v", err)
	}
	input.State.Player.CurrencyObservations[22] = after
	input.State.Player.Currencies[22] = 2
	if err := gate.Validate(coinGateContext("changed-same-time"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("equal watermark changed balance error=%v", err)
	}
}

func TestFinalDispatchCompositePreservesCoinGateAndCompensatesEarlierReservations(t *testing.T) {
	input := coinGateInput(coinGateGameData(t), 1000000)
	fundTravelTicketsForTest(&input.State)
	input.State.Player.Currencies[22] = 0
	coins := newCoinDispatchGate()
	tickets := newTravelTicketDispatchGate()
	gate := newFinalDispatchGates(coins, tickets)
	step := Intent.Step{Opcode: "cds", Payload: json.RawMessage(`{"SID":10,"TX":50,"TY":0,"HBW":-1,"PTT":1,"A":[[1,10]]}`)}
	if err := gate.Validate(coinGateContext("blocked"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatal(err)
	}
	if len(coins.pending) != 0 || len(tickets.pending) != 0 {
		t.Fatal("later gate leaked earlier reservation")
	}
	input.State.Player.Currencies[22] = 2
	input.State.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: 1}
	if err := gate.Validate(coinGateContext("funded"), input, step); err != nil {
		t.Fatal(err)
	}
	gate.DefinitiveFailure(coinGateContext("funded"), step)
	input.State.Player.Resources[1] = 1
	input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC().Add(time.Second), ConnectionGeneration: 1}
	if err := gate.Validate(coinGateContext("short-coins"), input, step); !errors.Is(err, Intent.ErrCoinUnavailable) {
		t.Fatalf("coin shortage=%v", err)
	}
}

func TestAttackTravelHelperUsesAuthorityAndPendingWithoutChangingChoice(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",1]]`)
	gate := newTravelTicketDispatchGate()
	input.CurrencyAvailability = gate
	body := attackBody{Booster: 1007, PremiumTravel: 0}
	if err := applyCastleHorseTravelBoost(&body, input, State.CastleState{}, -1); err != nil {
		t.Fatal(err)
	}
	if body.Booster != -1 || body.PremiumTravel != 1 {
		t.Fatal("funded selection changed")
	}
	step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"PTT":1}`)}
	if err := gate.Validate(coinGateContext("reserved"), input, step); err != nil {
		t.Fatal(err)
	}
	if err := applyCastleHorseTravelBoost(&body, input, State.CastleState{}, -1); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("planner ignored pending: %v", err)
	}
	if body.Booster != -1 || body.PremiumTravel != 1 {
		t.Fatal("short selection substituted")
	}
}

func TestSupportPlansAllBatchesAgainstTicketBudget(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",2]]`)
	amounts := map[State.UnitID]int64{}
	for i := 1; i <= 21; i++ {
		amounts[State.UnitID(i)] = 1
	}
	if _, err := supportDispatchStep(input, "Support", State.CastleState{ID: 10}, State.AllianceHolding{}, 0, amounts, Intent.Step{}); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("three batches ignored shortage: %v", err)
	}
	delete(amounts, 21)
	step, err := supportDispatchStep(input, "Support", State.CastleState{ID: 10}, State.AllianceHolding{}, 0, amounts, Intent.Step{})
	if err != nil || len(step.Batch) != 2 {
		t.Fatalf("two funded batches = %d, %v", len(step.Batch), err)
	}
}

func TestRiftCapturedChoiceWaitsWithoutFallback(t *testing.T) {
	state, launch := configuredRiftReplayFixture()
	state.Player.Currencies[22] = 0
	input := Intent.PlanningContext{State: state, GameData: coinGateGameData(t)}
	app := configuredRiftReplayApplication(t, RiftTemplates.Document{Version: 1, Launches: map[string]State.RiftLaunch{launch.ID: launch}, DeletedLaunchIDs: map[string]int64{}})
	_, err := app.planRiftReplay(t.Context(), input, json.RawMessage(`{"launchId":"rift-safe"}`))
	if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("captured Rift choice error=%v", err)
	}
}

// replayTravelSender validates exactly as the real outbound router does, and
// records only commands that passed the final dispatch boundary.
type replayTravelSender struct {
	payloads []json.RawMessage
	store    *State.Store
	observer *coinGateEngineObserver
}

func (s *replayTravelSender) CorrelatesResponses() bool { return true }
func (s *replayTravelSender) Ready() bool               { return true }
func (s *replayTravelSender) Namespace() string         { return "EmpireEx" }
func (s *replayTravelSender) Send(ctx context.Context, raw []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	frame, err := Protocol.Decode(string(raw), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	s.payloads = append(s.payloads, append(json.RawMessage(nil), frame.Payload...))
	responseAt := time.Now().UTC()
	_, err = s.store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Currencies[22]--
		state.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: responseAt, ConnectionGeneration: state.Session.ConnectionGeneration}
		return []string{"currencies"}, true, nil
	})
	if err != nil {
		return err
	}
	token := Outbound.MetadataFromContext(ctx).ResponseToken
	code := 0
	s.observer.emit(token, Protocol.CommittedFrame{Frame: Protocol.Frame{Opcode: frame.Opcode, ResponseCode: &code, ReceivedAt: responseAt, ResponseToken: token}})
	return nil
}

func TestCapturedCRA327AndCDS327SenderReplay(t *testing.T) {
	raw, err := os.ReadFile("testdata/travel-ticket-327.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]struct {
		CapturedDate string          `json:"capturedDate"`
		ResponseCode int             `json:"responseCode"`
		Payload      json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if len(fixtures) != 2 {
		t.Fatal("missing captured CRA/CDS fixtures")
	}
	for opcode, fixture := range fixtures {
		if fixture.CapturedDate != "2026-10-01" || fixture.ResponseCode != 327 {
			t.Fatal("incorrect captured provenance")
		}
		for _, rows := range []string{"", `[["PTT",0]]`, `[["PTT",5]]`} {
			t.Run(opcode+rows, func(t *testing.T) {
				input, store := ticketReplayInput(t, rows)
				registry := Intent.NewRegistry()
				if err := registry.Register(Intent.Definition{Name: "test.ticket." + opcode, Effect: Intent.EffectWrite, Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
					return Intent.Plan{Steps: []Intent.Step{{Opcode: opcode, Payload: fixture.Payload}}}, nil
				}}); err != nil {
					t.Fatal(err)
				}
				observer := newCoinGateEngineObserver()
				sender := &replayTravelSender{store: store, observer: observer}
				engine := Intent.NewEngine(registry, store, coinGateStoreProvider{input.GameData}, sender, observer)
				engine.SetFinalDispatchProvider(newTravelTicketDispatchGate())
				receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.ticket." + opcode, Actor: "automation:autoBird", AutomationLane: "autoBird"})
				if rows != `[["PTT",5]]` {
					if len(sender.payloads) != 0 || receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureAvailability || receipt.Failure.Toast || !store.Snapshot().Automations["autoBird"].SafetyLock.ObservedAt.IsZero() {
						t.Fatalf("short send=%d status=%s error=%s", len(sender.payloads), receipt.Status, receipt.DiagnosticError())
					}
				} else if len(sender.payloads) != 1 || !reflect.DeepEqual([]byte(sender.payloads[0]), []byte(fixture.Payload)) {
					t.Fatalf("funded payload changed: sends=%d status=%s error=%s", len(sender.payloads), receipt.Status, receipt.DiagnosticError())
				}
			})
		}
	}
}

func TestStormIslandReturnRequiresCurrentSessionTickets(t *testing.T) {
	state := State.NewGameState()
	fundTravelTicketsForTest(&state)
	state.Castles[10] = State.CastleState{ID: 10, KingdomID: 4, X: 100, Y: 100, Focused: true}
	state.Storm.IslandReturns[State.StormIslandReturnKey(4, 101, 101)] = State.StormIslandReturnState{KingdomID: 4, SourceCastleID: 10, TargetX: 101, TargetY: 101, IslandObjectID: 20, ReportID: 30, Status: State.StormIslandReturnReady, LeaveBehind: 1, Survivors: map[State.UnitID]int64{1: 2}, ReportedAt: time.Now().UTC()}
	args := json.RawMessage(`{"sourceCastleId":10,"kingdomId":4,"islandX":101,"islandY":101,"islandObjectId":20,"reportId":30,"units":[{"unitId":1,"amount":1}]}`)
	state.Player.Currencies[22] = 0
	if _, err := planStormIslandReturn(t.Context(), Intent.PlanningContext{State: state, GameData: coinGateGameData(t)}, args); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("short island return=%v", err)
	}
	state.Player.Currencies[22] = 5
	state.Session.ConnectionGeneration++
	if _, err := planStormIslandReturn(t.Context(), Intent.PlanningContext{State: state, GameData: coinGateGameData(t)}, args); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("stale island return=%v", err)
	}
}

package App

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestSophieAdvisorFeatherReservationCount(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",3]]`)
	gate := newTravelTicketDispatchGate()
	step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"HBW":-1,"PTT":1,"AAC":3,"AAM":0,"AAT":1}`)}
	if err := gate.Validate(coinGateContext("advisor"), input, step); err != nil {
		t.Fatal(err)
	}
	observed, pending, known := gate.AvailableCurrency(input.State, Intent.TravelTicketCurrencyID)
	t.Logf("three-attack CRA: observed=%d pending=%d known=%v", observed, pending, known)
	if pending != 3 {
		t.Errorf("three-attack CRA reserved %d tickets; expected 3", pending)
	}
	other := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"HBW":-1,"PTT":1}`)}
	err := gate.Validate(coinGateContext("other"), input, other)
	t.Logf("concurrent fourth movement error=%v", err)
	if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Errorf("fourth movement permitted after three-attack CRA: %v", err)
	}
}
func TestSophieAdvisorShortAtFinalDispatch(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",1]]`)
	gate := newTravelTicketDispatchGate()
	step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"HBW":-1,"PTT":1,"AAC":3,"AAM":0,"AAT":1}`)}
	err := gate.Validate(coinGateContext("advisor"), input, step)
	if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Fatalf("final gate permits three-attack CRA with one feather: %v", err)
	}
}

func TestSophieAdvisorCountAtCompositeDispatch(t *testing.T) {
	input, _ := ticketReplayInput(t, `[["PTT",1]]`)
	input.GameData = coinGateGameData(t)
	input.State.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: input.State.Session.ConnectionGeneration}
	raw, err := os.ReadFile("testdata/travel-ticket-327.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(fixtures["cra"].Payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["AAC"] = json.RawMessage(`3`)
	fields["AAM"] = json.RawMessage(`0`)
	fields["AAT"] = json.RawMessage(`1`)
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	gate := newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate())
	step := Intent.Step{Opcode: "cra", Payload: payload}
	err = gate.Validate(coinGateContext("advisor-full"), input, step)
	t.Logf("captured-key-set CRA with AAC=3 at composite coin+feather gate: %v", err)
	if !errors.Is(err, Intent.ErrCurrencyUnavailable) {
		t.Errorf("composite dispatch permits AAC=3 with one feather: %v", err)
	}
}

type sophieChangedBalanceSender struct{ *replayTravelSender }

func (s *sophieChangedBalanceSender) Send(ctx context.Context, raw []byte) error {
	_, err := s.store.ApplyComponents(State.Components(State.ComponentPlayer), func(st *State.GameState) ([]string, bool, error) {
		st.Player.Currencies[22] = 1
		st.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: st.Session.ConnectionGeneration}
		return []string{"currencies"}, true, nil
	})
	if err != nil {
		return err
	}
	return s.replayTravelSender.Send(ctx, raw)
}
func TestSophieAdvisorSenderRace(t *testing.T) {
	input, store := ticketReplayInput(t, `[["PTT",3]]`)
	data := coinGateGameData(t)
	_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(st *State.GameState) ([]string, bool, error) {
		st.Player.ResourceObservations[1] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: st.Session.ConnectionGeneration}
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/travel-ticket-327.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures map[string]struct {
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(fixtures["cra"].Payload, &fields); err != nil {
		t.Fatal(err)
	}
	fields["AAC"] = json.RawMessage(`3`)
	fields["AAM"] = json.RawMessage(`0`)
	fields["AAT"] = json.RawMessage(`1`)
	payload, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	registry := Intent.NewRegistry()
	err = registry.Register(Intent.Definition{Name: "test.sophie.advisor", Effect: Intent.EffectWrite, Planner: func(_ context.Context, in Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
		if err := Intent.RequireTravelTickets(in, 3); err != nil {
			return Intent.Plan{}, err
		}
		return Intent.Plan{Steps: []Intent.Step{{Opcode: "cra", Payload: payload}}}, nil
	}})
	if err != nil {
		t.Fatal(err)
	}
	observer := newCoinGateEngineObserver()
	recorder := &replayTravelSender{store: store, observer: observer}
	sender := &sophieChangedBalanceSender{recorder}
	engine := Intent.NewEngine(registry, store, coinGateStoreProvider{data}, sender, observer)
	engine.SetFinalDispatchProvider(newFinalDispatchGates(newCoinDispatchGate(), newTravelTicketDispatchGate()))
	receipt := engine.Submit(t.Context(), Intent.Request{Name: "test.sophie.advisor", Actor: "automation:autoAdvisor", AutomationLane: "autoAdvisor"})
	t.Logf("planned feathers=%v; final feathers=1; outbound CRA count=%d; status=%s; error=%s", input.State.Player.Currencies[22], len(recorder.payloads), receipt.Status, receipt.DiagnosticError())
	if receipt.Failure == nil || receipt.Failure.Kind != Intent.FailureAvailability || receipt.Failure.Toast || !store.Snapshot().Automations["autoAdvisor"].SafetyLock.ObservedAt.IsZero() {
		t.Fatalf("expected availability wait without lock: status=%s error=%s failure=%#v", receipt.Status, receipt.DiagnosticError(), receipt.Failure)
	}
	if !strings.Contains(receipt.DiagnosticError(), "3 needed; 1 available") {
		t.Fatalf("missing movement amounts in status: %s", receipt.DiagnosticError())
	}
	if len(recorder.payloads) != 0 {
		t.Errorf("three-attack CRA was sent after final balance fell to one feather")
	}
}

func TestTravelTicketGateAdvisorAndBaronPendingBudget(t *testing.T) {
	for _, advisorType := range []int{1, 4} {
		t.Run(fmt.Sprint(advisorType), func(t *testing.T) {
			input, _ := ticketReplayInput(t, `[["PTT",3]]`)
			gate := newTravelTicketDispatchGate()
			input.CurrencyAvailability = gate
			payload, err := json.Marshal(map[string]int{"PTT": 1, "HBW": -1, "AAC": 3, "AASM": 0, "AAT": advisorType})
			if err != nil {
				t.Fatal(err)
			}
			step := Intent.Step{Opcode: "cra", Payload: payload}
			ctx := coinGateContext("advisor")
			for i := 0; i < 2; i++ {
				if err := gate.Validate(ctx, input, step); err != nil {
					t.Fatal(err)
				}
			}
			if err := Intent.RequireTravelTickets(input, 1); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
				t.Fatalf("planner reused reserved feathers: %v", err)
			}
			state, data, request := advisorIntentFixture(t)
			state.Player = input.State.Player
			state.Session = input.State.Session
			args, err := json.Marshal(request)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := planAdvisorAttack(t.Context(), Intent.PlanningContext{State: state, GameData: data, CurrencyAvailability: gate}, args); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
				t.Fatalf("Advisor planner reused reserved feathers: %v", err)
			}
			gate.DefinitiveFailure(ctx, step)
			if err := Intent.RequireTravelTickets(input, 3); err != nil {
				t.Fatalf("rejection did not release three feathers: %v", err)
			}
			if err := gate.Validate(ctx, input, step); err != nil {
				t.Fatal(err)
			}
			gate.Indeterminate(ctx, step)
			if err := Intent.RequireTravelTickets(input, 3); err != nil {
				t.Fatalf("uncertain send did not release three feathers: %v", err)
			}
			if !bytes.Equal(step.Payload, payload) {
				t.Fatal("gate rewrote Advisor payload")
			}
		})
	}
}

func TestTravelTicketGateInvalidAdvisorCountsFailClosed(t *testing.T) {
	for _, fields := range []string{`"AAC":0`, `"AAC":-1`, `"AAC":null`, `"AAC":"3"`, `"AAC":1.5`, `"AAC":9223372036854775808`, `"AAT":1`, `"AASM":0`} {
		t.Run(fields, func(t *testing.T) {
			input, _ := ticketReplayInput(t, `[["PTT",100]]`)
			gate := newTravelTicketDispatchGate()
			step := Intent.Step{Opcode: "cra", Payload: json.RawMessage(`{"PTT":1,` + fields + `}`)}
			if err := gate.Validate(coinGateContext("invalid"), input, step); !errors.Is(err, Intent.ErrCurrencyUnavailable) {
				t.Fatalf("unknown movement count sent: %v", err)
			}
			_, pending, _ := gate.AvailableCurrency(input.State, Intent.TravelTicketCurrencyID)
			if pending != 0 {
				t.Fatalf("invalid count reserved %d feathers", pending)
			}
		})
	}
}

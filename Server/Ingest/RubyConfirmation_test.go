package Ingest

import (
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestRubyConfirmationIngestDistinguishesSettingFromQuote(t *testing.T) {
	state := State.NewGameState()
	state.Session = State.SessionState{Generation: 3, LoggedIn: true, SocketReady: true}
	for _, tc := range []struct {
		raw    string
		known  bool
		amount int64
	}{
		{`{"CC2T":1000000}`, true, 1000000}, {`{"CC2T":1000001}`, false, 0}, {`{"CC2T":9223372036854775807}`, false, 0},
		{`{"CC2T":1}`, true, 1}, {`{"CC2T":-1}`, true, -1}, {`{}`, false, 0}, {`{"CC2T":null}`, false, 0},
		{`{"CC2T":0}`, false, 0}, {`{"CC2T":-2}`, false, 0}, {`{"CC2T":1.5}`, false, 0}, {`{"CC2T":"1"}`, false, 0}, {`[]`, false, 0},
	} {
		frame := Protocol.Frame{Opcode: "opt", Payload: json.RawMessage(tc.raw), ReceivedAt: time.Now()}
		_, _, err := reduceRubyConfirmation(context.Background(), frame, &state, nil)
		if err != nil || state.Player.RubyConfirmation.Known != tc.known || state.Player.RubyConfirmation.Amount != tc.amount {
			t.Fatalf("%s: %+v %v", tc.raw, state.Player.RubyConfirmation, err)
		}
	}
	frame := Protocol.Frame{Opcode: "gbd", Payload: json.RawMessage(`{"opt":{"CC2T":2500}}`), ReceivedAt: time.Now()}
	if _, _, err := reduceInitialState(context.Background(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	if state.Player.RubyConfirmation.Amount != 2500 {
		t.Fatal(state.Player.RubyConfirmation)
	}
	frame.Payload = json.RawMessage(`{}`)
	if _, _, err := reduceInitialState(context.Background(), frame, &state, nil); err != nil {
		t.Fatal(err)
	}
	if state.Player.RubyConfirmation.Known {
		t.Fatal("missing GBD settings retained authority")
	}
}

func TestUnchangedGBDRubySettingDoesNotWakeBuilder(t *testing.T) {
	state := State.NewGameState()
	state.Session.Generation = 1
	frame := Protocol.Frame{Opcode: "gbd", Payload: json.RawMessage(`{"opt":{"CC2T":2500}}`), ReceivedAt: time.Now()}
	for _, want := range []bool{true, false} {
		domains, _, err := reduceInitialState(context.Background(), frame, &state, nil)
		if err != nil || slices.Contains(domains, "ruby-confirmation") != want {
			t.Fatalf("domains=%v err=%v want wake=%t", domains, err, want)
		}
	}
	state.Session.Generation++
	domains, _, err := reduceInitialState(context.Background(), frame, &state, nil)
	if err != nil || !slices.Contains(domains, "ruby-confirmation") {
		t.Fatalf("new generation domains=%v err=%v", domains, err)
	}
	frame.Payload = json.RawMessage(`{"opt":{"CC2T":null}}`)
	domains, _, err = reduceInitialState(context.Background(), frame, &state, nil)
	if err != nil || !slices.Contains(domains, "ruby-confirmation") || state.Player.RubyConfirmation.Known {
		t.Fatalf("unknown domains=%v err=%v", domains, err)
	}
}

func TestRubyConfirmationFreshUnchangedObservationWakesHeldOccurrence(t *testing.T) {
	rejectedAt := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	for _, opcode := range []string{"opt", "gbd"} {
		t.Run(opcode, func(t *testing.T) {
			state := State.NewGameState()
			state.Session.Generation = 1
			state.Player.RubyConfirmation = State.RubyConfirmationState{Known: true, Amount: -1, Generation: 1, ObservedAt: rejectedAt.Add(-time.Second)}
			state.EventScores.Inventory.GlobalEffects = map[int64]State.GlobalEffectAvailability{2: {GlobalEffectID: 2, EndsAt: rejectedAt.Add(time.Hour)}}
			state.EventScores.Inventory.GlobalEffectPurchases = map[int64]State.GlobalEffectPurchaseRecord{2: {GlobalEffectID: 2, OccurrenceEndsAt: rejectedAt.Add(time.Hour), Outcome: State.GlobalEffectPurchaseConfirmationRequired, ResultObservedAt: rejectedAt}}
			reducer := reduceRubyConfirmation
			payload := json.RawMessage(`{"CC2T":-1}`)
			if opcode == "gbd" {
				reducer = reduceInitialState
				payload = json.RawMessage(`{"opt":{"CC2T":-1}}`)
			}
			for _, tc := range []struct {
				at   time.Time
				wake bool
			}{{rejectedAt.Add(-time.Nanosecond), false}, {rejectedAt, false}, {rejectedAt.Add(time.Nanosecond), true}} {
				domains, _, err := reducer(t.Context(), Protocol.Frame{Opcode: opcode, Payload: payload, ReceivedAt: tc.at}, &state, nil)
				if err != nil || slices.Contains(domains, "ruby-confirmation") != tc.wake || !state.Player.RubyConfirmation.ObservedAt.Equal(tc.at) {
					t.Fatalf("domains=%v setting=%+v err=%v", domains, state.Player.RubyConfirmation, err)
				}
			}
		})
	}
}

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

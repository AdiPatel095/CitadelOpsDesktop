package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestResearchReducerRequiresValidCompleteBR(t *testing.T) {
	now := time.Now().UTC()
	zero := 0
	for _, tc := range []struct {
		name, payload          string
		wantChanged, wantError bool
	}{
		{"complete array", `{"BR":[271,272,275]}`, true, false},
		{"empty completed set", `{"BR":[]}`, true, false},
		{"missing BR", `{}`, false, false},
		{"malformed BR", `{"BR":{"271":true}}`, false, true},
		{"invalid ID", `{"BR":[0]}`, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			state := State.NewGameState()
			state.Session.Generation = 7
			frame := Protocol.Frame{Opcode: "rei", Direction: Protocol.DirectionInbound, ResponseCode: &zero, ReceivedAt: now, Payload: json.RawMessage(tc.payload)}
			_, changed, err := reduceResearch(t.Context(), frame, &state, nil)
			if changed != tc.wantChanged || (err != nil) != tc.wantError {
				t.Fatalf("changed=%v err=%v", changed, err)
			}
			if tc.wantChanged && (state.Research.ObservedAt != now || state.Research.Generation != 7) {
				t.Fatalf("research observation=%#v", state.Research)
			}
			if !tc.wantChanged && !state.Research.ObservedAt.IsZero() {
				t.Fatalf("invalid BR certified research: %#v", state.Research)
			}
		})
	}
}

func TestSubscriptionReducerTracksFreshEmptySnapshot(t *testing.T) {
	now := time.Now().UTC()
	zero := 0
	state := State.NewGameState()
	state.Session.Generation = 7
	frame := Protocol.Frame{Opcode: "sie", Direction: Protocol.DirectionInbound, ResponseCode: &zero, ReceivedAt: now, Payload: json.RawMessage(`{"SP":[]}`)}
	_, changed, err := reduceSubscriptions(t.Context(), frame, &state, nil)
	if err != nil || !changed || state.SubscriptionsObservedAt != now || state.SubscriptionsGeneration != 7 {
		t.Fatalf("changed=%v err=%v state=%#v", changed, err, state)
	}
}

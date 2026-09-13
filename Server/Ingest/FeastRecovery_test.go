package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestPendingFeastNeedsTwoSpacedCurrentConnectionInactiveReplies(t *testing.T) {
	start := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 7
	state.Session.ChangedAt = start
	state.Market.FeastPurchasePending = true
	state.Market.FeastPurchasePendingSince = start
	state.Market.FeastPurchaseExpectedExpiresAt = start.Add(6 * time.Hour)
	receive := func(seconds int, token, payload string) {
		t.Helper()
		zero := 0
		_, _, err := reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &zero, ResponseToken: token, ReceivedAt: start.Add(time.Duration(seconds) * time.Second), Payload: json.RawMessage(payload)}, &state, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	inactive := `{"BO":[],"bfs":{"T":-1,"RT":0}}`
	receive(5, "early", inactive)
	receive(30, "", inactive)
	if !state.Market.FeastPurchaseInactiveObservedAt.IsZero() {
		t.Fatal("early or unsolicited reply counted")
	}
	receive(31, "first", inactive)
	receive(40, "too-soon", inactive)
	receive(61, "first", inactive)
	if !state.Market.FeastPurchasePending {
		t.Fatal("duplicate/too-close reply released pending spend")
	}
	// Restarted state retains evidence, but a new connection must start again.
	raw, err := json.Marshal(state.Market)
	if err != nil {
		t.Fatal(err)
	}
	var restored State.MarketState
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatal(err)
	}
	state.Market = restored
	state.Session.ConnectionGeneration = 8
	state.Session.ChangedAt = start.Add(62 * time.Second)
	receive(63, "new-connection", inactive)
	if !state.Market.FeastPurchasePending {
		t.Fatal("mixed connections released pending spend")
	}
	receive(94, "confirmed", inactive)
	if state.Market.FeastPurchasePending || !state.Market.FeastPurchaseInactiveObservedAt.IsZero() {
		t.Fatal("confirmed inactive feast did not recover")
	}
	if !state.Market.FeastLastPurchaseAt.IsZero() {
		t.Fatal("failed purchase recorded as successful")
	}
}

func TestPendingFeastMissingMalformedOrStaleRepliesCannotConfirmInactivity(t *testing.T) {
	for _, tc := range []struct {
		name, payload string
		generation    uint64
		stale         bool
	}{
		{"omitted", `{"BO":[]}`, 1, false},
		{"null", `{"BO":[],"bfs":null}`, 1, false},
		{"malformed", `{"BO":[],"bfs":{"T":-1}}`, 1, false},
		{"incoherent", `{"BO":[],"bfs":{"T":-1,"RT":3600}}`, 1, false},
		{"unknown connection", `{"BO":[],"bfs":{"T":-1,"RT":0}}`, 0, false},
		{"pre-session reply", `{"BO":[],"bfs":{"T":-1,"RT":0}}`, 1, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			now := time.Now().UTC()
			state := State.NewGameState()
			state.Session.ConnectionGeneration = tc.generation
			state.Session.ChangedAt = now.Add(-time.Hour)
			if tc.stale {
				state.Session.ChangedAt = now.Add(time.Second)
			}
			state.Market.FeastPurchasePending = true
			state.Market.FeastPurchasePendingSince = now.Add(-2 * time.Minute)
			state.Market.FeastPurchaseExpectedExpiresAt = now.Add(6 * time.Hour)
			state.Market.FeastPurchaseInactiveObservedAt = now.Add(-time.Minute)
			state.Market.FeastPurchaseInactiveResponseToken = "first"
			state.Market.FeastPurchaseInactiveGeneration = 1
			zero := 0
			_, _, _ = reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", ResponseCode: &zero, ResponseToken: "second", ReceivedAt: now, Payload: json.RawMessage(tc.payload)}, &state, nil)
			if !state.Market.FeastPurchasePending {
				t.Fatal("invalid evidence released pending spend")
			}
		})
	}
}

func TestPendingFeastActiveEvidenceResetsInactiveConfirmation(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.ConnectionGeneration = 1
	state.Market.FeastPurchasePending = true
	state.Market.FeastPurchasePendingSince = now.Add(-time.Minute)
	state.Market.FeastPurchaseExpectedExpiresAt = now.Add(6 * time.Hour)
	state.Market.FeastPurchaseInactiveObservedAt = now.Add(-30 * time.Second)
	state.Market.FeastPurchaseInactiveGeneration = 1
	state.Market.FeastPurchaseInactiveResponseToken = "first"
	// An active but unextended/different feast is ambiguous, not a failed spend.
	zero := 0
	_, _, err := reduceMarketBooster(t.Context(), Protocol.Frame{Opcode: "boi", ResponseCode: &zero, ResponseToken: "active", ReceivedAt: now, Payload: json.RawMessage(`{"BO":[],"bfs":{"T":1,"RT":3600}}`)}, &state, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !state.Market.FeastPurchasePending || !state.Market.FeastPurchaseInactiveObservedAt.IsZero() {
		t.Fatal("conflicting active evidence did not reset confirmation")
	}
}

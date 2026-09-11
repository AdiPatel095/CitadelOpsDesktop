package Ingest

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestMarketFeastDecoderAcceptsOnlyCoherentActiveAndInactiveStates(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	testCases := []struct {
		name         string
		payload      json.RawMessage
		wantID       int64
		wantDuration int
		wantActive   bool
	}{
		{name: "direct inactive", payload: json.RawMessage(`{"T":-1,"RT":0}`)},
		{name: "nested inactive", payload: json.RawMessage(`{"bfs":{"T":-1,"RT":0}}`)},
		{name: "direct active feast zero", payload: json.RawMessage(`{"T":0,"RT":7200}`), wantDuration: 7200, wantActive: true},
		{name: "nested active", payload: json.RawMessage(`{"bfs":{"T":8,"RT":14400}}`), wantID: 8, wantDuration: 14400, wantActive: true},
		{name: "wire integer strings", payload: json.RawMessage(`{"T":"3","RT":"60"}`), wantID: 3, wantDuration: 60, wantActive: true},
		{name: "integral decimals", payload: json.RawMessage(`{"T":3.0,"RT":60.000}`), wantID: 3, wantDuration: 60, wantActive: true},
		{name: "integral exponent", payload: json.RawMessage(`{"T":3e0,"RT":6e1}`), wantID: 3, wantDuration: 60, wantActive: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			feast, err := marketFeastFromRaw(testCase.payload, observedAt)
			if err != nil {
				t.Fatal(err)
			}
			if feast.ID != testCase.wantID || feast.RemainingSec != testCase.wantDuration ||
				!feast.ObservedAt.Equal(observedAt) || feast.ActiveAt(observedAt) != testCase.wantActive {
				t.Fatalf("decoded feast = %+v", feast)
			}
			if testCase.wantActive {
				wantExpiry := observedAt.Add(time.Duration(testCase.wantDuration) * time.Second)
				if !feast.ExpiresAt.Equal(wantExpiry) {
					t.Fatalf("expiry = %s, want %s", feast.ExpiresAt, wantExpiry)
				}
			} else if !feast.ExpiresAt.IsZero() {
				t.Fatalf("inactive feast has expiry %s", feast.ExpiresAt)
			}
			if !feast.FreshAt(observedAt, observedAt, time.Minute) {
				t.Fatalf("decoded feast is not authoritative: %+v", feast)
			}
		})
	}
}

func TestMarketFeastDecoderRejectsMalformedAndIncoherentStates(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	testCases := []struct {
		name       string
		payload    json.RawMessage
		observedAt time.Time
	}{
		{name: "empty payload", observedAt: observedAt},
		{name: "null payload", payload: json.RawMessage(`null`), observedAt: observedAt},
		{name: "malformed json", payload: json.RawMessage(`{`), observedAt: observedAt},
		{name: "array payload", payload: json.RawMessage(`[]`), observedAt: observedAt},
		{name: "empty object", payload: json.RawMessage(`{}`), observedAt: observedAt},
		{name: "nested null", payload: json.RawMessage(`{"bfs":null}`), observedAt: observedAt},
		{name: "nested array", payload: json.RawMessage(`{"bfs":[]}`), observedAt: observedAt},
		{name: "missing type", payload: json.RawMessage(`{"RT":60}`), observedAt: observedAt},
		{name: "missing duration", payload: json.RawMessage(`{"T":1}`), observedAt: observedAt},
		{name: "null type", payload: json.RawMessage(`{"T":null,"RT":60}`), observedAt: observedAt},
		{name: "null duration", payload: json.RawMessage(`{"T":1,"RT":null}`), observedAt: observedAt},
		{name: "bad type", payload: json.RawMessage(`{"T":"festival","RT":60}`), observedAt: observedAt},
		{name: "bad duration", payload: json.RawMessage(`{"T":1,"RT":"later"}`), observedAt: observedAt},
		{name: "fractional type", payload: json.RawMessage(`{"T":1.5,"RT":60}`), observedAt: observedAt},
		{name: "fractional duration", payload: json.RawMessage(`{"T":1,"RT":60.5}`), observedAt: observedAt},
		{name: "fractional string", payload: json.RawMessage(`{"T":"1/1","RT":60}`), observedAt: observedAt},
		{name: "type underflow", payload: json.RawMessage(`{"T":"-9223372036854775809","RT":60}`), observedAt: observedAt},
		{name: "type overflow", payload: json.RawMessage(`{"T":"9223372036854775808","RT":60}`), observedAt: observedAt},
		{name: "unsupported negative sentinel", payload: json.RawMessage(`{"T":-2,"RT":0}`), observedAt: observedAt},
		{name: "inactive with duration", payload: json.RawMessage(`{"T":-1,"RT":60}`), observedAt: observedAt},
		{name: "active with zero duration", payload: json.RawMessage(`{"T":0,"RT":0}`), observedAt: observedAt},
		{name: "active with negative duration", payload: json.RawMessage(`{"T":0,"RT":-1}`), observedAt: observedAt},
		{name: "duration overflow", payload: json.RawMessage(`{"T":0,"RT":9223372037}`), observedAt: observedAt},
		{name: "missing observation timestamp", payload: json.RawMessage(`{"T":0,"RT":60}`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if feast, err := marketFeastFromRaw(testCase.payload, testCase.observedAt); err == nil {
				t.Fatalf("malformed payload decoded as %+v", feast)
			}
		})
	}
}

func TestMarketBoosterMissingOrNullBFSDoesNotAdvanceFeastAuthority(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	existingFeast := State.MarketFeastState{
		ID: 3, RemainingSec: 14400, ExpiresAt: observedAt.Add(4 * time.Hour), ObservedAt: observedAt,
	}
	lastPurchaseAt := observedAt.Add(-time.Hour)
	receivedAt := observedAt.Add(time.Minute)
	code := 0
	testCases := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "top-level missing", payload: json.RawMessage(`{"BO":[]}`)},
		{name: "top-level null", payload: json.RawMessage(`{"BO":[],"bfs":null}`)},
		{name: "top-level whitespace null", payload: json.RawMessage(`{"BO":[],"bfs": null }`)},
		{name: "nested missing", payload: json.RawMessage(`{"boi":{"BO":[]}}`)},
		{name: "nested null", payload: json.RawMessage(`{"boi":{"BO":[],"bfs":null}}`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.Feast = existingFeast
			gameState.Market.FeastLastPurchaseAt = lastPurchaseAt
			_, changed, err := reduceMarketBooster(t.Context(), Protocol.Frame{
				Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: receivedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("changed=%t err=%v", changed, err)
			}
			if gameState.Market.Feast != existingFeast {
				t.Fatalf("feast authority advanced: got %+v, want %+v", gameState.Market.Feast, existingFeast)
			}
			if !gameState.Market.FeastLastPurchaseAt.Equal(lastPurchaseAt) {
				t.Fatalf("BOI changed purchase time to %s", gameState.Market.FeastLastPurchaseAt)
			}
			if !gameState.Market.BoostersObservedAt.Equal(receivedAt) {
				t.Fatalf("booster observation = %s, want %s", gameState.Market.BoostersObservedAt, receivedAt)
			}
		})
	}
}

func TestMarketBoosterUpdatesFeastButNeverPurchaseTime(t *testing.T) {
	previousPurchaseAt := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	receivedAt := previousPurchaseAt.Add(time.Hour)
	code := 0
	testCases := []struct {
		name       string
		payload    json.RawMessage
		wantActive bool
	}{
		{name: "active", payload: json.RawMessage(`{"BO":[],"bfs":{"T":0,"RT":7200}}`), wantActive: true},
		{name: "inactive", payload: json.RawMessage(`{"BO":[],"bfs":{"T":-1,"RT":0}}`)},
		{name: "nested active", payload: json.RawMessage(`{"boi":{"BO":[],"bfs":{"T":8,"RT":60}}}`), wantActive: true},
		{name: "nested inactive", payload: json.RawMessage(`{"boi":{"BO":[],"bfs":{"T":-1,"RT":0}}}`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.FeastLastPurchaseAt = previousPurchaseAt
			_, changed, err := reduceMarketBooster(t.Context(), Protocol.Frame{
				Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: receivedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("changed=%t err=%v", changed, err)
			}
			if !gameState.Market.Feast.ObservedAt.Equal(receivedAt) ||
				gameState.Market.Feast.ActiveAt(receivedAt) != testCase.wantActive {
				t.Fatalf("BOI feast = %+v", gameState.Market.Feast)
			}
			if !gameState.Market.FeastLastPurchaseAt.Equal(previousPurchaseAt) {
				t.Fatalf("BOI changed purchase time to %s", gameState.Market.FeastLastPurchaseAt)
			}
		})
	}
}

func TestMalformedBOIFeastRejectsMarketMutationAtomically(t *testing.T) {
	initial := State.NewGameState()
	observedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	initial.Market.Boosters[11] = State.MarketBoosterState{ID: 11, Level: 21, Permanent: true}
	initial.Market.CaravanLevel = 21
	initial.Market.CaravanLevelLoaded = true
	initial.Market.BoostersObservedAt = observedAt
	initial.Market.Feast = State.MarketFeastState{ObservedAt: observedAt}
	initial.Market.FeastLastPurchaseAt = observedAt.Add(-time.Hour)
	store := State.NewStore(initial)
	before := store.Snapshot().Market
	registry := NewRegistry()
	if err := registry.RegisterComponents("boi", State.Components(State.ComponentMarket), reduceMarketBooster); err != nil {
		t.Fatal(err)
	}
	pipeline := NewPipeline(store, nil, registry)
	code := 0
	_, err := pipeline.HandleFrame(t.Context(), Protocol.Frame{
		Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt.Add(time.Minute),
		Payload:    json.RawMessage(`{"BO":[{"ID":11,"L":99,"RT":60}],"bfs":{"T":0,"RT":0}}`),
	})
	if err == nil || !strings.Contains(err.Error(), "decode market feast from boosters") {
		t.Fatalf("malformed BOI error = %v", err)
	}
	if after := store.Snapshot().Market; !reflect.DeepEqual(after, before) {
		t.Fatalf("malformed BOI partially mutated Market: before=%+v after=%+v", before, after)
	}
}

func TestDirectMarketFeastRecordsSuccessfulAcknowledgement(t *testing.T) {
	previousPurchaseAt := time.Date(2026, time.September, 8, 18, 0, 0, 0, time.UTC)
	receivedAt := previousPurchaseAt.Add(time.Hour)
	code := 0
	testCases := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "active direct", payload: json.RawMessage(`{"T":0,"RT":7200}`)},
		{name: "active nested", payload: json.RawMessage(`{"bfs":{"T":8,"RT":60}}`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.Feast = State.MarketFeastState{
				ID: 3, RemainingSec: 60, ObservedAt: previousPurchaseAt,
				ExpiresAt: previousPurchaseAt.Add(time.Minute),
			}
			gameState.Market.FeastLastPurchaseAt = previousPurchaseAt
			_, changed, err := reduceMarketFeast(t.Context(), Protocol.Frame{
				Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: receivedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("changed=%t err=%v", changed, err)
			}
			if !gameState.Market.Feast.ActiveAt(receivedAt) ||
				!gameState.Market.Feast.ObservedAt.Equal(receivedAt) {
				t.Fatalf("direct BFS feast = %+v", gameState.Market.Feast)
			}
			if !gameState.Market.FeastLastPurchaseAt.Equal(receivedAt) {
				t.Fatalf("purchase time = %s, want %s", gameState.Market.FeastLastPurchaseAt, receivedAt)
			}
		})
	}
}

func TestDirectMarketFeastFailureOrMalformedPayloadPreservesState(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	code := 0
	rejectedCode := 55
	testCases := []struct {
		name         string
		payload      json.RawMessage
		responseCode *int
		wantError    bool
	}{
		{name: "missing result code", payload: json.RawMessage(`{"T":0,"RT":7200}`), wantError: true},
		{name: "empty success", responseCode: &code, wantError: true},
		{name: "null success", payload: json.RawMessage(`null`), responseCode: &code, wantError: true},
		{name: "malformed success", payload: json.RawMessage(`{"T":0,"RT":0}`), responseCode: &code, wantError: true},
		{name: "inactive success", payload: json.RawMessage(`{"T":-1,"RT":0}`), responseCode: &code, wantError: true},
		{name: "game rejection", payload: json.RawMessage(`{"T":0,"RT":7200}`), responseCode: &rejectedCode},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.Feast = State.MarketFeastState{ObservedAt: observedAt.Add(-time.Minute)}
			gameState.Market.FeastLastPurchaseAt = observedAt.Add(-time.Hour)
			before := gameState.Market
			_, changed, err := reduceMarketFeast(t.Context(), Protocol.Frame{
				Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: testCase.responseCode,
				ReceivedAt: observedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if (err != nil) != testCase.wantError || changed {
				t.Fatalf("changed=%t err=%v", changed, err)
			}
			if !reflect.DeepEqual(gameState.Market.Feast, before.Feast) {
				t.Fatalf("BFS feast authority = %+v, want %+v", gameState.Market.Feast, before.Feast)
			}
			if !gameState.Market.FeastLastPurchaseAt.Equal(before.FeastLastPurchaseAt) {
				t.Fatalf("purchase acknowledgement = %s, want %s", gameState.Market.FeastLastPurchaseAt, before.FeastLastPurchaseAt)
			}
		})
	}
}

func TestPendingFeastRequiresExpectedExtensionBeforeClearing(t *testing.T) {
	dispatchedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	expectedExpiry := dispatchedAt.Add(6 * time.Hour)
	code := 0
	newPendingState := func() State.GameState {
		state := State.NewGameState()
		state.Market.FeastPurchasePending = true
		state.Market.FeastPurchaseExpectedID = 0
		state.Market.FeastPurchasePendingSince = dispatchedAt
		state.Market.FeastPurchaseExpectedExpiresAt = expectedExpiry
		return state
	}

	state := newPendingState()
	_, changed, err := reduceMarketFeast(t.Context(), Protocol.Frame{
		Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: dispatchedAt.Add(time.Second), Payload: json.RawMessage(`{"T":0,"RT":60}`),
	}, &state, nil)
	if err == nil || changed || !state.Market.FeastPurchasePending {
		t.Fatalf("unchanged direct feast cleared pending: changed=%t err=%v market=%+v", changed, err, state.Market)
	}

	state = newPendingState()
	_, changed, err = reduceMarketBooster(t.Context(), Protocol.Frame{
		Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: dispatchedAt.Add(time.Second), Payload: json.RawMessage(`{"BO":[],"bfs":{"T":0,"RT":60}}`),
	}, &state, nil)
	if err != nil || !changed || !state.Market.FeastPurchasePending {
		t.Fatalf("unchanged BOI feast cleared pending: changed=%t err=%v market=%+v", changed, err, state.Market)
	}

	state = newPendingState()
	_, changed, err = reduceMarketBooster(t.Context(), Protocol.Frame{
		Opcode: "boi", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: dispatchedAt.Add(time.Second), Payload: json.RawMessage(`{"BO":[],"bfs":{"T":0,"RT":21600}}`),
	}, &state, nil)
	if err != nil || !changed || state.Market.FeastPurchasePending {
		t.Fatalf("confirmed BOI extension remained pending: changed=%t err=%v market=%+v", changed, err, state.Market)
	}
}

func TestPendingFeastIgnoresUnrelatedRejectedResponse(t *testing.T) {
	dispatchedAt := time.Date(2026, time.September, 8, 19, 0, 0, 0, time.UTC)
	rejectedCode := 55
	state := State.NewGameState()
	state.Market.FeastPurchasePending = true
	state.Market.FeastPurchaseExpectedID = 0
	state.Market.FeastPurchasePendingSince = dispatchedAt
	state.Market.FeastPurchaseExpectedExpiresAt = dispatchedAt.Add(6 * time.Hour)
	state.Market.FeastPurchaseOperationID = "expected-operation"
	state.Market.FeastPurchaseResponseToken = "expected-token"

	_, changed, err := reduceMarketFeast(t.Context(), Protocol.Frame{
		Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &rejectedCode,
		ReceivedAt: dispatchedAt.Add(time.Second), ResponseToken: "other-token",
	}, &state, nil)
	if err != nil || changed || !state.Market.FeastPurchasePending {
		t.Fatalf("unrelated rejection cleared pending: changed=%t err=%v market=%+v", changed, err, state.Market)
	}
	_, changed, err = reduceMarketFeast(t.Context(), Protocol.Frame{
		Opcode: "bfs", Direction: Protocol.DirectionInbound, ResponseCode: &rejectedCode,
		ReceivedAt: dispatchedAt.Add(2 * time.Second), ResponseToken: "expected-token",
	}, &state, nil)
	if err != nil || !changed || state.Market.FeastPurchasePending {
		t.Fatalf("matched rejection did not clear pending: changed=%t err=%v market=%+v", changed, err, state.Market)
	}
}

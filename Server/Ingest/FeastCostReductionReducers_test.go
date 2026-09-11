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

func TestFeastCostReductionReducerRegistered(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	if !registry.HasInbound("fce") {
		t.Fatal("FCE has no committed inbound reducer")
	}
}

func TestFeastCostReductionReducerAcceptsDirectAndNestedPayloads(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 16, 0, 0, 0, time.UTC)
	code := 0
	testCases := []struct {
		name    string
		payload json.RawMessage
		want    int
	}{
		{name: "direct", payload: json.RawMessage(`{"FRM":25}`), want: 25},
		{name: "nested numeric string", payload: json.RawMessage(`{"fce":{"FRM":"75"}}`), want: 75},
		{name: "zero", payload: json.RawMessage(`{"FRM":0}`), want: 0},
		{name: "upper bound", payload: json.RawMessage(`{"FRM":100}`), want: 100},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			domains, changed, err := reduceFeastCostReduction(t.Context(), Protocol.Frame{
				Opcode: "fce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed {
				t.Fatalf("FCE reduction: changed=%t err=%v", changed, err)
			}
			if !reflect.DeepEqual(domains, []string{"market"}) {
				t.Fatalf("FCE domains = %v", domains)
			}
			if gameState.Market.FeastCostReductionPercent != testCase.want ||
				!gameState.Market.FeastCostReductionObservedAt.Equal(observedAt) {
				t.Fatalf("FCE state = %+v", gameState.Market)
			}

			_, changed, err = reduceFeastCostReduction(t.Context(), Protocol.Frame{
				Opcode: "fce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || changed {
				t.Fatalf("identical FCE reduction: changed=%t err=%v", changed, err)
			}

			refreshedAt := observedAt.Add(time.Minute)
			_, changed, err = reduceFeastCostReduction(t.Context(), Protocol.Frame{
				Opcode: "fce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: refreshedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err != nil || !changed || !gameState.Market.FeastCostReductionObservedAt.Equal(refreshedAt) {
				t.Fatalf("refreshed FCE observation: changed=%t state=%+v err=%v", changed, gameState.Market, err)
			}
		})
	}
}

func TestFeastCostReductionReducerRejectsInvalidFRMWithoutMutation(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 16, 0, 0, 0, time.UTC)
	previousObservedAt := observedAt.Add(-time.Hour)
	code := 0
	testCases := []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "empty", payload: nil},
		{name: "invalid json", payload: json.RawMessage(`{`)},
		{name: "missing", payload: json.RawMessage(`{}`)},
		{name: "nested missing", payload: json.RawMessage(`{"fce":{}}`)},
		{name: "nested malformed", payload: json.RawMessage(`{"fce":25}`)},
		{name: "null", payload: json.RawMessage(`{"FRM":null}`)},
		{name: "not numeric", payload: json.RawMessage(`{"FRM":"discount"}`)},
		{name: "fractional", payload: json.RawMessage(`{"FRM":12.5}`)},
		{name: "negative", payload: json.RawMessage(`{"FRM":-1}`)},
		{name: "above one hundred", payload: json.RawMessage(`{"FRM":101}`)},
		{name: "non finite string", payload: json.RawMessage(`{"FRM":"NaN"}`)},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Market.FeastCostReductionPercent = 40
			gameState.Market.FeastCostReductionObservedAt = previousObservedAt
			before := gameState.Market

			domains, changed, err := reduceFeastCostReduction(t.Context(), Protocol.Frame{
				Opcode: "fce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: observedAt, Payload: testCase.payload,
			}, &gameState, nil)
			if err == nil || changed || len(domains) != 0 {
				t.Fatalf("invalid FCE reduction: domains=%v changed=%t err=%v", domains, changed, err)
			}
			if !strings.Contains(err.Error(), "feast cost reduction") {
				t.Fatalf("invalid FCE error = %v", err)
			}
			if !reflect.DeepEqual(gameState.Market, before) {
				t.Fatalf("invalid FCE mutated state: got=%+v want=%+v", gameState.Market, before)
			}
		})
	}
}

func TestFeastCostReductionReducerIgnoresUnsuccessfulResponse(t *testing.T) {
	observedAt := time.Date(2026, time.September, 8, 16, 0, 0, 0, time.UTC)
	code := 55
	gameState := State.NewGameState()
	gameState.Market.FeastCostReductionPercent = 20
	gameState.Market.FeastCostReductionObservedAt = observedAt.Add(-time.Hour)
	before := gameState.Market

	domains, changed, err := reduceFeastCostReduction(t.Context(), Protocol.Frame{
		Opcode: "fce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: observedAt, Payload: json.RawMessage(`{"FRM":75}`),
	}, &gameState, nil)
	if err != nil || changed || len(domains) != 0 {
		t.Fatalf("unsuccessful FCE reduction: domains=%v changed=%t err=%v", domains, changed, err)
	}
	if !reflect.DeepEqual(gameState.Market, before) {
		t.Fatalf("unsuccessful FCE mutated state: got=%+v want=%+v", gameState.Market, before)
	}
}

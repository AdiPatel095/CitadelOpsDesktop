package Ingest

import (
	"encoding/json"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestDailyAttackCountTracksStandaloneUpdatesAndServerReset(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)

	domains, changed, err := reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"AC":1000,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || !slices.Contains(domains, "attacks") {
		t.Fatalf("daily attack update: domains=%v changed=%t err=%v", domains, changed, err)
	}
	if got := gameState.DailyAttacks; got.Count != 1000 || got.ServerThreshold != 3500 || got.GrowthRate != 0.007 || !got.ObservedAt.Equal(observedAt) {
		t.Fatalf("daily attack state = %#v", got)
	}

	resetAt := observedAt.Add(12 * time.Hour)
	_, changed, err = reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: resetAt,
		Payload: json.RawMessage(`{"AC":0,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.DailyAttacks.Count != 0 || !gameState.DailyAttacks.ObservedAt.Equal(resetAt) {
		t.Fatalf("daily attack reset: state=%#v changed=%t err=%v", gameState.DailyAttacks, changed, err)
	}
}

func TestInitialStateAppliesEmbeddedDailyAttackCount(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)
	domains, changed, err := reduceInitialState(t.Context(), Protocol.Frame{
		Opcode: "gbd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"gai":{"AC":42,"ACTH":3500,"ACGR":0.007}}`),
	}, &gameState, nil)
	if err != nil || !changed || !slices.Contains(domains, "attacks") {
		t.Fatalf("embedded daily attack count: domains=%v changed=%t err=%v", domains, changed, err)
	}
	if gameState.DailyAttacks.Count != 42 || gameState.DailyAttacks.ServerThreshold != 3500 {
		t.Fatalf("embedded daily attack state = %#v", gameState.DailyAttacks)
	}
}

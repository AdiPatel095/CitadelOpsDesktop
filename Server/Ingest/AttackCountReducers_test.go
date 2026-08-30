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
	if got := gameState.DailyAttacks; got.Count != 1000 || got.ServerThreshold != 3500 || got.GrowthRate != 0.007 ||
		!got.SessionStartedAt.IsZero() || !got.ObservedAt.Equal(observedAt) {
		t.Fatalf("daily attack state = %#v", got)
	}

	updatedAt := observedAt.Add(6 * time.Hour)
	_, changed, err = reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: updatedAt,
		Payload: json.RawMessage(`{"AC":1200,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || !gameState.DailyAttacks.SessionStartedAt.IsZero() {
		t.Fatalf("daily attack increase invented a session boundary: state=%#v changed=%t err=%v", gameState.DailyAttacks, changed, err)
	}

	resetAt := observedAt.Add(12 * time.Hour)
	_, changed, err = reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: resetAt,
		Payload: json.RawMessage(`{"AC":0,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.DailyAttacks.Count != 0 ||
		!gameState.DailyAttacks.SessionStartedAt.Equal(resetAt) || !gameState.DailyAttacks.ObservedAt.Equal(resetAt) {
		t.Fatalf("daily attack reset: state=%#v changed=%t err=%v", gameState.DailyAttacks, changed, err)
	}

	postResetAt := resetAt.Add(time.Hour)
	_, changed, err = reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: postResetAt,
		Payload: json.RawMessage(`{"AC":25,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || !gameState.DailyAttacks.SessionStartedAt.Equal(resetAt) {
		t.Fatalf("post-reset increase changed the session boundary: state=%#v changed=%t err=%v", gameState.DailyAttacks, changed, err)
	}
}

func TestInitialZeroDailyAttackCountEstablishesCleanSessionBoundary(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 21, 3, 0, 0, 0, time.UTC)

	_, changed, err := reduceDailyAttackCount(t.Context(), Protocol.Frame{
		Opcode: "gai", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"AC":0,"ACTH":3500,"ACGR":0.007}`),
	}, &gameState, nil)
	if err != nil || !changed || !gameState.DailyAttacks.SessionStartedAt.Equal(observedAt) {
		t.Fatalf("initial zero daily attack session: state=%#v changed=%t err=%v", gameState.DailyAttacks, changed, err)
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

package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceDailyAttackCount(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyDailyAttackCount(frame.Payload, frame.ReceivedAt, gameState)
	return []string{"attacks"}, changed, err
}

func applyDailyAttackCount(raw json.RawMessage, observedAt time.Time, gameState *State.GameState) (bool, error) {
	var payload struct {
		Count           *wireInt64  `json:"AC"`
		ServerThreshold wireInt64   `json:"ACTH"`
		GrowthRate      wireFloat64 `json:"ACGR"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode daily attack count: %w", err)
	}
	if payload.Count == nil {
		return false, fmt.Errorf("decode daily attack count: AC is missing")
	}
	count := int64(*payload.Count)
	threshold := int64(payload.ServerThreshold)
	if count < 0 || threshold < 0 {
		return false, fmt.Errorf("decode daily attack count: negative count or threshold")
	}
	next := State.DailyAttackState{
		Count: count, ServerThreshold: threshold, GrowthRate: float64(payload.GrowthRate), ObservedAt: observedAt.UTC(),
	}
	if reflect.DeepEqual(gameState.DailyAttacks, next) {
		return false, nil
	}
	gameState.DailyAttacks = next
	return true, nil
}

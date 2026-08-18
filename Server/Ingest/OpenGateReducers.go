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

func reduceOpenGate(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		CastleID     wireInt64 `json:"CID"`
		RemainingSec wireInt64 `json:"RPT"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode open-gate response: %w", err)
	}
	castleID := State.CastleID(payload.CastleID)
	castle, found := gameState.Castles[castleID]
	if castleID <= 0 || !found {
		castleID, castle, found = focusedCastle(gameState)
	}
	if castleID <= 0 || !found {
		return nil, false, nil
	}
	castle, found = gameState.MutableCastleParts(castleID, State.CastlePartDefense)
	if !found {
		return nil, false, nil
	}
	before := castle.Defense.OpenGateUntil
	castle.Defense.OpenGateUntil = nil
	if payload.RemainingSec > 0 {
		until := defenseObservedAt(frame.ReceivedAt).Add(time.Duration(payload.RemainingSec) * time.Second)
		castle.Defense.OpenGateUntil = &until
	}
	if reflect.DeepEqual(before, castle.Defense.OpenGateUntil) {
		return nil, false, nil
	}
	gameState.SetCastleParts(castleID, castle, State.CastlePartDefense)
	return []string{"castles", "defense", "khan"}, true, nil
}

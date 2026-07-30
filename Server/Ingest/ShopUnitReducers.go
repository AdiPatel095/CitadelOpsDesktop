package Ingest

import (
	"context"
	"encoding/json"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceShopFocusedUnits(
	ctx context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root struct {
		Units     json.RawMessage `json:"gui"`
		Resources json.RawMessage `json:"grc"`
	}
	if err := json.Unmarshal(frame.Payload, &root); err != nil ||
		len(root.Units) == 0 || len(root.Resources) == 0 {
		return nil, false, nil
	}
	var resources struct {
		CastleID wireInt64 `json:"AID"`
	}
	if err := json.Unmarshal(root.Resources, &resources); err != nil || resources.CastleID <= 0 {
		return nil, false, nil
	}
	focusedCastleID, _, focused := focusedCastle(gameState)
	if !focused || focusedCastleID != State.CastleID(resources.CastleID) {
		return nil, false, nil
	}
	return reduceFocusedUnits(ctx, frame, gameState, gameData)
}

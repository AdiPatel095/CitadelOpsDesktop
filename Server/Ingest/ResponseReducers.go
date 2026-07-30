package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceResponseResources(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, nil
	}
	changed := false
	if raw := root["gcu"]; len(raw) > 0 {
		updated, err := applyPlayerResources(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["sce"]; len(raw) > 0 {
		updated, err := applyPlayerCurrencies(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	castleResources := root["grc"]
	if len(castleResources) == 0 && strings.EqualFold(frame.Opcode, "grc") {
		castleResources = frame.Payload
	}
	if len(castleResources) > 0 {
		updated, err := applyCastleResourceUpdate(castleResources, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	production := root["gpa"]
	if len(production) == 0 && frame.Opcode == "gpa" {
		production = frame.Payload
	}
	if len(production) > 0 {
		updated, err := applyFocusedCastleProduction(production, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	return []string{"player", "castles", "resources", "currencies"}, changed, nil
}

func applyCastleResourceUpdate(
	raw json.RawMessage,
	gameState *State.GameState,
	gameData *GameData.Store,
) (bool, error) {
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return false, fmt.Errorf("decode castle resource update: %w", err)
	}
	castleID := State.CastleID(rawInteger(values["AID"]))
	castle, exists := gameState.Castles[castleID]
	if !exists || castleID <= 0 {
		var focused bool
		castleID, castle, focused = focusedCastle(gameState)
		if !focused {
			return false, nil
		}
	}
	ensureCastleMaps(&castle)
	before := copyResourceBalances(castle.Resources)
	applyCastleResourceValues(values, &castle, gameData)
	if reflect.DeepEqual(before, castle.Resources) {
		return false, nil
	}
	gameState.Castles[castleID] = castle
	return true, nil
}

func applyFocusedCastleProduction(
	raw json.RawMessage,
	gameState *State.GameState,
	gameData *GameData.Store,
) (bool, error) {
	castleID, castle, focused := focusedCastle(gameState)
	if !focused {
		return false, nil
	}
	ensureCastleMaps(&castle)
	before := copyResourceBalances(castle.Resources)
	if err := applyCastleProductionValues(raw, &castle, gameData); err != nil {
		return false, err
	}
	if reflect.DeepEqual(before, castle.Resources) {
		return false, nil
	}
	gameState.Castles[castleID] = castle
	return true, nil
}

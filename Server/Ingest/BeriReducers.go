package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceBeriCapacity(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode Berimond capacity: %w", err)
	}
	available, sourceID, byUnit, found := beriCapacity(root, 0)
	if !found {
		return nil, false, nil
	}
	next := gameState.Beri
	next.AvailableTroops = available
	next.TroopsByUnit = byUnit
	next.ParsedSourceID = State.CastleID(sourceID)
	next.ObservedAt = frame.ReceivedAt
	if reflect.DeepEqual(gameState.Beri, next) {
		return nil, false, nil
	}
	gameState.Beri = next
	return []string{"beri", "units"}, true, nil
}

func beriCapacity(root map[string]json.RawMessage, depth int) (int64, int64, map[State.UnitID]int64, bool) {
	byUnit := map[State.UnitID]int64{}
	sourceID, _ := rawInt64(root["SCID"])
	available := int64(0)
	found := false
	for _, key := range []string{"FUC", "N", "AMT", "CNT", "TU", "C", "FC", "AMOUNT"} {
		if value, exists := rawInt64(root[key]); exists && value >= 0 {
			available = value
			found = true
			break
		}
	}
	for _, key := range []string{"A", "TU"} {
		rows, ok := decodeRows(root[key])
		if !ok {
			continue
		}
		for _, row := range rows {
			unitID, amount := rowInt(row, 0), rowInt(row, 1)
			if unitID <= 0 || amount < 0 {
				continue
			}
			byUnit[State.UnitID(unitID)] += amount
			if !found || byUnit[State.UnitID(unitID)] > available {
				available = byUnit[State.UnitID(unitID)]
			}
			found = true
		}
	}
	if found || depth > 0 {
		return available, sourceID, byUnit, found
	}
	for _, raw := range root {
		var nested map[string]json.RawMessage
		if json.Unmarshal(raw, &nested) != nil {
			continue
		}
		value, nestedSourceID, nestedUnits, nestedFound := beriCapacity(nested, depth+1)
		if !nestedFound {
			continue
		}
		if sourceID <= 0 {
			sourceID = nestedSourceID
		}
		return value, sourceID, nestedUnits, true
	}
	return 0, sourceID, byUnit, false
}

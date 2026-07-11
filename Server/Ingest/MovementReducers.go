package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func newMovementReducer(authoritative bool) Reducer {
	var snapshotMu sync.Mutex
	var lastSnapshotFrame time.Time
	return func(
		_ context.Context,
		frame Protocol.Frame,
		gameState *State.GameState,
		_ *GameData.Store,
	) ([]string, bool, error) {
		if !frameSucceeded(frame) || len(frame.Payload) == 0 {
			return nil, false, nil
		}
		items, fullSnapshot, err := movementItems(frame.Payload)
		if err != nil {
			return nil, false, err
		}
		if authoritative && !fullSnapshot {
			return nil, false, fmt.Errorf("gam response does not contain a movement array")
		}
		before := gameState.Movements
		next := make(map[State.MovementID]State.MovementState, len(before)+len(items))
		for id, movement := range before {
			next[id] = movement
		}
		if authoritative {
			snapshotMu.Lock()
			reset := lastSnapshotFrame.IsZero() || frame.ReceivedAt.Sub(lastSnapshotFrame) > 250*time.Millisecond
			lastSnapshotFrame = frame.ReceivedAt
			snapshotMu.Unlock()
			if reset || len(items) == 0 {
				next = make(map[State.MovementID]State.MovementState, len(items))
			}
		}
		for _, raw := range items {
			movement, ok := parseMovement(raw, frame.ReceivedAt)
			if !ok {
				continue
			}
			next[movement.ID] = movement
		}
		if reflect.DeepEqual(before, next) {
			return nil, false, nil
		}
		gameState.Movements = next
		return []string{"movements", "commanders"}, true, nil
	}
}

func reduceMovementRemoval(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ID wireInt64 `json:"MID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode movement removal: %w", err)
	}
	id := State.MovementID(payload.ID)
	if _, exists := gameState.Movements[id]; !exists {
		return nil, false, nil
	}
	delete(gameState.Movements, id)
	return []string{"movements", "commanders"}, true, nil
}

func movementItems(raw json.RawMessage) ([]json.RawMessage, bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, false, fmt.Errorf("decode movements: %w", err)
	}
	if rawItems, exists := root["M"]; exists {
		var items []json.RawMessage
		if json.Unmarshal(rawItems, &items) == nil {
			return items, true, nil
		}
		var movement map[string]json.RawMessage
		if json.Unmarshal(rawItems, &movement) == nil {
			return []json.RawMessage{raw}, false, nil
		}
	}
	for _, wrapperName := range []string{"A", "AAM"} {
		wrapper, exists := root[wrapperName]
		if !exists {
			continue
		}
		var movement map[string]json.RawMessage
		if json.Unmarshal(wrapper, &movement) == nil {
			return []json.RawMessage{wrapper}, false, nil
		}
	}
	return nil, false, nil
}

func parseMovement(raw json.RawMessage, observedAt time.Time) (State.MovementState, bool) {
	var item map[string]json.RawMessage
	if json.Unmarshal(raw, &item) != nil {
		return State.MovementState{}, false
	}
	var details struct {
		ID        wireInt64         `json:"MID"`
		Progress  int               `json:"PT"`
		Travel    int               `json:"TT"`
		Direction int               `json:"D"`
		TypeID    int               `json:"T"`
		KingdomID wireInt64         `json:"KID"`
		OwnerID   wireInt64         `json:"OID"`
		TargetID  wireInt64         `json:"TID"`
		Source    []json.RawMessage `json:"SA"`
		Target    []json.RawMessage `json:"TA"`
	}
	if json.Unmarshal(item["M"], &details) != nil || details.ID <= 0 {
		return State.MovementState{}, false
	}
	movement := State.MovementState{
		ID: State.MovementID(details.ID), TypeID: details.TypeID, Direction: details.Direction,
		OwnerPlayerID: State.PlayerID(details.OwnerID), TargetPlayerID: State.PlayerID(details.TargetID),
		KingdomID: State.KingdomID(details.KingdomID), TravelSeconds: details.Travel,
		ProgressSeconds: details.Progress, Units: map[State.UnitID]int64{},
	}
	if len(details.Source) > 4 {
		movement.SourceX = int(rowInt(details.Source, 1))
		movement.SourceY = int(rowInt(details.Source, 2))
		movement.SourceCastleID = State.CastleID(rowInt(details.Source, 3))
		if movement.OwnerPlayerID == 0 {
			movement.OwnerPlayerID = State.PlayerID(rowInt(details.Source, 4))
		}
	}
	if len(details.Target) > 4 {
		movement.TargetX = int(rowInt(details.Target, 1))
		movement.TargetY = int(rowInt(details.Target, 2))
		movement.TargetCastleID = State.CastleID(rowInt(details.Target, 3))
		if targetPlayerID := rowInt(details.Target, 4); targetPlayerID != 0 {
			movement.TargetPlayerID = State.PlayerID(targetPlayerID)
		}
	}
	var unitMovement struct {
		Leader struct {
			ID *wireInt64 `json:"ID"`
		} `json:"L"`
	}
	if json.Unmarshal(item["UM"], &unitMovement) == nil && unitMovement.Leader.ID != nil && *unitMovement.Leader.ID >= 0 {
		movement.CommanderID = State.CommanderID(*unitMovement.Leader.ID)
	}
	for id, amount := range decodeUnitCounts(item["A"]) {
		movement.Units[State.UnitID(id)] = amount
	}
	remaining := details.Travel - details.Progress
	if remaining < 0 {
		remaining = 0
	}
	completion := observedAt.Add(time.Duration(remaining) * time.Second)
	if details.Direction == 0 {
		movement.ArrivesAt = &completion
	} else {
		movement.ReturnsAt = &completion
	}
	return movement, true
}

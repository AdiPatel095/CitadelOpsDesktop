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

func reduceDefenseContext(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		Identity      []json.RawMessage `json:"A"`
		Wall          json.RawMessage   `json:"dfw"`
		Keep          json.RawMessage   `json:"dfk"`
		Moat          json.RawMessage   `json:"dfm"`
		Units         json.RawMessage   `json:"gui"`
		Leader        json.RawMessage   `json:"L"`
		GateDefense   *wireFloat64      `json:"GD"`
		MeleeDefense  *wireFloat64      `json:"MDS"`
		RangedDefense *wireFloat64      `json:"RDS"`
		HDWL          *wireInt64        `json:"HDWL"`
		RangedUnitIDs json.RawMessage   `json:"PR"`
		MeleeUnitIDs  json.RawMessage   `json:"PM"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode defense context: %w", err)
	}
	if len(payload.Identity) < 4 {
		return nil, false, fmt.Errorf("defense context has no castle identity")
	}
	castleID := State.CastleID(rowInt(payload.Identity, 3))
	castle, found := gameState.MutableCastleParts(castleID, State.CastlePartDefense)
	if castleID <= 0 || !found {
		return nil, false, nil
	}
	before := castle
	observedAt := defenseObservedAt(frame.ReceivedAt)
	ensureDefenseSlices(&castle.Defense)
	hasDefense := false
	if len(payload.Wall) > 0 {
		wall, err := parseDefenseWall(payload.Wall, observedAt)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.Wall = wall
		hasDefense = true
	}
	if len(payload.Keep) > 0 {
		keep, err := parseDefenseKeep(payload.Keep, observedAt)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.Keep = keep
		hasDefense = true
	}
	if len(payload.Moat) > 0 {
		moat, err := parseDefenseMoat(payload.Moat, observedAt)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.Moat = moat
		hasDefense = true
	}
	if payload.GateDefense != nil {
		castle.Defense.GateDefense = float64(*payload.GateDefense)
		hasDefense = true
	}
	if payload.MeleeDefense != nil {
		castle.Defense.MeleeDefense = float64(*payload.MeleeDefense)
		hasDefense = true
	}
	if payload.RangedDefense != nil {
		castle.Defense.RangedDefense = float64(*payload.RangedDefense)
		hasDefense = true
	}
	if payload.HDWL != nil {
		castle.Defense.HDWL = int64(*payload.HDWL)
		hasDefense = true
	}
	if len(payload.RangedUnitIDs) > 0 {
		unitIDs, err := parseDefenseUnitIDs(payload.RangedUnitIDs)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.RangedUnitIDs = unitIDs
		hasDefense = true
	}
	if len(payload.MeleeUnitIDs) > 0 {
		unitIDs, err := parseDefenseUnitIDs(payload.MeleeUnitIDs)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.MeleeUnitIDs = unitIDs
		hasDefense = true
	}
	if len(payload.Leader) > 0 {
		var leader struct {
			ID wireInt64 `json:"ID"`
		}
		if err := json.Unmarshal(payload.Leader, &leader); err != nil {
			return nil, false, fmt.Errorf("decode defense castellan: %w", err)
		}
		castle.Defense.CastellanID = State.CastellanID(leader.ID)
		hasDefense = true
	}
	if hasDefense {
		castle.Defense.ObservedAt = observedAt
	}
	hasInventory := len(payload.Units) > 0
	if hasInventory {
		inventory, err := parseDefenseInventory(payload.Units)
		if err != nil {
			return nil, false, err
		}
		castle.Defense.Inventory = inventory
		castle.Defense.InventoryObservedAt = observedAt
	}
	ensureDefenseSlices(&castle.Defense)
	if reflect.DeepEqual(before, castle) {
		return nil, false, nil
	}
	gameState.SetCastleParts(castleID, castle, State.CastlePartDefense)
	return []string{"castles", "defense"}, true, nil
}

func reduceDefenseWall(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	raw, err := defenseSectionPayload(frame.Payload, "dfw")
	if err != nil {
		return nil, false, err
	}
	wall, err := parseDefenseWall(raw, defenseObservedAt(frame.ReceivedAt))
	if err != nil {
		return nil, false, err
	}
	return applyFocusedDefense(gameState, func(defense *State.CastleDefenseState) {
		defense.Wall = wall
	})
}

func reduceDefenseKeep(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	raw, err := defenseSectionPayload(frame.Payload, "dfk")
	if err != nil {
		return nil, false, err
	}
	keep, err := parseDefenseKeep(raw, defenseObservedAt(frame.ReceivedAt))
	if err != nil {
		return nil, false, err
	}
	return applyFocusedDefense(gameState, func(defense *State.CastleDefenseState) {
		defense.Keep = keep
	})
}

func reduceDefenseMoat(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	raw, err := defenseSectionPayload(frame.Payload, "dfm")
	if err != nil {
		return nil, false, err
	}
	moat, err := parseDefenseMoat(raw, defenseObservedAt(frame.ReceivedAt))
	if err != nil {
		return nil, false, err
	}
	return applyFocusedDefense(gameState, func(defense *State.CastleDefenseState) {
		defense.Moat = moat
	})
}

func applyFocusedDefense(
	gameState *State.GameState,
	apply func(defense *State.CastleDefenseState),
) ([]string, bool, error) {
	castleID, castle, found := focusedCastle(gameState)
	if !found {
		return nil, false, nil
	}
	before := castle.Defense
	ensureDefenseSlices(&castle.Defense)
	apply(&castle.Defense)
	ensureDefenseSlices(&castle.Defense)
	if reflect.DeepEqual(before, castle.Defense) {
		return nil, false, nil
	}
	gameState.SetCastleParts(castleID, castle, State.CastlePartDefense)
	return []string{"castles", "defense"}, true, nil
}

func parseDefenseWall(raw json.RawMessage, observedAt time.Time) (State.DefenseWallState, error) {
	var payload struct {
		Left           json.RawMessage `json:"L"`
		Middle         json.RawMessage `json:"M"`
		Right          json.RawMessage `json:"R"`
		UnitCount      wireInt64       `json:"U"`
		StationedUnits wireInt64       `json:"US"`
		Defense        wireFloat64     `json:"D"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return State.DefenseWallState{}, fmt.Errorf("decode defense wall: %w", err)
	}
	left, err := parseDefenseWallSection(payload.Left)
	if err != nil {
		return State.DefenseWallState{}, err
	}
	middle, err := parseDefenseWallSection(payload.Middle)
	if err != nil {
		return State.DefenseWallState{}, err
	}
	right, err := parseDefenseWallSection(payload.Right)
	if err != nil {
		return State.DefenseWallState{}, err
	}
	return State.DefenseWallState{
		Left: left, Middle: middle, Right: right,
		UnitCount: int64(payload.UnitCount), StationedUnits: int64(payload.StationedUnits),
		Defense: float64(payload.Defense), ObservedAt: observedAt,
	}, nil
}

func parseDefenseWallSection(raw json.RawMessage) (State.DefenseWallSection, error) {
	if len(raw) == 0 {
		return State.DefenseWallSection{ToolSlots: []State.DefenseToolSlot{}}, nil
	}
	var payload struct {
		ToolSlots       json.RawMessage `json:"S"`
		UnitPercent     wireInt64       `json:"UP"`
		UnitTypePercent wireInt64       `json:"UC"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return State.DefenseWallSection{}, fmt.Errorf("decode defense wall section: %w", err)
	}
	toolSlots, err := parseDefenseToolSlots(payload.ToolSlots)
	if err != nil {
		return State.DefenseWallSection{}, err
	}
	return State.DefenseWallSection{
		ToolSlots: toolSlots, UnitPercent: int(payload.UnitPercent), UnitTypePercent: int(payload.UnitTypePercent),
	}, nil
}

func parseDefenseKeep(raw json.RawMessage, observedAt time.Time) (State.DefenseKeepState, error) {
	var payload struct {
		PrimaryToolSlots   json.RawMessage `json:"S"`
		SecondaryToolSlots json.RawMessage `json:"STS"`
		MAUCT              wireInt64       `json:"MAUCT"`
		UnitTypePercent    wireInt64       `json:"UC"`
		UnitCount          wireInt64       `json:"U"`
		UYL                wireInt64       `json:"UYL"`
		AUYL               wireInt64       `json:"AUYL"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return State.DefenseKeepState{}, fmt.Errorf("decode defense keep: %w", err)
	}
	primary, err := parseDefenseToolSlots(payload.PrimaryToolSlots)
	if err != nil {
		return State.DefenseKeepState{}, err
	}
	secondary, err := parseDefenseToolSlots(payload.SecondaryToolSlots)
	if err != nil {
		return State.DefenseKeepState{}, err
	}
	return State.DefenseKeepState{
		PrimaryToolSlots: primary, SecondaryToolSlots: secondary,
		MAUCT: int64(payload.MAUCT), UnitTypePercent: int(payload.UnitTypePercent), UnitCount: int64(payload.UnitCount),
		UYL: int64(payload.UYL), AUYL: int64(payload.AUYL), ObservedAt: observedAt,
	}, nil
}

func parseDefenseMoat(raw json.RawMessage, observedAt time.Time) (State.DefenseMoatState, error) {
	var payload struct {
		LeftToolSlots   json.RawMessage `json:"LS"`
		MiddleToolSlots json.RawMessage `json:"MS"`
		RightToolSlots  json.RawMessage `json:"RS"`
		Defense         wireFloat64     `json:"D"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return State.DefenseMoatState{}, fmt.Errorf("decode defense moat: %w", err)
	}
	left, err := parseDefenseToolSlots(payload.LeftToolSlots)
	if err != nil {
		return State.DefenseMoatState{}, err
	}
	middle, err := parseDefenseToolSlots(payload.MiddleToolSlots)
	if err != nil {
		return State.DefenseMoatState{}, err
	}
	right, err := parseDefenseToolSlots(payload.RightToolSlots)
	if err != nil {
		return State.DefenseMoatState{}, err
	}
	return State.DefenseMoatState{
		LeftToolSlots: left, MiddleToolSlots: middle, RightToolSlots: right,
		Defense: float64(payload.Defense), ObservedAt: observedAt,
	}, nil
}

func parseDefenseToolSlots(raw json.RawMessage) ([]State.DefenseToolSlot, error) {
	if len(raw) == 0 {
		return []State.DefenseToolSlot{}, nil
	}
	var rows [][]json.RawMessage
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, fmt.Errorf("decode defense tool slots: %w", err)
	}
	result := make([]State.DefenseToolSlot, 0, len(rows))
	for index, row := range rows {
		if len(row) < 2 {
			return nil, fmt.Errorf("defense tool slot %d has fewer than two fields", index)
		}
		definitionID, definitionOK := rawInt64(row[0])
		amount, amountOK := rawInt64(row[1])
		if !definitionOK || !amountOK {
			return nil, fmt.Errorf("defense tool slot %d has non-numeric fields", index)
		}
		result = append(result, State.DefenseToolSlot{DefinitionID: State.UnitID(definitionID), Amount: amount})
	}
	return result, nil
}

func parseDefenseUnitIDs(raw json.RawMessage) ([]State.UnitID, error) {
	var values []wireInt64
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, fmt.Errorf("decode defense unit ids: %w", err)
	}
	result := make([]State.UnitID, 0, len(values))
	for _, value := range values {
		if value > 0 {
			result = append(result, State.UnitID(value))
		}
	}
	return result, nil
}

func parseDefenseInventory(raw json.RawMessage) (map[State.UnitID]int64, error) {
	var payload struct {
		Inventory json.RawMessage `json:"I"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("decode defense inventory: %w", err)
	}
	if len(payload.Inventory) == 0 {
		return map[State.UnitID]int64{}, nil
	}
	rows, ok := decodeRows(payload.Inventory)
	if !ok {
		return nil, fmt.Errorf("defense inventory does not contain valid I rows")
	}
	result := make(map[State.UnitID]int64, len(rows))
	for _, row := range rows {
		definitionID, amount := rowInt(row, 0), rowInt(row, 1)
		if definitionID > 0 && amount >= 0 {
			result[State.UnitID(definitionID)] += amount
		}
	}
	return result, nil
}

func defenseSectionPayload(raw json.RawMessage, key string) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode %s response: %w", key, err)
	}
	if nested := root[key]; len(nested) > 0 {
		return nested, nil
	}
	return raw, nil
}

func defenseObservedAt(receivedAt time.Time) time.Time {
	if receivedAt.IsZero() {
		return time.Time{}
	}
	return receivedAt.UTC()
}

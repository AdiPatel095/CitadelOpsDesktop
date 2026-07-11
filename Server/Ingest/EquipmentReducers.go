package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceLeaders(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyLeaders(frame.Payload, gameState)
	return []string{"commanders", "castellans", "equipment", "inventory"}, changed, err
}

func reduceEquipmentStorage(
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
		return nil, false, fmt.Errorf("decode equipment storage: %w", err)
	}
	rows, ok := decodeRows(root["I"])
	if !ok {
		return nil, false, fmt.Errorf("equipment storage does not contain I rows")
	}
	next := make(map[State.EquipmentInstanceID]State.EquipmentInstance, len(gameState.Inventory.Equipment)+len(rows))
	for id, equipment := range gameState.Inventory.Equipment {
		if equipment.WearerKind != "" {
			next[id] = equipment
		}
	}
	for _, row := range rows {
		equipment, _, ok := parseEquipment(row, "", 0)
		if ok {
			next[equipment.ID] = equipment
		}
	}
	if reflect.DeepEqual(gameState.Inventory.Equipment, next) {
		return nil, false, nil
	}
	gameState.Inventory.Equipment = next
	return []string{"inventory", "equipment"}, true, nil
}

func reduceConstructionInventory(
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
		return nil, false, fmt.Errorf("decode construction inventory: %w", err)
	}
	rows, ok := decodeRows(root["CI"])
	if !ok {
		return nil, false, nil
	}
	next := make(map[State.ConstructionItemID]int64, len(rows))
	for _, row := range rows {
		id, amount := rowInt(row, 0), rowInt(row, 1)
		if id > 0 && amount >= 0 {
			next[State.ConstructionItemID(id)] = amount
		}
	}
	if reflect.DeepEqual(gameState.Inventory.ConstructionItems, next) {
		return nil, false, nil
	}
	gameState.Inventory.ConstructionItems = next
	return []string{"inventory", "construction-items"}, true, nil
}

func applyLeaders(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("decode leaders: %w", err)
	}
	var commanderRows []json.RawMessage
	var castellanRows []json.RawMessage
	if value := root["C"]; len(value) > 0 {
		if err := json.Unmarshal(value, &commanderRows); err != nil {
			return false, fmt.Errorf("decode commander rows: %w", err)
		}
	}
	if value := root["B"]; len(value) > 0 {
		if err := json.Unmarshal(value, &castellanRows); err != nil {
			return false, fmt.Errorf("decode castellan rows: %w", err)
		}
	}
	if commanderRows == nil && castellanRows == nil {
		return false, fmt.Errorf("leader payload does not contain C or B rows")
	}

	equipment := make(map[State.EquipmentInstanceID]State.EquipmentInstance, len(gameState.Inventory.Equipment))
	gems := make(map[State.GemInstanceID]State.GemInstance, len(gameState.Inventory.Gems))
	for id, item := range gameState.Inventory.Equipment {
		if item.WearerKind == "" {
			equipment[id] = item
		}
	}
	for id, gem := range gameState.Inventory.Gems {
		if gem.WearerKind == "" {
			gems[id] = gem
		}
	}

	commanders := make(map[State.CommanderID]State.CommanderState, len(commanderRows))
	for _, row := range commanderRows {
		leader, ok := decodeLeader(row)
		if !ok || leader.ID < 0 {
			continue
		}
		id := State.CommanderID(leader.ID)
		commander := State.CommanderState{
			ID: id, Name: leader.Name, VisiblePosition: leader.VisiblePosition,
			Available: commanderAvailable(gameState.Movements, id),
			Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
		}
		applyLeaderEquipment(leader.Equipment, "commander", leader.ID, commander.Equipment, commander.Gems, equipment, gems)
		commanders[id] = commander
	}

	castellans := make(map[State.CastellanID]State.CastellanState, len(castellanRows))
	for _, row := range castellanRows {
		leader, ok := decodeLeader(row)
		if !ok || leader.ID < 0 {
			continue
		}
		id := State.CastellanID(leader.ID)
		name := leader.Name
		if name == "" {
			name = gameState.Castles[State.CastleID(leader.CastleID)].Name
		}
		castellan := State.CastellanState{
			ID: id, CastleID: State.CastleID(leader.CastleID), Name: name,
			Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
		}
		applyLeaderEquipment(leader.Equipment, "castellan", leader.ID, castellan.Equipment, castellan.Gems, equipment, gems)
		castellans[id] = castellan
	}

	if reflect.DeepEqual(gameState.Commanders, commanders) &&
		reflect.DeepEqual(gameState.Castellans, castellans) &&
		reflect.DeepEqual(gameState.Inventory.Equipment, equipment) &&
		reflect.DeepEqual(gameState.Inventory.Gems, gems) {
		return false, nil
	}
	gameState.Commanders = commanders
	gameState.Castellans = castellans
	gameState.Inventory.Equipment = equipment
	gameState.Inventory.Gems = gems
	return true, nil
}

type decodedLeader struct {
	ID              int64
	Name            string
	VisiblePosition int
	CastleID        int64
	Equipment       [][]json.RawMessage
}

func decodeLeader(raw json.RawMessage) (decodedLeader, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return decodedLeader{}, false
	}
	id, exists := rawInt64(fields["ID"])
	if !exists {
		return decodedLeader{}, false
	}
	var name string
	_ = json.Unmarshal(fields["N"], &name)
	visible, _ := rawInt64(fields["VIS"])
	castleID, _ := rawInt64(fields["LICID"])
	var equipment [][]json.RawMessage
	_ = json.Unmarshal(fields["EQ"], &equipment)
	return decodedLeader{
		ID: id, Name: name, VisiblePosition: int(visible) + 1,
		CastleID: castleID, Equipment: equipment,
	}, true
}

func applyLeaderEquipment(
	rows [][]json.RawMessage,
	wearerKind string,
	wearerID int64,
	leaderEquipment map[string]State.EquipmentInstanceID,
	leaderGems map[string]State.GemInstanceID,
	equipment map[State.EquipmentInstanceID]State.EquipmentInstance,
	gems map[State.GemInstanceID]State.GemInstance,
) {
	for _, row := range rows {
		item, gem, ok := parseEquipment(row, wearerKind, wearerID)
		if !ok {
			continue
		}
		slot := strconv.Itoa(item.Slot)
		equipment[item.ID] = item
		leaderEquipment[slot] = item.ID
		if gem != nil {
			gems[gem.ID] = *gem
			leaderGems[slot] = gem.ID
		}
	}
}

func parseEquipment(row []json.RawMessage, wearerKind string, wearerID int64) (State.EquipmentInstance, *State.GemInstance, bool) {
	if len(row) < 7 {
		return State.EquipmentInstance{}, nil, false
	}
	id := rowInt(row, 0)
	if id <= 0 {
		return State.EquipmentInstance{}, nil, false
	}
	definitionID := rowInt(row, 6)
	if definitionID <= 0 {
		definitionID = id
	}
	item := State.EquipmentInstance{
		ID: State.EquipmentInstanceID(id), DefinitionID: State.EquipmentID(definitionID),
		Slot: int(rowInt(row, 1)), TypeID: int(rowInt(row, 2)), RarityID: int(rowInt(row, 3)),
		SetID: rowInt(row, 7), Level: int(rowInt(row, 8)), WearerID: wearerID,
		WearerKind: wearerKind, Effects: decodeEffectValues(rowAt(row, 5)),
	}
	gemDefinitionID := rowInt(row, 10)
	gemSlot := int(rowInt(row, 11))
	gemLevel := 0
	gemEffects := map[int64][]float64{}
	if len(row) > 12 {
		var slotRow []json.RawMessage
		if json.Unmarshal(row[12], &slotRow) == nil {
			if slot := rowInt(slotRow, 0); slot > 0 {
				gemSlot = int(slot)
			}
			if len(slotRow) > 3 {
				var gemRow []json.RawMessage
				if json.Unmarshal(slotRow[3], &gemRow) == nil {
					if definition := rowInt(gemRow, 0); definition > 0 {
						gemDefinitionID = definition
					}
					gemEffects = decodeEffectValues(rowAt(gemRow, 4))
					gemLevel = int(rowInt(gemRow, 5))
				}
			}
		}
	}
	if gemDefinitionID <= 0 && item.TypeID == 1 {
		gemDefinitionID = rowInt(row, 9)
	}
	if gemDefinitionID <= 0 {
		return item, nil, true
	}
	gem := &State.GemInstance{
		ID: State.GemInstanceID(item.ID), DefinitionID: State.GemID(gemDefinitionID),
		Slot: gemSlot, Level: gemLevel, WearerID: wearerID, WearerKind: wearerKind,
		Effects: gemEffects,
	}
	return item, gem, true
}

func decodeEffectValues(raw json.RawMessage) map[int64][]float64 {
	result := map[int64][]float64{}
	rows, ok := decodeRows(raw)
	if !ok {
		return result
	}
	for _, row := range rows {
		id := rowInt(row, 0)
		if id <= 0 || len(row) < 2 {
			continue
		}
		var rawValues []json.RawMessage
		values := make([]float64, 0)
		if json.Unmarshal(row[1], &rawValues) != nil {
			if len(row) < 3 || json.Unmarshal(row[2], &rawValues) != nil {
				continue
			}
			if percent, ok := rawFloat64(row[1]); ok {
				values = append(values, percent)
			}
		}
		for _, rawValue := range rawValues {
			if value, ok := rawFloat64(rawValue); ok {
				values = append(values, value)
			}
		}
		result[id] = values
	}
	return result
}

func rowAt(row []json.RawMessage, index int) json.RawMessage {
	if index < 0 || index >= len(row) {
		return nil
	}
	return row[index]
}

func commanderAvailable(movements map[State.MovementID]State.MovementState, commanderID State.CommanderID) bool {
	for _, movement := range movements {
		if movement.CommanderID != nil && *movement.CommanderID == commanderID {
			return false
		}
	}
	return true
}

func syncCommanderAvailability(gameState *State.GameState) {
	for id, commander := range gameState.Commanders {
		commander.Available = commanderAvailable(gameState.Movements, id)
		gameState.Commanders[id] = commander
	}
}

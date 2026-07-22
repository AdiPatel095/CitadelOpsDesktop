package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"time"

	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceLeaders(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyLeaders(frame.Payload, gameState, gameData)
	return []string{"commanders", "castellans", "equipment", "inventory"}, changed, err
}

func reduceGenerals(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyGenerals(frame.Payload, frame.ReceivedAt, gameState)
	return []string{"generals", "general-skills"}, changed, err
}

func applyGenerals(raw json.RawMessage, observedAt time.Time, gameState *State.GameState) (bool, error) {
	var payload struct {
		Generals []struct {
			ID             wireInt64   `json:"GID"`
			ActiveSkillIDs []wireInt64 `json:"SIDS"`
		} `json:"G"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode generals: %w", err)
	}
	if payload.Generals == nil {
		return false, fmt.Errorf("general payload does not contain G rows")
	}
	next := make(map[int64]State.GeneralState, len(payload.Generals))
	for _, row := range payload.Generals {
		if row.ID <= 0 {
			continue
		}
		activeSkillIDs := make([]int64, 0, len(row.ActiveSkillIDs))
		for _, skillID := range row.ActiveSkillIDs {
			if skillID > 0 {
				activeSkillIDs = append(activeSkillIDs, int64(skillID))
			}
		}
		id := int64(row.ID)
		next[id] = State.GeneralState{ID: id, ActiveSkillIDs: activeSkillIDs, ObservedAt: observedAt}
	}
	if reflect.DeepEqual(gameState.Generals, next) {
		return false, nil
	}
	gameState.Generals = next
	return true, nil
}

func reduceEquipmentStorage(
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
		return nil, false, fmt.Errorf("decode equipment storage: %w", err)
	}
	rows, ok := decodeRows(root["I"])
	if !ok {
		return nil, false, fmt.Errorf("equipment storage does not contain I rows")
	}
	next := make(map[State.EquipmentInstanceID]State.EquipmentInstance, len(gameState.Inventory.Equipment)+len(rows))
	nextGems := make(map[State.GemInstanceID]State.GemInstance, len(gameState.Inventory.Gems)+len(rows))
	for id, equipment := range gameState.Inventory.Equipment {
		if equipment.WearerKind != "" {
			next[id] = equipment
		}
	}
	for id, gem := range gameState.Inventory.Gems {
		if gem.WearerKind != "" {
			nextGems[id] = gem
		}
	}
	for _, row := range rows {
		equipment, gem, ok := parseEquipment(row, "", 0, gameData)
		if ok {
			next[equipment.ID] = equipment
			if gem != nil {
				nextGems[gem.ID] = *gem
			}
		}
	}
	if reflect.DeepEqual(gameState.Inventory.Equipment, next) && reflect.DeepEqual(gameState.Inventory.Gems, nextGems) {
		return nil, false, nil
	}
	gameState.Inventory.Equipment = next
	gameState.Inventory.Gems = nextGems
	return []string{"inventory", "equipment", "gems"}, true, nil
}

func reduceGemStorage(
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
		return nil, false, fmt.Errorf("decode gem storage: %w", err)
	}
	stackRows, hasStacks := decodeRows(root["GEM"])
	relicRows, hasRelics := decodeRows(root["RGEM"])
	if !hasStacks && !hasRelics {
		return nil, false, fmt.Errorf("gem storage does not contain GEM or RGEM rows")
	}

	nextStacks := gameState.Inventory.GemStacks
	if hasStacks {
		nextStacks = make(map[State.GemID]int64, len(stackRows))
		for _, row := range stackRows {
			id, amount := rowInt(row, 0), rowInt(row, 1)
			if id > 0 && amount >= 0 {
				nextStacks[State.GemID(id)] = amount
			}
		}
	}
	nextGems := gameState.Inventory.Gems
	if hasRelics {
		nextGems = make(map[State.GemInstanceID]State.GemInstance, len(gameState.Inventory.Gems)+len(relicRows))
		for id, gem := range gameState.Inventory.Gems {
			if gem.WearerKind != "" {
				nextGems[id] = gem
			}
		}
		for _, row := range relicRows {
			if gem, ok := parseStoredGem(row, gameData); ok {
				nextGems[gem.ID] = gem
			}
		}
	}
	if reflect.DeepEqual(gameState.Inventory.GemStacks, nextStacks) && reflect.DeepEqual(gameState.Inventory.Gems, nextGems) {
		return nil, false, nil
	}
	gameState.Inventory.GemStacks = nextStacks
	gameState.Inventory.Gems = nextGems
	return []string{"inventory", "gems"}, true, nil
}

func reduceEquipmentMutation(
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
		return nil, false, fmt.Errorf("decode equipment mutation: %w", err)
	}
	changed := false
	if leaders := root["gli"]; len(leaders) > 0 {
		updated, err := applyLeaders(leaders, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	} else if len(root["C"]) > 0 || len(root["B"]) > 0 {
		updated, err := applyLeaders(frame.Payload, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["E"]; len(raw) > 0 {
		var row []json.RawMessage
		if json.Unmarshal(raw, &row) == nil && rowInt(row, 0) > 0 {
			id := State.EquipmentInstanceID(rowInt(row, 0))
			current := gameState.Inventory.Equipment[id]
			item, gem, ok := parseEquipment(row, current.WearerKind, current.WearerID, gameData)
			if ok {
				if !reflect.DeepEqual(current, item) {
					gameState.Inventory.Equipment[id] = item
					changed = true
				}
				if gem != nil && !reflect.DeepEqual(gameState.Inventory.Gems[gem.ID], *gem) {
					gameState.Inventory.Gems[gem.ID] = *gem
					changed = true
				}
			}
		}
	}
	if len(root["GEM"]) > 0 || len(root["RGEM"]) > 0 {
		_, updated, err := reduceGemStorage(context.Background(), frame, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	if raw := root["gcu"]; len(raw) > 0 {
		updated, err := applyPlayerResources(raw, gameState, gameData)
		if err != nil {
			return nil, false, err
		}
		changed = changed || updated
	}
	return []string{"commanders", "castellans", "equipment", "gems", "inventory", "resources"}, changed, nil
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
	if reflect.DeepEqual(gameState.Inventory.ConstructionItems, next) && gameState.Inventory.ConstructionItemsObservedAt.Equal(frame.ReceivedAt) {
		return nil, false, nil
	}
	gameState.Inventory.ConstructionItems = next
	gameState.Inventory.ConstructionItemsObservedAt = frame.ReceivedAt
	return []string{"inventory", "construction-items"}, true, nil
}

func applyLeaders(raw json.RawMessage, gameState *State.GameState, gameData *GameData.Store) (bool, error) {
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
			Available: commanderAvailable(gameState, id), GeneralID: leader.GeneralID,
			Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
		}
		applyLeaderEquipment(leader.Equipment, "commander", leader.ID, commander.Equipment, commander.Gems, equipment, gems, gameData)
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
		applyLeaderEquipment(leader.Equipment, "castellan", leader.ID, castellan.Equipment, castellan.Gems, equipment, gems, gameData)
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
	GeneralID       int64
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
	generalID, _ := rawInt64(fields["GID"])
	if generalID < 0 {
		generalID = 0
	}
	var equipment [][]json.RawMessage
	_ = json.Unmarshal(fields["EQ"], &equipment)
	return decodedLeader{
		ID: id, Name: name, VisiblePosition: int(visible) + 1,
		CastleID: castleID, GeneralID: generalID, Equipment: equipment,
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
	gameData *GameData.Store,
) {
	for _, row := range rows {
		item, gem, ok := parseEquipment(row, wearerKind, wearerID, gameData)
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

func parseEquipment(row []json.RawMessage, wearerKind string, wearerID int64, gameData *GameData.Store) (State.EquipmentInstance, *State.GemInstance, bool) {
	if len(row) < 7 {
		return State.EquipmentInstance{}, nil, false
	}
	id := rowInt(row, 0)
	if id <= 0 {
		return State.EquipmentInstance{}, nil, false
	}
	definitionID := rowInt(row, 6)
	if definitionID <= 0 {
		definitionID = rowInt(row, 4)
	}
	if definitionID <= 0 {
		definitionID = id
	}
	item := State.EquipmentInstance{
		ID: State.EquipmentInstanceID(id), DefinitionID: State.EquipmentID(definitionID),
		Slot: int(rowInt(row, 1)), TypeID: int(rowInt(row, 2)), RarityID: int(rowInt(row, 3)),
		SetID: rowInt(row, 7), Level: int(rowInt(row, 8)), WearerID: wearerID,
		WearerKind: wearerKind, Effects: decodeEquipmentEffects(rowAt(row, 5), gameData, itemUsesRelicEffects(int(rowInt(row, 3)))),
	}
	gemWireID := rowInt(row, 10)
	if gemWireID <= 0 && item.TypeID == 1 {
		gemWireID = rowInt(row, 9)
	}
	gemInstanceID := int64(0)
	gemTypeID := int64(0)
	gemSlot := int(rowInt(row, 11))
	gemLevel := 0
	gemEffects := State.EquipmentEffects{}
	if len(row) > 12 {
		var slotRow []json.RawMessage
		if json.Unmarshal(row[12], &slotRow) == nil {
			if slot := rowInt(slotRow, 0); slot > 0 {
				gemSlot = int(slot)
			}
			if len(slotRow) > 3 {
				var gemRow []json.RawMessage
				if json.Unmarshal(slotRow[3], &gemRow) == nil {
					if id := rowInt(gemRow, 0); id > 0 {
						gemInstanceID = id
						gemWireID = id
					}
					gemTypeID = rowInt(gemRow, 1)
					gemEffects = decodeEquipmentEffects(rowAt(gemRow, 4), gameData, true)
					gemLevel = int(rowInt(gemRow, 5))
				}
			}
		}
	}
	if gemWireID <= 0 {
		return item, nil, true
	}
	if gemInstanceID <= 0 {
		// Older catalog gems expose only a reusable definition id on the wire.
		// A negative parent-derived id keeps each socket unique without inventing
		// a second game identity; DefinitionID remains the command/catalog id.
		gemInstanceID = -int64(item.ID)
	}
	gem := &State.GemInstance{
		ID: State.GemInstanceID(gemInstanceID), DefinitionID: State.GemID(gemWireID), TypeID: int(gemTypeID),
		Slot: gemSlot, Level: gemLevel, EquipmentInstanceID: item.ID,
		WearerID: wearerID, WearerKind: wearerKind, Effects: gemEffects,
	}
	EquipmentDomain.HydrateGem(gem, gameData)
	return item, gem, true
}

func parseStoredGem(row []json.RawMessage, gameData *GameData.Store) (State.GemInstance, bool) {
	id := rowInt(row, 0)
	if id <= 0 {
		return State.GemInstance{}, false
	}
	gem := State.GemInstance{
		ID: State.GemInstanceID(id), DefinitionID: State.GemID(id), TypeID: int(rowInt(row, 1)),
		Level: int(rowInt(row, 5)), Effects: decodeEquipmentEffects(rowAt(row, 4), gameData, true),
	}
	EquipmentDomain.HydrateGem(&gem, gameData)
	return gem, true
}

func decodeEquipmentEffects(raw json.RawMessage, gameData *GameData.Store, relic bool) State.EquipmentEffects {
	result := State.EquipmentEffects{}
	rows, ok := decodeRows(raw)
	if !ok {
		return result
	}
	for _, row := range rows {
		id := rowInt(row, 0)
		if id <= 0 || len(row) < 2 {
			continue
		}
		var rollPercent *float64
		var rawValues []json.RawMessage
		if json.Unmarshal(row[1], &rawValues) != nil {
			if len(row) < 3 || json.Unmarshal(row[2], &rawValues) != nil {
				continue
			}
			if percent, ok := rawFloat64(row[1]); ok {
				rollPercent = floatPointer(percent)
			}
		}
		values := make([]float64, 0, len(rawValues))
		for _, rawValue := range rawValues {
			if value, ok := rawFloat64(rawValue); ok {
				values = append(values, value)
			}
		}
		result = append(result, State.EquipmentEffect{
			WireID: id, DefinitionID: officialEquipmentEffectID(gameData, id, relic),
			RollPercent: rollPercent, Values: values,
		})
	}
	return result
}

func itemUsesRelicEffects(rarityID int) bool {
	return rarityID == 5 || rarityID == 15
}

func officialEquipmentEffectID(gameData *GameData.Store, wireID int64, relic bool) int64 {
	if gameData == nil || wireID <= 0 {
		return wireID
	}
	collection := "equipment_effects"
	if relic {
		collection = "relicEffects"
	}
	catalog, err := gameData.Catalog(collection)
	if err != nil {
		return wireID
	}
	raw, ok := catalog.Find(strconv.FormatInt(wireID, 10))
	if !ok {
		return wireID
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return wireID
	}
	definitionID, ok := record.Int64("effectID")
	if !ok || definitionID <= 0 {
		return wireID
	}
	return definitionID
}

func rowAt(row []json.RawMessage, index int) json.RawMessage {
	if index < 0 || index >= len(row) {
		return nil
	}
	return row[index]
}

func commanderAvailable(gameState *State.GameState, commanderID State.CommanderID) bool {
	return commanderAvailableAt(gameState, commanderID, time.Now().UTC())
}

func commanderAvailableAt(gameState *State.GameState, commanderID State.CommanderID, now time.Time) bool {
	return gameState == nil || !State.CommanderHasActiveMovementAt(*gameState, commanderID, now)
}

func syncCommanderAvailability(gameState *State.GameState) {
	for id, commander := range gameState.Commanders {
		commander.Available = commanderAvailable(gameState, id)
		gameState.Commanders[id] = commander
	}
}

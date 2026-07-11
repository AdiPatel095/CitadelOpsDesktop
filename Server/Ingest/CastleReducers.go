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

func applyCastleList(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var payload struct {
		Kingdoms []struct {
			ID      wireInt64         `json:"KID"`
			Castles []json.RawMessage `json:"AI"`
		} `json:"C"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode castle list: %w", err)
	}
	next := make(map[State.CastleID]State.CastleState)
	for _, kingdom := range payload.Kingdoms {
		for _, castleRaw := range kingdom.Castles {
			var wrapper struct {
				Identity []json.RawMessage `json:"AI"`
			}
			if json.Unmarshal(castleRaw, &wrapper) != nil || len(wrapper.Identity) < 4 {
				continue
			}
			id := State.CastleID(rowInt(wrapper.Identity, 3))
			if id <= 0 {
				continue
			}
			castle, exists := gameState.Castles[id]
			if !exists {
				castle = newCastleState(id)
			}
			castle.ID = id
			castle.KingdomID = State.KingdomID(kingdom.ID)
			castle.SlotType = int(rowInt(wrapper.Identity, 0))
			castle.X = int(rowInt(wrapper.Identity, 1))
			castle.Y = int(rowInt(wrapper.Identity, 2))
			castle.Name = rowString(wrapper.Identity, 10)
			ensureCastleMaps(&castle)
			next[id] = castle
		}
	}
	if len(next) == 0 {
		return false, nil
	}
	if reflect.DeepEqual(gameState.Castles, next) {
		return false, nil
	}
	gameState.Castles = next
	return true, nil
}

func applyCastleDetails(raw json.RawMessage, gameState *State.GameState, gameData *GameData.Store) (bool, error) {
	var payload struct {
		Kingdoms []struct {
			ID      wireInt64         `json:"KID"`
			Castles []json.RawMessage `json:"AI"`
		} `json:"C"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode castle details: %w", err)
	}
	changed := false
	for _, kingdom := range payload.Kingdoms {
		for _, castleRaw := range kingdom.Castles {
			var values map[string]json.RawMessage
			if json.Unmarshal(castleRaw, &values) != nil {
				continue
			}
			castleIDValue, ok := rawInt64(values["AID"])
			if !ok || castleIDValue <= 0 {
				continue
			}
			castleID := State.CastleID(castleIDValue)
			castle, exists := gameState.Castles[castleID]
			if !exists {
				castle = newCastleState(castleID)
			}
			beforeKingdomID := castle.KingdomID
			beforeResources := copyResourceBalances(castle.Resources)
			beforeUnits := castle.Units
			castle.KingdomID = State.KingdomID(kingdom.ID)
			ensureCastleMaps(&castle)
			applyCastleResourceValues(values, &castle, gameData)
			castle.Units = castleUnitsFromGroups(values["AC"], values["TU"], values["HI"], values["SHI"])
			if beforeKingdomID != castle.KingdomID || !reflect.DeepEqual(beforeResources, castle.Resources) || !reflect.DeepEqual(beforeUnits, castle.Units) {
				gameState.Castles[castleID] = castle
				changed = true
			}
		}
	}
	return changed, nil
}

func reduceCastleSnapshot(
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
		return nil, false, fmt.Errorf("decode castle snapshot: %w", err)
	}
	var gca map[string]json.RawMessage
	if err := json.Unmarshal(root["gca"], &gca); err != nil {
		return nil, false, fmt.Errorf("castle snapshot is missing gca")
	}
	var identity []json.RawMessage
	if json.Unmarshal(gca["A"], &identity) != nil || len(identity) < 4 {
		return nil, false, fmt.Errorf("castle snapshot has no castle identity")
	}
	castleID := State.CastleID(rowInt(identity, 3))
	castle, known := gameState.Castles[castleID]
	if castleID <= 0 || !known {
		return nil, false, nil
	}
	beforePlayer := gameState.Player
	beforeAlliance := gameState.Alliance
	changed := false
	for id, candidate := range gameState.Castles {
		focused := id == castleID
		if candidate.Focused != focused {
			changed = true
		}
		candidate.Focused = focused
		gameState.Castles[id] = candidate
	}
	castle = gameState.Castles[castleID]
	beforeCastle := castle
	castle.KingdomID = State.KingdomID(rawInteger(root["KID"]))
	castle.SlotType = int(rowInt(identity, 0))
	castle.X = int(rowInt(identity, 1))
	castle.Y = int(rowInt(identity, 2))
	if name := rowString(identity, 10); name != "" {
		castle.Name = name
	}
	ensureCastleMaps(&castle)
	if _, hasBG := gca["BG"]; hasBG {
		castle.Buildings = parseCastleBuildings(gca["BG"], gca["BD"], gameData)
	} else if _, hasBD := gca["BD"]; hasBD {
		castle.Buildings = parseCastleBuildings(gca["BG"], gca["BD"], gameData)
	}
	if raw, exists := gca["CI"]; exists {
		castle.ConstructionSlots = parseConstructionSlots(raw, gameData)
	}
	if raw, exists := root["gui"]; exists {
		units, err := parseCastleUnits(raw)
		if err != nil {
			return nil, false, err
		}
		castle.Units = units
	}
	gameState.Castles[castleID] = castle
	applyCastleOwner(gca["O"], gameState)
	changed = changed || !reflect.DeepEqual(beforeCastle, castle) ||
		!reflect.DeepEqual(beforePlayer, gameState.Player) || !reflect.DeepEqual(beforeAlliance, gameState.Alliance)
	return []string{"castles", "player", "alliance"}, changed, nil
}

func reduceFocusedUnits(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, ok := focusedCastle(gameState)
	if !ok {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode unit snapshot: %w", err)
	}
	raw := frame.Payload
	if nested, exists := root["gui"]; exists {
		raw = nested
	}
	units, err := parseCastleUnits(raw)
	if err != nil {
		return nil, false, err
	}
	if reflect.DeepEqual(castle.Units, units) {
		return nil, false, nil
	}
	castle.Units = units
	gameState.Castles[castleID] = castle
	return []string{"castles", "units"}, true, nil
}

func reduceFocusedConstructionItems(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, ok := focusedCastle(gameState)
	if !ok {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode construction-item snapshot: %w", err)
	}
	raw, exists := root["CI"]
	if !exists {
		return nil, false, nil
	}
	next := parseConstructionSlots(raw, gameData)
	if reflect.DeepEqual(castle.ConstructionSlots, next) {
		return nil, false, nil
	}
	castle.ConstructionSlots = next
	gameState.Castles[castleID] = castle
	return []string{"castles", "construction-items"}, true, nil
}

func parseCastleBuildings(bg json.RawMessage, bd json.RawMessage, gameData *GameData.Store) map[State.BuildingInstanceID]State.Building {
	buildings := map[State.BuildingInstanceID]State.Building{}
	for _, raw := range []json.RawMessage{bg, bd} {
		rows, ok := decodeRows(raw)
		if !ok {
			continue
		}
		for _, row := range rows {
			definitionID := rowInt(row, 0)
			instanceID := rowInt(row, 1)
			if definitionID <= 0 || instanceID <= 0 {
				continue
			}
			level := int(rowInt(row, 14))
			if level < 0 {
				level = officialLevel(gameData, "buildings", definitionID)
			}
			buildings[State.BuildingInstanceID(instanceID)] = State.Building{
				InstanceID: State.BuildingInstanceID(instanceID), DefinitionID: State.BuildingID(definitionID),
				GridX: int(rowInt(row, 2)), GridY: int(rowInt(row, 3)), Rotation: int(rowInt(row, 4)), Level: level,
			}
		}
	}
	return buildings
}

func parseConstructionSlots(raw json.RawMessage, gameData *GameData.Store) map[State.BuildingInstanceID][]State.ConstructionSlot {
	result := map[State.BuildingInstanceID][]State.ConstructionSlot{}
	var buildings []struct {
		OID   wireInt64 `json:"OID"`
		Slots []struct {
			CID wireInt64       `json:"CID"`
			S   int             `json:"S"`
			RS  json.RawMessage `json:"RS"`
		} `json:"CIL"`
	}
	if json.Unmarshal(raw, &buildings) != nil {
		return result
	}
	for _, building := range buildings {
		if building.OID <= 0 {
			continue
		}
		slots := make([]State.ConstructionSlot, 0, len(building.Slots))
		for _, slot := range building.Slots {
			if slot.CID <= 0 {
				continue
			}
			var remaining *int
			if value, exists := rawInt64(slot.RS); exists {
				remaining = intPointer(int(value))
			}
			slots = append(slots, State.ConstructionSlot{
				DefinitionID: State.ConstructionItemID(slot.CID), Slot: slot.S,
				RemainingSec: remaining, Level: officialLevel(gameData, "constructionItems", int64(slot.CID)),
			})
		}
		result[State.BuildingInstanceID(building.OID)] = slots
	}
	return result
}

func parseCastleUnits(raw json.RawMessage) (State.CastleUnits, error) {
	var groups map[string]json.RawMessage
	if err := json.Unmarshal(raw, &groups); err != nil {
		return State.CastleUnits{}, fmt.Errorf("decode castle units: %w", err)
	}
	return castleUnitsFromGroups(groups["I"], groups["TU"], groups["HI"], groups["SHI"]), nil
}

func castleUnitsFromGroups(stationedRaw, travelingRaw, hospitalRaw, specialHospitalRaw json.RawMessage) State.CastleUnits {
	stationed := toUnitMap(decodeUnitCounts(stationedRaw))
	traveling := toUnitMap(decodeUnitCounts(travelingRaw))
	hospital := toUnitMap(decodeUnitCounts(hospitalRaw))
	specialHospital := toUnitMap(decodeUnitCounts(specialHospitalRaw))
	total := map[State.UnitID]int64{}
	for _, group := range []map[State.UnitID]int64{stationed, traveling, hospital, specialHospital} {
		for id, amount := range group {
			total[id] += amount
		}
	}
	return State.CastleUnits{
		Stationed: stationed, Traveling: traveling, Hospital: hospital,
		SpecialHospital: specialHospital, Total: total,
	}
}

func applyCastleResourceValues(values map[string]json.RawMessage, castle *State.CastleState, gameData *GameData.Store) {
	var production map[string]json.RawMessage
	_ = json.Unmarshal(values["gpa"], &production)
	for jsonKey, rawAmount := range values {
		definitionID, ok := officialDefinitionID(gameData, "resources", "resourceID", jsonKey)
		if !ok {
			continue
		}
		amount, ok := rawFloat64(rawAmount)
		if !ok {
			continue
		}
		balance := State.ResourceBalance{Amount: amount}
		if value, exists := rawFloat64(production["D"+jsonKey]); exists {
			balance.ProductionPerHour = floatPointer(value / 10)
		}
		if value, exists := rawFloat64(production["MR"+jsonKey]); exists {
			balance.Capacity = floatPointer(value)
		}
		castle.Resources[State.ResourceID(definitionID)] = balance
	}
}

func applyCastleOwner(raw json.RawMessage, gameState *State.GameState) {
	var owner struct {
		ID          wireInt64   `json:"OID"`
		Name        string      `json:"N"`
		Level       int         `json:"L"`
		LegendLevel int         `json:"LL"`
		Might       wireFloat64 `json:"MP"`
		Glory       wireFloat64 `json:"CF"`
		AllianceID  wireInt64   `json:"AID"`
		Alliance    string      `json:"AN"`
	}
	if len(raw) == 0 || json.Unmarshal(raw, &owner) != nil || State.PlayerID(owner.ID) != gameState.Player.ID {
		return
	}
	gameState.Player.Name = owner.Name
	gameState.Player.Level = owner.Level
	gameState.Player.LegendLevel = owner.LegendLevel
	gameState.Player.Might = float64(owner.Might)
	gameState.Player.Glory = float64(owner.Glory)
	gameState.Player.AllianceID = State.AllianceID(owner.AllianceID)
	if owner.AllianceID > 0 {
		gameState.Alliance.ID = State.AllianceID(owner.AllianceID)
	}
	if owner.Alliance != "" {
		gameState.Alliance.Name = owner.Alliance
	}
}

func focusedCastle(gameState *State.GameState) (State.CastleID, State.CastleState, bool) {
	for id, castle := range gameState.Castles {
		if castle.Focused {
			return id, castle, true
		}
	}
	return 0, State.CastleState{}, false
}

func newCastleState(id State.CastleID) State.CastleState {
	castle := State.CastleState{ID: id}
	ensureCastleMaps(&castle)
	return castle
}

func ensureCastleMaps(castle *State.CastleState) {
	if castle.Resources == nil {
		castle.Resources = map[State.ResourceID]State.ResourceBalance{}
	}
	if castle.Units.Stationed == nil {
		castle.Units.Stationed = map[State.UnitID]int64{}
	}
	if castle.Units.Traveling == nil {
		castle.Units.Traveling = map[State.UnitID]int64{}
	}
	if castle.Units.Hospital == nil {
		castle.Units.Hospital = map[State.UnitID]int64{}
	}
	if castle.Units.SpecialHospital == nil {
		castle.Units.SpecialHospital = map[State.UnitID]int64{}
	}
	if castle.Units.Total == nil {
		castle.Units.Total = map[State.UnitID]int64{}
	}
	if castle.Buildings == nil {
		castle.Buildings = map[State.BuildingInstanceID]State.Building{}
	}
	if castle.ConstructionSlots == nil {
		castle.ConstructionSlots = map[State.BuildingInstanceID][]State.ConstructionSlot{}
	}
	if castle.Queues == nil {
		castle.Queues = map[string][]State.QueueItem{}
	}
}

func copyResourceBalances(source map[State.ResourceID]State.ResourceBalance) map[State.ResourceID]State.ResourceBalance {
	copy := make(map[State.ResourceID]State.ResourceBalance, len(source))
	for id, balance := range source {
		copy[id] = balance
	}
	return copy
}

func toUnitMap(values map[int64]int64) map[State.UnitID]int64 {
	result := make(map[State.UnitID]int64, len(values))
	for id, amount := range values {
		result[State.UnitID(id)] = amount
	}
	return result
}

func rawInteger(raw json.RawMessage) int64 {
	value, _ := rawInt64(raw)
	return value
}

package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"time"

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

func reduceCastleList(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	raw := frame.Payload
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode castle-list response: %w", err)
	}
	if nested := root["gcl"]; len(nested) > 0 {
		raw = nested
	}
	var identity struct {
		PlayerID wireInt64 `json:"PID"`
	}
	if err := json.Unmarshal(raw, &identity); err != nil {
		return nil, false, fmt.Errorf("decode castle-list identity: %w", err)
	}
	playerID := State.PlayerID(identity.PlayerID)
	if playerID > 0 && gameState.Player.ID > 0 && playerID != gameState.Player.ID {
		return nil, false, nil
	}
	changed, err := applyCastleList(raw, gameState)
	return []string{"castles"}, changed, err
}

func applyCastleDetails(raw json.RawMessage, gameState *State.GameState, gameData *GameData.Store, observedAt time.Time) (bool, error) {
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
			beforeUnitsObservedAt := castle.UnitsObservedAt
			beforeOpenGateUntil := castle.Defense.OpenGateUntil
			castle.KingdomID = State.KingdomID(kingdom.ID)
			ensureCastleMaps(&castle)
			applyCastleResourceValues(values, &castle, gameData)
			castle.Units = castleUnitsFromGroups(values["AC"], values["TU"], values["HI"], values["SHI"])
			if remaining, present := rawInt64(values["OGT"]); present {
				castle.Defense.OpenGateUntil = castleOpenGateUntil(remaining, observedAt)
			}
			if !observedAt.IsZero() {
				castle.UnitsObservedAt = observedAt.UTC()
			}
			if beforeKingdomID != castle.KingdomID || !reflect.DeepEqual(beforeResources, castle.Resources) ||
				!reflect.DeepEqual(beforeUnits, castle.Units) || !beforeUnitsObservedAt.Equal(castle.UnitsObservedAt) ||
				!reflect.DeepEqual(beforeOpenGateUntil, castle.Defense.OpenGateUntil) {
				gameState.Castles[castleID] = castle
				changed = true
			}
		}
	}
	return changed, nil
}

func castleOpenGateUntil(value int64, observedAt time.Time) *time.Time {
	if value <= 0 {
		return nil
	}
	observedAt = defenseObservedAt(observedAt)
	var until time.Time
	switch {
	case value >= 1_000_000_000_000:
		until = time.UnixMilli(value).UTC()
	case value >= 1_000_000_000:
		until = time.Unix(value, 0).UTC()
	default:
		until = observedAt.Add(time.Duration(value) * time.Second)
	}
	if !until.After(observedAt) {
		return nil
	}
	return &until
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
	if rawKingdomID, found := root["KID"]; found {
		castle.KingdomID = State.KingdomID(rawInteger(rawKingdomID))
	} else if len(identity) > 16 {
		castle.KingdomID = State.KingdomID(rowInt(identity, 16))
	}
	castle.SlotType = int(rowInt(identity, 0))
	castle.X = int(rowInt(identity, 1))
	castle.Y = int(rowInt(identity, 2))
	if name := rowString(identity, 10); name != "" {
		castle.Name = name
	}
	ensureCastleMaps(&castle)
	hasLayout := false
	for _, key := range []string{"BG", "BD", "T", "G", "D", "FP"} {
		if _, found := gca[key]; found {
			hasLayout = true
			break
		}
	}
	if hasLayout {
		castle.Buildings, castle.Layout = parseCastleLayoutLayers([]castleWireLayer{
			{State.BuildingLayerBG, gca["BG"]}, {State.BuildingLayerBD, gca["BD"]},
			{State.BuildingLayerT, gca["T"]}, {State.BuildingLayerG, gca["G"]}, {State.BuildingLayerD, gca["D"]},
			{State.BuildingLayerFP, gca["FP"]},
		}, gameData)
		castle.Layout.ObservedAt = frame.ReceivedAt.UTC()
	}
	buildingProductionRaw, hasBuildingProduction := root["abpi"]
	if hasBuildingProduction {
		castle.BuildingProduction = parseBuildingProduction(buildingProductionRaw, frame.ReceivedAt)
	}
	buildingQueueRaw, hasBuildingQueue := gca["scl"]
	if hasBuildingQueue {
		castle.BuildingQueue = parseBuildingConstructionQueue(buildingQueueRaw)
		castle.BuildingQueue.ObservedAt = frame.ReceivedAt.UTC()
	}
	constructionItemsRaw, hasConstructionItems := gca["CI"]
	if hasConstructionItems {
		castle.ConstructionSlots = parseConstructionSlots(constructionItemsRaw, gameData)
		castle.ConstructionSlotsObservedAt = frame.ReceivedAt.UTC()
	}
	unitsRaw, hasUnits := root["gui"]
	if hasUnits {
		units, err := parseCastleUnits(unitsRaw)
		if err != nil {
			return nil, false, err
		}
		castle.Units = units
		castle.UnitsObservedAt = frame.ReceivedAt.UTC()
	}
	if !frame.ReceivedAt.IsZero() {
		castle.FoodStateObservedAt = frame.ReceivedAt.UTC()
	}
	gameState.Castles[castleID] = castle
	applyCastleOwner(gca["O"], gameState)
	changed = changed || !reflect.DeepEqual(beforeCastle, castle) ||
		!reflect.DeepEqual(beforePlayer, gameState.Player) || !reflect.DeepEqual(beforeAlliance, gameState.Alliance)
	domains := []string{"castles", "player", "alliance"}
	if gameState.Session.LoggedIn && gameState.Session.SocketReady &&
		gameState.Session.Generation > 0 && !frame.ReceivedAt.Before(gameState.Session.ChangedAt) &&
		gameState.Session.BaselineGeneration != gameState.Session.Generation {
		gameState.Session.BaselineGeneration = gameState.Session.Generation
		changed = true
		domains = append(domains, "session")
	}
	if hasLayout {
		domains = append(domains, "building-layout")
	}
	if hasBuildingProduction {
		domains = append(domains, "building-production")
	}
	if hasBuildingQueue {
		domains = append(domains, "building-construction")
	}
	if hasConstructionItems {
		domains = append(domains, "construction-items")
	}
	if hasUnits {
		domains = append(domains, "units")
	}
	return domains, changed, nil
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
	} else if _, hasStationed := root["I"]; !hasStationed {
		if _, hasTraveling := root["TU"]; !hasTraveling {
			if _, hasHospital := root["HI"]; !hasHospital {
				if _, hasSpecialHospital := root["SHI"]; !hasSpecialHospital {
					return nil, false, nil
				}
			}
		}
	}
	units, err := parseCastleUnits(raw)
	if err != nil {
		return nil, false, err
	}
	observedAt := frame.ReceivedAt.UTC()
	if reflect.DeepEqual(castle.Units, units) && castle.UnitsObservedAt.Equal(observedAt) {
		return nil, false, nil
	}
	castle.Units = units
	castle.UnitsObservedAt = observedAt
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
	observedAt := frame.ReceivedAt.UTC()
	if reflect.DeepEqual(castle.ConstructionSlots, next) && castle.ConstructionSlotsObservedAt.Equal(observedAt) {
		return nil, false, nil
	}
	castle.ConstructionSlots = next
	castle.ConstructionSlotsObservedAt = observedAt
	gameState.Castles[castleID] = castle
	return []string{"castles", "construction-items"}, true, nil
}

func parseCastleBuildings(bg json.RawMessage, bd json.RawMessage, gameData *GameData.Store) map[State.BuildingInstanceID]State.Building {
	buildings, _ := parseCastleLayout(bg, bd, gameData)
	return buildings
}

func parseCastleLayout(
	bg json.RawMessage,
	bd json.RawMessage,
	gameData *GameData.Store,
) (map[State.BuildingInstanceID]State.Building, State.CastleLayout) {
	return parseCastleLayoutLayers([]castleWireLayer{
		{State.BuildingLayerBG, bg}, {State.BuildingLayerBD, bd},
	}, gameData)
}

type castleWireLayer struct {
	name State.BuildingLayer
	raw  json.RawMessage
}

func parseCastleLayoutLayers(
	layers []castleWireLayer,
	gameData *GameData.Store,
) (map[State.BuildingInstanceID]State.Building, State.CastleLayout) {
	buildings := map[State.BuildingInstanceID]State.Building{}
	layout := State.CastleLayout{
		Ground:  map[State.BuildingInstanceID]State.Building{},
		Objects: map[State.BuildingInstanceID]State.Building{},
		Fixed:   map[State.BuildingInstanceID]State.Building{},
	}
	for _, layer := range layers {
		rows, ok := decodeRows(layer.raw)
		if !ok {
			continue
		}
		for _, row := range rows {
			definitionID := rowInt(row, 0)
			instanceID := rowInt(row, 1)
			if definitionID <= 0 || instanceID <= 0 {
				continue
			}
			constructionState := int(rowInt(row, 6))
			if constructionState == State.BuildingStateDisassembledCompleted {
				continue
			}
			level := int(rowInt(row, 14))
			if level < 0 {
				level = officialLevel(gameData, "buildings", definitionID)
			}
			building := State.Building{
				InstanceID: State.BuildingInstanceID(instanceID), DefinitionID: State.BuildingID(definitionID),
				GridX: int(rowInt(row, 2)), GridY: int(rowInt(row, 3)), Rotation: int(rowInt(row, 4)),
				ProgressSec: rowInt(row, 5), ConstructionState: constructionState, Level: level,
				Layer: layer.name,
			}
			building.Placed = building.GridX >= 0 && building.GridY >= 0
			if layer.name == State.BuildingLayerBG || layer.name == State.BuildingLayerBD {
				buildings[building.InstanceID] = building
			}
			if officialBuildingGroup(gameData, definitionID) == "Ground" {
				layout.Ground[building.InstanceID] = building
			} else if layer.name == State.BuildingLayerT || layer.name == State.BuildingLayerG ||
				layer.name == State.BuildingLayerD || layer.name == State.BuildingLayerFP {
				layout.Fixed[building.InstanceID] = building
			} else {
				layout.Objects[building.InstanceID] = building
			}
		}
	}
	return buildings, layout
}

func parseBuildingConstructionQueue(raw json.RawMessage) State.BuildingConstructionQueue {
	queue := State.BuildingConstructionQueue{Slots: []State.BuildingConstructionQueueSlot{}}
	var wire struct {
		ObjectIDs []wireInt64 `json:"OIDL"`
		SlotCount wireInt64   `json:"SSC"`
	}
	if json.Unmarshal(raw, &wire) != nil {
		return queue
	}
	queue.SlotCount = int(wire.SlotCount)
	queue.Slots = make([]State.BuildingConstructionQueueSlot, 0, len(wire.ObjectIDs))
	for index, rawObjectID := range wire.ObjectIDs {
		wireValue := int64(rawObjectID)
		slot := State.BuildingConstructionQueueSlot{Index: index, WireValue: wireValue, Status: State.BuildingQueueSlotUnknown}
		switch {
		case wireValue > 0:
			slot.Status = State.BuildingQueueSlotOccupied
			slot.BuildingID = State.BuildingInstanceID(wireValue)
		case wireValue == -1:
			slot.Status = State.BuildingQueueSlotAvailable
		case wireValue == -2:
			slot.Status = State.BuildingQueueSlotLocked
		}
		queue.Slots = append(queue.Slots, slot)
	}
	return queue
}

func officialBuildingGroup(gameData *GameData.Store, definitionID int64) string {
	if definitionID == 200 || definitionID == 201 {
		return "Ground"
	}
	if gameData == nil || definitionID <= 0 {
		return ""
	}
	catalog, err := gameData.Catalog("buildings")
	if err != nil {
		return ""
	}
	raw, found := catalog.Find(strconv.FormatInt(definitionID, 10))
	if !found {
		return ""
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return ""
	}
	group, _ := record.String("group")
	return strings.TrimSpace(group)
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
		resourceID := State.ResourceID(definitionID)
		balance := castle.Resources[resourceID]
		balance.Amount = amount
		applyResourceProductionValues(&balance, jsonKey, production)
		castle.Resources[resourceID] = balance
	}
}

func applyCastleProductionValues(raw json.RawMessage, castle *State.CastleState, gameData *GameData.Store) error {
	var production map[string]json.RawMessage
	if err := json.Unmarshal(raw, &production); err != nil {
		return fmt.Errorf("decode castle production: %w", err)
	}
	resources, err := gameData.Catalog("resources")
	if err != nil {
		return err
	}
	for _, rawResource := range resources.Rows() {
		record, decodeErr := GameData.DecodeRecord(rawResource)
		if decodeErr != nil {
			continue
		}
		definitionID, idExists := record.Int64("resourceID")
		jsonKey, keyExists := record.String("JSONKey")
		if !idExists || !keyExists || definitionID <= 0 || jsonKey == "" {
			continue
		}
		resourceID := State.ResourceID(definitionID)
		balance := castle.Resources[resourceID]
		before := balance
		applyResourceProductionValues(&balance, jsonKey, production)
		if !reflect.DeepEqual(before, balance) {
			castle.Resources[resourceID] = balance
		}
	}
	return nil
}

func applyResourceProductionValues(balance *State.ResourceBalance, jsonKey string, production map[string]json.RawMessage) {
	if value, exists := rawFloat64(production["D"+jsonKey]); exists {
		balance.ProductionPerHour = floatPointer(value / 10)
	}
	if value, exists := rawFloat64(production["D"+jsonKey+"C"]); exists {
		balance.ConsumptionPerHour = floatPointer(value / 10)
	}
	if multiplierKey := consumptionMultiplierKey(jsonKey); multiplierKey != "" {
		if value, exists := rawFloat64(production[multiplierKey]); exists {
			balance.ConsumptionMultiplier = floatPointer(value / 100)
		}
	}
	if value, exists := rawFloat64(production["MR"+jsonKey]); exists {
		balance.Capacity = floatPointer(value)
	}
}

func consumptionMultiplierKey(resourceJSONKey string) string {
	switch resourceJSONKey {
	case "F":
		return "FCR"
	case "MEAD":
		return "MEADCR"
	case "BEEF":
		return "BEEFCR"
	default:
		return ""
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
	ensureDefenseSlices(&castle.Defense)
	if castle.Buildings == nil {
		castle.Buildings = map[State.BuildingInstanceID]State.Building{}
	}
	if castle.BuildingProduction == nil {
		castle.BuildingProduction = map[State.BuildingInstanceID]State.BuildingProduction{}
	}
	if castle.Layout.Ground == nil {
		castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	}
	if castle.Layout.Objects == nil {
		castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	}
	if castle.Layout.Fixed == nil {
		castle.Layout.Fixed = map[State.BuildingInstanceID]State.Building{}
	}
	if castle.BuildingQueue.Slots == nil {
		castle.BuildingQueue.Slots = []State.BuildingConstructionQueueSlot{}
	}
	if castle.ConstructionSlots == nil {
		castle.ConstructionSlots = map[State.BuildingInstanceID][]State.ConstructionSlot{}
	}
	if castle.Production == nil {
		castle.Production = map[int]State.ProductionQueue{}
	}
	if castle.QueueableProduction == nil {
		castle.QueueableProduction = map[int][]State.DefinitionRef{}
	}
	if castle.Crafting.Buildings == nil {
		castle.Crafting.Buildings = map[State.BuildingInstanceID]State.CraftingBuilding{}
	}
	if castle.Crafting.EnabledRecipeIDs == nil {
		castle.Crafting.EnabledRecipeIDs = []int64{}
	}
	if castle.Crafting.EnabledRecipeGroupIDs == nil {
		castle.Crafting.EnabledRecipeGroupIDs = []int64{}
	}
	if castle.Crafting.OutputBoostByQueueType == nil {
		castle.Crafting.OutputBoostByQueueType = map[int]float64{}
	}
}

func ensureDefenseSlices(defense *State.CastleDefenseState) {
	if defense.RangedUnitIDs == nil {
		defense.RangedUnitIDs = []State.UnitID{}
	}
	if defense.MeleeUnitIDs == nil {
		defense.MeleeUnitIDs = []State.UnitID{}
	}
	if defense.Inventory == nil {
		defense.Inventory = map[State.UnitID]int64{}
	}
	if defense.Wall.Left.ToolSlots == nil {
		defense.Wall.Left.ToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Wall.Middle.ToolSlots == nil {
		defense.Wall.Middle.ToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Wall.Right.ToolSlots == nil {
		defense.Wall.Right.ToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Keep.PrimaryToolSlots == nil {
		defense.Keep.PrimaryToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Keep.SecondaryToolSlots == nil {
		defense.Keep.SecondaryToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Moat.LeftToolSlots == nil {
		defense.Moat.LeftToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Moat.MiddleToolSlots == nil {
		defense.Moat.MiddleToolSlots = []State.DefenseToolSlot{}
	}
	if defense.Moat.RightToolSlots == nil {
		defense.Moat.RightToolSlots = []State.DefenseToolSlot{}
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

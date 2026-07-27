package Ingest

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceExpansionMutation(
	ctx context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	var root map[string]json.RawMessage
	if json.Unmarshal(frame.Payload, &root) == nil && len(root["gca"]) > 0 {
		return reduceCastleSnapshot(ctx, frame, gameState, gameData)
	}
	return reduceBuildingMutation(ctx, frame, gameState, gameData)
}

func reduceBuildingMutation(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, focused := focusedCastle(gameState)
	if !focused {
		return nil, false, nil
	}
	ensureCastleMaps(&castle)
	cloneCastleBuildingMaps(&castle)
	var root map[string]json.RawMessage
	_ = json.Unmarshal(frame.Payload, &root)
	changed := false

	if frame.Opcode == "sob" || frame.Opcode == "sbd" || frame.Opcode == "etc" {
		if objectID, ok := rawInt64(root["OID"]); ok && objectID > 0 {
			changed = removeCastleBuilding(&castle, State.BuildingInstanceID(objectID)) || changed
		}
	} else {
		for _, row := range mutationBuildingRows(frame.Opcode, frame.Payload, root) {
			changed = applyBuildingMutationRow(&castle, row, gameData) || changed
		}
	}

	queueRaw := root["scl"]
	if frame.Opcode == "scl" {
		queueRaw = frame.Payload
	}
	if len(queueRaw) > 0 {
		queue := parseBuildingConstructionQueue(queueRaw)
		queue.ObservedAt = frame.ReceivedAt.UTC()
		if !reflect.DeepEqual(castle.BuildingQueue, queue) {
			castle.BuildingQueue = queue
			changed = true
		}
	}
	if !changed {
		return []string{"castles", "building-layout", "building-construction"}, false, nil
	}
	gameState.Castles[castleID] = castle
	return []string{"castles", "building-layout", "building-construction"}, true, nil
}

func reduceBuildingProduction(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	castleID, castle, focused := focusedCastle(gameState)
	if !focused {
		return nil, false, nil
	}
	updates := parseBuildingProduction(frame.Payload, frame.ReceivedAt)
	if len(updates) == 0 {
		return []string{"castles", "building-production"}, false, nil
	}
	ensureCastleMaps(&castle)
	before := castle.BuildingProduction
	castle.BuildingProduction = cloneBuildingProduction(before)
	for buildingID, update := range updates {
		castle.BuildingProduction[buildingID] = update
	}
	if reflect.DeepEqual(before, castle.BuildingProduction) {
		return []string{"castles", "building-production"}, false, nil
	}
	gameState.Castles[castleID] = castle
	return []string{"castles", "building-production"}, true, nil
}

func parseBuildingProduction(raw json.RawMessage, observedAt time.Time) map[State.BuildingInstanceID]State.BuildingProduction {
	type wireProduction struct {
		ObjectID wireInt64                  `json:"OID"`
		Percent  map[string]json.RawMessage `json:"PA"`
	}
	var rows []wireProduction
	if json.Unmarshal(raw, &rows) != nil {
		var row wireProduction
		if json.Unmarshal(raw, &row) != nil {
			return map[State.BuildingInstanceID]State.BuildingProduction{}
		}
		rows = []wireProduction{row}
	}
	result := make(map[State.BuildingInstanceID]State.BuildingProduction, len(rows))
	for _, row := range rows {
		buildingID := State.BuildingInstanceID(row.ObjectID)
		if buildingID <= 0 {
			continue
		}
		percentByResource := make(map[string]float64, len(row.Percent))
		for resource, rawPercent := range row.Percent {
			if percent, ok := rawFloat64(rawPercent); ok {
				percentByResource[resource] = percent
			}
		}
		if len(percentByResource) == 0 {
			continue
		}
		result[buildingID] = State.BuildingProduction{
			PercentByResource: percentByResource,
			ObservedAt:        observedAt.UTC(),
		}
	}
	return result
}

func mutationBuildingRows(
	opcode string,
	payload json.RawMessage,
	root map[string]json.RawMessage,
) [][]json.RawMessage {
	keys := []string{}
	switch opcode {
	case "ebe", "ebu":
		keys = []string{"NO"}
	case "emo":
		keys = []string{"MO"}
	case "eup":
		keys = []string{"O"}
	case "fco", "ego":
		keys = []string{"O"}
	case "edo":
		keys = []string{"O", "NO"}
	}
	for _, key := range keys {
		if rows := flexibleBuildingRows(root[key]); len(rows) > 0 {
			return rows
		}
	}
	if opcode == "edo" || opcode == "ego" {
		return flexibleBuildingRows(payload)
	}
	return nil
}

func flexibleBuildingRows(raw json.RawMessage) [][]json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] != '[' {
		return nil
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || len(values) == 0 {
		return nil
	}
	first := bytes.TrimSpace(values[0])
	if len(first) > 0 && first[0] == '[' {
		rows := make([][]json.RawMessage, 0, len(values))
		for _, value := range values {
			var row []json.RawMessage
			if json.Unmarshal(value, &row) == nil {
				rows = append(rows, row)
			}
		}
		return rows
	}
	return [][]json.RawMessage{values}
}

func applyBuildingMutationRow(
	castle *State.CastleState,
	row []json.RawMessage,
	gameData *GameData.Store,
) bool {
	if len(row) < 2 {
		return false
	}
	definitionID := rowInt(row, 0)
	instanceID := State.BuildingInstanceID(rowInt(row, 1))
	if instanceID <= 0 {
		return false
	}
	if definitionID <= 0 {
		return removeCastleBuilding(castle, instanceID)
	}
	constructionState := int(rowInt(row, 6))
	if constructionState == State.BuildingStateDisassembledCompleted {
		return removeCastleBuilding(castle, instanceID)
	}
	existing, found := buildingFromCastleMaps(*castle, instanceID)
	wasFixed := existingInFixedLayout(*castle, existing, instanceID)
	layer := existing.Layer
	if layer == "" {
		if officialBuildingGroup(gameData, definitionID) == "Ground" {
			layer = State.BuildingLayerBG
		} else {
			layer = State.BuildingLayerBD
		}
	}
	level := int(rowInt(row, 14))
	if level < 0 {
		level = officialLevel(gameData, "buildings", definitionID)
	}
	building := State.Building{
		InstanceID: instanceID, DefinitionID: State.BuildingID(definitionID),
		GridX: int(rowInt(row, 2)), GridY: int(rowInt(row, 3)), Rotation: int(rowInt(row, 4)),
		ProgressSec: rowInt(row, 5), ConstructionState: constructionState, Level: level, Layer: layer,
	}
	building.Placed = building.GridX >= 0 && building.GridY >= 0
	unchanged := found && reflect.DeepEqual(existing, building)
	removeCastleBuildingMaps(castle, instanceID)
	if layer == State.BuildingLayerBG || layer == State.BuildingLayerBD {
		castle.Buildings[instanceID] = building
	}
	if officialBuildingGroup(gameData, definitionID) == "Ground" {
		castle.Layout.Ground[instanceID] = building
	} else if wasFixed ||
		layer == State.BuildingLayerT || layer == State.BuildingLayerG || layer == State.BuildingLayerD {
		castle.Layout.Fixed[instanceID] = building
	} else {
		castle.Layout.Objects[instanceID] = building
	}
	return !unchanged
}

func buildingFromCastleMaps(castle State.CastleState, buildingID State.BuildingInstanceID) (State.Building, bool) {
	for _, buildings := range []map[State.BuildingInstanceID]State.Building{
		castle.Layout.Objects, castle.Layout.Ground, castle.Layout.Fixed, castle.Buildings,
	} {
		if building, found := buildings[buildingID]; found {
			return building, true
		}
	}
	return State.Building{}, false
}

func existingInFixedLayout(castle State.CastleState, existing State.Building, buildingID State.BuildingInstanceID) bool {
	if _, found := castle.Layout.Fixed[buildingID]; found {
		return true
	}
	switch existing.Layer {
	case State.BuildingLayerT, State.BuildingLayerG, State.BuildingLayerD:
		return true
	default:
		return false
	}
}

func removeCastleBuilding(castle *State.CastleState, buildingID State.BuildingInstanceID) bool {
	changed := removeCastleBuildingMaps(castle, buildingID)
	if _, found := castle.ConstructionSlots[buildingID]; found {
		delete(castle.ConstructionSlots, buildingID)
		changed = true
	}
	return changed
}

func removeCastleBuildingMaps(castle *State.CastleState, buildingID State.BuildingInstanceID) bool {
	changed := false
	for _, buildings := range []map[State.BuildingInstanceID]State.Building{
		castle.Buildings, castle.Layout.Objects, castle.Layout.Ground, castle.Layout.Fixed,
	} {
		if _, found := buildings[buildingID]; found {
			delete(buildings, buildingID)
			changed = true
		}
	}
	return changed
}

func cloneCastleBuildingMaps(castle *State.CastleState) {
	castle.Buildings = cloneBuildingMap(castle.Buildings)
	castle.Layout.Objects = cloneBuildingMap(castle.Layout.Objects)
	castle.Layout.Ground = cloneBuildingMap(castle.Layout.Ground)
	castle.Layout.Fixed = cloneBuildingMap(castle.Layout.Fixed)
	constructionSlots := make(map[State.BuildingInstanceID][]State.ConstructionSlot, len(castle.ConstructionSlots))
	for id, slots := range castle.ConstructionSlots {
		constructionSlots[id] = append([]State.ConstructionSlot(nil), slots...)
	}
	castle.ConstructionSlots = constructionSlots
}

func cloneBuildingMap(source map[State.BuildingInstanceID]State.Building) map[State.BuildingInstanceID]State.Building {
	result := make(map[State.BuildingInstanceID]State.Building, len(source))
	for id, building := range source {
		result[id] = building
	}
	return result
}

func cloneBuildingProduction(source map[State.BuildingInstanceID]State.BuildingProduction) map[State.BuildingInstanceID]State.BuildingProduction {
	result := make(map[State.BuildingInstanceID]State.BuildingProduction, len(source))
	for id, production := range source {
		percentByResource := make(map[string]float64, len(production.PercentByResource))
		for resource, percent := range production.PercentByResource {
			percentByResource[resource] = percent
		}
		production.PercentByResource = percentByResource
		result[id] = production
	}
	return result
}

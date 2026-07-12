package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceQueueableProduction(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyQueueableProduction(frame.Payload, gameState, gameData, frame.ReceivedAt)
	return []string{"castles", "production"}, changed, err
}

func applyQueueableProduction(
	raw json.RawMessage,
	gameState *State.GameState,
	gameData *GameData.Store,
	observedAt time.Time,
) (bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("decode queueable production: %w", err)
	}
	if nested := root["gpc"]; len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return false, fmt.Errorf("decode nested queueable production: %w", err)
		}
	}
	var rows []struct {
		CastleID wireInt64 `json:"AID"`
		Units    struct {
			Unlocked []wireInt64 `json:"U"`
		} `json:"U"`
	}
	if err := json.Unmarshal(root["A"], &rows); err != nil {
		return false, fmt.Errorf("decode queueable production rows: %w", err)
	}
	if gameData == nil {
		return false, nil
	}
	units, unitsErr := gameData.Catalog("units")
	if unitsErr != nil {
		return false, nil
	}
	changed := false
	for _, row := range rows {
		castleID := State.CastleID(row.CastleID)
		castle, exists := gameState.Castles[castleID]
		if !exists || castleID <= 0 {
			continue
		}
		next := map[int][]State.DefinitionRef{0: {}, 1: {}}
		seen := map[int]map[int64]struct{}{0: {}, 1: {}}
		for _, wireID := range row.Units.Unlocked {
			id := int64(wireID)
			if id <= 0 {
				continue
			}
			lineID := -1
			collection := ""
			if rawDefinition, found := units.Find(strconv.FormatInt(id, 10)); found {
				record, decodeErr := GameData.DecodeRecord(rawDefinition)
				if decodeErr != nil {
					continue
				}
				if GameData.IsToolRecord(record) {
					lineID, collection = 1, "tools"
				} else {
					lineID, collection = 0, "units"
				}
			}
			if lineID < 0 {
				continue
			}
			if _, duplicate := seen[lineID][id]; duplicate {
				continue
			}
			seen[lineID][id] = struct{}{}
			next[lineID] = append(next[lineID], State.DefinitionRef{Collection: collection, ID: id})
		}
		for lineID := range next {
			sort.Slice(next[lineID], func(left, right int) bool { return next[lineID][left].ID < next[lineID][right].ID })
		}
		ensureCastleMaps(&castle)
		if reflect.DeepEqual(castle.QueueableProduction, next) && castle.QueueableObservedAt.Equal(observedAt) {
			continue
		}
		castle.QueueableProduction = next
		castle.QueueableObservedAt = observedAt
		gameState.Castles[castleID] = castle
		changed = true
	}
	return changed, nil
}

func reduceStorageInventory(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyStorageInventory(frame.Payload, gameState)
	return []string{"inventory", "storage"}, changed, err
}

func reduceEmbeddedStorageInventory(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(frame.Payload, &root) != nil || len(root["sin"]) == 0 {
		return nil, false, nil
	}
	changed, err := applyStorageInventory(root["sin"], gameState)
	return []string{"inventory", "storage"}, changed, err
}

func applyStorageInventory(raw json.RawMessage, gameState *State.GameState) (bool, error) {
	var segments []struct {
		SegmentID wireInt64           `json:"SID"`
		Rows      [][]json.RawMessage `json:"RD"`
	}
	if err := json.Unmarshal(raw, &segments); err != nil {
		return false, fmt.Errorf("decode storage inventory: %w", err)
	}
	if gameState.Inventory.Items == nil {
		gameState.Inventory.Items = map[string]map[int64]int64{}
	}
	changed := false
	for _, segment := range segments {
		if segment.SegmentID <= 0 {
			continue
		}
		next := map[int64]int64{}
		for _, row := range segment.Rows {
			id, amount := rowInt(row, 0), rowInt(row, 1)
			if id > 0 && amount >= 0 {
				next[id] += amount
			}
		}
		key := fmt.Sprintf("storage:%d", segment.SegmentID)
		if reflect.DeepEqual(gameState.Inventory.Items[key], next) {
			continue
		}
		gameState.Inventory.Items[key] = next
		changed = true
	}
	return changed, nil
}

func reduceConstructionOffers(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		Products []struct {
			ProductID wireInt64 `json:"PID"`
			Amount    wireInt64 `json:"AMT"`
		} `json:"PL"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode construction offers: %w", err)
	}
	next := make(map[State.PackageID]int64, len(payload.Products))
	for _, product := range payload.Products {
		if product.ProductID > 0 && product.Amount >= 0 {
			next[State.PackageID(product.ProductID)] = int64(product.Amount)
		}
	}
	if reflect.DeepEqual(gameState.Inventory.ConstructionOffers, next) &&
		gameState.Inventory.ConstructionOffersObservedAt.Equal(frame.ReceivedAt) {
		return nil, false, nil
	}
	gameState.Inventory.ConstructionOffers = next
	gameState.Inventory.ConstructionOffersObservedAt = frame.ReceivedAt
	return []string{"inventory", "construction-offers"}, true, nil
}

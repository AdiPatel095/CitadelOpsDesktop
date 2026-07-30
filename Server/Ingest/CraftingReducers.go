package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"reflect"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type craftingWireBundle struct {
	RecipeIDs   []wireInt64   `json:"CRID"`
	BatchValues []wireFloat64 `json:"BV"`
	Rentals     []int         `json:"RUT"`
	Runtimes    []int         `json:"RCT"`
}

type craftingWireBuilding struct {
	KingdomID    wireInt64          `json:"KID"`
	CastleID     wireInt64          `json:"AID"`
	InstanceID   wireInt64          `json:"OID"`
	QueueTypeID  int                `json:"CQID"`
	DefinitionID wireInt64          `json:"WID"`
	SlotCount    int                `json:"S"`
	Active       craftingWireBundle `json:"PS"`
	Queued       craftingWireBundle `json:"QS"`
}

type craftingWireGroup struct {
	Buildings    []craftingWireBuilding `json:"CBI"`
	Entitlements [][]json.RawMessage    `json:"CE"`
}

func reduceCraftingSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	groups, flat, err := decodeCraftingPayload(frame.Payload)
	if err != nil {
		return nil, false, err
	}
	changed := false
	if flat != nil {
		changed = mergeCraftingBuilding(gameState, *flat, frame.ReceivedAt)
	}
	for _, group := range groups {
		if len(group.Buildings) == 0 {
			continue
		}
		castleID := State.CastleID(group.Buildings[0].CastleID)
		castle, ok := gameState.Castles[castleID]
		if !ok || castleID <= 0 {
			continue
		}
		ensureCastleMaps(&castle)
		next := make(map[State.BuildingInstanceID]State.CraftingBuilding, len(group.Buildings))
		for _, building := range group.Buildings {
			if State.CastleID(building.CastleID) != castleID || building.InstanceID <= 0 {
				continue
			}
			parsed := craftingBuildingFromWire(building, frame.ReceivedAt)
			next[parsed.InstanceID] = parsed
		}
		recipes, recipeGroups, boosts := parseCraftingEntitlements(group.Entitlements)
		if !reflect.DeepEqual(castle.Crafting.Buildings, next) ||
			!reflect.DeepEqual(castle.Crafting.EnabledRecipeIDs, recipes) ||
			!reflect.DeepEqual(castle.Crafting.EnabledRecipeGroupIDs, recipeGroups) ||
			!reflect.DeepEqual(castle.Crafting.OutputBoostByQueueType, boosts) {
			castle.Crafting.Buildings = next
			castle.Crafting.EnabledRecipeIDs = recipes
			castle.Crafting.EnabledRecipeGroupIDs = recipeGroups
			castle.Crafting.OutputBoostByQueueType = boosts
			gameState.Castles[castleID] = castle
			changed = true
		}
	}
	return []string{"castles", "crafting"}, changed, nil
}

func reduceCraftingBuilding(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	if !frameSucceeded(frame) {
		changed := reconcileRejectedCraftingResources(gameState, gameData, frame.Payload)
		return []string{"castles", "resources", "currencies", "player"}, changed, nil
	}
	_, flat, err := decodeCraftingPayload(frame.Payload)
	if err != nil {
		return nil, false, err
	}
	if flat == nil {
		return nil, false, nil
	}
	castleID := State.CastleID(flat.CastleID)
	castle, castleExists := gameState.Castles[castleID]
	before, buildingExists := castle.Crafting.Buildings[State.BuildingInstanceID(flat.InstanceID)]
	changed := mergeCraftingBuilding(gameState, *flat, frame.ReceivedAt)
	resourceChanged := false
	if changed && castleExists && buildingExists && strings.EqualFold(frame.Opcode, "crst") {
		after := craftingBuildingFromWire(*flat, frame.ReceivedAt)
		if recipeID, added := addedCraftingRecipe(before, after); added {
			resourceChanged = deductCraftingRecipeCosts(gameState, gameData, castleID, recipeID)
		}
	}
	domains := []string{"castles", "crafting"}
	if resourceChanged {
		domains = append(domains, "resources", "currencies", "player")
	}
	return domains, changed || resourceChanged, nil
}

func reconcileRejectedCraftingResources(
	gameState *State.GameState,
	gameData *GameData.Store,
	payload json.RawMessage,
) bool {
	if gameState == nil || gameData == nil {
		return false
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(payload, &root) != nil {
		return false
	}
	recipeID := rawInteger(root["CRID"])
	buildingID := State.BuildingInstanceID(rawInteger(root["OID"]))
	if recipeID <= 0 || buildingID <= 0 {
		return false
	}
	costs, err := GameData.CraftingRecipeCosts(gameData, recipeID)
	if err != nil {
		return false
	}
	castleID := State.CastleID(0)
	for id, castle := range gameState.Castles {
		if _, exists := castle.Crafting.Buildings[buildingID]; exists {
			castleID = id
			break
		}
	}
	castle, exists := gameState.Castles[castleID]
	if !exists {
		return false
	}
	ensureCastleMaps(&castle)
	changed := false
	for _, cost := range costs {
		stock, valid := rawFloat64(root[cost.JSONKey+"S"])
		if !valid {
			continue
		}
		switch {
		case cost.ResourceID > 0 && (strings.EqualFold(cost.JSONKey, "C1") || strings.EqualFold(cost.JSONKey, "C2")):
			resourceID := State.ResourceID(cost.ResourceID)
			if gameState.Player.Resources[resourceID] != stock {
				gameState.Player.Resources[resourceID] = stock
				changed = true
			}
		case cost.ResourceID > 0:
			resourceID := State.ResourceID(cost.ResourceID)
			balance := castle.Resources[resourceID]
			if balance.Amount != stock {
				balance.Amount = stock
				castle.Resources[resourceID] = balance
				changed = true
			}
		case cost.CurrencyID > 0:
			currencyID := State.CurrencyID(cost.CurrencyID)
			if gameState.Player.Currencies[currencyID] != stock {
				gameState.Player.Currencies[currencyID] = stock
				changed = true
			}
		}
	}
	if changed {
		gameState.Castles[castleID] = castle
	}
	return changed
}

func addedCraftingRecipe(before State.CraftingBuilding, after State.CraftingBuilding) (int64, bool) {
	counts := map[int64]int{}
	for _, item := range before.Active {
		counts[item.RecipeID]--
	}
	for _, item := range before.Queued {
		counts[item.RecipeID]--
	}
	for _, item := range after.Active {
		counts[item.RecipeID]++
	}
	for _, item := range after.Queued {
		counts[item.RecipeID]++
	}
	addedID := int64(0)
	addedCount := 0
	for recipeID, count := range counts {
		for count > 0 {
			addedID = recipeID
			addedCount++
			count--
		}
	}
	return addedID, addedCount == 1 && addedID > 0
}

func deductCraftingRecipeCosts(
	gameState *State.GameState,
	gameData *GameData.Store,
	castleID State.CastleID,
	recipeID int64,
) bool {
	if gameState == nil || gameData == nil {
		return false
	}
	costs, err := GameData.CraftingRecipeCosts(gameData, recipeID)
	if err != nil {
		return false
	}
	castle, exists := gameState.Castles[castleID]
	if !exists {
		return false
	}
	ensureCastleMaps(&castle)
	changed := false
	for _, cost := range costs {
		switch {
		case cost.ResourceID > 0 && (strings.EqualFold(cost.JSONKey, "C1") || strings.EqualFold(cost.JSONKey, "C2")):
			resourceID := State.ResourceID(cost.ResourceID)
			current := gameState.Player.Resources[resourceID]
			next := math.Max(0, current-cost.Amount)
			if next != current {
				gameState.Player.Resources[resourceID] = next
				changed = true
			}
		case cost.ResourceID > 0:
			resourceID := State.ResourceID(cost.ResourceID)
			balance := castle.Resources[resourceID]
			next := math.Max(0, balance.Amount-cost.Amount)
			if next != balance.Amount {
				balance.Amount = next
				castle.Resources[resourceID] = balance
				changed = true
			}
		case cost.CurrencyID > 0:
			currencyID := State.CurrencyID(cost.CurrencyID)
			current := gameState.Player.Currencies[currencyID]
			next := math.Max(0, current-cost.Amount)
			if next != current {
				gameState.Player.Currencies[currencyID] = next
				changed = true
			}
		}
	}
	if changed {
		gameState.Castles[castleID] = castle
	}
	return changed
}

func decodeCraftingPayload(raw json.RawMessage) ([]craftingWireGroup, *craftingWireBuilding, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil, nil, fmt.Errorf("decode crafting payload: %w", err)
	}
	if nested := firstRaw(root, "crai", "CRAI"); len(nested) > 0 {
		if err := json.Unmarshal(nested, &root); err != nil {
			return nil, nil, fmt.Errorf("decode nested crafting payload: %w", err)
		}
	}
	if cai := firstRaw(root, "CAI", "cai"); len(cai) > 0 {
		var groups []craftingWireGroup
		if err := json.Unmarshal(cai, &groups); err != nil {
			var group craftingWireGroup
			if singleErr := json.Unmarshal(cai, &group); singleErr != nil {
				return nil, nil, fmt.Errorf("decode crafting groups: %w", err)
			}
			groups = []craftingWireGroup{group}
		}
		return groups, nil, nil
	}
	if cbi := firstRaw(root, "CBI", "cbi"); len(cbi) > 0 {
		var building craftingWireBuilding
		if json.Unmarshal(cbi, &building) == nil && building.InstanceID > 0 {
			return nil, &building, nil
		}
		var buildings []craftingWireBuilding
		if err := json.Unmarshal(cbi, &buildings); err == nil && len(buildings) == 1 {
			return nil, &buildings[0], nil
		}
	}
	var building craftingWireBuilding
	if err := json.Unmarshal(raw, &building); err == nil && building.InstanceID > 0 {
		return nil, &building, nil
	}
	return nil, nil, nil
}

func mergeCraftingBuilding(gameState *State.GameState, wire craftingWireBuilding, observedAt time.Time) bool {
	castleID := State.CastleID(wire.CastleID)
	castle, ok := gameState.Castles[castleID]
	if !ok || castleID <= 0 || wire.InstanceID <= 0 {
		return false
	}
	ensureCastleMaps(&castle)
	next := craftingBuildingFromWire(wire, observedAt)
	if reflect.DeepEqual(castle.Crafting.Buildings[next.InstanceID], next) {
		return false
	}
	castle.Crafting.Buildings[next.InstanceID] = next
	gameState.Castles[castleID] = castle
	return true
}

func craftingBuildingFromWire(wire craftingWireBuilding, observedAt time.Time) State.CraftingBuilding {
	return State.CraftingBuilding{
		KingdomID: State.KingdomID(wire.KingdomID), CastleID: State.CastleID(wire.CastleID),
		InstanceID: State.BuildingInstanceID(wire.InstanceID), DefinitionID: State.BuildingID(wire.DefinitionID),
		QueueTypeID: wire.QueueTypeID, SlotCount: wire.SlotCount,
		ActiveSlotRentals: append([]int(nil), wire.Active.Rentals...),
		QueueSlotRentals:  append([]int(nil), wire.Queued.Rentals...),
		Active:            craftingQueueFromWire(wire.Active), Queued: craftingQueueFromWire(wire.Queued),
		ObservedAt: observedAt,
	}
}

func craftingQueueFromWire(bundle craftingWireBundle) []State.CraftingQueueItem {
	items := make([]State.CraftingQueueItem, 0, len(bundle.RecipeIDs))
	for index, recipeID := range bundle.RecipeIDs {
		if recipeID <= 0 {
			continue
		}
		item := State.CraftingQueueItem{RecipeID: int64(recipeID)}
		if index < len(bundle.BatchValues) {
			item.BatchValue = float64(bundle.BatchValues[index])
		}
		if index < len(bundle.Runtimes) {
			item.RemainingSec = intPointer(bundle.Runtimes[index])
		}
		items = append(items, item)
	}
	return items
}

func parseCraftingEntitlements(entries [][]json.RawMessage) ([]int64, []int64, map[int]float64) {
	recipes := map[int64]struct{}{}
	groups := map[int64]struct{}{}
	boosts := map[int]float64{}
	for _, entry := range entries {
		if len(entry) < 2 {
			continue
		}
		effectID, _ := rawInt64(entry[0])
		var values []json.RawMessage
		if json.Unmarshal(entry[1], &values) != nil {
			continue
		}
		switch effectID {
		case 616:
			for _, value := range values {
				if id, ok := rawInt64(value); ok && id > 0 {
					recipes[id] = struct{}{}
				}
			}
		case 377:
			for _, value := range values {
				if id, ok := rawInt64(value); ok && id > 0 {
					groups[id] = struct{}{}
				}
			}
		case 373:
			if len(values) >= 2 {
				queueID, _ := rawInt64(values[0])
				boost, _ := rawFloat64(values[1])
				if queueID > 0 {
					boosts[int(queueID)] += boost
				}
			}
		}
	}
	return sortedInt64Keys(recipes), sortedInt64Keys(groups), boosts
}

func sortedInt64Keys(values map[int64]struct{}) []int64 {
	result := make([]int64, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func firstRaw(values map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, key := range keys {
		if len(values[key]) > 0 {
			return values[key]
		}
	}
	return nil
}

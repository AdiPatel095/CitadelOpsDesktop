package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type productionWireProduct struct {
	DefinitionID wireInt64 `json:"WID"`
	Amount       wireInt64 `json:"TUA"`
	RuntimeSec   int       `json:"RCT"`
}

type productionWireSlot struct {
	Product productionWireProduct `json:"P"`
	Slot    struct {
		RentalUntil int `json:"RUT"`
		VIP         int `json:"VIP"`
	} `json:"SI"`
}

type productionWireSnapshot struct {
	Active  productionWireProduct `json:"PS"`
	Queued  []productionWireSlot  `json:"QS"`
	Compact [][]json.RawMessage   `json:"PIDL"`
	LineID  int                   `json:"LID"`
}

func reduceProductionSnapshot(
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
	changed, err := applyProductionSnapshot(frame.Payload, castleID, &castle, frame.ReceivedAt)
	if err != nil || !changed {
		return nil, false, err
	}
	gameState.Castles[castleID] = castle
	return []string{"castles", "production"}, true, nil
}

func reduceEmbeddedProductionSnapshots(
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
		return nil, false, fmt.Errorf("decode embedded production snapshots: %w", err)
	}
	changed := false
	for key, raw := range root {
		if !strings.HasPrefix(key, "spl") || key == "spl" || len(raw) == 0 {
			continue
		}
		updated, err := applyProductionSnapshot(raw, castleID, &castle, frame.ReceivedAt)
		if err != nil {
			return nil, false, err
		}
		if updated {
			changed = true
		}
	}
	if !changed {
		return nil, false, nil
	}
	gameState.Castles[castleID] = castle
	return []string{"castles", "production"}, true, nil
}

func applyProductionSnapshot(
	raw json.RawMessage,
	castleID State.CastleID,
	castle *State.CastleState,
	observedAt time.Time,
) (bool, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(raw, &root); err != nil {
		return false, fmt.Errorf("decode production snapshot: %w", err)
	}
	if nested := root["spl"]; len(nested) > 0 {
		raw = nested
	}
	var wire productionWireSnapshot
	if err := json.Unmarshal(raw, &wire); err != nil {
		return false, fmt.Errorf("decode production snapshot fields: %w", err)
	}
	if wire.LineID < 0 || len(root["PS"]) == 0 && len(root["QS"]) == 0 && len(root["PIDL"]) == 0 && len(root["spl"]) == 0 {
		return false, nil
	}
	if nested := root["spl"]; len(nested) > 0 {
		var nestedRoot map[string]json.RawMessage
		_ = json.Unmarshal(nested, &nestedRoot)
		root = nestedRoot
	}
	ensureCastleMaps(castle)
	queue := State.ProductionQueue{
		LineID: wire.LineID, ObservedAt: observedAt, Queued: []State.QueueItem{},
	}
	if item, exists := productionQueueItem(wire.LineID, wire.Active, observedAt, true); exists {
		queue.Active = &item
	}
	if len(root["QS"]) > 0 {
		for _, slot := range wire.Queued {
			if slot.Product.DefinitionID > 0 || slot.Slot.RentalUntil != 0 {
				queue.Capacity++
			}
			if item, exists := productionQueueItem(wire.LineID, slot.Product, observedAt, false); exists {
				queue.Queued = append(queue.Queued, item)
			}
		}
	}
	if len(root["PIDL"]) > 0 {
		queue.Capacity = 0
		queue.Queued = []State.QueueItem{}
		for _, row := range wire.Compact {
			if len(row) == 0 {
				continue
			}
			queue.Capacity++
			product := productionWireProduct{
				DefinitionID: wireInt64(rowInt(row, 0)), Amount: wireInt64(rowInt(row, 1)),
			}
			if len(row) > 2 {
				product.RuntimeSec = int(rowInt(row, 2))
			}
			if item, exists := productionQueueItem(wire.LineID, product, observedAt, false); exists {
				queue.Queued = append(queue.Queued, item)
			}
		}
	}
	if reflect.DeepEqual(castle.Production[wire.LineID], queue) {
		return false, nil
	}
	castle.Production[wire.LineID] = queue
	_ = castleID
	return true, nil
}

func reduceProductionCommandContext(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var command struct {
		SessionKey int `json:"SK"`
	}
	if err := json.Unmarshal(frame.Payload, &command); err != nil {
		return nil, false, fmt.Errorf("decode production command context: %w", err)
	}
	if command.SessionKey <= 0 || gameState.CommandContext.ProductionSessionKey == command.SessionKey {
		return nil, false, nil
	}
	gameState.CommandContext.ProductionSessionKey = command.SessionKey
	observedAt := frame.ReceivedAt
	gameState.CommandContext.ProductionObservedAt = &observedAt
	return []string{"command-context"}, true, nil
}

func productionQueueItem(lineID int, product productionWireProduct, observedAt time.Time, active bool) (State.QueueItem, bool) {
	if product.DefinitionID <= 0 || product.Amount <= 0 {
		return State.QueueItem{}, false
	}
	collection := "units"
	if lineID == 1 {
		collection = "tools"
	}
	item := State.QueueItem{
		Definition: State.DefinitionRef{Collection: collection, ID: int64(product.DefinitionID)},
		Amount:     int64(product.Amount),
	}
	if active {
		startedAt := observedAt
		item.StartedAt = &startedAt
		if product.RuntimeSec > 0 {
			completesAt := observedAt.Add(time.Duration(product.RuntimeSec) * time.Second)
			item.CompletesAt = &completesAt
		}
	}
	return item, true
}

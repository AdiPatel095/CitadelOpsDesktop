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

type productionWireProduct struct {
	DefinitionID wireInt64 `json:"WID"`
	Amount       wireInt64 `json:"TUA"`
	RuntimeSec   int       `json:"RCT"`
}

type productionWireSlot struct {
	Product productionWireProduct `json:"P"`
}

type productionWireSnapshot struct {
	Active productionWireProduct `json:"PS"`
	Queued []productionWireSlot  `json:"QS"`
	LineID int                   `json:"LID"`
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
	var wire productionWireSnapshot
	if err := json.Unmarshal(frame.Payload, &wire); err != nil {
		return nil, false, fmt.Errorf("decode production snapshot: %w", err)
	}
	castleID, castle, ok := focusedCastle(gameState)
	if !ok || wire.LineID < 0 {
		return nil, false, nil
	}
	ensureCastleMaps(&castle)
	queue := State.ProductionQueue{
		LineID: wire.LineID, Capacity: len(wire.Queued), ObservedAt: frame.ReceivedAt,
		Queued: make([]State.QueueItem, 0, len(wire.Queued)),
	}
	if item, exists := productionQueueItem(wire.LineID, wire.Active, frame.ReceivedAt, true); exists {
		queue.Active = &item
	}
	for _, slot := range wire.Queued {
		if item, exists := productionQueueItem(wire.LineID, slot.Product, frame.ReceivedAt, false); exists {
			queue.Queued = append(queue.Queued, item)
		}
	}
	if reflect.DeepEqual(castle.Production[wire.LineID], queue) {
		return nil, false, nil
	}
	castle.Production[wire.LineID] = queue
	gameState.Castles[castleID] = castle
	return []string{"castles", "production"}, true, nil
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

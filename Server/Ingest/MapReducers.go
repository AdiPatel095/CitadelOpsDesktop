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

func reduceMapSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		KingdomID wireInt64           `json:"KID"`
		Nodes     [][]json.RawMessage `json:"AI"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode map snapshot: %w", err)
	}
	kingdomID := State.KingdomID(payload.KingdomID)
	if gameState.Map == nil {
		gameState.Map = map[State.KingdomID]map[string]State.MapObservation{}
	}
	observations := gameState.Map[kingdomID]
	if observations == nil {
		observations = map[string]State.MapObservation{}
	}
	changed := false
	for _, row := range payload.Nodes {
		if len(row) < 3 {
			continue
		}
		typeID := int(rowInt(row, 0))
		x := int(rowInt(row, 1))
		y := int(rowInt(row, 2))
		observation := State.MapObservation{
			KingdomID: kingdomID, X: x, Y: y, TypeID: typeID, ObservedAt: frame.ReceivedAt,
		}
		if len(row) > 3 {
			observation.ObjectID = rowInt(row, 3)
		}
		if len(row) >= 9 {
			observation.OwnerID = State.PlayerID(rowInt(row, 4))
		}
		if len(row) == 20 {
			observation.Level = int(rowInt(row, 5))
		}
		for index := 3; index < len(row); index++ {
			if name := rowString(row, index); name != "" {
				observation.Name = name
				break
			}
		}
		key := fmt.Sprintf("%d:%d", x, y)
		if current, exists := observations[key]; !exists || !reflect.DeepEqual(current, observation) {
			observations[key] = observation
			changed = true
		}
	}
	if changed {
		gameState.Map[kingdomID] = observations
	}
	return []string{"map"}, changed, nil
}

func reduceNestedMapSnapshot(
	ctx context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &root); err != nil {
		return nil, false, fmt.Errorf("decode nested map snapshot: %w", err)
	}
	nested := root["gaa"]
	if len(nested) == 0 {
		return nil, false, nil
	}
	frame.Payload = nested
	return reduceMapSnapshot(ctx, frame, gameState, gameData)
}

package Ingest

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const riftObservationTypeID = 43
const maximumRiftLaunches = 15

func reduceRiftLaunchCapture(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.Direction != Protocol.DirectionOutbound || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var body map[string]json.RawMessage
	if err := json.Unmarshal(frame.Payload, &body); err != nil {
		return nil, false, fmt.Errorf("decode Rift launch: %w", err)
	}
	targetX := rawMapInt(body, "TX")
	targetY := rawMapInt(body, "TY")
	kingdomID := State.KingdomID(rawMapInt(body, "KID"))
	if !knownRiftTarget(gameState, kingdomID, targetX, targetY) || !riftBodyHasLayout(body) {
		return nil, false, nil
	}
	fingerprintBody := map[string]json.RawMessage{}
	for _, key := range []string{"A", "AST", "RW"} {
		if raw := body[key]; len(raw) > 0 {
			fingerprintBody[key] = raw
		}
	}
	canonical, _ := json.Marshal(fingerprintBody)
	digest := sha256.Sum256(canonical)
	id := fmt.Sprintf("rift-%x", digest[:8])
	if deletedAt, deleted := gameState.Rift.DeletedLaunchIDs[id]; deleted {
		if frame.ReceivedAt.UnixMilli() <= deletedAt {
			return nil, false, nil
		}
		delete(gameState.Rift.DeletedLaunchIDs, id)
	}
	var waves []json.RawMessage
	_ = json.Unmarshal(body["A"], &waves)
	existing := gameState.Rift.Launches[id]
	launch := State.RiftLaunch{
		ID: id, DisplayName: existing.DisplayName, SavedAtUnix: existing.SavedAtUnix,
		Body: append([]byte(nil), frame.Payload...), CommanderID: State.CommanderID(rawMapInt(body, "LID")),
		SourceX: rawMapInt(body, "SX"), SourceY: rawMapInt(body, "SY"),
		TargetX: targetX, TargetY: targetY, KingdomID: kingdomID,
		AttackValid: rawMapInt(body, "AV"), WaveCount: len(waves),
		UseTravelFeather: rawMapInt(body, "HBW") == -1 && rawMapInt(body, "PTT") == 1,
		OneWayTTSeconds:  existing.OneWayTTSeconds, LastSuccessAtUnix: existing.LastSuccessAtUnix,
	}
	if launch.SavedAtUnix == 0 {
		launch.SavedAtUnix = frame.ReceivedAt.Unix()
	}
	changed := !reflect.DeepEqual(existing, launch) || gameState.Rift.PendingLaunchID != id
	if !changed {
		return nil, false, nil
	}
	gameState.Rift.Launches[id] = launch
	gameState.Rift.PendingLaunchID = id
	trimRiftLaunches(gameState, id)
	return []string{"rift"}, true, nil
}

func reduceRiftLaunchAck(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || gameState.Rift.PendingLaunchID == "" || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	items, _, err := movementItems(frame.Payload)
	if err != nil {
		return nil, false, nil
	}
	travelSeconds := 0
	for _, raw := range items {
		movement, ok := parseMovement(raw, frame.ReceivedAt, gameData)
		if ok && movement.TravelSeconds > travelSeconds {
			travelSeconds = movement.TravelSeconds
		}
	}
	if travelSeconds <= 0 {
		return nil, false, nil
	}
	id := gameState.Rift.PendingLaunchID
	launch, exists := gameState.Rift.Launches[id]
	if !exists {
		gameState.Rift.PendingLaunchID = ""
		return []string{"rift"}, true, nil
	}
	launch.OneWayTTSeconds = travelSeconds
	launch.LastSuccessAtUnix = frame.ReceivedAt.Unix()
	gameState.Rift.Launches[id] = launch
	gameState.Rift.PendingLaunchID = ""
	return []string{"rift"}, true, nil
}

func combineReducers(reducers ...Reducer) Reducer {
	return func(ctx context.Context, frame Protocol.Frame, gameState *State.GameState, gameData *GameData.Store) ([]string, bool, error) {
		domains := []string{}
		changed := false
		for _, reducer := range reducers {
			if reducer == nil {
				continue
			}
			nextDomains, nextChanged, err := reducer(ctx, frame, gameState, gameData)
			if err != nil {
				return nil, false, err
			}
			domains = append(domains, nextDomains...)
			changed = changed || nextChanged
		}
		return domains, changed, nil
	}
}

func knownRiftTarget(gameState *State.GameState, kingdomID State.KingdomID, x, y int) bool {
	if gameState == nil {
		return false
	}
	observation, found := gameState.LookupMapObservation(kingdomID, fmt.Sprintf("%d:%d", x, y))
	return found && observation.TypeID == riftObservationTypeID && observation.X == x && observation.Y == y
}

func riftBodyHasLayout(body map[string]json.RawMessage) bool {
	for _, key := range []string{"A", "AST", "RW"} {
		if raw := body[key]; len(raw) > 0 && string(raw) != "null" && string(raw) != "[]" && string(raw) != "{}" {
			return true
		}
	}
	return false
}

func trimRiftLaunches(gameState *State.GameState, keepID string) {
	if len(gameState.Rift.Launches) <= maximumRiftLaunches {
		return
	}
	launches := make([]State.RiftLaunch, 0, len(gameState.Rift.Launches))
	for _, launch := range gameState.Rift.Launches {
		launches = append(launches, launch)
	}
	sort.Slice(launches, func(left, right int) bool {
		if launches[left].SavedAtUnix != launches[right].SavedAtUnix {
			return launches[left].SavedAtUnix < launches[right].SavedAtUnix
		}
		return launches[left].ID < launches[right].ID
	})
	for _, launch := range launches {
		if len(gameState.Rift.Launches) <= maximumRiftLaunches {
			return
		}
		if launch.ID != keepID {
			delete(gameState.Rift.Launches, launch.ID)
		}
	}
}

func rawMapInt(values map[string]json.RawMessage, key string) int {
	value, _ := rawInt64(values[key])
	return int(value)
}

package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	nomadCampMapTypeID   = 27
	samuraiCampMapTypeID = 29
	khanCampMapTypeID    = 35
	stormIslandMapTypeID = 24
	stormFortMapTypeID   = 25
)

func reduceInvasionFortification(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		X        wireInt64 `json:"XPOS"`
		Y        wireInt64 `json:"YPOS"`
		Currency string    `json:"RCK"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode invasion fortification: %w", err)
	}
	payload.Currency = strings.ToUpper(strings.TrimSpace(payload.Currency))
	if payload.X < 0 || payload.Y < 0 || !validInvasionFortifyCurrency(payload.Currency) {
		return nil, false, nil
	}
	if gameState.Invasion.FortifiedTargets == nil {
		gameState.Invasion.FortifiedTargets = map[string]string{}
	}
	key := fmt.Sprintf("0:%d:%d", payload.X, payload.Y)
	if gameState.Invasion.FortifiedTargets[key] == payload.Currency {
		return nil, false, nil
	}
	gameState.Invasion.FortifiedTargets[key] = payload.Currency
	return []string{"invasion"}, true, nil
}

func validInvasionFortifyCurrency(currency string) bool {
	switch currency {
	case "GTO", "STO", "KM", "ST", "C2":
		return true
	default:
		return false
	}
}

func reduceInvasionFortificationCounters(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		ResourceCount wireInt64 `json:"RCSC"`
		RubyCount     wireInt64 `json:"RCHC"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode invasion fortification counters: %w", err)
	}
	resourceCount, rubyCount := max(int64(0), int64(payload.ResourceCount)), max(int64(0), int64(payload.RubyCount))
	changed := gameState.Invasion.FortifyResourceCount != resourceCount || gameState.Invasion.FortifyRubyCount != rubyCount
	gameState.Invasion.FortifyResourceCount = resourceCount
	gameState.Invasion.FortifyRubyCount = rubyCount
	if resourceCount == 0 && rubyCount == 0 && len(gameState.Invasion.FortifiedTargets) > 0 {
		gameState.Invasion.FortifiedTargets = map[string]string{}
		changed = true
	}
	if !changed {
		return nil, false, nil
	}
	return []string{"invasion"}, true, nil
}

func reduceMapSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
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
	cooldownChanged := false
	eventCampChanged := false
	stormChanged := false
	beriChanged := false
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
		if len(row) >= 9 && !isRegularEventCampType(typeID) && typeID != khanCampMapTypeID {
			observation.OwnerID = State.PlayerID(rowInt(row, 4))
		}
		if isRegularEventCampType(typeID) {
			populateEventCampObservation(&observation, row, gameData)
		} else if typeID == khanCampMapTypeID {
			populateKhanCampObservation(&observation, row, gameData)
		} else if isStormMapType(typeID) {
			populateStormObservation(&observation, row, gameData)
			labelStormOpportunity(&observation, gameData)
		} else if len(row) > 3 {
			observation.ObjectID = rowInt(row, 3)
		}
		if len(row) == 20 {
			observation.Level = int(rowInt(row, 5))
		}
		populateTowerObservation(&observation, row)
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
		if refreshTowerCooldownFromMap(gameState, observation) {
			cooldownChanged = true
		}
		if refreshNomadCampCooldownFromMap(gameState, observation) {
			eventCampChanged = true
		}
		if refreshTrackedStormOpportunityFromMap(gameState, observation) {
			stormChanged = true
		}
		if invalidateUnavailableBeriTargetFromMap(gameState, observation, row) {
			beriChanged = true
		}
	}
	if changed {
		gameState.Map[kingdomID] = observations
	}
	if cooldownChanged || eventCampChanged || stormChanged || beriChanged {
		changed = true
	}
	domains := []string{"map"}
	if cooldownChanged {
		domains = append(domains, "tower-cooldowns")
	}
	if eventCampChanged {
		domains = append(domains, "nomad-camps")
	}
	if stormChanged {
		domains = append(domains, "storm")
	}
	if beriChanged {
		domains = append(domains, "beri")
	}
	return domains, changed, nil
}

func isRegularEventCampType(typeID int) bool {
	return typeID == nomadCampMapTypeID || typeID == samuraiCampMapTypeID
}

func isStormMapType(typeID int) bool {
	return typeID == stormIslandMapTypeID || typeID == stormFortMapTypeID
}

func populateStormObservation(observation *State.MapObservation, row []json.RawMessage, gameData *GameData.Store) {
	if observation == nil || !isStormMapType(observation.TypeID) {
		return
	}
	if len(row) > 3 {
		observation.ObjectID = rowInt(row, 3)
	}
	switch observation.TypeID {
	case stormIslandMapTypeID:
		if len(row) < 10 {
			return
		}
		observation.StormIsleID = rowInt(row, 8)
		observation.StormCooldownRemaining = boundedWireSeconds(rowInt(row, 9))
	case stormFortMapTypeID:
		if len(row) < 9 {
			return
		}
		observation.StormIsleID = rowInt(row, 5)
		observation.StormVictoryCount = rowInt(row, 7)
		observation.StormCooldownRemaining = boundedWireSeconds(max(rowInt(row, 6), rowInt(row, 8)))
	}
	if gameData == nil {
		return
	}
	definition, found := gameData.StormIsle(observation.StormIsleID)
	if !found {
		return
	}
	observation.Level = definition.Level
	observation.StormKind = definition.Kind
	observation.StormResource = definition.Resource
	observation.StormSize = definition.Size
	observation.StormFixedLoot = definition.FixedLoot
}

func labelStormOpportunity(observation *State.MapObservation, gameData *GameData.Store) {
	if observation == nil || !isStormMapType(observation.TypeID) || observation.StormIsleID <= 0 || observation.ObservedAt.IsZero() {
		return
	}
	observation.StormReadyAt = observation.ObservedAt
	observation.StormExpiresAt = time.Time{}
	timer := time.Duration(max(0, observation.StormCooldownRemaining)) * time.Second
	if observation.TypeID == stormFortMapTypeID {
		observation.StormReadyAt = observation.StormReadyAt.Add(timer)
		return
	}
	if observation.OwnerID > 0 {
		observation.StormReadyAt = observation.StormReadyAt.Add(timer)
		if gameData != nil {
			if definition, found := gameData.StormIsle(observation.StormIsleID); found && definition.GlobalCooldownSec > 0 {
				observation.StormExpiresAt = observation.StormReadyAt.Add(time.Duration(definition.GlobalCooldownSec) * time.Second)
			}
		}
		return
	}
	if timer > 0 {
		observation.StormExpiresAt = observation.ObservedAt.Add(timer)
	}
}

func refreshTrackedStormOpportunityFromMap(gameState *State.GameState, observation State.MapObservation) bool {
	if gameState == nil || observation.KingdomID != State.KingdomID(GameData.StormKingdomID) || gameState.Storm.Map.Targets == nil {
		return false
	}
	key := fmt.Sprintf("%d:%d", observation.X, observation.Y)
	current, tracked := gameState.Storm.Map.Targets[key]
	if !tracked || current.ObservedAt.After(observation.ObservedAt) {
		return false
	}
	if !isStormMapType(observation.TypeID) {
		delete(gameState.Storm.Map.Targets, key)
		return true
	}
	if reflect.DeepEqual(current, observation) {
		return false
	}
	gameState.Storm.Map.Targets[key] = observation
	return true
}

func populateEventCampObservation(observation *State.MapObservation, row []json.RawMessage, gameData *GameData.Store) {
	if observation == nil || !isRegularEventCampType(observation.TypeID) || len(row) < 9 {
		return
	}
	observation.EventCampVictoryCount = rowInt(row, 4)
	observation.EventCampID = rowInt(row, 8)
	observation.ObjectID = observation.EventCampID
	if cooldown := rowInt(row, 5); cooldown > 0 {
		observation.EventCampCooldownRemaining = boundedWireSeconds(cooldown)
	}
	if len(row) >= 12 {
		observation.EventCampBaseWallBonus = rowInt(row, 9)
		observation.EventCampBaseGateBonus = rowInt(row, 10)
		observation.EventCampBaseMoatBonus = rowInt(row, 11)
	}
	if gameData != nil {
		if definition, found := gameData.EventCamp(observation.EventCampID); found {
			observation.Level = definition.CampLevel
		}
	}
}

func populateKhanCampObservation(observation *State.MapObservation, row []json.RawMessage, gameData *GameData.Store) {
	if observation == nil || observation.TypeID != khanCampMapTypeID || len(row) < 13 {
		return
	}
	// Captured Khan rows are [35,X,Y,...,cooldownSec,...,eventCampID,wall,gate,moat].
	observation.EventCampCooldownRemaining = boundedWireSeconds(rowInt(row, 5))
	observation.EventCampID = rowInt(row, 9)
	observation.ObjectID = observation.EventCampID
	observation.EventCampBaseWallBonus = rowInt(row, 10)
	observation.EventCampBaseGateBonus = rowInt(row, 11)
	observation.EventCampBaseMoatBonus = rowInt(row, 12)
	if gameData != nil {
		if definition, found := gameData.EventCamp(observation.EventCampID); found {
			observation.Level = definition.CampLevel
		}
	}
}

func populateTowerObservation(observation *State.MapObservation, row []json.RawMessage) {
	if observation == nil {
		return
	}
	switch {
	case observation.TypeID == towerMapTypeID && len(row) >= 7:
		// Kingdom-tower rows are [2, X, Y, ..., victoryCount, cooldownSec, KID].
		observation.TowerVictoryCount = rowInt(row, 4)
		observation.Level = AttackCapacity.DungeonTargetLevel(observation.TowerVictoryCount, observation.KingdomID)
		observation.TowerCooldownRemaining = boundedWireSeconds(rowInt(row, 5))
	case observation.KingdomID == State.KingdomID(GameData.BerimondKingdomID) &&
		observation.TypeID == AttackCapacity.BerimondTowerMapTypeID && len(row) >= 8:
		// Captured Berimond watchtower rows expose their target level at index 7.
		observation.Level = int(rowInt(row, 7))
	}
}

func invalidateUnavailableBeriTargetFromMap(
	gameState *State.GameState,
	observation State.MapObservation,
	row []json.RawMessage,
) bool {
	if gameState == nil ||
		observation.KingdomID != State.KingdomID(GameData.BerimondKingdomID) ||
		observation.TypeID != AttackCapacity.BerimondTowerMapTypeID ||
		len(row) < 5 || rowInt(row, 4) != 1 {
		return false
	}
	beri := &gameState.Beri
	if beri.TargetObservedAt.IsZero() ||
		!beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) ||
		observation.ObservedAt.Before(beri.TargetObservedAt) ||
		beri.TargetX != observation.X || beri.TargetY != observation.Y ||
		beri.TargetTypeID != observation.TypeID {
		return false
	}
	// Captured Berimond rows switch index 4 from 0 to 1 when the selected
	// watchtower is no longer attackable.
	beri.TargetInvalidatedAt = observation.ObservedAt
	return true
}

func boundedWireSeconds(value int64) int {
	if value <= 0 {
		return 0
	}
	maximum := int64(^uint(0) >> 1)
	if value > maximum {
		return int(maximum)
	}
	return int(value)
}

func reduceDungeonCooldownSkip(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		KingdomID wireInt64         `json:"KID"`
		Node      []json.RawMessage `json:"AI"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode dungeon cooldown reset: %w", err)
	}
	typeID := int(rowInt(payload.Node, 0))
	if typeID != towerMapTypeID && !isRegularEventCampType(typeID) && typeID != khanCampMapTypeID ||
		typeID == towerMapTypeID && len(payload.Node) < 7 ||
		isRegularEventCampType(typeID) && len(payload.Node) < 9 ||
		typeID == khanCampMapTypeID && len(payload.Node) < 13 {
		return nil, false, fmt.Errorf("dungeon cooldown reset did not return a supported target row")
	}
	kingdomID := State.KingdomID(payload.KingdomID)
	kingdomResolved := kingdomID != 0
	x, y := int(rowInt(payload.Node, 1)), int(rowInt(payload.Node, 2))
	if kingdomID == 0 && gameState.NomadCamps.RBCTest != nil &&
		gameState.NomadCamps.RBCTest.TargetX == x && gameState.NomadCamps.RBCTest.TargetY == y {
		kingdomID = gameState.NomadCamps.RBCTest.KingdomID
		kingdomResolved = true
	}
	if kingdomID == 0 && gameState.Khan.RunID != "" && gameState.Khan.TargetX == x && gameState.Khan.TargetY == y {
		kingdomID = gameState.Khan.KingdomID
		kingdomResolved = true
	}
	if kingdomID == 0 && gameState.NomadCamps.LockedTarget != nil &&
		gameState.NomadCamps.LockedTarget.X == x && gameState.NomadCamps.LockedTarget.Y == y {
		kingdomID = gameState.NomadCamps.LockedTarget.KingdomID
		kingdomResolved = true
	}
	if !kingdomResolved {
		for candidateKingdomID, observations := range gameState.Map {
			if _, exists := observations[fmt.Sprintf("%d:%d", x, y)]; exists {
				kingdomID = candidateKingdomID
				break
			}
		}
	}
	observation := State.MapObservation{
		KingdomID: kingdomID, X: x, Y: y, TypeID: typeID, ObservedAt: frame.ReceivedAt,
	}
	if typeID == towerMapTypeID {
		if len(payload.Node) > 3 {
			observation.ObjectID = rowInt(payload.Node, 3)
		}
		populateTowerObservation(&observation, payload.Node)
	} else if typeID == khanCampMapTypeID {
		populateKhanCampObservation(&observation, payload.Node, gameData)
	} else {
		populateEventCampObservation(&observation, payload.Node, gameData)
	}
	if gameState.Map == nil {
		gameState.Map = map[State.KingdomID]map[string]State.MapObservation{}
	}
	if gameState.Map[kingdomID] == nil {
		gameState.Map[kingdomID] = map[string]State.MapObservation{}
	}
	key := fmt.Sprintf("%d:%d", x, y)
	mapChanged := !reflect.DeepEqual(gameState.Map[kingdomID][key], observation)
	gameState.Map[kingdomID][key] = observation
	if typeID == towerMapTypeID {
		cooldownChanged := refreshTowerCooldownFromMap(gameState, observation)
		testChanged := recordRBCTestCooldownSkip(gameState, observation, frame.ReceivedAt)
		domains := []string{"map", "tower-cooldowns"}
		if testChanged {
			domains = append(domains, "nomad-camps")
		}
		return domains, mapChanged || cooldownChanged || testChanged, nil
	}
	if typeID == khanCampMapTypeID {
		cooldownChanged := refreshNomadCampCooldownFromMap(gameState, observation)
		return []string{"map", "khan", "nomad-camps"}, mapChanged || cooldownChanged, nil
	}
	cooldownChanged := refreshNomadCampCooldownFromMap(gameState, observation)
	return []string{"map", "nomad-camps"}, mapChanged || cooldownChanged, nil
}

func recordRBCTestCooldownSkip(gameState *State.GameState, observation State.MapObservation, observedAt time.Time) bool {
	test := gameState.NomadCamps.RBCTest
	if test == nil || observation.TypeID != towerMapTypeID || observation.KingdomID != test.KingdomID ||
		observation.X != test.TargetX || observation.Y != test.TargetY || observation.TowerCooldownRemaining != 0 ||
		test.CooldownsSkipped >= test.AttacksLaunched {
		return false
	}
	test.CooldownsSkipped++
	test.LastCooldownSkippedAt = observedAt
	return true
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
	domains, changed, err := combineReducers(reduceMapSnapshot, reducePlayerProtectionMode)(ctx, frame, gameState, gameData)
	if err != nil || frame.Opcode != "fnt" {
		return domains, changed, err
	}
	var mapPayload struct {
		KingdomID wireInt64           `json:"KID"`
		Nodes     [][]json.RawMessage `json:"AI"`
	}
	if json.Unmarshal(nested, &mapPayload) != nil || State.KingdomID(mapPayload.KingdomID) != State.KingdomID(GameData.BerimondKingdomID) ||
		len(mapPayload.Nodes) == 0 {
		return domains, changed, nil
	}
	targetX, hasX := rawInt64(root["X"])
	targetY, hasY := rawInt64(root["Y"])
	selected := mapPayload.Nodes[0]
	if hasX && hasY {
		for _, row := range mapPayload.Nodes {
			if rowInt(row, 1) == targetX && rowInt(row, 2) == targetY {
				selected = row
				break
			}
		}
	}
	if len(selected) < 3 {
		return domains, changed, nil
	}
	targetX = rowInt(selected, 1)
	targetY = rowInt(selected, 2)
	targetTypeID := int(rowInt(selected, 0))
	observation, exists := gameState.Map[State.KingdomID(GameData.BerimondKingdomID)][fmt.Sprintf("%d:%d", targetX, targetY)]
	if !exists || targetTypeID != AttackCapacity.BerimondTowerMapTypeID ||
		observation.TypeID != targetTypeID || observation.Level <= 0 {
		return domains, changed, nil
	}
	targetChanged := gameState.Beri.TargetX != int(targetX) || gameState.Beri.TargetY != int(targetY) ||
		gameState.Beri.TargetTypeID != targetTypeID || !gameState.Beri.TargetObservedAt.Equal(frame.ReceivedAt)
	gameState.Beri.TargetX = int(targetX)
	gameState.Beri.TargetY = int(targetY)
	gameState.Beri.TargetTypeID = targetTypeID
	gameState.Beri.TargetObservedAt = frame.ReceivedAt
	if targetChanged {
		domains = append(domains, "beri")
	}
	return domains, changed || targetChanged, nil
}

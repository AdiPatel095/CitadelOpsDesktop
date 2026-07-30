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

// reduceAttackDialog records the selected source, target map row, and active
// effects supplied by the game's pre-attack dialog (ADI). The effect rows are
// intentionally stored as delivered rather than inferred from static data:
// ADI includes temporary and castle-context bonuses that may not have a local
// state representation.
func reduceAttackDialog(
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
		return nil, false, fmt.Errorf("decode attack dialog: %w", err)
	}
	dialog := State.AttackDialogState{
		SourceCastleID: State.CastleID(rawInteger(root["SCID"])),
		KingdomID:      State.KingdomID(rawInteger(root["KID"])),
		ActiveEffects:  parseAttackDialogEffects(root["AE"]),
		ObservedAt:     frame.ReceivedAt,
	}
	var khanObservation *State.MapObservation
	var stormObservation *State.MapObservation
	if rawTarget, exists := root["gaa"]; exists {
		var nested struct {
			Node json.RawMessage `json:"AI"`
		}
		var row []json.RawMessage
		if json.Unmarshal(rawTarget, &nested) == nil && json.Unmarshal(nested.Node, &row) == nil {
			dialog.Target = State.AttackDialogTarget{
				TypeID: int(rowInt(row, 0)), X: int(rowInt(row, 1)), Y: int(rowInt(row, 2)), ObjectID: rowInt(row, 3),
			}
			if len(row) >= 9 && !isRegularEventCampType(dialog.Target.TypeID) {
				dialog.Target.OwnerID = State.PlayerID(rowInt(row, 4))
			}
			if dialog.Target.TypeID == towerMapTypeID && len(row) >= 7 {
				dialog.Target.TowerVictoryCount = rowInt(row, 4)
				dialog.Target.TowerCooldownRemaining = boundedWireSeconds(rowInt(row, 5))
			}
			if isRegularEventCampType(dialog.Target.TypeID) {
				observation := State.MapObservation{TypeID: dialog.Target.TypeID}
				populateEventCampObservation(&observation, row, gameData)
				dialog.Target.ObjectID = observation.EventCampID
				dialog.Target.EventCampID = observation.EventCampID
				dialog.Target.EventCampVictoryCount = observation.EventCampVictoryCount
				dialog.Target.EventCampCooldownRemaining = observation.EventCampCooldownRemaining
			}
			if dialog.Target.TypeID == khanCampMapTypeID {
				observation := State.MapObservation{
					KingdomID: dialog.KingdomID, X: dialog.Target.X, Y: dialog.Target.Y, TypeID: dialog.Target.TypeID,
					ObservedAt: frame.ReceivedAt,
				}
				populateKhanCampObservation(&observation, row, gameData)
				dialog.Target.ObjectID = observation.EventCampID
				dialog.Target.EventCampID = observation.EventCampID
				dialog.Target.EventCampCooldownRemaining = observation.EventCampCooldownRemaining
				khanObservation = &observation
			}
			if isStormMapType(dialog.Target.TypeID) {
				observation := State.MapObservation{
					KingdomID: dialog.KingdomID, X: dialog.Target.X, Y: dialog.Target.Y, TypeID: dialog.Target.TypeID,
					OwnerID: dialog.Target.OwnerID, ObservedAt: frame.ReceivedAt,
				}
				populateStormObservation(&observation, row, gameData)
				labelStormOpportunity(&observation, gameData)
				dialog.Target.ObjectID = observation.ObjectID
				dialog.Target.OwnerID = observation.OwnerID
				dialog.Target.StormIsleID = observation.StormIsleID
				dialog.Target.StormVictoryCount = observation.StormVictoryCount
				dialog.Target.StormCooldownRemaining = observation.StormCooldownRemaining
				dialog.Target.StormReadyAt = observation.StormReadyAt
				dialog.Target.StormExpiresAt = observation.StormExpiresAt
				stormObservation = &observation
			}
		}
	}
	if dialog.SourceCastleID <= 0 {
		return nil, false, nil
	}
	domains := []string{"attack_dialog"}
	changed := false
	if !reflect.DeepEqual(gameState.AttackDialog, dialog) {
		gameState.AttackDialog = dialog
		changed = true
	}
	if khanObservation != nil {
		if gameState.Map == nil {
			gameState.Map = map[State.KingdomID]map[string]State.MapObservation{}
		}
		observations := gameState.Map[khanObservation.KingdomID]
		if observations == nil {
			observations = map[string]State.MapObservation{}
			gameState.Map[khanObservation.KingdomID] = observations
		}
		key := fmt.Sprintf("%d:%d", khanObservation.X, khanObservation.Y)
		if current, exists := observations[key]; exists {
			if khanObservation.Name == "" {
				khanObservation.Name = current.Name
			}
			if khanObservation.Level == 0 {
				khanObservation.Level = current.Level
			}
		}
		if current, exists := observations[key]; !exists || !reflect.DeepEqual(current, *khanObservation) {
			observations[key] = *khanObservation
			changed = true
			domains = append(domains, "map")
		}
		if refreshNomadCampCooldownFromMap(gameState, *khanObservation) {
			changed = true
			domains = append(domains, "nomad-camps")
		}
	}
	if stormObservation != nil {
		if gameState.Map == nil {
			gameState.Map = map[State.KingdomID]map[string]State.MapObservation{}
		}
		observations := gameState.Map[stormObservation.KingdomID]
		if observations == nil {
			observations = map[string]State.MapObservation{}
			gameState.Map[stormObservation.KingdomID] = observations
		}
		key := fmt.Sprintf("%d:%d", stormObservation.X, stormObservation.Y)
		if current, exists := observations[key]; !exists || !reflect.DeepEqual(current, *stormObservation) {
			observations[key] = *stormObservation
			changed = true
			domains = append(domains, "map")
		}
		if current, tracked := gameState.Storm.Map.Targets[key]; tracked && !reflect.DeepEqual(current, *stormObservation) {
			gameState.Storm.Map.Targets[key] = *stormObservation
			changed = true
			domains = append(domains, "storm")
		}
	}
	if !changed {
		return nil, false, nil
	}
	return domains, true, nil
}

func parseAttackDialogEffects(raw json.RawMessage) []State.AttackDialogEffect {
	rows, ok := decodeRows(raw)
	if !ok {
		return []State.AttackDialogEffect{}
	}
	effects := make([]State.AttackDialogEffect, 0, len(rows))
	for _, row := range rows {
		effectID := rowInt(row, 0)
		if effectID <= 0 {
			continue
		}
		var rawValues []json.RawMessage
		if len(row) < 2 || json.Unmarshal(row[1], &rawValues) != nil {
			continue
		}
		values := make([]float64, 0, len(rawValues))
		for _, rawValue := range rawValues {
			if value, ok := rawFloat64(rawValue); ok {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			continue
		}
		effects = append(effects, State.AttackDialogEffect{
			EffectID: effectID, Values: values, Source: rowString(row, 2),
		})
	}
	return effects
}

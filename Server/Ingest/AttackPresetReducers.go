package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// reduceAttackPresets stores GAS's saved formations in a typed form. GAS is
// deliberately not an attack-capacity input: it describes requested troop and
// tool stacks, while capacity remains target and effect dependent.
func reduceAttackPresets(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	var payload struct {
		Presets []struct {
			Formation string    `json:"A"`
			Slot      wireInt64 `json:"S"`
			Name      string    `json:"SN"`
		} `json:"S"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode attack presets: %w", err)
	}
	presets := make([]State.AttackPreset, 0, len(payload.Presets))
	for _, rawPreset := range payload.Presets {
		preset, ok := parseAttackPreset(int(rawPreset.Slot), rawPreset.Name, rawPreset.Formation)
		if ok {
			presets = append(presets, preset)
		}
	}
	sort.Slice(presets, func(left, right int) bool { return presets[left].Slot < presets[right].Slot })
	if reflect.DeepEqual(gameState.AttackPresets, presets) {
		return nil, false, nil
	}
	gameState.AttackPresets = presets
	return []string{"attack_presets"}, true, nil
}

func parseAttackPreset(slot int, name string, encoded string) (State.AttackPreset, bool) {
	var lanes [][]json.RawMessage
	if json.Unmarshal([]byte(encoded), &lanes) != nil || len(lanes) != 6 {
		return State.AttackPreset{}, false
	}
	preset := State.AttackPreset{Slot: slot, Name: name}
	for lane := 0; lane < 3; lane++ {
		preset.Units[lane] = parseAttackPresetLane(lanes[lane])
		preset.Tools[lane] = parseAttackPresetLane(lanes[lane+3])
	}
	return preset, true
}

func parseAttackPresetLane(values []json.RawMessage) []State.AttackPresetStack {
	stacks := make([]State.AttackPresetStack, 0, len(values)/2)
	for index := 0; index+1 < len(values); index += 2 {
		definitionID := rowInt(values, index)
		amount := rowInt(values, index+1)
		if definitionID <= 0 || amount <= 0 {
			continue
		}
		stacks = append(stacks, State.AttackPresetStack{DefinitionID: definitionID, Amount: amount})
	}
	return stacks
}

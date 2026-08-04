package AttackPresets

import (
	"encoding/json"
	"fmt"
	"strings"

	"CitadelDesktop/Server/AttackCapacity"
)

const (
	ConfigurationSection = "attacks.presets"
	MaximumWaves         = 30
	CourtyardTroopSlots  = 8
	CourtyardToolSlots   = 3
	TargetTypePvE        = "pve"
	TargetTypePvP        = "pvp"
)

type Document struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

type Preset struct {
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	TargetType       string           `json:"targetType,omitempty"`
	Waves            []Wave           `json:"waves"`
	CourtyardSupport CourtyardSupport `json:"courtyardSupport"`
	CreatedAt        string           `json:"createdAt,omitempty"`
	UpdatedAt        string           `json:"updatedAt,omitempty"`
}

type Wave struct {
	Left   Lane `json:"L"`
	Middle Lane `json:"M"`
	Right  Lane `json:"R"`
}

type Lane struct {
	Troops []Slot `json:"troops"`
	Tools  []Slot `json:"tools"`
}

type CourtyardSupport struct {
	Troops []Slot `json:"troops"`
	Tools  []Slot `json:"tools"`
}

type Slot struct {
	ItemID   *int64 `json:"itemId"`
	Quantity int64  `json:"quantity"`
}

func Decode(raw json.RawMessage) (Document, error) {
	if len(raw) == 0 {
		return Document{Version: 1, Presets: []Preset{}}, nil
	}
	var document Document
	if err := json.Unmarshal(raw, &document); err != nil {
		return Document{}, fmt.Errorf("decode attack presets: %w", err)
	}
	if document.Version != 1 {
		return Document{}, fmt.Errorf("unsupported attack preset document version %d", document.Version)
	}
	for index := range document.Presets {
		if err := validatePreset(document.Presets[index]); err != nil {
			return Document{}, fmt.Errorf("attack preset %d: %w", index+1, err)
		}
	}
	return document, nil
}

func Find(document Document, id string) (Preset, bool) {
	id = strings.TrimSpace(id)
	for _, preset := range document.Presets {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}

func Validate(preset Preset) error {
	return validatePreset(preset)
}

// LimitToCapacity applies the same wave, lane, and courtyard-support caps used
// immediately before building a CRA payload. Troop and tool slots are consumed
// in their saved priority order for every retained wave.
func LimitToCapacity(preset Preset, capacity AttackCapacity.Result) Preset {
	result := preset
	result.CourtyardSupport = CourtyardSupport{
		Troops: limitTroopSlotsToCapacity(preset.CourtyardSupport.Troops, capacity.SupportCapacity),
		Tools:  append([]Slot(nil), preset.CourtyardSupport.Tools...),
	}
	waveCount := min(len(preset.Waves), max(0, capacity.MaximumWaves))
	result.Waves = make([]Wave, waveCount)
	for index, wave := range preset.Waves[:waveCount] {
		result.Waves[index] = Wave{
			Left:   limitLaneToCapacity(wave.Left, capacity.Capacity.Left, capacity.ToolCapacity.Left),
			Middle: limitLaneToCapacity(wave.Middle, capacity.Capacity.Front, capacity.ToolCapacity.Front),
			Right:  limitLaneToCapacity(wave.Right, capacity.Capacity.Right, capacity.ToolCapacity.Right),
		}
	}
	return result
}

func limitLaneToCapacity(lane Lane, troopCapacity int64, toolCapacity int64) Lane {
	return Lane{
		Troops: limitSlotsToCapacity(lane.Troops, troopCapacity),
		Tools:  limitSlotsToCapacity(lane.Tools, toolCapacity),
	}
}

func limitTroopSlotsToCapacity(slots []Slot, capacity int64) []Slot {
	return limitSlotsToCapacity(slots, capacity)
}

func limitSlotsToCapacity(slots []Slot, capacity int64) []Slot {
	result := append([]Slot(nil), slots...)
	remaining := max(int64(0), capacity)
	for index := range result {
		quantity := result[index].Quantity
		if quantity <= 0 {
			continue
		}
		result[index].Quantity = min(quantity, remaining)
		remaining -= result[index].Quantity
	}
	return result
}

func validatePreset(preset Preset) error {
	if strings.TrimSpace(preset.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(preset.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if preset.TargetType != "" && preset.TargetType != TargetTypePvE && preset.TargetType != TargetTypePvP {
		return fmt.Errorf("targetType must be %q or %q", TargetTypePvE, TargetTypePvP)
	}
	if len(preset.Waves) < 1 || len(preset.Waves) > MaximumWaves {
		return fmt.Errorf("must contain between 1 and %d waves", MaximumWaves)
	}
	for waveIndex, wave := range preset.Waves {
		for label, lane := range map[string]Lane{"left": wave.Left, "middle": wave.Middle, "right": wave.Right} {
			unitCapacity, toolCapacity := 2, 2
			if label == "middle" {
				unitCapacity, toolCapacity = 6, 3
			}
			if len(lane.Troops) > unitCapacity || len(lane.Tools) > toolCapacity {
				return fmt.Errorf("wave %d %s lane exceeds supported slots", waveIndex+1, label)
			}
			for _, slots := range [][]Slot{lane.Troops, lane.Tools} {
				for _, slot := range slots {
					if slot.Quantity < 0 || (slot.ItemID != nil && *slot.ItemID <= 0) || (slot.ItemID == nil && slot.Quantity > 0) {
						return fmt.Errorf("wave %d %s lane contains an invalid allocation", waveIndex+1, label)
					}
				}
			}
		}
	}
	if len(preset.CourtyardSupport.Troops) > CourtyardTroopSlots {
		return fmt.Errorf("courtyard support exceeds %d troop slots", CourtyardTroopSlots)
	}
	if len(preset.CourtyardSupport.Tools) > CourtyardToolSlots {
		return fmt.Errorf("courtyard support exceeds %d Sceat tool slots", CourtyardToolSlots)
	}
	for _, slot := range preset.CourtyardSupport.Troops {
		if slot.Quantity < 0 || (slot.ItemID != nil && *slot.ItemID <= 0) || (slot.ItemID == nil && slot.Quantity > 0) {
			return fmt.Errorf("courtyard support contains an invalid troop allocation")
		}
	}
	for _, slot := range preset.CourtyardSupport.Tools {
		if slot.ItemID == nil {
			if slot.Quantity != 0 {
				return fmt.Errorf("courtyard support contains a Sceat tool quantity without an item")
			}
			continue
		}
		if *slot.ItemID <= 0 || slot.Quantity != 1 {
			return fmt.Errorf("each courtyard Sceat tool slot must contain exactly one valid item")
		}
	}
	return nil
}

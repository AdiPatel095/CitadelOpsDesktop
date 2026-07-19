package AttackPresets

import (
	"encoding/json"
	"fmt"
	"strings"
)

const ConfigurationSection = "attacks.presets"

type Document struct {
	Version int      `json:"version"`
	Presets []Preset `json:"presets"`
}

type Preset struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Waves     []Wave `json:"waves"`
	CreatedAt string `json:"createdAt,omitempty"`
	UpdatedAt string `json:"updatedAt,omitempty"`
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

func validatePreset(preset Preset) error {
	if strings.TrimSpace(preset.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(preset.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if len(preset.Waves) < 1 || len(preset.Waves) > 10 {
		return fmt.Errorf("must contain between 1 and 10 waves")
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
	return nil
}

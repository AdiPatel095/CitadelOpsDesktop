package AttackPresets

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/AttackCapacity"
)

func TestDecodePersistsOptionalTroopFamilyModeAndDefaultsLegacyPresetsOff(t *testing.T) {
	raw := json.RawMessage(`{
		"version":1,
		"presets":[
			{"id":"family","name":"Family","useTroopFamilies":true,"waves":[{"L":{"troops":[],"tools":[]},"M":{"troops":[],"tools":[]},"R":{"troops":[],"tools":[]}}]},
			{"id":"legacy","name":"Legacy","waves":[{"L":{"troops":[],"tools":[]},"M":{"troops":[],"tools":[]},"R":{"troops":[],"tools":[]}}]}
		]
	}`)
	document, err := Decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !document.Presets[0].UseTroopFamilies || document.Presets[1].UseTroopFamilies {
		t.Fatalf("decoded family modes = %t/%t, want true/false", document.Presets[0].UseTroopFamilies, document.Presets[1].UseTroopFamilies)
	}
}

func TestLimitToCapacityKeepsPresetPriorityAndCapsRetainedWaveTools(t *testing.T) {
	first, second, later := int64(216), int64(217), int64(218)
	tool, secondTool := int64(104), int64(105)
	supportTroop, secondSupportTroop, supportTool := int64(219), int64(220), int64(390)
	preset := Preset{
		ID: "trial", Name: "Trial",
		CourtyardSupport: CourtyardSupport{
			Troops: []Slot{
				{ItemID: &supportTroop, Quantity: 200},
				{ItemID: &secondSupportTroop, Quantity: 200},
			},
			Tools: []Slot{{ItemID: &supportTool, Quantity: 1}},
		},
		Waves: []Wave{
			{
				Left: Lane{
					Troops: []Slot{{ItemID: &first, Quantity: 50}, {ItemID: &second, Quantity: 50}},
					Tools:  []Slot{{ItemID: &tool, Quantity: 25}, {ItemID: &secondTool, Quantity: 25}},
				},
			},
			{Middle: Lane{Troops: []Slot{{ItemID: &later, Quantity: 10}}}},
		},
	}

	limited := LimitToCapacity(preset, AttackCapacity.Result{
		Capacity:     AttackCapacity.LaneCapacity{Left: 64, Front: 192, Right: 64},
		ToolCapacity: AttackCapacity.LaneCapacity{Left: 30, Front: 40, Right: 30},
		MaximumWaves: 1, SupportCapacity: 250,
	})
	if len(limited.Waves) != 1 {
		t.Fatalf("wave limit = %d, want 1", len(limited.Waves))
	}
	left := limited.Waves[0].Left
	if left.Troops[0].Quantity != 50 || left.Troops[1].Quantity != 14 {
		t.Fatalf("unexpected limited left flank: %#v", left.Troops)
	}
	if len(left.Tools) != 2 || left.Tools[0].Quantity != 25 || left.Tools[1].Quantity != 5 {
		t.Fatalf("retained wave tools were not capped in preset order: %#v", left.Tools)
	}
	if len(limited.CourtyardSupport.Troops) != 2 ||
		limited.CourtyardSupport.Troops[0].Quantity != 200 ||
		limited.CourtyardSupport.Troops[1].Quantity != 50 ||
		len(limited.CourtyardSupport.Tools) != 1 || limited.CourtyardSupport.Tools[0].Quantity != 1 {
		t.Fatalf("courtyard support changed: %#v", limited.CourtyardSupport)
	}
	if preset.Waves[0].Left.Troops[1].Quantity != 50 ||
		preset.Waves[0].Left.Tools[1].Quantity != 25 || len(preset.Waves) != 2 {
		t.Fatal("capacity limiting mutated the saved preset")
	}
	if preset.CourtyardSupport.Troops[0].Quantity != 200 || preset.CourtyardSupport.Troops[1].Quantity != 200 {
		t.Fatal("support capacity limiting mutated the saved preset")
	}
}

func TestValidatePresetTargetType(t *testing.T) {
	troop := int64(216)
	preset := Preset{
		ID: "trial", Name: "Trial", TargetType: TargetTypePvP,
		Waves: []Wave{{Middle: Lane{Troops: []Slot{{ItemID: &troop, Quantity: 1}}}}},
	}
	if err := Validate(preset); err != nil {
		t.Fatalf("valid PvP target type: %v", err)
	}
	preset.TargetType = "unknown"
	if err := Validate(preset); err == nil {
		t.Fatal("unknown preset target type was accepted")
	}
	preset.TargetType = ""
	if err := Validate(preset); err != nil {
		t.Fatalf("legacy preset without target type: %v", err)
	}
}

func TestValidateCourtyardSupportSlots(t *testing.T) {
	troop, tool := int64(216), int64(390)
	preset := Preset{
		ID: "trial", Name: "Trial",
		Waves: []Wave{{Middle: Lane{Troops: []Slot{{ItemID: &troop, Quantity: 1}}}}},
		CourtyardSupport: CourtyardSupport{
			Troops: []Slot{{ItemID: &troop, Quantity: 100}},
			Tools:  []Slot{{ItemID: &tool, Quantity: 1}},
		},
	}
	if err := Validate(preset); err != nil {
		t.Fatalf("valid courtyard support: %v", err)
	}
	preset.CourtyardSupport.Tools[0].Quantity = 2
	if err := Validate(preset); err == nil {
		t.Fatal("two items in one Sceat support slot were accepted")
	}
}

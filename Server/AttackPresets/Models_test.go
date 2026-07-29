package AttackPresets

import (
	"testing"

	"CitadelDesktop/Server/AttackCapacity"
)

func TestLimitToCapacityKeepsPresetPriorityAndRetainedWaveTools(t *testing.T) {
	first, second, later := int64(216), int64(217), int64(218)
	tool := int64(104)
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
					Tools:  []Slot{{ItemID: &tool, Quantity: 25}},
				},
			},
			{Middle: Lane{Troops: []Slot{{ItemID: &later, Quantity: 10}}}},
		},
	}

	limited := LimitToCapacity(preset, AttackCapacity.Result{
		Capacity:     AttackCapacity.LaneCapacity{Left: 64, Front: 192, Right: 64},
		MaximumWaves: 1, SupportCapacity: 250,
	})
	if len(limited.Waves) != 1 {
		t.Fatalf("wave limit = %d, want 1", len(limited.Waves))
	}
	left := limited.Waves[0].Left
	if left.Troops[0].Quantity != 50 || left.Troops[1].Quantity != 14 {
		t.Fatalf("unexpected limited left flank: %#v", left.Troops)
	}
	if len(left.Tools) != 1 || left.Tools[0].Quantity != 25 {
		t.Fatalf("retained wave tools changed: %#v", left.Tools)
	}
	if len(limited.CourtyardSupport.Troops) != 2 ||
		limited.CourtyardSupport.Troops[0].Quantity != 200 ||
		limited.CourtyardSupport.Troops[1].Quantity != 50 ||
		len(limited.CourtyardSupport.Tools) != 1 || limited.CourtyardSupport.Tools[0].Quantity != 1 {
		t.Fatalf("courtyard support changed: %#v", limited.CourtyardSupport)
	}
	if preset.Waves[0].Left.Troops[1].Quantity != 50 || len(preset.Waves) != 2 {
		t.Fatal("capacity limiting mutated the saved preset")
	}
	if preset.CourtyardSupport.Troops[0].Quantity != 200 || preset.CourtyardSupport.Troops[1].Quantity != 200 {
		t.Fatal("support capacity limiting mutated the saved preset")
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

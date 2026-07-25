package AttackPresets

import (
	"testing"

	"CitadelDesktop/Server/AttackCapacity"
)

func TestLimitToCapacityKeepsPresetPriorityAndRetainedWaveTools(t *testing.T) {
	first, second, later := int64(216), int64(217), int64(218)
	tool := int64(104)
	preset := Preset{ID: "trial", Name: "Trial", Waves: []Wave{
		{
			Left: Lane{
				Troops: []Slot{{ItemID: &first, Quantity: 50}, {ItemID: &second, Quantity: 50}},
				Tools:  []Slot{{ItemID: &tool, Quantity: 25}},
			},
		},
		{Middle: Lane{Troops: []Slot{{ItemID: &later, Quantity: 10}}}},
	}}

	limited := LimitToCapacity(preset, AttackCapacity.LaneCapacity{Left: 64, Front: 192, Right: 64}, 1)
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
	if preset.Waves[0].Left.Troops[1].Quantity != 50 || len(preset.Waves) != 2 {
		t.Fatal("capacity limiting mutated the saved preset")
	}
}

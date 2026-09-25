package Automation

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func beriCompositionGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[
			{"wodID":10,"foodSupply":1,"upgradeWodID":11},
			{"wodID":11,"foodSupply":1,"downgradeWodID":10},
			{"wodID":12,"foodSupply":1}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func beriMixedPreset() AttackPresets.Preset {
	a, b := int64(10), int64(11)
	return AttackPresets.Preset{ID: "mixed", Name: "Mixed", Waves: []AttackPresets.Wave{
		{Left: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &a, Quantity: 1}}},
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &a, Quantity: 1}}}},
		{Right: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &b, Quantity: 1}}}},
	}}
}

func TestBeriProportionalTransferCompletesMixedPresetRatio(t *testing.T) {
	gameData := beriCompositionGameData(t)
	preset := beriMixedPreset()
	donor := map[State.UnitID]int64{10: 10, 11: 10}
	capacity := State.BeriState{AvailableTroops: 6}
	id, amount, reason := beriProportionalTransfer(preset, donor, nil, capacity, gameData)
	if reason != "" || id != 10 || amount != 4 {
		t.Fatalf("first transfer = %d/%d %q", id, amount, reason)
	}
	capacity.AvailableTroops = 2
	id, amount, reason = beriProportionalTransfer(preset, donor, map[State.UnitID]int64{10: 4}, capacity, gameData)
	if reason != "" || id != 11 || amount != 2 {
		t.Fatalf("second transfer = %d/%d %q", id, amount, reason)
	}
}

func TestBeriProportionalTransferRoundsAndRespectsExistingInventory(t *testing.T) {
	gameData := beriCompositionGameData(t)
	preset := beriMixedPreset()
	donor := map[State.UnitID]int64{10: 10, 11: 10}
	id, amount, reason := beriProportionalTransfer(preset, donor, nil, State.BeriState{AvailableTroops: 5}, gameData)
	if reason != "" || id != 11 || amount != 2 {
		t.Fatalf("largest-remainder transfer = %d/%d %q", id, amount, reason)
	}
	id, amount, reason = beriProportionalTransfer(preset, donor,
		map[State.UnitID]int64{10: 10}, State.BeriState{AvailableTroops: 4}, gameData)
	if reason != "" || id != 11 || amount != 4 {
		t.Fatalf("imbalanced inventory transfer = %d/%d %q", id, amount, reason)
	}
}

func TestBeriProportionalTransferRespectsDonorAndPerUnitCapacity(t *testing.T) {
	gameData := beriCompositionGameData(t)
	preset := beriMixedPreset()
	id, amount, reason := beriProportionalTransfer(preset, map[State.UnitID]int64{10: 10, 11: 0}, nil,
		State.BeriState{AvailableTroops: 5}, gameData)
	if id != 0 || amount != 0 || !strings.Contains(reason, "donor troops") {
		t.Fatalf("missing donor = %d/%d %q", id, amount, reason)
	}
	id, amount, reason = beriProportionalTransfer(preset, map[State.UnitID]int64{10: 10, 11: 10}, nil,
		State.BeriState{AvailableTroops: 6, TroopsByUnit: map[State.UnitID]int64{10: 1}}, gameData)
	if reason != "" || id != 10 || amount != 1 {
		t.Fatalf("per-unit cap = %d/%d %q", id, amount, reason)
	}
}

func TestBeriProportionalTransferResolvesFamiliesOrWaits(t *testing.T) {
	gameData := beriCompositionGameData(t)
	anchor := int64(10)
	preset := AttackPresets.Preset{UseTroopFamilies: true, Waves: []AttackPresets.Wave{{Middle: AttackPresets.Lane{
		Troops: []AttackPresets.Slot{{ItemID: &anchor, Quantity: 3}},
	}}}}
	id, amount, reason := beriProportionalTransfer(preset, map[State.UnitID]int64{11: 3}, nil,
		State.BeriState{AvailableTroops: 3}, gameData)
	if reason != "" || id != 11 || amount != 3 {
		t.Fatalf("family resolution = %d/%d %q", id, amount, reason)
	}
	_, _, reason = beriProportionalTransfer(preset, nil, nil, State.BeriState{AvailableTroops: 3}, gameData)
	if !strings.Contains(reason, "resolvable") {
		t.Fatalf("family shortage reason = %q", reason)
	}
}

func TestBeriProportionalTransferFailsClosedOnMissingMetadata(t *testing.T) {
	id := int64(99)
	preset := AttackPresets.Preset{Waves: []AttackPresets.Wave{{Middle: AttackPresets.Lane{
		Troops: []AttackPresets.Slot{{ItemID: &id, Quantity: 1}},
	}}}}
	_, _, reason := beriProportionalTransfer(preset, map[State.UnitID]int64{99: 10}, nil,
		State.BeriState{AvailableTroops: 10}, beriCompositionGameData(t))
	if !strings.Contains(reason, "official provision data") {
		t.Fatalf("missing metadata reason = %q", reason)
	}
}

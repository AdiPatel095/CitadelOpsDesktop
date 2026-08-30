package AttackPresets

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestResolveTroopFamiliesKeepsPartialHigherTierThenFillsFromLowerTier(t *testing.T) {
	gameData := troopFamilyTestGameData(t)
	anchor := int64(3)
	preset := Preset{
		ID: "family", Name: "Family", UseTroopFamilies: true,
		Waves: []Wave{{Middle: Lane{Troops: []Slot{{ItemID: &anchor, Quantity: 100}}}}},
	}
	resolved, err := ResolveTroopFamilies(preset, map[State.UnitID]int64{3: 60, 4: 40}, gameData, 1)
	if err != nil {
		t.Fatal(err)
	}
	troops := resolved.Waves[0].Middle.Troops
	if len(troops) != 2 || *troops[0].ItemID != 4 || troops[0].Quantity != 40 ||
		*troops[1].ItemID != 3 || troops[1].Quantity != 60 {
		t.Fatalf("resolved family troops = %#v, want 40 tier-4 then 60 tier-3", troops)
	}
	if resolved.UseTroopFamilies {
		t.Fatal("materialized preset remained family-enabled")
	}
	if !preset.UseTroopFamilies || len(preset.Waves[0].Middle.Troops) != 1 {
		t.Fatal("family resolution mutated the saved preset")
	}
}

func TestResolveTroopFamiliesDividesStockAcrossIdenticalAttackCopies(t *testing.T) {
	gameData := troopFamilyTestGameData(t)
	anchor := int64(4)
	preset := Preset{
		ID: "family", Name: "Family", UseTroopFamilies: true,
		Waves: []Wave{{Middle: Lane{Troops: []Slot{{ItemID: &anchor, Quantity: 100}}}}},
	}
	resolved, err := ResolveTroopFamilies(preset, map[State.UnitID]int64{3: 50, 4: 150}, gameData, 2)
	if err != nil {
		t.Fatal(err)
	}
	troops := resolved.Waves[0].Middle.Troops
	if len(troops) != 2 || *troops[0].ItemID != 4 || troops[0].Quantity != 75 ||
		*troops[1].ItemID != 3 || troops[1].Quantity != 25 {
		t.Fatalf("two-copy family troops = %#v, want 75 tier-4 then 25 tier-3 per attack", troops)
	}
}

func TestMaximumAvailableCopiesUsesCombinedFamilyInventory(t *testing.T) {
	gameData := troopFamilyTestGameData(t)
	anchor := int64(3)
	preset := Preset{
		ID: "family", Name: "Family", UseTroopFamilies: true,
		Waves: []Wave{{Middle: Lane{Troops: []Slot{{ItemID: &anchor, Quantity: 100}}}}},
	}
	copies, err := MaximumAvailableCopies(preset, map[State.UnitID]int64{3: 50, 4: 150}, gameData, 5)
	if err != nil {
		t.Fatal(err)
	}
	if copies != 2 {
		t.Fatalf("available family copies = %d, want 2", copies)
	}
}

func troopFamilyTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"effects":[],"effectCaps":[],
		"units":[
			{"wodID":3,"upgradeWodID":4},
			{"wodID":4,"downgradeWodID":3}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

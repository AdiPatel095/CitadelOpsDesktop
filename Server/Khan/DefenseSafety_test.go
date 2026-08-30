package Khan

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestOffensiveWallUnitsCountsOnlyOffenseSelectedAfterDefenders(t *testing.T) {
	gameData := defenseSafetyGameData(t)
	preset := DefensePreset{ID: "defense", Name: "Defense"}
	preset.Wall.Left = State.DefenseWallSection{ToolSlots: []State.DefenseToolSlot{}, UnitPercent: 33, UnitTypePercent: 100}
	preset.Wall.Middle = State.DefenseWallSection{ToolSlots: []State.DefenseToolSlot{}, UnitPercent: 34, UnitTypePercent: 100}
	preset.Wall.Right = State.DefenseWallSection{ToolSlots: []State.DefenseToolSlot{}, UnitPercent: 33, UnitTypePercent: 100}

	castle := State.CastleState{Defense: State.CastleDefenseState{
		Wall:          State.DefenseWallState{UnitCount: 2_000},
		MeleeUnitIDs:  []State.UnitID{489, 215},
		RangedUnitIDs: []State.UnitID{},
		Inventory:     map[State.UnitID]int64{489: 1_000, 215: 2_000},
	}}
	risk, err := OffensiveWallUnits(castle, gameData, preset)
	if err != nil {
		t.Fatal(err)
	}
	if risk.Capacity != 2_000 || risk.Assigned != 2_000 || risk.OffensiveUnits != 1_000 {
		t.Fatalf("wall risk = %#v, want capacity 2000, assigned 2000, offense 1000", risk)
	}
}

func TestOffensiveWallUnitsRejectsMissingWallCapacity(t *testing.T) {
	_, err := OffensiveWallUnits(State.CastleState{}, defenseSafetyGameData(t), DefensePreset{})
	if err == nil {
		t.Fatal("missing wall capacity was treated as zero offensive risk")
	}
}

func defenseSafetyGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[
			{"wodID":215,"role":"melee","fightType":"0"},
			{"wodID":216,"role":"ranged","fightType":"0"},
			{"wodID":489,"role":"melee","fightType":"1"},
			{"wodID":493,"role":"ranged","fightType":"1"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

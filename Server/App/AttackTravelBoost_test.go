package App

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestHorseTravelBoostFields(t *testing.T) {
	for _, test := range []struct {
		input       int
		wantBooster int
		wantTravel  int
	}{
		{input: -1, wantBooster: -1, wantTravel: 1},
		{input: 0, wantBooster: -1, wantTravel: 1},
		{input: 1007, wantBooster: 1007, wantTravel: 0},
		{input: 1008, wantBooster: 1008, wantTravel: 0},
		{input: 1009, wantBooster: 1009, wantTravel: 0},
	} {
		booster, travel := horseTravelBoostFields(test.input)
		if booster != test.wantBooster || travel != test.wantTravel {
			t.Fatalf("horseTravelBoostFields(%d) = (%d, %d), want (%d, %d)", test.input, booster, travel, test.wantBooster, test.wantTravel)
		}
	}
}

func TestResolveCastleHorseTravelBoostFieldsTreatsSavedIDsAsTiers(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":46,"name":"Harbor","level":"2","unlockHorses":"1033,1034,1035"}],
		"units":[],
		"horses":[
			{"wodID":1033,"group":"Travelbooster"},
			{"wodID":1034,"group":"Travelbooster"},
			{"wodID":1035,"group":"Travelbooster"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	castle := State.CastleState{
		ID: 4,
		Buildings: map[State.BuildingInstanceID]State.Building{
			1: {InstanceID: 1, DefinitionID: 46, Placed: true},
		},
	}
	for _, test := range []struct {
		selected    int
		wantBooster int
		wantTravel  int
	}{
		{selected: -1, wantBooster: -1, wantTravel: 1},
		{selected: 0, wantBooster: -1, wantTravel: 1},
		{selected: 1007, wantBooster: 1033, wantTravel: 0},
		{selected: 1008, wantBooster: 1034, wantTravel: 0},
		{selected: 1009, wantBooster: 1035, wantTravel: 0},
	} {
		booster, travel, err := resolveCastleHorseTravelBoostFields(gameData, castle, test.selected)
		if err != nil {
			t.Fatalf("resolve saved selection %d: %v", test.selected, err)
		}
		if booster != test.wantBooster || travel != test.wantTravel {
			t.Fatalf(
				"selection %d = HBW %d PTT %d, want HBW %d PTT %d",
				test.selected, booster, travel, test.wantBooster, test.wantTravel,
			)
		}
	}
}

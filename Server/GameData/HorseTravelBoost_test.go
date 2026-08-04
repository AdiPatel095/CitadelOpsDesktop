package GameData

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestResolveHorseTravelBoostUsesBuildingUnlockCatalog(t *testing.T) {
	sets := []struct {
		name       string
		buildingID int64
		level      int
		horseIDs   [3]int64
	}{
		{name: "Stable", buildingID: 214, level: 1, horseIDs: [3]int64{1001, 1002, 1003}},
		{name: "Stable", buildingID: 215, level: 2, horseIDs: [3]int64{1004, 1005, 1006}},
		{name: "Stable", buildingID: 226, level: 3, horseIDs: [3]int64{1007, 1008, 1009}},
		{name: "FactionStable", buildingID: 247, level: 1, horseIDs: [3]int64{1021, 1022, 1023}},
		{name: "FactionStable", buildingID: 15, level: 2, horseIDs: [3]int64{1039, 1040, 1041}},
		{name: "FactionStable", buildingID: 293, level: 3, horseIDs: [3]int64{1024, 1025, 1026}},
		{name: "FactionStable", buildingID: 16, level: 4, horseIDs: [3]int64{1042, 1043, 1044}},
		{name: "FactionStable", buildingID: 294, level: 5, horseIDs: [3]int64{1027, 1028, 1029}},
		{name: "Harbor", buildingID: 45, level: 1, horseIDs: [3]int64{1030, 1031, 1032}},
		{name: "Harbor", buildingID: 46, level: 2, horseIDs: [3]int64{1033, 1034, 1035}},
		{name: "Harbor", buildingID: 47, level: 3, horseIDs: [3]int64{1036, 1037, 1038}},
	}
	buildings := make([]map[string]any, 0, len(sets))
	horses := make([]map[string]any, 0, len(sets)*3)
	for _, set := range sets {
		buildings = append(buildings, map[string]any{
			"wodID": set.buildingID, "name": set.name, "level": set.level,
			"unlockHorses": horseUnlockList(set.horseIDs),
		})
		for index, horseID := range set.horseIDs {
			horses = append(horses, map[string]any{
				"wodID": horseID, "group": "Travelbooster", "comment1": set.name,
				"comment2":  []string{"standard", "fast", "fastest"}[index],
				"unitBoost": 10 + index, "marketBoost": 20 + index, "spyBoost": 30 + index,
				"costFactorC1": boolNumber(index == 0), "costFactorC2": boolNumber(index > 0),
			})
		}
	}
	raw, err := json.Marshal(map[string]any{
		"versionInfo": []any{}, "buildings": buildings, "units": []any{}, "horses": horses,
	})
	if err != nil {
		t.Fatal(err)
	}
	store, err := DecodeStore(raw, SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}

	for _, set := range sets {
		for tier := HorseTravelBoostStandard; tier <= HorseTravelBoostFastest; tier++ {
			t.Run(set.name+" level "+strconv.Itoa(set.level)+" tier "+strconv.Itoa(int(tier)), func(t *testing.T) {
				castle := State.CastleState{
					ID: 1,
					Buildings: map[State.BuildingInstanceID]State.Building{
						1: {InstanceID: 1, DefinitionID: State.BuildingID(set.buildingID), Placed: true},
					},
				}
				definition, err := store.ResolveHorseTravelBoost(castle, tier)
				if err != nil {
					t.Fatal(err)
				}
				if definition.ID != set.horseIDs[int(tier)-1] ||
					definition.BuildingDefinitionID != set.buildingID ||
					definition.BuildingName != set.name ||
					definition.BuildingLevel != set.level {
					t.Fatalf("unexpected resolved boost: %#v", definition)
				}
			})
		}
	}
}

func TestResolveHorseTravelBoostFailsClosedWithoutOneActiveSet(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":226,"name":"Stable","level":"3","unlockHorses":"1007,1008,1009"},
			{"wodID":294,"name":"FactionStable","level":"5","unlockHorses":"1027,1028,1029"}
		],
		"units":[],
		"horses":[{"wodID":1007,"group":"Travelbooster"}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResolveHorseTravelBoost(
		State.CastleState{ID: 1, Layout: State.CastleLayout{ObservedAt: time.Now().UTC()}, Buildings: map[State.BuildingInstanceID]State.Building{
			1: {DefinitionID: 226, Placed: false},
		}},
		HorseTravelBoostStandard,
	); err == nil || !errors.Is(err, ErrHorseTravelBoostUnavailable) || !strings.Contains(err.Error(), "no placed") {
		t.Fatalf("stored Stable was accepted: %v", err)
	}
	if _, err := store.ResolveHorseTravelBoost(
		State.CastleState{ID: 2, Buildings: map[State.BuildingInstanceID]State.Building{
			1: {DefinitionID: 226, Placed: true},
			2: {DefinitionID: 294, Placed: true},
		}},
		HorseTravelBoostStandard,
	); err == nil || !errors.Is(err, ErrHorseTravelBoostConflict) || !strings.Contains(err.Error(), "conflicting") {
		t.Fatalf("conflicting travel buildings were accepted: %v", err)
	}
}

func TestResolveHorseTravelBoostDistinguishesUnobservedLayout(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[{"wodID":226,"name":"Stable","level":"3","unlockHorses":"1007,1008,1009"}],
		"units":[],"horses":[]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.ResolveHorseTravelBoost(State.CastleState{ID: 7}, HorseTravelBoostFastest)
	if !errors.Is(err, ErrHorseTravelBoostLayoutUnobserved) {
		t.Fatalf("unobserved layout error = %v", err)
	}
}

func horseUnlockList(ids [3]int64) string {
	return strconv.FormatInt(ids[0], 10) + "," +
		strconv.FormatInt(ids[1], 10) + "," +
		strconv.FormatInt(ids[2], 10)
}

func boolNumber(value bool) int {
	if value {
		return 1
	}
	return 0
}

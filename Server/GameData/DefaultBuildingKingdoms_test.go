package GameData

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

func officialKingdomCatalog(t *testing.T) *BuildingCatalog {
	t.Helper()
	raw, err := os.ReadFile("testdata/default_building_kingdoms_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	store, err := DecodeStore(raw, SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := store.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestAbsentBuildingKingdomsOfficialStorehouse132(t *testing.T) {
	catalog := officialKingdomCatalog(t)
	definition, _ := catalog.Definition(132)
	if !reflect.DeepEqual(definition.KingdomIDs, []int64{0, 1, 2, 3, 4}) || !definition.KingdomIDsDefaulted {
		t.Fatalf("default kingdoms: %#v", definition)
	}
	raw, err := json.Marshal(definition)
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		KingdomIDs []int64 `json:"kingdomIds"`
		Defaulted  bool    `json:"kingdomIdsDefaulted"`
	}
	if err := json.Unmarshal(raw, &projection); err != nil {
		t.Fatal(err)
	}
	if !projection.Defaulted || !reflect.DeepEqual(projection.KingdomIDs, definition.KingdomIDs) {
		t.Fatalf("projection: %s", raw)
	}
	definition.KingdomIDs[0] = 10
	again, _ := catalog.Definition(132)
	if again.KingdomIDs[0] != 0 || DefaultBuildingKingdomIDs[0] != 0 {
		t.Fatal("default list aliases a mutable projection")
	}
}

func TestExplicitBuildingKingdomsOfficialRecords(t *testing.T) {
	catalog := officialKingdomCatalog(t)
	for _, tt := range []struct {
		id   int64
		want []int64
	}{{246, []int64{10}}, {1923, []int64{0, 1, 2, 3}}} {
		definition, _ := catalog.Definition(tt.id)
		if !reflect.DeepEqual(definition.KingdomIDs, tt.want) || definition.KingdomIDsDefaulted {
			t.Fatalf("explicit kingdoms for %d: %#v", tt.id, definition)
		}
	}
}

func TestEmptyBuildingKingdomsUseOfficialDefaults(t *testing.T) {
	definition := decodeBuildingDefinition(Record{"name": json.RawMessage(`"Empty"`), "kIDs": json.RawMessage(`""`)}, 1, nil)
	if !definition.KingdomIDsDefaulted || !reflect.DeepEqual(definition.KingdomIDs, []int64{0, 1, 2, 3, 4}) {
		t.Fatalf("empty kingdoms: %#v", definition)
	}
}

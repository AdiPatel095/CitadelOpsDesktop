package GameData

import "testing"

func TestCheapestNonPremiumBerimondCamp(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"prebuiltcastles":[
			{"preBuiltCastleID":"3","spaceIDs":"10","minLevel":15,"costC2":"49000"},
			{"preBuiltCastleID":"2","spaceIDs":"10","minLevel":15,"costWood":"9000","costStone":"9000"},
			{"preBuiltCastleID":"1","spaceIDs":"10","minLevel":15,"costWood":"900","costStone":"900"},
			{"preBuiltCastleID":"4","spaceIDs":"4","minLevel":1,"costWood":"1"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if _, found := store.CheapestNonPremiumBerimondCamp(14); found {
		t.Fatal("camp should remain locked below its official minimum level")
	}
	option, found := store.CheapestNonPremiumBerimondCamp(15)
	if !found || option.ID != 1 || option.CostWood != 900 || option.CostStone != 900 {
		t.Fatalf("unexpected camp option: %#v found=%t", option, found)
	}
}

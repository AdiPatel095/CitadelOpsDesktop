package GameData

import "testing"

func TestValidateEventShopDestinationUsesOfficialRouteRestrictions(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"events":[{"eventID":116,"kIDs":"0","areaTypes":"1"}],
		"packages":[
			{"packageID":1972},
			{"packageID":1973,"excludedAreaTypes":"1,4"},
			{"packageID":1974,"excludedAreaTypes":"bad"},
			{"packageID":1975,"excludedAreaTypes":"-1"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ValidateEventShopDestination(1972, 116, 0, 1); err != nil {
		t.Fatalf("eligible main castle rejected: %v", err)
	}
	if err := store.ValidateEventShopDestination(1975, 116, 0, 1); err != nil {
		t.Fatalf("official -1 exclusion sentinel rejected: %v", err)
	}
	for name, values := range map[string][3]int64{
		"wrong kingdom":       {1972, 4, 1},
		"wrong area":          {1972, 0, 4},
		"package exclusion":   {1973, 0, 1},
		"malformed exclusion": {1974, 0, 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := store.ValidateEventShopDestination(values[0], 116, values[1], int(values[2])); err == nil {
				t.Fatal("ineligible route was accepted")
			}
		})
	}
}

func TestValidateEventShopDestinationRejectsMalformedEventRestrictions(t *testing.T) {
	for name, restriction := range map[string]string{
		"bad id": `"0,bad"`, "object": `{}`, "array": `[]`, "null": `null`,
	} {
		t.Run(name, func(t *testing.T) {
			store, err := DecodeStore([]byte(`{
				"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
				"events":[{"eventID":116,"kIDs":`+restriction+`,"areaTypes":"1"}],
				"packages":[{"packageID":1972}]
			}`), SourceMetadata{ItemVersion: "test"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.ValidateEventShopDestination(1972, 116, 0, 1); err == nil {
				t.Fatal("malformed event restriction was accepted")
			}
		})
	}
}

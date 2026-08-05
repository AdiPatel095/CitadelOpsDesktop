package GameData

import "testing"

func TestAllianceHelpMaximumHelpersUsesOfficialRequestCatalog(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"alliancehelprequests":[
			{"allianceHelpRequestID":"2","maxHelpersCount":"5"},
			{"allianceHelpRequestID":"6","maxHelpersCount":"3"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if maximum, found := store.AllianceHelpMaximumHelpers(2); !found || maximum != 5 {
		t.Fatalf("hospital maximum = %d found=%t, want 5 true", maximum, found)
	}
	if maximum, found := store.AllianceHelpMaximumHelpers(6); !found || maximum != 3 {
		t.Fatalf("recruitment maximum = %d found=%t, want 3 true", maximum, found)
	}
	if _, found := store.AllianceHelpMaximumHelpers(999); found {
		t.Fatal("unknown alliance-help request type should fail closed")
	}
}

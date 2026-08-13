package GameData

import "testing"

func TestGloryTitleCatalogResolvesDisplayTitleAndUnitUnlocks(t *testing.T) {
	store := gloryTitleTestStore(t)

	titleID, found := store.GloryTitleFromDisplayIDs(116, 31)
	if !found || titleID != 31 {
		t.Fatalf("glory title = %d, %t; want 31, true", titleID, found)
	}
	gallantryTitleID, found := store.GallantryTitleFromDisplayIDs(116, 31)
	if !found || gallantryTitleID != 116 {
		t.Fatalf("gallantry title = %d, %t; want 116, true", gallantryTitleID, found)
	}
	if !store.GloryTitleIncludes(32, 30) || !store.GloryTitleIncludes(31, 31) || store.GloryTitleIncludes(30, 31) {
		t.Fatal("glory title ancestry did not follow the official previous-title chain")
	}

	protector, found := store.GloryTitleUnlockForUnit(489)
	if !found || protector.RequiredTitleID != 30 || protector.Level10UnitID != 227 {
		t.Fatalf("protector unlock = %#v, %t", protector, found)
	}
	valkyrie, found := store.GloryTitleUnlockForUnit(493)
	if !found || valkyrie.RequiredTitleID != 31 || valkyrie.Level10UnitID != 238 {
		t.Fatalf("valkyrie unlock = %#v, %t", valkyrie, found)
	}
	if _, found := store.GloryTitleUnlockForUnit(238); found {
		t.Fatal("level-10 unit was incorrectly treated as title-gated")
	}
}

func gloryTitleTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[
			{"wodID":227,"type":"MeadMace","level":"10"},
			{"wodID":489,"type":"MeadMace","level":"11"},
			{"wodID":238,"type":"MeadBow","level":"10"},
			{"wodID":493,"type":"MeadBow","level":"11"}
		],
		"titles":[
			{"titleID":"29","type":"FAME","displayType":"suffix"},
			{"titleID":"30","previousTitleID":"29","type":"FAME","displayType":"suffix","effects":"46&489"},
			{"titleID":"31","previousTitleID":"30","type":"FAME","displayType":"suffix","effects":"46&493"},
			{"titleID":"32","previousTitleID":"31","type":"FAME","displayType":"suffix"},
			{"titleID":"116","type":"FACTION","displayType":"prefix"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

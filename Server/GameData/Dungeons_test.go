package GameData

import (
	"testing"

	"CitadelDesktop/Server/State"
)

func TestDungeonFindsOfficialSkipCostByKingdomAndVictoryCount(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"dungeons":[
			{"countVictories":"845","kID":"0","skipCosts":"6766","lordID":"-30"},
			{"countVictories":"845","kID":"1","skipCosts":"7000","lordID":"-30"}
		]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	definition, found := store.Dungeon(State.KingdomID(0), 845)
	if !found || definition.SkipCost != 6766 || definition.LordID != -30 {
		t.Fatalf("unexpected dungeon definition: found=%t definition=%#v", found, definition)
	}
	if _, found := store.Dungeon(State.KingdomID(0), 846); found {
		t.Fatal("matched a dungeon row with a different victory count")
	}
}

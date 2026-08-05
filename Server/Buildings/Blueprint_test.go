package Buildings

import (
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestCompileBlueprintDiffInfersBerimondEventContext(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":100,"name":"FactionUnittent","level":"1","upgradeWodID":"101","kIDs":"10","eventIDs":"3"},
			{"wodID":101,"name":"FactionUnittent","level":"2","downgradeWodID":"100","kIDs":"10","eventIDs":"3"}
		],
		"units":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	building := State.Building{InstanceID: 1, DefinitionID: 100, Placed: true}
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{
		ID: 1, KingdomID: State.KingdomID(GameData.BerimondKingdomID),
		Buildings: map[State.BuildingInstanceID]State.Building{1: building},
		Layout: State.CastleLayout{
			Objects: map[State.BuildingInstanceID]State.Building{1: building},
			Ground:  map[State.BuildingInstanceID]State.Building{},
			Fixed:   map[State.BuildingInstanceID]State.Building{},
		},
	}
	result, err := CompileBlueprintDiff(state, gameData, BlueprintDiffRequest{Target: TargetCaptureResult{
		Version: 1, CastleID: 1, KingdomID: State.KingdomID(GameData.BerimondKingdomID),
		Mode:      TargetCaptureModeFunctional,
		Buildings: []TargetBuilding{{TargetID: "tent", DefinitionID: 101}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compilable || result.ActionCount != 1 || targetDiffHasIssue(result.Normal.Issues, "event_context") {
		t.Fatalf("Berimond event context was not inferred: %#v", result)
	}
}

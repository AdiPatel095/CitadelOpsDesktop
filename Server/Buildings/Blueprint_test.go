package Buildings

import (
	"testing"
	"time"

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

func TestCaptureTargetKeepsCargoAcrossNonExactModes(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],"buildings":[
			{"wodID":35,"name":"Cargo","group":"Building","buildingGroundType":"DECO","level":1,"width":5,"height":10},
			{"wodID":600,"name":"EventDecoration","group":"Decoration","buildingGroundType":"DECO","level":1,"width":5,"height":5}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	cargo := State.Building{InstanceID: 1, DefinitionID: 35, Placed: true, GridX: 1, GridY: 1}
	decoration := State.Building{InstanceID: 2, DefinitionID: 600, Placed: true, GridX: 10, GridY: 1}
	state := State.NewGameState()
	state.Castles[1] = State.CastleState{ID: 1, KingdomID: 4, Buildings: map[State.BuildingInstanceID]State.Building{1: cargo, 2: decoration}, Layout: State.CastleLayout{
		Objects: map[State.BuildingInstanceID]State.Building{1: cargo, 2: decoration}, Ground: map[State.BuildingInstanceID]State.Building{},
		Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: time.Now().UTC(),
	}}
	for _, mode := range []string{TargetCaptureModeFunctional, TargetCaptureModeLayout} {
		captured, captureErr := CaptureTarget(state, gameData, TargetCaptureRequest{CastleID: 1, Mode: mode})
		if captureErr != nil {
			t.Fatal(captureErr)
		}
		if len(captured.Buildings) != 1 || captured.Buildings[0].DefinitionID != 35 || captured.Summary.BuildingCount != 1 || captured.Summary.DecorationCount != 0 {
			t.Fatalf("%s capture=%#v", mode, captured)
		}
		if mode == TargetCaptureModeFunctional && captured.Buildings[0].Placement != nil {
			t.Fatalf("functional Cargo placement=%#v, want nil", captured.Buildings[0].Placement)
		}
	}
}

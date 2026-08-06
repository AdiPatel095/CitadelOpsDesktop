package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriBuildPolicyUsesBuiltInTargetWhenNoBlueprintIsActive(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	decision, err := NewBeriBuildPolicy().Evaluate(t.Context(), Snapshot{
		State: State.NewGameState(),
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoBeriWorldSection:                            json.RawMessage(`{"build":{"enabled":true,"stableLevel":3}}`),
			Buildings.BerimondBlueprintConfigurationSection: json.RawMessage(`{"version":1,"blueprints":{}}`),
		}},
		GameData: gameData,
		Now:      now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Detail != "Waiting for an owned Berimond camp" {
		t.Fatalf("decision = %#v, want built-in target waiting for its camp", decision)
	}
}

func TestNormalizeBeriBuildSettingsDefaultsStableToMaximum(t *testing.T) {
	settings := beriBuildSettings{}
	normalizeBeriBuildSettings(&settings)
	if settings.StableLevel != Buildings.DefaultBerimondStableTargetLevel {
		t.Fatalf("Stable level = %d, want %d", settings.StableLevel, Buildings.DefaultBerimondStableTargetLevel)
	}
}

func TestBeriBuildTargetNeverDowngradesAnExistingStable(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":247,"name":"FactionStable","level":"1","upgradeWodID":"294","kIDs":"10"},
			{"wodID":294,"name":"FactionStable","level":"5","downgradeWodID":"247","kIDs":"10"}
		],
		"units":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	target := Buildings.TargetCaptureResult{Buildings: []Buildings.TargetBuilding{{
		TargetID: "stable", DefinitionID: 247,
	}}}
	castle := State.CastleState{Layout: State.CastleLayout{Objects: map[State.BuildingInstanceID]State.Building{
		1: {InstanceID: 1, DefinitionID: 294, Placed: true},
	}}}
	effective := preserveHigherBeriStableTarget(&target, castle, gameData, 1)
	if effective != 5 || target.Buildings[0].DefinitionID != 294 {
		t.Fatalf("effective Stable = %d / WoD %d, want level 5 / WoD 294", effective, target.Buildings[0].DefinitionID)
	}
}

func TestAutoEventBuildPassesBerimondEventContextThroughUpgradeIntent(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":100,"name":"FactionUnittent","level":"1","upgradeWodID":"101","kIDs":"10","eventIDs":"3"},
			{"wodID":101,"name":"FactionUnittent","level":"2","downgradeWodID":"100","kIDs":"10","eventIDs":"3","costWood":"1"}
		],
		"resources":[{"resourceID":3,"name":"Wood","JSONKey":"W"}],
		"units":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	building := State.Building{
		InstanceID: 1, DefinitionID: 100, Placed: true,
		ConstructionState: State.BuildingStateBuildCompleted,
	}
	capacity := 100.0
	castle := State.CastleState{
		ID: 1, KingdomID: State.KingdomID(GameData.BerimondKingdomID), Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{3: {Amount: 100, Capacity: &capacity}},
		Buildings: map[State.BuildingInstanceID]State.Building{1: building},
		Layout: State.CastleLayout{
			Objects: map[State.BuildingInstanceID]State.Building{1: building},
			Ground:  map[State.BuildingInstanceID]State.Building{}, Fixed: map[State.BuildingInstanceID]State.Building{},
			ObservedAt: now,
		},
		BuildingQueue: State.BuildingConstructionQueue{
			Slots: []State.BuildingConstructionQueueSlot{{Status: State.BuildingQueueSlotAvailable}}, ObservedAt: now,
		},
	}
	state := State.NewGameState()
	state.Castles[castle.ID] = castle
	target := Buildings.TargetCaptureResult{
		Version: 1, CastleID: castle.ID, KingdomID: castle.KingdomID,
		Mode:      Buildings.TargetCaptureModeFunctional,
		Buildings: []Buildings.TargetBuilding{{TargetID: "tent", DefinitionID: 101}},
	}
	settings := defaultAutoStormSettings()
	settings.Target = &target
	decision, _, _, err := evaluateAutoEventBuild(
		Snapshot{State: state, GameData: gameData, Now: now},
		settings,
		castle,
		map[string]float64{},
		autoEventBuildProfile{
			KingdomID: castle.KingdomID, FeatureLabel: "Berimond",
			AttackLootOnly: true, EventID: GameData.BerimondEventID,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision == nil || decision.Request == nil || decision.Metrics["targetActionsRemaining"] != 1 {
		t.Fatalf("decision = %#v, want an actionable event-authorized upgrade path", decision)
	}
	var arguments struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		EventID            int64                    `json:"eventId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if decision.Request.Name != "building.upgrade" || arguments.CastleID != castle.ID ||
		arguments.BuildingInstanceID != building.InstanceID || arguments.EventID != GameData.BerimondEventID {
		t.Fatalf("Berimond upgrade intent %s (%s) lost event context: %s", decision.Request.Name, decision.Detail, decision.Request.Arguments)
	}
}

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

func TestBeriBuildPolicyStopsBeforeSharedGatesWhenUserDisablesBuilder(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Market.BoostersObservedAt = now
	decision, err := NewBeriBuildPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoBeriWorldSection: json.RawMessage(`{"requireActiveGallantryBooster":true,"build":{"enabled":false}}`),
		}},
		Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "disabled" || decision.Detail != "Auto Beri Builder is disabled by the user" ||
		decision.Request != nil || !decision.EventDriven || !decision.NextCheckAt.IsZero() {
		t.Fatalf("disabled Builder decision = %#v", decision)
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

func TestAutoBeriBuilderSkipsStablesWhenChoosingDemolitionCandidates(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":247,"name":"FactionStable","level":"1","group":"Building","width":"5","height":"5","movable":"0"},
			{"wodID":100,"name":"FactionUnittent","level":"1","group":"Building","width":"5","height":"5","movable":"0"}
		],
		"units":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	castle := State.CastleState{
		ID: 1,
		Layout: State.CastleLayout{Objects: map[State.BuildingInstanceID]State.Building{
			17: {InstanceID: 17, DefinitionID: 247, Placed: true},
			18: {InstanceID: 18, DefinitionID: 100, Placed: true},
		}},
	}
	settings := defaultAutoStormSettings()
	settings.Build.AllowDemolition = true
	profile := autoEventBuildProfile{
		KingdomID: State.KingdomID(GameData.BerimondKingdomID), FeatureLabel: "Berimond",
		IgnoreDemolitionCandidate: isBeriStableDefinition,
	}
	decision := autoStormDiffRemediation(
		Snapshot{Now: time.Now().UTC()}, settings, castle, catalog,
		Buildings.TargetDiffResult{
			Exact: true,
			Issues: []Buildings.TargetIssue{{
				Code: "extra_building", BuildingIDs: []State.BuildingInstanceID{17, 18},
			}},
		},
		map[string]float64{},
		profile,
	)
	if decision == nil || decision.Request == nil || decision.Request.Name != "building.demolish" {
		t.Fatalf("Auto Beri demolition choice = %#v", decision)
	}
	var arguments struct {
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.BuildingInstanceID != 18 {
		t.Fatalf("Auto Beri chose building %d for demolition; Stable 17 must be ignored", arguments.BuildingInstanceID)
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

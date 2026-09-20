package Automation

import (
	"encoding/json"
	"strings"
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

func TestBeriBuilderFinishesStableBeforeSavingForExpansion(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 50)
	stable := castle.Layout.Objects[10]
	stable.DefinitionID = 247
	castle.Layout.Objects[10], castle.Buildings[10] = stable, stable
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{
		{DefinitionID: 900, GridX: 0, GridY: 0},
		{DefinitionID: 900, GridX: 20, GridY: 0},
	}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 21, GridY: 1}},
		{TargetID: "deco", DefinitionID: 200, Placement: &Buildings.TargetPlacement{GridX: 24, GridY: 1}},
		{TargetID: "tent", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 27, GridY: 1}},
	})

	decision, _, _, err := beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if decision == nil || decision.Request == nil || decision.Request.Name != "building.upgrade" {
		t.Fatalf("first Stable phase decision = %#v", decision)
	}

	stable.DefinitionID = 294
	castle.Layout.Objects[10], castle.Buildings[10] = stable, stable
	state.Castles[castle.ID] = castle
	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "expansion") {
		t.Fatalf("unaffordable expansion decision = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func TestBeriBuilderConsumesEveryExpansionBeforeCleanup(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 8, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{
		{DefinitionID: 900, GridX: 0, GridY: 0},
		{DefinitionID: 900, GridX: 20, GridY: 0},
		{DefinitionID: 900, GridX: 40, GridY: 0},
	}, []Buildings.TargetBuilding{{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}}})

	decision, _, _, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.expand")

	beriPhaseAddGround(&state, &castle, 2, 20)
	decision, _, _, err = beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.expand")

	beriPhaseAddGround(&state, &castle, 3, 40)
	decision, _, _, err = beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.demolish")
}

func TestBeriBuilderBlocksUnauthorizedCleanupAndRetainsUpgradeSource(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 0)
	tent := State.Building{InstanceID: 20, DefinitionID: 100, Placed: true, GridX: 5, GridY: 1, Layer: State.BuildingLayerBD}
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[20], castle.Buildings[20] = tent, tent
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "tent", DefinitionID: 101, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 1}},
	})

	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "enable demolition") {
		t.Fatalf("unauthorized cleanup = %#v complete=%t detail=%q", decision, complete, detail)
	}

	delete(castle.Layout.Objects, 30)
	delete(castle.Buildings, 30)
	state.Castles[castle.ID] = castle
	decision, complete, detail, err = beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "attack loot") {
		t.Fatalf("retained lower-level target = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func TestBeriBuilderMovesRetainedBuildingsBeforeDecorations(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	tent := State.Building{InstanceID: 20, DefinitionID: 100, Placed: true, GridX: 5, GridY: 1, Layer: State.BuildingLayerBD}
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[20], castle.Buildings[20] = tent, tent
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "tent", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 12, GridY: 1}},
		{TargetID: "deco", DefinitionID: 200, Placement: &Buildings.TargetPlacement{GridX: 15, GridY: 1}},
	})

	decision, _, _, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.demolish")
	delete(castle.Layout.Objects, 30)
	delete(castle.Buildings, 30)
	state.Castles[castle.ID] = castle
	decision, _, _, err = beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.move")

	tent.GridX = 12
	castle.Layout.Objects[20], castle.Buildings[20] = tent, tent
	state.Castles[castle.ID] = castle
	decision, _, _, err = beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.construct")
	var arguments struct {
		DefinitionID State.BuildingID `json:"definitionId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if arguments.DefinitionID != 200 {
		t.Fatalf("first post-move construction definition = %d, want decoration 200", arguments.DefinitionID)
	}
}

func TestBeriBuilderDoesNotBypassBlockedDecorationForCheapTent(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 10)
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "deco", DefinitionID: 200, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 1}},
		{TargetID: "tent", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 9, GridY: 1}},
	})

	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "decoration") {
		t.Fatalf("blocked decoration phase = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func TestBeriBuilderRejectsMalformedTargetBeforeCleanup(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "one", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 1}},
		{TargetID: "two", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 1}},
	})

	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || detail == "" {
		t.Fatalf("malformed cleanup guard = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func TestBeriBuilderStoresExtraDecorationAndNeverRemovesStable(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	decoration := State.Building{InstanceID: 30, DefinitionID: 200, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[30], castle.Buildings[30] = decoration, decoration
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, nil)

	decision, _, _, err := beriPhaseTestEvaluate(t, now, state, target, false)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.store")

	delete(castle.Layout.Objects, 30)
	delete(castle.Buildings, 30)
	state.Castles[castle.ID] = castle
	decision, complete, _, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || !complete {
		t.Fatalf("exact target omitting Stable = %#v complete=%t", decision, complete)
	}
}

func TestBeriBuilderKeepsFunctionalTargetExtras(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294},
	})
	target.Mode, target.Exact = Buildings.TargetCaptureModeFunctional, false

	decision, complete, _, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || !complete {
		t.Fatalf("functional target extra = %#v complete=%t", decision, complete)
	}
}

func TestBeriBuilderWaitsOnRetainedMoveCycle(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	tent := State.Building{InstanceID: 20, DefinitionID: 100, Placed: true, GridX: 5, GridY: 1, Layer: State.BuildingLayerBD}
	hunter := State.Building{InstanceID: 21, DefinitionID: 400, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[20], castle.Buildings[20] = tent, tent
	castle.Layout.Objects[21], castle.Buildings[21] = hunter, hunter
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "tent", DefinitionID: 100, Placement: &Buildings.TargetPlacement{GridX: 9, GridY: 1}},
		{TargetID: "hunter", DefinitionID: 400, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 1}},
	})

	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "Cannot move retained") {
		t.Fatalf("retained move cycle = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func TestBeriBuilderDefersPremiumUpgradeBlockerUntilAfterCleanupAndMove(t *testing.T) {
	now := time.Date(2026, 9, 18, 12, 0, 0, 0, time.UTC)
	state, castle := beriPhaseTestState(now, 1, 1_000)
	hunter := State.Building{InstanceID: 20, DefinitionID: 400, Placed: true, GridX: 5, GridY: 1, Layer: State.BuildingLayerBD}
	extra := State.Building{InstanceID: 30, DefinitionID: 300, Placed: true, GridX: 9, GridY: 1, Layer: State.BuildingLayerBD}
	castle.Layout.Objects[20], castle.Buildings[20] = hunter, hunter
	castle.Layout.Objects[30], castle.Buildings[30] = extra, extra
	state.Castles[castle.ID] = castle
	target := beriPhaseTarget(castle.ID, []Buildings.TargetGround{{DefinitionID: 900, GridX: 0, GridY: 0}}, []Buildings.TargetBuilding{
		{TargetID: "stable", DefinitionID: 294, Placement: &Buildings.TargetPlacement{GridX: 1, GridY: 1}},
		{TargetID: "hunter", DefinitionID: 401, Placement: &Buildings.TargetPlacement{GridX: 12, GridY: 1}},
	})

	decision, _, _, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.demolish")
	delete(castle.Layout.Objects, 30)
	delete(castle.Buildings, 30)
	state.Castles[castle.ID] = castle
	decision, _, _, err = beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	assertBeriRequest(t, decision, "building.move")

	hunter.GridX = 12
	castle.Layout.Objects[20], castle.Buildings[20] = hunter, hunter
	state.Castles[castle.ID] = castle
	decision, complete, detail, err := beriPhaseTestEvaluate(t, now, state, target, true)
	if err != nil {
		t.Fatal(err)
	}
	if decision != nil || complete || !strings.Contains(detail, "premium") {
		t.Fatalf("deferred premium blocker = %#v complete=%t detail=%q", decision, complete, detail)
	}
}

func beriPhaseTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"rubies"},{"resourceID":3,"JSONKey":"W","name":"wood"}],
		"expansions":[
			{"expansionID":901,"spaceIDs":"10","expansionLevel":1,"costWood":100},
			{"expansionID":902,"spaceIDs":"10","expansionLevel":2,"costWood":100}
		],
		"buildings":[
			{"wodID":900,"name":"Ground","group":"Ground","width":20,"height":20},
			{"wodID":247,"name":"FactionStable","group":"Building","shopCategory":"MILITARY","level":1,"width":2,"height":2,"upgradeWodID":294,"storeable":1,"movable":1,"destructable":1,"costWood":10,"kIDs":"10","eventIDs":"3"},
			{"wodID":294,"name":"FactionStable","group":"Building","level":5,"width":2,"height":2,"downgradeWodID":247,"storeable":1,"movable":1,"destructable":1,"costWood":10,"kIDs":"10","eventIDs":"3"},
			{"wodID":100,"name":"FactionUnittent","group":"Building","shopCategory":"MILITARY","level":1,"width":2,"height":2,"upgradeWodID":101,"movable":1,"destructable":1,"costWood":1,"kIDs":"10","eventIDs":"3"},
			{"wodID":101,"name":"FactionUnittent","group":"Building","level":2,"width":2,"height":2,"downgradeWodID":100,"movable":1,"destructable":1,"costWood":20,"kIDs":"10","eventIDs":"3"},
			{"wodID":200,"name":"FactionDeco","group":"Building","shopCategory":"DECO","level":1,"width":2,"height":2,"storeable":1,"movable":1,"destructable":1,"costWood":100,"kIDs":"10","eventIDs":"3"},
			{"wodID":300,"name":"FactionMainTent","group":"Building","shopCategory":"MILITARY","level":1,"width":2,"height":2,"movable":1,"destructable":1,"costWood":1,"kIDs":"10","eventIDs":"3"},
			{"wodID":400,"name":"FactionHunterTent","group":"Building","shopCategory":"MILITARY","level":1,"width":2,"height":2,"upgradeWodID":401,"movable":1,"destructable":1,"costWood":1,"kIDs":"10","eventIDs":"3"},
			{"wodID":401,"name":"FactionHunterTent","group":"Building","level":2,"width":2,"height":2,"downgradeWodID":400,"movable":1,"destructable":1,"costC2":5,"kIDs":"10","eventIDs":"3"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "beri-phases-test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func beriPhaseTestState(now time.Time, groundCount int, wood float64) (State.GameState, State.CastleState) {
	state := State.NewGameState()
	state.Player.Level = 70
	capacity := float64(1_000)
	ground := map[State.BuildingInstanceID]State.Building{}
	buildings := map[State.BuildingInstanceID]State.Building{}
	for index := 0; index < groundCount; index++ {
		id := State.BuildingInstanceID(index + 1)
		tile := State.Building{InstanceID: id, DefinitionID: 900, Placed: true, GridX: index * 20, GridY: 0, Layer: State.BuildingLayerBG}
		ground[id], buildings[id] = tile, tile
	}
	stable := State.Building{InstanceID: 10, DefinitionID: 294, Placed: true, GridX: 1, GridY: 1, Layer: State.BuildingLayerBD, ConstructionState: State.BuildingStateBuildCompleted}
	buildings[10] = stable
	castle := State.CastleState{
		ID: 77, KingdomID: State.KingdomID(GameData.BerimondKingdomID), Focused: true,
		Resources:     map[State.ResourceID]State.ResourceBalance{3: {Amount: wood, Capacity: &capacity}},
		Buildings:     buildings,
		Layout:        State.CastleLayout{Ground: ground, Objects: map[State.BuildingInstanceID]State.Building{10: stable}, Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: now},
		BuildingQueue: State.BuildingConstructionQueue{SlotCount: 1, ObservedAt: now, Slots: []State.BuildingConstructionQueueSlot{{Index: 0, Status: State.BuildingQueueSlotAvailable}}},
	}
	state.Castles[castle.ID] = castle
	return state, castle
}

func beriPhaseAddGround(state *State.GameState, castle *State.CastleState, id State.BuildingInstanceID, x int) {
	tile := State.Building{InstanceID: id, DefinitionID: 900, Placed: true, GridX: x, GridY: 0, Layer: State.BuildingLayerBG}
	castle.Layout.Ground[id], castle.Buildings[id] = tile, tile
	state.Castles[castle.ID] = *castle
}

func beriPhaseTarget(castleID State.CastleID, ground []Buildings.TargetGround, buildings []Buildings.TargetBuilding) Buildings.TargetCaptureResult {
	return Buildings.TargetCaptureResult{
		Version: 1, CastleID: castleID, KingdomID: State.KingdomID(GameData.BerimondKingdomID),
		Mode: Buildings.TargetCaptureModeExact, Exact: true, Ground: ground, Buildings: buildings,
	}
}

func beriPhaseTestEvaluate(
	t *testing.T,
	now time.Time,
	state State.GameState,
	target Buildings.TargetCaptureResult,
	allowDemolition bool,
) (*Decision, bool, string, error) {
	settings := defaultAutoStormSettings()
	settings.Target = &target
	settings.Harbor.Enabled = false
	settings.Build.AllowDemolition = allowDemolition
	settings.Build.ResourceReserves = map[string]float64{}
	castle := state.Castles[target.CastleID]
	return evaluateBeriEventBuild(
		Snapshot{State: state, GameData: beriPhaseTestGameData(t), Now: now},
		settings, castle, map[string]float64{}, autoEventBuildProfile{
			KingdomID: castle.KingdomID, FeatureLabel: "Berimond", AttackLootOnly: true, EventID: GameData.BerimondEventID,
			IgnoreDemolitionCandidate: isBeriStableDefinition,
		},
	)
}

func assertBeriRequest(t *testing.T, decision *Decision, name string) {
	t.Helper()
	if decision == nil || decision.Request == nil || decision.Request.Name != name {
		t.Fatalf("decision = %#v, want %s", decision, name)
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

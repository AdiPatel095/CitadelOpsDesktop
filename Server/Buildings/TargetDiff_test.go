package Buildings

import (
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestCompileTargetDiffReusesExistingBuildingAndEmitsFullUpgradePath(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	castle := state.Castles[1]
	building := targetDiffBuilding(10, 100, 1, 1)
	castle.Layout.Objects[10] = building
	castle.Buildings[10] = building
	state.Castles[1] = castle

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		ExpectedRevision: targetDiffRevision(42), CastleID: 1,
		Buildings: []TargetBuilding{{
			TargetID: "storehouse", DefinitionID: 102,
			Placement: &TargetPlacement{GridX: 5, GridY: 5, Rotation: 1},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Compilable || result.Satisfied || len(result.Targets) != 1 {
		t.Fatalf("unexpected result state: %#v", result)
	}
	match := result.Targets[0]
	if match.Source == nil || match.Source.Kind != TargetSourceExisting || match.Source.BuildingInstanceID != 10 {
		t.Fatalf("source = %#v, want existing building 10", match.Source)
	}
	if got := targetDiffPathIDs(match.UpgradePath); got != "100,101,102" {
		t.Fatalf("upgrade path = %s", got)
	}
	if len(result.Actions) != 3 || result.Actions[0].Kind != TargetActionMove ||
		result.Actions[1].Kind != ActionUpgrade || result.Actions[1].Definition.ID != 101 ||
		result.Actions[2].Kind != ActionUpgrade || result.Actions[2].Definition.ID != 102 {
		t.Fatalf("actions = %#v", result.Actions)
	}
	if len(result.Actions[1].DependsOn) != 1 || result.Actions[1].DependsOn[0] != result.Actions[0].ID ||
		len(result.Actions[2].DependsOn) != 1 || result.Actions[2].DependsOn[0] != result.Actions[1].ID {
		t.Fatalf("action dependencies = %#v", result.Actions)
	}
	if len(result.Requirements) != 2 || targetDiffRequired(result.Requirements, "W") != 50 ||
		targetDiffRequired(result.Requirements, "S") != 25 {
		t.Fatalf("requirements = %#v", result.Requirements)
	}
}

func TestCompileTargetDiffPrefersStoredPrecursorOverConstructingRoot(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	state.Inventory.Items["storage:1"] = map[int64]int64{101: 1}

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID: 1,
		Buildings: []TargetBuilding{{
			TargetID: "stored-upgrade", DefinitionID: 102,
			Placement: &TargetPlacement{GridX: 4, GridY: 4},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	match := result.Targets[0]
	if match.Source == nil || match.Source.Kind != TargetSourceStorage || match.Source.Definition.ID != 101 {
		t.Fatalf("source = %#v, want stored level 2", match.Source)
	}
	if len(result.Actions) != 2 || result.Actions[0].Intent != "building.place" ||
		result.Actions[1].Intent != "building.upgrade" || !result.Actions[1].RequiresBinding {
		t.Fatalf("actions = %#v", result.Actions)
	}
	if got := targetDiffPathIDs(match.UpgradePath); got != "101,102" {
		t.Fatalf("upgrade path = %s", got)
	}
}

func TestCompileTargetDiffConstructsRootAndBindsSubsequentUpgrades(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID:  1,
		Buildings: []TargetBuilding{{TargetID: "new-chain", DefinitionID: 102}},
	})
	if err != nil {
		t.Fatal(err)
	}
	match := result.Targets[0]
	if match.Source == nil || match.Source.Kind != TargetSourceConstruct || match.Source.Definition.ID != 100 {
		t.Fatalf("source = %#v, want construction root 100", match.Source)
	}
	if len(result.Actions) != 3 || result.Actions[0].Definition.ID != 100 ||
		result.Actions[1].Definition.ID != 101 || result.Actions[2].Definition.ID != 102 ||
		!result.Actions[1].RequiresBinding || !result.Actions[2].RequiresBinding {
		t.Fatalf("actions = %#v", result.Actions)
	}
}

func TestCompileTargetDiffUsesMaximumSourceReuseAcrossTargets(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	castle := state.Castles[1]
	first := targetDiffBuilding(10, 100, 1, 1)
	second := targetDiffBuilding(11, 200, 6, 1)
	castle.Layout.Objects[10], castle.Buildings[10] = first, first
	castle.Layout.Objects[11], castle.Buildings[11] = second, second
	state.Castles[1] = castle

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID: 1,
		Buildings: []TargetBuilding{
			{TargetID: "first-family", DefinitionID: 102},
			{TargetID: "second-family", DefinitionID: 201},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Targets[0].Source == nil || result.Targets[0].Source.BuildingInstanceID != 10 ||
		result.Targets[1].Source == nil || result.Targets[1].Source.BuildingInstanceID != 11 {
		t.Fatalf("assignments = %#v", result.Targets)
	}
	if len(result.Actions) != 3 {
		t.Fatalf("actions = %#v, want two first-family upgrades and one second-family upgrade", result.Actions)
	}
}

func TestCompileTargetDiffReservesSpecializedPrecursorForHigherTarget(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	castle := state.Castles[1]
	root := targetDiffBuilding(10, 100, 1, 1)
	levelTwo := targetDiffBuilding(11, 101, 6, 1)
	castle.Layout.Objects[10], castle.Buildings[10] = root, root
	castle.Layout.Objects[11], castle.Buildings[11] = levelTwo, levelTwo
	state.Castles[1] = castle

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID: 1,
		Buildings: []TargetBuilding{
			{TargetID: "higher", DefinitionID: 102},
			{TargetID: "root", DefinitionID: 100},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Targets[0].Source == nil || result.Targets[0].Source.BuildingInstanceID != 11 ||
		result.Targets[1].Source == nil || result.Targets[1].Source.BuildingInstanceID != 10 {
		t.Fatalf("assignments = %#v", result.Targets)
	}
	if len(result.Actions) != 1 || result.Actions[0].FromDefinitionID != 101 || result.Actions[0].Definition.ID != 102 {
		t.Fatalf("actions = %#v", result.Actions)
	}
}

func TestCompileTargetDiffSurfacesGiftCollisionAndExactExtras(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	castle := state.Castles[1]
	chest := targetDiffBuilding(46, 520, 4, 4)
	castle.Layout.Objects[46], castle.Buildings[46] = chest, chest
	state.Castles[1] = castle

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID: 1, Exact: true,
		Buildings: []TargetBuilding{{
			TargetID: "blocked", DefinitionID: 100,
			Placement: &TargetPlacement{GridX: 4, GridY: 4},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !targetDiffHasIssue(result.Issues, "expansion_gift_blocker") ||
		!targetDiffHasIssue(result.Issues, "extra_expansion_gift") {
		t.Fatalf("issues = %#v", result.Issues)
	}
	if !result.Compilable || result.Satisfied || len(result.Unmanaged) != 1 || result.Unmanaged[0].BuildingInstanceID != 46 {
		t.Fatalf("unexpected exact result = %#v", result)
	}
}

func TestCompileTargetDiffRejectsBrokenTargetLayoutAndStaleRevision(t *testing.T) {
	store := targetDiffStore(t)
	state := targetDiffState()
	_, err := CompileTargetDiff(state, store, TargetDiffRequest{
		ExpectedRevision: targetDiffRevision(41), CastleID: 1,
	})
	var mismatch RevisionMismatchError
	if !errors.As(err, &mismatch) {
		t.Fatalf("error = %T %v, want RevisionMismatchError", err, err)
	}

	result, err := CompileTargetDiff(state, store, TargetDiffRequest{
		CastleID: 1,
		Buildings: []TargetBuilding{
			{TargetID: "one", DefinitionID: 100, Placement: &TargetPlacement{GridX: 2, GridY: 2}},
			{TargetID: "two", DefinitionID: 200, Placement: &TargetPlacement{GridX: 2, GridY: 2}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Compilable || !targetDiffHasIssue(result.Issues, "target_collision") || result.Summary.BlockedCount != 1 {
		t.Fatalf("result = %#v", result)
	}
}

func TestBuildingCatalogBuildsOnlyOfficialLinkedPaths(t *testing.T) {
	store := targetDiffStore(t)
	catalog, err := store.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	path, found := catalog.UpgradePath(100, 102)
	if !found || targetDiffDefinitionPath(path) != "100,101,102" {
		t.Fatalf("forward path = %#v", path)
	}
	if _, found := catalog.UpgradePath(102, 100); found {
		t.Fatal("a downgrade was incorrectly accepted as an upgrade path")
	}
	path, found = catalog.ConstructionPath(102)
	if !found || targetDiffDefinitionPath(path) != "100,101,102" {
		t.Fatalf("construction path = %#v", path)
	}
}

func targetDiffStore(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{"version":"target-test"},
		"units":[],
		"resources":[
			{"resourceID":"2","JSONKey":"C2","name":"premium"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"}
		],
		"currencies":[],
		"buildings":[
			{"wodID":"900","name":"Ground","group":"Ground","width":"20","height":"20"},
			{"wodID":"100","name":"Storehouse","group":"Building","type":"Level1","shopCategory":"CIVIL","level":"1","width":"2","height":"2","rotateType":"1","upgradeWodID":"101","storeable":"1","movable":"1","destructable":"1","costWood":"10","costStone":"5","buildDuration":"10"},
			{"wodID":"101","name":"Storehouse","group":"Building","type":"Level2","level":"2","width":"2","height":"2","rotateType":"1","downgradeWodID":"100","upgradeWodID":"102","storeable":"1","movable":"1","destructable":"1","costWood":"20","costStone":"10","buildDuration":"20"},
			{"wodID":"102","name":"Storehouse","group":"Building","type":"Level3","level":"3","width":"3","height":"2","rotateType":"1","downgradeWodID":"101","storeable":"1","movable":"1","destructable":"1","costWood":"30","costStone":"15","buildDuration":"30"},
			{"wodID":"200","name":"Barracks","group":"Building","type":"Level1","shopCategory":"MILITARY","level":"1","width":"2","height":"2","upgradeWodID":"201","storeable":"1","movable":"1","destructable":"1","costWood":"12","costStone":"6"},
			{"wodID":"201","name":"Barracks","group":"Building","type":"Level2","level":"2","width":"2","height":"2","downgradeWodID":"200","storeable":"1","movable":"1","destructable":"1","costWood":"24","costStone":"12"},
			{"wodID":"520","name":"TreasureChest","group":"Building","type":"Level1","level":"1","width":"4","height":"4","movable":"0","destructable":"0"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "target-test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func targetDiffState() State.GameState {
	state := State.NewGameState()
	state.Revision = 42
	state.Player.Level = 70
	state.Player.Resources[2] = 1000
	ground := targetDiffBuilding(1, 900, 0, 0)
	ground.Layer = State.BuildingLayerBG
	castle := State.CastleState{
		ID: 1, KingdomID: 0, SlotType: 1, Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{
			3: {Amount: 1000}, 4: {Amount: 1000},
		},
		Buildings: map[State.BuildingInstanceID]State.Building{1: ground},
		Layout: State.CastleLayout{
			Ground: map[State.BuildingInstanceID]State.Building{1: ground}, Objects: map[State.BuildingInstanceID]State.Building{},
			Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		},
		BuildingQueue: State.BuildingConstructionQueue{
			SlotCount: 1, ObservedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
			Slots: []State.BuildingConstructionQueueSlot{{Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable}},
		},
	}
	state.Castles[1] = castle
	state.Inventory.Items["storage:1"] = map[int64]int64{}
	return state
}

func targetDiffBuilding(instanceID State.BuildingInstanceID, definitionID State.BuildingID, x int, y int) State.Building {
	return State.Building{
		InstanceID: instanceID, DefinitionID: definitionID, GridX: x, GridY: y,
		ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD, Placed: true,
	}
}

func targetDiffPathIDs(path []TargetDefinitionRef) string {
	parts := make([]string, len(path))
	for index, definition := range path {
		parts[index] = strconv.FormatInt(int64(definition.ID), 10)
	}
	return strings.Join(parts, ",")
}

func targetDiffDefinitionPath(path []GameData.BuildingDefinition) string {
	parts := make([]string, len(path))
	for index, definition := range path {
		parts[index] = strconv.FormatInt(definition.ID, 10)
	}
	return strings.Join(parts, ",")
}

func targetDiffRequired(costs []CostStatus, key string) float64 {
	for _, cost := range costs {
		if cost.Key == key {
			return cost.Required
		}
	}
	return 0
}

func targetDiffHasIssue(issues []TargetIssue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}

func targetDiffRevision(value uint64) *uint64 { return &value }

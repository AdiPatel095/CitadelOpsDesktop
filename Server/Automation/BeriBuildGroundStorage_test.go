package Automation

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func beriOfficialBuildFixture(t *testing.T, groundCount, stores int) (Snapshot, autoStormSettings, State.CastleState) {
	t.Helper()
	raw, err := os.ReadFile("../Buildings/testdata/berimond_storage_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	target, err := Buildings.DefaultBerimondTarget(77, 5, data)
	if err != nil {
		t.Fatal(err)
	}
	capacity := 3000 + float64(stores)*7500
	castle := State.CastleState{ID: 77, KingdomID: 10, Focused: true, Buildings: map[State.BuildingInstanceID]State.Building{}, Resources: map[State.ResourceID]State.ResourceBalance{3: {Amount: capacity, Capacity: &capacity}, 4: {Amount: capacity, Capacity: &capacity}}, Layout: State.CastleLayout{Ground: map[State.BuildingInstanceID]State.Building{}, Objects: map[State.BuildingInstanceID]State.Building{}, Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: now}, BuildingQueue: State.BuildingConstructionQueue{SlotCount: 1, ObservedAt: now, Slots: []State.BuildingConstructionQueueSlot{{Index: 0, Status: State.BuildingQueueSlotAvailable}}}}
	for i, g := range target.Ground {
		if i >= groundCount {
			break
		}
		id := State.BuildingInstanceID(i + 1)
		b := State.Building{InstanceID: id, DefinitionID: g.DefinitionID, GridX: g.GridX, GridY: g.GridY, Rotation: g.Direction, Placed: true, Layer: State.BuildingLayerBG}
		castle.Layout.Ground[id] = b
		castle.Buildings[id] = b
	}
	for _, spec := range []struct {
		id   State.BuildingInstanceID
		def  State.BuildingID
		x, y int
	}{{100, 294, 210, 200}, {101, 233, 200, 200}} {
		b := State.Building{InstanceID: spec.id, DefinitionID: spec.def, GridX: spec.x, GridY: spec.y, Placed: true, Level: 1, ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD}
		castle.Layout.Objects[b.InstanceID] = b
		castle.Buildings[b.InstanceID] = b
	}
	for i := 0; i < stores; i++ {
		id := State.BuildingInstanceID(i + 200)
		b := State.Building{InstanceID: id, DefinitionID: 246, GridX: 200 + i%3*4, GridY: 206 + i/3*6, Placed: true, Level: 1, ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD}
		castle.Layout.Objects[id] = b
		castle.Buildings[id] = b
	}
	state := State.NewGameState()
	state.Player.Level = 70
	state.Castles[castle.ID] = castle
	settings := defaultAutoStormSettings()
	settings.Target = &target
	settings.Harbor.Enabled = false
	return Snapshot{State: state, GameData: data, Now: now}, settings, castle
}

func evaluateOfficialBeri(t *testing.T, snapshot Snapshot, settings autoStormSettings, castle State.CastleState, builtIn bool) (*Decision, string, map[string]float64) {
	t.Helper()
	metrics := map[string]float64{}
	if builtIn {
		metrics["builtInTarget"] = 1
	}
	decision, _, detail, err := evaluateBeriEventBuild(snapshot, settings, castle, metrics, autoEventBuildProfile{KingdomID: 10, FeatureLabel: "Berimond", AttackLootOnly: true, EventID: GameData.BerimondEventID, IgnoreDemolitionCandidate: isBeriStableDefinition})
	if err != nil {
		t.Fatal(err)
	}
	return decision, detail, metrics
}

func TestBuiltInBeriEquivalentGroundAdvancesButCustomKeepsTupleContract(t *testing.T) {
	snapshot, settings, castle := beriOfficialBuildFixture(t, 17, 2)
	raw, err := os.ReadFile("../Buildings/testdata/berimond_equivalent_ground.json")
	if err != nil {
		t.Fatal(err)
	}
	var tiles []State.Building
	if err = json.Unmarshal(raw, &tiles); err != nil {
		t.Fatal(err)
	}
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	for i, b := range tiles {
		castle.Layout.Ground[State.BuildingInstanceID(i+1)] = b
	}
	snapshot.State.Castles[castle.ID] = castle
	_, _, metrics := evaluateOfficialBeri(t, snapshot, settings, castle, true)
	if metrics["targetGroundRemaining"] != 0 || metrics["beriBuildPhase"] <= beriBuildPhaseGround {
		t.Fatalf("built-in did not advance: %v", metrics)
	}
	_, detail, metrics := evaluateOfficialBeri(t, snapshot, settings, castle, false)
	if metrics["targetGroundRemaining"] != 6 || !strings.Contains(detail, "cannot be expanded further") {
		t.Fatalf("custom tuple behavior changed: %v %s", metrics, detail)
	}
}

func TestBuiltInBeriConstructsStorageBeforeCapacityBoundExpansion(t *testing.T) {
	snapshot, settings, castle := beriOfficialBuildFixture(t, 9, 2)
	decision, detail, metrics := evaluateOfficialBeri(t, snapshot, settings, castle, true)
	if decision == nil || decision.Request == nil || decision.Request.Name != "building.construct" {
		t.Fatalf("want storage construction; got%+v detail%s metrics%v", decision, detail, metrics)
	}
	var args struct {
		DefinitionID   int64 `json:"definitionId"`
		EventID        int64 `json:"eventId"`
		X, Y, Rotation int
	}
	if err := json.Unmarshal(decision.Request.Arguments, &args); err != nil {
		t.Fatal(err)
	}
	if args.DefinitionID != 246 || args.EventID != 3 {
		t.Fatalf("constructed non-storage %s", decision.Request.Arguments)
	}
	// The construction planner must place it in currently unlocked ground.
	catalog, _ := snapshot.GameData.BuildingCatalog()
	d, _ := catalog.DefinitionView(246)
	desired := Buildings.TargetGround{DefinitionID: 246, GridX: args.X, GridY: args.Y, Direction: args.Rotation}
	if d.Width != 4 || len(Buildings.MissingGroundCoverage(castle, []Buildings.TargetGround{desired}, catalog)) != 0 {
		t.Fatal("store planned outside current ground")
	}
}

func TestBuiltInBeriReserveDoesNotCreateEighthStore(t *testing.T) {
	snapshot, settings, castle := beriOfficialBuildFixture(t, 16, 7)
	settings.Build.ResourceReserves = map[string]float64{"4": 3000}
	decision, detail, _ := evaluateOfficialBeri(t, snapshot, settings, castle, true)
	if decision != nil || !strings.Contains(detail, "all seven stores") {
		t.Fatalf("want fixed-layout capacity blocker; got%+v %s", decision, detail)
	}
}

func TestBeriStorageDependenciesRequireCorrectExplicitEvent(t *testing.T) {
	snapshot, _, castle := beriOfficialBuildFixture(t, 9, 2)
	good, wrong := int64(3), int64(4)
	for _, tc := range []struct {
		name    string
		event   *int64
		allowed bool
	}{{"missing", nil, false}, {"wrong", &wrong, false}, {"Berimond", &good, true}} {
		t.Run(tc.name, func(t *testing.T) {
			expansion, err := Buildings.PreviewExpansion(snapshot.State, snapshot.GameData, Buildings.ExpansionPreviewRequest{CastleID: castle.ID, EventID: tc.event})
			if err != nil {
				t.Fatal(err)
			}
			dependency, err := Buildings.PreviewStorageDependency(snapshot.State, snapshot.GameData, Buildings.StorageDependencyRequest{CastleID: castle.ID, EventID: tc.event, Costs: []Buildings.CostStatus{{Key: "S", DefinitionID: 4, Scope: GameData.BuildingCostCastleResource, Required: 19390}}, AllowedBuildingDefinitionIDs: []State.BuildingID{246}})
			if err != nil {
				t.Fatal(err)
			}
			for name, action := range map[string]*Buildings.ExpansionAction{"expansion": expansion.RecommendedAction, "dependency": dependency.RecommendedAction} {
				if !tc.allowed {
					if action != nil {
						t.Fatalf("%s accepted %s event: %+v", name, tc.name, action)
					}
					continue
				}
				if action == nil || action.Intent != "building.construct" || action.Arguments["definitionId"] != int64(246) || action.Arguments["eventId"] != int64(3) {
					t.Fatalf("%s lost explicit event: %+v", name, action)
				}
			}
		})
	}
}

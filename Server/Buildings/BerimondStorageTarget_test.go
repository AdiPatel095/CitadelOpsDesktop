package Buildings

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func storageTargetCatalog(t *testing.T) *GameData.Store {
	t.Helper()
	// Bounded official catalog projection, items_v786.03.json, retrieved 2026-09-22.
	raw, err := os.ReadFile("testdata/berimond_storage_786_03.json")
	if err != nil {
		t.Fatal(err)
	}
	data, err := GameData.DecodeStore(raw, GameData.SourceMetadata{ItemVersion: "786.03"})
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestBerimondExpansionFootprintRejectsPartialAndCompleteOverlap(t *testing.T) {
	data := storageTargetCatalog(t)
	catalog, err := data.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile("testdata/berimond_equivalent_ground.json")
	if err != nil {
		t.Fatal(err)
	}
	var observed []State.Building
	if err := json.Unmarshal(raw, &observed); err != nil {
		t.Fatal(err)
	}
	candidate := TargetGround{DefinitionID: 201, GridX: 180, GridY: 220, Direction: 1}
	castle := State.CastleState{Focused: true}
	castle.Layout.ObservedAt = time.Now().UTC()
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	for i, building := range observed {
		building.InstanceID = State.BuildingInstanceID(i + 1)
		castle.Layout.Ground[building.InstanceID] = building
	}
	for id, building := range castle.Layout.Ground {
		if building.GridX == 180 && building.GridY == 220 && building.Rotation == 1 ||
			building.GridX == 200 && building.GridY == 220 && building.Rotation == 1 {
			delete(castle.Layout.Ground, id)
		}
	}
	castle.Layout.Ground[98] = State.Building{InstanceID: 98, DefinitionID: 201, GridX: 180, GridY: 220, Rotation: 2, Placed: true}
	castle.Layout.Ground[99] = State.Building{InstanceID: 99, DefinitionID: 201, GridX: 190, GridY: 220, Rotation: 2, Placed: true}
	if len(castle.Layout.Ground) != 17 {
		t.Fatalf("fixture has %d ground pieces", len(castle.Layout.Ground))
	}
	if err := ValidateExpansionFootprint(castle, candidate, catalog); err == nil {
		t.Fatal("17-piece complete overlap accepted")
	}
	delete(castle.Layout.Ground, 99)
	if len(castle.Layout.Ground) != 16 {
		t.Fatalf("partial fixture has %d ground pieces", len(castle.Layout.Ground))
	}
	if err := ValidateExpansionFootprint(castle, candidate, catalog); err == nil {
		t.Fatal("16-piece partial overlap accepted")
	}
	valid := TargetGround{DefinitionID: 201, GridX: 300, GridY: 300, Direction: 1}
	if err := ValidateExpansionFootprint(castle, valid, catalog); err != nil {
		t.Fatalf("nonoverlapping expansion: %v", err)
	}
	castle.Layout.Ground[99] = State.Building{InstanceID: 99, DefinitionID: 201, GridX: 300, GridY: 300, Rotation: 1, Placed: true}
	if err := ValidateExpansionFootprint(castle, valid, catalog); err == nil {
		t.Fatal("state change before dispatch accepted")
	}
	delete(castle.Layout.Ground, 99)
	castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{100: {InstanceID: 100, DefinitionID: 242, GridX: 300, GridY: 300, Placed: true}}
	if err := ValidateExpansionFootprint(castle, valid, catalog); err == nil {
		t.Fatal("placed building overlap accepted")
	}
}

func storageTargetCastle(target TargetCaptureResult, capacity float64, count int) State.CastleState {
	castle := State.CastleState{ID: 1, KingdomID: 10, Resources: map[State.ResourceID]State.ResourceBalance{3: {Capacity: &capacity}, 4: {Capacity: &capacity}}}
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	for i, g := range target.Ground {
		castle.Layout.Ground[State.BuildingInstanceID(i+1)] = State.Building{DefinitionID: g.DefinitionID, GridX: g.GridX, GridY: g.GridY, Rotation: g.Direction, Placed: true}
	}
	for _, b := range target.Buildings {
		if b.DefinitionID != 246 || count == 0 {
			continue
		}
		id := State.BuildingInstanceID(len(castle.Layout.Objects) + 1)
		castle.Layout.Objects[id] = State.Building{InstanceID: id, DefinitionID: 246, GridX: b.Placement.GridX, GridY: b.Placement.GridY, Rotation: b.Placement.Rotation, Placed: true, Level: 1, ConstructionState: State.BuildingStateBuildCompleted}
		count--
	}
	return castle
}

func TestBerimondGroundCoverageEquivalentObservedTiling(t *testing.T) {
	data := storageTargetCatalog(t)
	catalog, _ := data.BuildingCatalog()
	target, _ := DefaultBerimondTarget(1, 5, data)
	raw, err := os.ReadFile("testdata/berimond_equivalent_ground.json")
	if err != nil {
		t.Fatal(err)
	}
	var ground []State.Building
	if err := json.Unmarshal(raw, &ground); err != nil {
		t.Fatal(err)
	}
	castle := State.CastleState{}
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	for i, b := range ground {
		castle.Layout.Ground[State.BuildingInstanceID(i+1)] = b
	}
	if missing := MissingGroundCoverage(castle, target.Ground, catalog); len(missing) != 0 {
		t.Fatalf("equivalent tiling has %d missing tiles", len(missing))
	}
	delete(castle.Layout.Ground, 17)
	if len(MissingGroundCoverage(castle, target.Ground, catalog)) == 0 {
		t.Fatal("missing buildable area accepted")
	}
	castle.Layout.Ground[17] = State.Building{DefinitionID: 999999, Placed: true}
	if len(MissingGroundCoverage(castle, target.Ground, catalog)) != len(target.Ground) {
		t.Fatal("unknown footprint accepted")
	}
}

func TestDefaultBerimondSevenStoresFitOfficialLayout(t *testing.T) {
	data := storageTargetCatalog(t)
	catalog, _ := data.BuildingCatalog()
	target, err := DefaultBerimondTarget(1, 5, data)
	if err != nil {
		t.Fatal(err)
	}
	counts := targetDefinitionCounts(target.Buildings)
	if counts[246] != 7 || counts[12] != 82 || counts[14] != 2 || target.Summary.DecorationCount != 59 {
		t.Fatalf("unexpected final layout counts: %v %+v", counts, target.Summary)
	}
	castle := storageTargetCastle(target, 55500, 0)
	grid := buildLayoutGrid(castle, catalog, 0)
	used := map[gridPoint]string{}
	for _, b := range target.Buildings {
		d, _ := catalog.DefinitionView(int64(b.DefinitionID))
		w, h := rotatedDimensions(d.Width, d.Height, b.Placement.Rotation)
		for _, c := range footprintCells(Placement{GridX: b.Placement.GridX, GridY: b.Placement.GridY, Width: w, Height: h}) {
			if _, ok := grid.ground[c]; !ok {
				t.Fatalf("outside ground %s", b.TargetID)
			}
			if previous, ok := used[c]; ok {
				t.Fatalf("overlap %s / %s", previous, b.TargetID)
			}
			used[c] = b.TargetID
		}
	}
	main, _ := catalog.DefinitionView(233)
	storage, _ := catalog.DefinitionView(246)
	expansions, _ := data.ExpansionCatalog()
	last, ok := expansions.Definition(10, 16)
	if !ok {
		t.Fatal("missing last expansion")
	}
	for _, cost := range last.Costs {
		if cost.Scope != GameData.BuildingCostCastleResource {
			continue
		}
		metric := expansionStorageMetric(cost.Key)
		capacity := metricValue(main.Values, metric) + 7*metricValue(storage.Values, metric)
		if capacity != 55500 || capacity < cost.Amount {
			t.Fatalf("capacity %.0f cannot pay %+v", capacity, cost)
		}
	}
	deco, _ := catalog.DefinitionView(339)
	if metricValue(deco.Values, "Moral")*5 != 100 {
		t.Fatal("unexpected removed morale")
	}
}

func TestDefaultBerimondStoreCountsReuseExistingBuildings(t *testing.T) {
	data := storageTargetCatalog(t)
	target, _ := DefaultBerimondTarget(1, 5, data)
	stores := []TargetBuilding{}
	for _, b := range target.Buildings {
		if b.DefinitionID == 246 {
			stores = append(stores, b)
		}
	}
	for _, count := range []int{2, 5, 7} {
		state := State.NewGameState()
		castle := storageTargetCastle(target, 55500, count)
		castle.Buildings = castle.Layout.Objects
		castle.Focused = true
		state.Castles[1] = castle
		diff, err := CompileTargetDiff(state, data, TargetDiffRequest{CastleID: 1, Exact: true, Buildings: stores})
		if err != nil {
			t.Fatal(err)
		}
		constructs := 0
		for _, action := range diff.Actions {
			if action.Intent == "building.construct" {
				constructs++
			}
		}
		if diff.Summary.UnmanagedCount != 0 || constructs != 7-count {
			t.Fatalf("existing%d: unmanaged%d constructs%d issues%+v", count, diff.Summary.UnmanagedCount, constructs, diff.Issues)
		}
	}
}

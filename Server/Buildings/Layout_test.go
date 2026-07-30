package Buildings

import (
	"testing"

	"CitadelDesktop/Server/State"
)

func TestFindPlacementUsesGroundAndExistingOccupancy(t *testing.T) {
	store := testBuildingStore(t)
	catalog, err := store.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	castle := testBuildingCastle()
	definition, _ := catalog.Definition(242)
	placement, found := FindPlacement(castle, definition, catalog, 0)
	if !found {
		t.Fatal("expected a valid placement")
	}
	if placement.GridX != 2 || placement.GridY != 0 {
		t.Fatalf("placement = %#v, want first deterministic free position at 2,0", placement)
	}
	issues := ValidatePlacement(castle, definition, Placement{GridX: 0, GridY: 0}, catalog, 0)
	if len(issues) == 0 || issues[0].Code != "collision" {
		t.Fatalf("collision was not detected: %#v", issues)
	}
	analysis := AnalyzeLayout(castle, catalog)
	if !analysis.Valid || analysis.GroundCells != 100 || analysis.OccupiedCells != 4 || analysis.FreeCells != 96 {
		t.Fatalf("layout analysis = %#v", analysis)
	}
	if castle.Layout.Objects[700].Layer != State.BuildingLayerBD {
		t.Fatal("test castle lost its source layer")
	}
}

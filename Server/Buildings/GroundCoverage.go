package Buildings

import (
	"fmt"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

// ValidateExpansionFootprint rejects a Berimond expansion whose official tile
// intersects any observed ground or placed object. Unknown geometry is unsafe.
func ValidateExpansionFootprint(castle State.CastleState, candidate TargetGround, catalog *GameData.BuildingCatalog) error {
	if catalog == nil || !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return fmt.Errorf("focused expansion layout or official geometry is unavailable")
	}
	if candidate.Direction < 0 || candidate.Direction > 3 {
		return fmt.Errorf("invalid expansion rotation %d", candidate.Direction)
	}
	d, ok := catalog.DefinitionView(int64(candidate.DefinitionID))
	if !ok || d.Width <= 0 || d.Height <= 0 {
		return fmt.Errorf("official expansion geometry %d is unavailable", candidate.DefinitionID)
	}
	w, h := rotatedDimensions(d.Width, d.Height, candidate.Direction)
	cells := map[gridPoint]bool{}
	for _, cell := range footprintCells(Placement{GridX: candidate.GridX, GridY: candidate.GridY, Width: w, Height: h}) {
		cells[cell] = true
	}
	for _, layer := range []map[State.BuildingInstanceID]State.Building{castle.Layout.Ground, castle.Layout.Objects, castle.Layout.Fixed} {
		for _, building := range layer {
			if !building.Placed {
				continue
			}
			definition, found := catalog.DefinitionView(int64(building.DefinitionID))
			if !found || definition.Width <= 0 || definition.Height <= 0 || building.Rotation < 0 || building.Rotation > 3 {
				return fmt.Errorf("official geometry for placed building %d is unavailable", building.DefinitionID)
			}
			bw, bh := rotatedDimensions(definition.Width, definition.Height, building.Rotation)
			for _, cell := range footprintCells(Placement{GridX: building.GridX, GridY: building.GridY, Width: bw, Height: bh}) {
				if cells[cell] {
					return fmt.Errorf("expansion footprint overlaps placed ground or building %d", building.InstanceID)
				}
			}
		}
	}
	return nil
}

// MissingGroundCoverage compares buildable cells, allowing equivalent expansion
// tilings. Unknown or unplaced ground never supplies coverage. Callers opt in;
// custom blueprint and Storm tuple semantics are intentionally unchanged.
func MissingGroundCoverage(castle State.CastleState, target []TargetGround, catalog *GameData.BuildingCatalog) []TargetGround {
	if catalog == nil {
		return append([]TargetGround(nil), target...)
	}
	ground := map[gridPoint]bool{}
	for _, b := range castle.Layout.Ground {
		if !b.Placed {
			continue
		}
		d, ok := catalog.DefinitionView(int64(b.DefinitionID))
		if !ok || d.Width <= 0 || d.Height <= 0 {
			return append([]TargetGround(nil), target...)
		}
		w, h := rotatedDimensions(d.Width, d.Height, b.Rotation)
		for _, c := range footprintCells(Placement{GridX: b.GridX, GridY: b.GridY, Width: w, Height: h}) {
			ground[c] = true
		}
	}
	missing := []TargetGround{}
	for _, t := range target {
		d, ok := catalog.DefinitionView(int64(t.DefinitionID))
		if !ok || d.Width <= 0 || d.Height <= 0 {
			missing = append(missing, t)
			continue
		}
		w, h := rotatedDimensions(d.Width, d.Height, t.Direction)
		for _, c := range footprintCells(Placement{GridX: t.GridX, GridY: t.GridY, Width: w, Height: h}) {
			if !ground[c] {
				missing = append(missing, t)
				break
			}
		}
	}
	return missing
}

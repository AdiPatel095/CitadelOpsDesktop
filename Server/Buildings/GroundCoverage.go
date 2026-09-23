package Buildings

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

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

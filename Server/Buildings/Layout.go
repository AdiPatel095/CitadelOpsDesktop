package Buildings

import (
	"fmt"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type Bounds struct {
	MinX int `json:"minX"`
	MinY int `json:"minY"`
	MaxX int `json:"maxX"`
	MaxY int `json:"maxY"`
}

type Placement struct {
	GridX    int `json:"gridX"`
	GridY    int `json:"gridY"`
	Rotation int `json:"rotation"`
	Width    int `json:"width"`
	Height   int `json:"height"`
}

type LayoutIssue struct {
	Code        string                     `json:"code"`
	Message     string                     `json:"message"`
	Severity    string                     `json:"severity"`
	BuildingIDs []State.BuildingInstanceID `json:"buildingIds,omitempty"`
}

type LayoutAnalysis struct {
	Valid         bool          `json:"valid"`
	Bounds        *Bounds       `json:"bounds,omitempty"`
	GroundCells   int           `json:"groundCells"`
	OccupiedCells int           `json:"occupiedCells"`
	FreeCells     int           `json:"freeCells"`
	PlacedObjects int           `json:"placedObjects"`
	StoredObjects int           `json:"storedObjects"`
	FixedObjects  int           `json:"fixedObjects"`
	Issues        []LayoutIssue `json:"issues"`
}

type gridPoint struct {
	x int
	y int
}

type layoutGrid struct {
	ground   map[gridPoint]struct{}
	occupied map[gridPoint]State.BuildingInstanceID
	bounds   *Bounds
	issues   []LayoutIssue
	placed   int
	stored   int
}

func AnalyzeLayout(castle State.CastleState, catalog *GameData.BuildingCatalog) LayoutAnalysis {
	grid := buildLayoutGrid(castle, catalog, 0)
	occupiedGround := 0
	for cell := range grid.occupied {
		if _, found := grid.ground[cell]; found {
			occupiedGround++
		}
	}
	free := len(grid.ground) - occupiedGround
	if free < 0 {
		free = 0
	}
	valid := true
	for _, issue := range grid.issues {
		if issue.Severity == "error" {
			valid = false
			break
		}
	}
	return LayoutAnalysis{
		Valid: valid, Bounds: grid.bounds, GroundCells: len(grid.ground), OccupiedCells: len(grid.occupied),
		FreeCells: free, PlacedObjects: grid.placed, StoredObjects: grid.stored,
		FixedObjects: len(castle.Layout.Fixed), Issues: grid.issues,
	}
}

func FindPlacement(
	castle State.CastleState,
	definition GameData.BuildingDefinition,
	catalog *GameData.BuildingCatalog,
	ignoreBuildingID State.BuildingInstanceID,
) (*Placement, bool) {
	grid := buildLayoutGrid(castle, catalog, ignoreBuildingID)
	if grid.bounds == nil || definition.Width <= 0 || definition.Height <= 0 {
		return nil, false
	}
	for y := grid.bounds.MinY; y < grid.bounds.MaxY; y++ {
		for x := grid.bounds.MinX; x < grid.bounds.MaxX; x++ {
			for _, rotation := range candidateRotations(definition) {
				width, height := rotatedDimensions(definition.Width, definition.Height, rotation)
				placement := Placement{GridX: x, GridY: y, Rotation: rotation, Width: width, Height: height}
				if placementFits(grid, placement) {
					return &placement, true
				}
			}
		}
	}
	return nil, false
}

func ValidatePlacement(
	castle State.CastleState,
	definition GameData.BuildingDefinition,
	placement Placement,
	catalog *GameData.BuildingCatalog,
	ignoreBuildingID State.BuildingInstanceID,
) []LayoutIssue {
	grid := buildLayoutGrid(castle, catalog, ignoreBuildingID)
	if definition.Width <= 0 || definition.Height <= 0 {
		return []LayoutIssue{{Code: "invalid_dimensions", Message: "The official definition has no usable footprint", Severity: "error"}}
	}
	placement.Width, placement.Height = rotatedDimensions(definition.Width, definition.Height, placement.Rotation)
	issues := make([]LayoutIssue, 0, 2)
	outside := false
	collisions := map[State.BuildingInstanceID]struct{}{}
	for _, cell := range footprintCells(placement) {
		if _, found := grid.ground[cell]; !found {
			outside = true
		}
		if occupant, found := grid.occupied[cell]; found {
			collisions[occupant] = struct{}{}
		}
	}
	if outside {
		issues = append(issues, LayoutIssue{Code: "outside_ground", Message: "The footprint extends outside unlocked ground", Severity: "error"})
	}
	if len(collisions) > 0 {
		ids := make([]State.BuildingInstanceID, 0, len(collisions))
		for id := range collisions {
			ids = append(ids, id)
		}
		sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
		issues = append(issues, LayoutIssue{
			Code: "collision", Message: "The footprint overlaps an existing object", Severity: "error", BuildingIDs: ids,
		})
	}
	return issues
}

func buildLayoutGrid(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	ignoreBuildingID State.BuildingInstanceID,
) layoutGrid {
	grid := layoutGrid{ground: map[gridPoint]struct{}{}, occupied: map[gridPoint]State.BuildingInstanceID{}}
	if catalog == nil {
		grid.issues = append(grid.issues, LayoutIssue{
			Code: "catalog_unavailable", Message: "The official building catalog is unavailable", Severity: "error",
		})
		return grid
	}
	groundIDs := sortedBuildingIDs(castle.Layout.Ground)
	for _, id := range groundIDs {
		building := castle.Layout.Ground[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || definition.Width <= 0 || definition.Height <= 0 {
			grid.issues = append(grid.issues, LayoutIssue{
				Code: "unknown_ground", Message: fmt.Sprintf("Ground object %d has no usable official definition", id),
				Severity: "error", BuildingIDs: []State.BuildingInstanceID{id},
			})
			continue
		}
		if !building.Placed {
			continue
		}
		width, height := rotatedDimensions(definition.Width, definition.Height, building.Rotation)
		for _, cell := range footprintCells(Placement{GridX: building.GridX, GridY: building.GridY, Width: width, Height: height}) {
			grid.ground[cell] = struct{}{}
			grid.bounds = extendBounds(grid.bounds, cell)
		}
	}
	if len(grid.ground) == 0 {
		grid.issues = append(grid.issues, LayoutIssue{
			Code: "ground_unavailable", Message: "No normalized unlocked ground has been observed for this castle", Severity: "error",
		})
	}

	collisionPairs := map[string][]State.BuildingInstanceID{}
	objectIDs := sortedBuildingIDs(castle.Layout.Objects)
	for _, id := range objectIDs {
		if id == ignoreBuildingID {
			continue
		}
		building := castle.Layout.Objects[id]
		if !building.Placed {
			grid.stored++
			continue
		}
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || definition.Width <= 0 || definition.Height <= 0 {
			grid.issues = append(grid.issues, LayoutIssue{
				Code: "unknown_object", Message: fmt.Sprintf("Object %d has no usable official definition", id),
				Severity: "warning", BuildingIDs: []State.BuildingInstanceID{id},
			})
			continue
		}
		grid.placed++
		width, height := rotatedDimensions(definition.Width, definition.Height, building.Rotation)
		outside := false
		for _, cell := range footprintCells(Placement{GridX: building.GridX, GridY: building.GridY, Width: width, Height: height}) {
			if _, found := grid.ground[cell]; !found {
				outside = true
			}
			if occupant, found := grid.occupied[cell]; found && occupant != id {
				left, right := occupant, id
				if left > right {
					left, right = right, left
				}
				key := fmt.Sprintf("%d:%d", left, right)
				collisionPairs[key] = []State.BuildingInstanceID{left, right}
			}
			grid.occupied[cell] = id
		}
		if outside {
			grid.issues = append(grid.issues, LayoutIssue{
				Code: "object_outside_ground", Message: fmt.Sprintf("Object %d extends outside unlocked ground", id),
				Severity: "error", BuildingIDs: []State.BuildingInstanceID{id},
			})
		}
	}
	pairKeys := make([]string, 0, len(collisionPairs))
	for key := range collisionPairs {
		pairKeys = append(pairKeys, key)
	}
	sort.Strings(pairKeys)
	for _, key := range pairKeys {
		ids := collisionPairs[key]
		grid.issues = append(grid.issues, LayoutIssue{
			Code: "object_collision", Message: fmt.Sprintf("Objects %d and %d overlap", ids[0], ids[1]),
			Severity: "error", BuildingIDs: ids,
		})
	}
	return grid
}

func placementFits(grid layoutGrid, placement Placement) bool {
	for _, cell := range footprintCells(placement) {
		if _, found := grid.ground[cell]; !found {
			return false
		}
		if _, found := grid.occupied[cell]; found {
			return false
		}
	}
	return true
}

func footprintCells(placement Placement) []gridPoint {
	if placement.Width <= 0 || placement.Height <= 0 {
		return nil
	}
	result := make([]gridPoint, 0, placement.Width*placement.Height)
	for y := placement.GridY; y < placement.GridY+placement.Height; y++ {
		for x := placement.GridX; x < placement.GridX+placement.Width; x++ {
			result = append(result, gridPoint{x: x, y: y})
		}
	}
	return result
}

func rotatedDimensions(width, height, rotation int) (int, int) {
	if rotation%2 != 0 {
		return height, width
	}
	return width, height
}

func candidateRotations(definition GameData.BuildingDefinition) []int {
	if definition.Width != definition.Height {
		return []int{0, 1}
	}
	return []int{0}
}

func extendBounds(bounds *Bounds, point gridPoint) *Bounds {
	if bounds == nil {
		return &Bounds{MinX: point.x, MinY: point.y, MaxX: point.x + 1, MaxY: point.y + 1}
	}
	if point.x < bounds.MinX {
		bounds.MinX = point.x
	}
	if point.y < bounds.MinY {
		bounds.MinY = point.y
	}
	if point.x >= bounds.MaxX {
		bounds.MaxX = point.x + 1
	}
	if point.y >= bounds.MaxY {
		bounds.MaxY = point.y + 1
	}
	return bounds
}

func sortedBuildingIDs(buildings map[State.BuildingInstanceID]State.Building) []State.BuildingInstanceID {
	ids := make([]State.BuildingInstanceID, 0, len(buildings))
	for id := range buildings {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

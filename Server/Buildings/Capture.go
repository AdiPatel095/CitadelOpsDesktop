package Buildings

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	TargetCaptureModeFunctional = "functional"
	TargetCaptureModeLayout     = "layout"
	TargetCaptureModeExact      = "exact"

	TargetCaptureModeBuildings = "buildings"
	TargetCaptureModeFull      = "full"
)

type TargetCaptureRequest struct {
	ExpectedRevision *uint64        `json:"expectedRevision,omitempty"`
	CastleID         State.CastleID `json:"castleId"`
	Mode             string         `json:"mode"`
}

type TargetGround struct {
	DefinitionID State.BuildingID `json:"definitionId"`
	GridX        int              `json:"gridX"`
	GridY        int              `json:"gridY"`
	Direction    int              `json:"direction"`
}

type TargetCaptureSummary struct {
	GroundCount     int `json:"groundCount"`
	BuildingCount   int `json:"buildingCount"`
	DecorationCount int `json:"decorationCount"`
	FixedCount      int `json:"fixedCount"`
}

type TargetCaptureResult struct {
	Version        int                   `json:"version"`
	Revision       uint64                `json:"revision"`
	CatalogVersion string                `json:"catalogVersion,omitempty"`
	CapturedAt     time.Time             `json:"capturedAt"`
	CastleID       State.CastleID        `json:"castleId"`
	KingdomID      State.KingdomID       `json:"kingdomId"`
	Mode           string                `json:"mode"`
	Exact          bool                  `json:"exact"`
	Ground         []TargetGround        `json:"ground"`
	Buildings      []TargetBuilding      `json:"buildings"`
	Fixed          []TargetFixedBuilding `json:"fixed"`
	Summary        TargetCaptureSummary  `json:"summary"`
}

type TargetFixedBuilding struct {
	TargetID     string           `json:"targetId,omitempty"`
	DefinitionID State.BuildingID `json:"definitionId"`
	Slot         *TargetPlacement `json:"slot,omitempty"`
}

func CaptureTarget(state State.GameState, gameData *GameData.Store, request TargetCaptureRequest) (TargetCaptureResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return TargetCaptureResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return TargetCaptureResult{}, fmt.Errorf("official game data is unavailable")
	}
	mode := strings.ToLower(strings.TrimSpace(request.Mode))
	if mode == "" {
		mode = TargetCaptureModeExact
	}
	switch mode {
	case TargetCaptureModeBuildings:
		mode = TargetCaptureModeLayout
	case TargetCaptureModeFull:
		mode = TargetCaptureModeExact
	}
	if mode != TargetCaptureModeFunctional && mode != TargetCaptureModeLayout && mode != TargetCaptureModeExact {
		return TargetCaptureResult{}, fmt.Errorf(
			"mode must be %q, %q, or %q",
			TargetCaptureModeFunctional, TargetCaptureModeLayout, TargetCaptureModeExact,
		)
	}
	castle, found := state.Castles[request.CastleID]
	if !found || request.CastleID <= 0 {
		return TargetCaptureResult{}, fmt.Errorf("castle %d was not found", request.CastleID)
	}
	if castle.Layout.ObservedAt.IsZero() {
		return TargetCaptureResult{}, fmt.Errorf("castle %d has no observed layout", request.CastleID)
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return TargetCaptureResult{}, err
	}
	castle = normalizePreviewLayout(castle, catalog)
	result := TargetCaptureResult{
		Version: 1, Revision: state.Revision, CatalogVersion: gameData.Metadata().ItemVersion,
		CapturedAt: time.Now().UTC(), CastleID: castle.ID, KingdomID: castle.KingdomID,
		Mode: mode, Exact: mode == TargetCaptureModeExact,
		Ground: []TargetGround{}, Buildings: []TargetBuilding{}, Fixed: []TargetFixedBuilding{},
	}

	for _, id := range sortedBuildingIDs(castle.Layout.Ground) {
		ground := castle.Layout.Ground[id]
		if ground.DefinitionID <= 0 {
			continue
		}
		result.Ground = append(result.Ground, TargetGround{
			DefinitionID: ground.DefinitionID, GridX: ground.GridX, GridY: ground.GridY, Direction: ground.Rotation,
		})
	}
	result.Summary.GroundCount = len(result.Ground)

	type capturedObject struct {
		building State.Building
		fixed    bool
	}
	objects := make([]capturedObject, 0, len(castle.Layout.Objects)+len(castle.Layout.Fixed))
	for _, id := range sortedBuildingIDs(castle.Layout.Objects) {
		objects = append(objects, capturedObject{building: castle.Layout.Objects[id]})
	}
	for _, id := range sortedBuildingIDs(castle.Layout.Fixed) {
		objects = append(objects, capturedObject{building: castle.Layout.Fixed[id], fixed: true})
	}
	sort.SliceStable(objects, func(left, right int) bool {
		if objects[left].fixed != objects[right].fixed {
			return !objects[left].fixed
		}
		return objects[left].building.InstanceID < objects[right].building.InstanceID
	})
	for _, object := range objects {
		definition, exists := catalog.Definition(int64(object.building.DefinitionID))
		if !exists || !object.building.Placed || strings.EqualFold(definition.InternalName, "TreasureChest") {
			continue
		}
		decoration := strings.EqualFold(definition.GroundType, "DECO") || strings.EqualFold(definition.InternalName, "Deco")
		if mode != TargetCaptureModeExact && decoration {
			continue
		}
		placement := &TargetPlacement{
			GridX: object.building.GridX, GridY: object.building.GridY, Rotation: object.building.Rotation,
		}
		if object.fixed {
			result.Fixed = append(result.Fixed, TargetFixedBuilding{
				TargetID:     fmt.Sprintf("fixed-%03d-%d", len(result.Fixed)+1, definition.ID),
				DefinitionID: object.building.DefinitionID,
				Slot:         placement,
			})
			result.Summary.FixedCount++
			continue
		}
		if mode == TargetCaptureModeFunctional {
			placement = nil
		}
		result.Buildings = append(result.Buildings, TargetBuilding{
			TargetID:     fmt.Sprintf("target-%03d-%d", len(result.Buildings)+1, definition.ID),
			DefinitionID: object.building.DefinitionID, Placement: placement,
		})
		if decoration {
			result.Summary.DecorationCount++
		} else {
			result.Summary.BuildingCount++
		}
	}
	return result, nil
}

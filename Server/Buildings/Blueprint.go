package Buildings

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const StormBlueprintConfigurationSection = "automation.autoStormBlueprints"

type StormBlueprint struct {
	ID        string              `json:"id"`
	Name      string              `json:"name"`
	CreatedAt time.Time           `json:"createdAt"`
	UpdatedAt time.Time           `json:"updatedAt"`
	Target    TargetCaptureResult `json:"target"`
}

type StormBlueprintDocument struct {
	Version    int                       `json:"version"`
	ActiveID   string                    `json:"activeId,omitempty"`
	Blueprints map[string]StormBlueprint `json:"blueprints"`
}

type BlueprintDiffRequest struct {
	ExpectedRevision *uint64             `json:"expectedRevision,omitempty"`
	Target           TargetCaptureResult `json:"target"`
	Policy           TargetDiffPolicy    `json:"policy"`
}

type BlueprintDiffResult struct {
	Revision       uint64              `json:"revision"`
	CatalogVersion string              `json:"catalogVersion,omitempty"`
	Target         TargetCaptureResult `json:"target"`
	Normal         TargetDiffResult    `json:"normal"`
	Fixed          TargetDiffResult    `json:"fixed"`
	MissingGround  []TargetGround      `json:"missingGround"`
	Compilable     bool                `json:"compilable"`
	Satisfied      bool                `json:"satisfied"`
	TargetCount    int                 `json:"targetCount"`
	SatisfiedCount int                 `json:"satisfiedCount"`
	PlannedCount   int                 `json:"plannedCount"`
	WaitingCount   int                 `json:"waitingCount"`
	BlockedCount   int                 `json:"blockedCount"`
	ActionCount    int                 `json:"actionCount"`
}

func EmptyStormBlueprintDocument() StormBlueprintDocument {
	return StormBlueprintDocument{Version: 1, Blueprints: map[string]StormBlueprint{}}
}

func DecodeStormBlueprintDocument(raw json.RawMessage, gameData *GameData.Store) (StormBlueprintDocument, error) {
	document := EmptyStormBlueprintDocument()
	if len(raw) == 0 {
		return document, nil
	}
	if err := json.Unmarshal(raw, &document); err != nil {
		return StormBlueprintDocument{}, fmt.Errorf("decode Storm blueprint document: %w", err)
	}
	if document.Version == 0 {
		document.Version = 1
	}
	if document.Version != 1 {
		return StormBlueprintDocument{}, fmt.Errorf("unsupported Storm blueprint document version %d", document.Version)
	}
	if document.Blueprints == nil {
		document.Blueprints = map[string]StormBlueprint{}
	}
	var catalog *GameData.BuildingCatalog
	if gameData != nil {
		var err error
		catalog, err = gameData.BuildingCatalog()
		if err != nil {
			return StormBlueprintDocument{}, err
		}
	}
	normalized := make(map[string]StormBlueprint, len(document.Blueprints))
	for key, blueprint := range document.Blueprints {
		id := strings.TrimSpace(blueprint.ID)
		if id == "" {
			id = strings.TrimSpace(key)
		}
		if id == "" {
			continue
		}
		blueprint.ID = id
		blueprint.Name = strings.TrimSpace(blueprint.Name)
		blueprint.Target = NormalizeTargetCapture(blueprint.Target, catalog)
		if blueprint.Target.Mode != TargetCaptureModeFunctional &&
			blueprint.Target.Mode != TargetCaptureModeLayout &&
			blueprint.Target.Mode != TargetCaptureModeExact {
			return StormBlueprintDocument{}, fmt.Errorf(
				"Storm blueprint %q has unsupported mode %q", id, blueprint.Target.Mode,
			)
		}
		normalized[id] = blueprint
	}
	document.Blueprints = normalized
	document.ActiveID = strings.TrimSpace(document.ActiveID)
	if document.ActiveID != "" {
		if _, exists := document.Blueprints[document.ActiveID]; !exists {
			document.ActiveID = ""
		}
	}
	return document, nil
}

func (document StormBlueprintDocument) Active() (StormBlueprint, bool) {
	if strings.TrimSpace(document.ActiveID) == "" {
		return StormBlueprint{}, false
	}
	blueprint, found := document.Blueprints[document.ActiveID]
	return blueprint, found
}

func StormBlueprintID(mode string) string {
	switch normalizeTargetCaptureMode(mode) {
	case TargetCaptureModeFunctional:
		return "storm-functional"
	case TargetCaptureModeLayout:
		return "storm-layout"
	default:
		return "storm-exact"
	}
}

func StormBlueprintName(mode string) string {
	switch normalizeTargetCaptureMode(mode) {
	case TargetCaptureModeFunctional:
		return "Functional target"
	case TargetCaptureModeLayout:
		return "Layout target"
	default:
		return "Exact clone"
	}
}

func NormalizeTargetCapture(target TargetCaptureResult, catalog *GameData.BuildingCatalog) TargetCaptureResult {
	target.Version = 1
	target.Mode = normalizeTargetCaptureMode(target.Mode)
	target.Exact = target.Mode == TargetCaptureModeExact
	if target.Ground == nil {
		target.Ground = []TargetGround{}
	}
	if target.Buildings == nil {
		target.Buildings = []TargetBuilding{}
	}
	if target.Fixed == nil {
		target.Fixed = []TargetFixedBuilding{}
	}
	if catalog != nil {
		buildings := make([]TargetBuilding, 0, len(target.Buildings))
		for _, building := range target.Buildings {
			definition, found := catalog.Definition(int64(building.DefinitionID))
			if found && fixedTargetDefinition(definition) {
				target.Fixed = append(target.Fixed, TargetFixedBuilding{
					TargetID: building.TargetID, DefinitionID: building.DefinitionID,
					Slot: cloneTargetPlacement(building.Placement),
				})
				continue
			}
			if target.Mode == TargetCaptureModeFunctional {
				building.Placement = nil
			}
			buildings = append(buildings, building)
		}
		target.Buildings = buildings
	}
	target.Summary.GroundCount = len(target.Ground)
	target.Summary.FixedCount = len(target.Fixed)
	target.Summary.BuildingCount = 0
	target.Summary.DecorationCount = 0
	for _, building := range target.Buildings {
		definition, found := GameData.BuildingDefinition{}, false
		if catalog != nil {
			definition, found = catalog.Definition(int64(building.DefinitionID))
		}
		if found && (strings.EqualFold(definition.GroundType, "DECO") || strings.EqualFold(definition.InternalName, "Deco")) {
			target.Summary.DecorationCount++
		} else {
			target.Summary.BuildingCount++
		}
	}
	return target
}

func CompileBlueprintDiff(
	state State.GameState,
	gameData *GameData.Store,
	request BlueprintDiffRequest,
) (BlueprintDiffResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return BlueprintDiffResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return BlueprintDiffResult{}, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return BlueprintDiffResult{}, err
	}
	target := NormalizeTargetCapture(request.Target, catalog)
	if target.Mode != TargetCaptureModeFunctional && target.Mode != TargetCaptureModeLayout && target.Mode != TargetCaptureModeExact {
		return BlueprintDiffResult{}, fmt.Errorf("unsupported Storm blueprint mode %q", target.Mode)
	}
	if target.CastleID <= 0 {
		return BlueprintDiffResult{}, fmt.Errorf("Storm blueprint castleId is required")
	}
	if target.KingdomID != State.KingdomID(GameData.StormKingdomID) {
		return BlueprintDiffResult{}, fmt.Errorf("Storm blueprint must target kingdom %d", GameData.StormKingdomID)
	}
	if _, exists := state.Castles[target.CastleID]; !exists {
		return BlueprintDiffResult{}, fmt.Errorf("castle %d was not found", target.CastleID)
	}
	projected := projectBlueprintGround(state, target)
	ignoreDecorations := target.Mode != TargetCaptureModeExact
	normal, err := CompileTargetDiff(projected, gameData, TargetDiffRequest{
		CastleID: target.CastleID, Exact: target.Exact,
		Policy: TargetDiffPolicy{
			AllowPremium: request.Policy.AllowPremium, IgnoreDecorations: ignoreDecorations,
			ResourceReserves: request.Policy.ResourceReserves,
		},
		Buildings: target.Buildings,
	})
	if err != nil {
		return BlueprintDiffResult{}, err
	}
	fixed, err := CompileFixedTargetDiff(projected, gameData, FixedTargetDiffRequest{
		CastleID: target.CastleID, Policy: request.Policy, Fixed: target.Fixed,
	})
	if err != nil {
		return BlueprintDiffResult{}, err
	}
	missingGround := blueprintMissingGround(state.Castles[target.CastleID], target.Ground)
	result := BlueprintDiffResult{
		Revision: state.Revision, CatalogVersion: gameData.Metadata().ItemVersion, Target: target,
		Normal: normal, Fixed: fixed, MissingGround: missingGround,
		Compilable:     normal.Compilable && fixed.Compilable,
		Satisfied:      normal.Satisfied && fixed.Satisfied && len(missingGround) == 0,
		TargetCount:    normal.Summary.TargetCount + fixed.Summary.TargetCount,
		SatisfiedCount: normal.Summary.SatisfiedCount + fixed.Summary.SatisfiedCount,
		PlannedCount:   normal.Summary.PlannedCount + fixed.Summary.PlannedCount,
		WaitingCount:   normal.Summary.WaitingCount + fixed.Summary.WaitingCount,
		BlockedCount:   normal.Summary.BlockedCount + fixed.Summary.BlockedCount,
		ActionCount:    normal.Summary.ActionCount + fixed.Summary.ActionCount,
	}
	return result, nil
}

func normalizeTargetCaptureMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case TargetCaptureModeFunctional:
		return TargetCaptureModeFunctional
	case TargetCaptureModeBuildings, TargetCaptureModeLayout:
		return TargetCaptureModeLayout
	case TargetCaptureModeFull, TargetCaptureModeExact, "":
		return TargetCaptureModeExact
	default:
		return strings.ToLower(strings.TrimSpace(mode))
	}
}

func projectBlueprintGround(state State.GameState, target TargetCaptureResult) State.GameState {
	projected := state
	projected.Castles = make(map[State.CastleID]State.CastleState, len(state.Castles))
	for id, castle := range state.Castles {
		projected.Castles[id] = castle
	}
	castle := projected.Castles[target.CastleID]
	castle.Layout.Ground = cloneBuildingMap(castle.Layout.Ground)
	nextID := State.BuildingInstanceID(-1)
	for _, desired := range target.Ground {
		if blueprintGroundPresent(castle, desired) {
			continue
		}
		for {
			if _, exists := castle.Layout.Ground[nextID]; !exists {
				break
			}
			nextID--
		}
		castle.Layout.Ground[nextID] = State.Building{
			InstanceID: nextID, DefinitionID: desired.DefinitionID,
			GridX: desired.GridX, GridY: desired.GridY, Rotation: desired.Direction,
			ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerG, Placed: true,
		}
		nextID--
	}
	projected.Castles[target.CastleID] = castle
	return projected
}

func blueprintMissingGround(castle State.CastleState, target []TargetGround) []TargetGround {
	result := make([]TargetGround, 0)
	for _, desired := range target {
		if !blueprintGroundPresent(castle, desired) {
			result = append(result, desired)
		}
	}
	sort.Slice(result, func(left, right int) bool {
		if result[left].GridY != result[right].GridY {
			return result[left].GridY < result[right].GridY
		}
		if result[left].GridX != result[right].GridX {
			return result[left].GridX < result[right].GridX
		}
		return result[left].Direction < result[right].Direction
	})
	return result
}

func blueprintGroundPresent(castle State.CastleState, desired TargetGround) bool {
	for _, current := range castle.Layout.Ground {
		if current.Placed && current.DefinitionID == desired.DefinitionID &&
			current.GridX == desired.GridX && current.GridY == desired.GridY && current.Rotation == desired.Direction {
			return true
		}
	}
	return false
}

func cloneBuildingMap(source map[State.BuildingInstanceID]State.Building) map[State.BuildingInstanceID]State.Building {
	result := make(map[State.BuildingInstanceID]State.Building, len(source))
	for id, building := range source {
		result[id] = building
	}
	return result
}

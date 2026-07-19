package Buildings

import (
	"fmt"
	"sort"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	TargetActionPlace = "place"
	TargetActionMove  = "move"

	TargetSourceExisting  = "existing"
	TargetSourceFixed     = "fixed"
	TargetSourceStorage   = "storage"
	TargetSourceConstruct = "construct"

	TargetStatusSatisfied = "satisfied"
	TargetStatusPlanned   = "planned"
	TargetStatusWaiting   = "waiting"
	TargetStatusBlocked   = "blocked"

	TargetIssueError   = "error"
	TargetIssueWaiting = "waiting"
	TargetIssueWarning = "warning"
)

type TargetDiffPolicy struct {
	AllowPremium      bool               `json:"allowPremium"`
	IgnoreDecorations bool               `json:"ignoreDecorations,omitempty"`
	ResourceReserves  map[string]float64 `json:"resourceReserves,omitempty"`
}

type TargetDiffRequest struct {
	ExpectedRevision *uint64          `json:"expectedRevision,omitempty"`
	CastleID         State.CastleID   `json:"castleId,omitempty"`
	EventID          *int64           `json:"eventId,omitempty"`
	MapID            *int64           `json:"mapId,omitempty"`
	Exact            bool             `json:"exact"`
	Policy           TargetDiffPolicy `json:"policy"`
	Buildings        []TargetBuilding `json:"buildings"`
}

type TargetBuilding struct {
	TargetID     string           `json:"targetId,omitempty"`
	DefinitionID State.BuildingID `json:"definitionId"`
	Placement    *TargetPlacement `json:"placement,omitempty"`
}

type TargetPlacement struct {
	GridX    int `json:"gridX"`
	GridY    int `json:"gridY"`
	Rotation int `json:"rotation"`
}

type TargetDefinitionRef struct {
	ID           State.BuildingID `json:"id"`
	InternalName string           `json:"internalName,omitempty"`
	DisplayName  string           `json:"displayName,omitempty"`
	Level        int64            `json:"level,omitempty"`
}

type TargetSource struct {
	Kind               string                   `json:"kind"`
	Key                string                   `json:"key"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId,omitempty"`
	Definition         TargetDefinitionRef      `json:"definition"`
}

type TargetIssue struct {
	Severity     string                     `json:"severity"`
	Code         string                     `json:"code"`
	Message      string                     `json:"message"`
	TargetID     string                     `json:"targetId,omitempty"`
	DefinitionID State.BuildingID           `json:"definitionId,omitempty"`
	BuildingIDs  []State.BuildingInstanceID `json:"buildingIds,omitempty"`
}

type TargetAction struct {
	ID                 string                   `json:"id"`
	Sequence           int                      `json:"sequence"`
	Kind               string                   `json:"kind"`
	Intent             string                   `json:"intent"`
	TargetID           string                   `json:"targetId"`
	SubjectID          string                   `json:"subjectId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId,omitempty"`
	FromDefinitionID   State.BuildingID         `json:"fromDefinitionId,omitempty"`
	Definition         TargetDefinitionRef      `json:"definition"`
	FinalDefinitionID  State.BuildingID         `json:"finalDefinitionId"`
	Placement          *TargetPlacement         `json:"placement,omitempty"`
	DependsOn          []string                 `json:"dependsOn,omitempty"`
	DurationSec        int64                    `json:"durationSec,omitempty"`
	Costs              []CostStatus             `json:"costs"`
	AffordableNow      bool                     `json:"affordableNow"`
	RequiresBinding    bool                     `json:"requiresBinding"`
}

type TargetMatch struct {
	TargetID           string                `json:"targetId"`
	Desired            TargetDefinitionRef   `json:"desired"`
	RequestedPlacement *TargetPlacement      `json:"requestedPlacement,omitempty"`
	ResolvedPlacement  *TargetPlacement      `json:"resolvedPlacement,omitempty"`
	Source             *TargetSource         `json:"source,omitempty"`
	UpgradePath        []TargetDefinitionRef `json:"upgradePath"`
	ActionIDs          []string              `json:"actionIds"`
	Status             string                `json:"status"`
	Issues             []TargetIssue         `json:"issues"`
}

type TargetUnmanagedBuilding struct {
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	Definition         TargetDefinitionRef      `json:"definition"`
	Placement          *TargetPlacement         `json:"placement,omitempty"`
}

type TargetDiffSummary struct {
	TargetCount    int `json:"targetCount"`
	SatisfiedCount int `json:"satisfiedCount"`
	PlannedCount   int `json:"plannedCount"`
	WaitingCount   int `json:"waitingCount"`
	BlockedCount   int `json:"blockedCount"`
	ActionCount    int `json:"actionCount"`
	UnmanagedCount int `json:"unmanagedCount"`
}

type TargetDiffResult struct {
	Revision       uint64                    `json:"revision"`
	CatalogVersion string                    `json:"catalogVersion,omitempty"`
	CastleID       State.CastleID            `json:"castleId"`
	Exact          bool                      `json:"exact"`
	Compilable     bool                      `json:"compilable"`
	Satisfied      bool                      `json:"satisfied"`
	Targets        []TargetMatch             `json:"targets"`
	Actions        []TargetAction            `json:"actions"`
	Requirements   []CostStatus              `json:"requirements"`
	Unmanaged      []TargetUnmanagedBuilding `json:"unmanaged"`
	Issues         []TargetIssue             `json:"issues"`
	Warnings       []string                  `json:"warnings"`
	Summary        TargetDiffSummary         `json:"summary"`
}

type targetDiffWork struct {
	target     TargetBuilding
	definition GameData.BuildingDefinition
	valid      bool
}

type targetDiffSource struct {
	kind       string
	key        string
	building   State.Building
	definition GameData.BuildingDefinition
}

type targetDiffCandidate struct {
	sourceIndex int
	targetIndex int
	cost        int64
}

func CompileTargetDiff(state State.GameState, gameData *GameData.Store, request TargetDiffRequest) (TargetDiffResult, error) {
	if request.ExpectedRevision != nil && *request.ExpectedRevision != state.Revision {
		return TargetDiffResult{}, RevisionMismatchError{Expected: *request.ExpectedRevision, Actual: state.Revision}
	}
	if gameData == nil {
		return TargetDiffResult{}, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return TargetDiffResult{}, err
	}
	castleID, castle, found := previewCastle(state, request.CastleID)
	if !found {
		if request.CastleID > 0 {
			return TargetDiffResult{}, fmt.Errorf("castle %d was not found", request.CastleID)
		}
		return TargetDiffResult{}, fmt.Errorf("no focused castle is available")
	}
	castle = normalizePreviewLayout(castle, catalog)
	request.Policy.ResourceReserves = cloneFloatMap(request.Policy.ResourceReserves)
	result := TargetDiffResult{
		Revision: state.Revision, CatalogVersion: gameData.Metadata().ItemVersion, CastleID: castleID,
		Exact: request.Exact, Targets: []TargetMatch{}, Actions: []TargetAction{}, Requirements: []CostStatus{},
		Unmanaged: []TargetUnmanagedBuilding{}, Issues: []TargetIssue{}, Warnings: []string{},
	}
	if castle.Layout.ObservedAt.IsZero() {
		result.Warnings = append(result.Warnings, "The castle layout has not been observed in the current normalized format.")
	}

	works := normalizeTargetDiffWorks(catalog, request.Buildings, &result)
	validateTargetLayout(castle, catalog, works, &result)
	sources := targetDiffSources(state, castle, catalog, len(works))
	candidates := targetDiffCandidates(sources, works, catalog)
	assignments := minimumCostTargetAssignments(len(sources), len(works), candidates)
	assignedBuildings := map[State.BuildingInstanceID]struct{}{}

	for targetIndex := range works {
		work := &works[targetIndex]
		if !work.valid {
			continue
		}
		if sourceIndex, assigned := assignments[targetIndex]; assigned {
			source := sources[sourceIndex]
			if source.kind == TargetSourceExisting || source.kind == TargetSourceFixed {
				assignedBuildings[source.building.InstanceID] = struct{}{}
			}
			compileAssignedTarget(state, castle, catalog, request, targetIndex, work, source, &result)
			continue
		}
		compileConstructedTarget(state, castle, catalog, request, targetIndex, work, &result)
	}

	result.Unmanaged = targetDiffUnmanaged(castle, catalog, assignedBuildings, request.Policy.IgnoreDecorations)
	if request.Exact {
		for _, building := range result.Unmanaged {
			code := "extra_building"
			severity := TargetIssueError
			message := fmt.Sprintf("building %d is not represented by the exact target and requires an explicit store, move, or demolish policy", building.BuildingInstanceID)
			if strings.EqualFold(building.Definition.InternalName, "TreasureChest") {
				code = "extra_expansion_gift"
				severity = TargetIssueWaiting
				message = fmt.Sprintf("expansion gift %d must be collected before the exact target is satisfied", building.BuildingInstanceID)
			}
			addGlobalTargetIssue(&result, TargetIssue{
				Severity: severity, Code: code, Message: message, DefinitionID: building.Definition.ID,
				BuildingIDs: []State.BuildingInstanceID{building.BuildingInstanceID},
			})
		}
	}

	result.Requirements = aggregateTargetRequirements(state, castle, request.Policy.ResourceReserves, result.Actions)
	finalizeTargetDiff(&result)
	return result, nil
}

func normalizeTargetDiffWorks(
	catalog *GameData.BuildingCatalog,
	targets []TargetBuilding,
	result *TargetDiffResult,
) []targetDiffWork {
	works := make([]targetDiffWork, len(targets))
	seen := map[string]struct{}{}
	for index, target := range targets {
		target.TargetID = strings.TrimSpace(target.TargetID)
		if target.TargetID == "" {
			target.TargetID = fmt.Sprintf("building-%d", index+1)
		}
		match := TargetMatch{
			TargetID: target.TargetID, Desired: TargetDefinitionRef{ID: target.DefinitionID},
			RequestedPlacement: cloneTargetPlacement(target.Placement), UpgradePath: []TargetDefinitionRef{},
			ActionIDs: []string{}, Status: TargetStatusBlocked, Issues: []TargetIssue{},
		}
		result.Targets = append(result.Targets, match)
		works[index] = targetDiffWork{target: target}
		if _, duplicate := seen[target.TargetID]; duplicate {
			addTargetIssue(result, index, TargetIssueError, "duplicate_target_id", fmt.Sprintf("targetId %q is duplicated", target.TargetID), nil)
			continue
		}
		seen[target.TargetID] = struct{}{}
		definition, found := catalog.Definition(int64(target.DefinitionID))
		if !found || target.DefinitionID <= 0 {
			addTargetIssue(result, index, TargetIssueError, "unknown_definition", fmt.Sprintf("building definition %d is not in the official catalog", target.DefinitionID), nil)
			continue
		}
		works[index].definition = definition
		result.Targets[index].Desired = targetDefinitionRef(definition)
		if strings.EqualFold(definition.Group, "Ground") {
			addTargetIssue(result, index, TargetIssueError, "ground_definition", "ground and expansion targets must be compiled separately from building objects", nil)
			continue
		}
		if target.Placement != nil && (target.Placement.GridX < 0 || target.Placement.GridY < 0 || target.Placement.Rotation < 0 || target.Placement.Rotation > 3) {
			addTargetIssue(result, index, TargetIssueError, "invalid_placement", "target placement requires non-negative coordinates and rotation 0 through 3", nil)
			continue
		}
		works[index].valid = true
	}
	return works
}

func validateTargetLayout(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	works []targetDiffWork,
	result *TargetDiffResult,
) {
	targetCastle := castle
	targetCastle.Buildings = map[State.BuildingInstanceID]State.Building{}
	targetCastle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	targetCastle.Layout.Fixed = map[State.BuildingInstanceID]State.Building{}
	for index := range works {
		work := &works[index]
		if !work.valid || work.target.Placement == nil {
			continue
		}
		placement := Placement{
			GridX: work.target.Placement.GridX, GridY: work.target.Placement.GridY, Rotation: work.target.Placement.Rotation,
		}
		issues := ValidatePlacement(targetCastle, work.definition, placement, catalog, 0)
		for _, issue := range issues {
			code := "target_" + issue.Code
			message := issue.Message
			if issue.Code == "collision" {
				message = "The target footprint overlaps another requested target building"
			}
			addTargetIssue(result, index, TargetIssueError, code, message, nil)
			work.valid = false
		}
		instanceID := State.BuildingInstanceID(-index - 1)
		targetCastle.Layout.Objects[instanceID] = State.Building{
			InstanceID: instanceID, DefinitionID: State.BuildingID(work.definition.ID),
			GridX: placement.GridX, GridY: placement.GridY, Rotation: placement.Rotation,
			ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD, Placed: true,
		}
	}
}

func targetDiffSources(
	state State.GameState,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	targetCount int,
) []targetDiffSource {
	result := make([]targetDiffSource, 0, len(castle.Layout.Objects)+len(castle.Layout.Fixed))
	for _, id := range sortedBuildingIDs(castle.Layout.Objects) {
		building := castle.Layout.Objects[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || !building.Placed {
			continue
		}
		result = append(result, targetDiffSource{
			kind: TargetSourceExisting, key: fmt.Sprintf("building:%d", id), building: building, definition: definition,
		})
	}
	for _, id := range sortedBuildingIDs(castle.Layout.Fixed) {
		building := castle.Layout.Fixed[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || !building.Placed {
			continue
		}
		result = append(result, targetDiffSource{
			kind: TargetSourceFixed, key: fmt.Sprintf("fixed:%d", id), building: building, definition: definition,
		})
	}
	storage := state.Inventory.Items["storage:1"]
	definitionIDs := make([]int64, 0, len(storage))
	for definitionID, count := range storage {
		if definitionID > 0 && count > 0 {
			definitionIDs = append(definitionIDs, definitionID)
		}
	}
	sort.Slice(definitionIDs, func(left, right int) bool { return definitionIDs[left] < definitionIDs[right] })
	for _, definitionID := range definitionIDs {
		definition, found := catalog.Definition(definitionID)
		if !found || strings.EqualFold(definition.Group, "Ground") {
			continue
		}
		count := storage[definitionID]
		if int64(targetCount) < count {
			count = int64(targetCount)
		}
		for index := int64(0); index < count; index++ {
			result = append(result, targetDiffSource{
				kind: TargetSourceStorage, key: fmt.Sprintf("storage:%d:%d", definitionID, index+1), definition: definition,
			})
		}
	}
	return result
}

func targetDiffCandidates(
	sources []targetDiffSource,
	works []targetDiffWork,
	catalog *GameData.BuildingCatalog,
) []targetDiffCandidate {
	result := make([]targetDiffCandidate, 0)
	for sourceIndex, source := range sources {
		for targetIndex, work := range works {
			if !work.valid {
				continue
			}
			path, found := catalog.UpgradePath(source.definition.ID, work.definition.ID)
			if !found {
				continue
			}
			moveRequired := false
			if work.target.Placement != nil && (source.kind == TargetSourceExisting || source.kind == TargetSourceFixed) {
				moveRequired = targetPlacementDiffers(source.building, *work.target.Placement)
				if moveRequired && (source.kind == TargetSourceFixed || (source.definition.Movable != nil && !*source.definition.Movable)) {
					continue
				}
			}
			steps := len(path) - 1
			if source.kind == TargetSourceStorage || moveRequired {
				steps++
			}
			kindRank := int64(0)
			if source.kind == TargetSourceStorage {
				kindRank = 1
			}
			result = append(result, targetDiffCandidate{
				sourceIndex: sourceIndex, targetIndex: targetIndex,
				cost: int64(steps)*1_000_000 + kindRank*10_000 + int64(sourceIndex),
			})
		}
	}
	return result
}

func compileAssignedTarget(
	state State.GameState,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	request TargetDiffRequest,
	targetIndex int,
	work *targetDiffWork,
	source targetDiffSource,
	result *TargetDiffResult,
) {
	path, found := catalog.UpgradePath(source.definition.ID, work.definition.ID)
	if !found {
		addTargetIssue(result, targetIndex, TargetIssueError, "upgrade_path_unavailable", "the selected source no longer has an official upgrade path to the target", nil)
		return
	}
	match := &result.Targets[targetIndex]
	match.Source = &TargetSource{
		Kind: source.kind, Key: source.key, BuildingInstanceID: source.building.InstanceID,
		Definition: targetDefinitionRef(source.definition),
	}
	match.UpgradePath = targetDefinitionRefs(path)
	placement := resolvedTargetPlacement(castle, catalog, work, &source)
	if placement == nil {
		addTargetIssue(result, targetIndex, TargetIssueError, "no_space", "no collision-free placement for the final target footprint is currently available", nil)
		return
	}
	match.ResolvedPlacement = cloneTargetPlacement(placement)
	previousActionID := ""
	logicalBinding := source.kind == TargetSourceStorage
	if source.kind == TargetSourceExisting && targetPlacementDiffers(source.building, *placement) {
		action := TargetAction{
			Kind: TargetActionMove, Intent: "building.move", TargetID: match.TargetID, SubjectID: match.TargetID,
			BuildingInstanceID: source.building.InstanceID, FromDefinitionID: State.BuildingID(source.definition.ID),
			Definition: targetDefinitionRef(source.definition), FinalDefinitionID: work.target.DefinitionID,
			Placement: cloneTargetPlacement(placement), Costs: []CostStatus{}, AffordableNow: true,
		}
		previousActionID = appendTargetAction(result, targetIndex, action, previousActionID)
	}
	if source.kind == TargetSourceStorage {
		action := TargetAction{
			Kind: TargetActionPlace, Intent: "building.place", TargetID: match.TargetID, SubjectID: match.TargetID,
			Definition: targetDefinitionRef(source.definition), FinalDefinitionID: work.target.DefinitionID,
			Placement: cloneTargetPlacement(placement), Costs: []CostStatus{}, AffordableNow: true,
		}
		previousActionID = appendTargetAction(result, targetIndex, action, previousActionID)
	}
	for index := 1; index < len(path); index++ {
		previous := path[index-1]
		next := path[index]
		action := targetUpgradeAction(state, castle, request, *match, source.building.InstanceID, previous, next, logicalBinding)
		previousActionID = appendTargetAction(result, targetIndex, action, previousActionID)
		addTargetActionIssues(state, castle, request, targetIndex, next, action, result)
	}
	if len(match.ActionIDs) > 0 && (source.kind == TargetSourceExisting || source.kind == TargetSourceFixed) && targetBuildingQueued(castle.BuildingQueue, source.building.InstanceID) {
		addTargetIssue(result, targetIndex, TargetIssueWaiting, "source_busy", fmt.Sprintf("building %d is currently in the construction queue", source.building.InstanceID), []State.BuildingInstanceID{source.building.InstanceID})
	}
	addCurrentTargetPlacementIssues(castle, catalog, targetIndex, work.definition, placement, source.building.InstanceID, result)
}

func compileConstructedTarget(
	state State.GameState,
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	request TargetDiffRequest,
	targetIndex int,
	work *targetDiffWork,
	result *TargetDiffResult,
) {
	path, found := catalog.ConstructionPath(work.definition.ID)
	if !found || len(path) == 0 {
		addTargetIssue(result, targetIndex, TargetIssueError, "construction_path_unavailable", "the official catalog does not provide a consistent construction-root upgrade path to the target", nil)
		return
	}
	root := path[0]
	match := &result.Targets[targetIndex]
	match.Source = &TargetSource{
		Kind: TargetSourceConstruct, Key: fmt.Sprintf("construct:%d", root.ID), Definition: targetDefinitionRef(root),
	}
	match.UpgradePath = targetDefinitionRefs(path)
	if !strings.EqualFold(root.Group, "Building") || root.DowngradeDefinitionID != 0 {
		addTargetIssue(result, targetIndex, TargetIssueError, "invalid_construction_root", fmt.Sprintf("definition %d is not a normal construction root", root.ID), nil)
	}
	if root.ShopCategory == "" {
		addTargetIssue(result, targetIndex, TargetIssueError, "construction_availability_unknown", fmt.Sprintf("definition %d is not statically available from a building shop category", root.ID), nil)
	}
	if root.ForcedPosition != "" {
		addTargetIssue(result, targetIndex, TargetIssueError, "forced_position", fmt.Sprintf("definition %d requires a server-selected position", root.ID), nil)
	}
	if len(root.Costs) == 0 {
		addTargetIssue(result, targetIndex, TargetIssueError, "construction_cost_unknown", fmt.Sprintf("definition %d has no official construction cost", root.ID), nil)
	}
	placement := resolvedTargetPlacement(castle, catalog, work, nil)
	if placement == nil {
		addTargetIssue(result, targetIndex, TargetIssueError, "no_space", "no collision-free placement for the final target footprint is currently available", nil)
		return
	}
	match.ResolvedPlacement = cloneTargetPlacement(placement)
	costs, affordable := buildingCostStatus(state, castle, root.Costs, request.Policy.ResourceReserves)
	construct := TargetAction{
		Kind: ActionConstruct, Intent: "building.construct", TargetID: match.TargetID, SubjectID: match.TargetID,
		Definition: targetDefinitionRef(root), FinalDefinitionID: work.target.DefinitionID,
		Placement: cloneTargetPlacement(placement), DurationSec: root.DurationSec, Costs: costs, AffordableNow: affordable,
	}
	previousActionID := appendTargetAction(result, targetIndex, construct, "")
	addTargetActionIssues(state, castle, request, targetIndex, root, construct, result)
	for index := 1; index < len(path); index++ {
		previous := path[index-1]
		next := path[index]
		action := targetUpgradeAction(state, castle, request, *match, 0, previous, next, true)
		previousActionID = appendTargetAction(result, targetIndex, action, previousActionID)
		addTargetActionIssues(state, castle, request, targetIndex, next, action, result)
	}
	addCurrentTargetPlacementIssues(castle, catalog, targetIndex, work.definition, placement, 0, result)
}

func targetUpgradeAction(
	state State.GameState,
	castle State.CastleState,
	request TargetDiffRequest,
	match TargetMatch,
	buildingID State.BuildingInstanceID,
	previous GameData.BuildingDefinition,
	next GameData.BuildingDefinition,
	requiresBinding bool,
) TargetAction {
	costs, affordable := buildingCostStatus(state, castle, next.Costs, request.Policy.ResourceReserves)
	return TargetAction{
		Kind: ActionUpgrade, Intent: "building.upgrade", TargetID: match.TargetID, SubjectID: match.TargetID,
		BuildingInstanceID: buildingID, FromDefinitionID: State.BuildingID(previous.ID),
		Definition: targetDefinitionRef(next), FinalDefinitionID: match.Desired.ID,
		DurationSec: next.DurationSec, Costs: costs, AffordableNow: affordable, RequiresBinding: requiresBinding,
	}
}

func addTargetActionIssues(
	state State.GameState,
	castle State.CastleState,
	request TargetDiffRequest,
	targetIndex int,
	definition GameData.BuildingDefinition,
	action TargetAction,
	result *TargetDiffResult,
) {
	for _, blocker := range definitionBlockers(state.Player, castle, definition, request.EventID, request.MapID) {
		severity := TargetIssueError
		if blocker.Code == "player_level" || blocker.Code == "legend_level" {
			severity = TargetIssueWaiting
		}
		addTargetIssue(result, targetIndex, severity, blocker.Code, blocker.Message, nil)
	}
	if !action.AffordableNow {
		addTargetIssue(result, targetIndex, TargetIssueWaiting, "resources_pending", "observed balances do not yet cover every compiled action cost and configured reserve", nil)
	}
	for _, cost := range action.Costs {
		if cost.Premium && !request.Policy.AllowPremium {
			addTargetIssue(result, targetIndex, TargetIssueError, "premium_disallowed", "the compiled path includes premium cost while premium spending is disabled", nil)
			break
		}
	}
	if (action.Kind == ActionConstruct || action.Kind == ActionUpgrade) && buildingQueueObserved(castle.BuildingQueue) && !buildingQueueAvailable(castle.BuildingQueue) {
		addTargetIssue(result, targetIndex, TargetIssueWaiting, "construction_queue_full", "the building queue must become available before this path can advance", nil)
	}
}

func addCurrentTargetPlacementIssues(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	targetIndex int,
	definition GameData.BuildingDefinition,
	placement *TargetPlacement,
	ignoreBuildingID State.BuildingInstanceID,
	result *TargetDiffResult,
) {
	if placement == nil {
		return
	}
	issues := ValidatePlacement(castle, definition, Placement{
		GridX: placement.GridX, GridY: placement.GridY, Rotation: placement.Rotation,
	}, catalog, ignoreBuildingID)
	for _, issue := range issues {
		if issue.Code == "outside_ground" {
			addTargetIssue(result, targetIndex, TargetIssueError, "target_outside_ground", issue.Message, issue.BuildingIDs)
			continue
		}
		if issue.Code != "collision" {
			continue
		}
		hasGift := false
		for _, buildingID := range issue.BuildingIDs {
			building, found := castle.Layout.Objects[buildingID]
			if !found {
				continue
			}
			obstacle, definitionFound := catalog.Definition(int64(building.DefinitionID))
			if definitionFound && strings.EqualFold(obstacle.InternalName, "TreasureChest") {
				hasGift = true
				break
			}
		}
		if hasGift {
			addTargetIssue(result, targetIndex, TargetIssueWaiting, "expansion_gift_blocker", "one or more expansion gifts must be collected before this target can be placed", issue.BuildingIDs)
		} else {
			addTargetIssue(result, targetIndex, TargetIssueWaiting, "placement_reconciliation", "current castle objects must be moved, stored, or removed before this target footprint is available", issue.BuildingIDs)
		}
	}
}

func resolvedTargetPlacement(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	work *targetDiffWork,
	source *targetDiffSource,
) *TargetPlacement {
	if work.target.Placement != nil {
		return cloneTargetPlacement(work.target.Placement)
	}
	if source != nil && (source.kind == TargetSourceExisting || source.kind == TargetSourceFixed) {
		return &TargetPlacement{
			GridX: source.building.GridX, GridY: source.building.GridY, Rotation: source.building.Rotation,
		}
	}
	placement, found := FindPlacement(castle, work.definition, catalog, 0)
	if !found {
		return nil
	}
	return &TargetPlacement{GridX: placement.GridX, GridY: placement.GridY, Rotation: placement.Rotation}
}

func appendTargetAction(result *TargetDiffResult, targetIndex int, action TargetAction, previousActionID string) string {
	action.Sequence = len(result.Actions) + 1
	action.ID = fmt.Sprintf("action-%d", action.Sequence)
	if previousActionID != "" {
		action.DependsOn = []string{previousActionID}
	}
	if action.Costs == nil {
		action.Costs = []CostStatus{}
	}
	result.Actions = append(result.Actions, action)
	result.Targets[targetIndex].ActionIDs = append(result.Targets[targetIndex].ActionIDs, action.ID)
	return action.ID
}

func targetDiffUnmanaged(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	assigned map[State.BuildingInstanceID]struct{},
	ignoreDecorations bool,
) []TargetUnmanagedBuilding {
	result := make([]TargetUnmanagedBuilding, 0)
	for _, id := range sortedBuildingIDs(castle.Layout.Objects) {
		if _, used := assigned[id]; used {
			continue
		}
		building := castle.Layout.Objects[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found {
			continue
		}
		if ignoreDecorations && strings.EqualFold(definition.GroundType, "DECO") {
			continue
		}
		item := TargetUnmanagedBuilding{BuildingInstanceID: id, Definition: targetDefinitionRef(definition)}
		if building.Placed {
			item.Placement = &TargetPlacement{GridX: building.GridX, GridY: building.GridY, Rotation: building.Rotation}
		}
		result = append(result, item)
	}
	return result
}

func aggregateTargetRequirements(
	state State.GameState,
	castle State.CastleState,
	reserves map[string]float64,
	actions []TargetAction,
) []CostStatus {
	type aggregate struct {
		cost GameData.BuildingCost
	}
	byKey := map[string]aggregate{}
	for _, action := range actions {
		for _, cost := range action.Costs {
			key := fmt.Sprintf("%s:%d:%s:%t", cost.Scope, cost.DefinitionID, strings.ToLower(cost.Key), cost.Premium)
			current := byKey[key]
			if current.cost.Key == "" {
				current.cost = GameData.BuildingCost{
					Field: cost.Field, Key: cost.Key, Scope: cost.Scope, DefinitionID: cost.DefinitionID, Premium: cost.Premium,
				}
			}
			current.cost.Amount += cost.Required
			byKey[key] = current
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	costs := make([]GameData.BuildingCost, 0, len(keys))
	for _, key := range keys {
		costs = append(costs, byKey[key].cost)
	}
	statuses, _ := buildingCostStatus(state, castle, costs, reserves)
	return statuses
}

func finalizeTargetDiff(result *TargetDiffResult) {
	result.Compilable = true
	for _, issue := range result.Issues {
		if issue.Severity == TargetIssueError {
			result.Compilable = false
			break
		}
	}
	for index := range result.Targets {
		match := &result.Targets[index]
		hasError, hasWaiting := false, false
		for _, issue := range match.Issues {
			if issue.Severity == TargetIssueError {
				hasError = true
			}
			if issue.Severity == TargetIssueWaiting {
				hasWaiting = true
			}
		}
		switch {
		case hasError:
			match.Status = TargetStatusBlocked
			result.Summary.BlockedCount++
		case len(match.ActionIDs) == 0 && !hasWaiting:
			match.Status = TargetStatusSatisfied
			result.Summary.SatisfiedCount++
		case hasWaiting:
			match.Status = TargetStatusWaiting
			result.Summary.WaitingCount++
		default:
			match.Status = TargetStatusPlanned
			result.Summary.PlannedCount++
		}
	}
	result.Summary.TargetCount = len(result.Targets)
	result.Summary.ActionCount = len(result.Actions)
	result.Summary.UnmanagedCount = len(result.Unmanaged)
	result.Satisfied = result.Compilable && len(result.Actions) == 0 && result.Summary.SatisfiedCount == result.Summary.TargetCount
	if result.Exact && len(result.Unmanaged) > 0 {
		result.Satisfied = false
	}
	for _, action := range result.Actions {
		if action.RequiresBinding {
			result.Warnings = append(result.Warnings, "Actions after construct or place use a logical subject and must bind the returned building instance ID before execution.")
			break
		}
	}
}

func addTargetIssue(
	result *TargetDiffResult,
	targetIndex int,
	severity string,
	code string,
	message string,
	buildingIDs []State.BuildingInstanceID,
) {
	issue := TargetIssue{Severity: severity, Code: code, Message: message, BuildingIDs: append([]State.BuildingInstanceID(nil), buildingIDs...)}
	if targetIndex >= 0 && targetIndex < len(result.Targets) {
		issue.TargetID = result.Targets[targetIndex].TargetID
		issue.DefinitionID = result.Targets[targetIndex].Desired.ID
		for _, existing := range result.Targets[targetIndex].Issues {
			if existing.Severity == issue.Severity && existing.Code == issue.Code && existing.Message == issue.Message {
				return
			}
		}
		result.Targets[targetIndex].Issues = append(result.Targets[targetIndex].Issues, issue)
	}
	addGlobalTargetIssue(result, issue)
}

func addGlobalTargetIssue(result *TargetDiffResult, issue TargetIssue) {
	sort.Slice(issue.BuildingIDs, func(left, right int) bool { return issue.BuildingIDs[left] < issue.BuildingIDs[right] })
	for _, existing := range result.Issues {
		if existing.TargetID == issue.TargetID && existing.Severity == issue.Severity && existing.Code == issue.Code && existing.Message == issue.Message {
			return
		}
	}
	result.Issues = append(result.Issues, issue)
}

func targetDefinitionRef(definition GameData.BuildingDefinition) TargetDefinitionRef {
	return TargetDefinitionRef{
		ID: State.BuildingID(definition.ID), InternalName: definition.InternalName,
		DisplayName: definition.DisplayName, Level: definition.Level,
	}
}

func targetDefinitionRefs(definitions []GameData.BuildingDefinition) []TargetDefinitionRef {
	result := make([]TargetDefinitionRef, len(definitions))
	for index, definition := range definitions {
		result[index] = targetDefinitionRef(definition)
	}
	return result
}

func cloneTargetPlacement(source *TargetPlacement) *TargetPlacement {
	if source == nil {
		return nil
	}
	copy := *source
	return &copy
}

func targetPlacementDiffers(building State.Building, placement TargetPlacement) bool {
	return building.GridX != placement.GridX || building.GridY != placement.GridY || building.Rotation != placement.Rotation
}

func targetBuildingQueued(queue State.BuildingConstructionQueue, buildingID State.BuildingInstanceID) bool {
	for _, slot := range queue.Slots {
		if slot.Status == State.BuildingQueueSlotOccupied && slot.BuildingID == buildingID {
			return true
		}
	}
	return false
}

type targetFlowEdge struct {
	to       int
	reverse  int
	capacity int
	cost     int64
}

type targetFlowReference struct {
	from        int
	edgeIndex   int
	sourceIndex int
	targetIndex int
}

func minimumCostTargetAssignments(
	sourceCount int,
	targetCount int,
	candidates []targetDiffCandidate,
) map[int]int {
	assignments := map[int]int{}
	if sourceCount == 0 || targetCount == 0 || len(candidates) == 0 {
		return assignments
	}
	root := 0
	firstSource := 1
	firstTarget := firstSource + sourceCount
	sink := firstTarget + targetCount
	graph := make([][]targetFlowEdge, sink+1)
	addEdge := func(from int, to int, capacity int, cost int64) int {
		forwardIndex := len(graph[from])
		reverseIndex := len(graph[to])
		graph[from] = append(graph[from], targetFlowEdge{to: to, reverse: reverseIndex, capacity: capacity, cost: cost})
		graph[to] = append(graph[to], targetFlowEdge{to: from, reverse: forwardIndex, capacity: 0, cost: -cost})
		return forwardIndex
	}
	for sourceIndex := 0; sourceIndex < sourceCount; sourceIndex++ {
		addEdge(root, firstSource+sourceIndex, 1, 0)
	}
	for targetIndex := 0; targetIndex < targetCount; targetIndex++ {
		addEdge(firstTarget+targetIndex, sink, 1, 0)
	}
	references := make([]targetFlowReference, 0, len(candidates))
	for _, candidate := range candidates {
		from := firstSource + candidate.sourceIndex
		edgeIndex := addEdge(from, firstTarget+candidate.targetIndex, 1, candidate.cost)
		references = append(references, targetFlowReference{
			from: from, edgeIndex: edgeIndex, sourceIndex: candidate.sourceIndex, targetIndex: candidate.targetIndex,
		})
	}

	const infinity = int64(^uint64(0)>>1) / 4
	for {
		distance := make([]int64, len(graph))
		previousNode := make([]int, len(graph))
		previousEdge := make([]int, len(graph))
		for index := range distance {
			distance[index] = infinity
			previousNode[index] = -1
			previousEdge[index] = -1
		}
		distance[root] = 0
		for iteration := 0; iteration < len(graph)-1; iteration++ {
			changed := false
			for node := range graph {
				if distance[node] == infinity {
					continue
				}
				for edgeIndex, edge := range graph[node] {
					if edge.capacity <= 0 || distance[node]+edge.cost >= distance[edge.to] {
						continue
					}
					distance[edge.to] = distance[node] + edge.cost
					previousNode[edge.to] = node
					previousEdge[edge.to] = edgeIndex
					changed = true
				}
			}
			if !changed {
				break
			}
		}
		if previousNode[sink] < 0 {
			break
		}
		for node := sink; node != root; {
			from := previousNode[node]
			edgeIndex := previousEdge[node]
			reverse := graph[from][edgeIndex].reverse
			graph[from][edgeIndex].capacity--
			graph[node][reverse].capacity++
			node = from
		}
	}
	for _, reference := range references {
		if graph[reference.from][reference.edgeIndex].capacity == 0 {
			assignments[reference.targetIndex] = reference.sourceIndex
		}
	}
	return assignments
}

package Buildings

import (
	"fmt"
	"sort"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type FixedTargetDiffRequest struct {
	CastleID State.CastleID        `json:"castleId,omitempty"`
	Policy   TargetDiffPolicy      `json:"policy"`
	Fixed    []TargetFixedBuilding `json:"fixed"`
}

func CompileFixedTargetDiff(
	state State.GameState,
	gameData *GameData.Store,
	request FixedTargetDiffRequest,
) (TargetDiffResult, error) {
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
		Targets: []TargetMatch{}, Actions: []TargetAction{}, Requirements: []CostStatus{},
		Unmanaged: []TargetUnmanagedBuilding{}, Issues: []TargetIssue{}, Warnings: []string{},
	}
	used := map[State.BuildingInstanceID]struct{}{}
	fixedIDs := sortedBuildingIDs(castle.Layout.Fixed)
	seenTargets := map[string]struct{}{}
	genericRequest := TargetDiffRequest{
		CastleID: castleID,
		Policy:   request.Policy,
	}

	for index, target := range request.Fixed {
		target.TargetID = strings.TrimSpace(target.TargetID)
		if target.TargetID == "" {
			target.TargetID = fmt.Sprintf("fixed-%d", index+1)
		}
		result.Targets = append(result.Targets, TargetMatch{
			TargetID: target.TargetID, Desired: TargetDefinitionRef{ID: target.DefinitionID},
			RequestedPlacement: cloneTargetPlacement(target.Slot), UpgradePath: []TargetDefinitionRef{},
			ActionIDs: []string{}, Status: TargetStatusBlocked, Issues: []TargetIssue{},
		})
		if _, duplicate := seenTargets[target.TargetID]; duplicate {
			addTargetIssue(&result, index, TargetIssueError, "duplicate_target_id", fmt.Sprintf("targetId %q is duplicated", target.TargetID), nil)
			continue
		}
		seenTargets[target.TargetID] = struct{}{}
		desired, found := catalog.Definition(int64(target.DefinitionID))
		if !found || target.DefinitionID <= 0 {
			addTargetIssue(&result, index, TargetIssueError, "unknown_definition", fmt.Sprintf("fixed definition %d is not in the official catalog", target.DefinitionID), nil)
			continue
		}
		result.Targets[index].Desired = targetDefinitionRef(desired)
		if !fixedTargetDefinition(desired) {
			addTargetIssue(&result, index, TargetIssueError, "not_fixed_definition", fmt.Sprintf("definition %d is not a fixed castle object", desired.ID), nil)
			continue
		}

		source, sourceDefinition, sourcePath, sourceHigher, matched := matchFixedTarget(
			castle, catalog, fixedIDs, used, target, desired,
		)
		if !matched {
			message := fmt.Sprintf("No observed fixed %s slot can be upgraded to level %d", desired.DisplayName, desired.Level)
			code := "fixed_source_missing"
			if strings.EqualFold(desired.InternalName, "Harbor") {
				code = "harbor_root_missing"
				message = "Harbor level 1 is not observed; initial forced-position Harbor construction has no verified command"
			}
			addTargetIssue(&result, index, TargetIssueError, code, message, nil)
			continue
		}
		used[source.InstanceID] = struct{}{}
		match := &result.Targets[index]
		match.Source = &TargetSource{
			Kind: TargetSourceFixed, Key: fmt.Sprintf("fixed:%d", source.InstanceID),
			BuildingInstanceID: source.InstanceID, Definition: targetDefinitionRef(sourceDefinition),
		}
		match.ResolvedPlacement = &TargetPlacement{
			GridX: source.GridX, GridY: source.GridY, Rotation: source.Rotation,
		}
		if sourceHigher {
			match.UpgradePath = []TargetDefinitionRef{targetDefinitionRef(sourceDefinition)}
			continue
		}
		match.UpgradePath = targetDefinitionRefs(sourcePath)
		previousActionID := ""
		for pathIndex := 1; pathIndex < len(sourcePath); pathIndex++ {
			previous := sourcePath[pathIndex-1]
			next := sourcePath[pathIndex]
			action := targetUpgradeAction(
				state, castle, genericRequest, *match, source.InstanceID, previous, next, false,
			)
			previousActionID = appendTargetAction(&result, index, action, previousActionID)
			addTargetActionIssues(state, castle, genericRequest, index, next, action, &result)
		}
		if len(match.ActionIDs) > 0 && targetBuildingQueued(castle.BuildingQueue, source.InstanceID) {
			addTargetIssue(
				&result, index, TargetIssueWaiting, "source_busy",
				fmt.Sprintf("fixed building %d is currently in the construction queue", source.InstanceID),
				[]State.BuildingInstanceID{source.InstanceID},
			)
		}
	}

	result.Requirements = aggregateTargetRequirements(state, castle, request.Policy.ResourceReserves, result.Actions)
	finalizeTargetDiff(&result)
	return result, nil
}

func matchFixedTarget(
	castle State.CastleState,
	catalog *GameData.BuildingCatalog,
	fixedIDs []State.BuildingInstanceID,
	used map[State.BuildingInstanceID]struct{},
	target TargetFixedBuilding,
	desired GameData.BuildingDefinition,
) (State.Building, GameData.BuildingDefinition, []GameData.BuildingDefinition, bool, bool) {
	type candidate struct {
		building State.Building
		current  GameData.BuildingDefinition
		path     []GameData.BuildingDefinition
		higher   bool
		score    int
	}
	candidates := make([]candidate, 0)
	for _, buildingID := range fixedIDs {
		if _, alreadyUsed := used[buildingID]; alreadyUsed {
			continue
		}
		building := castle.Layout.Fixed[buildingID]
		current, found := catalog.Definition(int64(building.DefinitionID))
		if !found {
			continue
		}
		path, canUpgrade := catalog.UpgradePath(current.ID, desired.ID)
		_, alreadyHigher := catalog.UpgradePath(desired.ID, current.ID)
		if !canUpgrade && !alreadyHigher {
			continue
		}
		score := 1000
		if target.Slot != nil &&
			target.Slot.GridX == building.GridX &&
			target.Slot.GridY == building.GridY &&
			target.Slot.Rotation == building.Rotation {
			score = 0
		} else if strings.EqualFold(current.InternalName, desired.InternalName) &&
			strings.EqualFold(current.Group, desired.Group) {
			score = 100
		}
		score += absInt64(current.Level - desired.Level)
		candidates = append(candidates, candidate{
			building: building, current: current, path: path, higher: alreadyHigher && !canUpgrade, score: score,
		})
	}
	if len(candidates) == 0 {
		return State.Building{}, GameData.BuildingDefinition{}, nil, false, false
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].score != candidates[right].score {
			return candidates[left].score < candidates[right].score
		}
		return candidates[left].building.InstanceID < candidates[right].building.InstanceID
	})
	selected := candidates[0]
	return selected.building, selected.current, selected.path, selected.higher, true
}

func fixedTargetDefinition(definition GameData.BuildingDefinition) bool {
	if definition.ForcedPosition != "" {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(definition.Group)) {
	case "tower", "gate", "defence", "moat", "fixedpositionbuilding":
		return true
	default:
		return false
	}
}

func absInt64(value int64) int {
	if value < 0 {
		return int(-value)
	}
	return int(value)
}

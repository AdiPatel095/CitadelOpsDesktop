package Automation

import (
	"fmt"
	"strings"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	beriBuildPhaseStable = iota + 1
	beriBuildPhaseGround
	beriBuildPhaseCleanup
	beriBuildPhaseMove
	beriBuildPhaseDecorations
	beriBuildPhaseFinal
)

// evaluateBeriEventBuild deliberately does not share Storm's opportunistic
// fall-through planner. Each phase is recomputed from the latest authoritative
// castle state and either emits one action, waits in that phase, or advances.
func evaluateBeriEventBuild(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
	profile autoEventBuildProfile,
) (*Decision, bool, string, error) {
	if settings.Target == nil {
		return nil, true, "", nil
	}
	if settings.Target.KingdomID != profile.KingdomID {
		return nil, false, "Captured target is not a Berimond castle state", nil
	}
	layoutStale := castle.Layout.ObservedAt.IsZero() || snapshot.Now.Sub(castle.Layout.ObservedAt) > autoStormBuildingRefreshAge
	queueStale := castle.BuildingQueue.ObservedAt.IsZero() || snapshot.Now.Sub(castle.BuildingQueue.ObservedAt) > autoStormBuildingRefreshAge
	if layoutStale || queueStale {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh the Berimond castle building state", "building.refresh", map[string]any{
			"castleId": castle.ID,
		}), false, "", nil
	}
	catalog, err := snapshot.GameData.BuildingCatalog()
	if err != nil {
		return nil, false, "", err
	}
	if giftID, found := autoStormExpansionGift(castle, catalog); found {
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Collect expansion gift %d before reconciling the Berimond layout", giftID), "building.collect_expansion_gift", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": giftID,
		}), false, "", nil
	}
	queueDecision, queueBlocked := autoStormQueueDecision(snapshot, settings, castle, catalog, metrics, profile)
	if queueDecision != nil {
		return queueDecision, false, "", nil
	}

	normalized := Buildings.NormalizeTargetCapture(*settings.Target, catalog)
	stableTargets, decorationTargets, finalTargets := beriPartitionTargets(normalized.Buildings, catalog)
	allTargets := append(append(append([]Buildings.TargetBuilding{}, stableTargets...), decorationTargets...), finalTargets...)

	metrics["beriBuildPhase"] = beriBuildPhaseStable
	stableDiff, err := beriCompileTargetDiff(snapshot, settings, castle, beriTargetsWithoutPlacement(stableTargets), false)
	if err != nil {
		return nil, false, "", err
	}
	stableRubyBlocked := rubyPolicyOnlyBlocked(stableDiff)
	if !stableDiff.Satisfied && !stableRubyBlocked {
		if queueBlocked {
			return nil, false, "The Berimond construction queue is occupied while the selected Stable level is pending", nil
		}
		return beriPhaseAction(snapshot, settings, castle, stableDiff, metrics, profile,
			"Waiting for returned Berimond attack loot to finish the selected Stable level")
	}

	metrics["beriBuildPhase"] = beriBuildPhaseGround
	missingGround := autoStormMissingGround(castle, normalized.Ground)
	if metrics["builtInTarget"] == 1 {
		missingGround = Buildings.MissingGroundCoverage(castle, normalized.Ground, catalog)
	}
	metrics["targetGroundRemaining"] = float64(len(missingGround))
	if len(missingGround) > 0 {
		if queueBlocked {
			return nil, false, "The Berimond construction queue is occupied while target expansions are pending", nil
		}
		if metrics["builtInTarget"] == 1 && beriTargetStorageCountReached(castle, normalized, catalog) {
			preview, previewErr := Buildings.PreviewExpansion(snapshot.State, snapshot.GameData, Buildings.ExpansionPreviewRequest{
				CastleID: castle.ID, Payment: Buildings.ExpansionPaymentResources,
				ResourceReserves: settings.Build.ResourceReserves,
			})
			if previewErr != nil {
				return nil, false, "", previewErr
			}
			for _, cost := range preview.Costs {
				if cost.CapacityKnown && !cost.CapacitySufficient {
					return nil, false, "The built-in Berimond target already has all seven stores; expansion cost plus configured reserves exceeds storage capacity", nil
				}
			}
		}
		decision, detail, expansionErr := autoStormExpansionDecision(snapshot, settings, castle, missingGround, metrics, profile)
		if expansionErr != nil {
			return nil, false, "", expansionErr
		}
		if decision != nil {
			return decision, false, "", nil
		}
		if detail == "" {
			detail = "Waiting for returned Berimond attack loot to fund the next required target expansion"
		}
		return nil, false, detail, nil
	}

	metrics["beriBuildPhase"] = beriBuildPhaseCleanup
	validatedTargetDiff, err := beriCompileTargetDiff(snapshot, settings, castle, allTargets, false)
	if err != nil {
		return nil, false, "", err
	}
	if detail, unsafe := beriUnsafeTargetDiff(validatedTargetDiff); unsafe {
		return nil, false, detail, nil
	}
	fixedTargets, err := autoStormFixedTargets(settings, catalog)
	if err != nil {
		return nil, false, err.Error(), nil
	}
	fixedDiff, err := Buildings.CompileFixedTargetDiff(snapshot.State, snapshot.GameData, Buildings.FixedTargetDiffRequest{
		CastleID: castle.ID, EventID: optionalAutoEventBuildID(profile.EventID),
		Policy: Buildings.TargetDiffPolicy{
			AllowPremium: settings.Build.AllowPremium, ResourceReserves: settings.Build.ResourceReserves,
		},
		Fixed: fixedTargets,
	})
	if err != nil {
		return nil, false, "", err
	}
	if detail, unsafe := beriUnsafeTargetDiff(fixedDiff); unsafe {
		return nil, false, detail, nil
	}
	cleanupDiff, err := beriCompileTargetDiff(snapshot, settings, castle, beriTargetsWithoutPlacement(allTargets), false)
	if err != nil {
		return nil, false, "", err
	}
	metrics["unmanagedBuildings"] = float64(cleanupDiff.Summary.UnmanagedCount)
	if detail, unsafe := beriUnsafeTargetDiff(cleanupDiff); unsafe {
		return nil, false, detail, nil
	}
	if extra, definition, found := beriCleanupCandidate(castle, cleanupDiff, catalog); normalized.Exact && found {
		if queueBlocked {
			return nil, false, fmt.Sprintf("The Berimond construction queue is occupied while unmanaged %s is pending removal", definition.DisplayName), nil
		}
		if autoStormDecorationDefinition(definition) && definition.Storeable != nil && *definition.Storeable {
			return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Store unmanaged %s before arranging the target layout", definition.DisplayName), "building.store", map[string]any{
				"castleId": castle.ID, "buildingInstanceId": extra.BuildingInstanceID,
			}), false, "", nil
		}
		if !settings.Build.AllowDemolition {
			return nil, false, fmt.Sprintf("Unmanaged %s must be removed before layout moves; enable demolition to allow this action", definition.DisplayName), nil
		}
		if !autoStormBuildingOfficiallyDestructible(definition) {
			return nil, false, fmt.Sprintf("Unmanaged %s is officially protected and cannot be demolished", definition.DisplayName), nil
		}
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Demolish unmanaged %s before arranging the target layout", definition.DisplayName), "building.demolish", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": extra.BuildingInstanceID,
		}), false, "", nil
	}

	metrics["beriBuildPhase"] = beriBuildPhaseMove
	moveDiff := validatedTargetDiff
	if move, found := beriNextExecutableMove(moveDiff); found {
		return autoStormTargetActionDecision(snapshot.Now, settings, castle, move, metrics, profile), false, "", nil
	}
	if detail, blocked := beriRetainedPositionBlocker(castle, moveDiff); blocked {
		return nil, false, detail, nil
	}

	metrics["beriBuildPhase"] = beriBuildPhaseDecorations
	decorationDiff, err := beriCompileTargetDiff(snapshot, settings, castle, decorationTargets, false)
	if err != nil {
		return nil, false, "", err
	}
	if !decorationDiff.Satisfied {
		if queueBlocked {
			return nil, false, "The Berimond construction queue is occupied while target decorations are pending", nil
		}
		return beriPhaseAction(snapshot, settings, castle, decorationDiff, metrics, profile,
			"Waiting for resources or free target space to place every target decoration")
	}

	metrics["beriBuildPhase"] = beriBuildPhaseFinal
	finalDiff, err := beriCompileTargetDiff(snapshot, settings, castle, finalTargets, false)
	if err != nil {
		return nil, false, "", err
	}
	metrics["targetBuildingsSatisfied"] = float64(stableDiff.Summary.SatisfiedCount + decorationDiff.Summary.SatisfiedCount + finalDiff.Summary.SatisfiedCount + fixedDiff.Summary.SatisfiedCount)
	metrics["targetBuildingsTotal"] = float64(stableDiff.Summary.TargetCount + decorationDiff.Summary.TargetCount + finalDiff.Summary.TargetCount + fixedDiff.Summary.TargetCount)
	metrics["targetActionsRemaining"] = float64(finalDiff.Summary.ActionCount + fixedDiff.Summary.ActionCount)
	if finalDiff.Satisfied && fixedDiff.Satisfied {
		if stableRubyBlocked {
			return nil, false, rubyPolicyDetail(stableDiff), nil
		}
		return nil, true, "Captured Berimond target state satisfied", nil
	}
	if queueBlocked {
		return nil, false, "The Berimond construction queue is occupied while final camp construction or upgrades are pending", nil
	}
	if !finalDiff.Satisfied {
		return beriPhaseAction(snapshot, settings, castle, finalDiff, metrics, profile,
			"Waiting for returned Berimond attack loot to finish target camp construction and upgrades")
	}
	return beriPhaseAction(snapshot, settings, castle, fixedDiff, metrics, profile,
		"Waiting for returned Berimond attack loot to finish fixed target upgrades")
}

func beriPartitionTargets(
	targets []Buildings.TargetBuilding,
	catalog *GameData.BuildingCatalog,
) (stable []Buildings.TargetBuilding, decorations []Buildings.TargetBuilding, final []Buildings.TargetBuilding) {
	for _, target := range targets {
		definition, found := catalog.DefinitionView(int64(target.DefinitionID))
		if !found {
			final = append(final, target)
			continue
		}
		switch {
		case isBeriStableDefinition(definition):
			stable = append(stable, target)
		case autoStormDecorationDefinition(definition):
			decorations = append(decorations, target)
		default:
			final = append(final, target)
		}
	}
	return stable, decorations, final
}

func beriTargetsWithoutPlacement(targets []Buildings.TargetBuilding) []Buildings.TargetBuilding {
	result := make([]Buildings.TargetBuilding, len(targets))
	copy(result, targets)
	for index := range result {
		result[index].Placement = nil
	}
	return result
}

func beriCompileTargetDiff(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	targets []Buildings.TargetBuilding,
	ignoreDecorations bool,
) (Buildings.TargetDiffResult, error) {
	return Buildings.CompileTargetDiff(snapshot.State, snapshot.GameData, Buildings.TargetDiffRequest{
		CastleID: castle.ID, EventID: optionalAutoEventBuildID(GameData.BerimondEventID), Exact: false,
		Policy: Buildings.TargetDiffPolicy{
			AllowPremium: settings.Build.AllowPremium, IgnoreDecorations: ignoreDecorations,
			ResourceReserves: settings.Build.ResourceReserves,
		},
		Buildings: targets,
	})
}

func beriCleanupCandidate(
	castle State.CastleState,
	diff Buildings.TargetDiffResult,
	catalog *GameData.BuildingCatalog,
) (Buildings.TargetUnmanagedBuilding, GameData.BuildingDefinition, bool) {
	for _, extra := range diff.Unmanaged {
		building, exists := castle.Layout.Objects[extra.BuildingInstanceID]
		if !exists || !building.Placed || autoStormBuildingQueued(castle, building.InstanceID) {
			continue
		}
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if !found || isBeriStableDefinition(definition) ||
			fixedAutoStormDefinition(definition) || !autoStormBuildingOfficiallyDestructible(definition) {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(definition.Group), "Building") {
			continue
		}
		return extra, definition, true
	}
	return Buildings.TargetUnmanagedBuilding{}, GameData.BuildingDefinition{}, false
}

func beriUnsafeTargetDiff(diff Buildings.TargetDiffResult) (string, bool) {
	for _, issue := range diff.Issues {
		if issue.Severity != Buildings.TargetIssueError {
			continue
		}
		switch issue.Code {
		case "premium_disallowed", "ruby_confirmation_unknown", "ruby_confirmation_required", "no_space", "kingdom", "area_type", "event_context", "event", "map_context", "map":
			continue
		default:
			return issue.Message, true
		}
	}
	return "", false
}

func beriNextExecutableMove(diff Buildings.TargetDiffResult) (Buildings.TargetAction, bool) {
	for _, action := range diff.Actions {
		if action.Intent != "building.move" || len(action.DependsOn) > 0 {
			continue
		}
		if _, blocked := beriMoveIssue(diff, action); blocked {
			continue
		}
		return action, true
	}
	return Buildings.TargetAction{}, false
}

func beriMoveIssue(diff Buildings.TargetDiffResult, action Buildings.TargetAction) (Buildings.TargetIssue, bool) {
	for _, target := range diff.Targets {
		if target.TargetID != action.TargetID {
			continue
		}
		for _, issue := range target.Issues {
			switch issue.Code {
			case "source_busy", "placement_reconciliation", "expansion_gift_blocker", "target_outside_ground", "no_space":
				return issue, true
			}
		}
		break
	}
	return Buildings.TargetIssue{}, false
}

func beriRetainedPositionBlocker(castle State.CastleState, diff Buildings.TargetDiffResult) (string, bool) {
	for _, target := range diff.Targets {
		if target.Source == nil || target.Source.Kind != Buildings.TargetSourceExisting || target.RequestedPlacement == nil {
			continue
		}
		building, found := castle.Layout.Objects[target.Source.BuildingInstanceID]
		if !found || !building.Placed || (building.GridX == target.RequestedPlacement.GridX &&
			building.GridY == target.RequestedPlacement.GridY && building.Rotation == target.RequestedPlacement.Rotation) {
			continue
		}
		move := Buildings.TargetAction{TargetID: target.TargetID}
		if issue, blocked := beriMoveIssue(diff, move); blocked {
			return fmt.Sprintf("Cannot move retained %s to its target position yet: %s", target.Desired.DisplayName, issue.Message), true
		}
		return fmt.Sprintf("Cannot move retained %s to its target position until a collision-free staging position is available", target.Desired.DisplayName), true
	}
	return "", false
}

func beriPhaseAction(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	diff Buildings.TargetDiffResult,
	metrics map[string]float64,
	profile autoEventBuildProfile,
	waitDetail string,
) (*Decision, bool, string, error) {
	metrics["targetActionsRemaining"] = float64(diff.Summary.ActionCount)
	blockedDetail := ""
	for _, action := range diff.Actions {
		if len(action.DependsOn) > 0 {
			continue
		}
		if issue, blocked := autoStormTargetActionIssue(diff, action); blocked {
			if blockedDetail == "" {
				blockedDetail = issue.Message
			}
			continue
		}
		storage, err := Buildings.PreviewStorageDependency(snapshot.State, snapshot.GameData, Buildings.StorageDependencyRequest{
			CastleID: castle.ID, EventID: optionalAutoEventBuildID(profile.EventID), Costs: action.Costs,
			ResourceReserves: settings.Build.ResourceReserves,
			AllowPremium:     settings.Build.AllowPremium, AllowResourceTransport: false,
			AllowTimeSkips:               settings.Build.AllowTimeSkips,
			AllowedBuildingDefinitionIDs: beriPhaseStorageDefinitions(diff),
		})
		if err != nil {
			return nil, false, "", err
		}
		if storage.Required {
			if storage.RecommendedAction != nil {
				dependency := *storage.RecommendedAction
				if autoEventBuildActionAllowed(dependency, settings, profile) {
					autoStormApplyTimeSkipReserve(dependency.Arguments, dependency.Intent, settings.Build.TimeSkipReserve)
					return autoStormIntentDecision(snapshot.Now, metrics, dependency.Reason, dependency.Intent, dependency.Arguments), false, "", nil
				}
			}
			if len(storage.Blockers) > 0 {
				if blockedDetail == "" {
					blockedDetail = storage.Blockers[0].Message
				}
				continue
			}
			if blockedDetail == "" {
				blockedDetail = "The current Berimond phase requires more storage capacity"
			}
			continue
		}
		if !action.AffordableNow {
			if blockedDetail == "" {
				blockedDetail = waitDetail
			}
			continue
		}
		if decision := autoStormTargetActionDecision(snapshot.Now, settings, castle, action, metrics, profile); decision != nil {
			return decision, false, "", nil
		}
	}
	if blockedDetail != "" {
		return nil, false, blockedDetail, nil
	}
	if len(diff.Issues) > 0 {
		return nil, false, diff.Issues[0].Message, nil
	}
	return nil, false, waitDetail, nil
}

func beriPhaseStorageDefinitions(diff Buildings.TargetDiffResult) []State.BuildingID {
	result := make([]State.BuildingID, 0, len(diff.Actions))
	for _, action := range diff.Actions {
		if action.Intent == "building.construct" || action.Intent == "building.upgrade" {
			result = append(result, action.Definition.ID)
		}
	}
	return result
}

// The fixed built-in layout must never build an eighth prerequisite store and
// subsequently classify it as unmanaged. Custom targets keep their own policy.
func beriTargetStorageCountReached(castle State.CastleState, target Buildings.TargetCaptureResult, catalog *GameData.BuildingCatalog) bool {
	required := 0
	for _, building := range target.Buildings {
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if found && strings.EqualFold(definition.InternalName, "FactionStorage") {
			required++
		}
	}
	existing := 0
	for _, building := range castle.Layout.Objects {
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if building.Placed && found && strings.EqualFold(definition.InternalName, "FactionStorage") {
			existing++
		}
	}
	return required > 0 && existing >= required
}

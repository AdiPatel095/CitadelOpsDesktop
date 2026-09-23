package Automation

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	stormBuildPhaseHarbor = iota + 1
	stormBuildPhaseStorehouses
	stormBuildPhaseGround
	stormBuildPhaseCleanup
	stormBuildPhaseMove
	stormBuildPhaseDecorations
	stormBuildPhaseCargoEstablish
	stormBuildPhaseFinal

	stormDecorationStorageCollection = "storage:1"
	stormDecorationStorageMaxAge     = 5 * time.Minute
)

// evaluateStrictAutoStormBuild recomputes one ordered phase from authoritative
// state. A blocked phase never spends resources on a later phase.
func evaluateStrictAutoStormBuild(
	snapshot Snapshot,
	settings autoStormSettings,
	castle State.CastleState,
	metrics map[string]float64,
) (*Decision, bool, string, error) {
	if settings.Target == nil && !settings.Harbor.Enabled {
		return nil, true, "", nil
	}
	if settings.Target != nil && settings.Target.KingdomID != autoStormKingdomID {
		return nil, false, "Captured target is not a Storm castle state", nil
	}
	if castle.Layout.ObservedAt.IsZero() || snapshot.Now.Sub(castle.Layout.ObservedAt) > autoStormBuildingRefreshAge ||
		castle.BuildingQueue.ObservedAt.IsZero() || snapshot.Now.Sub(castle.BuildingQueue.ObservedAt) > autoStormBuildingRefreshAge {
		return autoStormIntentDecision(snapshot.Now, metrics, "Refresh the Storm castle building state", "building.refresh", map[string]any{
			"castleId": castle.ID,
		}, Localization.New("server.automation.refresh_the_storm_castle.80be70c1", "Refresh the Storm castle building state", nil)), false, "", nil
	}
	catalog, err := snapshot.GameData.BuildingCatalog()
	if err != nil {
		return nil, false, "", err
	}
	if giftID, found := autoStormExpansionGift(castle, catalog); found {
		return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Collect expansion gift %d before reconciling the Storm layout", giftID), "building.collect_expansion_gift", map[string]any{
			"castleId": castle.ID, "buildingInstanceId": giftID,
		}, Localization.New("server.automation.collect_expansion_gift_p.e9a012eb", "Collect expansion gift {p0} before reconciling the Storm layout", Localization.Params{"p0": fmt.Sprintf("%d", giftID)})), false, "", nil
	}
	profile := autoStormBuildProfile()
	queueDecision, queueBlocked := autoStormQueueDecision(snapshot, settings, castle, catalog, metrics, profile)
	if queueDecision != nil {
		return queueDecision, false, "", nil
	}

	allFixed, err := autoStormFixedTargets(settings, catalog)
	if err != nil {
		return nil, false, err.Error(), nil
	}
	harborTargets, _ := stormPartitionFixedTargets(allFixed, catalog)
	metrics["stormBuildPhase"] = stormBuildPhaseHarbor
	harborDiff, err := stormCompileFixedDiff(snapshot, settings, castle, harborTargets)
	if err != nil {
		return nil, false, "", err
	}
	harborRubyBlocked := rubyPolicyOnlyBlocked(harborDiff)
	if !harborDiff.Satisfied && !harborRubyBlocked {
		if queueBlocked {
			return nil, false, "The Storm construction queue is occupied while the Harbor target is pending", nil
		}
		return stormPhaseAction(snapshot, settings, castle, harborDiff, metrics, profile,
			"Waiting for the configured Harbor upgrade requirements", stormStorehouseDefinitions(catalog))
	}

	normalized := Buildings.TargetCaptureResult{}
	if settings.Target != nil {
		normalized = Buildings.NormalizeTargetCapture(*settings.Target, catalog)
	}
	effectiveTargets := append([]Buildings.TargetBuilding(nil), normalized.Buildings...)
	if !normalized.Exact && settings.DecorationPresetID != "" {
		preset, found := autoStormDecorationPresetFromConfiguration(
			snapshot.Configuration.Sections["decorations.presets"], settings.DecorationPresetCastleID, settings.DecorationPresetID,
		)
		if !found {
			return nil, false, "The selected decoration preset no longer exists", nil
		}
		effectiveTargets = stormAppendDecorationPresetTargets(effectiveTargets, preset)
	}
	fullTargets, storehousePhaseTargets := stormNormalizeStorehouseTargets(effectiveTargets, castle, catalog)
	storehouseTargets, decorationTargets, cargoTargets, finalTargets := stormPartitionOrdinaryTargets(fullTargets, catalog)
	_ = storehouseTargets

	metrics["stormBuildPhase"] = stormBuildPhaseStorehouses
	storehouseDiff, err := stormCompileTargetDiff(snapshot, settings, castle, storehousePhaseTargets)
	if err != nil {
		return nil, false, "", err
	}
	if !storehouseDiff.Satisfied {
		if queueBlocked {
			return nil, false, "The Storm construction queue is occupied while Storehouse upgrades through level 7 are pending", nil
		}
		return stormPhaseAction(snapshot, settings, castle, storehouseDiff, metrics, profile,
			"Waiting for resources or official prerequisites for Storehouse upgrades through level 7", stormStorehouseDefinitions(catalog))
	}
	if settings.Target == nil {
		if harborRubyBlocked {
			return nil, false, rubyPolicyDetail(harborDiff), nil
		}
		return nil, true, "Configured Storm Harbor and existing Storehouses are satisfied", nil
	}

	metrics["stormBuildPhase"] = stormBuildPhaseGround
	missingGround := autoStormMissingGround(castle, normalized.Ground)
	metrics["targetGroundRemaining"] = float64(len(missingGround))
	if len(missingGround) > 0 {
		if queueBlocked {
			return nil, false, "The Storm construction queue is occupied while target expansions are pending", nil
		}
		decision, detail, expansionErr := autoStormExpansionDecisionWithStorage(
			snapshot, settings, castle, missingGround, metrics, profile, stormStorehouseDefinitions(catalog),
		)
		if expansionErr != nil {
			return nil, false, "", expansionErr
		}
		if decision != nil {
			stormCapStorehouseUpgradeDecision(decision, castle, catalog)
			return decision, false, "", nil
		}
		if detail == "" {
			detail = "Waiting for resources to fund the next required Storm expansion"
		}
		return nil, false, detail, nil
	}

	// Validate the complete requested geometry before any exact-target removal.
	validated, err := stormCompileTargetDiff(snapshot, settings, castle, fullTargets)
	if err != nil {
		return nil, false, "", err
	}
	if detail, unsafe := stormUnsafeTargetDiff(validated); unsafe {
		return nil, false, detail, nil
	}
	validatedFixed, err := stormCompileFixedDiff(snapshot, settings, castle, allFixed)
	if err != nil {
		return nil, false, "", err
	}
	if detail, unsafe := stormUnsafeTargetDiff(validatedFixed); unsafe {
		return nil, false, detail, nil
	}

	metrics["stormBuildPhase"] = stormBuildPhaseCleanup
	cleanupDiff, err := stormCompileTargetDiff(snapshot, settings, castle, stormTargetsWithoutPlacement(fullTargets))
	if err != nil {
		return nil, false, "", err
	}
	metrics["unmanagedBuildings"] = float64(cleanupDiff.Summary.UnmanagedCount)
	if normalized.Exact {
		if extra, definition, found := stormCleanupCandidate(castle, cleanupDiff, catalog); found {
			if queueBlocked {
				return nil, false, fmt.Sprintf("The Storm construction queue is occupied while unmanaged %s is pending removal", definition.DisplayName), nil
			}
			if definition.Storeable != nil && *definition.Storeable {
				return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Store unmanaged %s before arranging the target layout", definition.DisplayName), "building.store", map[string]any{
					"castleId": castle.ID, "buildingInstanceId": extra.BuildingInstanceID,
				}, Localization.New("server.automation.store_unmanaged_p_before.64960dea", "Store unmanaged {p0} before arranging the target layout", Localization.Params{"p0": fmt.Sprintf("%s", definition.DisplayName)})), false, "", nil
			}
			if !settings.Build.AllowDemolition {
				return nil, false, fmt.Sprintf("Unmanaged %s must be removed before layout moves; enable demolition to allow this action", definition.DisplayName), nil
			}
			if !autoStormBuildingOfficiallyDestructible(definition) {
				return nil, false, fmt.Sprintf("Unmanaged %s is officially protected and cannot be demolished", definition.DisplayName), nil
			}
			return autoStormIntentDecision(snapshot.Now, metrics, fmt.Sprintf("Demolish unmanaged %s before arranging the target layout", definition.DisplayName), "building.demolish", map[string]any{
				"castleId": castle.ID, "buildingInstanceId": extra.BuildingInstanceID,
			}, Localization.New("server.automation.demolish_unmanaged_p_before.47b57709", "Demolish unmanaged {p0} before arranging the target layout", Localization.Params{"p0": fmt.Sprintf("%s", definition.DisplayName)})), false, "", nil
		}
	}

	metrics["stormBuildPhase"] = stormBuildPhaseMove
	if move, found := stormNextExecutableMove(validated); found {
		return autoStormTargetActionDecision(snapshot.Now, settings, castle, move, metrics, profile), false, "", nil
	}
	if detail, blocked := stormRetainedPositionBlocker(castle, validated); blocked {
		return nil, false, detail, nil
	}

	metrics["stormBuildPhase"] = stormBuildPhaseDecorations
	availableDecorations, missingDecorations, freshnessDecision := stormAvailableDecorationTargets(
		snapshot, castle, decorationTargets, metrics,
	)
	if freshnessDecision != nil {
		return freshnessDecision, false, "", nil
	}
	metrics["stormMissingDecorations"] = float64(missingDecorations)
	decorationDiff, err := stormCompileTargetDiff(snapshot, settings, castle, availableDecorations)
	if err != nil {
		return nil, false, "", err
	}
	if !decorationDiff.Satisfied {
		if queueBlocked {
			return nil, false, "The Storm construction queue is occupied while available target decorations are pending placement", nil
		}
		if stormDiffHasInitialPlacement(decorationDiff) {
			return stormInventoryPlacementAction(snapshot, settings, castle, decorationDiff, metrics, profile)
		}
		return stormPhaseAction(snapshot, settings, castle, decorationDiff, metrics, profile,
			"Waiting to arrange the available target decorations", stormStorehouseDefinitions(catalog))
	}

	metrics["stormBuildPhase"] = stormBuildPhaseCargoEstablish
	cargoDiff, err := stormCompileTargetDiff(snapshot, settings, castle, cargoTargets)
	if err != nil {
		return nil, false, "", err
	}
	if stormCargoNeedsInitialPlacement(cargoDiff) {
		if queueBlocked {
			return nil, false, "The Storm construction queue is occupied while required Cargo ships are still missing", nil
		}
		return stormInitialPlacementAction(snapshot, settings, castle, cargoDiff, metrics, profile,
			"Waiting for resources or free target space to establish every required Cargo ship", stormStorehouseDefinitions(catalog))
	}

	metrics["stormBuildPhase"] = stormBuildPhaseFinal
	finalDiff, err := stormCompileTargetDiff(snapshot, settings, castle, append(append([]Buildings.TargetBuilding{}, cargoTargets...), finalTargets...))
	if err != nil {
		return nil, false, "", err
	}
	metrics["targetBuildingsSatisfied"] = float64(decorationDiff.Summary.SatisfiedCount + finalDiff.Summary.SatisfiedCount + validatedFixed.Summary.SatisfiedCount)
	metrics["targetBuildingsTotal"] = float64(decorationDiff.Summary.TargetCount + finalDiff.Summary.TargetCount + validatedFixed.Summary.TargetCount)
	metrics["targetActionsRemaining"] = float64(finalDiff.Summary.ActionCount + validatedFixed.Summary.ActionCount)
	if finalDiff.Satisfied && validatedFixed.Satisfied {
		return nil, true, "Captured Storm target state satisfied", nil
	}
	if queueBlocked {
		return nil, false, "The Storm construction queue is occupied while final target upgrades are pending", nil
	}
	if !finalDiff.Satisfied {
		return stormPhaseAction(snapshot, settings, castle, finalDiff, metrics, profile,
			"Waiting for resources or official prerequisites to finish the Storm target", stormStorehouseDefinitions(catalog))
	}
	return stormPhaseAction(snapshot, settings, castle, validatedFixed, metrics, profile,
		"Waiting for resources or official prerequisites to finish fixed Storm targets", stormStorehouseDefinitions(catalog))
}

func stormPartitionFixedTargets(targets []Buildings.TargetFixedBuilding, catalog *GameData.BuildingCatalog) (harbor, other []Buildings.TargetFixedBuilding) {
	for _, target := range targets {
		definition, found := catalog.DefinitionView(int64(target.DefinitionID))
		if found && strings.EqualFold(definition.InternalName, "Harbor") {
			harbor = append(harbor, target)
		} else {
			other = append(other, target)
		}
	}
	return harbor, other
}

func stormPartitionOrdinaryTargets(targets []Buildings.TargetBuilding, catalog *GameData.BuildingCatalog) (storehouses, decorations, cargo, final []Buildings.TargetBuilding) {
	for _, target := range targets {
		definition, found := catalog.DefinitionView(int64(target.DefinitionID))
		if !found {
			final = append(final, target)
			continue
		}
		switch {
		case stormStorehouseDefinition(definition):
			storehouses = append(storehouses, target)
		case strings.EqualFold(definition.InternalName, "Cargo"):
			cargo = append(cargo, target)
		case autoStormDecorationDefinition(definition):
			decorations = append(decorations, target)
		default:
			final = append(final, target)
		}
	}
	return
}

func stormNormalizeStorehouseTargets(targets []Buildings.TargetBuilding, castle State.CastleState, catalog *GameData.BuildingCatalog) ([]Buildings.TargetBuilding, []Buildings.TargetBuilding) {
	full := append([]Buildings.TargetBuilding(nil), targets...)
	levelSeven, hasLevelSeven := stormMaximumStorehouseDefinition(catalog)
	existing := make([]State.Building, 0)
	for _, building := range castle.Layout.Objects {
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if found && building.Placed && stormStorehouseDefinition(definition) {
			existing = append(existing, building)
		}
	}
	sort.Slice(existing, func(i, j int) bool { return existing[i].InstanceID < existing[j].InstanceID })
	targetIndexes := make([]int, 0)
	for index := range full {
		definition, found := catalog.DefinitionView(int64(full[index].DefinitionID))
		if !found || !stormStorehouseDefinition(definition) {
			continue
		}
		targetIndexes = append(targetIndexes, index)
		if hasLevelSeven {
			full[index].DefinitionID = State.BuildingID(levelSeven.ID)
		}
	}
	// Existing levels above the cap are valid preserved sources, never downgrade targets.
	higher := make([]State.BuildingID, 0)
	for _, building := range existing {
		definition, _ := catalog.DefinitionView(int64(building.DefinitionID))
		if definition.Level > 7 {
			higher = append(higher, building.DefinitionID)
		}
	}
	for index, definitionID := range higher {
		if index >= len(targetIndexes) {
			break
		}
		full[targetIndexes[index]].DefinitionID = definitionID
	}
	phaseCount := max(len(targetIndexes), len(existing))
	phase := make([]Buildings.TargetBuilding, 0, phaseCount)
	for index, building := range existing {
		definitionID := building.DefinitionID
		definition, _ := catalog.DefinitionView(int64(definitionID))
		if definition.Level <= 7 && hasLevelSeven {
			definitionID = State.BuildingID(levelSeven.ID)
		}
		phase = append(phase, Buildings.TargetBuilding{TargetID: fmt.Sprintf("storm-storehouse-phase-%d", index+1), DefinitionID: definitionID})
	}
	for index := len(existing); index < len(targetIndexes); index++ {
		definitionID := full[targetIndexes[index]].DefinitionID
		phase = append(phase, Buildings.TargetBuilding{TargetID: fmt.Sprintf("storm-storehouse-phase-%d", index+1), DefinitionID: definitionID})
	}
	return full, phase
}

func stormFilterStorehouseTargets(targets []Buildings.TargetBuilding, catalog *GameData.BuildingCatalog) []Buildings.TargetBuilding {
	result := make([]Buildings.TargetBuilding, 0)
	for _, target := range targets {
		definition, found := catalog.DefinitionView(int64(target.DefinitionID))
		if found && stormStorehouseDefinition(definition) {
			result = append(result, target)
		}
	}
	return result
}

func stormStorehouseDefinition(definition GameData.BuildingDefinition) bool {
	return strings.EqualFold(definition.InternalName, "Storehouse")
}

func stormMaximumStorehouseDefinition(catalog *GameData.BuildingCatalog) (GameData.BuildingDefinition, bool) {
	best := GameData.BuildingDefinition{}
	found := false
	for _, definition := range catalog.Definitions() {
		if stormStorehouseDefinition(definition) && definition.Level <= 7 && (!found || definition.Level > best.Level) {
			best, found = definition, true
		}
	}
	return best, found
}

func stormStorehouseDefinitions(catalog *GameData.BuildingCatalog) []State.BuildingID {
	result := make([]State.BuildingID, 0)
	for _, definition := range catalog.Definitions() {
		if stormStorehouseDefinition(definition) && definition.Level <= 7 {
			result = append(result, State.BuildingID(definition.ID))
		}
	}
	if len(result) == 0 {
		return []State.BuildingID{0}
	}
	return result
}

func stormCompileTargetDiff(snapshot Snapshot, settings autoStormSettings, castle State.CastleState, targets []Buildings.TargetBuilding) (Buildings.TargetDiffResult, error) {
	return Buildings.CompileTargetDiff(snapshot.State, snapshot.GameData, Buildings.TargetDiffRequest{
		CastleID: castle.ID, EventID: optionalAutoEventBuildID(autoStormBuildProfile().EventID), Exact: false,
		Policy:    Buildings.TargetDiffPolicy{AllowPremium: settings.Build.AllowPremium, ResourceReserves: settings.Build.ResourceReserves},
		Buildings: targets,
	})
}

func stormCompileFixedDiff(snapshot Snapshot, settings autoStormSettings, castle State.CastleState, targets []Buildings.TargetFixedBuilding) (Buildings.TargetDiffResult, error) {
	return Buildings.CompileFixedTargetDiff(snapshot.State, snapshot.GameData, Buildings.FixedTargetDiffRequest{
		CastleID: castle.ID, EventID: optionalAutoEventBuildID(autoStormBuildProfile().EventID),
		Policy: Buildings.TargetDiffPolicy{AllowPremium: settings.Build.AllowPremium, ResourceReserves: settings.Build.ResourceReserves},
		Fixed:  targets,
	})
}

func stormTargetsWithoutPlacement(targets []Buildings.TargetBuilding) []Buildings.TargetBuilding {
	result := append([]Buildings.TargetBuilding(nil), targets...)
	for index := range result {
		result[index].Placement = nil
	}
	return result
}

func stormUnsafeTargetDiff(diff Buildings.TargetDiffResult) (string, bool) {
	for _, issue := range diff.Issues {
		if issue.Severity != Buildings.TargetIssueError {
			continue
		}
		if strings.HasPrefix(issue.Code, "target_") {
			return issue.Message, true
		}
		switch issue.Code {
		case "duplicate_target_id", "unknown_definition", "ground_definition", "invalid_placement", "not_fixed_definition", "fixed_source_missing":
			return issue.Message, true
		}
	}
	return "", false
}

func stormCleanupCandidate(castle State.CastleState, diff Buildings.TargetDiffResult, catalog *GameData.BuildingCatalog) (Buildings.TargetUnmanagedBuilding, GameData.BuildingDefinition, bool) {
	for _, extra := range diff.Unmanaged {
		building, exists := castle.Layout.Objects[extra.BuildingInstanceID]
		if !exists || !building.Placed || autoStormBuildingQueued(castle, building.InstanceID) {
			continue
		}
		definition, found := catalog.DefinitionView(int64(building.DefinitionID))
		if !found || strings.EqualFold(definition.InternalName, "Harbor") || stormStorehouseDefinition(definition) || fixedAutoStormDefinition(definition) {
			continue
		}
		return extra, definition, true
	}
	return Buildings.TargetUnmanagedBuilding{}, GameData.BuildingDefinition{}, false
}

func stormNextExecutableMove(diff Buildings.TargetDiffResult) (Buildings.TargetAction, bool) {
	for _, action := range diff.Actions {
		if action.Intent == "building.move" && len(action.DependsOn) == 0 {
			if _, blocked := stormMoveIssue(diff, action); !blocked {
				return action, true
			}
		}
	}
	return Buildings.TargetAction{}, false
}

func stormMoveIssue(diff Buildings.TargetDiffResult, action Buildings.TargetAction) (Buildings.TargetIssue, bool) {
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
	}
	return Buildings.TargetIssue{}, false
}

func stormRetainedPositionBlocker(castle State.CastleState, diff Buildings.TargetDiffResult) (string, bool) {
	for _, target := range diff.Targets {
		if target.Source == nil || target.Source.Kind != Buildings.TargetSourceExisting || target.RequestedPlacement == nil {
			continue
		}
		building, found := castle.Layout.Objects[target.Source.BuildingInstanceID]
		if !found || !building.Placed || (building.GridX == target.RequestedPlacement.GridX && building.GridY == target.RequestedPlacement.GridY && building.Rotation == target.RequestedPlacement.Rotation) {
			continue
		}
		if issue, blocked := stormMoveIssue(diff, Buildings.TargetAction{TargetID: target.TargetID}); blocked {
			return fmt.Sprintf("Cannot move retained %s to its target position yet: %s", target.Desired.DisplayName, issue.Message), true
		}
		return fmt.Sprintf("Cannot move retained %s until a collision-free target or staging position is available", target.Desired.DisplayName), true
	}
	return "", false
}

func stormAvailableDecorationTargets(snapshot Snapshot, castle State.CastleState, targets []Buildings.TargetBuilding, metrics map[string]float64) ([]Buildings.TargetBuilding, int, *Decision) {
	if len(targets) == 0 {
		return []Buildings.TargetBuilding{}, 0, nil
	}
	observedAt := snapshot.State.Inventory.ItemsObservedAt[stormDecorationStorageCollection]
	if observedAt.IsZero() || snapshot.Now.Sub(observedAt) > stormDecorationStorageMaxAge {
		decision := autoStormIntentDecision(snapshot.Now, metrics, "Refresh decoration storage before deciding which target decorations are available", "building.storage.refresh", map[string]any{}, Localization.New("server.automation.refresh_decoration_storage_before.78be6db4", "Refresh decoration storage before deciding which target decorations are available", nil))
		return nil, 0, decision
	}
	available := map[State.BuildingID]int64{}
	exactlyPlaced := map[string]int64{}
	for definitionID, count := range snapshot.State.Inventory.Items[stormDecorationStorageCollection] {
		available[State.BuildingID(definitionID)] += count
	}
	for _, building := range castle.Layout.Objects {
		if building.Placed {
			available[building.DefinitionID]++
			exactlyPlaced[fmt.Sprintf("%d:%d:%d:%d", building.DefinitionID, building.GridX, building.GridY, building.Rotation)]++
		}
	}
	selectedTarget := make([]bool, len(targets))
	used := map[State.BuildingID]int64{}
	for index, target := range targets {
		if target.Placement == nil {
			continue
		}
		key := fmt.Sprintf("%d:%d:%d:%d", target.DefinitionID, target.Placement.GridX, target.Placement.GridY, target.Placement.Rotation)
		if exactlyPlaced[key] > 0 {
			exactlyPlaced[key]--
			selectedTarget[index] = true
			used[target.DefinitionID]++
		}
	}
	for index, target := range targets {
		if !selectedTarget[index] && used[target.DefinitionID] < available[target.DefinitionID] {
			selectedTarget[index] = true
			used[target.DefinitionID]++
		}
	}
	selected := make([]Buildings.TargetBuilding, 0, len(targets))
	for index, target := range targets {
		if selectedTarget[index] {
			selected = append(selected, target)
		}
	}
	missing := len(targets) - len(selected)
	return selected, missing, nil
}

func stormDiffHasInitialPlacement(diff Buildings.TargetDiffResult) bool {
	for _, action := range diff.Actions {
		if action.Intent == "building.construct" || action.Intent == "building.place" {
			return true
		}
	}
	return false
}

func stormCargoNeedsInitialPlacement(diff Buildings.TargetDiffResult) bool {
	for _, target := range diff.Targets {
		if target.Source == nil || target.Source.Kind == Buildings.TargetSourceStorage || target.Source.Kind == Buildings.TargetSourceConstruct {
			return true
		}
	}
	return false
}

func stormAppendDecorationPresetTargets(targets []Buildings.TargetBuilding, preset autoStormDecorationPreset) []Buildings.TargetBuilding {
	result := append([]Buildings.TargetBuilding(nil), targets...)
	seen := map[string]struct{}{}
	for _, target := range result {
		if target.Placement == nil {
			continue
		}
		seen[fmt.Sprintf("%d:%d:%d:%d", target.DefinitionID, target.Placement.GridX, target.Placement.GridY, target.Placement.Rotation)] = struct{}{}
	}
	for index, item := range preset.Items {
		key := fmt.Sprintf("%d:%d:%d:%d", item.WID, item.X, item.Y, item.R)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, Buildings.TargetBuilding{
			TargetID: fmt.Sprintf("storm-decoration-preset-%s-%d", preset.ID, index+1), DefinitionID: item.WID,
			Placement: &Buildings.TargetPlacement{GridX: item.X, GridY: item.Y, Rotation: item.R},
		})
	}
	return result
}

func stormInventoryPlacementAction(snapshot Snapshot, settings autoStormSettings, castle State.CastleState, diff Buildings.TargetDiffResult, metrics map[string]float64, profile autoEventBuildProfile) (*Decision, bool, string, error) {
	for _, action := range diff.Actions {
		if action.Intent != "building.place" || len(action.DependsOn) > 0 {
			continue
		}
		if issue, blocked := autoStormTargetActionIssue(diff, action); blocked {
			return nil, false, issue.Message, nil
		}
		if decision := autoStormTargetActionDecision(snapshot.Now, settings, castle, action, metrics, profile); decision != nil {
			return decision, false, "", nil
		}
	}
	return autoStormIntentDecision(snapshot.Now, metrics, "Refresh decoration storage after its available count changed", "building.storage.refresh", map[string]any{}, Localization.New("server.automation.refresh_decoration_storage_after.8e78f1dd", "Refresh decoration storage after its available count changed", nil)), false, "", nil
}

func stormInitialPlacementAction(snapshot Snapshot, settings autoStormSettings, castle State.CastleState, diff Buildings.TargetDiffResult, metrics map[string]float64, profile autoEventBuildProfile, waitDetail string, allowedStorage []State.BuildingID) (*Decision, bool, string, error) {
	for _, action := range diff.Actions {
		if (action.Intent != "building.construct" && action.Intent != "building.place") || len(action.DependsOn) > 0 {
			continue
		}
		single := diff
		single.Actions = []Buildings.TargetAction{action}
		return stormPhaseAction(snapshot, settings, castle, single, metrics, profile, waitDetail, allowedStorage)
	}
	return nil, false, waitDetail, nil
}

func stormPhaseAction(snapshot Snapshot, settings autoStormSettings, castle State.CastleState, diff Buildings.TargetDiffResult, metrics map[string]float64, profile autoEventBuildProfile, waitDetail string, allowedStorage []State.BuildingID) (*Decision, bool, string, error) {
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
			CastleID: castle.ID, Costs: action.Costs, ResourceReserves: settings.Build.ResourceReserves,
			AllowPremium: settings.Build.AllowPremium, AllowResourceTransport: settings.Build.AllowResourceTransport,
			AllowTimeSkips: settings.Build.AllowTimeSkips, AllowedBuildingDefinitionIDs: allowedStorage,
		})
		if err != nil {
			return nil, false, "", err
		}
		if storage.Required {
			if storage.RecommendedAction != nil && autoEventBuildActionAllowed(*storage.RecommendedAction, settings, profile) {
				dependency := *storage.RecommendedAction
				stormCapStorehouseUpgradeArguments(dependency.Intent, dependency.Arguments, castle, snapshot.GameData)
				autoStormApplyTimeSkipReserve(dependency.Arguments, dependency.Intent, settings.Build.TimeSkipReserve)
				return autoStormIntentDecision(snapshot.Now, metrics, dependency.Reason, dependency.Intent, dependency.Arguments), false, "", nil
			}
			if len(storage.Blockers) > 0 && blockedDetail == "" {
				blockedDetail = storage.Blockers[0].Message
			}
			continue
		}
		if !action.AffordableNow {
			if settings.Build.AllowResourceTransport {
				if decision, detail := autoStormTargetTransportDecision(snapshot, settings, castle, action, metrics); decision != nil {
					return decision, false, "", nil
				} else if detail != "" && blockedDetail == "" {
					blockedDetail = detail
				}
			}
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

func stormCapStorehouseUpgradeDecision(decision *Decision, castle State.CastleState, catalog *GameData.BuildingCatalog) {
	if decision == nil || decision.Request == nil || decision.Request.Name != "building.upgrade" {
		return
	}
	arguments := map[string]any{}
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil {
		return
	}
	stormCapStorehouseUpgradeArgumentsWithCatalog(decision.Request.Name, arguments, castle, catalog)
	decision.Request.Arguments, _ = json.Marshal(arguments)
}

func stormCapStorehouseUpgradeArguments(intent string, arguments map[string]any, castle State.CastleState, gameData *GameData.Store) {
	if gameData == nil {
		return
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return
	}
	stormCapStorehouseUpgradeArgumentsWithCatalog(intent, arguments, castle, catalog)
}

func stormCapStorehouseUpgradeArgumentsWithCatalog(intent string, arguments map[string]any, castle State.CastleState, catalog *GameData.BuildingCatalog) {
	if intent != "building.upgrade" || arguments == nil {
		return
	}
	var buildingID State.BuildingInstanceID
	switch value := arguments["buildingInstanceId"].(type) {
	case State.BuildingInstanceID:
		buildingID = value
	case int64:
		buildingID = State.BuildingInstanceID(value)
	case float64:
		buildingID = State.BuildingInstanceID(value)
	}
	building, found := autoStormBuilding(castle, buildingID)
	if !found {
		return
	}
	definition, found := catalog.DefinitionView(int64(building.DefinitionID))
	if found && stormStorehouseDefinition(definition) {
		arguments["maximumLevel"] = int64(7)
	}
}

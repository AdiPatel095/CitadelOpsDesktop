package Buildings

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
	"time"
)

// OperationRemaining shares the automation timing estimate with final dispatch.
// Build/upgrade timing preserves the existing estimator; demolition uses the
// authoritative operation-start boost. Unknown timing remains explicitly unknown.
func OperationRemaining(castle State.CastleState, building State.Building, catalog *GameData.BuildingCatalog, now time.Time) (int64, bool) {
	if catalog == nil {
		return 0, false
	}
	current, found := catalog.DefinitionView(int64(building.DefinitionID))
	if !found {
		return 0, false
	}
	target := current
	inProgress := false
	switch building.ConstructionState {
	case State.BuildingStateBuildStopped:
	case State.BuildingStateBuildInProgress:
		inProgress = true
	case State.BuildingStateUpgradeStopped, State.BuildingStateUpgradeInProgress:
		if current.UpgradeDefinitionID <= 0 {
			return 0, false
		}
		target, found = catalog.DefinitionView(current.UpgradeDefinitionID)
		if !found {
			return 0, false
		}
		inProgress = building.ConstructionState == State.BuildingStateUpgradeInProgress
	case State.BuildingStateDisassembleStopped, State.BuildingStateDisassembleInProgress:
		return DemolitionRemaining(castle, building, catalog, now)
	default:
		return 0, false
	}
	if target.DurationSec <= 0 {
		return 0, false
	}
	progress := max(int64(0), building.ProgressSec)
	if inProgress && !castle.Layout.ObservedAt.IsZero() && now.After(castle.Layout.ObservedAt) {
		progress += int64(now.Sub(castle.Layout.ObservedAt) / time.Second)
	}
	return max(int64(0), target.DurationSec-progress), true
}

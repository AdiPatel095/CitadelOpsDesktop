package Buildings

import (
	"math"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

// DemolitionRemaining follows the official client's getDisassembleDuration:
// int(buildDuration * 0.5 / (wire row 8 / 100)). The client truncates the total
// duration before subtracting observed progress and elapsed whole seconds.
// Source: Game.bundle.a4a25ae6d735e29092f6.js and DLL145565eddbcbe244aab0,
// verified 2026-09-22. Missing boost data must not imply normal (100%) speed.
func DemolitionRemaining(castle State.CastleState, building State.Building, catalog *GameData.BuildingCatalog, now time.Time) (int64, bool) {
	if building.ConstructionState != State.BuildingStateDisassembleStopped && building.ConstructionState != State.BuildingStateDisassembleInProgress {
		return 0, false
	}
	if catalog == nil || castle.Layout.ObservedAt.IsZero() {
		return 0, false
	}
	boost := building.ConstructionBoostPercent
	if boost <= 0 || math.IsNaN(boost) || math.IsInf(boost, 0) {
		return 0, false
	}
	definition, found := catalog.DefinitionView(int64(building.DefinitionID))
	if !found || definition.DurationSec <= 0 {
		return 0, false
	}
	duration := math.Trunc(float64(definition.DurationSec) * 0.5 / (boost / 100))
	if duration < 0 || duration >= math.Exp2(63) || math.IsNaN(duration) || math.IsInf(duration, 0) {
		return 0, false
	}
	remaining := int64(duration) - max(int64(0), building.ProgressSec)
	if remaining <= 0 {
		return 0, true
	}
	if building.ConstructionState == State.BuildingStateDisassembleInProgress && now.After(castle.Layout.ObservedAt) {
		remaining -= int64(now.Sub(castle.Layout.ObservedAt) / time.Second)
	}
	return max(int64(0), remaining), true
}

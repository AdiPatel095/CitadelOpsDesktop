package Buildings

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const (
	MinimumBerimondStableTargetLevel = 1
	MaximumBerimondStableTargetLevel = 5
	DefaultBerimondStableTargetLevel = MaximumBerimondStableTargetLevel
)

// defaultBerimondTargetJSON is the exact reference Berimond camp capture.
// Runtime construction binds it to the current owned camp, upgrades all
// camp and tent families to the terminal WoD in the current official catalog,
// and substitutes the configured Stable level.
//
//go:embed berimond_default_target.json
var defaultBerimondTargetJSON []byte

// DefaultBerimondTarget returns the built-in Auto Beri builder target. The
// template is deliberately independent of a seasonal castle ID and catalog
// version; both are resolved against the active profile at evaluation time.
func DefaultBerimondTarget(
	castleID State.CastleID,
	stableLevel int64,
	gameData *GameData.Store,
) (TargetCaptureResult, error) {
	if castleID <= 0 {
		return TargetCaptureResult{}, fmt.Errorf("default Berimond target requires an owned camp")
	}
	if stableLevel < MinimumBerimondStableTargetLevel || stableLevel > MaximumBerimondStableTargetLevel {
		return TargetCaptureResult{}, fmt.Errorf(
			"Berimond Stable target level must be between %d and %d",
			MinimumBerimondStableTargetLevel,
			MaximumBerimondStableTargetLevel,
		)
	}
	if gameData == nil {
		return TargetCaptureResult{}, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return TargetCaptureResult{}, err
	}
	var target TargetCaptureResult
	if err := json.Unmarshal(defaultBerimondTargetJSON, &target); err != nil {
		return TargetCaptureResult{}, fmt.Errorf("decode built-in Berimond target: %w", err)
	}
	if target.Version != 1 || target.KingdomID != State.KingdomID(GameData.BerimondKingdomID) ||
		target.Mode != TargetCaptureModeExact || !target.Exact {
		return TargetCaptureResult{}, fmt.Errorf("built-in Berimond target metadata is invalid")
	}
	target.CastleID = castleID
	target.CatalogVersion = gameData.Metadata().ItemVersion
	if err := resolveDefaultBerimondDefinitions(&target, catalog, stableLevel); err != nil {
		return TargetCaptureResult{}, err
	}
	return NormalizeTargetCapture(target, catalog), nil
}

func resolveDefaultBerimondDefinitions(
	target *TargetCaptureResult,
	catalog *GameData.BuildingCatalog,
	stableLevel int64,
) error {
	stable, err := defaultBerimondStableDefinition(catalog, stableLevel)
	if err != nil {
		return err
	}
	stableTargets := 0
	for index := range target.Buildings {
		building := &target.Buildings[index]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found {
			return fmt.Errorf("official Berimond building definition %d is unavailable", building.DefinitionID)
		}
		if strings.EqualFold(definition.InternalName, "FactionStable") {
			building.DefinitionID = State.BuildingID(stable.ID)
			stableTargets++
			continue
		}
		if !defaultBerimondCampFamily(definition.InternalName) {
			continue
		}
		maximum, maximumErr := terminalBerimondDefinition(catalog, definition)
		if maximumErr != nil {
			return maximumErr
		}
		building.DefinitionID = State.BuildingID(maximum.ID)
	}
	if stableTargets != 1 {
		return fmt.Errorf("built-in Berimond target contains %d Stable targets; expected 1", stableTargets)
	}
	return nil
}

func defaultBerimondStableDefinition(
	catalog *GameData.BuildingCatalog,
	level int64,
) (GameData.BuildingDefinition, error) {
	var match GameData.BuildingDefinition
	found := false
	for _, definition := range catalog.Definitions() {
		if !strings.EqualFold(definition.InternalName, "FactionStable") ||
			definition.Level != level || !buildingAvailableInKingdom(definition, GameData.BerimondKingdomID) {
			continue
		}
		if found {
			return GameData.BuildingDefinition{}, fmt.Errorf(
				"official data has more than one Berimond Stable definition at level %d", level,
			)
		}
		match, found = definition, true
	}
	if !found {
		return GameData.BuildingDefinition{}, fmt.Errorf(
			"official data has no Berimond Stable definition at level %d", level,
		)
	}
	return match, nil
}

func terminalBerimondDefinition(
	catalog *GameData.BuildingCatalog,
	definition GameData.BuildingDefinition,
) (GameData.BuildingDefinition, error) {
	visited := map[int64]struct{}{}
	current := definition
	for current.UpgradeDefinitionID > 0 {
		if _, duplicate := visited[current.ID]; duplicate {
			return GameData.BuildingDefinition{}, fmt.Errorf(
				"official upgrade chain for Berimond %s contains a cycle", definition.DisplayName,
			)
		}
		visited[current.ID] = struct{}{}
		next, found := catalog.Definition(current.UpgradeDefinitionID)
		if !found {
			return GameData.BuildingDefinition{}, fmt.Errorf(
				"official upgrade definition %d for Berimond %s is unavailable",
				current.UpgradeDefinitionID,
				definition.DisplayName,
			)
		}
		if !strings.EqualFold(next.InternalName, definition.InternalName) {
			return GameData.BuildingDefinition{}, fmt.Errorf(
				"official upgrade chain for Berimond %s changes building family at definition %d",
				definition.DisplayName,
				next.ID,
			)
		}
		current = next
	}
	return current, nil
}

func defaultBerimondCampFamily(internalName string) bool {
	switch strings.ToLower(strings.TrimSpace(internalName)) {
	case "factionmaintent", "factionunittent", "factionpunittent", "factionunitcamp", "factionhuntertent":
		return true
	default:
		return false
	}
}

func buildingAvailableInKingdom(definition GameData.BuildingDefinition, kingdomID int64) bool {
	for _, candidate := range definition.KingdomIDs {
		if candidate == kingdomID {
			return true
		}
	}
	return false
}

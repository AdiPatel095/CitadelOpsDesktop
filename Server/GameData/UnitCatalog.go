package GameData

import "fmt"

const maximumUnitUpgradeFamilySize = 128

type UnitRuntimeDefinition struct {
	ID                    int64
	IsTool                bool
	InternalName          string
	UpgradeDefinitionID   int64
	DowngradeDefinitionID int64
}

// UnitIsTool resolves the immutable official unit kind from a compact index
// built once per game-data generation. A false result with found=true is a
// combat unit; found=false preserves fail-closed behavior for unknown IDs.
func (store *Store) UnitIsTool(id int64) (isTool bool, found bool) {
	if store == nil || id <= 0 {
		return false, false
	}
	store.unitKindsOnce.Do(func() {
		catalog, err := store.Catalog("units")
		if err != nil {
			store.unitKindsErr = err
			return
		}
		store.unitDefinitions = make(map[int64]UnitRuntimeDefinition, len(catalog.rows))
		for _, raw := range catalog.rows {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			unitID, ok := record.Int64("wodID")
			if !ok || unitID <= 0 {
				continue
			}
			store.unitDefinitions[unitID] = UnitRuntimeDefinition{
				ID: unitID, IsTool: IsToolRecord(record), InternalName: stringValue(record, "name"),
				UpgradeDefinitionID:   intValue(record, "upgradeWodID"),
				DowngradeDefinitionID: intValue(record, "downgradeWodID"),
			}
		}
	})
	if store.unitKindsErr != nil {
		return false, false
	}
	definition, found := store.unitDefinitions[id]
	return definition.IsTool, found
}

// UnitRuntimeView returns the compact immutable unit metadata used by
// production and automation hot paths.
func (store *Store) UnitRuntimeView(id int64) (UnitRuntimeDefinition, bool) {
	if store == nil || id <= 0 {
		return UnitRuntimeDefinition{}, false
	}
	// Initialize the shared projection through the same once gate.
	_, _ = store.UnitIsTool(id)
	if store.unitKindsErr != nil {
		return UnitRuntimeDefinition{}, false
	}
	definition, found := store.unitDefinitions[id]
	return definition, found
}

// UnitUpgradeFamily returns the selected combat unit's complete official
// downgrade-to-upgrade chain in ascending tier order. Missing definitions and
// cycles fail closed because callers use this result to substitute live units.
func (store *Store) UnitUpgradeFamily(id int64) ([]UnitRuntimeDefinition, error) {
	anchor, found := store.UnitRuntimeView(id)
	if !found {
		return nil, fmt.Errorf("official unit definition %d is unavailable", id)
	}
	if anchor.IsTool {
		return nil, fmt.Errorf("definition %d is a tool, not a combat unit", id)
	}
	seen := map[int64]struct{}{anchor.ID: {}}
	lower := []UnitRuntimeDefinition{anchor}
	current := anchor
	for current.DowngradeDefinitionID > 0 {
		if len(seen) >= maximumUnitUpgradeFamilySize {
			return nil, fmt.Errorf("unit family for definition %d exceeds %d members", id, maximumUnitUpgradeFamilySize)
		}
		nextID := current.DowngradeDefinitionID
		if _, duplicate := seen[nextID]; duplicate {
			return nil, fmt.Errorf("unit family for definition %d contains a cycle at %d", id, nextID)
		}
		next, exists := store.UnitRuntimeView(nextID)
		if !exists || next.IsTool {
			return nil, fmt.Errorf("unit family for definition %d has invalid downgrade %d", id, nextID)
		}
		seen[nextID] = struct{}{}
		lower = append(lower, next)
		current = next
	}
	for left, right := 0, len(lower)-1; left < right; left, right = left+1, right-1 {
		lower[left], lower[right] = lower[right], lower[left]
	}

	family := append([]UnitRuntimeDefinition(nil), lower...)
	current = anchor
	for current.UpgradeDefinitionID > 0 {
		if len(seen) >= maximumUnitUpgradeFamilySize {
			return nil, fmt.Errorf("unit family for definition %d exceeds %d members", id, maximumUnitUpgradeFamilySize)
		}
		nextID := current.UpgradeDefinitionID
		if _, duplicate := seen[nextID]; duplicate {
			return nil, fmt.Errorf("unit family for definition %d contains a cycle at %d", id, nextID)
		}
		next, exists := store.UnitRuntimeView(nextID)
		if !exists || next.IsTool {
			return nil, fmt.Errorf("unit family for definition %d has invalid upgrade %d", id, nextID)
		}
		seen[nextID] = struct{}{}
		family = append(family, next)
		current = next
	}
	return family, nil
}

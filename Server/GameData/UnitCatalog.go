package GameData

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

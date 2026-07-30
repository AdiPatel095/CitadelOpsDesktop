package GameData

import (
	"fmt"
	"sort"
)

type ExpansionDefinition struct {
	ID               int64          `json:"id"`
	SpaceIDs         []int64        `json:"spaceIds"`
	Level            int64          `json:"level"`
	Costs            []BuildingCost `json:"costs"`
	SceatSkillLocked int64          `json:"sceatSkillLocked,omitempty"`
	EffectLocked     bool           `json:"effectLocked"`
}

type ExpansionCatalog struct {
	definitions  []ExpansionDefinition
	bySpaceLevel map[int64]map[int64]ExpansionDefinition
}

func (store *Store) ExpansionCatalog() (*ExpansionCatalog, error) {
	store.expansionCatalogOnce.Do(func() {
		store.expansionCatalog, store.expansionCatalogErr = buildExpansionCatalog(store)
	})
	return store.expansionCatalog, store.expansionCatalogErr
}

func (catalog *ExpansionCatalog) Definition(spaceID int64, level int64) (ExpansionDefinition, bool) {
	if catalog == nil {
		return ExpansionDefinition{}, false
	}
	levels := catalog.bySpaceLevel[spaceID]
	definition, found := levels[level]
	if !found {
		return ExpansionDefinition{}, false
	}
	return cloneExpansionDefinition(definition), true
}

func (catalog *ExpansionCatalog) Definitions() []ExpansionDefinition {
	if catalog == nil {
		return nil
	}
	result := make([]ExpansionDefinition, len(catalog.definitions))
	for index, definition := range catalog.definitions {
		result[index] = cloneExpansionDefinition(definition)
	}
	return result
}

func buildExpansionCatalog(store *Store) (*ExpansionCatalog, error) {
	expansions, err := store.Catalog("expansions")
	if err != nil {
		return nil, err
	}
	references := buildingCostReferences(store)
	definitions := make([]ExpansionDefinition, 0, expansions.Summary().Count)
	bySpaceLevel := map[int64]map[int64]ExpansionDefinition{}
	for _, raw := range expansions.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode expansion definition: %w", decodeErr)
		}
		id, idOK := record.Int64("expansionID")
		level, levelOK := record.Int64("expansionLevel")
		spaceIDs := intList(record, "spaceIDs")
		if !idOK || id <= 0 || !levelOK || level <= 0 || len(spaceIDs) == 0 {
			continue
		}
		definition := ExpansionDefinition{
			ID: id, SpaceIDs: spaceIDs, Level: level,
			Costs:            decodeBuildingCosts(record, references),
			SceatSkillLocked: intValue(record, "sceatSkillLocked"),
			EffectLocked:     buildingBoolValue(record, "effectLocked"),
		}
		definitions = append(definitions, definition)
		for _, spaceID := range spaceIDs {
			if bySpaceLevel[spaceID] == nil {
				bySpaceLevel[spaceID] = map[int64]ExpansionDefinition{}
			}
			if _, duplicate := bySpaceLevel[spaceID][level]; duplicate {
				return nil, fmt.Errorf("official expansion data has duplicate space %d level %d", spaceID, level)
			}
			bySpaceLevel[spaceID][level] = definition
		}
	}
	sort.Slice(definitions, func(left, right int) bool {
		if definitions[left].Level != definitions[right].Level {
			return definitions[left].Level < definitions[right].Level
		}
		return definitions[left].ID < definitions[right].ID
	})
	return &ExpansionCatalog{definitions: definitions, bySpaceLevel: bySpaceLevel}, nil
}

func cloneExpansionDefinition(source ExpansionDefinition) ExpansionDefinition {
	clone := source
	clone.SpaceIDs = append([]int64(nil), source.SpaceIDs...)
	clone.Costs = append([]BuildingCost(nil), source.Costs...)
	return clone
}

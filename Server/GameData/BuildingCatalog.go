package GameData

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

const (
	BuildingCostCastleResource = "castle_resource"
	BuildingCostPlayerResource = "player_resource"
	BuildingCostCurrency       = "currency"
	BuildingCostUnknown        = "unknown"
)

type BuildingCatalog struct {
	definitions []BuildingDefinition
	byID        map[int64]BuildingDefinition
}

func (store *Store) BuildingCatalog() (*BuildingCatalog, error) {
	store.buildingCatalogOnce.Do(func() {
		store.buildingCatalog, store.buildingCatalogErr = buildBuildingCatalog(store)
	})
	return store.buildingCatalog, store.buildingCatalogErr
}

func (catalog *BuildingCatalog) Definitions() []BuildingDefinition {
	if catalog == nil {
		return nil
	}
	result := make([]BuildingDefinition, len(catalog.definitions))
	for index, definition := range catalog.definitions {
		result[index] = cloneBuildingDefinition(definition)
	}
	return result
}

func (catalog *BuildingCatalog) Definition(id int64) (BuildingDefinition, bool) {
	if catalog == nil {
		return BuildingDefinition{}, false
	}
	definition, found := catalog.byID[id]
	if !found {
		return BuildingDefinition{}, false
	}
	return cloneBuildingDefinition(definition), true
}

// UpgradePath returns the inclusive official forward-upgrade path from source
// to target. It rejects missing links and cycles instead of inferring a family
// from names or level numbers.
func (catalog *BuildingCatalog) UpgradePath(sourceID int64, targetID int64) ([]BuildingDefinition, bool) {
	if catalog == nil || sourceID <= 0 || targetID <= 0 {
		return nil, false
	}
	current, found := catalog.byID[sourceID]
	if !found {
		return nil, false
	}
	path := make([]BuildingDefinition, 0, 4)
	visited := map[int64]struct{}{}
	for {
		if _, duplicate := visited[current.ID]; duplicate {
			return nil, false
		}
		visited[current.ID] = struct{}{}
		path = append(path, cloneBuildingDefinition(current))
		if current.ID == targetID {
			return path, true
		}
		if current.UpgradeDefinitionID <= 0 {
			return nil, false
		}
		next, nextFound := catalog.byID[current.UpgradeDefinitionID]
		if !nextFound {
			return nil, false
		}
		current = next
	}
}

// ConstructionPath returns the inclusive official chain from the construction
// root to target. Both backward and forward links must agree so an executor can
// safely construct the root and then apply each returned upgrade in order.
func (catalog *BuildingCatalog) ConstructionPath(targetID int64) ([]BuildingDefinition, bool) {
	if catalog == nil || targetID <= 0 {
		return nil, false
	}
	current, found := catalog.byID[targetID]
	if !found {
		return nil, false
	}
	reversed := make([]BuildingDefinition, 0, 4)
	visited := map[int64]struct{}{}
	for {
		if _, duplicate := visited[current.ID]; duplicate {
			return nil, false
		}
		visited[current.ID] = struct{}{}
		reversed = append(reversed, cloneBuildingDefinition(current))
		if current.DowngradeDefinitionID <= 0 {
			break
		}
		previous, previousFound := catalog.byID[current.DowngradeDefinitionID]
		if !previousFound || previous.UpgradeDefinitionID != current.ID {
			return nil, false
		}
		current = previous
	}
	path := make([]BuildingDefinition, len(reversed))
	for index := range reversed {
		path[len(reversed)-1-index] = reversed[index]
	}
	return path, true
}

type buildingCostReference struct {
	id    int64
	key   string
	scope string
}

func buildBuildingCatalog(store *Store) (*BuildingCatalog, error) {
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return nil, err
	}
	costReferences := buildingCostReferences(store)
	definitions := make([]BuildingDefinition, 0, buildings.Summary().Count)
	byID := make(map[int64]BuildingDefinition, buildings.Summary().Count)
	for _, raw := range buildings.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			return nil, fmt.Errorf("decode building definition: %w", decodeErr)
		}
		id, ok := record.Int64("wodID")
		if !ok || id <= 0 {
			continue
		}
		definition := decodeBuildingDefinition(record, id, costReferences)
		definitions = append(definitions, definition)
		byID[id] = definition
	}
	objectivesByDefinition := buildingQuestObjectives(store)
	for index := range definitions {
		definitions[index].QuestObjectives = append(
			[]BuildingQuestObjective(nil), objectivesByDefinition[definitions[index].ID]...,
		)
		byID[definitions[index].ID] = definitions[index]
	}
	sort.Slice(definitions, func(left, right int) bool { return definitions[left].ID < definitions[right].ID })
	return &BuildingCatalog{definitions: definitions, byID: byID}, nil
}

func decodeBuildingDefinition(record Record, id int64, references map[string]buildingCostReference) BuildingDefinition {
	internalName := stringValue(record, "name")
	displayName := stringValue(record, "_display_name")
	if displayName == "" {
		displayName = internalName
	}
	if displayName == "" {
		displayName = fmt.Sprintf("Building %d", id)
	}
	return BuildingDefinition{
		ID:                       id,
		InternalName:             internalName,
		Type:                     stringValue(record, "type"),
		Group:                    stringValue(record, "group"),
		ShopCategory:             stringValue(record, "shopCategory"),
		GroundType:               stringValue(record, "buildingGroundType"),
		Level:                    intValue(record, "level"),
		Width:                    int(intValue(record, "width")),
		Height:                   int(intValue(record, "height")),
		RotateType:               int(intValue(record, "rotateType")),
		UpgradeDefinitionID:      intValue(record, "upgradeWodID"),
		DowngradeDefinitionID:    intValue(record, "downgradeWodID"),
		RequiredLevel:            optionalBuildingInt(record, "requiredLevel"),
		RequiredLegendLevel:      optionalBuildingInt(record, "requiredLegendLevel"),
		EarlyUnlockRequiredLevel: optionalBuildingInt(record, "earlyUnlockRequiredLevel"),
		MaximumCount:             optionalBuildingInt(record, "maximumCount"),
		KingdomIDs:               intList(record, "kIDs"),
		EventIDs:                 intList(record, "eventIDs"),
		AreaTypeIDs:              intList(record, "onlyInAreaTypes"),
		MapIDs:                   intList(record, "mapIDs"),
		DurationSec:              intValue(record, "buildDuration"),
		LowLevelDurationSec:      intValue(record, "lowLevelBuildDuration"),
		ForcedPosition:           buildingScalarString(record["forcedPosition"]),
		Storeable:                optionalBuildingBool(record, "storeable"),
		Movable:                  optionalBuildingBool(record, "movable"),
		Destructable:             optionalBuildingBool(record, "destructable"),
		BattleGround:             optionalBuildingBool(record, "isBattleGround"),
		District:                 optionalBuildingBool(record, "isDistrict"),
		ConstructionItemGroupIDs: intList(record, "constructionItemGroupIDs"),
		UnlockIDs:                intList(record, "unlockIDs"),
		Costs:                    decodeBuildingCosts(record, references),
		Values:                   decodeBuildingValues(record),
		LocalizationKeys:         buildingLocalizationKeys(record),
		DisplayName:              displayName,
	}
}

func buildingCostReferences(store *Store) map[string]buildingCostReference {
	result := map[string]buildingCostReference{}
	if resources, err := store.Catalog("resources"); err == nil {
		for _, raw := range resources.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, idOK := record.Int64("resourceID")
			key, keyOK := record.String("JSONKey")
			if !idOK || !keyOK || key == "" {
				continue
			}
			scope := BuildingCostCastleResource
			if strings.EqualFold(key, "C1") || strings.EqualFold(key, "C2") {
				scope = BuildingCostPlayerResource
			}
			reference := buildingCostReference{id: id, key: key, scope: scope}
			for _, name := range []string{key, stringValue(record, "name")} {
				if normalized := normalizeBuildingName(name); normalized != "" {
					result[normalized] = reference
				}
			}
		}
	}
	if currencies, err := store.Catalog("currencies"); err == nil {
		for _, raw := range currencies.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			id, idOK := record.Int64("currencyID")
			key, keyOK := record.String("JSONKey")
			if !idOK || !keyOK || key == "" {
				continue
			}
			reference := buildingCostReference{id: id, key: key, scope: BuildingCostCurrency}
			for _, name := range []string{key, stringValue(record, "Name"), stringValue(record, "name")} {
				if normalized := normalizeBuildingName(name); normalized != "" {
					if _, exists := result[normalized]; !exists {
						result[normalized] = reference
					}
				}
			}
		}
	}
	return result
}

func decodeBuildingCosts(record Record, references map[string]buildingCostReference) []BuildingCost {
	fields := make([]string, 0)
	for field := range record {
		if strings.HasPrefix(field, "cost") {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	costs := make([]BuildingCost, 0, len(fields))
	for _, field := range fields {
		amount, ok := record.Float64(field)
		if !ok || amount <= 0 {
			continue
		}
		suffix := strings.TrimPrefix(field, "cost")
		reference, found := references[normalizeBuildingName(suffix)]
		cost := BuildingCost{Field: field, Key: suffix, Scope: BuildingCostUnknown, Amount: amount}
		if found {
			cost.Key = reference.key
			cost.Scope = reference.scope
			cost.DefinitionID = reference.id
		}
		cost.Premium = strings.EqualFold(cost.Key, "C2") || strings.EqualFold(field, "costC2")
		costs = append(costs, cost)
	}
	return costs
}

var buildingStructuralFields = map[string]struct{}{
	"wodID": {}, "name": {}, "_display_name": {}, "type": {}, "group": {}, "shopCategory": {}, "buildingGroundType": {},
	"level": {}, "width": {}, "height": {}, "rotateType": {}, "upgradeWodID": {}, "downgradeWodID": {},
	"requiredLevel": {}, "requiredLegendLevel": {}, "earlyUnlockRequiredLevel": {}, "maximumCount": {},
	"kIDs": {}, "eventIDs": {}, "onlyInAreaTypes": {}, "mapIDs": {}, "buildDuration": {},
	"lowLevelBuildDuration": {}, "forcedPosition": {}, "constructionItemGroupIDs": {}, "unlockIDs": {}, "sortOrder": {},
	"storeable": {}, "movable": {}, "destructable": {}, "isBattleGround": {}, "isDistrict": {},
	"burnable": {}, "smashable": {}, "tempServerBurnable": {}, "tempServerDestructable": {},
}

func decodeBuildingValues(record Record) map[string]float64 {
	values := map[string]float64{}
	for field := range record {
		if strings.HasPrefix(field, "cost") {
			continue
		}
		if _, structural := buildingStructuralFields[field]; structural {
			continue
		}
		value, ok := record.Float64(field)
		if ok && value != 0 {
			values[field] = value
		}
	}
	return values
}

func buildingLocalizationKeys(record Record) []string {
	result := []string{}
	seen := map[string]struct{}{}
	for _, value := range []string{stringValue(record, "type"), stringValue(record, "name")} {
		for _, key := range []string{value + "_name", value} {
			if value == "" {
				continue
			}
			if _, duplicate := seen[key]; duplicate {
				continue
			}
			seen[key] = struct{}{}
			result = append(result, key)
		}
	}
	return result
}

func buildingQuestObjectives(store *Store) map[int64][]BuildingQuestObjective {
	result := map[int64][]BuildingQuestObjective{}
	quests, err := store.Catalog("quests")
	if err != nil {
		return result
	}
	for _, raw := range quests.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		questID, ok := record.Int64("questID")
		if !ok || questID <= 0 {
			continue
		}
		conditions := strings.Split(stringValue(record, "conditions"), "#")
		for _, condition := range conditions {
			parts := strings.Split(strings.TrimSpace(condition), "+")
			if len(parts) < 3 || !strings.EqualFold(strings.TrimSpace(parts[0]), "buildings") {
				continue
			}
			requiredCount, countErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64)
			definitionID, definitionErr := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
			if countErr != nil || definitionErr != nil || requiredCount <= 0 || definitionID <= 0 {
				continue
			}
			objective := BuildingQuestObjective{
				QuestID: questID, RequiredCount: requiredCount,
				RequiredLevel:       optionalBuildingInt(record, "requiredLevel"),
				RequiredLegendLevel: optionalBuildingInt(record, "requiredLegendLevel"),
				RequiredQuestID:     intValue(record, "requiredQuestID"),
				OrRequiredQuestID:   intValue(record, "orRequiredQuestID"),
				EventID:             intValue(record, "eventID"),
				ShownKingdomID:      intValue(record, "shownKingdomID"),
				TriggerKingdomID:    intValue(record, "triggerKingdomID"),
				XP:                  floatValue(record, "xp"),
				Hidden:              buildingBoolValue(record, "hidden"),
				SortPriority:        intValue(record, "sortPriority"),
			}
			result[definitionID] = append(result[definitionID], objective)
		}
	}
	for definitionID := range result {
		sort.Slice(result[definitionID], func(left, right int) bool {
			return result[definitionID][left].QuestID < result[definitionID][right].QuestID
		})
	}
	return result
}

func optionalBuildingInt(record Record, field string) *int64 {
	value, ok := record.Int64(field)
	if !ok {
		return nil
	}
	copy := value
	return &copy
}

func optionalBuildingBool(record Record, field string) *bool {
	if _, found := record[field]; !found {
		return nil
	}
	if value, ok := record.Int64(field); ok {
		result := value != 0
		return &result
	}
	value, ok := record.String(field)
	if !ok {
		return nil
	}
	value = strings.TrimSpace(strings.ToLower(value))
	result := value == "true" || value == "yes"
	return &result
}

func buildingBoolValue(record Record, field string) bool {
	value := optionalBuildingBool(record, field)
	return value != nil && *value
}

func buildingScalarString(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	if value, ok := scalarKey(raw); ok {
		return value
	}
	return strings.TrimSpace(string(raw))
}

func normalizeBuildingName(value string) string {
	return strings.Map(func(character rune) rune {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			return unicode.ToLower(character)
		}
		return -1
	}, value)
}

func cloneBuildingDefinition(source BuildingDefinition) BuildingDefinition {
	clone := source
	clone.RequiredLevel = cloneBuildingInt64Pointer(source.RequiredLevel)
	clone.RequiredLegendLevel = cloneBuildingInt64Pointer(source.RequiredLegendLevel)
	clone.EarlyUnlockRequiredLevel = cloneBuildingInt64Pointer(source.EarlyUnlockRequiredLevel)
	clone.MaximumCount = cloneBuildingInt64Pointer(source.MaximumCount)
	clone.Storeable = cloneBuildingBoolPointer(source.Storeable)
	clone.Movable = cloneBuildingBoolPointer(source.Movable)
	clone.Destructable = cloneBuildingBoolPointer(source.Destructable)
	clone.BattleGround = cloneBuildingBoolPointer(source.BattleGround)
	clone.District = cloneBuildingBoolPointer(source.District)
	clone.KingdomIDs = append([]int64(nil), source.KingdomIDs...)
	clone.EventIDs = append([]int64(nil), source.EventIDs...)
	clone.AreaTypeIDs = append([]int64(nil), source.AreaTypeIDs...)
	clone.MapIDs = append([]int64(nil), source.MapIDs...)
	clone.ConstructionItemGroupIDs = append([]int64(nil), source.ConstructionItemGroupIDs...)
	clone.UnlockIDs = append([]int64(nil), source.UnlockIDs...)
	clone.QuestObjectives = make([]BuildingQuestObjective, len(source.QuestObjectives))
	for index, objective := range source.QuestObjectives {
		objective.RequiredLevel = cloneBuildingInt64Pointer(objective.RequiredLevel)
		objective.RequiredLegendLevel = cloneBuildingInt64Pointer(objective.RequiredLegendLevel)
		clone.QuestObjectives[index] = objective
	}
	clone.Costs = append([]BuildingCost(nil), source.Costs...)
	clone.Values = make(map[string]float64, len(source.Values))
	for key, value := range source.Values {
		clone.Values[key] = value
	}
	clone.LocalizationKeys = append([]string(nil), source.LocalizationKeys...)
	return clone
}

func cloneBuildingInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func cloneBuildingBoolPointer(value *bool) *bool {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

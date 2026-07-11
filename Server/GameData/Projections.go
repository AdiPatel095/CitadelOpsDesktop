package GameData

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
)

type UnitDefinition struct {
	ID           int64  `json:"id"`
	InternalName string `json:"internalName,omitempty"`
	Type         string `json:"type,omitempty"`
	Group        string `json:"group,omitempty"`
	SortOrder    int64  `json:"sortOrder,omitempty"`
	DisplayName  string `json:"displayName"`
}

type BuildingDefinition struct {
	ID                       int64   `json:"id"`
	InternalName             string  `json:"internalName,omitempty"`
	Type                     string  `json:"type,omitempty"`
	Group                    string  `json:"group,omitempty"`
	Level                    int64   `json:"level,omitempty"`
	ConstructionItemGroupIDs []int64 `json:"constructionItemGroupIds,omitempty"`
	DisplayName              string  `json:"displayName"`
}

type ConstructionItemDefinition struct {
	ID            int64  `json:"id"`
	GroupID       int64  `json:"groupId,omitempty"`
	EffectGroupID int64  `json:"effectGroupId,omitempty"`
	RarenessID    int64  `json:"rarenessId,omitempty"`
	Level         int64  `json:"level,omitempty"`
	InternalName  string `json:"internalName,omitempty"`
	DisplayName   string `json:"displayName"`
}

type PackageDefinition struct {
	ID           int64   `json:"id"`
	PackageType  string  `json:"packageType,omitempty"`
	RelationIDs  []int64 `json:"relationIds,omitempty"`
	Stock        int64   `json:"stock,omitempty"`
	InternalName string  `json:"internalName,omitempty"`
	DisplayName  string  `json:"displayName"`
}

type ResourceDefinition struct {
	ID           int64  `json:"id"`
	JSONKey      string `json:"jsonKey,omitempty"`
	InternalName string `json:"internalName,omitempty"`
	DisplayName  string `json:"displayName"`
}

type CurrencyDefinition struct {
	ID           int64  `json:"id"`
	JSONKey      string `json:"jsonKey,omitempty"`
	AssetName    string `json:"assetName,omitempty"`
	InternalName string `json:"internalName,omitempty"`
	DisplayName  string `json:"displayName"`
}

type EffectDefinition struct {
	ID           int64  `json:"id"`
	EffectTypeID int64  `json:"effectTypeId,omitempty"`
	CapID        int64  `json:"capId,omitempty"`
	InternalName string `json:"internalName,omitempty"`
	DisplayName  string `json:"displayName"`
}

type EquipmentDefinition struct {
	ID          int64   `json:"id"`
	SetID       int64   `json:"setId,omitempty"`
	SlotID      int64   `json:"slotId,omitempty"`
	WearerID    int64   `json:"wearerId,omitempty"`
	MightValue  float64 `json:"mightValue,omitempty"`
	DisplayName string  `json:"displayName"`
}

type GemDefinition struct {
	ID            int64  `json:"id"`
	GemLevelID    int64  `json:"gemLevelId,omitempty"`
	SetID         int64  `json:"setId,omitempty"`
	WearerID      int64  `json:"wearerId,omitempty"`
	TriggerChance int64  `json:"triggerChance,omitempty"`
	DisplayName   string `json:"displayName"`
}

type CraftingRecipeDefinition struct {
	ID          int64   `json:"id"`
	GroupID     int64   `json:"groupId,omitempty"`
	QueueTypeID int64   `json:"queueTypeId,omitempty"`
	Level       int64   `json:"level,omitempty"`
	DurationSec int64   `json:"durationSec,omitempty"`
	RewardIDs   []int64 `json:"rewardIds,omitempty"`
	DisplayName string  `json:"displayName"`
}

func (manager *Manager) Unit(id int64) (UnitDefinition, error) {
	record, err := manager.record("units", id)
	if err != nil {
		return UnitDefinition{}, err
	}
	return UnitDefinition{
		ID:           id,
		InternalName: stringValue(record, "name"),
		Type:         stringValue(record, "type"),
		Group:        stringValue(record, "group"),
		SortOrder:    intValue(record, "sortOrder"),
		DisplayName:  manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Building(id int64) (BuildingDefinition, error) {
	record, err := manager.record("buildings", id)
	if err != nil {
		return BuildingDefinition{}, err
	}
	return BuildingDefinition{
		ID:                       id,
		InternalName:             stringValue(record, "name"),
		Type:                     stringValue(record, "type"),
		Group:                    stringValue(record, "group"),
		Level:                    intValue(record, "level"),
		ConstructionItemGroupIDs: intList(record, "constructionItemGroupIDs"),
		DisplayName:              manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) ConstructionItem(id int64) (ConstructionItemDefinition, error) {
	record, err := manager.record("constructionItems", id)
	if err != nil {
		return ConstructionItemDefinition{}, err
	}
	return ConstructionItemDefinition{
		ID:            id,
		GroupID:       intValue(record, "constructionItemGroupID"),
		EffectGroupID: intValue(record, "constructionItemEffectGroupID"),
		RarenessID:    intValue(record, "rarenessID"),
		Level:         intValue(record, "level"),
		InternalName:  stringValue(record, "name"),
		DisplayName:   manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Package(id int64) (PackageDefinition, error) {
	record, err := manager.record("packages", id)
	if err != nil {
		return PackageDefinition{}, err
	}
	return PackageDefinition{
		ID:           id,
		PackageType:  stringValue(record, "packageType"),
		RelationIDs:  intList(record, "relationIDs"),
		Stock:        intValue(record, "stock"),
		InternalName: stringValue(record, "name"),
		DisplayName:  manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Resource(id int64) (ResourceDefinition, error) {
	record, err := manager.record("resources", id)
	if err != nil {
		return ResourceDefinition{}, err
	}
	return ResourceDefinition{
		ID:           id,
		JSONKey:      stringValue(record, "JSONKey"),
		InternalName: stringValue(record, "name"),
		DisplayName:  manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Currency(id int64) (CurrencyDefinition, error) {
	record, err := manager.record("currencies", id)
	if err != nil {
		return CurrencyDefinition{}, err
	}
	return CurrencyDefinition{
		ID:           id,
		JSONKey:      stringValue(record, "JSONKey"),
		AssetName:    stringValue(record, "assetName"),
		InternalName: stringValue(record, "Name"),
		DisplayName:  manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Effect(id int64) (EffectDefinition, error) {
	record, err := manager.record("effects", id)
	if err != nil {
		return EffectDefinition{}, err
	}
	return EffectDefinition{
		ID:           id,
		EffectTypeID: intValue(record, "effectTypeID"),
		CapID:        intValue(record, "capID"),
		InternalName: stringValue(record, "name"),
		DisplayName:  manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Equipment(id int64) (EquipmentDefinition, error) {
	record, err := manager.record("equipments", id)
	if err != nil {
		return EquipmentDefinition{}, err
	}
	return EquipmentDefinition{
		ID:          id,
		SetID:       intValue(record, "setID"),
		SlotID:      intValue(record, "slotID"),
		WearerID:    intValue(record, "wearerID"),
		MightValue:  floatValue(record, "mightValue"),
		DisplayName: manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) Gem(id int64) (GemDefinition, error) {
	record, err := manager.record("gems", id)
	if err != nil {
		return GemDefinition{}, err
	}
	return GemDefinition{
		ID:            id,
		GemLevelID:    intValue(record, "gemLevelID"),
		SetID:         intValue(record, "setID"),
		WearerID:      intValue(record, "wearerID"),
		TriggerChance: intValue(record, "triggerChance"),
		DisplayName:   manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) CraftingRecipe(id int64) (CraftingRecipeDefinition, error) {
	record, err := manager.record("craftingRecipes", id)
	if err != nil {
		return CraftingRecipeDefinition{}, err
	}
	return CraftingRecipeDefinition{
		ID:          id,
		GroupID:     intValue(record, "recipeGroupID"),
		QueueTypeID: intValue(record, "queueTypeId"),
		Level:       intValue(record, "level"),
		DurationSec: intValue(record, "craftingDuration"),
		RewardIDs:   intList(record, "rewardIDs"),
		DisplayName: manager.displayName(record, strconv.FormatInt(id, 10)),
	}, nil
}

func (manager *Manager) record(collection string, id int64) (Record, error) {
	store, ready := manager.Current()
	if !ready {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return nil, err
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return nil, fmt.Errorf("%s definition %d was not found", collection, id)
	}
	return DecodeRecord(raw)
}

func (manager *Manager) displayName(record Record, fallback string) string {
	if displayName, ok := record.String("_display_name"); ok && displayName != "" {
		return displayName
	}
	internalNames := []string{
		stringValue(record, "type"),
		stringValue(record, "name"),
		stringValue(record, "Name"),
		stringValue(record, "JSONKey"),
	}
	if language, ready := manager.Language(); ready {
		keys := make([]string, 0, len(internalNames)*2)
		for _, name := range internalNames {
			if name != "" {
				keys = append(keys, name+"_name", name)
			}
		}
		if displayName, ok := language.Resolve(keys...); ok {
			return displayName
		}
	}
	for _, name := range internalNames {
		if name != "" {
			return name
		}
	}
	return fallback
}

func stringValue(record Record, field string) string {
	value, _ := record.String(field)
	return value
}

func intValue(record Record, field string) int64 {
	value, _ := record.Int64(field)
	return value
}

func floatValue(record Record, field string) float64 {
	value, _ := record.Float64(field)
	return value
}

func intList(record Record, field string) []int64 {
	raw := record[field]
	if len(raw) == 0 {
		return nil
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	result := make([]int64, 0, len(values))
	for _, value := range values {
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.UseNumber()
		var decoded any
		if decoder.Decode(&decoded) != nil {
			continue
		}
		switch typed := decoded.(type) {
		case json.Number:
			if integer, err := typed.Int64(); err == nil {
				result = append(result, integer)
			}
		case string:
			if integer, err := strconv.ParseInt(typed, 10, 64); err == nil {
				result = append(result, integer)
			}
		}
	}
	return result
}

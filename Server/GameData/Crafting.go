package GameData

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

type CraftingResource struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	JSONKey   string `json:"jsonKey,omitempty"`
	AssetName string `json:"assetName,omitempty"`
	IconURL   string `json:"iconUrl,omitempty"`
	SourceID  int64  `json:"-"`
}

type CraftingRecipeOutput struct {
	RewardID int64   `json:"rewardID"`
	Key      string  `json:"key"`
	Name     string  `json:"name"`
	Amount   float64 `json:"amount"`
	IconURL  string  `json:"iconUrl,omitempty"`
}

type CraftingRecipe struct {
	RecipeID             int64                `json:"recipeID"`
	QueueTypeID          int                  `json:"queueTypeID"`
	RecipeGroupID        int64                `json:"recipeGroupID"`
	ResearchGroupID      int64                `json:"researchGroupID,omitempty"`
	Level                int                  `json:"level"`
	Type                 string               `json:"type"`
	DurationSec          int                  `json:"durationSec"`
	SkipCostRubies       int                  `json:"skipCostRubies"`
	RequiredBuildingWIDs []int64              `json:"requiredBuildingWIDs,omitempty"`
	Costs                map[string]float64   `json:"costs"`
	Output               CraftingRecipeOutput `json:"output"`
}

type CraftingRecipeCost struct {
	Field      string  `json:"field"`
	JSONKey    string  `json:"jsonKey"`
	ResourceID int64   `json:"resourceId,omitempty"`
	CurrencyID int64   `json:"currencyId,omitempty"`
	Amount     float64 `json:"amount"`
}

type CraftingCatalog struct {
	Recipes      []CraftingRecipe            `json:"recipes"`
	Resources    map[string]CraftingResource `json:"resources"`
	ResourceByID map[int64]string            `json:"-"`
}

// CraftingRecipeCosts resolves the official cost fields to the resource or
// currency IDs used by live state. Recipe fields use names such as costGlass,
// while resource snapshots use compact JSON keys such as G.
func CraftingRecipeCosts(store *Store, recipeID int64) ([]CraftingRecipeCost, error) {
	if store == nil || recipeID <= 0 {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	recipes, err := store.Catalog("craftingRecipes")
	if err != nil {
		return nil, err
	}
	raw, exists := recipes.Find(strconv.FormatInt(recipeID, 10))
	if !exists {
		return nil, fmt.Errorf("crafting recipe %d is not in the official catalog", recipeID)
	}
	recipe, err := DecodeRecord(raw)
	if err != nil {
		return nil, fmt.Errorf("decode crafting recipe %d: %w", recipeID, err)
	}

	type costDefinition struct {
		jsonKey    string
		resourceID int64
		currencyID int64
	}
	definitions := map[string]costDefinition{}
	for _, collection := range []string{"resources", "currencies"} {
		catalog, catalogErr := store.Catalog(collection)
		if catalogErr != nil {
			continue
		}
		for _, rawDefinition := range catalog.Rows() {
			record, decodeErr := DecodeRecord(rawDefinition)
			if decodeErr != nil {
				continue
			}
			jsonKey, _ := record.String("JSONKey")
			nameField := "name"
			definition := costDefinition{jsonKey: jsonKey}
			if collection == "resources" {
				definition.resourceID, _ = record.Int64("resourceID")
			} else {
				nameField = "Name"
				definition.currencyID, _ = record.Int64("currencyID")
			}
			name, _ := record.String(nameField)
			assetName, _ := record.String("assetName")
			for _, alias := range []string{jsonKey, name, assetName} {
				alias = strings.ToLower(strings.TrimSpace(alias))
				if alias != "" {
					definitions[alias] = definition
				}
			}
		}
	}

	fields := make([]string, 0)
	for field := range recipe {
		if strings.HasPrefix(field, "cost") && field != "cost" {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	costs := make([]CraftingRecipeCost, 0, len(fields))
	for _, field := range fields {
		amount, valid := recipe.Float64(field)
		if !valid || amount <= 0 {
			continue
		}
		alias := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(field, "cost")))
		definition, found := definitions[alias]
		if !found || definition.resourceID <= 0 && definition.currencyID <= 0 {
			return nil, fmt.Errorf("crafting recipe %d cost field %s has no official resource mapping", recipeID, field)
		}
		costs = append(costs, CraftingRecipeCost{
			Field: field, JSONKey: definition.jsonKey, ResourceID: definition.resourceID,
			CurrencyID: definition.currencyID, Amount: amount,
		})
	}
	return costs, nil
}

func (manager *Manager) CraftingCatalog() (CraftingCatalog, error) {
	store, ready := manager.Current()
	if !ready {
		return CraftingCatalog{}, fmt.Errorf("official game data is unavailable")
	}
	resources, aliases, resourceByID, err := manager.craftingResources(store)
	if err != nil {
		return CraftingCatalog{}, err
	}
	rewards, err := manager.craftingRewards(store, resources, aliases)
	if err != nil {
		return CraftingCatalog{}, err
	}
	recipeCatalog, err := store.Catalog("craftingRecipes")
	if err != nil {
		return CraftingCatalog{}, err
	}
	recipes := make([]CraftingRecipe, 0, len(recipeCatalog.Rows()))
	for _, raw := range recipeCatalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		recipeID := intValue(record, "craftingRecipeId")
		queueTypeID := int(intValue(record, "queueTypeId"))
		if recipeID <= 0 || queueTypeID < 1 || queueTypeID > 4 {
			continue
		}
		recipe := CraftingRecipe{
			RecipeID: recipeID, QueueTypeID: queueTypeID,
			RecipeGroupID:   intValue(record, "recipeGroupID"),
			ResearchGroupID: intValue(record, "researchGroupID"),
			Level:           int(intValue(record, "level")), Type: stringValue(record, "type"),
			DurationSec:          int(intValue(record, "craftingDuration")),
			SkipCostRubies:       int(intValue(record, "skipCostC2")),
			RequiredBuildingWIDs: commaSeparatedInts(stringValue(record, "requiredCraftingBuildings")),
			Costs:                map[string]float64{},
		}
		for field := range record {
			if !strings.HasPrefix(field, "cost") || field == "cost" {
				continue
			}
			amount, ok := record.Float64(field)
			if !ok || amount <= 0 {
				continue
			}
			suffix := strings.TrimPrefix(field, "cost")
			key := craftingResourceKey(suffix, aliases)
			recipe.Costs[key] = amount
			if _, exists := resources[key]; !exists {
				resources[key] = CraftingResource{Key: key, Name: splitWords(suffix)}
			}
		}
		rewardID := firstScalarInt(record["rewardIDs"])
		recipe.Output = rewards[rewardID]
		if recipe.Output.RewardID == 0 {
			recipe.Output = CraftingRecipeOutput{RewardID: rewardID, Key: fmt.Sprintf("reward-%d", rewardID), Name: "Unknown reward"}
		}
		recipes = append(recipes, recipe)
	}
	sort.Slice(recipes, func(left, right int) bool { return recipes[left].RecipeID < recipes[right].RecipeID })
	return CraftingCatalog{Recipes: recipes, Resources: resources, ResourceByID: resourceByID}, nil
}

func (manager *Manager) craftingResources(store *Store) (map[string]CraftingResource, map[string]string, map[int64]string, error) {
	resources := map[string]CraftingResource{}
	aliases := map[string]string{"c1": "coins", "c2": "rubies"}
	resourceByID := map[int64]string{}
	for _, collection := range []string{"resources", "currencies"} {
		catalog, err := store.Catalog(collection)
		if err != nil {
			return nil, nil, nil, err
		}
		for _, raw := range catalog.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			idField := "resourceID"
			nameField := "name"
			if collection == "currencies" {
				idField = "currencyID"
				nameField = "Name"
			}
			id := intValue(record, idField)
			internalName := stringValue(record, nameField)
			jsonKey := stringValue(record, "JSONKey")
			if id <= 0 || internalName == "" {
				continue
			}
			key := lowerCamel(internalName)
			if jsonKey == "C1" {
				key = "coins"
			} else if jsonKey == "C2" {
				key = "rubies"
			}
			assetName := stringValue(record, "assetName")
			resource := CraftingResource{
				Key: key, Name: manager.craftingResourceName(record, internalName, jsonKey),
				JSONKey: jsonKey, AssetName: assetName, SourceID: id,
			}
			if assetName != "" {
				resource.IconURL = "/game-data/resources/images/" + assetName + ".webp"
			}
			resources[key] = resource
			aliases[strings.ToLower(internalName)] = key
			aliases[strings.ToLower(jsonKey)] = key
			if collection == "resources" {
				resourceByID[id] = key
			}
		}
	}
	return resources, aliases, resourceByID, nil
}

func (manager *Manager) craftingResourceName(record Record, internalName string, jsonKey string) string {
	if language, ready := manager.Language(); ready {
		keys := []string{
			"currency_name_" + lowerCamel(internalName),
			"currency_name_" + internalName,
		}
		if strings.EqualFold(jsonKey, "C2") {
			keys = append(keys, "subscription_rubies")
		}
		if displayName, ok := language.Resolve(keys...); ok {
			return displayName
		}
	}

	switch strings.ToUpper(strings.TrimSpace(jsonKey)) {
	case "C1":
		return "Coins"
	case "C2":
		return "Rubies"
	}

	fallback := splitWords(internalName)
	displayName := manager.displayName(record, fallback)
	if displayName == "" || displayName == internalName || displayName == jsonKey {
		return fallback
	}
	return displayName
}

func (manager *Manager) craftingRewards(store *Store, resources map[string]CraftingResource, aliases map[string]string) (map[int64]CraftingRecipeOutput, error) {
	catalog, err := store.Catalog("rewards")
	if err != nil {
		return nil, err
	}
	rewards := make(map[int64]CraftingRecipeOutput)
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		rewardID := intValue(record, "rewardID")
		if rewardID <= 0 {
			continue
		}
		for field := range record {
			if !strings.HasPrefix(field, "add") || field == "add" {
				continue
			}
			amount, ok := record.Float64(field)
			if !ok || amount <= 0 {
				continue
			}
			suffix := strings.TrimPrefix(field, "add")
			key := craftingResourceKey(suffix, aliases)
			resource, exists := resources[key]
			if !exists {
				resource = CraftingResource{Key: key, Name: splitWords(suffix)}
				resources[key] = resource
			}
			rewards[rewardID] = CraftingRecipeOutput{
				RewardID: rewardID, Key: key, Name: resource.Name, Amount: amount, IconURL: resource.IconURL,
			}
			break
		}
	}
	return rewards, nil
}

func craftingResourceKey(value string, aliases map[string]string) string {
	if key := aliases[strings.ToLower(strings.TrimSpace(value))]; key != "" {
		return key
	}
	return lowerCamel(value)
}

func firstScalarInt(raw json.RawMessage) int64 {
	if value, ok := scalarKey(raw); ok {
		parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '#' })
		if len(parts) > 0 {
			var record Record = map[string]json.RawMessage{"value": json.RawMessage(fmt.Sprintf("%q", strings.TrimSpace(parts[0])))}
			return intValue(record, "value")
		}
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) == nil && len(values) > 0 {
		if value, ok := scalarKey(values[0]); ok {
			var record Record = map[string]json.RawMessage{"value": json.RawMessage(fmt.Sprintf("%q", value))}
			return intValue(record, "value")
		}
	}
	return 0
}

func commaSeparatedInts(value string) []int64 {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == '#' })
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		var record Record = map[string]json.RawMessage{"value": json.RawMessage(fmt.Sprintf("%q", strings.TrimSpace(part)))}
		if parsed := intValue(record, "value"); parsed > 0 {
			result = append(result, parsed)
		}
	}
	return result
}

func lowerCamel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "unknown"
	}
	runes := []rune(value)
	runes[0] = unicode.ToLower(runes[0])
	return string(runes)
}

func splitWords(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "Unknown"
	}
	result := make([]rune, 0, len(value)+4)
	for index, current := range []rune(value) {
		if index > 0 && unicode.IsUpper(current) && unicode.IsLower([]rune(value)[index-1]) {
			result = append(result, ' ')
		}
		result = append(result, current)
	}
	result[0] = unicode.ToUpper(result[0])
	return string(result)
}

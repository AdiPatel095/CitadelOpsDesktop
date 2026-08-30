package GameData

import (
	"bytes"
	"encoding/json"
	"math"
	"sort"
	"strconv"
	"strings"
)

var constructionItemLegacyEffectFields = []string{
	"unitWallCount", "recruitSpeedBoost", "woodStorage", "stoneStorage", "ReduceResearchResourceCosts",
	"Stoneproduction", "Woodproduction", "Foodproduction", "foodStorage", "unboostedFoodProduction",
	"defensiveToolsSpeedBoost", "defensiveToolsCostsReduction", "meadStorage", "recruitCostReduction",
	"honeyStorage", "hospitalCapacity", "healSpeed", "marketCarriages", "XPBoostBuildBuildings",
	"stackSize", "glassStorage", "Glassproduction", "ironStorage", "Ironproduction", "coalStorage",
	"Coalproduction", "oilStorage", "Oilproduction", "offensiveToolsCostsReduction", "feastCostsReduction",
	"Meadreduction", "surviveBoost", "unboostedStoneProduction", "unboostedWoodProduction",
	"offensiveToolsSpeedBoost", "espionageTravelBoost", "decoPoints",
}

// ConstructionItemVariantKey separates selectable designs when the official
// data reuses a constructionItemGroupID across different names or effects.
func ConstructionItemVariantKey(record Record) string {
	name, _ := record.String("name")
	effectIDs := constructionItemEffectIDs(record)
	legacyFields := make([]string, 0)
	for _, field := range constructionItemLegacyEffectFields {
		if recordHasValue(record, field) {
			legacyFields = append(legacyFields, field)
		}
	}
	sort.Strings(legacyFields)
	appearance := positiveRecordInteger(record, "slotTypeID") == 0 && recordHasValue(record, "decoPoints")
	temporary := ConstructionItemIsTemporary(record)
	return strings.Join([]string{
		strings.TrimSpace(name),
		strings.Join(effectIDs, ","),
		strings.Join(legacyFields, ","),
		map[bool]string{true: "appearance", false: "normal"}[appearance],
		map[bool]string{true: "temporary", false: "permanent"}[temporary],
		recordScalarString(record, "slotTypeID"),
		recordScalarString(record, "constructionItemGroupID"),
	}, "|")
}

func ConstructionItemIsTemporary(record Record) bool {
	return positiveRecordInteger(record, "duration") > 0
}

func constructionItemEffectIDs(record Record) []string {
	effects, _ := record.String("effects")
	unique := map[int64]struct{}{}
	for _, effect := range strings.Split(effects, ",") {
		id := positiveStringInteger(strings.SplitN(strings.TrimSpace(effect), "&", 2)[0])
		if id > 0 {
			unique[id] = struct{}{}
		}
	}
	ids := make([]int64, 0, len(unique))
	for id := range unique {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	result := make([]string, 0, len(ids))
	for _, id := range ids {
		result = append(result, strconv.FormatInt(id, 10))
	}
	return result
}

func positiveRecordInteger(record Record, field string) int64 {
	return positiveStringInteger(recordScalarString(record, field))
}

func positiveStringInteger(value string) int64 {
	number, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
	if err != nil || math.IsNaN(number) || math.IsInf(number, 0) || number <= 0 {
		return 0
	}
	return int64(math.Floor(number))
}

func recordHasValue(record Record, field string) bool {
	raw, exists := record[field]
	if !exists {
		return false
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return false
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return false
	}
	return strings.TrimSpace(recordValueString(value)) != ""
}

func recordScalarString(record Record, field string) string {
	raw, exists := record[field]
	if !exists {
		return ""
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil || value == nil {
		return ""
	}
	return strings.TrimSpace(recordValueString(value))
}

func recordValueString(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case json.Number:
		number, err := typed.Float64()
		if err != nil {
			return ""
		}
		return strconv.FormatFloat(number, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(typed)
	default:
		return ""
	}
}

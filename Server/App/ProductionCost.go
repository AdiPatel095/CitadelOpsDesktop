package App

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

// Resolve every production field, including present malformed values. An absent
// cost is not a price; auxiliary healingCost/skipCost fields are not production.
func resolveProductionCosts(data *GameData.Store, language *GameData.LanguageStore, line int, unit int64, castle State.CastleID) ([]Automation.ProductionCost, error) {
	unknown := func(field string) ([]Automation.ProductionCost, error) {
		return nil, &Automation.ProductionCostBlock{Field: field}
	}
	if data == nil || unit <= 0 || (line != 0 && line != 1) {
		return unknown("definition")
	}
	catalog, err := data.Catalog("units")
	if err != nil {
		return unknown("definition")
	}
	raw, found := catalog.Find(strconv.FormatInt(unit, 10))
	if !found {
		return unknown("definition")
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return unknown("definition")
	}
	fields := []string{}
	for field := range record {
		suffix := strings.TrimPrefix(field, "cost")
		if suffix != field && len(suffix) > 0 && unicode.IsUpper([]rune(suffix)[0]) {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	costs := map[Intent.BalanceKey]Automation.ProductionCost{}
	for _, field := range fields {
		value, known := record.Float64(field)
		if !known || value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || value >= math.Exp2(63) {
			return unknown(field)
		}
		if field == "costC2" {
			if value > 0 {
				return nil, &Automation.ProductionCostBlock{Rubies: true}
			}
			continue // Ruby costs never become balance keys or spending authority.
		}
		suffix := strings.TrimPrefix(field, "cost")
		collection, lookup, idField := "currencies", "Name", "currencyID"
		if field == "costC1" {
			collection, lookup, idField = "resources", "JSONKey", "resourceID"
		}
		if field == "costWood" || field == "costStone" {
			collection, lookup, idField = "resources", "name", "resourceID"
		}
		balances, err := data.Catalog(collection)
		if err != nil {
			return unknown(field)
		}
		var id int64
		var name string
		for _, row := range balances.Rows() {
			balance, err := GameData.DecodeRecord(row)
			if err != nil {
				continue
			}
			candidate, _ := balance.String(lookup)
			if !strings.EqualFold(candidate, suffix) {
				continue
			}
			candidateID, ok := balance.Int64(idField)
			if !ok || candidateID <= 0 || id != 0 {
				return unknown(field)
			}
			id = candidateID
			name = candidate
		}
		if id <= 0 {
			return unknown(field)
		}
		if value == 0 {
			continue
		}
		key := Intent.CurrencyBalanceKey(State.CurrencyID(id))
		if field == "costC1" {
			key = Intent.PlayerResourceBalanceKey(State.ResourceID(id))
		}
		if field == "costWood" || field == "costStone" {
			key = Intent.CastleResourceBalanceKey(castle, State.ResourceID(id))
		}
		nameKey := data.DefinitionNameKey(language, collection, id)
		if language != nil {
			if text, ok := language.Text(nameKey); ok {
				name = text
			}
		}
		cost := Automation.ProductionCost{Key: key, PerUnit: value, Name: name, NameKey: nameKey}
		if prior, exists := costs[key]; exists {
			cost.PerUnit += prior.PerUnit
		}
		if math.IsInf(cost.PerUnit, 0) || cost.PerUnit >= math.Exp2(63) {
			return unknown(field)
		}
		costs[key] = cost
	}
	result := make([]Automation.ProductionCost, 0, len(costs))
	for _, cost := range costs {
		result = append(result, cost)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Key.String() < result[j].Key.String() })
	return result, nil
}

func productionCostError(input Intent.PlanningContext, unit int64, err error) error {
	block, ok := err.(*Automation.ProductionCostBlock)
	if !ok {
		return err
	}
	detail, message := Automation.ProductionCostStatus(Automation.Snapshot{State: input.State, GameData: input.GameData, Language: input.Language}, unit, block)
	return Localization.WithError(fmt.Errorf("%s", detail), message)
}

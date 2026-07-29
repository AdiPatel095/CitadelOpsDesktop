package App

import (
	"fmt"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
)

func productionDefinitionLabel(
	store *GameData.Store,
	language *GameData.LanguageStore,
	collection string,
	id int64,
) string {
	catalogName := collection
	kind := "unit"
	includeLevel := collection == "units"
	if collection == "tools" {
		catalogName = "units"
		kind = "tool"
	}
	fallback := fmt.Sprintf("%s %d", kind, id)
	if store == nil || id <= 0 {
		return fallback
	}
	catalog, err := store.Catalog(catalogName)
	if err != nil {
		return fallback
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return fallback
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return fallback
	}
	label := fallback
	if name, found := officialLocalizedRecordName(record, language); found {
		label = name
	}
	if includeLevel {
		if level, found := record.Int64("level"); found && level > 0 {
			label = fmt.Sprintf("%s (level %d)", label, level)
		}
	}
	return label
}

func officialLocalizedRecordName(record GameData.Record, language *GameData.LanguageStore) (string, bool) {
	if displayName, found := record.String("_display_name"); found && strings.TrimSpace(displayName) != "" {
		return strings.TrimSpace(displayName), true
	}
	if language == nil {
		return "", false
	}
	internalNames := []string{}
	for _, field := range []string{"type", "name", "Name", "JSONKey"} {
		if value, found := record.String(field); found && strings.TrimSpace(value) != "" {
			internalNames = append(internalNames, strings.TrimSpace(value))
		}
	}
	keys := make([]string, 0, len(internalNames)*2)
	for _, name := range internalNames {
		keys = append(keys, name+"_name", name)
	}
	name, found := language.Resolve(keys...)
	return strings.TrimSpace(name), found
}

func officialTimeSkipLabel(store *GameData.Store, currencyID int64, wireKey string) string {
	minutes := officialTimeSkipMinutes(store, currencyID, wireKey)
	if minutes <= 0 {
		return "time skip"
	}
	if minutes%60 == 0 {
		return fmt.Sprintf("%d-hour time skip", minutes/60)
	}
	return fmt.Sprintf("%d-minute time skip", minutes)
}

func officialTimeSkipMinutes(store *GameData.Store, currencyID int64, wireKey string) int64 {
	minutes := int64(0)
	if store != nil && currencyID > 0 {
		if catalog, err := store.Catalog("currencyMinutesSkipValues"); err == nil {
			if raw, found := catalog.FindByField("currencyID", strconv.FormatInt(currencyID, 10)); found {
				if record, decodeErr := GameData.DecodeRecord(raw); decodeErr == nil {
					minutes, _ = record.Int64("MinutesSkipValue")
				}
			}
		}
	}
	if minutes <= 0 {
		minutes = map[string]int64{
			"MS1": 1, "MS2": 5, "MS3": 10, "MS4": 30,
			"MS5": 60, "MS6": 300, "MS7": 1440,
		}[strings.ToUpper(strings.TrimSpace(wireKey))]
	}
	return minutes
}

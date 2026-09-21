package GameData

import (
	"strconv"
	"strings"
)

// FirstOfficialNameKey selects a key from source-defined candidates, never by
// reverse matching a rendered English label. The runtime language is immutable.
func FirstOfficialNameKey(language *LanguageStore, keys ...string) string {
	if language == nil {
		return ""
	}
	for _, key := range keys {
		if text, ok := language.Resolve(key); ok && strings.TrimSpace(text) != "" {
			return key
		}
	}
	return ""
}

// DefinitionNameKey follows the official item identity to its name keys. It is
// intentionally limited to atomic nouns, not composed package/bundle labels.
func (store *Store) DefinitionNameKey(language *LanguageStore, collection string, id int64) string {
	if store == nil || id <= 0 {
		return ""
	}
	switch collection {
	case "units", "buildings", "resources", "currencies", "constructionItems", "equipments", "gems":
	default:
		return ""
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return ""
	}
	raw, ok := catalog.Find(strconv.FormatInt(id, 10))
	if !ok {
		return ""
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return ""
	}
	keys := []string{}
	switch collection {
	case "units":
		for _, field := range []string{"type", "comment2", "name"} {
			if v := stringValue(record, field); v != "" {
				keys = append(keys, v+"_name", v)
			}
		}
	case "buildings":
		keys = append(keys, buildingLocalizationKeys(record)...)
	case "equipments":
		keys = append(keys, "equipment_unique_"+strconv.FormatInt(id, 10), "hero_unique_"+strconv.FormatInt(id, 10))
	case "gems":
		keys = append(keys, "gem_unique_"+strconv.FormatInt(id, 10))
	case "constructionItems":
		name := stringValue(record, "name")
		if name != "" {
			prefixes := []string{"ci_primary_", "ci_secondary_", "ci_appearance_", "ci_blueprint_"}
			comment := strings.ToLower(stringValue(record, "comment1"))
			if strings.Contains(comment, "appearance") || strings.Contains(comment, "temporary") {
				prefixes = []string{"ci_appearance_", "ci_primary_", "ci_secondary_", "ci_blueprint_"}
			} else if strings.Contains(comment, "secondary") {
				prefixes = []string{"ci_secondary_", "ci_primary_", "ci_appearance_", "ci_blueprint_"}
			} else if strings.Contains(comment, "blueprint") {
				prefixes = []string{"ci_blueprint_", "ci_primary_", "ci_secondary_", "ci_appearance_"}
			}
			for _, prefix := range prefixes {
				keys = append(keys, prefix+name)
			}
		}
	}
	fields := []string{"name", "Name", "type", "JSONKey", "comment1", "comment2"}
	if collection == "currencies" {
		fields = []string{"Name", "name", "assetName", "JSONKey"}
	}
	for _, field := range fields {
		if v := stringValue(record, field); v != "" {
			keys = append(keys, v+"_name", "currency_name_"+v, v)
		}
	}
	return FirstOfficialNameKey(language, keys...)
}

// FeastNameKey mirrors PremiumFestivalItemVO.nameTextID in the official client:
// "dialog_festival_" + the source feast type + "Event".
func FeastNameKey(language *LanguageStore, feast AutoBuyerFeast) string {
	return FirstOfficialNameKey(language, "dialog_festival_"+feast.Type+"Event")
}

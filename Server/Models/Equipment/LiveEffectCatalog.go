package equipment

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

type EquipmentExtraStat struct {
	Key      string  `json:"key"`
	EffectID float64 `json:"effectId"`
	Name     string  `json:"name"`
	Label    string  `json:"label"`
	Category string  `json:"category"`
	Unit     string  `json:"unit"`
	Value    float64 `json:"value"`
}

type EquipmentEffect struct {
	Key           string              `json:"key"`
	RawEffectID   float64             `json:"rawEffectId"`
	EffectID      float64             `json:"effectId"`
	Name          string              `json:"name"`
	Label         string              `json:"label"`
	Category      string              `json:"category"`
	EffectTypeID  float64             `json:"effectTypeId"`
	SortCategory  string              `json:"sortCategory,omitempty"`
	SortGroup     string              `json:"sortGroup,omitempty"`
	CategoryLabel string              `json:"categoryLabel,omitempty"`
	GroupLabel    string              `json:"groupLabel,omitempty"`
	Unit          string              `json:"unit"`
	Scope         string              `json:"scope"`
	Source        CatalogEffectSource `json:"source"`
	Value         float64             `json:"value"`
	ArgumentID    float64             `json:"argumentId,omitempty"`
	ArgumentLabel string              `json:"argumentLabel,omitempty"`
	CapID         float64             `json:"capId,omitempty"`
	MaxTotalBonus *float64            `json:"maxTotalBonus,omitempty"`
	SortOrder     string              `json:"sortOrder,omitempty"`
}

type liveEffectDefinition struct {
	EffectID      int64
	Name          string
	TypeID        int64
	CapID         int64
	SortOrder     string
	Template      string
	IsPercent     bool
	IsPVP         bool
	IsPVE         bool
	SortCategory  string
	SortGroup     string
	CategoryLabel string
	GroupLabel    string
}

type liveEffectTypeDefinition struct {
	EffectTypeID int64
	SortCategory string
	SortGroup    string
	Name         string
}

type liveResolvedEffect struct {
	ID         int64
	RawID      int64
	Definition liveEffectDefinition
	HasDef     bool
}

type liveCatalogEffect struct {
	ID     int64
	Values []float64
}

type liveEquipmentSetEffect struct {
	Needed  int
	Effects []liveCatalogEffect
}

type liveEffectDataCache struct {
	EquipmentEffectIDs     map[int64]int64
	EquipmentEffectWearers map[int64]int64
	RelicEffectIDs         map[int64]int64
	EffectDefinitions      map[int64]liveEffectDefinition
	EffectTypes            map[int64]liveEffectTypeDefinition
	GemEffects             map[int64][]liveCatalogEffect
	GemSetIDs              map[int64]int64
	EquipmentSetEffects    map[int64][]liveEquipmentSetEffect
	EffectCaps             map[int64]float64
	UnitNames              map[int64]string
	Lang                   map[string]string
}

var (
	liveEffectDataOnce sync.Once
	liveEffectData     liveEffectDataCache
)

var liveLangPlaceholderPattern = regexp.MustCompile(`[+\-]?\s*\{[0-9]+\}\s*%?`)
var liveNegativeValuePattern = regexp.MustCompile(`-\s*\{0\}|\{0\}\s*-`)
var livePositiveValuePattern = regexp.MustCompile(`\+\s*\{0\}|\{0\}\s*\+`)

func ApplyCommanderSetBonusStats(dst *CommStatModel, setID float64, equippedCount int) {
	if dst == nil || setID <= 0 || equippedCount <= 0 {
		return
	}
	for _, effect := range liveEquipmentSetEffects(int64(setID), equippedCount) {
		ApplyCommanderLiveStat(dst, float64(effect.ID), effect.Values, CatalogEffectSourceSetBonus)
	}
}

func ApplyCastellanSetBonusStats(dst *CastStatModel, setID float64, equippedCount int) {
	if dst == nil || setID <= 0 || equippedCount <= 0 {
		return
	}
	for _, effect := range liveEquipmentSetEffects(int64(setID), equippedCount) {
		ApplyCastellanLiveStat(dst, float64(effect.ID), effect.Values, CatalogEffectSourceSetBonus)
	}
}

func ApplyCommanderCatalogGemStats(dst *CommStatModel, gemID float64) {
	if dst == nil || gemID <= 0 {
		return
	}
	for _, effect := range loadLiveEffectData().GemEffects[int64(gemID)] {
		ApplyCommanderLiveStat(dst, float64(effect.ID), effect.Values, CatalogEffectSourceGem)
	}
}

func ApplyCastellanCatalogGemStats(dst *CastStatModel, gemID float64) {
	if dst == nil || gemID <= 0 {
		return
	}
	for _, effect := range loadLiveEffectData().GemEffects[int64(gemID)] {
		ApplyCastellanLiveStat(dst, float64(effect.ID), effect.Values, CatalogEffectSourceGem)
	}
}

func CatalogGemSetID(gemID float64) float64 {
	if gemID <= 0 {
		return 0
	}
	return float64(loadLiveEffectData().GemSetIDs[int64(gemID)])
}

func MergeEquipmentExtraStats(existing []EquipmentExtraStat, groups ...[]EquipmentExtraStat) []EquipmentExtraStat {
	merged := make([]EquipmentExtraStat, 0, len(existing))
	byKey := make(map[string]int)

	add := func(stat EquipmentExtraStat) {
		if stat.Key == "" {
			stat.Key = fmt.Sprintf("%.0f", stat.EffectID)
		}
		if idx, ok := byKey[stat.Key]; ok {
			merged[idx].Value += stat.Value
			return
		}
		byKey[stat.Key] = len(merged)
		merged = append(merged, stat)
	}

	for _, stat := range existing {
		add(stat)
	}
	for _, group := range groups {
		for _, stat := range group {
			add(stat)
		}
	}

	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].Category != merged[j].Category {
			return merged[i].Category < merged[j].Category
		}
		return merged[i].Label < merged[j].Label
	})
	return merged
}

func MergeEquipmentEffects(existing []EquipmentEffect, groups ...[]EquipmentEffect) []EquipmentEffect {
	merged := make([]EquipmentEffect, 0, len(existing))
	byKey := make(map[string]int)
	add := func(effect EquipmentEffect) {
		if idx, ok := byKey[effect.Key]; ok {
			merged[idx].Value += effect.Value
			return
		}
		byKey[effect.Key] = len(merged)
		merged = append(merged, effect)
	}
	for _, effect := range existing {
		add(effect)
	}
	for _, group := range groups {
		for _, effect := range group {
			add(effect)
		}
	}
	sort.SliceStable(merged, func(i, j int) bool {
		if merged[i].SortOrder != merged[j].SortOrder {
			return compareLiveSortOrder(merged[i].SortOrder, merged[j].SortOrder) < 0
		}
		return merged[i].Label < merged[j].Label
	})
	return merged
}

func resolveLiveCatalogEffect(rawID float64, source CatalogEffectSource, wearerID int64) (liveResolvedEffect, bool) {
	raw := int64(rawID)
	if raw <= 0 {
		return liveResolvedEffect{}, false
	}
	data := loadLiveEffectData()
	equipmentMappedID := data.EquipmentEffectIDs[raw]
	equipmentWearerID := data.EquipmentEffectWearers[raw]
	equipmentMappingMatches := equipmentMappedID > 0 && (wearerID == 0 || equipmentWearerID == 0 || equipmentWearerID == wearerID)

	resolve := func(id int64) (liveResolvedEffect, bool) {
		def, ok := data.EffectDefinitions[id]
		if !ok {
			return liveResolvedEffect{ID: id, RawID: raw}, false
		}
		return liveResolvedEffect{ID: id, RawID: raw, Definition: def, HasDef: true}, true
	}

	switch source {
	case CatalogEffectSourceRelicEquipment, CatalogEffectSourceRelicGem:
		if mapped := data.RelicEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
		if effect, ok := resolve(raw); ok {
			return effect, true
		}
		if equipmentMappingMatches {
			if effect, ok := resolve(equipmentMappedID); ok {
				return effect, true
			}
		}
	case CatalogEffectSourceGem:
		if effect, ok := resolve(raw); ok {
			return effect, true
		}
		if equipmentMappingMatches {
			if effect, ok := resolve(equipmentMappedID); ok {
				return effect, true
			}
		}
		if mapped := data.RelicEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
	default:
		if equipmentMappingMatches {
			if effect, ok := resolve(equipmentMappedID); ok {
				return effect, true
			}
		}
		if equipmentMappedID > 0 && !equipmentMappingMatches {
			return liveResolvedEffect{ID: raw, RawID: raw}, false
		}
		if effect, ok := resolve(raw); ok {
			return effect, true
		}
		if mapped := data.RelicEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
	}

	return liveResolvedEffect{ID: raw, RawID: raw}, false
}

func liveEquipmentSetEffects(setID int64, equippedCount int) []liveCatalogEffect {
	if setID <= 0 || equippedCount <= 0 {
		return nil
	}
	var out []liveCatalogEffect
	for _, entry := range loadLiveEffectData().EquipmentSetEffects[setID] {
		if entry.Needed <= equippedCount {
			out = append(out, entry.Effects...)
		}
	}
	return out
}

func loadLiveEffectData() liveEffectDataCache {
	liveEffectDataOnce.Do(func() {
		liveEffectData = buildLiveEffectData()
	})
	return liveEffectData
}

func buildLiveEffectData() liveEffectDataCache {
	data := liveEffectDataCache{
		EquipmentEffectIDs:     map[int64]int64{},
		EquipmentEffectWearers: map[int64]int64{},
		RelicEffectIDs:         map[int64]int64{},
		EffectDefinitions:      map[int64]liveEffectDefinition{},
		EffectTypes:            map[int64]liveEffectTypeDefinition{},
		GemEffects:             map[int64][]liveCatalogEffect{},
		GemSetIDs:              map[int64]int64{},
		EquipmentSetEffects:    map[int64][]liveEquipmentSetEffect{},
		EffectCaps:             map[int64]float64{},
		UnitNames:              map[int64]string{},
		Lang:                   readLiveLang(),
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEffectCapsItemsJSON) {
		capID := liveIntFromValue(entry["capID"])
		if capID > 0 {
			data.EffectCaps[capID] = liveFloatFromValue(entry["maxTotalBonus"])
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEffectTypesItemsJSON) {
		typeID := liveIntFromValue(entry["effectTypeID"])
		data.EffectTypes[typeID] = liveEffectTypeDefinition{
			EffectTypeID: typeID,
			SortCategory: liveStringFromValue(entry["sortCategory"]),
			SortGroup:    liveStringFromValue(entry["sortGroup"]),
			Name:         liveStringFromValue(entry["name"]),
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadTroopsJSON) {
		unitID := liveIntFromValue(entry["wodID"])
		unitType := liveStringFromValue(entry["type"])
		if unitID <= 0 || unitType == "" {
			continue
		}
		data.UnitNames[unitID] = liveLangText(data.Lang, unitType+"_name", unitType)
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEffectsItemsJSON) {
		effectID := liveIntFromValue(entry["effectID"])
		if effectID == 0 {
			continue
		}
		name := liveStringFromValue(entry["name"])
		lowerName := strings.ToLower(name)
		definition := liveEffectDefinition{
			EffectID:  effectID,
			Name:      name,
			TypeID:    liveIntFromValue(entry["effectTypeID"]),
			CapID:     liveIntFromValue(entry["capID"]),
			SortOrder: liveStringFromValue(entry["sortOrder"]),
			IsPVP:     liveStringFromValue(entry["isPvPFight"]) == "1" || strings.Contains(lowerName, "pvp"),
			IsPVE:     liveStringFromValue(entry["isPvEFight"]) == "1" || strings.Contains(lowerName, "pve"),
		}
		definition.Template = liveEffectTemplate(data.Lang, definition)
		definition.IsPercent = liveEffectIsPercent(definition)
		if effectType, ok := data.EffectTypes[definition.TypeID]; ok {
			definition.SortCategory = effectType.SortCategory
			definition.SortGroup = effectType.SortGroup
			definition.CategoryLabel = liveEffectCategoryLabel(data.Lang, effectType)
			definition.GroupLabel = liveEffectGroupLabel(data.Lang, effectType)
		}
		data.EffectDefinitions[effectID] = definition
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEquipmentEffectsItemsJSON) {
		equipmentEffectID := liveIntFromValue(entry["equipmentEffectID"])
		effectID := liveIntFromValue(entry["effectID"])
		if equipmentEffectID > 0 && effectID > 0 {
			data.EquipmentEffectIDs[equipmentEffectID] = effectID
			data.EquipmentEffectWearers[equipmentEffectID] = liveIntFromValue(entry["wearerID"])
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadRelicEffectsItemsJSON) {
		relicEffectID := liveIntFromValue(entry["id"])
		effectID := liveIntFromValue(entry["effectID"])
		if relicEffectID > 0 && effectID > 0 {
			data.RelicEffectIDs[relicEffectID] = effectID
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadGemsItemsJSON) {
		gemID := liveIntFromValue(entry["gemID"])
		if gemID <= 0 {
			continue
		}
		if effects := liveCatalogEffectsFromString(liveStringFromValue(entry["effects"])); len(effects) > 0 {
			data.GemEffects[gemID] = effects
		}
		if setID := liveIntFromValue(entry["setID"]); setID > 0 {
			data.GemSetIDs[gemID] = setID
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEquipmentSetsItemsJSON) {
		setID := liveIntFromValue(entry["setID"])
		needed := int(liveIntFromValue(entry["neededItems"]))
		effects := liveCatalogEffectsFromString(liveStringFromValue(entry["effects"]))
		if setID > 0 && needed > 0 && len(effects) > 0 {
			data.EquipmentSetEffects[setID] = append(data.EquipmentSetEffects[setID], liveEquipmentSetEffect{
				Needed:  needed,
				Effects: effects,
			})
		}
	}

	return data
}

func readLiveDataArray(read func() ([]byte, error)) []map[string]interface{} {
	raw, err := read()
	if err != nil {
		return nil
	}
	var rows []map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil {
		return nil
	}
	return rows
}

func readLiveLang() map[string]string {
	raw, err := serverdata.ReadLangEnJSON()
	if err != nil {
		return nil
	}
	var rows map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&rows); err != nil {
		return nil
	}
	out := make(map[string]string, len(rows))
	for key, value := range rows {
		if text, ok := value.(string); ok {
			out[key] = text
			out[strings.ToLower(key)] = text
		}
	}
	return out
}

func liveCatalogEffectsFromString(value string) []liveCatalogEffect {
	if value == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]liveCatalogEffect, 0, len(parts))
	for _, part := range parts {
		pair := strings.SplitN(strings.TrimSpace(part), "&", 2)
		id := liveIntFromString(pair[0])
		if id <= 0 || len(pair) < 2 {
			continue
		}
		values := liveEffectValuesFromToken(pair[1])
		if len(values) == 0 {
			continue
		}
		out = append(out, liveCatalogEffect{ID: id, Values: values})
	}
	return out
}

func liveEffectValuesFromToken(token string) []float64 {
	var out []float64
	for _, segment := range strings.Split(token, "#") {
		segment = strings.TrimSpace(segment)
		if segment == "" {
			continue
		}
		if strings.Contains(segment, "+") {
			pair := strings.SplitN(segment, "+", 2)
			arg := liveFloatFromString(strings.TrimSpace(pair[0]))
			value := liveFloatFromString(strings.TrimSpace(pair[1]))
			if arg != 0 {
				out = append(out, arg)
			}
			if value != 0 {
				out = append(out, value)
			}
			continue
		}
		value := liveFloatFromString(segment)
		if value != 0 {
			out = append(out, value)
		}
	}
	return out
}

func liveCatalogEffectValue(effect liveResolvedEffect, values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	if effect.HasDef && (effect.Definition.TypeID == 148 || strings.Contains(strings.ToLower(effect.Definition.Name), "attackbonusunit")) {
		return valueAt(values, 1)
	}
	if len(values) > 1 && len(values)%2 == 0 && values[0] > 100 {
		var sum float64
		for i := 1; i < len(values); i += 2 {
			sum += values[i]
		}
		if sum != 0 {
			return sum
		}
	}
	return values[0]
}

func liveEffectScope(effect liveResolvedEffect) string {
	if !effect.HasDef {
		return "generic"
	}
	name := strings.ToLower(effect.Definition.Name)
	switch {
	case effect.Definition.IsPVP || strings.Contains(name, "pvp"):
		return "pvp"
	case effect.Definition.IsPVE ||
		strings.Contains(name, "pve") ||
		strings.Contains(name, "nomad") ||
		strings.Contains(name, "samurai") ||
		strings.Contains(name, "alien") ||
		strings.Contains(name, "bloodcrow") ||
		strings.Contains(name, "berimond") ||
		strings.Contains(name, "daimyo") ||
		strings.Contains(name, "khan"):
		return "pve"
	default:
		return "generic"
	}
}

func addEquipmentExtraStat(stats *[]EquipmentExtraStat, effect liveResolvedEffect, values []float64) {
	if stats == nil || !effect.HasDef {
		return
	}
	value := liveCatalogEffectValue(effect, values)
	if value == 0 {
		return
	}
	label := liveEffectLabel(effect.Definition)
	extra := EquipmentExtraStat{
		Key:      fmt.Sprintf("%d", effect.ID),
		EffectID: float64(effect.ID),
		Name:     effect.Definition.Name,
		Label:    label,
		Category: liveEffectCategory(effect.Definition),
		Unit:     liveEffectUnit(effect.Definition),
		Value:    value,
	}
	*stats = MergeEquipmentExtraStats(*stats, []EquipmentExtraStat{extra})
}

func addEquipmentEffects(stats *[]EquipmentEffect, effect liveResolvedEffect, values []float64, source CatalogEffectSource) {
	if stats == nil || !effect.HasDef {
		return
	}
	definition := effect.Definition
	data := loadLiveEffectData()
	add := func(value float64, argumentID int64) {
		if value == 0 {
			return
		}
		value = liveEffectSemanticValue(definition, value)
		argumentLabel := data.UnitNames[argumentID]
		if argumentID > 0 && argumentLabel == "" {
			argumentLabel = strconv.FormatInt(argumentID, 10)
		}
		label := liveEffectLabel(definition)
		if argumentLabel != "" {
			label = strings.ReplaceAll(label, "{1}", argumentLabel)
		}
		label = cleanLiveLangLabel(label)
		key := fmt.Sprintf("%d:%d:%s", effect.ID, argumentID, source)
		resolved := EquipmentEffect{
			Key:           key,
			RawEffectID:   float64(effect.RawID),
			EffectID:      float64(effect.ID),
			Name:          definition.Name,
			Label:         label,
			Category:      liveEffectCategory(definition),
			EffectTypeID:  float64(definition.TypeID),
			SortCategory:  definition.SortCategory,
			SortGroup:     definition.SortGroup,
			CategoryLabel: definition.CategoryLabel,
			GroupLabel:    definition.GroupLabel,
			Unit:          liveEffectUnit(definition),
			Scope:         liveEffectScope(effect),
			Source:        source,
			Value:         value,
			ArgumentID:    float64(argumentID),
			ArgumentLabel: argumentLabel,
			CapID:         float64(definition.CapID),
			SortOrder:     definition.SortOrder,
		}
		if capValue, ok := data.EffectCaps[definition.CapID]; ok {
			resolved.MaxTotalBonus = &capValue
		}
		*stats = MergeEquipmentEffects(*stats, []EquipmentEffect{resolved})
	}

	if strings.Contains(definition.Template, "{1}") && len(values) >= 2 && len(values)%2 == 0 {
		for i := 0; i < len(values); i += 2 {
			add(values[i+1], int64(values[i]))
		}
		return
	}
	add(liveCatalogEffectValue(effect, values), 0)
}

func liveEffectSemanticValue(def liveEffectDefinition, value float64) float64 {
	switch {
	case liveNegativeValuePattern.MatchString(def.Template):
		return -math.Abs(value)
	case livePositiveValuePattern.MatchString(def.Template):
		return math.Abs(value)
	default:
		return value
	}
}

func liveEffectLabel(def liveEffectDefinition) string {
	if strings.TrimSpace(def.Template) != "" {
		return def.Template
	}
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return fmt.Sprintf("Effect %d", def.EffectID)
	}
	if label := officialLiveEffectLabel(name); label != "" {
		return label
	}
	for _, prefix := range []string{"relic", "newPVP", "equipmentARE"} {
		name = strings.TrimPrefix(name, prefix)
	}
	var b strings.Builder
	var prev rune
	for _, current := range name {
		if current == '_' || current == '-' {
			b.WriteByte(' ')
			prev = ' '
			continue
		}
		if prev != 0 && prev != ' ' && ((prev >= 'a' && prev <= 'z') || (prev >= '0' && prev <= '9')) && current >= 'A' && current <= 'Z' {
			b.WriteByte(' ')
		}
		b.WriteRune(current)
		prev = current
	}
	words := strings.Fields(b.String())
	for i, word := range words {
		switch strings.ToLower(word) {
		case "pvp":
			words[i] = "PvP"
		case "pve":
			words[i] = "PvE"
		case "are":
			words[i] = "ARE"
		default:
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}
	if len(words) == 0 {
		return fmt.Sprintf("Effect %d", def.EffectID)
	}
	return strings.Join(words, " ")
}

func liveEffectTemplate(lang map[string]string, def liveEffectDefinition) string {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return ""
	}
	key := strings.ToLower(name)
	for _, candidate := range []string{
		"equip_effect_description_" + key,
		"ci_effect_" + key,
		"effect_name_" + key,
		"effect_desc_" + key,
		"equip_effect_description_short_" + key,
		key,
	} {
		value := liveLangText(lang, candidate, "")
		if value == "" || strings.Contains(strings.ToLower(value), "lost its powers") || strings.Contains(strings.ToLower(value), "seems to have run out") {
			continue
		}
		return value
	}
	return ""
}

func liveEffectIsPercent(def liveEffectDefinition) bool {
	template := def.Template
	isPercent := strings.Contains(template, "%") || strings.Contains(strings.ToLower(def.Name), "boost")
	return isPercent && !strings.Contains(strings.ToLower(def.Name), "unboosted")
}

func liveLangText(lang map[string]string, key string, fallback string) string {
	if value := strings.TrimSpace(lang[key]); value != "" {
		return value
	}
	if value := strings.TrimSpace(lang[strings.ToLower(key)]); value != "" {
		return value
	}
	return fallback
}

func officialLiveEffectLabel(name string) string {
	lang := loadLiveEffectData().Lang
	if len(lang) == 0 || strings.TrimSpace(name) == "" {
		return ""
	}
	for _, key := range []string{
		"effect_name_" + name,
		"equip_effect_description_short_" + name,
		"equip_effect_description_" + name,
		"relicequip_effect_description_" + name + "_undefined",
		"relicequip_effect_description_" + name,
		"effect_description_" + name,
		"ci_effect_" + name,
	} {
		if label := cleanLiveLangLabel(lang[key]); label != "" {
			return label
		}
		if label := cleanLiveLangLabel(lang[strings.ToLower(key)]); label != "" {
			return label
		}
	}
	return ""
}

func cleanLiveLangLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = strings.NewReplacer(
		"↵", " ",
		"\n", " ",
		"\\n", " ",
		"\t", " ",
	).Replace(value)
	value = liveLangPlaceholderPattern.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	value = strings.Trim(value, " +-:.,")
	if len(value) > 3 && strings.HasPrefix(strings.ToLower(value), "to ") {
		value = strings.TrimSpace(value[3:])
	}
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

func liveEffectCategory(def liveEffectDefinition) string {
	if def.CategoryLabel != "" {
		return def.CategoryLabel
	}
	name := strings.ToLower(def.Name)
	switch {
	case def.TypeID == 23 || def.TypeID == 24 || def.TypeID == 33 || def.TypeID == 36 || def.TypeID == 53 || def.TypeID == 54 || def.TypeID == 148:
		return "Unit Stats"
	case def.TypeID == 19 || def.TypeID == 20 || def.TypeID == 21 || def.TypeID == 6 || def.TypeID == 7 || def.TypeID == 8:
		return "Wall, Gate, and Moat"
	case def.TypeID == 28 || def.TypeID == 34 || def.TypeID == 12 || def.TypeID == 46 || def.TypeID == 51 || def.TypeID == 47 || def.TypeID == 156:
		return "Capacity Stats"
	case strings.Contains(name, "production") || strings.Contains(name, "resource") || strings.Contains(name, "research") || strings.Contains(name, "market") || strings.Contains(name, "storage") || strings.Contains(name, "capacity"):
		return "Economy Stats"
	case strings.Contains(name, "nomad") || strings.Contains(name, "samurai") || strings.Contains(name, "alien") || strings.Contains(name, "berimond") || strings.Contains(name, "khan") || strings.Contains(name, "daimyo"):
		return "Event Stats"
	default:
		return "Other Stats"
	}
}

func liveEffectCategoryLabel(lang map[string]string, effectType liveEffectTypeDefinition) string {
	if effectType.SortCategory == "" {
		return effectType.Name
	}
	return liveLangText(lang, "effect_category_"+effectType.SortCategory, effectType.Name)
}

func liveEffectGroupLabel(lang map[string]string, effectType liveEffectTypeDefinition) string {
	if effectType.SortCategory == "" || effectType.SortGroup == "" {
		return effectType.Name
	}
	prefix := "effect_group_" + effectType.SortCategory + "_" + effectType.SortGroup
	for _, suffix := range []string{"_passive", "_active"} {
		if label := cleanLiveLangLabel(liveLangText(lang, prefix+suffix, "")); label != "" {
			return label
		}
	}
	return effectType.Name
}

func liveEffectUnit(def liveEffectDefinition) string {
	if def.IsPercent {
		return "percent"
	}
	switch def.TypeID {
	case 47, 51, 148, 150, 156, 179, 181:
		return "number"
	default:
		return "percent"
	}
}

func compareLiveSortOrder(left string, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	length := len(leftParts)
	if len(rightParts) > length {
		length = len(rightParts)
	}
	for i := 0; i < length; i++ {
		var leftValue, rightValue int64
		if i < len(leftParts) {
			leftValue = liveIntFromString(leftParts[i])
		}
		if i < len(rightParts) {
			rightValue = liveIntFromString(rightParts[i])
		}
		if leftValue < rightValue {
			return -1
		}
		if leftValue > rightValue {
			return 1
		}
	}
	return 0
}

func liveIntFromValue(v interface{}) int64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Int64()
		return n
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case string:
		return liveIntFromString(x)
	default:
		return 0
	}
}

func liveIntFromString(value string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return n
}

func liveFloatFromString(value string) float64 {
	n, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return n
}

func liveFloatFromValue(v interface{}) float64 {
	switch x := v.(type) {
	case json.Number:
		n, _ := x.Float64()
		return n
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case string:
		return liveFloatFromString(x)
	default:
		return 0
	}
}

func liveStringFromValue(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case nil:
		return ""
	default:
		return fmt.Sprint(x)
	}
}

package equipment

import (
	"bytes"
	"encoding/json"
	"fmt"
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

type liveEffectDefinition struct {
	EffectID int64
	Name     string
	TypeID   int64
	IsPVP    bool
	IsPVE    bool
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
	EquipmentEffectIDs  map[int64]int64
	RelicEffectIDs      map[int64]int64
	EffectDefinitions   map[int64]liveEffectDefinition
	GemEffects          map[int64][]liveCatalogEffect
	GemSetIDs           map[int64]int64
	EquipmentSetEffects map[int64][]liveEquipmentSetEffect
}

var (
	liveEffectDataOnce sync.Once
	liveEffectData     liveEffectDataCache
)

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

func resolveLiveCatalogEffect(rawID float64, source CatalogEffectSource) (liveResolvedEffect, bool) {
	raw := int64(rawID)
	if raw <= 0 {
		return liveResolvedEffect{}, false
	}
	data := loadLiveEffectData()

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
		if mapped := data.EquipmentEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
	case CatalogEffectSourceGem:
		if effect, ok := resolve(raw); ok {
			return effect, true
		}
		if mapped := data.EquipmentEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
		if mapped := data.RelicEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
		}
	default:
		if mapped := data.EquipmentEffectIDs[raw]; mapped > 0 {
			if effect, ok := resolve(mapped); ok {
				return effect, true
			}
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
		EquipmentEffectIDs:  map[int64]int64{},
		RelicEffectIDs:      map[int64]int64{},
		EffectDefinitions:   map[int64]liveEffectDefinition{},
		GemEffects:          map[int64][]liveCatalogEffect{},
		GemSetIDs:           map[int64]int64{},
		EquipmentSetEffects: map[int64][]liveEquipmentSetEffect{},
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEffectsItemsJSON) {
		effectID := liveIntFromValue(entry["effectID"])
		if effectID == 0 {
			continue
		}
		name := liveStringFromValue(entry["name"])
		lowerName := strings.ToLower(name)
		data.EffectDefinitions[effectID] = liveEffectDefinition{
			EffectID: effectID,
			Name:     name,
			TypeID:   liveIntFromValue(entry["effectTypeID"]),
			IsPVP:    liveStringFromValue(entry["isPvPFight"]) == "1" || strings.Contains(lowerName, "pvp"),
			IsPVE:    liveStringFromValue(entry["isPvEFight"]) == "1" || strings.Contains(lowerName, "pve"),
		}
	}

	for _, entry := range readLiveDataArray(serverdata.ReadEquipmentEffectsItemsJSON) {
		equipmentEffectID := liveIntFromValue(entry["equipmentEffectID"])
		effectID := liveIntFromValue(entry["effectID"])
		if equipmentEffectID > 0 && effectID > 0 {
			data.EquipmentEffectIDs[equipmentEffectID] = effectID
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

func liveEffectLabel(def liveEffectDefinition) string {
	name := strings.TrimSpace(def.Name)
	if name == "" {
		return fmt.Sprintf("Effect %d", def.EffectID)
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

func liveEffectCategory(def liveEffectDefinition) string {
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

func liveEffectUnit(def liveEffectDefinition) string {
	switch def.TypeID {
	case 47, 51, 148, 150, 156, 179, 181:
		return "number"
	default:
		return "percent"
	}
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

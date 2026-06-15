package featureview

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// legacyEffectFields mirrors GeneralsCamp building_items/script.js — stat columns that define
// the effect line when the wire "effects" field is empty (see generalscamp overview).
var legacyEffectFields = []string{
	"unitWallCount", "recruitSpeedBoost", "woodStorage", "stoneStorage", "ReduceResearchResourceCosts",
	"Stoneproduction", "Woodproduction", "Foodproduction", "foodStorage", "unboostedFoodProduction",
	"defensiveToolsSpeedBoost", "defensiveToolsCostsReduction", "meadStorage", "recruitCostReduction",
	"honeyStorage", "hospitalCapacity", "healSpeed", "marketCarriages", "XPBoostBuildBuildings",
	"stackSize", "glassStorage", "Glassproduction", "ironStorage", "Ironproduction", "coalStorage",
	"Coalproduction", "oilStorage", "Oilproduction", "offensiveToolsCostsReduction", "feastCostsReduction",
	"Meadreduction", "surviveBoost", "unboostedStoneProduction", "unboostedWoodProduction",
	"offensiveToolsSpeedBoost", "espionageTravelBoost",
}

func mapString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case float64:
		if t == math.Trunc(t) {
			return strconv.Itoa(int(t))
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case json.Number:
		return strings.TrimSpace(t.String())
	default:
		return strings.TrimSpace(fmt.Sprint(v))
	}
}

func mapInt(m map[string]interface{}, key string) int {
	s := mapString(m, key)
	if s == "" {
		return 0
	}
	n, _ := strconv.Atoi(s)
	return n
}

func isTestingTCIRow(m map[string]interface{}) bool {
	testing := func(s string) bool { return strings.Contains(strings.ToLower(s), "testing") }
	return testing(mapString(m, "comment1")) || testing(mapString(m, "comment2"))
}

func isLegacyFourDigitWireCID(cid int) bool {
	return cid >= 1000 && cid <= 9999
}

// isTCIRow is true for temporary construction items included in the AutoTCI catalog picker.
// Appearance rows and legacy 4-digit wire CIDs (1000–9999) are excluded.
func isTCIRow(m map[string]interface{}) bool {
	if isTestingTCIRow(m) {
		return false
	}
	if mapInt(m, "duration") <= 0 {
		return false
	}
	if strings.EqualFold(mapString(m, "comment1"), "Appearance") {
		return false
	}
	return !isLegacyFourDigitWireCID(mapInt(m, "constructionItemID"))
}

// tciGroupKey follows GeneralsCamp name/effect/legacy/slot grouping, plus constructionItemGroupID
// so each selectable AutoTCI row stays a single upgrade line (≤4 tiers) when the game reuses names.
func tciGroupKey(m map[string]interface{}) string {
	name := mapString(m, "name")
	effectIDSet := make(map[string]struct{})
	if eff := mapString(m, "effects"); eff != "" {
		for _, part := range strings.Split(eff, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			idVal := strings.Split(part, "&")
			if len(idVal) == 0 || idVal[0] == "" {
				continue
			}
			effectIDSet[idVal[0]] = struct{}{}
		}
	}
	effectIDs := make([]string, 0, len(effectIDSet))
	for id := range effectIDSet {
		effectIDs = append(effectIDs, id)
	}
	sort.Strings(effectIDs)
	allEffects := strings.Join(effectIDs, ",")

	var legacyPresent []string
	for _, field := range legacyEffectFields {
		if v := mapString(m, field); v != "" {
			legacyPresent = append(legacyPresent, field)
		}
	}
	sort.Strings(legacyPresent)
	legacyParts := strings.Join(legacyPresent, ",")

	appearanceFlag := "normal"
	if mapInt(m, "slotTypeID") == 0 && mapString(m, "decoPoints") != "" {
		appearanceFlag = "appearance"
	}
	durationFlag := "permanent"
	if mapInt(m, "duration") > 0 {
		durationFlag = "temporary"
	}
	slotTypeID := mapString(m, "slotTypeID")
	groupID := mapString(m, "constructionItemGroupID")
	return fmt.Sprintf("%s_%s_%s_%s_%s_%s_%s", name, allEffects, legacyParts, appearanceFlag, durationFlag, slotTypeID, groupID)
}

func formatEffectNumber(n int) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := strconv.Itoa(n)
	if len(s) <= 3 {
		if neg {
			return "-" + s
		}
		return s
	}
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

func parseEffectsDisplay(effectsStr string) []string {
	if effectsStr == "" {
		return nil
	}
	effectNamesOnce.Do(loadEffectNames)
	var out []string
	for _, eff := range strings.Split(effectsStr, ",") {
		eff = strings.TrimSpace(eff)
		if eff == "" {
			continue
		}
		idVal := strings.Split(eff, "&")
		if len(idVal) == 0 {
			continue
		}
		effID := idVal[0]
		valRaw := ""
		if len(idVal) > 1 {
			valRaw = idVal[1]
		}
		var variant int
		val := 0
		if strings.Contains(valRaw, "+") {
			parts := strings.SplitN(valRaw, "+", 2)
			variant, _ = strconv.Atoi(parts[0])
			if len(parts) > 1 {
				val, _ = strconv.Atoi(parts[1])
			}
		} else if valRaw != "" {
			val, _ = strconv.Atoi(valRaw)
		}
		_ = variant

		internalName, ok := effectNamesMap[effID]
		nameText := internalName
		if !ok || nameText == "" {
			nameText = fmt.Sprintf("Effect ID %s", effID)
		} else {
			translated := GetTCIEffectName(internalName)
			if translated != "" {
				nameText = translated
			}
		}

		suffix := ""
		if strings.Contains(strings.ToLower(nameText), "%") ||
			strings.Contains(strings.ToLower(internalName), "percent") ||
			strings.Contains(strings.ToLower(internalName), "boost") {
			if !strings.Contains(strings.ToLower(internalName), "unboosted") {
				suffix = "%"
			}
		}

		if strings.Contains(nameText, "{0}") {
			label := nameText
			label = strings.ReplaceAll(label, "{0}", "")
			label = strings.ReplaceAll(label, "%", "")
			label = strings.ReplaceAll(label, "+", "")
			label = strings.ReplaceAll(label, "-", "")
			label = strings.Trim(label, ": ")
			if label != "" {
				label = strings.ToUpper(label[:1]) + label[1:]
			}
			sign := "+"
			if strings.Contains(nameText, "-{0}") || strings.Contains(nameText, "- {0}") {
				sign = "-"
			} else if val < 0 {
				sign = "-"
			}
			out = append(out, fmt.Sprintf("%s: %s%s%s", label, sign, formatEffectNumber(int(math.Abs(float64(val)))), suffix))
			continue
		}
		sep := ""
		if !strings.Contains(nameText, ":") {
			sep = ": "
		} else {
			sep = " "
		}
		out = append(out, fmt.Sprintf("%s%s%s%s", nameText, sep, formatEffectNumber(val), suffix))
	}
	return out
}

func appendLegacyEffectsDisplay(item map[string]interface{}, effects []string) []string {
	rendered := strings.ToLower(strings.Join(effects, " "))
	for _, field := range legacyEffectFields {
		valStr := mapString(item, field)
		if valStr == "" {
			continue
		}
		val, err := strconv.Atoi(valStr)
		if err != nil {
			continue
		}
		template := GetTCIEffectName(field)
		label := template
		if label == field || label == strings.ToLower(field) {
			label = formatDisplayName(field)
		}
		if strings.Contains(template, "{0}") {
			label = strings.ReplaceAll(template, "{0}", "")
			label = strings.ReplaceAll(label, "%", "")
			label = strings.ReplaceAll(label, "+", "")
			label = strings.ReplaceAll(label, "-", "")
			label = strings.Trim(label, ": ")
			if label != "" {
				label = strings.ToUpper(label[:1]) + label[1:]
			}
			sign := "+"
			if strings.Contains(template, "-{0}") || strings.Contains(template, "- {0}") {
				sign = "-"
			} else if val < 0 {
				sign = "-"
			}
			suffix := ""
			if strings.Contains(template, "%") {
				suffix = "%"
			}
			renderedLine := fmt.Sprintf("%s: %s%s%s", label, sign, formatEffectNumber(int(math.Abs(float64(val)))), suffix)
			if !strings.Contains(rendered, strings.ToLower(renderedLine)) {
				effects = append(effects, renderedLine)
				rendered = strings.ToLower(strings.Join(effects, " "))
			}
			continue
		}
		renderedLine := fmt.Sprintf("%s: %s", label, formatEffectNumber(val))
		if !strings.Contains(rendered, strings.ToLower(renderedLine)) {
			effects = append(effects, renderedLine)
			rendered = strings.ToLower(strings.Join(effects, " "))
		}
	}
	return effects
}

func buildTCIDisplayEffects(item map[string]interface{}) string {
	parts := parseEffectsDisplay(mapString(item, "effects"))
	parts = appendLegacyEffectsDisplay(item, parts)
	if dp := mapString(item, "decoPoints"); dp != "" {
		if n, err := strconv.Atoi(dp); err == nil {
			line := fmt.Sprintf("Public order: %s", formatEffectNumber(n))
			if !strings.Contains(strings.ToLower(strings.Join(parts, " ")), strings.ToLower(line)) {
				parts = append(parts, line)
			}
		}
	}
	return strings.Join(parts, " • ")
}

func sortTCIGroupItems(items []map[string]interface{}) {
	sort.Slice(items, func(i, j int) bool {
		a, b := items[i], items[j]
		if mapInt(a, "slotTypeID") == 1 && mapInt(b, "slotTypeID") == 1 {
			return mapInt(a, "level") < mapInt(b, "level")
		}
		rarA, rarB := mapInt(a, "rarenessID"), mapInt(b, "rarenessID")
		if rarA != rarB {
			return rarA < rarB
		}
		return totalEffectValue(a) < totalEffectValue(b)
	})
}

func totalEffectValue(item map[string]interface{}) int {
	total := 0
	eff := mapString(item, "effects")
	if eff == "" {
		return total
	}
	for _, part := range strings.Split(eff, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		idVal := strings.Split(part, "&")
		if len(idVal) < 2 {
			continue
		}
		valRaw := idVal[1]
		if strings.Contains(valRaw, "+") {
			pieces := strings.SplitN(valRaw, "+", 2)
			if len(pieces) > 1 {
				if v, err := strconv.Atoi(pieces[1]); err == nil {
					total += v
				}
			}
		} else if v, err := strconv.Atoi(valRaw); err == nil {
			total += v
		}
	}
	return total
}

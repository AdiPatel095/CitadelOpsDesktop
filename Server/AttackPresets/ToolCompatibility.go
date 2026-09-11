package AttackPresets

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"CitadelDesktop/Server/GameData"
)

// ToolTarget identifies the official game context in which a saved attack
// preset will be used. Some tools are valid attack items but are restricted to
// specific map targets or events by the live units catalog.
type ToolTarget struct {
	KingdomID int64
	TypeID    int
	EventID   int64
	Label     string
}

// ValidateToolCompatibility rejects a saved preset before CRA when the
// official units catalog says one of its tools cannot be used for the selected
// target. Empty restriction fields mean that the catalog does not constrain
// the tool by that dimension.
func ValidateToolCompatibility(preset Preset, gameData *GameData.Store, target ToolTarget) error {
	if gameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return err
	}
	for _, toolID := range presetToolIDs(preset) {
		raw, found := catalog.Find(strconv.FormatInt(toolID, 10))
		if !found {
			// The ordinary preset builder owns missing-definition errors.
			continue
		}
		record, err := GameData.DecodeRecord(raw)
		if err != nil {
			return fmt.Errorf("decode official tool definition %d: %w", toolID, err)
		}
		allowedTargets, targetRestricted, err := decodeAllowedAttackTargets(record["allowedToAttack"])
		if err != nil {
			return fmt.Errorf("validate official target restrictions for tool %d: %w", toolID, err)
		}
		allowedEvents, eventRestricted, err := decodeRestrictedIDs(record["usageEventID"])
		if err != nil {
			return fmt.Errorf("validate official event restrictions for tool %d: %w", toolID, err)
		}
		targetAllowed := !targetRestricted
		for _, allowed := range allowedTargets {
			if (allowed.kingdomID < 0 || allowed.kingdomID == target.KingdomID) &&
				(allowed.typeID < 0 || allowed.typeID == int64(target.TypeID)) {
				targetAllowed = true
				break
			}
		}
		eventAllowed := !eventRestricted
		for _, eventID := range allowedEvents {
			if eventID < 0 || eventID == target.EventID {
				eventAllowed = true
				break
			}
		}
		if targetAllowed && eventAllowed {
			continue
		}
		toolName := restrictedToolName(record, gameData, toolID)
		targetName := strings.TrimSpace(target.Label)
		if targetName == "" {
			targetName = "this target"
		}
		presetName := strings.TrimSpace(preset.Name)
		if presetName == "" {
			presetName = "selected attack"
		}
		return fmt.Errorf(
			"the selected preset %q uses %s, which the game does not allow against %s; remove or replace that tool in the preset before retrying",
			presetName, toolName, targetName,
		)
	}
	return nil
}

func restrictedToolName(record GameData.Record, gameData *GameData.Store, toolID int64) string {
	// The units catalog's generic name is often Eventtool. Its type/comment
	// fields retain the recognizable tool name even when language data is not
	// present in a policy snapshot.
	for _, field := range []string{"type", "comment2", "comment1"} {
		if value, found := record.String(field); found {
			if name := humanizeToolName(value); name != "" {
				return name
			}
		}
	}
	if name, found := GameData.OfficialDefinitionName(gameData, nil, "units", toolID); found {
		return name
	}
	return fmt.Sprintf("tool %d", toolID)
}

func humanizeToolName(value string) string {
	value = strings.TrimSpace(strings.NewReplacer("_", " ", "-", " ").Replace(value))
	if value == "" || strings.EqualFold(value, "unknown") || strings.EqualFold(value, "placeholder") {
		return ""
	}
	runes := []rune(value)
	var expanded strings.Builder
	for index, current := range runes {
		if index > 0 && unicode.IsUpper(current) &&
			(unicode.IsLower(runes[index-1]) || unicode.IsDigit(runes[index-1])) {
			expanded.WriteByte(' ')
		}
		expanded.WriteRune(current)
	}
	words := strings.Fields(expanded.String())
	for index, word := range words {
		if word == strings.ToUpper(word) && len(word) > 1 {
			continue
		}
		words[index] = strings.ToUpper(word[:1]) + word[1:]
	}
	return strings.Join(words, " ")
}

type allowedAttackTarget struct {
	kingdomID int64
	typeID    int64
}

func decodeAllowedAttackTargets(raw json.RawMessage) ([]allowedAttackTarget, bool, error) {
	values, restricted, err := decodeRestrictionValues(raw)
	if err != nil || !restricted {
		return nil, restricted, err
	}
	result := make([]allowedAttackTarget, 0, len(values))
	for _, value := range values {
		for _, entry := range strings.FieldsFunc(value, func(r rune) bool {
			return r == '#' || r == ',' || r == ';' || r == '|'
		}) {
			entry = strings.TrimSpace(entry)
			kingdom, targetType, found := strings.Cut(entry, "+")
			if !found {
				return nil, true, fmt.Errorf("unsupported allowedToAttack value %q", entry)
			}
			kingdomID, err := decodeRestrictionID(kingdom)
			if err != nil {
				return nil, true, fmt.Errorf("decode allowed kingdom %q: %w", kingdom, err)
			}
			typeID, err := decodeRestrictionID(targetType)
			if err != nil {
				return nil, true, fmt.Errorf("decode allowed target type %q: %w", targetType, err)
			}
			result = append(result, allowedAttackTarget{kingdomID: kingdomID, typeID: typeID})
		}
	}
	if len(result) == 0 {
		return nil, true, fmt.Errorf("allowedToAttack contains no target entries")
	}
	return result, true, nil
}

func decodeRestrictedIDs(raw json.RawMessage) ([]int64, bool, error) {
	values, restricted, err := decodeRestrictionValues(raw)
	if err != nil || !restricted {
		return nil, restricted, err
	}
	result := make([]int64, 0, len(values))
	for _, value := range values {
		for _, entry := range strings.FieldsFunc(value, func(r rune) bool {
			return r == ',' || r == '#' || r == ';' || r == '|' || r == ' ' || r == '\t'
		}) {
			id, err := decodeRestrictionID(entry)
			if err != nil {
				return nil, true, fmt.Errorf("decode restricted id %q: %w", entry, err)
			}
			result = append(result, id)
		}
	}
	if len(result) == 0 {
		return nil, true, fmt.Errorf("restriction contains no ids")
	}
	return result, true, nil
}

func decodeRestrictionValues(raw json.RawMessage) ([]string, bool, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" || trimmed == `""` || trimmed == "[]" {
		return nil, false, nil
	}
	var scalar string
	if json.Unmarshal(raw, &scalar) == nil {
		if strings.TrimSpace(scalar) == "" {
			return nil, false, nil
		}
		return []string{scalar}, true, nil
	}
	var list []string
	if json.Unmarshal(raw, &list) == nil {
		values := make([]string, 0, len(list))
		for _, value := range list {
			if strings.TrimSpace(value) != "" {
				values = append(values, value)
			}
		}
		if len(values) == 0 {
			return nil, false, nil
		}
		return values, true, nil
	}
	var number json.Number
	if json.Unmarshal(raw, &number) == nil {
		return []string{number.String()}, true, nil
	}
	return nil, true, fmt.Errorf("unsupported restriction value %s", trimmed)
}

func decodeRestrictionID(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "*" {
		return -1, nil
	}
	id, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, err
	}
	return id, nil
}

func presetToolIDs(preset Preset) []int64 {
	seen := map[int64]struct{}{}
	result := make([]int64, 0)
	appendSlots := func(slots []Slot) {
		for _, slot := range slots {
			if slot.ItemID == nil || *slot.ItemID <= 0 || slot.Quantity <= 0 {
				continue
			}
			if _, duplicate := seen[*slot.ItemID]; duplicate {
				continue
			}
			seen[*slot.ItemID] = struct{}{}
			result = append(result, *slot.ItemID)
		}
	}
	for _, wave := range preset.Waves {
		appendSlots(wave.Left.Tools)
		appendSlots(wave.Middle.Tools)
		appendSlots(wave.Right.Tools)
	}
	appendSlots(preset.CourtyardSupport.Tools)
	return result
}

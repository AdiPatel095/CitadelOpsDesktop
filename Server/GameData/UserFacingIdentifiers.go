package GameData

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode"

	"CitadelDesktop/Server/State"
)

// IdentifierLabels resolves the numeric identifiers that commonly leak into
// intent errors and automation status text. Names come from the current player
// state first and the official item/language catalogs second. Unknown IDs are
// intentionally left unchanged so a guessed label can never misidentify a
// live game object.
type IdentifierLabels struct {
	state    State.GameState
	store    *Store
	language *LanguageStore
}

func NewIdentifierLabels(state State.GameState, store *Store, language *LanguageStore) IdentifierLabels {
	return IdentifierLabels{state: state, store: store, language: language}
}

type identifierPattern struct {
	expression *regexp.Regexp
	resolve    func(IdentifierLabels, int64) (string, bool)
}

var userFacingIdentifierPatterns = []identifierPattern{
	{regexp.MustCompile(`(?i)\bconstruction item ([0-9]+)\b`), resolveConstructionItem},
	{regexp.MustCompile(`(?i)\bcrafting recipe ([0-9]+)\b`), resolveCraftingRecipe},
	{regexp.MustCompile(`(?i)\bhall of legends skill ([0-9]+)\b`), collectionResolver("legendskills", "Hall of Legends skill")},
	{regexp.MustCompile(`(?i)\bsceat skill ([0-9]+)\b`), collectionResolver("sceatSkills", "Sceat skill")},
	{regexp.MustCompile(`(?i)\bgeneral skill ([0-9]+)\b`), collectionResolver("generalSkills", "general skill")},
	{regexp.MustCompile(`(?i)\bbuilding instance ([0-9]+)\b`), resolveBuildingInstance},
	{regexp.MustCompile(`(?i)\bbuilding definition ([0-9]+)\b`), resolveBuildingDefinition},
	{regexp.MustCompile(`(?i)\bprebuilt castle ([0-9]+)\b`), collectionResolver("prebuiltcastles", "prebuilt castle")},
	{regexp.MustCompile(`(?i)\bevent camp ([0-9]+)\b`), collectionResolver("eventAutoScalingCamps", "event camp")},
	{regexp.MustCompile(`(?i)\bstorm isle ([0-9]+)\b`), resolveStormIsle},
	{regexp.MustCompile(`(?i)\bauto bird target ([0-9]+)\b`), resolveAutoBirdTarget},
	{regexp.MustCompile(`(?i)\bcastle ([0-9]+)\b`), resolveCastle},
	{regexp.MustCompile(`(?i)\bcamp ([0-9]+)\b`), resolveCamp},
	{regexp.MustCompile(`(?i)\bcommander ([0-9]+)\b`), resolveCommander},
	{regexp.MustCompile(`(?i)\bcastellan ([0-9]+)\b`), resolveCastellan},
	{regexp.MustCompile(`(?i)\bequipment ([0-9]+)\b`), resolveEquipmentInstance},
	{regexp.MustCompile(`(?i)\bgem ([0-9]+)\b`), resolveGemInstance},
	{regexp.MustCompile(`(?i)\bgem carrier ([0-9]+)\b`), resolveGemCarrier},
	{regexp.MustCompile(`(?i)\bmovement ([0-9]+)\b`), resolveMovement},
	{regexp.MustCompile(`(?i)\bplayer ([0-9]+)\b`), resolvePlayer},
	{regexp.MustCompile(`(?i)\balliance ([0-9]+)\b`), resolveAlliance},
	{regexp.MustCompile(`(?i)\bbuilding ([0-9]+)\b`), resolveBuildingInstanceOrDefinition},
	{regexp.MustCompile(`(?i)\brecipe ([0-9]+)\b`), resolveCraftingRecipe},
	{regexp.MustCompile(`(?i)\bskill ([0-9]+)\b`), resolveSkill},
	{regexp.MustCompile(`(?i)\bunit ([0-9]+)\b`), resolveUnit},
	{regexp.MustCompile(`(?i)\btool ([0-9]+)\b`), resolveTool},
	{regexp.MustCompile(`(?i)\bresource ([0-9]+)\b`), collectionResolver("resources", "resource")},
	{regexp.MustCompile(`(?i)\bcurrency ([0-9]+)\b`), collectionResolver("currencies", "currency")},
	{regexp.MustCompile(`(?i)\bpackage ([0-9]+)\b`), collectionResolver("packages", "package")},
	{regexp.MustCompile(`(?i)\bhorse ([0-9]+)\b`), collectionResolver("horses", "horse")},
	{regexp.MustCompile(`(?i)\bkingdom ([0-9]+)\b`), collectionResolver("kingdoms", "kingdom")},
	{regexp.MustCompile(`(?i)\bdifficulty ([0-9]+)\b`), resolveDifficulty},
	{regexp.MustCompile(`(?i)\bevent ([0-9]+)\b`), resolveEvent},
	{regexp.MustCompile(`(?i)\bachievement ([0-9]+)\b`), collectionResolver("achievements", "achievement")},
	{regexp.MustCompile(`(?i)\breward ([0-9]+)\b`), collectionResolver("rewards", "reward")},
	{regexp.MustCompile(`(?i)\beffect ([0-9]+)\b`), collectionResolver("effects", "effect")},
	{regexp.MustCompile(`(?i)\bitem ([0-9]+)\b`), resolveGenericItem},
}

// Humanize replaces recognized raw identifier phrases with a user-facing name
// and retains the original typed ID in parentheses for support diagnostics.
func (labels IdentifierLabels) Humanize(text string) string {
	for _, pattern := range userFacingIdentifierPatterns {
		text = pattern.expression.ReplaceAllStringFunc(text, func(match string) string {
			parts := pattern.expression.FindStringSubmatch(match)
			if len(parts) != 2 {
				return match
			}
			id, err := strconv.ParseInt(parts[1], 10, 64)
			if err != nil || id < 0 {
				return match
			}
			if resolved, found := pattern.resolve(labels, id); found {
				return resolved
			}
			return match
		})
	}
	return text
}

// OfficialDefinitionName resolves one definition from the official catalogs.
// The boolean is false when the ID or its label is not authoritative.
func OfficialDefinitionName(store *Store, language *LanguageStore, collection string, id int64) (string, bool) {
	if store == nil || id < 0 {
		return "", false
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return "", false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return "", false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return "", false
	}
	name, found := officialDefinitionRecordName(record, language, collection)
	if !found {
		return "", false
	}
	if level, hasLevel := record.Int64("level"); hasLevel && level > 0 &&
		(collection == "buildings" || collection == "constructionItems" || collection == "craftingRecipes") {
		name = fmt.Sprintf("%s level %d", name, level)
	}
	return name, true
}

func officialDefinitionRecordName(record Record, language *LanguageStore, collection string) (string, bool) {
	if displayName, found := record.String("_display_name"); found && strings.TrimSpace(displayName) != "" {
		return strings.TrimSpace(displayName), true
	}
	fields := []string{"name", "Name", "type", "JSONKey", "comment1", "comment2"}
	switch collection {
	case "equipments", "gems":
		fields = []string{"comment2", "comment1", "name", "Name", "type"}
	case "packages", "horses":
		fields = []string{"comment1", "name", "Name", "comment2", "type"}
	case "currencies":
		fields = []string{"Name", "name", "assetName", "JSONKey"}
	}
	internalNames := make([]string, 0, len(fields))
	for _, field := range fields {
		if value, found := record.String(field); found && strings.TrimSpace(value) != "" {
			internalNames = append(internalNames, strings.TrimSpace(value))
		}
	}
	if language != nil {
		keys := make([]string, 0, len(internalNames)*3)
		for _, name := range internalNames {
			keys = append(keys, name+"_name", "currency_name_"+name, name)
		}
		if name, found := language.Resolve(keys...); found && strings.TrimSpace(name) != "" {
			return strings.TrimSpace(name), true
		}
	}
	for _, name := range internalNames {
		if cleaned := humanizeInternalName(name); cleaned != "" {
			return cleaned, true
		}
	}
	return "", false
}

func humanizeInternalName(value string) string {
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

func resolveCastle(labels IdentifierLabels, id int64) (string, bool) {
	castle, found := labels.state.Castles[State.CastleID(id)]
	if !found {
		return "", false
	}
	name := strings.TrimSpace(castle.Name)
	if name == "" && (castle.X != 0 || castle.Y != 0) {
		name = fmt.Sprintf("Castle at %d:%d", castle.X, castle.Y)
	}
	if name == "" {
		return "", false
	}
	return fmt.Sprintf("%s (castle ID %d)", name, id), true
}

func resolveAutoBirdTarget(labels IdentifierLabels, id int64) (string, bool) {
	name, found := resolveCastle(labels, id)
	if !found {
		return "", false
	}
	return "Auto Bird target " + name, true
}

func resolveCommander(labels IdentifierLabels, id int64) (string, bool) {
	commander, found := labels.state.Commanders[State.CommanderID(id)]
	if !found {
		return "", false
	}
	name := strings.TrimSpace(commander.Name)
	if name == "" && commander.VisiblePosition > 0 {
		name = fmt.Sprintf("Commander slot %d", commander.VisiblePosition)
	}
	if name == "" {
		return "", false
	}
	return fmt.Sprintf("%s (commander ID %d)", name, id), true
}

func resolveCastellan(labels IdentifierLabels, id int64) (string, bool) {
	castellan, found := labels.state.Castellans[State.CastellanID(id)]
	if !found {
		return "", false
	}
	name := strings.TrimSpace(castellan.Name)
	if name == "" && castellan.CastleID > 0 {
		if castle, castleFound := labels.state.Castles[castellan.CastleID]; castleFound && strings.TrimSpace(castle.Name) != "" {
			name = "Castellan of " + strings.TrimSpace(castle.Name)
		}
	}
	if name == "" {
		return "", false
	}
	return fmt.Sprintf("%s (castellan ID %d)", name, id), true
}

func resolveMovement(labels IdentifierLabels, id int64) (string, bool) {
	movement, found := labels.state.LookupMovement(State.MovementID(id))
	if !found {
		return "", false
	}
	label := "Movement"
	if source, sourceFound := labels.state.Castles[movement.SourceCastleID]; sourceFound && strings.TrimSpace(source.Name) != "" {
		label = "Movement from " + strings.TrimSpace(source.Name)
	}
	if movement.TargetX != 0 || movement.TargetY != 0 {
		label += fmt.Sprintf(" to %d:%d", movement.TargetX, movement.TargetY)
	}
	return fmt.Sprintf("%s (movement ID %d)", label, id), true
}

func resolvePlayer(labels IdentifierLabels, id int64) (string, bool) {
	if int64(labels.state.Player.ID) == id && strings.TrimSpace(labels.state.Player.Name) != "" {
		return fmt.Sprintf("%s (player ID %d)", strings.TrimSpace(labels.state.Player.Name), id), true
	}
	alliances := make([]State.AllianceState, 0, len(labels.state.Alliances)+1)
	alliances = append(alliances, labels.state.Alliance)
	for _, alliance := range labels.state.Alliances {
		alliances = append(alliances, alliance)
	}
	for _, alliance := range alliances {
		for _, member := range alliance.Members {
			if int64(member.PlayerID) == id && strings.TrimSpace(member.Name) != "" {
				return fmt.Sprintf("%s (player ID %d)", strings.TrimSpace(member.Name), id), true
			}
		}
	}
	return "", false
}

func resolveAlliance(labels IdentifierLabels, id int64) (string, bool) {
	if int64(labels.state.Alliance.ID) == id && strings.TrimSpace(labels.state.Alliance.Name) != "" {
		return fmt.Sprintf("%s (alliance ID %d)", strings.TrimSpace(labels.state.Alliance.Name), id), true
	}
	if alliance, found := labels.state.Alliances[State.AllianceID(id)]; found && strings.TrimSpace(alliance.Name) != "" {
		return fmt.Sprintf("%s (alliance ID %d)", strings.TrimSpace(alliance.Name), id), true
	}
	return "", false
}

func resolveEquipmentInstance(labels IdentifierLabels, id int64) (string, bool) {
	item, found := labels.state.Inventory.Equipment[State.EquipmentInstanceID(id)]
	if !found {
		return "", false
	}
	name, named := OfficialDefinitionName(labels.store, labels.language, "equipments", int64(item.DefinitionID))
	if !named {
		return "", false
	}
	return fmt.Sprintf("%s (equipment ID %d)", name, id), true
}

func resolveGemInstance(labels IdentifierLabels, id int64) (string, bool) {
	gem, found := labels.state.Inventory.Gems[State.GemInstanceID(id)]
	if !found {
		return "", false
	}
	name, named := OfficialDefinitionName(labels.store, labels.language, "gems", int64(gem.DefinitionID))
	if !named {
		return "", false
	}
	if gem.Level > 0 {
		name = fmt.Sprintf("%s level %d", name, gem.Level)
	}
	return fmt.Sprintf("%s (gem ID %d)", name, id), true
}

func resolveGemCarrier(labels IdentifierLabels, id int64) (string, bool) {
	name, found := resolveEquipmentInstance(labels, id)
	if !found {
		return "", false
	}
	return "gem carrier " + name, true
}

func resolveBuildingInstance(labels IdentifierLabels, id int64) (string, bool) {
	var matched *State.Building
	for _, castle := range labels.state.Castles {
		building, found := castle.Buildings[State.BuildingInstanceID(id)]
		if !found {
			for _, layer := range []map[State.BuildingInstanceID]State.Building{castle.Layout.Ground, castle.Layout.Objects, castle.Layout.Fixed} {
				if candidate, exists := layer[State.BuildingInstanceID(id)]; exists {
					building, found = candidate, true
					break
				}
			}
		}
		if found {
			copy := building
			matched = &copy
			break
		}
	}
	if matched == nil {
		return "", false
	}
	name, found := OfficialDefinitionName(labels.store, labels.language, "buildings", int64(matched.DefinitionID))
	if !found {
		return "", false
	}
	if matched.Level > 0 && !strings.Contains(strings.ToLower(name), " level ") {
		name = fmt.Sprintf("%s level %d", name, matched.Level)
	}
	return fmt.Sprintf("%s (building instance ID %d)", name, id), true
}

func resolveBuildingDefinition(labels IdentifierLabels, id int64) (string, bool) {
	return resolveDefinition(labels, "buildings", "building definition", id)
}

func resolveBuildingInstanceOrDefinition(labels IdentifierLabels, id int64) (string, bool) {
	if name, found := resolveBuildingInstance(labels, id); found {
		return name, true
	}
	return resolveBuildingDefinition(labels, id)
}

func resolveCamp(labels IdentifierLabels, id int64) (string, bool) {
	if name, found := resolveCastle(labels, id); found {
		return name, true
	}
	return resolveDefinition(labels, "prebuiltcastles", "camp", id)
}

func resolveStormIsle(labels IdentifierLabels, id int64) (string, bool) {
	if labels.store == nil {
		return "", false
	}
	isle, found := labels.store.StormIsleView(id)
	if !found {
		return "", false
	}
	name := "Storm island"
	if isle.Kind == StormIsleKindFort {
		name = "Storm fort"
	} else if isle.Resource != "" {
		name = strings.TrimSpace(strings.Join([]string{humanizeInternalName(isle.Size), humanizeInternalName(isle.Resource), "island"}, " "))
	}
	if isle.Level > 0 {
		name = fmt.Sprintf("%s level %d", name, isle.Level)
	}
	return fmt.Sprintf("%s (Storm isle ID %d)", name, id), true
}

func resolveConstructionItem(labels IdentifierLabels, id int64) (string, bool) {
	return resolveDefinition(labels, "constructionItems", "construction item", id)
}

func resolveCraftingRecipe(labels IdentifierLabels, id int64) (string, bool) {
	return resolveDefinition(labels, "craftingRecipes", "crafting recipe", id)
}

func resolveSkill(labels IdentifierLabels, id int64) (string, bool) {
	for _, collection := range []string{"legendskills", "sceatSkills", "generalSkills"} {
		if name, found := resolveDefinition(labels, collection, "skill", id); found {
			return name, true
		}
	}
	return "", false
}

func resolveEvent(labels IdentifierLabels, id int64) (string, bool) {
	if score, found := labels.state.LookupScalableEventScore(id); found {
		if name := localizedEventName(labels.language, score.LocalizationKey, score.Name, score.EventType, id); name != "" {
			return fmt.Sprintf("%s (event ID %d)", name, id), true
		}
	}
	if labels.store != nil {
		if definition, found := labels.store.ScalableEvent(id, 0); found {
			if name := localizedEventName(labels.language, definition.LocalizationKey, definition.Name, definition.EventType, id); name != "" {
				return fmt.Sprintf("%s (event ID %d)", name, id), true
			}
		}
		if catalog, err := labels.store.Catalog("events"); err == nil {
			if raw, found := catalog.FindByField("eventID", strconv.FormatInt(id, 10)); found {
				if record, decodeErr := DecodeRecord(raw); decodeErr == nil {
					if name, named := officialDefinitionRecordName(record, labels.language, "events"); named {
						return fmt.Sprintf("%s (event ID %d)", name, id), true
					}
				}
			}
		}
	}
	return "", false
}

func localizedEventName(language *LanguageStore, localizationKey string, name string, eventType string, id int64) string {
	if language != nil {
		if resolved, found := language.Resolve(localizationKey, fmt.Sprintf("event_title_%d", id)); found {
			return strings.TrimSpace(resolved)
		}
	}
	for _, candidate := range []string{name, eventType} {
		if resolved := humanizeInternalName(candidate); resolved != "" {
			return resolved
		}
	}
	return ""
}

func resolveDifficulty(labels IdentifierLabels, id int64) (string, bool) {
	var resolved string
	labels.state.RangeScalableEventScores(func(eventID int64, score State.ScalableEventScore) bool {
		if score.DifficultyID != id {
			return true
		}
		if name := humanizeInternalName(score.DifficultyTypeName); name != "" {
			resolved = fmt.Sprintf("%s (difficulty ID %d)", name, id)
			return false
		}
		if labels.store != nil {
			if definition, found := labels.store.ScalableEvent(eventID, id); found {
				if name := humanizeInternalName(definition.DifficultyTypeName); name != "" {
					resolved = fmt.Sprintf("%s (difficulty ID %d)", name, id)
					return false
				}
			}
		}
		return true
	})
	if resolved != "" {
		return resolved, true
	}
	if labels.store == nil {
		return "", false
	}
	difficulties, err := labels.store.Catalog("eventAutoScalingDifficulties")
	if err != nil {
		return "", false
	}
	raw, found := difficulties.Find(strconv.FormatInt(id, 10))
	if !found {
		return "", false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return "", false
	}
	typeID, found := record.Int64("difficultyTypeID")
	if !found {
		return "", false
	}
	types, err := labels.store.Catalog("eventAutoScalingDifficultyTypes")
	if err != nil {
		return "", false
	}
	typeRaw, found := types.Find(strconv.FormatInt(typeID, 10))
	if !found {
		return "", false
	}
	typeRecord, err := DecodeRecord(typeRaw)
	if err != nil {
		return "", false
	}
	name, found := officialDefinitionRecordName(typeRecord, labels.language, "eventAutoScalingDifficultyTypes")
	if !found {
		return "", false
	}
	return fmt.Sprintf("%s (difficulty ID %d)", name, id), true
}

func resolveUnit(labels IdentifierLabels, id int64) (string, bool) {
	return resolveDefinition(labels, "units", "unit", id)
}

func resolveTool(labels IdentifierLabels, id int64) (string, bool) {
	return resolveDefinition(labels, "units", "tool", id)
}

func collectionResolver(collection string, kind string) func(IdentifierLabels, int64) (string, bool) {
	return func(labels IdentifierLabels, id int64) (string, bool) {
		return resolveDefinition(labels, collection, kind, id)
	}
}

func resolveDefinition(labels IdentifierLabels, collection string, kind string, id int64) (string, bool) {
	name, found := OfficialDefinitionName(labels.store, labels.language, collection, id)
	if !found {
		return "", false
	}
	return fmt.Sprintf("%s (%s ID %d)", name, kind, id), true
}

func resolveGenericItem(labels IdentifierLabels, id int64) (string, bool) {
	if name, found := resolveEquipmentInstance(labels, id); found {
		return name, true
	}
	for _, castle := range labels.state.Castles {
		if castle.Units.Stationed[State.UnitID(id)] > 0 || castle.Units.Total[State.UnitID(id)] > 0 || castle.Defense.Inventory[State.UnitID(id)] > 0 {
			return resolveDefinition(labels, "units", "item", id)
		}
	}
	if labels.state.Inventory.ConstructionItems[State.ConstructionItemID(id)] > 0 {
		return resolveDefinition(labels, "constructionItems", "item", id)
	}
	return resolveDefinition(labels, "units", "item", id)
}

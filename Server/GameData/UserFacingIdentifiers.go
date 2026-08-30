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
// intent errors, automation status, and user activity. Names come from the
// current player state first and the official item/language catalogs second.
// Unknown identifiers are reduced to an honest generic label instead of
// exposing an implementation detail or guessing at a live game object.
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
	{regexp.MustCompile(`(?i)\bconstruction[- ]item (?:id )?([0-9]+)\b`), resolveConstructionItem},
	{regexp.MustCompile(`(?i)\bcrafting recipe (?:id )?([0-9]+)\b`), resolveCraftingRecipe},
	{regexp.MustCompile(`(?i)\bhall of legends skill (?:id )?([0-9]+)\b`), collectionResolver("legendskills", "Hall of Legends skill")},
	{regexp.MustCompile(`(?i)\bsceat skill (?:id )?([0-9]+)\b`), collectionResolver("sceatSkills", "Sceat skill")},
	{regexp.MustCompile(`(?i)\bgeneral skill (?:id )?([0-9]+)\b`), collectionResolver("generalSkills", "general skill")},
	{regexp.MustCompile(`(?i)\bbuilding instance (?:id )?([0-9]+)\b`), resolveBuildingInstance},
	{regexp.MustCompile(`(?i)\bbuilding operation (?:id )?([0-9]+)\b`), resolveBuildingInstance},
	{regexp.MustCompile(`(?i)\bbuilding definition (?:id )?([0-9]+)\b`), resolveBuildingDefinition},
	{regexp.MustCompile(`(?i)\btools? definition (?:id )?([0-9]+)\b`), resolveTool},
	{regexp.MustCompile(`(?i)\b(?:troops?|units?) definition (?:id )?([0-9]+)\b`), resolveUnit},
	{regexp.MustCompile(`(?i)\bprebuilt castle (?:id )?([0-9]+)\b`), collectionResolver("prebuiltcastles", "prebuilt castle")},
	{regexp.MustCompile(`(?i)\bevent camp (?:id )?([0-9]+)\b`), collectionResolver("eventAutoScalingCamps", "event camp")},
	{regexp.MustCompile(`(?i)\bstorm isle (?:id )?([0-9]+)\b`), resolveStormIsle},
	{regexp.MustCompile(`(?i)\bauto bird target (?:id )?([0-9]+)\b`), resolveAutoBirdTarget},
	{regexp.MustCompile(`(?i)\bcastle (?:id )?([0-9]+)(?::[0-9]+)?\b`), resolveCastle},
	{regexp.MustCompile(`(?i)\bcamp (?:id )?([0-9]+)(?::[0-9]+)?\b`), resolveCamp},
	{regexp.MustCompile(`(?i)\bcommander (?:id )?([0-9]+)\b`), resolveCommander},
	{regexp.MustCompile(`(?i)\bcastellan (?:id )?([0-9]+)\b`), resolveCastellan},
	{regexp.MustCompile(`(?i)\bequipment (?:id )?([0-9]+)\b`), resolveEquipmentInstance},
	{regexp.MustCompile(`(?i)\bgem (?:id )?([0-9]+)\b`), resolveGemInstance},
	{regexp.MustCompile(`(?i)\bgem carrier (?:id )?([0-9]+)\b`), resolveGemCarrier},
	{regexp.MustCompile(`(?i)\bmovement (?:id )?([0-9]+)\b`), resolveMovement},
	{regexp.MustCompile(`(?i)\bplayer (?:id )?([0-9]+)\b`), resolvePlayer},
	{regexp.MustCompile(`(?i)\balliance (?:id )?([0-9]+)\b`), resolveAlliance},
	{regexp.MustCompile(`(?i)\bbuilding (?:id )?([0-9]+)\b`), resolveBuildingInstanceOrDefinition},
	{regexp.MustCompile(`(?i)\brecipe (?:id )?([0-9]+)\b`), resolveCraftingRecipe},
	{regexp.MustCompile(`(?i)\bskill (?:id )?([0-9]+)\b`), resolveSkill},
	{regexp.MustCompile(`(?i)\bproduction definition (?:id )?([0-9]+)\b`), resolveUnit},
	{regexp.MustCompile(`(?i)\btroop (?:id )?([0-9]+)\b`), resolveUnit},
	{regexp.MustCompile(`(?i)\bunit (?:id )?([0-9]+)\b`), resolveUnit},
	{regexp.MustCompile(`(?i)\btool (?:id )?([0-9]+)\b`), resolveTool},
	{regexp.MustCompile(`(?i)\bresource (?:id )?([0-9]+)\b`), collectionResolver("resources", "resource")},
	{regexp.MustCompile(`(?i)\bcurrency (?:id )?([0-9]+)\b`), collectionResolver("currencies", "currency")},
	{regexp.MustCompile(`(?i)\bpackage (?:id )?([0-9]+)\b`), collectionResolver("packages", "package")},
	{regexp.MustCompile(`(?i)\bhorse (?:id )?([0-9]+)\b`), collectionResolver("horses", "horse")},
	{regexp.MustCompile(`(?i)\bkingdom (?:id )?([0-9]+)\b`), collectionResolver("kingdoms", "kingdom")},
	{regexp.MustCompile(`(?i)\bdifficulty (?:id )?([0-9]+)\b`), resolveDifficulty},
	{regexp.MustCompile(`(?i)\bevent (?:id )?([0-9]+)\b`), resolveEvent},
	{regexp.MustCompile(`(?i)\bachievement (?:id )?([0-9]+)\b`), collectionResolver("achievements", "achievement")},
	{regexp.MustCompile(`(?i)\breward (?:id )?([0-9]+)\b`), collectionResolver("rewards", "reward")},
	{regexp.MustCompile(`(?i)\beffect (?:id )?([0-9]+)\b`), collectionResolver("effects", "effect")},
	{regexp.MustCompile(`(?i)\bitem (?:id )?([0-9]+)\b`), resolveGenericItem},
}

var legacyUserFacingIdentifierAnnotation = regexp.MustCompile(`(?i)\s*\([^()]{0,64}\bID\s+[0-9]+\)`)
var trailingUserFacingIdentifier = regexp.MustCompile(`(?i)\s+(?:id\s+)?[0-9]+$`)

// Humanize replaces recognized raw identifier phrases with a user-facing name.
// Raw IDs remain available through Intent.Receipt.RawError for diagnostics and
// are deliberately absent from text serialized or displayed to users.
func (labels IdentifierLabels) Humanize(text string) string {
	text = legacyUserFacingIdentifierAnnotation.ReplaceAllString(text, "")
	for _, pattern := range userFacingIdentifierPatterns {
		text = pattern.expression.ReplaceAllStringFunc(text, func(match string) string {
			if strings.Contains(match, ":") {
				return match
			}
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
			return unresolvedIdentifierLabel(match)
		})
	}
	return legacyUserFacingIdentifierAnnotation.ReplaceAllString(text, "")
}

func unresolvedIdentifierLabel(match string) string {
	label := strings.ToLower(strings.TrimSpace(match))
	label = strings.TrimSpace(trailingUserFacingIdentifier.ReplaceAllString(label, ""))
	switch {
	case strings.Contains(label, "auto bird target"):
		return "Auto Bird target"
	case strings.Contains(label, "construction") && strings.Contains(label, "item"):
		return "construction item"
	case strings.Contains(label, "crafting recipe"), label == "recipe":
		return "crafting recipe"
	case strings.Contains(label, "hall of legends skill"):
		return "Hall of Legends skill"
	case strings.Contains(label, "skill"):
		return "skill"
	case strings.Contains(label, "building"):
		return "the selected building"
	case strings.Contains(label, "prebuilt castle"), strings.Contains(label, "event camp"), label == "camp":
		return "the camp"
	case strings.Contains(label, "storm isle"):
		return "the Storm island"
	case label == "castle":
		return "the castle"
	case label == "commander":
		return "a commander"
	case label == "castellan":
		return "a castellan"
	case label == "equipment":
		return "the equipment"
	case strings.Contains(label, "gem carrier"):
		return "the equipment"
	case label == "gem":
		return "the gem"
	case label == "movement":
		return "the movement"
	case label == "player":
		return "the player"
	case label == "alliance":
		return "the alliance"
	case strings.HasPrefix(label, "troops"), strings.HasPrefix(label, "units"):
		return "troops"
	case strings.Contains(label, "troop"), strings.Contains(label, "unit"), label == "unit":
		return "troop"
	case strings.HasPrefix(label, "tools"):
		return "tools"
	case strings.Contains(label, "tool"):
		return "tool"
	case label == "resource":
		return "resource"
	case label == "currency":
		return "currency"
	case label == "package":
		return "package"
	case label == "horse":
		return "horse"
	case label == "kingdom":
		return "kingdom"
	case label == "difficulty":
		return "difficulty"
	case label == "event":
		return "event"
	case label == "achievement":
		return "achievement"
	case label == "reward":
		return "reward"
	case label == "effect":
		return "effect"
	default:
		return "item"
	}
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
	return name, true
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
	return name, true
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
	return name, true
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
	return label, true
}

func resolvePlayer(labels IdentifierLabels, id int64) (string, bool) {
	if int64(labels.state.Player.ID) == id && strings.TrimSpace(labels.state.Player.Name) != "" {
		return strings.TrimSpace(labels.state.Player.Name), true
	}
	alliances := make([]State.AllianceState, 0, len(labels.state.Alliances)+1)
	alliances = append(alliances, labels.state.Alliance)
	for _, alliance := range labels.state.Alliances {
		alliances = append(alliances, alliance)
	}
	for _, alliance := range alliances {
		for _, member := range alliance.Members {
			if int64(member.PlayerID) == id && strings.TrimSpace(member.Name) != "" {
				return strings.TrimSpace(member.Name), true
			}
		}
	}
	return "", false
}

func resolveAlliance(labels IdentifierLabels, id int64) (string, bool) {
	if int64(labels.state.Alliance.ID) == id && strings.TrimSpace(labels.state.Alliance.Name) != "" {
		return strings.TrimSpace(labels.state.Alliance.Name), true
	}
	if alliance, found := labels.state.Alliances[State.AllianceID(id)]; found && strings.TrimSpace(alliance.Name) != "" {
		return strings.TrimSpace(alliance.Name), true
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
	return name, true
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
	return name, true
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
	return name, true
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
	return name, true
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
			return name, true
		}
	}
	if labels.store != nil {
		if definition, found := labels.store.ScalableEvent(id, 0); found {
			if name := localizedEventName(labels.language, definition.LocalizationKey, definition.Name, definition.EventType, id); name != "" {
				return name, true
			}
		}
		if catalog, err := labels.store.Catalog("events"); err == nil {
			if raw, found := catalog.FindByField("eventID", strconv.FormatInt(id, 10)); found {
				if record, decodeErr := DecodeRecord(raw); decodeErr == nil {
					if name, named := officialDefinitionRecordName(record, labels.language, "events"); named {
						return name, true
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
			resolved = name
			return false
		}
		if labels.store != nil {
			if definition, found := labels.store.ScalableEvent(eventID, id); found {
				if name := humanizeInternalName(definition.DifficultyTypeName); name != "" {
					resolved = name
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
	return name, true
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
	return name, true
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

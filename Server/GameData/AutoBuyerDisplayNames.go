package GameData

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// LocalizedAutoBuyerCatalog replaces developer-facing package comments with
// names derived from the official reward definitions and language catalog.
// The underlying Auto Buyer package identities and purchase guards are not
// changed; unresolved rows retain the capture-backed fallback label.
func (store *Store) LocalizedAutoBuyerCatalog(language *LanguageStore) (AutoBuyerCatalog, error) {
	catalog, err := store.AutoBuyerCatalog()
	if err != nil {
		return AutoBuyerCatalog{}, err
	}
	packages, err := store.Catalog("packages")
	if err != nil {
		return catalog, nil
	}
	aliases, _ := store.autoBuyerPriceAliases()
	resolver := autoBuyerDisplayResolver{
		store: store, language: language, aliases: aliases,
		definitionNames: map[string]string{}, missingDefinitions: map[string]struct{}{},
	}
	for index := range catalog.Packages {
		product := &catalog.Packages[index]
		raw, found := packages.Find(strconv.FormatInt(product.PackageID, 10))
		if !found {
			continue
		}
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		if name, detail, resolved := resolver.packageLabel(record); resolved {
			product.Name = name
			product.Detail = detail
		}
		if name, resolved := resolver.priceName(product.Price); resolved {
			product.Price.Name = name
		}
	}
	for index := range catalog.Feasts {
		if name, resolved := resolver.priceName(catalog.Feasts[index].Price); resolved {
			catalog.Feasts[index].Price.Name = name
		}
	}
	return catalog, nil
}

type autoBuyerDisplayResolver struct {
	store              *Store
	language           *LanguageStore
	aliases            map[string]autoBuyerPriceAlias
	definitionNames    map[string]string
	missingDefinitions map[string]struct{}
}

func (resolver *autoBuyerDisplayResolver) packageLabel(record Record) (string, string, bool) {
	switch strings.ToLower(strings.TrimSpace(stringValue(record, "packageType"))) {
	case "soldier", "tool":
		return resolver.unitPackageLabel(record)
	case "constructionitem":
		return resolver.definitionPackageLabel(record, "constructionItems", "constructionItemID", "constructionItemAmount")
	case "deco":
		return resolver.definitionPackageLabel(record, "buildings", "buildingID", "buildingAmount")
	case "item":
		return resolver.equipmentPackageLabel(record)
	case "gem":
		return resolver.definitionPackageLabel(record, "gems", "gemIDs", "gemAmount")
	case "lootbox":
		return resolver.lootBoxPackageLabel(record)
	case "packagebundle":
		return resolver.packageBundleLabel(record)
	case "relicitem", "relicgem":
		return resolver.relicPackageLabel(record)
	case "currency":
		return resolver.currencyPackageLabel(record)
	case "resources":
		return resolver.resourcePackageLabel(record)
	case "rewardbag":
		return resolver.rewardBagPackageLabel(record)
	case "vip":
		return resolver.vipPackageLabel(record)
	default:
		return "", "", false
	}
}

func (resolver *autoBuyerDisplayResolver) unitPackageLabel(record Record) (string, string, bool) {
	id, found := record.Int64("unitID")
	if !found || id <= 0 {
		return "", "", false
	}
	name, resolved := resolver.definitionName("units", id)
	if !resolved {
		return "", "", false
	}
	amount := positiveAutoBuyerAmount(record, "unitAmount", 1)
	return autoBuyerQuantityLabel(amount, name), "", true
}

func (resolver *autoBuyerDisplayResolver) definitionPackageLabel(
	record Record,
	collection string,
	idField string,
	amountField string,
) (string, string, bool) {
	id, found := firstAutoBuyerID(stringValue(record, idField))
	if !found {
		if numericID, numericFound := record.Int64(idField); numericFound && numericID > 0 {
			id, found = numericID, true
		}
	}
	if !found {
		return "", "", false
	}
	name, resolved := resolver.definitionName(collection, id)
	if !resolved {
		return "", "", false
	}
	amount := positiveAutoBuyerAmount(record, amountField, 1)
	return autoBuyerQuantityLabel(amount, name), "", true
}

func (resolver *autoBuyerDisplayResolver) equipmentPackageLabel(record Record) (string, string, bool) {
	id, found := firstAutoBuyerRecordID(record, "equipmentIDs", "enchantedEquipmentIDs")
	if !found {
		return "", "", false
	}
	name, resolved := resolver.definitionName("equipments", id)
	if !resolved {
		return "", "", false
	}
	amount := positiveAutoBuyerAmount(record, "equipmentAmount", 1)
	return autoBuyerQuantityLabel(amount, name), "", true
}

func (resolver *autoBuyerDisplayResolver) lootBoxPackageLabel(record Record) (string, string, bool) {
	id, amount, found := autoBuyerReference(stringValue(record, "lootBox"))
	if !found {
		return "", "", false
	}
	definition, definitionFound := resolver.definitionRecord("lootBoxes", id)
	if !definitionFound {
		return "", "", false
	}
	internalName := stringValue(definition, "name")
	rarity := positiveAutoBuyerAmount(definition, "rarity", 0)
	keys := []string{}
	if internalName != "" && rarity > 0 {
		keys = append(keys, fmt.Sprintf("mysteryBox_boxName_%s_%d", internalName, rarity))
	}
	if internalName != "" {
		keys = append(keys, "mysteryBox_boxName_"+internalName, internalName+"_name")
	}
	name, resolved := resolver.languageText(keys...)
	if !resolved {
		name, resolved = officialDefinitionRecordName(definition, resolver.language, "lootBoxes")
	}
	if !resolved || strings.TrimSpace(name) == "" {
		return "", "", false
	}
	return autoBuyerQuantityLabel(max(int64(1), amount), name), "", true
}

func (resolver *autoBuyerDisplayResolver) packageBundleLabel(record Record) (string, string, bool) {
	packageIDs := autoBuyerIDList(stringValue(record, "packageIDs"))
	if len(packageIDs) == 0 {
		return "", "", false
	}
	packages, err := resolver.store.Catalog("packages")
	if err != nil {
		return "", "", false
	}
	itemNames := []string{}
	setIDs := map[int64]struct{}{}
	for _, packageID := range packageIDs {
		raw, found := packages.Find(strconv.FormatInt(packageID, 10))
		if !found {
			continue
		}
		child, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		collection := "equipments"
		id, idFound := firstAutoBuyerRecordID(child, "equipmentIDs", "enchantedEquipmentIDs")
		if !idFound {
			collection = "gems"
			id, idFound = firstAutoBuyerRecordID(child, "gemIDs")
		}
		if !idFound {
			continue
		}
		if name, nameFound := resolver.definitionName(collection, id); nameFound {
			itemNames = append(itemNames, name)
		}
		if definition, definitionFound := resolver.definitionRecord(collection, id); definitionFound {
			if setID, setFound := definition.Int64("setID"); setFound && setID > 0 {
				setIDs[setID] = struct{}{}
			}
		}
	}
	if len(setIDs) == 1 {
		for setID := range setIDs {
			if name, found := resolver.languageText(fmt.Sprintf("equipment_set_%d", setID)); found {
				return name, fmt.Sprintf("%d-piece equipment set", len(packageIDs)), true
			}
		}
	}
	if len(itemNames) == 0 {
		return "", "", false
	}
	detail := strings.Join(itemNames, " · ")
	if len(itemNames) > 4 {
		detail = strings.Join(itemNames[:4], " · ") + fmt.Sprintf(" · +%d more", len(itemNames)-4)
	}
	return "Equipment set", detail, true
}

func (resolver *autoBuyerDisplayResolver) relicPackageLabel(record Record) (string, string, bool) {
	value := stringValue(record, "relicEquipments")
	base := value
	if separator := strings.IndexAny(base, "@,"); separator >= 0 {
		base = base[:separator]
	}
	id, err := strconv.ParseInt(strings.TrimSpace(base), 10, 64)
	if err != nil {
		return "", "", false
	}
	if id < 0 {
		id = -id
	}
	category := autoBuyerRelicCategory(id, stringValue(record, "packageType"))
	if category == "" {
		return "", "", false
	}
	wielder := "General"
	if id >= 10000 {
		wielder = "Baron"
	}
	if name, found := resolver.languageText("relicequip_dialog_" + category + wielder + "_desc"); found {
		return name, "", true
	}
	if name, found := resolver.languageText("relicequip_dialog_category_relic" + category); found {
		return name, "", true
	}
	return "", "", false
}

func (resolver *autoBuyerDisplayResolver) currencyPackageLabel(record Record) (string, string, bool) {
	components := resolver.fieldRewardLabels(record, "add")
	if len(components) != 1 {
		return "", "", false
	}
	return components[0], "", true
}

func (resolver *autoBuyerDisplayResolver) resourcePackageLabel(record Record) (string, string, bool) {
	components := resolver.fieldRewardLabels(record, "amount")
	if len(components) == 0 {
		return "", "", false
	}
	if len(components) == 1 {
		return components[0], "", true
	}
	names := make([]string, 0, len(components))
	for _, component := range components {
		name := component
		if separator := strings.Index(component, " × "); separator >= 0 {
			name = component[separator+len(" × "):]
		}
		names = append(names, name)
	}
	return strings.Join(names, " + "), strings.Join(components, " · "), true
}

func (resolver *autoBuyerDisplayResolver) rewardBagPackageLabel(record Record) (string, string, bool) {
	id, amount, found := autoBuyerReference(stringValue(record, "rewardBags"))
	if !found {
		return "", "", false
	}
	bag, bagFound := resolver.definitionRecord("rewardBags", id)
	if !bagFound {
		return "", "", false
	}
	components := resolver.fieldRewardLabels(bag, "add")
	focusedName := ""
	if focusedMaterialID, focused := bag.Int64("focusedMaterialID"); focused && focusedMaterialID > 0 {
		focusedName, _ = resolver.definitionName("currencies", focusedMaterialID)
	}
	name := "Build-item material bag"
	if focusedName != "" {
		name = focusedName + " material bag"
	} else if len(components) > 0 {
		componentNames := make([]string, 0, len(components))
		for _, component := range components {
			if separator := strings.Index(component, " × "); separator >= 0 {
				component = component[separator+len(" × "):]
			}
			componentNames = append(componentNames, component)
		}
		name = strings.Join(componentNames, " + ") + " material bag"
	}
	if amount > 1 {
		name = autoBuyerQuantityLabel(amount, name)
	}
	detail := strings.Join(components, " · ")
	if autoBuyerHasPositiveFieldPrefix(bag, "variable") {
		if detail != "" {
			detail += " · "
		}
		detail += "plus random build-item materials"
	}
	return name, detail, true
}

func (resolver *autoBuyerDisplayResolver) vipPackageLabel(record Record) (string, string, bool) {
	if points, found := record.Int64("vipPoints"); found && points > 0 {
		name, resolved := resolver.languageText("vipPoints_name")
		if !resolved {
			return "", "", false
		}
		return fmt.Sprintf("%s %s", formatAutoBuyerAmount(points), name), "", true
	}
	if seconds, found := record.Int64("vipTime"); found && seconds > 0 {
		name, resolved := resolver.languageText("vipTime_name")
		if !resolved {
			return "", "", false
		}
		if seconds%(24*60*60) == 0 {
			days := seconds / (24 * 60 * 60)
			unit := "days"
			if days == 1 {
				unit = "day"
			}
			return fmt.Sprintf("%s %s of %s", formatAutoBuyerAmount(days), unit, name), "", true
		}
		return autoBuyerQuantityLabel(seconds, name), "", true
	}
	return "", "", false
}

func (resolver *autoBuyerDisplayResolver) fieldRewardLabels(record Record, prefix string) []string {
	fields := make([]string, 0)
	for field := range record {
		if strings.HasPrefix(field, prefix) {
			if amount, found := record.Int64(field); found && amount > 0 {
				fields = append(fields, field)
			}
		}
	}
	sort.Strings(fields)
	result := make([]string, 0, len(fields))
	for _, field := range fields {
		amount, _ := record.Int64(field)
		suffix := strings.TrimPrefix(field, prefix)
		name, found := resolver.rewardName(suffix)
		if !found {
			continue
		}
		result = append(result, autoBuyerQuantityLabel(amount, name))
	}
	return result
}

func (resolver *autoBuyerDisplayResolver) rewardName(suffix string) (string, bool) {
	if alias, found := resolver.aliases[normalizeAutoBuyerAlias(suffix)]; found {
		if alias.resourceID > 0 {
			if name, resolved := resolver.definitionName("resources", alias.resourceID); resolved {
				return name, true
			}
		}
		if alias.currencyID > 0 {
			if name, resolved := resolver.definitionName("currencies", alias.currencyID); resolved {
				return name, true
			}
		}
	}
	if name, found := resolver.languageText("currency_name_"+suffix, suffix+"_name", suffix); found {
		return name, true
	}
	return "", false
}

func (resolver *autoBuyerDisplayResolver) priceName(price AutoBuyerPrice) (string, bool) {
	if price.ResourceID > 0 {
		return resolver.definitionName("resources", price.ResourceID)
	}
	if price.CurrencyID > 0 {
		return resolver.definitionName("currencies", price.CurrencyID)
	}
	return "", false
}

func (resolver *autoBuyerDisplayResolver) definitionName(collection string, id int64) (string, bool) {
	cacheKey := collection + ":" + strconv.FormatInt(id, 10)
	if name, found := resolver.definitionNames[cacheKey]; found {
		return name, true
	}
	if _, missing := resolver.missingDefinitions[cacheKey]; missing {
		return "", false
	}
	record, found := resolver.definitionRecord(collection, id)
	if !found {
		resolver.missingDefinitions[cacheKey] = struct{}{}
		return "", false
	}
	name, resolved := resolver.specialDefinitionName(collection, id, record)
	if !resolved {
		name, resolved = officialDefinitionRecordName(record, resolver.language, collection)
	}
	if !resolved || strings.TrimSpace(name) == "" {
		resolver.missingDefinitions[cacheKey] = struct{}{}
		return "", false
	}
	name = strings.TrimSpace(name)
	resolver.definitionNames[cacheKey] = name
	return name, true
}

func (resolver *autoBuyerDisplayResolver) specialDefinitionName(collection string, id int64, record Record) (string, bool) {
	switch collection {
	case "units":
		values := []string{stringValue(record, "type"), stringValue(record, "comment2"), stringValue(record, "name")}
		keys := make([]string, 0, len(values)*2)
		for _, value := range values {
			if value != "" {
				keys = append(keys, value+"_name", value)
			}
		}
		return resolver.languageText(keys...)
	case "buildings":
		buildingType := stringValue(record, "type")
		name := stringValue(record, "name")
		return resolver.languageText("deco_"+buildingType+"_name", buildingType+"_name", "building_"+buildingType+"_name", name+"_name")
	case "constructionItems":
		name := stringValue(record, "name")
		comment := strings.ToLower(stringValue(record, "comment1"))
		prefixes := []string{"ci_primary_", "ci_secondary_", "ci_appearance_", "ci_blueprint_"}
		if strings.Contains(comment, "appearance") || strings.Contains(comment, "temporary") {
			prefixes = []string{"ci_appearance_", "ci_primary_", "ci_secondary_", "ci_blueprint_"}
		} else if strings.Contains(comment, "secondary") {
			prefixes = []string{"ci_secondary_", "ci_primary_", "ci_appearance_", "ci_blueprint_"}
		} else if strings.Contains(comment, "blueprint") {
			prefixes = []string{"ci_blueprint_", "ci_primary_", "ci_secondary_", "ci_appearance_"}
		}
		keys := make([]string, 0, len(prefixes)+2)
		for _, prefix := range prefixes {
			keys = append(keys, prefix+name)
		}
		keys = append(keys, name+"_name", name)
		resolved, found := resolver.languageText(keys...)
		if found {
			if level, hasLevel := record.Int64("level"); hasLevel && level > 0 {
				resolved = fmt.Sprintf("%s (level %d)", resolved, level)
			}
			return resolved, true
		}
	case "equipments":
		return resolver.languageText(fmt.Sprintf("equipment_unique_%d", id), fmt.Sprintf("hero_unique_%d", id))
	case "gems":
		if name, found := resolver.languageText(fmt.Sprintf("gem_unique_%d", id)); found {
			return name, true
		}
		return resolver.gemEffectName(record)
	case "currencies":
		values := []string{stringValue(record, "Name"), stringValue(record, "name"), stringValue(record, "assetName"), stringValue(record, "JSONKey")}
		keys := make([]string, 0, len(values)*2)
		for _, value := range values {
			if value != "" {
				keys = append(keys, "currency_name_"+value, value+"_name")
			}
		}
		return resolver.languageText(keys...)
	case "resources":
		if strings.EqualFold(stringValue(record, "JSONKey"), "C2") {
			if name, found := resolver.languageText("gold", "webshop_offer_hardCurrencyBundles_currency_rubies"); found {
				return name, true
			}
			return "Rubies", true
		}
		values := []string{stringValue(record, "name"), stringValue(record, "assetName"), stringValue(record, "JSONKey")}
		keys := make([]string, 0, len(values)*2)
		for _, value := range values {
			if value != "" {
				keys = append(keys, "currency_name_"+value, value+"_name")
			}
		}
		return resolver.languageText(keys...)
	}
	return "", false
}

func (resolver *autoBuyerDisplayResolver) gemEffectName(record Record) (string, bool) {
	effects := stringValue(record, "effects")
	first := strings.Split(effects, ",")[0]
	parts := strings.Split(first, "&")
	if len(parts) < 2 {
		return "", false
	}
	effectID, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || effectID <= 0 {
		return "", false
	}
	effect, found := resolver.definitionRecord("effects", effectID)
	if !found {
		return "", false
	}
	effectName := stringValue(effect, "name")
	if effectName == "" {
		return "", false
	}
	triggerChance := positiveAutoBuyerAmount(record, "triggerChance", 100)
	key := fmt.Sprintf("gem_effect_name_gem%s_%d", upperAutoBuyerFirst(effectName), triggerChance)
	name, resolved := resolver.languageText(key)
	if !resolved {
		return "", false
	}
	return strings.ReplaceAll(name, "{0}", strings.TrimSpace(parts[1])), true
}

func (resolver *autoBuyerDisplayResolver) definitionRecord(collection string, id int64) (Record, bool) {
	if resolver.store == nil || id <= 0 {
		return nil, false
	}
	catalog, err := resolver.store.Catalog(collection)
	if err != nil {
		return nil, false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return nil, false
	}
	record, err := DecodeRecord(raw)
	return record, err == nil
}

func (resolver *autoBuyerDisplayResolver) languageText(keys ...string) (string, bool) {
	if resolver.language == nil {
		return "", false
	}
	if value, found := resolver.language.Resolve(keys...); found && strings.TrimSpace(value) != "" {
		return strings.TrimSpace(value), true
	}
	resolved := resolver.language.ResolveMany(keys)
	for _, key := range keys {
		if value := strings.TrimSpace(resolved[key]); value != "" {
			return value, true
		}
	}
	return "", false
}

func autoBuyerRelicCategory(id int64, packageType string) string {
	if strings.EqualFold(strings.TrimSpace(packageType), "relicGem") {
		return "Gem"
	}
	if id >= 16000 || id >= 6000 && id < 10000 {
		return "Hero"
	}
	if id >= 13000 && id < 14000 || id >= 3000 && id < 4000 {
		return "Gem"
	}
	if id >= 10000 {
		switch (id - 10000) / 100 {
		case 0:
			return "Armor"
		case 1:
			return "Weapon"
		case 2:
			return "Helmet"
		case 3:
			return "Artifact"
		}
	}
	switch id / 100 {
	case 0:
		return "Armor"
	case 1:
		return "Weapon"
	case 2:
		return "Helmet"
	case 3:
		return "Artifact"
	}
	return ""
}

func autoBuyerReference(value string) (int64, int64, bool) {
	parts := strings.Split(strings.TrimSpace(value), "+")
	if len(parts) == 0 {
		return 0, 0, false
	}
	id, err := strconv.ParseInt(strings.TrimSpace(parts[0]), 10, 64)
	if err != nil || id <= 0 {
		return 0, 0, false
	}
	amount := int64(1)
	if len(parts) > 1 {
		if parsed, parseErr := strconv.ParseInt(strings.TrimSpace(parts[1]), 10, 64); parseErr == nil && parsed > 0 {
			amount = parsed
		}
	}
	return id, amount, true
}

func firstAutoBuyerID(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if separator := strings.IndexAny(value, ",+#@"); separator >= 0 {
		value = value[:separator]
	}
	id, err := strconv.ParseInt(strings.TrimSpace(value), 10, 64)
	return id, err == nil && id > 0
}

func firstAutoBuyerRecordID(record Record, fields ...string) (int64, bool) {
	for _, field := range fields {
		if id, found := firstAutoBuyerID(stringValue(record, field)); found {
			return id, true
		}
		if id, found := record.Int64(field); found && id > 0 {
			return id, true
		}
	}
	return 0, false
}

func autoBuyerIDList(value string) []int64 {
	parts := strings.FieldsFunc(value, func(character rune) bool {
		return character == ',' || character == '#'
	})
	result := make([]int64, 0, len(parts))
	for _, part := range parts {
		if id, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64); err == nil && id > 0 {
			result = append(result, id)
		}
	}
	return result
}

func positiveAutoBuyerAmount(record Record, field string, fallback int64) int64 {
	if amount, found := record.Int64(field); found && amount > 0 {
		return amount
	}
	return fallback
}

func autoBuyerHasPositiveFieldPrefix(record Record, prefix string) bool {
	for field := range record {
		if strings.HasPrefix(field, prefix) {
			if amount, found := record.Int64(field); found && amount > 0 {
				return true
			}
		}
	}
	return false
}

func autoBuyerQuantityLabel(amount int64, name string) string {
	name = strings.TrimSpace(name)
	if amount <= 1 {
		return name
	}
	return fmt.Sprintf("%s × %s", formatAutoBuyerAmount(amount), name)
}

func formatAutoBuyerAmount(value int64) string {
	digits := strconv.FormatInt(value, 10)
	if len(digits) <= 3 {
		return digits
	}
	first := len(digits) % 3
	if first == 0 {
		first = 3
	}
	var result strings.Builder
	result.WriteString(digits[:first])
	for index := first; index < len(digits); index += 3 {
		result.WriteByte(',')
		result.WriteString(digits[index : index+3])
	}
	return result.String()
}

func upperAutoBuyerFirst(value string) string {
	if value == "" {
		return ""
	}
	return strings.ToUpper(value[:1]) + value[1:]
}

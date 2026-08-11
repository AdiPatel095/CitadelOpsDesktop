package GameData

import (
	"fmt"
	"sort"
	"strings"
)

const (
	AutoBuyerShopMasterBlacksmith  = "master-blacksmith"
	AutoBuyerShopRift              = "rift"
	AutoBuyerShopNomad             = "nomad"
	AutoBuyerShopTravelingMerchant = "traveling-merchant"
	AutoBuyerShopArmorer           = "armorer"
	AutoBuyerShopEquipmentTrader   = "equipment-trader"

	AutoBuyerMasterBlacksmithTableID  int64 = 116
	AutoBuyerNomadTableID             int64 = 94
	AutoBuyerTravelingMerchantTableID int64 = 22
	AutoBuyerArmorerTableID           int64 = 27
	AutoBuyerEquipmentTraderTableID   int64 = 101

	AutoBuyerPricePlayerResource = "playerResource"
	AutoBuyerPriceCastleResource = "castleResource"
	AutoBuyerPriceCurrency       = "currency"
)

// AutoBuyerCatalog is the bounded, capture-backed subset of official shop data
// that Auto Buyer may use. Packages with ambiguous prices or unknown table
// ownership are intentionally omitted.
type AutoBuyerCatalog struct {
	Shops       []AutoBuyerShop       `json:"shops"`
	Packages    []AutoBuyerPackage    `json:"packages"`
	Specialists []AutoBuyerSpecialist `json:"specialists"`
	Feasts      []AutoBuyerFeast      `json:"feasts"`
	TimedOffers AutoBuyerCapability   `json:"timedOffers"`
}

type AutoBuyerCapability struct {
	Supported bool   `json:"supported"`
	Reason    string `json:"reason,omitempty"`
}

type AutoBuyerShop struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	TableID          int64  `json:"tableId"`
	RequiresEvent    bool   `json:"requiresEvent"`
	ResetTracking    string `json:"resetTracking"`
	PackageCount     int    `json:"packageCount"`
	UnsupportedCount int    `json:"unsupportedCount,omitempty"`
}

type AutoBuyerPrice struct {
	Field      string `json:"field"`
	JSONKey    string `json:"jsonKey,omitempty"`
	Name       string `json:"name"`
	Scope      string `json:"scope"`
	ResourceID int64  `json:"resourceId,omitempty"`
	CurrencyID int64  `json:"currencyId,omitempty"`
	Amount     int64  `json:"amount"`
	Premium    bool   `json:"premium"`
}

type AutoBuyerPackage struct {
	ShopID         string         `json:"shopId"`
	ShopName       string         `json:"shopName"`
	ShopKind       string         `json:"shopKind"`
	TableID        int64          `json:"tableId"`
	RequiresEvent  bool           `json:"requiresEvent"`
	PackageID      int64          `json:"packageId"`
	PackageType    string         `json:"packageType,omitempty"`
	Name           string         `json:"name"`
	Detail         string         `json:"detail,omitempty"`
	Stock          int64          `json:"stock"`
	MaxBuyPerClick int64          `json:"maxBuyPerClick,omitempty"`
	MinLevel       int64          `json:"minLevel,omitempty"`
	MaxLevel       int64          `json:"maxLevel,omitempty"`
	MinLegendLevel int64          `json:"minLegendLevel,omitempty"`
	MaxLegendLevel int64          `json:"maxLegendLevel,omitempty"`
	Price          AutoBuyerPrice `json:"price"`
}

type AutoBuyerSpecialist struct {
	ID           int    `json:"id"`
	Key          string `json:"key"`
	Name         string `json:"name"`
	DurationSec  int64  `json:"durationSec"`
	BaseRubyCost int64  `json:"baseRubyCost"`
	BonusPercent int64  `json:"bonusPercent,omitempty"`
	Opcode       string `json:"-"`
	ResourceType int    `json:"-"`
}

type AutoBuyerFeast struct {
	ID                     int64          `json:"id"`
	Name                   string         `json:"name"`
	Type                   string         `json:"type,omitempty"`
	DurationSec            int64          `json:"durationSec"`
	ProductionBoostPercent int64          `json:"productionBoostPercent"`
	MinLevel               int64          `json:"minLevel,omitempty"`
	MaxLevel               int64          `json:"maxLevel,omitempty"`
	Price                  AutoBuyerPrice `json:"price"`
}

type autoBuyerShopDefinition struct {
	id            string
	name          string
	kind          string
	tableID       int64
	requiresEvent bool
	match         func(string, string) bool
}

var autoBuyerShopDefinitions = []autoBuyerShopDefinition{
	{
		id: AutoBuyerShopRift, name: "Rift shop", kind: "event",
		tableID: AutoBuyerMasterBlacksmithTableID, requiresEvent: true,
		match: func(comment1, _ string) bool {
			return strings.Contains(strings.ToLower(comment1), "are blacksmith")
		},
	},
	{
		id: AutoBuyerShopMasterBlacksmith, name: "Master Blacksmith", kind: "merchant",
		tableID: AutoBuyerMasterBlacksmithTableID,
		match: func(comment1, comment2 string) bool {
			text := strings.ToLower(comment1 + " " + comment2)
			return strings.Contains(text, "master blacksmith") ||
				strings.Contains(text, "central silver shop") ||
				strings.Contains(text, "central gold shop")
		},
	},
	{
		id: AutoBuyerShopNomad, name: "Nomad shop", kind: "event",
		tableID: AutoBuyerNomadTableID, requiresEvent: true,
		match: func(comment1, comment2 string) bool {
			return strings.EqualFold(strings.TrimSpace(comment2), "Nomad Invasion Vendor") ||
				strings.Contains(strings.ToLower(comment1), "nomad eds shop")
		},
	},
	{
		id: AutoBuyerShopTravelingMerchant, name: "Traveling Merchant", kind: "merchant",
		tableID: AutoBuyerTravelingMerchantTableID,
		match: func(_, comment2 string) bool {
			return strings.HasPrefix(strings.ToLower(strings.TrimSpace(comment2)), "traveling merchant")
		},
	},
	{
		id: AutoBuyerShopArmorer, name: "Armorer", kind: "merchant",
		tableID: AutoBuyerArmorerTableID,
		match: func(_, comment2 string) bool {
			return strings.EqualFold(strings.TrimSpace(comment2), "Armorer")
		},
	},
	{
		id: AutoBuyerShopEquipmentTrader, name: "Equipment Trader", kind: "merchant",
		tableID: AutoBuyerEquipmentTraderTableID,
		match: func(_, comment2 string) bool {
			return strings.HasPrefix(strings.ToLower(strings.TrimSpace(comment2)), "equipment trader")
		},
	},
}

// AutoBuyerCatalog returns a copy of the capture-backed Auto Buyer projection.
func (store *Store) AutoBuyerCatalog() (AutoBuyerCatalog, error) {
	if store == nil {
		return AutoBuyerCatalog{}, fmt.Errorf("official game data is unavailable")
	}
	store.autoBuyerOnce.Do(func() {
		store.autoBuyerCatalog, store.autoBuyerProducts, store.autoBuyerFeasts, store.autoBuyerErr = store.loadAutoBuyerCatalog()
	})
	if store.autoBuyerErr != nil {
		return AutoBuyerCatalog{}, store.autoBuyerErr
	}
	return copyAutoBuyerCatalog(store.autoBuyerCatalog), nil
}

func (store *Store) AutoBuyerPackage(shopID string, packageID int64) (AutoBuyerPackage, bool) {
	if store == nil || packageID <= 0 {
		return AutoBuyerPackage{}, false
	}
	if _, err := store.AutoBuyerCatalog(); err != nil {
		return AutoBuyerPackage{}, false
	}
	product, found := store.autoBuyerProducts[strings.TrimSpace(shopID)][packageID]
	return product, found
}

func (store *Store) AutoBuyerFeast(feastID int64) (AutoBuyerFeast, bool) {
	if store == nil || feastID < 0 {
		return AutoBuyerFeast{}, false
	}
	if _, err := store.AutoBuyerCatalog(); err != nil {
		return AutoBuyerFeast{}, false
	}
	feast, found := store.autoBuyerFeasts[feastID]
	return feast, found
}

func AutoBuyerSpecialistByID(id int) (AutoBuyerSpecialist, bool) {
	for _, specialist := range autoBuyerSpecialists() {
		if specialist.ID == id {
			return specialist, true
		}
	}
	return AutoBuyerSpecialist{}, false
}

func (store *Store) loadAutoBuyerCatalog() (
	AutoBuyerCatalog,
	map[string]map[int64]AutoBuyerPackage,
	map[int64]AutoBuyerFeast,
	error,
) {
	packages, err := store.Catalog("packages")
	if err != nil {
		return AutoBuyerCatalog{}, nil, nil, err
	}
	aliases, err := store.autoBuyerPriceAliases()
	if err != nil {
		return AutoBuyerCatalog{}, nil, nil, err
	}
	projection := AutoBuyerCatalog{
		Specialists: autoBuyerSpecialists(),
		TimedOffers: AutoBuyerCapability{
			Supported: false,
			Reason:    "Timed offers require a server-quoted confirmation and are not enabled for unattended purchases yet.",
		},
	}
	byPackage := map[string]map[int64]AutoBuyerPackage{}
	shopIndex := map[string]int{}
	for _, definition := range autoBuyerShopDefinitions {
		shop := AutoBuyerShop{
			ID: definition.id, Name: definition.name, Kind: definition.kind,
			TableID: definition.tableID, RequiresEvent: definition.requiresEvent,
			ResetTracking: "serverPurchaseCounters",
		}
		projection.Shops = append(projection.Shops, shop)
		shopIndex[definition.id] = len(projection.Shops) - 1
		byPackage[definition.id] = map[int64]AutoBuyerPackage{}
	}
	for _, raw := range packages.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		comment1 := strings.TrimSpace(stringValue(record, "comment1"))
		comment2 := strings.TrimSpace(stringValue(record, "comment2"))
		for _, shop := range autoBuyerShopDefinitions {
			if !shop.match(comment1, comment2) {
				continue
			}
			product, supported := decodeAutoBuyerPackage(record, shop, aliases)
			if !supported {
				index := shopIndex[shop.id]
				projection.Shops[index].UnsupportedCount++
				break
			}
			projection.Packages = append(projection.Packages, product)
			byPackage[shop.id][product.PackageID] = product
			index := shopIndex[shop.id]
			projection.Shops[index].PackageCount++
			break
		}
	}
	sort.Slice(projection.Packages, func(left, right int) bool {
		if projection.Packages[left].ShopID != projection.Packages[right].ShopID {
			return projection.Packages[left].ShopID < projection.Packages[right].ShopID
		}
		return projection.Packages[left].PackageID < projection.Packages[right].PackageID
	})

	feasts, byFeast, feastErr := store.loadAutoBuyerFeasts(aliases)
	if feastErr != nil {
		return AutoBuyerCatalog{}, nil, nil, feastErr
	}
	projection.Feasts = feasts
	return projection, byPackage, byFeast, nil
}

type autoBuyerPriceAlias struct {
	jsonKey    string
	name       string
	resourceID int64
	currencyID int64
}

func (store *Store) autoBuyerPriceAliases() (map[string]autoBuyerPriceAlias, error) {
	result := map[string]autoBuyerPriceAlias{}
	for _, collection := range []string{"resources", "currencies"} {
		catalog, err := store.Catalog(collection)
		if err != nil {
			return nil, fmt.Errorf("load official %s: %w", collection, err)
		}
		for _, raw := range catalog.Rows() {
			record, decodeErr := DecodeRecord(raw)
			if decodeErr != nil {
				continue
			}
			alias := autoBuyerPriceAlias{
				jsonKey: strings.TrimSpace(stringValue(record, "JSONKey")),
			}
			if collection == "resources" {
				alias.resourceID, _ = record.Int64("resourceID")
				alias.name = strings.TrimSpace(stringValue(record, "name"))
			} else {
				alias.currencyID, _ = record.Int64("currencyID")
				alias.name = strings.TrimSpace(stringValue(record, "Name"))
				if alias.name == "" {
					alias.name = strings.TrimSpace(stringValue(record, "name"))
				}
			}
			if alias.resourceID <= 0 && alias.currencyID <= 0 {
				continue
			}
			assetName := strings.TrimSpace(stringValue(record, "assetName"))
			for _, value := range []string{alias.jsonKey, alias.name, assetName} {
				if key := normalizeAutoBuyerAlias(value); key != "" {
					// Currency names and resource names can overlap. Compact JSON keys
					// are authoritative; otherwise currencies win for costX fields.
					existing, exists := result[key]
					if !exists || value == alias.jsonKey || alias.currencyID > 0 && existing.currencyID == 0 {
						result[key] = alias
					}
				}
			}
		}
	}
	return result, nil
}

func decodeAutoBuyerPackage(
	record Record,
	shop autoBuyerShopDefinition,
	aliases map[string]autoBuyerPriceAlias,
) (AutoBuyerPackage, bool) {
	packageID, hasPackageID := record.Int64("packageID")
	stock, hasStock := record.Int64("stock")
	if !hasPackageID || packageID <= 0 || !hasStock || stock <= 0 {
		// GBC is the authoritative reset signal. Unlimited or untracked items
		// cannot be retried safely without a separate purchase ledger.
		return AutoBuyerPackage{}, false
	}
	price, validPrice := decodeAutoBuyerPackagePrice(record, aliases)
	if !validPrice {
		return AutoBuyerPackage{}, false
	}
	comment1 := strings.TrimSpace(stringValue(record, "comment1"))
	comment2 := strings.TrimSpace(stringValue(record, "comment2"))
	name, detail := autoBuyerPackageLabels(comment1, comment2)
	if name == "" {
		name = fmt.Sprintf("Package %d", packageID)
	}
	packageType := strings.TrimSpace(stringValue(record, "packageType"))
	maxBuyPerClick, _ := record.Int64("maxBuyPerClick")
	minLevel, _ := record.Int64("minLevel")
	maxLevel, _ := record.Int64("maxLevel")
	minLegendLevel, _ := record.Int64("minLegendLevel")
	maxLegendLevel, _ := record.Int64("maxLegendLevel")
	return AutoBuyerPackage{
		ShopID: shop.id, ShopName: shop.name, ShopKind: shop.kind,
		TableID: shop.tableID, RequiresEvent: shop.requiresEvent,
		PackageID: packageID, PackageType: packageType, Name: name, Detail: detail,
		Stock: stock, MaxBuyPerClick: max(int64(0), maxBuyPerClick),
		MinLevel: minLevel, MaxLevel: maxLevel,
		MinLegendLevel: minLegendLevel, MaxLegendLevel: maxLegendLevel,
		Price: price,
	}, true
}

func decodeAutoBuyerPackagePrice(record Record, aliases map[string]autoBuyerPriceAlias) (AutoBuyerPrice, bool) {
	prices := []AutoBuyerPrice{}
	unsupported := false
	for field := range record {
		if !strings.HasPrefix(field, "cost") && !strings.HasPrefix(field, "packagePrice") {
			continue
		}
		amount, found := record.Int64(field)
		if !found || amount <= 0 {
			continue
		}
		price, supported := autoBuyerPriceForField(field, amount, aliases)
		if !supported {
			unsupported = true
			continue
		}
		prices = append(prices, price)
	}
	if unsupported || len(prices) != 1 {
		return AutoBuyerPrice{}, false
	}
	return prices[0], true
}

func autoBuyerPriceForField(
	field string,
	amount int64,
	aliases map[string]autoBuyerPriceAlias,
) (AutoBuyerPrice, bool) {
	var suffix string
	switch {
	case strings.HasPrefix(field, "packagePrice"):
		suffix = strings.TrimPrefix(field, "packagePrice")
	case strings.HasPrefix(field, "cost"):
		suffix = strings.TrimPrefix(field, "cost")
	default:
		return AutoBuyerPrice{}, false
	}
	alias, found := aliases[normalizeAutoBuyerAlias(suffix)]
	if !found {
		return AutoBuyerPrice{}, false
	}
	price := AutoBuyerPrice{Field: field, JSONKey: alias.jsonKey, Name: alias.name, Amount: amount}
	if alias.resourceID > 0 {
		price.ResourceID = alias.resourceID
		if strings.EqualFold(alias.jsonKey, "C1") || strings.EqualFold(alias.jsonKey, "C2") {
			price.Scope = AutoBuyerPricePlayerResource
			price.Premium = strings.EqualFold(alias.jsonKey, "C2")
		} else {
			price.Scope = AutoBuyerPriceCastleResource
		}
	} else if alias.currencyID > 0 {
		price.Scope = AutoBuyerPriceCurrency
		price.CurrencyID = alias.currencyID
	}
	price.Name = autoBuyerPriceName(alias, suffix)
	return price, price.Scope != ""
}

func (store *Store) loadAutoBuyerFeasts(
	aliases map[string]autoBuyerPriceAlias,
) ([]AutoBuyerFeast, map[int64]AutoBuyerFeast, error) {
	catalog, err := store.Catalog("feasts")
	if err != nil {
		return nil, nil, err
	}
	result := []AutoBuyerFeast{}
	byID := map[int64]AutoBuyerFeast{}
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, hasID := record.Int64("feastID")
		duration, hasDuration := record.Int64("duration")
		if !hasID || id < 0 || !hasDuration || duration <= 0 {
			continue
		}
		price, validPrice := decodeAutoBuyerFeastPrice(record, aliases)
		if !validPrice {
			continue
		}
		boost, _ := record.Int64("productionBoost")
		minLevel, _ := record.Int64("minLevel")
		maxLevel, _ := record.Int64("maxLevel")
		feastType := strings.TrimSpace(stringValue(record, "type"))
		name := strings.TrimSpace(stringValue(record, "comment"))
		if name == "" {
			name = strings.TrimSpace(stringValue(record, "name"))
		}
		if name == "" {
			name = autoBuyerFeastName(feastType, id)
		}
		feast := AutoBuyerFeast{
			ID: id, Name: name, Type: feastType, DurationSec: duration,
			ProductionBoostPercent: boost, MinLevel: minLevel, MaxLevel: maxLevel, Price: price,
		}
		result = append(result, feast)
		byID[id] = feast
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result, byID, nil
}

func decodeAutoBuyerFeastPrice(record Record, aliases map[string]autoBuyerPriceAlias) (AutoBuyerPrice, bool) {
	fields := []string{}
	for _, field := range []string{"costFood", "costC2"} {
		if amount, found := record.Int64(field); found && amount > 0 {
			fields = append(fields, field)
		}
	}
	if len(fields) != 1 {
		return AutoBuyerPrice{}, false
	}
	amount, _ := record.Int64(fields[0])
	return autoBuyerPriceForField(fields[0], amount, aliases)
}

func autoBuyerSpecialists() []AutoBuyerSpecialist {
	const week = int64(7 * 24 * 60 * 60)
	return []AutoBuyerSpecialist{
		{ID: 0, Key: "wood-overseer", Name: "Wood overseer", DurationSec: week, BaseRubyCost: 625, BonusPercent: 25, Opcode: "ovs", ResourceType: 0},
		{ID: 1, Key: "stone-overseer", Name: "Stone overseer", DurationSec: week, BaseRubyCost: 625, BonusPercent: 25, Opcode: "ovs", ResourceType: 1},
		{ID: 2, Key: "food-overseer", Name: "Food overseer", DurationSec: week, BaseRubyCost: 625, BonusPercent: 25, Opcode: "ovs", ResourceType: 2},
		{ID: 3, Key: "honey-overseer", Name: "Honey overseer", DurationSec: week, BaseRubyCost: 625, BonusPercent: 25, Opcode: "ovs", ResourceType: 3},
		{ID: 4, Key: "mead-overseer", Name: "Mead overseer", DurationSec: week, BaseRubyCost: 625, BonusPercent: 25, Opcode: "ovs", ResourceType: 4},
		{ID: 5, Key: "beef-overseer", Name: "Beef overseer", DurationSec: week, BaseRubyCost: 4900, BonusPercent: 125, Opcode: "ovs", ResourceType: 5},
		{ID: 6, Key: "marauder", Name: "Marauder", DurationSec: week, BaseRubyCost: 990, BonusPercent: 90, Opcode: "bms"},
		{ID: 8, Key: "tax-collector", Name: "Tax collector", DurationSec: week, BaseRubyCost: 750, BonusPercent: 20, Opcode: "btx"},
		{ID: 10, Key: "drill-instructor", Name: "Drill instructor", DurationSec: week, BaseRubyCost: 990, BonusPercent: 80, Opcode: "bis"},
	}
}

func normalizeAutoBuyerAlias(value string) string {
	return strings.Map(func(character rune) rune {
		if character >= 'a' && character <= 'z' || character >= 'A' && character <= 'Z' || character >= '0' && character <= '9' {
			return character
		}
		return -1
	}, strings.ToLower(strings.TrimSpace(value)))
}

func autoBuyerPackageLabels(comment1, comment2 string) (string, string) {
	if autoBuyerShopLabel(comment1) && comment2 != "" && !autoBuyerShopLabel(comment2) {
		return autoBuyerHumanize(comment2), ""
	}
	if autoBuyerShopLabel(comment2) && comment1 != "" && !autoBuyerShopLabel(comment1) {
		return autoBuyerHumanize(comment1), ""
	}
	if comment2 != "" {
		return autoBuyerHumanize(comment2), autoBuyerHumanize(comment1)
	}
	return autoBuyerHumanize(comment1), autoBuyerHumanize(comment2)
}

func autoBuyerShopLabel(value string) bool {
	text := strings.ToLower(strings.TrimSpace(value))
	return strings.Contains(text, "master blacksmith") ||
		strings.Contains(text, "central silver shop") ||
		strings.Contains(text, "central gold shop") ||
		strings.Contains(text, "are blacksmith") ||
		strings.Contains(text, "nomad eds shop") ||
		strings.EqualFold(text, "nomad invasion vendor") ||
		strings.HasPrefix(text, "traveling merchant") ||
		strings.HasPrefix(text, "armorer") ||
		strings.HasPrefix(text, "equipment trader")
}

func autoBuyerPriceName(alias autoBuyerPriceAlias, fallback string) string {
	switch strings.ToUpper(strings.TrimSpace(alias.jsonKey)) {
	case "C1":
		return "Coins"
	case "C2":
		return "Rubies"
	case "W":
		return "Wood"
	case "S":
		return "Stone"
	case "F":
		return "Food"
	case "HONEY":
		return "Honey"
	case "MEAD":
		return "Mead"
	case "BEEF":
		return "Beef"
	}
	if name := autoBuyerHumanize(alias.name); name != "" {
		return name
	}
	return autoBuyerHumanize(fallback)
}

func autoBuyerFeastName(feastType string, id int64) string {
	labels := map[string]string{
		"small": "Small feast", "medium": "Medium feast", "big": "Large feast", "fourth": "Grand feast", "ruby": "Ruby feast",
		"smalllevel2": "Legendary small feast", "mediumlevel2": "Legendary medium feast", "biglevel2": "Legendary large feast",
		"fourthlevel2": "Legendary grand feast", "rubylevel2": "Legendary ruby feast",
	}
	if name := labels[strings.ToLower(strings.TrimSpace(feastType))]; name != "" {
		return name
	}
	return fmt.Sprintf("Feast %d", id)
}

func autoBuyerHumanize(value string) string {
	value = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(value, "_", " "), "-", " "))
	if value == "" {
		return ""
	}
	var result []rune
	var previous rune
	for index, character := range []rune(value) {
		if index > 0 && character >= 'A' && character <= 'Z' &&
			(previous >= 'a' && previous <= 'z' || previous >= '0' && previous <= '9') {
			result = append(result, ' ')
		}
		result = append(result, character)
		previous = character
	}
	return strings.Join(strings.Fields(string(result)), " ")
}

func copyAutoBuyerCatalog(source AutoBuyerCatalog) AutoBuyerCatalog {
	clone := source
	clone.Shops = append([]AutoBuyerShop(nil), source.Shops...)
	clone.Packages = append([]AutoBuyerPackage(nil), source.Packages...)
	clone.Specialists = append([]AutoBuyerSpecialist(nil), source.Specialists...)
	clone.Feasts = append([]AutoBuyerFeast(nil), source.Feasts...)
	return clone
}

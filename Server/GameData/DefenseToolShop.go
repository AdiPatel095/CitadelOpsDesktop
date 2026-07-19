package GameData

import (
	"fmt"
	"sort"
	"strings"
)

const (
	DefenseToolPricePlayerResource = "playerResource"
	DefenseToolPriceCastleResource = "castleResource"
	DefenseToolPriceCurrency       = "currency"
)

type DefenseToolShopPackage struct {
	PackageID      int64  `json:"packageId"`
	ToolID         int64  `json:"toolId"`
	ToolAmount     int64  `json:"toolAmount"`
	Price          int64  `json:"price"`
	PriceScope     string `json:"priceScope"`
	PriceID        int64  `json:"priceId"`
	PriceName      string `json:"priceName"`
	Stock          int64  `json:"stock,omitempty"`
	MaxBuyPerClick int64  `json:"maxBuyPerClick,omitempty"`
	Name           string `json:"name"`
}

func (store *Store) DefenseToolShopPackages(toolID int64) ([]DefenseToolShopPackage, error) {
	if store == nil || toolID <= 0 {
		return []DefenseToolShopPackage{}, nil
	}
	store.defenseToolShopOnce.Do(func() {
		store.defenseToolShopProducts, store.defenseToolShopErr = store.loadDefenseToolShopPackages()
	})
	if store.defenseToolShopErr != nil {
		return nil, store.defenseToolShopErr
	}
	return append([]DefenseToolShopPackage(nil), store.defenseToolShopProducts[toolID]...), nil
}

func (store *Store) DefenseToolShopPackage(packageID int64) (DefenseToolShopPackage, bool) {
	if store == nil || packageID <= 0 {
		return DefenseToolShopPackage{}, false
	}
	store.defenseToolShopOnce.Do(func() {
		store.defenseToolShopProducts, store.defenseToolShopErr = store.loadDefenseToolShopPackages()
	})
	if store.defenseToolShopErr != nil {
		return DefenseToolShopPackage{}, false
	}
	for _, packages := range store.defenseToolShopProducts {
		for _, candidate := range packages {
			if candidate.PackageID == packageID {
				return candidate, true
			}
		}
	}
	return DefenseToolShopPackage{}, false
}

func (store *Store) loadDefenseToolShopPackages() (map[int64][]DefenseToolShopPackage, error) {
	packages, err := store.Catalog("packages")
	if err != nil {
		return nil, err
	}
	currencyReferences, err := store.defenseToolCurrencyReferences()
	if err != nil {
		return nil, err
	}
	result := map[int64][]DefenseToolShopPackage{}
	for _, raw := range packages.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, valid := decodeDefenseToolShopPackage(record, currencyReferences)
		if !valid {
			continue
		}
		result[candidate.ToolID] = append(result[candidate.ToolID], candidate)
	}
	for toolID := range result {
		sort.Slice(result[toolID], func(left, right int) bool {
			return result[toolID][left].PackageID < result[toolID][right].PackageID
		})
	}
	return result, nil
}

type defenseToolCurrencyReference struct {
	ID   int64
	Name string
}

func (store *Store) defenseToolCurrencyReferences() (map[string]defenseToolCurrencyReference, error) {
	currencies, err := store.Catalog("currencies")
	if err != nil {
		return nil, fmt.Errorf("load official currencies: %w", err)
	}
	result := map[string]defenseToolCurrencyReference{}
	for _, raw := range currencies.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, hasID := record.Int64("currencyID")
		if !hasID || id <= 0 {
			continue
		}
		name := strings.TrimSpace(stringValue(record, "Name"))
		if name == "" {
			name = strings.TrimSpace(stringValue(record, "name"))
		}
		for _, alias := range []string{name, stringValue(record, "JSONKey")} {
			if normalized := normalizeBuildingName(alias); normalized != "" {
				result[normalized] = defenseToolCurrencyReference{ID: id, Name: name}
			}
		}
	}
	return result, nil
}

func decodeDefenseToolShopPackage(
	record Record,
	currencies map[string]defenseToolCurrencyReference,
) (DefenseToolShopPackage, bool) {
	packageType, _ := record.String("packageType")
	packageID, hasPackageID := record.Int64("packageID")
	toolID, hasToolID := record.Int64("unitID")
	toolAmount, hasToolAmount := record.Int64("unitAmount")
	if !strings.EqualFold(strings.TrimSpace(packageType), "tool") || !hasPackageID || packageID <= 0 ||
		!hasToolID || toolID <= 0 || !hasToolAmount || toolAmount <= 0 {
		return DefenseToolShopPackage{}, false
	}
	if premium, found := record.Int64("packagePriceC2"); found && premium > 0 {
		return DefenseToolShopPackage{}, false
	}

	prices := make([]DefenseToolShopPackage, 0, 1)
	unsupportedPrice := false
	for field := range record {
		if !strings.HasPrefix(field, "cost") && !strings.HasPrefix(field, "packagePrice") {
			continue
		}
		amount, found := record.Int64(field)
		if !found || amount <= 0 {
			continue
		}
		price := DefenseToolShopPackage{Price: amount}
		switch field {
		case "packagePriceC1":
			price.PriceScope, price.PriceID, price.PriceName = DefenseToolPricePlayerResource, 1, "coins"
		case "packagePriceWood":
			price.PriceScope, price.PriceID, price.PriceName = DefenseToolPriceCastleResource, 3, "wood"
		case "packagePriceStone":
			price.PriceScope, price.PriceID, price.PriceName = DefenseToolPriceCastleResource, 4, "stone"
		case "packagePriceAquamarine":
			price.PriceScope, price.PriceID, price.PriceName = DefenseToolPriceCastleResource, StormAquamarineID, "Aquamarine"
		case "packagePriceC2":
			unsupportedPrice = true
		default:
			if !strings.HasPrefix(field, "cost") {
				unsupportedPrice = true
				continue
			}
			reference, found := currencies[normalizeBuildingName(strings.TrimPrefix(field, "cost"))]
			if !found {
				unsupportedPrice = true
				continue
			}
			price.PriceScope, price.PriceID, price.PriceName = DefenseToolPriceCurrency, reference.ID, reference.Name
		}
		if price.PriceScope != "" {
			prices = append(prices, price)
		}
	}
	if unsupportedPrice || len(prices) != 1 {
		return DefenseToolShopPackage{}, false
	}
	stock, _ := record.Int64("stock")
	notRebuyable, _ := record.Int64("notRebuyable")
	if notRebuyable > 0 {
		stock = max(stock, int64(1))
	}
	maxBuyPerClick, _ := record.Int64("maxBuyPerClick")
	name := strings.TrimSpace(stringValue(record, "comment1"))
	if name == "" {
		name = fmt.Sprintf("tool %d", toolID)
	}
	price := prices[0]
	price.PackageID = packageID
	price.ToolID = toolID
	price.ToolAmount = toolAmount
	price.Stock = max(int64(0), stock)
	price.MaxBuyPerClick = max(int64(0), maxBuyPerClick)
	price.Name = name
	return price, true
}

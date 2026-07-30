package GameData

import (
	"fmt"
	"strings"
)

const (
	BerimondArmorerShopTableID       = 27
	BerimondArmorerMaxPurchaseAmount = 1_000

	BerimondScalingLadderToolID = 614
	BerimondBatteringRamToolID  = 611
	BerimondMantletToolID       = 620
)

var berimondCoinAttackToolIDs = []int64{
	BerimondScalingLadderToolID,
	BerimondBatteringRamToolID,
	BerimondMantletToolID,
}

type BerimondArmorerToolPackage struct {
	PackageID  int64  `json:"packageId"`
	ToolID     int64  `json:"toolId"`
	ToolAmount int64  `json:"toolAmount"`
	CoinPrice  int64  `json:"coinPrice"`
	MinLevel   int    `json:"minLevel,omitempty"`
	Name       string `json:"name"`
}

func BerimondCoinAttackToolIDs() []int64 {
	return append([]int64(nil), berimondCoinAttackToolIDs...)
}

func (store *Store) BerimondArmorerAttackTool(toolID int64) (BerimondArmorerToolPackage, bool) {
	if store == nil || toolID <= 0 {
		return BerimondArmorerToolPackage{}, false
	}
	store.berimondArmorerOnce.Do(func() {
		store.berimondArmorerProducts, store.berimondArmorerErr = store.loadBerimondArmorerAttackTools()
	})
	if store.berimondArmorerErr != nil {
		return BerimondArmorerToolPackage{}, false
	}
	item, found := store.berimondArmorerProducts[toolID]
	return item, found
}

func (store *Store) BerimondArmorerAttackTools() ([]BerimondArmorerToolPackage, error) {
	if store == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	store.berimondArmorerOnce.Do(func() {
		store.berimondArmorerProducts, store.berimondArmorerErr = store.loadBerimondArmorerAttackTools()
	})
	if store.berimondArmorerErr != nil {
		return nil, store.berimondArmorerErr
	}
	result := make([]BerimondArmorerToolPackage, 0, len(berimondCoinAttackToolIDs))
	for _, toolID := range berimondCoinAttackToolIDs {
		if item, found := store.berimondArmorerProducts[toolID]; found {
			result = append(result, item)
		}
	}
	return result, nil
}

func (store *Store) loadBerimondArmorerAttackTools() (map[int64]BerimondArmorerToolPackage, error) {
	units, err := store.Catalog("units")
	if err != nil {
		return nil, fmt.Errorf("load official Berimond armorer units: %w", err)
	}
	allowed := map[int64]struct{}{}
	for _, toolID := range berimondCoinAttackToolIDs {
		allowed[toolID] = struct{}{}
	}
	attackTools := map[int64]struct{}{}
	for _, raw := range units.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		toolID, hasToolID := record.Int64("wodID")
		typ, _ := record.String("typ")
		if _, supported := allowed[toolID]; supported && hasToolID &&
			strings.EqualFold(strings.TrimSpace(typ), "attack") {
			attackTools[toolID] = struct{}{}
		}
	}

	packages, err := store.Catalog("packages")
	if err != nil {
		return nil, fmt.Errorf("load official Berimond armorer packages: %w", err)
	}
	result := map[int64]BerimondArmorerToolPackage{}
	for _, raw := range packages.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, valid := decodeBerimondArmorerToolPackage(record, attackTools)
		if !valid {
			continue
		}
		current, exists := result[candidate.ToolID]
		if !exists || berimondToolPackageLess(candidate, current) {
			result[candidate.ToolID] = candidate
		}
	}
	return result, nil
}

func decodeBerimondArmorerToolPackage(
	record Record,
	attackTools map[int64]struct{},
) (BerimondArmorerToolPackage, bool) {
	packageType, _ := record.String("packageType")
	shop, _ := record.String("comment2")
	packageID, hasPackageID := record.Int64("packageID")
	toolID, hasToolID := record.Int64("unitID")
	toolAmount, hasToolAmount := record.Int64("unitAmount")
	coinPrice, hasCoinPrice := record.Int64("packagePriceC1")
	if !strings.EqualFold(strings.TrimSpace(packageType), "tool") ||
		!strings.EqualFold(strings.TrimSpace(shop), "armorer") ||
		!hasPackageID || packageID <= 0 ||
		!hasToolID || toolID <= 0 ||
		!hasToolAmount || toolAmount <= 0 ||
		!hasCoinPrice || coinPrice <= 0 {
		return BerimondArmorerToolPackage{}, false
	}
	if _, supported := attackTools[toolID]; !supported {
		return BerimondArmorerToolPackage{}, false
	}
	for field := range record {
		if field == "packagePriceC1" ||
			(!strings.HasPrefix(field, "packagePrice") && !strings.HasPrefix(field, "cost")) {
			continue
		}
		if amount, found := record.Int64(field); found && amount > 0 {
			return BerimondArmorerToolPackage{}, false
		}
	}
	minLevel, _ := record.Int64("minLevel")
	name := strings.TrimSpace(stringValue(record, "comment1"))
	if name == "" {
		name = fmt.Sprintf("tool %d", toolID)
	}
	return BerimondArmorerToolPackage{
		PackageID: packageID, ToolID: toolID, ToolAmount: toolAmount,
		CoinPrice: coinPrice, MinLevel: int(minLevel), Name: name,
	}, true
}

func berimondToolPackageLess(left BerimondArmorerToolPackage, right BerimondArmorerToolPackage) bool {
	leftUnitPrice := float64(left.CoinPrice) / float64(left.ToolAmount)
	rightUnitPrice := float64(right.CoinPrice) / float64(right.ToolAmount)
	if leftUnitPrice != rightUnitPrice {
		return leftUnitPrice < rightUnitPrice
	}
	return left.PackageID < right.PackageID
}

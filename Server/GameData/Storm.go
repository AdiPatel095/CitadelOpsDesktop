package GameData

import (
	"sort"
	"strconv"
	"strings"
)

const (
	StormKingdomID         = 4
	StormAquamarineID      = 9
	StormLunaShopBuildType = 3
	StormLunaShopTableID   = -1
	StormLunaShopCastleID  = -1
	StormTroopSupportMead  = 50_000
	StormIsleKindFort      = "fort"
	StormIsleKindIsland    = "island"
	StormIslandSizeLarge   = "large"
	StormIslandSizeSmall   = "small"
)

var stormShopProductIDs = [...]int64{
	3116, 3117, 3118, 3119, 3120, 3122, 3123, 3124, 3125,
	245, 246, 247, 248,
	2795, 2796, 2797, 2798,
}

type StormIsleDefinition struct {
	ID                    int64   `json:"id"`
	Type                  string  `json:"type"`
	Kind                  string  `json:"kind"`
	Resource              string  `json:"resource,omitempty"`
	Size                  string  `json:"size,omitempty"`
	Level                 int     `json:"level"`
	MaximumVictoryCount   int64   `json:"maximumVictoryCount,omitempty"`
	VictoryCounts         []int64 `json:"victoryCounts"`
	FixedLoot             int64   `json:"fixedLoot,omitempty"`
	AquamarineLoot        int64   `json:"aquamarineLoot,omitempty"`
	GlobalCooldownSec     int64   `json:"globalCooldownSec,omitempty"`
	LocalCooldownSec      int64   `json:"localCooldownSec,omitempty"`
	OccupationDurationSec int64   `json:"occupationDurationSec,omitempty"`
}

type StormShopPackage struct {
	ProductID       int64  `json:"productId"`
	Name            string `json:"name"`
	PackageType     string `json:"packageType,omitempty"`
	AquamarinePrice int64  `json:"aquamarinePrice"`
	Stock           int64  `json:"stock,omitempty"`
}

func (store *Store) StormIsle(id int64) (StormIsleDefinition, bool) {
	if store == nil || id <= 0 {
		return StormIsleDefinition{}, false
	}
	catalog, err := store.Catalog("isles")
	if err != nil {
		return StormIsleDefinition{}, false
	}
	raw, found := catalog.Find(strconv.FormatInt(id, 10))
	if !found {
		return StormIsleDefinition{}, false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return StormIsleDefinition{}, false
	}
	return decodeStormIsle(record)
}

func (store *Store) StormIsles() []StormIsleDefinition {
	if store == nil {
		return []StormIsleDefinition{}
	}
	catalog, err := store.Catalog("isles")
	if err != nil {
		return []StormIsleDefinition{}
	}
	result := make([]StormIsleDefinition, 0, len(catalog.Rows()))
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		definition, valid := decodeStormIsle(record)
		if valid {
			result = append(result, definition)
		}
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func StormFortAttacksRemaining(definition StormIsleDefinition, victoryCount int64) (int64, bool) {
	if definition.Kind != StormIsleKindFort || definition.MaximumVictoryCount <= 0 ||
		victoryCount < 0 || victoryCount > definition.MaximumVictoryCount {
		return 0, false
	}
	return definition.MaximumVictoryCount - victoryCount, true
}

func (store *Store) StormShopPackage(productID int64) (StormShopPackage, bool) {
	if store == nil || productID <= 0 || !isStormShopProductID(productID) {
		return StormShopPackage{}, false
	}
	catalog, err := store.Catalog("packages")
	if err != nil {
		return StormShopPackage{}, false
	}
	raw, found := catalog.Find(strconv.FormatInt(productID, 10))
	if !found {
		return StormShopPackage{}, false
	}
	record, err := DecodeRecord(raw)
	if err != nil {
		return StormShopPackage{}, false
	}
	return decodeStormShopPackage(record)
}

func (store *Store) StormShopPackages() []StormShopPackage {
	if store == nil {
		return []StormShopPackage{}
	}
	catalog, err := store.Catalog("packages")
	if err != nil {
		return []StormShopPackage{}
	}
	result := make([]StormShopPackage, 0, len(stormShopProductIDs))
	for _, productID := range stormShopProductIDs {
		raw, found := catalog.Find(strconv.FormatInt(productID, 10))
		if !found {
			continue
		}
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		item, valid := decodeStormShopPackage(record)
		if valid {
			result = append(result, item)
		}
	}
	return result
}

func isStormShopProductID(productID int64) bool {
	for _, candidate := range stormShopProductIDs {
		if candidate == productID {
			return true
		}
	}
	return false
}

func decodeStormIsle(record Record) (StormIsleDefinition, bool) {
	id, hasID := record.Int64("IsleID")
	typeName, hasType := record.String("type")
	if !hasID || id <= 0 || !hasType {
		return StormIsleDefinition{}, false
	}
	typeName = strings.ToUpper(strings.TrimSpace(typeName))
	definition := StormIsleDefinition{ID: id, Type: typeName, Kind: StormIsleKindIsland, VictoryCounts: []int64{}}
	if strings.EqualFold(typeName, "DUNGEON") {
		definition.Kind = StormIsleKindFort
	} else {
		switch {
		case strings.Contains(typeName, "WOOD"):
			definition.Resource = "wood"
			definition.FixedLoot, _ = record.Int64("fixedLootWood")
		case strings.Contains(typeName, "STONE"):
			definition.Resource = "stone"
			definition.FixedLoot, _ = record.Int64("fixedLootStone")
		case strings.Contains(typeName, "AQUAMARINE"):
			definition.Resource = "aquamarine"
			definition.FixedLoot, _ = record.Int64("fixedLootAquamarine")
		}
		if id >= 1 && id <= 3 {
			definition.Size = StormIslandSizeLarge
		} else {
			definition.Size = StormIslandSizeSmall
		}
	}
	level, _ := record.Int64("dungeonlevel")
	definition.Level = int(level)
	definition.MaximumVictoryCount, _ = record.Int64("maxCountVictories")
	definition.AquamarineLoot, _ = record.Int64("lootAquamarine")
	definition.GlobalCooldownSec, _ = record.Int64("globalCooldown")
	definition.LocalCooldownSec, _ = record.Int64("localCooldown")
	definition.OccupationDurationSec, _ = record.Int64("occupationTime")
	if rawVictories, found := record.String("countVictories"); found {
		for _, part := range strings.Split(rawVictories, "#") {
			value, err := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if err == nil && value >= 0 {
				definition.VictoryCounts = append(definition.VictoryCounts, value)
			}
		}
	}
	if definition.Kind == StormIsleKindFort && definition.MaximumVictoryCount <= 0 {
		definition.MaximumVictoryCount = int64(len(definition.VictoryCounts))
	}
	return definition, definition.Kind == StormIsleKindFort || definition.Resource != ""
}

func decodeStormShopPackage(record Record) (StormShopPackage, bool) {
	shop, _ := record.String("comment2")
	if !strings.EqualFold(strings.TrimSpace(shop), "Luna's trade boat") {
		return StormShopPackage{}, false
	}
	productID, hasID := record.Int64("packageID")
	price, hasPrice := record.Int64("packagePriceAquamarine")
	if !hasID || productID <= 0 || !isStormShopProductID(productID) || !hasPrice || price <= 0 {
		return StormShopPackage{}, false
	}
	name, _ := record.String("comment1")
	packageType, _ := record.String("packageType")
	stock, _ := record.Int64("stock")
	return StormShopPackage{
		ProductID: productID, Name: strings.TrimSpace(name), PackageType: strings.TrimSpace(packageType),
		AquamarinePrice: price, Stock: stock,
	}, true
}

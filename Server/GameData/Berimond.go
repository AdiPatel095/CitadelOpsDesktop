package GameData

import (
	"sort"
	"strconv"
	"strings"
)

const (
	BerimondKingdomID = 10

	// GallantryPointsBoosterID is the official BoosterConst ID used by the
	// game's boi response for every strength of the timed Gallantry booster.
	GallantryPointsBoosterID = 24
)

type BerimondCampOption struct {
	ID        int64 `json:"id"`
	MinLevel  int   `json:"minLevel"`
	CostWood  int64 `json:"costWood,omitempty"`
	CostStone int64 `json:"costStone,omitempty"`
	CostFood  int64 `json:"costFood,omitempty"`
	CostCoins int64 `json:"costCoins,omitempty"`
}

func (store *Store) CheapestNonPremiumBerimondCamp(playerLevel int) (BerimondCampOption, bool) {
	if store == nil {
		return BerimondCampOption{}, false
	}
	catalog, err := store.Catalog("prebuiltcastles")
	if err != nil {
		return BerimondCampOption{}, false
	}
	options := make([]BerimondCampOption, 0, len(catalog.Rows()))
	for _, raw := range catalog.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil || !recordIncludesInteger(record, "spaceIDs", BerimondKingdomID) {
			continue
		}
		id, hasID := record.Int64("preBuiltCastleID")
		minLevel, _ := record.Int64("minLevel")
		premiumCost, _ := record.Int64("costC2")
		if !hasID || id <= 0 || premiumCost > 0 || minLevel > int64(playerLevel) {
			continue
		}
		option := BerimondCampOption{ID: id, MinLevel: int(minLevel)}
		option.CostWood, _ = record.Int64("costWood")
		option.CostStone, _ = record.Int64("costStone")
		option.CostFood, _ = record.Int64("costFood")
		option.CostCoins, _ = record.Int64("costC1")
		options = append(options, option)
	}
	sort.Slice(options, func(left, right int) bool {
		leftCost := options[left].CostWood + options[left].CostStone + options[left].CostFood + options[left].CostCoins
		rightCost := options[right].CostWood + options[right].CostStone + options[right].CostFood + options[right].CostCoins
		if leftCost != rightCost {
			return leftCost < rightCost
		}
		return options[left].ID < options[right].ID
	})
	if len(options) == 0 {
		return BerimondCampOption{}, false
	}
	return options[0], true
}

func recordIncludesInteger(record Record, field string, expected int) bool {
	value, found := record.String(field)
	if !found {
		if integer, ok := record.Int64(field); ok {
			return integer == int64(expected)
		}
		return false
	}
	for _, part := range strings.Split(value, ",") {
		integer, err := strconv.Atoi(strings.TrimSpace(part))
		if err == nil && integer == expected {
			return true
		}
	}
	return false
}

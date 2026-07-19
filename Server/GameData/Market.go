package GameData

import (
	"fmt"
	"math"

	"CitadelDesktop/Server/State"
)

const marketBaseCapacityPerBarrow = 100

type MarketEffect struct {
	EffectID int64
	Values   []float64
}

type MarketCapacity struct {
	CaravanBoostPercent      float64 `json:"caravanBoostPercent"`
	AreaCapacityBoostPercent float64 `json:"areaCapacityBoostPercent"`
	CapacityBonus            float64 `json:"capacityBonus"`
	CapacityPerBarrow        int     `json:"capacityPerBarrow"`
}

type marketProjection struct {
	barrowsByBuilding         map[int64]int
	barrowsByConstructionItem map[int64]int
	caravanBoostByLevel       map[int]float64
	capacityBoostEffectIDs    map[int64]struct{}
	capacityBonusEffectIDs    map[int64]struct{}
}

func (store *Store) MarketBarrowsForBuilding(definitionID int64) (int, error) {
	projection, err := store.marketData()
	if err != nil {
		return 0, err
	}
	return projection.barrowsByBuilding[definitionID], nil
}

func (store *Store) MarketBarrowsForConstructionItem(definitionID int64) (int, error) {
	projection, err := store.marketData()
	if err != nil {
		return 0, err
	}
	return projection.barrowsByConstructionItem[definitionID], nil
}

func (store *Store) CastleHasMarketplace(castle State.CastleState) (bool, error) {
	projection, err := store.marketData()
	if err != nil {
		return false, err
	}
	for _, building := range castle.Buildings {
		if projection.barrowsByBuilding[int64(building.DefinitionID)] > 0 {
			return true, nil
		}
	}
	return false, nil
}

func (store *Store) MarketCapacity(caravanLevel int, effects []MarketEffect) (MarketCapacity, error) {
	projection, err := store.marketData()
	if err != nil {
		return MarketCapacity{}, err
	}
	result := MarketCapacity{CaravanBoostPercent: projection.caravanBoostByLevel[caravanLevel]}
	for _, effect := range effects {
		if len(effect.Values) == 0 {
			continue
		}
		if _, exists := projection.capacityBoostEffectIDs[effect.EffectID]; exists {
			result.AreaCapacityBoostPercent += effect.Values[0]
		}
		if _, exists := projection.capacityBonusEffectIDs[effect.EffectID]; exists {
			result.CapacityBonus += effect.Values[0]
		}
	}
	capacity := float64(marketBaseCapacityPerBarrow) * (1 + result.AreaCapacityBoostPercent/100) * (1 + result.CaravanBoostPercent/100)
	capacity += result.CapacityBonus
	result.CapacityPerBarrow = 1 + int(math.Round(capacity))
	return result, nil
}

func (store *Store) marketData() (marketProjection, error) {
	if store == nil {
		return marketProjection{}, fmt.Errorf("official game data is unavailable")
	}
	store.marketOnce.Do(func() {
		store.marketProjection, store.marketErr = buildMarketProjection(store)
	})
	return store.marketProjection, store.marketErr
}

func buildMarketProjection(store *Store) (marketProjection, error) {
	projection := marketProjection{
		barrowsByBuilding:         map[int64]int{},
		barrowsByConstructionItem: map[int64]int{},
		caravanBoostByLevel:       map[int]float64{},
		capacityBoostEffectIDs:    map[int64]struct{}{},
		capacityBonusEffectIDs:    map[int64]struct{}{},
	}
	buildings, err := store.Catalog("buildings")
	if err != nil {
		return projection, err
	}
	for _, raw := range buildings.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		name, _ := record.String("name")
		barrows, _ := record.Int64("marketCarriages")
		id, _ := record.Int64("wodID")
		if name == "Market" && id > 0 && barrows > 0 {
			projection.barrowsByBuilding[id] = int(barrows)
		}
	}
	constructionItems, err := store.Catalog("constructionItems")
	if err != nil {
		return projection, err
	}
	for _, raw := range constructionItems.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		barrows, _ := record.Int64("marketCarriages")
		id, _ := record.Int64("constructionItemID")
		if id > 0 && barrows > 0 {
			projection.barrowsByConstructionItem[id] = int(barrows)
		}
	}
	levelBoosters, err := store.Catalog("levelBoosters")
	if err != nil {
		return projection, err
	}
	for _, raw := range levelBoosters.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		boosterType, _ := record.Int64("boosterType")
		level, _ := record.Int64("level")
		boost, _ := record.Float64("boostPercentage")
		if boosterType == 11 && level >= 0 {
			projection.caravanBoostByLevel[int(level)] = boost
		}
	}
	effects, err := store.Catalog("effects")
	if err != nil {
		return projection, err
	}
	for _, raw := range effects.Rows() {
		record, decodeErr := DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		id, _ := record.Int64("effectID")
		name, _ := record.String("name")
		switch name {
		case "marketCarriageCapacityBoost", "marketCarriageCapacityBoostCapped":
			projection.capacityBoostEffectIDs[id] = struct{}{}
		case "marketCarriageCapacityBonus":
			projection.capacityBonusEffectIDs[id] = struct{}{}
		}
	}
	return projection, nil
}

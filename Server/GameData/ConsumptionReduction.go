// Package gamedata holds embedded game indexes used by the server (no runtime reads of Server/Data).
package gamedata

import (
	"log"
	"sync"
)

// ConsumptionReductionPP is percentage-point reduction per resource for one building definition (wodID).
// Effective consumption multiplies raw by (1 − sum(PP)/100) per resource, capped at 100 PP total each.
type ConsumptionReductionPP struct {
	FoodPP float64
	MeadPP float64
	BeefPP float64
}

var consumptionReductionLoadOnce sync.Once

// LoadConsumptionReductionBuildings logs embedded catalog size once; data lives in ConsumptionReductionByBuildingWodID.
func LoadConsumptionReductionBuildings() error {
	consumptionReductionLoadOnce.Do(func() {
		log.Printf("[gamedata] consumption reduction: %d building wodIDs", len(ConsumptionReductionByBuildingWodID))
	})
	return nil
}

// ApplyConsumptionBuildingReductions scales raw hourly food/mead/beef consumption (from troops × unit consumption)
// using summed percentage-point reductions from matching building wodIDs (Bakery, BGBakery, MeadDistillery, ButcherShop levels).
// wodIDs are JAA gca building definition ids from BG+BD rows; each id counted at most once.
// Total PP per resource type is capped at 100; effective consumption = raw × (1 − PP/100).
func ApplyConsumptionBuildingReductions(wodIDs []int, rawFood, rawMead, rawBeef float64) (float64, float64, float64) {
	if len(wodIDs) == 0 || len(ConsumptionReductionByBuildingWodID) == 0 {
		return rawFood, rawMead, rawBeef
	}
	seen := make(map[int]struct{}, len(wodIDs))
	var foodPP, meadPP, beefPP float64
	for _, wid := range wodIDs {
		if wid <= 0 {
			continue
		}
		if _, dup := seen[wid]; dup {
			continue
		}
		seen[wid] = struct{}{}
		r, ok := ConsumptionReductionByBuildingWodID[wid]
		if !ok {
			continue
		}
		foodPP += r.FoodPP
		meadPP += r.MeadPP
		beefPP += r.BeefPP
	}
	foodPP = clampPctPoints(foodPP)
	meadPP = clampPctPoints(meadPP)
	beefPP = clampPctPoints(beefPP)
	return rawFood * (1 - foodPP/100.0),
		rawMead * (1 - meadPP/100.0),
		rawBeef * (1 - beefPP/100.0)
}

func clampPctPoints(pp float64) float64 {
	if pp < 0 {
		return 0
	}
	if pp > 100 {
		return 100
	}
	return pp
}

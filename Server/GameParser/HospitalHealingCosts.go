package GameParser

import (
	"encoding/json"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
)

// HospitalHealingCost describes the catalog healing cost for one unit.
type HospitalHealingCost struct {
	UnitID   int     `json:"unitId"`
	CoinCost float64 `json:"coinCost,omitempty"`
	RubyCost float64 `json:"rubyCost,omitempty"`
	Known    bool    `json:"known"`
}

var (
	hospitalHealingCostsOnce sync.Once
	hospitalHealingCostsByID map[int]HospitalHealingCost
)

func buildHospitalHealingCostsByID() {
	hospitalHealingCostsByID = make(map[int]HospitalHealingCost)
	b, err := serverdata.ReadTroopsJSON()
	if err != nil {
		return
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, row := range rows {
		unitID := jsonIntFromAny(row["wodID"])
		if unitID <= 0 {
			continue
		}
		hospitalHealingCostsByID[unitID] = HospitalHealingCost{
			UnitID:   unitID,
			CoinCost: jsonFloatFromAny(row["healingCostC1"]),
			RubyCost: jsonFloatFromAny(row["healingCostC2"]),
			Known:    true,
		}
	}
}

// HospitalHealingCostDetails returns the official catalog healing costs for a unit.
func HospitalHealingCostDetails(unitID int) HospitalHealingCost {
	if unitID <= 0 {
		return HospitalHealingCost{}
	}
	hospitalHealingCostsOnce.Do(buildHospitalHealingCostsByID)
	cost, ok := hospitalHealingCostsByID[unitID]
	if !ok {
		return HospitalHealingCost{UnitID: unitID}
	}
	return cost
}

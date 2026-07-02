package GameParser

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models"
)

type toolProductionCostRow struct {
	ToolID       int
	Costs        map[string]float64
	UnknownCosts map[string]float64
}

// ToolResourceCost is one resource requirement for a tool production stack.
type ToolResourceCost struct {
	Key            string  `json:"key"`
	Label          string  `json:"label"`
	Scope          string  `json:"scope"`
	ReservationKey string  `json:"reservationKey"`
	UnitCost       float64 `json:"unitCost"`
	Required       float64 `json:"required"`
	Available      float64 `json:"available"`
	Missing        float64 `json:"missing,omitempty"`
	KnownBalance   bool    `json:"knownBalance"`
}

// ToolResourceCostCheck describes whether a tool stack can be queued with known resources.
type ToolResourceCostCheck struct {
	ToolID                int                `json:"toolId"`
	Amount                int                `json:"amount"`
	Costs                 []ToolResourceCost `json:"costs,omitempty"`
	Missing               []ToolResourceCost `json:"missing,omitempty"`
	UnknownToolCost       bool               `json:"unknownToolCost,omitempty"`
	UnknownBalancePresent bool               `json:"unknownBalancePresent,omitempty"`
}

var (
	toolProductionCostsOnce sync.Once
	toolProductionCostsByID map[int]toolProductionCostRow
)

func buildToolProductionCostsByID() {
	toolProductionCostsByID = make(map[int]toolProductionCostRow)
	b, err := serverdata.ReadToolsJSON()
	if err != nil {
		return
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(b, &rows); err != nil {
		return
	}
	for _, row := range rows {
		toolID := jsonIntFromAny(row["wodID"])
		if toolID <= 0 {
			continue
		}
		costs := make(map[string]float64)
		unknownCosts := make(map[string]float64)
		for field, raw := range row {
			if !strings.HasPrefix(field, "cost") {
				continue
			}
			value := jsonFloatFromAny(raw)
			if value <= 0 {
				continue
			}
			meta, ok := recruitCostFieldMap[field]
			if !ok {
				unknownCosts[field] += value
				continue
			}
			costs[meta.Key] += value
		}
		toolProductionCostsByID[toolID] = toolProductionCostRow{
			ToolID:       toolID,
			Costs:        costs,
			UnknownCosts: unknownCosts,
		}
	}
}

func toolProductionCosts(toolID int) (toolProductionCostRow, bool) {
	if toolID <= 0 {
		return toolProductionCostRow{}, false
	}
	toolProductionCostsOnce.Do(buildToolProductionCostsByID)
	row, ok := toolProductionCostsByID[toolID]
	return row, ok
}

func sortedToolCostKeys(costs map[string]float64) []string {
	keys := make([]string, 0, len(costs))
	for key, value := range costs {
		if value > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// ToolProductionResourceCostCheck verifies that known balances can pay for the requested tool stack.
func ToolProductionResourceCostCheck(gs *Models.GameState, castleInfo *Models.PlayerCastleInfo, toolID int, amount int, reservations map[string]float64) ToolResourceCostCheck {
	check := ToolResourceCostCheck{
		ToolID: toolID,
		Amount: amount,
	}
	if amount <= 0 {
		return check
	}
	toolCost, ok := toolProductionCosts(toolID)
	if !ok {
		check.UnknownToolCost = true
		return check
	}
	castleID := 0
	if castleInfo != nil {
		castleID = int(castleInfo.Aid)
	}
	for _, key := range sortedRecruitCostKeys(toolCost.Costs) {
		unitValue := toolCost.Costs[key]
		if unitValue <= 0 {
			continue
		}
		label, scope := recruitCostFieldMeta(key)
		required := math.Ceil(unitValue * float64(amount))
		if required <= 0 {
			continue
		}
		reservationKey := recruitReservationKey(scope, key, castleID)
		available, known := recruitResourceBalance(gs, castleInfo, key)
		if known && reservations != nil {
			available -= reservations[reservationKey]
		}
		cost := ToolResourceCost{
			Key:            key,
			Label:          label,
			Scope:          string(scope),
			ReservationKey: reservationKey,
			UnitCost:       unitValue,
			Required:       required,
			Available:      available,
			KnownBalance:   known,
		}
		if !known {
			check.UnknownBalancePresent = true
		} else if available < required {
			cost.Missing = required - available
			check.Missing = append(check.Missing, cost)
		}
		check.Costs = append(check.Costs, cost)
	}
	for _, field := range sortedToolCostKeys(toolCost.UnknownCosts) {
		unitValue := toolCost.UnknownCosts[field]
		required := math.Ceil(unitValue * float64(amount))
		if required <= 0 {
			continue
		}
		check.UnknownBalancePresent = true
		check.Costs = append(check.Costs, ToolResourceCost{
			Key:          field,
			Label:        field,
			Scope:        "unknown",
			UnitCost:     unitValue,
			Required:     required,
			KnownBalance: false,
		})
	}
	return check
}

// CanAfford reports whether every required tool-production balance is known and sufficient.
func (check ToolResourceCostCheck) CanAfford() bool {
	return !check.UnknownToolCost && !check.UnknownBalancePresent && len(check.Missing) == 0
}

// ReserveToolResourceCosts records costs sent in this cycle so repeated bup calls do not reuse stale balances.
func ReserveToolResourceCosts(reservations map[string]float64, check ToolResourceCostCheck) {
	if reservations == nil || !check.CanAfford() {
		return
	}
	for _, cost := range check.Costs {
		if !cost.KnownBalance || cost.Required <= 0 || cost.ReservationKey == "" {
			continue
		}
		reservations[cost.ReservationKey] += cost.Required
	}
}

func (check ToolResourceCostCheck) MissingSummary() string {
	if check.UnknownToolCost {
		return "cost data unavailable"
	}
	parts := make([]string, 0, len(check.Missing))
	for _, cost := range check.Missing {
		parts = append(parts, fmt.Sprintf("%s required=%.0f available=%.0f missing=%.0f", cost.Label, cost.Required, cost.Available, cost.Missing))
	}
	for _, cost := range check.Costs {
		if cost.KnownBalance {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s required=%.0f balance=unknown", cost.Label, cost.Required))
	}
	return strings.Join(parts, "; ")
}

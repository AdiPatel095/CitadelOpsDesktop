package GameParser

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"

	serverdata "CitadelDesktop/Server/Data"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/Models/Castle"
)

type recruitResourceScope string

const (
	recruitResourceScopeGlobal recruitResourceScope = "global"
	recruitResourceScopeCastle recruitResourceScope = "castle"
)

type recruitUnitCostRow struct {
	UnitID int
	Costs  map[string]float64
}

// RecruitResourceCost is one resource requirement for a recruit stack.
type RecruitResourceCost struct {
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

// RecruitResourceCostCheck describes whether a recruit stack can be queued with known resources.
type RecruitResourceCostCheck struct {
	UnitID                int                   `json:"unitId"`
	Amount                int                   `json:"amount"`
	CostReductionPercent  int                   `json:"costReductionPercent,omitempty"`
	Costs                 []RecruitResourceCost `json:"costs,omitempty"`
	Missing               []RecruitResourceCost `json:"missing,omitempty"`
	UnknownUnitCost       bool                  `json:"unknownUnitCost,omitempty"`
	UnknownBalancePresent bool                  `json:"unknownBalancePresent,omitempty"`
}

var (
	recruitUnitCostsOnce sync.Once
	recruitUnitCostsByID map[int]recruitUnitCostRow
)

var recruitCostFieldMap = map[string]struct {
	Key   string
	Label string
	Scope recruitResourceScope
}{
	"costC1":                {Key: "coins", Label: "coins", Scope: recruitResourceScopeGlobal},
	"costC2":                {Key: "rubies", Label: "rubies", Scope: recruitResourceScopeGlobal},
	"costWood":              {Key: "wood", Label: "wood", Scope: recruitResourceScopeCastle},
	"costStone":             {Key: "stone", Label: "stone", Scope: recruitResourceScopeCastle},
	"costSceatToken":        {Key: "sceat", Label: "sceat", Scope: recruitResourceScopeGlobal},
	"costLegendaryToken":    {Key: "legendary_token", Label: "legendary tokens", Scope: recruitResourceScopeGlobal},
	"costComponent1":        {Key: "component1", Label: "component 1", Scope: recruitResourceScopeGlobal},
	"costComponent2":        {Key: "component2", Label: "component 2", Scope: recruitResourceScopeGlobal},
	"costComponent3":        {Key: "component3", Label: "component 3", Scope: recruitResourceScopeGlobal},
	"costComponent4":        {Key: "component4", Label: "component 4", Scope: recruitResourceScopeGlobal},
	"costComponent5":        {Key: "component5", Label: "component 5", Scope: recruitResourceScopeGlobal},
	"costComponent6":        {Key: "component6", Label: "component 6", Scope: recruitResourceScopeGlobal},
	"costComponent7":        {Key: "component7", Label: "component 7", Scope: recruitResourceScopeGlobal},
	"costComponent8":        {Key: "component8", Label: "component 8", Scope: recruitResourceScopeGlobal},
	"costDragonGlassArrows": {Key: "dragon_glass_arrows", Label: "dragon glass arrows", Scope: recruitResourceScopeGlobal},
	"costDragonScaleArmor":  {Key: "dragon_scale_armor", Label: "dragon scale armor", Scope: recruitResourceScopeGlobal},
	"costDragonScaleArrows": {Key: "dragon_scale_arrows", Label: "dragon scale arrows", Scope: recruitResourceScopeGlobal},
	"costTwinFlameAxes":     {Key: "twin_flame_axes", Label: "twin flame axes", Scope: recruitResourceScopeGlobal},
}

func buildRecruitUnitCostsByID() {
	recruitUnitCostsByID = make(map[int]recruitUnitCostRow)
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
		costs := make(map[string]float64)
		for field, meta := range recruitCostFieldMap {
			value := jsonFloatFromAny(row[field])
			if value > 0 {
				costs[meta.Key] += value
			}
		}
		recruitUnitCostsByID[unitID] = recruitUnitCostRow{
			UnitID: unitID,
			Costs:  costs,
		}
	}
}

func recruitUnitCosts(unitID int) (recruitUnitCostRow, bool) {
	if unitID <= 0 {
		return recruitUnitCostRow{}, false
	}
	recruitUnitCostsOnce.Do(buildRecruitUnitCostsByID)
	row, ok := recruitUnitCostsByID[unitID]
	return row, ok
}

func jsonIntFromAny(value interface{}) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case int64:
		return int(typed)
	case json.Number:
		i, _ := typed.Int64()
		return int(i)
	case string:
		i, _ := strconv.Atoi(strings.TrimSpace(typed))
		return i
	default:
		return 0
	}
}

func jsonFloatFromAny(value interface{}) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case float32:
		return float64(typed)
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case json.Number:
		f, _ := typed.Float64()
		return f
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(typed), 64)
		return f
	default:
		return 0
	}
}

func recruitCostReductionAppliesToBuilding(meta ConstructionItemMeta, buildingWID int) bool {
	if meta.RecruitCostReduction <= 0 {
		return false
	}
	if meta.ConstructionItemGroupID > 0 {
		wids, ok := BuildingWodIDsForConstructionItemGroup(meta.ConstructionItemGroupID)
		if ok && len(wids) > 0 {
			_, applies := wids[buildingWID]
			return applies
		}
	}
	return strings.EqualFold(strings.TrimSpace(meta.LockRemoval), "SOLDIER_RECRUITMENT")
}

// BarracksRecruitCostReductionPercent returns the recruit cost reduction equipped on the selected barracks.
func BarracksRecruitCostReductionPercent(c *castle.PlayerCastleInfo, barracksOID int, barracksWID int) int {
	if c == nil || barracksOID <= 0 {
		return 0
	}
	total := 0
	for _, building := range c.ConstructionByBuilding {
		if building.OID != barracksOID {
			continue
		}
		for _, slot := range building.Slots {
			meta, ok := ConstructionItemMetaByCID(slot.CID)
			if !ok || !recruitCostReductionAppliesToBuilding(meta, barracksWID) {
				continue
			}
			total += meta.RecruitCostReduction
		}
	}
	if total < 0 {
		return 0
	}
	if total > 100 {
		return 100
	}
	return total
}

func recruitReservationKey(scope recruitResourceScope, key string, castleID int) string {
	if scope == recruitResourceScopeCastle {
		return fmt.Sprintf("%s:%d:%s", scope, castleID, key)
	}
	return fmt.Sprintf("%s:%s", scope, key)
}

func recruitResourceBalance(gs *Models.GameState, castleInfo *Models.PlayerCastleInfo, key string) (float64, bool) {
	switch key {
	case "coins":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Coins, true
	case "rubies":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Rubies, true
	case "sceat":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Sceat, true
	case "legendary_token":
		if gs == nil {
			return 0, false
		}
		if gs.GlobalResources.LegendaryToken > 0 {
			return gs.GlobalResources.LegendaryToken, true
		}
		return gs.GlobalResources.UpgrToken, true
	case "dragon_glass_arrows":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.DrgGlassArrow, true
	case "dragon_scale_armor":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.DrgScaleArmor, true
	case "dragon_scale_arrows":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.DrgScaleArrow, true
	case "twin_flame_axes":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.TwinFlameAxes, true
	case "component1":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component1, true
	case "component2":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component2, true
	case "component3":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component3, true
	case "component4":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component4, true
	case "component5":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component5, true
	case "component6":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component6, true
	case "component7":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component7, true
	case "component8":
		if gs == nil {
			return 0, false
		}
		return gs.GlobalResources.Component8, true
	case "wood":
		if castleInfo == nil {
			return 0, false
		}
		return castleInfo.Amount.WoodAmount, true
	case "stone":
		if castleInfo == nil {
			return 0, false
		}
		return castleInfo.Amount.StoneAmount, true
	default:
		return 0, false
	}
}

func recruitCostFieldMeta(key string) (string, recruitResourceScope) {
	for _, meta := range recruitCostFieldMap {
		if meta.Key == key {
			return meta.Label, meta.Scope
		}
	}
	return key, recruitResourceScopeGlobal
}

func sortedRecruitCostKeys(costs map[string]float64) []string {
	keys := make([]string, 0, len(costs))
	for key, value := range costs {
		if value > 0 {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	return keys
}

// RecruitUnitResourceCostCheck verifies that known balances can pay for the requested recruit stack.
func RecruitUnitResourceCostCheck(gs *Models.GameState, castleInfo *Models.PlayerCastleInfo, unitID int, amount int, costReductionPercent int, reservations map[string]float64) RecruitResourceCostCheck {
	check := RecruitResourceCostCheck{
		UnitID:               unitID,
		Amount:               amount,
		CostReductionPercent: costReductionPercent,
	}
	if amount <= 0 {
		return check
	}
	unitCost, ok := recruitUnitCosts(unitID)
	if !ok {
		check.UnknownUnitCost = true
		return check
	}
	castleID := 0
	if castleInfo != nil {
		castleID = int(castleInfo.Aid)
	}
	multiplier := 1 - float64(costReductionPercent)/100
	if multiplier < 0 {
		multiplier = 0
	}
	for _, key := range sortedRecruitCostKeys(unitCost.Costs) {
		unitValue := unitCost.Costs[key]
		if unitValue <= 0 {
			continue
		}
		label, scope := recruitCostFieldMeta(key)
		required := math.Ceil(unitValue * float64(amount) * multiplier)
		if required <= 0 {
			continue
		}
		reservationKey := recruitReservationKey(scope, key, castleID)
		available, known := recruitResourceBalance(gs, castleInfo, key)
		if known && reservations != nil {
			available -= reservations[reservationKey]
		}
		cost := RecruitResourceCost{
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
	return check
}

// CanAfford reports whether all known required balances are sufficient.
func (check RecruitResourceCostCheck) CanAfford() bool {
	return len(check.Missing) == 0
}

// ReserveRecruitResourceCosts records costs sent in this cycle so repeated bup calls do not reuse stale balances.
func ReserveRecruitResourceCosts(reservations map[string]float64, check RecruitResourceCostCheck) {
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

func (check RecruitResourceCostCheck) MissingSummary() string {
	if len(check.Missing) == 0 {
		return ""
	}
	parts := make([]string, 0, len(check.Missing))
	for _, cost := range check.Missing {
		parts = append(parts, fmt.Sprintf("%s required=%.0f available=%.0f missing=%.0f", cost.Label, cost.Required, cost.Available, cost.Missing))
	}
	return strings.Join(parts, "; ")
}

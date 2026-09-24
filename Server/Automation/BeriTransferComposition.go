package Automation

import (
	"fmt"
	"math"
	"math/big"
	"sort"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

type beriTroopTarget struct {
	id        State.UnitID
	weight    int64
	stock     int64
	target    int64
	remainder *big.Int
}

// beriProportionalTransfer chooses one guarded transfer at a time. Re-evaluation
// after its confirmed KUT uses refreshed capacity and both castle inventories.
func beriProportionalTransfer(
	preset AttackPresets.Preset,
	donor, camp map[State.UnitID]int64,
	capacity State.BeriState,
	gameData *GameData.Store,
) (State.UnitID, int64, string) {
	if capacity.AvailableTroops <= 0 {
		return 0, 0, "Waiting for free Berimond troop capacity"
	}
	if gameData == nil {
		return 0, 0, "Official unit provision data is unavailable"
	}
	if preset.UseTroopFamilies {
		combined := make(map[State.UnitID]int64, len(donor)+len(camp))
		for id, amount := range donor {
			if amount > 0 {
				combined[id] = amount
			}
		}
		for id, amount := range camp {
			if amount <= 0 {
				continue
			}
			if combined[id] > math.MaxInt64-amount {
				return 0, 0, "Troop-family inventory is too large to resolve safely"
			}
			combined[id] += amount
		}
		resolved, err := AttackPresets.ResolveTroopFamilies(preset, combined, gameData, 1)
		if err != nil {
			return 0, 0, "Waiting for a resolvable Berimond troop-family preset: " + err.Error()
		}
		preset = resolved
	}
	weights := map[State.UnitID]int64{}
	add := func(slots []AttackPresets.Slot) bool {
		for _, slot := range slots {
			if slot.ItemID == nil || *slot.ItemID <= 0 || slot.Quantity <= 0 {
				continue
			}
			id := State.UnitID(*slot.ItemID)
			if weights[id] > math.MaxInt64-slot.Quantity {
				return false
			}
			weights[id] += slot.Quantity
		}
		return true
	}
	for _, wave := range preset.Waves {
		for _, lane := range []AttackPresets.Lane{wave.Left, wave.Middle, wave.Right} {
			if !add(lane.Troops) {
				return 0, 0, "Preset troop totals are too large to transfer safely"
			}
		}
	}
	if !add(preset.CourtyardSupport.Troops) {
		return 0, 0, "Preset troop totals are too large to transfer safely"
	}
	if len(weights) == 0 {
		return 0, 0, "The selected Berimond attack preset has no troops"
	}
	ids := make([]State.UnitID, 0, len(weights))
	for id := range weights {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	totalWeight, campTotal := int64(0), int64(0)
	for _, id := range ids {
		usesFood, err := gameData.UnitUsesFoodSupply(id)
		if err != nil {
			return 0, 0, fmt.Sprintf("Waiting for official provision data for preset unit %d: %v", id, err)
		}
		if !usesFood {
			return 0, 0, fmt.Sprintf("Preset unit %d is not Food-fed and cannot transfer to Berimond", id)
		}
		if totalWeight > math.MaxInt64-weights[id] || camp[id] < 0 || campTotal > math.MaxInt64-camp[id] {
			return 0, 0, "Preset or camp troop totals are too large to transfer safely"
		}
		totalWeight += weights[id]
		campTotal += camp[id]
	}
	if campTotal > math.MaxInt64-capacity.AvailableTroops {
		return 0, 0, "Camp inventory and transfer capacity are too large to allocate safely"
	}
	finalTotal := campTotal + capacity.AvailableTroops
	targets := make([]beriTroopTarget, 0, len(ids))
	baseTotal := int64(0)
	for _, id := range ids {
		product := new(big.Int).Mul(big.NewInt(finalTotal), big.NewInt(weights[id]))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(product, big.NewInt(totalWeight), remainder)
		target := quotient.Int64()
		baseTotal += target
		targets = append(targets, beriTroopTarget{id: id, weight: weights[id], stock: camp[id], target: target, remainder: remainder})
	}
	sort.Slice(targets, func(i, j int) bool {
		if cmp := targets[i].remainder.Cmp(targets[j].remainder); cmp != 0 {
			return cmp > 0
		}
		return targets[i].id < targets[j].id
	})
	for i := int64(0); i < finalTotal-baseTotal; i++ {
		targets[i].target++
	}
	sort.Slice(targets, func(i, j int) bool {
		left, right := targets[i], targets[j]
		leftDeficit, rightDeficit := max(int64(0), left.target-left.stock), max(int64(0), right.target-right.stock)
		cmp := new(big.Int).Mul(big.NewInt(leftDeficit), big.NewInt(right.weight)).Cmp(
			new(big.Int).Mul(big.NewInt(rightDeficit), big.NewInt(left.weight)))
		if cmp != 0 {
			return cmp > 0
		}
		return left.id < right.id
	})
	for _, target := range targets {
		deficit := target.target - target.stock
		if deficit <= 0 {
			continue
		}
		available := min(deficit, capacity.AvailableTroops)
		if exact, present := capacity.TroopsByUnit[target.id]; present {
			available = min(available, exact)
		}
		if available <= 0 {
			return 0, 0, fmt.Sprintf("Waiting for Berimond capacity for preset unit %d", target.id)
		}
		if donor[target.id] <= 0 {
			return 0, 0, fmt.Sprintf("Waiting for donor troops of preset unit %d", target.id)
		}
		return target.id, min(available, donor[target.id]), ""
	}
	return 0, 0, "Waiting for Berimond capacity for the selected preset proportions"
}

package AttackPresets

import (
	"errors"
	"fmt"
	"math"
	"sort"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

// TroopFamilyShortageError describes a family that cannot satisfy one saved
// allocation after stock is divided safely across all requested copies.
type TroopFamilyShortageError struct {
	AnchorID         int64
	RequiredPerCopy  int64
	AvailablePerCopy int64
	Copies           int
}

// InventoryShortage is the first deterministic exact-item or family shortage
// after a preset has been materialized for its requested attack copies.
type InventoryShortage struct {
	ItemID    State.UnitID
	Required  int64
	Available int64
	Family    bool
}

func (problem *TroopFamilyShortageError) Error() string {
	if problem == nil {
		return "troop family inventory is insufficient"
	}
	return fmt.Sprintf(
		"troop family for item %d has %d available per attack across %d attack copies; %d are required",
		problem.AnchorID, problem.AvailablePerCopy, problem.Copies, problem.RequiredPerCopy,
	)
}

// ResolveTroopFamilies materializes a family-enabled preset against one
// castle's current inventory. Every saved troop is treated as an official
// family anchor; higher tiers are consumed first and partial higher-tier
// allocations are retained before lower tiers fill the remaining quantity.
// The returned preset is exact-ID and safe to reuse for the requested number
// of identical attack copies.
func ResolveTroopFamilies(
	preset Preset,
	inventory map[State.UnitID]int64,
	gameData *GameData.Store,
	copies int,
) (Preset, error) {
	if !preset.UseTroopFamilies {
		return preset, nil
	}
	if copies < 1 {
		return Preset{}, fmt.Errorf("troop-family resolution requires at least one attack copy")
	}
	if gameData == nil {
		return Preset{}, fmt.Errorf("troop-family resolution requires the loaded official unit catalog")
	}
	remaining := make(map[State.UnitID]int64, len(inventory))
	for id, quantity := range inventory {
		remaining[id] = max(int64(0), quantity)
	}
	result := preset
	result.UseTroopFamilies = false
	result.Waves = make([]Wave, len(preset.Waves))
	for waveIndex, wave := range preset.Waves {
		left, err := resolveTroopFamilyLane(wave.Left, 2, remaining, gameData, copies, fmt.Sprintf("wave %d left flank", waveIndex+1))
		if err != nil {
			return Preset{}, err
		}
		middle, err := resolveTroopFamilyLane(wave.Middle, 6, remaining, gameData, copies, fmt.Sprintf("wave %d middle flank", waveIndex+1))
		if err != nil {
			return Preset{}, err
		}
		right, err := resolveTroopFamilyLane(wave.Right, 2, remaining, gameData, copies, fmt.Sprintf("wave %d right flank", waveIndex+1))
		if err != nil {
			return Preset{}, err
		}
		result.Waves[waveIndex] = Wave{Left: left, Middle: middle, Right: right}
	}
	supportTroops, err := resolveTroopFamilySlots(
		preset.CourtyardSupport.Troops,
		CourtyardTroopSlots,
		remaining,
		gameData,
		copies,
		"courtyard support",
	)
	if err != nil {
		return Preset{}, err
	}
	result.CourtyardSupport = CourtyardSupport{
		Troops: supportTroops,
		Tools:  append([]Slot(nil), preset.CourtyardSupport.Tools...),
	}
	return result, nil
}

// CheckInventory applies troop-family substitution and then validates every
// concrete troop and tool requirement against the same castle inventory.
func CheckInventory(
	preset Preset,
	inventory map[State.UnitID]int64,
	gameData *GameData.Store,
	copies int,
) (Preset, *InventoryShortage, error) {
	resolved, err := ResolveTroopFamilies(preset, inventory, gameData, copies)
	if err != nil {
		var familyShortage *TroopFamilyShortageError
		if errors.As(err, &familyShortage) {
			return preset, &InventoryShortage{
				ItemID:    State.UnitID(familyShortage.AnchorID),
				Required:  familyShortage.RequiredPerCopy * int64(familyShortage.Copies),
				Available: familyShortage.AvailablePerCopy * int64(familyShortage.Copies),
				Family:    true,
			}, nil
		}
		return Preset{}, nil, err
	}
	if copies < 1 {
		return Preset{}, nil, fmt.Errorf("inventory validation requires at least one attack copy")
	}
	required, _ := Requirements(resolved)
	ids := make([]State.UnitID, 0, len(required))
	for itemID := range required {
		ids = append(ids, itemID)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, itemID := range ids {
		if required[itemID] > math.MaxInt64/int64(copies) {
			return Preset{}, nil, fmt.Errorf("attack preset quantity for item %d is too large", itemID)
		}
		totalRequired := required[itemID] * int64(copies)
		available := max(int64(0), inventory[itemID])
		if totalRequired > available {
			return resolved, &InventoryShortage{
				ItemID: itemID, Required: totalRequired, Available: available,
			}, nil
		}
	}
	return resolved, nil, nil
}

// MaximumAvailableCopies finds the largest identical attack count that the
// current exact tool stock and family-aware troop stock can fully satisfy.
func MaximumAvailableCopies(
	preset Preset,
	inventory map[State.UnitID]int64,
	gameData *GameData.Store,
	maximum int,
) (int, error) {
	for copies := max(0, maximum); copies >= 1; copies-- {
		_, shortage, err := CheckInventory(preset, inventory, gameData, copies)
		if err != nil {
			return 0, err
		}
		if shortage == nil {
			return copies, nil
		}
	}
	return 0, nil
}

// Requirements returns concrete per-attack troop/tool totals and whether the
// formation waves (excluding courtyard support) contain a launch troop.
func Requirements(preset Preset) (map[State.UnitID]int64, bool) {
	requested := map[State.UnitID]int64{}
	hasLaunchTroops := false
	add := func(slots []Slot, launchTroops bool) {
		for _, slot := range slots {
			if slot.ItemID == nil || *slot.ItemID <= 0 || slot.Quantity <= 0 {
				continue
			}
			requested[State.UnitID(*slot.ItemID)] += slot.Quantity
			if launchTroops {
				hasLaunchTroops = true
			}
		}
	}
	for _, wave := range preset.Waves {
		for _, lane := range []Lane{wave.Left, wave.Middle, wave.Right} {
			add(lane.Troops, true)
			add(lane.Tools, false)
		}
	}
	add(preset.CourtyardSupport.Troops, false)
	for _, slot := range preset.CourtyardSupport.Tools {
		if slot.ItemID != nil && *slot.ItemID > 0 {
			requested[State.UnitID(*slot.ItemID)]++
		}
	}
	return requested, hasLaunchTroops
}

func resolveTroopFamilyLane(
	lane Lane,
	capacity int,
	remaining map[State.UnitID]int64,
	gameData *GameData.Store,
	copies int,
	context string,
) (Lane, error) {
	troops, err := resolveTroopFamilySlots(lane.Troops, capacity, remaining, gameData, copies, context)
	if err != nil {
		return Lane{}, err
	}
	return Lane{Troops: troops, Tools: append([]Slot(nil), lane.Tools...)}, nil
}

func resolveTroopFamilySlots(
	slots []Slot,
	capacity int,
	remaining map[State.UnitID]int64,
	gameData *GameData.Store,
	copies int,
	context string,
) ([]Slot, error) {
	resolved := make([]Slot, 0, min(capacity, len(slots)+1))
	for slotIndex, slot := range slots {
		if slot.Quantity < 0 {
			return nil, fmt.Errorf("%s troop slot %d has a negative quantity", context, slotIndex+1)
		}
		if slot.ItemID == nil {
			if slot.Quantity > 0 {
				return nil, fmt.Errorf("%s troop slot %d has a quantity without an item", context, slotIndex+1)
			}
			continue
		}
		if slot.Quantity == 0 {
			continue
		}
		family, err := gameData.UnitUpgradeFamily(*slot.ItemID)
		if err != nil {
			return nil, fmt.Errorf("%s troop slot %d: %w", context, slotIndex+1, err)
		}
		needed := slot.Quantity
		availablePerCopy := int64(0)
		for familyIndex := len(family) - 1; familyIndex >= 0 && needed > 0; familyIndex-- {
			unitID := State.UnitID(family[familyIndex].ID)
			available := remaining[unitID] / int64(copies)
			if available <= 0 {
				continue
			}
			quantity := min(needed, available)
			var appendErr error
			resolved, appendErr = appendResolvedTroopSlot(resolved, capacity, int64(unitID), quantity, context)
			if appendErr != nil {
				return nil, appendErr
			}
			remaining[unitID] -= quantity * int64(copies)
			availablePerCopy += quantity
			needed -= quantity
		}
		if needed > 0 {
			return nil, &TroopFamilyShortageError{
				AnchorID: *slot.ItemID, RequiredPerCopy: slot.Quantity,
				AvailablePerCopy: availablePerCopy, Copies: copies,
			}
		}
	}
	return resolved, nil
}

func appendResolvedTroopSlot(slots []Slot, capacity int, itemID int64, quantity int64, context string) ([]Slot, error) {
	for index := range slots {
		if slots[index].ItemID != nil && *slots[index].ItemID == itemID {
			slots[index].Quantity += quantity
			return slots, nil
		}
	}
	if len(slots) >= capacity {
		return nil, fmt.Errorf("%s needs more than %d troop types after family substitution", context, capacity)
	}
	id := itemID
	return append(slots, Slot{ItemID: &id, Quantity: quantity}), nil
}

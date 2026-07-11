package equipmentview

import (
	"fmt"
	"log"
	"time"
)

const (
	swapUnequipDelay = 500 * time.Millisecond
	swapEquipDelay   = 800 * time.Millisecond
)

var baseEquipmentSlotOrder = []int{1, 2, 3, 4, 6}

type BaseEquipmentSlot struct {
	SlotNumber  int
	Label       string
	EquipmentID float64
}

type EquipmentSwapResult struct {
	Success       bool
	Message       string
	UnequipCount  int
	EquipCount    int
	FailureDetail string
}

// SwapBaseEquipment swaps equipped base pieces and heroes between two commanders or two castellans.
// Gems are not touched; any socketed gem stays attached to the equipment instance being moved.
func SwapBaseEquipment(equipmentMode string, firstIndex int, secondIndex int) EquipmentSwapResult {
	if equipmentMode != "Commander" && equipmentMode != "Castellan" {
		return EquipmentSwapResult{Success: false, Message: "Invalid equipment mode"}
	}
	if firstIndex == secondIndex {
		return EquipmentSwapResult{Success: false, Message: "Select two different loadouts"}
	}
	if GetLeaderID(equipmentMode, firstIndex) == 0 || GetLeaderID(equipmentMode, secondIndex) == 0 {
		return EquipmentSwapResult{Success: false, Message: "Selected commander/castellan is not available"}
	}

	firstLoadout := CurrentBaseEquipmentLoadout(equipmentMode, firstIndex)
	secondLoadout := CurrentBaseEquipmentLoadout(equipmentMode, secondIndex)
	if len(nonEmptyBaseSlots(firstLoadout))+len(nonEmptyBaseSlots(secondLoadout)) == 0 {
		return EquipmentSwapResult{Success: false, Message: "No base equipment to swap"}
	}

	result := EquipmentSwapResult{Success: true}
	for _, item := range nonEmptyBaseSlots(firstLoadout) {
		if !unequipForSwap(equipmentMode, firstIndex, item, &result) {
			return result
		}
	}
	for _, item := range nonEmptyBaseSlots(secondLoadout) {
		if !unequipForSwap(equipmentMode, secondIndex, item, &result) {
			return result
		}
	}

	for _, item := range nonEmptyBaseSlots(secondLoadout) {
		if !equipForSwap(equipmentMode, firstIndex, item, &result) {
			return result
		}
	}
	for _, item := range nonEmptyBaseSlots(firstLoadout) {
		if !equipForSwap(equipmentMode, secondIndex, item, &result) {
			return result
		}
	}

	result.Message = fmt.Sprintf("Swapped %d base equipment item(s)", result.EquipCount)
	return result
}

func GetLeaderID(equipmentMode string, targetIndex int) float64 {
	if equipmentMode == "Commander" {
		return GetCommanderID(targetIndex)
	}
	if equipmentMode == "Castellan" {
		return GetCastellanID(targetIndex)
	}
	return 0
}

func CurrentBaseEquipmentLoadout(equipmentMode string, targetIndex int) []BaseEquipmentSlot {
	loadout := make([]BaseEquipmentSlot, 0, len(baseEquipmentSlotOrder))
	for _, slot := range baseEquipmentSlotOrder {
		loadout = append(loadout, BaseEquipmentSlot{
			SlotNumber:  slot,
			Label:       equipmentSlotLabels[slot],
			EquipmentID: equippedItemID(equipmentMode, targetIndex, slot),
		})
	}
	return loadout
}

func nonEmptyBaseSlots(loadout []BaseEquipmentSlot) []BaseEquipmentSlot {
	out := make([]BaseEquipmentSlot, 0, len(loadout))
	for _, slot := range loadout {
		if slot.EquipmentID != 0 {
			out = append(out, slot)
		}
	}
	return out
}

func unequipForSwap(equipmentMode string, targetIndex int, item BaseEquipmentSlot, result *EquipmentSwapResult) bool {
	log.Printf("[SwapEquipment] Unequipping %s index %d slot %d (%s) item %.0f", equipmentMode, targetIndex, item.SlotNumber, item.Label, item.EquipmentID)
	unequipResult := UnequipEquipmentRaw(equipmentMode, targetIndex, item.EquipmentID)
	if !unequipResult.Success {
		result.Success = false
		result.FailureDetail = unequipResult.Message
		result.Message = fmt.Sprintf("Failed to unequip %s item %.0f: %s", item.Label, item.EquipmentID, unequipResult.Message)
		return false
	}
	result.UnequipCount++
	time.Sleep(swapUnequipDelay)
	return true
}

func equipForSwap(equipmentMode string, targetIndex int, item BaseEquipmentSlot, result *EquipmentSwapResult) bool {
	log.Printf("[SwapEquipment] Equipping %s index %d slot %d (%s) item %.0f", equipmentMode, targetIndex, item.SlotNumber, item.Label, item.EquipmentID)
	if !EquipEquipment(equipmentMode, targetIndex, item.SlotNumber, item.EquipmentID) {
		result.Success = false
		result.FailureDetail = "game rejected equip command"
		result.Message = fmt.Sprintf("Failed to equip %s item %.0f", item.Label, item.EquipmentID)
		return false
	}
	result.EquipCount++
	time.Sleep(swapEquipDelay)
	return true
}

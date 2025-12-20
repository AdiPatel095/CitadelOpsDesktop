package GameWebsocket

import (
	"CitadelDesktop/Server/Models"
	"fmt"
	"log"
)

// UnequipEquipment removes equipment from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The slot to unequip (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// expectedEquipmentId: The equipment ID the frontend expects to unequip (for validation)
// Returns true if successful, false if validation fails
// UnequipEquipment removes equipment from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The slot to unequip (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// expectedEquipmentId: The equipment ID the frontend expects to unequip (for validation)
// Returns true if successful, false if validation fails
func UnequipEquipment(equipmentMode string, targetIndex int, slotNumber int, expectedEquipmentId float64) bool {

	var actualEquipmentId float64
	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
		if targetIndex >= 0 && targetIndex < len(Models.CommStatArray) {
			comm := Models.CommStatArray[targetIndex]
			switch slotNumber {
			case 1:
				actualEquipmentId = comm.Equip1
			case 2:
				actualEquipmentId = comm.Equip2
			case 3:
				actualEquipmentId = comm.Equip3
			case 4:
				actualEquipmentId = comm.Equip4
			case 6:
				actualEquipmentId = comm.Hero
			}
		}
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
		cast := GetCastellanStat(targetIndex)
		switch slotNumber {
		case 1:
			actualEquipmentId = cast.Equip1
		case 2:
			actualEquipmentId = cast.Equip2
		case 3:
			actualEquipmentId = cast.Equip3
		case 4:
			actualEquipmentId = cast.Equip4
		case 6:
			actualEquipmentId = cast.Hero
		}
	} else {
		return false
	}

	log.Printf("[Unequip Internal Debug] Mode=%s, Index=%d, Slot=%d -> LocalModelID=%.0f, ExpectedID=%.0f, LID=%.0f", equipmentMode, targetIndex, slotNumber, actualEquipmentId, expectedEquipmentId, lidValue)

	// Logic Update: Prioritize what the frontend sends (user intent), but log mismatch
	// Use expectedEquipmentId as the ID for the payload
	payloadEquipmentId := expectedEquipmentId

	// Validation / Warning logic
	if actualEquipmentId != 0 {
		// Backend thinks something is equipped
		if actualEquipmentId != expectedEquipmentId {
			fmt.Printf("[Sort Warning] Unequip mismatch! Slot: %d. Backend has: %.0f, Frontend sent: %.0f. Proceeding with frontend ID.\n", slotNumber, actualEquipmentId, expectedEquipmentId)
		}
	} else {
		// Backend thinks nothing is equipped
		if expectedEquipmentId != 0 {
			fmt.Printf("[Sort Warning] Backend thinks slot %d is empty, but frontend wants to unequip %.0f. Proceeding.\n", slotNumber, expectedEquipmentId)
		} else {
			// Both are 0, nothing to unequip
			return false
		}
	}

	// Game message format: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	// E:0 means unequip, E:1 means equip
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":0}%%`, payloadEquipmentId, lidValue)
	OutgoingMessages <- OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
	return true
}

// UnequipGem removes a gem from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The gem slot to unequip (1-4)
// expectedGemId: The gem ID the frontend expects to unequip (for validation)
// expectedEquipmentId: The equipment ID the gem is attached to (for validation)
// Note: The gem is removed from the equipment piece in that slot. Equipment must be equipped.
// Returns true if successful, false if validation fails
func UnequipGem(equipmentMode string, targetIndex int, slotNumber int, expectedGemId float64, expectedEquipmentId float64) bool {

	var actualEquipmentId float64
	var actualGemId float64
	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
		if targetIndex >= 0 && targetIndex < len(Models.CommStatArray) {
			comm := Models.CommStatArray[targetIndex]
			switch slotNumber {
			case 1:
				actualEquipmentId = comm.Equip1
				actualGemId = comm.Gem1
			case 2:
				actualEquipmentId = comm.Equip2
				actualGemId = comm.Gem2
			case 3:
				actualEquipmentId = comm.Equip3
				actualGemId = comm.Gem3
			case 4:
				actualEquipmentId = comm.Equip4
				actualGemId = comm.Gem4
			}
		}
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
		cast := GetCastellanStat(targetIndex)
		switch slotNumber {
		case 1:
			actualEquipmentId = cast.Equip1
			actualGemId = cast.Gem1
		case 2:
			actualEquipmentId = cast.Equip2
			actualGemId = cast.Gem2
		case 3:
			actualEquipmentId = cast.Equip3
			actualGemId = cast.Gem3
		case 4:
			actualEquipmentId = cast.Equip4
			actualGemId = cast.Gem4
		}
	} else {
		return false
	}

	// Logic Update: Prioritize what the frontend sends (user intent), but log mismatch
	// Use expectedEquipmentId as the EID for the payload (Gem unequip uses EID of the parent item)
	payloadEquipmentId := expectedEquipmentId

	// Validation / Warning logic
	if actualEquipmentId != 0 {
		if actualEquipmentId != expectedEquipmentId {
			fmt.Printf("[Sort Warning] UnequipGem mismatch (Parent Item)! Slot: %d. Backend EID: %.0f, Frontend EID: %.0f. Proceeding with frontend ID.\n", slotNumber, actualEquipmentId, expectedEquipmentId)
		}
		if actualGemId != expectedGemId {
			fmt.Printf("[Sort Warning] UnequipGem mismatch (Gem)! Slot: %d. Backend GID: %.0f, Frontend GID: %.0f. Proceeding.\n", slotNumber, actualGemId, expectedGemId)
		}
	} else {
		// Backend thinks parent item is missing
		fmt.Printf("[Sort Warning] Backend thinks slot %d is empty (no parent), but frontend wants to unequip gem %.0f from item %.0f. Proceeding.\n", slotNumber, expectedGemId, expectedEquipmentId)
	}

	// Game message format: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	// Note: Unequip gem payload DOES NOT use gem ID, it uses the EQUIPMENT ID the gem is in.
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%.0f}%%`, payloadEquipmentId, lidValue)
	OutgoingMessages <- OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
	return true
}

// UnequipEquipmentRaw un-equips an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipEquipmentRaw(equipmentMode string, targetIndex int, equipmentId float64) bool {

	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return false
	}

	// Payload: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":0}%%`, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

// UnequipGemRaw un-equips a gem from an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipGemRaw(equipmentMode string, targetIndex int, equipmentId float64) bool {

	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return false
	}

	// Payload: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	// Note: Standard unequip doesn't seem to send GID, just EID+LID.
	//%xt%EmpireEx_21%ege%1%{"EID":6365410569,"LID":2}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%.0f}%%`, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

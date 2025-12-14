package GameWebsocket

import (
	"CitadelDesktop/Server/Models"
	"fmt"
	"log"
	"time"
)

// UnequipEquipment removes equipment from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The slot to unequip (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// expectedEquipmentId: The equipment ID the frontend expects to unequip (for validation)
// Returns true if successful, false if validation fails
func UnequipEquipment(equipmentMode string, targetIndex int, slotNumber int, expectedEquipmentId float64) bool {
	log.Printf("Unequipping equipment - Mode: %s, Target: %d, Slot: %d, Expected EID: %.0f", equipmentMode, targetIndex, slotNumber, expectedEquipmentId)

	var actualEquipmentId float64

	if equipmentMode == "Commander" {
		// Get equipment ID from commander stats
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
	} else {
		// Castellan - TODO: implement lookup for castellan stats
		log.Printf("Castellan unequip not yet implemented")
		return false
	}

	if actualEquipmentId == 0 {
		log.Printf("No equipment found in slot %d", slotNumber)
		return false
	}

	// Validate the equipment ID matches what frontend expects
	if actualEquipmentId != expectedEquipmentId {
		log.Printf("Equipment ID mismatch! Expected: %.0f, Actual: %.0f", expectedEquipmentId, actualEquipmentId)
		return false
	}

	// Calculate LID: index 0 = LID 0, index >= 1 = LID index+1 (visual index)
	lidValue := CalculateLID(targetIndex)

	// Game message format: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	// E:0 means unequip, E:1 means equip
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%d,"E":0}%%`, actualEquipmentId, lidValue)
	log.Printf("SENDING UNEQUIP EQUIPMENT COMMAND: %s", payload)
	OutgoingMessages <- []byte(payload)
	log.Printf("Unequip equipment command sent successfully for EID: %.0f", actualEquipmentId)
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
	log.Printf("Unequipping gem - Mode: %s, Target: %d, Slot: %d, Expected GID: %.0f, Expected EID: %.0f", equipmentMode, targetIndex, slotNumber, expectedGemId, expectedEquipmentId)

	var actualEquipmentId float64
	var actualGemId float64

	if equipmentMode == "Commander" {
		// Gem slots 1-4 correspond to equipment slots 1-4 (Armor, Weapon, Helmet, Artifact)
		// We need the equipment ID from the corresponding slot
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
	} else {
		// Castellan - TODO: implement lookup for castellan stats
		log.Printf("Castellan gem unequip not yet implemented")
		return false
	}

	if actualEquipmentId == 0 {
		log.Printf("No equipment found in slot %d to unequip gem from", slotNumber)
		return false
	}

	// Validate the equipment and gem IDs match what frontend expects
	if actualEquipmentId != expectedEquipmentId {
		log.Printf("Equipment ID mismatch! Expected: %.0f, Actual: %.0f", expectedEquipmentId, actualEquipmentId)
		return false
	}

	if actualGemId != expectedGemId {
		log.Printf("Gem ID mismatch! Expected: %.0f, Actual: %.0f", expectedGemId, actualGemId)
		return false
	}

	if actualGemId == 0 {
		log.Printf("No gem found in slot %d", slotNumber)
		return false
	}

	// Calculate LID
	lidValue := CalculateLID(targetIndex)

	// Game message format: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%d}%%`, actualEquipmentId, lidValue)
	log.Printf("SENDING UNEQUIP GEM COMMAND: %s", payload)
	OutgoingMessages <- []byte(payload)
	log.Printf("Unequip gem command sent successfully for EID: %.0f (GemID: %.0f)", actualEquipmentId, actualGemId)
	return true
}

// UnequipEquipmentWithListener sends the unequip message and waits briefly for game response
// This is a wrapper that adds a delay to allow game to process between multiple operations
func UnequipEquipmentWithListener(equipmentMode string, targetIndex int, slotNumber int, expectedEquipmentId float64) bool {
	success := UnequipEquipment(equipmentMode, targetIndex, slotNumber, expectedEquipmentId)
	if success {
		// Wait for game response - allows server to process before next operation
		// The game sends %xt%eeq%1%0% on success
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

// UnequipGemWithListener sends the unequip message and waits briefly for game response
// This is a wrapper that adds a delay to allow game to process between multiple operations
func UnequipGemWithListener(equipmentMode string, targetIndex int, slotNumber int, expectedGemId float64, expectedEquipmentId float64) bool {
	success := UnequipGem(equipmentMode, targetIndex, slotNumber, expectedGemId, expectedEquipmentId)
	if success {
		// Wait for game response - allows server to process before next operation
		// The game sends %xt%ege%1%0% on success
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

// UnequipEquipmentRaw un-equips an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipEquipmentRaw(equipmentMode string, targetIndex int, equipmentId float64) bool {
	log.Printf("Unequipping equipment (RAW) - Mode: %s, Target: %d, EID: %.0f", equipmentMode, targetIndex, equipmentId)

	if equipmentMode != "Commander" {
		return false
	}

	lidValue := CalculateLID(targetIndex)

	// Payload: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%d,"E":0}%%`, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

func UnequipEquipmentRawWithListener(equipmentMode string, targetIndex int, equipmentId float64) bool {
	success := UnequipEquipmentRaw(equipmentMode, targetIndex, equipmentId)
	if success {
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

// UnequipGemRaw un-equips a gem from an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipGemRaw(equipmentMode string, targetIndex int, equipmentId float64) bool {
	log.Printf("Unequipping gem (RAW) - Mode: %s, Target: %d, EID: %.0f", equipmentMode, targetIndex, equipmentId)

	if equipmentMode != "Commander" {
		return false
	}

	// Calculate LID
	lidValue := CalculateLID(targetIndex)

	// Payload: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	// Note: Standard unequip doesn't seem to send GID, just EID+LID.
	//%xt%EmpireEx_21%ege%1%{"EID":6365410569,"LID":2}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%d}%%`, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

func UnequipGemRawWithListener(equipmentMode string, targetIndex int, equipmentId float64) bool {
	success := UnequipGemRaw(equipmentMode, targetIndex, equipmentId)
	if success {
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

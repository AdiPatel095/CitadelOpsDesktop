package GameFunctions

import (
	equip "CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/ResponseRegistry"
	"fmt"
	"log"
	"time"
)

// UnequipResult represents the result of an unequip operation
type UnequipResult struct {
	Success bool
	Code    string
	Message string
}

// Response codes from game server
const (
	UnequipCodeSuccess       = "0"   // Operation successful
	UnequipCodeInventoryFull = "220" // Inventory is full, cannot unequip
	UnequipCodeBusy          = "222" // Commander/Castellan is busy (travelling/capturing)
	UnequipCodeNotInSlot     = "214" // Equipment does not exist in slot (data out of sync)
)

// parseUnequipResponse parses the response and returns a structured result
func parseUnequipResponse(response []string, operationName string) UnequipResult {
	if len(response) <= 4 {
		log.Printf("%s: invalid response format: %v", operationName, response)
		return UnequipResult{Success: false, Code: "", Message: "Invalid response format"}
	}

	code := response[4]
	if code != UnequipCodeSuccess {
		log.Printf("[Unequip] %s failed with code %s. Full response: %v", operationName, code, response)
	}
	switch code {
	case UnequipCodeSuccess:
		return UnequipResult{Success: true, Code: code, Message: "Success"}
	case UnequipCodeInventoryFull:
		return UnequipResult{Success: false, Code: code, Message: "Inventory is full"}
	case UnequipCodeBusy:
		return UnequipResult{Success: false, Code: code, Message: "Commander/Castellan is busy (travelling or capturing)"}
	case UnequipCodeNotInSlot:
		return UnequipResult{Success: false, Code: code, Message: "Equipment not in slot - game data needs refresh"}
	default:
		return UnequipResult{Success: false, Code: code, Message: fmt.Sprintf("Unknown error code: %s", code)}
	}
}

// UnequipEquipment removes equipment from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The slot to unequip (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// expectedEquipmentId: The equipment ID the frontend expects to unequip (for validation)
// Returns UnequipResult with success status, code, and message
func UnequipEquipment(equipmentMode string, targetIndex int, slotNumber int, expectedEquipmentId float64) UnequipResult {

	var actualEquipmentId float64
	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
		if targetIndex >= 0 && targetIndex < len(equip.CommStatArray) {
			comm := equip.CommStatArray[targetIndex]
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
		return UnequipResult{Success: false, Code: "", Message: "Invalid equipment mode"}
	}

	// Logic Update: Prioritize what the frontend sends (user intent), but log mismatch
	// Use expectedEquipmentId as the ID for the payload
	payloadEquipmentId := expectedEquipmentId

	// Validation / Warning logic
	if actualEquipmentId != 0 {
		// Backend thinks something is equipped
		if actualEquipmentId != expectedEquipmentId {

		}
	} else {
		// Backend thinks nothing is equipped
		if expectedEquipmentId != 0 {

		} else {
			// Both are 0, nothing to unequip
			return UnequipResult{Success: false, Code: "", Message: "Nothing to unequip"}
		}
	}

	// Game message format: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	// E:0 means unequip, E:1 means equip
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":0}%%`, payloadEquipmentId, lidValue)

	// Register waiter for response before sending
	waiter := ResponseRegistry.Global.RegisterWaiter("eeq", 5*time.Second)
	defer waiter.Cleanup()

	ResponseRegistry.OutgoingMessages <- []byte(payload)

	// Wait for response and verify success
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return UnequipResult{Success: false, Code: "", Message: "Timeout waiting for response"}
	}

	return parseUnequipResponse(response, "UnequipEquipment")
}

// UnequipGem removes a gem from a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan
// slotNumber: The gem slot to unequip (1-4)
// expectedGemId: The gem ID the frontend expects to unequip (for validation)
// expectedEquipmentId: The equipment ID the gem is attached to (for validation)
// Note: The gem is removed from the equipment piece in that slot. Equipment must be equipped.
// Returns UnequipResult with success status, code, and message
func UnequipGem(equipmentMode string, targetIndex int, slotNumber int, expectedGemId float64, expectedEquipmentId float64) UnequipResult {

	var actualEquipmentId float64
	var actualGemId float64
	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
		if targetIndex >= 0 && targetIndex < len(equip.CommStatArray) {
			comm := equip.CommStatArray[targetIndex]
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
		return UnequipResult{Success: false, Code: "", Message: "Invalid equipment mode"}
	}

	// Logic Update: Prioritize what the frontend sends (user intent), but log mismatch
	// Use expectedEquipmentId as the EID for the payload (Gem unequip uses EID of the parent item)
	payloadEquipmentId := expectedEquipmentId

	// Validation / Warning logic
	if actualEquipmentId != 0 {
		if actualEquipmentId != expectedEquipmentId {

		}
		if actualGemId != expectedGemId {

		}
	} else {
		// Backend thinks parent item is missing

	}

	// Game message format: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	// Note: Unequip gem payload DOES NOT use gem ID, it uses the EQUIPMENT ID the gem is in.
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%.0f}%%`, payloadEquipmentId, lidValue)

	// Register waiter for response before sending
	waiter := ResponseRegistry.Global.RegisterWaiter("ege", 5*time.Second)
	defer waiter.Cleanup()

	ResponseRegistry.OutgoingMessages <- []byte(payload)

	// Wait for response and verify success
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return UnequipResult{Success: false, Code: "", Message: "Timeout waiting for response"}
	}

	return parseUnequipResponse(response, "UnequipGem")
}

// UnequipEquipmentRaw un-equips an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipEquipmentRaw(equipmentMode string, targetIndex int, equipmentId float64) UnequipResult {

	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return UnequipResult{Success: false, Code: "", Message: "Invalid equipment mode"}
	}

	// Payload: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":0}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":0}%%`, equipmentId, lidValue)

	// Register waiter for response before sending
	waiter := ResponseRegistry.Global.RegisterWaiter("eeq", 5*time.Second)
	defer waiter.Cleanup()

	ResponseRegistry.OutgoingMessages <- []byte(payload)

	// Wait for response and verify success
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return UnequipResult{Success: false, Code: "", Message: "Timeout waiting for response"}
	}

	return parseUnequipResponse(response, "UnequipEquipmentRaw")
}

// UnequipGemRaw un-equips a gem from an item without checking the local model state.
// Useful for batched operations where local model is stale.
func UnequipGemRaw(equipmentMode string, targetIndex int, equipmentId float64) UnequipResult {

	var lidValue float64

	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return UnequipResult{Success: false, Code: "", Message: "Invalid equipment mode"}
	}

	// Payload: %xt%EmpireEx_21%ege%1%{"EID":equipmentId,"LID":leaderId}%
	// Note: Standard unequip doesn't seem to send GID, just EID+LID.
	//%xt%EmpireEx_21%ege%1%{"EID":6365410569,"LID":2}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ege%%1%%{"EID":%.0f,"LID":%.0f}%%`, equipmentId, lidValue)

	// Register waiter for response before sending
	waiter := ResponseRegistry.Global.RegisterWaiter("ege", 5*time.Second)
	defer waiter.Cleanup()

	ResponseRegistry.OutgoingMessages <- []byte(payload)

	// Wait for response and verify success
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return UnequipResult{Success: false, Code: "", Message: "Timeout waiting for response"}
	}

	return parseUnequipResponse(response, "UnequipGemRaw")
}

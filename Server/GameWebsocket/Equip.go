package GameWebsocket

import (
	"fmt"
	"log"
	"time"
)

// EquipEquipment equips an item to a commander
// equipmentMode: "Commander" only for now
// targetIndex: The index of the commander (0-49)
// slotNumber: The slot to equip to (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// equipmentId: The equipment ID to equip
func EquipEquipment(equipmentMode string, targetIndex int, slotNumber int, equipmentId float64) bool {
	log.Printf("Equipping equipment - Mode: %s, Target: %d, Slot: %d, EID: %.0f", equipmentMode, targetIndex, slotNumber, equipmentId)

	if equipmentMode != "Commander" {
		log.Printf("Castellan equip not yet implemented")
		return false
	}

	// Calculate LID
	lidValue := CalculateLID(targetIndex)

	// Game message format: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":1}%
	// E:1 means equip
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%d,"E":1}%%`, equipmentId, lidValue)
	log.Printf("SENDING EQUIP EQUIPMENT COMMAND: %s", payload)
	OutgoingMessages <- []byte(payload)
	return true
}

func EquipEquipmentWithListener(equipmentMode string, targetIndex int, slotNumber int, equipmentId float64) bool {
	success := EquipEquipment(equipmentMode, targetIndex, slotNumber, equipmentId)
	if success {
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

// EquipGem equips a gem to an item on a commander
// equipmentMode: "Commander"
// targetIndex: Commander index
// equipmentId: The equipment ID to attach the gem to (must be equipped on commander)
// gemId: The gem ID to equip
func EquipGem(equipmentMode string, targetIndex int, equipmentId float64, gemId float64) bool {
	log.Printf("Equipping gem - Mode: %s, Target: %d, EID: %.0f, GID: %.0f", equipmentMode, targetIndex, equipmentId, gemId)

	if equipmentMode != "Commander" {
		return false
	}

	// Calculate LID
	lidValue := CalculateLID(targetIndex)

	// Game message format: %xt%EmpireEx_21%bge%1%{"GID":gemId,"EID":equipmentId,"LID":leaderId,"M":0,"RGEM":1}%
	// Updated based on user feedback to use bge and correct parameter mapping
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%bge%%1%%{"GID":%.0f,"EID":%.0f,"LID":%d,"M":0,"RGEM":1}%%`, gemId, equipmentId, lidValue)
	log.Printf("SENDING EQUIP GEM COMMAND: %s", payload)
	OutgoingMessages <- []byte(payload)
	return true
}

func EquipGemWithListener(equipmentMode string, targetIndex int, equipmentId float64, gemId float64) bool {
	success := EquipGem(equipmentMode, targetIndex, equipmentId, gemId)
	if success {
		time.Sleep(500 * time.Millisecond)
	}
	return success
}

package GameWebsocket

import (
	"fmt"
)

// EquipEquipment equips an item to a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: The index of the commander (0-49) or castellan (0-7)
// slotNumber: The slot to equip to (1=Armor, 2=Weapon, 3=Helmet, 4=Artifact, 6=Hero)
// equipmentId: The equipment ID to equip
func EquipEquipment(equipmentMode string, targetIndex int, slotNumber int, equipmentId float64) bool {

	var lidValue float64
	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return false
	}

	if lidValue == 0 {
		return false
	}

	// Game message format: %xt%EmpireEx_21%eeq%1%{"EID":equipmentId,"LID":leaderId,"E":1}%
	// E:1 means equip
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%eeq%%1%%{"EID":%.0f,"LID":%.0f,"E":1}%%`, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

// EquipGem equips a gem to an item on a commander or castellan
// equipmentMode: "Commander" or "Castellan"
// targetIndex: Commander or Castellan index
// equipmentId: The equipment ID to attach the gem to (must be equipped on leader)
// gemId: The gem ID to equip
func EquipGem(equipmentMode string, targetIndex int, equipmentId float64, gemId float64) bool {

	var lidValue float64
	if equipmentMode == "Commander" {
		lidValue = GetCommanderID(targetIndex)
	} else if equipmentMode == "Castellan" {
		lidValue = GetCastellanID(targetIndex)
	} else {
		return false
	}

	if lidValue == 0 {
		return false
	}

	// Game message format: %xt%EmpireEx_21%bge%1%{"GID":gemId,"EID":equipmentId,"LID":leaderId,"M":0,"RGEM":1}%
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%bge%%1%%{"GID":%.0f,"EID":%.0f,"LID":%.0f,"M":0,"RGEM":1}%%`, gemId, equipmentId, lidValue)
	OutgoingMessages <- []byte(payload)
	return true
}

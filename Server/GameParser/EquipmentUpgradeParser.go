package GameParser

import (
	"CitadelDesktop/Server/Models"
)

// EquipmentLevelFromEQEPayload reads the post-upgrade level from an **eqe** response body map.
func EquipmentLevelFromEQEPayload(payload map[string]interface{}) (level int, ok bool) {
	eRaw, exists := payload["E"]
	if !exists {
		return 0, false
	}
	eArr, ok := eRaw.([]interface{})
	if !ok || len(eArr) <= 8 {
		return 0, false
	}
	lvl, ok := eArr[8].(float64)
	if !ok {
		return 0, false
	}
	return int(lvl), true
}

// ApplyEQEResponse merges one **eqe** equipment row into storage and equipped loadouts.
func ApplyEQEResponse(payload map[string]interface{}) {
	eRaw, exists := payload["E"]
	if !exists {
		return
	}
	eArr, ok := eRaw.([]interface{})
	if !ok {
		return
	}
	var equipment Models.EquipmentModel
	ProcessEquipment(eArr, &equipment)
	if equipment.ID == 0 {
		return
	}

	gs := Models.GetGameState()
	for i := range gs.Equipment.EquipmentStorage {
		if gs.Equipment.EquipmentStorage[i].ID == equipment.ID {
			gs.Equipment.EquipmentStorage[i] = equipment
			break
		}
	}
	for i := range gs.Equipment.CommActualArray {
		for j := range gs.Equipment.CommActualArray[i].Equipment {
			if gs.Equipment.CommActualArray[i].Equipment[j].ID == equipment.ID {
				gs.Equipment.CommActualArray[i].Equipment[j] = equipment
			}
		}
	}
	for i := range gs.Equipment.CastActualArray {
		for j := range gs.Equipment.CastActualArray[i].Equipment {
			if gs.Equipment.CastActualArray[i].Equipment[j].ID == equipment.ID {
				gs.Equipment.CastActualArray[i].Equipment[j] = equipment
			}
		}
	}
}

package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

func UpdateGemStorage(gemStorageString string) {
	gs := Models.GetGameState()
	var gemStorageMap map[string]interface{}
	if err := json.Unmarshal([]byte(gemStorageString), &gemStorageMap); err != nil {
		log.Printf("[parser] ggm unmarshal: %v", err)
		return
	}
	gs.Equipment.NonRelicGemIDs = make(map[float64]float64)
	nonRelicGemArray, ok := gemStorageMap["GEM"].([]interface{})
	if !ok {
		log.Printf("[parser] ggm: GEM not an array")
		return
	}
	for _, gem := range nonRelicGemArray {
		gemRawArray, ok := gem.([]interface{})
		if !ok || len(gemRawArray) < 2 {
			continue
		}
		id, ok0 := gemRawArray[0].(float64)
		count, ok1 := gemRawArray[1].(float64)
		if !ok0 || !ok1 {
			continue
		}
		gs.Equipment.NonRelicGemIDs[id] = count
	}

	relicGemArray, ok := gemStorageMap["RGEM"].([]interface{})
	if !ok {
		log.Printf("[parser] ggm: RGEM not an array")
		return
	}
	gs.Equipment.GemsStorage = make([]Models.Gem, 0, len(relicGemArray))
	for _, gem := range relicGemArray {
		gemRawArray, ok := gem.([]interface{})
		if !ok {
			log.Printf("[parser] ggm: relic gem row not an array, skipping")
			continue
		}
		var gemFinal Models.Gem
		ProcessGem(gemRawArray, &gemFinal, 5)
		if gemFinal.GemStats != nil {
			gs.Equipment.GemsStorage = append(gs.Equipment.GemsStorage, gemFinal)
		}
	}
}

func UpdateEquipmentStorage(storageMap string) {
	gs := Models.GetGameState()
	var storageMapUnmarshalled map[string]interface{}
	if err := json.Unmarshal([]byte(storageMap), &storageMapUnmarshalled); err != nil {
		log.Printf("[parser] gei unmarshal: %v", err)
		return
	}
	equipmentRawArray, ok := storageMapUnmarshalled["I"].([]interface{})
	if !ok {
		log.Printf("[parser] gei: I not an array")
		return
	}

	gs.Equipment.EquipmentStorage = make([]Models.EquipmentModel, 0, len(equipmentRawArray))
	for _, equipmentData := range equipmentRawArray {
		equipmentDataArray, ok := equipmentData.([]interface{})
		if !ok {
			continue
		}
		var equipmentFinal Models.EquipmentModel
		ProcessEquipment(equipmentDataArray, &equipmentFinal)
		gs.Equipment.EquipmentStorage = append(gs.Equipment.EquipmentStorage, equipmentFinal)
	}
}

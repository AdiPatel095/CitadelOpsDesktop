package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

func UpdateGemStorage(gemStorageString string) {
	gs := Models.GetGameState()
	var gemStorageMap map[string]interface{}
	_ = json.Unmarshal([]byte(gemStorageString), &gemStorageMap)
	gs.Equipment.NonRelicGemIDs = make(map[float64]float64)
	nonRelicGemArray, ok := gemStorageMap["GEM"].([]interface{})
	if !ok {
		log.Fatal("nonRelicGemArray is not a slice")
	}
	for _, gem := range nonRelicGemArray {
		gemRawArray, ok := gem.([]interface{})
		if ok {
			id := gemRawArray[0].(float64)
			count := gemRawArray[1].(float64)
			gs.Equipment.NonRelicGemIDs[id] = count
		}
	}

	relicGemArray, ok := gemStorageMap["RGEM"].([]interface{})
	if !ok {
		log.Fatal("relicGemArray is not a slice")
	}
	gs.Equipment.GemsStorage = make([]Models.Gem, 0, len(relicGemArray))
	for _, gem := range relicGemArray {
		gemRawArray, ok := gem.([]interface{})
		if !ok {
			log.Fatal("gemRawArray is not a slice")
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
	err := json.Unmarshal([]byte(storageMap), &storageMapUnmarshalled)
	if err != nil {
		log.Fatal(err)
	}
	equipmentRawArray, ok := storageMapUnmarshalled["I"].([]interface{})
	if !ok {
		log.Fatal("equipmentRawArray is not a slice")
	}

	gs.Equipment.EquipmentStorage = make([]Models.EquipmentModel, 0, len(equipmentRawArray))
	for _, equipmentData := range equipmentRawArray {
		equipmentDataArray, ok := equipmentData.([]interface{})
		if !ok {
			log.Fatal("equipmentDataArray is not a slice")
		}

		var equipmentFinal Models.EquipmentModel
		ProcessEquipment(equipmentDataArray, &equipmentFinal)

		gs.Equipment.EquipmentStorage = append(gs.Equipment.EquipmentStorage, equipmentFinal)
	}
}

package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

func UpdateGemStorage(gemStorageMap map[string]interface{}) {
	relicGemArray, ok := gemStorageMap["RGEM"].([]interface{})
	if !ok {
		log.Fatal("relicGemArray is not a slice")
	}
	Models.GemsStorage = make([]Models.Gem, 0, len(relicGemArray))
	for _, gem := range relicGemArray {
		gemRawArray, ok := gem.([]interface{})
		if !ok {
			log.Fatal("gemRawArray is not a slice")
		}
		var gemFinal Models.Gem
		ProcessGem(gemRawArray, &gemFinal, 5)
		if gemFinal.GemStats != nil {
			Models.GemsStorage = append(Models.GemsStorage, gemFinal)
		}
	}
}

func UpdateEquipmentStorage(storageMap string) {
	var storageMapUnmarshalled map[string]interface{}
	err := json.Unmarshal([]byte(storageMap), &storageMapUnmarshalled)
	if err != nil {
		log.Fatal(err)
	}
	equipmentRawArray, ok := storageMapUnmarshalled["I"].([]interface{})
	if !ok {
		log.Fatal("equipmentRawArray is not a slice")
	}
	Models.EquipmentStorage = make([]Models.EquipmentModel, 0, len(equipmentRawArray))
	for _, equipmentData := range equipmentRawArray {
		var equipmentFinal Models.EquipmentModel
		equipmentDataArray, ok := equipmentData.([]interface{})
		if !ok {
			log.Fatal("equipmentDataArray is not a slice")
		}
		ProcessEquipment(equipmentDataArray, &equipmentFinal)
		Models.EquipmentStorage = append(Models.EquipmentStorage, equipmentFinal)
	}
}

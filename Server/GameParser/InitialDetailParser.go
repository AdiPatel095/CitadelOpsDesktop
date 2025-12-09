package GameParser

import (
	"encoding/json"
	"log"
	"sync"
)

func InitiateDetails(data string) {
	byteString := []byte(data)
	var jsonDataInterface interface{}
	err := json.Unmarshal(byteString, &jsonDataInterface)
	if err != nil {
		log.Printf("Error unmarshalling: %v", err)
	}
	jsonDataMap, ok := jsonDataInterface.(map[string]interface{})
	if !ok {
		log.Fatal("jsonDataMap is not a map")
	}

	var wgInitialDetail sync.WaitGroup
	wgInitialDetail.Add(7)

	go func() {
		defer wgInitialDetail.Done()
		gcuMap, ok := jsonDataMap["gcu"].(map[string]interface{})
		if !ok {
			log.Fatal("gcuMap is not a map")
		}
		UpdateCoins(gcuMap)
	}()

	go func() {
		defer wgInitialDetail.Done()
		sceArray, ok := jsonDataMap["sce"].([]interface{})
		if !ok {
			log.Fatal("sceArray is not a slice")
		}
		UpdateSCE(sceArray)
	}()

	go func() {
		defer wgInitialDetail.Done()
		gmuMap, ok := jsonDataMap["gmu"].(map[string]interface{})
		if !ok {
			log.Fatal("gmuMap is not a map")
		}
		UpdateMight(gmuMap)
	}()

	go func() {
		defer wgInitialDetail.Done()
		ufaMap, ok := jsonDataMap["ufa"].(map[string]interface{})
		if !ok {
			log.Fatal("ufaMap is not a map")
		}
		UpdateGlory(ufaMap)
	}()

	go func() {
		defer wgInitialDetail.Done()
		ufpMap, ok := jsonDataMap["ufp"].(map[string]interface{})
		if !ok {
			log.Fatal("ufpMap is not a map")
		}
		UpdateGallantry(ufpMap)
	}()

	go func() {
		defer wgInitialDetail.Done()
		gclMap, ok := jsonDataMap["gcl"].(map[string]interface{})
		if !ok {
			log.Fatal("gclMap is not a map")
		}
		dclMap, ok := jsonDataMap["dcl"].(map[string]interface{})
		if !ok {
			log.Fatal("dclMap is not a map")
		}
		CastleDetailParser(gclMap, dclMap)
		gliMap, ok := jsonDataMap["gli"].(map[string]interface{})
		if !ok {
			log.Fatal("gliMap is not a map")
		}
		UpdateEquipmentList(gliMap)
	}()

	go func() {
		defer wgInitialDetail.Done()
		gemStorageMap, ok := jsonDataMap["ggm"].(map[string]interface{})
		if !ok {
			log.Fatal("gemStorageMap is not a map")
		}
		UpdateGemStorage(gemStorageMap)
	}()

	wgInitialDetail.Wait()

}

// contains checks if a string is present in a slice of strings.
func contains(s []string, str string) bool {
	for _, v := range s {
		if v == str {
			return true
		}
	}
	return false
}

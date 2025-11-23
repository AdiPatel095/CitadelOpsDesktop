package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
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
	gcuMap, ok := jsonDataMap["gcu"].(map[string]interface{})
	if !ok {
		log.Fatal("gcuMap is not a map")
	}
	sceArray, ok := jsonDataMap["sce"].([]interface{})
	if !ok {
		log.Fatal("sceArray is not a slice")
	}
	gclMap, ok := jsonDataMap["gcl"].(map[string]interface{})
	if !ok {
		log.Fatal("gclMap is not a map")
	}
	dclMap, ok := jsonDataMap["dcl"].(map[string]interface{})
	if !ok {
		log.Fatal("dclMap is not a map")
	}

	UpdateCoins(gcuMap)
	UpdateSCE(sceArray)
	CastleDetailParser(gclMap, dclMap)
	playerResources := Models.GetPlayerCastleInfo()

	// Marshal the struct into a pretty-printed JSON string
	prettyJSON, err := json.MarshalIndent(playerResources, "", "  ")
	if err != nil {
		log.Printf("Error marshalling player resources to JSON: %v", err)
		return
	}

	log.Println("--- Player Global Resources ---")
	log.Println(string(prettyJSON))
	log.Println("-----------------------------")
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

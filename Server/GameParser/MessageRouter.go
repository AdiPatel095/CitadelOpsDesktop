package GameParser

import (
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"log"
)

func MessageRouter(messageParts []string) {
	if len(messageParts) <= 2 {
		return
	}
	indexedList := []string{"gie", "gbl", "dcl", "gcu", "gmu", "gpa", "grc", "sce", "gbd", "sei", "dcl"}
	messageType := messageParts[2]
	if contains(indexedList, messageType) {
		//log.Printf("Received message type: %s which has already been indexed", messageType)
	} else if !contains(indexedList, messageType) {
		//log.Printf("Received unhandled message type: %s, full message: %v", messageType, messageParts)
	}
	if messageType == "gbd" {
		InitiateDetails(messageParts[5])
	}

}

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

	UpdateCoins(gcuMap)
	UpdateSCE(sceArray)
	playerResources := Models.GetPlayerGlobalResources()

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

func UpdateCoins(gcuMap map[string]interface{}) {
	coins, ok := gcuMap["C1"].(float64)
	if ok {
		Models.GetPlayerGlobalResources().Coins = coins
	}
	rubies, ok := gcuMap["C2"].(float64)
	if ok {
		Models.GetPlayerGlobalResources().Rubies = rubies
	}
}

func UpdateSCE(sceArray []interface{}) {
	for _, item := range sceArray {
		valueArray, ok := item.([]interface{})
		if !ok || len(valueArray) != 2 {
			log.Printf("Skipping invalid item in sceArray: %v", item)
			continue
		}

		label, labelOk := valueArray[0].(string)
		value, valueOk := valueArray[1].(float64)

		if !labelOk || !valueOk {
			log.Printf("Skipping item with invalid types in sceArray: %v", valueArray)
			continue
		}
		updateResourceByLabel(label, value)
	}
}

func updateResourceByLabel(label string, value float64) {
	playerResources := Models.GetPlayerGlobalResources()
	switch label {
	case "STP":
		playerResources.Sceat = value
	case "IDCT":
		playerResources.Ducat = value
	case "LM":
		playerResources.ConstToken = value
	case "LT":
		playerResources.UpgrToken = value
	case "SLWT":
		playerResources.AfflTix = value
	case "PL":
		playerResources.Plaster = value
	case "DST":
		playerResources.DrgScale = value
	case "DSS":
		playerResources.DrgSpl = value
	case "RF":
		playerResources.RelicShard = value
	case "MS1":
		playerResources.Min1 = value
	case "MS2":
		playerResources.Min5 = value
	case "MS3":
		playerResources.Min10 = value
	case "MS4":
		playerResources.Min30 = value
	case "MS5":
		playerResources.Hr1 = value
	case "MS6":
		playerResources.Hr5 = value
	case "MS7":
		playerResources.Hr24 = value
	default:
		log.Printf("Unhandled resource label: %s", label)
	}
}

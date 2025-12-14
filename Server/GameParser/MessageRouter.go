package GameParser

import (
	"encoding/json"
	"log"
)

func MessageRouter(messageParts []string) {
	indexedList := []string{"gie", "gbl", "dcl", "gcu", "gmu", "gpa", "grc", "sce", "gbd", "sei"}
	messageType := messageParts[2]
	if contains(indexedList, messageType) {
		//log.Printf("Received message type: %s which has already been indexed", messageType)
	} else if !contains(indexedList, messageType) {
		//log.Printf("Received unhandled message type: %s, full message: %v", messageType, messageParts)
	}
	if messageType == "gbd" {
		InitiateDetails(messageParts[5])
	}
	if messageType == "gei" {
		UpdateEquipmentStorage(messageParts[5])
	}
	if messageType == "ggm" {
		UpdateGemStorage(messageParts[5])
	}
	// Log equipment equip/unequip responses
	if messageType == "eeq" {
		log.Printf("GAME RESPONSE (EQUIPMENT EQUIP/UNEQUIP): %v", messageParts)
	}
	// Log gem equip/unequip responses
	if messageType == "ege" {
		log.Printf("GAME RESPONSE (GEM EQUIP/UNEQUIP): %v", messageParts)
	}

	if messageType == "gli" {
		var gliMap map[string]interface{}
		_ = json.Unmarshal([]byte(messageParts[5]), &gliMap)
		UpdateEquipmentList(gliMap)
	}
}

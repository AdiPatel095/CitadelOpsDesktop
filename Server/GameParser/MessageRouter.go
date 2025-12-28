package GameParser

import (
	"CitadelDesktop/Server/ResponseRegistry"
	"encoding/json"
)

func MessageRouter(messageParts []string) {
	messageType := messageParts[2]

	// Check if any waiters are registered for this message type
	ResponseRegistry.Global.CheckWaiters(messageType, messageParts)

	indexedList := []string{"gie", "gbl", "dcl", "gcu", "gmu", "gpa", "grc", "sce", "gbd", "sei"}
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

	if messageType == "gli" {
		var gliMap map[string]interface{}
		_ = json.Unmarshal([]byte(messageParts[5]), &gliMap)
		UpdateEquipmentList(gliMap)
	}
}

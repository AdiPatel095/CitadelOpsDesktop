package GameParser

import "log"

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
		log.Printf("Received gei message")
		UpdateEquipmentStorage(messageParts[5])
	}
}

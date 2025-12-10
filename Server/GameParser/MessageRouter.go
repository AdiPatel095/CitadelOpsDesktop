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
	if messageType == "ggm" {
		log.Printf("Received ggm message")
		UpdateGemStorage(messageParts[5])
	}
	if messageType == "sge" {
		// Check for success code if present, user said %xt%sge%1%0%
		// 1 is request ID? 0 might be success?
	}
}

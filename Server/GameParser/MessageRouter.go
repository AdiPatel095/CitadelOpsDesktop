package GameParser

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
	if messageType == "sge" {
		// Check for success code if present, user said %xt%sge%1%0%
		// 1 is request ID? 0 might be success?
	}
}

package GameParser

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

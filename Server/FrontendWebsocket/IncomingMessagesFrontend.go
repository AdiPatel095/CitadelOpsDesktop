package FrontendWebsocket

import (
	"encoding/json"
	"log"
)

func ParseFrontendMessage(message []byte) {
	var data map[string]interface{}
	err := json.Unmarshal(message, &data)
	if err != nil {
		log.Fatalf("Not a json message:%v", string(message))
	}
	messageType := data["type"].(string)
	switch messageType {
	case "getCastUpdate":
		{
			castleLocation := data["castleLocation"].(string)
			SendCastStat(castleLocation)
		}
	case "getCommUpdate":
		{
			// JSON numbers are float64 when unmarshaled into interface{}
			if commanderIndexFloat, ok := data["commanderIndex"].(float64); ok {
				commanderIndexInt := int(commanderIndexFloat)
				SendCommStat(commanderIndexInt)
			}
		}
	case "getGlobalResources":
		SendGlobalResourceUpdate()
	case "getCastleResourceUpdate":
		castleLocation := data["castleLocation"].(string)
		SendCastleResource(castleLocation)
	}
}

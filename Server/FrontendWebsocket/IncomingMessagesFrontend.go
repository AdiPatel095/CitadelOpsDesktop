package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
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
	case "sellNonRelicEquipment":
		log.Println("Received request to sell non-relic equipment")
		soldCount := SellNonRelicEquipment()
		log.Printf("SoldCount: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic equipment items", soldCount))
	case "sellNonRelicGems":
		log.Println("Received request to sell non-relic gems")
		soldCount := SellNonRelicGems()
		log.Printf("SoldCount: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic gems", soldCount))
	case "startGame":
		log.Println("Received request to start game")
		GameWebsocket.StartGame()
	case "stopGame":
		log.Println("Received request to stop game")
		GameWebsocket.StopGame()
	case "refreshEquipment":
		// Send equipment data if registered
		// We can reuse SendInitialData or parts of it
		if registrationState.Registered {
			// Trigger updates for equipment
			// We can call the individual send functions
			// Or we can just call SendInitialData with a dummy client if we had access to the client
			// But since we want to broadcast to the requesting client...
			// ParseFrontendMessage doesn't have reference to the client currently!
			// We might need to change ParseFrontendMessage signature or use a broadcast for now.
			// The request is likely from the single Connected desktop client, so broadcast is fine.

			for i, comm := range Models.CommStatArray {
				SendFrontendMessage("commStatUpdate", comm, strconv.Itoa(i))
			}
			SendCastStat("mainCastle")
			SendCastStat("outpost1")
			SendCastStat("outpost2")
			SendCastStat("outpost3")
			SendCastStat("iceCastle")
			SendCastStat("desertCastle")
			SendCastStat("dungeonCastle")
			SendCastStat("stormCastle")

			SendGlobalResourceUpdate()

			SendCastleResource("mainCastle")
			SendCastleResource("outpost1")
			SendCastleResource("outpost2")
			SendCastleResource("outpost3")
			SendCastleResource("iceCastle")
			SendCastleResource("desertCastle")
			SendCastleResource("dungeonCastle")
			SendCastleResource("stormCastle")
		}
	}
}

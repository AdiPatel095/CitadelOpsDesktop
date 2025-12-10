package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/Models"
	"fmt"
	"log"
	"strconv"
	"time"
)

// Registration state - set by main.go via SetRegistrationState
var registrationState struct {
	Registered bool
	HardwareID string
	Credits    int
}

// SetRegistrationState is called from main.go to update the registration state
func SetRegistrationState(registered bool, hardwareID string, credits int) {
	registrationState.Registered = registered
	registrationState.HardwareID = hardwareID
	registrationState.Credits = credits
}

// SendRegistrationStatusMessage sends registration status to all connected clients
func SendRegistrationStatusMessage(registered bool, hardwareID string, credits int) {
	SetRegistrationState(registered, hardwareID, credits)
	SendFrontendMessage("registrationStatus", map[string]interface{}{
		"registered": registered,
		"hardwareID": hardwareID,
		"credits":    credits,
	}, "")
}

// SendCreditsUpdateMessage sends credits update to all connected clients
func SendCreditsUpdateMessage(credits int) {
	log.Printf("SendCreditsUpdateMessage called with: %d", credits)
	registrationState.Credits = credits
	SendFrontendMessage("creditsUpdate", map[string]interface{}{
		"credits": credits,
	}, "")
}

// SendInsufficientCreditsMessage sends a notification that credits are exhausted
func SendInsufficientCreditsMessage() {
	SendFrontendMessage("insufficientCredits", map[string]interface{}{
		"message": "Insufficient credits to perform action",
	}, "")
}

// SendGameLoginStatusMessage sends the current game login status
func SendGameLoginStatusMessage(loggedIn bool, cooldown int) {
	SendFrontendMessage("gameLoginStatus", map[string]interface{}{
		"loggedIn": loggedIn,
		"cooldown": cooldown,
	}, "")
}

func SendInitialData(client *Client) {
	// Send registration status first
	client.SendToClient("registrationStatus", map[string]interface{}{
		"registered": registrationState.Registered,
		"hardwareID": registrationState.HardwareID,
		"credits":    registrationState.Credits,
	}, "")

	// Only send game data if registered
	if !registrationState.Registered {
		return
	}

	// Send all commanders
	for i, comm := range Models.CommStatArray {
		client.SendToClient("commStatUpdate", comm, strconv.Itoa(i))
	}

	// Send all castle stats
	client.SendToClient("castStatUpdate", Models.CastStatArray.MainCastleCast, "mainCastle")
	client.SendToClient("castStatUpdate", Models.CastStatArray.Outpost1Cast, "outpost1")
	client.SendToClient("castStatUpdate", Models.CastStatArray.Outpost2Cast, "outpost2")
	client.SendToClient("castStatUpdate", Models.CastStatArray.Outpost3Cast, "outpost3")
	client.SendToClient("castStatUpdate", Models.CastStatArray.IceCastleCast, "iceCastle")
	client.SendToClient("castStatUpdate", Models.CastStatArray.DesertCastleCast, "desertCastle")
	client.SendToClient("castStatUpdate", Models.CastStatArray.DungeonCastleCast, "dungeonCastle")
	client.SendToClient("castStatUpdate", Models.CastStatArray.StormCastleCast, "stormCastle")

	// Send global resources
	client.SendToClient("globalResourceUpdate", Models.GetPlayerGlobalResources(), "")

	// Send all castle resources
	client.SendToClient("castleResourceUpdate", Models.MainCastleResources, "mainCastle")
	client.SendToClient("castleResourceUpdate", Models.Outpost1Resources, "outpost1")
	client.SendToClient("castleResourceUpdate", Models.Outpost2Resources, "outpost2")
	client.SendToClient("castleResourceUpdate", Models.Outpost3Resources, "outpost3")
	client.SendToClient("castleResourceUpdate", Models.IceCastleResources, "iceCastle")
	client.SendToClient("castleResourceUpdate", Models.DesertCastleResources, "desertCastle")
	client.SendToClient("castleResourceUpdate", Models.DungeonCastleResources, "dungeonCastle")
	client.SendToClient("castleResourceUpdate", Models.StormCastleResources, "stormCastle")
}

func SendCastStat(castleLocation string) {
	switch castleLocation {
	case "mainCastle":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.MainCastleCast, "mainCastle")
	case "outpost1":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.Outpost1Cast, "outpost1")
	case "outpost2":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.Outpost2Cast, "outpost2")
	case "outpost3":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.Outpost3Cast, "outpost3")
	case "iceCastle":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.IceCastleCast, "iceCastle")
	case "desertCastle":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.DesertCastleCast, "desertCastle")
	case "dungeonCastle":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.DungeonCastleCast, "dungeonCastle")
	case "stormCastle":
		SendFrontendMessage("castStatUpdate", Models.CastStatArray.StormCastleCast, "stormCastle")
	}
}

func SendCommStat(commanderIndex int) {
	if commanderIndex >= 0 && commanderIndex < len(Models.CommStatArray) {
		SendFrontendMessage("commStatUpdate", Models.CommStatArray[commanderIndex], strconv.Itoa(commanderIndex))
	}

}

func SendGlobalResourceUpdate() {
	SendFrontendMessage("globalResourceUpdate", Models.GetPlayerGlobalResources(), "")
}

func SendCastleResource(castleLocation string) {
	switch castleLocation {
	case "mainCastle":
		SendFrontendMessage("castleResourceUpdate", Models.MainCastleResources, "mainCastle")
	case "outpost1":
		SendFrontendMessage("castleResourceUpdate", Models.Outpost1Resources, "outpost1")
	case "outpost2":
		SendFrontendMessage("castleResourceUpdate", Models.Outpost2Resources, "outpost2")
	case "outpost3":
		SendFrontendMessage("castleResourceUpdate", Models.Outpost3Resources, "outpost3")
	case "iceCastle":
		SendFrontendMessage("castleResourceUpdate", Models.IceCastleResources, "iceCastle")
	case "desertCastle":
		SendFrontendMessage("castleResourceUpdate", Models.DesertCastleResources, "desertCastle")
	case "dungeonCastle":
		SendFrontendMessage("castleResourceUpdate", Models.DungeonCastleResources, "dungeonCastle")
	case "stormCastle":
		SendFrontendMessage("castleResourceUpdate", Models.StormCastleResources, "stormCastle")
	}

}

func SellNonRelicEquipment() int {
	counter := 0

	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)
	log.Printf("Storage equipment amount : %v ", len(Models.EquipmentStorage))
	for _, equipment := range Models.EquipmentStorage {
		if equipment.EquipRarity != 5 && equipment.EquipRarity != 15 {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%seq%%1%%{"EID":%.0f,"LID":-1,"EX":0,"LFID":-1}%%`, equipment.ID)
			GameWebsocket.OutgoingMessages <- []byte(payload)
			counter++
		}
	}
	return counter
}

func SellNonRelicGems() int {
	counter := 0

	GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)
	log.Printf("Storage gem amount : %v ", len(Models.NonRelicGemIDs))
	for id, count := range Models.NonRelicGemIDs {
		for i := 0; i < int(count); i++ {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sge%%1%%{"GID":%03.0f,"RGEM":0,"LFID":-1}%%`, id)
			GameWebsocket.OutgoingMessages <- []byte(payload)
			counter++
		}
	}
	log.Printf("Storage Gem amount : %v", len(Models.NonRelicGemIDs))
	return counter
}

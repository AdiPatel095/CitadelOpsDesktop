package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameFunctions"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
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

// SendMemoryStatsMessage sends memory usage stats to the frontend
func SendMemoryStatsMessage(goMemMB int, chromeMemMB int) {
	SendFrontendMessage("memoryStats", map[string]interface{}{
		"goMem":     goMemMB,
		"chromeMem": chromeMemMB,
	}, "")
}

// SendAutoBirdStatus sends the current auto bird enabled state and next wake up time to all clients
func SendAutoBirdStatus(enabled bool, nextWakeUp int64) {
	SendFrontendMessage("autoBirdStatus", map[string]interface{}{
		"enabled":    enabled,
		"nextWakeUp": nextWakeUp,
	}, "")
}

// SendRecruitTroopsStatus sends the current recruit troops enabled state to all clients
func SendRecruitTroopsStatus(enabled bool) {
	SendFrontendMessage("recruitTroopsStatus", map[string]interface{}{
		"enabled": enabled,
	}, "")
}

func SendInitialData(client *Client) {
	// Send registration status first
	client.SendToClient("registrationStatus", map[string]interface{}{
		"registered": registrationState.Registered,
		"hardwareID": registrationState.HardwareID,
		"credits":    registrationState.Credits,
	}, "")

	// Send current game login status so frontend knows if game is connected after page refresh
	client.SendToClient("gameLoginStatus", map[string]interface{}{
		"loggedIn": ResponseRegistry.LoginStatus,
		"cooldown": ResponseRegistry.LoginCooldown,
	}, "")

	// Only send game data if registered
	if !registrationState.Registered {
		return
	}

	// Send autoBird status (including sleep timer for persistence across reloads)
	client.SendToClient("autoBirdStatus", map[string]interface{}{
		"enabled":    GameFunctions.IsAutoBirdRunning(),
		"nextWakeUp": GameFunctions.GetAutoBirdNextWakeUp(),
	}, "")

	// Send recruitTroops status
	client.SendToClient("recruitTroopsStatus", map[string]interface{}{
		"enabled": GameFunctions.IsRecruitTroopsRunning(),
	}, "")

	// Send all commanders
	for i, comm := range Models.CommStatArray {
		client.SendToClient("commStatUpdate", comm, strconv.Itoa(i))
	}

	// Send all castle stats with index-based identification (0-8)
	for i := 0; i < 9; i++ {
		castStat := GameFunctions.GetCastellanStat(i)
		client.SendToClient("castStatUpdate", castStat, strconv.Itoa(i))
	}

	// Send global resources
	gs := Models.GetGameState()
	client.SendToClient("globalResourceUpdate", gs.GlobalResources, "")

	// Send all castle resources
	client.SendToClient("castleResourceUpdate", gs.MainCastle, "mainCastle")
	client.SendToClient("castleResourceUpdate", gs.Outpost1, "outpost1")
	client.SendToClient("castleResourceUpdate", gs.Outpost2, "outpost2")
	client.SendToClient("castleResourceUpdate", gs.Outpost3, "outpost3")
	client.SendToClient("castleResourceUpdate", gs.IceCastle, "iceCastle")
	client.SendToClient("castleResourceUpdate", gs.DesertCastle, "desertCastle")
	client.SendToClient("castleResourceUpdate", gs.DungeonCastle, "dungeonCastle")
	client.SendToClient("castleResourceUpdate", gs.StormCastle, "stormCastle")
	client.SendToClient("castleResourceUpdate", gs.BeriWorldCastle, "beriWorldCastle")

}

// SendCastStat sends a single castle's stats by index (0-7)
func SendCastStat(castleIndex int) {
	if castleIndex >= 0 && castleIndex < 9 {
		castStat := GameFunctions.GetCastellanStat(castleIndex)
		SendFrontendMessage("castStatUpdate", castStat, strconv.Itoa(castleIndex))
	}
}

func SendCommStat(commanderIndex int) {
	if commanderIndex >= 0 && commanderIndex < len(Models.CommStatArray) {
		SendFrontendMessage("commStatUpdate", Models.CommStatArray[commanderIndex], strconv.Itoa(commanderIndex))
	}

}

func SendGlobalResourceUpdate() {
	SendFrontendMessage("globalResourceUpdate", Models.GetGameState().GlobalResources, "")
}

func SendCastleResource(castleLocation string) {
	gs := Models.GetGameState()
	switch castleLocation {
	case "mainCastle":
		SendFrontendMessage("castleResourceUpdate", gs.MainCastle, "mainCastle")
	case "outpost1":
		SendFrontendMessage("castleResourceUpdate", gs.Outpost1, "outpost1")
	case "outpost2":
		SendFrontendMessage("castleResourceUpdate", gs.Outpost2, "outpost2")
	case "outpost3":
		SendFrontendMessage("castleResourceUpdate", gs.Outpost3, "outpost3")
	case "iceCastle":
		SendFrontendMessage("castleResourceUpdate", gs.IceCastle, "iceCastle")
	case "desertCastle":
		SendFrontendMessage("castleResourceUpdate", gs.DesertCastle, "desertCastle")
	case "dungeonCastle":
		SendFrontendMessage("castleResourceUpdate", gs.DungeonCastle, "dungeonCastle")
	case "stormCastle":
		SendFrontendMessage("castleResourceUpdate", gs.StormCastle, "stormCastle")
	case "beriWorldCastle":
		SendFrontendMessage("castleResourceUpdate", gs.BeriWorldCastle, "beriWorldCastle")
	}

}

func SellNonRelicEquipment(sellRift bool, sellLookItems bool) int {
	counter := 0
	gs := Models.GetGameState()

	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.EquipmentStorage {
		// Filter Look Items (Slot 5) if sellLookItems is false
		if !sellLookItems && equipment.EquipSlotNumber == 5 {
			continue
		}

		// Check for Rift Gear (StatID 158-165 on first stat)
		isRift := false
		if len(equipment.EquipStats) > 0 {
			statID := equipment.EquipStats[0].ID
			if statID >= 158 && statID <= 165 {
				isRift = true
			}
		}

		// If it is Rift Gear and we are NOT selling Rift Gear, skip it (save it)
		if isRift && !sellRift {
			continue
		}

		if equipment.EquipRarity != 5 && equipment.EquipRarity != 15 {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%seq%%1%%{"EID":%.0f,"LID":-1,"EX":0,"LFID":-1}%%`, equipment.ID)
			ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
			counter++
		}
	}
	return counter
}

func SellNonRelicGems(sellRiftGems bool) int {
	counter := 0
	gs := Models.GetGameState()

	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)
	for id, count := range gs.NonRelicGemIDs {
		// Rift Gems (IDs 450-475)
		isRift := id >= 450 && id <= 475

		// If it is a Rift Gem and we are NOT selling Rift Gems, skip
		if isRift && !sellRiftGems {
			continue
		}

		for i := 0; i < int(count); i++ {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sge%%1%%{"GID":%03.0f,"RGEM":0,"LFID":-1}%%`, id)
			ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
			counter++
		}
	}
	log.Printf("Storage Gem amount : %v", len(gs.NonRelicGemIDs))
	return counter
}

func SellRelic1Equipment() int {
	counter := 0
	gs := Models.GetGameState()
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.EquipmentStorage {
		// Relic 1.0 is Rarity 5 but NOT 4 stats (which is Relic 2.0)
		if equipment.EquipRarity == 5 && len(equipment.EquipStats) < 4 {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%seq%%1%%{"EID":%.0f,"LID":-1,"EX":0,"LFID":-1}%%`, equipment.ID)
			ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
			counter++
		}
	}
	return counter
}

func SellRelic2Equipment(keepStars int) int {
	counter := 0
	gs := Models.GetGameState()
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gei%1%{}%`)
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.EquipmentStorage {
		// Relic 2.0 Filters:
		// 1. Standard Equipment: Rarity 5, 4 Stats, Slot != 6 (Hero)
		// 2. Hero Equipment: Rarity 15, 6 Stats, Slot == 6
		isStandardRelic := (equipment.EquipRarity == 5 && len(equipment.EquipStats) == 4 && equipment.EquipSlotNumber != 6)
		isHeroRelic := (equipment.EquipRarity == 15 && len(equipment.EquipStats) == 6 && equipment.EquipSlotNumber == 6)

		if isStandardRelic || isHeroRelic {
			totalStars := 0
			for _, stat := range equipment.EquipStats {
				totalStars += GetStarFromPercent(stat.Percent)
			}

			// Sell if below the keep threshold
			if totalStars < keepStars {
				payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%seq%%1%%{"EID":%.0f,"LID":-1,"EX":0,"LFID":-1}%%`, equipment.ID)
				ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
				counter++
			}
		}
	}
	return counter
}

func SellRelic1Gems() int {
	counter := 0
	gs := Models.GetGameState()
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	for _, gem := range gs.GemsStorage {
		if len(gem.GemStats) == 3 {
			payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sge%%1%%{"GID":%.0f,"RGEM":1,"LFID":-1}%%`, gem.ID)
			ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
			counter++
		}
	}
	return counter
}

func SellRelic2Gems(keepStars int) int {
	counter := 0
	gs := Models.GetGameState()
	ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%ggm%1%{}%`)
	time.Sleep(2 * time.Second)

	for _, gem := range gs.GemsStorage {
		// Filter for Relic 2.0 Gems (Type 131 and 132) AND 4 Stats
		if (gem.GemType == 131 || gem.GemType == 132) && len(gem.GemStats) == 4 {
			totalStars := 0
			for _, stat := range gem.GemStats {
				totalStars += GetStarFromPercent(stat.Percent)
			}

			// Sell if below the keep threshold
			if totalStars < keepStars {
				payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sge%%1%%{"GID":%.0f,"RGEM":1,"LFID":-1}%%`, gem.ID)
				ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(payload), Cost: 1}
				counter++
			}
		}
	}
	return counter
}

// GetStarFromPercent converts a stat percentage to a star rating (1-7).
// Thresholds per user:
// 100: 7
// 90-99: 6
// 80-89: 5
// 70-79: 4
// 60-69: 3
// 40-59: 2
// 0-39: 1
func GetStarFromPercent(percent float64) int {
	switch {
	case percent >= 100.0:
		return 7
	case percent >= 90.0:
		return 6
	case percent >= 80.0:
		return 5
	case percent >= 70.0:
		return 4
	case percent >= 60.0:
		return 3
	case percent >= 40.0:
		return 2
	default:
		return 1
	}
}

// SendAlertMessage sends an alert message to the frontend
func SendAlertMessage(category string, message string) {
	SendFrontendMessage("alert", map[string]interface{}{
		"category": category,
		"message":  message,
	}, "")
}

// SendVersionUpdateMessage sends a new version notification to the frontend
func SendVersionUpdateMessage(newVersion string, downloadUrl string) {
	SendFrontendMessage("versionUpdate", map[string]interface{}{
		"newVersion":  newVersion,
		"downloadUrl": downloadUrl,
	}, "")
}

// SendUpdateProgressMessage sends update download progress to the frontend
func SendUpdateProgressMessage(stage string, percent int) {
	SendFrontendMessage("updateProgress", map[string]interface{}{
		"stage":   stage,
		"percent": percent,
	}, "")
}

// SendUpdateCompleteMessage notifies the frontend that the update is complete
func SendUpdateCompleteMessage() {
	SendFrontendMessage("updateComplete", map[string]interface{}{
		"message": "Update installed successfully. Restarting...",
	}, "")
}

// SendUpdateErrorMessage notifies the frontend of an update error
func SendUpdateErrorMessage(errMsg string) {
	SendFrontendMessage("updateError", map[string]interface{}{
		"error": errMsg,
	}, "")
}

// SendRequestCredentialsMessage sends a request for credentials to the frontend
func SendRequestCredentialsMessage() {
	SendFrontendMessage("requestCredentials", nil, "")
}

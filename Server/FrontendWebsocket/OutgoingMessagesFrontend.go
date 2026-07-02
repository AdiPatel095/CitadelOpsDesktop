package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameCommands"
	equipmentview "CitadelDesktop/Server/GameFeatures/EquipmentView"
	featureview "CitadelDesktop/Server/GameFeatures/FeatureView"
	settingsview "CitadelDesktop/Server/GameFeatures/SettingsView"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	equip "CitadelDesktop/Server/Models/Equipment"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/ResponseRegistry"
	"log"
	"strconv"
	"time"
)

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

// SendAutoBirdStatus pushes auto bird enabled state and next wake time (unix ms, 0 if none).
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

// SendAutoToolStatus sends the current Auto Tool enabled state to all clients.
func SendAutoToolStatus(enabled bool) {
	SendFrontendMessage("autoToolStatus", map[string]interface{}{
		"enabled": enabled,
	}, "")
}

// SendAutoHospitalStatus sends the current Auto Hospital enabled state to all clients.
func SendAutoHospitalStatus(enabled bool) {
	SendFrontendMessage("autoHospitalStatus", map[string]interface{}{
		"enabled": enabled,
	}, "")
}

// SendAutoTCIStatus sends whether AutoTCI (temporary construction items) automation is running.
// nextWakeUp is the next scheduled wake: either login prep (1m before a slot expires) or the ubc window.
func SendAutoTCIStatus(enabled bool) {
	nextWakeUp := int64(0)
	if enabled {
		nextWakeUp = featureview.GetAutoTCINextWakeUp()
	}
	SendFrontendMessage("autoTCIStatus", map[string]interface{}{
		"enabled":    enabled,
		"nextWakeUp": nextWakeUp,
	}, "")
}

// SendAutoBeriWorldStatus pushes Auto Beri World enabled state and next pass time (unix ms, 0 if none).
func SendAutoBeriWorldStatus(enabled bool, nextWakeUp int64) {
	SendFrontendMessage("autoBeriWorldStatus", map[string]interface{}{
		"enabled":    enabled,
		"nextWakeUp": nextWakeUp,
	}, "")
}

func SendInitialData(client *Client) {
	// Send current game login status so frontend knows if game is connected after page refresh
	client.SendToClient("gameLoginStatus", map[string]interface{}{
		"loggedIn": ResponseRegistry.LoginStatus,
		"cooldown": ResponseRegistry.LoginCooldown,
	}, "")

	client.SendToClient("autoBirdStatus", map[string]interface{}{
		"enabled":    featureview.IsAutoBirdRunning(),
		"nextWakeUp": featureview.GetAutoBirdNextWakeUp(),
	}, "")

	// Send recruitTroops status
	client.SendToClient("recruitTroopsStatus", map[string]interface{}{
		"enabled": settingsview.IsRecruitTroopsRunning(),
	}, "")
	state := Models.GetSettingsState()
	recruitTroopsConfig := state.RecruitTroopsList.Normalize()
	state.RecruitTroopsList = recruitTroopsConfig
	client.SendToClient("recruitTroopsSettings", recruitTroopsConfig, "")
	client.SendToClient("autoToolStatus", map[string]interface{}{
		"enabled": settingsview.IsAutoToolRunning(),
	}, "")
	autoToolConfig := state.AutoToolList.Normalize()
	state.AutoToolList = autoToolConfig
	client.SendToClient("autoToolSettings", autoToolConfig, "")
	client.SendToClient("schedulerSettings", state, "")

	client.SendToClient("autoHospitalStatus", map[string]interface{}{
		"enabled": settingsview.IsAutoHospitalRunning(),
	}, "")
	autoHospitalConfig := state.AutoHospital.Normalize()
	state.AutoHospital = autoHospitalConfig
	client.SendToClient("autoHospitalSettings", autoHospitalConfig, "")

	client.SendToClient("autoTCIStatus", map[string]interface{}{
		"enabled":    featureview.IsAutoTCIRunning(),
		"nextWakeUp": featureview.GetAutoTCINextWakeUp(),
	}, "")

	client.SendToClient("autoBeriWorldStatus", map[string]interface{}{
		"enabled":    featureview.IsAutoBeriWorldRunning(),
		"nextWakeUp": featureview.GetAutoBeriWorldNextWakeUp(),
	}, "")

	// Send all commanders
	for i, comm := range equip.CommStatArray {
		client.SendToClient("commStatUpdate", comm, strconv.Itoa(i))
	}

	// Send all castle stats with index-based identification (0-10)
	for i := 0; i < Models.NumPlayerCastleSlots; i++ {
		castStat := equipmentview.GetCastellanStat(i)
		client.SendToClient("castStatUpdate", castStat, strconv.Itoa(i))
	}

	// Send global resources
	gs := Models.GetGameState()
	client.SendToClient("globalResourceUpdate", gs.GlobalResources, "")
	client.SendToClient("allianceInfo", gs.Alliance, "")

	c := &gs.Castle
	client.SendToClient("castleResourceUpdate", c.MainCastle, "mainCastle")
	client.SendToClient("castleResourceUpdate", c.Outpost1, "outpost1")
	client.SendToClient("castleResourceUpdate", c.Outpost2, "outpost2")
	client.SendToClient("castleResourceUpdate", c.Outpost3, "outpost3")
	client.SendToClient("castleResourceUpdate", c.IceCastle, "iceCastle")
	client.SendToClient("castleResourceUpdate", c.DesertCastle, "desertCastle")
	client.SendToClient("castleResourceUpdate", c.DungeonCastle, "dungeonCastle")
	client.SendToClient("castleResourceUpdate", c.StormCastle, "stormCastle")
	client.SendToClient("castleResourceUpdate", c.Metropolis, "metropolisCastle")
	client.SendToClient("castleResourceUpdate", c.Capital, "capitalCastle")

	client.SendToClient("castleFocus", Models.CastleFocusMessagePayload(), "")

	SendRiftMapCoordsToClient(client)
	SendRiftCRALaunchToClient(client)
	SendRiftMaidenCommsSettingsToClient(client)
	SendMovementUpdateToClient(client)

	SendLastKnownGameStateSnapshot(client)
}

// BroadcastLastKnownGameStateSnapshot pushes the on-disk snapshot to all connected clients (e.g. after game disconnect).
func BroadcastLastKnownGameStateSnapshot() {
	m, err := Models.ReadGameStateSnapshotMap()
	if err != nil {
		return
	}
	SendFrontendMessage("lastKnownGameStateSnapshot", m, "")
}

// SendLastKnownGameStateSnapshot sends one client's snapshot from disk (supplements in-memory SendInitialData when reconnecting).
func SendLastKnownGameStateSnapshot(client *Client) {
	m, err := Models.ReadGameStateSnapshotMap()
	if err != nil {
		return
	}
	client.SendToClient("lastKnownGameStateSnapshot", m, "")
}

// SendCastStat sends a single castle's stats by index (0-10)
func SendCastStat(castleIndex int) {
	if castleIndex >= 0 && castleIndex < Models.NumPlayerCastleSlots {
		castStat := equipmentview.GetCastellanStat(castleIndex)
		SendFrontendMessage("castStatUpdate", castStat, strconv.Itoa(castleIndex))
	}
}

func SendCommStat(commanderIndex int) {
	if commanderIndex >= 0 && commanderIndex < len(equip.CommStatArray) {
		SendFrontendMessage("commStatUpdate", equip.CommStatArray[commanderIndex], strconv.Itoa(commanderIndex))
	}

}

func SendGlobalResourceUpdate() {
	SendFrontendMessage("globalResourceUpdate", Models.GetGameState().GlobalResources, "")
}

func SendRiftMapCoords() {
	SendFrontendMessage("riftMapCoords", Models.RiftMapCoordsPayload(), "")
}

func SendRiftMapCoordsToClient(client *Client) {
	client.SendToClient("riftMapCoords", Models.RiftMapCoordsPayload(), "")
}

func SendRiftCRALaunch() {
	ScheduleSendRiftCRALaunch()
}

func SendRiftCRALaunchToClient(client *Client) {
	client.SendToClient("riftCRALaunch", GameParser.RiftCRALaunchWirePayload(), "")
}

func riftMaidenCommsSettingsPayload() map[string]interface{} {
	cfg := stsettings.ReadRiftMaidenCommsSettings()
	return map[string]interface{}{
		"unitWodID": cfg.UnitWodID,
	}
}

func SendRiftMaidenCommsSettingsToClient(client *Client) {
	client.SendToClient("riftMaidenCommsSettings", riftMaidenCommsSettingsPayload(), "")
}

func SendMovementUpdate() {
	SendFrontendMessage("movementUpdate", Models.MovementUpdatePayload(), "")
}

func SendMovementUpdateToClient(client *Client) {
	client.SendToClient("movementUpdate", Models.MovementUpdatePayload(), "")
}

func SendCastleResource(castleLocation string) {
	gs := Models.GetGameState()
	c := &gs.Castle
	switch castleLocation {
	case "mainCastle":
		SendFrontendMessage("castleResourceUpdate", c.MainCastle, "mainCastle")
	case "outpost1":
		SendFrontendMessage("castleResourceUpdate", c.Outpost1, "outpost1")
	case "outpost2":
		SendFrontendMessage("castleResourceUpdate", c.Outpost2, "outpost2")
	case "outpost3":
		SendFrontendMessage("castleResourceUpdate", c.Outpost3, "outpost3")
	case "iceCastle":
		SendFrontendMessage("castleResourceUpdate", c.IceCastle, "iceCastle")
	case "desertCastle":
		SendFrontendMessage("castleResourceUpdate", c.DesertCastle, "desertCastle")
	case "dungeonCastle":
		SendFrontendMessage("castleResourceUpdate", c.DungeonCastle, "dungeonCastle")
	case "stormCastle":
		SendFrontendMessage("castleResourceUpdate", c.StormCastle, "stormCastle")
	case "metropolisCastle":
		SendFrontendMessage("castleResourceUpdate", c.Metropolis, "metropolisCastle")
	case "capitalCastle":
		SendFrontendMessage("castleResourceUpdate", c.Capital, "capitalCastle")
	}

}

func SellNonRelicEquipment(sellLookItems bool, sellSpecialPost2026 bool) int {
	counter := 0
	gs := Models.GetGameState()

	GameCommands.SendGEI()
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.Equipment.EquipmentStorage {
		// Filter Look Items (Slot 5) if sellLookItems is false
		if !sellLookItems && equipment.EquipSlotNumber == 5 {
			continue
		}

		id := int(equipment.TemplateID)
		if id == 0 {
			id = int(equipment.PlaceHolder6)
		}

		// Selling Logic:
		// 1. Sell if TemplateID < 1366 (Old Pre-2026 Gear)
		// 2. Sell if TemplateID >= 1366 (New Post-2026 Gear) AND the user enabled that toggle
		shouldSell := id < 1366 || sellSpecialPost2026

		if shouldSell && equipment.EquipRarity != 5 && equipment.EquipRarity != 15 {
			GameCommands.SendSEQSellEquipment(equipment.ID)
			counter++
		}
	}
	return counter
}

func SellNonRelicGems(sellSpecialPost2026 bool) int {
	counter := 0
	gs := Models.GetGameState()

	GameCommands.SendGGM()
	time.Sleep(2 * time.Second)
	for id, count := range gs.Equipment.NonRelicGemIDs {
		gemID := id

		// Selling Logic:
		// 1. Sell if GemID < 450 (Old Pre-2026 Gems)
		// 2. Sell if GemID >= 450 (New Post-2026 Gems) AND the user enabled that toggle
		shouldSell := gemID < 450 || sellSpecialPost2026

		if shouldSell {
			for i := 0; i < int(count); i++ {
				GameCommands.SendSGENonRelicGem(float64(id))
				counter++
			}
		}
	}
	log.Printf("Storage Gem amount : %v", len(gs.Equipment.NonRelicGemIDs))
	return counter
}

func SellRelic1Equipment() int {
	counter := 0
	gs := Models.GetGameState()
	GameCommands.SendGEI()
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.Equipment.EquipmentStorage {
		// Relic 1.0 is Rarity 5 but NOT 4 stats (which is Relic 2.0)
		if equipment.EquipRarity == 5 && len(equipment.EquipStats) < 4 {
			GameCommands.SendSEQSellEquipment(equipment.ID)
			counter++
		}
	}
	return counter
}

func SellRelic2Equipment(keepStars int) int {
	counter := 0
	gs := Models.GetGameState()
	GameCommands.SendGEI()
	time.Sleep(2 * time.Second)

	for _, equipment := range gs.Equipment.EquipmentStorage {
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
				GameCommands.SendSEQSellEquipment(equipment.ID)
				counter++
			}
		}
	}
	return counter
}

func SellRelic1Gems() int {
	counter := 0
	gs := Models.GetGameState()
	GameCommands.SendGGM()
	time.Sleep(2 * time.Second)

	for _, gem := range gs.Equipment.GemsStorage {
		if len(gem.GemStats) == 3 {
			GameCommands.SendSGERelicGem(gem.ID)
			counter++
		}
	}
	return counter
}

func SellRelic2Gems(keepStars int) int {
	counter := 0
	gs := Models.GetGameState()
	GameCommands.SendGGM()
	time.Sleep(2 * time.Second)

	for _, gem := range gs.Equipment.GemsStorage {
		// Filter for Relic 2.0 Gems (Type 131 and 132) AND 4 Stats
		if (gem.GemType == 131 || gem.GemType == 132) && len(gem.GemStats) == 4 {
			totalStars := 0
			for _, stat := range gem.GemStats {
				totalStars += GetStarFromPercent(stat.Percent)
			}

			// Sell if below the keep threshold
			if totalStars < keepStars {
				GameCommands.SendSGERelicGem(gem.ID)
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

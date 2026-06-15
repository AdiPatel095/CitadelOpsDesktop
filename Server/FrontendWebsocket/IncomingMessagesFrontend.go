package FrontendWebsocket

import (
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameCommands"
	castleview "CitadelDesktop/Server/GameFeatures/CastleView"
	equipmentview "CitadelDesktop/Server/GameFeatures/EquipmentView"
	featureview "CitadelDesktop/Server/GameFeatures/FeatureView"
	settingsview "CitadelDesktop/Server/GameFeatures/SettingsView"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	dec "CitadelDesktop/Server/Models/Decoration"
	equip "CitadelDesktop/Server/Models/Equipment"
	mapstate "CitadelDesktop/Server/Models/MapState"
	riftattack "CitadelDesktop/Server/Models/RiftAttack"
	sentbird "CitadelDesktop/Server/Models/SentBird"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/Paths"
	"CitadelDesktop/Server/ReconfigureLoadout"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Scheduler"
	"CitadelDesktop/Server/Version"
)

// triggerSelfUpdate is a wrapper that calls Version.PerformSelfUpdate
func triggerSelfUpdate(downloadUrl string) error {
	return Version.PerformSelfUpdate(downloadUrl)
}

func clampAutoTCILevelCeiling(v int) int {
	if v < 1 {
		return 1
	}
	if v > 4 {
		return 4
	}
	return v
}

func autoTCILevelTargetFromClientItem(item map[string]interface{}) stsettings.AutoTCILevelTarget {
	maxLevel := 1
	if v, ok := item["amount"].(float64); ok {
		maxLevel = int(v)
	}
	minLevel := 1
	if v, ok := item["minLevel"].(float64); ok {
		minLevel = int(v)
	}
	return stsettings.AutoTCILevelTarget{MinLevel: minLevel, MaxLevel: maxLevel}.Normalize()
}

func init() {
	// Wire up callback so GameParser can notify frontend when castle data changes
	GameParser.UpdateCastleResourceFunc = func(castleLocation string) {
		SendCastleResource(castleLocation)
	}
	ResponseRegistry.SendRecruitTroopsStatusFunc = SendRecruitTroopsStatus
	ResponseRegistry.SendAutoTCIStatusFunc = SendAutoTCIStatus
	ResponseRegistry.SendAutoBeriWorldStatusFunc = SendAutoBeriWorldStatus
	GameParser.NotifyCastleFocusChanged = func() {
		SendFrontendMessage("castleFocus", Models.CastleFocusMessagePayload(), "")
	}
	GameParser.NotifyGlobalResourcesChanged = SendGlobalResourceUpdate
	GameParser.NotifyRiftMapCoordsChanged = SendRiftMapCoords
	GameParser.NotifyRiftCRALaunchChanged = ScheduleSendRiftCRALaunch
	GameParser.NotifyMovementChanged = SendMovementUpdate
	ResponseRegistry.OutboundGameWireSendHook = GameParser.TryCaptureOutboundRiftCRA
	GameParser.NotifyBeriCastleDiscovered = featureview.RegisterBeriCastleDiscovery
	GameParser.NotifyMainCastleAIDForAutoBeri = featureview.RegisterMainCastleKutSource
}

func ParseFrontendMessage(message []byte) {
	var data map[string]interface{}
	if err := json.Unmarshal(message, &data); err != nil {
		log.Printf("[frontend-ws] invalid JSON: %v", err)
		return
	}
	messageType, ok := data["type"].(string)
	if !ok || messageType == "" {
		log.Printf("[frontend-ws] missing or invalid type")
		return
	}
	switch messageType {
	case "getCastUpdate":
		{
			if castleIndexFloat, ok := data["castleIndex"].(float64); ok {
				castleIndex := int(castleIndexFloat)
				SendCastStat(castleIndex)
			}
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
		castleLocation := ""
		if s, ok := data["castleLocation"].(string); ok && s != "" {
			castleLocation = s
		} else if v, ok := data["castleId"].(float64); ok && int(v) > 0 {
			castleLocation = GameParser.GetCastleLocationName(int(v))
		} else if payload, ok := data["payload"].(map[string]interface{}); ok {
			if v, ok := payload["castleId"].(float64); ok && int(v) > 0 {
				castleLocation = GameParser.GetCastleLocationName(int(v))
			}
		}
		if castleLocation == "" {
			log.Printf("[getCastleResourceUpdate] unknown castle (need castleLocation or castleId)")
			return
		}
		SendCastleResource(castleLocation)
	case "sellNonRelicEquipment":
		log.Println("Received request to sell non-relic equipment")

		// Parse payload
		var sellLookItems, sellSpecialPost2026 bool
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["sellLookItems"].(bool); ok {
				sellLookItems = val
			}
			if val, ok := payload["sellSpecialPost2026"].(bool); ok {
				sellSpecialPost2026 = val
			}
		}

		log.Printf("Flags - Sell Look Items: %v, Sell Special Post-2026: %v", sellLookItems, sellSpecialPost2026)

		go func() {
			soldCount := SellNonRelicEquipment(sellLookItems, sellSpecialPost2026)
			log.Printf("SoldCount: %v", soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic equipment items", soldCount))
		}()
	case "sellNonRelicGems":
		log.Println("Received request to sell non-relic gems")
		var sellSpecialPost2026 bool
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["sellSpecialPost2026"].(bool); ok {
				sellSpecialPost2026 = val
			}
		}
		go func() {
			soldCount := SellNonRelicGems(sellSpecialPost2026)
			log.Printf("SoldCount: %v", soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic gems", soldCount))
		}()
	case "sellRelic1Equipment":
		log.Println("Received request to sell Relic 1.0 equipment")
		go func() {
			soldCount := SellRelic1Equipment()
			log.Printf("SoldCount: %v", soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 1.0 items", soldCount))
		}()
	case "sellRelic2Equipment":
		log.Println("Received request to sell Relic 2.0 equipment")
		var keepStars int
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["keepStars"].(float64); ok {
				keepStars = int(val)
			}
		}
		go func() {
			soldCount := SellRelic2Equipment(keepStars)
			log.Printf("SoldCount Relic 2.0 (Keep %d+ Stars): %v", keepStars, soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 2.0 items", soldCount))
		}()

	case "sellRelic1Gems":
		log.Println("Received request to sell Relic 1.0 gems")
		go func() {
			soldCount := SellRelic1Gems()
			log.Printf("SoldCount Relic 1.0 Gems: %v", soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 1.0 gems", soldCount))
		}()

	case "sellRelic2Gems":
		log.Println("Received request to sell Relic 2.0 gems")
		var keepStars int
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["keepStars"].(float64); ok {
				keepStars = int(val)
			}
		}
		go func() {
			soldCount := SellRelic2Gems(keepStars)
			log.Printf("SoldCount Relic 2.0 Gems (Keep %d+ Stars): %v", keepStars, soldCount)
			SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 2.0 gems", soldCount))
		}()
	case "startGame":
		log.Println("Manual Start Bot pressed. Reloading game tab...")
		Models.GetSettingsState().BotEnabled = true
		Scheduler.GetScheduler().Start()
		ResponseRegistry.ReloadGameTab()
	case "stopGame":
		Models.GetSettingsState().BotEnabled = false
		Scheduler.GetScheduler().Stop()
		ResponseRegistry.DisconnectGameWebSocket()
	case "fetchAllianceInfo":
		featureview.FetchAllianceInfo()
	case "toggleAutoBird":
		wasRunning := featureview.IsAutoBirdRunning()
		if wasRunning {
			featureview.StopAutoBird()
			Models.GetSettingsState().AutoBirdEnabled = false
			SendAutoBirdStatus(false, 0)
			Logging.AppendAutoBirdLine("toggle", "stopped (UI)")
		} else {
			Models.GetSettingsState().AutoBirdEnabled = true
			if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {
				applyAutoBirdIgnoreSettingsFromMap(payloadRaw)
			}
			featureview.StartAutoBird()
			SendAutoBirdStatus(true, 0)
			Logging.AppendAutoBirdLine("toggle", "started (UI)")
		}
	case "clearAutoBirdSentBirds":
		log.Println("[frontend-ws] clear AutoBird sent-bird log")
		sentbird.Clear()
		Logging.AppendAutoBirdLine("sent_log_cleared", "persisted sent-bird list cleared")
		SendAlertMessage("green", "AutoBird sent-bird log cleared")
	case "toggleRecruitTroops":
		wasRunning := settingsview.IsRecruitTroopsRunning()
		log.Printf("[RecruitTroops] Toggle requested. Was running: %v", wasRunning)

		if wasRunning {
			log.Println("[RecruitTroops] Calling StopRecruitTroops...")
			settingsview.StopRecruitTroops()
			Models.GetSettingsState().RecruitTroopsEnabled = false
			SendRecruitTroopsStatus(false)
		} else {
			Models.GetSettingsState().RecruitTroopsEnabled = true
			if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {
				newSettings := make(map[int]map[int]int)
				if settingsRaw, ok := payloadRaw["settings"].(map[string]interface{}); ok {
					for castleIDStr, itemsRaw := range settingsRaw {
						castleID, _ := strconv.Atoi(castleIDStr)
						if castleID == 0 {
							continue
						}

						if items, ok := itemsRaw.([]interface{}); ok {
							castleMap := make(map[int]int)
							for _, itemRaw := range items {
								if item, ok := itemRaw.(map[string]interface{}); ok {
									unitID := int(item["id"].(float64))
									amount := int(item["amount"].(float64))
									if unitID > 0 && amount >= 0 {
										castleMap[unitID] = amount
									}
								}
							}
							newSettings[castleID] = castleMap
						}
					}
					Models.GetSettingsState().UpdateRecruitTroopsList(newSettings)
				}
				log.Println("[RecruitTroops] Calling StartRecruitTroops...")
				settingsview.StartRecruitTroops()
				SendRecruitTroopsStatus(true)
			}
		}
	case "toggleAutoTCI":
		wasRunning := featureview.IsAutoTCIRunning()
		if wasRunning {
			featureview.StopAutoTCI()
			Models.GetSettingsState().AutoTCIEnabled = false
			SendAutoTCIStatus(false)
			Logging.AppendAutoTCILine("toggle", "stopped (UI)")
		} else {
			Models.GetSettingsState().AutoTCIEnabled = true
			if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {
				newSettings := make(map[int]map[int]stsettings.AutoTCILevelTarget)
				if settingsRaw, ok := payloadRaw["settings"].(map[string]interface{}); ok {
					for castleIDStr, itemsRaw := range settingsRaw {
						castleID, _ := strconv.Atoi(castleIDStr)
						if castleID == 0 {
							continue
						}
						if items, ok := itemsRaw.([]interface{}); ok {
							castleMap := make(map[int]stsettings.AutoTCILevelTarget)
							for _, itemRaw := range items {
								if item, ok := itemRaw.(map[string]interface{}); ok {
									tciID := int(item["id"].(float64))
									if tciID > 0 {
										castleMap[tciID] = autoTCILevelTargetFromClientItem(item)
									}
								}
							}
							newSettings[castleID] = castleMap
						}
					}
					Models.GetSettingsState().UpdateAutoTCIList(newSettings)
				}
			}
			featureview.StartAutoTCI()
			SendAutoTCIStatus(true)
			Logging.AppendAutoTCILine("toggle", "started (UI)")
		}
	case "toggleAutoBeriWorld":
		wasRunning := featureview.IsAutoBeriWorldRunning()
		if wasRunning {
			featureview.StopAutoBeriWorld()
			Models.GetSettingsState().AutoBeriWorldEnabled = false
			SendAutoBeriWorldStatus(false, 0)
			Logging.AppendAutoBeriWorldLine("toggle", "stopped (UI)")
		} else {
			Models.GetSettingsState().AutoBeriWorldEnabled = true
			if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {
				applyAutoBeriWorldConfigFromMap(payloadRaw)
			}
			featureview.StartAutoBeriWorld()
			SendAutoBeriWorldStatus(true, 0)
			Logging.AppendAutoBeriWorldLine("toggle", "started (UI)")
		}
	case "refreshEquipment":
		// Trigger updates for equipment
		for i, comm := range equip.CommStatArray {
			SendFrontendMessage("commStatUpdate", comm, strconv.Itoa(i))
		}
		for i := 0; i < Models.NumPlayerCastleSlots; i++ {
			SendCastStat(i)
		}

		SendGlobalResourceUpdate()

		SendCastleResource("mainCastle")
		SendCastleResource("outpost1")
		SendCastleResource("outpost2")
		SendCastleResource("outpost3")
		SendCastleResource("iceCastle")
		SendCastleResource("desertCastle")
		SendCastleResource("dungeonCastle")
		SendCastleResource("stormCastle")
		SendCastleResource("metropolisCastle")
		SendCastleResource("capitalCastle")

	case "refreshSingleCommander":
		{
			// Refresh a single commander or castellan
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				log.Println("Invalid refreshSingleCommander request")
				return
			}

			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)

			if equipmentMode == "Commander" {
				idx := int(targetIndex)
				if idx >= 0 && idx < len(equip.CommStatArray) {
					log.Printf("Refreshing single commander at index %d", idx)
					SendFrontendMessage("commStatUpdate", equip.CommStatArray[idx], strconv.Itoa(idx))
				}
			} else if equipmentMode == "Castellan" {
				// TODO: implement castellan single refresh
				log.Println("Castellan single refresh not yet implemented")
			}
		}
	case "reconfigureLoadout":
		{
			GameCommands.SendGLI()
			time.Sleep(2 * time.Second)

			// Parse the payload into ReconfigurePayload struct
			var msg struct {
				Type    string                                `json:"type"`
				Payload ReconfigureLoadout.ReconfigurePayload `json:"payload"`
			}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error parsing reconfigure msg: %v", err)
				SendAlertMessage("red", "Invalid reconfiguration request")
				return
			}

			if msg.Payload.EquipmentMode == "Commander" {
				// Get current loadout from the target index
				targetIndex := msg.Payload.TargetIndex
				var currentLoadout Models.CommStatModel
				if targetIndex >= 0 && targetIndex < len(equip.CommStatArray) {
					currentLoadout = equip.CommStatArray[targetIndex]
				}

				// Calculate the optimized loadout
				newLoadout, errMsg := ReconfigureLoadout.ReconfigureCommander(msg.Payload)
				if errMsg != "" {
					SendFrontendMessage("reconfigureError", map[string]interface{}{"error": errMsg}, "")
					SendAlertMessage("red", errMsg)
					return
				}

				// Send comparison data to frontend
				comparisonData := map[string]interface{}{
					"currentLoadout": currentLoadout,
					"newLoadout":     newLoadout,
					"targetIndex":    targetIndex,
				}
				SendFrontendMessage("reconfigureComparison", comparisonData, "")

			} else if msg.Payload.EquipmentMode == "Castellan" {
				// Get current loadout from the target index
				targetIndex := msg.Payload.TargetIndex
				currentLoadout := equipmentview.GetCastellanStat(targetIndex)

				// Calculate the optimized loadout
				newLoadout, errMsg := ReconfigureLoadout.ReconfigureCastellan(msg.Payload)
				if errMsg != "" {
					SendFrontendMessage("reconfigureError", map[string]interface{}{"error": errMsg}, "")
					SendAlertMessage("red", errMsg)
					return
				}

				// Send comparison data to frontend
				// Send comparison data to frontend
				comparisonData := map[string]interface{}{
					"currentLoadout": currentLoadout,
					"newLoadout":     newLoadout,
					"targetIndex":    targetIndex,
				}

				SendFrontendMessage("reconfigureComparison", comparisonData, "")
			}
		}
	case "confirmReconfigure":
		{

			// Re-parse message to get the structured payload
			var msg struct {
				Type    string `json:"type"`
				Payload struct {
					TargetIndex    float64              `json:"targetIndex"`
					CurrentLoadout Models.CommStatModel `json:"currentLoadout"`
					NewLoadout     Models.CommStatModel `json:"newLoadout"`
					EquipmentMode  string               `json:"equipmentMode"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error parsing confirmReconfigure msg: %v", err)
				SendAlertMessage("red", "Invalid confirm reconfiguration request")
				return
			}

			targetIndex := int(msg.Payload.TargetIndex)
			current := msg.Payload.CurrentLoadout
			newLoadout := msg.Payload.NewLoadout
			equipmentMode := msg.Payload.EquipmentMode
			if equipmentMode == "" {
				equipmentMode = "Commander" // Default
			}

			SendAlertMessage("green", "Starting reconfiguration...")

			// --- Step 1: Unequip Target Commander (all 5 slots) ---
			log.Printf("[Reconfigure] Step 1: Unequipping target %s at index %d", equipmentMode, targetIndex)
			slotsToClear := []int{1, 2, 3, 4, 6}
			for _, slot := range slotsToClear {
				var equipID float64
				switch slot {
				case 1:
					equipID = current.Equip1
				case 2:
					equipID = current.Equip2
				case 3:
					equipID = current.Equip3
				case 4:
					equipID = current.Equip4
				case 6:
					equipID = current.Hero
				}
				if equipID != 0 {
					log.Printf("[Reconfigure] Unequipping slot %d (equipID: %f)", slot, equipID)
					equipmentview.UnequipEquipmentRaw(equipmentMode, targetIndex, equipID)
					time.Sleep(500 * time.Millisecond)
				}
			}

			// --- Step 2: Clean Gems ---
			log.Printf("[Reconfigure] Step 2: Cleaning target gems from storage equipment")
			// Check if any target gems are socketted in storage equipment
			targetGems := []float64{newLoadout.Gem1, newLoadout.Gem2, newLoadout.Gem3, newLoadout.Gem4}
			targetGemMap := make(map[float64]bool)
			for _, g := range targetGems {
				if g != 0 {
					targetGemMap[g] = true
				}
			}

			if len(targetGemMap) > 0 {
				for _, eq := range Models.GetGameState().Equipment.EquipmentStorage {
					if eq.GemSlot.Gem != nil {
						gemID := eq.GemSlot.Gem.ID
						if targetGemMap[gemID] {
							log.Printf("[Reconfigure] Found target gem (ID: %f) in storage equipment (ID: %f, Slot: %.0f). Performing double jump.", gemID, eq.ID, eq.EquipSlotNumber)
							// Double Jump: Equip -> UnequipGem -> UnequipItem
							slot := int(eq.EquipSlotNumber)
							equipmentview.EquipEquipment(equipmentMode, targetIndex, slot, eq.ID)
							time.Sleep(1 * time.Second)
							equipmentview.UnequipGemRaw(equipmentMode, targetIndex, eq.ID)
							time.Sleep(500 * time.Millisecond)
							equipmentview.UnequipEquipmentRaw(equipmentMode, targetIndex, eq.ID)
							time.Sleep(500 * time.Millisecond)
						}
					}
				}
			}

			// --- Step 3: Equip New Base Pieces & Hero ---
			log.Printf("[Reconfigure] Step 3: Equipping new base pieces and hero")
			newEquips := map[int]float64{
				1: newLoadout.Equip1,
				2: newLoadout.Equip2,
				3: newLoadout.Equip3,
				4: newLoadout.Equip4,
				6: newLoadout.Hero,
			}

			for slot, eid := range newEquips {
				if eid != 0 {
					log.Printf("[Reconfigure] Equipping slot %d with item ID %f", slot, eid)
					equipmentview.EquipEquipment(equipmentMode, targetIndex, slot, eid)
					time.Sleep(800 * time.Millisecond)
				}
			}

			// --- Step 4: Socket Gems ---
			log.Printf("[Reconfigure] Step 4: Socketing gems into new equipment")
			gemMapping := map[int]float64{
				1: newLoadout.Gem1,
				2: newLoadout.Gem2,
				3: newLoadout.Gem3,
				4: newLoadout.Gem4,
			}

			for slot, gid := range gemMapping {
				eid := newEquips[slot]
				if gid != 0 && eid != 0 {
					log.Printf("[Reconfigure] Socketing gem ID %f into equipment ID %f (Slot %d)", gid, eid, slot)
					equipmentview.EquipGem(equipmentMode, targetIndex, eid, gid)
					time.Sleep(800 * time.Millisecond)
				}
			}

			// Final Sync
			SendAlertMessage("green", "Reconfiguration complete!")
			GameCommands.SendGLI()
			time.Sleep(2 * time.Second)

			// Trigger frontend refresh
			var refreshMsg string
			if equipmentMode == "Commander" {
				refreshMsg = fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, targetIndex)
			} else {
				refreshMsg = fmt.Sprintf(`{"type":"getCastUpdate","castleIndex":%d}`, targetIndex)
			}
			ParseFrontendMessage([]byte(refreshMsg))
		}

	case "unequipEquipment":
		{
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				SendAlertMessage("red", "Invalid unequip equipment request")
				return
			}

			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)
			selectionsRaw, _ := payloadRaw["selections"].([]interface{})

			if len(selectionsRaw) == 0 {
				SendAlertMessage("red", "No equipment selected")
				return
			}

			// Fetch fresh data from game before attempting unequip
			GameCommands.SendGLI()
			time.Sleep(2 * time.Second)

			// Process each selection
			successCount := 0
			failCount := 0
			var lastErrorMsg string
			for _, selRaw := range selectionsRaw {
				sel, ok := selRaw.(map[string]interface{})
				if !ok {
					continue
				}
				slotNumber, _ := sel["slotNumber"].(float64)
				equipmentId, _ := sel["equipmentId"].(float64)

				result := equipmentview.UnequipEquipment(equipmentMode, int(targetIndex), int(slotNumber), equipmentId)
				if result.Success {
					successCount++
				} else {
					lastErrorMsg = result.Message
					failCount++
				}
			}

			// Report results with specific error messages
			if failCount == 0 {
				SendAlertMessage("green", fmt.Sprintf("Unequipped %d equipment successfully", successCount))
			} else if successCount == 0 {
				SendAlertMessage("red", lastErrorMsg)
			} else {
				SendAlertMessage("yellow", fmt.Sprintf("Unequipped %d equipment, %d failed: %s", successCount, failCount, lastErrorMsg))
			}

			// Refresh equipment data from game and update frontend via ParseFrontendMessage
			go func() {
				GameCommands.SendGLI()
				time.Sleep(2 * time.Second)
				// Call ParseFrontendMessage with getCommUpdate/getCastUpdate to trigger targeted refresh
				var refreshMsg string
				if equipmentMode == "Commander" {
					refreshMsg = fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
				} else {
					refreshMsg = fmt.Sprintf(`{"type":"getCastUpdate","castleIndex":%d}`, int(targetIndex))
				}
				ParseFrontendMessage([]byte(refreshMsg))
			}()
		}
	case "unequipGem":
		{
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				SendAlertMessage("red", "Invalid unequip gem request")
				return
			}

			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)
			selectionsRaw, _ := payloadRaw["selections"].([]interface{})

			if len(selectionsRaw) == 0 {
				SendAlertMessage("red", "No gems selected")
				return
			}

			// Fetch fresh data from game before attempting unequip
			GameCommands.SendGLI()
			time.Sleep(2 * time.Second)

			// Process each selection
			successCount := 0
			failCount := 0
			var lastErrorMsg string
			for _, selRaw := range selectionsRaw {
				sel, ok := selRaw.(map[string]interface{})
				if !ok {
					continue
				}
				slotNumber, _ := sel["slotNumber"].(float64)
				gemId, _ := sel["gemId"].(float64)
				equipmentId, _ := sel["equipmentId"].(float64)

				result := equipmentview.UnequipGem(equipmentMode, int(targetIndex), int(slotNumber), gemId, equipmentId)
				if result.Success {
					successCount++
				} else {
					lastErrorMsg = result.Message
					failCount++
				}
			}

			// Report results with specific error messages
			if failCount == 0 {
				SendAlertMessage("green", fmt.Sprintf("Unequipped %d gems successfully", successCount))
			} else if successCount == 0 {
				SendAlertMessage("red", lastErrorMsg)
			} else {
				SendAlertMessage("yellow", fmt.Sprintf("Unequipped %d gems, %d failed: %s", successCount, failCount, lastErrorMsg))
			}

			// Refresh equipment data from game and update frontend via ParseFrontendMessage
			go func() {
				GameCommands.SendGLI()
				time.Sleep(2 * time.Second)
				// Call ParseFrontendMessage with getCommUpdate/getCastUpdate to trigger targeted refresh
				var refreshMsg string
				if equipmentMode == "Commander" {
					refreshMsg = fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
				} else {
					refreshMsg = fmt.Sprintf(`{"type":"getCastUpdate","castleIndex":%d}`, int(targetIndex))
				}
				ParseFrontendMessage([]byte(refreshMsg))
			}()
		}
	case "getEquipmentUpgradeInfo":
		{
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				SendAlertMessage("red", "Invalid upgrade info request")
				return
			}
			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)
			if equipmentMode != "Commander" && equipmentMode != "Castellan" {
				SendAlertMessage("red", "Invalid equipment mode")
				return
			}
			GameCommands.SendUpgradeMenuRefresh()
			time.Sleep(2 * time.Second)
			info := equipmentview.BuildUpgradeInfo(equipmentMode, int(targetIndex))
			SendFrontendMessage("equipmentUpgradeInfo", info, "")
		}
	case "upgradeEquipment":
		{
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				SendAlertMessage("red", "Invalid upgrade equipment request")
				return
			}
			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)
			slotNumber, _ := payloadRaw["slotNumber"].(float64)
			equipmentId, _ := payloadRaw["equipmentId"].(float64)
			targetLevel, _ := payloadRaw["targetLevel"].(float64)

			if blocked, msg := equipmentview.UpgradeBlockedByCoinReserve(); blocked {
				SendAlertMessage("red", msg)
				return
			}

			go func() {
				result := equipmentview.UpgradeEquipmentToLevel(
					equipmentMode, int(targetIndex), int(slotNumber), equipmentId, int(targetLevel),
				)
				if result.Success {
					SendAlertMessage("green", result.Message)
				} else if result.UpgradesDone > 0 {
					SendAlertMessage("yellow", fmt.Sprintf("%s (stopped at level %d)", result.Message, result.FinalLevel))
				} else {
					SendAlertMessage("red", result.Message)
				}
				GameCommands.SendGLI()
				time.Sleep(2 * time.Second)
				var refreshMsg string
				if equipmentMode == "Commander" {
					refreshMsg = fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
				} else {
					refreshMsg = fmt.Sprintf(`{"type":"getCastUpdate","castleIndex":%d}`, int(targetIndex))
				}
				ParseFrontendMessage([]byte(refreshMsg))
			}()
		}
	case "upgradeGem":
		{
			payloadRaw, ok := data["payload"].(map[string]interface{})
			if !ok {
				SendAlertMessage("red", "Invalid upgrade gem request")
				return
			}
			equipmentMode, _ := payloadRaw["equipmentMode"].(string)
			targetIndex, _ := payloadRaw["targetIndex"].(float64)
			slotNumber, _ := payloadRaw["slotNumber"].(float64)
			gemId, _ := payloadRaw["gemId"].(float64)
			targetLevel, _ := payloadRaw["targetLevel"].(float64)

			if blocked, msg := equipmentview.UpgradeBlockedByCoinReserve(); blocked {
				SendAlertMessage("red", msg)
				return
			}

			go func() {
				result := equipmentview.UpgradeGemToLevel(
					equipmentMode, int(targetIndex), int(slotNumber), gemId, int(targetLevel),
				)
				if result.Success {
					SendAlertMessage("green", result.Message)
				} else if result.UpgradesDone > 0 {
					SendAlertMessage("yellow", fmt.Sprintf("%s (stopped at level %d)", result.Message, result.FinalLevel))
				} else {
					SendAlertMessage("red", result.Message)
				}
				GameCommands.SendGLI()
				time.Sleep(2 * time.Second)
				var refreshMsg string
				if equipmentMode == "Commander" {
					refreshMsg = fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
				} else {
					refreshMsg = fmt.Sprintf(`{"type":"getCastUpdate","castleIndex":%d}`, int(targetIndex))
				}
				ParseFrontendMessage([]byte(refreshMsg))
			}()
		}
	case "triggerUpdate":
		log.Println("Received request to trigger self-update")
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			SendAlertMessage("red", "Invalid update request")
			return
		}

		downloadUrl, _ := payloadRaw["downloadUrl"].(string)
		if downloadUrl == "" {
			SendAlertMessage("red", "No download URL provided")
			return
		}

		// Import Version package dynamically called via callback
		// The actual update is performed in a goroutine
		go func() {
			if err := triggerSelfUpdate(downloadUrl); err != nil {
				log.Printf("Self-update failed: %v", err)
				SendUpdateErrorMessage(err.Error())
				SendAlertMessage("red", fmt.Sprintf("Update failed: %v", err))
			}
		}()

	case "getCastleList":
		gs := Models.GetGameState()
		type CastleItem struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		}
		var castles []CastleItem

		addCastle := func(aid float64, name, cType string) {
			if aid > 0 {
				castles = append(castles, CastleItem{ID: int(aid), Name: name, Type: cType})
			}
		}

		c := &gs.Castle
		addCastle(c.MainCastle.Aid, c.MainCastle.Name, "Main")
		addCastle(c.Outpost1.Aid, c.Outpost1.Name, "Outpost")
		addCastle(c.Outpost2.Aid, c.Outpost2.Name, "Outpost")
		addCastle(c.Outpost3.Aid, c.Outpost3.Name, "Outpost")
		addCastle(c.IceCastle.Aid, c.IceCastle.Name, "Ice")
		addCastle(c.DesertCastle.Aid, c.DesertCastle.Name, "Desert")
		addCastle(c.DungeonCastle.Aid, c.DungeonCastle.Name, "Dungeon")
		addCastle(c.StormCastle.Aid, c.StormCastle.Name, "Storm")
		addCastle(c.Metropolis.Aid, c.Metropolis.Name, "Metropolis")
		addCastle(c.Capital.Aid, c.Capital.Name, "Capital")

		SendFrontendMessage("castleList", castles, "")

	case "getRecruitTroopsSettings":
		if Models.GetSettingsState().RecruitTroopsList.Targets == nil {
			Models.GetSettingsState().RecruitTroopsList.Targets = make(map[int]map[int]int)
		}
		SendFrontendMessage("recruitTroopsSettings", Models.GetSettingsState().RecruitTroopsList.Targets, "")

	case "saveRecruitTroopsSettings":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}

		newSettings := make(map[int]map[int]int)

		for castleIDStr, itemsRaw := range payloadRaw {
			castleID, _ := strconv.Atoi(castleIDStr)
			if castleID == 0 {
				continue
			}

			if items, ok := itemsRaw.([]interface{}); ok {
				castleMap := make(map[int]int)
				for _, itemRaw := range items {
					if item, ok := itemRaw.(map[string]interface{}); ok {
						unitID := int(item["id"].(float64))
						amount := int(item["amount"].(float64))
						if unitID > 0 && amount >= 0 {
							castleMap[unitID] = amount
						}
					}
				}
				newSettings[castleID] = castleMap
			}
		}

		Models.GetSettingsState().UpdateRecruitTroopsList(newSettings)
		SendFrontendMessage("recruitTroopsSettings", newSettings, "")

	case "getConstructionItemsCatalog":
		cat, err := featureview.ConstructionItemsCatalog()
		if err != nil {
			log.Printf("[frontend-ws] getConstructionItemsCatalog: %v", err)
			SendFrontendMessage("constructionItemsCatalog", []featureview.ConstructionItemCatalogEntry{}, "")
			break
		}
		SendFrontendMessage("constructionItemsCatalog", cat, "")

	case "getAutoTCISettings":
		targets := stsettings.ReadAutoTCITargets()
		Models.GetSettingsState().AutoTCIList.Targets = targets
		SendFrontendMessage("autoTCISettings", stsettings.TargetsToWire(targets), "")

	case "saveAutoTCISettings":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		newSettings := autoTCITargetsFromClientPayload(payloadRaw)
		Models.GetSettingsState().UpdateAutoTCIList(newSettings)
		SendFrontendMessage("autoTCISettings", stsettings.TargetsToWire(newSettings), "")
		log.Println("[frontend-ws] Auto TCI settings saved:", filepath.Join(Paths.DataDir(), "AutoTCI.json"))

	case "getAutoTCIClientState":
		raw := stsettings.ReadAutoTCIClientFile()
		var payload interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			log.Printf("[frontend-ws] getAutoTCIClientState parse: %v", err)
			_ = json.Unmarshal(stsettings.DefaultAutoTCIClientJSON(), &payload)
		}
		SendFrontendMessage("autoTCIClientState", payload, "")

	case "saveAutoTCIClientState":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			log.Println("[frontend-ws] saveAutoTCIClientState: missing payload")
			return
		}
		out, err := json.Marshal(payloadRaw)
		if err != nil {
			log.Printf("[frontend-ws] saveAutoTCIClientState marshal: %v", err)
			SendAlertMessage("red", "Auto TCI: invalid data")
			return
		}
		if err := stsettings.WriteAutoTCIClientFile(out); err != nil {
			log.Printf("[frontend-ws] saveAutoTCIClientState write: %v", err)
			SendAlertMessage("red", "Could not save Auto TCI settings to disk")
			return
		}
		applyAutoTCIClientStatePayload(payloadRaw)
		log.Println("[frontend-ws] Auto TCI client state saved:", filepath.Join(Paths.DataDir(), "AutoTCI.json"))

	case "getAutoBeriWorldSettings":
		cfg := stsettings.ReadAutoBeriWorldConfig()
		Models.GetSettingsState().AutoBeriWorld = cfg
		featureview.SyncBeriCastleFromSettings()
		featureview.SyncKutSourceFromMainCastle()
		SendFrontendMessage("autoBeriWorldSettings", autoBeriWorldConfigToWire(cfg), "")

	case "saveAutoBeriWorldSettings":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		applyAutoBeriWorldConfigFromMap(payloadRaw)
		cfg := Models.GetSettingsState().AutoBeriWorld
		SendFrontendMessage("autoBeriWorldSettings", autoBeriWorldConfigToWire(cfg), "")
		log.Println("[frontend-ws] Auto Beri World settings saved:", filepath.Join(Paths.DataDir(), "AutoBeriWorld.json"))

	case "sendCustomMessage":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			log.Println("Invalid sendCustomMessage request")
			return
		}
		messageCode, ok := payloadRaw["messageCode"].(string)
		if !ok || messageCode == "" {
			SendAlertMessage("red", "No message code provided")
			return
		}

		log.Printf("[Custom Message] Sending %s code: %s", ResponseRegistry.EmpireExToken, messageCode)
		GameCommands.SendEmpireEx21EmptyCommand(messageCode)
		SendAlertMessage("green", fmt.Sprintf("Sent custom message: %s", messageCode))
	case "getSchedulerSettings":
		SendFrontendMessage("schedulerSettings", Models.GetSettingsState(), "")
	case "getAutoBirdClientState":
		raw := stsettings.ReadAutoBirdClientFile()
		var payload interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			log.Printf("[frontend-ws] getAutoBirdClientState parse: %v", err)
			_ = json.Unmarshal(stsettings.DefaultAutoBirdClientJSON(), &payload)
		}
		SendFrontendMessage("autoBirdClientState", payload, "")
	case "saveAutoBirdClientState":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			log.Println("[frontend-ws] saveAutoBirdClientState: missing payload")
			return
		}
		out, err := json.Marshal(payloadRaw)
		if err != nil {
			log.Printf("[frontend-ws] saveAutoBirdClientState marshal: %v", err)
			SendAlertMessage("red", "Auto Bird: invalid data")
			return
		}
		if err := stsettings.WriteAutoBirdClientFile(out); err != nil {
			log.Printf("[frontend-ws] saveAutoBirdClientState write: %v", err)
			SendAlertMessage("red", "Could not save Auto Bird settings to disk")
			return
		}
		applyAutoBirdClientStatePayload(payloadRaw)
		log.Println("[frontend-ws] Auto Bird client state saved:", filepath.Join(Paths.DataDir(), "AutoBird.json"))
	case "saveSchedulerSettings":
		payloadRaw, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}

		state := Models.GetSettingsState()

		if minDelay, ok := payloadRaw["minAttackDelay"].(float64); ok {
			state.MinAttackDelay = minDelay
		}
		if maxDelay, ok := payloadRaw["maxAttackDelay"].(float64); ok {
			state.MaxAttackDelay = maxDelay
		}
		if ereDelay, ok := payloadRaw["upgradeEreDelayMs"].(float64); ok {
			state.UpgradeEreDelayMs = int(ereDelay)
		}
		if coinThreshold, ok := payloadRaw["upgradeCoinThreshold"].(float64); ok {
			state.UpgradeCoinThreshold = coinThreshold
		}

		if priorities, ok := payloadRaw["tabPriorities"].(map[string]interface{}); ok {
			for tabID, pRaw := range priorities {
				if priorityStr, ok := pRaw.(string); ok {
					state.TabPriorities[tabID] = Models.TabPriority(priorityStr)
				}
			}
		}

		// Echo back confirmed settings
		if err := stsettings.PersistSchedulerSettings(state); err != nil {
			log.Printf("[frontend-ws] saveSchedulerSettings write: %v", err)
			SendAlertMessage("red", "Could not save scheduler settings to disk")
			return
		}
		SendFrontendMessage("schedulerSettings", state, "")
		SendAlertMessage("green", "Scheduler Settings saved")
		log.Println("[frontend-ws] Scheduler settings saved:", filepath.Join(Paths.DataDir(), "SchedulerSettings.json"))

	case "getCastleFocus":
		SendFrontendMessage("castleFocus", Models.CastleFocusMessagePayload(), "")

	case "getRiftMaidenCommsSettings":
		cfg := stsettings.ReadRiftMaidenCommsSettings()
		SendFrontendMessage("riftMaidenCommsSettings", map[string]interface{}{
			"unitWodID": cfg.UnitWodID,
		}, "")

	case "saveRiftMaidenCommsSettings":
		payload, _ := data["payload"].(map[string]interface{})
		unitWodID := optionalIntPayload(payload, "unitWodID", 0)
		if unitWodID <= 0 {
			SendAlertMessage("red", "Invalid probe unit id")
			return
		}
		cfg := stsettings.RiftMaidenCommsSettings{UnitWodID: unitWodID}
		if err := stsettings.WriteRiftMaidenCommsSettings(cfg); err != nil {
			log.Printf("[frontend-ws] saveRiftMaidenCommsSettings write: %v", err)
			SendAlertMessage("red", "Could not save maiden comms settings to disk")
			return
		}
		SendFrontendMessage("riftMaidenCommsSettings", map[string]interface{}{
			"unitWodID": cfg.UnitWodID,
		}, "")
		log.Println("[frontend-ws] Rift maiden comms settings saved:", filepath.Join(Paths.DataDir(), "RiftMaidenComms.json"))

	case "getRiftCRALaunch":
		SendRiftCRALaunch()

	case "getMovement":
		refresh := false
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if v, ok := payload["refresh"].(bool); ok {
				refresh = v
			}
		}
		if refresh && ResponseRegistry.LoginStatus {
			GameCommands.SendGAM()
		}
		SendMovementUpdate()

	case "replayRiftCRALaunch":
		payload, _ := data["payload"].(map[string]interface{})
		launchID, _ := payload["launchId"].(string)
		if launchID == "" {
			SendAlertMessage("red", "Missing launch id")
			Logging.RiftLog("resend_failed", "missing launchId in payload")
			return
		}
		if !ResponseRegistry.LoginStatus {
			SendAlertMessage("red", "Game not connected — log in before resending")
			Logging.RiftLogf("resend_blocked", "launch=%s game not logged in", launchID)
			return
		}
		commanderID := optionalIntPayload(payload, "commanderID", -1)
		sourceX := optionalIntPayload(payload, "sourceX", -1)
		sourceY := optionalIntPayload(payload, "sourceY", -1)
		arriveAtUnix := int64(0)
		if v, ok := payload["arriveAtUnix"]; ok && v != nil {
			arriveAtUnix = int64(optionalIntPayload(payload, "arriveAtUnix", 0))
		}
		Logging.RiftLogf("resend_request", "UI launch=%s LID=%d src=(%d,%d) arriveAt=%d",
			launchID, commanderID, sourceX, sourceY, arriveAtUnix)
		if err := featureview.ReplayOrScheduleRiftCRA(launchID, arriveAtUnix, commanderID, sourceX, sourceY); err != nil {
			SendAlertMessage("red", err.Error())
			return
		}
		if arriveAtUnix > time.Now().Unix()+30 {
			SendAlertMessage("green", "Rift attack scheduled")
		} else {
			SendAlertMessage("green", "Rift attack resend queued")
		}

	case "cancelRiftCRALaunchSchedule":
		payload, _ := data["payload"].(map[string]interface{})
		launchID, _ := payload["launchId"].(string)
		if launchID == "" {
			return
		}
		featureview.CancelRiftCRASchedule(launchID)
		SendRiftCRALaunch()

	case "renameRiftCRALaunch":
		payload, _ := data["payload"].(map[string]interface{})
		launchID, _ := payload["launchId"].(string)
		if launchID == "" {
			SendAlertMessage("red", "Missing launch id")
			return
		}
		displayName, _ := payload["displayName"].(string)
		if !riftattack.RenameLaunch(launchID, displayName) {
			SendAlertMessage("red", "Rift attack template not found")
			return
		}
		SendRiftCRALaunch()
		SendAlertMessage("green", "Rift attack renamed")

	case "deleteRiftCRALaunch":
		payload, _ := data["payload"].(map[string]interface{})
		launchID, _ := payload["launchId"].(string)
		if launchID == "" {
			SendAlertMessage("red", "Missing launch id")
			return
		}
		featureview.CancelRiftCRAScheduleQuiet(launchID)
		if !riftattack.DeleteLaunch(launchID) {
			SendAlertMessage("red", fmt.Sprintf("Rift attack not found (id %q)", launchID))
			Logging.RiftLogf("delete_failed", "launch %q not in %s", launchID, riftattack.FilePath())
			return
		}
		Logging.RiftLogf("delete", "removed launch %q", launchID)
		SendRiftCRALaunch()
		SendAlertMessage("green", "Rift attack deleted")

	case "sendMaidenCommsWave":
		payload, _ := data["payload"].(map[string]interface{})
		sourceX := optionalIntPayload(payload, "sourceX", -1)
		sourceY := optionalIntPayload(payload, "sourceY", -1)
		unitWodID := optionalIntPayload(payload, "unitWodID", 0)
		log.Printf("[frontend-ws] sendMaidenCommsWave src=(%d,%d) unit=%d", sourceX, sourceY, unitWodID)
		if !ResponseRegistry.LoginStatus {
			SendAlertMessage("red", "Game not connected — log in before sending maiden comms")
			return
		}
		SendAlertMessage("yellow", "Maiden comms wave started…")
		go func(sx, sy, unit int) {
			result, err := GameParser.SendMaidenCommsWave(sx, sy, unit)
			if err != nil {
				SendAlertMessage("red", err.Error())
				return
			}
			if result.Sent == 0 {
				SendAlertMessage(
					"yellow",
					fmt.Sprintf("No maiden comms sent (%d busy, %d without shield-maiden artifact)", result.SkippedBusy, result.SkippedNoArtifact),
				)
			} else {
				SendAlertMessage(
					"green",
					fmt.Sprintf("Queued %d maiden comm attack(s) (skipped %d busy, %d no artifact)", result.Sent, result.SkippedBusy, result.SkippedNoArtifact),
				)
			}
			SendFrontendMessage("maidenCommsWaveResult", result, "")
		}(sourceX, sourceY, unitWodID)

	case "getRiftMapCoords":
		refresh := false
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if v, ok := payload["refresh"].(bool); ok {
				refresh = v
			}
		}
		if refresh {
			gs := Models.GetGameState()
			f := gs.CastleFocus
			if f.CastleAID > 0 {
				if x, y, ok := gs.ResolveCastleMapCoords(f.CastleAID, f.KingdomID); ok {
					GameCommands.SendCastleFocus(f.KingdomID, f.CastleAID, x, y)
				}
			}
			if rift, riftKid, ok := Models.GetMapState().FindRift(); ok {
				GameCommands.SendGAAAroundTile(riftKid, rift.X, rift.Y, 12)
			} else if f.KingdomID == 0 {
				cx, cy := f.MapPX, f.MapPY
				if cx == 0 && cy == 0 {
					cx, cy, _ = gs.ResolveCastleMapCoords(f.CastleAID, f.KingdomID)
				}
				if cx != 0 || cy != 0 {
					GameCommands.SendGAAAroundTile(0, cx, cy, mapstate.RiftMapRadius)
				}
			}
		}
		SendRiftMapCoords()

	case "focusPlayerCastle":
		payload, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		castleID := 0
		if v, ok := payload["castleId"].(float64); ok {
			castleID = int(v)
		}
		if castleID <= 0 {
			SendAlertMessage("red", "focusPlayerCastle: castleId required.")
			return
		}
		gs := Models.GetGameState()
		if !gs.IsKnownPlayerCastleID(castleID) {
			SendAlertMessage("red", "That castle is not in your account.")
			return
		}
		kingdomID := 0
		if v, ok := payload["kingdomId"].(float64); ok {
			kingdomID = int(v)
		}
		mapX, mapY := 0, 0
		if v, ok := payload["mapX"].(float64); ok {
			mapX = int(v)
		}
		if v, ok := payload["mapY"].(float64); ok {
			mapY = int(v)
		}
		// Same rule as GameCommands.SendCastleFocus: JAA uses map PX/PY for KID 0, 4, 10; other kingdoms use JCA.
		needsMapCoords := kingdomID == 0 || kingdomID == 4 || kingdomID == 10
		if needsMapCoords && mapX == 0 && mapY == 0 {
			var okCoords bool
			mapX, mapY, okCoords = gs.ResolveCastleMapCoords(castleID, kingdomID)
			if !okCoords || (mapX == 0 && mapY == 0) {
				for i := range gs.Alliance.PlayerCastleLocations {
					L := &gs.Alliance.PlayerCastleLocations[i]
					if L.CastleID != castleID {
						continue
					}
					if kingdomID != 0 && L.KingdomID != kingdomID {
						continue
					}
					if L.X != 0 || L.Y != 0 {
						mapX, mapY, okCoords = L.X, L.Y, true
						if kingdomID == 0 {
							kingdomID = L.KingdomID
						}
						break
					}
				}
			}
			if !okCoords || (mapX == 0 && mapY == 0) {
				for i := range gs.Alliance.PlayerCastleLocations {
					L := &gs.Alliance.PlayerCastleLocations[i]
					if L.CastleID == castleID && (L.X != 0 || L.Y != 0) {
						mapX, mapY, okCoords = L.X, L.Y, true
						if kingdomID == 0 {
							kingdomID = L.KingdomID
						}
						break
					}
				}
			}
			if !okCoords || (mapX == 0 && mapY == 0) {
				SendAlertMessage("red", "Could not resolve map coordinates for that castle.")
				return
			}
		}
		if !GameParser.FocusPlayerCastleTroops(kingdomID, castleID, mapX, mapY) {
			SendAlertMessage("red", "Focus timed out — ensure the game client is connected.")
			return
		}
		SendFrontendMessage("castleFocus", Models.CastleFocusMessagePayload(), "")
		SendAlertMessage("green", "Switched castle focus.")

	case "getDecorationPresets":
		payload, _ := data["payload"].(map[string]interface{})
		cid := 0
		if payload != nil {
			if v, ok := payload["castleId"].(float64); ok {
				cid = int(v)
			}
		}
		if cid <= 0 {
			cid = Models.GetGameState().CastleFocus.CastleAID
		}
		if cid <= 0 {
			SendFrontendMessage("decorationPresets", []dec.NamedPreset{}, strconv.Itoa(cid))
			return
		}
		SendFrontendMessage("decorationPresets", dec.ListPresetsForKey(GameParser.DecorationPresetStorageKey(cid), cid), strconv.Itoa(cid))

	case "saveDecorationPreset":
		payload, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		name, _ := payload["name"].(string)
		castleID := int(0)
		if v, ok := payload["castleId"].(float64); ok {
			castleID = int(v)
		}
		if castleID <= 0 {
			castleID = Models.GetGameState().CastleFocus.CastleAID
		}
		f := Models.GetGameState().CastleFocus
		if castleID <= 0 || castleID != f.CastleAID {
			SendAlertMessage("red", "Decoration preset: focus the target castle in-game first (castle id mismatch).")
			return
		}
		c := Models.GetGameState().GetCastleByID(castleID)
		if c == nil {
			SendAlertMessage("red", "Decoration preset: castle not in GameState.")
			return
		}
		storageKey := GameParser.DecorationPresetStorageKey(castleID)
		items := castleview.BuildPresetPlacementsFromCastle(c)
		saved, err := dec.SavePresetForKey(storageKey, castleID, name, items)
		if err != nil {
			SendAlertMessage("red", fmt.Sprintf("Save preset failed: %v", err))
			return
		}
		SendFrontendMessage("decorationPresets", dec.ListPresetsForKey(storageKey, castleID), strconv.Itoa(castleID))
		SendAlertMessage("green", fmt.Sprintf("Saved decoration preset %q (%d items)", saved.Name, len(saved.Items)))

	case "deleteDecorationPreset":
		payload, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		castleID := int(0)
		if v, ok := payload["castleId"].(float64); ok {
			castleID = int(v)
		}
		presetID, _ := payload["presetId"].(string)
		if castleID <= 0 || presetID == "" {
			return
		}
		storageKey := GameParser.DecorationPresetStorageKey(castleID)
		if err := dec.DeletePresetForKey(storageKey, castleID, presetID); err != nil {
			SendAlertMessage("red", fmt.Sprintf("Delete preset failed: %v", err))
			return
		}
		SendFrontendMessage("decorationPresets", dec.ListPresetsForKey(storageKey, castleID), strconv.Itoa(castleID))

	case "applyDecorationPreset":
		payload, ok := data["payload"].(map[string]interface{})
		if !ok {
			return
		}
		castleID := int(0)
		if v, ok := payload["castleId"].(float64); ok {
			castleID = int(v)
		}
		kingdomID := Models.GetGameState().CastleFocus.KingdomID
		if v, ok := payload["kingdomId"].(float64); ok {
			kingdomID = int(v)
		}
		presetID, _ := payload["presetId"].(string)
		if castleID <= 0 || presetID == "" {
			SendAlertMessage("red", "applyDecorationPreset: castleId and presetId required.")
			return
		}
		gs := Models.GetGameState()
		mapX, mapY, okCoords := gs.ResolveCastleMapCoords(castleID, kingdomID)
		if !okCoords {
			SendAlertMessage("red", "Decoration apply: could not resolve map coordinates for castle.")
			return
		}
		castleview.StartDecorationPresetApply(castleID, kingdomID, mapX, mapY, GameParser.DecorationPresetStorageKey(castleID), presetID, func(msg string) {
			SendFrontendMessage("decorationPlacerProgress", map[string]interface{}{"message": msg}, "")
		}, func(category string, message string) {
			SendAlertMessage(category, message)
		}, func(shortfalls []castleview.DecorationStorageShortfall) {
			SendFrontendMessage("decorationPlacerStorageMismatch", map[string]interface{}{"items": shortfalls}, "")
		})
		SendAlertMessage("green", "Decoration preset apply started.")

	case "cancelDecorationApply":
		castleview.CancelDecorationApply()
		SendAlertMessage("yellow", "Decoration apply cancel requested.")

	default:
		log.Printf("[frontend-ws] unhandled message type: %q", messageType)
	}
}

// applyAutoBirdIgnoreSettingsFromMap updates in-memory AutoBird ignore list and delay settings from UI JSON.
func applyAutoBirdIgnoreSettingsFromMap(ignoreRaw map[string]interface{}) {
	if ignoreRaw == nil {
		return
	}
	st := Models.GetSettingsState()
	if settingsRaw, ok := ignoreRaw["settings"].(map[string]interface{}); ok {
		newSettings := make(map[int]map[int]int)
		for castleIDStr, itemsRaw := range settingsRaw {
			castleID, _ := strconv.Atoi(castleIDStr)
			if castleID == 0 {
				continue
			}
			if items, ok := itemsRaw.([]interface{}); ok {
				castleMap := make(map[int]int)
				for _, itemRaw := range items {
					if item, ok := itemRaw.(map[string]interface{}); ok {
						unitID := int(item["id"].(float64))
						amount := int(item["amount"].(float64))
						if unitID > 0 && amount >= 0 {
							castleMap[unitID] = amount
						}
					}
				}
				newSettings[castleID] = castleMap
			}
		}
		st.UpdateBirdIgnoreList(newSettings)
	}
	if minDelay, ok := ignoreRaw["minDelay"].(float64); ok {
		st.AutoBirdDelay.MinDelay = int(minDelay)
	}
	if maxDelay, ok := ignoreRaw["maxDelay"].(float64); ok {
		st.AutoBirdDelay.MaxDelay = int(maxDelay)
	}
	if minSend, ok := ignoreRaw["minSend"].(float64); ok {
		st.AutoBirdDelay.MinSend = int(minSend)
	}
	if minRPTDays, ok := ignoreRaw["minRPTDays"].(float64); ok {
		st.AutoBirdDelay.MinRPTDays = int(minRPTDays)
	}
}

// autoBeriWorldConfigToWire shapes the Beri world options for the frontend.
func autoBeriWorldConfigToWire(cfg stsettings.AutoBeriWorldConfig) map[string]interface{} {
	return map[string]interface{}{
		"minTroopsToTransfer":        cfg.MinTroopsToTransfer,
		"beriCastleCID":              cfg.BeriCastleCID,
		"beriMapX":                   cfg.BeriMapX,
		"beriMapY":                   cfg.BeriMapY,
		"transferTroopWID":           cfg.TransferTroopWID,
		"kutSourceCastleSCID":        cfg.KutSourceCastleSCID,
		"kutCastleCID":               cfg.KutCastleCID,
		"troopSpaceCheckIntervalSec": cfg.TroopSpaceCheckIntervalSec,
	}
}

// applyAutoBeriWorldConfigFromMap updates and persists Auto Beri World options from UI JSON.
func applyAutoBeriWorldConfigFromMap(payloadRaw map[string]interface{}) {
	if payloadRaw == nil {
		return
	}
	cfg := Models.GetSettingsState().AutoBeriWorld
	if v, ok := payloadRaw["minTroopsToTransfer"].(float64); ok {
		cfg.MinTroopsToTransfer = int(v)
	}
	if v, ok := payloadRaw["beriCastleCID"].(float64); ok {
		cfg.BeriCastleCID = int(v)
	}
	if v, ok := payloadRaw["beriMapX"].(float64); ok {
		cfg.BeriMapX = int(v)
	}
	if v, ok := payloadRaw["beriMapY"].(float64); ok {
		cfg.BeriMapY = int(v)
	}
	if v, ok := payloadRaw["transferTroopWID"].(float64); ok {
		cfg.TransferTroopWID = int(v)
	}
	if v, ok := payloadRaw["kutSourceCastleSCID"].(float64); ok {
		cfg.KutSourceCastleSCID = int(v)
	}
	if v, ok := payloadRaw["kutCastleCID"].(float64); ok {
		cfg.KutCastleCID = int(v)
	}
	if v, ok := payloadRaw["troopSpaceCheckIntervalSec"].(float64); ok {
		cfg.TroopSpaceCheckIntervalSec = int(v)
	}
	Models.GetSettingsState().UpdateAutoBeriWorldConfig(cfg)
	featureview.SyncBeriCastleFromSettings()
	featureview.SyncKutSourceFromMainCastle()
}

func applyAutoBirdClientStatePayload(payloadRaw map[string]interface{}) {
	if payloadRaw == nil {
		return
	}
	if ignoreRaw, ok := payloadRaw["ignoreSettings"].(map[string]interface{}); ok {
		applyAutoBirdIgnoreSettingsFromMap(ignoreRaw)
	}
}

func autoTCITargetsFromClientPayload(payloadRaw map[string]interface{}) map[int]map[int]stsettings.AutoTCILevelTarget {
	if payloadRaw == nil {
		return map[int]map[int]stsettings.AutoTCILevelTarget{}
	}
	raw := make(map[string]interface{}, len(payloadRaw))
	for k, v := range payloadRaw {
		raw[k] = v
	}
	out := stsettings.AutoTCITargetsFromClientMap(raw)
	for castleID, perCastle := range out {
		for tciID, tgt := range perCastle {
			perCastle[tciID] = tgt.Normalize()
		}
		out[castleID] = perCastle
	}
	return out
}

func applyAutoTCIClientStatePayload(payloadRaw map[string]interface{}) {
	if payloadRaw == nil {
		return
	}
	if targetsRaw, ok := payloadRaw["targets"].(map[string]interface{}); ok {
		Models.GetSettingsState().AutoTCIList.Targets = autoTCITargetsFromClientPayload(targetsRaw)
	}
}

func optionalIntPayload(payload map[string]interface{}, key string, fallback int) int {
	if payload == nil {
		return fallback
	}
	switch v := payload[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case int64:
		return int(v)
	}
	return fallback
}

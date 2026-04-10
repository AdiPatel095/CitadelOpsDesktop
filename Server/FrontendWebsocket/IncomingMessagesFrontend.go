package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFunctions"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	dec "CitadelDesktop/Server/Models/Decoration"
	equip "CitadelDesktop/Server/Models/Equipment"
	"CitadelDesktop/Server/ReconfigureLoadout"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Scheduler"
	"CitadelDesktop/Server/Version"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"
)

// triggerSelfUpdate is a wrapper that calls Version.PerformSelfUpdate
func triggerSelfUpdate(downloadUrl string) error {
	return Version.PerformSelfUpdate(downloadUrl)
}

func init() {
	// Wire up callback so GameParser can notify frontend when castle data changes
	GameParser.UpdateCastleResourceFunc = func(castleLocation string) {
		SendCastleResource(castleLocation)
	}
	ResponseRegistry.SendRecruitTroopsStatusFunc = SendRecruitTroopsStatus
	GameParser.NotifyCastleFocusChanged = func() {
		SendFrontendMessage("castleFocus", Models.CastleFocusMessagePayload(), "")
	}
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
		GameFunctions.FetchAllianceInfo()
	case "toggleAutoBird":
		wasRunning := GameFunctions.IsAutoBirdRunning()
		if wasRunning {
			GameFunctions.StopAutoBird()
			Models.GetSettingsState().AutoBirdEnabled = false
			SendAutoBirdStatus(false, 0)
		} else {
			Models.GetSettingsState().AutoBirdEnabled = true
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
					Models.GetSettingsState().UpdateBirdIgnoreList(newSettings)
				}
				if minDelay, ok := payloadRaw["minDelay"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MinDelay = int(minDelay)
				}
				if maxDelay, ok := payloadRaw["maxDelay"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MaxDelay = int(maxDelay)
				}
				if minSend, ok := payloadRaw["minSend"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MinSend = int(minSend)
				}
			}
			GameFunctions.StartAutoBird()
			SendAutoBirdStatus(true, 0)
		}
	case "toggleRecruitTroops":
		wasRunning := GameFunctions.IsRecruitTroopsRunning()
		log.Printf("[RecruitTroops] Toggle requested. Was running: %v", wasRunning)

		if wasRunning {
			log.Println("[RecruitTroops] Calling StopRecruitTroops...")
			GameFunctions.StopRecruitTroops()
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
				GameFunctions.StartRecruitTroops()
				SendRecruitTroopsStatus(true)
			}
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
			ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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
				currentLoadout := GameFunctions.GetCastellanStat(targetIndex)

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
					GameFunctions.UnequipEquipmentRaw(equipmentMode, targetIndex, equipID)
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
							GameFunctions.EquipEquipment(equipmentMode, targetIndex, slot, eq.ID)
							time.Sleep(1 * time.Second)
							GameFunctions.UnequipGemRaw(equipmentMode, targetIndex, eq.ID)
							time.Sleep(500 * time.Millisecond)
							GameFunctions.UnequipEquipmentRaw(equipmentMode, targetIndex, eq.ID)
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
					GameFunctions.EquipEquipment(equipmentMode, targetIndex, slot, eid)
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
					GameFunctions.EquipGem(equipmentMode, targetIndex, eid, gid)
					time.Sleep(800 * time.Millisecond)
				}
			}

			// Final Sync
			SendAlertMessage("green", "Reconfiguration complete!")
			ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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
			ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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

				result := GameFunctions.UnequipEquipment(equipmentMode, int(targetIndex), int(slotNumber), equipmentId)
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
				ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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
			ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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

				result := GameFunctions.UnequipGem(equipmentMode, int(targetIndex), int(slotNumber), gemId, equipmentId)
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
				ResponseRegistry.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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

		// Format and send the message to the game server
		formattedMessage := fmt.Sprintf("%%xt%%EmpireEx_21%%%s%%1%%{}%%", messageCode)
		log.Printf("[Custom Message] Sending: %s", formattedMessage)
		ResponseRegistry.OutgoingMessages <- []byte(formattedMessage)
		SendAlertMessage("green", fmt.Sprintf("Sent custom message: %s", messageCode))
	case "getSchedulerSettings":
		SendFrontendMessage("schedulerSettings", Models.GetSettingsState(), "")
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

		if priorities, ok := payloadRaw["tabPriorities"].(map[string]interface{}); ok {
			for tabID, pRaw := range priorities {
				if priorityStr, ok := pRaw.(string); ok {
					state.TabPriorities[tabID] = Models.TabPriority(priorityStr)
				}
			}
		}

		// Echo back confirmed settings
		SendFrontendMessage("schedulerSettings", state, "")
		SendAlertMessage("green", "Scheduler Settings saved")

	case "getCastleFocus":
		SendFrontendMessage("castleFocus", Models.CastleFocusMessagePayload(), "")
		GameCommands.SendSPLRefreshDefaultProductionLIDs()

	case "requestSlotProduction":
		lid := 0
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if v, ok := payload["lid"].(float64); ok {
				lid = int(v)
			}
		}
		GameCommands.SendSPL(lid)

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
		// Same rule as GameCommands.SendTroopFocus: JAA uses map PX/PY for KID 0, 4, 10; other kingdoms use JCA.
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
		go func() {
			time.Sleep(280 * time.Millisecond)
			GameCommands.SendSPLRefreshDefaultProductionLIDs()
		}()

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
		SendFrontendMessage("decorationPresets", dec.ListPresetsForCastle(cid), strconv.Itoa(cid))

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
		items := GameFunctions.BuildPresetPlacementsFromCastle(c)
		saved, err := dec.SavePresetForCastle(castleID, name, items)
		if err != nil {
			SendAlertMessage("red", fmt.Sprintf("Save preset failed: %v", err))
			return
		}
		SendFrontendMessage("decorationPresets", dec.ListPresetsForCastle(castleID), strconv.Itoa(castleID))
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
		if err := dec.DeletePreset(castleID, presetID); err != nil {
			SendAlertMessage("red", fmt.Sprintf("Delete preset failed: %v", err))
			return
		}
		SendFrontendMessage("decorationPresets", dec.ListPresetsForCastle(castleID), strconv.Itoa(castleID))

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
		GameFunctions.StartDecorationPresetApply(castleID, kingdomID, mapX, mapY, presetID, func(msg string) {
			SendFrontendMessage("decorationPlacerProgress", map[string]interface{}{"message": msg}, "")
		})
		SendAlertMessage("green", "Decoration preset apply started (watch progress in UI).")

	case "cancelDecorationApply":
		GameFunctions.CancelDecorationApply()
		SendAlertMessage("yellow", "Decoration apply cancel requested.")
	}
}

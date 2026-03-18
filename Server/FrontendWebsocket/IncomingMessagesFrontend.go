package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameFunctions"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ReconfigureLoadout"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/Scheduler"
	"CitadelDesktop/Server/Version"
	"encoding/json"
	"fmt"
	"log"
	"os"
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
}

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
		castleLocation := data["castleLocation"].(string)
		SendCastleResource(castleLocation)
	case "sellNonRelicEquipment":
		log.Println("Received request to sell non-relic equipment")

		// Parse payload
		var sellLookItems, sellRift bool
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["sellLookItems"].(bool); ok {
				sellLookItems = val
			}
			if val, ok := payload["sellRift"].(bool); ok {
				sellRift = val
			}
		}

		log.Printf("Flags - Sell Look Items: %v, Sell Rift: %v", sellLookItems, sellRift)

		soldCount := SellNonRelicEquipment(sellRift, sellLookItems)
		log.Printf("SoldCount: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic equipment items", soldCount))
	case "sellNonRelicGems":
		log.Println("Received request to sell non-relic gems")
		var sellRiftGems bool
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["sellRiftGems"].(bool); ok {
				sellRiftGems = val
			}
		}
		soldCount := SellNonRelicGems(sellRiftGems)
		log.Printf("SoldCount: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d non-relic gems", soldCount))
	case "sellRelic1Equipment":
		log.Println("Received request to sell Relic 1.0 equipment")
		soldCount := SellRelic1Equipment()
		log.Printf("SoldCount: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 1.0 items", soldCount))
	case "sellRelic2Equipment":
		log.Println("Received request to sell Relic 2.0 equipment")
		var keepStars int
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["keepStars"].(float64); ok {
				keepStars = int(val)
			}
		}
		soldCount := SellRelic2Equipment(keepStars)
		log.Printf("SoldCount Relic 2.0 (Keep %d+ Stars): %v", keepStars, soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 2.0 items", soldCount))

	case "sellRelic1Gems":
		log.Println("Received request to sell Relic 1.0 gems")
		soldCount := SellRelic1Gems()
		log.Printf("SoldCount Relic 1.0 Gems: %v", soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 1.0 gems", soldCount))

	case "sellRelic2Gems":
		log.Println("Received request to sell Relic 2.0 gems")
		var keepStars int
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["keepStars"].(float64); ok {
				keepStars = int(val)
			}
		}
		soldCount := SellRelic2Gems(keepStars)
		log.Printf("SoldCount Relic 2.0 Gems (Keep %d+ Stars): %v", keepStars, soldCount)
		SendAlertMessage("green", fmt.Sprintf("Sold %d Relic 2.0 gems", soldCount))
	case "startGame":
		log.Println("Manual Start Bot pressed. Reloading game tab...")
		Models.GetSettingsState().BotEnabled = true
		Scheduler.GetScheduler().Start()
		ResponseRegistry.ReloadGameTab()
	case "stopGame":
		Models.GetSettingsState().BotEnabled = false
		Scheduler.GetScheduler().Stop()
		ResponseRegistry.DisconnectGameWebSocket()
	case "startBeriWorld":
		GameFunctions.StartBeriWorld()
	case "stopBeriWorld":
		GameFunctions.StopBeriWorld()
	case "fetchAllianceInfo":
		GameFunctions.FetchAllianceInfo()
	case "toggleAutoBird":
		// Check if AutoBird is currently running (actual goroutine state)
		wasRunning := GameFunctions.IsAutoBirdRunning()
		log.Printf("[AutoBird] Toggle requested. Was running: %v", wasRunning)

		// Toggle based on actual running state
		if wasRunning {
			log.Println("[AutoBird] Calling StopAutoBird...")
			GameFunctions.StopAutoBird()
			Models.GetSettingsState().AutoBirdEnabled = false
			SendAutoBirdStatus(false, 0)
		} else {
			Models.GetSettingsState().AutoBirdEnabled = true
			if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {

				// Parse settings payload
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

				// Parse Delay Settings
				if minDelay, ok := payloadRaw["minDelay"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MinDelay = int(minDelay)
				}
				if maxDelay, ok := payloadRaw["maxDelay"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MaxDelay = int(maxDelay)
				}
				if minSend, ok := payloadRaw["minSend"].(float64); ok {
					Models.GetSettingsState().AutoBirdDelay.MinSend = int(minSend)
				}
				log.Println("[AutoBird] Calling StartAutoBird...")
				GameFunctions.StartAutoBird()
				SendAutoBirdStatus(true, 0)
			}
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
			for i := 0; i < 8; i++ {
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
		}
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
				if idx >= 0 && idx < len(Models.CommStatArray) {
					log.Printf("Refreshing single commander at index %d", idx)
					SendFrontendMessage("commStatUpdate", Models.CommStatArray[idx], strconv.Itoa(idx))
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
				if targetIndex >= 0 && targetIndex < len(Models.CommStatArray) {
					currentLoadout = Models.CommStatArray[targetIndex]
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
				for _, eq := range Models.GetGameState().EquipmentStorage {
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
			ResponseRegistry.OutgoingMessages <- ResponseRegistry.OutgoingMessageWithCost{Payload: []byte(`%xt%EmpireEx_21%gli%1%{}%`), Cost: 10000}
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
	case "changeLoginDetails":
		log.Println("Received request to change login details")
		// Delete loginBytes.json
		err := os.Remove("loginBytes.json")
		if err != nil {
			log.Printf("Error deleting loginBytes.json: %v", err)
			SendAlertMessage("red", "Failed to delete login details")
			return
		}
		SendAlertMessage("green", "Login details deleted. Please restart the bot (Start Bot).")
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

		addCastle(gs.MainCastle.Aid, gs.MainCastle.Name, "Main")
		addCastle(gs.Outpost1.Aid, gs.Outpost1.Name, "Outpost")
		addCastle(gs.Outpost2.Aid, gs.Outpost2.Name, "Outpost")
		addCastle(gs.Outpost3.Aid, gs.Outpost3.Name, "Outpost")
		addCastle(gs.IceCastle.Aid, gs.IceCastle.Name, "Ice")
		addCastle(gs.DesertCastle.Aid, gs.DesertCastle.Name, "Desert")
		addCastle(gs.DungeonCastle.Aid, gs.DungeonCastle.Name, "Dungeon")
		addCastle(gs.StormCastle.Aid, gs.StormCastle.Name, "Storm")

		SendFrontendMessage("castleList", castles, "")

	case "getBirdSettings":
		// Persistence moved to frontend.
		// Return current in-memory settings if available (e.g. if bot ran), otherwise empty.
		if Models.GetSettingsState().BirdIgnoreList.Troops == nil {
			Models.GetSettingsState().BirdIgnoreList.Troops = make(map[int]map[int]int)
		}
		SendFrontendMessage("birdSettings", Models.GetSettingsState().BirdIgnoreList.Troops, "")

	case "getRecruitTroopsSettings":
		if Models.GetSettingsState().RecruitTroopsList.Targets == nil {
			Models.GetSettingsState().RecruitTroopsList.Targets = make(map[int]map[int]int)
		}
		SendFrontendMessage("recruitTroopsSettings", Models.GetSettingsState().RecruitTroopsList.Targets, "")

	case "saveBirdSettings":
		// Persistence moved to frontend. This is just for runtime update if needed.
		// Or if user clicks save while bot is running.
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

		// Just update memory
		Models.GetSettingsState().UpdateBirdIgnoreList(newSettings)

		// Also update delay/minSend settings if present
		if minDelay, ok := payloadRaw["minDelay"].(float64); ok {
			Models.GetSettingsState().AutoBirdDelay.MinDelay = int(minDelay)
		}
		if maxDelay, ok := payloadRaw["maxDelay"].(float64); ok {
			Models.GetSettingsState().AutoBirdDelay.MaxDelay = int(maxDelay)
		}
		if minSend, ok := payloadRaw["minSend"].(float64); ok {
			Models.GetSettingsState().AutoBirdDelay.MinSend = int(minSend)
		}

		// Echo back for consistency (optional)
		SendFrontendMessage("birdSettings", newSettings, "")

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

	case "getServerList":
		var serverNames []string
		for name := range ResponseRegistry.ServerURLMap {
			serverNames = append(serverNames, name)
		}
		// Sort for consistent display
		// Simple bubble sort or import sort not needed if we trust frontend to sort?
		// Better to just send raw list and let frontend sort or sort here if we import "sort"
		// Let's import "sort"
		// Wait, I can't easily add import with replace_file_content if imports block is far away without reading file content fully or multiple blocks.
		// I'll just send the list, Frontend can sort it.
		SendFrontendMessage("serverList", serverNames, "")
	case "updateCredentials":
		if payloadRaw, ok := data["payload"].(map[string]interface{}); ok {
			server, _ := payloadRaw["server"].(string)
			if server != "" {
				ResponseRegistry.StoredCredentials.Server = server
			}
		}
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
	}
}

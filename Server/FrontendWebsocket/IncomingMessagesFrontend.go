package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ReconfigureLoadout"
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
		var sellLookItems, saveRift bool
		if payload, ok := data["payload"].(map[string]interface{}); ok {
			if val, ok := payload["sellLookItems"].(bool); ok {
				sellLookItems = val
			}
			if val, ok := payload["saveRift"].(bool); ok {
				saveRift = val
			}
		}

		log.Printf("Flags - Sell Look Items: %v, Save Rift: %v", sellLookItems, saveRift)

		soldCount := SellNonRelicEquipment(saveRift, sellLookItems)
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
			GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
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
				newLoadout := ReconfigureLoadout.ReconfigureCommander(msg.Payload)

				// Marshal to JSON for readable logging
				statCommJSON, err := json.MarshalIndent(newLoadout, "", "  ")
				if err != nil {
					log.Printf("Error marshaling statComm: %v", err)
				} else {
					log.Printf("Received reconfigure commander:\n%s", string(statCommJSON))
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
				currentLoadout := GameWebsocket.GetCastellanStat(targetIndex)

				// Calculate the optimized loadout
				newLoadout := ReconfigureLoadout.ReconfigureCastellan(msg.Payload)

				// Marshal to JSON for readable logging
				statCastJSON, err := json.MarshalIndent(newLoadout, "", "  ")
				if err != nil {
					log.Printf("Error marshaling statCast: %v", err)
				} else {
					log.Printf("Received reconfigure castellan:\n%s", string(statCastJSON))
				}

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
			log.Println("Received confirmReconfigure message from frontend")

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
			log.Println("Step 1: Unequipping target commander")
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
					log.Printf("Unequipping slot %d, equipID: %.0f", slot, equipID)
					GameWebsocket.UnequipEquipmentRaw(equipmentMode, targetIndex, equipID)
					time.Sleep(500 * time.Millisecond)
				}
			}

			// --- Step 2: Clean Gems ---
			// Check if any target gems are socketted in storage equipment
			log.Println("Step 2: Cleaning target gems")
			targetGems := []float64{newLoadout.Gem1, newLoadout.Gem2, newLoadout.Gem3, newLoadout.Gem4}
			targetGemMap := make(map[float64]bool)
			for _, g := range targetGems {
				if g != 0 {
					targetGemMap[g] = true
				}
			}

			if len(targetGemMap) > 0 {
				for _, eq := range Models.EquipmentStorage {
					if eq.GemSlot.Gem != nil {
						gemID := eq.GemSlot.Gem.ID
						if targetGemMap[gemID] {
							log.Printf("Cleaning gem ID: %.0f from equipment ID: %.0f", gemID, eq.ID)
							// Double Jump: Equip -> UnequipGem -> UnequipItem
							slot := int(eq.EquipSlotNumber)
							GameWebsocket.EquipEquipment(equipmentMode, targetIndex, slot, eq.ID)
							time.Sleep(1 * time.Second)
							GameWebsocket.UnequipGemRaw(equipmentMode, targetIndex, eq.ID)
							time.Sleep(500 * time.Millisecond)
							GameWebsocket.UnequipEquipmentRaw(equipmentMode, targetIndex, eq.ID)
							time.Sleep(500 * time.Millisecond)
						}
					}
				}
			}

			// --- Step 3: Equip New Base Pieces & Hero ---
			log.Println("Step 3: Equipping new base pieces and hero")
			newEquips := map[int]float64{
				1: newLoadout.Equip1,
				2: newLoadout.Equip2,
				3: newLoadout.Equip3,
				4: newLoadout.Equip4,
				6: newLoadout.Hero,
			}

			for slot, eid := range newEquips {
				if eid != 0 {
					log.Printf("Equipping slot %d, equipID: %.0f", slot, eid)
					GameWebsocket.EquipEquipment(equipmentMode, targetIndex, slot, eid)
					time.Sleep(800 * time.Millisecond)
				}
			}

			// --- Step 4: Socket Gems ---
			log.Println("Step 4: Socketing new gems")
			gemMapping := map[int]float64{
				1: newLoadout.Gem1,
				2: newLoadout.Gem2,
				3: newLoadout.Gem3,
				4: newLoadout.Gem4,
			}

			for slot, gid := range gemMapping {
				eid := newEquips[slot]
				if gid != 0 && eid != 0 {
					log.Printf("Socketing gem ID: %.0f into equipment ID: %.0f (slot %d)", gid, eid, slot)
					GameWebsocket.EquipGem(equipmentMode, targetIndex, eid, gid)
					time.Sleep(800 * time.Millisecond)
				}
			}

			// Final Sync
			SendAlertMessage("green", "Reconfiguration complete!")
			GameWebsocket.OutgoingMessages <- GameWebsocket.OutgoingMessageWithCost{Payload: []byte(`%xt%EmpireEx_21%gli%1%{}%`), Cost: 10000}
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
			log.Println("Received request to unequip equipment")
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
			log.Println("Fetching fresh equipment data before unequip...")
			GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
			time.Sleep(2 * time.Second)

			log.Printf("[Unequip Debug] Received Request: Mode=%s, Index=%.0f, SelectionsCount=%d", equipmentMode, targetIndex, len(selectionsRaw))

			// Process each selection
			successCount := 0
			failCount := 0
			for _, selRaw := range selectionsRaw {
				sel, ok := selRaw.(map[string]interface{})
				if !ok {
					continue
				}
				slotNumber, _ := sel["slotNumber"].(float64)
				equipmentId, _ := sel["equipmentId"].(float64)

				log.Printf("[Unequip Debug] Processing Item: Slot=%.0f, ID=%.0f", slotNumber, equipmentId)

				if GameWebsocket.UnequipEquipment(equipmentMode, int(targetIndex), int(slotNumber), equipmentId) {
					successCount++
				} else {
					log.Printf("[Unequip Debug] Failed to unequip item: Slot=%.0f, ID=%.0f", slotNumber, equipmentId)
					failCount++
				}
			}

			// Report results
			if failCount == 0 {
				SendAlertMessage("green", fmt.Sprintf("Unequipped %d equipment successfully", successCount))
			} else if successCount == 0 {
				SendAlertMessage("yellow", "Equipment data out of sync - refreshing...")
			} else {
				SendAlertMessage("yellow", fmt.Sprintf("Unequipped %d equipment, %d failed", successCount, failCount))
			}

			// Refresh equipment data from game and update frontend via ParseFrontendMessage
			go func() {
				log.Printf("Sending gli to refresh game data after unequip...")
				GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
				time.Sleep(2 * time.Second)
				// Call ParseFrontendMessage with getCommUpdate to trigger targeted refresh
				refreshMsg := fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
				ParseFrontendMessage([]byte(refreshMsg))
			}()
		}
	case "unequipGem":
		{
			log.Println("Received request to unequip gem")
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
			log.Println("Fetching fresh equipment data before gem unequip...")
			GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
			time.Sleep(2 * time.Second)

			// Process each selection
			successCount := 0
			failCount := 0
			for _, selRaw := range selectionsRaw {
				sel, ok := selRaw.(map[string]interface{})
				if !ok {
					continue
				}
				slotNumber, _ := sel["slotNumber"].(float64)
				gemId, _ := sel["gemId"].(float64)
				equipmentId, _ := sel["equipmentId"].(float64)

				if GameWebsocket.UnequipGem(equipmentMode, int(targetIndex), int(slotNumber), gemId, equipmentId) {
					successCount++
				} else {
					failCount++
				}
			}

			// Report results
			if failCount == 0 {
				SendAlertMessage("green", fmt.Sprintf("Unequipped %d gems successfully", successCount))
			} else if successCount == 0 {
				SendAlertMessage("yellow", "Equipment data out of sync - refreshing...")
			} else {
				SendAlertMessage("yellow", fmt.Sprintf("Unequipped %d gems, %d failed", successCount, failCount))
			}

			// Refresh equipment data from game and update frontend via ParseFrontendMessage
			go func() {
				log.Printf("Sending gli to refresh game data after gem unequip...")
				GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
				time.Sleep(2 * time.Second)
				// Call ParseFrontendMessage with getCommUpdate to trigger targeted refresh
				refreshMsg := fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, int(targetIndex))
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
		SendAlertMessage("green", "Login details deleted. Please restart the bot (Start Game).")
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
	}
}

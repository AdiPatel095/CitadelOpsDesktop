package FrontendWebsocket

import (
	"CitadelDesktop/Server/GameWebsocket"
	"CitadelDesktop/Server/License"
	"CitadelDesktop/Server/LoadoutReconfigure"
	"CitadelDesktop/Server/Models"
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"
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
			// Need custom struct for this payload

			// var payload ReconfigurePayload
			// Extract payload from map to struct or just use map
			// Since data["payload"] is interface{}, we need to marshal/unmarshal or mapstructure
			// Simplest is to re-marshal the payload part or just use the data map if it was flat
			// But data is map[string]interface{}.

			// Faster way: re-marshal just the payload part if possible, or manual cast.
			// Let's rely on JSON unmarshal of the original message into a struct that has struct payload

			// Re-parsing the whole message with a specific struct for this type is safer
			var msg struct {
				Type    string                                `json:"type"`
				Payload LoadoutReconfigure.ReconfigurePayload `json:"payload"`
			}
			if err := json.Unmarshal(message, &msg); err != nil {
				log.Printf("Error parsing reconfigure msg: %v", err)
				SendAlertMessage("red", "Invalid reconfiguration request")
				return
			}

			// Validate credits
			const cost = 10000
			if !License.UseCredits(cost, "reconfigure") {
				SendInsufficientCreditsMessage()
				SendAlertMessage("red", "Insufficient credits for reconfiguration")
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
				newLoadout := LoadoutReconfigure.ReconfigureCommander(msg.Payload)

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
			} else if msg.Payload.CombatMode == "Castellan" {
				//statCast := ReconfigureCastellan(msg.Payload)
				log.Printf("Received reconfigureCastellan: ")
				//return statCast
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
			equipmentMode := "Commander"

			SendAlertMessage("green", "Starting reconfiguration...")

			// --- Step 1: Unequip Current Loadout ---
			log.Println("Step 1: Unequipping old loadout...")
			oldSlots := []int{1, 2, 3, 4, 6}
			for _, slot := range oldSlots {
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
					GameWebsocket.UnequipEquipmentWithListener(equipmentMode, targetIndex, slot, equipID)
				}
			}

			// --- Step 2: Clean Target Gems (Ensure connection gems are free) ---
			log.Println("Step 2: Checking storage for target gems...")
			targetGems := []float64{newLoadout.Gem1, newLoadout.Gem2, newLoadout.Gem3, newLoadout.Gem4}
			targetGemMap := make(map[float64]bool)
			for _, g := range targetGems {
				if g != 0 {
					targetGemMap[g] = true
				}
			}

			// We need to track which gems we successfully freed from storage
			// simply to avoid double processing or incorrect assumptions
			if len(targetGemMap) > 0 {
				for _, eq := range Models.EquipmentStorage {
					if eq.GemSlot.Gem != nil {
						gemID := eq.GemSlot.Gem.ID
						if targetGemMap[gemID] {
							log.Printf("Found target gem %.0f on stored item %.0f - Extracting...", gemID, eq.ID)
							// Equip -> UnequipGem -> UnequipItem
							slot := int(eq.EquipSlotNumber)
							GameWebsocket.EquipEquipmentWithListener(equipmentMode, targetIndex, slot, eq.ID)
							GameWebsocket.UnequipGemRawWithListener(equipmentMode, targetIndex, eq.ID)
							GameWebsocket.UnequipEquipmentRawWithListener(equipmentMode, targetIndex, eq.ID)
						}
					}
				}
			}

			// --- Step 3 & 4: New Base Equipment Analysis ---
			log.Println("Step 3 & 4: Analyzing new base equipment...")

			// Map Slot Index (1-4) to Equipment ID
			newEquipMap := map[int]float64{
				1: newLoadout.Equip1,
				2: newLoadout.Equip2,
				3: newLoadout.Equip3,
				4: newLoadout.Equip4,
			}

			dirtyList := make(map[int]float64) // Slot -> ID
			cleanList := make(map[int]float64) // Slot -> ID

			for slot := 1; slot <= 4; slot++ {
				eid := newEquipMap[slot]
				if eid == 0 {
					continue
				}

				hasGem := false

				// Check if it was in Old Loadout (which we just unequipped in Step 1)
				// The gem would still be on it.
				if current.Equip1 == eid && current.Gem1 != 0 {
					hasGem = true
				} else if current.Equip2 == eid && current.Gem2 != 0 {
					hasGem = true
				} else if current.Equip3 == eid && current.Gem3 != 0 {
					hasGem = true
				} else if current.Equip4 == eid && current.Gem4 != 0 {
					hasGem = true
				} else {
					// Check Storage
					// If it wasn't equipped, it must be in storage
					for _, eq := range Models.EquipmentStorage {
						if eq.ID == eid {
							if eq.GemSlot.Gem != nil {
								// Special check: IF the gem on this item matches a Target Gem (Step 2),
								// we technically already removed it in Step 2.
								// However, Models.EquipmentStorage is STALE.
								// So we must trust that if Step 2 ran, that gem is gone.
								// But what if it has a Different Gem? Then it's still dirty.

								// If the gem ID on this storage item is one of our target gems,
								// we know we just cleaned it. So it's effectively clean.
								if targetGemMap[eq.GemSlot.Gem.ID] {
									hasGem = false
								} else {
									hasGem = true
								}
							}
							break
						}
					}
				}

				if hasGem {
					dirtyList[slot] = eid
				} else {
					cleanList[slot] = eid
				}
			}

			// --- Step 5: Process Dirty Items ---
			if len(dirtyList) > 0 {
				log.Println("Step 5: Cleaning dirty base items...")
				for slot, eid := range dirtyList {
					log.Printf("Equipping & Cleaning dirty item %.0f in slot %d", eid, slot)
					GameWebsocket.EquipEquipmentWithListener(equipmentMode, targetIndex, slot, eid)
					GameWebsocket.UnequipGemRawWithListener(equipmentMode, targetIndex, eid)
					// Now it's equipped and empty.
				}
			}

			// --- Step 6: Process Clean Items ---
			if len(cleanList) > 0 {
				log.Println("Step 6: Equipping clean base items...")
				for slot, eid := range cleanList {
					GameWebsocket.EquipEquipmentWithListener(equipmentMode, targetIndex, slot, eid)
				}
			}
			// At this point, all 4 base items are Equipped and Empty.

			// --- Step 7: Equip New Gems ---
			log.Println("Step 7: Equipping new gems...")

			// Gem1 -> Slot 1 (Equip1)
			// Gem2 -> Slot 2 (Equip2)
			// ...
			gemMap := map[int]float64{
				1: newLoadout.Gem1,
				2: newLoadout.Gem2,
				3: newLoadout.Gem3,
				4: newLoadout.Gem4,
			}

			for slot := 1; slot <= 4; slot++ {
				gemID := gemMap[slot]
				equipID := newEquipMap[slot]

				if gemID != 0 && equipID != 0 {
					GameWebsocket.EquipGemWithListener(equipmentMode, targetIndex, equipID, gemID)
				}
			}

			// --- Step 8: Equip Hero ---
			log.Println("Step 8: Equipping hero...")
			if newLoadout.Hero != 0 {
				GameWebsocket.EquipEquipmentWithListener(equipmentMode, targetIndex, 6, newLoadout.Hero)
			}

			// --- Step 9: Refresh ---
			log.Println("Step 9: Refreshing...")
			SendAlertMessage("green", "Reconfiguration complete!")

			GameWebsocket.OutgoingMessages <- []byte(`%xt%EmpireEx_21%gli%1%{}%`)
			time.Sleep(2 * time.Second)
			refreshMsg := fmt.Sprintf(`{"type":"getCommUpdate","commanderIndex":%d}`, targetIndex)
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

				if GameWebsocket.UnequipEquipmentWithListener(equipmentMode, int(targetIndex), int(slotNumber), equipmentId) {
					successCount++
				} else {
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

				if GameWebsocket.UnequipGemWithListener(equipmentMode, int(targetIndex), int(slotNumber), gemId, equipmentId) {
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
	}
}

package GameWebsocket

import (
	"CitadelDesktop/Server/License"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"math/rand"
	"sync"
	"time"
)

var (
	autoBirdCancel context.CancelFunc
	autoBirdMu     sync.Mutex
)

// StartAutoBird starts the auto bird goroutine. If already running, it does nothing.
func StartAutoBird() {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()

	// If already running, don't start another
	if autoBirdCancel != nil {
		return
	}

	// Load logic removed - settings passed from frontend on toggle
	// Models.LoadBirdIgnoreList()

	ctx, cancel := context.WithCancel(context.Background())
	autoBirdCancel = cancel

	go runAutoBird(ctx)
}

// StopAutoBird stops the auto bird goroutine if running.
func StopAutoBird() {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()

	if autoBirdCancel != nil {
		autoBirdCancel()
		autoBirdCancel = nil
		// Clear bird ignore list from memory
		Models.ClearBirdIgnoreList()
		if SendAutoBirdStatusFunc != nil {
			go SendAutoBirdStatusFunc(false, 0)
		}
	}
}

// runAutoBird is the main initialization routine when auto bird is enabled.
// It runs in a loop, sending birds and then sleeping until they need to return.
// runAutoBird is the main initialization routine when auto bird is enabled.
// It runs in a loop, sending birds and then sleeping until they need to return.
func runAutoBird(ctx context.Context) {
	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sleepDuration := 15 * time.Minute

		// Check if connected, if not start game
		if !LoginStatus {
			StartGame()

			// Wait for login
			loginTimeout := time.After(45 * time.Second)
			loginSuccess := false

			for {
				select {
				case <-ctx.Done():
					return
				case <-loginTimeout:
					break
				case <-time.After(1 * time.Second):
					if LoginStatus {
						loginSuccess = true
						break
					}
				}
				if loginSuccess {
					break
				}
			}

			if !loginSuccess {
				// Failed to login, retry after a delay
				select {
				case <-ctx.Done():
					return
				case <-time.After(30 * time.Second):
					continue // Retry loop
				}
			}

			// Give it a moment to stabilize
			time.Sleep(2 * time.Second)
		}

		// Clear previous cycle data
		gs.BirdMovements = make(map[int][]Models.BirdMovement)

		log.Println("[AutoBird] Starting processing cycle...")

		// Step 1: Fetch fresh alliance info (bird locations)
		FetchAllianceInfo()

		// Wait a bit for the response to be processed
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		// Step 2: Get player's own castle locations from castle info
		playerCastles := getPlayerCastleLocations()

		// Step 3: Process each castle sequentially
		// Fetch troops -> Plan -> Send -> Wait -> Next

		// Calculate random delay once per cycle (Configurable)
		minDelay := Models.AutoBirdDelay.MinDelay
		maxDelay := Models.AutoBirdDelay.MaxDelay
		if maxDelay < minDelay {
			maxDelay = minDelay
		}
		delayRange := maxDelay - minDelay + 1
		if delayRange < 1 {
			delayRange = 1
		}
		randomDelay := rand.Intn(delayRange) + minDelay
		ptt := gs.GlobalResources.PTT

		for _, castleLoc := range playerCastles {
			select {
			case <-ctx.Done():
				return
			default:
			}

			// 3.1 Fetch Troops
			troops := fetchCastleTroops(castleLoc.KingdomID, castleLoc.CastleID, castleLoc.X, castleLoc.Y)
			if troops == nil {
				continue
			}

			// Delay after JAA command
			time.Sleep(1 * time.Second)

			// 3.2 Calculate All Troops to Send first
			var allTroopsToSend [][]int
			ignoredSummary := ""

			for id, amount := range troops.Troops {
				saveAmount := Models.BirdIgnoreList.GetSaveAmount(castleLoc.CastleID, id)
				sendAmount := amount - saveAmount

				if sendAmount > 0 {
					allTroopsToSend = append(allTroopsToSend, []int{id, sendAmount})
					// Kept amount is saveAmount
					if saveAmount > 0 {
						ignoredSummary += fmt.Sprintf("ID:%d x%d, ", id, saveAmount)
					}
				} else {
					// Kept amount is total amount (none sent)
					if amount > 0 {
						ignoredSummary += fmt.Sprintf("ID:%d x%d, ", id, amount)
					}
				}
			}
			if len(ignoredSummary) > 2 {
				ignoredSummary = ignoredSummary[:len(ignoredSummary)-2]
			}

			if len(allTroopsToSend) == 0 {
				continue
			}

			// 3.3 Process in batches of 10
			batchSize := 10
			for i := 0; i < len(allTroopsToSend); i += batchSize {
				end := i + batchSize
				if end > len(allTroopsToSend) {
					end = len(allTroopsToSend)
				}
				batch := allTroopsToSend[i:end]

				// Small delay between batches if not the first one
				if i > 0 {
					time.Sleep(2 * time.Second)
				}

				// Find Target (refresh each batch to be safe, though likely same)
				effectiveKID := troops.KingdomID
				target := findClosestBirdLocation(*troops, gs.Alliance.BirdLocations)

				// If no target found and it's Ice/Desert, try with swapped KID for bird matching
				if target == nil && (effectiveKID == 1 || effectiveKID == 2) {
					swappedKID := 1
					if effectiveKID == 1 {
						swappedKID = 2
					}
					// Create a temp troops struct with swapped KID for bird matching
					tempTroops := *troops
					tempTroops.KingdomID = swappedKID
					target = findClosestBirdLocation(tempTroops, gs.Alliance.BirdLocations)
				}

				if target == nil {
					continue
				}

				// Send SDI (Bird Intent) for this batch
				sdiPayload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sdi%%1%%{"TX":%d,"TY":%d,"SX":%d,"SY":%d}%%`,
					target.X, target.Y, castleLoc.X, castleLoc.Y)
				OutgoingMessages <- []byte(sdiPayload)

				// Delay after SDI command
				time.Sleep(1 * time.Second)

				// Prepare CDS
				pttValue := 0
				if ptt > 1000 {
					pttValue = 1
				}

				hbwValue := -1
				if pttValue == 0 {
					hbwValue = 1007
				}

				// Check credits before sending
				if !License.HasCredits(1) {
					StopAutoBird()
					return
				}

				troopsJSON, _ := json.Marshal(batch)

				// Helper to send CDS and wait for response
				sendCDS := func(targetX, targetY int) ([]string, error) {
					cdsPayload := fmt.Sprintf(`%%xt%%EmpireEx_21%%cds%%1%%{"SID":%d,"TX":%d,"TY":%d,"LID":-14,"WT":%d,"HBW":%d,"BPC":1,"PTT":%d,"SD":0,"A":%s}%%`,
						castleLoc.CastleID, targetX, targetY, randomDelay, hbwValue, pttValue, string(troopsJSON))

					waiter := ResponseRegistry.Global.RegisterWaiter("cds", 15*time.Second)
					OutgoingMessages <- []byte(cdsPayload)

					response, err := waiter.WaitWithTimeout()
					waiter.Cleanup()
					return response, err
				}

				// Try sending CDS with current target
				response, err := sendCDS(target.X, target.Y)

				// If CDS failed and it's Ice/Desert, try with swapped KID's bird target
				if err != nil && (effectiveKID == 1 || effectiveKID == 2) {
					swappedKID := 1
					if effectiveKID == 1 {
						swappedKID = 2
					}

					// Find new target with swapped KID
					tempTroops := *troops
					tempTroops.KingdomID = swappedKID
					newTarget := findClosestBirdLocation(tempTroops, gs.Alliance.BirdLocations)

					if newTarget != nil {
						// Send new SDI for the new target
						sdiPayload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sdi%%1%%{"TX":%d,"TY":%d,"SX":%d,"SY":%d}%%`,
							newTarget.X, newTarget.Y, castleLoc.X, castleLoc.Y)
						OutgoingMessages <- []byte(sdiPayload)
						time.Sleep(1 * time.Second)

						// Retry CDS with new target
						response, err = sendCDS(newTarget.X, newTarget.Y)
					}
				}

				if err != nil {
					continue
				}

				// Process Response
				if len(response) > 5 {
					var responseData map[string]interface{}
					if err := json.Unmarshal([]byte(response[5]), &responseData); err == nil {
						if aObj, ok := responseData["A"].(map[string]interface{}); ok {
							if mObj, ok := aObj["M"].(map[string]interface{}); ok {
								mid, _ := mObj["MID"].(float64)
								tt, _ := mObj["TT"].(float64)

								if mid != 0 && tt != 0 {
									// Deduct credit for successful send
									License.UseCredits(1, "Auto Bird")

									totalDurationSeconds := (int(tt) * 2) + (randomDelay * 3600)
									returnTime := time.Now().Add(time.Duration(totalDurationSeconds) * time.Second)

									movement := Models.BirdMovement{
										MovementID:        int(mid),
										CastleID:          castleLoc.CastleID,
										ReturnTime:        returnTime,
										OneWayTimeSeconds: int(tt),
										DelayHrs:          randomDelay,
									}

									if gs.BirdMovements == nil {
										gs.BirdMovements = make(map[int][]Models.BirdMovement)
									}
									gs.BirdMovements[castleLoc.CastleID] = append(gs.BirdMovements[castleLoc.CastleID], movement)

									// Log Success
									castleName := getCastleName(castleLoc.CastleID)
									log.Printf("[AutoBird] Successfully sent batch from %s. Data Left/Ignored: %s", castleName, ignoredSummary)

								}
							}
						}
					}
				}
			}

			// Small delay before next castle
			time.Sleep(1 * time.Second)
		}

		// Calculate sleep time based on max return time
		var maxReturnTime time.Time
		for _, castleMovements := range gs.BirdMovements {
			for _, m := range castleMovements {
				if m.ReturnTime.After(maxReturnTime) {
					maxReturnTime = m.ReturnTime
				}
			}
		}

		if !maxReturnTime.IsZero() {
			// Sleep until last bird returns + 1 minute buffer
			maxReturnTime = maxReturnTime.Add(1 * time.Minute)
			sleepDuration = time.Until(maxReturnTime)
			if sleepDuration < 0 {
				sleepDuration = 1 * time.Minute // Already passed, just wait a bit
			}
		}

		// Notify frontend of sleep
		if SendAutoBirdStatusFunc != nil {
			wakeUpTime := time.Now().Add(sleepDuration).UnixMilli()
			go SendAutoBirdStatusFunc(true, wakeUpTime)
		}

		// Disconnect from game to save resources/remain stealthy
		StopGame()

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			// Waking up for next cycle
			if SendAutoBirdStatusFunc != nil {
				go SendAutoBirdStatusFunc(true, 0)
			}
		}
	}
}

// findClosestBirdLocation returns the closest bird location in the same kingdom
func findClosestBirdLocation(source Models.CastleTroops, targets []Models.BirdLocation) *Models.BirdLocation {
	var closest *Models.BirdLocation
	minDist := 1000000.0 // Large initial value

	for i := range targets {
		target := &targets[i]

		// Must be in same kingdom
		if target.KingdomID != source.KingdomID {
			continue
		}

		dist := calculateDistance(source.X, source.Y, target.X, target.Y)
		if dist < minDist {
			minDist = dist
			closest = target
		}
	}
	return closest
}

// calculateDistance returns Euclidean distance between two points
func calculateDistance(x1, y1, x2, y2 int) float64 {
	dx := float64(x2 - x1)
	dy := float64(y2 - y1)
	return math.Sqrt(dx*dx + dy*dy)
}

// CastleLocation represents a simple castle location
type CastleLocation struct {
	KingdomID int
	CastleID  int
	X         int
	Y         int
}

// getPlayerCastleLocations returns the locations of all player castles from GameState
func getPlayerCastleLocations() []CastleLocation {
	gs := Models.GetGameState()
	var locations []CastleLocation

	// Use the PlayerCastleLocations parsed from the gal/alliance data
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		locations = append(locations, CastleLocation{
			KingdomID: loc.KingdomID,
			CastleID:  loc.CastleID,
			X:         loc.X,
			Y:         loc.Y,
		})
	}

	return locations
}

// getCastleName returns the name of the castle with the given ID
func getCastleName(castleID int) string {
	gs := Models.GetGameState()
	if int(gs.MainCastle.Aid) == castleID {
		return gs.MainCastle.Name
	}
	if int(gs.Outpost1.Aid) == castleID {
		return gs.Outpost1.Name
	}
	if int(gs.Outpost2.Aid) == castleID {
		return gs.Outpost2.Name
	}
	if int(gs.Outpost3.Aid) == castleID {
		return gs.Outpost3.Name
	}
	if int(gs.IceCastle.Aid) == castleID {
		return gs.IceCastle.Name
	}
	if int(gs.DesertCastle.Aid) == castleID {
		return gs.DesertCastle.Name
	}
	if int(gs.DungeonCastle.Aid) == castleID {
		return gs.DungeonCastle.Name
	}
	if int(gs.StormCastle.Aid) == castleID {
		return gs.StormCastle.Name
	}
	return fmt.Sprintf("Castle %d", castleID)
}

// fetchCastleTroops sends JAA command and waits for response to get troop counts
func fetchCastleTroops(kingdomID, castleID, x, y int) *Models.CastleTroops {
	// Try with original kingdomID first
	result := sendTroopRequest(kingdomID, castleID, x, y)

	// If failed and it's Ice (2) or Desert (1), try with swapped KID
	if result == nil && (kingdomID == 1 || kingdomID == 2) {
		swappedKID := 1
		if kingdomID == 1 {
			swappedKID = 2
		}
		result = sendTroopRequest(swappedKID, castleID, x, y)
	}

	return result
}

// sendTroopRequest sends a single JAA/JCA request and returns the parsed result
func sendTroopRequest(kingdomID, castleID, x, y int) *Models.CastleTroops {
	// Send JAA/JCA command
	// Kingdom 0 (Main) and 4 (Storm) use JAA with Coordinates
	// Kingdom 1,2,3 use JCA with CastleID
	var payload string

	if kingdomID == 0 || kingdomID == 4 {
		payload = fmt.Sprintf(`%%xt%%EmpireEx_21%%jaa%%1%%{"PX":%d,"PY":%d,"KID":%d}%%`, x, y, kingdomID)
	} else {
		payload = fmt.Sprintf(`%%xt%%EmpireEx_21%%jca%%1%%{"CID":%d,"KID":%d}%%`, castleID, kingdomID)
	}

	// Log JCA requests for Ice (2) and Desert (1)
	if kingdomID == 1 || kingdomID == 2 {
		// Log removed
	}

	// Create a waiter for the response using the ResponseRegistry pattern
	waiter := ResponseRegistry.Global.RegisterWaiter("jaa", 5*time.Second)
	defer waiter.Cleanup()

	// Send the command
	OutgoingMessages <- []byte(payload)

	// Wait for response with timeout
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return nil
	}

	if len(response) > 5 {
		// Log JCA response for Ice (2) and Desert (1)
		if kingdomID == 1 || kingdomID == 2 {
			// Log removed
		}
		return parseTroopsFromJAA(response[5], kingdomID, castleID, x, y)
	}

	return nil
}

// parseTroopsFromJAA extracts troop data from JAA response
func parseTroopsFromJAA(data string, kingdomID, castleID, x, y int) *Models.CastleTroops {
	var jaaData map[string]interface{}
	err := json.Unmarshal([]byte(data), &jaaData)
	if err != nil {
		return nil
	}

	// Get gui.I array
	gui, ok := jaaData["gui"].(map[string]interface{})
	if !ok {
		return nil
	}

	iArray, ok := gui["I"].([]interface{})
	if !ok {
		return nil
	}

	// Log the specific troop map/array as requested
	// iArrayJSON, _ := json.Marshal(iArray)
	// log.Printf("[AutoBird][Debug] Parsed Troop Array (gui.I) for Castle %d (K%d): %s", castleID, kingdomID, string(iArrayJSON))

	troops := make(map[int]int)

	// Parse each [unitID, count] pair
	for _, item := range iArray {
		pair, ok := item.([]interface{})
		if !ok || len(pair) < 2 {
			continue
		}

		unitID, ok1 := pair[0].(float64)
		count, ok2 := pair[1].(float64)
		if !ok1 || !ok2 {
			continue
		}

		// Only include if it's a troop (not a tool)
		if Models.IsTroop(int(unitID)) {
			troops[int(unitID)] = int(count)
		}
	}

	return &Models.CastleTroops{
		KingdomID: kingdomID,
		CastleID:  castleID,
		X:         x,
		Y:         y,
		Troops:    troops,
	}
}

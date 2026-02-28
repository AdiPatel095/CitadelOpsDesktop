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
	autoBirdCancel     context.CancelFunc
	autoBirdMu         sync.Mutex
	autoBirdNextWakeUp int64 // Unix milliseconds, 0 if not sleeping
)

// IsAutoBirdRunning returns true if the AutoBird goroutine is currently active
func IsAutoBirdRunning() bool {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	return autoBirdCancel != nil
}

// GetAutoBirdNextWakeUp returns the next wake-up time in Unix milliseconds (0 if not sleeping)
func GetAutoBirdNextWakeUp() int64 {
	autoBirdMu.Lock()
	defer autoBirdMu.Unlock()
	return autoBirdNextWakeUp
}

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
		autoBirdNextWakeUp = 0
		// Clear bird ignore list from memory
		Models.ClearBirdIgnoreList()
		if SendAutoBirdStatusFunc != nil {
			go SendAutoBirdStatusFunc(false, 0)
		}
	}
}

// runAutoBird is the main initialization routine when auto bird is enabled.
// It runs in a loop, sending birds and then sleeping until they need to return.
func runAutoBird(ctx context.Context) {
	gs := Models.GetGameState()
	// isFirstCycle removed - reconciliation runs every cycle

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sleepDuration := 15 * time.Minute

		// ---------------------------------------------------------
		// STEP 0: Startup Reconciliation (First Cycle Only)
		// ---------------------------------------------------------
		// ---------------------------------------------------------
		// STEP 0: Reconciliation (Every Cycle)
		// ---------------------------------------------------------
		// Run every cycle to clean up expired birds and check GAM
		reconcileDuration, readyToSend := reconcileOnStartup(ctx)
		if !readyToSend {
			// We need to wait for existing birds
			// Notify frontend of sleep
			wakeUpTime := time.Now().Add(reconcileDuration).UnixMilli()

			autoBirdMu.Lock()
			autoBirdNextWakeUp = wakeUpTime
			autoBirdMu.Unlock()

			if SendAutoBirdStatusFunc != nil {
				go SendAutoBirdStatusFunc(true, wakeUpTime)
			}

			// Game stays connected while sleeping for coplay

			log.Printf("[AutoBird] Reconciliation complete. Sleeping for %v until birds return/expire.", reconcileDuration)

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconcileDuration):
				// Waking up
				autoBirdMu.Lock()
				autoBirdNextWakeUp = 0
				autoBirdMu.Unlock()
				if SendAutoBirdStatusFunc != nil {
					go SendAutoBirdStatusFunc(true, 0)
				}
				// Continue to next loop iteration (which will re-run reconciliation/login)
				continue
			}
		}
		// Ready to send immediately

		// ---------------------------------------------------------
		// STEP 1: Login and Prepare
		// ---------------------------------------------------------
		if !LoginStatus {
			log.Println("[AutoBird] Disconnected. Reloading game tab to reconnect...")
			ReloadGameTab()
			// Wait for login to complete after reload
		LoginWaitLoop:
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					if LoginStatus {
						break LoginWaitLoop
					}
				}
			}
		}

		// Clear previous cycle data
		gs.BirdMovements = make(map[int][]Models.BirdMovement)

		log.Println("[AutoBird] Starting processing cycle...")

		// Load existing saved birds to know which castles already have active birds
		existingBirds := Models.LoadSentBirds()
		activeCastleMap := make(map[int]bool)
		if existingBirds != nil {
			for _, bird := range existingBirds.Birds {
				activeCastleMap[bird.CastleID] = true
			}
			log.Printf("[AutoBird] Loaded %d existing active birds. Castles with active birds: %v", len(existingBirds.Birds), activeCastleMap)
		}

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

		// Helper to find player OID (use first castle's aid if available, though OID usually better from login)
		// We'll rely on OID from login if we had it, but for now we can infer or pass 0 if unknown
		// (though persistence asks for it). Let's try to find OID from game state if possible.
		// For now, we will use a placeholder or derived ID since the persistence mainly uses it for validation.
		// Actually, let's use the first castle ID as a proxy for player ID if we don't have login OID stored globally.
		playerOID := 0
		if len(playerCastles) > 0 {
			// Try to find OID from main castle if possible, or just use 0.
			// The SentBirdTracker uses it to reset if ID changes.
			// We can use MainCastle.Aid
			playerOID = int(gs.MainCastle.Aid)
		}

		// Step 3: Process each castle sequentially
		log.Printf("[AutoBird] Processing %d player castles...", len(playerCastles))

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

			// 3.1 Fetch Troops first (to check ratio)
			troops := fetchCastleTroops(castleLoc.KingdomID, castleLoc.CastleID, castleLoc.X, castleLoc.Y)
			if troops == nil {
				log.Printf("[AutoBird] Failed to fetch troops for Castle %d (K%d). Skipping.", castleLoc.CastleID, castleLoc.KingdomID)
				continue
			}

			// Delay after JAA command
			time.Sleep(1 * time.Second)

			// 3.2 Check Surplus Ratio
			// User Logic: "ratio of troops need to be sent / ignore total >= 0.1"
			// Meaning: We need at least 10% of the KEEP amount in SURPLUS to send.
			totalToSend := 0
			totalKeep := 0

			for id, amount := range troops.Troops {
				saveAmountConfigured, configured := Models.BirdIgnoreList.GetSaveAmount(castleLoc.CastleID, id)
				actualSaveAmount := 0
				sendAmount := 0

				if configured {
					if saveAmountConfigured == 0 {
						actualSaveAmount = amount
						sendAmount = 0
					} else {
						if amount > saveAmountConfigured {
							actualSaveAmount = saveAmountConfigured
							sendAmount = amount - saveAmountConfigured
						} else {
							actualSaveAmount = amount
							sendAmount = 0
						}
					}
				} else {
					actualSaveAmount = 0
					sendAmount = amount
				}

				// Calculate retention total
				if actualSaveAmount > 0 {
					totalKeep += actualSaveAmount
				}

				// Calculate surplus total (only valid troops)
				if sendAmount > 0 {
					totalToSend += sendAmount
				}
			}

			// Check if we should send
			shouldSend := false
			ratio := 0.0

			if totalKeep == 0 {
				// No retention set, send everything if we have troops
				if totalToSend > 0 { // totalToSend equals totalCurrent in this case
					shouldSend = true
				}
			} else {
				ratio = float64(totalToSend) / float64(totalKeep)
				// Need 10% surplus relative to keep amount => Ratio >= 0.10
				if ratio >= 0.10 {
					shouldSend = true
				}
			}

			if activeCastleMap[castleLoc.CastleID] {
				if shouldSend {
					// Proceed to MinSend check
				} else {
					continue
				}
			} else {
				// No active bird
				if !shouldSend {
					// Apply same ratio logic to prevent micro-sends
					if totalToSend > 0 && totalKeep > 0 && ratio < 0.10 {
						continue
					}
				}
			}

			if totalToSend > 0 && totalToSend < Models.AutoBirdDelay.MinSend {
				log.Printf("[AutoBird] Castle %d has troops to send (%d), but is below Global Minimum to Send (%d). Skipping.",
					castleLoc.CastleID, totalToSend, Models.AutoBirdDelay.MinSend)
				continue
			}

			// Move the confirmation log here, after passing all checks
			if activeCastleMap[castleLoc.CastleID] {
				log.Printf("[AutoBird] Castle %d has active bird, but found significant surplus and passed MinSend (%d > %d). Sending new bird.",
					castleLoc.CastleID, totalToSend, Models.AutoBirdDelay.MinSend)
			}

			// 3.3 Process in batches (logic below uses same calc so we proceed)
			// But check if we have anything to send first
			if totalToSend == 0 {
				continue
			}

			// 3.3 Calculate All Troops to Send
			var allTroopsToSend [][]int
			ignoredSummary := ""

			for id, amount := range troops.Troops {
				saveAmountConfigured, configured := Models.BirdIgnoreList.GetSaveAmount(castleLoc.CastleID, id)
				actualSaveAmount := 0
				sendAmount := 0

				if configured {
					if saveAmountConfigured == 0 {
						actualSaveAmount = amount
						sendAmount = 0
					} else {
						if amount > saveAmountConfigured {
							actualSaveAmount = saveAmountConfigured
							sendAmount = amount - saveAmountConfigured
						} else {
							actualSaveAmount = amount
							sendAmount = 0
						}
					}
				} else {
					actualSaveAmount = 0
					sendAmount = amount
				}

				if sendAmount > 0 {
					allTroopsToSend = append(allTroopsToSend, []int{id, sendAmount})
					// Kept amount is actualSaveAmount
					if actualSaveAmount > 0 {
						ignoredSummary += fmt.Sprintf("ID:%d x%d, ", id, actualSaveAmount)
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

				// If no target found, and it's Ice/Desert, try with swapped KID for bird matching
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

				// If CDS failed, and it's Ice/Desert, try with swapped KID's bird target
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

									// Persist the sent bird with troop composition
									Models.AppendSentBird(playerOID, Models.SentBird{
										CastleID:          castleLoc.CastleID,
										TargetX:           target.X,
										TargetY:           target.Y,
										KingdomID:         effectiveKID,
										TroopComposition:  batch,
										SentTime:          time.Now(),
										ExpectedExpiry:    returnTime,
										OneWayTimeSeconds: int(tt),
										DelayHrs:          randomDelay,
									})

								}
							}
						}
					}
				}
			}

			// Small delay before next castle
			time.Sleep(1 * time.Second)
		}

		// Calculate sleep time based on EARLIEST return time from ALL saved birds
		// This ensures we wake up when the first bird returns
		allBirds := Models.LoadSentBirds()
		var earliestReturnTime time.Time

		if allBirds != nil && len(allBirds.Birds) > 0 {
			for _, bird := range allBirds.Birds {
				if earliestReturnTime.IsZero() || bird.ExpectedExpiry.Before(earliestReturnTime) {
					earliestReturnTime = bird.ExpectedExpiry
				}
			}
			log.Printf("[AutoBird] Found %d active birds. Earliest return: %v", len(allBirds.Birds), earliestReturnTime)
		}

		if !earliestReturnTime.IsZero() {
			// Sleep until first bird returns + 15 minute buffer
			// User Request: "add a 15min delay to the next login to allow for a full batch return"
			sleepDuration = time.Until(earliestReturnTime) + 15*time.Minute
			if sleepDuration < 0 {
				sleepDuration = 1 * time.Minute // Already passed, just wait a bit
			}
		}

		// Notify frontend of sleep and store for persistence
		if SendAutoBirdStatusFunc != nil {
			wakeUpTime := time.Now().Add(sleepDuration).UnixMilli()
			autoBirdMu.Lock()
			autoBirdNextWakeUp = wakeUpTime
			autoBirdMu.Unlock()
			go SendAutoBirdStatusFunc(true, wakeUpTime)
		}

		// Game stays connected while sleeping for coplay

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			// Waking up for next cycle
			autoBirdMu.Lock()
			autoBirdNextWakeUp = 0
			autoBirdMu.Unlock()
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

		// Filter by castle type based on kingdom
		isValidType := false
		if source.KingdomID == 0 {
			// Green kingdom: main castle (1) or outpost (4)
			if target.CastleType == 1 || target.CastleType == 4 {
				isValidType = true
			}
		} else {
			// Other kingdoms: only KW castle (12)
			if target.CastleType == 12 {
				isValidType = true
			}
		}

		if !isValidType {
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

	// If failed, and it's Ice (2) or Desert (1), try with swapped KID
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

// reconcileOnStartup checks persisted birds and reconciles with live GAM data
// Returns duration to sleep if birds are still out, or 0 if ready to send.
// Returns bool readyToSend: true if we can start sending, false if we should sleep
func reconcileOnStartup(ctx context.Context) (time.Duration, bool) {
	// 1. Load sentBirds.json
	savedBirds := Models.LoadSentBirds()
	if savedBirds == nil || len(savedBirds.Birds) == 0 {
		return 0, true // No saved birds, proceed to send
	}

	// 2. Check if all birds have definitely expired based on ExpectedExpiry
	// Add a small buffer to be safe
	allExpired := true
	now := time.Now()
	for _, bird := range savedBirds.Birds {
		if now.Before(bird.ExpectedExpiry) {
			allExpired = false
			break
		}
	}

	if allExpired {
		log.Println("[AutoBird] All saved birds have expired locally. Ready to send new batch.")
		Models.ClearSentBirds()
		return 0, true
	}

	log.Println("[AutoBird] Found active saved birds. Reconciling with game server...")

	// 3. Login to game and fetch GAM
	if !LoginStatus {
		log.Println("[AutoBird] Disconnected during reconciliation. Reloading game tab to reconnect...")
		ReloadGameTab()

		// Wait for login after reload
		for {
			select {
			case <-ctx.Done():
				return 0, false
			case <-time.After(5 * time.Second):
				if LoginStatus {
					return 0, false // Break out cleanly to re-run reconciliation when connected
				}
			}
		}
	}

	// 4. Request GAM message
	// First, clear existing movements because parser now appends to this list
	// This allows us to accumulate multiple GAM messages if they come in sequence
	gs := Models.GetGameState()
	gs.ActiveMovements = nil

	FetchMovements()

	// 5. Wait for GAM response
	// We wait a few seconds for the websocket to receive and parse the GAM message
	select {
	case <-ctx.Done():
		return 0, false
	case <-time.After(5 * time.Second):
	}

	gs = Models.GetGameState()

	// 6. First, filter out already-expired birds from the saved list
	log.Printf("[AutoBird] Checking %d saved birds for expiration...", len(savedBirds.Birds))
	var stillPotentiallyActive []Models.SentBird
	for _, bird := range savedBirds.Birds {
		if now.Before(bird.ExpectedExpiry) {
			stillPotentiallyActive = append(stillPotentiallyActive, bird)
		} else {
			log.Printf("[AutoBird] Bird from castle %d has expired (expected return: %v)", bird.CastleID, bird.ExpectedExpiry)
		}
	}

	log.Printf("[AutoBird] %d birds still potentially active (not expired)", len(stillPotentiallyActive))

	if len(stillPotentiallyActive) == 0 {
		log.Println("[AutoBird] All saved birds have expired. Clearing file and ready to send.")
		Models.ClearSentBirds()
		return 0, true
	}

	// 7. Match remaining birds against GAM movements by troop composition
	log.Printf("[AutoBird] Reconciling %d birds against %d active movements", len(stillPotentiallyActive), len(gs.ActiveMovements))

	var matchedBirds []Models.SentBird
	var maxReturnTime time.Time

	for _, bird := range stillPotentiallyActive {
		// Look for a movement that matches by troop composition
		matched := false

		for _, movement := range gs.ActiveMovements {
			// Check if troop composition matches exactly
			if troopsMatch(bird.TroopComposition, movement.TroopArray) {
				// Found a match - keep this bird
				matched = true
				matchedBirds = append(matchedBirds, bird)

				// Track max return time
				if bird.ExpectedExpiry.After(maxReturnTime) {
					maxReturnTime = bird.ExpectedExpiry
				}
				break
			}
		}

		if !matched {
			// Not matched in GAM.
			// IMPORTANT: If high load, parser might miss it.
			// We only discard if it's explicitly expired (which we filtered above).
			// Since we already filtered expired birds into 'stillPotentiallyActive',
			// all birds here are NOT expired. So we KEEP them to be safe.

			// User Request: Keep log of birds not found with KID, Name, Troop Composition
			castleName := getCastleName(bird.CastleID)
			log.Printf("[AutoBird] ✗ No GAM match for bird from %s (KID: %d). Keeping safe. Troops: %v",
				castleName, bird.KingdomID, bird.TroopComposition)

			matchedBirds = append(matchedBirds, bird)

			// Track max return time
			if bird.ExpectedExpiry.After(maxReturnTime) {
				maxReturnTime = bird.ExpectedExpiry
			}
		}
	}

	// 8. Update saved file with the kept birds (matched + unexpired unmatched)
	if len(matchedBirds) > 0 {
		log.Printf("[AutoBird] Updating saved birds file with %d active birds", len(matchedBirds))
		Models.SaveSentBirds(savedBirds.PlayerID, matchedBirds)
		// Removed per-castle log loop to reduce spam
	} else {
		// No birds matched - all have returned or were recalled
		log.Println("[AutoBird] No saved birds found in active movements. Clearing file.")
		Models.ClearSentBirds()
	}

	// Always proceed to send loop - it will skip castles with active birds
	// and send to any castles without birds, then calculate proper sleep time
	log.Println("[AutoBird] Reconciliation complete. Proceeding to check for castles needing birds...")
	return 0, true
}

// troopsMatch checks if two troop compositions are the same or very similar
// Returns true if compositions match (same troop IDs and counts, order doesn't matter)
func troopsMatch(sent [][]int, gam [][]int) bool {
	if len(sent) != len(gam) {
		return false
	}

	// Create maps for easier comparison
	sentMap := make(map[int]int)
	for _, pair := range sent {
		if len(pair) == 2 {
			sentMap[pair[0]] = pair[1]
		}
	}

	gamMap := make(map[int]int)
	for _, pair := range gam {
		if len(pair) == 2 {
			gamMap[pair[0]] = pair[1]
		}
	}

	// Check if maps are identical
	if len(sentMap) != len(gamMap) {
		return false
	}

	for troopID, count := range sentMap {
		if gamMap[troopID] != count {
			return false
		}
	}

	return true
}

// FetchMovements sends the GAM command to get all active movements
func FetchMovements() {
	// Cmd: %xt%EmpireEx_21%gam%1%{}%
	cmd := `%xt%EmpireEx_21%gam%1%{}%`
	OutgoingMessages <- []byte(cmd)
	log.Println("[AutoBird] Sent GAM request to fetch movements")
}

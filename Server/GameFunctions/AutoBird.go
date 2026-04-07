package GameFunctions

import (
	"CitadelDesktop/Server/Channels"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/GameWebsocket"
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

const reconcileSleepThreshold = 10 * time.Minute

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
		Models.GetSettingsState().ClearBirdIgnoreList()
		if GameWebsocket.SendAutoBirdStatusFunc != nil {
			go GameWebsocket.SendAutoBirdStatusFunc(false, 0)
		}
	}
}

// runAutoBird is the main initialization routine when auto bird is enabled.
// It runs in a loop, fetching fresh GameState, reconciling, sending birds, and sleeping.
func runAutoBird(ctx context.Context) {
	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sleepDuration := 15 * time.Minute

		// ---------------------------------------------------------
		// STEP 1: Login and Prepare
		// ---------------------------------------------------------
		if !GameWebsocket.LoginStatus {
			log.Println("[AutoBird] Disconnected. Reloading game tab to reconnect...")
			GameWebsocket.ReloadGameTab()
			// Wait for login to complete after reload
		LoginWaitLoop:
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					if GameWebsocket.LoginStatus {
						break LoginWaitLoop
					}
				}
			}
		}

		log.Println("[AutoBird] Starting processing cycle...")

		// Clear previous cycle data
		gs.Movement.BirdMovements = make(map[int][]Models.BirdMovement)

		// Step 1: Fetch fresh alliance info (bird locations and player castles)
		FetchAllianceInfo()

		// Wait a bit for the response to be processed
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}

		playerCastles := getPlayerCastleLocations()

		// Helper to find player OID
		playerOID := gs.PlayerID
		if playerOID == 0 && len(playerCastles) > 0 {
			playerOID = int(gs.Castle.MainCastle.Aid) // Fallback if GameState isn't fully refreshed yet
		}

		// ---------------------------------------------------------
		// STEP 2: Reconciliation
		// ---------------------------------------------------------
		// Run every cycle to clean up expired birds and aggressive-prune unmatched birds via GAM
		reconcileDuration, readyToSend := reconcileOnStartup(ctx)
		if !readyToSend {
			// Fast-sleep if waiting for first birds
			wakeUpTime := time.Now().Add(reconcileDuration).UnixMilli()

			autoBirdMu.Lock()
			autoBirdNextWakeUp = wakeUpTime
			autoBirdMu.Unlock()

			if GameWebsocket.SendAutoBirdStatusFunc != nil {
				go GameWebsocket.SendAutoBirdStatusFunc(true, wakeUpTime)
			}

			log.Printf("[AutoBird] Reconciliation sleep request. Sleeping for %v", reconcileDuration)

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconcileDuration):
				autoBirdMu.Lock()
				autoBirdNextWakeUp = 0
				autoBirdMu.Unlock()
				if GameWebsocket.SendAutoBirdStatusFunc != nil {
					go GameWebsocket.SendAutoBirdStatusFunc(true, 0)
				}
				continue
			}
		}

		// ---------------------------------------------------------
		// Smart Sleep: If earliest bird return is within threshold, sleep until then
		// ---------------------------------------------------------
		reconciledBirds := Models.LoadSentBirds()
		if reconciledBirds != nil && len(reconciledBirds.Birds) > 0 {
			var earliest time.Time
			for i, bird := range reconciledBirds.Birds {
				if i == 0 || bird.ExpectedExpiry.Before(earliest) {
					earliest = bird.ExpectedExpiry
				}
			}
			timeUntilReturn := time.Until(earliest)
			if timeUntilReturn > 0 && timeUntilReturn <= reconcileSleepThreshold {
				sleepDur := timeUntilReturn + 2*time.Minute
				log.Printf("[AutoBird] Earliest bird returns in %v — sleeping until then", timeUntilReturn)

				wakeUpTime := time.Now().Add(sleepDur).UnixMilli()
				autoBirdMu.Lock()
				autoBirdNextWakeUp = wakeUpTime
				autoBirdMu.Unlock()

				if GameWebsocket.SendAutoBirdStatusFunc != nil {
					go GameWebsocket.SendAutoBirdStatusFunc(true, wakeUpTime)
				}

				select {
				case <-time.After(sleepDur):
				case <-ctx.Done():
					return
				}

				autoBirdMu.Lock()
				autoBirdNextWakeUp = 0
				autoBirdMu.Unlock()

				if GameWebsocket.SendAutoBirdStatusFunc != nil {
					go GameWebsocket.SendAutoBirdStatusFunc(true, 0)
				}

				continue // restart loop (re-runs reconciliation on wake)
			}
		}
		// Otherwise fall through to send loop

		// ---------------------------------------------------------
		// STEP 3: Refresh GameState & Send Birds
		// ---------------------------------------------------------
		// Build activeCastleMap from existing saved birds
		existingBirds := Models.LoadSentBirds()
		activeCastleMap := make(map[int]bool)
		if existingBirds != nil {
			for _, bird := range existingBirds.Birds {
				activeCastleMap[bird.CastleID] = true
			}
		}

		// Calculate random delay once per cycle
		minDelay := Models.GetSettingsState().AutoBirdDelay.MinDelay
		maxDelay := Models.GetSettingsState().AutoBirdDelay.MaxDelay
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

			// FOCUS + FETCH: Triggers JAA/JCA command to focus castle and update GameState BEFORE sending
			troops := GameParser.FetchCastleTroops(castleLoc.KingdomID, castleLoc.CastleID, castleLoc.X, castleLoc.Y)
			if troops == nil || troops.TroopsI == nil {
				log.Printf("[AutoBird] Missing troops data for Castle %d. Skipping.", castleLoc.CastleID)
				continue
			}

			// Update GameState with fetched troops
			castle := gs.GetCastleByID(castleLoc.CastleID)
			if castle != nil {
				castle.Troops = Models.CastleTroopData{
					KingdomID:   troops.KingdomID,
					X:           troops.X,
					Y:           troops.Y,
					TroopsI:     troops.TroopsI,
					TroopsTU:    troops.TroopsTU,
					TroopsHI:    troops.TroopsHI,
					TroopsSHI:   troops.TroopsSHI,
					TroopsMixed: troops.TroopsMixed,
				}
				Models.SetCastleBuildingRows(castle, troops.BGRows, troops.BDRows)
			}
			time.Sleep(1 * time.Second)

			// Copy TroopsI to a local map so we can simulate deducting troops if we send multiple birds
			localTroopsI := make(map[int]int)
			for id, amount := range troops.TroopsI {
				localTroopsI[id] = amount
			}

			// Loop to allow multiple bird dispatches per cycle
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}

				totalToSend := 0
				totalKeep := 0

				var allTroopsToSend [][]int
				ignoredSummary := ""

				// Calculate what to send based on localTroopsI
				for id, amount := range localTroopsI {
					saveAmountConfigured, configured := Models.GetSettingsState().BirdIgnoreList.GetSaveAmount(castleLoc.CastleID, id)
					actualSaveAmount := 0
					sendAmount := 0

					if configured {
						if saveAmountConfigured == 0 {
							// 0 means completely ignore/remove this troopID from send list
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
						// Not explicitly configured -> send all
						actualSaveAmount = 0
						sendAmount = amount
					}

					if actualSaveAmount > 0 {
						totalKeep += actualSaveAmount
					}

					if sendAmount > 0 {
						totalToSend += sendAmount
						allTroopsToSend = append(allTroopsToSend, []int{id, sendAmount})
						if actualSaveAmount > 0 {
							ignoredSummary += fmt.Sprintf("ID:%d x%d, ", id, actualSaveAmount)
						}
					} else {
						if amount > 0 {
							ignoredSummary += fmt.Sprintf("ID:%d x%d, ", id, amount)
						}
					}
				}

				if len(ignoredSummary) > 2 {
					ignoredSummary = ignoredSummary[:len(ignoredSummary)-2]
				}

				// Check Surplus Ratio
				shouldSend := false
				ratio := 0.0
				if totalKeep == 0 {
					if totalToSend > 0 {
						shouldSend = true
					}
				} else {
					ratio = float64(totalToSend) / float64(totalKeep)
					if ratio >= 0.10 {
						shouldSend = true
					}
				}

				// Active bird logic: only send if surplus passes ratio+MinSend
				if activeCastleMap[castleLoc.CastleID] {
					if shouldSend {
						// Proceed to MinSend check
					} else {
						// Has active bird but doesn't meet surplus ratio. Skip.
						break
					}
				} else {
					// No active bird
					if !shouldSend {
						// Doesn't meet surplus ratio. Move to next castle.
						break
					}
				}

				delayCfg := Models.GetSettingsState().AutoBirdDelay
				if totalToSend > 0 && totalToSend < delayCfg.MinSend {
					log.Printf("[AutoBird] Castle %d has troops to send (%d), but is below Global Minimum to Send (%d). Skipping.",
						castleLoc.CastleID, totalToSend, delayCfg.MinSend)
					break
				}

				// Log if sending despite active bird
				if activeCastleMap[castleLoc.CastleID] {
					log.Printf("[AutoBird] Castle %d has active bird, but found significant surplus and passed MinSend (%d > %d). Sending new bird.",
						castleLoc.CastleID, totalToSend, delayCfg.MinSend)
				}

				if len(allTroopsToSend) == 0 {
					break
				}

				// Process first batch of 10
				batchSize := 10
				end := batchSize
				if end > len(allTroopsToSend) {
					end = len(allTroopsToSend)
				}
				batch := allTroopsToSend[0:end]

				// Create temp Models.CastleTroops purely for the distance calculation
				tempSourceLoc := Models.CastleTroops{
					KingdomID: castleLoc.KingdomID,
					X:         castleLoc.X,
					Y:         castleLoc.Y,
				}

				target := findClosestBirdLocation(tempSourceLoc, gs.Alliance.BirdLocations)
				if target == nil && (castleLoc.KingdomID == 1 || castleLoc.KingdomID == 2) {
					swappedKID := 1
					if castleLoc.KingdomID == 1 {
						swappedKID = 2
					}
					tempSourceLoc.KingdomID = swappedKID
					target = findClosestBirdLocation(tempSourceLoc, gs.Alliance.BirdLocations)
				}

				if target == nil {
					// No target found, give up and move to next castle
					break
				}

				// Send SDI (Bird Intent) for this batch
				sdiPayload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sdi%%1%%{"TX":%d,"TY":%d,"SX":%d,"SY":%d}%%`,
					target.X, target.Y, castleLoc.X, castleLoc.Y)
				Channels.OutgoingMessages <- []byte(sdiPayload)

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
					Channels.OutgoingMessages <- []byte(cdsPayload)

					response, err := waiter.WaitWithTimeout()
					waiter.Cleanup()
					return response, err
				}

				response, err := sendCDS(target.X, target.Y)

				if err != nil && (castleLoc.KingdomID == 1 || castleLoc.KingdomID == 2) {
					swappedKID := 1
					if castleLoc.KingdomID == 1 {
						swappedKID = 2
					}
					tempSourceLoc.KingdomID = swappedKID
					newTarget := findClosestBirdLocation(tempSourceLoc, gs.Alliance.BirdLocations)

					if newTarget != nil {
						sdiPayload := fmt.Sprintf(`%%xt%%EmpireEx_21%%sdi%%1%%{"TX":%d,"TY":%d,"SX":%d,"SY":%d}%%`,
							newTarget.X, newTarget.Y, castleLoc.X, castleLoc.Y)
						Channels.OutgoingMessages <- []byte(sdiPayload)
						time.Sleep(1 * time.Second)
						target = newTarget // Update for persistence
						response, err = sendCDS(newTarget.X, newTarget.Y)
					}
				}

				if err != nil {
					// Break inner batch loop to try next castle if send fails
					break
				}

				// Process Response
				batchSuccess := false
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

									if gs.Movement.BirdMovements == nil {
										gs.Movement.BirdMovements = make(map[int][]Models.BirdMovement)
									}
									gs.Movement.BirdMovements[castleLoc.CastleID] = append(gs.Movement.BirdMovements[castleLoc.CastleID], movement)

									castleName := getCastleName(castleLoc.CastleID)
									log.Printf("[AutoBird] Successfully sent batch from %s (MID: %.0f). Left/Ignored: %s", castleName, mid, ignoredSummary)

									// Persist the sent bird
									Models.AppendSentBird(playerOID, Models.SentBird{
										CastleID:          castleLoc.CastleID,
										TargetX:           target.X,
										TargetY:           target.Y,
										KingdomID:         castleLoc.KingdomID,
										TroopComposition:  batch,
										SentTime:          time.Now(),
										ExpectedExpiry:    returnTime,
										OneWayTimeSeconds: int(tt),
										DelayHrs:          randomDelay,
									})

									batchSuccess = true

									// Deduct the successfully sent troops from our local map before the next iteration
									for _, troopPair := range batch {
										id := troopPair[0]
										amount := troopPair[1]
										if localTroopsI[id] >= amount {
											localTroopsI[id] -= amount
										} else {
											localTroopsI[id] = 0
										}
									}
								}
							}
						}
					}
				}

				if !batchSuccess {
					// Failed to send this batch properly, break out to move to next castle
					break
				}

				time.Sleep(2 * time.Second)
			}

			// Small delay before next castle
			time.Sleep(1 * time.Second)
		}

		// Calculate sleep time based on EARLIEST return time from ALL saved birds
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
			sleepDuration = time.Until(earliestReturnTime) + 15*time.Minute
			if sleepDuration < 0 {
				sleepDuration = 1 * time.Minute // Already passed, just wait a bit
			}
		}

		// Notify frontend of sleep and store for persistence
		if GameWebsocket.SendAutoBirdStatusFunc != nil {
			wakeUpTime := time.Now().Add(sleepDuration).UnixMilli()
			autoBirdMu.Lock()
			autoBirdNextWakeUp = wakeUpTime
			autoBirdMu.Unlock()
			go GameWebsocket.SendAutoBirdStatusFunc(true, wakeUpTime)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			autoBirdMu.Lock()
			autoBirdNextWakeUp = 0
			autoBirdMu.Unlock()
			if GameWebsocket.SendAutoBirdStatusFunc != nil {
				go GameWebsocket.SendAutoBirdStatusFunc(true, 0)
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
		} else if source.KingdomID == 10 {
			// Berimond: main castle (1) or outpost (4)
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
	c := &gs.Castle
	if int(c.MainCastle.Aid) == castleID {
		return c.MainCastle.Name
	}
	if int(c.Outpost1.Aid) == castleID {
		return c.Outpost1.Name
	}
	if int(c.Outpost2.Aid) == castleID {
		return c.Outpost2.Name
	}
	if int(c.Outpost3.Aid) == castleID {
		return c.Outpost3.Name
	}
	if int(c.IceCastle.Aid) == castleID {
		return c.IceCastle.Name
	}
	if int(c.DesertCastle.Aid) == castleID {
		return c.DesertCastle.Name
	}
	if int(c.DungeonCastle.Aid) == castleID {
		return c.DungeonCastle.Name
	}
	if int(c.StormCastle.Aid) == castleID {
		return c.StormCastle.Name
	}
	if int(c.BeriWorldCastle.Aid) == castleID {
		return c.BeriWorldCastle.Name
	}
	if int(c.Metropolis.Aid) == castleID {
		return c.Metropolis.Name
	}
	if int(c.Capital.Aid) == castleID {
		return c.Capital.Name
	}
	return fmt.Sprintf("Castle %d", castleID)
}

// reconcileOnStartup is the dispatcher that routes to GAM or troop-based reconciliation.
// Loads saved birds, checks expiry, and routes based on connection state.
// Returns duration to sleep if birds are still out, or 0 if ready to send.
// Returns bool readyToSend: true if we can start sending immediately.
func reconcileOnStartup(ctx context.Context) (time.Duration, bool) {
	savedBirds := Models.LoadSentBirds()
	if savedBirds == nil || len(savedBirds.Birds) == 0 {
		return 0, true // No saved birds, ready to send
	}

	// Check if all birds are expired
	allExpired := true
	now := time.Now()
	for _, bird := range savedBirds.Birds {
		if now.Before(bird.ExpectedExpiry) {
			allExpired = false
			break
		}
	}

	if allExpired {
		Models.ClearSentBirds()
		return 0, true
	}

	// Route based on connection state
	if isGameConnected() {
		return reconcileViaGAM(ctx, savedBirds)
	}
	return reconcileViaTroops(ctx, savedBirds)
}

// isGameConnected checks if the game websocket is currently logged in
func isGameConnected() bool {
	return GameWebsocket.LoginStatus
}

// reconcileViaGAM checks persisted birds and reconciles with live GAM data.
// Connected path: reads from live ActiveMovements first, falls back to GAM fetch if empty.
// Conservative matching: unexpired unmatched birds are kept with a log.
// Always returns readyToSend=true.
func reconcileViaGAM(ctx context.Context, savedBirds *Models.SentBirdFile) (time.Duration, bool) {
	gs := Models.GetGameState()

	// 1. Check live ActiveMovements
	movements := gs.Movement.ActiveMovements
	if len(movements) == 0 {
		// Fallback: send a GAM request and wait up to 5s for the parser to populate
		log.Println("[AutoBird] ActiveMovements empty. Fetching GAM...")
		FetchMovements()
		select {
		case <-time.After(5 * time.Second):
		case <-ctx.Done():
			return 0, false
		}
		movements = gs.Movement.ActiveMovements
	}

	// 2. Reconcile each saved bird
	var keepBirds []Models.SentBird
	now := time.Now()
	for _, bird := range savedBirds.Birds {
		if now.After(bird.ExpectedExpiry) {
			// Expired — drop it
			continue
		}
		matched := false
		for _, movement := range movements {
			if troopsMatch(bird.TroopComposition, movement.TroopArray) {
				matched = true
				break
			}
		}
		if matched {
			keepBirds = append(keepBirds, bird)
		} else {
			// Conservative: keep unexpired unmatched birds (parser might miss under load)
			log.Printf("[AutoBird] Unmatched unexpired bird kept — castle: %s KID: %d troops: %v",
				getCastleName(bird.CastleID), bird.KingdomID, bird.TroopComposition)
			keepBirds = append(keepBirds, bird)
		}
	}

	// 3. Save reconciled list
	if len(keepBirds) > 0 {
		log.Printf("[AutoBird] Reconciliation: keeping %d birds (from %d saved)", len(keepBirds), len(savedBirds.Birds))
		Models.SaveSentBirds(savedBirds.PlayerID, keepBirds)
	} else {
		log.Println("[AutoBird] Reconciliation: all birds expired, clearing file")
		Models.ClearSentBirds()
	}

	return 0, true
}

// reconcileViaTroops reconciles saved birds using troop-based verification when disconnected.
// This path is used when the game websocket is not connected.
// Returns duration to sleep if birds are still out, or 0 if ready to send.
// Returns bool readyToSend: true if we can start sending immediately.
func reconcileViaTroops(ctx context.Context, savedBirds *Models.SentBirdFile) (time.Duration, bool) {
	// 1. Reconnect: reload game tab and wait for login
	GameWebsocket.ReloadGameTab()
	loginTimeout := time.After(2 * time.Minute)
	for !isGameConnected() {
		select {
		case <-ctx.Done():
			return 0, false
		case <-loginTimeout:
			log.Println("[AutoBird] Troop reconciliation: timed out waiting for login")
			return 0, true // Fall through to send loop
		case <-time.After(2 * time.Second):
			// retry
		}
	}

	// 2. Get castle locations for cross-referencing
	castleLocations := getPlayerCastleLocations()
	castleLocationMap := make(map[int]CastleLocation) // keyed by CastleID
	for _, loc := range castleLocations {
		castleLocationMap[loc.CastleID] = loc
	}

	// 3. Reconcile per castle
	var keepBirds []Models.SentBird
	now := time.Now()
	for _, bird := range savedBirds.Birds {
		if now.After(bird.ExpectedExpiry) {
			continue // expired, drop
		}
		loc, ok := castleLocationMap[bird.CastleID]
		if !ok {
			// Can't verify, keep conservatively
			keepBirds = append(keepBirds, bird)
			continue
		}
		troops := GameParser.FetchCastleTroops(bird.KingdomID, bird.CastleID, loc.X, loc.Y)
		if troops == nil {
			keepBirds = append(keepBirds, bird)
			continue
		}

		// Check TroopsTU (travelling troops) against bird composition
		allPresent := true
		partialMatch := false
		for _, troopPair := range bird.TroopComposition {
			troopID := troopPair[0]
			count := troopPair[1]
			tuCount := troops.TroopsTU[troopID]
			iCount := troops.TroopsI[troopID]
			if tuCount >= count {
				partialMatch = true
			} else if iCount >= count {
				// Troops are back home — bird returned
				allPresent = false
			}
		}

		if allPresent && partialMatch {
			keepBirds = append(keepBirds, bird)
		} else if !allPresent && partialMatch {
			// Partial match — ambiguous, keep with warning
			log.Printf("[AutoBird] Partial match for bird castle %s — keeping conservatively", getCastleName(bird.CastleID))
			keepBirds = append(keepBirds, bird)
		} else {
			log.Printf("[AutoBird] Bird returned (troops home) for castle %s — dropping", getCastleName(bird.CastleID))
			// Drop: bird has returned
		}
	}

	if len(keepBirds) > 0 {
		log.Printf("[AutoBird] Troop reconciliation: keeping %d birds (from %d saved)", len(keepBirds), len(savedBirds.Birds))
		Models.SaveSentBirds(savedBirds.PlayerID, keepBirds)
	} else {
		log.Println("[AutoBird] Troop reconciliation: all birds expired/returned, clearing file")
		Models.ClearSentBirds()
	}

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
	Channels.OutgoingMessages <- []byte(cmd)
	log.Println("[AutoBird] Sent GAM request to fetch movements")
}

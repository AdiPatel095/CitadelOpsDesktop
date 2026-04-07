package GameFunctions

import (
	"CitadelDesktop/Server/Channels"
	"CitadelDesktop/Server/GameParser"
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
		Models.GetSettingsState().ClearBirdIgnoreList()
		if ResponseRegistry.SendAutoBirdStatusFunc != nil {
			go ResponseRegistry.SendAutoBirdStatusFunc(false, 0)
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
		if !ResponseRegistry.LoginStatus {
			log.Println("[AutoBird] Disconnected. Reloading game tab to reconnect...")
			ResponseRegistry.ReloadGameTab()
			// Wait for login to complete after reload
		LoginWaitLoop:
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					if ResponseRegistry.LoginStatus {
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

			if ResponseRegistry.SendAutoBirdStatusFunc != nil {
				go ResponseRegistry.SendAutoBirdStatusFunc(true, wakeUpTime)
			}

			log.Printf("[AutoBird] Reconciliation sleep request. Sleeping for %v", reconcileDuration)

			select {
			case <-ctx.Done():
				return
			case <-time.After(reconcileDuration):
				autoBirdMu.Lock()
				autoBirdNextWakeUp = 0
				autoBirdMu.Unlock()
				if ResponseRegistry.SendAutoBirdStatusFunc != nil {
					go ResponseRegistry.SendAutoBirdStatusFunc(true, 0)
				}
				continue
			}
		}

		// ---------------------------------------------------------
		// STEP 3: Refresh GameState & Send Birds
		// ---------------------------------------------------------
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
			if troops != nil {
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
			}
			time.Sleep(1 * time.Second)

			// Look up castle in GameState
			castleInfo := gs.GetCastleByID(castleLoc.CastleID)
			if castleInfo == nil || castleInfo.Troops.TroopsI == nil {
				log.Printf("[AutoBird] Missing GameState troops for Castle %d. Skipping.", castleLoc.CastleID)
				continue
			}

			// Copy TroopsI to a local map so we can simulate deducting troops if we send multiple birds
			localTroopsI := make(map[int]int)
			for id, amount := range castleInfo.Troops.TroopsI {
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
				if totalKeep == 0 {
					if totalToSend > 0 {
						shouldSend = true
					}
				} else {
					ratio := float64(totalToSend) / float64(totalKeep)
					if ratio >= 0.10 {
						shouldSend = true
					}
				}

				if !shouldSend {
					// Doesn't meet surplus ratio. Move to next castle.
					break
				}
				if totalToSend > 0 && totalToSend < Models.GetSettingsState().AutoBirdDelay.MinSend {
					// Doesn't meet minimum send total. Move to next castle.
					break
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
		if ResponseRegistry.SendAutoBirdStatusFunc != nil {
			wakeUpTime := time.Now().Add(sleepDuration).UnixMilli()
			autoBirdMu.Lock()
			autoBirdNextWakeUp = wakeUpTime
			autoBirdMu.Unlock()
			go ResponseRegistry.SendAutoBirdStatusFunc(true, wakeUpTime)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			autoBirdMu.Lock()
			autoBirdNextWakeUp = 0
			autoBirdMu.Unlock()
			if ResponseRegistry.SendAutoBirdStatusFunc != nil {
				go ResponseRegistry.SendAutoBirdStatusFunc(true, 0)
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

// reconcileOnStartup checks persisted birds and reconciles with live GAM data.
// It aggressively removes any saved birds that DO NOT MATCH returning GAM payloads.
// Returns duration to sleep if birds are still out, or 0 if ready to send.
// Returns bool readyToSend: true if we can start sending immediately.
func reconcileOnStartup(ctx context.Context) (time.Duration, bool) {
	// 1. Load sentBirds.json
	savedBirds := Models.LoadSentBirds()
	if savedBirds == nil || len(savedBirds.Birds) == 0 {
		return 0, true // No saved birds, ready
	}

	// 2. Fetch GAM
	log.Println("[AutoBird] Found active saved birds. Reconciling with game server GAM data...")
	gs := Models.GetGameState()
	gs.Movement.ActiveMovements = nil

	FetchMovements()

	// Wait for GAM response
	select {
	case <-ctx.Done():
		return 0, false
	case <-time.After(5 * time.Second):
	}

	gs = Models.GetGameState()

	// 3. Match birds against GAM movements by troop composition
	// If a bird does NOT exist in GAM, it is dropped aggressively.
	now := time.Now()
	var matchedBirds []Models.SentBird

	for _, bird := range savedBirds.Birds {
		// Automatically drop if locally expired
		if !now.Before(bird.ExpectedExpiry) {
			log.Printf("[AutoBird] Bird from castle %d expired normally (expected return: %v)", bird.CastleID, bird.ExpectedExpiry)
			continue
		}

		matched := false
		for _, movement := range gs.Movement.ActiveMovements {
			if troopsMatch(bird.TroopComposition, movement.TroopArray) {
				matched = true
				matchedBirds = append(matchedBirds, bird)
				break
			}
		}

		if !matched {
			castleName := getCastleName(bird.CastleID)
			log.Printf("[AutoBird] ✗ No GAM match for unexpired bird from %s (KID: %d). Dropping from save list.", castleName, bird.KingdomID)
		}
	}

	// 4. Update saved file with the kept birds
	if len(matchedBirds) > 0 {
		log.Printf("[AutoBird] Found %d active birds still in flight. Updating saved file.", len(matchedBirds))
		Models.SaveSentBirds(savedBirds.PlayerID, matchedBirds)
	} else {
		log.Println("[AutoBird] No saved birds matched GAM movements. Clearing file.")
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

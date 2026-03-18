package GameFunctions

import (
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"fmt"
	"log"
	"sync"
	"time"
)

var (
	recruitTroopsCancel context.CancelFunc
	recruitTroopsMu     sync.Mutex
)

// IsRecruitTroopsRunning returns true if the RecruitTroops goroutine is currently active
func IsRecruitTroopsRunning() bool {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()
	return recruitTroopsCancel != nil
}

// StartRecruitTroops starts the recruit troops goroutine.
func StartRecruitTroops() {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()

	// If already running, don't start another
	if recruitTroopsCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	recruitTroopsCancel = cancel

	go runRecruitTroops(ctx)
}

// StopRecruitTroops stops the recruit troops goroutine if running.
func StopRecruitTroops() {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()

	if recruitTroopsCancel != nil {
		recruitTroopsCancel()
		recruitTroopsCancel = nil
		Models.GetSettingsState().ClearRecruitTroopsList()
		if ResponseRegistry.SendRecruitTroopsStatusFunc != nil {
			go ResponseRegistry.SendRecruitTroopsStatusFunc(false)
		}
	}
}

// runRecruitTroops is the main loop for Auto-Recruiting
func runRecruitTroops(ctx context.Context) {
	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		sleepDuration := 5 * time.Minute

		// If disconnected, handle reload
		if !ResponseRegistry.LoginStatus {
			log.Println("[RecruitTroops] Disconnected. Waiting for login...")
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

		log.Println("[RecruitTroops] Starting processing cycle...")

		settings := Models.GetSettingsState().RecruitTroopsList
		if settings.Targets == nil || len(settings.Targets) == 0 {
			log.Println("[RecruitTroops] No targets configured. Sleeping...")
		} else {
			// Iterate through configured castles
			for castleID, targets := range settings.Targets {
				select {
				case <-ctx.Done():
					return
				default:
				}

				castleInfo := gs.GetCastleByID(castleID)
				if castleInfo == nil {
					log.Printf("[RecruitTroops] Castle %d not found in GameState. Skipping.", castleID)
					continue
				}

				// Find location details for the `FetchCastleTroops` call
				var kingdomID, x, y int
				foundLoc := false
				for _, loc := range gs.Alliance.PlayerCastleLocations {
					if loc.CastleID == castleID {
						kingdomID = loc.KingdomID
						x = loc.X
						y = loc.Y
						foundLoc = true
						break
					}
				}

				if !foundLoc {
					log.Printf("[RecruitTroops] Could not find map location for Castle %d. Skipping.", castleID)
					continue
				}

				// Focus castle to get the latest troop counts
				troops := GameParser.FetchCastleTroops(kingdomID, castleID, x, y)
				if troops != nil {
					castleInfo.Troops = Models.CastleTroopData{
						KingdomID:   troops.KingdomID,
						X:           troops.X,
						Y:           troops.Y,
						TroopsI:     troops.TroopsI,
						TroopsTU:    troops.TroopsTU,
						TroopsHI:    troops.TroopsHI,
						TroopsSHI:   troops.TroopsSHI,
						TroopsMixed: troops.TroopsMixed,
					}
					castleInfo.Buildings = troops.Buildings
				}
				time.Sleep(1 * time.Second)

				// For each target troop type, check if we need to recruit
				for troopID, targetAmount := range targets {
					currentAmount := 0
					if troops != nil && troops.TroopsMixed != nil {
						currentAmount = troops.TroopsMixed[troopID]
					} else {
						// Fallback to local GameState if fetch failed
						currentAmount = castleInfo.Troops.TroopsI[troopID] + castleInfo.Troops.TroopsTU[troopID]
					}

					amountNeeded := targetAmount - currentAmount
					if amountNeeded > 0 {
						// Send standard batch size
						// Based on typical recruitment slots, we can send e.g. 50 at a time, or however much is needed
						amountToRecruit := amountNeeded
						// Let's cap it at a reasonable number per cycle to not spam if the queue is full
						if amountToRecruit > 100 {
							amountToRecruit = 100
						}

						log.Printf("[RecruitTroops] Castle: %d. Found %d/%d of Troop %d. Recruiting %d.", castleID, currentAmount, targetAmount, troopID, amountToRecruit)
						// Command format: %xt%EmpireEx_21%bup%1%{"LID":0,"WID":<troopID>,"AMT":<amount>,"PO":-1,"PWR":0,"SK":73,"SID":0,"AID":<castleID>}%
						payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%bup%%1%%{"LID":0,"WID":%d,"AMT":%d,"PO":-1,"PWR":0,"SK":73,"SID":0,"AID":%d}%%`, troopID, amountToRecruit, castleID)

						ResponseRegistry.OutgoingMessages <- []byte(payload)
						time.Sleep(1 * time.Second) // Small delay between recruit commands
					}
				}
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			// Waking up for next cycle
		}
	}
}

package GameFunctions

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
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

				// Focus castle; inbound **jaa** updates troops/buildings/focus in MessageRouter.
				_ = GameParser.FetchCastleTroops(kingdomID, castleID, x, y)
				time.Sleep(1 * time.Second)

				// For each target troop type, check if we need to recruit
				tmix := castleInfo.Troops.TroopsMixed
				for troopID, targetAmount := range targets {
					currentAmount := 0
					if tmix != nil {
						currentAmount = tmix[troopID]
					} else {
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
						// SK must match live session (captured from browser bup); TODO: read from game state when available.
						const recruitSessionKey = 73
						GameCommands.SendBarracksUnitPurchase(0, troopID, amountToRecruit, -1, 0, recruitSessionKey, 0, castleID)
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

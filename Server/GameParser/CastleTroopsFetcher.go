package GameParser

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"fmt"
	"log"
	"sort"
	"time"
)

// FetchAllCastleTroopsAndConsumption requests troops for all player castles, stores them on the castle objects,
// and recalculates resource consumption. Called after GCL/DCL parsing completes.
func FetchAllCastleTroopsAndConsumption() {
	gs := Models.GetGameState()

	// Make a copy of the slice to sort it without affecting the original order in GameState
	castles := make([]Models.PlayerCastleLocation, len(gs.Alliance.PlayerCastleLocations))
	copy(castles, gs.Alliance.PlayerCastleLocations)

	// Sort order: Main Castle (KingdomID 0, CastleID == gs.MainCastle.Aid) -> Outposts (KingdomID 0) -> Others by KingdomID
	sort.SliceStable(castles, func(i, j int) bool {
		c1, c2 := castles[i], castles[j]
		if c1.KingdomID != c2.KingdomID {
			return c1.KingdomID < c2.KingdomID
		}
		// If both are Kingdom 0, Main Castle comes first
		if c1.KingdomID == 0 {
			if float64(c1.CastleID) == gs.MainCastle.Aid {
				return true
			}
			if float64(c2.CastleID) == gs.MainCastle.Aid {
				return false
			}
		}
		return c1.CastleID < c2.CastleID
	})

	for i, loc := range castles {
		// Delay between requests to avoid overwhelming the server (skip delay for first)
		if i > 0 {
			time.Sleep(1500 * time.Millisecond)
		}

		troops := sendTroopRequest(loc.KingdomID, loc.CastleID, loc.X, loc.Y)
		if troops != nil {

			castle := gs.GetCastleByID(loc.CastleID)
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
				castle.Buildings = troops.Buildings
			} else {
				log.Printf("[TroopFetch ERROR] gs.GetCastleByID returned nil for CastleID=%d", loc.CastleID)
			}

			// Recalculate consumption and get the string id of the castle
			castleLocationName := RecalculateCastleConsumption(loc.CastleID)

			// Notify the frontend of the updated castle (resources + troops + consumption)
			if castleLocationName != "" && UpdateCastleResourceFunc != nil {
				UpdateCastleResourceFunc(castleLocationName)
			}
		}
	}
}

// UpdateCastleResourceFunc is a callback to tell the frontend to refresh castle data.
// Wired by FrontendWebsocket init().
var UpdateCastleResourceFunc func(string)

// FetchCastleTroops sends JAA command and waits for response to get troop counts.
// Tries swapped KingdomID for Ice/Desert if first attempt fails.
func FetchCastleTroops(kingdomID, castleID, x, y int) *Models.CastleTroops {
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

// sendTroopRequest sends a single JAA/JCA request and returns the parsed result.
func sendTroopRequest(kingdomID, castleID, x, y int) *Models.CastleTroops {
	var payload string

	if kingdomID == 0 || kingdomID == 4 || kingdomID == 10 {
		payload = fmt.Sprintf(`%%xt%%EmpireEx_21%%jaa%%1%%{"PX":%d,"PY":%d,"KID":%d}%%`, x, y, kingdomID)
	} else {
		payload = fmt.Sprintf(`%%xt%%EmpireEx_21%%jca%%1%%{"CID":%d,"KID":%d}%%`, castleID, kingdomID)
	}

	// Create a waiter for the response
	waiter := ResponseRegistry.Global.RegisterWaiter("jaa", 5*time.Second)
	defer waiter.Cleanup()

	// Send the command
	ResponseRegistry.OutgoingMessages <- []byte(payload)

	// Wait for response with timeout
	response, err := waiter.WaitWithTimeout()
	if err != nil {
		return nil
	}

	if len(response) > 5 {
		return parseTroopsFromJAA(response[5], kingdomID, castleID, x, y)
	}

	return nil
}

// parseTroopsFromJAA extracts troop data from JAA response.
func parseTroopsFromJAA(data string, kingdomID, castleID, x, y int) *Models.CastleTroops {
	troops := ParseCastleTroops(data, kingdomID, x, y)
	if troops != nil {
		troops.CastleID = castleID
	}
	return troops
}

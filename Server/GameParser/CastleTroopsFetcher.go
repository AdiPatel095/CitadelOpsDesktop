package GameParser

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"log"
	"sync"
	"time"
)

var (
	troopFetchMutex  sync.Mutex
	isFetchingTroops bool
)

// jaaTroopFetchWait is how long we wait for the game's jaa response after SendTroopFocus (JCA/JAA).
// Must be long enough for a real round trip; the previous 25ms value effectively never received data.
const jaaTroopFetchWait = 8 * time.Second

// FetchAllCastleTroopsAndConsumption runs once after GCL/DCL: a single JAA/JCA for the main castle only.
// Other castles are refreshed when the player focuses them (in-game JAA or frontend focusPlayerCastle).
func FetchAllCastleTroopsAndConsumption() {
	troopFetchMutex.Lock()
	if isFetchingTroops {
		troopFetchMutex.Unlock()
		return
	}
	isFetchingTroops = true
	troopFetchMutex.Unlock()

	defer func() {
		troopFetchMutex.Lock()
		isFetchingTroops = false
		troopFetchMutex.Unlock()
	}()

	gs := Models.GetGameState()
	refocusMainCastleAfterTroopFetch(gs)
}

// FocusPlayerCastleTroops sends one JAA/JCA for the castle, merges troops and buildings into GameState,
// and the jaa handler updates CastleFocus and notifies the frontend.
func FocusPlayerCastleTroops(kingdomID, castleID, x, y int) bool {
	troops := FetchCastleTroops(kingdomID, castleID, x, y)
	if troops == nil {
		return false
	}
	gs := Models.GetGameState()
	applyTroopFetchToCastle(gs, castleID, troops)
	return true
}

func applyTroopFetchToCastle(gs *Models.GameState, castleID int, troops *Models.CastleTroops) {
	if troops == nil {
		return
	}
	castle := gs.GetCastleByID(castleID)
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
	} else {
		log.Printf("[TroopFetch ERROR] gs.GetCastleByID returned nil for CastleID=%d", castleID)
		return
	}
	castleLocationName := RecalculateCastleConsumption(castleID)
	if castleLocationName != "" && UpdateCastleResourceFunc != nil {
		UpdateCastleResourceFunc(castleLocationName)
	}
}

func refocusMainCastleAfterTroopFetch(gs *Models.GameState) {
	if !ResponseRegistry.LoginStatus {
		return
	}
	mainAID := int(gs.Castle.MainCastle.Aid)
	if mainAID <= 0 {
		return
	}
	kingdomID, mapX, mapY, ok := mainCastleMapCoords(gs, mainAID)
	if !ok {
		return
	}
	time.Sleep(25 * time.Millisecond)
	if !ResponseRegistry.LoginStatus {
		return
	}
	// FetchCastleTroops includes desert/ice KID retry; refreshes state and drives JAA focus for main.
	troops := FetchCastleTroops(kingdomID, mainAID, mapX, mapY)
	if troops != nil {
		applyTroopFetchToCastle(gs, mainAID, troops)
	}
}

func mainCastleMapCoords(gs *Models.GameState, mainAID int) (kingdomID, x, y int, ok bool) {
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == mainAID {
			return loc.KingdomID, loc.X, loc.Y, true
		}
	}
	td := gs.Castle.MainCastle.Troops
	if td.X == 0 && td.Y == 0 {
		return 0, 0, 0, false
	}
	return td.KingdomID, td.X, td.Y, true
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
	// Create a waiter for the response
	waiter := ResponseRegistry.Global.RegisterWaiter("jaa", jaaTroopFetchWait)
	defer waiter.Cleanup()

	GameCommands.SendTroopFocus(kingdomID, castleID, x, y)

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

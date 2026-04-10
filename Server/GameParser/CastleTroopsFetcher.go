package GameParser

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"log"
	"sync"
	"sync/atomic"
	"time"
)

var (
	troopFetchMutex  sync.Mutex
	isFetchingTroops bool
)

// jaaTroopFetchWait is how long we wait for an inbound **jaa** after SendTroopFocus (see markJAAProcessed).
const jaaTroopFetchWait = 8 * time.Second

// FetchAllCastleTroopsAndConsumption runs once after GCL/DCL: send focus for the main castle; inbound jaa updates GameState.
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

// FocusPlayerCastleTroops sends JAA/JCA for the castle and waits until the game's **jaa** response is processed
// (focus + BG/BD + troops applied in MessageRouter).
func FocusPlayerCastleTroops(kingdomID, castleID, x, y int) bool {
	gs := Models.GetGameState()
	if trySendAndAwaitJAA(gs, kingdomID, castleID, x, y) {
		return true
	}
	if kingdomID == 1 || kingdomID == 2 {
		swapped := 2
		if kingdomID == 2 {
			swapped = 1
		}
		return trySendAndAwaitJAA(gs, swapped, castleID, x, y)
	}
	return false
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
	GameCommands.SendTroopFocus(kingdomID, mainAID, mapX, mapY)
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

// FetchCastleTroops sends focus and waits for the next processed **jaa** for that castle, then returns a snapshot from GameState.
func FetchCastleTroops(kingdomID, castleID, x, y int) *Models.CastleTroops {
	gs := Models.GetGameState()
	if trySendAndAwaitJAA(gs, kingdomID, castleID, x, y) {
		return troopSnapshotFromCastle(gs, castleID)
	}
	if kingdomID == 1 || kingdomID == 2 {
		swapped := 2
		if kingdomID == 2 {
			swapped = 1
		}
		if trySendAndAwaitJAA(gs, swapped, castleID, x, y) {
			return troopSnapshotFromCastle(gs, castleID)
		}
	}
	return nil
}

func trySendAndAwaitJAA(gs *Models.GameState, kingdomID, castleID, mapX, mapY int) bool {
	prev := atomic.LoadInt64(&lastJAAProcessedNs)
	GameCommands.SendTroopFocus(kingdomID, castleID, mapX, mapY)
	if !awaitNextJAAAfter(prev, jaaTroopFetchWait) {
		return false
	}
	return gs.CastleFocus.CastleAID == castleID
}

func troopSnapshotFromCastle(gs *Models.GameState, castleID int) *Models.CastleTroops {
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return nil
	}
	td := c.Troops
	return &Models.CastleTroops{
		KingdomID:   td.KingdomID,
		CastleID:    castleID,
		X:           td.X,
		Y:           td.Y,
		Troops:      td.TroopsI,
		TroopsI:     td.TroopsI,
		TroopsTU:    td.TroopsTU,
		TroopsHI:    td.TroopsHI,
		TroopsSHI:   td.TroopsSHI,
		TroopsMixed: td.TroopsMixed,
	}
}

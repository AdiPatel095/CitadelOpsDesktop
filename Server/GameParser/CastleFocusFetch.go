package GameParser

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/ResponseRegistry"
	"time"
)

const focusJAAWaitTimeout = 8 * time.Second

// FocusPlayerCastle sends JAA or JCA for the castle and waits for a jaa response so MessageRouter
// can refresh GameState (including BG/BD). Returns true if a jaa frame was received in time.
func FocusPlayerCastle(kingdomID, castleID, mapX, mapY int) bool {
	waiter := ResponseRegistry.Global.RegisterWaiter("jaa", focusJAAWaitTimeout)
	defer waiter.Cleanup()

	GameCommands.SendTroopFocus(kingdomID, castleID, mapX, mapY)

	_, err := waiter.WaitWithTimeout()
	if err != nil {
		return false
	}
	// Desert/Ice ambiguity: if JCA failed to update, caller may retry swapped KID elsewhere.
	return true
}

// FocusPlayerCastleWithRetry matches FetchCastleTroops: retry JCA with swapped KID for desert (1) / ice (2).
func FocusPlayerCastleWithRetry(kingdomID, castleID, mapX, mapY int) bool {
	if FocusPlayerCastle(kingdomID, castleID, mapX, mapY) {
		return true
	}
	if kingdomID == 1 || kingdomID == 2 {
		swapped := 2
		if kingdomID == 2 {
			swapped = 1
		}
		return FocusPlayerCastle(swapped, castleID, mapX, mapY)
	}
	return false
}

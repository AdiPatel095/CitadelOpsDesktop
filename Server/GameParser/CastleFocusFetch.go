package GameParser

import "CitadelDesktop/Server/Models"

// FocusPlayerCastle sends JAA or JCA for the castle and waits until MessageRouter has processed the game's **jaa**
// (focus, BG/BD, troops). Returns true if the focused castle matches.
func FocusPlayerCastle(kingdomID, castleID, mapX, mapY int) bool {
	gs := Models.GetGameState()
	return trySendAndAwaitJAA(gs, kingdomID, castleID, mapX, mapY)
}

func FocusPlayerCastleWithLease(lease focusLease, kingdomID, castleID, mapX, mapY int) bool {
	gs := Models.GetGameState()
	return trySendAndAwaitJAAWithLease(lease, gs, kingdomID, castleID, mapX, mapY)
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

func FocusPlayerCastleWithRetryAndLease(lease focusLease, kingdomID, castleID, mapX, mapY int) bool {
	if FocusPlayerCastleWithLease(lease, kingdomID, castleID, mapX, mapY) {
		return true
	}
	if kingdomID == 1 || kingdomID == 2 {
		swapped := 2
		if kingdomID == 2 {
			swapped = 1
		}
		return FocusPlayerCastleWithLease(lease, swapped, castleID, mapX, mapY)
	}
	return false
}

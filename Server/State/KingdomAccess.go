package State

const stormCastleKingdomID KingdomID = 4

// CastleFocusKnownUnavailable reports only an authoritative, current-session
// inability to enter a retained seasonal castle. Focus-neutral commands can
// remain valid for that kingdom, so callers must use this only for work that
// requires JAA/JCA castle focus. Missing or pre-session transport data remains
// unknown and is not treated as unavailable.
func CastleFocusKnownUnavailable(gameState GameState, castle CastleState) bool {
	if castle.ID <= 0 || castle.KingdomID == 0 {
		return false
	}
	observedAt := gameState.KingdomTransport.ObservedAt
	if observedAt.IsZero() ||
		!gameState.Session.ChangedAt.IsZero() && observedAt.Before(gameState.Session.ChangedAt) {
		return false
	}
	unlock, observed := gameState.KingdomTransport.Unlocks[castle.KingdomID]
	// Storm's authoritative transport row remains Unlocked=true while its
	// current castle is represented with Created=false. Unlike retained closed
	// event kingdoms, that is an enterable Storm castle, not an absent one.
	if castle.KingdomID == stormCastleKingdomID {
		return observed && !unlock.Unlocked && !unlock.Created
	}
	return observed && !unlock.Created
}

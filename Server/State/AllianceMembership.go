package State

// AllianceMembershipCurrent requires explicit membership observed in this
// process and session. Saved alliance IDs and prior sessions cannot authorize help.
func AllianceMembershipCurrent(state *GameState) bool {
	return state != nil && state.Player.AllianceID > 0 &&
		!state.Player.AllianceMembershipObservedAt.IsZero() &&
		state.Player.AllianceMembershipGeneration == state.Session.Generation
}

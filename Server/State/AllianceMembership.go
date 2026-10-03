package State

import "time"

// ObserveAllianceMembership records authoritative evidence in frame commit order.
// Within a session, none holds until a positive with a strictly later frame time.
func ObserveAllianceMembership(state *GameState, id AllianceID, observedAt time.Time) bool {
	if state == nil {
		return false
	}
	if id < 0 {
		id = 0
	}
	player := &state.Player
	if id > 0 && player.AllianceMembershipID == 0 &&
		player.AllianceMembershipGeneration == state.Session.Generation &&
		!player.AllianceMembershipObservedAt.IsZero() &&
		!observedAt.After(player.AllianceMembershipObservedAt) {
		return false
	}
	if player.AllianceMembershipID == id &&
		player.AllianceMembershipObservedAt.Equal(observedAt) &&
		player.AllianceMembershipGeneration == state.Session.Generation {
		return false
	}
	player.AllianceMembershipID = id
	player.AllianceMembershipObservedAt = observedAt
	player.AllianceMembershipGeneration = state.Session.Generation
	return true
}

// AllianceMembershipCurrent requires explicit membership observed in this
// process and session. Saved alliance IDs and prior sessions cannot authorize help.
func AllianceMembershipCurrent(state *GameState) bool {
	return state != nil && state.Player.AllianceMembershipID > 0 &&
		!state.Player.AllianceMembershipObservedAt.IsZero() &&
		state.Player.AllianceMembershipGeneration == state.Session.Generation &&
		state.Player.AllianceID == state.Player.AllianceMembershipID
}

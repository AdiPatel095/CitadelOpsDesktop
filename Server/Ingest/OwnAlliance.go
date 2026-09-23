package Ingest

import (
	"CitadelDesktop/Server/State"
	"encoding/json"
	"time"
)

func setOwnAlliance(state *State.GameState, id State.AllianceID) {
	if state.Player.AllianceID != id || state.Alliance.ID != id {
		state.Alliance = State.AllianceState{ID: id, Members: []State.AllianceMember{}, Holdings: []State.AllianceHolding{}}
	}
	state.Player.AllianceID = id
}

// Only an explicitly identified own-player AID is membership authority. An
// omitted AID is not a departure, and an inspected alliance is not membership.
func applyOwnAllianceSnapshot(root map[string]json.RawMessage, observed time.Time, state *State.GameState) bool {
	if observed.IsZero() || observed.Before(state.Player.AllianceObservedAt) {
		return false
	}
	var visit func(map[string]json.RawMessage) bool
	visit = func(owner map[string]json.RawMessage) bool {
		id, own := rawInt64(owner["OID"])
		aid, present := rawInt64(owner["AID"])
		if own && State.PlayerID(id) == state.Player.ID && id > 0 && present && aid >= 0 {
			setOwnAlliance(state, State.AllianceID(aid))
			state.Player.AllianceObservedAt = observed
			return true
		}
		for _, key := range []string{"O", "gca", "gaa"} {
			var nested map[string]json.RawMessage
			if json.Unmarshal(owner[key], &nested) == nil && visit(nested) {
				return true
			}
		}
		var owners []map[string]json.RawMessage
		if json.Unmarshal(owner["OI"], &owners) == nil {
			for _, next := range owners {
				if visit(next) {
					return true
				}
			}
		}
		return false
	}
	return visit(root)
}

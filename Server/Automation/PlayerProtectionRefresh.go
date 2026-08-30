package Automation

import (
	"encoding/json"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const playerProtectionRefreshInterval = 2 * time.Minute

func playerProtectionRefreshDecision(snapshot Snapshot) (Decision, bool) {
	observedAt := snapshot.State.Player.ProtectionMode.ObservedAt
	stale := !observedAt.IsZero() && !snapshot.Now.Before(observedAt.Add(playerProtectionRefreshInterval))
	if !snapshot.PolicyConfigurationChanged && !stale {
		return Decision{}, false
	}

	selected := State.CastleState{}
	for _, castleID := range sortedCastleIDs(snapshot.State.Castles) {
		castle := snapshot.State.Castles[castleID]
		if castle.ID <= 0 {
			continue
		}
		if selected.ID <= 0 || castle.Focused {
			selected = castle
		}
		if castle.Focused {
			break
		}
	}
	if selected.ID <= 0 {
		return Decision{
			Status: "waiting", Detail: "Waiting for an owned castle before refreshing Protection Mode",
			NextCheckAt: snapshot.Now.Add(10 * time.Second),
		}, true
	}

	arguments, _ := json.Marshal(map[string]any{
		"kingdomId": selected.KingdomID,
		"x1":        selected.X, "y1": selected.Y,
		"x2": selected.X, "y2": selected.Y,
	})
	return Decision{
		Status:              "refreshing",
		Detail:              "Refreshing Protection Mode from the game",
		NextCheckAt:         snapshot.Now.Add(playerProtectionRefreshInterval),
		Request:             &Intent.Request{Name: "map.query", Arguments: arguments},
		ReevaluateOnSuccess: true,
	}, true
}

func capAtPlayerProtectionRefresh(snapshot Snapshot, decision Decision) Decision {
	observedAt := snapshot.State.Player.ProtectionMode.ObservedAt
	if observedAt.IsZero() {
		return decision
	}
	dueAt := observedAt.Add(playerProtectionRefreshInterval)
	if decision.NextCheckAt.IsZero() || dueAt.Before(decision.NextCheckAt) {
		decision.NextCheckAt = dueAt
	}
	return decision
}

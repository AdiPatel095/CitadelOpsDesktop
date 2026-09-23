package Automation

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"time"
)

// EligibleStationRemainder uses current inventory, not the original manifest:
// accepted support batches have already removed their troops from the castle.
func EligibleStationRemainder(data *GameData.Store, castle State.CastleState, reserves map[State.UnitID]int64) (int64, bool) {
	var total int64
	for id, amount := range castle.Units.Stationed {
		amount -= max(reserves[id], 0)
		if amount <= 0 {
			continue
		}
		if data == nil {
			return 0, false
		}
		tool, known := data.UnitIsTool(int64(id))
		if !known {
			return 0, false
		}
		if !tool {
			total += amount
		}
	}
	return total, true
}
func trackedStationRemainder(snapshot Snapshot, castle State.CastleState, reserves []reserveSetting) (active, fresh bool, remaining int64, known bool) {
	var updated time.Time
	for _, op := range snapshot.State.Stationing {
		if op.SourceCastleID == castle.ID && op.ActiveInState(snapshot.State, snapshot.Now) {
			active = true
			if op.UpdatedAt.After(updated) {
				updated = op.UpdatedAt
			}
		}
	}
	if !active {
		return false, false, 0, false
	}
	if castle.UnitsObservedAt.IsZero() || castle.UnitsObservedAt.Before(updated) || castle.UnitsObservedAt.After(snapshot.Now) || snapshot.Now.Sub(castle.UnitsObservedAt) > 30*time.Second {
		return true, false, 0, false
	}
	reserved := map[State.UnitID]int64{}
	for _, r := range reserves {
		reserved[r.ID] = r.Amount
	}
	remaining, known = EligibleStationRemainder(snapshot.GameData, castle, reserved)
	return true, true, remaining, known
}
func refreshTrackedStationInventory(snapshot Snapshot, castle State.CastleState) Decision {
	args, _ := json.Marshal(map[string]any{"castleId": castle.ID, "refresh": true})
	return Decision{Status: "waiting", Detail: "Refreshing the castle after a tracked evacuation to check for unsent troops", NextCheckAt: snapshot.Now.Add(3 * time.Second), Request: &Intent.Request{Name: "castle.focus", Arguments: args}, ReevaluateOnSuccess: true}
}

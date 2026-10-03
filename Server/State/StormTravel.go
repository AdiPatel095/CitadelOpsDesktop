package State

import (
	"fmt"
	"math"
	"time"
)

// StormTravelObservation retains official outbound TT reports after movements
// disappear. Each route/option keeps its newest report and its slowest rate.
type StormTravelObservation struct {
	SourceCastleID        CastleID   `json:"sourceCastleId"`
	SourceX               int        `json:"sourceX"`
	SourceY               int        `json:"sourceY"`
	TargetX               int        `json:"targetX"`
	TargetY               int        `json:"targetY"`
	HorseBoosterWID       *int64     `json:"horseBoosterWid,omitempty"`
	MovementID            MovementID `json:"movementId"`
	TravelSeconds         int        `json:"travelSeconds"`
	StartedAt             time.Time  `json:"startedAt"`
	EventEndsAt           time.Time  `json:"eventEndsAt"`
	SlowestSecondsPerTile float64    `json:"slowestSecondsPerTile"`
}

// An absolute official countdown remains authoritative until its deadline.
// Pre-connection, future, missing, closed and expired observations are stale.
// A partial UT/KUT update cannot refresh the UL deadline's own observation.
func (state *GameState) StormEventEndAt(now time.Time) (time.Time, bool) {
	row, found := state.KingdomTransport.Unlocks[stormKingdomID]
	if !found || !row.Unlocked || row.EventEndsAt.IsZero() || !now.Before(row.EventEndsAt) ||
		row.EventEndObservedAt.IsZero() || row.EventEndObservedAt.After(now) ||
		state.Session.ConnectionGeneration != row.EventEndConnectionGeneration ||
		!state.Session.ChangedAt.IsZero() && row.EventEndObservedAt.Before(state.Session.ChangedAt) {
		return time.Time{}, false
	}
	return row.EventEndsAt, true
}

func (state *GameState) ObserveStormTravel(movement MovementState) bool {
	end, known := state.StormEventEndAt(movement.ObservedAt)
	if !known || state.Player.ID <= 0 || movement.OwnerPlayerID != state.Player.ID ||
		movement.KingdomID != stormKingdomID || movement.Direction != 0 ||
		(movement.TargetTypeID != MapTypeStormIsland && movement.TargetTypeID != MapTypeStormFort) ||
		movement.TravelSeconds < 0 || movement.StartedAt.IsZero() ||
		movement.StartedAt.Before(state.KingdomTransport.Unlocks[stormKingdomID].EventObservedFrom) {
		return false
	}
	source, found := state.Castles[movement.SourceCastleID]
	if !found || source.KingdomID != stormKingdomID || source.X != movement.SourceX || source.Y != movement.SourceY {
		return false
	}
	distance := math.Hypot(float64(movement.TargetX-movement.SourceX), float64(movement.TargetY-movement.SourceY))
	if distance <= 0 {
		return false
	}
	option := "unknown"
	if movement.HorseBoosterWID != nil {
		option = fmt.Sprint(*movement.HorseBoosterWID)
	}
	key := fmt.Sprintf("%d:%d:%d:%s", source.ID, movement.TargetX, movement.TargetY, option)
	previous, found := state.Storm.TravelObservations[key]
	rate := float64(movement.TravelSeconds) / distance
	if found && SameEventOccurrence(previous.EventEndsAt, end) &&
		(previous.StartedAt.After(movement.StartedAt) || previous.StartedAt.Equal(movement.StartedAt) && (previous.MovementID > movement.ID || previous.MovementID == movement.ID && previous.TravelSeconds == movement.TravelSeconds)) {
		// GAM rows need not be ordered by launch. An older movement can still
		// increase the conservative rate without replacing the newest target TT.
		if rate > previous.SlowestSecondsPerTile {
			previous.SlowestSecondsPerTile = rate
			state.mutableStormTravelObservations()[key] = previous
			return true
		}
		return false
	}
	if found && SameEventOccurrence(previous.EventEndsAt, end) {
		rate = max(rate, previous.SlowestSecondsPerTile)
	}
	observations := state.mutableStormTravelObservations()
	for route, observation := range observations {
		if !SameEventOccurrence(observation.EventEndsAt, end) {
			delete(observations, route)
		}
	}
	observations[key] = StormTravelObservation{
		SourceCastleID: source.ID, SourceX: source.X, SourceY: source.Y,
		TargetX: movement.TargetX, TargetY: movement.TargetY, HorseBoosterWID: movement.HorseBoosterWID,
		MovementID: movement.ID, TravelSeconds: movement.TravelSeconds, StartedAt: movement.StartedAt,
		EventEndsAt: end, SlowestSecondsPerTile: rate,
	}
	return true
}

func (state *GameState) mutableStormTravelObservations() map[string]StormTravelObservation {
	if state.stormMutationCOW && state.mutableStormParts&stormTravelMutable == 0 {
		state.Storm.TravelObservations = cloneMap(state.Storm.TravelObservations)
		state.mutableStormParts |= stormTravelMutable
	}
	if state.Storm.TravelObservations == nil {
		state.Storm.TravelObservations = map[string]StormTravelObservation{}
	}
	return state.Storm.TravelObservations
}

package Automation

import (
	"math"
	"strconv"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

// Covers CRA pacing (4–6 seconds), multi-wave launches and network receipt.
const stormDispatchAllowance = 60 * time.Second

func stormEndUnavailable() *Localization.Message {
	return Localization.New("server.storm.arrival.end_unavailable", "Waiting for the game to confirm the Storm event end", nil)
}

func stormArrivalTooLate() *Localization.Message {
	return Localization.New("server.storm.arrival.too_late", "Skip this move: it cannot arrive before the Storm event ends", nil)
}

func stormTravelUnavailable() *Localization.Message {
	return Localization.New("server.storm.arrival.travel_unavailable", "Waiting for a reported Storm travel time before sending this attack", nil)
}

func StormAttackArrivalBlock(state *State.GameState, gameData *GameData.Store, source State.CastleState, target State.MapObservation, horseSelection int, now time.Time) *Localization.Message {
	end, known := state.StormEventEndAt(now)
	if !known {
		return stormEndUnavailable()
	}
	option := int64(-1)
	if tier, selected := GameData.HorseTravelBoostTierForSelection(horseSelection); selected {
		if gameData == nil {
			return stormTravelUnavailable()
		}
		definition, err := gameData.ResolveHorseTravelBoost(source, tier)
		if err != nil {
			return Localization.FromError(err)
		}
		option = definition.ID
	}
	seconds, observed := stormObservedTravelSeconds(state, source, target, option, end)
	if !observed {
		if stormUnknownEtaAllowed(now, end) {
			return nil
		}
		return stormTravelUnavailable()
	}
	// Reject invalid/overflowing rates instead of allowing duration conversion to
	// wrap to a negative arrival time. Round upward, never toward an earlier ETA.
	if math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds < 0 || seconds >= float64(math.MaxInt64/int64(time.Second)) {
		return stormArrivalTooLate()
	}
	travel := time.Duration(math.Ceil(seconds)) * time.Second
	if !now.Add(travel).Add(stormDispatchAllowance).Before(end) {
		return stormArrivalTooLate()
	}
	return nil
}

func stormObservedTravelSeconds(state *State.GameState, source State.CastleState, target State.MapObservation, option int64, end time.Time) (float64, bool) {
	var latest State.StormTravelObservation
	slowest := float64(0)
	rateKnown := false
	for _, observation := range state.Storm.TravelObservations {
		if observation.SourceCastleID != source.ID || observation.SourceX != source.X || observation.SourceY != source.Y ||
			!State.SameEventOccurrence(observation.EventEndsAt, end) || observation.TravelSeconds < 0 || observation.StartedAt.IsZero() {
			continue
		}
		rateKnown = true
		slowest = max(slowest, observation.SlowestSecondsPerTile)
		if observation.TargetX == target.X && observation.TargetY == target.Y && observation.HorseBoosterWID != nil && *observation.HorseBoosterWID == option &&
			(latest.StartedAt.IsZero() || observation.StartedAt.After(latest.StartedAt) || observation.StartedAt.Equal(latest.StartedAt) && observation.MovementID > latest.MovementID) {
			latest = observation
		}
	}
	if !latest.StartedAt.IsZero() {
		return float64(latest.TravelSeconds), true
	}
	if !rateKnown {
		return 0, false
	}
	distance := math.Hypot(float64(target.X-source.X), float64(target.Y-source.Y))
	return slowest * distance, true
}

func StormKingdomArrivalBlock(state *State.GameState, gameData *GameData.Store, destination State.KingdomID, now time.Time) *Localization.Message {
	if destination != GameData.StormKingdomID {
		return nil
	}
	end, known := state.StormEventEndAt(now)
	if !known {
		return stormEndUnavailable()
	}
	if gameData == nil {
		return stormTravelUnavailable()
	}
	catalog, err := gameData.Catalog("kingdoms")
	if err != nil {
		return stormTravelUnavailable()
	}
	seconds, found := catalog.Int64(strconv.FormatInt(int64(destination), 10), "unitTravelTime")
	if !found || seconds < 0 || seconds > math.MaxInt64/int64(time.Second) {
		return stormTravelUnavailable()
	}
	if !now.Add(time.Duration(seconds) * time.Second).Add(stormDispatchAllowance).Before(end) {
		return stormArrivalTooLate()
	}
	return nil
}

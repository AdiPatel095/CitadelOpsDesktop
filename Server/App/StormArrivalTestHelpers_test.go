package App

import (
	"CitadelDesktop/Server/State"
	"time"
)

func stormArrivalTestUnlock(now time.Time, generation uint64) State.KingdomTransportUnlock {
	return State.KingdomTransportUnlock{
		KingdomID: 4, Unlocked: true, EventEndsAt: now.Add(24 * time.Hour),
		EventObservedFrom: now.Add(-time.Hour), EventEndObservedAt: now,
		EventEndConnectionGeneration: generation,
	}
}

func fundStormArrivalForTest(state *State.GameState, now time.Time) {
	state.Session.ChangedAt = now.Add(-time.Hour)
	state.KingdomTransport.Unlocks[4] = stormArrivalTestUnlock(now, state.Session.ConnectionGeneration)
	state.Storm.TravelObservations = map[string]State.StormTravelObservation{}
	option := int64(-1)
	for _, source := range state.Castles {
		if source.KingdomID != 4 {
			continue
		}
		state.Storm.TravelObservations["synthetic-route"] = State.StormTravelObservation{
			SourceCastleID: source.ID, SourceX: source.X, SourceY: source.Y,
			TargetX: source.X + 1, TargetY: source.Y, HorseBoosterWID: &option,
			MovementID: 91001, TravelSeconds: 1, StartedAt: now.Add(-time.Minute),
			EventEndsAt: now.Add(24 * time.Hour), SlowestSecondsPerTile: 1,
		}
	}
}

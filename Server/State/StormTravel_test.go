package State

import (
	"testing"
	"time"
)

func TestStormTravelCopyOnWriteAndNewEvent(t *testing.T) {
	now := time.Now().UTC()
	state := NewGameState()
	state.Player.ID = 71001
	state.Session.ChangedAt = now.Add(-time.Hour)
	state.Castles[81001] = CastleState{ID: 81001, KingdomID: 4, X: 100, Y: 100}
	state.KingdomTransport.Unlocks[4] = KingdomTransportUnlock{KingdomID: 4, Unlocked: true,
		EventEndsAt: now.Add(time.Hour), EventEndObservedAt: now.Add(-time.Minute), EventObservedFrom: now.Add(-time.Hour)}
	option := int64(-1)
	movement := MovementState{ID: 91001, SourceCastleID: 81001, OwnerPlayerID: 71001, KingdomID: 4,
		SourceX: 100, SourceY: 100, TargetX: 110, TargetY: 100, TargetTypeID: 25, TravelSeconds: 180,
		StartedAt: now.Add(-30 * time.Second), ObservedAt: now, HorseBoosterWID: &option}
	if !state.ObserveStormTravel(movement) {
		t.Fatal("initial travel not observed")
	}
	store := NewStore(&state)
	before := store.ReadOnlyView()
	_, err := store.ApplyComponents(Components(ComponentStorm), func(current *GameState) ([]string, bool, error) {
		movement.ID, movement.TargetX = 91002, 120
		return []string{"storm"}, current.ObserveStormTravel(movement), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(before.Storm.TravelObservations) != 1 || len(store.ReadOnlyView().Storm.TravelObservations) != 2 {
		t.Fatal("travel mutation changed an earlier generation")
	}
	snapshot := store.Snapshot()
	for _, observation := range snapshot.Storm.TravelObservations {
		*observation.HorseBoosterWID = 123456
	}
	for _, observation := range store.ReadOnlyView().Storm.TravelObservations {
		if *observation.HorseBoosterWID != -1 {
			t.Fatal("public snapshot mutated internal travel options")
		}
	}
	_, err = store.ApplyComponents(Components(ComponentStorm, ComponentKingdomTransport), func(current *GameState) ([]string, bool, error) {
		row := current.KingdomTransport.Unlocks[4]
		row.EventEndsAt = now.Add(30 * 24 * time.Hour)
		current.KingdomTransport.Unlocks[4] = row
		movement.ID, movement.TargetX = 91003, 130
		return []string{"storm"}, current.ObserveStormTravel(movement), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(store.ReadOnlyView().Storm.TravelObservations) != 1 {
		t.Fatal("new event retained old event reports")
	}
}

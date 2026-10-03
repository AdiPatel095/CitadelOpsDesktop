package App

import (
	"CitadelDesktop/Server/State"
	"time"
)

func fundTravelTicketsForTest(state *State.GameState) {
	if state.Session.ConnectionGeneration == 0 {
		state.Session.ConnectionGeneration = 1
	}
	state.Player.Currencies[22] = 1000
	state.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: state.Session.ConnectionGeneration}
}

func travelTicketTestStore(initial *State.GameState) *State.Store {
	store := State.NewStore(initial)
	_, err := store.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.CurrencyObservations[22] = State.PlayerResourceObservation{ObservedAt: time.Now().UTC(), ConnectionGeneration: state.Session.ConnectionGeneration}
		return []string{"currencies"}, true, nil
	})
	if err != nil {
		panic(err)
	}
	return store
}

package App

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestStatePersistenceFenceCarriesEventPublishedBeforeSubscription(t *testing.T) {
	stateStore := State.NewStore(State.NewGameState())
	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	event, err := stateStore.ApplyComponents(
		State.Components(State.ComponentInvasion),
		func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: 71, OccurrenceEndsAt: reservedAt.Add(time.Hour),
				TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
				SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
				OperationID: "pre-subscriber-cra", ReservedAt: reservedAt,
			})
			return []string{"invasion"}, true, nil
		},
	)
	if err != nil || event.Patch == nil {
		t.Fatalf("stage pre-subscriber event: event=%#v err=%v", event, err)
	}

	application := &Application{
		DataDir:              t.TempDir(),
		State:                stateStore,
		statePersistence:     make(chan statePersistenceRequest),
		statePersistenceDone: make(chan struct{}),
	}
	// Reproduce the old startup window deterministically: the event has already
	// been published, but the worker has not subscribed yet.
	application.statePersistenceStarted.Store(true)
	result := make(chan error, 1)
	go func() {
		result <- application.saveStateEvent(context.Background(), event)
	}()

	workerContext, cancelWorker := context.WithCancel(context.Background())
	ready := make(chan struct{})
	go application.persistState(workerContext, ready)
	<-ready
	if err := <-result; err != nil {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatal(err)
	}

	persisted, err := State.LoadSnapshot(application.DataDir)
	if err != nil {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatal(err)
	}
	reservation, found := persisted.Invasion.TargetReservation(0, 101, 102)
	if !found || reservation.OperationID != "pre-subscriber-cra" || persisted.Revision < event.Revision {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatalf("pre-subscriber event was not durable: revision=%d reservation=%#v found=%t", persisted.Revision, reservation, found)
	}

	cancelWorker()
	<-application.statePersistenceDone
}

func TestStatePersistenceLaterFenceCannotSkipEarlierSparsePatch(t *testing.T) {
	dataDir := t.TempDir()
	stateStore := State.NewStore(State.NewGameState())
	bootstrap, err := stateStore.ApplyComponents(
		State.Components(State.ComponentPlayer),
		func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Player.Name = "before"
			return []string{"player"}, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := State.SaveComponentSnapshot(dataDir, bootstrap, State.Components(bootstrap.Components...)); err != nil {
		t.Fatal(err)
	}

	reservedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	invasionEvent, err := stateStore.ApplyComponents(
		State.Components(State.ComponentInvasion),
		func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
				KingdomID: 0, EventID: 71, OccurrenceEndsAt: reservedAt.Add(time.Hour),
				TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
				SourceCastleID: 1, CommanderID: 7, CommanderKnown: true,
				OperationID: "earlier-invasion", ReservedAt: reservedAt,
			})
			return []string{"invasion"}, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	playerEvent, err := stateStore.ApplyComponents(
		State.Components(State.ComponentPlayer),
		func(gameState *State.GameState) ([]string, bool, error) {
			gameState.Player.Name = "after"
			return []string{"player"}, true, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	requests := make(chan statePersistenceRequest, 2)
	playerResult := make(chan error, 1)
	invasionResult := make(chan error, 1)
	// Queue the later sparse fence first. A scalar revision shortcut used to
	// acknowledge the earlier invasion event afterward without ever writing it.
	requests <- statePersistenceRequest{event: playerEvent, result: playerResult}
	requests <- statePersistenceRequest{event: invasionEvent, result: invasionResult}
	application := &Application{
		DataDir: dataDir, State: stateStore,
		statePersistence: requests, statePersistenceDone: make(chan struct{}),
	}
	workerContext, cancelWorker := context.WithCancel(context.Background())
	ready := make(chan struct{})
	go application.persistState(workerContext, ready)
	<-ready
	if err := <-playerResult; err != nil {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatal(err)
	}
	if err := <-invasionResult; err != nil {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatal(err)
	}

	persisted, err := State.LoadSnapshot(dataDir)
	if err != nil {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatal(err)
	}
	reservation, found := persisted.Invasion.TargetReservation(0, 101, 102)
	if persisted.Player.Name != "after" || !found || reservation.OperationID != "earlier-invasion" ||
		persisted.Revision < playerEvent.Revision {
		cancelWorker()
		<-application.statePersistenceDone
		t.Fatalf("out-of-order fences lost sparse state: revision=%d player=%q reservation=%#v found=%t",
			persisted.Revision, persisted.Player.Name, reservation, found)
	}

	cancelWorker()
	<-application.statePersistenceDone
}

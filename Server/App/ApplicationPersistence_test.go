package App

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/synctest"
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

// startPersistenceTestWorker uses the same readiness and shutdown handshake as production.
func startPersistenceTestWorker(t *testing.T, directory string, store *State.Store, window time.Duration) *Application {
	t.Helper()
	app := &Application{DataDir: directory, State: store, statePersistenceWindow: window,
		statePersistence: make(chan statePersistenceRequest), statePersistenceDone: make(chan struct{})}
	if got := app.StatePersistenceStats(); got != (State.PersistenceStats{}) {
		t.Fatalf("stats before start: %+v", got)
	}
	ctx, cancel := context.WithCancel(t.Context())
	ready := make(chan struct{})
	go app.persistState(ctx, ready)
	<-ready
	app.statePersistenceStarted.Store(true)
	t.Cleanup(func() { cancel(); <-app.statePersistenceDone })
	return app
}

func applyPersistenceTestChange(t *testing.T, store *State.Store, components State.ComponentSet, change func(*State.GameState)) State.Event {
	t.Helper()
	event, err := store.ApplyComponents(components, func(state *State.GameState) ([]string, bool, error) {
		change(state)
		return []string{"persistence-test"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return event
}

func TestStatePersistenceSyncBudget(t *testing.T) {
	if testing.Short() {
		t.Skip("scaled three-minute persistence simulation")
	}
	const scale = 60
	const simulatedSeconds = 180
	window := defaultStatePersistenceWindow / scale
	store := State.NewStore(State.NewGameState())
	app := startPersistenceTestWorker(t, t.TempDir(), store, window)
	initial := applyPersistenceTestChange(t, store, State.Components(State.ComponentAutomations), func(state *State.GameState) {
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("p%d", i)
			state.Automations[id] = State.AutomationState{ID: id, Status: "waiting"}
		}
	})
	if err := app.saveStateEvent(t.Context(), initial); err != nil {
		t.Fatal(err)
	}
	baseline := app.StatePersistenceStats()
	previous := baseline
	forced := 0
	previousForced := 0
	started := time.Now()
	for second := 5; second <= simulatedSeconds; second += 5 {
		time.Sleep(time.Until(started.Add(time.Duration(second) * time.Second / scale)))
		components := State.Components(State.ComponentPlayer, State.ComponentCastles)
		if second%10 == 0 {
			components = components.Union(State.Components(State.ComponentMovements))
		}
		if second%20 == 0 {
			components = components.Union(State.Components(State.ComponentCommanders))
		}
		if second%15 == 0 {
			components = components.Union(State.Components(State.ComponentAutomations))
		}
		if second%60 == 0 {
			components = components.Union(State.Components(State.ComponentWorldMap))
		}
		if second%90 == 0 {
			components = components.Union(State.Components(State.ComponentInvasion))
		}
		event := applyPersistenceTestChange(t, store, components, func(state *State.GameState) {
			state.Player.Currencies[1] = float64(second)
			state.SetCastleParts(1, State.CastleState{ID: 1, Resources: map[State.ResourceID]State.ResourceBalance{1: {Amount: float64(second)}}}, State.CastlePartResources)
			if second%10 == 0 {
				if second%20 == 0 {
					state.DeleteMovement(1)
				} else {
					state.SetMovement(1, State.MovementState{ID: 1})
				}
			}
			if second%20 == 0 {
				state.Commanders[1] = State.CommanderState{ID: 1, Available: second%40 == 0}
			}
			if second%15 == 0 {
				id := fmt.Sprintf("p%d", (second/15-1)%10)
				automation := state.Automations[id]
				next := started.Add(time.Duration(second+15) * time.Second / scale)
				automation.NextCheckAt = &next
				automation.UpdatedAt = next
				state.Automations[id] = automation
			}
			if second%60 == 0 {
				automation := state.Automations["p0"]
				automation.Status = fmt.Sprintf("minute-%d", second/60)
				state.Automations["p0"] = automation
				for i := 0; i < 50; i++ {
					state.SetMapObservation(State.MapObservation{KingdomID: 0, X: i, Y: 1, TypeID: State.MapTypeForeignLord, Level: second})
				}
			}
			if second%90 == 0 {
				state.Invasion.ReserveTarget(State.InvasionTargetReservation{
					KingdomID: 0, EventID: 71, TargetTypeID: State.MapTypeForeignLord, X: second, Y: 1,
					SourceCastleID: 1, OperationID: fmt.Sprintf("budget-%d", second), ReservedAt: time.Now().UTC(),
				})
			}
		})
		if second%90 == 0 {
			if err := app.saveStateEvent(t.Context(), event); err != nil {
				t.Fatal(err)
			}
			forced++
		}
		if second%60 == 0 {
			// Each minute is observed once. The final event is forced, so no tail is lost.
			stats := app.StatePersistenceStats()
			t.Logf("minute=%d flushes=%d forced=%d fileSyncs=%d directorySyncs=%d skippedVolatileWrites=%d", second/60,
				stats.Flushes-previous.Flushes, forced-previousForced, stats.FileSyncs-previous.FileSyncs,
				stats.DirectorySyncs-previous.DirectorySyncs, stats.SkippedVolatileWrites-previous.SkippedVolatileWrites)
			previous, previousForced = stats, forced
		}
	}
	stats := app.StatePersistenceStats()
	maxFlushes := uint64(forced + int((simulatedSeconds*time.Second+defaultStatePersistenceWindow-1)/defaultStatePersistenceWindow) + 1)
	if stats.Flushes-baseline.Flushes > maxFlushes {
		t.Fatalf("flushes=%d budget=%d", stats.Flushes-baseline.Flushes, maxFlushes)
	}
	if stats.SkippedVolatileWrites-baseline.SkippedVolatileWrites == 0 {
		t.Fatal("budget stream skipped no volatile automation writes")
	}
	t.Logf("total flushes=%d forced=%d fileSyncs=%d directorySyncs=%d skippedVolatileWrites=%d", stats.Flushes-baseline.Flushes,
		forced, stats.FileSyncs-baseline.FileSyncs, stats.DirectorySyncs-baseline.DirectorySyncs, stats.SkippedVolatileWrites-baseline.SkippedVolatileWrites)
}

func TestStatePersistenceWaitsForTheWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const window = 300 * time.Millisecond
		store := State.NewStore(State.NewGameState())
		directory := t.TempDir()
		app := startPersistenceTestWorker(t, directory, store, window)
		initial := applyPersistenceTestChange(t, store, State.Components(State.ComponentPlayer), func(state *State.GameState) { state.Player.Name = "before" })
		if err := app.saveStateEvent(t.Context(), initial); err != nil {
			t.Fatal(err)
		}
		event := applyPersistenceTestChange(t, store, State.Components(State.ComponentPlayer), func(state *State.GameState) { state.Player.Name = "after" })
		// Wait until the event has armed the timer, then advance only fake time.
		synctest.Wait()
		time.Sleep(window - time.Nanosecond)
		synctest.Wait()
		loaded, err := State.LoadSnapshot(directory)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Revision != initial.Revision || loaded.Player.Name != "before" {
			t.Fatalf("background change persisted before window: %+v", loaded.Player)
		}
		time.Sleep(time.Nanosecond)
		synctest.Wait()
		loaded, err = State.LoadSnapshot(directory)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Revision != event.Revision {
			t.Fatalf("revision at window boundary=%d want=%d", loaded.Revision, event.Revision)
		}
		if loaded.Player.Name != "after" {
			t.Fatalf("background change did not persist: %+v", loaded.Player)
		}
		if app.statePersistenceWindowDuration() != window || (&Application{}).statePersistenceWindowDuration() != 15*time.Second {
			t.Fatal("wrong persistence window")
		}
	})
}

// Copy only the crash image: cancelling a worker would perform a graceful flush.
func copyPersistenceTestImage(t *testing.T, source string) string {
	t.Helper()
	target := t.TempDir()
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		destination := filepath.Join(target, relative)
		if entry.IsDir() {
			return os.MkdirAll(destination, 0o700)
		}
		contents, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(destination, contents, 0o600)
	})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

func TestStatePersistenceCrashLosesAtMostTheWindowAndRecovers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const window = 300 * time.Millisecond
		store := State.NewStore(State.NewGameState())
		app := startPersistenceTestWorker(t, t.TempDir(), store, window)
		a := applyPersistenceTestChange(t, store, State.Components(State.ComponentPlayer), func(state *State.GameState) { state.Player.Name = "A" })
		if err := app.saveStateEvent(t.Context(), a); err != nil {
			t.Fatal(err)
		}
		b := applyPersistenceTestChange(t, store, State.Components(State.ComponentPlayer), func(state *State.GameState) { state.Player.Name = "B" })
		// Quiesce the worker without advancing its fake timer. The clock stays
		// frozen during filesystem I/O, so no flush can replace files mid-copy.
		synctest.Wait()
		image2 := copyPersistenceTestImage(t, app.DataDir)
		loaded, err := State.LoadSnapshot(image2)
		if err != nil {
			t.Fatal(err)
		}
		if loaded.Revision != a.Revision || loaded.Player.Name != "A" {
			t.Fatalf("pre-window crash image: revision=%d name=%s", loaded.Revision, loaded.Player.Name)
		}
		reservedAt := time.Now().UTC()
		c := applyPersistenceTestChange(t, store, State.Components(State.ComponentInvasion), func(state *State.GameState) {
			state.Invasion.ReserveTarget(State.InvasionTargetReservation{KingdomID: 0, EventID: 71,
				TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102, SourceCastleID: 1,
				OperationID: "crash-C", ReservedAt: reservedAt})
		})
		if err := app.saveStateEvent(t.Context(), c); err != nil {
			t.Fatal(err)
		}
		image3 := copyPersistenceTestImage(t, app.DataDir)
		forced, err := State.LoadSnapshot(image3)
		if err != nil {
			t.Fatal(err)
		}
		reservation, found := forced.Invasion.TargetReservation(0, 101, 102)
		if forced.Revision != c.Revision || forced.Revision <= b.Revision || forced.Player.Name != "B" || !found || reservation.OperationID != "crash-C" {
			t.Fatalf("forced crash image lost B or C: revision=%d player=%s reservation=%+v", forced.Revision, forced.Player.Name, reservation)
		}
		recovered := State.NewStore(loaded)
		startPersistenceTestWorker(t, image2, recovered, window)
		d := applyPersistenceTestChange(t, recovered, State.Components(State.ComponentPlayer), func(state *State.GameState) { state.Player.Name = "D" })
		if d.Revision != loaded.Revision+1 {
			t.Fatalf("recovery revision=%d want=%d", d.Revision, loaded.Revision+1)
		}
		synctest.Wait()
		time.Sleep(window)
		synctest.Wait()
		durable, err := State.LoadSnapshot(image2)
		if err != nil {
			t.Fatal(err)
		}
		if durable.Revision != d.Revision {
			t.Fatalf("recovered durable revision=%d want=%d", durable.Revision, d.Revision)
		}
		if durable.Player.Name != "D" {
			t.Fatalf("recovery lost D: %s", durable.Player.Name)
		}
		if _, found := durable.Invasion.TargetReservation(0, 101, 102); found {
			t.Fatal("recovery included C absent from crash image")
		}
		entries, err := os.ReadDir(image2)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "Components.superseded-") {
				t.Fatalf("recovery quarantined image: %s", entry.Name())
			}
		}
	})
}

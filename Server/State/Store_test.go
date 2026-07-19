package State

import (
	"reflect"
	"testing"
	"time"
)

func TestStoreHotPathMetadataDoesNotWaitForLongMutation(t *testing.T) {
	initial := NewGameState()
	initial.Session.ConnectionGeneration = 7
	store := NewStore(initial)
	mutationStarted := make(chan struct{})
	releaseMutation := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		_, _ = store.Apply(func(*GameState) ([]string, bool, error) {
			close(mutationStarted)
			<-releaseMutation
			return []string{"slow"}, true, nil
		})
	}()
	<-mutationStarted

	read := make(chan struct{})
	go func() {
		_ = store.Revision()
		if session := store.Session(); session.ConnectionGeneration != 7 {
			t.Errorf("connection generation = %d, want 7", session.ConnectionGeneration)
		}
		close(read)
	}()
	select {
	case <-read:
	case <-time.After(50 * time.Millisecond):
		t.Fatal("revision/session metadata blocked behind a state mutation")
	}
	close(releaseMutation)
	<-finished
}

func TestStoreCoalescesFullSubscriberBuffer(t *testing.T) {
	store := NewStore(NewGameState())
	events, unsubscribe := store.Subscribe(1)
	defer unsubscribe()

	for _, domain := range []string{"units", "movements", "beri"} {
		if _, err := store.Apply(func(*GameState) ([]string, bool, error) {
			return []string{domain}, true, nil
		}); err != nil {
			t.Fatalf("apply %s mutation: %v", domain, err)
		}
	}

	event := <-events
	if event.Revision != 3 {
		t.Fatalf("coalesced revision = %d, want 3", event.Revision)
	}
	if event.Sequence != 3 || !event.Gap {
		t.Fatalf("coalesced stream metadata = sequence %d gap %t, want 3/true", event.Sequence, event.Gap)
	}
	if want := []string{"beri", "movements", "units"}; !reflect.DeepEqual(event.Domains, want) {
		t.Fatalf("coalesced domains = %v, want %v", event.Domains, want)
	}
	if event.OccurredAt.IsZero() {
		t.Fatal("coalesced event has no occurrence time")
	}
}

func TestStoreOnlyCoalescesSubscribersWithFullBuffers(t *testing.T) {
	store := NewStore(NewGameState())
	coalescedEvents, unsubscribeCoalesced := store.Subscribe(1)
	defer unsubscribeCoalesced()
	discreteEvents, unsubscribeDiscrete := store.Subscribe(2)
	defer unsubscribeDiscrete()

	for _, domain := range []string{"units", "movements"} {
		if _, err := store.Apply(func(*GameState) ([]string, bool, error) {
			return []string{domain}, true, nil
		}); err != nil {
			t.Fatalf("apply %s mutation: %v", domain, err)
		}
	}

	if got, want := (<-coalescedEvents).Domains, []string{"movements", "units"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("coalesced subscriber domains = %v, want %v", got, want)
	}
	first, second := <-discreteEvents, <-discreteEvents
	if first.Revision != 1 || !reflect.DeepEqual(first.Domains, []string{"units"}) {
		t.Fatalf("first discrete event = %#v", first)
	}
	if second.Revision != 2 || !reflect.DeepEqual(second.Domains, []string{"movements"}) {
		t.Fatalf("second discrete event = %#v", second)
	}
	if first.Sequence != 1 || second.Sequence != 2 || first.Gap || second.Gap {
		t.Fatalf("discrete stream metadata = %#v / %#v", first, second)
	}
}

func TestPlanningViewKeepsImmutableGenerationAfterMutation(t *testing.T) {
	initial := NewGameState()
	initial.Castles[11] = CastleState{
		ID: 11, KingdomID: 1, Name: "before",
		Resources: map[ResourceID]ResourceBalance{3: {Amount: 100}},
		Buildings: map[BuildingInstanceID]Building{},
	}
	store := NewStore(initial)
	view := store.PlanningView()

	if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
		castle := state.Castles[11]
		castle.Name = "after"
		balance := castle.Resources[3]
		balance.Amount = 250
		castle.Resources[3] = balance
		state.Castles[11] = castle
		return []string{"castles", "resources"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}

	if castle := view.State.Castles[11]; castle.Name != "before" || castle.Resources[3].Amount != 100 {
		t.Fatalf("old planning generation changed: %#v", castle)
	}
	if castle := store.PlanningView().State.Castles[11]; castle.Name != "after" || castle.Resources[3].Amount != 250 {
		t.Fatalf("current planning generation = %#v", castle)
	}
}

func TestApplyScopedAdvancesOnlyDeclaredPartition(t *testing.T) {
	initial := NewGameState()
	initial.Session.ServerURL = "https://example.invalid"
	initial.Player.ID = 7
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1}
	initial.Castles[12] = CastleState{ID: 12, KingdomID: 1}
	store := NewStore(initial)
	changedKey := CastlePartition(initial, CapabilityConstruction, 11)
	unchangedKey := CastlePartition(initial, CapabilityConstruction, 12)

	event, err := store.ApplyScoped(func(state *GameState) (ScopedChange, error) {
		state.Player.Level++
		return ScopedChange{Partitions: []PartitionKey{changedKey}, Changed: true}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	versions := store.PlanningView().Partitions
	if got := versions.Version(changedKey); got != 1 {
		t.Fatalf("changed partition version = %d, want 1", got)
	}
	if got := versions.Version(unchangedKey); got != 0 {
		t.Fatalf("unrelated partition version = %d, want 0", got)
	}
	if len(event.Partitions) != 1 || event.Partitions[0].Key.Canonical() != changedKey.Canonical() {
		t.Fatalf("event partitions = %#v", event.Partitions)
	}

	if _, err := store.ApplyScoped(func(*GameState) (ScopedChange, error) {
		return ScopedChange{Partitions: []PartitionKey{changedKey}}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if got := store.PlanningView().Partitions.Version(changedKey); got != 1 {
		t.Fatalf("no-op advanced partition to %d", got)
	}
}

func TestProtocolContextUsesExplicitFocusEpoch(t *testing.T) {
	initial := NewGameState()
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	initial.Castles[12] = CastleState{ID: 12, KingdomID: 1}
	store := NewStore(initial)
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 || context.FocusEpoch != 1 {
		t.Fatalf("initial protocol context = %#v", context)
	}

	_, err := store.ApplyScoped(func(state *GameState) (ScopedChange, error) {
		first := state.Castles[11]
		first.Focused = false
		state.Castles[11] = first
		second := state.Castles[12]
		second.Focused = true
		state.Castles[12] = second
		return ScopedChange{
			Partitions: []PartitionKey{SessionPartition(*state, CapabilitySessionContext)},
			Changed:    true,
		}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if context := store.ProtocolContext(); context.FocusedCastleID != 12 || context.FocusEpoch != 2 {
		t.Fatalf("updated protocol context = %#v", context)
	}
}

func TestStoreNormalizesMultipleRecoveredFocusFlagsDeterministically(t *testing.T) {
	initial := NewGameState()
	initial.Castles[12] = CastleState{ID: 12, KingdomID: 1, Focused: true}
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	snapshot := store.Snapshot()
	if !snapshot.Castles[11].Focused || snapshot.Castles[12].Focused {
		t.Fatalf("recovered focus was not normalized: %+v", snapshot.Castles)
	}
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 || context.FocusEpoch != 1 {
		t.Fatalf("normalized protocol context = %+v", context)
	}
}

func TestStorePreservesCurrentFocusWhenMutationSetsMultipleFlags(t *testing.T) {
	initial := NewGameState()
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	initial.Castles[12] = CastleState{ID: 12, KingdomID: 1}
	store := NewStore(initial)
	if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
		second := state.Castles[12]
		second.Focused = true
		state.Castles[12] = second
		return []string{"session-context"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := store.Snapshot()
	if !snapshot.Castles[11].Focused || snapshot.Castles[12].Focused {
		t.Fatalf("current focus was not preserved: %+v", snapshot.Castles)
	}
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 || context.FocusEpoch != 1 {
		t.Fatalf("focus epoch changed for normalized duplicate: %+v", context)
	}
}

func TestPlanningViewPublishesStateAndVersionsAsOneGeneration(t *testing.T) {
	initial := NewGameState()
	store := NewStore(initial)
	key := AccountPartition(initial, CapabilityAccountProfile)
	writerResult := make(chan error, 1)
	go func() {
		for range 1000 {
			if _, err := store.ApplyScoped(func(state *GameState) (ScopedChange, error) {
				state.Player.Level++
				return ScopedChange{Partitions: []PartitionKey{key}, Changed: true}, nil
			}); err != nil {
				writerResult <- err
				return
			}
		}
		writerResult <- nil
	}()
	for {
		select {
		case err := <-writerResult:
			if err != nil {
				t.Fatal(err)
			}
			view := store.PlanningView()
			if uint64(view.State.Player.Level) != view.Partitions.Version(key) {
				t.Fatalf("final generation mismatch: level %d, version %d", view.State.Player.Level, view.Partitions.Version(key))
			}
			return
		default:
			view := store.PlanningView()
			if uint64(view.State.Player.Level) != view.Partitions.Version(key) {
				t.Fatalf("generation mismatch: level %d, version %d", view.State.Player.Level, view.Partitions.Version(key))
			}
		}
	}
}

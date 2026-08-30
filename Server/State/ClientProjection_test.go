package State

import (
	"bytes"
	"encoding/json"
	"slices"
	"testing"
	"time"
)

func TestClientStateSnapshotKeepsOnlyDashboardMapKinds(t *testing.T) {
	state := NewGameState()
	observedAt := time.Now().UTC()
	state.Map[0] = map[string]MapObservation{
		"10:10": {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypePlayerCastle, OwnerID: 91, ObservedAt: observedAt},
		"20:20": {KingdomID: 0, X: 20, Y: 20, TypeID: MapTypeNomadCamp, ObjectID: 92, ObservedAt: observedAt},
	}
	state.Map[4] = map[string]MapObservation{
		"30:30": {KingdomID: 4, X: 30, Y: 30, TypeID: MapTypeRift, ObjectID: 93, ObservedAt: observedAt},
	}
	contents, err := json.Marshal(NewClientStateSnapshot(state))
	if err != nil {
		t.Fatal(err)
	}
	var decoded GameState
	if err := json.Unmarshal(contents, &decoded); err != nil {
		t.Fatal(err)
	}
	if len(decoded.Map) != 1 || decoded.Map[4]["30:30"].ObjectID != 93 {
		t.Fatalf("dashboard map projection = %+v", decoded.Map)
	}
	if decoded.SchemaVersion != state.SchemaVersion || decoded.Session.Status != state.Session.Status {
		t.Fatal("client projection changed the official state contract")
	}
}

func TestClientEventPayloadReusesImmutableEncoding(t *testing.T) {
	store := NewStore(NewGameState())
	event, err := store.ApplyComponents(Components(ComponentSession), func(state *GameState) ([]string, bool, error) {
		state.Session.Generation++
		return []string{"session"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := ClientEventPayload(event)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ClientEventPayload(event)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) == 0 || !bytes.Equal(first, second) || &first[0] != &second[0] {
		t.Fatal("client event encoding was not reused")
	}
	var decoded ClientStateEvent
	if err := json.Unmarshal(first, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Revision != event.Revision || decoded.Patch == nil {
		t.Fatalf("decoded client event = %#v", decoded)
	}
}

func TestClientProjectionPublishesConnectionScopedMovementSnapshot(t *testing.T) {
	observedAt := time.Date(2026, time.August, 14, 13, 30, 0, 0, time.UTC)
	state := NewGameState()
	state.MovementSnapshot = MovementSnapshot{
		Version: 7, ConnectionGeneration: 3, ObservedAt: observedAt,
	}
	contents, err := json.Marshal(NewClientStateSnapshot(state))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot GameState
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.MovementSnapshot != state.MovementSnapshot {
		t.Fatalf("client movement snapshot = %+v, want %+v", snapshot.MovementSnapshot, state.MovementSnapshot)
	}

	store := NewStore(state)
	event, err := store.ApplyComponents(Components(ComponentMovementSnapshot), func(state *GameState) ([]string, bool, error) {
		state.MovementSnapshot.Version++
		state.MovementSnapshot.ObservedAt = observedAt.Add(time.Second)
		return []string{"movement-snapshot"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := ClientEvent(event)
	if projected.Patch == nil || projected.Patch.MovementSnapshot == nil ||
		projected.Patch.MovementSnapshot.Version != 8 ||
		projected.Patch.MovementSnapshot.ConnectionGeneration != 3 {
		t.Fatalf("client movement delta = %+v", projected.Patch)
	}
	if !slices.Contains(projected.Components, ComponentMovementSnapshot) {
		t.Fatalf("client components = %v", projected.Components)
	}
}

func TestClientProjectionPublishesAuthoritativeEventInventory(t *testing.T) {
	observedAt := time.Date(2026, time.August, 15, 15, 0, 0, 0, time.UTC)
	availability := EventAvailability{EventID: 3, EndsAt: observedAt.Add(24 * time.Hour)}
	state := NewGameState()
	state.EventScores.Inventory = EventInventoryState{
		ObservedAt: observedAt, ActiveByEvent: map[int64]EventAvailability{3: availability},
	}

	contents, err := json.Marshal(NewClientStateSnapshot(state))
	if err != nil {
		t.Fatal(err)
	}
	var snapshot GameState
	if err := json.Unmarshal(contents, &snapshot); err != nil {
		t.Fatal(err)
	}
	if got, found := snapshot.EventScores.Inventory.ActiveByEvent[3]; !found || got != availability ||
		!snapshot.EventScores.Inventory.ObservedAt.Equal(observedAt) {
		t.Fatalf("client event inventory = %+v", snapshot.EventScores.Inventory)
	}

	store := NewStore(NewGameState())
	event, err := store.ApplyComponents(Components(ComponentEventScores), func(state *GameState) ([]string, bool, error) {
		changed := state.ReplaceEventInventory(EventInventoryState{
			ObservedAt: observedAt, ActiveByEvent: map[int64]EventAvailability{3: availability},
		})
		return []string{"events"}, changed, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	projected := ClientEvent(event)
	if projected.Patch == nil || projected.Patch.EventScoreChanges == nil ||
		projected.Patch.EventScoreChanges.Inventory == nil {
		t.Fatalf("client event inventory patch = %+v", projected.Patch)
	}
	if got := projected.Patch.EventScoreChanges.Inventory.ActiveByEvent[3]; got != availability {
		t.Fatalf("client event inventory availability = %+v", got)
	}
}

func TestClientEventFiltersBackendOnlyMapDeltas(t *testing.T) {
	rift := MapObservation{KingdomID: 4, X: 30, Y: 30, TypeID: MapTypeRift}
	tower := MapObservation{KingdomID: 0, X: 20, Y: 20, TypeID: MapTypeKingdomTower}
	changes := []MapChange{
		{KingdomID: 0, Key: "20:20", TypeID: tower.TypeID, Observation: &tower},
		{KingdomID: 4, Key: "30:30", TypeID: rift.TypeID, Observation: &rift},
		{KingdomID: 4, Key: "31:31", TypeID: MapTypeRift, Deleted: true},
	}
	event := Event{Patch: &ComponentPatch{MapChanges: &changes}}
	projected := ClientEvent(event)
	if projected.Patch.ComponentPatch == event.Patch {
		t.Fatal("client projection mutated the durable event patch")
	}
	if len(*projected.Patch.MapChanges) != 2 {
		t.Fatalf("client map changes = %+v", *projected.Patch.MapChanges)
	}
	if len(*event.Patch.MapChanges) != 3 {
		t.Fatal("source event was modified")
	}
}

func TestClientEventDefersStormProjectionUntilSharedScanCompletes(t *testing.T) {
	state := NewGameState()
	state.SetMapObservation(MapObservation{
		KingdomID: stormKingdomID, X: 20, Y: 20, TypeID: MapTypeStormFort,
		ObservedAt: time.Now().UTC(),
	})
	generation := &storeGeneration{state: &state}
	change := MapChange{
		KingdomID: stormKingdomID, Key: "20:20", TypeID: MapTypeStormFort,
		Observation: &MapObservation{
			KingdomID: stormKingdomID, X: 20, Y: 20, TypeID: MapTypeStormFort,
			ObservedAt: time.Now().UTC(),
		},
	}
	changes := []MapChange{change}
	patch := &ComponentPatch{
		SchemaVersion: state.SchemaVersion, Revision: 1, UpdatedAt: time.Now().UTC(),
		MapChanges: &changes,
	}

	progress := ClientEvent(Event{
		Revision: 1, Domains: []string{"storm-scan-progress"}, Patch: patch, generation: generation,
	})
	if progress.Patch.Storm != nil {
		t.Fatal("cooperative tile rebuilt the complete client Storm projection")
	}
	if progress.Patch.MapChanges == nil || len(*progress.Patch.MapChanges) != 0 {
		t.Fatalf("backend-only Storm map changes = %+v", progress.Patch.MapChanges)
	}

	completed := ClientEvent(Event{
		Revision: 2, Domains: []string{"storm-scan"}, Patch: &ComponentPatch{
			SchemaVersion: state.SchemaVersion, Revision: 2, UpdatedAt: time.Now().UTC(),
		}, generation: generation,
	})
	if completed.Patch.Storm == nil || completed.Patch.Storm.Map.TargetCount != 1 {
		t.Fatalf("completed Storm projection = %+v", completed.Patch.Storm)
	}
}

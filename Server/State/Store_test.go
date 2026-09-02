package State

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
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

func TestStoreSnapshotKeepsEquipmentEffectsAsArrays(t *testing.T) {
	initial := NewGameState()
	initial.Inventory.Equipment[1] = EquipmentInstance{ID: 1, Effects: EquipmentEffects{}}
	initial.Inventory.Equipment[2] = EquipmentInstance{ID: 2}
	initial.Inventory.Gems[1] = GemInstance{ID: 1, Effects: EquipmentEffects{}}
	initial.Inventory.Gems[2] = GemInstance{ID: 2}

	snapshot := NewStore(initial).Snapshot()
	for id, item := range snapshot.Inventory.Equipment {
		if item.Effects == nil {
			t.Fatalf("equipment %d effects serialized as null", id)
		}
	}
	for id, gem := range snapshot.Inventory.Gems {
		if gem.Effects == nil {
			t.Fatalf("gem %d effects serialized as null", id)
		}
	}
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

func TestApplyComponentsClonesOnlyMutableCastle(t *testing.T) {
	initial := NewGameState()
	initial.Castles[11] = CastleState{
		ID:        11,
		Resources: map[ResourceID]ResourceBalance{3: {Amount: 100}},
		Buildings: map[BuildingInstanceID]Building{101: {InstanceID: 101, Level: 5}},
	}
	initial.Castles[22] = CastleState{
		ID:        22,
		Resources: map[ResourceID]ResourceBalance{3: {Amount: 200}},
		Buildings: map[BuildingInstanceID]Building{202: {InstanceID: 202, Level: 9}},
	}
	store := NewStore(initial)
	before := store.PlanningView().State

	event, err := store.ApplyComponents(Components(ComponentCastles), func(state *GameState) ([]string, bool, error) {
		castle, found := state.MutableCastleParts(11, CastlePartResources|CastlePartBuildings)
		if !found {
			t.Fatal("mutable castle 11 missing")
		}
		balance := castle.Resources[3]
		balance.Amount = 125
		castle.Resources[3] = balance
		castle.Buildings[101] = Building{InstanceID: 101, Level: 6}
		state.SetCastleParts(11, castle, CastlePartResources|CastlePartBuildings)
		return []string{"castles", "resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Patch == nil || event.Patch.Castles != nil || event.Patch.CastleChanges == nil ||
		len(*event.Patch.CastleChanges) != 1 || (*event.Patch.CastleChanges)[0].ID != 11 {
		t.Fatalf("castle delta patch = %#v", event.Patch)
	}
	after := store.PlanningView().State

	if got := before.Castles[11].Resources[3].Amount; got != 100 {
		t.Fatalf("prior castle resource changed to %v", got)
	}
	if got := before.Castles[11].Buildings[101].Level; got != 5 {
		t.Fatalf("prior castle building changed to level %d", got)
	}
	if got := after.Castles[11].Resources[3].Amount; got != 125 {
		t.Fatalf("current castle resource = %v, want 125", got)
	}
	if reflect.ValueOf(before.Castles[11].Resources).Pointer() == reflect.ValueOf(after.Castles[11].Resources).Pointer() {
		t.Fatal("mutated castle retained its prior resource map")
	}
	if reflect.ValueOf(before.Castles[22].Resources).Pointer() != reflect.ValueOf(after.Castles[22].Resources).Pointer() {
		t.Fatal("untouched castle resource map was cloned")
	}
	if reflect.ValueOf(before.Castles[22].Buildings).Pointer() != reflect.ValueOf(after.Castles[22].Buildings).Pointer() {
		t.Fatal("untouched castle building map was cloned")
	}
}

func TestApplyComponentsClonesOnlyMutableInventoryParts(t *testing.T) {
	initial := NewGameState()
	initial.Inventory.ConstructionItems[1] = 4
	initial.Inventory.Equipment[11] = EquipmentInstance{
		ID: 11, Level: 1, Effects: EquipmentEffects{{WireID: 7, Values: []float64{1}}},
	}
	initial.Inventory.Equipment[22] = EquipmentInstance{
		ID: 22, Level: 2, Effects: EquipmentEffects{{WireID: 8, Values: []float64{2}}},
	}
	initial.Inventory.Items["storage:1"] = map[int64]int64{100: 3}
	initial.Inventory.Items["storage:2"] = map[int64]int64{200: 5}
	store := NewStore(initial)
	before := store.PlanningView().State

	event, err := store.ApplyComponents(Components(ComponentInventory), func(state *GameState) ([]string, bool, error) {
		state.MutableInventoryConstructionItems()[1] = 3
		item := state.Inventory.Equipment[11]
		item.Level = 3
		item.Effects = EquipmentEffects{{WireID: 7, Values: []float64{9}}}
		state.SetInventoryEquipment(11, item)
		state.SetInventoryItemsCollection("storage:1", map[int64]int64{100: 2})
		return []string{"inventory"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Patch == nil || event.Patch.Inventory != nil || event.Patch.InventoryChanges == nil {
		t.Fatalf("inventory delta patch = %#v", event.Patch)
	}
	changes := event.Patch.InventoryChanges
	if changes.ConstructionItems == nil || changes.EquipmentChanges == nil || changes.ItemChanges == nil ||
		changes.Gems != nil || changes.GemStacks != nil || changes.ConstructionOffers != nil {
		t.Fatalf("inventory part delta = %#v", changes)
	}
	if changes.Equipment != nil || len(*changes.EquipmentChanges) != 1 || (*changes.EquipmentChanges)[0].ID != 11 ||
		changes.Items != nil || len(*changes.ItemChanges) != 1 || (*changes.ItemChanges)[0].Collection != "storage:1" {
		t.Fatalf("inventory keyed delta = %#v", changes)
	}
	after := store.PlanningView().State

	if before.Inventory.ConstructionItems[1] != 4 || after.Inventory.ConstructionItems[1] != 3 {
		t.Fatalf("construction inventory generations = %d / %d", before.Inventory.ConstructionItems[1], after.Inventory.ConstructionItems[1])
	}
	if before.Inventory.Equipment[11].Level != 1 || before.Inventory.Equipment[11].Effects[0].Values[0] != 1 {
		t.Fatalf("prior equipment generation changed: %+v", before.Inventory.Equipment[11])
	}
	if after.Inventory.Equipment[11].Level != 3 || after.Inventory.Equipment[11].Effects[0].Values[0] != 9 {
		t.Fatalf("current equipment generation = %+v", after.Inventory.Equipment[11])
	}
	if reflect.ValueOf(before.Inventory.Items["storage:1"]).Pointer() == reflect.ValueOf(after.Inventory.Items["storage:1"]).Pointer() {
		t.Fatal("changed storage collection retained its old map")
	}
	if reflect.ValueOf(before.Inventory.Items["storage:2"]).Pointer() != reflect.ValueOf(after.Inventory.Items["storage:2"]).Pointer() {
		t.Fatal("unchanged storage collection was cloned")
	}
	if reflect.ValueOf(before.Inventory.Equipment[22].Effects[0].Values).Pointer() !=
		reflect.ValueOf(after.Inventory.Equipment[22].Effects[0].Values).Pointer() {
		t.Fatal("unchanged equipment effects were cloned")
	}
	if reflect.ValueOf(before.Inventory.Gems).Pointer() != reflect.ValueOf(after.Inventory.Gems).Pointer() {
		t.Fatal("unwritten gem index was cloned")
	}
}

func TestConstructionOffersRemainScopedAcrossCastleUpdates(t *testing.T) {
	now := time.Date(2026, 8, 14, 10, 0, 0, 0, time.UTC)
	initial := NewGameState()
	initial.ReplaceInventoryConstructionOffers(map[PackageID]int64{100: 2}, now, 10, 0)
	store := NewStore(initial)
	before := store.ReadOnlyView()

	event, err := store.ApplyComponents(Components(ComponentInventory), func(state *GameState) ([]string, bool, error) {
		state.ReplaceInventoryConstructionOffers(map[PackageID]int64{245: 12}, now.Add(time.Minute), 40, 4)
		return []string{"inventory", "construction-offers"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Patch == nil || event.Patch.InventoryChanges == nil ||
		event.Patch.InventoryChanges.ConstructionOffersByCastle == nil {
		t.Fatalf("construction-offer patch = %#v", event.Patch)
	}
	after := store.ReadOnlyView()
	greatEmpireOffers, greatEmpireObservedAt, found := after.ConstructionOffersFor(10, 0)
	if !found || greatEmpireOffers[100] != 2 || !greatEmpireObservedAt.Equal(now) {
		t.Fatalf("Great Empire counters = %#v at %s found=%t", greatEmpireOffers, greatEmpireObservedAt, found)
	}
	stormOffers, stormObservedAt, found := after.ConstructionOffersFor(40, 4)
	if !found || stormOffers[245] != 12 || !stormObservedAt.Equal(now.Add(time.Minute)) {
		t.Fatalf("Storm counters = %#v at %s found=%t", stormOffers, stormObservedAt, found)
	}
	if priorOffers, _, found := before.ConstructionOffersFor(10, 0); !found || priorOffers[100] != 2 {
		t.Fatalf("prior generation changed: %#v found=%t", priorOffers, found)
	}
	if _, _, found := before.ConstructionOffersFor(40, 4); found {
		t.Fatal("prior generation acquired the later Storm counters")
	}
}

func TestApplyWithoutMapMutationKeepsSharedMapImmutableAcrossLaterMapWrite(t *testing.T) {
	initial := NewGameState()
	initial.Map[4] = map[string]MapObservation{
		"100:101": {KingdomID: 4, X: 100, Y: 101, TypeID: MapTypePlayerCastle, Name: "before"},
	}
	store := NewStore(initial)
	before := store.ReadOnlyView()

	if _, err := store.ApplyWithoutMapMutation(func(state *GameState) ([]string, bool, error) {
		state.Player.Level = 70
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	shared := store.ReadOnlyView()
	sharedMap, sharedFound := shared.LookupMapObservation(4, "100:101")
	if shared.Player.Level != 70 || !sharedFound || sharedMap.Name != "before" {
		t.Fatalf("map-preserving mutation = %#v", shared)
	}

	if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
		observation := state.Map[4]["100:101"]
		observation.Name = "after"
		state.Map[4]["100:101"] = observation
		return []string{"map"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	beforeMap, beforeFound := before.LookupMapObservation(4, "100:101")
	sharedMap, sharedFound = shared.LookupMapObservation(4, "100:101")
	if !beforeFound || !sharedFound || beforeMap.Name != "before" || sharedMap.Name != "before" {
		t.Fatal("a later full map mutation changed an immutable earlier generation")
	}
	if latest, found := store.ReadOnlyView().LookupMapObservation(4, "100:101"); !found || latest.Name != "after" {
		t.Fatalf("latest map observation = %+v, found %t", latest, found)
	}
}

func TestApplyComponentsPublishesOnlyPrivateWriteComponents(t *testing.T) {
	initial := NewGameState()
	initial.Player.Resources[1] = 100
	initial.Map[4] = map[string]MapObservation{
		"100:101": {KingdomID: 4, X: 100, Y: 101, TypeID: MapTypePlayerCastle, Name: "before"},
	}
	store := NewStore(initial)
	before := store.ReadOnlyView()

	event, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Resources[1] = 250
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if before.Player.Resources[1] != 100 {
		t.Fatalf("earlier player generation changed to %v", before.Player.Resources[1])
	}
	if observation, found := before.LookupMapObservation(4, "100:101"); !found || observation.Name != "before" {
		t.Fatal("unwritten map component changed")
	}
	if got := store.ReadOnlyView().Player.Resources[1]; got != 250 {
		t.Fatalf("current player resource = %v, want 250", got)
	}
	if !reflect.DeepEqual(event.Components, []Component{ComponentPlayer}) {
		t.Fatalf("event components = %v", event.Components)
	}
}

func TestApplyComponentsDiscardsFailedPrivateCandidate(t *testing.T) {
	initial := NewGameState()
	initial.Player.Resources[1] = 100
	store := NewStore(initial)
	wantErr := errors.New("reject candidate")

	_, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Resources[1] = 250
		return nil, false, wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("apply error = %v", err)
	}
	if got := store.ReadOnlyView().Player.Resources[1]; got != 100 {
		t.Fatalf("rejected candidate leaked resource %v", got)
	}
	if store.Revision() != 0 {
		t.Fatalf("rejected candidate advanced revision to %d", store.Revision())
	}
}

func TestApplyComponentsPublishesSparsePatchWithNamedComponents(t *testing.T) {
	initial := NewGameState()
	initial.Player.Resources[1] = 100
	initial.Map[4] = map[string]MapObservation{
		"100:101": {KingdomID: 4, X: 100, Y: 101},
	}
	store := NewStore(initial)

	event, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Resources[1] = 250
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Patch == nil || event.Patch.Player == nil {
		t.Fatalf("player patch missing: %#v", event.Patch)
	}
	if event.Patch.Map != nil || event.Patch.Session != nil {
		t.Fatalf("unwritten component leaked into patch: %#v", event.Patch)
	}
	if got := event.Patch.Player.Resources[1]; got != 250 {
		t.Fatalf("patched resource = %v, want 250", got)
	}
	raw, err := json.Marshal(event)
	if err != nil {
		t.Fatal(err)
	}
	jsonText := string(raw)
	if !strings.Contains(jsonText, `"components":["player"]`) {
		t.Fatalf("components are not named on the wire: %s", jsonText)
	}
	if strings.Contains(jsonText, `"map"`) || strings.Contains(jsonText, `"session"`) {
		t.Fatalf("sparse patch serialized an unwritten component: %s", jsonText)
	}
}

func TestCoalescedPatchContainsLatestValuesForEveryChangedComponent(t *testing.T) {
	initial := NewGameState()
	initial.Player.Level = 10
	initial.Map[4] = map[string]MapObservation{
		"100:101": {KingdomID: 4, X: 100, Y: 101, TypeID: MapTypePlayerCastle, Name: "before"},
	}
	store := NewStore(initial)
	events, unsubscribe := store.Subscribe(1)
	defer unsubscribe()

	if _, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
		state.Player.Level = 11
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		observation, _ := state.LookupMapObservation(4, "100:101")
		observation.Name = "after"
		return []string{"map"}, state.SetMapObservation(observation), nil
	}); err != nil {
		t.Fatal(err)
	}

	event := <-events
	if !event.Gap || event.Revision != 2 {
		t.Fatalf("coalesced event metadata = %#v", event)
	}
	if event.Patch == nil || event.Patch.Player == nil || event.Patch.MapChanges == nil {
		t.Fatalf("coalesced patch missing changed components: %#v", event.Patch)
	}
	if got := event.Patch.Player.Level; got != 11 {
		t.Fatalf("coalesced player level = %d, want 11", got)
	}
	if len(*event.Patch.MapChanges) != 1 || (*event.Patch.MapChanges)[0].Observation == nil {
		t.Fatalf("coalesced map delta = %+v", event.Patch.MapChanges)
	}
	if got := (*event.Patch.MapChanges)[0].Observation.Name; got != "after" {
		t.Fatalf("coalesced map value = %q, want after", got)
	}
}

func TestSparsePatchCanExplicitlyClearCollection(t *testing.T) {
	initial := NewGameState()
	initial.Movements[1] = MovementState{ID: 1}
	store := NewStore(initial)

	event, err := store.ApplyComponents(Components(ComponentMovements), func(state *GameState) ([]string, bool, error) {
		state.Movements = map[MovementID]MovementState{}
		return []string{"movements"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(event.Patch)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), `"movementChanges":[{"id":1,"deleted":true}]`) {
		t.Fatalf("cleared collection was omitted from patch: %s", raw)
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
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 ||
		context.FocusSubcontext != FocusSubcontextCastle || context.FocusEpoch != 1 {
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
	if context := store.ProtocolContext(); context.FocusedCastleID != 12 ||
		context.FocusSubcontext != FocusSubcontextCastle || context.FocusEpoch != 2 {
		t.Fatalf("updated protocol context = %#v", context)
	}
}

func TestProtocolContextTracksMapSubcontextForFocusedCastle(t *testing.T) {
	initial := NewGameState()
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	setSubcontext := func(subcontext FocusSubcontext) {
		if _, err := store.ApplyScoped(func(state *GameState) (ScopedChange, error) {
			return ScopedChange{
				Partitions:      []PartitionKey{SessionPartition(*state, CapabilitySessionContext)},
				FocusSubcontext: subcontext, Changed: true,
			}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}

	setSubcontext(FocusSubcontextMap)
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 ||
		context.FocusSubcontext != FocusSubcontextMap || context.FocusEpoch != 2 {
		t.Fatalf("map protocol context = %#v", context)
	}
	setSubcontext(FocusSubcontextMap)
	if context := store.ProtocolContext(); context.FocusEpoch != 2 {
		t.Fatalf("repeated map context advanced focus epoch: %#v", context)
	}
	setSubcontext(FocusSubcontextCastle)
	if context := store.ProtocolContext(); context.FocusedCastleID != 11 ||
		context.FocusSubcontext != FocusSubcontextCastle || context.FocusEpoch != 3 {
		t.Fatalf("restored castle protocol context = %#v", context)
	}
}

func TestRecruitmentBUPAllianceHelpBatchIsScopedToFocusEpoch(t *testing.T) {
	initial := NewGameState()
	initial.Session.Generation = 7
	initial.Session.ConnectionGeneration = 3
	initial.AllianceHelpRequests = AllianceHelpRequestState{
		RecruitmentCastleIDs: []CastleID{}, OwnObservedGeneration: 7,
		OwnRecruitmentRequests: []RecruitmentAllianceHelpRequest{}, OwnRecruitmentObservedGeneration: 7,
	}
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	context := store.ProtocolContext()
	if !store.ObserveRecruitmentBUP(11, 7, 3, context.FocusEpoch) ||
		!store.ObserveRecruitmentBUP(11, 7, 3, context.FocusEpoch) {
		t.Fatal("committed recruitment BUPs were not recorded")
	}
	context = store.ProtocolContext()
	if context.RecruitmentBUPCastleID != 11 || context.RecruitmentBUPFocusEpoch != context.FocusEpoch ||
		context.RecruitmentBUPSerial != 2 || context.RecruitmentAHRCoveredSerial != 0 {
		t.Fatalf("two-BUP batch = %#v", context)
	}
	if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
		state.AllianceHelpRequests.RecruitmentCastleIDs = []CastleID{11}
		state.AllianceHelpRequests.OwnRecruitmentRequests = []RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 11, Progress: 1, MaximumHelpers: 3, ObservedAt: time.Now().UTC()},
		}
		return []string{"alliance-help"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !store.ObserveRecruitmentAHRCovered(11, 7, 3, context.FocusEpoch, context.RecruitmentBUPSerial) {
		t.Fatal("correlated recruitment AHR did not cover the BUP batch")
	}
	context = store.ProtocolContext()
	if context.RecruitmentAHRCoveredSerial != 2 || !context.RecruitmentAHRFocusCovered {
		t.Fatalf("covered BUP context = %#v", context)
	}

	store.ObserveProtocolFocus(FocusSubcontextCastle, time.Now().UTC())
	if duplicate := store.ProtocolContext(); duplicate.FocusEpoch != context.FocusEpoch ||
		duplicate.RecruitmentAHRCoveredSerial != 2 {
		t.Fatalf("duplicate castle context changed the covered batch: %#v", duplicate)
	}
	if !store.ObserveRecruitmentBUP(11, 7, 3, context.FocusEpoch) {
		t.Fatal("later same-epoch BUP was not recorded")
	}
	context = store.ProtocolContext()
	if context.RecruitmentBUPSerial != 3 || context.RecruitmentAHRCoveredSerial != 3 {
		t.Fatalf("successful AHR did not cover the later same-epoch BUP: %#v", context)
	}
	if _, err := store.Apply(func(state *GameState) ([]string, bool, error) {
		state.AllianceHelpRequests.RecruitmentCastleIDs = []CastleID{}
		state.AllianceHelpRequests.OwnRecruitmentRequests = []RecruitmentAllianceHelpRequest{}
		state.AllianceHelpRequests.ObservedAt = time.Now().UTC()
		return []string{"alliance-help"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if !store.ObserveRecruitmentBUP(11, 7, 3, context.FocusEpoch) {
		t.Fatal("post-lifecycle same-epoch BUP was not recorded")
	}
	context = store.ProtocolContext()
	if context.RecruitmentBUPSerial != 4 || context.RecruitmentAHRCoveredSerial != 3 ||
		context.RecruitmentAHRFocusCovered {
		t.Fatalf("ended alliance-help lifecycle incorrectly covered a new BUP: %#v", context)
	}

	store.ObserveProtocolFocus(FocusSubcontextMap, time.Now().UTC())
	mapContext := store.ProtocolContext()
	if mapContext.FocusEpoch != context.FocusEpoch+1 || mapContext.RecruitmentBUPSerial != 0 ||
		mapContext.RecruitmentAHRCoveredSerial != 0 {
		t.Fatalf("map focus did not end the recruitment BUP batch: %#v", mapContext)
	}
	store.ObserveProtocolFocus(FocusSubcontextCastle, time.Now().UTC())
	returned := store.ProtocolContext()
	if returned.FocusEpoch != mapContext.FocusEpoch+1 ||
		!store.ObserveRecruitmentBUP(11, 7, 3, returned.FocusEpoch) {
		t.Fatalf("returning castle focus did not start a fresh BUP batch: %#v", returned)
	}
	returned = store.ProtocolContext()
	if returned.RecruitmentBUPSerial != 1 || returned.RecruitmentAHRCoveredSerial != 0 ||
		returned.RecruitmentBUPFocusEpoch != returned.FocusEpoch || returned.RecruitmentAHRFocusCovered {
		t.Fatalf("post-refocus recruitment BUP batch = %#v", returned)
	}
}

func TestRecruitmentBUPInheritsCurrentLifecycleAcrossFocusEpoch(t *testing.T) {
	now := time.Now().UTC()
	initial := NewGameState()
	initial.Session.Generation = 7
	initial.Session.ConnectionGeneration = 3
	initial.Session.ChangedAt = now.Add(-time.Minute)
	initial.AllianceHelpRequests = AllianceHelpRequestState{
		RecruitmentCastleIDs: []CastleID{11}, OwnObservedGeneration: 7,
		OwnRecruitmentRequests: []RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 11, Progress: 2, MaximumHelpers: 3, ObservedAt: now},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	store.ObserveProtocolFocus(FocusSubcontextMap, now)
	store.ObserveProtocolFocus(FocusSubcontextCastle, now)
	protocol := store.ProtocolContext()
	if !store.ObserveRecruitmentBUP(11, 7, 3, protocol.FocusEpoch) {
		t.Fatal("post-refocus recruitment BUP was not recorded")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 1 || protocol.RecruitmentAHRCoveredSerial != 1 ||
		!protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("same-castle lifecycle did not cover the post-refocus BUP: %#v", protocol)
	}
}

func TestStandaloneRecruitmentAHRMarkerIsFocusScopedWhileLifecycleCoversAcrossFocus(t *testing.T) {
	now := time.Now().UTC()
	initial := NewGameState()
	initial.Session.Generation = 7
	initial.Session.ConnectionGeneration = 3
	initial.Session.ChangedAt = now.Add(-time.Minute)
	initial.AllianceHelpRequests = AllianceHelpRequestState{
		OwnRecruitmentRequests: []RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 11, Progress: 1, MaximumHelpers: 3, ObservedAt: now},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	protocol := store.ProtocolContext()
	if !store.PrepareStandaloneRecruitmentAHR(11, 7, 3, protocol.FocusEpoch) {
		t.Fatal("standalone AHR did not bind current focus")
	}
	if protocol = store.ProtocolContext(); !protocol.RecruitmentAHRPending {
		t.Fatalf("standalone AHR pending marker = %#v", protocol)
	}
	if !store.ObserveStandaloneRecruitmentAHRCovered(
		11, 7, 3, protocol.FocusEpoch, now,
	) {
		t.Fatal("standalone AHR did not establish current-focus coverage")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPCastleID != 11 || protocol.RecruitmentBUPFocusEpoch != protocol.FocusEpoch ||
		protocol.RecruitmentBUPSerial != 0 || protocol.RecruitmentAHRCoveredSerial != 0 ||
		!protocol.RecruitmentAHRFocusCovered || protocol.RecruitmentAHRPending {
		t.Fatalf("standalone zero-serial coverage = %#v", protocol)
	}
	if !store.ObserveRecruitmentBUP(11, 7, 3, protocol.FocusEpoch) {
		t.Fatal("later same-focus BUP was not recorded")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 1 || protocol.RecruitmentAHRCoveredSerial != 1 ||
		!protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("standalone AHR did not cover the first later BUP: %#v", protocol)
	}

	store.ObserveProtocolFocus(FocusSubcontextMap, now.Add(time.Second))
	store.ObserveProtocolFocus(FocusSubcontextCastle, now.Add(2*time.Second))
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 0 || protocol.RecruitmentAHRCoveredSerial != 0 ||
		protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("focus change retained standalone AHR coverage: %#v", protocol)
	}
	if !store.ObserveRecruitmentBUP(11, 7, 3, protocol.FocusEpoch) {
		t.Fatal("post-refocus BUP was not recorded")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentBUPSerial != 1 || protocol.RecruitmentAHRCoveredSerial != 1 ||
		!protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("post-refocus BUP did not inherit the current same-castle lifecycle: %#v", protocol)
	}
}

func TestStandaloneRecruitmentAHRPendingMarkerCannotCrossFocusEpoch(t *testing.T) {
	now := time.Now().UTC()
	initial := NewGameState()
	initial.Session.Generation = 7
	initial.Session.ConnectionGeneration = 3
	initial.Session.ChangedAt = now.Add(-time.Minute)
	initial.AllianceHelpRequests = AllianceHelpRequestState{
		OwnRecruitmentRequests: []RecruitmentAllianceHelpRequest{
			{ListID: 91, CastleID: 11, Progress: 1, MaximumHelpers: 3, ObservedAt: now},
		},
		OwnRecruitmentObservedGeneration: 7,
	}
	initial.Castles[11] = CastleState{ID: 11, KingdomID: 1, Focused: true}
	store := NewStore(initial)
	protocol := store.ProtocolContext()
	if !store.PrepareStandaloneRecruitmentAHR(11, 7, 3, protocol.FocusEpoch) {
		t.Fatal("standalone AHR did not bind initial focus")
	}
	store.ObserveProtocolFocus(FocusSubcontextMap, now.Add(time.Second))
	store.ObserveProtocolFocus(FocusSubcontextCastle, now.Add(2*time.Second))
	protocol = store.ProtocolContext()
	if store.ObserveStandaloneRecruitmentAHRCovered(11, 7, 3, protocol.FocusEpoch, now.Add(2*time.Second)) {
		t.Fatal("old standalone AHR marker covered a new focus epoch")
	}
	protocol = store.ProtocolContext()
	if protocol.RecruitmentAHRPending || protocol.RecruitmentAHRFocusCovered {
		t.Fatalf("new focus retained old standalone AHR binding: %#v", protocol)
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

func TestStoreReconcilesTrackedStormTargetWithNewerLiveMap(t *testing.T) {
	initial := NewGameState()
	initial.Storm.Map.Targets["612:667"] = MapObservation{
		KingdomID: 4, X: 612, Y: 667, TypeID: MapTypeStormFort, StormIsleID: 9,
		ObservedAt: time.Date(2026, time.July, 21, 17, 33, 0, 0, time.UTC),
	}
	observedAt := time.Date(2026, time.July, 21, 19, 18, 0, 0, time.UTC)
	readyAt := observedAt.Add(10 * time.Hour)
	initial.Map[4] = map[string]MapObservation{
		"612:667": {
			KingdomID: 4, X: 612, Y: 667, TypeID: MapTypeStormFort, StormIsleID: 7,
			StormCooldownRemaining: 36_000, ObservedAt: observedAt,
		},
	}

	tracked := NewStore(initial).Snapshot().Storm.Map.Targets["612:667"]
	if tracked.StormIsleID != 7 || tracked.StormCooldownRemaining != 36_000 || !tracked.StormReadyAt().Equal(readyAt) {
		t.Fatalf("reconciled Storm target = %#v", tracked)
	}
}

func TestStoreCompactsStormTargetsToMapBackedMembership(t *testing.T) {
	initial := NewGameState()
	observedAt := time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC)
	target := MapObservation{
		KingdomID: stormKingdomID, X: 612, Y: 667, TypeID: MapTypeStormFort,
		StormIsleID: 7, ObservedAt: observedAt,
	}
	initial.Map[stormKingdomID] = map[string]MapObservation{"612:667": target}
	initial.Storm.Map.Targets["612:667"] = target

	store := NewStore(initial)
	view := store.ReadOnlyView()
	if len(view.Storm.Map.Targets) != 0 {
		t.Fatalf("tenant generation retained duplicate Storm observations: %#v", view.Storm.Map.Targets)
	}
	if view.StormTargetCount() != 1 {
		t.Fatalf("Storm target count = %d", view.StormTargetCount())
	}
	if tracked, found := view.LookupStormTarget("612:667"); !found || tracked != target {
		t.Fatalf("map-backed Storm target = %#v, found %v", tracked, found)
	}

	snapshot := store.Snapshot()
	if tracked := snapshot.Storm.Map.Targets["612:667"]; tracked != target {
		t.Fatalf("logical snapshot Storm target = %#v", tracked)
	}
	if snapshot.stormTargets != nil {
		t.Fatal("defensive logical snapshot exposed tenant target storage")
	}

	event, err := store.ApplyComponents(Components(ComponentStorm), func(state *GameState) ([]string, bool, error) {
		return []string{"storm"}, state.DeleteStormTarget("612:667"), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if event.Revision == 0 || len(event.stormTargetKeys) != 1 || event.stormTargetKeys[0] != "612:667" {
		t.Fatalf("Storm membership event = %#v", event)
	}
	if store.ReadOnlyView().StormTargetCount() != 0 {
		t.Fatal("deleted Storm membership remained visible")
	}
}

func TestApplyScopedComponentsPrunesUntouchedTrackedComponents(t *testing.T) {
	store := NewStore(NewGameState())
	writes := Components(
		ComponentWorldMap,
		ComponentTowerCooldowns,
		ComponentTowerQueue,
		ComponentEventScores,
		ComponentAttackAnalytics,
		ComponentCommanders,
		ComponentKhan,
		ComponentAdvisor,
		ComponentBeri,
		ComponentNomadCamps,
		ComponentStorm,
	)
	event, err := store.ApplyScopedComponents(writes, func(state *GameState) (ScopedChange, error) {
		changed := state.SetMapObservation(MapObservation{
			KingdomID: 0, X: 10, Y: 20, TypeID: MapTypeKingdomTower, Level: 1,
			ObservedAt: time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC),
		})
		return ScopedChange{Domains: []string{"map"}, Changed: changed}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := Components(event.Components...); got != Components(ComponentWorldMap) {
		t.Fatalf("dirty components = %v, want map only", event.Components)
	}
}

func TestStoreMultiComponentMutationPublishesOnlyChangedProjection(t *testing.T) {
	initial := NewGameState()
	initial.Player.Level = 70
	initial.Castles[91] = CastleState{
		ID: 91, Resources: map[ResourceID]ResourceBalance{1: {Amount: 10}},
	}
	store := NewStore(initial)
	event, err := store.ApplyComponents(Components(ComponentPlayer, ComponentCastles), func(state *GameState) ([]string, bool, error) {
		castle, found := state.MutableCastleParts(91, CastlePartResources)
		if !found {
			return nil, false, errors.New("castle missing")
		}
		balance := castle.Resources[1]
		balance.Amount++
		castle.Resources[1] = balance
		state.SetCastleParts(91, castle, CastlePartResources)
		return []string{"resources"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(event.Components) != 1 || event.Components[0] != ComponentCastles {
		t.Fatalf("dirty components = %v, want castles only", event.Components)
	}
	if event.Patch == nil || event.Patch.Player != nil || event.Patch.CastleChanges == nil {
		t.Fatalf("targeted component patch = %+v", event.Patch)
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

package State

import (
	"fmt"
	"testing"
	"time"
)

func TestStoreMapRetentionExpiresOnlyPrivateStaleFacts(t *testing.T) {
	now := time.Now().UTC()
	state := NewGameState()
	state.Map[0] = map[string]MapObservation{
		"10:10": {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower, ObservedAt: now},
		"20:20": {KingdomID: 0, X: 20, Y: 20, TypeID: MapTypeRift, ObservedAt: now},
		"30:30": {KingdomID: 0, X: 30, Y: 30, TypeID: MapTypeRift},
	}
	store := NewStore(state)
	removed, err := store.PruneMap(now.Add(15 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if removed != 1 {
		t.Fatalf("removed observations = %d, want one expired Rift", removed)
	}
	view := store.ReadOnlyView()
	if _, found := view.LookupMapObservation(0, "20:20"); found {
		t.Fatal("expired Rift observation remained")
	}
	if _, found := view.LookupMapObservation(0, "10:10"); !found {
		t.Fatal("long-lived tower observation was pruned early")
	}
	if _, found := view.LookupMapObservation(0, "30:30"); !found {
		t.Fatal("observation without authoritative time was pruned")
	}
}

func TestMapRetentionLimitKeepsNewestDeterministically(t *testing.T) {
	now := time.Now().UTC()
	candidates := make([]mapRetentionCandidate, 0, 4)
	for index := range 4 {
		candidates = append(candidates, mapRetentionCandidate{
			kingdomID: 0, key: fmt.Sprintf("%d:0", index), typeID: MapTypeKingdomTower,
			observed: now.Add(time.Duration(index) * time.Minute),
		})
	}
	removed := mapRetentionRemovals(candidates, now, 2)
	if len(removed) != 2 || removed[0].key != "1:0" || removed[1].key != "0:0" {
		t.Fatalf("limit removals = %+v", removed)
	}
}

func TestWorldMapRetentionDoesNotDeleteNewerReplacement(t *testing.T) {
	now := time.Now().UTC()
	store := NewWorldMapStore()
	old := MapObservation{
		KingdomID: 0, X: 10, Y: 10, TypeID: MapTypePlayerCastle,
		OwnerID: 1, ObservedAt: now.Add(-31 * 24 * time.Hour),
	}
	store.commit("world", nil, []MapChange{{KingdomID: 0, Key: "10:10", Observation: &old}})
	newer := old
	newer.OwnerID = 2
	newer.ObservedAt = now
	store.commit("world", nil, []MapChange{{KingdomID: 0, Key: "10:10", Observation: &newer}})
	staleDelete := MapChange{
		KingdomID: 0, Key: "10:10", TypeID: MapTypePlayerCastle, Deleted: true,
		expectedObservedAt: old.ObservedAt,
	}
	store.commit("world", nil, []MapChange{staleDelete})
	fact, found := lookupWorldFact(store.Snapshot("world").values, 0, "10:10")
	if !found || fact.OwnerID != 2 {
		t.Fatalf("newer shared fact was deleted: %+v, found %v", fact, found)
	}
}

func TestPrivateMapShardUsesPhysicalShardAndMapOverlay(t *testing.T) {
	state := NewGameState()
	state.Map[0] = map[string]MapObservation{
		"10:10": {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypeKingdomTower},
		"20:20": {KingdomID: 0, X: 20, Y: 20, TypeID: MapTypeRift},
	}
	state.compactMapOverlay()
	shard := mapShardIndex("10:10")
	state.Map = WorldMap{0: {
		"10:10": {KingdomID: 0, X: 10, Y: 10, TypeID: MapTypePlayerCastle},
	}}
	got := state.privateMapShard(0, shard)
	if got["10:10"].TypeID != MapTypePlayerCastle {
		t.Fatalf("overlay observation = %+v", got["10:10"])
	}
	for key := range got {
		if mapShardIndex(key) != shard {
			t.Fatalf("private shard %d included key %q from shard %d", shard, key, mapShardIndex(key))
		}
	}
}

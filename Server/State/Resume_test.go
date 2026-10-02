package State

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestResumeRingBoundsAndFloor(t *testing.T) {
	now := time.Now()
	event := func(revision uint64) Event { return Event{BaseRevision: revision - 1, Revision: revision} }
	t.Run("count", func(t *testing.T) {
		var ring resumeRing
		for revision := uint64(1); revision <= 600; revision++ {
			ring.append(event(revision), now)
		}
		if ring.count != 512 || ring.floor != 88 {
			t.Fatalf("count %d floor %d", ring.count, ring.floor)
		}
	})
	t.Run("bytes", func(t *testing.T) {
		var ring resumeRing
		for revision := uint64(1); revision <= 100; revision++ {
			value := event(revision)
			value.itemKeys = []string{strings.Repeat("x", 32<<10)}
			ring.append(value, now)
		}
		if ring.bytes > resumeMaxBytes || ring.floor == 0 || ring.count == 0 {
			t.Fatalf("bytes %d count %d floor %d", ring.bytes, ring.count, ring.floor)
		}
		value := event(101)
		value.itemKeys = []string{strings.Repeat("x", resumeMaxBytes)}
		ring.append(value, now)
		if ring.count != 0 || ring.bytes != 0 || ring.floor != 101 {
			t.Fatalf("oversized entry retained: %+v", ring)
		}
		ring.append(event(102), now)
		if ring.floor != 101 {
			t.Fatalf("floor rewound: %d", ring.floor)
		}
	})
	t.Run("age", func(t *testing.T) {
		var ring resumeRing
		ring.append(event(1), now)
		ring.append(event(2), now.Add(time.Minute))
		ring.expire(now.Add(resumeMaxAge))
		if ring.floor != 0 {
			t.Fatal("evicted at the inclusive age bound")
		}
		ring.expire(now.Add(resumeMaxAge + time.Second))
		if ring.floor != 1 || ring.count != 1 {
			t.Fatalf("floor %d count %d", ring.floor, ring.count)
		}
		ring.expire(now.Add(2 * resumeMaxAge))
		if ring.floor != 2 || ring.count != 0 || ring.bytes != 0 {
			t.Fatal("idle history did not expire")
		}
		for _, entry := range ring.entries {
			if entry.event.Revision != 0 {
				t.Fatal("eviction retains metadata")
			}
		}
	})
}

func TestResumeRetainsOnlyMetadataAndCurrentGeneration(t *testing.T) {
	initial := NewGameState()
	store := NewStore(&initial)
	_, err := store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(MapObservation{KingdomID: 4, X: 1, Y: 2, TypeID: MapTypePlayerCastle, Name: "old"}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	metadata := store.resume.entries[0].event
	if metadata.generation != nil || metadata.Patch != nil || metadata.clientEncoding != nil || metadata.mapChanges[0].Observation != nil {
		t.Fatal("history retains state data")
	}
	_, err = store.ApplyComponents(Components(ComponentWorldMap), func(state *GameState) ([]string, bool, error) {
		return []string{"map"}, state.SetMapObservation(MapObservation{KingdomID: 4, X: 1, Y: 2, TypeID: MapTypePlayerCastle, Name: "new"}), nil
	})
	if err != nil {
		t.Fatal(err)
	}
	view, patch, ok := store.Resume(store.Instance(), 0)
	if !ok || patch == nil || patch.BaseRevision != 0 || patch.Revision != view.Revision || !patch.Gap {
		t.Fatal("resume metadata is incorrect")
	}
	if got := (*patch.Patch.MapChanges)[0].Observation.Name; got != "new" {
		t.Fatalf("resumed map value = %q", got)
	}
	_, patch, ok = store.Resume(store.Instance(), 2)
	if !ok || patch != nil {
		t.Fatal("at-head resume must not send a patch")
	}
	for _, test := range []struct {
		instance string
		revision uint64
	}{{"", 0}, {"wrong", 0}, {store.Instance(), 3}} {
		if _, _, ok := store.Resume(test.instance, test.revision); ok {
			t.Fatal("invalid cursor resumed")
		}
	}
	store.resume.entries[store.resume.start].at = time.Now().Add(-2 * resumeMaxAge)
	if _, _, ok := store.Resume(store.Instance(), 0); ok {
		t.Fatal("expired history resumed")
	}
	other := NewStore(&initial)
	if len(store.Instance()) != 32 || store.Instance() == other.Instance() {
		t.Fatal("instance must be random per store")
	}
}

// Independently apply the wire projection, rather than reusing server merging
// or copy-on-write code. Random disconnect points exercise both sparse deltas
// and replacement followed by upserts/deletions of the same keys.
func applyResumeProjection(t *testing.T, client map[string]any, event Event) {
	t.Helper()
	raw, err := ClientEventPayload(event)
	if err != nil {
		t.Fatal(err)
	}
	var message struct {
		Patch map[string]any `json:"patch"`
	}
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}
	for key, value := range message.Patch {
		switch key {
		case "castleChanges", "movementChanges":
			collection, valueKey := "castles", "castle"
			if key == "movementChanges" {
				collection, valueKey = "movements", "movement"
			}
			values, _ := client[collection].(map[string]any)
			if values == nil {
				values = map[string]any{}
				client[collection] = values
			}
			for _, entry := range value.([]any) {
				change := entry.(map[string]any)
				id := strconv.FormatInt(int64(change["id"].(float64)), 10)
				if change["deleted"] == true {
					delete(values, id)
				} else if replacement, ok := change[valueKey]; ok {
					values[id] = replacement
				} else {
					current, _ := values[id].(map[string]any)
					if current == nil {
						current = map[string]any{}
						values[id] = current
					}
					for field, next := range change["patch"].(map[string]any) {
						current[field] = next
					}
				}
			}
		case "mapChanges":
			world, _ := client["map"].(map[string]any)
			if world == nil {
				world = map[string]any{}
				client["map"] = world
			}
			for _, entry := range value.([]any) {
				change := entry.(map[string]any)
				kingdom := strconv.FormatInt(int64(change["kingdomId"].(float64)), 10)
				values, _ := world[kingdom].(map[string]any)
				if values == nil {
					values = map[string]any{}
					world[kingdom] = values
				}
				key := change["key"].(string)
				if change["deleted"] == true {
					delete(values, key)
				} else {
					values[key] = change["observation"]
				}
			}
		default:
			client[key] = value
		}
	}
}

func resumeProjection(t *testing.T, state GameState) map[string]any {
	t.Helper()
	raw, err := json.Marshal(NewClientStateSnapshot(&state))
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestResumeRandomMutationStreamsMatchServerProjection(t *testing.T) {
	for seed := int64(0); seed < 40; seed++ {
		random := rand.New(rand.NewSource(seed))
		initial := NewGameState()
		store := NewStore(&initial)
		client := resumeProjection(t, store.ReadOnlyView())
		since := uint64(0)
		for step := 0; step < 120; step++ {
			operation := random.Intn(9)
			id := int64(1 + random.Intn(8))
			_, err := store.ApplyComponents(Components(ComponentPlayer, ComponentCastles, ComponentWorldMap, ComponentMovements), func(state *GameState) ([]string, bool, error) {
				state.Player.Level = step + 1
				switch operation {
				case 0:
					state.SetCastle(CastleID(id), CastleState{ID: CastleID(id), Name: fmt.Sprintf("castle-%d", step)})
				case 1:
					state.DeleteCastle(CastleID(id))
				case 2:
					state.ReplaceCastles(map[CastleID]CastleState{CastleID(id): {ID: CastleID(id), Name: fmt.Sprintf("replacement-%d", step)}})
				case 3:
					state.SetMapObservation(MapObservation{KingdomID: 4, X: int(id), Y: 2, TypeID: MapTypePlayerCastle, Name: fmt.Sprintf("map-%d", step)})
				case 4:
					state.DeleteMapObservation(4, fmt.Sprintf("%d:2", id))
				case 5:
					state.ReplaceMapState()
					state.SetMapObservation(MapObservation{KingdomID: 4, X: int(id), Y: 2, TypeID: MapTypePlayerCastle, Name: fmt.Sprintf("replace-%d", step)})
				case 6:
					state.SetMovement(MovementID(id), MovementState{ID: MovementID(id), Units: map[UnitID]int64{}})
				case 7:
					state.DeleteMovement(MovementID(id))
				case 8:
					state.ReplaceMovements(map[MovementID]MovementState{MovementID(id): {ID: MovementID(id), Units: map[UnitID]int64{}}})
				}
				return []string{"castles", "map", "movements"}, true, nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if random.Intn(8) == 0 || step == 119 {
				view, event, ok := store.Resume(store.Instance(), since)
				if !ok {
					t.Fatalf("seed %d step %d missed window", seed, step)
				}
				if event != nil {
					applyResumeProjection(t, client, *event)
				}
				want := resumeProjection(t, view)
				if !reflect.DeepEqual(client, want) {
					for key, value := range want {
						if !reflect.DeepEqual(client[key], value) {
							t.Fatalf("seed %d step %d %s differs: got %v want %v", seed, step, key, client[key], value)
						}
					}
				}
				since = view.Revision
			}
		}
	}
}

func TestResumeDuringConcurrentPublishing(t *testing.T) {
	initial := NewGameState()
	store := NewStore(&initial)
	var group sync.WaitGroup
	group.Add(1)
	go func() {
		defer group.Done()
		for level := 1; level <= 300; level++ {
			_, err := store.ApplyComponents(Components(ComponentPlayer), func(state *GameState) ([]string, bool, error) {
				state.Player.Level = level
				return []string{"player"}, true, nil
			})
			if err != nil {
				t.Error(err)
				return
			}
		}
	}()
	for range 300 {
		events, cancel := store.Subscribe(1)
		view, event, ok := store.Resume(store.Instance(), 0)
		if !ok || (view.Revision > 0 && (event == nil || event.Revision != view.Revision || event.Patch.Player.Level != view.Player.Level)) {
			t.Error("resume was not atomic")
		}
		cancel()
		_ = events
	}
	group.Wait()
}

package State

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
	"time"
)

func stateMarshalFixture() GameState {
	state := NewGameState()
	now := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	state.Revision = 7
	state.UpdatedAt = now
	state.Session.ChangedAt = now
	state.Player.ID, state.Player.Name = 42, "Synthetic player"
	state.CatalogVersion, state.LanguageVersion = "fixture-catalog", "fixture-language"
	state.SetMovement(11, MovementState{ID: 11, OwnerPlayerID: 42, TargetX: 21, TargetY: 22, StartedAt: now, ObservedAt: now})
	state.SetMapObservation(MapObservation{KingdomID: 0, X: 10, Y: 11, TypeID: MapTypeForeignLord, Level: 40, ObservedAt: now})
	state.SetMapObservation(MapObservation{KingdomID: 4, X: 20, Y: 21, TypeID: MapTypeRift, Level: 50, ObservedAt: now})
	state.SetScalableEventScore(71, ScalableEventScore{EventID: 71, PlayerScore: 123, ObservedAt: now})
	state.EventScores.ActiveEventID = 71
	state.Castles[1] = CastleState{ID: 1, Production: map[int]ProductionQueue{0: {
		LineID: 0, Capacity: 2, ObservedAt: now, Queued: []QueueItem{},
		Slots: []QueueSlot{{Permanent: true, Occupied: true}, {ExpiresAt: now.Add(time.Minute)}, {}},
	}}}
	return state
}

func TestStateMarshalMatchesGolden(t *testing.T) {
	state := stateMarshalFixture()
	marshal := func(value any) json.RawMessage {
		t.Helper()
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		return raw
	}
	value, pointer := marshal(state), marshal(&state)
	if !bytes.Equal(value, pointer) {
		t.Fatal("value and pointer JSON differ")
	}
	actual := marshal(struct {
		Value          json.RawMessage `json:"value"`
		Pointer        json.RawMessage `json:"pointer"`
		Projection     json.RawMessage `json:"projection"`
		ClientSnapshot json.RawMessage `json:"clientSnapshot"`
	}{value, pointer, marshal(state.clientStateProjection()), marshal(NewClientStateSnapshot(&state))})
	actual = append(actual, '\n')
	const path = "testdata/state_marshal.golden.json"
	if os.Getenv("CITADEL_UPDATE_STATE_MARSHAL_GOLDEN") == "1" {
		if err := os.WriteFile(path, actual, 0644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(actual, want) {
		t.Fatal("state/client JSON changed from commit A golden")
	}
}

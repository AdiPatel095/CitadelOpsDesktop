package State

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBuildingCompletionHistoryIsEphemeralAndSnapshotIsolated(t *testing.T) {
	state := NewGameState()
	event := BuildingCompletionEvent{Opcode: "fco", ConstructionState: BuildingStateUpgradeCompleted, ObservedAt: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}
	building := Building{InstanceID: 31, CompletionEvents: []BuildingCompletionEvent{event}}
	state.Castles[17] = CastleState{ID: 17, Buildings: map[BuildingInstanceID]Building{31: building}, Layout: CastleLayout{Objects: map[BuildingInstanceID]Building{31: building}}}
	store := NewStore(&state)
	snapshot := store.Snapshot()
	snapshot.Castles[17].Buildings[31].CompletionEvents[0].Opcode = "mutated"
	snapshot.Castles[17].Layout.Objects[31].CompletionEvents[0].Opcode = "mutated"
	if current := store.ReadOnlyView(); current.Castles[17].Buildings[31].CompletionEvents[0] != event || current.Castles[17].Layout.Objects[31].CompletionEvents[0] != event {
		t.Fatal("snapshot aliased committed completion history")
	}
	encoded, err := json.Marshal(store.ReadOnlyView())
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "completionEvents") || strings.Contains(string(encoded), "2020-01-01") {
		t.Fatal("diagnostic observations leaked into state persistence or transport")
	}
}

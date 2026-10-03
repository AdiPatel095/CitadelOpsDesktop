package Ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestBuildingCompletionEvidenceIsBoundedAndSurvivesSnapshot(t *testing.T) {
	state := State.NewGameState()
	const castleID State.CastleID = 17
	const objectID State.BuildingInstanceID = 31
	state.Castles[castleID] = State.CastleState{ID: castleID, Focused: true, Buildings: map[State.BuildingInstanceID]State.Building{objectID: {InstanceID: objectID, DefinitionID: 301, ConstructionState: State.BuildingStateUpgradeInProgress}}}
	start := time.Now().UTC()
	code := 0
	for index := 0; index < 12; index++ {
		frame := Protocol.Frame{Opcode: "fco", ResponseCode: &code, ReceivedAt: start.Add(time.Duration(index) * time.Second), Payload: json.RawMessage(`{"O":[301,31,2,3,0,0,15,0,0,0,0,0,0,0,2]}`)}
		_, changed, err := reduceBuildingMutation(context.Background(), frame, &state, nil)
		if err != nil || !changed {
			t.Fatalf("completion event not committed: %v %v", changed, err)
		}
	}
	history := state.Castles[castleID].Buildings[objectID].CompletionEvents
	if len(history) != 8 || !history[0].ObservedAt.Equal(start.Add(4*time.Second)) {
		t.Fatalf("history not bounded: %+v", history)
	}
	// A later JAA replaces object structs but must retain committed events.
	frame := Protocol.Frame{Opcode: "jaa", ResponseCode: &code, ReceivedAt: start.Add(20 * time.Second), Payload: json.RawMessage(`{"gca":{"A":[0,0,0,17],"BD":[[301,31,2,3,0,0,0,0,0,0,0,0,0,0,2]]}}`)}
	_, _, err := reduceCastleSnapshot(context.Background(), frame, &state, nil)
	if err != nil {
		t.Fatal(err)
	}
	history = state.Castles[castleID].Buildings[objectID].CompletionEvents
	if len(history) != 8 || history[7].Opcode != "fco" {
		t.Fatalf("JAA discarded history: %+v", history)
	}
}

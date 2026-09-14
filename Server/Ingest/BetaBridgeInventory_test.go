package Ingest

import (
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestStableEventRefreshRetainsBetaBoosterState(t *testing.T) {
	state := State.NewGameState()
	now := time.Now().UTC()
	state.EventScores.Inventory.GlobalEffectBoostsObservedAt = now
	state.EventScores.Inventory.GlobalEffectBoosts = map[int64]State.GlobalEffectBoostState{7: {GlobalEffectID: 7, Boosted: true, OccurrenceEndsAt: now.Add(time.Hour), ObservedAt: now}}
	expected := state.EventScores.Inventory.GlobalEffectBoosts
	code := 0
	_, _, err := reduceScalableEventSnapshot(t.Context(), Protocol.Frame{Opcode: "sei", ResponseCode: &code, ReceivedAt: now.Add(time.Minute), Payload: json.RawMessage(`{"E":[]}`)}, &state, scalableEventTestGameData(t))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(expected, state.EventScores.Inventory.GlobalEffectBoosts) || !state.EventScores.Inventory.GlobalEffectBoostsObservedAt.Equal(now) {
		t.Fatal("stable event refresh dropped beta purchase state")
	}
}

package Automation

import (
	"encoding/json"
	"reflect"
	"testing"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func TestCommanderFeatureCandidatesFailClosedWhenFeatureIsUnassigned(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Commanders[16] = State.CommanderState{ID: 16, Available: true}

	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		commanderFeatureSection: json.RawMessage(`{"version":1,"assignments":{}}`),
	}}
	candidates, restricted := commanderFeatureCandidates(gameState, configuration, "autoStorm")
	if !restricted || len(candidates) != 0 {
		t.Fatalf("unassigned feature candidates = %#v, restricted = %t", candidates, restricted)
	}

	configuration.Sections[commanderFeatureSection] = json.RawMessage(`{"version":1,"assignments":{"autoStorm":[16,0,16,99]}}`)
	candidates, restricted = commanderFeatureCandidates(gameState, configuration, "autoStorm")
	if !restricted || !reflect.DeepEqual(candidates, []State.CommanderID{0, 16}) {
		t.Fatalf("assigned feature candidates = %#v, restricted = %t", candidates, restricted)
	}
}

package Automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func TestProductionPolicyRequiresObservedCommandContext(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := productionPolicySnapshot(now)
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Status != "blocked" || decision.Request != nil {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestProductionPolicyBuildsIntentFromObservedQueue(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	snapshot := productionPolicySnapshot(now)
	snapshot.State.CommandContext.ProductionSessionKey = 91
	decision, err := NewRecruitPolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatalf("evaluate policy: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "production.enqueue" {
		t.Fatalf("unexpected decision: %#v", decision)
	}
	var arguments map[string]any
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatalf("decode intent arguments: %v", err)
	}
	if arguments["castleId"] != float64(77) || arguments["definitionId"] != float64(489) {
		t.Fatalf("unexpected intent arguments: %#v", arguments)
	}
}

func productionPolicySnapshot(now time.Time) Snapshot {
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77, Name: "Test Castle",
		Production: map[int]State.ProductionQueue{
			0: {
				LineID: 0, Capacity: 5, ObservedAt: now,
				Queued: []State.QueueItem{{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 444}},
			},
		},
	}
	return Snapshot{
		State: gameState,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.recruitTroops": json.RawMessage(`{
				"mode":"global","checkIntervalSec":300,"globalItems":[{"id":489,"amount":0}],
				"castles":{"77":{"enabled":true,"items":[]}}
			}`),
		}},
		Now: now,
	}
}

package Automation

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/State"
)

func TestAutoEquipmentCleanupPolicyMetadata(t *testing.T) {
	policy := NewAutoEquipmentCleanupPolicy()
	if policy.ID() != "autoEquipmentCleanup" || policy.EnabledKey() != "auto_equipment_cleanup" {
		t.Fatalf("unexpected policy identity: %s %s", policy.ID(), policy.EnabledKey())
	}
	sections := policy.WakeSections()
	if len(sections) != 1 || sections[0] != autoEquipmentCleanupSection {
		t.Fatalf("wake sections = %#v", sections)
	}
	if _, stateDriven := any(policy).(StateWakePolicy); stateDriven {
		t.Fatal("equipment cleanup must honor its interval instead of waking for every inventory change")
	}
}

func TestAutoEquipmentCleanupPolicyRunsRefreshEquipmentGemChain(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	policy := NewAutoEquipmentCleanupPolicy()
	gameState := State.NewGameState()

	decision, err := policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	assertCleanupRequest(t, decision, err, "equipment.refresh", "")
	if !decision.ReevaluateOnSuccess {
		t.Fatal("refresh does not continue the cleanup chain")
	}

	markCleanupSnapshotsFresh(&gameState, now)
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, DefinitionID: 100, Slot: 1}
	gameState.Inventory.GemStacks[100] = 2
	decision, err = policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	assertCleanupRequest(t, decision, err, "equipment.sell", "non_relic_equipment")
	if decision.Metrics["eligibleEquipment"] != 1 || decision.Metrics["eligibleGems"] != 2 {
		t.Fatalf("equipment decision metrics = %#v", decision.Metrics)
	}

	delete(gameState.Inventory.Equipment, 101)
	decision, err = policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	assertCleanupRequest(t, decision, err, "equipment.sell", "non_relic_gems")

	delete(gameState.Inventory.GemStacks, 100)
	decision, err = policy.Evaluate(t.Context(), Snapshot{State: gameState, Now: now})
	if err != nil || decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("clean decision = %#v err=%v", decision, err)
	}
}

func TestAutoEquipmentCleanupPolicyUsesPersistedInterval(t *testing.T) {
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	markCleanupSnapshotsFresh(&gameState, now)
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoEquipmentCleanupSection: json.RawMessage(`{"version":1,"checkIntervalSec":600}`),
	}}
	decision, err := NewAutoEquipmentCleanupPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decision.NextCheckAt, now.Add(10*time.Minute); !got.Equal(want) {
		t.Fatalf("next check = %s, want %s", got, want)
	}

	configuration.Sections[autoEquipmentCleanupSection] = json.RawMessage(`{"version":1,"checkIntervalSec":5}`)
	decision, err = NewAutoEquipmentCleanupPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := decision.NextCheckAt, now.Add(time.Minute); !got.Equal(want) {
		t.Fatalf("minimum next check = %s, want %s", got, want)
	}
}

func markCleanupSnapshotsFresh(gameState *State.GameState, now time.Time) {
	gameState.Observations["gei"] = State.ProtocolObservation{LastSuccessfulInboundAt: now}
	gameState.Observations["ggm"] = State.ProtocolObservation{LastSuccessfulInboundAt: now}
}

func assertCleanupRequest(t *testing.T, decision Decision, err error, name string, category string) {
	t.Helper()
	if err != nil || decision.Request == nil || decision.Request.Name != name {
		t.Fatalf("decision = %#v err=%v, want request %s", decision, err, name)
	}
	if !decision.ReevaluateOnStale {
		t.Fatalf("%s request does not recover from stale planning state", name)
	}
	if category == "" {
		return
	}
	var arguments struct {
		Category string `json:"category"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.Category != category {
		t.Fatalf("request arguments = %s err=%v, want category %s", decision.Request.Arguments, err, category)
	}
}

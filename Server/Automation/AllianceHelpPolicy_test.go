package Automation

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAllianceHelpPolicyUsesUrgentAllianceHelpWake(t *testing.T) {
	policy := NewAllianceHelpPolicy()
	if policy.ID() != "autoAllianceHelp" || policy.EnabledKey() != "auto_alliance_help" {
		t.Fatalf("unexpected policy identity: %s %s", policy.ID(), policy.EnabledKey())
	}
	if got := policy.WakeDomains(); len(got) != 1 || got[0] != "alliance-help" {
		t.Fatalf("wake domains = %#v", got)
	}
	if got := policy.UrgentWakeDomains(); len(got) != 1 || got[0] != "alliance-help" {
		t.Fatalf("urgent wake domains = %#v", got)
	}
}

func TestAllianceHelpPolicyBootstrapsCurrentSessionOnce(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.Generation = 7
	decision, err := NewAllianceHelpPolicy().Evaluate(context.Background(), Snapshot{
		State: state, GameData: allianceHelpPolicyTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "ready" || decision.Request == nil ||
		decision.Request.Name != "alliance.help.answer_all" ||
		string(decision.Request.Arguments) != `{"allowUnobserved":true}` {
		t.Fatalf("unexpected unobserved decision: %#v", decision)
	}

	state.AllianceHelpRequests.LastHelpAllGeneration = 7
	decision, err = NewAllianceHelpPolicy().Evaluate(context.Background(), Snapshot{
		State: state, GameData: allianceHelpPolicyTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Request != nil {
		t.Fatalf("bootstrap repeated in the same session: %#v", decision)
	}
}

func TestAllianceHelpPolicyImmediatelyAnswersPendingRequests(t *testing.T) {
	now := time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Session.Generation = 7
	state.AllianceHelpRequests.OthersObservedGeneration = 7
	state.AllianceHelpRequests.OthersObservedAt = now
	state.AllianceHelpRequests.PendingOtherListIDs = []int64{11, 22}
	decision, err := NewAllianceHelpPolicy().Evaluate(context.Background(), Snapshot{
		State: state, GameData: allianceHelpPolicyTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "ready" || decision.Request == nil || decision.Request.Name != "alliance.help.answer_all" {
		t.Fatalf("unexpected ready decision: %#v", decision)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale || decision.Metrics["pendingRequests"] != 2 {
		t.Fatalf("unexpected response workflow: %#v", decision)
	}

	state.AllianceHelpRequests.PendingOtherListIDs = []int64{}
	decision, err = NewAllianceHelpPolicy().Evaluate(context.Background(), Snapshot{
		State: state, GameData: allianceHelpPolicyTestGameData(t), Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "idle" || decision.Request != nil {
		t.Fatalf("unexpected idle decision: %#v", decision)
	}
}

func allianceHelpPolicyTestGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"alliancehelprequests":[{"allianceHelpRequestID":"2","maxHelpersCount":"5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

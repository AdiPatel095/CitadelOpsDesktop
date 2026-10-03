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
	if policy.ID() != "autoAllianceHelp" || policy.EnabledKey() != "" {
		t.Fatalf("unexpected policy identity: %s %s", policy.ID(), policy.EnabledKey())
	}
	if _, ok := any(policy).(CorePolicy); !ok {
		t.Fatal("alliance help must be a core policy")
	}
	accessorState1 := State.GameState{}
	if !policyEnabled(policy, map[string]bool{}, &accessorState1) {
		t.Fatal("core alliance help was disabled without a feature toggle")
	}
	if got := policyScheduleKey(policy); got != "" {
		t.Fatalf("core alliance help schedule key = %q, want none", got)
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
	state.Player.AllianceID = 9
	state.Player.AllianceMembershipObservedAt = now
	state.Player.AllianceMembershipGeneration = 7
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
	state.Player.AllianceID = 9
	state.Player.AllianceMembershipObservedAt = now
	state.Player.AllianceMembershipGeneration = 7
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

func TestAllianceHelpMembershipPausedStatusDeduplicatesAndResumes(t *testing.T) {
	now := time.Now().UTC()
	state := State.NewGameState()
	state.Session.Generation = 7
	store := State.NewStore(&state)
	policy := NewAllianceHelpPolicy()
	coordinator := NewCoordinator(store, nil, nil, nil, policy)
	snapshot := Snapshot{State: state, Now: now, GameData: allianceHelpPolicyTestGameData(t)}
	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		decision.Detail != "Alliance help is paused: you aren't in an alliance" ||
		decision.DetailDescriptor == nil || decision.DetailDescriptor.Key != "server.automation.alliance_help_membership.paused" {
		t.Fatalf("paused status = %+v err=%v", decision, err)
	}
	coordinator.recordDecision(policy.ID(), true, decision)
	revision := store.Revision()
	coordinator.recordDecision(policy.ID(), true, decision)
	if store.Revision() != revision {
		t.Fatal("identical paused status was published twice")
	}
	_, err = store.ApplyComponents(State.Components(State.ComponentPlayer), func(s *State.GameState) ([]string, bool, error) {
		s.Player.AllianceID = 9
		s.Player.AllianceMembershipObservedAt = now
		s.Player.AllianceMembershipGeneration = 7
		return []string{"player", "alliance-help"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot.State = store.ReadOnlyView()
	resumed, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || resumed.Request == nil {
		t.Fatalf("fresh membership did not resume: %+v %v", resumed, err)
	}
	coordinator.recordDecision(policy.ID(), true, resumed)
	if store.ReadOnlyView().Automations[policy.ID()].Detail == decision.Detail {
		t.Fatal("paused message remained after resume")
	}
	coordinator.recordDecision(policy.ID(), true, decision)
	if store.ReadOnlyView().Automations[policy.ID()].Detail != decision.Detail {
		t.Fatal("pause after resume was incorrectly deduplicated")
	}
}

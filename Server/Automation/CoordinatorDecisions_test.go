package Automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

// fixedDecisionPolicy makes the same decision every time apart from a next-check
// time that a test controls.
type fixedDecisionPolicy struct {
	id     string
	detail string
	next   time.Time
}

func (policy *fixedDecisionPolicy) ID() string            { return policy.id }
func (policy *fixedDecisionPolicy) EnabledKey() string    { return policy.id }
func (policy *fixedDecisionPolicy) WakeDomains() []string { return []string{"units"} }
func (policy *fixedDecisionPolicy) Evaluate(_ context.Context, _ Snapshot) (Decision, error) {
	return Decision{Status: "idle", Detail: policy.detail, NextCheckAt: policy.next, Metrics: map[string]float64{"ready": 1}}, nil
}

func decisionFixture(t *testing.T, ids ...string) (*Coordinator, *State.Store, []*fixedDecisionPolicy, map[string]*policyRuntime) {
	t.Helper()
	enabled := map[string]bool{}
	policies := make([]*fixedDecisionPolicy, 0, len(ids))
	asPolicies := make([]Policy, 0, len(ids))
	for _, id := range ids {
		enabled[id] = true
		policy := &fixedDecisionPolicy{id: id, detail: "Waiting for work"}
		policies = append(policies, policy)
		asPolicies = append(asPolicies, policy)
	}
	document, _ := json.Marshal(enabled)
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.enabled": document})
	if err != nil {
		t.Fatal(err)
	}
	state := State.NewStore(coordinatorReadyState())
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}, asPolicies...)
	runtime := map[string]*policyRuntime{}
	for _, id := range ids {
		runtime[id] = &policyRuntime{}
	}
	return coordinator, state, policies, runtime
}

func evaluateAll(coordinator *Coordinator, t *testing.T, runtime map[string]*policyRuntime) {
	t.Helper()
	// What a state-event wake does: stash the deadline, clear it, mark the policy pending.
	for _, current := range runtime {
		if current.stateWakeNextCheck.IsZero() {
			current.stateWakeNextCheck = current.nextCheck
		}
		current.nextCheck = time.Time{}
		current.evaluationPending = true
		current.eventOnly = false
	}
	coordinator.evaluate(t.Context(), runtime, make(chan operationResult, 8))
}

// CIT-44: re-evaluating a policy that makes the same decision does not change the
// store revision, even though its next-check time moves within the same minute
// (it used to be "now + 30 s" and so a new revision on nearly every evaluation).
func TestSameDecisionDoesNotCreateANewRevision(t *testing.T) {
	coordinator, state, policies, runtime := decisionFixture(t, "steady")
	minute := time.Now().UTC().Truncate(time.Minute).Add(time.Hour)
	policies[0].next = minute.Add(5 * time.Second)
	evaluateAll(coordinator, t, runtime)
	first := state.Revision()
	recorded := state.Snapshot().Automations["steady"]
	if first == 0 || recorded.Status != "idle" || recorded.NextCheckAt == nil || !recorded.NextCheckAt.Equal(minute.Add(time.Minute)) {
		t.Fatalf("first decision recorded as %+v at revision %d", recorded, first)
	}

	events, unsubscribe := state.Subscribe(16)
	defer unsubscribe()
	for _, offset := range []time.Duration{5, 20, 35, 55} {
		policies[0].next = minute.Add(offset * time.Second)
		evaluateAll(coordinator, t, runtime)
		if state.Revision() != first {
			t.Fatalf("an unchanged decision with next check +%ds created revision %d", offset, state.Revision())
		}
	}
	if len(events) != 0 {
		t.Fatalf("unchanged decisions published %d state events", len(events))
	}
	// The coordinator's own deadline is exact (an unchanged passive decision keeps the earlier one);
	// only the recorded copy is rounded.
	if got := runtime["steady"].nextCheck; !got.Equal(minute.Add(5 * time.Second)) {
		t.Fatalf("coordinator deadline = %s, want the exact %s", got, minute.Add(5*time.Second))
	}

	// A real change still commits: an earlier deadline in a new minute bucket, then new content.
	policies[0].next = minute
	evaluateAll(coordinator, t, runtime)
	if state.Revision() != first+1 || !state.Snapshot().Automations["steady"].NextCheckAt.Equal(minute) {
		t.Fatalf("a new minute bucket: revision %d next %v", state.Revision(), state.Snapshot().Automations["steady"].NextCheckAt)
	}
	policies[0].detail = "Different detail"
	evaluateAll(coordinator, t, runtime)
	if state.Revision() != first+2 || state.Snapshot().Automations["steady"].Detail != "Different detail" {
		t.Fatalf("a new detail: revision %d detail %q", state.Revision(), state.Snapshot().Automations["steady"].Detail)
	}
}

// CIT-44: one evaluation pass commits at most one automation revision, however many policies decided.
func TestOneEvaluationPassCommitsAtMostOnce(t *testing.T) {
	coordinator, state, policies, runtime := decisionFixture(t, "a", "b", "c", "d", "e")
	minute := time.Now().UTC().Truncate(time.Minute).Add(time.Hour)
	for _, policy := range policies {
		policy.next = minute.Add(10 * time.Second)
	}
	events, unsubscribe := state.Subscribe(32)
	defer unsubscribe()
	before := state.Revision()
	evaluateAll(coordinator, t, runtime)
	if state.Revision() != before+1 || len(events) != 1 {
		t.Fatalf("five first decisions produced %d revisions and %d events, want one", state.Revision()-before, len(events))
	}
	if event := <-events; len(event.Domains) != 1 || event.Domains[0] != "automation" {
		t.Fatalf("event domains = %v", event.Domains)
	}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if state.Snapshot().Automations[id].Status != "idle" {
			t.Fatalf("policy %s was not recorded", id)
		}
	}
	// Two of five change: still one commit.
	policies[1].detail, policies[3].detail = "changed b", "changed d"
	before = state.Revision()
	evaluateAll(coordinator, t, runtime)
	if state.Revision() != before+1 {
		t.Fatalf("two changed decisions produced %d revisions, want one", state.Revision()-before)
	}
	// None change: none.
	before = state.Revision()
	evaluateAll(coordinator, t, runtime)
	if state.Revision() != before {
		t.Fatalf("an unchanged pass produced %d revisions", state.Revision()-before)
	}
}

// A policy's recorded state can move between the read-only view a pass computes
// from and its commit (an intent's own write, a safety lock). The pass replays its
// updates on what is stored then instead of overwriting it, and it computes them
// without holding the store's write lock (the update closure below writes to the store).
func TestBatchReplaysOnStateThatMovedAfterTheView(t *testing.T) {
	coordinator, state, _, _ := decisionFixture(t, "moving")
	_, err := state.ApplyComponents(State.Components(State.ComponentAutomations), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Automations["moving"] = State.AutomationState{ID: "moving", Enabled: true, Status: "idle", LastOperationID: "op-old"}
		return []string{"automation"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	calls := 0
	coordinator.applyAutomationUpdates([]automationUpdate{{id: "moving", update: func(current State.AutomationState) State.AutomationState {
		calls++
		if calls == 1 {
			// Another writer records an operation after this pass took its view.
			_, _ = state.ApplyComponents(State.Components(State.ComponentAutomations), func(gameState *State.GameState) ([]string, bool, error) {
				automation := gameState.Automations["moving"]
				automation.LastOperationID = "op-new"
				gameState.Automations["moving"] = automation
				return []string{"automation"}, true, nil
			})
		}
		current.Status = "waiting"
		current.Detail = "Waiting for something"
		return current
	}}})
	final := state.Snapshot().Automations["moving"]
	if calls != 2 {
		t.Fatalf("the update ran %d times, want once against the view and once replayed", calls)
	}
	if final.Status != "waiting" || final.Detail != "Waiting for something" || final.LastOperationID != "op-new" {
		t.Fatalf("final = %+v; the pass must keep the concurrent write and apply its own change", final)
	}
}

func TestRoundNextCheckRoundsUpToTheMinute(t *testing.T) {
	base := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	for offset, want := range map[time.Duration]time.Duration{
		0: 0, time.Nanosecond: time.Minute, 30 * time.Second: time.Minute, time.Minute: time.Minute,
		time.Minute + time.Second: 2 * time.Minute, 90*time.Minute + 59*time.Second: 91 * time.Minute,
	} {
		value := base.Add(offset)
		if got := roundNextCheck(&value); !got.Equal(base.Add(want)) {
			t.Errorf("roundNextCheck(+%s) = %s, want +%s", offset, got, want)
		}
	}
	if roundNextCheck(nil) != nil {
		t.Error("nil must stay nil")
	}
	var zero time.Time
	if got := roundNextCheck(&zero); got == nil || !got.IsZero() {
		t.Error("a zero time must stay zero")
	}
}

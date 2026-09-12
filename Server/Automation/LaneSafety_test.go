package Automation

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type safetyTestSubmitter struct{ calls chan Intent.Request }

func (submitter *safetyTestSubmitter) Submit(_ context.Context, request Intent.Request) Intent.Receipt {
	submitter.calls <- request
	return Intent.Receipt{Status: Intent.StatusFailed, Failure: &Intent.FailurePresentation{
		SafetyLock: &State.AutomationSafetyLock{OperationID: "incident", Lane: request.AutomationLane},
	}}
}

type safetySharedFeaturePolicy struct {
	coordinatorTestPolicy
	feature string
}

func (policy *safetySharedFeaturePolicy) ActorID() string    { return policy.feature }
func (policy *safetySharedFeaturePolicy) EnabledKey() string { return policy.feature }

func TestSafetyLocksOnlyOriginatingLaneWithinSharedFeature(t *testing.T) {
	families := [][]Policy{
		{NewAutoKhanPolicy(), NewAutoKhanCooldownPolicy(), NewAutoKhanRagePolicy(), NewAutoKhanDefensePolicy()},
		{NewAutoStormPolicy(), NewAutoStormShopPolicy(), NewAutoStormBuildPolicy()},
	}
	for _, family := range families {
		for _, origin := range family {
			t.Run(origin.ID(), func(t *testing.T) {
				feature := policyActorID(origin)
				store := State.NewStore(coordinatorReadyState())
				configuration := openCoordinatorTestConfiguration(t, feature)
				before := configuration.Snapshot().Revision
				registry := Intent.NewRegistry()
				for _, name := range []string{"test.reject", "test.pass"} {
					if err := registry.Register(Intent.Definition{Name: name, Effect: Intent.EffectWrite,
						Planner: func(context.Context, Intent.PlanningContext, json.RawMessage) (Intent.Plan, error) {
							return Intent.Plan{Steps: []Intent.Step{{Action: name}}}, nil
						}}); err != nil {
						t.Fatal(err)
					}
				}
				engine := Intent.NewEngine(registry, store, nil, nil, nil)
				if err := engine.RegisterAction("test.reject", func(context.Context, json.RawMessage) error {
					return Intent.NewResponseCodeError(nil, "sbp", 55)
				}); err != nil {
					t.Fatal(err)
				}
				if err := engine.RegisterAction("test.pass", func(context.Context, json.RawMessage) error { return nil }); err != nil {
					t.Fatal(err)
				}
				var policies []Policy
				runtime := map[string]*policyRuntime{}
				for _, lane := range family {
					if policyActorID(lane) != feature {
						t.Fatal("test family does not share a feature actor")
					}
					name := "test.pass"
					if lane.ID() == origin.ID() {
						name = "test.reject"
					}
					policies = append(policies, &safetySharedFeaturePolicy{feature: feature, coordinatorTestPolicy: coordinatorTestPolicy{
						id: lane.ID(), decision: Decision{Status: "ready", Request: &Intent.Request{Name: name}, NextCheckAt: time.Now().Add(time.Hour)},
					}})
					runtime[lane.ID()] = &policyRuntime{running: lane.ID() != origin.ID()}
				}
				coordinator := NewCoordinator(store, configuration, nil, engine, policies...)
				results := make(chan operationResult, len(family))
				coordinator.evaluate(t.Context(), runtime, results)
				select {
				case result := <-results:
					if result.policyID != origin.ID() || result.receipt.Failure == nil || result.receipt.Failure.SafetyLock == nil || result.receipt.Failure.SafetyLock.Lane != origin.ID() {
						t.Fatalf("wrong originating lock: %#v", result)
					}
				case <-time.After(time.Second):
					t.Fatal("origin operation did not finish")
				}
				for _, current := range runtime {
					current.running = false
					current.evaluationPending = true
				}
				coordinator.evaluate(t.Context(), runtime, results)
				seen := map[string]bool{}
				for range len(family) - 1 {
					select {
					case result := <-results:
						if result.policyID == origin.ID() || result.receipt.Status != Intent.StatusSucceeded {
							t.Fatalf("sibling blocked or origin retried: %#v", result)
						}
						seen[result.policyID] = true
					case <-time.After(time.Second):
						t.Fatal("sibling lane did not run")
					}
				}
				for _, lane := range family {
					locked := store.ReadOnlyView().Automations[lane.ID()].SafetyLock.Active(time.Now())
					if locked != (lane.ID() == origin.ID()) {
						t.Fatalf("lane %s locked=%t", lane.ID(), locked)
					}
					if lane.ID() != origin.ID() && !seen[lane.ID()] {
						t.Fatalf("sibling %s missing", lane.ID())
					}
				}
				if configuration.Snapshot().Revision != before {
					t.Fatal("safety lock modified shared feature configuration")
				}
			})
		}
	}
}

func TestCoordinatorSafetyReceiptSuppressesFailureFallback(t *testing.T) {
	store := State.NewStore(coordinatorReadyState())
	submitter := &safetyTestSubmitter{calls: make(chan Intent.Request, 3)}
	policy := &coordinatorTestPolicy{id: "lane", decision: Decision{Status: "ready",
		Request: &Intent.Request{Name: "primary"}, FailureFallback: &Intent.Request{Name: "fallback"},
		FollowUp: &Intent.Request{Name: "followup"}, NextCheckAt: time.Now().Add(time.Hour)}}
	coordinator := NewCoordinator(store, openCoordinatorTestConfiguration(t, "lane"), nil, submitter, policy)
	results := make(chan operationResult, 1)
	coordinator.evaluate(t.Context(), map[string]*policyRuntime{"lane": {}}, results)
	select {
	case result := <-results:
		if result.failureFallback != nil || result.followUp != nil {
			t.Fatalf("safety lock continued chain: %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("operation did not finish")
	}
	request := <-submitter.calls
	if request.AutomationLane != "lane" || request.Actor != "automation:lane" {
		t.Fatalf("lost lane attribution: %#v", request)
	}
	if len(submitter.calls) != 0 {
		t.Fatal("fallback was submitted")
	}
}

func TestCoordinatorRestoredLockSurvivesConfigurationAndSessionChanges(t *testing.T) {
	initial := coordinatorReadyState()
	lock := State.AutomationSafetyLock{Lane: "lane", Opcode: "cra", Code: 256, OperationID: "incident", ObservedAt: time.Now().UTC()}
	initial.Automations["lane"] = State.AutomationState{ID: "lane", SafetyLock: lock}
	store := State.NewStore(initial)
	policy := &coordinatorTestPolicy{id: "lane", snapshots: make(chan Snapshot, 5), decision: Decision{Status: "idle", EventDriven: true}}
	coordinator := NewCoordinator(store, openCoordinatorTestConfiguration(t, "lane"), nil, nil, policy)
	runtime := map[string]*policyRuntime{"lane": {evaluationPending: true, configurationRebuildPending: true}}
	results := make(chan operationResult, 1)
	coordinator.evaluate(t.Context(), runtime, results)
	coordinator.recordDecision("lane", false, Decision{Status: "disabled"})
	coordinator.recordReceipt(operationResult{policyID: "lane", receipt: Intent.Receipt{Status: Intent.StatusSucceeded}})
	if current := store.ReadOnlyView().Automations["lane"]; current.Status != "gated" || current.SafetyLock != lock {
		t.Fatalf("state update erased lock: %#v", current)
	}
	if len(policy.snapshots) != 0 {
		t.Fatal("locked policy evaluated")
	}
	if next := nextPolicyEvaluationAt(runtime, time.Now()); !next.IsZero() {
		t.Fatalf("unknown lock polling at %v", next)
	}
	_, err := store.ApplyComponents(State.Components(State.ComponentAutomations), func(state *State.GameState) ([]string, bool, error) {
		current := state.Automations["lane"]
		current.SafetyLock.ClearedAt = time.Now().UTC()
		state.Automations["lane"] = current
		return []string{"automation-safety"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	runtime["lane"].evaluationPending = true
	coordinator.evaluate(t.Context(), runtime, results)
	if len(policy.snapshots) != 1 {
		t.Fatal("reviewed policy did not resume evaluation")
	}
}

package Automation

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type coordinatorTestPolicy struct {
	id       string
	domains  []string
	sections []string
	decision Decision
}

func (policy *coordinatorTestPolicy) ID() string { return policy.id }

func (policy *coordinatorTestPolicy) EnabledKey() string { return policy.id }

func (policy *coordinatorTestPolicy) Evaluate(context.Context, Snapshot) (Decision, error) {
	return policy.decision, nil
}

func (policy *coordinatorTestPolicy) WakeDomains() []string { return policy.domains }

func (policy *coordinatorTestPolicy) WakeSections() []string { return policy.sections }

type coordinatorTestPassivePolicy struct {
	id string
}

func (policy *coordinatorTestPassivePolicy) ID() string { return policy.id }

func (policy *coordinatorTestPassivePolicy) EnabledKey() string { return policy.id }

func (policy *coordinatorTestPassivePolicy) Evaluate(context.Context, Snapshot) (Decision, error) {
	return Decision{}, nil
}

type coordinatorTestSubmitter struct {
	calls chan Intent.Request
}

func (submitter *coordinatorTestSubmitter) Submit(_ context.Context, request Intent.Request) Intent.Receipt {
	submitter.calls <- request
	return Intent.Receipt{Status: Intent.StatusSucceeded}
}

type coordinatorTestChainPolicy struct {
	quiesced chan struct{}
	once     sync.Once
}

func (*coordinatorTestChainPolicy) ID() string { return "chain" }

func (*coordinatorTestChainPolicy) EnabledKey() string { return "chain" }

func (*coordinatorTestChainPolicy) WakeDomains() []string { return []string{"chain-progress"} }

func (*coordinatorTestChainPolicy) WakeSections() []string { return []string{"automation.chain"} }

func (policy *coordinatorTestChainPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	next := snapshot.Now.Add(time.Hour)
	if snapshot.State.Player.Level >= 3 {
		policy.once.Do(func() { close(policy.quiesced) })
		return Decision{Status: "idle", Detail: "Chain complete", NextCheckAt: next}, nil
	}
	return Decision{
		Status:              "ready",
		Detail:              "Advance chain",
		NextCheckAt:         next,
		Request:             &Intent.Request{Name: "test.advance", Arguments: json.RawMessage(`{"step":"next"}`)},
		ReevaluateOnSuccess: true,
	}, nil
}

type coordinatorTestChainSubmitter struct {
	state *State.Store
	calls chan struct{}
}

func (submitter *coordinatorTestChainSubmitter) Submit(_ context.Context, _ Intent.Request) Intent.Receipt {
	_, err := submitter.state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Player.Level++
		return []string{"chain-progress"}, true, nil
	})
	if err != nil {
		return Intent.Receipt{Status: Intent.StatusFailed, Error: err.Error()}
	}
	submitter.calls <- struct{}{}
	return Intent.Receipt{Status: Intent.StatusSucceeded}
}

type coordinatorTestBlockingSubmitter struct {
	startedOnce  sync.Once
	canceledOnce sync.Once
	started      chan struct{}
	canceled     chan struct{}
}

func (submitter *coordinatorTestBlockingSubmitter) Submit(ctx context.Context, _ Intent.Request) Intent.Receipt {
	submitter.startedOnce.Do(func() { close(submitter.started) })
	<-ctx.Done()
	submitter.canceledOnce.Do(func() { close(submitter.canceled) })
	return Intent.Receipt{Status: Intent.StatusCancelled, Error: ctx.Err().Error()}
}

func TestCoordinatorIndexesAndRoutesDeclaredWakeDomains(t *testing.T) {
	policies := []Policy{
		&coordinatorTestPolicy{id: "zeta", domains: []string{" MOVEMENTS ", "movements", "protocol", "automation", "session", ""}},
		&coordinatorTestPolicy{id: "alpha", domains: []string{"movements", "inventory"}},
		&coordinatorTestPassivePolicy{id: "passive"},
	}
	indexed := indexPolicyWakeDomains(policies)
	want := map[string][]string{
		"inventory": {"alpha"},
		"movements": {"alpha", "zeta"},
	}
	if !reflect.DeepEqual(indexed, want) {
		t.Fatalf("unexpected wake index: got %#v, want %#v", indexed, want)
	}

	next := time.Now().UTC().Add(time.Hour)
	runtime := map[string]*policyRuntime{
		"alpha":   {nextCheck: next},
		"zeta":    {nextCheck: next},
		"passive": {nextCheck: next},
	}
	if !wakePoliciesForStateEvent(runtime, indexed, State.Event{Revision: 1, Domains: []string{" MOVEMENTS "}}) {
		t.Fatal("state event did not report waking idle policies")
	}
	if !runtime["alpha"].nextCheck.IsZero() || !runtime["zeta"].nextCheck.IsZero() {
		t.Fatal("declared movement consumers were not woken")
	}
	if !runtime["passive"].nextCheck.Equal(next) {
		t.Fatal("state event woke a policy that did not declare the domain")
	}
}

func TestCoordinatorIndexesAndRoutesDeclaredWakeSections(t *testing.T) {
	policies := []Policy{
		&coordinatorTestPolicy{id: "zeta", sections: []string{" automation.alpha ", "automation.alpha", "automation.enabled", "scheduler", ""}},
		&coordinatorTestPolicy{id: "alpha", sections: []string{"automation.alpha", "automation.beta"}},
		&coordinatorTestPassivePolicy{id: "passive"},
	}
	indexed := indexPolicyWakeSections(policies)
	want := map[string][]string{
		"automation.alpha": {"alpha", "zeta"},
		"automation.beta":  {"alpha"},
	}
	if !reflect.DeepEqual(indexed, want) {
		t.Fatalf("unexpected configuration wake index: got %#v, want %#v", indexed, want)
	}

	next := time.Now().UTC().Add(time.Hour)
	runtime := map[string]*policyRuntime{
		"alpha":   {nextCheck: next},
		"zeta":    {nextCheck: next},
		"passive": {nextCheck: next},
	}
	if !wakePoliciesForConfigurationEvent(runtime, indexed, Configuration.Event{
		Revision: 1,
		Section:  " automation.alpha ",
	}) {
		t.Fatal("configuration event did not report waking idle policies")
	}
	if !runtime["alpha"].nextCheck.IsZero() || !runtime["zeta"].nextCheck.IsZero() {
		t.Fatal("declared configuration consumers were not woken")
	}
	if !runtime["passive"].nextCheck.Equal(next) {
		t.Fatal("configuration event woke an unrelated policy")
	}
}

func TestCoordinatorConfigurationFingerprintTracksOnlyRelevantSections(t *testing.T) {
	alpha := &coordinatorTestPolicy{id: "alpha", sections: []string{"automation.alpha"}}
	beta := &coordinatorTestPolicy{id: "beta", sections: []string{"automation.beta"}}
	before := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.enabled": json.RawMessage(`{"alpha":true,"beta":true}`),
		"scheduler":          json.RawMessage(`{}`),
		"automation.alpha":   json.RawMessage(`{"value":1}`),
		"automation.beta":    json.RawMessage(`{"value":1}`),
	}}
	after := before
	after.Sections = map[string]json.RawMessage{}
	for section, value := range before.Sections {
		after.Sections[section] = value
	}
	after.Sections["automation.alpha"] = json.RawMessage(`{"value":2}`)
	if policyConfigurationFingerprint(alpha, before) == policyConfigurationFingerprint(alpha, after) {
		t.Fatal("relevant section change did not change the policy fingerprint")
	}
	if policyConfigurationFingerprint(beta, before) != policyConfigurationFingerprint(beta, after) {
		t.Fatal("unrelated section change changed another policy fingerprint")
	}
}

func TestCoordinatorGlobalConfigurationSectionsWakeOnlyAffectedPolicies(t *testing.T) {
	for _, section := range []string{"automation.enabled", "scheduler"} {
		t.Run(section, func(t *testing.T) {
			configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
				"automation.enabled": json.RawMessage(`{"alpha":true,"beta":true}`),
				"scheduler":          json.RawMessage(`{}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			alpha := &coordinatorTestPolicy{id: "alpha"}
			beta := &coordinatorTestPolicy{id: "beta"}
			coordinator := NewCoordinator(nil, configuration, nil, nil, alpha, beta)
			before := configuration.Snapshot()
			next := time.Now().UTC().Add(time.Hour)
			runtime := map[string]*policyRuntime{
				"alpha": {
					nextCheck: next, evaluatedConfigRevision: before.Revision,
					evaluatedConfiguration: policyConfigurationFingerprint(alpha, before),
				},
				"beta": {
					nextCheck: next, evaluatedConfigRevision: before.Revision,
					evaluatedConfiguration: policyConfigurationFingerprint(beta, before),
				},
			}
			var updated Configuration.Snapshot
			if section == "automation.enabled" {
				updated, err = configuration.Update(section, json.RawMessage(`{"alpha":false,"beta":true}`))
			} else {
				updated, err = configuration.Update(section, json.RawMessage(`{
					"featureSchedules":{"alpha":{"enabled":true,"timeZone":"UTC","slots":[]}}
				}`))
			}
			if err != nil {
				t.Fatal(err)
			}
			if !coordinator.wakePoliciesForConfigurationEvent(runtime, Configuration.Event{
				Revision: updated.Revision, Section: section,
			}) {
				t.Fatal("affected global configuration did not report an idle wake")
			}
			if !runtime["alpha"].nextCheck.IsZero() {
				t.Fatal("affected policy was not woken")
			}
			if !runtime["beta"].nextCheck.Equal(next) {
				t.Fatal("unrelated global configuration change bypassed policy cadence")
			}
		})
	}
}

func TestCoordinatorDisablingPolicyCancelsRunningIntent(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "cancel")
	policy := &coordinatorTestPolicy{id: "cancel", decision: Decision{
		Status:      "ready",
		Detail:      "Wait until cancelled",
		NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request:     &Intent.Request{Name: "test.block"},
	}}
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("policy intent did not start")
	}
	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{"cancel":false}`)); err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("disabling automation did not cancel its running intent")
	}

	deadline := time.Now().Add(time.Second)
	for state.Snapshot().Automations["cancel"].Status != "disabled" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if automation := state.Snapshot().Automations["cancel"]; automation.Status != "disabled" {
		cancel()
		<-done
		t.Fatalf("cancelled policy status = %+v", automation)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not stop after cancellation")
	}
}

func TestCoordinatorPolicySettingsChangeCancelsRunningIntent(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "configured")
	if _, err := configuration.Update("automation.configured", json.RawMessage(`{"unitId":1}`)); err != nil {
		t.Fatal(err)
	}
	policy := &coordinatorTestPolicy{
		id: "configured", sections: []string{"automation.configured"},
		decision: Decision{
			Status: "ready", Detail: "Wait with configuration-derived arguments",
			NextCheckAt: time.Now().UTC().Add(time.Hour),
			Request:     &Intent.Request{Name: "test.block", Arguments: json.RawMessage(`{"unitId":1}`)},
		},
	}
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("configuration-derived intent did not start")
	}
	if _, err := configuration.Update("automation.configured", json.RawMessage(`{"unitId":2}`)); err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("policy settings change did not cancel stale in-flight arguments")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not stop after cancellation")
	}
}

func TestCoordinatorTickerCancelsRunningIntentAfterDroppedConfigurationEvent(t *testing.T) {
	configuration := openCoordinatorTestConfiguration(t, "configured")
	if _, err := configuration.Update("automation.configured", json.RawMessage(`{"unitId":1}`)); err != nil {
		t.Fatal(err)
	}
	policy := &coordinatorTestPolicy{id: "configured", sections: []string{"automation.configured"}}
	coordinator := NewCoordinator(nil, configuration, nil, nil, policy)
	before := configuration.Snapshot()
	cancelled := false
	runtime := map[string]*policyRuntime{"configured": {
		running:                true,
		evaluatedConfiguration: policyConfigurationFingerprint(policy, before),
		cancelRun:              func() { cancelled = true },
	}}
	if _, err := configuration.Update("automation.configured", json.RawMessage(`{"unitId":2}`)); err != nil {
		t.Fatal(err)
	}

	coordinator.cancelRunsDisallowedByConfiguration(runtime, configuration.Snapshot(), time.Now().UTC())

	if !cancelled || !runtime["configured"].configurationWakePending {
		t.Fatalf("dropped configuration event left stale work running: %+v cancelled=%t", runtime["configured"], cancelled)
	}
}

func TestCoordinatorIntentionalConfigurationFollowUpIsNotCancelled(t *testing.T) {
	configuration := openCoordinatorTestConfiguration(t, "configured")
	if _, err := configuration.Update("automation.configured", json.RawMessage(`{"cursor":0}`)); err != nil {
		t.Fatal(err)
	}
	policy := &coordinatorTestPolicy{id: "configured", sections: []string{"automation.configured"}}
	coordinator := NewCoordinator(nil, configuration, nil, nil, policy)
	before := configuration.Snapshot()
	followUpArguments, err := json.Marshal(map[string]any{
		"section": "automation.configured", "value": map[string]any{"cursor": 1},
		"expectedRevision": before.Revision,
	})
	if err != nil {
		t.Fatal(err)
	}
	decision := Decision{FollowUp: &Intent.Request{Name: "config.update", Arguments: followUpArguments}}
	cancelled := false
	runtime := map[string]*policyRuntime{"configured": {
		running:                    true,
		allowedConfigurationChange: configurationFollowUpFingerprint(policy, decision, before),
		evaluatedConfigRevision:    before.Revision,
		evaluatedConfiguration:     policyConfigurationFingerprint(policy, before),
		cancelRun:                  func() { cancelled = true },
	}}
	after, err := configuration.Update("automation.configured", json.RawMessage(`{"cursor":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if coordinator.wakePoliciesForConfigurationEvent(runtime, Configuration.Event{
		Revision: after.Revision, Section: "automation.configured",
	}) {
		t.Fatal("running policy reported an idle configuration wake")
	}
	if cancelled || !runtime["configured"].configurationWakePending {
		t.Fatalf("intentional configuration follow-up handling = %+v cancelled=%t", runtime["configured"], cancelled)
	}
	coordinator.cancelRunsDisallowedByConfiguration(runtime, configuration.Snapshot(), time.Now().UTC())
	if cancelled {
		t.Fatal("ticker fallback cancelled the operation's declared configuration follow-up")
	}
	after, err = configuration.Update("automation.configured", json.RawMessage(`{"cursor":2}`))
	if err != nil {
		t.Fatal(err)
	}
	coordinator.wakePoliciesForConfigurationEvent(runtime, Configuration.Event{
		Revision: after.Revision, Section: "automation.configured",
	})
	if !cancelled {
		t.Fatal("unexpected settings change matched the operation's declared configuration follow-up")
	}
}

func TestCoordinatorSessionLossCancelsRunningIntent(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "session")
	policy := &coordinatorTestPolicy{id: "session", decision: Decision{
		Status:      "ready",
		Detail:      "Wait until the session closes",
		NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request:     &Intent.Request{Name: "test.block"},
	}}
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()

	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("policy intent did not start")
	}
	if _, err := state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.LoggedIn = false
		gameState.Session.SocketReady = false
		return []string{"session"}, true, nil
	}); err != nil {
		cancel()
		<-done
		t.Fatal(err)
	}
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		cancel()
		<-done
		t.Fatal("session loss did not cancel the running intent")
	}

	deadline := time.Now().Add(time.Second)
	for state.Snapshot().Automations["session"].Status != "waiting" && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if automation := state.Snapshot().Automations["session"]; automation.Status != "waiting" {
		cancel()
		<-done
		t.Fatalf("session-cancelled policy status = %+v", automation)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not stop after cancellation")
	}
}

func TestCoordinatorSessionGenerationChangeCancelsRunningIntent(t *testing.T) {
	cancelled := false
	runtime := map[string]*policyRuntime{"policy": {
		running: true, runningSessionGeneration: 4, cancelRun: func() { cancelled = true },
	}}
	state := coordinatorReadyState()
	state.Session.Generation = 5
	state.Session.BaselineGeneration = 5
	NewCoordinator(nil, nil, nil, nil).cancelRunsForUnavailableSession(runtime, state)
	if !cancelled || !runtime["policy"].runtimeWakePending {
		t.Fatalf("session generation change did not cancel stale work: %+v", runtime["policy"])
	}
}

func TestCoordinatorDisallowedScheduleCancelsRunningIntent(t *testing.T) {
	configuration := openCoordinatorTestConfiguration(t, "scheduled")
	policy := &coordinatorTestPolicy{id: "scheduled", decision: Decision{
		Status:      "ready",
		Detail:      "Run inside the castle schedule",
		NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request:     &Intent.Request{Name: "test.block"},
		ScheduleKey: "scheduled:1",
	}}
	state := State.NewStore(coordinatorReadyState())
	submitter := &coordinatorTestBlockingSubmitter{
		started: make(chan struct{}), canceled: make(chan struct{}),
	}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	runContext, cancelRun := context.WithCancel(context.Background())
	defer cancelRun()
	runtime := map[string]*policyRuntime{
		"scheduled": {},
	}
	results := make(chan operationResult, 1)
	coordinator.evaluate(runContext, runtime, results)
	select {
	case <-submitter.started:
	case <-time.After(time.Second):
		t.Fatal("scheduled intent did not start")
	}
	if current := runtime["scheduled"]; current.runningScheduleKey != "scheduled:1" {
		t.Fatalf("decision schedule key was not propagated: %+v", current)
	}
	if _, err := configuration.Update("scheduler", json.RawMessage(`{
		"featureSchedules":{"scheduled:1":{"enabled":true,"timeZone":"UTC","slots":[]}}
	}`)); err != nil {
		t.Fatal(err)
	}
	coordinator.cancelDisallowedPolicyRuns(
		runtime,
		Configuration.Event{Section: "scheduler"},
		time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC),
	)
	select {
	case <-submitter.canceled:
	case <-time.After(time.Second):
		t.Fatal("disallowed schedule did not cancel the running intent")
	}
	current := runtime["scheduled"]
	if !current.configurationWakePending {
		t.Fatal("schedule cancellation did not latch a latest-configuration evaluation")
	}
	var result operationResult
	select {
	case result = <-results:
	case <-time.After(time.Second):
		t.Fatal("cancelled scheduled intent did not return a result")
	}
	result, wakeImmediately := completePolicyRun(current, result, time.Now().UTC())
	if !wakeImmediately || !result.nextCheck.IsZero() {
		t.Fatalf("schedule cancellation retained the stale decision deadline: result=%+v runtime=%+v", result, current)
	}
}

func TestCoordinatorSchedulerChangeBypassesStaleDeadline(t *testing.T) {
	configuration := openCoordinatorTestConfiguration(t, "scheduled")
	if _, err := configuration.Update("scheduler", json.RawMessage(`{
		"featureSchedules":{"scheduled":{"enabled":true,"timeZone":"UTC","slots":[]}}
	}`)); err != nil {
		t.Fatal(err)
	}
	previousConfiguration := configuration.Snapshot()
	if _, err := configuration.Update("scheduler", json.RawMessage(`{}`)); err != nil {
		t.Fatal(err)
	}
	next := time.Now().UTC().Add(2 * time.Hour)
	policy := &coordinatorTestPolicy{id: "scheduled", decision: Decision{
		Status: "idle", Detail: "Reevaluated latest scheduler", NextCheckAt: next,
	}}
	state := State.NewStore(coordinatorReadyState())
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	runtime := map[string]*policyRuntime{
		"scheduled": {
			nextCheck:              time.Now().UTC().Add(time.Hour),
			evaluatedConfiguration: policyConfigurationFingerprint(policy, previousConfiguration),
		},
	}
	coordinator.evaluate(context.Background(), runtime, make(chan operationResult, 1))
	if !runtime["scheduled"].nextCheck.Equal(next) {
		t.Fatalf("scheduler change retained stale deadline: %+v", runtime["scheduled"])
	}
	if automation := state.Snapshot().Automations["scheduled"]; automation.Detail != "Reevaluated latest scheduler" {
		t.Fatalf("scheduler change did not reevaluate policy: %+v", automation)
	}
}

func TestCoordinatorSessionReadinessBypassesRetryDeadline(t *testing.T) {
	gameState := State.NewGameState()
	state := State.NewStore(gameState)
	configuration := openCoordinatorTestConfiguration(t, "session-ready")
	next := time.Now().UTC().Add(2 * time.Hour)
	policy := &coordinatorTestPolicy{id: "session-ready", decision: Decision{
		Status: "idle", Detail: "Evaluated after session became ready", NextCheckAt: next,
	}}
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	runtime := map[string]*policyRuntime{"session-ready": {}}
	results := make(chan operationResult, 1)

	coordinator.evaluate(context.Background(), runtime, results)
	waitingDeadline := runtime["session-ready"].nextCheck
	if waitingDeadline.IsZero() || !runtime["session-ready"].evaluatedSessionKnown ||
		runtime["session-ready"].evaluatedSessionReady {
		t.Fatalf("unready session did not establish a retry deadline: %+v", runtime["session-ready"])
	}
	if _, err := state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.LoggedIn = true
		gameState.Session.SocketReady = true
		gameState.Session.Generation = 1
		gameState.Session.BaselineGeneration = 1
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	coordinator.evaluate(context.Background(), runtime, results)

	current := runtime["session-ready"]
	if !current.evaluatedSessionReady || !current.nextCheck.Equal(next) || current.nextCheck.Equal(waitingDeadline) {
		t.Fatalf("ready session remained behind its stale retry deadline: %+v", current)
	}
	if automation := state.Snapshot().Automations["session-ready"]; automation.Detail != "Evaluated after session became ready" {
		t.Fatalf("ready session did not reevaluate policy: %+v", automation)
	}
}

func TestCoordinatorWaitsForCurrentSessionBaseline(t *testing.T) {
	gameState := coordinatorReadyState()
	gameState.Session.BaselineGeneration = 0
	state := State.NewStore(gameState)
	configuration := openCoordinatorTestConfiguration(t, "baseline")
	next := time.Now().UTC().Add(time.Hour)
	policy := &coordinatorTestPolicy{id: "baseline", decision: Decision{
		Status: "idle", Detail: "Evaluated hydrated session", NextCheckAt: next,
	}}
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	runtime := map[string]*policyRuntime{"baseline": {}}
	results := make(chan operationResult, 1)
	coordinator.evaluate(context.Background(), runtime, results)
	if automation := state.Snapshot().Automations["baseline"]; automation.Status != "waiting" ||
		automation.Detail != "Waiting for the current game-session baseline" {
		t.Fatalf("unhydrated session decision = %+v", automation)
	}
	if _, err := state.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Session.BaselineGeneration = gameState.Session.Generation
		return []string{"session"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	coordinator.evaluate(context.Background(), runtime, results)
	if automation := state.Snapshot().Automations["baseline"]; automation.Detail != "Evaluated hydrated session" ||
		!runtime["baseline"].nextCheck.Equal(next) {
		t.Fatalf("hydrated session did not bypass retry deadline: automation=%+v runtime=%+v", automation, runtime["baseline"])
	}
}

func TestCoordinatorSessionEventWakesIdleAndLatchesRunningPolicies(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	runtime := map[string]*policyRuntime{
		"idle":    {nextCheck: next, evaluatedStateRevision: 3},
		"running": {nextCheck: next, running: true, evaluatedStateRevision: 3},
	}
	if !wakePoliciesForStateEvent(runtime, nil, State.Event{Revision: 4, Domains: []string{"session"}}) {
		t.Fatal("session event did not report waking an idle policy")
	}
	if !runtime["idle"].nextCheck.IsZero() {
		t.Fatal("session event did not wake an idle policy")
	}
	if !runtime["running"].runtimeWakePending {
		t.Fatal("session event was not latched for a running policy")
	}
	if !runtime["running"].nextCheck.Equal(next) {
		t.Fatal("session event changed a running policy deadline before completion")
	}
}

func TestCoordinatorDomainEventRecordsRunningStateProgress(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	runtime := map[string]*policyRuntime{
		"autoRecruit": {nextCheck: next, running: true},
	}
	if wakePoliciesForStateEvent(
		runtime,
		map[string][]string{"production": {"autoRecruit"}},
		State.Event{Revision: 1, Domains: []string{"production"}},
	) {
		t.Fatal("running-only state event incorrectly reported an idle wake")
	}
	current := runtime["autoRecruit"]
	if !current.stateProgressPending || current.runtimeWakePending {
		t.Fatalf("running state progress was not recorded correctly: %+v", current)
	}
	if !current.nextCheck.Equal(next) {
		t.Fatal("domain event changed a running policy deadline")
	}

	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:            "autoRecruit",
		receipt:             Intent.Receipt{Status: Intent.StatusSucceeded},
		nextCheck:           next,
		reevaluateOnSuccess: true,
	}, time.Now().UTC())
	if !wakeImmediately || !result.nextCheck.IsZero() {
		t.Fatalf("successful progress chain was not continued: result=%+v runtime=%+v", result, current)
	}
	if current.stateProgressPending || current.rejectRepeatedDecision {
		t.Fatalf("observed state progress was not consumed correctly: %+v", current)
	}
}

func TestCoordinatorIgnoresAlreadyEvaluatedWakeRevisions(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	stateRuntime := map[string]*policyRuntime{
		"policy": {
			nextCheck: next, evaluatedStateRevision: 10, immediateRuns: maxImmediatePolicyRuns,
			submissionBlockedUntil: next, blockedDecisionFingerprint: "stale-state",
		},
	}
	indexedDomains := map[string][]string{"production": {"policy"}}
	if wakePoliciesForStateEvent(stateRuntime, indexedDomains, State.Event{Revision: 10, Domains: []string{"production"}}) {
		t.Fatal("already-evaluated state revision reported a wake")
	}
	if !stateRuntime["policy"].nextCheck.Equal(next) {
		t.Fatal("already-evaluated state revision woke the policy")
	}
	if !wakePoliciesForStateEvent(stateRuntime, indexedDomains, State.Event{Revision: 11, Domains: []string{"production"}}) {
		t.Fatal("new state revision did not report a wake")
	}
	if !stateRuntime["policy"].nextCheck.IsZero() {
		t.Fatal("new state revision did not wake the policy")
	}
	if !stateRuntime["policy"].submissionBlockedUntil.IsZero() || stateRuntime["policy"].immediateRuns != 0 {
		t.Fatalf("authoritative state progress retained a safety pause: %+v", stateRuntime["policy"])
	}

	configurationRuntime := map[string]*policyRuntime{
		"policy": {
			nextCheck: next, evaluatedConfigRevision: 6, immediateRuns: maxImmediatePolicyRuns,
			submissionBlockedUntil: next, blockedDecisionFingerprint: "stale-configuration",
		},
	}
	indexedSections := map[string][]string{"automation.policy": {"policy"}}
	if wakePoliciesForConfigurationEvent(configurationRuntime, indexedSections, Configuration.Event{
		Revision: 6,
		Section:  "automation.policy",
	}) {
		t.Fatal("already-evaluated configuration revision reported a wake")
	}
	if !configurationRuntime["policy"].nextCheck.Equal(next) {
		t.Fatal("already-evaluated configuration revision woke the policy")
	}
	if !wakePoliciesForConfigurationEvent(configurationRuntime, indexedSections, Configuration.Event{
		Revision: 7,
		Section:  "automation.policy",
	}) {
		t.Fatal("new configuration revision did not report a wake")
	}
	if !configurationRuntime["policy"].nextCheck.IsZero() {
		t.Fatal("new configuration revision did not wake the policy")
	}
	if !configurationRuntime["policy"].submissionBlockedUntil.IsZero() || configurationRuntime["policy"].immediateRuns != 0 {
		t.Fatalf("authoritative configuration change retained a safety pause: %+v", configurationRuntime["policy"])
	}
}

func TestCoordinatorOptInSuccessReevaluatesImmediately(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	current := &policyRuntime{nextCheck: next, running: true, lastDecisionFingerprint: "request"}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:            "autoRecruit",
		receipt:             Intent.Receipt{Status: Intent.StatusSucceeded},
		nextCheck:           next,
		reevaluateOnSuccess: true,
	}, time.Now().UTC())
	if !wakeImmediately || !result.nextCheck.IsZero() || !current.nextCheck.IsZero() {
		t.Fatalf("opt-in success was deferred: result=%+v runtime=%+v", result, current)
	}
	if current.running || current.immediateRuns != 1 || !current.rejectRepeatedDecision {
		t.Fatalf("completed runtime was not prepared for guarded immediate reevaluation: %+v", current)
	}
}

func TestCoordinatorNonOptInSuccessPreservesSchedule(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	current := &policyRuntime{
		nextCheck:               next,
		running:                 true,
		immediateRuns:           4,
		lastDecisionFingerprint: "request",
		rejectRepeatedDecision:  true,
		submissionBlockedUntil:  next.Add(time.Hour),
	}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:  "autoBeriWorld",
		receipt:   Intent.Receipt{Status: Intent.StatusSucceeded},
		nextCheck: next,
	}, time.Now().UTC())
	if wakeImmediately || !result.nextCheck.Equal(next) || !current.nextCheck.Equal(next) {
		t.Fatalf("non-opt-in success changed cadence: result=%+v runtime=%+v", result, current)
	}
	if current.immediateRuns != 0 || current.lastDecisionFingerprint != "" ||
		current.rejectRepeatedDecision || !current.submissionBlockedUntil.IsZero() {
		t.Fatalf("completed scheduled run retained continuation state: %+v", current)
	}
}

func TestCoordinatorFailedOperationsPreserveSchedule(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	tests := []struct {
		name     string
		receipt  Intent.Receipt
		followUp *Intent.Receipt
	}{
		{name: "primary", receipt: Intent.Receipt{Status: Intent.StatusFailed}},
		{
			name:     "follow-up",
			receipt:  Intent.Receipt{Status: Intent.StatusSucceeded},
			followUp: &Intent.Receipt{Status: Intent.StatusFailed},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			current := &policyRuntime{nextCheck: next, running: true}
			result, wakeImmediately := completePolicyRun(current, operationResult{
				policyID:            "policy",
				receipt:             test.receipt,
				followUp:            test.followUp,
				nextCheck:           next,
				reevaluateOnSuccess: true,
			}, time.Now().UTC())
			if wakeImmediately || !result.nextCheck.Equal(next) || !current.nextCheck.Equal(next) {
				t.Fatalf("failed operation changed cadence: result=%+v runtime=%+v", result, current)
			}
		})
	}
}

func TestCoordinatorConfigurationWakePendingRunsImmediately(t *testing.T) {
	next := time.Now().UTC().Add(time.Hour)
	current := &policyRuntime{
		nextCheck:                next,
		running:                  true,
		configurationWakePending: true,
	}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:  "policy",
		receipt:   Intent.Receipt{Status: Intent.StatusFailed},
		nextCheck: next,
	}, time.Now().UTC())
	if !wakeImmediately || !result.nextCheck.IsZero() || !current.nextCheck.IsZero() {
		t.Fatalf("pending configuration wake was deferred: result=%+v runtime=%+v", result, current)
	}
	if current.configurationWakePending || current.running {
		t.Fatalf("pending configuration wake was not cleared on completion: %+v", current)
	}
}

func TestCoordinatorImmediateRunCapAllowsTerminalEvaluation(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	current := &policyRuntime{
		nextCheck:     next,
		running:       true,
		immediateRuns: maxImmediatePolicyRuns - 1,
	}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:            "autoRecruit",
		receipt:             Intent.Receipt{Status: Intent.StatusSucceeded},
		nextCheck:           next,
		reevaluateOnSuccess: true,
	}, now)
	if !wakeImmediately || !result.nextCheck.IsZero() || !current.nextCheck.IsZero() {
		t.Fatalf("run cap skipped its terminal evaluation: result=%+v runtime=%+v", result, current)
	}
	if current.immediateRuns != 0 || current.rejectRepeatedDecision {
		t.Fatalf("run cap did not reset continuation guards: %+v", current)
	}
	wantBlock := now.Add(defaultRetry)
	if !current.submissionBlockedUntil.Equal(wantBlock) {
		t.Fatalf("run cap block = %s, want %s", current.submissionBlockedUntil, wantBlock)
	}
}

func TestCoordinatorProgressingChainsDoNotConsumeNoProgressCap(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	current := &policyRuntime{}
	for run := 0; run < maxImmediatePolicyRuns*2; run++ {
		current.running = true
		current.stateProgressPending = true
		result, wakeImmediately := completePolicyRun(current, operationResult{
			policyID:            "progressing",
			receipt:             Intent.Receipt{Status: Intent.StatusSucceeded},
			nextCheck:           now.Add(time.Hour),
			reevaluateOnSuccess: true,
		}, now)
		if !wakeImmediately || !result.nextCheck.IsZero() || current.immediateRuns != 0 ||
			!current.submissionBlockedUntil.IsZero() {
			t.Fatalf("progressing run %d consumed the no-progress budget: result=%+v runtime=%+v", run, result, current)
		}
	}
}

func TestCoordinatorRepeatedDecisionFingerprintPausesSubmission(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "repeat")
	decision := Decision{
		Status:              "ready",
		Detail:              "Repeat action",
		NextCheckAt:         time.Now().UTC().Add(time.Hour),
		Request:             &Intent.Request{Name: "test.repeat", Arguments: json.RawMessage(`{"slot":1}`)},
		ReevaluateOnSuccess: true,
	}
	policy := &coordinatorTestPolicy{id: "repeat", decision: decision}
	submitter := &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	fingerprint := decisionRequestFingerprint(decision)
	runtime := map[string]*policyRuntime{
		"repeat": {
			rejectRepeatedDecision:     true,
			lastDecisionFingerprint:    fingerprint,
			evaluatedConfiguration:     policyConfigurationFingerprint(policy, configuration.Snapshot()),
			evaluatedSessionKnown:      true,
			evaluatedSessionReady:      true,
			evaluatedSessionGeneration: 1,
		},
	}
	results := make(chan operationResult, 1)
	started := time.Now().UTC()
	coordinator.evaluate(context.Background(), runtime, results)

	select {
	case request := <-submitter.calls:
		t.Fatalf("repeated decision was submitted: %+v", request)
	case <-time.After(20 * time.Millisecond):
	}
	current := runtime["repeat"]
	if current.running || current.submissionBlockedUntil.IsZero() ||
		!current.nextCheck.Equal(current.submissionBlockedUntil) {
		t.Fatalf("repeated decision did not enter a safety pause: %+v", current)
	}
	if current.submissionBlockedUntil.Before(started.Add(defaultRetry)) {
		t.Fatalf("repeated decision pause was shorter than retry interval: %+v", current)
	}
	automation := state.Snapshot().Automations["repeat"]
	if automation.Status != "waiting" || !strings.Contains(automation.Detail, "Safety pause") {
		t.Fatalf("repeated decision status was not recorded: %+v", automation)
	}
}

func TestCoordinatorChangedDecisionBypassesFingerprintPause(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "changed")
	decision := Decision{
		Status:      "ready",
		Detail:      "Urgent changed action",
		NextCheckAt: time.Now().UTC().Add(time.Hour),
		Request:     &Intent.Request{Name: "test.changed", Arguments: json.RawMessage(`{"slot":2}`)},
	}
	policy := &coordinatorTestPolicy{id: "changed", decision: decision}
	submitter := &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	runtime := map[string]*policyRuntime{
		"changed": {
			submissionBlockedUntil:     time.Now().UTC().Add(time.Hour),
			blockedDecisionFingerprint: "test.previous\x00{\"slot\":1}\x00no-follow-up",
		},
	}
	results := make(chan operationResult, 1)
	coordinator.evaluate(context.Background(), runtime, results)

	select {
	case request := <-submitter.calls:
		if request.Name != "test.changed" {
			t.Fatalf("submitted request = %+v", request)
		}
	case <-time.After(time.Second):
		t.Fatal("changed decision remained behind the repeated-decision pause")
	}
	if !runtime["changed"].submissionBlockedUntil.IsZero() ||
		runtime["changed"].blockedDecisionFingerprint != "" {
		t.Fatalf("changed decision retained the old fingerprint pause: %+v", runtime["changed"])
	}
}

func TestCoordinatorRunCompletesResponseGatedChainBeforeTicker(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, "chain")
	policy := &coordinatorTestChainPolicy{quiesced: make(chan struct{})}
	submitter := &coordinatorTestChainSubmitter{state: state, calls: make(chan struct{}, 4)}
	coordinator := NewCoordinator(state, configuration, nil, submitter, policy)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	started := time.Now()
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()

	timeout := time.After(time.Second)
	for count := 0; count < 3; count++ {
		select {
		case <-submitter.calls:
		case <-timeout:
			cancel()
			<-done
			t.Fatalf("response-gated chain submitted only %d actions before timeout", count)
		}
	}
	select {
	case <-policy.quiesced:
	case <-timeout:
		cancel()
		<-done
		t.Fatal("response-gated chain did not reach its terminal evaluation")
	}
	if elapsed := time.Since(started); elapsed >= time.Second {
		t.Fatalf("response-gated chain took %s; expected less than one second", elapsed)
	}

	select {
	case <-submitter.calls:
		cancel()
		<-done
		t.Fatal("response-gated chain submitted after reaching terminal state")
	case <-time.After(100 * time.Millisecond):
	}
	cancel()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("coordinator did not stop after cancellation")
	}
}

func TestCoordinatorDrainHelpersHonorBounds(t *testing.T) {
	stateEvents := make(chan State.Event, 3)
	for revision := uint64(1); revision <= 3; revision++ {
		stateEvents <- State.Event{Revision: revision}
	}
	stateHandled := 0
	stateWake := drainStateEvents(stateEvents, 2, func(event State.Event) bool {
		stateHandled++
		return event.Revision == 2
	})
	if !stateWake || stateHandled != 2 || len(stateEvents) != 1 {
		t.Fatalf("bounded state drain handled=%d remaining=%d wake=%t", stateHandled, len(stateEvents), stateWake)
	}

	configurationEvents := make(chan Configuration.Event, 3)
	for revision := uint64(1); revision <= 3; revision++ {
		configurationEvents <- Configuration.Event{Revision: revision}
	}
	configurationHandled := 0
	configurationWake := drainConfigurationEvents(configurationEvents, 2, func(event Configuration.Event) bool {
		configurationHandled++
		return event.Revision == 1
	})
	if !configurationWake || configurationHandled != 2 || len(configurationEvents) != 1 {
		t.Fatalf(
			"bounded configuration drain handled=%d remaining=%d wake=%t",
			configurationHandled,
			len(configurationEvents),
			configurationWake,
		)
	}
}

func TestCoordinatorAutoTowerFailureUsesRetryBackoff(t *testing.T) {
	now := time.Date(2026, time.July, 13, 12, 0, 0, 0, time.UTC)
	next := now.Add(time.Hour)
	current := &policyRuntime{nextCheck: next, running: true}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:  "autoTowers",
		receipt:   Intent.Receipt{Status: Intent.StatusFailed},
		nextCheck: next,
	}, now)
	want := now.Add(defaultRetry)
	if wakeImmediately || !result.nextCheck.Equal(want) || !current.nextCheck.Equal(want) {
		t.Fatalf("Auto Tower failure did not use retry backoff: result=%+v runtime=%+v", result, current)
	}
}

func TestCoordinatorAutoInvasionFailureBackoffSurvivesStateWake(t *testing.T) {
	now := time.Date(2026, time.July, 15, 20, 0, 0, 0, time.UTC)
	current := &policyRuntime{running: true, evaluatedStateRevision: 10}
	result, wakeImmediately := completePolicyRun(current, operationResult{
		policyID:  "autoInvasion",
		receipt:   Intent.Receipt{Status: Intent.StatusFailed},
		nextCheck: now.Add(2 * time.Second),
	}, now)
	want := now.Add(defaultRetry)
	if wakeImmediately || !result.nextCheck.Equal(want) || !current.failureBlockedUntil.Equal(want) {
		t.Fatalf("Auto Invasion failure did not establish retry backoff: result=%+v runtime=%+v", result, current)
	}

	runtime := map[string]*policyRuntime{"autoInvasion": current}
	indexed := map[string][]string{"units": {"autoInvasion"}}
	if !wakePoliciesForStateEvent(runtime, indexed, State.Event{Revision: 11, Domains: []string{"units"}}) {
		t.Fatal("fresh unit state did not wake the idle policy")
	}
	if !current.nextCheck.IsZero() || !current.failureBlockedUntil.Equal(want) {
		t.Fatalf("state wake erased failed-operation backoff: %+v", current)
	}
}

func coordinatorReadyState() State.GameState {
	gameState := State.NewGameState()
	gameState.Session.LoggedIn = true
	gameState.Session.SocketReady = true
	gameState.Session.Generation = 1
	gameState.Session.BaselineGeneration = 1
	return gameState
}

func openCoordinatorTestConfiguration(t *testing.T, enabledPolicy string) *Configuration.Store {
	t.Helper()
	enabled, err := json.Marshal(map[string]bool{enabledPolicy: true})
	if err != nil {
		t.Fatal(err)
	}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled": enabled,
	})
	if err != nil {
		t.Fatal(err)
	}
	return configuration
}

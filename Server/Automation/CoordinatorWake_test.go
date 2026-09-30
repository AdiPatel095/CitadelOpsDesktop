package Automation

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

// wakeProbePolicy records when the coordinator evaluates it.
type wakeProbePolicy struct {
	id        string
	domains   []string
	urgent    []string
	nextCheck time.Duration // 0: event-driven
	mu        sync.Mutex
	times     []time.Time
}

func (policy *wakeProbePolicy) ID() string                  { return policy.id }
func (policy *wakeProbePolicy) EnabledKey() string          { return policy.id }
func (policy *wakeProbePolicy) WakeDomains() []string       { return policy.domains }
func (policy *wakeProbePolicy) UrgentWakeDomains() []string { return policy.urgent }

func (policy *wakeProbePolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	policy.mu.Lock()
	policy.times = append(policy.times, time.Now())
	policy.mu.Unlock()
	if policy.nextCheck > 0 {
		return Decision{Status: "waiting", Detail: "Waiting for its deadline", NextCheckAt: snapshot.Now.Add(policy.nextCheck)}, nil
	}
	return Decision{Status: "armed", Detail: "Watching", EventDriven: true}, nil
}

func (policy *wakeProbePolicy) evaluations() []time.Time {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	return append([]time.Time(nil), policy.times...)
}

func (policy *wakeProbePolicy) count() int { return len(policy.evaluations()) }

func openWakeConfiguration(t *testing.T, enabled ...string) *Configuration.Store {
	t.Helper()
	controls := map[string]bool{}
	for _, key := range enabled {
		controls[key] = true
	}
	document, _ := json.Marshal(controls)
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{"automation.enabled": document})
	if err != nil {
		t.Fatal(err)
	}
	return configuration
}

func runCoordinator(t *testing.T, state *State.Store, configuration *Configuration.Store, policies ...Policy) {
	t.Helper()
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 4)}, policies...)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		coordinator.Run(ctx)
		close(done)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitUntil(t *testing.T, description string, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(4 * time.Second)
	for !condition() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", description)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func commitDomain(t *testing.T, state *State.Store, domain string) {
	t.Helper()
	if _, err := state.Apply(func(*State.GameState) ([]string, bool, error) { return []string{domain}, true, nil }); err != nil {
		t.Fatal(err)
	}
}

// CIT-43 behaviour 1: a disabled policy is never evaluated because of a state event.
func TestDisabledPolicyIsNeverEvaluatedBecauseOfAStateEvent(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openWakeConfiguration(t, "wake-on")
	on := &wakeProbePolicy{id: "wake-on", domains: []string{"units"}}
	off := &wakeProbePolicy{id: "wake-off", domains: []string{"units"}}
	runCoordinator(t, state, configuration, on, off)
	waitUntil(t, "the initial evaluation", func() bool { return on.count() == 1 })
	waitUntil(t, "the disabled policy's status to be recorded", func() bool { return state.Snapshot().Automations["wake-off"].Status == "disabled" })

	for index := 0; index < 5; index++ {
		commitDomain(t, state, "units")
		time.Sleep(20 * time.Millisecond)
	}
	waitUntil(t, "the enabled policy to follow the events", func() bool { return on.count() >= 2 })
	time.Sleep(2 * stateChangeDebounce)
	if got := off.count(); got != 0 {
		t.Fatalf("a disabled policy was evaluated %d times", got)
	}
	if automation := state.Snapshot().Automations["wake-off"]; automation.Enabled || automation.Status != "disabled" {
		t.Fatalf("disabled policy status = %+v", automation)
	}

	// Switching it on is never missed: the configuration change re-evaluates it, and then state events wake it.
	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{"wake-on":true,"wake-off":true}`)); err != nil {
		t.Fatal(err)
	}
	waitUntil(t, "the newly enabled policy's first evaluation", func() bool { return off.count() == 1 })
	commitDomain(t, state, "units")
	waitUntil(t, "the newly enabled policy to follow a state event", func() bool { return off.count() == 2 })
}

// The disabled policy's pending flag is what a wake would set; a state event must leave it alone.
func TestStateEventDoesNotMarkADisabledPolicyPending(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openWakeConfiguration(t, "wake-on")
	on := &wakeProbePolicy{id: "wake-on", domains: []string{"units"}}
	off := &wakeProbePolicy{id: "wake-off", domains: []string{"units"}}
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}, on, off)
	runtime := map[string]*policyRuntime{"wake-on": {}, "wake-off": {}}
	coordinator.evaluate(t.Context(), runtime, make(chan operationResult, 2))
	if !runtime["wake-off"].enabledKnown || runtime["wake-off"].enabled || !runtime["wake-on"].enabled {
		t.Fatalf("enablement after the first evaluation: on %+v off %+v", runtime["wake-on"], runtime["wake-off"])
	}
	event := State.Event{Revision: state.Revision() + 1, Domains: []string{"units"}}
	woke, _ := wakePoliciesForStateEvent(runtime, coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, event)
	if !woke || !runtime["wake-on"].evaluationPending || runtime["wake-off"].evaluationPending {
		t.Fatalf("woke %t; on pending %t, off pending %t", woke, runtime["wake-on"].evaluationPending, runtime["wake-off"].evaluationPending)
	}
	// A session change wakes every enabled policy and still no disabled one.
	runtime["wake-on"].evaluationPending = false
	woke, _ = wakePoliciesForStateEvent(runtime, coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, State.Event{Revision: event.Revision + 1, Domains: []string{"session"}})
	if !woke || !runtime["wake-on"].evaluationPending || runtime["wake-off"].evaluationPending {
		t.Fatalf("session wake: on pending %t, off pending %t", runtime["wake-on"].evaluationPending, runtime["wake-off"].evaluationPending)
	}
}

// CIT-43 behaviour 2: a policy's own deadline fires on time and unrelated
// domain events neither evaluate it nor move it.
func TestPolicyDeadlineFiresOnTimeAfterUnrelatedDomainEvents(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openWakeConfiguration(t, "deadline")
	policy := &wakeProbePolicy{id: "deadline", domains: []string{"beri"}, nextCheck: 600 * time.Millisecond}
	runCoordinator(t, state, configuration, policy)
	waitUntil(t, "the initial evaluation", func() bool { return policy.count() == 1 })
	first := policy.evaluations()[0]

	stop := time.Now().Add(450 * time.Millisecond)
	for time.Now().Before(stop) {
		commitDomain(t, state, "units") // a domain the policy does not read
		time.Sleep(15 * time.Millisecond)
	}
	if got := policy.count(); got != 1 {
		t.Fatalf("unrelated events evaluated the policy %d times before its deadline", got)
	}
	waitUntil(t, "the deadline evaluation", func() bool { return policy.count() == 2 })
	elapsed := policy.evaluations()[1].Sub(first)
	if elapsed < 550*time.Millisecond || elapsed > 900*time.Millisecond {
		t.Fatalf("deadline evaluation came %s after the previous one, want about 600 ms", elapsed)
	}
}

// CIT-43 behaviour 3 (unit level): urgency counts only while the policy is enabled.
func TestUrgentDomainsAreUrgentOnlyWhileThePolicyIsEnabled(t *testing.T) {
	policy := &wakeProbePolicy{id: "beri", domains: []string{"movements"}, urgent: []string{"movements"}}
	coordinator := NewCoordinator(State.NewStore(coordinatorReadyState()), openWakeConfiguration(t, "beri"), nil, &coordinatorTestSubmitter{}, policy)
	event := State.Event{Revision: 5, Domains: []string{"movements"}}
	runtime := func(known bool, enabled bool) map[string]*policyRuntime {
		return map[string]*policyRuntime{"beri": {enabledKnown: known, enabled: enabled}}
	}
	if woke, urgent := wakePoliciesForStateEvent(runtime(true, true), coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, event); !woke || !urgent {
		t.Fatalf("enabled: woke %t urgent %t, want both", woke, urgent)
	}
	if woke, urgent := wakePoliciesForStateEvent(runtime(false, false), coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, event); !woke || !urgent {
		t.Fatalf("not yet evaluated: woke %t urgent %t, want both (unknown is treated as enabled)", woke, urgent)
	}
	if woke, urgent := wakePoliciesForStateEvent(runtime(true, false), coordinator.stateWakeByDomain, coordinator.urgentWakeByDomain, event); woke || urgent {
		t.Fatalf("disabled: woke %t urgent %t, want neither", woke, urgent)
	}
}

// CIT-43 behaviour 3 (timing): with the urgent policy off, its domains take the
// normal debounce for the policies that are on; with it on they are immediate.
func TestBeriStyleUrgentWakeSkipsTheDebounceOnlyWhileEnabled(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		enabled []string
		watcher string
		atMost  time.Duration
		atLeast time.Duration
	}{
		{name: "urgent policy enabled", enabled: []string{"beri", "watcher"}, atMost: stateChangeDebounce / 2},
		{name: "urgent policy disabled", enabled: []string{"watcher"}, atLeast: stateChangeDebounce - 30*time.Millisecond},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			state := State.NewStore(coordinatorReadyState())
			configuration := openWakeConfiguration(t, testCase.enabled...)
			beri := &wakeProbePolicy{id: "beri", domains: []string{"movements"}, urgent: []string{"movements"}}
			watcher := &wakeProbePolicy{id: "watcher", domains: []string{"movements"}}
			runCoordinator(t, state, configuration, beri, watcher)
			waitUntil(t, "the initial evaluations", func() bool { return watcher.count() == 1 && (beri.count() == 1) == (len(testCase.enabled) == 2) })
			time.Sleep(50 * time.Millisecond)

			committedAt := time.Now()
			commitDomain(t, state, "movements")
			waitUntil(t, "the watcher's evaluation", func() bool { return watcher.count() == 2 })
			latency := watcher.evaluations()[1].Sub(committedAt)
			if testCase.atMost > 0 && latency > testCase.atMost {
				t.Fatalf("urgent wake took %s, want under %s", latency, testCase.atMost)
			}
			if testCase.atLeast > 0 && latency < testCase.atLeast {
				t.Fatalf("with the urgent policy disabled the wake took %s, want the normal debounce of %s", latency, stateChangeDebounce)
			}
			if beri.count() > 1 && len(testCase.enabled) == 1 {
				t.Fatalf("the disabled urgent policy was evaluated %d times", beri.count())
			}
		})
	}
}

// Core and on-demand policies are never treated as disabled for waking.
func TestCoreAndOnDemandPoliciesAreNotGatedByEnablement(t *testing.T) {
	if policyGatedByEnablement(&coordinatorTestPolicy{id: "plain"}) != true {
		t.Fatal("a switch-controlled policy must be gated")
	}
	if policyGatedByEnablement(NewAllianceHelpPolicy()) {
		t.Fatal("a core policy was treated as switch-controlled")
	}
}

// CIT-43: the enabled controls and scheduler are parsed once per configuration
// revision, not per evaluation, and evaluation no longer copies the configuration.
func TestConfigurationIsParsedOncePerRevisionAndNotCopiedPerEvaluation(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"automation.enabled": json.RawMessage(`{"a":true,"b":false}`),
		"scheduler":          json.RawMessage(`{"featureSchedules":{"a":{"enabled":true,"timeZone":"UTC","slots":[{"day":0,"startMinute":0,"endMinute":1440},{"day":1,"startMinute":0,"endMinute":1440},{"day":2,"startMinute":0,"endMinute":1440},{"day":3,"startMinute":0,"endMinute":1440},{"day":4,"startMinute":0,"endMinute":1440},{"day":5,"startMinute":0,"endMinute":1440},{"day":6,"startMinute":0,"endMinute":1440}]}}}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	a := &wakeProbePolicy{id: "a", domains: []string{"units"}}
	b := &wakeProbePolicy{id: "b", domains: []string{"units"}}
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{calls: make(chan Intent.Request, 1)}, a, b)
	runtime := map[string]*policyRuntime{"a": {}, "b": {}}
	results := make(chan operationResult, 4)

	coordinator.evaluate(t.Context(), runtime, results) // warms every cache
	before := configurationParses.Load()
	for index := 0; index < 50; index++ {
		for _, current := range runtime {
			current.evaluationPending = true
		}
		coordinator.evaluate(t.Context(), runtime, results)
	}
	if delta := configurationParses.Load() - before; delta != 0 {
		t.Fatalf("50 evaluations at one revision parsed configuration %d times, want 0", delta)
	}
	if a.count() < 40 {
		t.Fatalf("policy a evaluated %d times; the loop did not exercise evaluation", a.count())
	}

	// A change to another section is a new revision but does not reparse these two sections.
	if _, err := configuration.Update("scheduler", json.RawMessage(`{"featureSchedules":{}}`)); err != nil {
		t.Fatal(err)
	}
	before = configurationParses.Load()
	coordinator.evaluate(t.Context(), runtime, results)
	coordinator.evaluate(t.Context(), runtime, results)
	if delta := configurationParses.Load() - before; delta != 1 {
		t.Fatalf("after a scheduler change the parses = %d, want exactly 1 (the scheduler; enabled controls are unchanged)", delta)
	}
	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{"a":true,"b":true}`)); err != nil {
		t.Fatal(err)
	}
	before = configurationParses.Load()
	for _, current := range runtime {
		current.evaluationPending = true
	}
	coordinator.evaluate(t.Context(), runtime, results)
	coordinator.evaluate(t.Context(), runtime, results)
	if delta := configurationParses.Load() - before; delta != 1 {
		t.Fatalf("after an enabled-controls change the parses = %d, want exactly 1", delta)
	}

	first, second := configuration.SharedSnapshot(), configuration.SharedSnapshot()
	if reflect.ValueOf(first.Sections).Pointer() != reflect.ValueOf(second.Sections).Pointer() {
		t.Fatal("SharedSnapshot copied the configuration")
	}
	private := configuration.Snapshot()
	if reflect.ValueOf(private.Sections).Pointer() == reflect.ValueOf(first.Sections).Pointer() {
		t.Fatal("Snapshot must still return a private copy")
	}
}

func TestPolicyFingerprintsAreCachedPerSnapshotAndRefreshedOnChange(t *testing.T) {
	state := State.NewStore(coordinatorReadyState())
	configuration := openWakeConfiguration(t, "a")
	policy := &wakeProbePolicy{id: "a", domains: []string{"units"}}
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	snapshot := configuration.SharedSnapshot()
	first := coordinator.policyFingerprint(policy, snapshot)
	if first != policyConfigurationFingerprint(policy, snapshot) {
		t.Fatal("the cached fingerprint differs from the computed one")
	}
	if _, err := configuration.Update("automation.enabled", json.RawMessage(`{"a":false}`)); err != nil {
		t.Fatal(err)
	}
	next := configuration.SharedSnapshot()
	if coordinator.policyFingerprint(policy, next) == first {
		t.Fatal("the fingerprint did not change with the enabled control")
	}
	// A private copy of the same revision is a different snapshot and is recomputed, never served stale.
	if got := coordinator.policyFingerprint(policy, configuration.Snapshot()); got != policyConfigurationFingerprint(policy, next) {
		t.Fatal("a copied snapshot got a stale fingerprint")
	}
}

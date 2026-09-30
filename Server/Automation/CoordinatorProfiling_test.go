package Automation

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"

	"CitadelDesktop/Server/Profiling"
	"CitadelDesktop/Server/State"
)

// blockingLabelPolicy blocks inside Evaluate so a goroutine profile can be taken
// while the policy is "on CPU", and reports the labels its context carries.
type blockingLabelPolicy struct {
	coordinatorTestPolicy
	entered chan struct{}
	release chan struct{}
	labels  map[string]string
}

func (policy *blockingLabelPolicy) Evaluate(ctx context.Context, _ Snapshot) (Decision, error) {
	policy.labels = map[string]string{}
	for _, key := range []string{Profiling.LabelRuntime, Profiling.LabelStage, Profiling.LabelPolicy} {
		if value, found := pprof.Label(ctx, key); found {
			policy.labels[key] = value
		}
	}
	close(policy.entered)
	<-policy.release
	return Decision{EventDriven: true}, nil
}

func TestEvaluateLabelsEachPolicyUnderItsRuntimeAndStage(t *testing.T) {
	listener, err := Profiling.Start(context.Background(), "127.0.0.1:0", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()

	policy := &blockingLabelPolicy{
		coordinatorTestPolicy: coordinatorTestPolicy{id: "autoStorm"},
		entered:               make(chan struct{}), release: make(chan struct{}),
	}
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, policy.ID())
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	ctx := Profiling.WithRuntime(context.Background(), "acct-9")
	finished := make(chan struct{})
	go Profiling.Do(ctx, func(labeled context.Context) {
		defer close(finished)
		coordinator.evaluate(labeled, map[string]*policyRuntime{policy.ID(): {}}, make(chan operationResult, 1))
	}, Profiling.LabelStage, Profiling.StageAutomation)

	<-policy.entered
	var dump bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&dump, 1); err != nil {
		t.Fatal(err)
	}
	close(policy.release)
	<-finished

	for key, want := range map[string]string{
		Profiling.LabelRuntime: "acct-9", Profiling.LabelStage: Profiling.StageAutomation, Profiling.LabelPolicy: "autoStorm",
	} {
		if policy.labels[key] != want {
			t.Errorf("Evaluate context label %s = %q, want %q", key, policy.labels[key], want)
		}
	}
	found := false
	for _, line := range strings.Split(dump.String(), "\n") {
		if strings.HasPrefix(line, "# labels:") && strings.Contains(line, `"policy":"autoStorm"`) &&
			strings.Contains(line, `"runtime":"acct-9"`) && strings.Contains(line, `"stage":"automation"`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("no goroutine carried runtime, stage and policy labels while the policy evaluated:\n%s", dump.String())
	}
}

func TestEvaluateAddsNoLabelsWithoutProfiling(t *testing.T) {
	policy := &blockingLabelPolicy{
		coordinatorTestPolicy: coordinatorTestPolicy{id: "autoStorm"},
		entered:               make(chan struct{}), release: make(chan struct{}),
	}
	close(policy.release)
	state := State.NewStore(coordinatorReadyState())
	configuration := openCoordinatorTestConfiguration(t, policy.ID())
	coordinator := NewCoordinator(state, configuration, nil, &coordinatorTestSubmitter{}, policy)
	coordinator.evaluate(t.Context(), map[string]*policyRuntime{policy.ID(): {}}, make(chan operationResult, 1))
	if len(policy.labels) != 0 {
		t.Fatalf("labels applied with profiling off: %v", policy.labels)
	}
}

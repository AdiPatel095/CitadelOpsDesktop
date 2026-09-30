package Accounts

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"

	"CitadelDesktop/Server/Profiling"
)

// goroutineLabelLines returns the label sets of every goroutine that has one
// (the debug=1 goroutine profile prints `# labels: {...}` under each stack group).
func goroutineLabelLines(t *testing.T) []string {
	t.Helper()
	var dump bytes.Buffer
	if err := pprof.Lookup("goroutine").WriteTo(&dump, 1); err != nil {
		t.Fatal(err)
	}
	var labels []string
	for _, line := range strings.Split(dump.String(), "\n") {
		if strings.HasPrefix(line, "# labels:") {
			labels = append(labels, line)
		}
	}
	return labels
}

func hasLabelSet(labels []string, want ...string) bool {
	for _, line := range labels {
		matched := true
		for _, fragment := range want {
			if !strings.Contains(line, fragment) {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func TestAddAccountLabelsRuntimeGoroutinesByRuntimeAndStage(t *testing.T) {
	listener, err := Profiling.Start(context.Background(), "127.0.0.1:0", t.Logf)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Stop()
	supervisor := newTestSupervisor(t)
	addTestAccount(t, supervisor, "alpha")
	addTestAccount(t, supervisor, "bravo")

	labels := goroutineLabelLines(t)
	for _, id := range []string{"alpha", "bravo"} {
		runtimeLabel := `"runtime":"` + id + `"`
		if !hasLabelSet(labels, runtimeLabel, `"stage":"automation"`) {
			t.Errorf("no automation goroutine labelled for %s in %v", id, labels)
		}
		if !hasLabelSet(labels, runtimeLabel, `"stage":"persist"`) {
			t.Errorf("no persistence goroutine labelled for %s in %v", id, labels)
		}
	}
	// Runtimes stay separate: no goroutine carries both runtime labels.
	if hasLabelSet(labels, `"runtime":"alpha"`, `"runtime":"bravo"`) {
		t.Fatal("a goroutine carries two runtime labels")
	}
}

func TestAddAccountAppliesNoLabelsWithoutProfiling(t *testing.T) {
	if Profiling.Enabled() {
		t.Fatal("profiling unexpectedly enabled by another test")
	}
	supervisor := newTestSupervisor(t)
	addTestAccount(t, supervisor, "alpha")
	for _, line := range goroutineLabelLines(t) {
		if strings.Contains(line, `"runtime"`) || strings.Contains(line, `"stage"`) {
			t.Fatalf("profiler label applied with profiling off: %s", line)
		}
	}
}

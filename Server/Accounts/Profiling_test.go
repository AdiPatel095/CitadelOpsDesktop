package Accounts

import (
	"bytes"
	"context"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

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

// awaitGoroutineLabels samples the goroutine labels until done reports true or
// the deadline passes, and returns the last sample. The caller asserts on what
// done left behind, so a genuine failure still names what is missing.
func awaitGoroutineLabels(t *testing.T, deadline time.Duration, done func([]string) bool) []string {
	t.Helper()
	limit := time.Now().Add(deadline)
	for {
		labels := goroutineLabelLines(t)
		if done(labels) || time.Now().After(limit) {
			return labels
		}
		time.Sleep(10 * time.Millisecond)
	}
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

	// AddAccount returns before every runtime goroutine has started and labelled
	// itself (the automation coordinator's goroutine is spawned asynchronously),
	// so wait for the labels with a deadline instead of sampling once.
	var missing []string
	labels := awaitGoroutineLabels(t, 10*time.Second, func(labels []string) bool {
		missing = missing[:0]
		for _, id := range []string{"alpha", "bravo"} {
			runtimeLabel := `"runtime":"` + id + `"`
			if !hasLabelSet(labels, runtimeLabel, `"stage":"automation"`) {
				missing = append(missing, "automation goroutine for "+id)
			}
			if !hasLabelSet(labels, runtimeLabel, `"stage":"persist"`) {
				missing = append(missing, "persistence goroutine for "+id)
			}
		}
		return len(missing) == 0
	})
	for _, absent := range missing {
		t.Errorf("no labelled %s within the deadline; labels: %v", absent, labels)
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

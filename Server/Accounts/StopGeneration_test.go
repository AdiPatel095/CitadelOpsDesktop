package Accounts

import (
	"CitadelDesktop/Server/App"
	"CitadelDesktop/Server/State"
	"testing"
	"time"
)

func TestDelayedStopCleanupCannotClaimReplacementGeneration(t *testing.T) {
	old := accountRuntime{application: &App.Application{DataDir: "old-profile"}}
	replacement := accountRuntime{application: &App.Application{DataDir: "replacement-profile"}}
	supervisor := &Supervisor{stopping: map[AccountID]accountRuntime{"alpha": old}, dataDirs: map[string]AccountID{"old-profile": "alpha", "replacement-profile": "alpha"}}
	// The old RemoveAccount's wait timed out before claiming its observer. A
	// different caller finished old teardown, reopened alpha, and stopped it.
	supervisor.mu.Lock()
	delete(supervisor.stopping, "alpha")
	supervisor.stopping["alpha"] = replacement
	supervisor.mu.Unlock()
	if supervisor.claimStopWatcher("alpha", old) {
		t.Fatal("stale cleanup claimed replacement watcher")
	}
	if !supervisor.claimStopWatcher("alpha", replacement) {
		t.Fatal("replacement cleanup has no observer")
	}
	if supervisor.claimStopWatcher("alpha", replacement) {
		t.Fatal("duplicate generation watcher admitted")
	}
}

func TestStaleStopReleasePreservesReplacementScanner(t *testing.T) {
	old := accountRuntime{application: &App.Application{DataDir: "old-profile"}}
	replacement := accountRuntime{application: &App.Application{DataDir: "replacement-profile"}}
	worlds := State.NewWorldMapStore()
	supervisor := &Supervisor{stopping: map[AccountID]accountRuntime{"alpha": replacement}, dataDirs: map[string]AccountID{"replacement-profile": "alpha"}, worldMaps: worlds}
	now := time.Now()
	worlds.AcquireStormScan("alpha", "world-one", 4, now)
	supervisor.releaseStoppedAccount("alpha", old)
	if assignment := worlds.AcquireStormScan("bravo", "world-one", 4, now.Add(time.Millisecond)); assignment.ParticipantCount != 2 {
		t.Fatal("stale cleanup unregistered replacement scanner")
	}
	if supervisor.stopping["alpha"].application != replacement.application || supervisor.dataDirs[replacement.application.DataDir] != "alpha" {
		t.Fatal("stale cleanup released replacement ownership")
	}
	supervisor.releaseStoppedAccount("alpha", replacement)
	if _, exists := supervisor.stopping["alpha"]; exists {
		t.Fatal("current generation was not released")
	}
	if assignment := worlds.AcquireStormScan("bravo", "world-one", 4, now.Add(2*time.Millisecond)); assignment.ParticipantCount != 1 {
		t.Fatal("current scanner was not unregistered")
	}
}

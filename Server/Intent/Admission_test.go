package Intent

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
)

type acquiredAdmission struct {
	name    string
	release func()
	err     error
}

func TestAdmissionManagerUsesUserWeightsWithinAutomationTier(t *testing.T) {
	manager := newAdmissionManager(nil)
	holder, err := manager.acquire(t.Context(), admissionRequest{
		operationID: "holder", actor: "automation:holder", priority: Outbound.PriorityBackground,
		weight: 50, admission: Admission{Class: AdmissionAttackLaunch, Module: "holder"},
	})
	if err != nil {
		t.Fatal(err)
	}
	results := make(chan acquiredAdmission, 2)
	go acquireAdmissionForTest(manager, "low", admissionRequest{
		operationID: "low", actor: "automation:low", priority: Outbound.PriorityBackground,
		weight: 10, admission: Admission{Class: AdmissionAttackLaunch, Module: "low"},
	}, results)
	waitForAdmissionWaiters(t, manager, 1)
	go acquireAdmissionForTest(manager, "high", admissionRequest{
		operationID: "high", actor: "automation:high", priority: Outbound.PriorityBackground,
		weight: 90, admission: Admission{Class: AdmissionAttackLaunch, Module: "high"},
	}, results)
	waitForAdmissionWaiters(t, manager, 2)
	holder()
	first := <-results
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.name != "high" {
		t.Fatalf("first admission = %q", first.name)
	}
	first.release()
	second := <-results
	if second.err != nil {
		t.Fatal(second.err)
	}
	if second.name != "low" {
		t.Fatalf("second admission = %q", second.name)
	}
	second.release()
}

func TestAdmissionManagerProtectsScheduledAndInteractiveTiers(t *testing.T) {
	now := time.Now().UTC()
	automation := &admissionWaiter{id: 1, request: admissionRequest{
		actor: "automation:autoTowers", priority: Outbound.PriorityBackground, weight: 100, submittedAt: now,
		admission: Admission{Class: AdmissionAttackLaunch, Module: "autoTowers"},
	}}
	scheduled := &admissionWaiter{id: 2, request: admissionRequest{
		actor: "scheduler:rift", priority: Outbound.PriorityScheduled, weight: 1, submittedAt: now,
		admission: Admission{Class: AdmissionAttackLaunch, Module: "riftReplay"},
	}}
	interactive := &admissionWaiter{id: 3, request: admissionRequest{
		actor: "ui", priority: Outbound.PriorityInteractive, weight: 1, submittedAt: now,
		admission: Admission{Class: AdmissionAttackLaunch, Module: "riftReplay"},
	}}
	if !admissionWaiterBefore(scheduled, automation, now) {
		t.Fatal("scheduled attack did not outrank automation")
	}
	if !admissionWaiterBefore(interactive, scheduled, now) {
		t.Fatal("interactive attack did not outrank scheduled work")
	}
}

func TestAdmissionManagerAgingEventuallyPreventsAutomationStarvation(t *testing.T) {
	now := time.Now().UTC()
	oldLowPriority := &admissionWaiter{id: 1, request: admissionRequest{
		actor: "automation:autoTowers", priority: Outbound.PriorityBackground, weight: 10,
		submittedAt: now.Add(-30 * time.Minute), admission: Admission{Class: AdmissionAttackLaunch, Module: "autoTowers"},
	}}
	newHighPriority := &admissionWaiter{id: 2, request: admissionRequest{
		actor: "automation:future", priority: Outbound.PriorityBackground, weight: 90,
		submittedAt: now, admission: Admission{Class: AdmissionAttackLaunch, Module: "future"},
	}}
	if !admissionWaiterBefore(oldLowPriority, newHighPriority, now) {
		t.Fatal("aged automation work did not eventually outrank a new high-weight proposal")
	}
}

func TestAdmissionManagerWaitsForActualLaneAvailability(t *testing.T) {
	availableAt := time.Now().UTC().Add(60 * time.Millisecond)
	manager := newAdmissionManager(func(class AdmissionClass) time.Time {
		if class == AdmissionAttackLaunch {
			return availableAt
		}
		return time.Time{}
	})
	started := time.Now()
	release, err := manager.acquire(t.Context(), admissionRequest{
		operationID: "paced", actor: "automation:autoTowers", priority: Outbound.PriorityBackground,
		admission: Admission{Class: AdmissionAttackLaunch, Module: "autoTowers"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	if elapsed := time.Since(started); elapsed < 45*time.Millisecond {
		t.Fatalf("admission granted before lane availability: %s", elapsed)
	}
}

func TestAdmissionManagerRemovesCancelledWaiter(t *testing.T) {
	manager := newAdmissionManager(nil)
	holder, err := manager.acquire(t.Context(), admissionRequest{
		operationID: "holder", actor: "ui", priority: Outbound.PriorityInteractive,
		admission: Admission{Class: AdmissionAttackLaunch, Module: "manual"},
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	result := make(chan error, 1)
	go func() {
		_, err := manager.acquire(ctx, admissionRequest{
			operationID: "cancel", actor: "automation:autoTowers", priority: Outbound.PriorityBackground,
			admission: Admission{Class: AdmissionAttackLaunch, Module: "autoTowers"},
		})
		result <- err
	}()
	waitForAdmissionWaiters(t, manager, 1)
	cancel()
	if err := <-result; err != context.Canceled {
		t.Fatalf("cancelled admission error = %v", err)
	}
	holder()
}

func acquireAdmissionForTest(manager *admissionManager, name string, request admissionRequest, result chan<- acquiredAdmission) {
	release, err := manager.acquire(context.Background(), request)
	result <- acquiredAdmission{name: name, release: release, err: err}
}

func waitForAdmissionWaiters(t *testing.T, manager *admissionManager, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		manager.mu.Lock()
		actual := len(manager.waiters)
		manager.mu.Unlock()
		if actual >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("admission waiters did not reach %d", count)
}

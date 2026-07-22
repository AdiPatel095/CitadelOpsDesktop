package Intent

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

func TestLegacyClaimsUseHierarchicalResources(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 42
	gameState.Castles[1] = State.CastleState{ID: 1, KingdomID: 9}
	gameState.Castles[2] = State.CastleState{ID: 2, KingdomID: 9}
	tests := []struct {
		name     string
		left     []string
		right    []string
		overlaps bool
	}{
		{name: "castle parent and building child", left: []string{"castle:1"}, right: []string{"castle:1", "building:7"}, overlaps: true},
		{name: "same building id in different castles", left: []string{"castle:1", "building:7"}, right: []string{"castle:2", "building:7"}, overlaps: false},
		{name: "wallet parent and currency", left: []string{"account-resources"}, right: []string{"currency:1"}, overlaps: true},
		{name: "commander aliases", left: []string{"commander:7"}, right: []string{"leader:commander:7"}, overlaps: true},
		{name: "shop parent and offer", left: []string{"shop"}, right: []string{"shop:offer"}, overlaps: true},
		{name: "different responses", left: []string{"response:ain"}, right: []string{"response:rpc"}, overlaps: false},
		{name: "game UI and attack context", left: []string{"game-ui"}, right: []string{"attack-context"}, overlaps: true},
		{name: "global crafting and castle crafting", left: []string{"game:crafting"}, right: []string{"castle:1", "crafting-building:7"}, overlaps: true},
		{name: "target aliases at same coordinate", left: []string{"tower-target:9:12:13"}, right: []string{"spy-target:9:12:13"}, overlaps: true},
		{name: "same unit in different castles remains independent", left: []string{"castle:1", "unit:5"}, right: []string{"castle:2", "unit:5"}, overlaps: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			left := legacyClaimsToResources(gameState, test.left)
			right := legacyClaimsToResources(gameState, test.right)
			if actual := resourcesOverlap(left, right); actual != test.overlaps {
				t.Fatalf("resourcesOverlap() = %t, want %t\nleft: %#v\nright: %#v", actual, test.overlaps, left, right)
			}
		})
	}
}

func TestAdvisorClaimsDeclareTypedResources(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Session.ServerURL = "https://world.example"
	gameState.Player.ID = 42
	resources := legacyClaimsToResources(gameState, []string{
		"advisor:activation",
		"advisor:overview",
		"advisor:event:72",
		"event:72",
	})
	if hasLegacyResource(resources) {
		t.Fatalf("advisor claims resolved to legacy resources: %#v", resources)
	}
}

type acquiredClaim struct {
	name    string
	release func()
	waited  bool
	err     error
}

func TestClaimManagerPrioritizesConflictingWaiters(t *testing.T) {
	manager := newClaimManager()
	holder, waited, err := manager.acquire(context.Background(), []string{"castle:1"}, Outbound.PriorityBackground)
	if err != nil {
		t.Fatal(err)
	}
	if waited {
		t.Fatal("uncontended claim was reported as waited")
	}
	acquired := make(chan acquiredClaim, 2)
	go acquireClaimForTest(manager, "low", Outbound.PriorityBackground, []string{"castle:1"}, acquired)
	waitForClaimWaiters(t, manager, 1)
	go acquireClaimForTest(manager, "high", Outbound.PriorityInteractive, []string{"castle:1"}, acquired)
	waitForClaimWaiters(t, manager, 2)
	holder()
	first := <-acquired
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.name != "high" {
		t.Fatalf("first claim owner = %q", first.name)
	}
	if !first.waited {
		t.Fatal("conflicting high-priority claim was not reported as waited")
	}
	first.release()
	second := <-acquired
	if second.err != nil {
		t.Fatal(second.err)
	}
	if second.name != "low" {
		t.Fatalf("second claim owner = %q", second.name)
	}
	if !second.waited {
		t.Fatal("conflicting low-priority claim was not reported as waited")
	}
	second.release()
}

func TestClaimOrderingAgesOldConflictingWorkToPreventStarvation(t *testing.T) {
	now := time.Now().UTC()
	oldBackground := &claimWaiter{
		id: 1, priority: Outbound.PriorityBackground, enqueuedAt: now.Add(-2 * time.Minute),
	}
	newInteractive := &claimWaiter{
		id: 2, priority: Outbound.PriorityInteractive, enqueuedAt: now,
	}
	if !claimWaiterBeforeAt(oldBackground, newInteractive, now) {
		t.Fatal("aged background claim did not eventually win the FIFO tie")
	}
}

func TestClaimManagerRunsNonConflictingWorkWhilePriorityWaits(t *testing.T) {
	manager := newClaimManager()
	holder, waited, err := manager.acquire(context.Background(), []string{"castle:1"}, Outbound.PriorityBackground)
	if err != nil {
		t.Fatal(err)
	}
	if waited {
		t.Fatal("uncontended claim was reported as waited")
	}
	acquired := make(chan acquiredClaim, 3)
	go acquireClaimForTest(manager, "high", Outbound.PriorityInteractive, []string{"castle:1", "commander:1"}, acquired)
	waitForClaimWaiters(t, manager, 1)
	go acquireClaimForTest(manager, "blocked-low", Outbound.PriorityBackground, []string{"commander:1"}, acquired)
	waitForClaimWaiters(t, manager, 2)
	go acquireClaimForTest(manager, "independent", Outbound.PriorityBackground, []string{"castle:2"}, acquired)
	first := <-acquired
	if first.err != nil {
		t.Fatal(first.err)
	}
	if first.name != "independent" {
		t.Fatalf("first claim owner = %q", first.name)
	}
	if first.waited {
		t.Fatal("independent claim was reported as waited")
	}
	first.release()
	holder()
	second := <-acquired
	if second.err != nil {
		t.Fatal(second.err)
	}
	if second.name != "high" {
		t.Fatalf("second claim owner = %q", second.name)
	}
	if !second.waited {
		t.Fatal("conflicting high-priority claim was not reported as waited")
	}
	second.release()
	third := <-acquired
	if third.err != nil {
		t.Fatal(third.err)
	}
	if third.name != "blocked-low" {
		t.Fatalf("third claim owner = %q", third.name)
	}
	if !third.waited {
		t.Fatal("reserved low-priority claim was not reported as waited")
	}
	third.release()
}

func TestClaimManagerRemovesCancelledWaiter(t *testing.T) {
	manager := newClaimManager()
	holder, waited, err := manager.acquire(context.Background(), []string{"castle:1"}, Outbound.PriorityBackground)
	if err != nil {
		t.Fatal(err)
	}
	if waited {
		t.Fatal("uncontended claim was reported as waited")
	}
	cancelledContext, cancel := context.WithCancel(context.Background())
	cancelled := make(chan error, 1)
	go func() {
		_, _, err := manager.acquire(cancelledContext, []string{"castle:1"}, Outbound.PriorityInteractive)
		cancelled <- err
	}()
	waitForClaimWaiters(t, manager, 1)
	acquired := make(chan acquiredClaim, 1)
	go acquireClaimForTest(manager, "low", Outbound.PriorityBackground, []string{"castle:1"}, acquired)
	waitForClaimWaiters(t, manager, 2)
	cancel()
	if err := <-cancelled; err != context.Canceled {
		t.Fatalf("cancelled claim error = %v", err)
	}
	holder()
	next := <-acquired
	if next.err != nil {
		t.Fatal(next.err)
	}
	if next.name != "low" {
		t.Fatalf("next claim owner = %q", next.name)
	}
	if !next.waited {
		t.Fatal("conflicting claim was not reported as waited")
	}
	next.release()
}

func acquireClaimForTest(
	manager *claimManager,
	name string,
	priority Outbound.Priority,
	claims []string,
	result chan<- acquiredClaim,
) {
	release, waited, err := manager.acquire(context.Background(), claims, priority)
	result <- acquiredClaim{name: name, release: release, waited: waited, err: err}
}

func waitForClaimWaiters(t *testing.T, manager *claimManager, count int) {
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
	t.Fatalf("claim waiters did not reach %d", count)
}

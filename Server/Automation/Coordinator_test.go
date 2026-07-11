package Automation

import (
	"context"
	"testing"
	"time"
)

func TestCoordinatorAllowsNonConflictingAndSharedClaims(t *testing.T) {
	c := NewCoordinator()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	first, ok := c.Acquire(ctx, Request{
		Owner:  "first",
		Claims: []Claim{ExclusiveClaim("castle:1:queue")},
	})
	if !ok {
		t.Fatal("first non-conflicting lease was not granted")
	}
	defer first.Release()

	second, ok := c.Acquire(ctx, Request{
		Owner:  "second",
		Claims: []Claim{ExclusiveClaim("castle:2:queue")},
	})
	if !ok {
		t.Fatal("second non-conflicting lease was not granted")
	}
	defer second.Release()

	sharedOne, ok := c.Acquire(ctx, Request{Owner: "reader-one", Claims: []Claim{SharedClaim("state:castles")}})
	if !ok {
		t.Fatal("first shared lease was not granted")
	}
	defer sharedOne.Release()
	sharedTwo, ok := c.Acquire(ctx, Request{Owner: "reader-two", Claims: []Claim{SharedClaim("state:castles")}})
	if !ok {
		t.Fatal("second shared lease was not granted")
	}
	defer sharedTwo.Release()
}

func TestCoordinatorQueuesConflictsByPriority(t *testing.T) {
	c := NewCoordinator()
	blocker, ok := c.Acquire(context.Background(), Request{
		Owner:  "blocker",
		Claims: []Claim{ExclusiveClaim("account:equipment")},
	})
	if !ok {
		t.Fatal("blocker was not granted")
	}

	type result struct {
		owner string
		lease *Lease
		ok    bool
	}
	results := make(chan result, 2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	go func() {
		lease, acquired := c.Acquire(ctx, Request{
			Owner:    "low",
			Priority: PriorityBackground,
			Claims:   []Claim{ExclusiveClaim("account:equipment")},
		})
		results <- result{owner: "low", lease: lease, ok: acquired}
	}()
	go func() {
		lease, acquired := c.Acquire(ctx, Request{
			Owner:    "high",
			Priority: PriorityManual,
			Claims:   []Claim{ExclusiveClaim("account:equipment")},
		})
		results <- result{owner: "high", lease: lease, ok: acquired}
	}()

	waitForCoordinatorQueue(t, c, 2)
	blocker.Release()
	first := <-results
	if !first.ok || first.owner != "high" {
		t.Fatalf("first granted waiter = %q, ok=%v; want high", first.owner, first.ok)
	}
	first.lease.Release()
	second := <-results
	if !second.ok || second.owner != "low" {
		t.Fatalf("second granted waiter = %q, ok=%v; want low", second.owner, second.ok)
	}
	second.lease.Release()
}

func TestCoordinatorPreemptsLowerPriorityLease(t *testing.T) {
	c := NewCoordinator()
	low, ok := c.Acquire(context.Background(), Request{
		Owner:    "low",
		Priority: PriorityBackground,
		Claims:   []Claim{ExclusiveClaim(ClaimGameFocus)},
	})
	if !ok {
		t.Fatal("low-priority lease was not granted")
	}

	high, ok := c.Acquire(context.Background(), Request{
		Owner:        "high",
		Priority:     PriorityManual,
		Claims:       []Claim{ExclusiveClaim(ClaimGameFocus)},
		PreemptLower: true,
	})
	if !ok {
		t.Fatal("high-priority preempting lease was not granted")
	}
	defer high.Release()
	if !low.Revoked() || low.Active() {
		t.Fatal("preempted lease remained active")
	}
}

func TestCoordinatorContextCancellationUnblocksWaiter(t *testing.T) {
	c := NewCoordinator()
	ownerCtx, cancelOwner := context.WithCancel(context.Background())
	owner, ok := c.Acquire(ownerCtx, Request{Owner: "owner", Claims: []Claim{ExclusiveClaim(ClaimTransport)}})
	if !ok {
		t.Fatal("owner lease was not granted")
	}

	waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
	defer cancelWait()
	result := make(chan *Lease, 1)
	go func() {
		lease, acquired := c.Acquire(waitCtx, Request{Owner: "waiter", Claims: []Claim{ExclusiveClaim(ClaimTransport)}})
		if acquired {
			result <- lease
			return
		}
		result <- nil
	}()
	waitForCoordinatorQueue(t, c, 1)
	cancelOwner()

	select {
	case lease := <-result:
		if lease == nil {
			t.Fatal("waiter was not granted after owner context cancellation")
		}
		lease.Release()
	case <-time.After(time.Second):
		t.Fatal("waiter stayed blocked after owner context cancellation")
	}
	owner.Release()
}

func TestCoordinatorRejectsExpiredQueuedRequest(t *testing.T) {
	c := NewCoordinator()
	blocker, ok := c.Acquire(context.Background(), Request{Owner: "blocker", Claims: []Claim{ExclusiveClaim(ClaimEquipment)}})
	if !ok {
		t.Fatal("blocker was not granted")
	}
	defer blocker.Release()

	started := time.Now()
	lease, ok := c.Acquire(context.Background(), Request{
		Owner:    "deadline",
		Claims:   []Claim{ExclusiveClaim(ClaimEquipment)},
		Deadline: time.Now().Add(30 * time.Millisecond),
	})
	if ok || lease != nil {
		t.Fatal("request was granted after its queue deadline")
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("expired request was not removed promptly")
	}
}

func waitForCoordinatorQueue(t *testing.T, c *Coordinator, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if len(c.Snapshot().Queued) >= count {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("coordinator queue did not reach %d waiters", count)
}

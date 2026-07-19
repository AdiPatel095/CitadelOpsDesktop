package Intent

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type claimWaiter struct {
	id         uint64
	resources  []ResourceKey
	priority   Outbound.Priority
	enqueuedAt time.Time
	deadline   time.Time
	ctx        context.Context
	ready      chan struct{}
	granted    bool
}

type claimManager struct {
	mu      sync.Mutex
	nextID  uint64
	held    map[uint64][]ResourceKey
	waiters []*claimWaiter
}

func newClaimManager() *claimManager {
	return &claimManager{held: map[uint64][]ResourceKey{}}
}

func (manager *claimManager) acquire(
	ctx context.Context,
	requested []string,
	priority Outbound.Priority,
) (func(), bool, error) {
	return manager.acquireResources(ctx, legacyClaimsToResources(State.GameState{}, requested), priority)
}

func (manager *claimManager) acquireResources(
	ctx context.Context,
	requested []ResourceKey,
	priority Outbound.Priority,
) (func(), bool, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	resources := normalizeResourceKeys(requested)
	if len(resources) == 0 {
		return func() {}, false, nil
	}
	deadline, _ := ctx.Deadline()
	waiter := &claimWaiter{
		resources: resources, priority: priority, enqueuedAt: time.Now().UTC(), deadline: deadline,
		ctx: ctx, ready: make(chan struct{}),
	}
	manager.mu.Lock()
	manager.nextID++
	waiter.id = manager.nextID
	manager.waiters = append(manager.waiters, waiter)
	manager.dispatchLocked()
	waited := !waiter.granted
	manager.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			manager.mu.Lock()
			delete(manager.held, waiter.id)
			manager.dispatchLocked()
			manager.mu.Unlock()
		})
	}

	select {
	case <-waiter.ready:
		if err := ctx.Err(); err != nil {
			release()
			return nil, waited, err
		}
		return release, waited, nil
	case <-ctx.Done():
		manager.mu.Lock()
		granted := waiter.granted
		if !granted {
			manager.removeWaiterLocked(waiter.id)
			manager.dispatchLocked()
		}
		manager.mu.Unlock()
		if granted {
			release()
		}
		return nil, waited, ctx.Err()
	}
}

func (manager *claimManager) dispatchLocked() {
	now := time.Now().UTC()
	sort.SliceStable(manager.waiters, func(left, right int) bool {
		return claimWaiterBeforeAt(manager.waiters[left], manager.waiters[right], now)
	})
	reserved := make([][]ResourceKey, 0, len(manager.waiters))
	remaining := make([]*claimWaiter, 0, len(manager.waiters))
	for _, waiter := range manager.waiters {
		if waiter.ctx.Err() != nil {
			continue
		}
		if !manager.claimsAvailableLocked(waiter.resources, reserved) {
			reserved = append(reserved, waiter.resources)
			remaining = append(remaining, waiter)
			continue
		}
		waiter.granted = true
		manager.held[waiter.id] = waiter.resources
		close(waiter.ready)
	}
	manager.waiters = remaining
}

func (manager *claimManager) claimsAvailableLocked(resources []ResourceKey, reserved [][]ResourceKey) bool {
	for _, held := range manager.held {
		if resourcesOverlap(resources, held) {
			return false
		}
	}
	for _, earlier := range reserved {
		if resourcesOverlap(resources, earlier) {
			return false
		}
	}
	return true
}

func (manager *claimManager) removeWaiterLocked(id uint64) {
	for index, waiter := range manager.waiters {
		if waiter.id == id {
			manager.waiters = append(manager.waiters[:index], manager.waiters[index+1:]...)
			return
		}
	}
}

func claimWaiterBefore(left, right *claimWaiter) bool {
	return claimWaiterBeforeAt(left, right, time.Now().UTC())
}

func claimWaiterBeforeAt(left, right *claimWaiter, now time.Time) bool {
	leftPriority := Outbound.AgedPriority(left.priority, left.enqueuedAt, now)
	rightPriority := Outbound.AgedPriority(right.priority, right.enqueuedAt, now)
	if leftPriority != rightPriority {
		return leftPriority > rightPriority
	}
	if !left.deadline.IsZero() || !right.deadline.IsZero() {
		if left.deadline.IsZero() {
			return false
		}
		if right.deadline.IsZero() {
			return true
		}
		if !left.deadline.Equal(right.deadline) {
			return left.deadline.Before(right.deadline)
		}
	}
	return left.id < right.id
}

func normalizeClaims(claims []string) []string {
	set := map[string]struct{}{}
	for _, claim := range claims {
		claim = strings.TrimSpace(claim)
		if claim != "" {
			set[claim] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for claim := range set {
		out = append(out, claim)
	}
	sort.Strings(out)
	return out
}

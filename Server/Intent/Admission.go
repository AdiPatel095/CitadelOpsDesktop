package Intent

import (
	"context"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Outbound"
)

const (
	defaultAdmissionWeight = 50
	admissionAgingInterval = 15 * time.Second
	admissionClaimTimeout  = 5 * time.Second
)

type admissionAvailability func(AdmissionClass) time.Time

type admissionRequest struct {
	operationID string
	actor       string
	priority    Outbound.Priority
	weight      int
	submittedAt time.Time
	admission   Admission
}

type admissionWaiter struct {
	id      uint64
	request admissionRequest
	ctx     context.Context
	ready   chan struct{}
	notify  chan struct{}
	granted bool
}

type admissionManager struct {
	mu           sync.Mutex
	nextID       uint64
	held         map[AdmissionClass]uint64
	waiters      []*admissionWaiter
	availability admissionAvailability
}

func newAdmissionManager(availability admissionAvailability) *admissionManager {
	return &admissionManager{
		held: map[AdmissionClass]uint64{}, availability: availability,
	}
}

func (manager *admissionManager) acquire(ctx context.Context, request admissionRequest) (func(), error) {
	if ctx == nil {
		ctx = context.Background()
	}
	request.admission.Class = AdmissionClass(strings.TrimSpace(string(request.admission.Class)))
	request.admission.Module = strings.TrimSpace(request.admission.Module)
	request.admission.Affinity = strings.TrimSpace(request.admission.Affinity)
	request.weight = normalizeAdmissionWeight(request.weight)
	if request.submittedAt.IsZero() {
		request.submittedAt = time.Now().UTC()
	}
	if request.admission.Class == "" {
		return func() {}, nil
	}
	waiter := &admissionWaiter{request: request, ctx: ctx, ready: make(chan struct{}), notify: make(chan struct{}, 1)}
	manager.mu.Lock()
	manager.nextID++
	waiter.id = manager.nextID
	manager.waiters = append(manager.waiters, waiter)
	manager.dispatchLocked(time.Now().UTC())
	manager.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			manager.mu.Lock()
			if manager.held[request.admission.Class] == waiter.id {
				delete(manager.held, request.admission.Class)
			}
			manager.removeWaiterLocked(waiter.id)
			manager.dispatchLocked(time.Now().UTC())
			manager.notifyWaitersLocked()
			manager.mu.Unlock()
		})
	}

	for {
		manager.mu.Lock()
		wakeAt := manager.nextWakeLocked(waiter, time.Now().UTC())
		manager.mu.Unlock()
		var timer *time.Timer
		var timerChannel <-chan time.Time
		if !wakeAt.IsZero() {
			wait := time.Until(wakeAt)
			if wait < 0 {
				wait = 0
			}
			timer = time.NewTimer(wait)
			timerChannel = timer.C
		}
		select {
		case <-waiter.ready:
			if timer != nil {
				timer.Stop()
			}
			if err := ctx.Err(); err != nil {
				release()
				return nil, err
			}
			return release, nil
		case <-ctx.Done():
			if timer != nil {
				timer.Stop()
			}
			release()
			return nil, ctx.Err()
		case <-waiter.notify:
			if timer != nil {
				timer.Stop()
			}
		case <-timerChannel:
			manager.mu.Lock()
			manager.dispatchLocked(time.Now().UTC())
			manager.mu.Unlock()
		}
	}
}

func (manager *admissionManager) notifyWaitersLocked() {
	for _, waiter := range manager.waiters {
		select {
		case waiter.notify <- struct{}{}:
		default:
		}
	}
}

func (manager *admissionManager) dispatchLocked(now time.Time) {
	manager.removeCancelledLocked()
	classes := map[AdmissionClass]struct{}{}
	for _, waiter := range manager.waiters {
		classes[waiter.request.admission.Class] = struct{}{}
	}
	for class := range classes {
		if manager.held[class] != 0 || manager.classAvailableAtLocked(class).After(now) {
			continue
		}
		selected := -1
		for index, waiter := range manager.waiters {
			if waiter.request.admission.Class != class || waiter.request.admission.NotBefore.After(now) {
				continue
			}
			if selected < 0 || admissionWaiterBefore(waiter, manager.waiters[selected], now) {
				selected = index
			}
		}
		if selected < 0 {
			continue
		}
		waiter := manager.waiters[selected]
		manager.waiters = append(manager.waiters[:selected], manager.waiters[selected+1:]...)
		waiter.granted = true
		manager.held[class] = waiter.id
		close(waiter.ready)
	}
}

func (manager *admissionManager) nextWakeLocked(waiter *admissionWaiter, now time.Time) time.Time {
	if waiter.granted || waiter.ctx.Err() != nil {
		return time.Time{}
	}
	wakeAt := waiter.request.admission.NotBefore
	availableAt := manager.classAvailableAtLocked(waiter.request.admission.Class)
	if availableAt.After(wakeAt) {
		wakeAt = availableAt
	}
	if !wakeAt.After(now) || manager.held[waiter.request.admission.Class] != 0 {
		return time.Time{}
	}
	return wakeAt
}

func (manager *admissionManager) classAvailableAtLocked(class AdmissionClass) time.Time {
	if manager.availability == nil {
		return time.Time{}
	}
	return manager.availability(class)
}

func (manager *admissionManager) removeCancelledLocked() {
	remaining := manager.waiters[:0]
	for _, waiter := range manager.waiters {
		if waiter.ctx.Err() == nil {
			remaining = append(remaining, waiter)
		}
	}
	manager.waiters = remaining
}

func (manager *admissionManager) removeWaiterLocked(id uint64) {
	for index, waiter := range manager.waiters {
		if waiter.id == id {
			manager.waiters = append(manager.waiters[:index], manager.waiters[index+1:]...)
			return
		}
	}
}

func admissionWaiterBefore(left, right *admissionWaiter, now time.Time) bool {
	leftTier := admissionTier(left.request.actor)
	rightTier := admissionTier(right.request.actor)
	if leftTier != rightTier {
		return leftTier > rightTier
	}
	if !left.request.admission.Deadline.IsZero() || !right.request.admission.Deadline.IsZero() {
		if left.request.admission.Deadline.IsZero() {
			return false
		}
		if right.request.admission.Deadline.IsZero() {
			return true
		}
		if !left.request.admission.Deadline.Equal(right.request.admission.Deadline) {
			return left.request.admission.Deadline.Before(right.request.admission.Deadline)
		}
	}
	if leftTier == 1 {
		leftWeight := effectiveAdmissionWeight(left.request, now)
		rightWeight := effectiveAdmissionWeight(right.request, now)
		if leftWeight != rightWeight {
			return leftWeight > rightWeight
		}
	}
	if left.request.priority != right.request.priority {
		return left.request.priority > right.request.priority
	}
	if !left.request.submittedAt.Equal(right.request.submittedAt) {
		return left.request.submittedAt.Before(right.request.submittedAt)
	}
	return left.id < right.id
}

func admissionTier(actor string) int {
	actor = strings.ToLower(strings.TrimSpace(actor))
	switch {
	case strings.HasPrefix(actor, "automation:"), strings.HasPrefix(actor, "background:"), actor == "report-manager":
		return 1
	case strings.HasPrefix(actor, "scheduler"):
		return 3
	default:
		return 4
	}
}

func effectiveAdmissionWeight(request admissionRequest, now time.Time) int {
	age := now.Sub(request.submittedAt)
	if age < 0 {
		age = 0
	}
	bonus := int(age / admissionAgingInterval)
	if bonus > 100 {
		bonus = 100
	}
	return normalizeAdmissionWeight(request.weight) + bonus
}

func normalizeAdmissionWeight(value int) int {
	if value <= 0 {
		return defaultAdmissionWeight
	}
	if value > 100 {
		return 100
	}
	return value
}

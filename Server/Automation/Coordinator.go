package Automation

import (
	"context"
	"log"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Lease struct {
	mu          sync.RWMutex
	id          int64
	owner       string
	reason      string
	priority    Priority
	workID      string
	claims      []Claim
	ctx         context.Context
	cancel      context.CancelFunc
	coordinator *Coordinator
	acquiredAt  time.Time
	closed      bool
	revoked     bool
	protected   bool
}

func (l *Lease) ID() int64 {
	if l == nil {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.id
}

func (l *Lease) Owner() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.owner
}

func (l *Lease) Priority() Priority {
	if l == nil {
		return 0
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.priority
}

func (l *Lease) Claims() []Claim {
	if l == nil {
		return nil
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return append([]Claim(nil), l.claims...)
}

func (l *Lease) Context() context.Context {
	if l == nil {
		return context.Background()
	}
	return l.ctx
}

func (l *Lease) Revoked() bool {
	if l == nil {
		return true
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.revoked
}

func (l *Lease) Active() bool {
	if l == nil {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return !l.closed && !l.revoked && l.ctx.Err() == nil
}

func (l *Lease) Guard() func() bool {
	return func() bool { return l.Active() }
}

func (l *Lease) WorkID() string {
	if l == nil {
		return ""
	}
	l.mu.RLock()
	workID := l.workID
	id := l.id
	l.mu.RUnlock()
	if workID != "" {
		return workID
	}
	return "lease:" + strconv.FormatInt(id, 10)
}

func (l *Lease) Release() {
	if l == nil || l.coordinator == nil {
		return
	}
	l.coordinator.release(l, false)
}

func (l *Lease) close(revoked bool) {
	l.mu.Lock()
	if l.closed {
		l.mu.Unlock()
		return
	}
	l.closed = true
	l.revoked = l.revoked || revoked
	cancel := l.cancel
	l.mu.Unlock()
	cancel()
}

type waiter struct {
	req      Request
	ctx      context.Context
	seq      int64
	queuedAt time.Time
	ch       chan *Lease
	lease    *Lease
	canceled bool
}

type holdState struct {
	lease *Lease
	seq   int64
}

type Coordinator struct {
	mu      sync.Mutex
	active  map[int64]*Lease
	waiters []*waiter
	holds   map[string]*holdState
	nextID  int64
	nextSeq int64
}

func NewCoordinator() *Coordinator {
	return &Coordinator{
		active: make(map[int64]*Lease),
		holds:  make(map[string]*holdState),
	}
}

var Global = NewCoordinator()

func Acquire(ctx context.Context, req Request) (*Lease, bool) {
	return Global.Acquire(ctx, req)
}

func Hold(req Request, duration time.Duration) *Lease {
	return Global.Hold(req, duration)
}

func Snapshot() CoordinatorStatus {
	return Global.Snapshot()
}

func (c *Coordinator) Acquire(ctx context.Context, req Request) (*Lease, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = normalizeRequest(req)
	if len(req.Claims) == 0 {
		return nil, false
	}
	if !req.Deadline.IsZero() && !req.Deadline.After(time.Now()) {
		return nil, false
	}
	w := &waiter{req: req, ctx: ctx, queuedAt: time.Now(), ch: make(chan *Lease, 1)}

	c.mu.Lock()
	c.nextSeq++
	w.seq = c.nextSeq
	c.waiters = append(c.waiters, w)
	c.preemptLowerLocked(req)
	c.grantReadyLocked()
	c.mu.Unlock()

	var deadline <-chan time.Time
	var deadlineTimer *time.Timer
	if !req.Deadline.IsZero() {
		deadlineTimer = time.NewTimer(time.Until(req.Deadline))
		deadline = deadlineTimer.C
		defer deadlineTimer.Stop()
	}
	select {
	case lease := <-w.ch:
		if lease == nil || !lease.Active() {
			if lease != nil {
				lease.Release()
			}
			return nil, false
		}
		return lease, true
	case <-ctx.Done():
		c.mu.Lock()
		if w.lease != nil {
			c.releaseLocked(w.lease, false)
		} else {
			w.canceled = true
			c.removeWaiterLocked(w)
		}
		c.mu.Unlock()
		return nil, false
	case <-deadline:
		c.mu.Lock()
		if w.lease != nil {
			c.releaseLocked(w.lease, false)
		} else {
			w.canceled = true
			c.removeWaiterLocked(w)
		}
		c.mu.Unlock()
		return nil, false
	}
}

func (c *Coordinator) Hold(req Request, duration time.Duration) *Lease {
	if duration <= 0 {
		duration = 30 * time.Second
	}
	req = normalizeRequest(req)
	req.PreemptLower = true
	req.Protected = true
	if len(req.Claims) == 0 {
		return nil
	}
	holdKey := requestHoldKey(req)

	c.mu.Lock()
	state := c.holds[holdKey]
	if state != nil && state.lease != nil && state.lease.Active() {
		state.seq++
		seq := state.seq
		lease := state.lease
		c.mu.Unlock()
		time.AfterFunc(duration, func() { c.expireHold(holdKey, seq, lease) })
		return lease
	}

	c.preemptLowerLocked(req)
	if c.conflictsLocked(req.Claims, 0) {
		c.mu.Unlock()
		return nil
	}
	lease := c.newLeaseLocked(req, context.Background())
	c.active[lease.id] = lease
	state = &holdState{lease: lease, seq: 1}
	c.holds[holdKey] = state
	c.mu.Unlock()

	time.AfterFunc(duration, func() { c.expireHold(holdKey, 1, lease) })
	return lease
}

func (c *Coordinator) expireHold(key string, seq int64, lease *Lease) {
	c.mu.Lock()
	state := c.holds[key]
	if state == nil || state.seq != seq || state.lease != lease {
		c.mu.Unlock()
		return
	}
	delete(c.holds, key)
	c.releaseLocked(lease, false)
	c.mu.Unlock()
}

func (c *Coordinator) release(lease *Lease, revoked bool) {
	c.mu.Lock()
	c.releaseLocked(lease, revoked)
	c.mu.Unlock()
}

func (c *Coordinator) releaseLocked(lease *Lease, revoked bool) {
	if lease == nil {
		return
	}
	if current, ok := c.active[lease.id]; ok && current == lease {
		delete(c.active, lease.id)
	}
	for key, state := range c.holds {
		if state != nil && state.lease == lease {
			delete(c.holds, key)
		}
	}
	lease.close(revoked)
	c.grantReadyLocked()
}

func (c *Coordinator) preemptLowerLocked(req Request) {
	if !req.PreemptLower && len(req.SupersedeOwners) == 0 {
		return
	}
	for id, lease := range c.active {
		if lease == nil || !claimsConflict(req.Claims, lease.claims) {
			continue
		}
		if ownerInList(lease.owner, req.SupersedeOwners) {
			log.Printf("[Automation] superseding %s lease for %s (%s)", lease.owner, req.Owner, req.Reason)
			delete(c.active, id)
			for key, state := range c.holds {
				if state != nil && state.lease == lease {
					delete(c.holds, key)
				}
			}
			lease.close(true)
			continue
		}
		if lease.protected || lease.priority > req.Priority ||
			(lease.priority == req.Priority && !req.PreemptEqual) {
			continue
		}
		log.Printf("[Automation] revoking %s lease for %s (%s)", lease.owner, req.Owner, req.Reason)
		delete(c.active, id)
		lease.close(true)
	}
}

func ownerInList(owner string, owners []string) bool {
	for _, candidate := range owners {
		if owner == candidate {
			return true
		}
	}
	return false
}

func (c *Coordinator) grantReadyLocked() {
	for {
		now := time.Now()
		kept := c.waiters[:0]
		for _, w := range c.waiters {
			if w == nil || w.canceled || w.ctx.Err() != nil ||
				(!w.req.Deadline.IsZero() && !w.req.Deadline.After(now)) {
				if w != nil {
					w.canceled = true
				}
				continue
			}
			kept = append(kept, w)
		}
		c.waiters = kept

		best := -1
		for i, w := range c.waiters {
			if w == nil || w.canceled || c.conflictsLocked(w.req.Claims, 0) {
				continue
			}
			if best == -1 || waiterBefore(w, c.waiters[best]) {
				best = i
			}
		}
		if best == -1 {
			return
		}
		w := c.waiters[best]
		c.waiters = append(c.waiters[:best], c.waiters[best+1:]...)
		lease := c.newLeaseLocked(w.req, w.ctx)
		c.active[lease.id] = lease
		w.lease = lease
		w.ch <- lease
		if w.req.MaxHold > 0 {
			time.AfterFunc(w.req.MaxHold, func() { c.revokeIfActive(lease) })
		}
	}
}

func (c *Coordinator) revokeIfActive(lease *Lease) {
	c.mu.Lock()
	current, ok := c.active[lease.id]
	if ok && current == lease {
		log.Printf("[Automation] revoking %s lease after max hold", lease.owner)
		c.releaseLocked(lease, true)
	}
	c.mu.Unlock()
}

func (c *Coordinator) newLeaseLocked(req Request, parent context.Context) *Lease {
	c.nextID++
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	lease := &Lease{
		id:          c.nextID,
		owner:       req.Owner,
		reason:      req.Reason,
		priority:    req.Priority,
		workID:      strings.TrimSpace(req.WorkID),
		claims:      append([]Claim(nil), req.Claims...),
		ctx:         ctx,
		cancel:      cancel,
		coordinator: c,
		acquiredAt:  time.Now(),
		protected:   req.Protected,
	}
	go c.releaseWhenContextEnds(lease)
	return lease
}

func (c *Coordinator) releaseWhenContextEnds(lease *Lease) {
	<-lease.ctx.Done()
	c.mu.Lock()
	if current := c.active[lease.id]; current == lease {
		c.releaseLocked(lease, lease.Revoked())
	}
	c.mu.Unlock()
}

func (c *Coordinator) conflictsLocked(claims []Claim, exceptID int64) bool {
	for id, lease := range c.active {
		if id == exceptID || lease == nil || !lease.Active() {
			continue
		}
		if claimsConflict(claims, lease.claims) {
			return true
		}
	}
	return false
}

func (c *Coordinator) removeWaiterLocked(target *waiter) {
	for i, w := range c.waiters {
		if w == target {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			return
		}
	}
}

func claimsConflict(a, b []Claim) bool {
	for _, left := range a {
		for _, right := range b {
			if left.Key == right.Key && (left.Mode == ClaimExclusive || right.Mode == ClaimExclusive) {
				return true
			}
		}
	}
	return false
}

func waiterBefore(a, b *waiter) bool {
	if a.req.Priority != b.req.Priority {
		return a.req.Priority > b.req.Priority
	}
	aDeadline := a.req.Deadline
	bDeadline := b.req.Deadline
	if !aDeadline.IsZero() || !bDeadline.IsZero() {
		if aDeadline.IsZero() {
			return false
		}
		if bDeadline.IsZero() {
			return true
		}
		if !aDeadline.Equal(bDeadline) {
			return aDeadline.Before(bDeadline)
		}
	}
	return a.seq < b.seq
}

func normalizeRequest(req Request) Request {
	req.Owner = strings.TrimSpace(req.Owner)
	if req.Owner == "" {
		req.Owner = OwnerBackground
	}
	if req.Priority == 0 {
		req.Priority = DefaultPriority(req.Owner)
	}
	if strings.TrimSpace(req.Reason) == "" {
		req.Reason = req.Owner
	}
	req.Claims = normalizeClaims(req.Claims)
	return req
}

func requestHoldKey(req Request) string {
	parts := make([]string, 0, len(req.Claims)+1)
	parts = append(parts, req.Owner)
	for _, claim := range req.Claims {
		parts = append(parts, claim.Key)
	}
	return strings.Join(parts, "|")
}

type LeaseStatus struct {
	ID         int64     `json:"id"`
	WorkID     string    `json:"workId"`
	Owner      string    `json:"owner"`
	Reason     string    `json:"reason"`
	Priority   Priority  `json:"priority"`
	Claims     []Claim   `json:"claims"`
	AcquiredAt time.Time `json:"acquiredAt"`
}

type WaiterStatus struct {
	Owner    string    `json:"owner"`
	WorkID   string    `json:"workId,omitempty"`
	Reason   string    `json:"reason"`
	Priority Priority  `json:"priority"`
	Claims   []Claim   `json:"claims"`
	QueuedAt time.Time `json:"queuedAt"`
	Deadline time.Time `json:"deadline,omitempty"`
}

type CoordinatorStatus struct {
	Active []LeaseStatus  `json:"active"`
	Queued []WaiterStatus `json:"queued"`
}

func (c *Coordinator) Snapshot() CoordinatorStatus {
	c.mu.Lock()
	defer c.mu.Unlock()
	status := CoordinatorStatus{}
	for _, lease := range c.active {
		if lease == nil || !lease.Active() {
			continue
		}
		status.Active = append(status.Active, LeaseStatus{
			ID:         lease.id,
			WorkID:     lease.WorkID(),
			Owner:      lease.owner,
			Reason:     lease.reason,
			Priority:   lease.priority,
			Claims:     append([]Claim(nil), lease.claims...),
			AcquiredAt: lease.acquiredAt,
		})
	}
	for _, w := range c.waiters {
		if w == nil || w.canceled {
			continue
		}
		status.Queued = append(status.Queued, WaiterStatus{
			Owner:    w.req.Owner,
			WorkID:   w.req.WorkID,
			Reason:   w.req.Reason,
			Priority: w.req.Priority,
			Claims:   append([]Claim(nil), w.req.Claims...),
			QueuedAt: w.queuedAt,
			Deadline: w.req.Deadline,
		})
	}
	return status
}

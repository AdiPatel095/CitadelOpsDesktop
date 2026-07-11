package GameFocus

import (
	"context"
	"log"
	"sync"
	"time"
)

type Priority int

const (
	PriorityBackground Priority = 10
	PriorityAutoTool   Priority = 40
	PriorityRecruit    Priority = 40
	PriorityHospital   Priority = 50
	PriorityAutoBird   Priority = 60
	PriorityAutoTCI    Priority = 70
	PriorityManual     Priority = 100
)

const (
	OwnerManual       = "manual"
	OwnerAutoTCI      = "autoTCI"
	OwnerAutoBird     = "autoBird"
	OwnerAutoHospital = "autoHospital"
	OwnerAutoRecruit  = "autoRecruit"
	OwnerAutoTool     = "autoTool"
	OwnerDecoration   = "decoration"
)

const defaultManualIdleHold = 30 * time.Second

type Request struct {
	Owner    string
	Priority Priority
	Reason   string
	MaxHold  time.Duration
}

type Lease struct {
	mu       sync.RWMutex
	id       int64
	owner    string
	reason   string
	priority Priority
	ctx      context.Context
	cancel   context.CancelFunc
	closed   bool
	revoked  bool
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

func (l *Lease) Release() {
	if l == nil {
		return
	}
	global.release(l)
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
	seq      int64
	ch       chan *Lease
	lease    *Lease
	canceled bool
}

type Coordinator struct {
	mu        sync.Mutex
	current   *Lease
	nextID    int64
	nextSeq   int64
	waiters   []*waiter
	manualSeq int64
}

var global = &Coordinator{}

func Acquire(ctx context.Context, req Request) (*Lease, bool) {
	return global.Acquire(ctx, req)
}

func RecordManualActivity(reason string, hold time.Duration) {
	global.RecordManualActivity(reason, hold)
}

func (c *Coordinator) Acquire(ctx context.Context, req Request) (*Lease, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	req = normalizeRequest(req)
	w := &waiter{req: req, ch: make(chan *Lease, 1)}

	c.mu.Lock()
	c.nextSeq++
	w.seq = c.nextSeq
	c.waiters = append(c.waiters, w)
	c.grantNextLocked()
	c.mu.Unlock()

	select {
	case lease := <-w.ch:
		return lease, lease != nil
	case <-ctx.Done():
		c.mu.Lock()
		if w.lease != nil {
			c.releaseLocked(w.lease)
		} else {
			w.canceled = true
			c.removeWaiterLocked(w)
		}
		c.mu.Unlock()
		return nil, false
	}
}

func (c *Coordinator) RecordManualActivity(reason string, hold time.Duration) {
	if hold <= 0 {
		hold = defaultManualIdleHold
	}
	req := Request{
		Owner:    OwnerManual,
		Priority: PriorityManual,
		Reason:   reason,
	}

	c.mu.Lock()
	if c.current != nil && c.current.owner != OwnerManual {
		log.Printf("[GameFocus] revoking %s lease for manual activity (%s)", c.current.owner, reason)
		c.current.close(true)
		c.current = nil
	}
	if c.current == nil {
		c.current = c.newLeaseLocked(req)
		log.Printf("[GameFocus] manual focus hold active for %s", hold)
	}
	c.manualSeq++
	seq := c.manualSeq
	lease := c.current
	c.mu.Unlock()

	time.AfterFunc(hold, func() {
		c.expireManual(seq, lease)
	})
}

func (c *Coordinator) release(l *Lease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.releaseLocked(l)
}

func (c *Coordinator) releaseLocked(l *Lease) {
	if l == nil {
		return
	}
	if c.current == l {
		c.current.close(false)
		c.current = nil
		c.grantNextLocked()
		return
	}
	l.close(false)
}

func (c *Coordinator) expireManual(seq int64, lease *Lease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if seq != c.manualSeq || lease == nil || c.current != lease || lease.owner != OwnerManual {
		return
	}
	log.Printf("[GameFocus] manual focus hold expired")
	c.current.close(false)
	c.current = nil
	c.grantNextLocked()
}

func (c *Coordinator) grantNextLocked() {
	if c.current != nil || len(c.waiters) == 0 {
		return
	}

	best := -1
	for i, w := range c.waiters {
		if w.canceled {
			continue
		}
		if best == -1 ||
			w.req.Priority > c.waiters[best].req.Priority ||
			(w.req.Priority == c.waiters[best].req.Priority && w.seq < c.waiters[best].seq) {
			best = i
		}
	}
	if best == -1 {
		c.waiters = nil
		return
	}

	w := c.waiters[best]
	c.waiters = append(c.waiters[:best], c.waiters[best+1:]...)
	lease := c.newLeaseLocked(w.req)
	c.current = lease
	w.lease = lease
	w.ch <- lease

	if w.req.MaxHold > 0 {
		time.AfterFunc(w.req.MaxHold, func() {
			c.revokeIfCurrent(lease)
		})
	}
}

func (c *Coordinator) revokeIfCurrent(lease *Lease) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.current != lease {
		return
	}
	log.Printf("[GameFocus] revoking %s lease after max hold", lease.owner)
	lease.close(true)
	c.current = nil
	c.grantNextLocked()
}

func (c *Coordinator) newLeaseLocked(req Request) *Lease {
	c.nextID++
	ctx, cancel := context.WithCancel(context.Background())
	return &Lease{
		id:       c.nextID,
		owner:    req.Owner,
		reason:   req.Reason,
		priority: req.Priority,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (c *Coordinator) removeWaiterLocked(target *waiter) {
	for i, w := range c.waiters {
		if w == target {
			c.waiters = append(c.waiters[:i], c.waiters[i+1:]...)
			return
		}
	}
}

func normalizeRequest(req Request) Request {
	if req.Owner == "" {
		req.Owner = "unknown"
	}
	if req.Priority == 0 {
		req.Priority = DefaultPriority(req.Owner)
	}
	if req.Reason == "" {
		req.Reason = req.Owner
	}
	return req
}

func DefaultPriority(owner string) Priority {
	switch owner {
	case OwnerManual, OwnerDecoration:
		return PriorityManual
	case OwnerAutoTCI:
		return PriorityAutoTCI
	case OwnerAutoBird:
		return PriorityAutoBird
	case OwnerAutoHospital:
		return PriorityHospital
	case OwnerAutoRecruit:
		return PriorityRecruit
	case OwnerAutoTool:
		return PriorityAutoTool
	default:
		return PriorityBackground
	}
}

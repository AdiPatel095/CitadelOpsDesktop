package Automation

import (
	"context"
	"errors"
	"sort"
	"strconv"
	"sync"
	"time"
)

var ErrWorkCancelled = errors.New("automation work cancelled")

type Operation func(context.Context, *Lease) error

type WorkItem struct {
	ID        string
	DedupeKey string
	NotBefore time.Time
	Request   Request
	Run       Operation
}

type WorkResult struct {
	ID         string    `json:"id"`
	Owner      string    `json:"owner"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt"`
	Err        error     `json:"-"`
}

type WorkHandle struct {
	mu     sync.RWMutex
	result WorkResult
	done   chan struct{}
	cancel context.CancelFunc
}

func (h *WorkHandle) Done() <-chan struct{} {
	if h == nil {
		closed := make(chan struct{})
		close(closed)
		return closed
	}
	return h.done
}

func (h *WorkHandle) Wait(ctx context.Context) (WorkResult, bool) {
	if h == nil {
		return WorkResult{}, false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-ctx.Done():
		return WorkResult{}, false
	case <-h.done:
		h.mu.RLock()
		result := h.result
		h.mu.RUnlock()
		return result, true
	}
}

func (h *WorkHandle) Cancel() {
	if h != nil && h.cancel != nil {
		h.cancel()
	}
}

type WorkStatus struct {
	ID          string    `json:"id"`
	DedupeKey   string    `json:"dedupeKey,omitempty"`
	Owner       string    `json:"owner"`
	Priority    Priority  `json:"priority"`
	Reason      string    `json:"reason"`
	Claims      []Claim   `json:"claims"`
	Phase       string    `json:"phase"`
	NotBefore   time.Time `json:"notBefore,omitempty"`
	SubmittedAt time.Time `json:"submittedAt"`
	StartedAt   time.Time `json:"startedAt,omitempty"`
}

type activeWork struct {
	item        WorkItem
	handle      *WorkHandle
	submittedAt time.Time
	startedAt   time.Time
	phase       string
}

type WorkBroker struct {
	mu       sync.Mutex
	nextID   uint64
	active   map[string]*activeWork
	byDedupe map[string]*activeWork
}

func NewWorkBroker() *WorkBroker {
	return &WorkBroker{
		active:   make(map[string]*activeWork),
		byDedupe: make(map[string]*activeWork),
	}
}

var Work = NewWorkBroker()

func SubmitWork(ctx context.Context, item WorkItem) *WorkHandle {
	return Work.Submit(ctx, item)
}

func RunWork(ctx context.Context, item WorkItem) error {
	handle := SubmitWork(ctx, item)
	result, ok := handle.Wait(ctx)
	if !ok {
		return ErrWorkCancelled
	}
	return result.Err
}

// CancelOwner stops queued/running work and drops that owner's commands that have not reached
// the game socket yet. Feature stop functions should use this single cancellation entry point.
func CancelOwner(owner string) int {
	return Work.CancelOwner(owner) + Commands.CancelOwner(owner)
}

func (b *WorkBroker) Submit(ctx context.Context, item WorkItem) *WorkHandle {
	if ctx == nil {
		ctx = context.Background()
	}
	item.Request = normalizeRequest(item.Request)

	b.mu.Lock()
	if item.DedupeKey != "" {
		if existing := b.byDedupe[item.DedupeKey]; existing != nil {
			handle := existing.handle
			b.mu.Unlock()
			return handle
		}
	}
	if item.ID == "" {
		b.nextID++
		item.ID = workID(b.nextID)
	}
	item.Request.WorkID = item.ID
	workCtx, cancel := context.WithCancel(ctx)
	handle := &WorkHandle{
		result: WorkResult{ID: item.ID, Owner: item.Request.Owner},
		done:   make(chan struct{}),
		cancel: cancel,
	}
	active := &activeWork{
		item:        item,
		handle:      handle,
		submittedAt: time.Now(),
		phase:       "queued",
	}
	b.active[item.ID] = active
	if item.DedupeKey != "" {
		b.byDedupe[item.DedupeKey] = active
	}
	b.mu.Unlock()

	go b.execute(workCtx, active)
	return handle
}

func (b *WorkBroker) execute(ctx context.Context, active *activeWork) {
	item := active.item
	result := WorkResult{ID: item.ID, Owner: item.Request.Owner}
	if !item.NotBefore.IsZero() {
		delay := time.Until(item.NotBefore)
		if delay > 0 {
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				timer.Stop()
				result.Err = ctx.Err()
				b.finish(active, result)
				return
			case <-timer.C:
			}
		}
	}

	b.setPhase(active, "waiting-for-claims", time.Time{})
	lease, ok := Global.Acquire(ctx, item.Request)
	if !ok {
		result.Err = ctx.Err()
		if result.Err == nil {
			result.Err = ErrWorkCancelled
		}
		b.finish(active, result)
		return
	}
	result.StartedAt = time.Now()
	b.setPhase(active, "running", result.StartedAt)
	if item.Run == nil {
		result.Err = nil
	} else {
		result.Err = item.Run(lease.Context(), lease)
	}
	b.setPhase(active, "draining-commands", result.StartedAt)
	if !WaitForWork(lease.Context(), lease.WorkID()) {
		Commands.CancelWork(lease.WorkID())
	}
	lease.Release()
	result.FinishedAt = time.Now()
	b.finish(active, result)
}

func (b *WorkBroker) setPhase(active *activeWork, phase string, startedAt time.Time) {
	b.mu.Lock()
	if current := b.active[active.item.ID]; current == active {
		active.phase = phase
		if !startedAt.IsZero() {
			active.startedAt = startedAt
		}
	}
	b.mu.Unlock()
}

func (b *WorkBroker) finish(active *activeWork, result WorkResult) {
	if result.FinishedAt.IsZero() {
		result.FinishedAt = time.Now()
	}
	active.handle.mu.Lock()
	active.handle.result = result
	active.handle.mu.Unlock()

	b.mu.Lock()
	delete(b.active, active.item.ID)
	if active.item.DedupeKey != "" && b.byDedupe[active.item.DedupeKey] == active {
		delete(b.byDedupe, active.item.DedupeKey)
	}
	b.mu.Unlock()
	close(active.handle.done)
}

func (b *WorkBroker) CancelOwner(owner string) int {
	b.mu.Lock()
	handles := make([]*WorkHandle, 0)
	for _, active := range b.active {
		if active.item.Request.Owner == owner {
			handles = append(handles, active.handle)
		}
	}
	b.mu.Unlock()
	for _, handle := range handles {
		handle.Cancel()
	}
	return len(handles)
}

func (b *WorkBroker) Snapshot() []WorkStatus {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]WorkStatus, 0, len(b.active))
	for _, active := range b.active {
		out = append(out, WorkStatus{
			ID:          active.item.ID,
			DedupeKey:   active.item.DedupeKey,
			Owner:       active.item.Request.Owner,
			Priority:    active.item.Request.Priority,
			Reason:      active.item.Request.Reason,
			Claims:      append([]Claim(nil), active.item.Request.Claims...),
			Phase:       active.phase,
			NotBefore:   active.item.NotBefore,
			SubmittedAt: active.submittedAt,
			StartedAt:   active.startedAt,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].SubmittedAt.Before(out[j].SubmittedAt)
	})
	return out
}

func workID(id uint64) string {
	return "work:" + strconv.FormatUint(id, 10)
}

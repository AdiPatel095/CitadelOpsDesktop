package Automation

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	StateAll       = "state:all"
	StateSession   = "state:session"
	StateFocus     = "state:focus"
	StateCastles   = "state:castles"
	StateResources = "state:resources"
	StateMovement  = "state:movement"
	StateInventory = "state:inventory"
	StateTransport = "state:transport"
	StateEquipment = "state:equipment"
	StateAlliance  = "state:alliance"
)

func StateOpcode(opcode string) string {
	return "state:opcode:" + strings.ToLower(strings.TrimSpace(opcode))
}

func StateEntity(domain string, id int) string {
	return fmt.Sprintf("state:%s:%d", strings.ToLower(strings.TrimSpace(domain)), id)
}

type StateStamp struct {
	Version   uint64    `json:"version"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type StateTracker struct {
	mu     sync.RWMutex
	stamps map[string]StateStamp
	notify chan struct{}
}

func NewStateTracker() *StateTracker {
	return &StateTracker{
		stamps: make(map[string]StateStamp),
		notify: make(chan struct{}),
	}
}

var State = NewStateTracker()

func ObserveState(keys ...string) {
	State.Observe(keys...)
}

func StateSnapshot(key string) StateStamp {
	return State.Snapshot(key)
}

func AwaitStateAfter(ctx context.Context, key string, version uint64) (StateStamp, bool) {
	return State.AwaitAfter(ctx, key, version)
}

func (s *StateTracker) Observe(keys ...string) {
	now := time.Now()
	keys = append(keys, StateAll)
	seen := make(map[string]struct{}, len(keys))
	s.mu.Lock()
	for _, rawKey := range keys {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		stamp := s.stamps[key]
		stamp.Version++
		stamp.UpdatedAt = now
		s.stamps[key] = stamp
	}
	close(s.notify)
	s.notify = make(chan struct{})
	s.mu.Unlock()
}

func (s *StateTracker) Snapshot(key string) StateStamp {
	s.mu.RLock()
	stamp := s.stamps[strings.TrimSpace(key)]
	s.mu.RUnlock()
	return stamp
}

func (s *StateTracker) AwaitAfter(ctx context.Context, key string, version uint64) (StateStamp, bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	for {
		s.mu.RLock()
		stamp := s.stamps[strings.TrimSpace(key)]
		notify := s.notify
		s.mu.RUnlock()
		if stamp.Version > version {
			return stamp, true
		}
		select {
		case <-ctx.Done():
			return StateStamp{}, false
		case <-notify:
		}
	}
}

func (s *StateTracker) All() map[string]StateStamp {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]StateStamp, len(s.stamps))
	for key, stamp := range s.stamps {
		out[key] = stamp
	}
	return out
}

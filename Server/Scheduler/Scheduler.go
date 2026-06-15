package Scheduler

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"log"
	"sync"
	"time"
)

// Scheduler manages the dispatching of attack requests
type Scheduler struct {
	Producers       map[Models.TabPriority]AttackProducer
	CooldownTracker *CooldownTracker
	RegularQueue    []*AttackRequest // Queue for normal priority
	mu              sync.Mutex
	ctx             context.Context
	cancel          context.CancelFunc
}

// Global instance
var (
	GlobalScheduler *Scheduler
	once            sync.Once
)

// GetScheduler returns the singleton instance
func GetScheduler() *Scheduler {
	once.Do(func() {
		GlobalScheduler = &Scheduler{
			Producers:       make(map[Models.TabPriority]AttackProducer),
			CooldownTracker: NewCooldownTracker(),
			RegularQueue:    make([]*AttackRequest, 0),
		}
	})
	return GlobalScheduler
}

// RegisterProducer adds a producer for a specific priority level
func (s *Scheduler) RegisterProducer(priority Models.TabPriority, p AttackProducer) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Producers[priority] = p
}

// Start begins the main scheduler loop
func (s *Scheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return // Already running
	}

	ctx, cancel := context.WithCancel(context.Background())
	s.ctx = ctx
	s.cancel = cancel

	go s.runLoop(ctx)
}

// Stop halts the scheduler loop
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
}

// runLoop is the main loop processing attack dispatch and basic 50ms ticks
func (s *Scheduler) runLoop(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond) // Fast ticker for checking priority queues
	defer ticker.Stop()

	var nextDispatchTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// 1. Check if we are ready to send the next attack
			if time.Now().Before(nextDispatchTime) {
				continue // Wait for attack cooldown
			}

			// Attempt to dispatch
			if req := s.getNextTarget(); req != nil {
				// Process the attack
				success := s.dispatchAttack(req)

				if success {
					nextDispatchTime = time.Now().Add(Models.RandomAttackDelay())
				}
			}
		}
	}
}

func (s *Scheduler) getNextTarget() *AttackRequest {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Prioritize P1 > P2 > P3 > RegularQueue

	// Try P1
	if p1, ok := s.Producers[Models.Priority1]; ok {
		if req := p1.GetNextAttack(); req != nil {
			return req
		}
	}

	// Try P2
	if p2, ok := s.Producers[Models.Priority2]; ok {
		if req := p2.GetNextAttack(); req != nil {
			return req
		}
	}

	// Try P3
	if p3, ok := s.Producers[Models.Priority3]; ok {
		if req := p3.GetNextAttack(); req != nil {
			return req
		}
	}

	// Try RegularQueue
	if len(s.RegularQueue) > 0 {
		req := s.RegularQueue[0]
		s.RegularQueue = s.RegularQueue[1:]
		return req
	}

	return nil
}

func (s *Scheduler) dispatchAttack(req *AttackRequest) bool {
	// If target is on cooldown, we skip.
	if s.CooldownTracker.IsOnCooldown(req.TargetID) {
		log.Printf("[Scheduler] Target %s is on cooldown, skipping dispatch.", req.TargetID)
		return false
	}

	// Build Payload for attack
	payload, ok := req.Payload.(string)
	if !ok {
		log.Printf("[Scheduler] Invalid parameter struct for target %s", req.TargetID)
		return false
	}

	log.Printf("[Scheduler] Dispatching attack payload to %s (Priority %s)", req.TargetID, req.Priority)

	// Mark as in-flight tracking
	s.CooldownTracker.SetInFlight(req.TargetID)

	ResponseRegistry.OutgoingMessages <- []byte(payload)
	return true
}

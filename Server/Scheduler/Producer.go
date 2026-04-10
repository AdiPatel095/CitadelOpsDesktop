package Scheduler

import (
	"CitadelDesktop/Server/Models"
	"context"
	"sync"
)

// AttackRequest represents a single scheduled attack intent
type AttackRequest struct {
	TargetID string
	Priority Models.TabPriority
	Payload  interface{} // Holds arbitrary data for the attack dispatch (target coords, etc.)
}

// AttackProducer defines the interface for creating and yielding targets for attack
type AttackProducer interface {
	Run(ctx context.Context)
	GetNextAttack() *AttackRequest
	Refresh()
	NotifyUpdate()
}

// BasicPriorityProducer implements an AttackProducer for a given TabPriority tier
type BasicPriorityProducer struct {
	priority Models.TabPriority
	queue    []*AttackRequest
	mu       sync.Mutex
	ch       chan struct{} // channel for awake notifications
}

// NewBasicPriorityProducer creates a new basic queue
func NewBasicPriorityProducer(priority Models.TabPriority) *BasicPriorityProducer {
	return &BasicPriorityProducer{
		priority: priority,
		queue:    make([]*AttackRequest, 0),
		ch:       make(chan struct{}, 1),
	}
}

// Run can handle background replenishment loops
func (p *BasicPriorityProducer) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-p.ch:
			// awake and process
			p.Refresh()
		}
	}
}

// GetNextAttack pops the front attack request off the queue
func (p *BasicPriorityProducer) GetNextAttack() *AttackRequest {
	p.mu.Lock()
	defer p.mu.Unlock()

	if len(p.queue) == 0 {
		return nil
	}

	req := p.queue[0]
	p.queue = p.queue[1:]
	return req
}

// Refresh handles queue replenishment
func (p *BasicPriorityProducer) Refresh() {
	p.mu.Lock()
	defer p.mu.Unlock()
	// Logic to refill the queue from models or database based on p.priority
	// Left blank for now as it depends on exact GameParser models
}

// NotifyUpdate signals the producer that new targets might be available or settings changed
func (p *BasicPriorityProducer) NotifyUpdate() {
	select {
	case p.ch <- struct{}{}:
	default:
	}
}

// Enqueue allows manually injecting an attack requirement into the producer
func (p *BasicPriorityProducer) Enqueue(req *AttackRequest) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.queue = append(p.queue, req)
	p.NotifyUpdate()
}

// Package ResponseRegistry provides game websocket lifecycle handling and an exact-one
// request/response matching pattern for callers that must wait for specific response types.
//
// # Data Flow
//
//	┌─────────────────────────────────────────────────────────────────────────────┐
//	│                           CALLER (e.g., Unequip.go)                         │
//	│                                                                             │
//	│  1. waiter := Global.RegisterWaiter("eeq", 5*time.Second)                   │
//	│  2. defer waiter.Cleanup()                                                  │
//	│  3. GameCommands.QueueOutgoingPayload(payload)                              │
//	│  4. response, err := waiter.WaitWithTimeout()  // blocks here               │
//	│  5. Process response or handle timeout                                      │
//	└─────────────────────────────────────────────────────────────────────────────┘
//	                    │                                    ▲
//	                    │ (1) Register                       │ (4) Response delivered
//	                    ▼                                    │
//	┌─────────────────────────────────────────────────────────────────────────────┐
//	│                         ResponseRegistry.Global                             │
//	│                                                                             │
//	│  waiters map[string][]*ResponseWaiter                                       │
//	│    "eeq" -> [waiter1, waiter2, ...]                                         │
//	│    "ege" -> [waiter3, ...]                                                  │
//	└─────────────────────────────────────────────────────────────────────────────┘
//	                                   ▲
//	                                   │ CheckWaiters(messageType, message)
//	                                   │
//	┌─────────────────────────────────────────────────────────────────────────────┐
//	│                     MessageRouter (GameParser package)                      │
//	│                                                                             │
//	│  func MessageRouter(parts []string) {                                       │
//	│      cmd, _ := GameParser.CommandType(parts)                                │
//	│      ResponseRegistry.Global.CheckWaiters(cmd, parts)                       │
//	│      // dispatch by cmd; JSON payload at index 5 when present               │
//	│  }                                                                          │
//	└─────────────────────────────────────────────────────────────────────────────┘
//	                                   ▲
//	                                   │ Incoming messages from game server
//	                                   │
//	┌─────────────────────────────────────────────────────────────────────────────┐
//	│                           Game WebSocket                                    │
//	└─────────────────────────────────────────────────────────────────────────────┘
//
// # Key Properties
//
//   - Non-blocking delivery: Waiters use buffered channels, delivery never blocks router
//   - First-match semantics: sync.Once ensures only the first matching message is delivered
//   - Self-cleaning: Cleanup() removes waiter from registry after use
//   - Timeout support: WaitWithTimeout returns ErrTimeout if no response arrives
//   - Multiple waiters: one frame is delivered to the oldest waiter whose matcher accepts it
//
// # Usage Example
//
//	waiter := ResponseRegistry.Global.RegisterWaiter("eeq", 5*time.Second)
//	defer waiter.Cleanup()
//
//	GameCommands.QueueOutgoingPayload(payload)
//
//	response, err := waiter.WaitWithTimeout()
//	if err == ResponseRegistry.ErrTimeout {
//	    return UnequipResult{Success: false, Message: "Timeout"}
//	}
//	return parseResponse(response)
package ResponseRegistry

import (
	"context"
	"errors"
	"sync"
	"time"
)

// ErrTimeout is returned when a waiter times out waiting for a response
var ErrTimeout = errors.New("response timeout")

// ResponseWaiter represents a registered listener waiting for a specific message type
type ResponseWaiter struct {
	MessageType string
	ResponseCh  chan []string
	// Match optionally narrows same-opcode responses to the request that owns this waiter.
	Match func([]string) bool
	// OnDeliver, if set, is invoked with a copy of the first matching frame before it is sent to ResponseCh.
	// Use this to parse **cds** (or other) payloads synchronously in the MessageRouter goroutine.
	OnDeliver func([]string)
	once      sync.Once     // Ensures only first match is delivered
	timeout   time.Duration // Cooldown/timeout duration
	createdAt time.Time
	registry  *Registry // Reference to parent registry for cleanup
}

// WaitWithTimeout blocks until a message is received or the timeout expires.
// Returns the message parts on success, or ErrTimeout if no message arrives in time.
func (w *ResponseWaiter) WaitWithTimeout() ([]string, error) {
	select {
	case msg := <-w.ResponseCh:
		return msg, nil
	case <-time.After(w.timeout):
		return nil, ErrTimeout
	}
}

// WaitWithContext waits for the matching response, the configured timeout, or caller cancellation.
func (w *ResponseWaiter) WaitWithContext(ctx context.Context) ([]string, error) {
	if ctx == nil {
		return w.WaitWithTimeout()
	}
	timer := time.NewTimer(w.timeout)
	defer timer.Stop()
	select {
	case msg := <-w.ResponseCh:
		return msg, nil
	case <-timer.C:
		return nil, ErrTimeout
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// Cleanup removes this waiter from the registry.
// Should be called (typically via defer) after WaitWithTimeout returns.
func (w *ResponseWaiter) Cleanup() {
	w.registry.removeWaiter(w)
}

// deliver sends the message to the waiter's channel (non-blocking, only once)
func (w *ResponseWaiter) deliver(message []string) bool {
	delivered := false
	w.once.Do(func() {
		if w.OnDeliver != nil {
			cp := append([]string(nil), message...)
			w.OnDeliver(cp)
		}
		select {
		case w.ResponseCh <- message:
			delivered = true
		default:
			// Channel full or closed, skip
		}
	})
	return delivered
}

// Registry manages waiters for incoming message types
type Registry struct {
	mu      sync.RWMutex
	waiters map[string][]*ResponseWaiter // Multiple waiters per message type
}

// NewRegistry creates a new empty registry
func NewRegistry() *Registry {
	return &Registry{
		waiters: make(map[string][]*ResponseWaiter),
	}
}

// Global is the shared registry instance for the application
var Global = NewRegistry()

// RegisterWaiter registers a new waiter for the given message type with a timeout.
// The waiter will receive a copy of the first matching message that arrives.
func (r *Registry) RegisterWaiter(messageType string, timeout time.Duration) *ResponseWaiter {
	return r.RegisterWaiterMatching(messageType, timeout, nil, nil)
}

// RegisterWaiterWithDeliver is like RegisterWaiter but runs onDeliver with a copy of the frame before pushing to ResponseCh.
func (r *Registry) RegisterWaiterWithDeliver(messageType string, timeout time.Duration, onDeliver func([]string)) *ResponseWaiter {
	return r.RegisterWaiterMatching(messageType, timeout, nil, onDeliver)
}

// RegisterWaiterMatching registers an exact-one waiter. When several callers wait on the same
// opcode, the oldest waiter whose matcher accepts the frame receives it; other waiters remain queued.
func (r *Registry) RegisterWaiterMatching(messageType string, timeout time.Duration, match func([]string) bool, onDeliver func([]string)) *ResponseWaiter {
	waiter := &ResponseWaiter{
		MessageType: messageType,
		ResponseCh:  make(chan []string, 1), // Buffered to prevent blocking
		Match:       match,
		OnDeliver:   onDeliver,
		timeout:     timeout,
		createdAt:   time.Now(),
		registry:    r,
	}

	r.mu.Lock()
	r.waiters[messageType] = append(r.waiters[messageType], waiter)
	r.mu.Unlock()

	return waiter
}

// CheckWaiters delivers a frame to exactly one matching waiter. This prevents concurrent
// same-opcode operations from all accepting the first response.
func (r *Registry) CheckWaiters(messageType string, message []string) {
	r.mu.Lock()
	waiters := r.waiters[messageType]
	matchIndex := -1
	var selected *ResponseWaiter
	for i, waiter := range waiters {
		if waiter == nil || (waiter.Match != nil && !waiter.Match(message)) {
			continue
		}
		matchIndex = i
		selected = waiter
		break
	}
	if matchIndex >= 0 {
		waiters = append(waiters[:matchIndex], waiters[matchIndex+1:]...)
		if len(waiters) == 0 {
			delete(r.waiters, messageType)
		} else {
			r.waiters[messageType] = waiters
		}
	}
	r.mu.Unlock()

	if selected != nil {
		selected.deliver(message)
	}
}

// removeWaiter removes a specific waiter from the registry
func (r *Registry) removeWaiter(waiter *ResponseWaiter) {
	r.mu.Lock()
	defer r.mu.Unlock()

	waiters, exists := r.waiters[waiter.MessageType]
	if !exists {
		return
	}

	// Find and remove the waiter while preserving registration order. CheckWaiters
	// intentionally gives a same-opcode frame to the oldest matching waiter.
	for i, w := range waiters {
		if w == waiter {
			r.waiters[waiter.MessageType] = append(waiters[:i], waiters[i+1:]...)
			break
		}
	}

	// Clean up empty slices
	if len(r.waiters[waiter.MessageType]) == 0 {
		delete(r.waiters, waiter.MessageType)
	}
}

// GetWaiterCount returns the number of active waiters (for testing/debugging)
func (r *Registry) GetWaiterCount(messageType string) int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.waiters[messageType])
}

// OutboundGameWireSendHook observes every native game websocket SEND (browser UI + Citadel queue).
var OutboundGameWireSendHook func(payload string)

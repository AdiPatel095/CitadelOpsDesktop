// Package ResponseRegistry provides game communication infrastructure:
// the outgoing message channel for sending commands, and a request/response
// matching pattern for waiting on specific response types.
//
// a command, then block until the expected response arrives (or timeout).
//
// # Data Flow
//
//	┌─────────────────────────────────────────────────────────────────────────────┐
//	│                           CALLER (e.g., Unequip.go)                         │
//	│                                                                             │
//	│  1. waiter := Global.RegisterWaiter("eeq", 5*time.Second)                   │
//	│  2. defer waiter.Cleanup()                                                  │
//	│  3. OutgoingMessages <- payload                                             │
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
//   - Multiple waiters: Multiple callers can wait for the same message type
//
// # Usage Example
//
//	waiter := ResponseRegistry.Global.RegisterWaiter("eeq", 5*time.Second)
//	defer waiter.Cleanup()
//
//	ResponseRegistry.OutgoingMessages <- payload
//
//	response, err := waiter.WaitWithTimeout()
//	if err == ResponseRegistry.ErrTimeout {
//	    return UnequipResult{Success: false, Message: "Timeout"}
//	}
//	return parseResponse(response)
package ResponseRegistry

import (
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
	return r.RegisterWaiterWithDeliver(messageType, timeout, nil)
}

// RegisterWaiterWithDeliver is like RegisterWaiter but runs onDeliver with a copy of the frame before pushing to ResponseCh.
func (r *Registry) RegisterWaiterWithDeliver(messageType string, timeout time.Duration, onDeliver func([]string)) *ResponseWaiter {
	waiter := &ResponseWaiter{
		MessageType: messageType,
		ResponseCh:  make(chan []string, 1), // Buffered to prevent blocking
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

// CheckWaiters checks if any waiters are registered for the given message type
// and delivers the message to them. This should be called by the message router.
func (r *Registry) CheckWaiters(messageType string, message []string) {
	r.mu.RLock()
	waiters, exists := r.waiters[messageType]
	r.mu.RUnlock()

	if !exists || len(waiters) == 0 {
		return
	}

	// Deliver to all registered waiters for this type
	for _, waiter := range waiters {
		waiter.deliver(message)
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

	// Find and remove the waiter
	for i, w := range waiters {
		if w == waiter {
			// Remove by swapping with last element and truncating
			waiters[i] = waiters[len(waiters)-1]
			r.waiters[waiter.MessageType] = waiters[:len(waiters)-1]
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

// OutgoingMessages is the shared channel for sending commands to the game server.
// Any package can send messages by pushing []byte or string onto this channel.
var OutgoingMessages = make(chan interface{}, 100)

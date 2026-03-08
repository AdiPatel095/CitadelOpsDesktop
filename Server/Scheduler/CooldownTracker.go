package Scheduler

import (
	"sync"
	"time"
)

// CooldownTracker manages cooldowns and in-flight statuses for targets
type CooldownTracker struct {
	mu        sync.RWMutex
	cooldowns map[string]time.Time
}

// NewCooldownTracker creates a new CooldownTracker
func NewCooldownTracker() *CooldownTracker {
	return &CooldownTracker{
		cooldowns: make(map[string]time.Time),
	}
}

// IsOnCooldown checks if a target is currently on cooldown or in-flight
func (ct *CooldownTracker) IsOnCooldown(targetID string) bool {
	ct.mu.RLock()
	defer ct.mu.RUnlock()

	expiry, exists := ct.cooldowns[targetID]
	if !exists {
		return false
	}
	return time.Now().Before(expiry)
}

// SetCooldown sets a cooldown for a target for a specific duration
func (ct *CooldownTracker) SetCooldown(targetID string, duration time.Duration) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	ct.cooldowns[targetID] = time.Now().Add(duration)
}

// ClearCooldown removes a target from cooldown
func (ct *CooldownTracker) ClearCooldown(targetID string) {
	ct.mu.Lock()
	defer ct.mu.Unlock()

	delete(ct.cooldowns, targetID)
}

// SetInFlight sets a target as in-flight (basically a long cooldown until confirmed)
func (ct *CooldownTracker) SetInFlight(targetID string) {
	// Set a 30 second timeout for in-flight requests as a safety net.
	// This will be overwritten by a real travel-time cooldown when the confirmation arrives.
	ct.SetCooldown(targetID, 30*time.Second)
}

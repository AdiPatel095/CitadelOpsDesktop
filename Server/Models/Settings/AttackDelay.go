package settings

import (
	"time"
)

const (
	minAttackDelayFloorSec        = 4.0
	maidenCommsDefaultMinDelaySec = 4.0
	maidenCommsDefaultMaxDelaySec = 5.0
)

// schedulerAttackDelaysPersisted is true after SchedulerSettings.json was loaded or saved.
var schedulerAttackDelaysPersisted bool

// SchedulerAttackDelaysPersisted reports whether attack delay bounds came from Settings view persistence.
func SchedulerAttackDelaysPersisted() bool {
	return schedulerAttackDelaysPersisted
}

// MarkSchedulerAttackDelaysPersisted records that user/settings-file attack delays are active.
func MarkSchedulerAttackDelaysPersisted() {
	schedulerAttackDelaysPersisted = true
}

// RandomAttackDelayRange returns a random duration between minSec and maxSec (same jitter as the scheduler).
func RandomAttackDelayRange(minSec, maxSec float64) time.Duration {
	if minSec < minAttackDelayFloorSec {
		minSec = minAttackDelayFloorSec
	}
	if maxSec < minSec {
		maxSec = minSec
	}
	delayRangeMs := int((maxSec - minSec) * 1000)
	if delayRangeMs <= 0 {
		return time.Duration(minSec * float64(time.Second))
	}
	delayMs := int(minSec*1000) + (time.Now().Nanosecond() % delayRangeMs)
	return time.Duration(delayMs) * time.Millisecond
}

// RandomAttackDelay returns a random duration between SettingsState min/max attack delays.
func RandomAttackDelay(s *SettingsState) time.Duration {
	minSec := maidenCommsDefaultMinDelaySec
	maxSec := maidenCommsDefaultMaxDelaySec
	if s != nil {
		if s.MinAttackDelay > 0 {
			minSec = s.MinAttackDelay
		}
		if s.MaxAttackDelay > 0 {
			maxSec = s.MaxAttackDelay
		}
	}
	return RandomAttackDelayRange(minSec, maxSec)
}

// AttackDelayBetweenSends picks the delay between consecutive outbound attacks.
// Uses Settings view min/max when SchedulerSettings.json exists; otherwise 4–5s.
func AttackDelayBetweenSends() time.Duration {
	if schedulerAttackDelaysPersisted {
		return RandomAttackDelay(GetSettingsState())
	}
	return RandomAttackDelayRange(maidenCommsDefaultMinDelaySec, maidenCommsDefaultMaxDelaySec)
}

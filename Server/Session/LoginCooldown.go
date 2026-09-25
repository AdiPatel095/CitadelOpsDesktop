package Session

import (
	"encoding/json"
	"time"
)

const loginCooldownSafetyMargin = 2 * time.Second

// Valid server cooldowns replace the configured displacement delay. Unknown
// cooldowns retain a bounded fallback and never turn zero relog delay into a loop.
func loginCooldownDeadlines(payload []byte, observedAt time.Time, fallback time.Duration) (*time.Time, time.Time) {
	var response struct {
		Seconds *int64 `json:"CD"`
	}
	if json.Unmarshal(payload, &response) == nil && response.Seconds != nil && *response.Seconds >= 0 &&
		*response.Seconds <= int64((time.Duration(1<<63-1)-loginCooldownSafetyMargin)/time.Second) {
		until := observedAt.Add(time.Duration(*response.Seconds) * time.Second)
		return &until, until.Add(loginCooldownSafetyMargin)
	}
	if fallback < loginCooldownSafetyMargin {
		fallback = loginCooldownSafetyMargin
	}
	return nil, observedAt.Add(fallback)
}

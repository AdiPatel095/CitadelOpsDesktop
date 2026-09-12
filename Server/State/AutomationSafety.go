package State

import (
	"fmt"
	"strings"
	"time"
)

// Stored with automations in account snapshots and hosted checkpoints. Value
// fields keep copy-on-write snapshots isolated without additional cloning.
type AutomationSafetyLock struct {
	Lane        string    `json:"lane,omitempty"`
	Opcode      string    `json:"opcode,omitempty"`
	Code        int       `json:"code,omitempty"`
	OperationID string    `json:"operationId,omitempty"`
	Intent      string    `json:"intent,omitempty"`
	ObservedAt  time.Time `json:"observedAt,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	Until       time.Time `json:"until,omitempty"`
	ClearedAt   time.Time `json:"clearedAt,omitempty"`
	Review      string    `json:"review,omitempty"`
	ReviewedBy  string    `json:"reviewedBy,omitempty"`
}

func (lock AutomationSafetyLock) Active(now time.Time) bool {
	return lock.OperationID != "" && lock.ClearedAt.IsZero() && (lock.Until.IsZero() || now.Before(lock.Until))
}

func (lock AutomationSafetyLock) Detail() string {
	detail := fmt.Sprintf("Safety lock after %s %d (operation %s).", strings.ToUpper(lock.Opcode), lock.Code, lock.OperationID)
	if !lock.Until.IsZero() {
		return detail + " Paused until " + lock.Until.UTC().Format(time.RFC3339) + "."
	}
	return detail + " Review this rejection before explicitly clearing the lane lock."
}

package State

import (
	"CitadelDesktop/Server/Localization"
	"fmt"
	"strings"
	"time"
)

const AutomationSafetyLockDuration = 30 * time.Minute

// Shared by admission, scheduling and persisted-lock refresh. A known message
// alone is not an exception: only these exact opcode/code pairs bypass locking.
func AutomationRejectionWhitelisted(opcode string, code int) bool {
	switch strings.ToLower(strings.TrimSpace(opcode)) {
	case "adi":
		return code == 95
	case "ere", "eqe":
		return code == 227
	case "bup":
		return code == 87
	case "ahr":
		return code == 273
	default:
		return false
	}
}

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
	until := lock.ExpiresAt()
	return lock.OperationID != "" && lock.ClearedAt.IsZero() && !AutomationRejectionWhitelisted(lock.Opcode, lock.Code) && (until.IsZero() || now.Before(until))
}

// Legacy indefinite locks use the original rejection time, never restart time.
// Malformed timestamp-free records stay closed until the durable startup refresh.
func (lock AutomationSafetyLock) ExpiresAt() time.Time {
	if !lock.ObservedAt.IsZero() {
		return lock.ObservedAt.Add(AutomationSafetyLockDuration)
	}
	return lock.Until
}

func (lock AutomationSafetyLock) Detail() string {
	detail := fmt.Sprintf("Safety lock after %s %d (operation %s).", strings.ToUpper(lock.Opcode), lock.Code, lock.OperationID)
	if until := lock.ExpiresAt(); !until.IsZero() {
		return detail + " Paused until " + until.UTC().Format(time.RFC3339) + "."
	}
	return detail + " Review this rejection before explicitly clearing the lane lock."
}

// DetailDescriptor mirrors Detail using a producer-owned template. Protocol
// identifiers and the UTC deadline remain exact evidence values.
func (lock AutomationSafetyLock) DetailDescriptor() *Localization.Message {
	params := Localization.Params{"opcode": strings.ToUpper(lock.Opcode), "code": lock.Code, "operation": lock.OperationID}
	if until := lock.ExpiresAt(); !until.IsZero() {
		params["until"] = until.UTC().Format(time.RFC3339)
		return Localization.Bind(Localization.New("server.state.safety_lock.until", "Safety lock after {opcode} {code} (operation {operation}). Paused until {until}.", params), lock.Detail())
	}
	return Localization.Bind(Localization.New("server.state.safety_lock.review", "Safety lock after {opcode} {code} (operation {operation}). Review this rejection before explicitly clearing the lane lock.", params), lock.Detail())
}

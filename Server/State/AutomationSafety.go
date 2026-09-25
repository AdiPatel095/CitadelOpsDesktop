package State

import (
	"CitadelDesktop/Server/Localization"
	"fmt"
	"strconv"
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
// descriptors are cloned with automation snapshots.
type AutomationSafetyLock struct {
	MeaningDescriptor *Localization.Message `json:"meaningDescriptor,omitempty"`
	ContextDescriptor *Localization.Message `json:"contextDescriptor,omitempty"`
	Meaning           string                `json:"meaning,omitempty"`
	MeaningSource     string                `json:"meaningSource,omitempty"`
	Context           string                `json:"context,omitempty"`
	Lane              string                `json:"lane,omitempty"`
	Opcode            string                `json:"opcode,omitempty"`
	Code              int                   `json:"code,omitempty"`
	OperationID       string                `json:"operationId,omitempty"`
	Intent            string                `json:"intent,omitempty"`
	ObservedAt        time.Time             `json:"observedAt,omitempty"`
	Reason            string                `json:"reason,omitempty"`
	Until             time.Time             `json:"until,omitempty"`
	ClearedAt         time.Time             `json:"clearedAt,omitempty"`
	Review            string                `json:"review,omitempty"`
	ReviewedBy        string                `json:"reviewedBy,omitempty"`
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
	detail := fmt.Sprintf("Safety lock after %s %d", strings.ToUpper(lock.Opcode), lock.Code)
	if lock.Context != "" {
		detail += ": " + lock.Context
	} else if lock.Meaning != "" {
		detail += ": " + lock.Meaning
	}
	detail = strings.TrimRight(detail, ".") + "."
	if until := lock.ExpiresAt(); !until.IsZero() {
		return detail + " Paused until " + until.UTC().Format(time.RFC3339) + "."
	}
	return detail + " Review this rejection before explicitly clearing the lane lock."
}

// DetailDescriptor mirrors Detail using a producer-owned template. Protocol
// identifiers and the UTC deadline remain exact evidence values.
func (lock AutomationSafetyLock) DetailDescriptor() *Localization.Message {
	params := Localization.Params{"opcode": strings.ToUpper(lock.Opcode), "code": strconv.Itoa(lock.Code)}
	raw := lock.Detail()
	reason, descriptor := lock.Meaning, lock.MeaningDescriptor
	if lock.Context != "" {
		reason, descriptor = lock.Context, lock.ContextDescriptor
	}
	if reason != "" && (descriptor == nil || descriptor.FallbackText != reason || descriptor.Context != nil || descriptor.ListParams != nil) {
		return nil
	}
	var message *Localization.Message
	if until := lock.ExpiresAt(); !until.IsZero() {
		params["until"] = until.UTC().Format(time.RFC3339)
		if reason != "" {
			message = Localization.New("server.state.safety_lock.reason_until", "Safety lock after {opcode} {code}: {reason} Paused until {until}.", params)
		} else {
			message = Localization.New("server.state.safety_lock.until", "Safety lock after {opcode} {code}. Paused until {until}.", params)
		}
	} else {
		if reason != "" {
			message = Localization.New("server.state.safety_lock.reason_review", "Safety lock after {opcode} {code}: {reason} Review this rejection before explicitly clearing the lane lock.", params)
		} else {
			message = Localization.New("server.state.safety_lock.review", "Safety lock after {opcode} {code}. Review this rejection before explicitly clearing the lane lock.", params)
		}
	}
	if reason != "" {
		return Localization.WithLists(message, raw, map[string][]*Localization.Message{"reason": {descriptor}})
	}
	return Localization.Bind(message, raw)
}

func (lock AutomationSafetyLock) Clone() AutomationSafetyLock {
	lock.MeaningDescriptor = Localization.Clone(lock.MeaningDescriptor)
	lock.ContextDescriptor = Localization.Clone(lock.ContextDescriptor)
	return lock
}

package Automation

import (
	"context"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type Snapshot struct {
	State                        State.GameState
	Configuration                Configuration.Snapshot
	GameData                     *GameData.Store
	Now                          time.Time
	PolicyConfigurationChanged   bool
	ConfigurationExternallyOwned bool
}

// OperationalCursorUpdate advances runtime-owned workflow progress only after
// the preceding game action succeeds. It deliberately does not mutate the
// user's desired configuration document.
type OperationalCursorUpdate struct {
	Key   string
	Value int
}

type Decision struct {
	Status      string
	Detail      string
	NextCheckAt time.Time
	// EventDriven leaves a passive decision asleep when NextCheckAt is zero.
	// Only a declared state/configuration/session wake will reevaluate it. This
	// is used for authoritative blockers whose state cannot change merely
	// because another polling interval elapsed.
	EventDriven       bool
	Metrics           map[string]float64
	Request           *Intent.Request
	FollowUp          *Intent.Request
	OperationalCursor *OperationalCursorUpdate
	// FailureFallback runs only when Request reaches a terminal failed,
	// partially-succeeded, or indeterminate status. Cancellation never triggers it.
	FailureFallback *Intent.Request
	// FailureFallbackIndeterminateOnly limits the fallback to an uncertain write outcome.
	FailureFallbackIndeterminateOnly bool
	FailureDetail                    string
	// ScheduleKey identifies an additional decision-specific schedule, such as
	// a per-castle production window, that must remain open while the request waits.
	ScheduleKey string
	// ReevaluateOnSuccess continues a response-gated workflow immediately
	// after both the request and its optional follow-up have succeeded.
	ReevaluateOnSuccess bool
	// ReevaluateOnStale immediately asks the policy for a different action when
	// an operation safely stops before dispatch because its plan became stale.
	ReevaluateOnStale bool
}

type Policy interface {
	ID() string
	EnabledKey() string
	Evaluate(context.Context, Snapshot) (Decision, error)
}

// CorePolicy participates in the coordinator's session-readiness, intent,
// retry, and Bot Lock safeguards without being controlled by a feature toggle
// or weekly feature schedule.
type CorePolicy interface {
	CorePolicy()
}

// OnDemandPolicy is enabled by persisted runtime state instead of an
// automation.enabled switch. It is intended for explicit, bounded user runs
// that must survive restarts and wait for authoritative game state changes.
type OnDemandPolicy interface {
	Active(State.GameState) bool
}

// ActorIDPolicy lets an independent policy lane attribute its operations to a
// parent feature. This keeps shared workflow ownership and activity routing
// stable while the lane retains its own coordinator runtime and status.
type ActorIDPolicy interface {
	ActorID() string
}

// ScheduleKeyPolicy lets related policy lanes share one configured schedule
// while retaining independent coordinator runtimes.
type ScheduleKeyPolicy interface {
	ScheduleKey() string
}

// StateWakePolicy declares authoritative state domains that can make an idle
// policy actionable before its scheduled check. Successful operation chains
// use Decision.ReevaluateOnSuccess instead, so response events cannot override
// an intentionally paced decision.
type StateWakePolicy interface {
	WakeDomains() []string
}

// UrgentWakePolicy declares the subset of WakeDomains whose changes open a
// short game-side opportunity. The coordinator evaluates those events on
// arrival instead of holding them in the shared coalescing window, so a
// time-critical trigger is not paid for by unrelated state churn. Every
// declared domain must also appear in WakeDomains.
type UrgentWakePolicy interface {
	UrgentWakeDomains() []string
}

// ConfigurationWakePolicy declares the configuration sections that affect a
// policy. The coordinator separately fingerprints each policy's own enable bit
// and top-level or per-castle schedules.
type ConfigurationWakePolicy interface {
	WakeSections() []string
}

// ConfigurationDerivedStatePolicy declares configuration sections whose
// values are materialized into cached state. The coordinator invalidates that
// state after in-flight work has stopped and before the policy is reevaluated.
// ResetConfigurationDerivedState must not modify GameState.Map.
type ConfigurationDerivedStatePolicy interface {
	ConfigurationDerivedStateSections() []string
	ConfigurationDerivedStateComponents() State.ComponentSet
	ResetConfigurationDerivedState(*State.GameState) (domains []string, changed bool)
}

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type IntentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}

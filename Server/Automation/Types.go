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
	State                      State.GameState
	Configuration              Configuration.Snapshot
	GameData                   *GameData.Store
	Now                        time.Time
	PolicyConfigurationChanged bool
}

type Decision struct {
	Status      string
	Detail      string
	NextCheckAt time.Time
	Metrics     map[string]float64
	Request     *Intent.Request
	FollowUp    *Intent.Request
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

// StateWakePolicy declares authoritative state domains that can make an idle
// policy actionable before its scheduled check. Successful operation chains
// use Decision.ReevaluateOnSuccess instead, so response events cannot override
// an intentionally paced decision.
type StateWakePolicy interface {
	WakeDomains() []string
}

// ConfigurationWakePolicy declares the configuration sections that affect a
// policy. The coordinator separately fingerprints each policy's own enable bit
// and top-level or per-castle schedules.
type ConfigurationWakePolicy interface {
	WakeSections() []string
}

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type IntentSubmitter interface {
	Submit(context.Context, Intent.Request) Intent.Receipt
}

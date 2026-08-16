package Session

import (
	"context"
	"errors"
	"strings"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

var (
	ErrTransportUnavailable           = errors.New("game transport is unavailable")
	ErrFrontendInteractionUnavailable = errors.New("game frontend interaction is unavailable")
)

type ConnectionMode string

const (
	ConnectionModeFull       ConnectionMode = "full"
	ConnectionModeBackground ConnectionMode = "background"
)

func ParseConnectionMode(value string) ConnectionMode {
	if ConnectionMode(strings.ToLower(strings.TrimSpace(value))) == ConnectionModeBackground {
		return ConnectionModeBackground
	}
	return ConnectionModeFull
}

type Status struct {
	Mode                 ConnectionMode `json:"mode"`
	State                string         `json:"state"`
	LoggedIn             bool           `json:"loggedIn"`
	SocketReady          bool           `json:"socketReady"`
	ConnectionGeneration uint64         `json:"connectionGeneration,omitempty"`
	BrowserID            string         `json:"browserId,omitempty"`
	BrowserName          string         `json:"browserName,omitempty"`
	ServerURL            string         `json:"serverUrl,omitempty"`
	Namespace            string         `json:"namespace,omitempty"`
	Detail               string         `json:"detail,omitempty"`
	CooldownUntil        *time.Time     `json:"cooldownUntil,omitempty"`
	RetryAt              *time.Time     `json:"retryAt,omitempty"`
	// LoginFailure carries the structured outcome of a failed game login for
	// cooldown, error, and reconnecting statuses; other statuses leave it nil.
	LoginFailure *State.LoginFailure `json:"loginFailure,omitempty"`
	ChangedAt    time.Time           `json:"changedAt"`
}

type RawFrame struct {
	Payload              string
	Direction            Protocol.Direction
	ObservedAt           time.Time
	ConnectionGeneration uint64
	ResponseToken        string
	CausationOperationID string
}

type Transport interface {
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	Send(ctx context.Context, payload []byte) error
	Frames() <-chan RawFrame
	StatusChanges() <-chan Status
	Status() Status
}

type BrowserSelector interface {
	SelectBrowser(preference string) error
}

type BrowserInventoryProvider interface {
	BrowserInventory() BrowserInventory
}

type RelogDelayTransport interface {
	SetRelogDelayProvider(func() time.Duration)
}

// RunningTransport is implemented by transports whose connection loop can stop
// on its own (for example after a login failure that retrying cannot fix). The
// controller uses it to restart such a transport on the next Start instead of
// treating the parked session as already started.
type RunningTransport interface {
	Running() bool
}

// ReconnectPolicy decides who owns the wait after a game disconnect.
type ReconnectPolicy string

const (
	// ReconnectPolicyHold keeps the runtime and its session loop alive and
	// reconnects on its own after the relog delay, cooldown, or suspension.
	ReconnectPolicyHold ReconnectPolicy = "hold"
	// ReconnectPolicyRelease makes the runtime's lifetime follow the game
	// connection: after a short immediate retry window the transport reports
	// state "released" with the earliest retry time and stops, so the control
	// plane can drain the runtime, free its slot, and create a fresh runtime
	// when the wait elapses (or when the user forces a reconnect).
	ReconnectPolicyRelease ReconnectPolicy = "release"
)

func ParseReconnectPolicy(raw string) (ReconnectPolicy, bool) {
	switch ReconnectPolicy(strings.ToLower(strings.TrimSpace(raw))) {
	case "", ReconnectPolicyHold:
		return ReconnectPolicyHold, true
	case ReconnectPolicyRelease:
		return ReconnectPolicyRelease, true
	default:
		return "", false
	}
}

// ReconnectPolicyTransport is implemented by transports that can switch
// between holding and releasing the session after a disconnect.
type ReconnectPolicyTransport interface {
	SetReconnectPolicy(ReconnectPolicy)
}

// ReconnectHoldTransport is implemented by transports that refuse an automatic
// Start while a park or scheduled retry is in effect; ClearReconnectHold lets
// an explicit user or control-plane reconnect bypass that wait once.
type ReconnectHoldTransport interface {
	ClearReconnectHold()
}

type FrontendInteractionTransport interface {
	CloseGameUI(ctx context.Context) error
}

type ResponseCorrelationTransport interface {
	CorrelatesResponses() bool
}

// OutboundCausationTransport guarantees that outbound RawFrame values carry
// CausationOperationID for sends initiated by CitadelOps. An empty causation
// value can therefore be treated as direct browser traffic.
type OutboundCausationTransport interface {
	ReportsOutboundCausation() bool
}

type UnavailableTransport struct {
	frames   chan RawFrame
	statuses chan Status
	status   Status
}

func NewUnavailableTransport() *UnavailableTransport {
	return &UnavailableTransport{
		frames:   make(chan RawFrame),
		statuses: make(chan Status),
		status: Status{
			Mode: ConnectionModeFull, State: "unavailable", Namespace: "EmpireEx_21",
			Detail: "No game transport adapter is configured", ChangedAt: time.Now().UTC(),
		},
	}
}

func (transport *UnavailableTransport) Start(context.Context) error {
	return ErrTransportUnavailable
}

func (transport *UnavailableTransport) Stop(context.Context) error {
	return nil
}

func (transport *UnavailableTransport) Send(context.Context, []byte) error {
	return ErrTransportUnavailable
}

func (transport *UnavailableTransport) Frames() <-chan RawFrame {
	return transport.frames
}

func (transport *UnavailableTransport) StatusChanges() <-chan Status {
	return transport.statuses
}

func (transport *UnavailableTransport) Status() Status {
	return transport.status
}

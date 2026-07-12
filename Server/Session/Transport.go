package Session

import (
	"context"
	"errors"
	"time"

	"CitadelDesktop/Server/Protocol"
)

var ErrTransportUnavailable = errors.New("game transport is unavailable")

type Status struct {
	State         string     `json:"state"`
	LoggedIn      bool       `json:"loggedIn"`
	SocketReady   bool       `json:"socketReady"`
	BrowserID     string     `json:"browserId,omitempty"`
	BrowserName   string     `json:"browserName,omitempty"`
	ServerURL     string     `json:"serverUrl,omitempty"`
	Namespace     string     `json:"namespace,omitempty"`
	Detail        string     `json:"detail,omitempty"`
	CooldownUntil *time.Time `json:"cooldownUntil,omitempty"`
	RetryAt       *time.Time `json:"retryAt,omitempty"`
	ChangedAt     time.Time  `json:"changedAt"`
}

type RawFrame struct {
	Payload    string
	Direction  Protocol.Direction
	ObservedAt time.Time
}

type Activity struct {
	Kind       string
	ObservedAt time.Time
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

type ActivitySource interface {
	Activities() <-chan Activity
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
			State: "unavailable", Namespace: "EmpireEx_21",
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

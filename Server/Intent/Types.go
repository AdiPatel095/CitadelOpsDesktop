package Intent

import (
	"context"
	"encoding/json"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type Effect string

const (
	EffectRead     Effect = "read"
	EffectWrite    Effect = "write"
	EffectLaunch   Effect = "launch"
	EffectExternal Effect = "external"
)

type Request struct {
	ID               string          `json:"id,omitempty"`
	Name             string          `json:"name"`
	Actor            string          `json:"actor,omitempty"`
	Arguments        json.RawMessage `json:"arguments,omitempty"`
	ExpectedRevision *uint64         `json:"expectedRevision,omitempty"`
	DryRun           bool            `json:"dryRun,omitempty"`
}

type PlanningContext struct {
	State    State.GameState
	GameData *GameData.Store
}

type Step struct {
	Name            string           `json:"name,omitempty"`
	Action          string           `json:"action,omitempty"`
	ActionArguments json.RawMessage  `json:"arguments,omitempty"`
	Opcode          string           `json:"opcode,omitempty"`
	Payload         json.RawMessage  `json:"payload,omitempty"`
	AwaitOpcode     string           `json:"awaitOpcode,omitempty"`
	AwaitOpcodes    []string         `json:"awaitOpcodes,omitempty"`
	TimeoutMillis   int              `json:"timeoutMillis,omitempty"`
	DelayMillis     int              `json:"delayMillis,omitempty"`
	SuccessCodes    []int            `json:"successCodes,omitempty"`
	Command         Protocol.Command `json:"-"`
}

type Plan struct {
	Intent        string   `json:"intent"`
	Effect        Effect   `json:"effect"`
	StateRevision uint64   `json:"stateRevision"`
	Claims        []string `json:"claims,omitempty"`
	Steps         []Step   `json:"steps"`
	Summary       string   `json:"summary,omitempty"`
}

type Planner func(ctx context.Context, input PlanningContext, arguments json.RawMessage) (Plan, error)

type Definition struct {
	Name        string  `json:"name"`
	Description string  `json:"description"`
	Effect      Effect  `json:"effect"`
	Planner     Planner `json:"-"`
}

type Status string

const (
	StatusPlanning  Status = "planning"
	StatusPlanned   Status = "planned"
	StatusRunning   Status = "running"
	StatusSucceeded Status = "succeeded"
	StatusCancelled Status = "cancelled"
	StatusFailed    Status = "failed"
)

type Receipt struct {
	ID          string     `json:"id"`
	Intent      string     `json:"intent"`
	Actor       string     `json:"actor"`
	Status      Status     `json:"status"`
	Plan        *Plan      `json:"plan,omitempty"`
	Error       string     `json:"error,omitempty"`
	SubmittedAt time.Time  `json:"submittedAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

type StateReader interface {
	Snapshot() State.GameState
}

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type Sender interface {
	Ready() bool
	Namespace() string
	Send(ctx context.Context, payload []byte) error
}

type Observer interface {
	Watch(opcode string, afterRevision uint64) (<-chan Protocol.CommittedFrame, func())
}

type Action func(ctx context.Context, arguments json.RawMessage) error

// ExecutionGate can delay or reject a mutating plan immediately before it
// acquires claims and before each step is executed.
type ExecutionGate func(ctx context.Context, request Request, plan Plan) error

package Intent

import (
	"context"
	"encoding/json"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Outbound"
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
	ID               string            `json:"id,omitempty"`
	Name             string            `json:"name"`
	Actor            string            `json:"actor,omitempty"`
	Priority         Outbound.Priority `json:"priority,omitempty"`
	Arguments        json.RawMessage   `json:"arguments,omitempty"`
	ExpectedRevision *uint64           `json:"expectedRevision,omitempty"`
	DryRun           bool              `json:"dryRun,omitempty"`
}

type PlanningContext struct {
	State           State.GameState
	GameData        *GameData.Store
	Language        *GameData.LanguageStore
	Partitions      State.PartitionVersions
	ProtocolContext State.ProtocolContextState
}

func (input PlanningContext) Dependencies(keys ...State.PartitionKey) []State.PartitionDependency {
	return input.Partitions.Dependencies(keys...)
}

type ResumePolicy string

const (
	ResumeOnce    ResumePolicy = "once"
	ResumeRebuild ResumePolicy = "rebuild"
)

type ResponseBarrier string

const (
	// ResponseBarrierWire advances as soon as an inbound response is decoded.
	// ResponseBarrierCommitted waits until that response has updated GameState.
	ResponseBarrierWire              ResponseBarrier = "wire"
	ResponseBarrierWireThenCommitted ResponseBarrier = "wire-then-committed"
	ResponseBarrierCommitted         ResponseBarrier = "committed"
)

type AdmissionClass string

const AdmissionAttackLaunch AdmissionClass = "attack-launch"

// Admission describes work that must wait for a shared execution slot before
// acquiring its ordinary intent claims. Module is a stable product-level key
// used for user preference and telemetry; it does not replace the intent name.
type Admission struct {
	Class     AdmissionClass `json:"class"`
	Module    string         `json:"module"`
	Affinity  string         `json:"affinity,omitempty"`
	NotBefore time.Time      `json:"notBefore,omitempty"`
	Deadline  time.Time      `json:"deadline,omitempty"`
}

type Step struct {
	Name                    string                    `json:"name,omitempty"`
	Action                  string                    `json:"action,omitempty"`
	ActionArguments         json.RawMessage           `json:"arguments,omitempty"`
	Resolver                string                    `json:"resolver,omitempty"`
	ResolverArguments       json.RawMessage           `json:"resolverArguments,omitempty"`
	Opcode                  string                    `json:"opcode,omitempty"`
	Payload                 json.RawMessage           `json:"payload,omitempty"`
	AwaitOpcode             string                    `json:"awaitOpcode,omitempty"`
	AwaitOpcodes            []string                  `json:"awaitOpcodes,omitempty"`
	TimeoutMillis           int                       `json:"timeoutMillis,omitempty"`
	DelayMillis             int                       `json:"delayMillis,omitempty"`
	SuccessCodes            []int                     `json:"successCodes,omitempty"`
	StaleCodes              []int                     `json:"staleCodes,omitempty"`
	CaptureResponse         bool                      `json:"captureResponse,omitempty"`
	ExpectedResponsePayload json.RawMessage           `json:"expectedResponsePayload,omitempty"`
	ResponseBarrier         ResponseBarrier           `json:"responseBarrier,omitempty"`
	ResumePolicy            ResumePolicy              `json:"resumePolicy,omitempty"`
	CommandDependencies     *CommandDependencyRequest `json:"commandDependencies,omitempty"`
	Command                 Protocol.Command          `json:"-"`
}

// CommandDependencyRequest declares the concrete opcode and route payload for
// dependencies that must run before a deferred command resolver. Static
// commands use their own opcode and payload automatically.
type CommandDependencyRequest struct {
	Opcode  string          `json:"opcode"`
	Payload json.RawMessage `json:"payload"`
}

func RebuildOnResume(step Step) Step {
	step.ResumePolicy = ResumeRebuild
	return step
}

type Plan struct {
	Intent         string                      `json:"intent"`
	Effect         Effect                      `json:"effect"`
	StateRevision  uint64                      `json:"stateRevision"`
	CatalogVersion string                      `json:"catalogVersion,omitempty"`
	Dependencies   []State.PartitionDependency `json:"dependencies,omitempty"`
	Claims         []string                    `json:"claims,omitempty"`
	Resources      []ResourceKey               `json:"resources,omitempty"`
	Steps          []Step                      `json:"steps"`
	Summary        string                      `json:"summary,omitempty"`
	Admission      *Admission                  `json:"admission,omitempty"`
}

type Planner func(ctx context.Context, input PlanningContext, arguments json.RawMessage) (Plan, error)

type ReadSetResolver func(
	input PlanningContext,
	arguments json.RawMessage,
	plan Plan,
) ([]State.PartitionKey, error)

type AttackModuleDefinition struct {
	ID            string `json:"id"`
	Label         string `json:"label"`
	Description   string `json:"description,omitempty"`
	DefaultWeight int    `json:"defaultWeight"`
}

type Definition struct {
	Name             string                  `json:"name"`
	Description      string                  `json:"description"`
	Effect           Effect                  `json:"effect"`
	ArgumentsExample json.RawMessage         `json:"argumentsExample,omitempty"`
	AttackModule     *AttackModuleDefinition `json:"attackModule,omitempty"`
	Planner          Planner                 `json:"-"`
	ReadSet          ReadSetResolver         `json:"-"`
	RequireResources bool                    `json:"-"`
}

type Status string

type EffectPhase string

const (
	StatusPlanning           Status = "planning"
	StatusPlanned            Status = "planned"
	StatusQueued             Status = "queued"
	StatusRunning            Status = "running"
	StatusPaused             Status = "paused"
	StatusSucceeded          Status = "succeeded"
	StatusCancelled          Status = "cancelled"
	StatusFailed             Status = "failed"
	StatusPartiallySucceeded Status = "partially_succeeded"
	StatusIndeterminate      Status = "indeterminate"
	StatusReconciling        Status = "reconciling"
)

const (
	EffectPhaseAccepted               EffectPhase = "accepted"
	EffectPhasePlanned                EffectPhase = "planned"
	EffectPhaseDispatching            EffectPhase = "dispatching"
	EffectPhaseSent                   EffectPhase = "sent"
	EffectPhaseAwaitingResponse       EffectPhase = "awaiting_response"
	EffectPhaseObserved               EffectPhase = "observed"
	EffectPhaseReconciliationRequired EffectPhase = "reconciliation_required"
	EffectPhaseCompleted              EffectPhase = "completed"
)

type Receipt struct {
	StreamSequence uint64            `json:"streamSequence,omitempty"`
	StreamGap      bool              `json:"streamGap,omitempty"`
	ID             string            `json:"id"`
	Intent         string            `json:"intent"`
	Actor          string            `json:"actor"`
	Priority       Outbound.Priority `json:"priority"`
	Status         Status            `json:"status"`
	Phase          EffectPhase       `json:"phase,omitempty"`
	Attempt        int               `json:"attempt,omitempty"`
	Plan           *Plan             `json:"plan,omitempty"`
	Exchanges      []CommandExchange `json:"exchanges,omitempty"`
	Error          string            `json:"error,omitempty"`
	// RawError preserves the machine-oriented wording for in-process recovery
	// and gating logic. It is never serialized or shown to users.
	RawError    string     `json:"-"`
	SubmittedAt time.Time  `json:"submittedAt"`
	StartedAt   *time.Time `json:"startedAt,omitempty"`
	CompletedAt *time.Time `json:"completedAt,omitempty"`
}

// Terminal reports whether the operation has finished. Every completion path
// (succeeded, failed, cancelled, partially succeeded, indeterminate, and a
// planned dry run) stamps CompletedAt; paused and reconciling checkpoints clear
// it because the operation may still resume.
func (receipt Receipt) Terminal() bool {
	return receipt.CompletedAt != nil
}

func (receipt Receipt) DiagnosticError() string {
	if receipt.RawError != "" {
		return receipt.RawError
	}
	return receipt.Error
}

// CommandExchange is the exact encoded command input and matching decoded
// reply for steps that explicitly opt in to response capture.
type CommandExchange struct {
	Step     string           `json:"step,omitempty"`
	Command  Protocol.Command `json:"command"`
	Response *Protocol.Frame  `json:"response,omitempty"`
}

type StateReader interface {
	// ReadOnlyView returns an immutable generation. Intent planning is read-only
	// and must not pay for a defensive clone on every plan/revalidation.
	ReadOnlyView() State.GameState
	Revision() uint64
	Session() State.SessionState
}

type GameDataProvider interface {
	Current() (*GameData.Store, bool)
}

type Sender interface {
	Ready() bool
	Namespace() string
	Send(ctx context.Context, payload []byte) error
}

type LaneAvailability interface {
	NextAllowed(lane Outbound.Lane) time.Time
}

type ConnectionGenerationProvider interface {
	ConnectionGeneration() uint64
}

type ResponseCorrelationProvider interface {
	CorrelatesResponses() bool
}

type Observer interface {
	Watch(opcode string, afterRevision uint64) (<-chan Protocol.CommittedFrame, func())
}

type CorrelatedObserver interface {
	WatchResponse(opcode string, afterRevision uint64, responseToken string) (<-chan Protocol.CommittedFrame, func())
}

type WireObserver interface {
	WatchWire(opcode string) (<-chan Protocol.CommittedFrame, func())
}

type CorrelatedWireObserver interface {
	WatchWireResponse(opcode string, responseToken string) (<-chan Protocol.CommittedFrame, func())
}

type CommitObserver interface {
	WaitCommitted(ctx context.Context, ingressID uint64) (Protocol.CommittedFrame, error)
	ForgetCommitted(ingressID uint64)
}

type Action func(ctx context.Context, arguments json.RawMessage) error

type StepResolver func(ctx context.Context, input PlanningContext, arguments json.RawMessage) (Step, error)

type CommandDependencyPlan struct {
	Key   string
	Steps []Step
}

// CommandDependencyResolver returns commands and guards that must succeed
// immediately before a concrete opcode can be sent. Key identifies the route
// so a deferred resolver cannot change the command after its dependencies run.
type CommandDependencyResolver func(ctx context.Context, input PlanningContext, step Step) (CommandDependencyPlan, error)

type AdmissionWeightProvider func(request Request, admission Admission) int

type ExecutionPoint string

const (
	ExecutionBeforeClaims ExecutionPoint = "before-claims"
	ExecutionBeforeStep   ExecutionPoint = "before-step"
)

// ExecutionGate can delay or reject a plan before it acquires claims and at
// each safe boundary before a step is executed.
type ExecutionGate func(ctx context.Context, request Request, plan Plan, point ExecutionPoint) error

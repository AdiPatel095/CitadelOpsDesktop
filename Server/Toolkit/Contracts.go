package Toolkit

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

const ContractVersion = 1

// Effect describes the strongest side effect an invocation can have. Policies
// authorize the resolved effect before a handler is allowed to run.
type Effect string

const (
	EffectRead        Effect = "read"
	EffectControl     Effect = "control"
	EffectGameQuery   Effect = "game_query"
	EffectGameAction  Effect = "game_action"
	EffectDestructive Effect = "destructive"
	EffectExternal    Effect = "external_communication"
)

// ToolDefinition is deliberately transport-neutral. It maps directly to the
// common name/description/input-schema shape used by LLM tool APIs and MCP.
type ToolDefinition struct {
	Name            string          `json:"name"`
	Description     string          `json:"description"`
	InputSchema     json.RawMessage `json:"inputSchema"`
	Effect          Effect          `json:"effect"`
	PossibleEffects []Effect        `json:"possibleEffects,omitempty"`
	Tags            []string        `json:"tags,omitempty"`
}

// Call is one platform-neutral tool invocation. Arguments must be a JSON object.
type Call struct {
	ID        string          `json:"id,omitempty"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// CallError is safe to serialize across a process or platform boundary.
type CallError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result contains a stable JSON snapshot of the handler result.
type Result struct {
	ID         string          `json:"id,omitempty"`
	Name       string          `json:"name"`
	Effect     Effect          `json:"effect,omitempty"`
	OK         bool            `json:"ok"`
	Content    json.RawMessage `json:"content,omitempty"`
	Error      *CallError      `json:"error,omitempty"`
	DurationMS int64           `json:"durationMs"`
}

// Manifest is the versioned discovery document a future transport can expose.
type Manifest struct {
	ContractVersion int              `json:"contractVersion"`
	Toolkit         string           `json:"toolkit"`
	Tools           []ToolDefinition `json:"tools"`
}

// Authorization is passed to an Authorizer after dynamic effect resolution.
type Authorization struct {
	Tool      ToolDefinition
	Effect    Effect
	Arguments json.RawMessage
}

// Authorizer is the platform seam for approvals, user scopes, audit policy, or
// per-call confirmation. The built-in default permits read-only calls.
type Authorizer interface {
	Authorize(context.Context, Authorization) error
}

// Observer receives completed invocations for audit logging or telemetry. It
// does not participate in authorization and cannot alter the returned result.
type Observer interface {
	Observe(context.Context, Call, Result)
}

type ObserverFunc func(context.Context, Call, Result)

func (f ObserverFunc) Observe(ctx context.Context, call Call, result Result) {
	f(ctx, call, result)
}

// AuthorizerFunc adapts a function into an Authorizer.
type AuthorizerFunc func(context.Context, Authorization) error

func (f AuthorizerFunc) Authorize(ctx context.Context, request Authorization) error {
	return f(ctx, request)
}

// EffectPolicy authorizes only the explicitly listed effects.
type EffectPolicy map[Effect]bool

func (p EffectPolicy) Authorize(_ context.Context, request Authorization) error {
	if p[request.Effect] {
		return nil
	}
	return fmt.Errorf("effect %q is not authorized", request.Effect)
}

// ReadOnlyPolicy is the safe default for an LLM-facing harness.
func ReadOnlyPolicy() EffectPolicy {
	return EffectPolicy{EffectRead: true}
}

// AllowEffects builds an allow-list policy for an embedding application.
func AllowEffects(effects ...Effect) EffectPolicy {
	policy := make(EffectPolicy, len(effects))
	for _, effect := range effects {
		policy[effect] = true
	}
	return policy
}

// ToolError lets handlers return a stable error code without coupling callers
// to Go error types.
type ToolError struct {
	Code    string
	Message string
}

func (e *ToolError) Error() string {
	return e.Message
}

func toolError(code, format string, args ...interface{}) error {
	return &ToolError{Code: code, Message: fmt.Sprintf(format, args...)}
}

func callError(err error, fallbackCode string) *CallError {
	var typed *ToolError
	if errors.As(err, &typed) {
		return &CallError{Code: typed.Code, Message: typed.Message}
	}
	return &CallError{Code: fallbackCode, Message: err.Error()}
}

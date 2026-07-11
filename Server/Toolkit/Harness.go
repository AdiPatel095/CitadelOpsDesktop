package Toolkit

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
)

type Handler func(context.Context, json.RawMessage) (interface{}, error)

type EffectResolver func(json.RawMessage) (Effect, error)

// Tool combines portable metadata with its in-process implementation.
type Tool struct {
	Definition    ToolDefinition
	Handler       Handler
	ResolveEffect EffectResolver
}

// Option customizes a Harness without tying it to a transport or LLM vendor.
type Option func(*Harness)

// WithAuthorizer installs the policy consulted before every invocation.
func WithAuthorizer(authorizer Authorizer) Option {
	return func(h *Harness) {
		if authorizer != nil {
			h.authorizer = authorizer
		}
	}
}

// WithAllowedEffects is a convenience option for trusted embedding processes.
func WithAllowedEffects(effects ...Effect) Option {
	return WithAuthorizer(AllowEffects(effects...))
}

// WithObserver installs a post-invocation audit/telemetry hook.
func WithObserver(observer Observer) Option {
	return func(h *Harness) {
		h.observer = observer
	}
}

// Harness owns a discoverable registry and executes JSON tool calls.
type Harness struct {
	mu         sync.RWMutex
	tools      map[string]Tool
	authorizer Authorizer
	observer   Observer
}

func New(options ...Option) *Harness {
	h := &Harness{
		tools:      make(map[string]Tool),
		authorizer: ReadOnlyPolicy(),
	}
	for _, option := range options {
		if option != nil {
			option(h)
		}
	}
	return h
}

// Register adds an extension tool. Names are unique and schemas must be valid
// JSON objects so discovery failures happen during startup, not mid-call.
func (h *Harness) Register(tool Tool) error {
	name := strings.TrimSpace(tool.Definition.Name)
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("invalid tool name %q", tool.Definition.Name)
	}
	if tool.Handler == nil {
		return fmt.Errorf("tool %q has no handler", name)
	}
	if tool.Definition.Effect == "" {
		return fmt.Errorf("tool %q has no effect classification", name)
	}
	if len(tool.Definition.InputSchema) == 0 || !json.Valid(tool.Definition.InputSchema) {
		return fmt.Errorf("tool %q has an invalid input schema", name)
	}
	var schema interface{}
	if err := json.Unmarshal(tool.Definition.InputSchema, &schema); err != nil {
		return fmt.Errorf("tool %q input schema: %w", name, err)
	}
	if _, ok := schema.(map[string]interface{}); !ok {
		return fmt.Errorf("tool %q input schema must be an object", name)
	}

	tool.Definition.Name = name
	tool.Definition = cloneToolDefinition(tool.Definition)
	h.mu.Lock()
	defer h.mu.Unlock()
	if _, exists := h.tools[name]; exists {
		return fmt.Errorf("tool %q is already registered", name)
	}
	h.tools[name] = tool
	return nil
}

// Definitions returns a deterministic copy suitable for an LLM tool manifest.
func (h *Harness) Definitions() []ToolDefinition {
	h.mu.RLock()
	definitions := make([]ToolDefinition, 0, len(h.tools))
	for _, tool := range h.tools {
		definitions = append(definitions, cloneToolDefinition(tool.Definition))
	}
	h.mu.RUnlock()
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	return definitions
}

// Manifest returns a versioned discovery document for a transport adapter.
func (h *Harness) Manifest() Manifest {
	return Manifest{
		ContractVersion: ContractVersion,
		Toolkit:         "citadel_ops",
		Tools:           h.Definitions(),
	}
}

// Execute resolves authorization and captures the handler output as immutable JSON.
func (h *Harness) Execute(ctx context.Context, call Call) (result Result) {
	started := time.Now()
	result.ID = call.ID
	result.Name = call.Name
	defer func() {
		result.DurationMS = time.Since(started).Milliseconds()
		if recovered := recover(); recovered != nil {
			result.OK = false
			result.Content = nil
			result.Error = &CallError{Code: "handler_panic", Message: fmt.Sprintf("tool handler panicked: %v", recovered)}
		}
		h.observe(ctx, call, result)
	}()

	h.mu.RLock()
	tool, exists := h.tools[call.Name]
	h.mu.RUnlock()
	if !exists {
		result.Error = &CallError{Code: "unknown_tool", Message: fmt.Sprintf("unknown tool %q", call.Name)}
		return result
	}

	arguments := bytes.TrimSpace(call.Arguments)
	if len(arguments) == 0 || bytes.Equal(arguments, []byte("null")) {
		arguments = json.RawMessage("{}")
	}
	if !json.Valid(arguments) || arguments[0] != '{' {
		result.Error = &CallError{Code: "invalid_arguments", Message: "arguments must be a JSON object"}
		return result
	}

	effect := tool.Definition.Effect
	if tool.ResolveEffect != nil {
		resolved, err := tool.ResolveEffect(arguments)
		if err != nil {
			result.Error = callError(err, "invalid_arguments")
			return result
		}
		effect = resolved
	}
	result.Effect = effect

	if err := ctx.Err(); err != nil {
		result.Error = &CallError{Code: "cancelled", Message: err.Error()}
		return result
	}
	if err := h.authorizer.Authorize(ctx, Authorization{
		Tool:      cloneToolDefinition(tool.Definition),
		Effect:    effect,
		Arguments: append(json.RawMessage(nil), arguments...),
	}); err != nil {
		result.Error = &CallError{Code: "unauthorized", Message: err.Error()}
		return result
	}

	content, err := tool.Handler(ctx, arguments)
	if err != nil {
		result.Error = callError(err, "execution_failed")
		return result
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		result.Error = &CallError{Code: "serialization_failed", Message: err.Error()}
		return result
	}
	result.Content = encoded
	result.OK = true
	return result
}

func (h *Harness) observe(ctx context.Context, call Call, result Result) {
	if h.observer == nil {
		return
	}
	defer func() {
		_ = recover()
	}()
	observedCall := call
	observedCall.Arguments = append(json.RawMessage(nil), call.Arguments...)
	observedResult := result
	observedResult.Content = append(json.RawMessage(nil), result.Content...)
	if result.Error != nil {
		errorCopy := *result.Error
		observedResult.Error = &errorCopy
	}
	h.observer.Observe(ctx, observedCall, observedResult)
}

func cloneToolDefinition(definition ToolDefinition) ToolDefinition {
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	definition.PossibleEffects = append([]Effect(nil), definition.PossibleEffects...)
	definition.Tags = append([]string(nil), definition.Tags...)
	return definition
}

// ExecuteJSON is a minimal boundary for stdio, HTTP, WebSocket, or IPC adapters.
func (h *Harness) ExecuteJSON(ctx context.Context, data []byte) ([]byte, error) {
	var call Call
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&call); err != nil {
		return nil, err
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("unexpected JSON after call")
		}
		return nil, err
	}
	return json.Marshal(h.Execute(ctx, call))
}

func decodeStrict[T any](raw json.RawMessage) (T, error) {
	var value T
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return value, toolError("invalid_arguments", "invalid arguments: %v", err)
	}
	var trailing interface{}
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return value, toolError("invalid_arguments", "unexpected JSON after arguments")
		}
		return value, toolError("invalid_arguments", "invalid trailing JSON: %v", err)
	}
	return value, nil
}

func mustSchema(value interface{}) json.RawMessage {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return data
}

func objectSchema(properties map[string]interface{}, required ...string) json.RawMessage {
	schema := map[string]interface{}{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return mustSchema(schema)
}

func schemaProperty(kind, description string) map[string]interface{} {
	return map[string]interface{}{"type": kind, "description": description}
}

func enumProperty(description string, values ...string) map[string]interface{} {
	return map[string]interface{}{"type": "string", "description": description, "enum": values}
}

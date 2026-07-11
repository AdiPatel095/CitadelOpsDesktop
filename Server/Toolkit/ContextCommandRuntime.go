package Toolkit

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"CitadelDesktop/Server/Automation"
)

type ContextCommandInvocation struct {
	Name           string                    `json:"name"`
	Arguments      json.RawMessage           `json:"arguments"`
	ExpectedPlanID string                    `json:"expectedPlanId,omitempty"`
	Surface        string                    `json:"surface,omitempty"`
	Options        Automation.CommandOptions `json:"-"`
}

func (r *CommandRuntime) ContextCatalog(name string, effect Effect) ([]ContextCommandDefinition, error) {
	name = strings.TrimSpace(name)
	definitions := make([]ContextCommandDefinition, 0, len(r.contextSpecs))
	for specName, spec := range r.contextSpecs {
		if name != "" && name != specName {
			continue
		}
		if effect != "" && effect != spec.definition.Effect {
			continue
		}
		definition := spec.definition
		definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
		definition.Resolves = append([]string(nil), definition.Resolves...)
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	if name != "" && len(definitions) == 0 {
		return nil, toolError("not_found", "context command %q is not registered", name)
	}
	return definitions, nil
}

func (r *CommandRuntime) ContextDefinition(name string) (ContextCommandDefinition, error) {
	spec, ok := r.contextSpecs[strings.TrimSpace(name)]
	if !ok {
		return ContextCommandDefinition{}, toolError("not_found", "context command %q is not registered", name)
	}
	definition := spec.definition
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	definition.Resolves = append([]string(nil), definition.Resolves...)
	return definition, nil
}

func (r *CommandRuntime) PlanContext(ctx context.Context, invocation ContextCommandInvocation) (ContextCommandPlan, error) {
	input, spec, err := r.contextInvocation(invocation)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	return resolveContextCommandPlan(ctx, spec, input.Arguments, r.specs)
}

// ExecuteContext is the trusted in-process API; expose it to untrusted callers through the
// authorized Toolkit Harness rather than calling it directly from a transport handler.
func (r *CommandRuntime) ExecuteContext(ctx context.Context, invocation ContextCommandInvocation) (ContextCommandExecution, error) {
	input, spec, err := r.contextInvocation(invocation)
	if err != nil {
		return ContextCommandExecution{}, err
	}
	options := invocation.Options
	if options.Owner == "" {
		options.Owner = Automation.OwnerToolkit
	}
	if options.Surface == "" {
		options.Surface = strings.TrimSpace(invocation.Surface)
	}
	if options.Surface == "" {
		options.Surface = Automation.CommandSurfaceRuntime
	}
	return executeContextCommand(ctx, input, spec, r, options)
}

func (r *CommandRuntime) contextInvocation(invocation ContextCommandInvocation) (contextCommandCallInput, contextCommandSpec, error) {
	input := contextCommandCallInput{
		Name:           strings.TrimSpace(invocation.Name),
		Arguments:      json.RawMessage(strings.TrimSpace(string(invocation.Arguments))),
		ExpectedPlanID: strings.TrimSpace(invocation.ExpectedPlanID),
	}
	spec, ok := r.contextSpecs[input.Name]
	if !ok {
		return input, contextCommandSpec{}, toolError("not_found", "context command %q is not registered", input.Name)
	}
	if len(input.Arguments) == 0 || !json.Valid(input.Arguments) || input.Arguments[0] != '{' {
		return input, spec, toolError("invalid_arguments", "context command arguments must be a JSON object")
	}
	return input, spec, nil
}

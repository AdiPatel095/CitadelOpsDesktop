package Toolkit

import (
	"context"
	"encoding/json"
	"sort"
	"strings"

	"CitadelDesktop/Server/Automation"
)

// CommandInvocation is the direct Go API corresponding to citadel.command.send.
// A desktop feature, companion app, or another transport can use it without encoding a tool call.
type CommandInvocation struct {
	Name      string                    `json:"name"`
	Arguments json.RawMessage           `json:"arguments"`
	Intent    string                    `json:"intent,omitempty"`
	Surface   string                    `json:"surface,omitempty"`
	Options   Automation.CommandOptions `json:"-"`
}

type BuiltCommand struct {
	Command  CommandDefinition `json:"command"`
	Payloads []string          `json:"payloads"`
}

type CommandDispatchResult struct {
	Automation.CommandReceipt
	Opcode       string `json:"opcode"`
	Effect       Effect `json:"effect"`
	PayloadCount int    `json:"payloadCount"`
}

// CommandRuntime owns the structured command definitions/builders and delegates admission through
// a CommandDispatcher. Toolkit tools are adapters over this same reusable runtime.
type CommandRuntime struct {
	specs        map[string]commandSpec
	contextSpecs map[string]contextCommandSpec
	dispatcher   Automation.CommandDispatcher
}

func NewCommandRuntime(dispatcher Automation.CommandDispatcher) (*CommandRuntime, error) {
	specs, err := newCommandSpecs()
	if err != nil {
		return nil, err
	}
	if dispatcher == nil {
		dispatcher = Automation.OutboundCommandHarness
	}
	contextSpecs, err := newContextCommandSpecs(specs)
	if err != nil {
		return nil, err
	}
	return &CommandRuntime{specs: specs, contextSpecs: contextSpecs, dispatcher: dispatcher}, nil
}

func (r *CommandRuntime) Catalog(name string, effect Effect) ([]CommandDefinition, error) {
	name = strings.TrimSpace(name)
	definitions := make([]CommandDefinition, 0, len(r.specs))
	for specName, spec := range r.specs {
		if name != "" && name != specName {
			continue
		}
		if effect != "" && effect != spec.definition.Effect {
			continue
		}
		definition := spec.definition
		definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
		definitions = append(definitions, definition)
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].Name < definitions[j].Name })
	if name != "" && len(definitions) == 0 {
		return nil, toolError("not_found", "command %q is not registered", name)
	}
	return definitions, nil
}

func (r *CommandRuntime) Definition(name string) (CommandDefinition, error) {
	spec, ok := r.specs[strings.TrimSpace(name)]
	if !ok {
		return CommandDefinition{}, toolError("not_found", "command %q is not registered", name)
	}
	definition := spec.definition
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	return definition, nil
}

func (r *CommandRuntime) Build(name string, arguments json.RawMessage) (BuiltCommand, error) {
	spec, payloads, err := buildNamedCommand(r.specs, name, arguments)
	if err != nil {
		return BuiltCommand{}, err
	}
	definition := spec.definition
	definition.InputSchema = append(json.RawMessage(nil), definition.InputSchema...)
	return BuiltCommand{
		Command:  definition,
		Payloads: append([]string(nil), payloads...),
	}, nil
}

// Dispatch is the trusted in-process API. Untrusted callers should invoke the same runtime through
// NewDefaultHarnessWithRuntime so effect authorization runs before this method is reached.
func (r *CommandRuntime) Dispatch(ctx context.Context, invocation CommandInvocation) (CommandDispatchResult, error) {
	built, err := r.Build(invocation.Name, invocation.Arguments)
	if err != nil {
		return CommandDispatchResult{}, err
	}
	intent := strings.TrimSpace(invocation.Intent)
	if intent == "" {
		intent = strings.TrimSpace(invocation.Options.Intent)
	}
	if intent == "" {
		intent = "direct_command"
	}
	surface := strings.TrimSpace(invocation.Surface)
	if surface == "" {
		surface = strings.TrimSpace(invocation.Options.Surface)
	}
	if surface == "" {
		surface = Automation.CommandSurfaceRuntime
	}
	invocation.Options.Surface = surface
	invocation.Options.Effect = string(built.Command.Effect)
	receipt := r.dispatchPayloads(ctx, built.Command.Name, intent, built.Payloads, invocation.Options)
	return CommandDispatchResult{
		CommandReceipt: receipt,
		Opcode:         built.Command.Opcode,
		Effect:         built.Command.Effect,
		PayloadCount:   len(built.Payloads),
	}, nil
}

func (r *CommandRuntime) dispatchPayloads(
	ctx context.Context,
	command string,
	intent string,
	payloads []string,
	options Automation.CommandOptions,
) Automation.CommandReceipt {
	effect := ""
	if spec, ok := r.specs[command]; ok {
		effect = string(spec.definition.Effect)
	}
	if options.Effect == "" {
		options.Effect = effect
	}
	if options.Surface == "" {
		options.Surface = Automation.CommandSurfaceRuntime
	}
	frames := make([]Automation.CommandFrame, len(payloads))
	for index, payload := range payloads {
		frames[index] = Automation.CommandFrame{Payload: []byte(payload)}
	}
	return r.dispatcher.Dispatch(ctx, Automation.CommandSubmission{
		ContractVersion: Automation.CommandContractVersion,
		Command:         command,
		Intent:          intent,
		Surface:         options.Surface,
		Effect:          options.Effect,
		Frames:          frames,
		Options:         options,
	})
}

func buildNamedCommand(
	specs map[string]commandSpec,
	name string,
	arguments json.RawMessage,
) (commandSpec, []string, error) {
	name = strings.TrimSpace(name)
	spec, ok := specs[name]
	if !ok {
		return commandSpec{}, nil, toolError("not_found", "command %q is not registered", name)
	}
	arguments = json.RawMessage(strings.TrimSpace(string(arguments)))
	if len(arguments) == 0 || string(arguments) == "null" {
		arguments = json.RawMessage("{}")
	}
	if !json.Valid(arguments) || arguments[0] != '{' {
		return spec, nil, toolError("invalid_arguments", "command arguments must be a JSON object")
	}
	payloads, err := spec.build(arguments)
	if err != nil {
		return spec, nil, err
	}
	if len(payloads) == 0 {
		return spec, nil, toolError("command_build_failed", "command %q produced no payload", name)
	}
	return spec, payloads, nil
}

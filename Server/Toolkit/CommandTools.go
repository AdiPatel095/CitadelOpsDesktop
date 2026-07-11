package Toolkit

import (
	"context"
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/ResponseRegistry"
)

// CommandDefinition describes one structured adapter over GameCommands payload builders.
type CommandDefinition struct {
	Name        string          `json:"name"`
	Opcode      string          `json:"opcode"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Effect      Effect          `json:"effect"`
}

type commandSpec struct {
	definition CommandDefinition
	build      func(json.RawMessage) ([]string, error)
}

type commandCatalogInput struct {
	Name   string `json:"name,omitempty"`
	Effect Effect `json:"effect,omitempty"`
}

type commandCallInput struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

type commandPreview struct {
	Command   CommandDefinition `json:"command"`
	Payloads  []string          `json:"payloads"`
	Connected bool              `json:"connected"`
}

type stateObservationCursor struct {
	StateKey     string `json:"stateKey"`
	AfterVersion uint64 `json:"afterVersion"`
}

func registerCommandTools(harness *Harness, runtime *CommandRuntime) error {
	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.command.catalog",
			Description: "Discover structured game commands and their argument schemas. Filter by exact command name or effect when possible.",
			InputSchema: objectSchema(map[string]interface{}{
				"name":   schemaProperty("string", "Optional exact command name."),
				"effect": enumProperty("Optional effect filter.", string(EffectGameQuery), string(EffectGameAction), string(EffectDestructive), string(EffectExternal)),
			}),
			Effect: EffectRead,
			Tags:   []string{"command", "discovery"},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[commandCatalogInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			return runtime.Catalog(input.Name, input.Effect)
		},
	}); err != nil {
		return err
	}

	callSchema := objectSchema(map[string]interface{}{
		"name":      schemaProperty("string", "Exact name from citadel.command.catalog."),
		"arguments": map[string]interface{}{"type": "object", "description": "Arguments matching that command's inputSchema."},
	}, "name", "arguments")

	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.command.preview",
			Description: "Validate structured command arguments and build the exact wire payload without sending it.",
			InputSchema: callSchema,
			Effect:      EffectRead,
			Tags:        []string{"command", "preview"},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[commandCallInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			built, buildErr := runtime.Build(input.Name, input.Arguments)
			if buildErr != nil {
				return nil, buildErr
			}
			return commandPreview{
				Command:   built.Command,
				Payloads:  built.Payloads,
				Connected: ResponseRegistry.IsGameWebSocketReady(),
			}, nil
		},
	}); err != nil {
		return err
	}

	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:            "citadel.command.send",
			Description:     "Validate, build, and enqueue one structured game command. Authorization uses the selected command's resolved effect.",
			InputSchema:     callSchema,
			Effect:          EffectGameAction,
			PossibleEffects: []Effect{EffectGameQuery, EffectGameAction, EffectDestructive, EffectExternal},
			Tags:            []string{"command", "send"},
		},
		ResolveEffect: func(raw json.RawMessage) (Effect, error) {
			input, decodeErr := decodeStrict[commandCallInput](raw)
			if decodeErr != nil {
				return "", decodeErr
			}
			definition, definitionErr := runtime.Definition(input.Name)
			if definitionErr != nil {
				return "", definitionErr
			}
			return definition.Effect, nil
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[commandCallInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			if !ResponseRegistry.IsGameWebSocketReady() {
				return nil, toolError("game_disconnected", "the game websocket is not ready")
			}
			result, dispatchErr := runtime.Dispatch(ctx, CommandInvocation{
				Name:      input.Name,
				Arguments: input.Arguments,
				Intent:    "direct_command",
				Surface:   Automation.CommandSurfaceToolkit,
				Options: Automation.CommandOptions{
					Owner:    Automation.OwnerToolkit,
					Priority: Automation.DefaultPriority(Automation.OwnerToolkit),
				},
			})
			if dispatchErr != nil {
				return nil, dispatchErr
			}
			if !result.Accepted {
				code := result.Code
				if code == "" {
					code = "queue_rejected"
				}
				return nil, toolError(code, "%s", result.Message)
			}
			return result, nil
		},
	})
}

func commandObservationCursors(payloads []string) []stateObservationCursor {
	seen := make(map[string]bool)
	cursors := make([]stateObservationCursor, 0, len(payloads))
	for _, payload := range payloads {
		opcode := payloadOpcode(payload)
		if opcode == "" {
			continue
		}
		stateKey := Automation.StateOpcode(opcode)
		if seen[stateKey] {
			continue
		}
		seen[stateKey] = true
		cursors = append(cursors, stateObservationCursor{
			StateKey:     stateKey,
			AfterVersion: Automation.StateSnapshot(stateKey).Version,
		})
	}
	return cursors
}

func payloadOpcode(payload string) string {
	parts := strings.Split(payload, "%")
	if len(parts) > 3 && strings.HasPrefix(parts[2], "EmpireEx_") {
		return strings.ToLower(strings.TrimSpace(parts[3]))
	}
	if len(parts) > 2 {
		return strings.ToLower(strings.TrimSpace(parts[2]))
	}
	return ""
}

type commandSpecBuilder struct {
	specs map[string]commandSpec
}

func newCommandSpecs() (map[string]commandSpec, error) {
	builder := &commandSpecBuilder{specs: make(map[string]commandSpec)}
	registrars := []func(*commandSpecBuilder) error{
		registerNavigationAndStateCommands,
		registerArmyAndMovementCommands,
		registerEquipmentAndTCICommands,
		registerReportCommands,
		registerCraftingAndTransportCommands,
		registerCapturedOpcodeCommands,
	}
	for _, register := range registrars {
		if err := register(builder); err != nil {
			return nil, err
		}
	}
	return builder.specs, nil
}

func (b *commandSpecBuilder) add(name, opcode, description string, effect Effect, schema json.RawMessage, build func(json.RawMessage) ([]string, error)) error {
	if name == "" || opcode == "" || build == nil || !json.Valid(schema) {
		return toolError("invalid_command_spec", "invalid command spec %q", name)
	}
	if _, exists := b.specs[name]; exists {
		return toolError("invalid_command_spec", "duplicate command spec %q", name)
	}
	b.specs[name] = commandSpec{
		definition: CommandDefinition{
			Name:        name,
			Opcode:      opcode,
			Description: description,
			InputSchema: schema,
			Effect:      effect,
		},
		build: build,
	}
	return nil
}

func onePayload(payload string) []string {
	return []string{payload}
}

func noArgumentBuilder(build func() string) func(json.RawMessage) ([]string, error) {
	return func(raw json.RawMessage) ([]string, error) {
		if _, err := decodeStrict[struct{}](raw); err != nil {
			return nil, err
		}
		return onePayload(build()), nil
	}
}

func positive(value int, field string) error {
	if value <= 0 {
		return toolError("invalid_arguments", "%s must be positive", field)
	}
	return nil
}

func positive64(value int64, field string) error {
	if value <= 0 {
		return toolError("invalid_arguments", "%s must be positive", field)
	}
	return nil
}

func nonNegative(value int, field string) error {
	if value < 0 {
		return toolError("invalid_arguments", "%s must not be negative", field)
	}
	return nil
}

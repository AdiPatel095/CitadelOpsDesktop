package Toolkit

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"sync/atomic"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/ResponseRegistry"
)

// ContextCommandDefinition describes a high-level command whose protocol fields
// are resolved from live state, catalogs, and safe defaults by the harness.
type ContextCommandDefinition struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Effect      Effect          `json:"effect"`
	Resolves    []string        `json:"resolves"`
}

// ContextResolution explains where one planned value came from.
type ContextResolution struct {
	Field  string      `json:"field"`
	Value  interface{} `json:"value"`
	Source string      `json:"source"`
	Detail string      `json:"detail,omitempty"`
}

type ContextCastleReference struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	CastleID  int    `json:"castleId"`
	Name      string `json:"name"`
	KingdomID int    `json:"kingdomId"`
	MapX      int    `json:"mapX"`
	MapY      int    `json:"mapY"`
}

type ContextPrimitiveCommand struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ContextCommandStep struct {
	Kind        string                   `json:"kind"`
	Description string                   `json:"description"`
	Castle      *ContextCastleReference  `json:"castle,omitempty"`
	Command     *ContextPrimitiveCommand `json:"command,omitempty"`
}

type ContextCommandPlan struct {
	PlanID        string                           `json:"planId"`
	Command       string                           `json:"command"`
	Effect        Effect                           `json:"effect"`
	Resolutions   []ContextResolution              `json:"resolutions"`
	Steps         []ContextCommandStep             `json:"steps"`
	Claims        []Automation.Claim               `json:"claims"`
	StateVersions map[string]Automation.StateStamp `json:"stateVersions"`
	Warnings      []string                         `json:"warnings,omitempty"`
	stateKeys     []string
}

type ContextCommandExecution struct {
	WorkID       string                      `json:"workId"`
	Dispatched   bool                        `json:"dispatched"`
	Plan         ContextCommandPlan          `json:"plan"`
	Receipts     []Automation.CommandReceipt `json:"receipts"`
	Observations []stateObservationCursor    `json:"observations"`
}

type contextCommandSpec struct {
	definition ContextCommandDefinition
	resolve    func(context.Context, json.RawMessage) (ContextCommandPlan, error)
}

type contextCommandCatalogInput struct {
	Name   string `json:"name,omitempty"`
	Effect Effect `json:"effect,omitempty"`
}

type contextCommandCallInput struct {
	Name           string          `json:"name"`
	Arguments      json.RawMessage `json:"arguments"`
	ExpectedPlanID string          `json:"expectedPlanId,omitempty"`
}

var contextWorkSequence atomic.Uint64

func registerContextCommandTools(harness *Harness, runtime *CommandRuntime) error {
	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.context_command.catalog",
			Description: "Discover high-level commands that accept semantic inputs and resolve protocol fields from Citadel's live context.",
			InputSchema: objectSchema(map[string]interface{}{
				"name":   schemaProperty("string", "Optional exact contextual command name."),
				"effect": enumProperty("Optional effect filter.", string(EffectGameQuery), string(EffectGameAction), string(EffectDestructive)),
			}),
			Effect: EffectRead,
			Tags:   []string{"context-command", "discovery"},
		},
		Handler: func(_ context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[contextCommandCatalogInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			return runtime.ContextCatalog(input.Name, input.Effect)
		},
	}); err != nil {
		return err
	}

	callSchema := objectSchema(map[string]interface{}{
		"name":      schemaProperty("string", "Exact name from citadel.context_command.catalog."),
		"arguments": map[string]interface{}{"type": "object", "description": "Minimal semantic arguments matching the contextual command schema."},
	}, "name", "arguments")
	if err := harness.Register(Tool{
		Definition: ToolDefinition{
			Name:        "citadel.context_command.plan",
			Description: "Resolve a high-level command against current state without sending anything. Returns provenance, claims, warnings, and underlying commands.",
			InputSchema: callSchema,
			Effect:      EffectRead,
			Tags:        []string{"context-command", "plan"},
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[contextCommandCallInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			return runtime.PlanContext(ctx, ContextCommandInvocation{
				Name:           input.Name,
				Arguments:      input.Arguments,
				ExpectedPlanID: input.ExpectedPlanID,
			})
		},
	}); err != nil {
		return err
	}

	executeSchema := objectSchema(map[string]interface{}{
		"name":           schemaProperty("string", "Exact name from citadel.context_command.catalog."),
		"arguments":      map[string]interface{}{"type": "object", "description": "Minimal semantic arguments matching the contextual command schema."},
		"expectedPlanId": schemaProperty("string", "Optional planId from a prior plan call; execution stops if context now resolves differently."),
	}, "name", "arguments")
	return harness.Register(Tool{
		Definition: ToolDefinition{
			Name:            "citadel.context_command.execute",
			Description:     "Resolve and execute a high-level command through the automation work broker. Authorization uses the selected command's resolved effect.",
			InputSchema:     executeSchema,
			Effect:          EffectGameAction,
			PossibleEffects: []Effect{EffectGameQuery, EffectGameAction, EffectDestructive},
			Tags:            []string{"context-command", "execute"},
		},
		ResolveEffect: func(raw json.RawMessage) (Effect, error) {
			input, decodeErr := decodeStrict[contextCommandCallInput](raw)
			if decodeErr != nil {
				return "", decodeErr
			}
			_, spec, invocationErr := runtime.contextInvocation(ContextCommandInvocation{
				Name:           input.Name,
				Arguments:      input.Arguments,
				ExpectedPlanID: input.ExpectedPlanID,
			})
			if invocationErr != nil {
				return "", invocationErr
			}
			return spec.definition.Effect, nil
		},
		Handler: func(ctx context.Context, raw json.RawMessage) (interface{}, error) {
			input, decodeErr := decodeStrict[contextCommandCallInput](raw)
			if decodeErr != nil {
				return nil, decodeErr
			}
			return runtime.ExecuteContext(ctx, ContextCommandInvocation{
				Name:           input.Name,
				Arguments:      input.Arguments,
				ExpectedPlanID: input.ExpectedPlanID,
				Surface:        Automation.CommandSurfaceToolkit,
				Options:        Automation.CommandOptions{Owner: Automation.OwnerToolkit},
			})
		},
	})
}

func resolveContextCommandPlan(ctx context.Context, spec contextCommandSpec, arguments json.RawMessage, commandSpecs map[string]commandSpec) (ContextCommandPlan, error) {
	plan, err := spec.resolve(ctx, arguments)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	plan.Command = spec.definition.Name
	plan.Effect = spec.definition.Effect
	if len(plan.Claims) == 0 {
		return ContextCommandPlan{}, toolError("plan_invalid", "context command %q declared no automation claims", plan.Command)
	}
	if len(plan.Steps) == 0 {
		return ContextCommandPlan{}, toolError("plan_invalid", "context command %q declared no execution steps", plan.Command)
	}
	focusSeen := false
	for index, step := range plan.Steps {
		switch step.Kind {
		case "focus_castle":
			if step.Castle == nil || step.Castle.CastleID <= 0 {
				return ContextCommandPlan{}, toolError("plan_invalid", "context command %q has an invalid focus step", plan.Command)
			}
			if effectRank(EffectGameQuery) > effectRank(plan.Effect) {
				return ContextCommandPlan{}, toolError("plan_invalid", "context command %q cannot focus a castle with effect %q", plan.Command, plan.Effect)
			}
			if focusSeen || index != 0 {
				return ContextCommandPlan{}, toolError("plan_invalid", "context command %q must declare at most one focus step and it must be first", plan.Command)
			}
			focusSeen = true
		case "command":
			if step.Command == nil {
				return ContextCommandPlan{}, toolError("plan_invalid", "context command %q has an empty command step", plan.Command)
			}
			primitive, ok := commandSpecs[step.Command.Name]
			if !ok {
				return ContextCommandPlan{}, toolError("plan_invalid", "underlying command %q is not registered", step.Command.Name)
			}
			if effectRank(primitive.definition.Effect) > effectRank(plan.Effect) {
				return ContextCommandPlan{}, toolError("plan_invalid", "underlying command %q has stronger effect %q than plan %q", step.Command.Name, primitive.definition.Effect, plan.Effect)
			}
			if _, buildErr := primitive.build(step.Command.Arguments); buildErr != nil {
				return ContextCommandPlan{}, buildErr
			}
		default:
			return ContextCommandPlan{}, toolError("plan_invalid", "unsupported contextual step kind %q", step.Kind)
		}
	}
	plan.StateVersions = make(map[string]Automation.StateStamp, len(plan.stateKeys))
	for _, key := range plan.stateKeys {
		plan.StateVersions[key] = Automation.StateSnapshot(key)
	}
	plan.PlanID = contextPlanID(plan)
	return plan, nil
}

func executeContextCommand(
	ctx context.Context,
	input contextCommandCallInput,
	spec contextCommandSpec,
	runtime *CommandRuntime,
	options Automation.CommandOptions,
) (ContextCommandExecution, error) {
	commandSpecs := runtime.specs
	if !ResponseRegistry.IsGameWebSocketReady() {
		return ContextCommandExecution{}, toolError("game_disconnected", "the game websocket is not ready")
	}
	initialPlan, err := resolveContextCommandPlan(ctx, spec, input.Arguments, commandSpecs)
	if err != nil {
		return ContextCommandExecution{}, err
	}
	if input.ExpectedPlanID != "" && input.ExpectedPlanID != initialPlan.PlanID {
		return ContextCommandExecution{}, toolError("plan_changed", "expected plan %q but current context resolves to %q", input.ExpectedPlanID, initialPlan.PlanID)
	}

	owner := options.Owner
	if owner == "" {
		owner = Automation.OwnerToolkit
	}
	priority := options.Priority
	if priority == 0 {
		priority = Automation.DefaultPriority(owner)
	}
	workID := fmt.Sprintf("command:%d:%d", time.Now().UnixMilli(), contextWorkSequence.Add(1))
	actualPlan := initialPlan
	observations := make([]stateObservationCursor, 0)
	receipts := make([]Automation.CommandReceipt, 0)
	err = Automation.RunWork(ctx, Automation.WorkItem{
		ID: workID,
		Request: Automation.Request{
			Owner:    owner,
			Priority: priority,
			Reason:   "context command: " + input.Name,
			Claims:   append([]Automation.Claim(nil), initialPlan.Claims...),
			MaxHold:  45 * time.Second,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			resolved, resolveErr := resolveContextCommandPlan(workCtx, spec, input.Arguments, commandSpecs)
			if resolveErr != nil {
				return resolveErr
			}
			if input.ExpectedPlanID != "" && input.ExpectedPlanID != resolved.PlanID {
				return toolError("plan_changed", "context changed while waiting for claims: expected %q, resolved %q", input.ExpectedPlanID, resolved.PlanID)
			}
			actualPlan = resolved
			steps := actualPlan.Steps
			focusCompleted := false
			for index := 0; index < len(steps); index++ {
				if err := workCtx.Err(); err != nil {
					return err
				}
				step := steps[index]
				switch step.Kind {
				case "focus_castle":
					if focusCompleted {
						continue
					}
					castle := step.Castle
					focusPayload := GameCommands.CastleFocusCommand(castle.KingdomID, castle.CastleID, castle.MapX, castle.MapY)
					observations = mergeObservationCursors(observations, observationCursorsForStateKeys(Automation.StateFocus, Automation.StateCastles))
					observations = mergeObservationCursors(observations, commandObservationCursors([]string{focusPayload}))
					queueFocus := func(payload string) bool {
						commandName := payloadOpcode(payload)
						receipt := runtime.dispatchPayloads(workCtx, commandName, input.Name, []string{payload}, Automation.CommandOptions{
							WorkID:   lease.WorkID(),
							Owner:    owner,
							Surface:  options.Surface,
							Priority: lease.Priority(),
							Guard:    lease.Active,
						})
						if receipt.Accepted {
							receipts = append(receipts, receipt)
						}
						return receipt.Accepted
					}
					if GameParser.FetchCastleTroopsWithLeaseQueue(lease, castle.KingdomID, castle.CastleID, castle.MapX, castle.MapY, queueFocus) == nil {
						if !lease.Active() {
							return context.Canceled
						}
						return toolError("focus_failed", "could not refresh and focus castle %d", castle.CastleID)
					}
					focusCompleted = true
					refreshed, refreshErr := resolveContextCommandPlan(workCtx, spec, input.Arguments, commandSpecs)
					if refreshErr != nil {
						return refreshErr
					}
					if input.ExpectedPlanID != "" && input.ExpectedPlanID != refreshed.PlanID {
						return toolError("plan_changed", "focused state changes plan: expected %q, resolved %q", input.ExpectedPlanID, refreshed.PlanID)
					}
					actualPlan = refreshed
					steps = actualPlan.Steps
				case "command":
					primitive := commandSpecs[step.Command.Name]
					payloads, buildErr := primitive.build(step.Command.Arguments)
					if buildErr != nil {
						return buildErr
					}
					observations = mergeObservationCursors(observations, commandObservationCursors(payloads))
					receipt := runtime.dispatchPayloads(workCtx, step.Command.Name, input.Name, payloads, Automation.CommandOptions{
						WorkID:   lease.WorkID(),
						Owner:    owner,
						Surface:  options.Surface,
						Priority: lease.Priority(),
						Guard:    lease.Active,
					})
					if !receipt.Accepted {
						return toolError("queue_rejected", "command harness rejected contextual command %q: %s", step.Command.Name, receipt.Message)
					}
					receipts = append(receipts, receipt)
				}
			}
			return nil
		},
	})
	if err != nil {
		return ContextCommandExecution{}, err
	}
	return ContextCommandExecution{
		WorkID:       workID,
		Dispatched:   true,
		Plan:         actualPlan,
		Receipts:     receipts,
		Observations: observations,
	}, nil
}

func contextPlanID(plan ContextCommandPlan) string {
	fingerprint := struct {
		Command string               `json:"command"`
		Effect  Effect               `json:"effect"`
		Steps   []ContextCommandStep `json:"steps"`
		Claims  []Automation.Claim   `json:"claims"`
	}{Command: plan.Command, Effect: plan.Effect, Steps: plan.Steps, Claims: plan.Claims}
	data, _ := json.Marshal(fingerprint)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:12])
}

func effectRank(effect Effect) int {
	switch effect {
	case EffectRead:
		return 0
	case EffectControl:
		return 1
	case EffectGameQuery:
		return 2
	case EffectGameAction:
		return 3
	case EffectDestructive:
		return 4
	case EffectExternal:
		return 5
	default:
		return 100
	}
}

func mergeObservationCursors(current, next []stateObservationCursor) []stateObservationCursor {
	byKey := make(map[string]stateObservationCursor, len(current)+len(next))
	for _, cursor := range current {
		byKey[cursor.StateKey] = cursor
	}
	for _, cursor := range next {
		if _, exists := byKey[cursor.StateKey]; !exists {
			byKey[cursor.StateKey] = cursor
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]stateObservationCursor, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func observationCursorsForStateKeys(keys ...string) []stateObservationCursor {
	cursors := make([]stateObservationCursor, 0, len(keys))
	for _, key := range normalizeContextStateKeys(keys) {
		cursors = append(cursors, stateObservationCursor{
			StateKey:     key,
			AfterVersion: Automation.StateSnapshot(key).Version,
		})
	}
	return cursors
}

func primitiveCommand(name string, arguments interface{}, description string) ContextCommandStep {
	data, _ := json.Marshal(arguments)
	return ContextCommandStep{
		Kind:        "command",
		Description: description,
		Command:     &ContextPrimitiveCommand{Name: name, Arguments: data},
	}
}

func focusCastleStep(castle ContextCastleReference) ContextCommandStep {
	return ContextCommandStep{
		Kind:        "focus_castle",
		Description: fmt.Sprintf("Refresh and focus %s (%d)", castle.Name, castle.CastleID),
		Castle:      &castle,
	}
}

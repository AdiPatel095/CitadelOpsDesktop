package Toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"

	"CitadelDesktop/Server/Automation"
)

type contextCommandSpecBuilder struct {
	commandSpecs map[string]commandSpec
	specs        map[string]contextCommandSpec
}

func newContextCommandSpecs(commandSpecs map[string]commandSpec) (map[string]contextCommandSpec, error) {
	builder := &contextCommandSpecBuilder{
		commandSpecs: commandSpecs,
		specs:        make(map[string]contextCommandSpec),
	}
	registrars := []func(*contextCommandSpecBuilder) error{
		registerContextAccountCommands,
		registerContextProductionCommands,
		registerContextResourceCommands,
		registerContextCastleCommands,
	}
	for _, register := range registrars {
		if err := register(builder); err != nil {
			return nil, err
		}
	}
	return builder.specs, nil
}

func (builder *contextCommandSpecBuilder) add(definition ContextCommandDefinition, resolve func(context.Context, json.RawMessage) (ContextCommandPlan, error)) error {
	if definition.Name == "" || definition.Description == "" || definition.Effect == "" || resolve == nil || !json.Valid(definition.InputSchema) {
		return toolError("invalid_context_command_spec", "invalid contextual command spec %q", definition.Name)
	}
	if _, exists := builder.specs[definition.Name]; exists {
		return toolError("invalid_context_command_spec", "duplicate contextual command spec %q", definition.Name)
	}
	builder.specs[definition.Name] = contextCommandSpec{definition: definition, resolve: resolve}
	return nil
}

func contextPlan(claims []Automation.Claim, stateKeys ...string) ContextCommandPlan {
	return ContextCommandPlan{
		Claims:      normalizeContextClaims(claims),
		Resolutions: []ContextResolution{},
		Steps:       []ContextCommandStep{},
		stateKeys:   normalizeContextStateKeys(stateKeys),
	}
}

func normalizeContextClaims(claims []Automation.Claim) []Automation.Claim {
	byKey := make(map[string]Automation.ClaimMode)
	for _, claim := range claims {
		if claim.Key == "" {
			continue
		}
		mode := claim.Mode
		if existing, ok := byKey[claim.Key]; !ok || mode == Automation.ClaimExclusive || existing == 0 {
			byKey[claim.Key] = mode
		}
	}
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]Automation.Claim, 0, len(keys))
	for _, key := range keys {
		out = append(out, Automation.Claim{Key: key, Mode: byKey[key]})
	}
	return out
}

func normalizeContextStateKeys(keys []string) []string {
	seen := make(map[string]bool)
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func containsInt(values []int, target int) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func contextCastleClaim(castleID int, domain string) Automation.Claim {
	return Automation.CastleClaim(castleID, domain)
}

func resolution(field string, value interface{}, source string, detail ...string) ContextResolution {
	entry := ContextResolution{Field: field, Value: value, Source: source}
	if len(detail) > 0 {
		entry.Detail = detail[0]
	}
	return entry
}

func contextCommandError(command, format string, args ...interface{}) error {
	return toolError("context_unavailable", "%s: %s", command, fmt.Sprintf(format, args...))
}

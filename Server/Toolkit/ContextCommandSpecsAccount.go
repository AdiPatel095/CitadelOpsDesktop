package Toolkit

import (
	"context"
	"encoding/json"
	"sort"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
)

type contextRefreshAccountArgs struct {
	Domains []string `json:"domains"`
}

type contextCastleOnlyArgs struct {
	Castle contextCastleSelector `json:"castle"`
}

func registerContextAccountCommands(builder *contextCommandSpecBuilder) error {
	if err := builder.add(ContextCommandDefinition{
		Name:        "refresh_account",
		Description: "Refresh selected authoritative state domains without requiring the caller to know refresh opcodes or alliance identifiers.",
		InputSchema: objectSchema(map[string]interface{}{
			"domains": map[string]interface{}{
				"type":        "array",
				"description": "State domains to refresh. all expands to every supported domain.",
				"items":       enumProperty("Refresh domain.", "all", "castles", "resources", "movements", "alliance", "equipment", "inventory", "transport", "crafting"),
				"minItems":    1,
			},
		}, "domains"),
		Effect:   EffectGameQuery,
		Resolves: []string{"refresh opcodes", "alliance AID", "broker claims", "state observation keys"},
	}, resolveRefreshAccount); err != nil {
		return err
	}

	return builder.add(ContextCommandDefinition{
		Name:        "focus_castle",
		Description: "Resolve a player castle by ID, Citadel key, display name, or current focus, then refresh and focus it with the correct kingdom command.",
		InputSchema: objectSchema(map[string]interface{}{
			"castle": contextCastleSelectorSchema("Player castle to focus."),
		}, "castle"),
		Effect:   EffectGameQuery,
		Resolves: []string{"castle AID", "kingdom KID", "map coordinates", "JAA/JCA selection"},
	}, resolveFocusCastle)
}

func resolveRefreshAccount(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextRefreshAccountArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if len(args.Domains) == 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "domains must not be empty")
	}
	supported := []string{"castles", "resources", "movements", "alliance", "equipment", "inventory", "transport", "crafting"}
	selected := make(map[string]bool)
	for _, domain := range args.Domains {
		if domain == "all" {
			for _, supportedDomain := range supported {
				selected[supportedDomain] = true
			}
			continue
		}
		valid := false
		for _, supportedDomain := range supported {
			if domain == supportedDomain {
				valid = true
				break
			}
		}
		if !valid {
			return ContextCommandPlan{}, toolError("invalid_arguments", "unsupported refresh domain %q", domain)
		}
		selected[domain] = true
	}
	domains := make([]string, 0, len(selected))
	for domain := range selected {
		domains = append(domains, domain)
	}
	sort.Strings(domains)

	claims := []Automation.Claim{}
	stateKeys := []string{}
	plan := ContextCommandPlan{}
	addDCL := selected["castles"] || selected["resources"]
	if addDCL {
		plan.Steps = append(plan.Steps, primitiveCommand("dcl_refresh", struct{}{}, "Refresh owned castle details and resources"))
		claims = append(claims, Automation.SharedClaim("account:castles"), Automation.SharedClaim(Automation.ClaimAccountResources))
		stateKeys = append(stateKeys, Automation.StateCastles, Automation.StateResources, Automation.StateOpcode("dcl"))
	}
	if selected["movements"] {
		plan.Steps = append(plan.Steps, primitiveCommand("gam", struct{}{}, "Refresh active movements and commander state"))
		claims = append(claims, Automation.SharedClaim("account:movement"))
		stateKeys = append(stateKeys, Automation.StateMovement, Automation.StateOpcode("gam"))
	}
	if selected["alliance"] {
		allianceID := Models.GetGameState().Alliance.AID
		if allianceID <= 0 {
			if len(domains) == 1 {
				return ContextCommandPlan{}, contextCommandError("refresh_account", "alliance state does not contain an AID")
			}
			plan.Warnings = append(plan.Warnings, "Alliance refresh was skipped because live state does not contain an alliance AID.")
		} else {
			plan.Steps = append(plan.Steps, primitiveCommand("ain", allianceIDArgs{AllianceID: allianceID}, "Refresh alliance members and castles"))
			plan.Resolutions = append(plan.Resolutions, resolution("allianceId", allianceID, "state.alliance.aid"))
			claims = append(claims, Automation.SharedClaim("account:alliance"))
			stateKeys = append(stateKeys, Automation.StateAlliance, Automation.StateOpcode("ain"))
		}
	}
	if selected["equipment"] {
		plan.Steps = append(plan.Steps, primitiveCommand("upgrade_menu_refresh", struct{}{}, "Refresh equipment, gems, leaders, and upgrade state"))
		claims = append(claims, Automation.SharedClaim(Automation.ClaimEquipment))
		stateKeys = append(stateKeys, Automation.StateEquipment, Automation.StateOpcode("gnr"), Automation.StateOpcode("ggm"), Automation.StateOpcode("gei"), Automation.StateOpcode("gli"))
	}
	if selected["inventory"] {
		plan.Steps = append(plan.Steps,
			primitiveCommand("gii", struct{}{}, "Refresh construction-item inventory"),
			primitiveCommand("sin", struct{}{}, "Refresh building and decoration storage"),
		)
		claims = append(claims, Automation.SharedClaim(Automation.ClaimTCIInventory))
		stateKeys = append(stateKeys, Automation.StateInventory, Automation.StateOpcode("gii"), Automation.StateOpcode("sin"))
	}
	if selected["transport"] {
		plan.Steps = append(plan.Steps,
			primitiveCommand("kpi", struct{}{}, "Refresh kingdom transports"),
			primitiveCommand("cmi", struct{}{}, "Refresh market resources and barrows"),
			primitiveCommand("boi", struct{}{}, "Refresh caravan booster state"),
		)
		claims = append(claims, Automation.SharedClaim(Automation.ClaimTransport))
		stateKeys = append(stateKeys, Automation.StateTransport, Automation.StateOpcode("kpi"), Automation.StateOpcode("cmi"), Automation.StateOpcode("boi"))
	}
	if selected["crafting"] {
		plan.Steps = append(plan.Steps, primitiveCommand("crin", struct{}{}, "Refresh crafting queues and entitlements"))
		claims = append(claims, Automation.SharedClaim(Automation.ClaimCrafting))
		stateKeys = append(stateKeys, Automation.StateCastles, Automation.StateOpcode("crin"))
	}
	plan.Claims = normalizeContextClaims(claims)
	plan.stateKeys = normalizeContextStateKeys(stateKeys)
	plan.Resolutions = append(plan.Resolutions, resolution("domains", domains, "input.domains", "Expanded and deduplicated refresh domains"))
	return plan, nil
}

func resolveFocusCastle(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextCastleOnlyArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	castle, resolutions, err := resolveContextCastle(args.Castle)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimGameFocus),
		contextCastleClaim(castle.CastleID, "focus"),
	}, Automation.StateFocus, Automation.StateCastles)
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Steps = append(plan.Steps, focusCastleStep(castle))
	return plan, nil
}

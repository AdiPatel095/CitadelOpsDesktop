package Toolkit

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
)

type contextMarketResourceArgs struct {
	Source   contextCastleSelector `json:"source"`
	Target   contextCastleSelector `json:"target"`
	Resource string                `json:"resource"`
	Amount   int                   `json:"amount"`
}

type contextKingdomResourceArgs struct {
	Source          contextCastleSelector `json:"source"`
	TargetKingdomID int                   `json:"targetKingdomId"`
	Resource        string                `json:"resource"`
	Amount          int                   `json:"amount"`
}

func registerContextResourceCommands(builder *contextCommandSpecBuilder) error {
	resourceProperty := enumProperty("Resource name resolved to the protocol goods code.", "wood", "stone", "coal", "oil", "glass", "iron")
	if err := builder.add(ContextCommandDefinition{
		Name:        "send_market_resource",
		Description: "Send a same-kingdom market shipment between two player castles; resolves both AIDs, kingdom, target coordinates, and the wire resource code.",
		InputSchema: objectSchema(map[string]interface{}{
			"source":   contextCastleSelectorSchema("Player castle sending the resource."),
			"target":   contextCastleSelectorSchema("Player castle receiving the resource."),
			"resource": resourceProperty,
			"amount":   schemaProperty("integer", "Positive resource amount."),
		}, "source", "target", "resource", "amount"),
		Effect:   EffectGameAction,
		Resolves: []string{"source and target castle context", "same-kingdom validation", "target coordinates", "resource wire code", "live source balance"},
	}, resolveMarketResource); err != nil {
		return err
	}

	return builder.add(ContextCommandDefinition{
		Name:        "send_kingdom_resource",
		Description: "Send a kingdom resource from a selected player castle to another kingdom; resolves source AID/KID, resource code, balance, and known unlock state.",
		InputSchema: objectSchema(map[string]interface{}{
			"source":          contextCastleSelectorSchema("Player castle sending the kingdom resource."),
			"targetKingdomId": schemaProperty("integer", "Destination kingdom KID."),
			"resource":        resourceProperty,
			"amount":          schemaProperty("integer", "Positive resource amount."),
		}, "source", "targetKingdomId", "resource", "amount"),
		Effect:   EffectGameAction,
		Resolves: []string{"source castle AID/KID", "resource wire code", "live source balance", "known transport unlock"},
	}, resolveKingdomResource)
}

func resolveMarketResource(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextMarketResourceArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.Amount <= 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "amount must be positive")
	}
	resourceCode, ok := contextResourceCode(args.Resource)
	if !ok {
		return ContextCommandPlan{}, toolError("invalid_arguments", "unsupported resource %q", args.Resource)
	}
	source, sourceResolutions, err := resolveContextCastle(args.Source)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	target, targetResolutions, err := resolveContextCastle(args.Target)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if source.CastleID == target.CastleID {
		return ContextCommandPlan{}, toolError("invalid_arguments", "source and target castles must differ")
	}
	if source.KingdomID != target.KingdomID {
		return ContextCommandPlan{}, toolError("invalid_arguments", "market shipments require the same kingdom; source KID=%d target KID=%d", source.KingdomID, target.KingdomID)
	}
	if target.MapX == 0 && target.MapY == 0 {
		return ContextCommandPlan{}, contextCommandError("send_market_resource", "target castle %d has no loaded map coordinates", target.CastleID)
	}
	available, err := contextCastleResourceAmount(source.CastleID, args.Resource)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if float64(args.Amount) > math.Floor(available) {
		return ContextCommandPlan{}, toolError("invalid_arguments", "requested amount %d exceeds live %s balance %.0f in source castle", args.Amount, args.Resource, available)
	}

	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimTransport),
		Automation.ExclusiveClaim(Automation.ClaimAccountResources),
		contextCastleClaim(source.CastleID, "market-send"),
		contextCastleClaim(target.CastleID, "market-receive"),
	}, Automation.StateTransport, Automation.StateResources, Automation.StateMovement, Automation.StateOpcode("crm"))
	plan.Resolutions = append(plan.Resolutions, sourceResolutions...)
	for index := range plan.Resolutions {
		plan.Resolutions[index].Field = "source." + plan.Resolutions[index].Field
	}
	for _, targetResolution := range targetResolutions {
		targetResolution.Field = "target." + targetResolution.Field
		plan.Resolutions = append(plan.Resolutions, targetResolution)
	}
	plan.Resolutions = append(plan.Resolutions,
		resolution("resource.code", resourceCode, "catalog.resourceCodes", args.Resource),
		resolution("resource.available", available, "state.castle.amount"),
		resolution("shipment.kingdomId", source.KingdomID, "state.castle", "Validated source and target kingdom equality"),
	)
	plan.Steps = append(plan.Steps, primitiveCommand("crm", marketTransportArgs{
		SourceKingdomID: source.KingdomID,
		SourceCastleID:  source.CastleID,
		TargetX:         target.MapX,
		TargetY:         target.MapY,
		ResourceCode:    resourceCode,
		Amount:          args.Amount,
	}, fmt.Sprintf("Send %d %s from %s to %s", args.Amount, args.Resource, source.Name, target.Name)))
	return plan, nil
}

func resolveKingdomResource(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextKingdomResourceArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.TargetKingdomID < 0 || args.Amount <= 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "targetKingdomId must not be negative and amount must be positive")
	}
	resourceCode, ok := contextResourceCode(args.Resource)
	if !ok {
		return ContextCommandPlan{}, toolError("invalid_arguments", "unsupported resource %q", args.Resource)
	}
	source, resolutions, err := resolveContextCastle(args.Source)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if source.KingdomID == args.TargetKingdomID {
		return ContextCommandPlan{}, toolError("invalid_arguments", "source and target kingdom are both %d; use send_market_resource for same-kingdom transfers", source.KingdomID)
	}
	available, err := contextCastleResourceAmount(source.CastleID, args.Resource)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if float64(args.Amount) > math.Floor(available) {
		return ContextCommandPlan{}, toolError("invalid_arguments", "requested amount %d exceeds live %s balance %.0f in source castle", args.Amount, args.Resource, available)
	}
	unlockKnown := false
	unlocked := false
	for _, unlock := range Models.GetGameState().KingdomTransportSnapshot().Unlocks {
		if unlock.KingdomID == args.TargetKingdomID {
			unlockKnown = true
			unlocked = unlock.Unlocked > 0 || unlock.Created > 0
			break
		}
	}
	if unlockKnown && !unlocked {
		return ContextCommandPlan{}, contextCommandError("send_kingdom_resource", "target kingdom %d is present but not unlocked", args.TargetKingdomID)
	}

	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimTransport),
		Automation.ExclusiveClaim(Automation.ClaimAccountResources),
		contextCastleClaim(source.CastleID, "kingdom-resource-send"),
	}, Automation.StateTransport, Automation.StateResources, Automation.StateOpcode("kgt"))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions,
		resolution("resource.code", resourceCode, "catalog.resourceCodes", args.Resource),
		resolution("resource.available", available, "state.castle.amount"),
		resolution("transport.targetKingdomId", args.TargetKingdomID, "input.targetKingdomId"),
		resolution("transport.unlockKnown", unlockKnown, "state.kingdomTransport.unlocks"),
	)
	if !unlockKnown {
		plan.Warnings = append(plan.Warnings, "Target kingdom unlock state is not loaded; the game server remains authoritative.")
	}
	plan.Steps = append(plan.Steps, primitiveCommand("kgt", kingdomTransportArgs{
		SourceCastleID:  source.CastleID,
		SourceKingdomID: source.KingdomID,
		TargetKingdomID: args.TargetKingdomID,
		ResourceCode:    resourceCode,
		Amount:          args.Amount,
	}, fmt.Sprintf("Send %d %s from %s to kingdom %d", args.Amount, args.Resource, source.Name, args.TargetKingdomID)))
	return plan, nil
}

func contextResourceCode(resource string) (string, bool) {
	codes := map[string]string{
		"wood": "W", "stone": "S", "coal": "C", "oil": "O", "glass": "G", "iron": "I",
	}
	code, ok := codes[resource]
	return code, ok
}

func contextCastleResourceAmount(castleID int, resource string) (float64, error) {
	castle := Models.GetGameState().GetCastleByID(castleID)
	if castle == nil {
		return 0, contextCommandError("resource resolution", "castle %d is missing from live state", castleID)
	}
	switch resource {
	case "wood":
		return castle.Amount.WoodAmount, nil
	case "stone":
		return castle.Amount.StoneAmount, nil
	case "coal":
		return castle.Amount.CoalAmount, nil
	case "oil":
		return castle.Amount.OilAmount, nil
	case "glass":
		return castle.Amount.GlassAmount, nil
	case "iron":
		return castle.Amount.IronAmount, nil
	default:
		return 0, toolError("invalid_arguments", "unsupported resource %q", resource)
	}
}

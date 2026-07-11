package Toolkit

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/Automation"
	gamedata "CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Models"
)

const contextProductionSessionKey = 73

type contextQueueProductionArgs struct {
	Castle contextCastleSelector `json:"castle"`
	Kind   string                `json:"kind"`
	ItemID int                   `json:"itemId"`
	Amount int                   `json:"amount,omitempty"`
}

type contextWoundedUnitArgs struct {
	Castle contextCastleSelector `json:"castle"`
	UnitID int                   `json:"unitId"`
	Amount int                   `json:"amount,omitempty"`
}

func registerContextProductionCommands(builder *contextCommandSpecBuilder) error {
	if err := builder.add(ContextCommandDefinition{
		Name:        "queue_production",
		Description: "Queue troop recruitment or tool production using a castle selector; resolves focus, AID/KID/coordinates, production line, session defaults, and queueability.",
		InputSchema: objectSchema(map[string]interface{}{
			"castle": contextCastleSelectorSchema("Castle whose production queue should receive the item."),
			"kind":   enumProperty("Production line.", "troop", "tool"),
			"itemId": schemaProperty("integer", "Troop or tool WOD ID."),
			"amount": schemaProperty("integer", "Optional positive amount. Omit or use 0 to derive one full stack from the selected castle's buildings and equipped construction items."),
		}, "castle", "kind", "itemId"),
		Effect: EffectGameAction,
		Resolves: []string{
			"castle AID", "kingdom and coordinates", "BUP line ID", "session key", "protocol defaults", "queueable catalog membership",
		},
	}, resolveQueueProduction); err != nil {
		return err
	}

	woundedSchema := objectSchema(map[string]interface{}{
		"castle": contextCastleSelectorSchema("Castle hospital containing the wounded units."),
		"unitId": schemaProperty("integer", "Wounded troop WOD ID."),
		"amount": schemaProperty("integer", "Optional positive amount. Omit or use 0 to select every currently wounded unit of this type in HI."),
	}, "castle", "unitId")
	if err := builder.add(ContextCommandDefinition{
		Name:        "heal_wounded",
		Description: "Heal wounded troops in a selected castle; resolves focus and the live wounded amount when amount is omitted.",
		InputSchema: woundedSchema,
		Effect:      EffectGameAction,
		Resolves:    []string{"castle focus", "live HI wounded count", "heal amount"},
	}, func(ctx context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
		return resolveWoundedCommand(ctx, raw, false)
	}); err != nil {
		return err
	}

	return builder.add(ContextCommandDefinition{
		Name:        "discard_wounded",
		Description: "Permanently discard wounded troops in a selected castle; resolves focus and the live wounded amount when amount is omitted.",
		InputSchema: woundedSchema,
		Effect:      EffectDestructive,
		Resolves:    []string{"castle focus", "live HI wounded count", "discard amount"},
	}, func(ctx context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
		return resolveWoundedCommand(ctx, raw, true)
	})
}

func resolveQueueProduction(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextQueueProductionArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.ItemID <= 0 || args.Amount < 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "itemId must be positive and amount must not be negative")
	}
	if args.Kind != "troop" && args.Kind != "tool" {
		return ContextCommandPlan{}, toolError("invalid_arguments", "kind must be troop or tool")
	}
	castle, resolutions, err := resolveContextCastle(args.Castle)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	castleInfo := Models.GetGameState().GetCastleByID(castle.CastleID)
	if castleInfo == nil {
		return ContextCommandPlan{}, contextCommandError("queue_production", "castle %d is missing from live state", castle.CastleID)
	}
	lineID := 0
	queueDomain := "barracks-queue"
	if args.Kind == "tool" {
		lineID = 1
		queueDomain = "tool-workshop-queue"
	} else if !gamedata.IsTroop(args.ItemID) {
		return ContextCommandPlan{}, toolError("invalid_arguments", "itemId %d is not a known troop WOD ID", args.ItemID)
	}
	queueable := GameParser.QueueableProductionForCastle(castleInfo)
	allowed := containsInt(queueable.RecruitUnitIDs, args.ItemID)
	if args.Kind == "tool" {
		allowed = containsInt(queueable.ToolIDs, args.ItemID)
	}
	if !allowed {
		return ContextCommandPlan{}, contextCommandError(
			"queue_production",
			"item %d is not currently queueable as %s in castle %d; refresh/focus the castle or choose an unlocked item",
			args.ItemID, args.Kind, castle.CastleID,
		)
	}
	amount := args.Amount
	amountSource := "input.amount"
	if amount == 0 {
		amountSource = "state.castle.productionCapacity"
		if args.Kind == "tool" {
			amount = GameParser.ToolProductionStackCapacityForTool(castleInfo, args.ItemID)
		} else {
			amount = GameParser.BarracksRecruitStackCapacity(castleInfo)
		}
		if amount <= 0 {
			return ContextCommandPlan{}, contextCommandError(
				"queue_production",
				"could not derive a %s stack amount for castle %d; refresh/focus its building state or provide amount explicitly",
				args.Kind, castle.CastleID,
			)
		}
	}
	if queue := castleInfo.SlotProductionByLID[lineID]; queue != nil && queue.QueueCapacity > 0 && len(queue.Queued) >= queue.QueueCapacity {
		return ContextCommandPlan{}, contextCommandError(
			"queue_production",
			"castle %d %s queue is full (%d/%d)",
			castle.CastleID, args.Kind, len(queue.Queued), queue.QueueCapacity,
		)
	}

	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimGameFocus),
		contextCastleClaim(castle.CastleID, queueDomain),
		Automation.ExclusiveClaim(Automation.ClaimAccountResources),
	}, Automation.StateFocus, Automation.StateCastles, Automation.StateResources, Automation.StateOpcode("bup"))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions,
		resolution("production.lineId", lineID, "context.kind", fmt.Sprintf("%s maps to BUP LID %d", args.Kind, lineID)),
		resolution("production.sessionKey", contextProductionSessionKey, "protocol.default", "Current app production flows use captured SK 73"),
		resolution("production.protocolDefaults", map[string]int{"publicOrder": -1, "power": 0, "slotId": 0}, "protocol.default"),
		resolution("production.queueable", true, "state.castle.queueableIds"),
		resolution("production.amount", amount, amountSource),
	)
	plan.Warnings = append(plan.Warnings, "Production session key is still a protocol default (73) because live SK is not yet modeled in GameState.")
	plan.Steps = append(plan.Steps,
		focusCastleStep(castle),
		primitiveCommand("bup", productionPurchaseArgs{
			LineID:      lineID,
			UnitWodID:   args.ItemID,
			Amount:      amount,
			PublicOrder: -1,
			Power:       0,
			SessionKey:  contextProductionSessionKey,
			SlotID:      0,
			CastleID:    castle.CastleID,
		}, fmt.Sprintf("Queue %d of %s %d in castle %d", amount, args.Kind, args.ItemID, castle.CastleID)),
	)
	return plan, nil
}

func resolveWoundedCommand(_ context.Context, raw json.RawMessage, discard bool) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextWoundedUnitArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.UnitID <= 0 || args.Amount < 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "unitId must be positive and amount must not be negative")
	}
	if !gamedata.IsTroop(args.UnitID) {
		return ContextCommandPlan{}, toolError("invalid_arguments", "unitId %d is not a known troop WOD ID", args.UnitID)
	}
	castle, resolutions, err := resolveContextCastle(args.Castle)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	castleInfo := Models.GetGameState().GetCastleByID(castle.CastleID)
	if castleInfo == nil {
		return ContextCommandPlan{}, contextCommandError("wounded command", "castle %d is missing from live state", castle.CastleID)
	}
	available := castleInfo.Troops.TroopsHI[args.UnitID]
	if available <= 0 {
		return ContextCommandPlan{}, contextCommandError("wounded command", "castle %d has no unit %d in the standard hospital HI state", castle.CastleID, args.UnitID)
	}
	amount := args.Amount
	amountSource := "input.amount"
	if amount == 0 {
		amount = available
		amountSource = "state.castle.troopsHI"
	}
	if amount > available {
		return ContextCommandPlan{}, toolError("invalid_arguments", "requested amount %d exceeds live wounded count %d", amount, available)
	}

	commandName := "hru"
	action := "Heal"
	effectClaim := "hospital-queue"
	claims := []Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimGameFocus),
		contextCastleClaim(castle.CastleID, effectClaim),
		Automation.ExclusiveClaim(Automation.ClaimAccountResources),
	}
	if discard {
		commandName = "hdu"
		action = "Discard"
		claims = claims[:2]
	}
	plan := contextPlan(claims, Automation.StateFocus, Automation.StateCastles, Automation.StateResources, Automation.StateOpcode(commandName))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions,
		resolution("wounded.available", available, "state.castle.troopsHI", "Special-hospital SHI units are intentionally excluded"),
		resolution("wounded.amount", amount, amountSource),
	)
	plan.Steps = append(plan.Steps,
		focusCastleStep(castle),
		primitiveCommand(commandName, unitAmountArgs{UnitWodID: args.UnitID, Amount: amount}, fmt.Sprintf("%s %d of wounded unit %d", action, amount, args.UnitID)),
	)
	return plan, nil
}

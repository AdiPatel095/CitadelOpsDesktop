package Toolkit

import (
	"encoding/json"

	"CitadelDesktop/Server/GameCommands"
)

type productionPurchaseArgs struct {
	LineID      int `json:"lineId"`
	UnitWodID   int `json:"unitWodId"`
	Amount      int `json:"amount"`
	PublicOrder int `json:"publicOrder"`
	Power       int `json:"power"`
	SessionKey  int `json:"sessionKey"`
	SlotID      int `json:"slotId"`
	CastleID    int `json:"castleId"`
}

type unitAmountArgs struct {
	UnitWodID int `json:"unitWodId"`
	Amount    int `json:"amount"`
}

type routePreviewArgs struct {
	TargetX int `json:"targetX"`
	TargetY int `json:"targetY"`
	SourceX int `json:"sourceX"`
	SourceY int `json:"sourceY"`
}

type birdDispatchArgs struct {
	CastleID   int                         `json:"castleId"`
	TargetX    int                         `json:"targetX"`
	TargetY    int                         `json:"targetY"`
	RouteID    int                         `json:"routeId"`
	DelayHours int                         `json:"delayHours"`
	HBW        int                         `json:"hbw"`
	PTT        int                         `json:"ptt"`
	Troops     []GameCommands.CRATroopPair `json:"troops"`
}

type movementIDArgs struct {
	MovementID int `json:"movementId"`
}

type allianceIDArgs struct {
	AllianceID int `json:"allianceId"`
}

type spyMissionArgs struct {
	SourceCastleID int `json:"sourceCastleId"`
	TargetX        int `json:"targetX"`
	TargetY        int `json:"targetY"`
	SpyCount       int `json:"spyCount"`
}

type castleAttackArgs struct {
	SourceX            int                         `json:"sourceX"`
	SourceY            int                         `json:"sourceY"`
	TargetX            int                         `json:"targetX"`
	TargetY            int                         `json:"targetY"`
	KingdomID          int                         `json:"kingdomId"`
	CommanderID        int                         `json:"commanderId"`
	WaitHours          int                         `json:"waitHours"`
	UseTravelFeather   bool                        `json:"useTravelFeather"`
	AttackValid        int                         `json:"attackValid"`
	Waves              []GameCommands.CRAWave      `json:"waves"`
	AttackSupportTools []int                       `json:"attackSupportTools"`
	SupportTroops      []GameCommands.CRATroopPair `json:"supportTroops"`
}

func registerArmyAndMovementCommands(builder *commandSpecBuilder) error {
	if err := builder.add("bup", "bup", "Queue troop or tool production in a castle production line.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"lineId":      schemaProperty("integer", "Production line LID; 0 is barracks and 1 is tool workshop."),
			"unitWodId":   schemaProperty("integer", "Global unit or tool WOD ID."),
			"amount":      schemaProperty("integer", "Positive stack amount."),
			"publicOrder": schemaProperty("integer", "Captured PO wire field, commonly -1."),
			"power":       schemaProperty("integer", "Captured PWR wire field, commonly 0."),
			"sessionKey":  schemaProperty("integer", "Current production session key SK."),
			"slotId":      schemaProperty("integer", "Production SID, commonly 0."),
			"castleId":    schemaProperty("integer", "Castle instance AID."),
		}, "lineId", "unitWodId", "amount", "publicOrder", "power", "sessionKey", "slotId", "castleId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[productionPurchaseArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.LineID < 0 || args.SlotID < 0 {
				return nil, toolError("invalid_arguments", "lineId and slotId must not be negative")
			}
			if err := positive(args.UnitWodID, "unitWodId"); err != nil {
				return nil, err
			}
			if err := positive(args.Amount, "amount"); err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.BUPPayload(args.LineID, args.UnitWodID, args.Amount, args.PublicOrder, args.Power, args.SessionKey, args.SlotID, args.CastleID)), nil
		}); err != nil {
		return err
	}

	unitAmountSchema := objectSchema(map[string]interface{}{
		"unitWodId": schemaProperty("integer", "Global unit WOD ID."),
		"amount":    schemaProperty("integer", "Positive unit count."),
	}, "unitWodId", "amount")
	if err := builder.add("hru", "hru", "Queue healing for wounded units in the currently focused castle.", EffectGameAction, unitAmountSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[unitAmountArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.UnitWodID, "unitWodId"); err != nil {
				return nil, err
			}
			if err := positive(args.Amount, "amount"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.HRUPayload(args.UnitWodID, args.Amount)), nil
		}); err != nil {
		return err
	}
	if err := builder.add("hdu", "hdu", "Permanently discard wounded units from the currently focused castle.", EffectDestructive, unitAmountSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[unitAmountArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.UnitWodID, "unitWodId"); err != nil {
				return nil, err
			}
			if err := positive(args.Amount, "amount"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.HDUPayload(args.UnitWodID, args.Amount)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("sdi", "sdi", "Preview or select a movement route from source to target coordinates.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"targetX": schemaProperty("integer", "Target X coordinate."),
			"targetY": schemaProperty("integer", "Target Y coordinate."),
			"sourceX": schemaProperty("integer", "Source X coordinate."),
			"sourceY": schemaProperty("integer", "Source Y coordinate."),
		}, "targetX", "targetY", "sourceX", "sourceY"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[routePreviewArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.TargetX < 0 || args.TargetY < 0 || args.SourceX < 0 || args.SourceY < 0 {
				return nil, toolError("invalid_arguments", "movement coordinates must not be negative")
			}
			return onePayload(GameCommands.SDIPayload(args.TargetX, args.TargetY, args.SourceX, args.SourceY)), nil
		}); err != nil {
		return err
	}

	pairSchema := troopPairSchema()
	if err := builder.add("cds", "cds", "Dispatch a bird/station movement with an explicit working HBW/PTT pair and troop batch.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"castleId":   schemaProperty("integer", "Source castle instance AID."),
			"targetX":    schemaProperty("integer", "Target X coordinate."),
			"targetY":    schemaProperty("integer", "Target Y coordinate."),
			"routeId":    schemaProperty("integer", "SDI route LID; Citadel bird flows commonly use -14."),
			"delayHours": schemaProperty("integer", "Non-negative movement wait time WT in hours."),
			"hbw":        schemaProperty("integer", "Movement booster field; known pairs include -1/1 and 1007/0."),
			"ptt":        schemaProperty("integer", "Paid travel-time flag paired with HBW."),
			"troops": map[string]interface{}{
				"type":        "array",
				"description": "One or more [unitWodId, amount] pairs.",
				"items":       pairSchema,
				"minItems":    1,
			},
		}, "castleId", "targetX", "targetY", "routeId", "delayHours", "hbw", "ptt", "troops"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[birdDispatchArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			if args.TargetX < 0 || args.TargetY < 0 || args.DelayHours < 0 {
				return nil, toolError("invalid_arguments", "target coordinates and delayHours must not be negative")
			}
			if err := validateTroopPairs(args.Troops, "troops"); err != nil {
				return nil, err
			}
			troops, _ := json.Marshal(args.Troops)
			return onePayload(GameCommands.CDSPayload(args.CastleID, args.TargetX, args.TargetY, args.RouteID, args.DelayHours, args.HBW, args.PTT, string(troops))), nil
		}); err != nil {
		return err
	}

	if err := builder.add("mcm", "mcm", "Recall an active troop movement by movement ID.", EffectGameAction,
		objectSchema(map[string]interface{}{"movementId": schemaProperty("integer", "Active movement MID.")}, "movementId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[movementIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.MovementID, "movementId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.MCMPayload(args.MovementID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ain", "ain", "Refresh alliance member and castle information.", EffectGameQuery,
		objectSchema(map[string]interface{}{"allianceId": schemaProperty("integer", "Alliance AID.")}, "allianceId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[allianceIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.AllianceID, "allianceId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.AINPayload(args.AllianceID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("csm", "csm", "Send a military spy mission from a source castle to a target tile.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"sourceCastleId": schemaProperty("integer", "Source castle instance AID."),
			"targetX":        schemaProperty("integer", "Target X coordinate."),
			"targetY":        schemaProperty("integer", "Target Y coordinate."),
			"spyCount":       schemaProperty("integer", "Positive number of agents to send."),
		}, "sourceCastleId", "targetX", "targetY", "spyCount"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[spyMissionArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.SourceCastleID, "sourceCastleId"); err != nil {
				return nil, err
			}
			if args.TargetX < 0 || args.TargetY < 0 {
				return nil, toolError("invalid_arguments", "target coordinates must not be negative")
			}
			if err := positive(args.SpyCount, "spyCount"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.CSMPayload(args.SourceCastleID, args.TargetX, args.TargetY, args.SpyCount)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("cra", "cra", "Launch a castle attack with explicit waves, commander, support tools, and support troops.", EffectDestructive,
		castleAttackSchema(pairSchema),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[castleAttackArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.SourceX < 0 || args.SourceY < 0 || args.TargetX < 0 || args.TargetY < 0 || args.KingdomID < 0 || args.WaitHours < 0 {
				return nil, toolError("invalid_arguments", "coordinates, kingdomId, and waitHours must not be negative")
			}
			if err := positive(args.CommanderID, "commanderId"); err != nil {
				return nil, err
			}
			if args.AttackValid != 0 && args.AttackValid != 1 {
				return nil, toolError("invalid_arguments", "attackValid must be 0 or 1")
			}
			if len(args.Waves) == 0 {
				return nil, toolError("invalid_arguments", "waves must include at least one attack wave")
			}
			if err := validateCRAWaves(args.Waves); err != nil {
				return nil, err
			}
			if len(args.AttackSupportTools) > GameCommands.CRAAttackSupportToolSlots {
				return nil, toolError("invalid_arguments", "attackSupportTools exceeds %d slots", GameCommands.CRAAttackSupportToolSlots)
			}
			if len(args.SupportTroops) > GameCommands.CRASupportTroopSlots {
				return nil, toolError("invalid_arguments", "supportTroops exceeds %d slots", GameCommands.CRASupportTroopSlots)
			}
			if len(args.SupportTroops) > 0 {
				if err := validateTroopPairs(args.SupportTroops, "supportTroops"); err != nil {
					return nil, err
				}
			}
			payload, payloadErr := GameCommands.CRAPayload(GameCommands.CRALaunchParams{
				SourceX:            args.SourceX,
				SourceY:            args.SourceY,
				TargetX:            args.TargetX,
				TargetY:            args.TargetY,
				KingdomID:          args.KingdomID,
				CommanderID:        args.CommanderID,
				WaitHours:          args.WaitHours,
				UseTravelFeather:   args.UseTravelFeather,
				AttackValid:        args.AttackValid,
				Waves:              args.Waves,
				AttackSupportTools: args.AttackSupportTools,
				SupportTroops:      args.SupportTroops,
			})
			if payloadErr != nil {
				return nil, toolError("command_build_failed", "cra: %v", payloadErr)
			}
			return onePayload(payload), nil
		}); err != nil {
		return err
	}

	return nil
}

func troopPairSchema() map[string]interface{} {
	return map[string]interface{}{
		"type":        "array",
		"description": "A [wodId, amount] pair.",
		"items":       schemaProperty("integer", "Pair value."),
		"minItems":    2,
		"maxItems":    2,
	}
}

func castleAttackSchema(pairSchema map[string]interface{}) json.RawMessage {
	flankSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"T": map[string]interface{}{"type": "array", "items": pairSchema},
			"U": map[string]interface{}{"type": "array", "items": pairSchema},
		},
		"required": []string{"T", "U"},
	}
	waveSchema := map[string]interface{}{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]interface{}{
			"L": flankSchema,
			"R": flankSchema,
			"M": flankSchema,
		},
		"required": []string{"L", "R", "M"},
	}
	return objectSchema(map[string]interface{}{
		"sourceX":            schemaProperty("integer", "Source X coordinate."),
		"sourceY":            schemaProperty("integer", "Source Y coordinate."),
		"targetX":            schemaProperty("integer", "Target X coordinate."),
		"targetY":            schemaProperty("integer", "Target Y coordinate."),
		"kingdomId":          schemaProperty("integer", "Target map kingdom KID."),
		"commanderId":        schemaProperty("integer", "Wire commander LID; check movement state before use."),
		"waitHours":          schemaProperty("integer", "Non-negative WT delay."),
		"useTravelFeather":   schemaProperty("boolean", "Select feather HBW/PTT instead of coin movement booster."),
		"attackValid":        schemaProperty("integer", "Wire AV flag, 0 or 1."),
		"waves":              map[string]interface{}{"type": "array", "items": waveSchema, "minItems": 1},
		"attackSupportTools": map[string]interface{}{"type": "array", "items": schemaProperty("integer", "Support tool WOD ID."), "maxItems": GameCommands.CRAAttackSupportToolSlots},
		"supportTroops":      map[string]interface{}{"type": "array", "items": pairSchema, "maxItems": GameCommands.CRASupportTroopSlots},
	}, "sourceX", "sourceY", "targetX", "targetY", "kingdomId", "commanderId", "waitHours", "useTravelFeather", "attackValid", "waves", "attackSupportTools", "supportTroops")
}

func validateTroopPairs(pairs []GameCommands.CRATroopPair, field string) error {
	if len(pairs) == 0 {
		return toolError("invalid_arguments", "%s must not be empty", field)
	}
	for _, pair := range pairs {
		if pair[0] <= 0 || pair[1] <= 0 {
			return toolError("invalid_arguments", "%s pairs require positive WOD IDs and amounts", field)
		}
	}
	return nil
}

func validateCRAWaves(waves []GameCommands.CRAWave) error {
	hasUnits := false
	for _, wave := range waves {
		for _, flank := range []GameCommands.CRAFlank{wave.L, wave.R, wave.M} {
			for _, pair := range flank.U {
				if pair[0] > 0 && pair[1] > 0 {
					hasUnits = true
				}
			}
		}
	}
	if !hasUnits {
		return toolError("invalid_arguments", "waves must contain at least one positive unit pair")
	}
	return nil
}

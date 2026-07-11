package Toolkit

import (
	"encoding/json"
	"strings"

	"CitadelDesktop/Server/GameCommands"
)

type craftingStartArgs struct {
	KingdomID   int `json:"kingdomId"`
	CastleID    int `json:"castleId"`
	BuildingOID int `json:"buildingOid"`
	Power       int `json:"power"`
	RecipeID    int `json:"recipeId"`
}

type craftingRentArgs struct {
	KingdomID   int    `json:"kingdomId"`
	CastleID    int    `json:"castleId"`
	BuildingOID int    `json:"buildingOid"`
	Slot        int    `json:"slot"`
	SlotType    string `json:"slotType"`
}

type craftingSkipArgs struct {
	KingdomID   int    `json:"kingdomId"`
	CastleID    int    `json:"castleId"`
	BuildingOID int    `json:"buildingOid"`
	Slot        int    `json:"slot"`
	SlotType    string `json:"slotType"`
	PriceRubies int    `json:"priceRubies"`
}

type marketTransportArgs struct {
	SourceKingdomID int    `json:"sourceKingdomId"`
	SourceCastleID  int    `json:"sourceCastleId"`
	TargetX         int    `json:"targetX"`
	TargetY         int    `json:"targetY"`
	ResourceCode    string `json:"resourceCode"`
	Amount          int    `json:"amount"`
}

type kingdomTransportArgs struct {
	SourceCastleID  int    `json:"sourceCastleId"`
	SourceKingdomID int    `json:"sourceKingdomId"`
	TargetKingdomID int    `json:"targetKingdomId"`
	ResourceCode    string `json:"resourceCode"`
	Amount          int    `json:"amount"`
}

type kingdomTransportSkipArgs struct {
	TimeSkipID      string `json:"timeSkipId"`
	TargetKingdomID int    `json:"targetKingdomId"`
}

type beriCastleArgs struct {
	CastleID int `json:"castleId"`
}

type beriTransferArgs struct {
	SourceCastleID  int                         `json:"sourceCastleId"`
	BeriCastleID    int                         `json:"beriCastleId"`
	SourceKingdomID int                         `json:"sourceKingdomId"`
	TargetKingdomID int                         `json:"targetKingdomId"`
	Troops          []GameCommands.CRATroopPair `json:"troops"`
}

func registerCraftingAndTransportCommands(builder *commandSpecBuilder) error {
	if err := builder.add("crin", "crin", "Refresh sovereign crafting queues and research entitlements.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.CRINPayload)); err != nil {
		return err
	}
	if err := builder.add("crst", "crst", "Start or queue one official sovereign crafting recipe.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"kingdomId":   schemaProperty("integer", "Castle kingdom KID."),
			"castleId":    schemaProperty("integer", "Castle instance AID."),
			"buildingOid": schemaProperty("integer", "Crafting building OID."),
			"power":       schemaProperty("integer", "Captured PWR field."),
			"recipeId":    schemaProperty("integer", "Official crafting recipe CRID."),
		}, "kingdomId", "castleId", "buildingOid", "power", "recipeId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[craftingStartArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.CastleID <= 0 || args.BuildingOID <= 0 || args.RecipeID <= 0 {
				return nil, toolError("invalid_arguments", "kingdomId, castleId, buildingOid, or recipeId is invalid")
			}
			return onePayload(GameCommands.CRSTPayload(args.KingdomID, args.CastleID, args.BuildingOID, args.Power, args.RecipeID)), nil
		}); err != nil {
		return err
	}

	slotTypeProperty := enumProperty("Crafting slot family.", "production", "queue")
	if err := builder.add("crun", "crun", "Rent a crafting production or queue slot for seven days.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"kingdomId":   schemaProperty("integer", "Castle kingdom KID."),
			"castleId":    schemaProperty("integer", "Castle instance AID."),
			"buildingOid": schemaProperty("integer", "Crafting building OID."),
			"slot":        schemaProperty("integer", "One-based slot number."),
			"slotType":    slotTypeProperty,
		}, "kingdomId", "castleId", "buildingOid", "slot", "slotType"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[craftingRentArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.CastleID <= 0 || args.BuildingOID <= 0 || args.Slot <= 0 {
				return nil, toolError("invalid_arguments", "kingdomId, castleId, buildingOid, or slot is invalid")
			}
			if !validCraftingSlotType(args.SlotType) {
				return nil, toolError("invalid_arguments", "slotType must be production or queue")
			}
			return onePayload(GameCommands.CRUNPayload(args.KingdomID, args.CastleID, args.BuildingOID, args.Slot, args.SlotType)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("crsk", "crsk", "Spend the official ruby price to complete one crafting slot immediately.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"kingdomId":   schemaProperty("integer", "Castle kingdom KID."),
			"castleId":    schemaProperty("integer", "Castle instance AID."),
			"buildingOid": schemaProperty("integer", "Crafting building OID."),
			"slot":        schemaProperty("integer", "Zero-based slot index."),
			"slotType":    slotTypeProperty,
			"priceRubies": schemaProperty("integer", "Exact current PC2 ruby price from live state."),
		}, "kingdomId", "castleId", "buildingOid", "slot", "slotType", "priceRubies"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[craftingSkipArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.CastleID <= 0 || args.BuildingOID <= 0 || args.Slot < 0 || args.PriceRubies <= 0 {
				return nil, toolError("invalid_arguments", "kingdomId, castleId, buildingOid, slot, or priceRubies is invalid")
			}
			if !validCraftingSlotType(args.SlotType) {
				return nil, toolError("invalid_arguments", "slotType must be production or queue")
			}
			return onePayload(GameCommands.CRSKPayload(args.KingdomID, args.CastleID, args.BuildingOID, args.Slot, args.SlotType, args.PriceRubies)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("cmi", "cmi", "Refresh market resources and barrow capacity for owned castles.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.CMIPayload)); err != nil {
		return err
	}
	if err := builder.add("boi", "boi", "Refresh premium and permanent booster state.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.BOIPayload)); err != nil {
		return err
	}
	if err := builder.add("kpi", "kpi", "Refresh kingdom-resource transport unlocks and pending shipments.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.KPIPayload)); err != nil {
		return err
	}

	if err := builder.add("crm", "crm", "Start a same-kingdom market shipment without a paid horse or scheduled delay.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"sourceKingdomId": schemaProperty("integer", "Source kingdom KID."),
			"sourceCastleId":  schemaProperty("integer", "Source castle AID."),
			"targetX":         schemaProperty("integer", "Target castle X coordinate."),
			"targetY":         schemaProperty("integer", "Target castle Y coordinate."),
			"resourceCode":    schemaProperty("string", "Wire resource code, such as W, S, C, O, G, or I."),
			"amount":          schemaProperty("integer", "Positive resource amount."),
		}, "sourceKingdomId", "sourceCastleId", "targetX", "targetY", "resourceCode", "amount"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[marketTransportArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.SourceKingdomID < 0 || args.SourceCastleID <= 0 || args.TargetX < 0 || args.TargetY < 0 || args.Amount <= 0 || !validResourceCode(args.ResourceCode) {
				return nil, toolError("invalid_arguments", "source, target, resourceCode, or amount is invalid")
			}
			return onePayload(GameCommands.CRMPayload(args.SourceKingdomID, args.SourceCastleID, args.TargetX, args.TargetY, args.ResourceCode, args.Amount)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("kgt", "kgt", "Send one kingdom resource from a source castle to a target kingdom.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"sourceCastleId":  schemaProperty("integer", "Source castle AID."),
			"sourceKingdomId": schemaProperty("integer", "Source kingdom KID."),
			"targetKingdomId": schemaProperty("integer", "Target kingdom KID."),
			"resourceCode":    schemaProperty("string", "Wire resource code."),
			"amount":          schemaProperty("integer", "Positive resource amount."),
		}, "sourceCastleId", "sourceKingdomId", "targetKingdomId", "resourceCode", "amount"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[kingdomTransportArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.SourceCastleID <= 0 || args.SourceKingdomID < 0 || args.TargetKingdomID < 0 || args.Amount <= 0 || !validResourceCode(args.ResourceCode) {
				return nil, toolError("invalid_arguments", "source, target, resourceCode, or amount is invalid")
			}
			return onePayload(GameCommands.KGTPayload(args.SourceCastleID, args.SourceKingdomID, args.TargetKingdomID, args.ResourceCode, args.Amount)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("kingdom_resource_msk", "msk", "Consume a selected time skip on an in-flight kingdom resource shipment.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"timeSkipId":      schemaProperty("string", "Time-skip ID such as MS1 through MS7."),
			"targetKingdomId": schemaProperty("integer", "Target kingdom KID."),
		}, "timeSkipId", "targetKingdomId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[kingdomTransportSkipArgs](raw)
			if err != nil {
				return nil, err
			}
			if !validTimeSkipID(args.TimeSkipID) || args.TargetKingdomID < 0 {
				return nil, toolError("invalid_arguments", "timeSkipId or targetKingdomId is invalid")
			}
			return onePayload(GameCommands.KingdomResourceMSKPayload(args.TimeSkipID, args.TargetKingdomID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("fuc", "fuc", "Refresh Beri-world troop capacity for a castle.", EffectGameQuery,
		objectSchema(map[string]interface{}{"castleId": schemaProperty("integer", "Beri castle instance CID.")}, "castleId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[beriCastleArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.FUCPayload(args.CastleID)), nil
		}); err != nil {
		return err
	}
	if err := builder.add("sei", "sei", "Run the Beri troop-space follow-up query after FUC.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.SEIPayload)); err != nil {
		return err
	}
	if err := builder.add("dcl_refresh", "dcl", "Refresh castle details, resources, and troop snapshots.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.DCLRefreshPayload)); err != nil {
		return err
	}

	if err := builder.add("kut", "kut", "Transfer a troop batch from a source castle to the Beri world.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"sourceCastleId":  schemaProperty("integer", "Source castle SCID."),
			"beriCastleId":    schemaProperty("integer", "Wire CID, commonly -1 when no explicit castle is required."),
			"sourceKingdomId": schemaProperty("integer", "Source kingdom SKID."),
			"targetKingdomId": schemaProperty("integer", "Target kingdom TKID, normally 10."),
			"troops": map[string]interface{}{
				"type":        "array",
				"description": "One or more [unitWodId, amount] pairs.",
				"items":       troopPairSchema(),
				"minItems":    1,
			},
		}, "sourceCastleId", "beriCastleId", "sourceKingdomId", "targetKingdomId", "troops"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[beriTransferArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.SourceCastleID <= 0 || args.BeriCastleID < -1 || args.SourceKingdomID < 0 || args.TargetKingdomID < 0 {
				return nil, toolError("invalid_arguments", "source or target identifiers are invalid")
			}
			if err := validateTroopPairs(args.Troops, "troops"); err != nil {
				return nil, err
			}
			troops, _ := json.Marshal(args.Troops)
			return onePayload(GameCommands.KUTPayload(args.SourceCastleID, args.BeriCastleID, args.SourceKingdomID, args.TargetKingdomID, string(troops))), nil
		}); err != nil {
		return err
	}

	if err := builder.add("beri_msk", "msk", "Consume the fixed five-hour speed-up on a Beri-world troop transfer.", EffectDestructive, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.MSKPayload)); err != nil {
		return err
	}

	return nil
}

func validCraftingSlotType(value string) bool {
	return value == "production" || value == "queue"
}

func validResourceCode(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 16 {
		return false
	}
	for _, r := range value {
		if (r < 'A' || r > 'Z') && (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

func validTimeSkipID(value string) bool {
	if len(value) < 3 || len(value) > 8 || !strings.HasPrefix(value, "MS") {
		return false
	}
	for _, r := range value[2:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

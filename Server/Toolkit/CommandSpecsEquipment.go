package Toolkit

import (
	"encoding/json"

	"CitadelDesktop/Server/GameCommands"
)

type equipmentEquipArgs struct {
	EquipmentID float64 `json:"equipmentId"`
	LeaderID    float64 `json:"leaderId"`
	Equip       bool    `json:"equip"`
}

type gemEquipArgs struct {
	GemID       float64 `json:"gemId"`
	EquipmentID float64 `json:"equipmentId"`
	LeaderID    float64 `json:"leaderId"`
}

type equipmentLeaderArgs struct {
	EquipmentID float64 `json:"equipmentId"`
	LeaderID    float64 `json:"leaderId"`
}

type upgradeItemArgs struct {
	ItemID        float64 `json:"itemId"`
	EquipmentFlag int     `json:"equipmentFlag"`
	Currency      int     `json:"currency"`
}

type equipmentIDArgs struct {
	EquipmentID float64 `json:"equipmentId"`
}

type gemIDArgs struct {
	GemID float64 `json:"gemId"`
}

type equipConstructionItemArgs struct {
	BuildingOID        int `json:"buildingOid"`
	ConstructionItemID int `json:"constructionItemId"`
	SlotID             int `json:"slotId"`
	Mode               int `json:"mode"`
	KingdomID          int `json:"kingdomId"`
	CastleID           int `json:"castleId"`
}

type upgradeConstructionItemArgs struct {
	BuildingOID        int `json:"buildingOid"`
	UpgradeCode        int `json:"upgradeCode"`
	SlotID             int `json:"slotId"`
	KingdomID          int `json:"kingdomId"`
	CastleID           int `json:"castleId"`
	ConstructionItemID int `json:"constructionItemId"`
}

type trivialPurchaseArgs struct {
	ProductID   int `json:"productId"`
	BuildType   int `json:"buildType"`
	TypeID      int `json:"typeId"`
	Amount      int `json:"amount"`
	KingdomID   int `json:"kingdomId"`
	CastleID    int `json:"castleId"`
	PriceCode2  int `json:"priceCode2"`
	BuildAux    int `json:"buildAux"`
	Power       int `json:"power"`
	PublicOrder int `json:"publicOrder"`
}

func registerEquipmentAndTCICommands(builder *commandSpecBuilder) error {
	if err := builder.add("eeq", "eeq", "Equip or unequip one equipment instance on a leader.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"equipmentId": schemaProperty("number", "Equipment instance EID."),
			"leaderId":    schemaProperty("number", "Commander or castellan leader LID."),
			"equip":       schemaProperty("boolean", "True equips; false unequips."),
		}, "equipmentId", "leaderId", "equip"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[equipmentEquipArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.EquipmentID <= 0 || args.LeaderID <= 0 {
				return nil, toolError("invalid_arguments", "equipmentId and leaderId must be positive")
			}
			return onePayload(GameCommands.EEQPayload(args.EquipmentID, args.LeaderID, args.Equip)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("bge", "bge", "Attach a gem instance to an equipment instance on a leader.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"gemId":       schemaProperty("number", "Gem instance GID."),
			"equipmentId": schemaProperty("number", "Parent equipment EID."),
			"leaderId":    schemaProperty("number", "Commander or castellan leader LID."),
		}, "gemId", "equipmentId", "leaderId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[gemEquipArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.GemID <= 0 || args.EquipmentID <= 0 || args.LeaderID <= 0 {
				return nil, toolError("invalid_arguments", "gemId, equipmentId, and leaderId must be positive")
			}
			return onePayload(GameCommands.BGEPayload(args.GemID, args.EquipmentID, args.LeaderID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ege", "ege", "Remove a gem from an equipped item on a leader.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"equipmentId": schemaProperty("number", "Parent equipment EID."),
			"leaderId":    schemaProperty("number", "Commander or castellan leader LID."),
		}, "equipmentId", "leaderId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[equipmentLeaderArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.EquipmentID <= 0 || args.LeaderID <= 0 {
				return nil, toolError("invalid_arguments", "equipmentId and leaderId must be positive")
			}
			return onePayload(GameCommands.EGEPayload(args.EquipmentID, args.LeaderID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ere", "ere", "Spend the selected currency to upgrade equipment, a hero, or a gem by one level.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"itemId":        schemaProperty("number", "Equipment EID or gem GID instance."),
			"equipmentFlag": schemaProperty("integer", "1 for equipment/hero, 0 for gem."),
			"currency":      schemaProperty("integer", "Captured C2 cost selector, commonly 0."),
		}, "itemId", "equipmentFlag", "currency"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[upgradeItemArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.ItemID <= 0 {
				return nil, toolError("invalid_arguments", "itemId must be positive")
			}
			if args.EquipmentFlag != 0 && args.EquipmentFlag != 1 {
				return nil, toolError("invalid_arguments", "equipmentFlag must be 0 or 1")
			}
			if args.Currency < 0 {
				return nil, toolError("invalid_arguments", "currency must not be negative")
			}
			return onePayload(GameCommands.EREPayload(args.ItemID, args.EquipmentFlag, args.Currency)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("upgrade_menu_refresh", "gnr|ggm|gei|gli", "Run the four-command inventory refresh sequence used before equipment upgrades.", EffectGameQuery,
		objectSchema(map[string]interface{}{}),
		func(raw json.RawMessage) ([]string, error) {
			if _, err := decodeStrict[struct{}](raw); err != nil {
				return nil, err
			}
			return []string{GameCommands.GNRPayload(), GameCommands.GGMPayload(), GameCommands.GEIPayload(), GameCommands.GLIPayload()}, nil
		}); err != nil {
		return err
	}

	if err := builder.add("seq", "seq", "Permanently sell one stored equipment instance.", EffectDestructive,
		objectSchema(map[string]interface{}{"equipmentId": schemaProperty("number", "Stored equipment EID.")}, "equipmentId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[equipmentIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.EquipmentID <= 0 {
				return nil, toolError("invalid_arguments", "equipmentId must be positive")
			}
			return onePayload(GameCommands.SEQPayload(args.EquipmentID)), nil
		}); err != nil {
		return err
	}

	gemSellSchema := objectSchema(map[string]interface{}{"gemId": schemaProperty("number", "Stored gem instance GID.")}, "gemId")
	if err := builder.add("sge_non_relic", "sge", "Permanently sell one non-relic gem instance.", EffectDestructive, gemSellSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[gemIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.GemID <= 0 {
				return nil, toolError("invalid_arguments", "gemId must be positive")
			}
			return onePayload(GameCommands.SGENonRelicGemPayload(args.GemID)), nil
		}); err != nil {
		return err
	}
	if err := builder.add("sge_relic", "sge", "Permanently sell one relic gem instance.", EffectDestructive, gemSellSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[gemIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.GemID <= 0 {
				return nil, toolError("invalid_arguments", "gemId must be positive")
			}
			return onePayload(GameCommands.SGERelicGemPayload(args.GemID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("rpc", "rpc", "Equip a construction item CID onto a building OID in a castle.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"buildingOid":        schemaProperty("integer", "Per-castle building OID."),
			"constructionItemId": schemaProperty("integer", "Construction item definition CID; never a building/decor WOD ID."),
			"slotId":             schemaProperty("integer", "Construction slot SID."),
			"mode":               schemaProperty("integer", "Captured M field, commonly 0."),
			"kingdomId":          schemaProperty("integer", "Map kingdom KID."),
			"castleId":           schemaProperty("integer", "Castle instance AID."),
		}, "buildingOid", "constructionItemId", "slotId", "mode", "kingdomId", "castleId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[equipConstructionItemArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.BuildingOID <= 0 || args.ConstructionItemID <= 0 || args.CastleID <= 0 {
				return nil, toolError("invalid_arguments", "buildingOid, constructionItemId, and castleId must be positive")
			}
			if args.SlotID < 0 || args.KingdomID < 0 {
				return nil, toolError("invalid_arguments", "slotId and kingdomId must not be negative")
			}
			return onePayload(GameCommands.RPCPayload(args.BuildingOID, args.ConstructionItemID, args.SlotID, args.Mode, args.KingdomID, args.CastleID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ubc", "ubc", "Upgrade an equipped construction item using an explicit SUC offer code.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"buildingOid":        schemaProperty("integer", "Per-castle building OID."),
			"upgradeCode":        schemaProperty("integer", "SUC offer/session code, such as 2001 or 2002 from live state."),
			"slotId":             schemaProperty("integer", "Construction slot SID."),
			"kingdomId":          schemaProperty("integer", "Map kingdom KID."),
			"castleId":           schemaProperty("integer", "Castle instance AID."),
			"constructionItemId": schemaProperty("integer", "Current construction item CID."),
		}, "buildingOid", "upgradeCode", "slotId", "kingdomId", "castleId", "constructionItemId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[upgradeConstructionItemArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.BuildingOID <= 0 || args.UpgradeCode <= 0 || args.CastleID <= 0 || args.ConstructionItemID <= 0 {
				return nil, toolError("invalid_arguments", "buildingOid, upgradeCode, castleId, and constructionItemId must be positive")
			}
			if args.SlotID < 0 || args.KingdomID < 0 {
				return nil, toolError("invalid_arguments", "slotId and kingdomId must not be negative")
			}
			return onePayload(GameCommands.UBCPayload(args.BuildingOID, args.UpgradeCode, args.SlotID, args.KingdomID, args.CastleID, args.ConstructionItemID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("gbc", "gbc", "Request trivial construction-item purchase offers for a castle.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"castleId":  schemaProperty("integer", "Castle instance AID; the wire field is named CID but is not a construction item ID."),
			"kingdomId": schemaProperty("integer", "Map kingdom KID."),
		}, "castleId", "kingdomId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[castleKingdomArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			if args.KingdomID < 0 {
				return nil, toolError("invalid_arguments", "kingdomId must not be negative")
			}
			return onePayload(GameCommands.GBCPayload(args.CastleID, args.KingdomID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("sbp", "sbp", "Purchase a product from a GBC offer using every captured pricing field.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"productId":   schemaProperty("integer", "GBC product PID."),
			"buildType":   schemaProperty("integer", "Wire BT field."),
			"typeId":      schemaProperty("integer", "Wire TID field, 116 for trivial construction items."),
			"amount":      schemaProperty("integer", "Positive purchase quantity."),
			"kingdomId":   schemaProperty("integer", "Map kingdom KID."),
			"castleId":    schemaProperty("integer", "Wire AID; -1 is valid for global offers."),
			"priceCode2":  schemaProperty("integer", "Wire PC2 field."),
			"buildAux":    schemaProperty("integer", "Wire BA field."),
			"power":       schemaProperty("integer", "Wire PWR field."),
			"publicOrder": schemaProperty("integer", "Wire _PO field."),
		}, "productId", "buildType", "typeId", "amount", "kingdomId", "castleId", "priceCode2", "buildAux", "power", "publicOrder"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[trivialPurchaseArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.ProductID <= 0 || args.TypeID <= 0 || args.Amount <= 0 {
				return nil, toolError("invalid_arguments", "productId, typeId, and amount must be positive")
			}
			if args.KingdomID < 0 || args.CastleID < -1 {
				return nil, toolError("invalid_arguments", "kingdomId or castleId is invalid")
			}
			return onePayload(GameCommands.SBPPayload(args.ProductID, args.BuildType, args.TypeID, args.Amount, args.KingdomID, args.CastleID, args.PriceCode2, args.BuildAux, args.Power, args.PublicOrder)), nil
		}); err != nil {
		return err
	}

	return nil
}

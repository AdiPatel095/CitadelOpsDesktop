package Toolkit

import (
	"encoding/json"

	"CitadelDesktop/Server/GameCommands"
)

type castleKingdomArgs struct {
	CastleID  int `json:"castleId"`
	KingdomID int `json:"kingdomId"`
}

type tileKingdomArgs struct {
	X         int `json:"x"`
	Y         int `json:"y"`
	KingdomID int `json:"kingdomId"`
}

type viewportArgs struct {
	KingdomID int `json:"kingdomId"`
	X1        int `json:"x1"`
	Y1        int `json:"y1"`
	X2        int `json:"x2"`
	Y2        int `json:"y2"`
}

type aroundTileArgs struct {
	KingdomID int `json:"kingdomId"`
	X         int `json:"x"`
	Y         int `json:"y"`
	Padding   int `json:"padding"`
}

type castleFocusArgs struct {
	KingdomID int `json:"kingdomId"`
	CastleID  int `json:"castleId"`
	MapX      int `json:"mapX"`
	MapY      int `json:"mapY"`
}

type playerIDArgs struct {
	PlayerID int `json:"playerId"`
}

type erectBuildingArgs struct {
	WodID int `json:"wodId"`
	X     int `json:"x"`
	Y     int `json:"y"`
}

type erectBuildingFullArgs struct {
	WodID             int `json:"wodId"`
	X                 int `json:"x"`
	Y                 int `json:"y"`
	Rotation          int `json:"rotation"`
	Power             int `json:"power"`
	PublicOrder       int `json:"publicOrder"`
	DecorationOwnerID int `json:"decorationOwnerId"`
}

type storeBuildingArgs struct {
	CastleID int `json:"castleId"`
	OID      int `json:"oid"`
}

func registerNavigationAndStateCommands(builder *commandSpecBuilder) error {
	castleKingdomSchema := objectSchema(map[string]interface{}{
		"castleId":  schemaProperty("integer", "Player castle instance AID/CID."),
		"kingdomId": schemaProperty("integer", "Map kingdom KID; zero is the main kingdom."),
	}, "castleId", "kingdomId")
	if err := builder.add("jca", "jca", "Focus a player castle by castle instance ID and kingdom.", EffectGameQuery, castleKingdomSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[castleKingdomArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			if err := nonNegative(args.KingdomID, "kingdomId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.JCAPayload(args.CastleID, args.KingdomID)), nil
		}); err != nil {
		return err
	}

	tileSchema := objectSchema(map[string]interface{}{
		"x":         schemaProperty("integer", "World-map X coordinate."),
		"y":         schemaProperty("integer", "World-map Y coordinate."),
		"kingdomId": schemaProperty("integer", "Map kingdom KID; zero is valid."),
	}, "x", "y", "kingdomId")
	if err := builder.add("jaa", "jaa", "Focus a world-map tile and request its castle-area state.", EffectGameQuery, tileSchema,
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[tileKingdomArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.X < 0 || args.Y < 0 || args.KingdomID < 0 {
				return nil, toolError("invalid_arguments", "x, y, and kingdomId must not be negative")
			}
			return onePayload(GameCommands.JAAPayload(args.X, args.Y, args.KingdomID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("gaa_viewport", "gaa", "Request map nodes in an inclusive rectangular viewport.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"kingdomId": schemaProperty("integer", "Map kingdom KID."),
			"x1":        schemaProperty("integer", "Minimum X coordinate."),
			"y1":        schemaProperty("integer", "Minimum Y coordinate."),
			"x2":        schemaProperty("integer", "Maximum X coordinate."),
			"y2":        schemaProperty("integer", "Maximum Y coordinate."),
		}, "kingdomId", "x1", "y1", "x2", "y2"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[viewportArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.X1 < 0 || args.Y1 < 0 || args.X2 < args.X1 || args.Y2 < args.Y1 {
				return nil, toolError("invalid_arguments", "viewport coordinates or kingdomId are invalid")
			}
			return onePayload(GameCommands.GAAPayload(args.KingdomID, args.X1, args.Y1, args.X2, args.Y2)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("gaa_around_tile", "gaa", "Request a square map viewport around one tile.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"kingdomId": schemaProperty("integer", "Map kingdom KID."),
			"x":         schemaProperty("integer", "Center X coordinate."),
			"y":         schemaProperty("integer", "Center Y coordinate."),
			"padding":   schemaProperty("integer", "Chebyshev radius; values below one become one."),
		}, "kingdomId", "x", "y", "padding"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[aroundTileArgs](raw)
			if err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.X < 0 || args.Y < 0 {
				return nil, toolError("invalid_arguments", "x, y, and kingdomId must not be negative")
			}
			if args.Padding < 1 {
				args.Padding = 1
			}
			return onePayload(GameCommands.GAAPayload(args.KingdomID, args.X-args.Padding, args.Y-args.Padding, args.X+args.Padding, args.Y+args.Padding)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("castle_focus", "jaa|jca", "Select JAA or JCA using the same kingdom rules as Citadel automation.", EffectGameQuery,
		objectSchema(map[string]interface{}{
			"kingdomId": schemaProperty("integer", "Map kingdom KID."),
			"castleId":  schemaProperty("integer", "Player castle instance AID."),
			"mapX":      schemaProperty("integer", "Castle map X coordinate for JAA kingdoms."),
			"mapY":      schemaProperty("integer", "Castle map Y coordinate for JAA kingdoms."),
		}, "kingdomId", "castleId", "mapX", "mapY"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[castleFocusArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			if args.KingdomID < 0 || args.MapX < 0 || args.MapY < 0 {
				return nil, toolError("invalid_arguments", "kingdomId, mapX, and mapY must not be negative")
			}
			return onePayload(GameCommands.CastleFocusCommand(args.KingdomID, args.CastleID, args.MapX, args.MapY)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("gdi", "gdi", "Request the current public player summary by player ID.", EffectGameQuery,
		objectSchema(map[string]interface{}{"playerId": schemaProperty("integer", "Player OID.")}, "playerId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[playerIDArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.PlayerID, "playerId"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.GDIPayload(args.PlayerID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ebu", "ebu", "Place a building or decoration in the currently focused castle using default wire fields.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"wodId": schemaProperty("integer", "Global building or decoration WOD ID, not an OID or TCI CID."),
			"x":     schemaProperty("integer", "Castle-grid X coordinate."),
			"y":     schemaProperty("integer", "Castle-grid Y coordinate."),
		}, "wodId", "x", "y"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[erectBuildingArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.WodID, "wodId"); err != nil {
				return nil, err
			}
			if args.X < 0 || args.Y < 0 {
				return nil, toolError("invalid_arguments", "x and y must not be negative")
			}
			return onePayload(GameCommands.EBUPayload(args.WodID, args.X, args.Y)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("ebu_with_params", "ebu", "Place a building or decoration with every captured EBU field explicit.", EffectDestructive,
		objectSchema(map[string]interface{}{
			"wodId":             schemaProperty("integer", "Global building or decoration WOD ID."),
			"x":                 schemaProperty("integer", "Castle-grid X coordinate."),
			"y":                 schemaProperty("integer", "Castle-grid Y coordinate."),
			"rotation":          schemaProperty("integer", "Wire R value."),
			"power":             schemaProperty("integer", "Wire PWR value."),
			"publicOrder":       schemaProperty("integer", "Wire PO value."),
			"decorationOwnerId": schemaProperty("integer", "Wire DOID value."),
		}, "wodId", "x", "y", "rotation", "power", "publicOrder", "decorationOwnerId"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[erectBuildingFullArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.WodID, "wodId"); err != nil {
				return nil, err
			}
			if args.X < 0 || args.Y < 0 {
				return nil, toolError("invalid_arguments", "x and y must not be negative")
			}
			return onePayload(GameCommands.EBUWithParamsPayload(args.WodID, args.X, args.Y, args.Rotation, args.Power, args.PublicOrder, args.DecorationOwnerID)), nil
		}); err != nil {
		return err
	}

	if err := builder.add("sin", "sin", "Refresh decoration and building storage inventory.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.SINPayload)); err != nil {
		return err
	}
	if err := builder.add("gii", "gii", "Refresh construction-item inventory by wire CID.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GIIPayload)); err != nil {
		return err
	}
	if err := builder.add("gam", "gam", "Refresh active troop movements and commander availability.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GAMPayload)); err != nil {
		return err
	}
	if err := builder.add("gei", "gei", "Refresh equipment inventory.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GEIPayload)); err != nil {
		return err
	}
	if err := builder.add("ggm", "ggm", "Refresh gem storage.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GGMPayload)); err != nil {
		return err
	}
	if err := builder.add("gli", "gli", "Refresh equipment leader/loadout data.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GLIPayload)); err != nil {
		return err
	}
	if err := builder.add("gnr", "gnr", "Open or refresh the equipment upgrade menu shell.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.GNRPayload)); err != nil {
		return err
	}
	if err := builder.add("aec", "aec", "Open the construction-item menu shell.", EffectGameQuery, objectSchema(map[string]interface{}{}), noArgumentBuilder(GameCommands.AECPayload)); err != nil {
		return err
	}

	if err := builder.add("sob", "sob", "Pick up a castle building or decoration instance into storage.", EffectGameAction,
		objectSchema(map[string]interface{}{
			"castleId": schemaProperty("integer", "Castle instance AID."),
			"oid":      schemaProperty("integer", "Per-castle building/decor OID, not WOD ID."),
		}, "castleId", "oid"),
		func(raw json.RawMessage) ([]string, error) {
			args, err := decodeStrict[storeBuildingArgs](raw)
			if err != nil {
				return nil, err
			}
			if err := positive(args.CastleID, "castleId"); err != nil {
				return nil, err
			}
			if err := positive(args.OID, "oid"); err != nil {
				return nil, err
			}
			return onePayload(GameCommands.SOBPayload(args.CastleID, args.OID)), nil
		}); err != nil {
		return err
	}

	return nil
}

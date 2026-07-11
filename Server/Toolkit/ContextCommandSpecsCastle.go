package Toolkit

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Models"
)

type contextStoreDecorationArgs struct {
	Castle contextCastleSelector `json:"castle"`
	OID    int                   `json:"oid"`
}

type contextPlaceItemArgs struct {
	Castle contextCastleSelector `json:"castle"`
	WodID  int                   `json:"wodId"`
	X      int                   `json:"x"`
	Y      int                   `json:"y"`
}

func registerContextCastleCommands(builder *contextCommandSpecBuilder) error {
	if err := builder.add(ContextCommandDefinition{
		Name:        "store_decoration",
		Description: "Move a known decoration instance into storage using its castle OID; resolves and refreshes castle focus, verifies the OID, and blocks essential structures.",
		InputSchema: objectSchema(map[string]interface{}{
			"castle": contextCastleSelectorSchema("Castle containing the decoration instance."),
			"oid":    schemaProperty("integer", "Per-castle decoration OID from live building rows."),
		}, "castle", "oid"),
		Effect:   EffectGameAction,
		Resolves: []string{"castle focus", "OID to WOD identity", "decoration eligibility", "castle AID"},
	}, resolveStoreDecoration); err != nil {
		return err
	}

	if err := builder.add(ContextCommandDefinition{
		Name:        "place_item",
		Description: "Place a known building or decoration in a selected castle; resolves focus and applies safe default EBU protocol fields.",
		InputSchema: objectSchema(map[string]interface{}{
			"castle": contextCastleSelectorSchema("Castle where the item should be placed."),
			"wodId":  schemaProperty("integer", "Known building or decoration WOD ID."),
			"x":      schemaProperty("integer", "Castle-grid X coordinate."),
			"y":      schemaProperty("integer", "Castle-grid Y coordinate."),
		}, "castle", "wodId", "x", "y"),
		Effect:   EffectDestructive,
		Resolves: []string{"castle focus", "WOD catalog identity", "EBU rotation/power/public-order defaults"},
	}, resolvePlaceItem); err != nil {
		return err
	}

	return builder.add(ContextCommandDefinition{
		Name:        "open_tci_offers",
		Description: "Open construction-item menus and request trivial-item purchase offers for a selected castle without requiring AEC/GBC protocol fields.",
		InputSchema: objectSchema(map[string]interface{}{
			"castle": contextCastleSelectorSchema("Castle whose trivial construction-item offers should be requested."),
		}, "castle"),
		Effect:   EffectGameQuery,
		Resolves: []string{"castle focus", "GBC castle CID meaning", "kingdom KID", "AEC/GBC sequence"},
	}, resolveOpenTCIOffers)
}

func resolveStoreDecoration(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextStoreDecorationArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.OID <= 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "oid must be positive")
	}
	castle, resolutions, err := resolveContextCastle(args.Castle)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	castleInfo := Models.GetGameState().GetCastleByID(castle.CastleID)
	if castleInfo == nil || !castleInfo.BuildingRowsLoaded {
		return ContextCommandPlan{}, contextCommandError("store_decoration", "castle %d building rows are not loaded; focus it first", castle.CastleID)
	}
	var building *Models.BuildingData
	for _, row := range castleInfo.AllBuildingRows() {
		if row.OID == args.OID {
			copyRow := row
			building = &copyRow
			break
		}
	}
	if building == nil {
		return ContextCommandPlan{}, toolError("not_found", "OID %d is not present in castle %d building rows", args.OID, castle.CastleID)
	}
	if !Models.IsDecorationPickupCandidateWID(building.BuildingID) || Models.DecorationSOBBlockedWID(building.BuildingID) {
		return ContextCommandPlan{}, toolError("unsafe_context", "OID %d (%s, WOD %d) is not an eligible decoration pickup", building.OID, building.Name, building.BuildingID)
	}

	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimGameFocus),
		contextCastleClaim(castle.CastleID, "layout"),
		Automation.ExclusiveClaim("account:storage"),
	}, Automation.StateFocus, Automation.StateCastles, Automation.StateInventory, Automation.StateOpcode("sob"))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions,
		resolution("decoration.oid", building.OID, "state.castle.buildingRows"),
		resolution("decoration.wodId", building.BuildingID, "state.castle.buildingRows"),
		resolution("decoration.name", building.Name, "state.castle.buildingRows"),
		resolution("decoration.pickupEligible", true, "catalog.decorationSafety"),
	)
	plan.Steps = append(plan.Steps,
		focusCastleStep(castle),
		primitiveCommand("sob", storeBuildingArgs{CastleID: castle.CastleID, OID: building.OID}, fmt.Sprintf("Store %s (OID %d)", building.Name, building.OID)),
	)
	return plan, nil
}

func resolvePlaceItem(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
	args, err := decodeStrict[contextPlaceItemArgs](raw)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	if args.WodID <= 0 || args.X < 0 || args.Y < 0 {
		return ContextCommandPlan{}, toolError("invalid_arguments", "wodId must be positive and grid coordinates must not be negative")
	}
	buildingInfo := Models.GetBuildingInfo(args.WodID)
	knownDecoration := Models.IsKnownDecorationWID(args.WodID)
	if buildingInfo.Name == "Unknown" && !knownDecoration {
		return ContextCommandPlan{}, toolError("invalid_arguments", "wodId %d is not a known building or decoration", args.WodID)
	}
	itemName := buildingInfo.Name
	if knownDecoration {
		if name, ok := Models.DecorationDisplayName(args.WodID); ok {
			itemName = name
		}
	}
	castle, resolutions, err := resolveContextCastle(args.Castle)
	if err != nil {
		return ContextCommandPlan{}, err
	}
	plan := contextPlan([]Automation.Claim{
		Automation.ExclusiveClaim(Automation.ClaimGameFocus),
		contextCastleClaim(castle.CastleID, "layout"),
		Automation.ExclusiveClaim(Automation.ClaimAccountResources),
	}, Automation.StateFocus, Automation.StateCastles, Automation.StateResources, Automation.StateOpcode("ebu"))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions,
		resolution("item.name", itemName, "catalog.buildingsDecorations"),
		resolution("item.protocolDefaults", map[string]int{"rotation": 0, "power": 0, "publicOrder": -1, "decorationOwnerId": -1}, "protocol.default"),
	)
	plan.Steps = append(plan.Steps,
		focusCastleStep(castle),
		primitiveCommand("ebu", erectBuildingArgs{WodID: args.WodID, X: args.X, Y: args.Y}, fmt.Sprintf("Place %s at grid %d,%d", itemName, args.X, args.Y)),
	)
	return plan, nil
}

func resolveOpenTCIOffers(_ context.Context, raw json.RawMessage) (ContextCommandPlan, error) {
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
		contextCastleClaim(castle.CastleID, "tci-shop"),
		Automation.ExclusiveClaim(Automation.ClaimTCIInventory),
	}, Automation.StateFocus, Automation.StateInventory, Automation.StateOpcode("aec"), Automation.StateOpcode("gbc"))
	plan.Resolutions = append(plan.Resolutions, resolutions...)
	plan.Resolutions = append(plan.Resolutions, resolution(
		"gbc.castleId",
		castle.CastleID,
		"state.castle.aid",
		"GBC wire CID is the castle instance AID, not a construction-item CID",
	))
	plan.Steps = append(plan.Steps,
		focusCastleStep(castle),
		primitiveCommand("aec", struct{}{}, "Open the construction-item menu shell"),
		primitiveCommand("gbc", castleKingdomArgs{CastleID: castle.CastleID, KingdomID: castle.KingdomID}, "Request trivial construction-item offers"),
	)
	return plan, nil
}

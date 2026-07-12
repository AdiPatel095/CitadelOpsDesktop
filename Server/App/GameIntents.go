package App

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func (application *Application) registerGameIntents() error {
	if err := application.Intents.RegisterAction("movement.track_station", application.trackStationMovement); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("equipment.verify_coin_reserve", application.verifyEquipmentCoinReserve); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("rift.template.rename", application.renameRiftTemplate); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("rift.template.delete", application.deleteRiftTemplate); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "game.refresh_movements", Description: "Request a fresh movement snapshot", Effect: Intent.EffectRead,
			Planner: func(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
				return Intent.Plan{
					Claims: []string{"game:movements"}, Summary: "Refresh movements",
					Steps: []Intent.Step{commandStep("Refresh movements", "gam", json.RawMessage(`{}`), "gam")},
				}, nil
			},
		},
		{
			Name: "troops.station", Description: "Station validated troop stacks at a same-kingdom alliance holding", Effect: Intent.EffectLaunch,
			Planner: planTroopsStation,
		},
		{
			Name: "movement.recall", Description: "Recall an active player-owned movement", Effect: Intent.EffectLaunch,
			Planner: planMovementRecall,
		},
		{
			Name: "equipment.refresh", Description: "Refresh loadouts, equipment storage, and gem storage", Effect: Intent.EffectRead,
			Planner: planEquipmentRefresh,
		},
		{
			Name: "equipment.equip", Description: "Equip a validated storage item on a commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentEquip,
		},
		{
			Name: "equipment.unequip", Description: "Unequip one or more validated items from a commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentUnequip,
		},
		{
			Name: "equipment.gem.equip", Description: "Socket a validated relic gem into equipped gear", Effect: Intent.EffectWrite,
			Planner: planGemEquip,
		},
		{
			Name: "equipment.gem.unequip", Description: "Remove a socketed gem from equipped gear", Effect: Intent.EffectWrite,
			Planner: planGemUnequip,
		},
		{
			Name: "equipment.swap", Description: "Swap base equipment and attached gems between two leaders", Effect: Intent.EffectWrite,
			Planner: planEquipmentSwap,
		},
		{
			Name: "equipment.reconfigure", Description: "Apply a validated optimizer loadout to one commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentReconfigure,
		},
		{
			Name: "equipment.upgrade", Description: "Upgrade equipment or a relic gem to a validated target level", Effect: Intent.EffectWrite,
			Planner: application.planEquipmentUpgrade,
		},
		{
			Name: "equipment.sell", Description: "Sell a deterministic equipment or gem storage selection", Effect: Intent.EffectWrite,
			Planner: planEquipmentSell,
		},
		{
			Name: "game.focus_castle", Description: "Focus and refresh one of the player's castles", Effect: Intent.EffectRead,
			Planner: planCastleFocus,
		},
		{
			Name: "alliance.refresh", Description: "Refresh the current alliance and member state", Effect: Intent.EffectRead,
			Planner: planAllianceRefresh,
		},
		{
			Name: "map.query", Description: "Query an inclusive rectangular world-map viewport", Effect: Intent.EffectRead,
			Planner: planMapQuery,
		},
		{
			Name: "construction.equip", Description: "Equip an official construction-item definition on a castle building", Effect: Intent.EffectWrite,
			Planner: planConstructionEquip,
		},
		{
			Name: "construction.upgrade", Description: "Upgrade the construction item currently equipped in a building slot", Effect: Intent.EffectWrite,
			Planner: planConstructionUpgrade,
		},
		{
			Name: "construction.shop", Description: "Request the live construction-item offers for a castle", Effect: Intent.EffectRead,
			Planner: planConstructionShop,
		},
		{
			Name: "crafting.refresh", Description: "Request all sovereign crafting queues and research entitlements", Effect: Intent.EffectRead,
			Planner: func(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
				return Intent.Plan{
					Claims: []string{"game:crafting"}, Summary: "Refresh crafting queues",
					Steps: []Intent.Step{commandStep("Refresh crafting queues", "crin", json.RawMessage(`{}`), "crin")},
				}, nil
			},
		},
		{
			Name: "crafting.start", Description: "Start or queue one official crafting recipe", Effect: Intent.EffectWrite,
			Planner: planCraftingStart,
		},
		{
			Name: "production.enqueue", Description: "Enqueue an official troop or tool definition using observed production context", Effect: Intent.EffectWrite,
			Planner: planProductionEnqueue,
		},
		{
			Name: "hospital.heal", Description: "Heal a non-premium wounded unit stack at an owned castle", Effect: Intent.EffectWrite,
			Planner: planHospitalHeal,
		},
		{
			Name: "hospital.discard", Description: "Discard a wounded unit stack at an owned castle", Effect: Intent.EffectWrite,
			Planner: planHospitalDiscard,
		},
		{
			Name: "spy.launch", Description: "Launch a military espionage mission from an owned castle", Effect: Intent.EffectLaunch,
			Planner: planSpyLaunch,
		},
		{
			Name: "rift.maiden_wave.launch", Description: "Launch deterministic Rift probe waves with eligible shield-maiden commanders", Effect: Intent.EffectLaunch,
			Planner: planMaidenCommsWave,
		},
		{
			Name: "rift.launch.replay", Description: "Replay a captured Rift attack template", Effect: Intent.EffectLaunch,
			Planner: application.planRiftReplay,
		},
		{
			Name: "rift.template.rename", Description: "Rename a captured Rift attack template", Effect: Intent.EffectWrite,
			Planner: planRiftTemplateRename,
		},
		{
			Name: "rift.template.delete", Description: "Delete a captured Rift attack template and cancel its schedule", Effect: Intent.EffectWrite,
			Planner: planRiftTemplateDelete,
		},
		{
			Name: "decoration.apply_preset", Description: "Reconcile one castle's decoration layout with an official-definition preset", Effect: Intent.EffectWrite,
			Planner: planDecorationPreset,
		},
		{
			Name: "report.spy.fetch", Description: "Fetch one spy report from an observed inbox notice", Effect: Intent.EffectRead,
			Planner: planSpyReportFetch,
		},
		{
			Name: "report.battle.summary", Description: "Fetch one battle report summary from an observed inbox notice", Effect: Intent.EffectRead,
			Planner: planBattleReportSummary,
		},
		{
			Name: "report.battle.details", Description: "Fetch battle waves, units, and tools using summary-derived report context", Effect: Intent.EffectRead,
			Planner: planBattleReportDetails,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planCraftingStart(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		RecipeID           int64                    `json:"recipeId"`
		Power              int                      `json:"power,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	building, ok := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !ok || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID)
	}
	if request.RecipeID <= 0 || input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("recipeId must reference the loaded official catalog")
	}
	catalog, err := input.GameData.Catalog("craftingRecipes")
	if err != nil {
		return Intent.Plan{}, err
	}
	rawRecipe, exists := catalog.Find(strconv.FormatInt(request.RecipeID, 10))
	if !exists {
		return Intent.Plan{}, fmt.Errorf("crafting recipe %d is not in the current official catalog", request.RecipeID)
	}
	recipe, err := GameData.DecodeRecord(rawRecipe)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("decode crafting recipe %d: %w", request.RecipeID, err)
	}
	queueTypeID, _ := recipe.Int64("queueTypeId")
	if int(queueTypeID) != building.QueueTypeID {
		return Intent.Plan{}, fmt.Errorf("crafting recipe %d belongs to queue %d, not queue %d", request.RecipeID, queueTypeID, building.QueueTypeID)
	}
	if required, _ := recipe.String("requiredCraftingBuildings"); strings.TrimSpace(required) != "" {
		allowed := false
		for _, part := range strings.FieldsFunc(required, func(character rune) bool { return character == ',' || character == '#' }) {
			definitionID, parseErr := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if parseErr == nil && State.BuildingID(definitionID) == building.DefinitionID {
				allowed = true
				break
			}
		}
		if !allowed {
			return Intent.Plan{}, fmt.Errorf("crafting recipe %d is not valid for building definition %d", request.RecipeID, building.DefinitionID)
		}
	}
	payload, _ := json.Marshal(struct {
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
		BuildingID State.BuildingInstanceID `json:"OID"`
		Power      int                      `json:"PWR"`
		RecipeID   int64                    `json:"CRID"`
	}{castle.KingdomID, castle.ID, building.InstanceID, request.Power, request.RecipeID})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"crafting-building:" + strconv.FormatInt(int64(building.InstanceID), 10),
		},
		Summary: fmt.Sprintf("Queue crafting recipe %d at %s", request.RecipeID, castleLabel(castle)),
		Steps:   []Intent.Step{commandStep("Queue crafting recipe", "crst", payload, "crst")},
	}, nil
}

func planCastleFocus(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	return Intent.Plan{
		Claims: []string{"castle-focus"}, Summary: fmt.Sprintf("Focus %s", castleLabel(castle)),
		Steps: []Intent.Step{castleFocusStep(castle)},
	}, nil
}

func planAllianceRefresh(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	if input.State.Alliance.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("the current player's alliance is not known")
	}
	payload, _ := json.Marshal(struct {
		AllianceID State.AllianceID `json:"AID"`
	}{AllianceID: input.State.Alliance.ID})
	return Intent.Plan{
		Claims: []string{"alliance"}, Summary: "Refresh alliance",
		Steps: []Intent.Step{commandStep("Refresh alliance", "ain", payload, "ain")},
	}, nil
}

func planMapQuery(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		KingdomID State.KingdomID `json:"kingdomId"`
		X1        int             `json:"x1"`
		Y1        int             `json:"y1"`
		X2        int             `json:"x2"`
		Y2        int             `json:"y2"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.X1 > request.X2 || request.Y1 > request.Y2 {
		return Intent.Plan{}, fmt.Errorf("map bounds must be ordered from minimum to maximum")
	}
	if request.X2-request.X1 > 50 || request.Y2-request.Y1 > 50 {
		return Intent.Plan{}, fmt.Errorf("map query dimensions may not exceed 51 by 51 tiles")
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{request.KingdomID, request.X1, request.Y1, request.X2, request.Y2})
	return Intent.Plan{
		Claims:  []string{"map:" + strconv.FormatInt(int64(request.KingdomID), 10)},
		Summary: fmt.Sprintf("Query map %d (%d,%d)-(%d,%d)", request.KingdomID, request.X1, request.Y1, request.X2, request.Y2),
		Steps:   []Intent.Step{commandStep("Query map", "gaa", payload, "gaa")},
	}, nil
}

func planConstructionEquip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		DefinitionID       State.ConstructionItemID `json:"constructionItemId"`
		Slot               int                      `json:"slot"`
		Mode               int                      `json:"mode,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := constructionContext(input, request.CastleID, request.BuildingInstanceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.DefinitionID <= 0 || input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("constructionItemId must reference the loaded official catalog")
	}
	catalog, err := input.GameData.Catalog("constructionItems")
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, exists := catalog.Find(strconv.FormatInt(int64(request.DefinitionID), 10)); !exists {
		return Intent.Plan{}, fmt.Errorf("construction item %d is not in the current official catalog", request.DefinitionID)
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		ItemID     State.ConstructionItemID `json:"CID"`
		Slot       int                      `json:"SID"`
		Mode       int                      `json:"M"`
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
	}{request.BuildingInstanceID, request.DefinitionID, request.Slot, request.Mode, castle.KingdomID, castle.ID})
	steps := castleContextSteps(castle)
	steps = append(steps, commandStep("Equip construction item", "rpc", payload, "rpc"))
	return Intent.Plan{
		Claims:  constructionClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Equip construction item %d on building %d", request.DefinitionID, request.BuildingInstanceID),
		Steps:   steps,
	}, nil
}

func planConstructionUpgrade(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		Slot               int                      `json:"slot"`
		OfferCode          int                      `json:"offerCode"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := constructionContext(input, request.CastleID, request.BuildingInstanceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.OfferCode <= 0 {
		return Intent.Plan{}, fmt.Errorf("offerCode must come from the current live upgrade offer")
	}
	var equipped State.ConstructionItemID
	for _, slot := range castle.ConstructionSlots[request.BuildingInstanceID] {
		if slot.Slot == request.Slot {
			equipped = slot.DefinitionID
			break
		}
	}
	if equipped <= 0 {
		return Intent.Plan{}, fmt.Errorf("building %d has no construction item in slot %d", request.BuildingInstanceID, request.Slot)
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		OfferCode  int                      `json:"SUC"`
		Slot       int                      `json:"SID"`
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
		ItemID     State.ConstructionItemID `json:"CID"`
	}{request.BuildingInstanceID, request.OfferCode, request.Slot, castle.KingdomID, castle.ID, equipped})
	steps := castleContextSteps(castle)
	steps = append(steps, commandStep("Upgrade construction item", "ubc", payload, "ubc"))
	return Intent.Plan{
		Claims:  constructionClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Upgrade construction item %d on building %d", equipped, request.BuildingInstanceID),
		Steps:   steps,
	}, nil
}

func planConstructionShop(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, castle.KingdomID})
	steps := castleContextSteps(castle)
	steps = append(steps, commandStep("Load construction-item offers", "gbc", payload, "gbc"))
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10), "construction-shop"},
		Summary: fmt.Sprintf("Load construction-item offers for %s", castleLabel(castle)), Steps: steps,
	}, nil
}

func constructionContext(input Intent.PlanningContext, castleID State.CastleID, buildingID State.BuildingInstanceID) (State.CastleState, error) {
	castle, ok := input.State.Castles[castleID]
	if !ok || castleID <= 0 {
		return State.CastleState{}, fmt.Errorf("castle %d is not in the current player state", castleID)
	}
	if _, ok := castle.Buildings[buildingID]; !ok || buildingID <= 0 {
		return State.CastleState{}, fmt.Errorf("building instance %d is not in castle %d", buildingID, castleID)
	}
	return castle, nil
}

func castleContextSteps(castle State.CastleState) []Intent.Step {
	if castle.Focused {
		return nil
	}
	return []Intent.Step{castleFocusStep(castle)}
}

func castleFocusStep(castle State.CastleState) Intent.Step {
	payload, _ := json.Marshal(struct {
		X         int             `json:"PX"`
		Y         int             `json:"PY"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.X, castle.Y, castle.KingdomID})
	return commandStep("Focus castle", "jaa", payload, "jaa")
}

func commandStep(name string, opcode string, payload json.RawMessage, awaitOpcode string) Intent.Step {
	return Intent.Step{
		Name: name, Opcode: opcode, AwaitOpcode: awaitOpcode, TimeoutMillis: 10_000, SuccessCodes: []int{0},
		Command: Protocol.Command{Opcode: opcode, Payload: payload},
	}
}

func constructionClaims(castleID State.CastleID, buildingID State.BuildingInstanceID) []string {
	return []string{
		"castle-focus", "castle:" + strconv.FormatInt(int64(castleID), 10),
		"building:" + strconv.FormatInt(int64(buildingID), 10),
	}
}

func castleLabel(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	return "castle " + strconv.FormatInt(int64(castle.ID), 10)
}

func decodeIntentArguments(raw json.RawMessage, destination any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode intent arguments: %w", err)
	}
	return nil
}

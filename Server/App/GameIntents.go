package App

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func (application *Application) registerGameIntents() error {
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
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
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

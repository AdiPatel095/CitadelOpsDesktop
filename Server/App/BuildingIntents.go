package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	buildingMutationConstruct   = "construct"
	buildingMutationCollectGift = "collect_expansion_gift"
	buildingMutationExpand      = "expand"
	buildingMutationPlace       = "place"
	buildingMutationMove        = "move"
	buildingMutationUpgrade     = "upgrade"
	buildingMutationFinishFree  = "finish_free"
	buildingMutationSkipTime    = "skip_time"
	buildingMutationStore       = "store"
	buildingMutationDemolish    = "demolish"
)

type buildingPlacementIntentRequest struct {
	CastleID         State.CastleID     `json:"castleId"`
	DefinitionID     State.BuildingID   `json:"definitionId"`
	X                int                `json:"x"`
	Y                int                `json:"y"`
	Rotation         int                `json:"rotation,omitempty"`
	EventID          *int64             `json:"eventId,omitempty"`
	MapID            *int64             `json:"mapId,omitempty"`
	ResourceReserves map[string]float64 `json:"resourceReserves,omitempty"`
	AllowPremium     bool               `json:"allowPremium,omitempty"`
}

type buildingPlacementResolverArguments struct {
	Kind    string                         `json:"kind"`
	Request buildingPlacementIntentRequest `json:"request"`
}

type buildingExpansionIntentRequest struct {
	CastleID         State.CastleID     `json:"castleId"`
	X                int                `json:"x"`
	Y                int                `json:"y"`
	Direction        int                `json:"direction"`
	Payment          string             `json:"payment,omitempty"`
	ResourceReserves map[string]float64 `json:"resourceReserves,omitempty"`
	AllowPremium     bool               `json:"allowPremium,omitempty"`
}

type buildingInstanceIntentRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
}

type buildingMoveIntentRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	X                  int                      `json:"x"`
	Y                  int                      `json:"y"`
	Rotation           int                      `json:"rotation,omitempty"`
}

type buildingUpgradeIntentRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	EventID            *int64                   `json:"eventId,omitempty"`
	MapID              *int64                   `json:"mapId,omitempty"`
	ResourceReserves   map[string]float64       `json:"resourceReserves,omitempty"`
	AllowPremium       bool                     `json:"allowPremium,omitempty"`
}

type buildingTimeSkipIntentRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	Minutes            int                      `json:"minutes"`
	MinimumRemaining   int64                    `json:"minimumRemaining,omitempty"`
}

type buildingTimeSkipOption struct {
	Minutes    int
	CurrencyID State.CurrencyID
	WireKey    string
}

type buildingVerification struct {
	Kind                     string                   `json:"kind"`
	CastleID                 State.CastleID           `json:"castleId"`
	BuildingInstanceID       State.BuildingInstanceID `json:"buildingInstanceId,omitempty"`
	DefinitionID             State.BuildingID         `json:"definitionId,omitempty"`
	TargetDefinitionID       State.BuildingID         `json:"targetDefinitionId,omitempty"`
	InitialConstructionState int                      `json:"initialConstructionState,omitempty"`
	SkipCurrencyID           State.CurrencyID         `json:"skipCurrencyId,omitempty"`
	InitialSkipBalance       float64                  `json:"initialSkipBalance,omitempty"`
	InitialProgressSec       int64                    `json:"initialProgressSec,omitempty"`
	InitialGroundCount       int                      `json:"initialGroundCount,omitempty"`
	X                        int                      `json:"x,omitempty"`
	Y                        int                      `json:"y,omitempty"`
	Rotation                 int                      `json:"rotation,omitempty"`
}

func (application *Application) registerBuildingIntents() error {
	for name, resolver := range map[string]Intent.StepResolver{
		"building.expand.build":                 resolveBuildingExpansionStep,
		"building.collect_expansion_gift.build": resolveBuildingCollectExpansionGiftStep,
		"building.placement.build":              resolveBuildingPlacementStep,
		"building.move.build":                   resolveBuildingMoveStep,
		"building.upgrade.build":                resolveBuildingUpgradeStep,
		"building.finish_free.build":            resolveBuildingFinishFreeStep,
		"building.skip_time.build":              resolveBuildingTimeSkipStep,
		"building.store.build":                  resolveBuildingStoreStep,
		"building.demolish.build":               resolveBuildingDemolishStep,
	} {
		if err := application.Intents.RegisterStepResolver(name, resolver); err != nil {
			return err
		}
	}
	if err := application.Intents.RegisterAction("building.verify", application.verifyBuildingMutation); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "building.refresh", Description: "Focus a castle and rebuild its layout, construction queue, resources, and storage snapshot", Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717}`), Planner: planBuildingRefresh, ReadSet: buildingReadSet,
		},
		{
			Name: "building.expand", Description: "Buy the next official castle expansion at an exact captured position after validating cost and storage capacity", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"x":220,"y":220,"direction":1,"payment":"resources"}`),
			Planner:          planBuildingExpansion, ReadSet: buildingReadSet,
		},
		{
			Name: "building.collect_expansion_gift", Description: "Collect one validated expansion treasure chest that blocks castle placement", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"buildingInstanceId":46}`),
			Planner:          planBuildingCollectExpansionGift, ReadSet: buildingReadSet,
		},
		{
			Name: "building.construct", Description: "Construct one validated official building definition without implicit premium spending", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"definitionId":301,"x":200,"y":200,"rotation":0}`),
			Planner:          planBuildingConstruct, ReadSet: buildingReadSet,
		},
		{
			Name: "building.place", Description: "Place one validated official definition already present in ordinary castle storage", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"definitionId":301,"x":200,"y":200,"rotation":0}`),
			Planner:          planBuildingPlace, ReadSet: buildingReadSet,
		},
		{
			Name: "building.move", Description: "Move or rotate one movable castle object after collision validation", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":835,"x":195,"y":220,"rotation":1}`),
			Planner:          planBuildingMove, ReadSet: buildingReadSet,
		},
		{
			Name: "building.upgrade", Description: "Start the next official upgrade for one normal castle object when its queue and costs are valid", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430}`),
			Planner:          planBuildingUpgrade, ReadSet: buildingReadSet,
		},
		{
			Name: "building.finish_free", Description: "Finish one queued building operation only through the server's explicit free-skip path", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430}`),
			Planner:          planBuildingFinishFree, ReadSet: buildingReadSet,
		},
		{
			Name: "building.skip_time", Description: "Consume one selected non-premium time-skip currency on a queued building operation", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430,"minutes":10,"minimumRemaining":5}`),
			Planner:          planBuildingTimeSkip, ReadSet: buildingReadSet,
		},
		{
			Name: "building.store", Description: "Move one storeable, idle castle object into ordinary castle storage", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":835}`),
			Planner:          planBuildingStore, ReadSet: buildingReadSet,
		},
		{
			Name: "building.demolish", Description: "Start demolition of one destructible normal castle object using a free construction slot", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":835}`),
			Planner:          planBuildingDemolish, ReadSet: buildingReadSet,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planBuildingExpansion(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingExpansionIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, definition, initialGroundCount, payment, err := validatedBuildingExpansion(
		input, request, buildingStateIsFresh(input.State, request.CastleID),
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationExpand, CastleID: castle.ID, InitialGroundCount: initialGroundCount,
		X: request.X, Y: request.Y, Rotation: request.Direction,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Buy castle expansion", "building.expand.build", resolverArguments, "ebe"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify castle expansion", Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingCastleClaims(castle.ID), buildingPositionClaim(castle.ID, request.X, request.Y), "account-resources")
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Buy expansion level %d at %d,%d direction %d in %s with %s", definition.Level, request.X, request.Y, request.Direction, castleLabel(castle), payment),
		Steps:   steps,
	}, nil
}

func resolveBuildingExpansionStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingExpansionIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	_, _, _, payment, err := validatedBuildingExpansion(input, request, true)
	if err != nil {
		return Intent.Step{}, err
	}
	paymentType := 1
	if payment == Buildings.ExpansionPaymentPremium {
		paymentType = 0
	}
	payload, _ := json.Marshal(struct {
		X           int `json:"X"`
		Y           int `json:"Y"`
		Direction   int `json:"R"`
		PaymentType int `json:"CT"`
	}{request.X, request.Y, request.Direction, paymentType})
	return buildingMutationStep("Buy castle expansion", "ebe", payload), nil
}

func planBuildingCollectExpansionGift(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, _, err := validatedBuildingExpansionGift(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationCollectGift, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Collect expansion gift", "building.collect_expansion_gift.build", resolverArguments, "etc"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify expansion gift collection", Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Collect expansion gift %d in %s", request.BuildingInstanceID, castleLabel(castle)),
		Steps:   steps,
	}, nil
}

func resolveBuildingCollectExpansionGiftStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingExpansionGift(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
	}{request.BuildingInstanceID})
	return buildingMutationStep("Collect expansion gift", "etc", payload), nil
}

func planBuildingRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := buildingCastle(input.State, request.CastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims: buildingCastleClaims(castle.ID), Summary: fmt.Sprintf("Refresh building state for %s", castleLabel(castle)),
		Steps: castleContextSteps(castle),
	}, nil
}

func planBuildingConstruct(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planBuildingPlacement(input, arguments, buildingMutationConstruct)
}

func planBuildingPlace(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planBuildingPlacement(input, arguments, buildingMutationPlace)
}

func planBuildingPlacement(input Intent.PlanningContext, arguments json.RawMessage, kind string) (Intent.Plan, error) {
	var request buildingPlacementIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, definition, err := validatedBuildingPlacement(input, request, kind, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(buildingPlacementResolverArguments{Kind: kind, Request: request})
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: kind, CastleID: castle.ID, DefinitionID: request.DefinitionID,
		X: request.X, Y: request.Y, Rotation: request.Rotation,
	})
	name := "Construct building"
	summary := fmt.Sprintf("Construct %s at %d,%d in %s", definition.DisplayName, request.X, request.Y, castleLabel(castle))
	if kind == buildingMutationPlace {
		name = "Place stored building"
		summary = fmt.Sprintf("Place stored %s at %d,%d in %s", definition.DisplayName, request.X, request.Y, castleLabel(castle))
	}
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep(name, "building.placement.build", resolverArguments, "ebu"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building placement", Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingCastleClaims(castle.ID), buildingPositionClaim(castle.ID, request.X, request.Y))
	if kind == buildingMutationPlace {
		claims = append(claims, "inventory:storage")
	}
	return Intent.Plan{Claims: claims, Summary: summary, Steps: steps}, nil
}

func resolveBuildingPlacementStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var resolver buildingPlacementResolverArguments
	if err := decodeIntentArguments(arguments, &resolver); err != nil {
		return Intent.Step{}, err
	}
	_, _, err := validatedBuildingPlacement(input, resolver.Request, resolver.Kind, true)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		DefinitionID State.BuildingID `json:"WID"`
		X            int              `json:"X"`
		Y            int              `json:"Y"`
		Rotation     int              `json:"R"`
		Power        int              `json:"PWR"`
		Offer        int              `json:"PO"`
		DistrictID   int              `json:"DOID"`
	}{resolver.Request.DefinitionID, resolver.Request.X, resolver.Request.Y, resolver.Request.Rotation, 0, -1, -1})
	name := "Construct building"
	if resolver.Kind == buildingMutationPlace {
		name = "Place stored building"
	}
	return buildingMutationStep(name, "ebu", payload), nil
}

func planBuildingMove(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingMoveIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, _, err := validatedBuildingMove(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationMove, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		X: request.X, Y: request.Y, Rotation: request.Rotation,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Move building", "building.move.build", resolverArguments, "emo"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building position", Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID), buildingPositionClaim(castle.ID, request.X, request.Y))
	return Intent.Plan{
		Claims: claims, Summary: fmt.Sprintf("Move building %d to %d,%d in %s", request.BuildingInstanceID, request.X, request.Y, castleLabel(castle)),
		Steps: steps,
	}, nil
}

func resolveBuildingMoveStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingMoveIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingMove(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		X          int                      `json:"X"`
		Y          int                      `json:"Y"`
		Rotation   int                      `json:"R"`
	}{request.BuildingInstanceID, request.X, request.Y, request.Rotation})
	return buildingMutationStep("Move building", "emo", payload), nil
}

func planBuildingUpgrade(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingUpgradeIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, target, err := validatedBuildingUpgrade(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationUpgrade, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		TargetDefinitionID: State.BuildingID(target.ID),
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Upgrade building", "building.upgrade.build", resolverArguments, "eup"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building upgrade", Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Upgrade building %d to %s in %s", request.BuildingInstanceID, target.DisplayName, castleLabel(castle)),
		Steps:   steps,
	}, nil
}

func resolveBuildingUpgradeStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingUpgradeIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingUpgrade(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		Power      int                      `json:"PWR"`
		Offer      int                      `json:"PO"`
	}{request.BuildingInstanceID, 0, -1})
	return buildingMutationStep("Upgrade building", "eup", payload), nil
}

func planBuildingFinishFree(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, building, _, err := validatedBuildingFinishFree(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationFinishFree, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		InitialConstructionState: building.ConstructionState,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Finish building operation for free", "building.finish_free.build", resolverArguments, "fco"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify free building completion", Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Finish queued building operation %d for free in %s", request.BuildingInstanceID, castleLabel(castle)),
		Steps:   steps,
	}, nil
}

func resolveBuildingFinishFreeStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingFinishFree(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		FreeSkip   int                      `json:"FS"`
	}{request.BuildingInstanceID, 1})
	return buildingMutationStep("Finish building operation for free", "fco", payload), nil
}

func planBuildingTimeSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingTimeSkipIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, building, option, balance, err := validatedBuildingTimeSkip(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationSkipTime, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		InitialConstructionState: building.ConstructionState, SkipCurrencyID: option.CurrencyID,
		InitialSkipBalance: balance, InitialProgressSec: building.ProgressSec,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Apply building time skip", "building.skip_time.build", resolverArguments, "msb"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building time skip", Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		"currency:"+strconv.FormatInt(int64(option.CurrencyID), 10))
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Apply one %d-minute skip to building operation %d in %s", option.Minutes, request.BuildingInstanceID, castleLabel(castle)),
		Steps:   steps,
	}, nil
}

func resolveBuildingTimeSkipStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingTimeSkipIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	_, _, option, _, err := validatedBuildingTimeSkip(input, request, true)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		MinuteSkip string                   `json:"MST"`
	}{request.BuildingInstanceID, option.WireKey})
	return buildingMutationStep("Apply building time skip", "msb", payload), nil
}

func planBuildingStore(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, building, _, err := validatedBuildingStore(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationStore, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		DefinitionID: building.DefinitionID,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Store building", "building.store.build", resolverArguments, "sob"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify stored building", Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID), "inventory:storage")
	return Intent.Plan{
		Claims: claims, Summary: fmt.Sprintf("Store building %d from %s", request.BuildingInstanceID, castleLabel(castle)), Steps: steps,
	}, nil
}

func resolveBuildingStoreStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingStore(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
	}{request.BuildingInstanceID})
	return buildingMutationStep("Store building", "sob", payload), nil
}

func planBuildingDemolish(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, _, err := validatedBuildingDemolish(input, request, buildingStateIsFresh(input.State, request.CastleID))
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationDemolish, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
	})
	steps := castleContextSteps(castle)
	steps = append(steps, buildingResolverStep("Demolish building", "building.demolish.build", resolverArguments, "edo"))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building demolition", Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Demolish building %d in %s", request.BuildingInstanceID, castleLabel(castle)), Steps: steps,
	}, nil
}

func resolveBuildingDemolishStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingDemolish(input, request, true); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
	}{request.BuildingInstanceID})
	return buildingMutationStep("Demolish building", "edo", payload), nil
}

func validatedBuildingExpansion(
	input Intent.PlanningContext,
	request buildingExpansionIntentRequest,
	requireFresh bool,
) (State.CastleState, GameData.ExpansionDefinition, int, string, error) {
	castle, err := buildingCastle(input.State, request.CastleID)
	if err != nil {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", err
	}
	if request.X < 0 || request.Y < 0 || request.Direction < 0 || request.Direction > 3 {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"expansion coordinates must be non-negative and direction must be 0 through 3",
		)
	}
	payment := strings.ToLower(strings.TrimSpace(request.Payment))
	if payment == "" {
		payment = Buildings.ExpansionPaymentResources
	}
	if payment != Buildings.ExpansionPaymentResources && payment != Buildings.ExpansionPaymentPremium {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"payment must be %q or %q", Buildings.ExpansionPaymentResources, Buildings.ExpansionPaymentPremium,
		)
	}
	if payment == Buildings.ExpansionPaymentPremium && !request.AllowPremium {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf("premium expansion payment requires allowPremium=true")
	}
	if input.GameData == nil {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf("official game data is unavailable")
	}
	catalog, err := input.GameData.ExpansionCatalog()
	if err != nil {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", err
	}
	initialGroundCount := len(castle.Layout.Ground)
	nextLevel := int64(initialGroundCount)
	if nextLevel < 1 {
		nextLevel = 1
	}
	definition, found := catalog.Definition(int64(castle.KingdomID), nextLevel)
	if !found {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"official data has no expansion level %d for kingdom %d", nextLevel, castle.KingdomID,
		)
	}
	if buildingExistsOnGround(castle, request.X, request.Y, request.Direction) {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"an expansion ground tile already exists at %d,%d direction %d", request.X, request.Y, request.Direction,
		)
	}
	if !requireFresh {
		return castle, definition, initialGroundCount, payment, nil
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"castle %d does not have a fresh focused building layout", castle.ID,
		)
	}
	preview, err := Buildings.PreviewExpansion(input.State, input.GameData, Buildings.ExpansionPreviewRequest{
		CastleID: castle.ID, Payment: payment, ResourceReserves: request.ResourceReserves, AllowPremium: request.AllowPremium,
	})
	if err != nil {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", err
	}
	if !preview.Ready {
		parts := make([]string, 0, len(preview.Blockers))
		for _, blocker := range preview.Blockers {
			parts = append(parts, blocker.Code+": "+blocker.Message)
		}
		if len(parts) == 0 {
			parts = append(parts, "the next expansion is not currently ready")
		}
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf(
			"castle expansion rejected: %s", strings.Join(parts, "; "),
		)
	}
	if preview.NextExpansion == nil || preview.NextExpansion.ID != definition.ID {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", fmt.Errorf("expansion catalog changed during live validation")
	}
	return castle, definition, initialGroundCount, payment, nil
}

func validatedBuildingExpansionGift(
	input Intent.PlanningContext,
	request buildingInstanceIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := observedNormalBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if !strings.EqualFold(definition.InternalName, "TreasureChest") {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d is %s, not an expansion treasure chest", request.BuildingInstanceID, definition.DisplayName,
		)
	}
	if requireFresh && (!building.Placed || building.Layer != State.BuildingLayerBD) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"expansion treasure chest %d is not a placed castle object", request.BuildingInstanceID,
		)
	}
	return castle, building, definition, nil
}

func validatedBuildingPlacement(
	input Intent.PlanningContext,
	request buildingPlacementIntentRequest,
	kind string,
	requireFresh bool,
) (State.CastleState, GameData.BuildingDefinition, error) {
	castle, err := buildingCastle(input.State, request.CastleID)
	if err != nil {
		return State.CastleState{}, GameData.BuildingDefinition{}, err
	}
	catalog, err := buildingCatalog(input.GameData)
	if err != nil {
		return State.CastleState{}, GameData.BuildingDefinition{}, err
	}
	definition, found := catalog.Definition(int64(request.DefinitionID))
	if !found || request.DefinitionID <= 0 {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building definition %d is not in the current official catalog", request.DefinitionID)
	}
	if request.X < 0 || request.Y < 0 || request.Rotation < 0 || request.Rotation > 3 {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building placement must use non-negative coordinates and rotation 0 through 3")
	}
	if kind != buildingMutationConstruct && kind != buildingMutationPlace {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("unsupported building placement kind %q", kind)
	}
	if !requireFresh {
		return castle, definition, nil
	}
	if err := requireFreshBuildingState(castle); err != nil {
		return State.CastleState{}, GameData.BuildingDefinition{}, err
	}
	issues := Buildings.ValidatePlacement(castle, definition, Buildings.Placement{
		GridX: request.X, GridY: request.Y, Rotation: request.Rotation,
	}, catalog, 0)
	if len(issues) > 0 {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building placement is invalid: %s", issues[0].Message)
	}
	if kind == buildingMutationPlace {
		if definition.Storeable == nil || !*definition.Storeable {
			return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building definition %d is not placeable from ordinary storage", definition.ID)
		}
		if ordinaryStorageCount(input.State, request.DefinitionID) <= 0 {
			return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building definition %d is not present in ordinary storage", definition.ID)
		}
		return castle, definition, nil
	}
	if ordinaryStorageCount(input.State, request.DefinitionID) > 0 {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building definition %d is present in ordinary storage; use building.place so inventory consumption is explicit",
			definition.ID,
		)
	}
	candidate, err := validatedConstructionCandidate(input, request)
	if err != nil {
		return State.CastleState{}, GameData.BuildingDefinition{}, err
	}
	if candidate.Definition.ID != definition.ID {
		return State.CastleState{}, GameData.BuildingDefinition{}, fmt.Errorf("building definition %d did not produce a matching construction candidate", definition.ID)
	}
	return castle, definition, nil
}

func validatedBuildingMove(
	input Intent.PlanningContext,
	request buildingMoveIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := validatedNormalBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if request.X < 0 || request.Y < 0 || request.Rotation < 0 || request.Rotation > 3 {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building placement must use non-negative coordinates and rotation 0 through 3")
	}
	if definition.Movable != nil && !*definition.Movable {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is not movable", request.BuildingInstanceID)
	}
	if requireFresh {
		if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is in the construction queue", request.BuildingInstanceID)
		}
		catalog, _ := input.GameData.BuildingCatalog()
		issues := Buildings.ValidatePlacement(castle, definition, Buildings.Placement{
			GridX: request.X, GridY: request.Y, Rotation: request.Rotation,
		}, catalog, request.BuildingInstanceID)
		if len(issues) > 0 {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building move is invalid: %s", issues[0].Message)
		}
	}
	return castle, building, definition, nil
}

func validatedBuildingUpgrade(
	input Intent.PlanningContext,
	request buildingUpgradeIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := validatedUpgradeableBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	catalog, _ := input.GameData.BuildingCatalog()
	target, found := catalog.Definition(definition.UpgradeDefinitionID)
	if !found || definition.UpgradeDefinitionID <= 0 {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d has no next official upgrade", request.BuildingInstanceID)
	}
	if !requireFresh {
		return castle, building, target, nil
	}
	if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is already in the construction queue", request.BuildingInstanceID)
	}
	construct, upgrades := false, true
	preview, err := Buildings.Preview(input.State, input.GameData, Buildings.PreviewRequest{
		CastleID: castle.ID, Profile: "custom", EventID: request.EventID, MapID: request.MapID,
		Objectives:       []Buildings.Objective{{Metric: "buildingLevel", Weight: 1}},
		Constraints:      Buildings.Constraints{AllowPremium: request.AllowPremium, ResourceReserves: request.ResourceReserves},
		IncludeConstruct: &construct, IncludeUpgrades: &upgrades, IncludeBlocked: true, MaxCandidates: 500,
	})
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	for _, candidate := range preview.Candidates {
		if candidate.Kind == Buildings.ActionUpgrade && candidate.BuildingID == request.BuildingInstanceID {
			if !candidate.Eligible {
				return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, buildingCandidateError("upgrade", candidate)
			}
			return castle, building, target, nil
		}
	}
	return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d has no upgrade candidate in the refreshed castle state", request.BuildingInstanceID)
}

func validatedBuildingStore(
	input Intent.PlanningContext,
	request buildingInstanceIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := validatedNormalBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if definition.Storeable == nil || !*definition.Storeable {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is not storeable", request.BuildingInstanceID)
	}
	if requireFresh && buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is in the construction queue", request.BuildingInstanceID)
	}
	return castle, building, definition, nil
}

func validatedBuildingFinishFree(
	input Intent.PlanningContext,
	request buildingInstanceIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := observedUpgradeableBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if !requireFresh {
		return castle, building, definition, nil
	}
	if !buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d is not in the construction queue", request.BuildingInstanceID,
		)
	}
	if !buildingOperationInProgress(building.ConstructionState) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d is not in a finishable construction state (%d)", request.BuildingInstanceID, building.ConstructionState,
		)
	}
	return castle, building, definition, nil
}

func validatedBuildingTimeSkip(
	input Intent.PlanningContext,
	request buildingTimeSkipIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, buildingTimeSkipOption, float64, error) {
	castle, building, _, err := observedUpgradeableBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, err
	}
	option, err := officialBuildingTimeSkipOption(input.GameData, request.Minutes)
	if err != nil {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, err
	}
	if request.MinimumRemaining < 0 {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, fmt.Errorf("minimumRemaining cannot be negative")
	}
	balance := input.State.Player.Currencies[option.CurrencyID]
	if balance < 1 || balance-1 < float64(request.MinimumRemaining) {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, fmt.Errorf(
			"%d-minute time skip balance %.0f cannot preserve minimumRemaining %d",
			option.Minutes, balance, request.MinimumRemaining,
		)
	}
	if requireFresh {
		if !buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, fmt.Errorf(
				"building %d is not in the construction queue", request.BuildingInstanceID,
			)
		}
		if !buildingOperationInProgress(building.ConstructionState) {
			return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, fmt.Errorf(
				"building %d is not in a skippable construction state (%d)", request.BuildingInstanceID, building.ConstructionState,
			)
		}
	}
	return castle, building, option, balance, nil
}

func officialBuildingTimeSkipOption(gameData *GameData.Store, minutes int) (buildingTimeSkipOption, error) {
	options := map[int]buildingTimeSkipOption{
		1:    {Minutes: 1, CurrencyID: 1001, WireKey: "MS1"},
		5:    {Minutes: 5, CurrencyID: 1002, WireKey: "MS2"},
		10:   {Minutes: 10, CurrencyID: 1003, WireKey: "MS3"},
		30:   {Minutes: 30, CurrencyID: 1004, WireKey: "MS4"},
		60:   {Minutes: 60, CurrencyID: 1005, WireKey: "MS5"},
		300:  {Minutes: 300, CurrencyID: 1006, WireKey: "MS6"},
		1440: {Minutes: 1440, CurrencyID: 1007, WireKey: "MS7"},
	}
	option, found := options[minutes]
	if !found {
		return buildingTimeSkipOption{}, fmt.Errorf("minutes must be one of 1, 5, 10, 30, 60, 300, or 1440")
	}
	if gameData == nil {
		return buildingTimeSkipOption{}, fmt.Errorf("official game data is unavailable")
	}
	currencies, err := gameData.Catalog("currencies")
	if err != nil {
		return buildingTimeSkipOption{}, err
	}
	rawCurrency, found := currencies.Find(strconv.FormatInt(int64(option.CurrencyID), 10))
	if !found {
		return buildingTimeSkipOption{}, fmt.Errorf("official currency %d is unavailable", option.CurrencyID)
	}
	var currency struct {
		JSONKey string `json:"JSONKey"`
	}
	if err := json.Unmarshal(rawCurrency, &currency); err != nil || currency.JSONKey != option.WireKey {
		return buildingTimeSkipOption{}, fmt.Errorf("official currency %d does not map to %s", option.CurrencyID, option.WireKey)
	}
	values, err := gameData.Catalog("currencyMinutesSkipValues")
	if err != nil {
		return buildingTimeSkipOption{}, err
	}
	rawValue, found := values.FindByField("currencyID", strconv.FormatInt(int64(option.CurrencyID), 10))
	if !found {
		return buildingTimeSkipOption{}, fmt.Errorf("official minute-skip value for currency %d is unavailable", option.CurrencyID)
	}
	var value struct {
		Minutes string `json:"MinutesSkipValue"`
	}
	if err := json.Unmarshal(rawValue, &value); err != nil || value.Minutes != strconv.Itoa(option.Minutes) {
		return buildingTimeSkipOption{}, fmt.Errorf("official currency %d does not represent a %d-minute skip", option.CurrencyID, option.Minutes)
	}
	return option, nil
}

func validatedBuildingDemolish(
	input Intent.PlanningContext,
	request buildingInstanceIntentRequest,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := validatedNormalBuilding(input, request.CastleID, request.BuildingInstanceID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if definition.Destructable != nil && !*definition.Destructable {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is not destructible", request.BuildingInstanceID)
	}
	if requireFresh {
		if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d is already in the construction queue", request.BuildingInstanceID)
		}
		if !buildingQueueAvailable(castle.BuildingQueue) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("no building construction slot is available in castle %d", castle.ID)
		}
	}
	return castle, building, definition, nil
}

func validatedNormalBuilding(
	input Intent.PlanningContext,
	castleID State.CastleID,
	buildingID State.BuildingInstanceID,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := observedNormalBuilding(input, castleID, buildingID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if requireFresh && building.ConstructionState != State.BuildingStateInitial &&
		building.ConstructionState != State.BuildingStateBuildCompleted {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d is not idle (construction state %d)", buildingID, building.ConstructionState,
		)
	}
	return castle, building, definition, nil
}

func validatedUpgradeableBuilding(
	input Intent.PlanningContext,
	castleID State.CastleID,
	buildingID State.BuildingInstanceID,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, building, definition, err := observedUpgradeableBuilding(input, castleID, buildingID, requireFresh)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if requireFresh && building.ConstructionState != State.BuildingStateInitial &&
		building.ConstructionState != State.BuildingStateBuildCompleted {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d is not idle (construction state %d)", buildingID, building.ConstructionState,
		)
	}
	return castle, building, definition, nil
}

func observedUpgradeableBuilding(
	input Intent.PlanningContext,
	castleID State.CastleID,
	buildingID State.BuildingInstanceID,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, err := buildingCastle(input.State, castleID)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if requireFresh {
		if err := requireFreshBuildingState(castle); err != nil {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
		}
	}
	building, found := castle.Layout.Objects[buildingID]
	if !found {
		building, found = castle.Layout.Fixed[buildingID]
	}
	if !found || buildingID <= 0 {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"upgradeable building instance %d is not in castle %d", buildingID, castleID,
		)
	}
	catalog, err := buildingCatalog(input.GameData)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	definition, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf(
			"building %d uses unknown official definition %d", buildingID, building.DefinitionID,
		)
	}
	return castle, building, definition, nil
}

func observedNormalBuilding(
	input Intent.PlanningContext,
	castleID State.CastleID,
	buildingID State.BuildingInstanceID,
	requireFresh bool,
) (State.CastleState, State.Building, GameData.BuildingDefinition, error) {
	castle, err := buildingCastle(input.State, castleID)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	if requireFresh {
		if err := requireFreshBuildingState(castle); err != nil {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
		}
	}
	building, found := castle.Layout.Objects[buildingID]
	if !found || buildingID <= 0 {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("normal building instance %d is not in castle %d", buildingID, castleID)
	}
	catalog, err := buildingCatalog(input.GameData)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	definition, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("building %d uses unknown official definition %d", buildingID, building.DefinitionID)
	}
	return castle, building, definition, nil
}

func validatedConstructionCandidate(
	input Intent.PlanningContext,
	request buildingPlacementIntentRequest,
) (Buildings.Candidate, error) {
	construct, upgrades := true, false
	preview, err := Buildings.Preview(input.State, input.GameData, Buildings.PreviewRequest{
		CastleID: request.CastleID, Profile: "custom", EventID: request.EventID, MapID: request.MapID,
		Objectives:             []Buildings.Objective{{Metric: "buildingLevel", Weight: 1}},
		Constraints:            Buildings.Constraints{AllowPremium: request.AllowPremium, ResourceReserves: request.ResourceReserves},
		CandidateDefinitionIDs: []int64{int64(request.DefinitionID)}, IncludeConstruct: &construct,
		IncludeUpgrades: &upgrades, IncludeBlocked: true, MaxCandidates: 1,
	})
	if err != nil {
		return Buildings.Candidate{}, err
	}
	if len(preview.Candidates) == 0 {
		return Buildings.Candidate{}, fmt.Errorf("building definition %d did not produce a construction candidate", request.DefinitionID)
	}
	candidate := preview.Candidates[0]
	if !candidate.Eligible {
		return Buildings.Candidate{}, buildingCandidateError("construction", candidate)
	}
	return candidate, nil
}

func buildingCandidateError(action string, candidate Buildings.Candidate) error {
	parts := make([]string, 0, len(candidate.Blockers))
	for _, blocker := range candidate.Blockers {
		parts = append(parts, blocker.Code+": "+blocker.Message)
	}
	if len(parts) == 0 {
		parts = append(parts, "candidate is not eligible")
	}
	return fmt.Errorf("building %s rejected: %s", action, strings.Join(parts, "; "))
}

func (application *Application) verifyBuildingMutation(ctx context.Context, arguments json.RawMessage) error {
	var verification buildingVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	if verification.Kind == buildingMutationFinishFree {
		return application.waitForFreeBuildingCompletion(ctx, verification)
	}
	if verification.Kind == buildingMutationSkipTime {
		return application.waitForBuildingTimeSkip(ctx, verification)
	}
	castle, err := buildingCastle(application.State.Snapshot(), verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID)
	}
	switch verification.Kind {
	case buildingMutationExpand:
		if len(castle.Layout.Ground) <= verification.InitialGroundCount {
			return fmt.Errorf("castle %d ground-tile count did not increase after expansion", castle.ID)
		}
		if !buildingExistsOnGround(castle, verification.X, verification.Y, verification.Rotation) {
			return fmt.Errorf("expansion ground tile was not observed at %d,%d direction %d", verification.X, verification.Y, verification.Rotation)
		}
	case buildingMutationCollectGift:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found {
			return fmt.Errorf("expansion gift %d is still placed after collection", verification.BuildingInstanceID)
		}
	case buildingMutationConstruct, buildingMutationPlace:
		if !buildingExistsAt(castle, verification.DefinitionID, verification.X, verification.Y, verification.Rotation) {
			return fmt.Errorf("building definition %d was not observed at %d,%d after placement", verification.DefinitionID, verification.X, verification.Y)
		}
	case buildingMutationMove:
		building, found := buildingByID(castle, verification.BuildingInstanceID)
		if !found || building.GridX != verification.X || building.GridY != verification.Y || building.Rotation != verification.Rotation {
			return fmt.Errorf("building %d did not reconcile to %d,%d rotation %d", verification.BuildingInstanceID, verification.X, verification.Y, verification.Rotation)
		}
	case buildingMutationUpgrade:
		building, found := buildingByID(castle, verification.BuildingInstanceID)
		if !found {
			return fmt.Errorf("building %d disappeared after its upgrade was accepted", verification.BuildingInstanceID)
		}
		if building.DefinitionID != verification.TargetDefinitionID && !buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
			return fmt.Errorf("building %d is neither upgraded to %d nor present in the construction queue", verification.BuildingInstanceID, verification.TargetDefinitionID)
		}
	case buildingMutationStore:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found {
			return fmt.Errorf("building %d is still placed after the store command", verification.BuildingInstanceID)
		}
		if ordinaryStorageCount(application.State.Snapshot(), verification.DefinitionID) <= 0 {
			return fmt.Errorf("building definition %d was not observed in ordinary storage after storing", verification.DefinitionID)
		}
	case buildingMutationDemolish:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found && !buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
			return fmt.Errorf("building %d is neither removed nor present in the demolition queue", verification.BuildingInstanceID)
		}
	default:
		return fmt.Errorf("unsupported building verification kind %q", verification.Kind)
	}
	return nil
}

func (application *Application) waitForFreeBuildingCompletion(ctx context.Context, verification buildingVerification) error {
	events, unsubscribe := application.State.Subscribe(8)
	defer unsubscribe()

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		err := application.verifyFreeBuildingCompletion(verification)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("free building completion verification interrupted: %w", ctx.Err())
		case <-timer.C:
			return err
		case <-events:
		}
	}
}

func (application *Application) verifyFreeBuildingCompletion(verification buildingVerification) error {
	castle, err := buildingCastle(application.State.Snapshot(), verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID)
	}
	if buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
		return fmt.Errorf("building %d remains in the construction queue after a free completion", verification.BuildingInstanceID)
	}
	building, found := buildingByID(castle, verification.BuildingInstanceID)
	if !found {
		if verification.InitialConstructionState == State.BuildingStateDisassembleStopped ||
			verification.InitialConstructionState == State.BuildingStateDisassembleInProgress {
			return nil
		}
		return fmt.Errorf("building %d disappeared after a non-demolition free completion", verification.BuildingInstanceID)
	}
	if buildingOperationInProgress(building.ConstructionState) {
		return fmt.Errorf("building %d remains in construction state %d after a free completion", verification.BuildingInstanceID, building.ConstructionState)
	}
	return nil
}

func (application *Application) waitForBuildingTimeSkip(ctx context.Context, verification buildingVerification) error {
	events, unsubscribe := application.State.Subscribe(8)
	defer unsubscribe()

	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for {
		err := application.verifyBuildingTimeSkip(verification)
		if err == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("building time-skip verification interrupted: %w", ctx.Err())
		case <-timer.C:
			return err
		case <-events:
		}
	}
}

func (application *Application) verifyBuildingTimeSkip(verification buildingVerification) error {
	state := application.State.Snapshot()
	balance := state.Player.Currencies[verification.SkipCurrencyID]
	if balance != verification.InitialSkipBalance-1 {
		return fmt.Errorf(
			"time-skip currency %d balance is %.0f; expected %.0f after one use",
			verification.SkipCurrencyID, balance, verification.InitialSkipBalance-1,
		)
	}
	castle, err := buildingCastle(state, verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID)
	}
	building, found := buildingByID(castle, verification.BuildingInstanceID)
	if !found {
		if verification.InitialConstructionState == State.BuildingStateDisassembleStopped ||
			verification.InitialConstructionState == State.BuildingStateDisassembleInProgress {
			return nil
		}
		return fmt.Errorf("building %d disappeared after a non-demolition time skip", verification.BuildingInstanceID)
	}
	if buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) &&
		building.ProgressSec <= verification.InitialProgressSec {
		return fmt.Errorf("building %d did not advance after a time skip", verification.BuildingInstanceID)
	}
	return nil
}

func buildingMutationStep(name string, opcode string, payload json.RawMessage) Intent.Step {
	step := commandStep(name, opcode, payload, opcode)
	step.CaptureResponse = true
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func buildingResolverStep(name string, resolver string, arguments json.RawMessage, awaitOpcode string) Intent.Step {
	return Intent.Step{
		Name: name, Resolver: resolver, ResolverArguments: arguments, AwaitOpcode: awaitOpcode,
		TimeoutMillis: 10_000, SuccessCodes: []int{0}, CaptureResponse: true,
		ResponseBarrier: Intent.ResponseBarrierCommitted,
	}
}

func buildingCastle(state State.GameState, castleID State.CastleID) (State.CastleState, error) {
	castle, found := state.Castles[castleID]
	if !found || castleID <= 0 {
		return State.CastleState{}, fmt.Errorf("castle %d is not in the current player state", castleID)
	}
	return castle, nil
}

func buildingCatalog(gameData *GameData.Store) (*GameData.BuildingCatalog, error) {
	if gameData == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	return gameData.BuildingCatalog()
}

func buildingStateIsFresh(state State.GameState, castleID State.CastleID) bool {
	castle, found := state.Castles[castleID]
	return found && castle.Focused && !castle.Layout.ObservedAt.IsZero()
}

func requireFreshBuildingState(castle State.CastleState) error {
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return fmt.Errorf("castle %d does not have a fresh focused building layout", castle.ID)
	}
	if castle.BuildingQueue.ObservedAt.IsZero() {
		return fmt.Errorf("castle %d does not have a fresh building construction queue", castle.ID)
	}
	return nil
}

func buildingQueueAvailable(queue State.BuildingConstructionQueue) bool {
	if len(queue.Slots) == 0 {
		return false
	}
	for _, slot := range queue.Slots {
		if slot.Status == State.BuildingQueueSlotAvailable {
			return true
		}
	}
	return false
}

func buildingQueued(queue State.BuildingConstructionQueue, buildingID State.BuildingInstanceID) bool {
	for _, slot := range queue.Slots {
		if slot.Status == State.BuildingQueueSlotOccupied && slot.BuildingID == buildingID {
			return true
		}
	}
	return false
}

func buildingOperationInProgress(constructionState int) bool {
	switch constructionState {
	case State.BuildingStateBuildStopped, State.BuildingStateBuildInProgress,
		State.BuildingStateDisassembleStopped, State.BuildingStateDisassembleInProgress,
		State.BuildingStateRepairStopped, State.BuildingStateRepairInProgress,
		State.BuildingStateUpgradeStopped, State.BuildingStateUpgradeInProgress:
		return true
	default:
		return false
	}
}

func ordinaryStorageCount(state State.GameState, definitionID State.BuildingID) int64 {
	return state.Inventory.Items["storage:1"][int64(definitionID)]
}

func buildingByID(castle State.CastleState, buildingID State.BuildingInstanceID) (State.Building, bool) {
	for _, buildings := range []map[State.BuildingInstanceID]State.Building{
		castle.Layout.Objects, castle.Layout.Ground, castle.Layout.Fixed, castle.Buildings,
	} {
		if building, found := buildings[buildingID]; found {
			return building, true
		}
	}
	return State.Building{}, false
}

func buildingExistsAt(castle State.CastleState, definitionID State.BuildingID, x int, y int, rotation int) bool {
	for _, buildings := range []map[State.BuildingInstanceID]State.Building{
		castle.Layout.Objects, castle.Layout.Ground, castle.Layout.Fixed,
	} {
		for _, building := range buildings {
			if building.DefinitionID == definitionID && building.GridX == x && building.GridY == y && building.Rotation == rotation {
				return true
			}
		}
	}
	return false
}

func buildingExistsOnGround(castle State.CastleState, x int, y int, rotation int) bool {
	for _, building := range castle.Layout.Ground {
		if building.GridX == x && building.GridY == y && building.Rotation == rotation {
			return true
		}
	}
	return false
}

func buildingCastleClaims(castleID State.CastleID) []string {
	id := strconv.FormatInt(int64(castleID), 10)
	return []string{"castle-focus", "castle:" + id, "building-layout:" + id, "building-construction:" + id}
}

func buildingInstanceClaims(castleID State.CastleID, buildingID State.BuildingInstanceID) []string {
	claims := buildingCastleClaims(castleID)
	return append(claims, "building:"+strconv.FormatInt(int64(buildingID), 10))
}

func buildingPositionClaim(castleID State.CastleID, x int, y int) string {
	return fmt.Sprintf("building-position:%d:%d:%d", castleID, x, y)
}

package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
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
	MaximumLevel       int64                    `json:"maximumLevel,omitempty"`
}

// Legacy premium fields remain decodable so saved confirmation plans can be
// rejected explicitly; new plans never emit them.
type buildingUpgradeResolverArguments struct {
	Request             buildingUpgradeIntentRequest `json:"request"`
	PremiumMode         string                       `json:"premiumMode,omitempty"`
	TargetDefinitionID  int64                        `json:"targetDefinitionId,omitempty"`
	ExpectedPremiumCost int64                        `json:"expectedPremiumCost,omitempty"`
}

const (
	buildingPremiumModeConfirm = "confirm"
)

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
	if err := application.Intents.RegisterAction("building.upgrade.guard", application.guardBuildingUpgrade); err != nil {
		return err
	}
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
			Name: "building.refresh", Description: "Focus a castle and rebuild its layout, construction queue, resources, and storage snapshot", DescriptionDescriptor: Localization.New("server.intent.description.304a12ff", "Focus a castle and rebuild its layout, construction queue, resources, and storage snapshot", nil), Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717}`), Planner: planBuildingRefresh, ReadSet: buildingReadSet,
		},
		{
			Name: "building.storage.refresh", Description: "Refresh authoritative ordinary building and decoration storage", DescriptionDescriptor: Localization.New("server.intent.description.8457b798", "Refresh authoritative ordinary building and decoration storage", nil), Effect: Intent.EffectRead,
			Planner: planBuildingStorageRefresh,
		},
		{
			Name: "building.expand", Description: "Buy the next official castle expansion at an exact captured position after validating cost and storage capacity", DescriptionDescriptor: Localization.New("server.intent.description.2d609b96", "Buy the next official castle expansion at an exact captured position after validating cost and storage capacity", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"x":220,"y":220,"direction":1,"payment":"resources"}`),
			Planner:          planBuildingExpansion, ReadSet: buildingReadSet,
		},
		{
			Name: "building.collect_expansion_gift", Description: "Collect one validated expansion treasure chest that blocks castle placement", DescriptionDescriptor: Localization.New("server.intent.description.63aa5d9e", "Collect one validated expansion treasure chest that blocks castle placement", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"buildingInstanceId":46}`),
			Planner:          planBuildingCollectExpansionGift, ReadSet: buildingReadSet,
		},
		{
			Name: "building.construct", Description: "Construct one validated official building definition without implicit premium spending", DescriptionDescriptor: Localization.New("server.intent.description.488f8bf0", "Construct one validated official building definition without implicit premium spending", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"definitionId":301,"x":200,"y":200,"rotation":0}`),
			Planner:          planBuildingConstruct, ReadSet: buildingReadSet,
		},
		{
			Name: "building.place", Description: "Place one validated official definition already present in ordinary castle storage", DescriptionDescriptor: Localization.New("server.intent.description.fd035efd", "Place one validated official definition already present in ordinary castle storage", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"definitionId":301,"x":200,"y":200,"rotation":0}`),
			Planner:          planBuildingPlace, ReadSet: buildingReadSet,
		},
		{
			Name: "building.move", Description: "Move or rotate one movable castle object after collision validation", DescriptionDescriptor: Localization.New("server.intent.description.c9faee42", "Move or rotate one movable castle object after collision validation", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":835,"x":195,"y":220,"rotation":1}`),
			Planner:          planBuildingMove, ReadSet: buildingReadSet,
		},
		{
			Name: "building.upgrade", Description: "Start the next official upgrade for one observed castle building when its queue and costs are valid", DescriptionDescriptor: Localization.New("server.intent.description.634c4042", "Start the next official upgrade for one observed castle building when its queue and costs are valid", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430}`),
			Planner:          planBuildingUpgrade, ReadSet: buildingReadSet,
		},
		{
			Name: "building.finish_free", Description: "Finish one queued building operation only through the server's explicit free-skip path", DescriptionDescriptor: Localization.New("server.intent.description.514dd09f", "Finish one queued building operation only through the server's explicit free-skip path", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430}`),
			Planner:          planBuildingFinishFree, ReadSet: buildingReadSet,
		},
		{
			Name: "building.skip_time", Description: "Consume one selected non-premium time-skip currency on a queued building operation", DescriptionDescriptor: Localization.New("server.intent.description.e697996b", "Consume one selected non-premium time-skip currency on a queued building operation", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":430,"minutes":10,"minimumRemaining":5}`),
			Planner:          planBuildingTimeSkip, ReadSet: buildingReadSet,
		},
		{
			Name: "building.store", Description: "Move one storeable, idle castle object into ordinary castle storage", DescriptionDescriptor: Localization.New("server.intent.description.7723e680", "Move one storeable, idle castle object into ordinary castle storage", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":16326717,"buildingInstanceId":835}`),
			Planner:          planBuildingStore, ReadSet: buildingReadSet,
		},
		{
			Name: "building.demolish", Description: "Start demolition of one destructible normal castle object using a free construction slot", DescriptionDescriptor: Localization.New("server.intent.description.c7fa31d4", "Start demolition of one destructible normal castle object using a free construction slot", nil), Effect: Intent.EffectWrite,
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

func planBuildingStorageRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	return Intent.Plan{
		Claims: []string{"inventory:storage"}, Summary: "Refresh ordinary building storage", SummaryDescriptor: Localization.New("server.app.refresh_ordinary_building_storage.329a65f0", "Refresh ordinary building storage", nil),
		Steps: []Intent.Step{Intent.RebuildOnResume(Intent.Step{
			Name: "Refresh ordinary building storage", NameDescriptor: Localization.New("server.app.refresh_ordinary_building_storage.329a65f0", "Refresh ordinary building storage", nil), Opcode: "sin", AwaitOpcode: "sin", TimeoutMillis: 10_000,
			SuccessCodes: []int{0}, Command: Protocol.Command{Opcode: "sin", Bare: true},
		})},
	}, nil
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
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep("Buy castle expansion", "building.expand.build", resolverArguments, "ebe", Localization.New("server.app.buy_castle_expansion.3fb31a21", "Buy castle expansion", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify castle expansion", NameDescriptor: Localization.New("server.app.verify_castle_expansion.6f518e94", "Verify castle expansion", nil), Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingCastleClaims(castle.ID), buildingPositionClaim(castle.ID, request.X, request.Y), "account-resources")
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Buy expansion level %d in %s with %s", definition.Level, castleLabel(castle), payment), SummaryDescriptor: Localization.New("server.app.buy_expansion_level_p.2225253e", "Buy expansion level {p0} in {p1} with {p2}", Localization.Params{"p0": definition.Level, "p1": fmt.Sprintf("%s", castleLabel(castle)), "p2": fmt.Sprintf("%s", payment)}),
		Steps: steps,
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
	return buildingMutationStep("Buy castle expansion", "ebe", payload).WithNameDescriptor(Localization.New("server.app.buy_castle_expansion.3fb31a21", "Buy castle expansion", nil)), nil
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
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep("Collect expansion gift", "building.collect_expansion_gift.build", resolverArguments, "etc", Localization.New("server.app.collect_expansion_gift.bbcffec1", "Collect expansion gift", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify expansion gift collection", NameDescriptor: Localization.New("server.app.verify_expansion_gift_collection.348e8604", "Verify expansion gift collection", nil), Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Collect expansion gift %d in %s", request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.collect_expansion_gift_p.2c100b88", "Collect expansion gift {p0} in {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: steps,
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
	return buildingMutationStep("Collect expansion gift", "etc", payload).WithNameDescriptor(Localization.New("server.app.collect_expansion_gift.bbcffec1", "Collect expansion gift", nil)), nil
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
		Claims: buildingCastleClaims(castle.ID), Summary: fmt.Sprintf("Refresh building state for %s", castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.refresh_building_state_for.83f7e8b8", "Refresh building state for {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: castleContextSteps(input, castle),
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
	var nameLocalizationMessage *Localization.Message = Localization.New("server.app.construct_building.f1811844", "Construct building", nil)
	summary := fmt.Sprintf("Construct %s in %s", definition.DisplayName, castleLabel(castle))
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.construct_p_in_p.817c6301", "Construct {p0} in {p1}", Localization.Params{"p0": fmt.Sprintf("%s", definition.DisplayName), "p1": fmt.Sprintf("%s", castleLabel(castle))}).WithGameParam("p0", GameData.FirstOfficialNameKey(input.Language, definition.LocalizationKeys...), definition.DisplayName)
	if kind == buildingMutationPlace {
		name = "Place stored building"
		nameLocalizationMessage = Localization.New("server.app.place_stored_building.29ca06c3", "Place stored building", nil)
		summary = fmt.Sprintf("Place stored %s in %s", definition.DisplayName, castleLabel(castle))
		summaryLocalizationMessage = Localization.New("server.app.place_stored_p_in.67a6cdc1", "Place stored {p0} in {p1}", Localization.Params{"p0": fmt.Sprintf("%s", definition.DisplayName), "p1": fmt.Sprintf("%s", castleLabel(castle))}).WithGameParam("p0", GameData.FirstOfficialNameKey(input.Language, definition.LocalizationKeys...), definition.DisplayName)
	}
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep(name, "building.placement.build", resolverArguments, "ebu", Localization.Clone(nameLocalizationMessage)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building placement", NameDescriptor: Localization.New("server.app.verify_building_placement.86770384", "Verify building placement", nil), Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingCastleClaims(castle.ID), buildingPositionClaim(castle.ID, request.X, request.Y))
	if kind == buildingMutationPlace {
		claims = append(claims, "inventory:storage")
	}
	return Intent.Plan{Claims: claims, Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage), Steps: steps}, nil
}

func resolveBuildingPlacementStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var resolver buildingPlacementResolverArguments
	if err := decodeIntentArguments(arguments, &resolver); err != nil {
		return Intent.Step{}, err
	}
	_, definition, err := validatedBuildingPlacement(input, resolver.Request, resolver.Kind, true)
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
	step := buildingMutationStep(name, "ebu", payload)
	if resolver.Kind == buildingMutationConstruct {
		step.CoinCost, err = buildingCoinCostRequirement(input.GameData, definition)
		if err != nil {
			return Intent.Step{}, err
		}
	}
	return step, nil
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
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep("Move building", "building.move.build", resolverArguments, "emo", Localization.New("server.app.move_building.298cf6eb", "Move building", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building position", NameDescriptor: Localization.New("server.app.verify_building_position.9ace6687", "Verify building position", nil), Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID), buildingPositionClaim(castle.ID, request.X, request.Y))
	return Intent.Plan{
		Claims: claims, Summary: fmt.Sprintf("Move building %d in %s", request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.move_building_p_in.a3de577d", "Move building {p0} in {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", castleLabel(castle))}),
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
	return buildingMutationStep("Move building", "emo", payload).WithNameDescriptor(Localization.New("server.app.move_building.298cf6eb", "Move building", nil)), nil
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
	verificationArguments, _ := json.Marshal(buildingVerification{
		Kind: buildingMutationUpgrade, CastleID: castle.ID, BuildingInstanceID: request.BuildingInstanceID,
		TargetDefinitionID: State.BuildingID(target.ID),
	})
	steps := castleContextSteps(input, castle)
	resolverArguments, _ := json.Marshal(buildingUpgradeResolverArguments{Request: request, TargetDefinitionID: target.ID})
	steps = append(steps, buildingResolverStep("Upgrade building", "building.upgrade.build", resolverArguments, "eup", Localization.New("server.app.upgrade_building.a2bf6f68", "Upgrade building", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building upgrade", NameDescriptor: Localization.New("server.app.verify_building_upgrade.2ac80367", "Verify building upgrade", nil), Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Upgrade building %d to %s in %s", request.BuildingInstanceID, target.DisplayName, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.upgrade_building_p_to.1662917f", "Upgrade building {p0} to {p1} in {p2}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", target.DisplayName), "p2": fmt.Sprintf("%s", castleLabel(castle))}).WithGameParam("p1", GameData.FirstOfficialNameKey(input.Language, target.LocalizationKeys...), target.DisplayName),
		Steps: steps,
	}, nil
}

func resolveBuildingUpgradeStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var resolverArguments buildingUpgradeResolverArguments
	if err := decodeIntentArguments(arguments, &resolverArguments); err != nil {
		return Intent.Step{}, err
	}
	request := resolverArguments.Request
	_, _, target, err := validatedBuildingUpgrade(input, request, true)
	if err != nil {
		return Intent.Step{}, err
	}
	// Never execute a stored legacy quote/confirmation plan unattended.
	if resolverArguments.PremiumMode != "" {
		return Intent.Step{}, fmt.Errorf("premium upgrade confirmation requires a new guarded plan")
	}

	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		Power      int                      `json:"PWR"`
		Offer      int                      `json:"PO"`
	}{request.BuildingInstanceID, 0, -1})
	if resolverArguments.TargetDefinitionID > 0 && resolverArguments.TargetDefinitionID != target.ID {
		return Intent.Step{}, fmt.Errorf("%w: upgrade target changed", Intent.ErrPlanStale)
	}
	step := buildingMutationStep("Upgrade building", "eup", payload).WithNameDescriptor(Localization.New("server.app.upgrade_building.a2bf6f68", "Upgrade building", nil))
	step.FinalDispatchAction = "building.upgrade.guard"
	step.FinalDispatchArguments = arguments
	step.CoinCost, err = buildingCoinCostRequirement(input.GameData, target)
	if err != nil {
		return Intent.Step{}, err
	}

	return step, nil
}

func buildingCoinCostRequirement(store *GameData.Store, definition GameData.BuildingDefinition) (*Intent.CoinCostRequirement, error) {
	cost, known, err := officialNumberOrZero(store, "buildings", definition.ID, "costC1")
	if err != nil || !known || cost < 0 || math.IsNaN(cost) || math.IsInf(cost, 0) || cost >= math.Exp2(63) {
		return nil, Localization.WithError(fmt.Errorf("official building %d coin cost is missing or malformed", definition.ID), Localization.New("server.app.official_building_p_coin.930aade4", "official building {p0} coin cost is missing or malformed", Localization.Params{"p0": fmt.Sprintf("%d", definition.ID)}))
	}
	if cost == 0 {
		return nil, nil
	}
	return &Intent.CoinCostRequirement{Amount: int64(math.Ceil(cost)), Source: "official building definition cost"}, nil
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
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep("Finish building operation for free", "building.finish_free.build", resolverArguments, "fco", Localization.New("server.app.finish_building_operation_for.00d9c467", "Finish building operation for free", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify free building completion", NameDescriptor: Localization.New("server.app.verify_free_building_completion.23d987dd", "Verify free building completion", nil), Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Finish queued building operation %d for free in %s", request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.finish_queued_building_operation.536bb2d4", "Finish queued building operation {p0} for free in {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: steps,
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
	return buildingMutationStep("Finish building operation for free", "fco", payload).WithNameDescriptor(Localization.New("server.app.finish_building_operation_for.00d9c467", "Finish building operation for free", nil)), nil
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
	steps := castleContextSteps(input, castle)
	skipStep := buildingResolverStep("Apply building time skip", "building.skip_time.build", resolverArguments, "msb", Localization.New("server.app.apply_building_time_skip.e23a82b0", "Apply building time skip", nil))
	skipStep.StaleCodes = []int{147}
	steps = append(steps, skipStep)
	steps = append(steps, timeSkipConsumeStep(input, option.CurrencyID))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building time skip", NameDescriptor: Localization.New("server.app.verify_building_time_skip.1f673d87", "Verify building time skip", nil), Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		"currency:"+strconv.FormatInt(int64(option.CurrencyID), 10))
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Apply one %d-minute skip to building operation %d in %s", option.Minutes, request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.apply_one_p_minute.97b6b079", "Apply one {p0}-minute skip to building operation {p1} in {p2}", Localization.Params{"p0": option.Minutes, "p1": fmt.Sprintf("%d", request.BuildingInstanceID), "p2": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: steps,
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
	step := buildingMutationStep("Apply building time skip", "msb", payload).WithNameDescriptor(Localization.New("server.app.apply_building_time_skip.e23a82b0", "Apply building time skip", nil))
	step.StaleCodes = []int{147}
	return step, nil
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
	steps := castleContextSteps(input, castle)
	steps = append(steps, buildingResolverStep("Store building", "building.store.build", resolverArguments, "sob", Localization.New("server.app.store_building.e47d9e78", "Store building", nil)))
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify stored building", NameDescriptor: Localization.New("server.app.verify_stored_building.f9795f1d", "Verify stored building", nil), Action: "building.verify", ActionArguments: verificationArguments})
	claims := append(buildingInstanceClaims(castle.ID, request.BuildingInstanceID), "inventory:storage")
	return Intent.Plan{
		Claims: claims, Summary: fmt.Sprintf("Store building %d from %s", request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.store_building_p_from.15ad81b0", "Store building {p0} from {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", castleLabel(castle))}), Steps: steps,
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
	return buildingMutationStep("Store building", "sob", payload).WithNameDescriptor(Localization.New("server.app.store_building.e47d9e78", "Store building", nil)), nil
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
	steps := castleContextSteps(input, castle)
	demolishStep := buildingResolverStep("Demolish building", "building.demolish.build", resolverArguments, "edo", Localization.New("server.app.demolish_building.620f2c52", "Demolish building", nil))
	demolishStep.StaleCodes = []int{147}
	steps = append(steps, demolishStep)
	steps = append(steps, castleFocusStep(castle))
	steps = append(steps, Intent.Step{Name: "Verify building demolition", NameDescriptor: Localization.New("server.app.verify_building_demolition.3de62ce7", "Verify building demolition", nil), Action: "building.verify", ActionArguments: verificationArguments})
	return Intent.Plan{
		Claims:  buildingInstanceClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Demolish building %d in %s", request.BuildingInstanceID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.demolish_building_p_in.62e8233c", "Demolish building {p0} in {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", castleLabel(castle))}), Steps: steps,
	}, nil
}

func resolveBuildingDemolishStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request buildingInstanceIntentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if _, _, _, err := validatedBuildingDemolish(input, request, true); err != nil {
		return Intent.Step{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
	}{request.BuildingInstanceID})
	step := buildingMutationStep("Demolish building", "edo", payload).WithNameDescriptor(Localization.New("server.app.demolish_building.620f2c52", "Demolish building", nil))
	step.StaleCodes = []int{147}
	return step, nil
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
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"expansion coordinates must be non-negative and direction must be 0 through 3",
		), Localization.New("server.app.expansion_coordinates_must_be.581fed95", "expansion coordinates must be non-negative and direction must be 0 through 3", nil))
	}
	payment := strings.ToLower(strings.TrimSpace(request.Payment))
	if payment == "" {
		payment = Buildings.ExpansionPaymentResources
	}
	if payment != Buildings.ExpansionPaymentResources && payment != Buildings.ExpansionPaymentPremium {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"payment must be %q or %q", Buildings.ExpansionPaymentResources, Buildings.ExpansionPaymentPremium,
		), Localization.New("server.app.payment_must_be_p.384c137b", "payment must be {p0} or {p1}", Localization.Params{"p0": fmt.Sprintf("%q", Buildings.ExpansionPaymentResources), "p1": fmt.Sprintf("%q", Buildings.ExpansionPaymentPremium)}))
	}
	if payment == Buildings.ExpansionPaymentPremium && !request.AllowPremium {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf("premium expansion payment requires allowPremium=true"), Localization.New("server.app.premium_expansion_payment_requires.388adbab", "premium expansion payment requires allowPremium=true", nil))
	}
	if input.GameData == nil {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"official data has no expansion level %d for kingdom %d", nextLevel, castle.KingdomID,
		), Localization.New("server.app.official_data_has_no.ca062be5", "official data has no expansion level {p0} for kingdom {p1}", Localization.Params{"p0": nextLevel, "p1": fmt.Sprintf("%d", castle.KingdomID)}))
	}
	if buildingExistsOnGround(castle, request.X, request.Y, request.Direction) {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"an expansion ground tile already exists at %d,%d direction %d", request.X, request.Y, request.Direction,
		), Localization.New("server.app.an_expansion_ground_tile.fc62a1e7", "an expansion ground tile already exists at {p0},{p1} direction {p2}", Localization.Params{"p0": request.X, "p1": request.Y, "p2": request.Direction}))
	}
	if !requireFresh {
		return castle, definition, initialGroundCount, payment, nil
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"castle %d does not have a fresh focused building layout", castle.ID,
		), Localization.New("server.app.castle_p_does_not.9d77d0f9", "castle {p0} does not have a fresh focused building layout", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf(
			"castle expansion rejected: %s", strings.Join(parts, "; "),
		), Localization.New("server.app.castle_expansion_rejected_p.54df4b4e", "castle expansion rejected: {p0}", Localization.Params{"p0": fmt.Sprintf("%s", strings.Join(parts, "; "))}))
	}
	if preview.NextExpansion == nil || preview.NextExpansion.ID != definition.ID {
		return State.CastleState{}, GameData.ExpansionDefinition{}, 0, "", Localization.WithError(fmt.Errorf("expansion catalog changed during live validation"), Localization.New("server.app.expansion_catalog_changed_during.362edd68", "expansion catalog changed during live validation", nil))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d is %s, not an expansion treasure chest", request.BuildingInstanceID, definition.DisplayName,
		), Localization.New("server.app.building_p_is_p.446ca746", "building {p0} is {p1}, not an expansion treasure chest", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": fmt.Sprintf("%s", definition.DisplayName)}).WithGameParam("p1", GameData.FirstOfficialNameKey(input.Language, definition.LocalizationKeys...), definition.DisplayName))
	}
	if requireFresh && (!building.Placed || building.Layer != State.BuildingLayerBD) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"expansion treasure chest %d is not a placed castle object", request.BuildingInstanceID,
		), Localization.New("server.app.expansion_treasure_chest_p.1a273b55", "expansion treasure chest {p0} is not a placed castle object", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
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
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building definition %d is not in the current official catalog", request.DefinitionID), Localization.New("server.app.building_definition_p_is.d79e02ae", "building definition {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", request.DefinitionID)}))
	}
	if request.X < 0 || request.Y < 0 || request.Rotation < 0 || request.Rotation > 3 {
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building placement must use non-negative coordinates and rotation 0 through 3"), Localization.New("server.app.building_placement_must_use.28f6e207", "building placement must use non-negative coordinates and rotation 0 through 3", nil))
	}
	if kind != buildingMutationConstruct && kind != buildingMutationPlace {
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("unsupported building placement kind %q", kind), Localization.New("server.app.unsupported_building_placement_kind.b88e4930", "unsupported building placement kind {p0}", Localization.Params{"p0": fmt.Sprintf("%q", kind)}))
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
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building placement is invalid: %s", issues[0].Message), Localization.New("server.app.building_placement_is_invalid.5454b243", "building placement is invalid: {p0}", Localization.Params{"p0": fmt.Sprintf("%s", issues[0].Message)}))
	}
	if kind == buildingMutationPlace {
		if definition.Storeable == nil || !*definition.Storeable {
			return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building definition %d is not placeable from ordinary storage", definition.ID), Localization.New("server.app.building_definition_p_is.b91e5189", "building definition {p0} is not placeable from ordinary storage", Localization.Params{"p0": fmt.Sprintf("%d", definition.ID)}))
		}
		if ordinaryStorageCount(input.State, request.DefinitionID) <= 0 {
			return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building definition %d is not present in ordinary storage", definition.ID), Localization.New("server.app.building_definition_p_is.022f50f1", "building definition {p0} is not present in ordinary storage", Localization.Params{"p0": fmt.Sprintf("%d", definition.ID)}))
		}
		return castle, definition, nil
	}
	if ordinaryStorageCount(input.State, request.DefinitionID) > 0 {
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building definition %d is present in ordinary storage; use building.place so inventory consumption is explicit",
			definition.ID,
		), Localization.New("server.app.building_definition_p_is.09fce280", "building definition {p0} is present in ordinary storage; use building.place so inventory consumption is explicit", Localization.Params{"p0": fmt.Sprintf("%d", definition.ID)}))
	}
	candidate, err := validatedConstructionCandidate(input, request)
	if err != nil {
		return State.CastleState{}, GameData.BuildingDefinition{}, err
	}
	if candidate.Definition.ID != definition.ID {
		return State.CastleState{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building definition %d did not produce a matching construction candidate", definition.ID), Localization.New("server.app.building_definition_p_did.95689947", "building definition {p0} did not produce a matching construction candidate", Localization.Params{"p0": fmt.Sprintf("%d", definition.ID)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building placement must use non-negative coordinates and rotation 0 through 3"), Localization.New("server.app.building_placement_must_use.28f6e207", "building placement must use non-negative coordinates and rotation 0 through 3", nil))
	}
	if definition.Movable != nil && !*definition.Movable {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is not movable", request.BuildingInstanceID), Localization.New("server.app.building_p_is_not.edcb08c7", "building {p0} is not movable", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
	}
	if requireFresh {
		if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is in the construction queue", request.BuildingInstanceID), Localization.New("server.app.building_p_is_in.acb50e8e", "building {p0} is in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
		}
		catalog, _ := input.GameData.BuildingCatalog()
		issues := Buildings.ValidatePlacement(castle, definition, Buildings.Placement{
			GridX: request.X, GridY: request.Y, Rotation: request.Rotation,
		}, catalog, request.BuildingInstanceID)
		if len(issues) > 0 {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building move is invalid: %s", issues[0].Message), Localization.New("server.app.building_move_is_invalid.d61190b5", "building move is invalid: {p0}", Localization.Params{"p0": fmt.Sprintf("%s", issues[0].Message)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d has no next official upgrade", request.BuildingInstanceID), Localization.New("server.app.building_p_has_no.837371f3", "building {p0} has no next official upgrade", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
	}
	if request.MaximumLevel > 0 && target.Level > request.MaximumLevel {
		capErr := Localization.WithError(fmt.Errorf("building %d refreshed next level %d exceeds maximum level %d", request.BuildingInstanceID, target.Level, request.MaximumLevel), Localization.New("server.app.building_p_refreshed_next.7c76faa3", "building {p0} refreshed next level {p1} exceeds maximum level {p2}", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": target.Level, "p2": request.MaximumLevel}))
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, capErr)
	}
	if !requireFresh {
		return castle, building, target, nil
	}
	if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is already in the construction queue", request.BuildingInstanceID), Localization.New("server.app.building_p_is_already.c5a50f7d", "building {p0} is already in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
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
	return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d has no upgrade candidate in the refreshed castle state", request.BuildingInstanceID), Localization.New("server.app.building_p_has_no.273a098a", "building {p0} has no upgrade candidate in the refreshed castle state", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is not storeable", request.BuildingInstanceID), Localization.New("server.app.building_p_is_not.295afc39", "building {p0} is not storeable", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
	}
	if requireFresh && buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is in the construction queue", request.BuildingInstanceID), Localization.New("server.app.building_p_is_in.acb50e8e", "building {p0} is in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d is not in the construction queue", request.BuildingInstanceID,
		), Localization.New("server.app.building_p_is_not.8bdf8853", "building {p0} is not in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
	}
	if !buildingOperationInProgress(building.ConstructionState) {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d is not in a finishable construction state (%d)", request.BuildingInstanceID, building.ConstructionState,
		), Localization.New("server.app.building_p_is_not.3d21cde7", "building {p0} is not in a finishable construction state ({p1})", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID), "p1": building.ConstructionState}))
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
		if requireFresh {
			return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
		}
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, err
	}
	option, err := officialBuildingTimeSkipOption(input.GameData, request.Minutes)
	if err != nil {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, err
	}
	if request.MinimumRemaining < 0 {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, 0, Localization.WithError(fmt.Errorf("minimumRemaining cannot be negative"), Localization.New("server.app.minimumremaining_cannot_be_negative.1793e608", "minimumRemaining cannot be negative", nil))
	}
	balance := input.State.Player.Currencies[option.CurrencyID]
	if balance < 1 || balance-1 < float64(request.MinimumRemaining) {
		return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, Localization.WithError(fmt.Errorf(
			"%d-minute time skip balance %.0f cannot preserve minimumRemaining %d",
			option.Minutes, balance, request.MinimumRemaining,
		), Localization.New("server.app.p_minute_time_skip.6643e4fa", "{p0}-minute time skip balance {p1} cannot preserve minimumRemaining {p2}", Localization.Params{"p0": option.Minutes, "p1": balance, "p2": request.MinimumRemaining}))
	}
	if requireFresh {
		if !buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, Localization.WithError(fmt.Errorf(
				"%w: building %d is not in the construction queue", Intent.ErrPlanStale, request.BuildingInstanceID,
			), Localization.New("server.app.intent_plan_became_stale.798f7aaf", "intent plan became stale before dispatch: building {p1} is not in the construction queue", Localization.Params{"p1": fmt.Sprintf("%d", request.BuildingInstanceID)}))
		}
		if !buildingOperationInProgress(building.ConstructionState) {
			return State.CastleState{}, State.Building{}, buildingTimeSkipOption{}, balance, Localization.WithError(fmt.Errorf(
				"%w: building %d is not in a skippable construction state (%d)",
				Intent.ErrPlanStale, request.BuildingInstanceID, building.ConstructionState,
			), Localization.New("server.app.intent_plan_became_stale.e1d30d0f", "intent plan became stale before dispatch: building {p1} is not in a skippable construction state ({p2, number})", Localization.Params{"p1": fmt.Sprintf("%d", request.BuildingInstanceID), "p2": building.ConstructionState}))
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
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("minutes must be one of 1, 5, 10, 30, 60, 300, or 1440"), Localization.New("server.app.minutes_must_be_one.64c74130", "minutes must be one of 1, 5, 10, 30, 60, 300, or 1440", nil))
	}
	if gameData == nil {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	currencies, err := gameData.Catalog("currencies")
	if err != nil {
		return buildingTimeSkipOption{}, err
	}
	rawCurrency, found := currencies.Find(strconv.FormatInt(int64(option.CurrencyID), 10))
	if !found {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("official currency %d is unavailable", option.CurrencyID), Localization.New("server.app.official_currency_p_is.0a59ea47", "official currency {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", option.CurrencyID)}))
	}
	var currency struct {
		JSONKey string `json:"JSONKey"`
	}
	if err := json.Unmarshal(rawCurrency, &currency); err != nil || currency.JSONKey != option.WireKey {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("official currency %d does not map to %s", option.CurrencyID, option.WireKey), Localization.New("server.app.official_currency_p_does.8473ad38", "official currency {p0} does not map to {p1}", Localization.Params{"p0": fmt.Sprintf("%d", option.CurrencyID), "p1": fmt.Sprintf("%s", option.WireKey)}))
	}
	values, err := gameData.Catalog("currencyMinutesSkipValues")
	if err != nil {
		return buildingTimeSkipOption{}, err
	}
	rawValue, found := values.FindByField("currencyID", strconv.FormatInt(int64(option.CurrencyID), 10))
	if !found {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("official minute-skip value for currency %d is unavailable", option.CurrencyID), Localization.New("server.app.official_minute_skip_value.245ceb44", "official minute-skip value for currency {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", option.CurrencyID)}))
	}
	var value struct {
		Minutes string `json:"MinutesSkipValue"`
	}
	if err := json.Unmarshal(rawValue, &value); err != nil || value.Minutes != strconv.Itoa(option.Minutes) {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("official currency %d does not represent a %d-minute skip", option.CurrencyID, option.Minutes), Localization.New("server.app.official_currency_p_does.6263c4ed", "official currency {p0} does not represent a {p1}-minute skip", Localization.Params{"p0": fmt.Sprintf("%d", option.CurrencyID), "p1": option.Minutes}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is not destructible", request.BuildingInstanceID), Localization.New("server.app.building_p_is_not.badbce19", "building {p0} is not destructible", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
	}
	if requireFresh {
		if buildingQueued(castle.BuildingQueue, request.BuildingInstanceID) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d is already in the construction queue", request.BuildingInstanceID), Localization.New("server.app.building_p_is_already.c5a50f7d", "building {p0} is already in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", request.BuildingInstanceID)}))
		}
		if !buildingQueueAvailable(castle.BuildingQueue) {
			return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("no building construction slot is available in castle %d", castle.ID), Localization.New("server.app.no_building_construction_slot.88c2f923", "no building construction slot is available in castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d is not idle (construction state %d)", buildingID, building.ConstructionState,
		), Localization.New("server.app.building_p_is_not.a488ef90", "building {p0} is not idle (construction state {p1})", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": building.ConstructionState}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d is not idle (construction state %d)", buildingID, building.ConstructionState,
		), Localization.New("server.app.building_p_is_not.a488ef90", "building {p0} is not idle (construction state {p1})", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": building.ConstructionState}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"upgradeable building instance %d is not in castle %d", buildingID, castleID,
		), Localization.New("server.app.upgradeable_building_instance_p.430291f1", "upgradeable building instance {p0} is not in castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": fmt.Sprintf("%d", castleID)}))
	}
	catalog, err := buildingCatalog(input.GameData)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	definition, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf(
			"building %d uses unknown official definition %d", buildingID, building.DefinitionID,
		), Localization.New("server.app.building_p_uses_unknown.ba392913", "building {p0} uses unknown official definition {p1}", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": fmt.Sprintf("%d", building.DefinitionID)}))
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
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("normal building instance %d is not in castle %d", buildingID, castleID), Localization.New("server.app.normal_building_instance_p.1b3f8e8e", "normal building instance {p0} is not in castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": fmt.Sprintf("%d", castleID)}))
	}
	catalog, err := buildingCatalog(input.GameData)
	if err != nil {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, err
	}
	definition, found := catalog.Definition(int64(building.DefinitionID))
	if !found {
		return State.CastleState{}, State.Building{}, GameData.BuildingDefinition{}, Localization.WithError(fmt.Errorf("building %d uses unknown official definition %d", buildingID, building.DefinitionID), Localization.New("server.app.building_p_uses_unknown.ba392913", "building {p0} uses unknown official definition {p1}", Localization.Params{"p0": fmt.Sprintf("%d", buildingID), "p1": fmt.Sprintf("%d", building.DefinitionID)}))
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
		return Buildings.Candidate{}, Localization.WithError(fmt.Errorf("building definition %d did not produce a construction candidate", request.DefinitionID), Localization.New("server.app.building_definition_p_did.e40ab988", "building definition {p0} did not produce a construction candidate", Localization.Params{"p0": fmt.Sprintf("%d", request.DefinitionID)}))
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
	return Localization.WithError(fmt.Errorf("building %s rejected: %s", action, strings.Join(parts, "; ")), Localization.New("server.app.building_p_rejected_p.8b0da55e", "building {p0} rejected: {p1}", Localization.Params{"p0": fmt.Sprintf("%s", action), "p1": fmt.Sprintf("%s", strings.Join(parts, "; "))}))
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
	castle, err := buildingCastle(application.State.ReadOnlyView(), verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID), Localization.New("server.app.castle_p_did_not.3dfcb7db", "castle {p0} did not return a fresh reconciled building snapshot", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	switch verification.Kind {
	case buildingMutationExpand:
		if len(castle.Layout.Ground) <= verification.InitialGroundCount {
			return Localization.WithError(fmt.Errorf("castle %d ground-tile count did not increase after expansion", castle.ID), Localization.New("server.app.castle_p_ground_tile.6a7196fc", "castle {p0} ground-tile count did not increase after expansion", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
		}
		if !buildingExistsOnGround(castle, verification.X, verification.Y, verification.Rotation) {
			return Localization.WithError(fmt.Errorf("expansion ground tile was not observed at %d,%d direction %d", verification.X, verification.Y, verification.Rotation), Localization.New("server.app.expansion_ground_tile_was.e645e1ce", "expansion ground tile was not observed at {p0},{p1} direction {p2}", Localization.Params{"p0": verification.X, "p1": verification.Y, "p2": verification.Rotation}))
		}
	case buildingMutationCollectGift:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found {
			return Localization.WithError(fmt.Errorf("expansion gift %d is still placed after collection", verification.BuildingInstanceID), Localization.New("server.app.expansion_gift_p_is.73f05965", "expansion gift {p0} is still placed after collection", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
		}
	case buildingMutationConstruct, buildingMutationPlace:
		if !buildingExistsAt(castle, verification.DefinitionID, verification.X, verification.Y, verification.Rotation) {
			return Localization.WithError(fmt.Errorf("building definition %d was not observed at %d,%d after placement", verification.DefinitionID, verification.X, verification.Y), Localization.New("server.app.building_definition_p_was.b3ecd8ae", "building definition {p0} was not observed at {p1},{p2} after placement", Localization.Params{"p0": fmt.Sprintf("%d", verification.DefinitionID), "p1": verification.X, "p2": verification.Y}))
		}
	case buildingMutationMove:
		building, found := buildingByID(castle, verification.BuildingInstanceID)
		if !found || building.GridX != verification.X || building.GridY != verification.Y || building.Rotation != verification.Rotation {
			return Localization.WithError(fmt.Errorf("building %d did not reconcile to %d,%d rotation %d", verification.BuildingInstanceID, verification.X, verification.Y, verification.Rotation), Localization.New("server.app.building_p_did_not.53fb92de", "building {p0} did not reconcile to {p1},{p2} rotation {p3}", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID), "p1": verification.X, "p2": verification.Y, "p3": verification.Rotation}))
		}
	case buildingMutationUpgrade:
		building, found := buildingByID(castle, verification.BuildingInstanceID)
		if !found {
			return Localization.WithError(fmt.Errorf("building %d disappeared after its upgrade was accepted", verification.BuildingInstanceID), Localization.New("server.app.building_p_disappeared_after.fb751e6f", "building {p0} disappeared after its upgrade was accepted", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
		}
		if building.DefinitionID != verification.TargetDefinitionID && !buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
			return Localization.WithError(fmt.Errorf("building %d is neither upgraded to %d nor present in the construction queue", verification.BuildingInstanceID, verification.TargetDefinitionID), Localization.New("server.app.building_p_is_neither.2407887d", "building {p0} is neither upgraded to {p1} nor present in the construction queue", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID), "p1": fmt.Sprintf("%d", verification.TargetDefinitionID)}))
		}
	case buildingMutationStore:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found {
			return Localization.WithError(fmt.Errorf("building %d is still placed after the store command", verification.BuildingInstanceID), Localization.New("server.app.building_p_is_still.60c925ef", "building {p0} is still placed after the store command", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
		}
		if ordinaryStorageCount(application.State.ReadOnlyView(), verification.DefinitionID) <= 0 {
			return Localization.WithError(fmt.Errorf("building definition %d was not observed in ordinary storage after storing", verification.DefinitionID), Localization.New("server.app.building_definition_p_was.8252d6ff", "building definition {p0} was not observed in ordinary storage after storing", Localization.Params{"p0": fmt.Sprintf("%d", verification.DefinitionID)}))
		}
	case buildingMutationDemolish:
		if _, found := buildingByID(castle, verification.BuildingInstanceID); found && !buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
			return Localization.WithError(fmt.Errorf("building %d is neither removed nor present in the demolition queue", verification.BuildingInstanceID), Localization.New("server.app.building_p_is_neither.bd2be4c1", "building {p0} is neither removed nor present in the demolition queue", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
		}
	default:
		return Localization.WithError(fmt.Errorf("unsupported building verification kind %q", verification.Kind), Localization.New("server.app.unsupported_building_verification_kind.0548b80f", "unsupported building verification kind {p0}", Localization.Params{"p0": fmt.Sprintf("%q", verification.Kind)}))
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
	castle, err := buildingCastle(application.State.ReadOnlyView(), verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID), Localization.New("server.app.castle_p_did_not.3dfcb7db", "castle {p0} did not return a fresh reconciled building snapshot", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	if buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) {
		return Localization.WithError(fmt.Errorf("building %d remains in the construction queue after a free completion", verification.BuildingInstanceID), Localization.New("server.app.building_p_remains_in.a9df18b6", "building {p0} remains in the construction queue after a free completion", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
	}
	building, found := buildingByID(castle, verification.BuildingInstanceID)
	if !found {
		if verification.InitialConstructionState == State.BuildingStateDisassembleStopped ||
			verification.InitialConstructionState == State.BuildingStateDisassembleInProgress {
			return nil
		}
		return Localization.WithError(fmt.Errorf("building %d disappeared after a non-demolition free completion", verification.BuildingInstanceID), Localization.New("server.app.building_p_disappeared_after.bf0c7b83", "building {p0} disappeared after a non-demolition free completion", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
	}
	if buildingOperationInProgress(building.ConstructionState) {
		return Localization.WithError(fmt.Errorf("building %d remains in construction state %d after a free completion", verification.BuildingInstanceID, building.ConstructionState), Localization.New("server.app.building_p_remains_in.a4a364c3", "building {p0} remains in construction state {p1} after a free completion", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID), "p1": building.ConstructionState}))
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
	state := application.State.ReadOnlyView()
	balance := state.Player.Currencies[verification.SkipCurrencyID]
	if balance != verification.InitialSkipBalance-1 {
		return Localization.WithError(fmt.Errorf(
			"time-skip currency %d balance is %.0f; expected %.0f after one use",
			verification.SkipCurrencyID, balance, verification.InitialSkipBalance-1,
		), Localization.New("server.app.time_skip_currency_p.a0882924", "time-skip currency {p0} balance is {p1}; expected {p2} after one use", Localization.Params{"p0": fmt.Sprintf("%d", verification.SkipCurrencyID), "p1": balance, "p2": verification.InitialSkipBalance - 1}))
	}
	castle, err := buildingCastle(state, verification.CastleID)
	if err != nil {
		return err
	}
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("castle %d did not return a fresh reconciled building snapshot", castle.ID), Localization.New("server.app.castle_p_did_not.3dfcb7db", "castle {p0} did not return a fresh reconciled building snapshot", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	building, found := buildingByID(castle, verification.BuildingInstanceID)
	if !found {
		if verification.InitialConstructionState == State.BuildingStateDisassembleStopped ||
			verification.InitialConstructionState == State.BuildingStateDisassembleInProgress {
			return nil
		}
		return Localization.WithError(fmt.Errorf("building %d disappeared after a non-demolition time skip", verification.BuildingInstanceID), Localization.New("server.app.building_p_disappeared_after.205f2f55", "building {p0} disappeared after a non-demolition time skip", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
	}
	if buildingQueued(castle.BuildingQueue, verification.BuildingInstanceID) &&
		building.ProgressSec <= verification.InitialProgressSec {
		return Localization.WithError(fmt.Errorf("building %d did not advance after a time skip", verification.BuildingInstanceID), Localization.New("server.app.building_p_did_not.60035177", "building {p0} did not advance after a time skip", Localization.Params{"p0": fmt.Sprintf("%d", verification.BuildingInstanceID)}))
	}
	return nil
}

func buildingMutationStep(name string, opcode string, payload json.RawMessage) Intent.Step {
	step := commandStep(name, opcode, payload, opcode)
	step.CaptureResponse = true
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func buildingResolverStep(name string, resolver string, arguments json.RawMessage, awaitOpcode string, descriptors ...*Localization.Message) Intent.Step {
	return Intent.Step{
		Name: name, NameDescriptor: Localization.First(descriptors), Resolver: resolver, ResolverArguments: arguments, AwaitOpcode: awaitOpcode,
		TimeoutMillis: 10_000, SuccessCodes: []int{0}, CaptureResponse: true,
		ResponseBarrier: Intent.ResponseBarrierCommitted,
	}
}

func buildingCastle(state State.GameState, castleID State.CastleID) (State.CastleState, error) {
	castle, found := state.Castles[castleID]
	if !found || castleID <= 0 {
		return State.CastleState{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", castleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", castleID)}))
	}
	return castle, nil
}

func buildingCatalog(gameData *GameData.Store) (*GameData.BuildingCatalog, error) {
	if gameData == nil {
		return nil, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	return gameData.BuildingCatalog()
}

func buildingStateIsFresh(state State.GameState, castleID State.CastleID) bool {
	castle, found := state.Castles[castleID]
	return found && castle.Focused && !castle.Layout.ObservedAt.IsZero()
}

func requireFreshBuildingState(castle State.CastleState) error {
	if !castle.Focused || castle.Layout.ObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("castle %d does not have a fresh focused building layout", castle.ID), Localization.New("server.app.castle_p_does_not.9d77d0f9", "castle {p0} does not have a fresh focused building layout", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	if castle.BuildingQueue.ObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("castle %d does not have a fresh building construction queue", castle.ID), Localization.New("server.app.castle_p_does_not.2bb143cb", "castle {p0} does not have a fresh building construction queue", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
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

func (application *Application) guardBuildingUpgrade(_ context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("%w: current building upgrade authority is unavailable", Intent.ErrPlanStale)
	}
	data, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("%w: official building data is unavailable", Intent.ErrPlanStale)
	}
	return validateFinalBuildingUpgrade(Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: data}, arguments)
}

func validateFinalBuildingUpgrade(input Intent.PlanningContext, arguments json.RawMessage) error {
	var args buildingUpgradeResolverArguments
	if err := decodeIntentArguments(arguments, &args); err != nil {
		return err
	}
	if args.PremiumMode != "" {
		return fmt.Errorf("%w: unattended ruby confirmation is not allowed", Intent.ErrPlanStale)
	}
	_, _, target, err := validatedBuildingUpgrade(input, args.Request, true)
	if err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	if args.TargetDefinitionID <= 0 || target.ID != args.TargetDefinitionID {
		return fmt.Errorf("%w: upgrade target changed", Intent.ErrPlanStale)
	}
	return nil
}

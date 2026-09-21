package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/State"
)

type kingdomResourceShipmentGood struct {
	ResourceID State.ResourceID `json:"resourceId"`
	Amount     int64            `json:"amount"`
}

const (
	kingdomResourceDeliveryRatio         = 0.90
	autoFoodBalanceResourceWorkflowOwner = "autoFoodBalance"
)

type kingdomResourceShipmentRequest struct {
	SourceCastleID     State.CastleID                `json:"sourceCastleId"`
	TargetCastleID     State.CastleID                `json:"targetCastleId,omitempty"`
	TargetKingdomID    State.KingdomID               `json:"targetKingdomId"`
	ResourceID         State.ResourceID              `json:"resourceId,omitempty"`
	Amount             int64                         `json:"amount,omitempty"`
	Goods              []kingdomResourceShipmentGood `json:"goods,omitempty"`
	WorkflowOwner      string                        `json:"workflowOwner,omitempty"`
	EnforceTargetCap   bool                          `json:"enforceTargetCapacity,omitempty"`
	SettleAfterSkip    bool                          `json:"settleAfterTimeSkip,omitempty"`
	TimeSkipID         string                        `json:"timeSkipId,omitempty"`
	MinimumRemaining   int64                         `json:"minimumRemaining,omitempty"`
	HorseTravelBoostID int                           `json:"horseTravelBoostId,omitempty"`
	MinimumCoinReserve float64                       `json:"minimumCoinReserve,omitempty"`
}

type kingdomResourceSettlementRequest struct {
	Owner           string          `json:"owner"`
	TargetKingdomID State.KingdomID `json:"targetKingdomId"`
}

type kingdomResourceSkipRequest struct {
	TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
	TimeSkipID       string          `json:"timeSkipId"`
	MinimumRemaining int64           `json:"minimumRemaining,omitempty"`
}

type kingdomResourcePendingGuard struct {
	TargetKingdomID State.KingdomID `json:"targetKingdomId"`
}

type kingdomTransportAvailabilityGuard struct {
	TargetKingdomID  State.KingdomID               `json:"targetKingdomId"`
	TransportKind    string                        `json:"transportKind"`
	SourceCastleID   State.CastleID                `json:"sourceCastleId,omitempty"`
	Goods            []kingdomResourceShipmentGood `json:"goods,omitempty"`
	TargetCastleID   State.CastleID                `json:"targetCastleId,omitempty"`
	DeliveryRatio    float64                       `json:"deliveryRatio,omitempty"`
	EnforceTargetCap bool                          `json:"enforceTargetCapacity,omitempty"`
}

type resourceTargetCapacityGuard struct {
	TargetCastleID State.CastleID                `json:"targetCastleId"`
	Goods          []kingdomResourceShipmentGood `json:"goods"`
	DeliveryRatio  float64                       `json:"deliveryRatio"`
}

func planResourceLogisticsRefresh(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	claims := []string{"resource-transport"}
	steps := []Intent.Step{
		commandStep("Refresh kingdom transports", "kpi", json.RawMessage(`{}`), "kpi", Localization.New("server.app.refresh_kingdom_transports.ad5d0438", "Refresh kingdom transports", nil)),
	}
	summary := "Refresh kingdom-resource logistics state"
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.refresh_kingdom_resource_logistics.4a8cc316", "Refresh kingdom-resource logistics state", nil)

	marketCastle, marketRequired, err := resourceLogisticsMarketCastle(input)
	if err != nil {
		return Intent.Plan{}, err
	}
	if marketRequired {
		claims = append(claims, "castle-focus")
		originalCastle, hadOriginalFocus := resourceLogisticsFocusedCastle(input.State)
		// GAA retains the selected castle but leaves the session in map mode.
		// CMI needs castle context even when the cached Focused flag is true.
		steps = append(steps, castleContextSteps(input, marketCastle)...)
		steps = append(steps,
			commandStep("Refresh caravan boosters", "boi", json.RawMessage(`{}`), "boi", Localization.New("server.app.refresh_caravan_boosters.5aebc1d2", "Refresh caravan boosters", nil)),
			commandStep("Refresh market capacity", "cmi", json.RawMessage(`{"S":1,"KID":-1}`), "cmi", Localization.New("server.app.refresh_market_capacity.6d2166f5", "Refresh market capacity", nil)),
		)
		if hadOriginalFocus && originalCastle.ID != marketCastle.ID {
			steps = append(steps, castleFocusStep(originalCastle))
		}
		summary = "Refresh market and kingdom-resource logistics state"
		summaryLocalizationMessage = Localization.New("server.app.refresh_market_and_kingdom.b7274adb", "Refresh market and kingdom-resource logistics state", nil)
	}

	return Intent.Plan{Claims: claims, Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage), Steps: steps}, nil
}

func resourceLogisticsMarketCastle(input Intent.PlanningContext) (State.CastleState, bool, error) {
	kingdomCastleCounts := map[State.KingdomID]int{}
	for _, castle := range input.State.Castles {
		kingdomCastleCounts[castle.KingdomID]++
	}
	hasSameKingdomPair := false
	for _, count := range kingdomCastleCounts {
		if count > 1 {
			hasSameKingdomPair = true
			break
		}
	}
	if !hasSameKingdomPair {
		return State.CastleState{}, false, nil
	}
	if !State.NextMarketBarrowLeaseRelease(input.State, time.Now().UTC()).IsZero() {
		return State.CastleState{}, false, nil
	}
	if input.GameData == nil {
		return State.CastleState{}, false, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}

	castleIDs := make([]State.CastleID, 0, len(input.State.Castles))
	for castleID := range input.State.Castles {
		castleIDs = append(castleIDs, castleID)
	}
	sort.Slice(castleIDs, func(left, right int) bool { return castleIDs[left] < castleIDs[right] })
	selected := State.CastleState{}
	for _, castleID := range castleIDs {
		castle := input.State.Castles[castleID]
		if kingdomCastleCounts[castle.KingdomID] < 2 {
			continue
		}
		hasMarketplace, err := input.GameData.CastleHasMarketplace(castle)
		if err != nil {
			return State.CastleState{}, false, fmt.Errorf("check marketplace at %s: %w", castleLabel(castle), err)
		}
		if !hasMarketplace {
			continue
		}
		if castle.Focused {
			return castle, true, nil
		}
		if selected.ID <= 0 {
			selected = castle
		}
	}
	return selected, selected.ID > 0, nil
}

func resourceLogisticsFocusedCastle(gameState State.GameState) (State.CastleState, bool) {
	for _, castle := range gameState.Castles {
		if castle.Focused {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

func planResourceShipment(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomResourceShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if !targetExists || request.TargetCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("target castle %d is not in the current player state", request.TargetCastleID), Localization.New("server.app.target_castle_p_is.ac5c0cba", "target castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID)}))
	}
	if source.ID == target.ID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("resource shipments require distinct source and target castles"), Localization.New("server.app.resource_shipments_require_distinct.f74da6e2", "resource shipments require distinct source and target castles", nil))
	}

	if source.KingdomID == target.KingdomID {
		goods, err := normalizeKingdomResourceGoods(request)
		if err != nil {
			return Intent.Plan{}, err
		}
		if len(goods) != 1 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("same-kingdom market shipments require exactly one resource"), Localization.New("server.app.same_kingdom_market_shipments.91fc1906", "same-kingdom market shipments require exactly one resource", nil))
		}
		marketArguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "targetCastleId": target.ID,
			"resourceId": goods[0].ResourceID, "amount": goods[0].Amount,
			"workflowOwner": request.WorkflowOwner, "enforceTargetCapacity": request.EnforceTargetCap,
			"horseTravelBoostId": request.HorseTravelBoostID, "minimumCoinReserve": request.MinimumCoinReserve,
		})
		return planMarketResourceShipment(ctx, input, marketArguments)
	}

	request.TargetKingdomID = target.KingdomID
	kingdomArguments, _ := json.Marshal(request)
	return planKingdomResourceShipment(ctx, input, kingdomArguments)
}

func planMarketResourceShipment(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID     State.CastleID   `json:"sourceCastleId"`
		TargetCastleID     State.CastleID   `json:"targetCastleId"`
		ResourceID         State.ResourceID `json:"resourceId"`
		Amount             int64            `json:"amount"`
		WorkflowOwner      string           `json:"workflowOwner,omitempty"`
		EnforceTargetCap   bool             `json:"enforceTargetCapacity,omitempty"`
		HorseTravelBoostID int              `json:"horseTravelBoostId,omitempty"`
		MinimumCoinReserve float64          `json:"minimumCoinReserve,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if !targetExists || request.TargetCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("target castle %d is not in the current player state", request.TargetCastleID), Localization.New("server.app.target_castle_p_is.ac5c0cba", "target castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID)}))
	}
	if source.ID == target.ID || source.KingdomID != target.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("market shipments require distinct castles in the same kingdom"), Localization.New("server.app.market_shipments_require_distinct.da55e0f6", "market shipments require distinct castles in the same kingdom", nil))
	}
	if err := validateAutoFoodBalanceResourceRoute(
		ctx, request.WorkflowOwner, source.KingdomID, target.KingdomID,
	); err != nil {
		return Intent.Plan{}, err
	}
	hasMarketplace, err := input.GameData.CastleHasMarketplace(source)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("check marketplace at %s: %w", castleLabel(source), err)
	}
	if !hasMarketplace {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d has no Marketplace building", source.ID), Localization.New("server.app.source_castle_p_has.a451fb87", "source castle {p0} has no Marketplace building", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
	}
	resourceKey, err := officialResourceJSONKey(input.GameData, request.ResourceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	resourceName := officialResourceDisplayName(input.GameData, request.ResourceID, resourceKey)
	if request.Amount <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("amount must be positive"), Localization.New("server.app.amount_must_be_positive.86cf8f4f", "amount must be positive", nil))
	}
	if request.MinimumCoinReserve < 0 || math.IsNaN(request.MinimumCoinReserve) || math.IsInf(request.MinimumCoinReserve, 0) || request.MinimumCoinReserve >= math.Exp2(63) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("minimumCoinReserve is invalid"), Localization.New("server.app.minimumcoinreserve_is_invalid.dc22c9e5", "minimumCoinReserve is invalid", nil))
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, err
	}
	horseTravelBoostID, err := resolveCastleHorseTravelBoostID(
		input.GameData, source, request.HorseTravelBoostID,
	)
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("resolve market horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_market_horse_travel.0d4c0b02", "resolve market horse travel boost", nil), err))
	}
	if source.Resources[request.ResourceID].Amount < float64(request.Amount) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceName), gameNameDescriptor(Localization.New("server.app.source_castle_p_has.09417080", "source castle {p0} has insufficient {p1}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": fmt.Sprintf("%s", resourceName)}), input, "p1", "resources", int64(request.ResourceID), resourceName))
	}
	goods := []kingdomResourceShipmentGood{{ResourceID: request.ResourceID, Amount: request.Amount}}
	if request.EnforceTargetCap {
		if err := validateResourceTargetCapacity(input.State, target.ID, goods, 1, time.Now().UTC()); err != nil {
			return Intent.Plan{}, err
		}
	}
	market, observed := input.State.Market.Castles[source.ID]
	availableBarrows := State.AvailableMarketBarrowsAt(input.State, market, time.Now().UTC())
	if !observed || input.State.Market.ObservedAt.IsZero() || availableBarrows <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d has no observed available market barrows", source.ID), Localization.New("server.app.source_castle_p_has.f0ba5631", "source castle {p0} has no observed available market barrows", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		SourceID  State.CastleID  `json:"SID"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		HorseWID  int             `json:"HBW"`
		Goods     [][]any         `json:"G"`
		PaidTime  int             `json:"PTT"`
		DelaySec  int             `json:"SD"`
	}{
		KingdomID: source.KingdomID, SourceID: source.ID, TargetX: target.X, TargetY: target.Y,
		HorseWID: horseTravelBoostID, Goods: [][]any{{resourceKey, request.Amount}},
	})
	shipmentStep := commandStep("Start market shipment", "crm", payload, "crm", Localization.New("server.app.start_market_shipment.b771ea76", "Start market shipment", nil))
	if request.MinimumCoinReserve > 0 {
		shipmentStep.CoinCost = &Intent.CoinCostRequirement{
			Reserve: int64(math.Ceil(request.MinimumCoinReserve)), Source: "configured market coin reserve",
		}
	}
	steps := []Intent.Step{shipmentStep}
	if request.EnforceTargetCap {
		guardArguments, _ := json.Marshal(resourceTargetCapacityGuard{
			TargetCastleID: target.ID, Goods: goods, DeliveryRatio: 1,
		})
		steps = append([]Intent.Step{Intent.RebuildOnResume(Intent.Step{
			Name: "Verify destination resource storage", NameDescriptor: Localization.New("server.app.verify_destination_resource_storage.9ee76d5b", "Verify destination resource storage", nil), Action: "resources.verify_target_capacity",
			ActionArguments: guardArguments,
		})}, steps...)
	}
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"castle:" + strconv.FormatInt(int64(target.ID), 10),
		},
		Summary: fmt.Sprintf("Ship %d %s from %s to %s", request.Amount, resourceName, castleLabel(source), castleLabel(target)), SummaryDescriptor: gameNameDescriptor(Localization.New("server.app.ship_p_p_from.7de39e09", "Ship {p0} {p1} from {p2} to {p3}", Localization.Params{"p0": request.Amount, "p1": fmt.Sprintf("%s", resourceName), "p2": fmt.Sprintf("%s", castleLabel(source)), "p3": fmt.Sprintf("%s", castleLabel(target))}), input, "p1", "resources", int64(request.ResourceID), resourceName),
		Steps: steps,
	}, nil
}

func planKingdomResourceShipment(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomResourceShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	goods, err := normalizeKingdomResourceGoods(request)
	if err != nil {
		return Intent.Plan{}, err
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if request.TargetKingdomID < 0 || request.TargetKingdomID == source.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("targetKingdomId must identify a different owned kingdom"), Localization.New("server.app.targetkingdomid_must_identify_a.fa4a367f", "targetKingdomId must identify a different owned kingdom", nil))
	}
	if err := validateAutoFoodBalanceResourceRoute(
		ctx, request.WorkflowOwner, source.KingdomID, request.TargetKingdomID,
	); err != nil {
		return Intent.Plan{}, err
	}
	targetOwned := false
	for _, castle := range input.State.Castles {
		if castle.KingdomID == request.TargetKingdomID {
			targetOwned = true
			break
		}
	}
	if !targetOwned {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom %d has no castle in the current player state", request.TargetKingdomID), Localization.New("server.app.kingdom_p_has_no.9dd00625", "kingdom {p0} has no castle in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
	}
	var target State.CastleState
	if request.TargetCastleID > 0 {
		var targetExists bool
		target, targetExists = input.State.Castles[request.TargetCastleID]
		if !targetExists || target.KingdomID != request.TargetKingdomID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("target castle %d is not in kingdom %d", request.TargetCastleID, request.TargetKingdomID), Localization.New("server.app.target_castle_p_is.9e66647b", "target castle {p0} is not in kingdom {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID), "p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[request.TargetKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom transport to %d is not observed as unlocked", request.TargetKingdomID), Localization.New("server.app.kingdom_transport_to_p.5f8b17e2", "kingdom transport to {p0} is not observed as unlocked", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
	}
	if kingdomResourceTransportBusy(input.State, request.TargetKingdomID) {
		if strings.TrimSpace(request.WorkflowOwner) != "" {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf(
				"%w: kingdom %d already has a pending or settling resource transport",
				Intent.ErrPlanStale, request.TargetKingdomID,
			), Localization.New("server.app.intent_plan_became_stale.b131d670", "intent plan became stale before dispatch: kingdom {p1} already has a pending or settling resource transport", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom %d already has a pending or settling resource transport", request.TargetKingdomID), Localization.New("server.app.kingdom_p_already_has.4becafdf", "kingdom {p0} already has a pending or settling resource transport", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
	}
	wireGoods := make([][]any, 0, len(goods))
	summaryGoods := make([]string, 0, len(goods))
	for _, good := range goods {
		resourceKey, resourceErr := officialResourceJSONKey(input.GameData, good.ResourceID)
		if resourceErr != nil {
			return Intent.Plan{}, resourceErr
		}
		resourceName := officialResourceDisplayName(input.GameData, good.ResourceID, resourceKey)
		if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceName), gameNameDescriptor(Localization.New("server.app.source_castle_p_has.09417080", "source castle {p0} has insufficient {p1}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": fmt.Sprintf("%s", resourceName)}), input, "p1", "resources", int64(good.ResourceID), resourceName))
		}
		wireGoods = append(wireGoods, []any{resourceKey, good.Amount})
		summaryGoods = append(summaryGoods, fmt.Sprintf("%d %s", good.Amount, resourceName))
	}
	if request.EnforceTargetCap {
		if target.ID <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("targetCastleId is required when destination storage enforcement is enabled"), Localization.New("server.app.targetcastleid_is_required_when.b17f97ad", "targetCastleId is required when destination storage enforcement is enabled", nil))
		}
		if err := validateResourceTargetCapacity(
			input.State, target.ID, goods, kingdomResourceDeliveryRatio, time.Now().UTC(),
		); err != nil {
			return Intent.Plan{}, err
		}
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		Goods          [][]any         `json:"G"`
	}{source.ID, source.KingdomID, request.TargetKingdomID, wireGoods})
	resourceRefreshPayload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"AID"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.ID, source.KingdomID})
	summary := fmt.Sprintf("Ship %s from kingdom %d to %d", strings.Join(summaryGoods, " and "), source.KingdomID, request.TargetKingdomID)
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.ship_p_from_kingdom.a2507dee", "Ship {p0} from kingdom {p1} to {p2}", Localization.Params{"p0": fmt.Sprintf("%s", strings.Join(summaryGoods, " and ")), "p1": fmt.Sprintf("%d", source.KingdomID), "p2": fmt.Sprintf("%d", request.TargetKingdomID)})
	if request.TargetCastleID > 0 {
		summary = fmt.Sprintf("Ship %s from %s to %s by kingdom transport", strings.Join(summaryGoods, " and "), castleLabel(source), castleLabel(target))
		summaryLocalizationMessage = Localization.New("server.app.ship_p_from_p.38fc7db7", "Ship {p0} from {p1} to {p2} by kingdom transport", Localization.Params{"p0": fmt.Sprintf("%s", strings.Join(summaryGoods, " and ")), "p1": fmt.Sprintf("%s", castleLabel(source)), "p2": fmt.Sprintf("%s", castleLabel(target))})
	}
	guardArguments, _ := json.Marshal(kingdomTransportAvailabilityGuard{
		TargetKingdomID: request.TargetKingdomID, TransportKind: "resource",
		SourceCastleID: source.ID, Goods: goods, TargetCastleID: target.ID,
		DeliveryRatio: kingdomResourceDeliveryRatio, EnforceTargetCap: request.EnforceTargetCap,
	})
	consumeArguments, _ := json.Marshal(kingdomResourceShipmentRequest{
		SourceCastleID: source.ID, TargetCastleID: target.ID, TargetKingdomID: request.TargetKingdomID,
		Goods: goods, WorkflowOwner: strings.TrimSpace(request.WorkflowOwner),
	})
	claims := []string{
		"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		"kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
	}
	if request.EnforceTargetCap {
		claims = append(claims, "castle:"+strconv.FormatInt(int64(target.ID), 10))
	}
	steps := []Intent.Step{
		kingdomTransportContextStep(),
		contextCommandStep("Refresh kingdom resource donor", "grc", resourceRefreshPayload, "grc").WithNameDescriptor(Localization.New("server.app.refresh_kingdom_resource_donor.dd4e788a", "Refresh kingdom resource donor", nil)),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Verify kingdom resource transport and donor balance", NameDescriptor: Localization.New("server.app.verify_kingdom_resource_transport.83b46fc2", "Verify kingdom resource transport and donor balance", nil), Action: "kingdom.transport.verify_available",
			ActionArguments: guardArguments,
		}),
		commandStep("Start kingdom resource shipment", "kgt", payload, "kgt", Localization.New("server.app.start_kingdom_resource_shipment.13ab9230", "Start kingdom resource shipment", nil)),
		{Name: "Consume confirmed donor resources", NameDescriptor: Localization.New("server.app.consume_confirmed_donor_resources.d45042df", "Consume confirmed donor resources", nil), Action: "resources.kingdom.consume_source", ActionArguments: consumeArguments},
	}
	if strings.TrimSpace(request.TimeSkipID) != "" {
		skipStep, currencyID, _, skipErr := kingdomResourceSkipStep(input, kingdomResourceSkipRequest{
			TargetKingdomID: request.TargetKingdomID, TimeSkipID: request.TimeSkipID,
			MinimumRemaining: request.MinimumRemaining,
		}, false)
		if skipErr != nil {
			return Intent.Plan{}, skipErr
		}
		skipStep.Name = "Immediately skip kingdom resource transport time"
		steps = append(steps, skipStep, timeSkipConsumeStep(input, currencyID))
		claims = append(claims, "currency:"+strconv.FormatInt(int64(currencyID), 10))
	}
	if request.SettleAfterSkip {
		workflowOwner := strings.TrimSpace(request.WorkflowOwner)
		if strings.TrimSpace(request.TimeSkipID) == "" {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("settleAfterTimeSkip requires timeSkipId"), Localization.New("server.app.settleaftertimeskip_requires_timeskipid.4d5cf4f7", "settleAfterTimeSkip requires timeSkipId", nil))
		}
		if workflowOwner == "" || target.ID <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("settleAfterTimeSkip requires workflowOwner and targetCastleId"), Localization.New("server.app.settleaftertimeskip_requires_workflowowner_and.61981e73", "settleAfterTimeSkip requires workflowOwner and targetCastleId", nil))
		}
		targetRefreshPayload, _ := json.Marshal(struct {
			CastleID  State.CastleID  `json:"AID"`
			KingdomID State.KingdomID `json:"KID"`
		}{target.ID, target.KingdomID})
		settlementArguments, _ := json.Marshal(kingdomResourceSettlementRequest{
			Owner: workflowOwner, TargetKingdomID: request.TargetKingdomID,
		})
		steps = append(steps,
			kingdomTransportContextStep(),
			contextCommandStep("Refresh skipped kingdom resource destination", "grc", targetRefreshPayload, "grc").WithNameDescriptor(Localization.New("server.app.refresh_skipped_kingdom_resource.cca0a5d0", "Refresh skipped kingdom resource destination", nil)),
			Intent.Step{
				Name: "Complete immediately skipped kingdom resource workflow", NameDescriptor: Localization.New("server.app.complete_immediately_skipped_kingdom.0d48f7e0", "Complete immediately skipped kingdom resource workflow", nil),
				Action: "resources.kingdom.complete_workflow", ActionArguments: settlementArguments,
			},
		)
		if !request.EnforceTargetCap {
			claims = append(claims, "castle:"+strconv.FormatInt(int64(target.ID), 10))
		}
	}
	return Intent.Plan{Claims: claims, Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage), Steps: steps}, nil
}

func validateAutoFoodBalanceResourceRoute(
	ctx context.Context,
	workflowOwner string,
	sourceKingdomID State.KingdomID,
	targetKingdomID State.KingdomID,
) error {
	metadata := Outbound.MetadataFromContext(ctx)
	if strings.TrimSpace(workflowOwner) != autoFoodBalanceResourceWorkflowOwner &&
		strings.TrimSpace(metadata.Actor) != "automation:"+autoFoodBalanceResourceWorkflowOwner {
		return nil
	}
	berimondKingdomID := State.KingdomID(GameData.BerimondKingdomID)
	if sourceKingdomID == berimondKingdomID || targetKingdomID == berimondKingdomID {
		return Localization.WithError(fmt.Errorf("Auto Food cannot send resources to or from Berimond castles"), Localization.New("server.app.auto_food_cannot_send.4f184448", "Auto Food cannot send resources to or from Berimond castles", nil))
	}
	return nil
}

func kingdomResourceTransportPending(gameState State.GameState, kingdomID State.KingdomID) bool {
	for _, pending := range gameState.KingdomTransport.Pending {
		if pending.KingdomID == kingdomID {
			return true
		}
	}
	return false
}

func kingdomResourceTransportBusy(gameState State.GameState, kingdomID State.KingdomID) bool {
	if kingdomResourceTransportPending(gameState, kingdomID) {
		return true
	}
	_, settling := gameState.KingdomTransport.ResourceWorkflows[kingdomID]
	return settling
}

func kingdomTroopTransportPending(gameState State.GameState, kingdomID State.KingdomID) bool {
	for _, pending := range gameState.KingdomTransport.PendingUnits {
		if pending.KingdomID == kingdomID {
			return true
		}
	}
	return false
}

func (application *Application) verifyKingdomTransportAvailable(_ context.Context, arguments json.RawMessage) error {
	var guard kingdomTransportAvailabilityGuard
	if err := decodeIntentArguments(arguments, &guard); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("kingdom transport state is unavailable"), Localization.New("server.app.kingdom_transport_state_is.22a5dd6a", "kingdom transport state is unavailable", nil))
	}
	gameState := application.State.ReadOnlyView()
	switch guard.TransportKind {
	case "resource":
		if kingdomResourceTransportBusy(gameState, guard.TargetKingdomID) {
			return Localization.WithError(fmt.Errorf(
				"%w: kingdom %d has a pending or settling resource transport",
				Intent.ErrPlanStale, guard.TargetKingdomID,
			), Localization.New("server.app.intent_plan_became_stale.33e57a2c", "intent plan became stale before dispatch: kingdom {p1} has a pending or settling resource transport", Localization.Params{"p1": fmt.Sprintf("%d", guard.TargetKingdomID)}))
		}
		if guard.SourceCastleID > 0 || len(guard.Goods) > 0 {
			source, found := gameState.Castles[guard.SourceCastleID]
			if !found || guard.SourceCastleID <= 0 {
				return Localization.WithError(fmt.Errorf("%w: kingdom resource donor %d is unavailable", Intent.ErrPlanStale, guard.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.1c73da7e", "intent plan became stale before dispatch: kingdom resource donor {p1} is unavailable", Localization.Params{"p1": fmt.Sprintf("%d", guard.SourceCastleID)}))
			}
			goods, err := normalizeKingdomResourceGoods(kingdomResourceShipmentRequest{Goods: guard.Goods})
			if err != nil {
				return err
			}
			for _, good := range goods {
				if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
					return Localization.WithError(fmt.Errorf(
						"%w: kingdom resource donor %d has insufficient resource %d after refresh",
						Intent.ErrPlanStale, source.ID, good.ResourceID,
					), Localization.New("server.app.intent_plan_became_stale.204897c7", "intent plan became stale before dispatch: kingdom resource donor {p1} has insufficient resource {p2} after refresh", Localization.Params{"p1": fmt.Sprintf("%d", source.ID), "p2": fmt.Sprintf("%d", good.ResourceID)}))
				}
			}
		}
		if guard.EnforceTargetCap {
			if err := validateResourceTargetCapacity(
				gameState, guard.TargetCastleID, guard.Goods, guard.DeliveryRatio, time.Now().UTC(),
			); err != nil {
				return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
			}
		}
	case "troop":
		if kingdomTroopTransportPending(gameState, guard.TargetKingdomID) {
			return Localization.WithError(fmt.Errorf(
				"%w: kingdom %d has a pending or settling troop transport",
				Intent.ErrPlanStale, guard.TargetKingdomID,
			), Localization.New("server.app.intent_plan_became_stale.bbc42730", "intent plan became stale before dispatch: kingdom {p1} has a pending or settling troop transport", Localization.Params{"p1": fmt.Sprintf("%d", guard.TargetKingdomID)}))
		}
	default:
		return Localization.WithError(fmt.Errorf("unsupported kingdom transport kind %q", guard.TransportKind), Localization.New("server.app.unsupported_kingdom_transport_kind.29a11978", "unsupported kingdom transport kind {p0}", Localization.Params{"p0": fmt.Sprintf("%q", guard.TransportKind)}))
	}
	return nil
}

func (application *Application) verifyKingdomResourceTransportPending(_ context.Context, arguments json.RawMessage) error {
	var guard kingdomResourcePendingGuard
	if err := decodeIntentArguments(arguments, &guard); err != nil {
		return err
	}
	if guard.TargetKingdomID < 0 {
		return Localization.WithError(fmt.Errorf("targetKingdomId is required"), Localization.New("server.app.targetkingdomid_is_required.2e4ab030", "targetKingdomId is required", nil))
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("kingdom transport state is unavailable"), Localization.New("server.app.kingdom_transport_state_is.22a5dd6a", "kingdom transport state is unavailable", nil))
	}
	for _, pending := range application.State.ReadOnlyView().KingdomTransport.Pending {
		if pending.KingdomID == guard.TargetKingdomID && pending.RemainingSec > 0 {
			return nil
		}
	}
	return Localization.WithError(fmt.Errorf(
		"%w: kingdom %d resource transport is no longer pending",
		Intent.ErrPlanStale, guard.TargetKingdomID,
	), Localization.New("server.app.intent_plan_became_stale.e6c57ad8", "intent plan became stale before dispatch: kingdom {p1} resource transport is no longer pending", Localization.Params{"p1": fmt.Sprintf("%d", guard.TargetKingdomID)}))
}

func (application *Application) verifyResourceTargetCapacity(_ context.Context, arguments json.RawMessage) error {
	var guard resourceTargetCapacityGuard
	if err := decodeIntentArguments(arguments, &guard); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("resource target state is unavailable"), Localization.New("server.app.resource_target_state_is.345c80e8", "resource target state is unavailable", nil))
	}
	if err := validateResourceTargetCapacity(
		application.State.ReadOnlyView(), guard.TargetCastleID, guard.Goods, guard.DeliveryRatio, time.Now().UTC(),
	); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func validateResourceTargetCapacity(
	gameState State.GameState,
	targetCastleID State.CastleID,
	goods []kingdomResourceShipmentGood,
	deliveryRatio float64,
	now time.Time,
) error {
	target, exists := gameState.Castles[targetCastleID]
	if !exists || targetCastleID <= 0 {
		return Localization.WithError(fmt.Errorf("resource target castle %d is unavailable", targetCastleID), Localization.New("server.app.resource_target_castle_p.b98e4468", "resource target castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", targetCastleID)}))
	}
	if deliveryRatio <= 0 || deliveryRatio > 1 {
		return Localization.WithError(fmt.Errorf("resource delivery ratio %.4f is invalid", deliveryRatio), Localization.New("server.app.resource_delivery_ratio_p.022727b7", "resource delivery ratio {p0} is invalid", Localization.Params{"p0": deliveryRatio}))
	}
	for _, good := range goods {
		balance, observed := target.Resources[good.ResourceID]
		if !observed || balance.Capacity == nil {
			return Localization.WithError(fmt.Errorf("resource %d storage capacity is unavailable at target castle %d", good.ResourceID, target.ID), Localization.New("server.app.resource_p_storage_capacity.621c05d0", "resource {p0} storage capacity is unavailable at target castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", good.ResourceID), "p1": fmt.Sprintf("%d", target.ID)}))
		}
		incoming := incomingMarketResourceAt(gameState, target, good.ResourceID, now)
		free := math.Max(0, *balance.Capacity-balance.Amount-incoming)
		delivery := float64(good.Amount) * deliveryRatio
		if delivery > free+0.000001 {
			return Localization.WithError(fmt.Errorf(
				"resource %d delivery %.0f exceeds %.0f free storage at target castle %d",
				good.ResourceID, delivery, free, target.ID,
			), Localization.New("server.app.resource_p_delivery_p.12bac133", "resource {p0} delivery {p1} exceeds {p2} free storage at target castle {p3}", Localization.Params{"p0": fmt.Sprintf("%d", good.ResourceID), "p1": delivery, "p2": free, "p3": fmt.Sprintf("%d", target.ID)}))
		}
	}
	return nil
}

func incomingMarketResourceAt(
	gameState State.GameState,
	target State.CastleState,
	resourceID State.ResourceID,
	now time.Time,
) float64 {
	amount := float64(0)
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID ||
			movement.TargetX != target.X || movement.TargetY != target.Y {
			return true
		}
		if movement.ArrivesAt != nil && !movement.ArrivesAt.After(now) {
			return true
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID {
				amount += good.Amount
			}
		}
		return true
	})
	return amount
}

func (application *Application) consumeKingdomResourceSource(ctx context.Context, arguments json.RawMessage) error {
	var request kingdomResourceShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	goods, err := normalizeKingdomResourceGoods(request)
	if err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("kingdom resource state is unavailable"), Localization.New("server.app.kingdom_resource_state_is.ce567752", "kingdom resource state is unavailable", nil))
	}
	metadata := Outbound.MetadataFromContext(ctx)
	workflowOwner := strings.TrimSpace(request.WorkflowOwner)
	_, err = application.State.ApplyComponents(State.Components(
		State.ComponentCastles, State.ComponentKingdomTransport,
	), func(gameState *State.GameState) ([]string, bool, error) {
		source, found := gameState.MutableCastleParts(request.SourceCastleID, State.CastlePartResources)
		if !found {
			return nil, false, Localization.WithError(fmt.Errorf("confirmed kingdom resource donor %d is unavailable", request.SourceCastleID), Localization.New("server.app.confirmed_kingdom_resource_donor.766f86d8", "confirmed kingdom resource donor {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
		}
		for _, good := range goods {
			if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
				return nil, false, Localization.WithError(fmt.Errorf(
					"confirmed kingdom resource donor %d has insufficient resource %d in state",
					source.ID, good.ResourceID,
				), Localization.New("server.app.confirmed_kingdom_resource_donor.f9dee493", "confirmed kingdom resource donor {p0} has insufficient resource {p1} in state", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": fmt.Sprintf("%d", good.ResourceID)}))
			}
		}
		for _, good := range goods {
			balance := source.Resources[good.ResourceID]
			balance.Amount -= float64(good.Amount)
			source.Resources[good.ResourceID] = balance
		}
		gameState.SetCastleParts(source.ID, source, State.CastlePartResources)
		if workflowOwner != "" && metadata.Actor == "automation:"+workflowOwner && request.TargetKingdomID >= 0 && request.TargetCastleID > 0 {
			if gameState.KingdomTransport.ResourceWorkflows == nil {
				gameState.KingdomTransport.ResourceWorkflows = map[State.KingdomID]State.KingdomResourceTransportWorkflow{}
			}
			launchedAt := metadata.SubmittedAt
			if launchedAt.IsZero() {
				launchedAt = time.Now().UTC()
			}
			workflowGoods := make([]State.KingdomTransportGood, 0, len(goods))
			for _, good := range goods {
				workflowGoods = append(workflowGoods, State.KingdomTransportGood{ResourceID: good.ResourceID, Amount: float64(good.Amount)})
			}
			gameState.KingdomTransport.ResourceWorkflows[request.TargetKingdomID] = State.KingdomResourceTransportWorkflow{
				Owner: workflowOwner, KingdomID: request.TargetKingdomID,
				SourceCastleID: source.ID, TargetCastleID: request.TargetCastleID,
				Goods: workflowGoods, LaunchedAt: launchedAt.UTC(),
			}
		}
		return []string{"castles", "resources", "kingdom-transport"}, true, nil
	})
	return err
}

func planKingdomResourceSettlement(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomResourceSettlementRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.Owner = strings.TrimSpace(request.Owner)
	if request.Owner == "" || request.TargetKingdomID < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("owner and targetKingdomId are required"), Localization.New("server.app.owner_and_targetkingdomid_are.ffeb0660", "owner and targetKingdomId are required", nil))
	}
	workflow, exists := input.State.KingdomTransport.ResourceWorkflows[request.TargetKingdomID]
	if !exists || workflow.Owner != request.Owner {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom %d has no %s resource workflow", request.TargetKingdomID, request.Owner), Localization.New("server.app.kingdom_p_has_no.3c1efb00", "kingdom {p0} has no {p1} resource workflow", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID), "p1": fmt.Sprintf("%s", request.Owner)}))
	}
	if kingdomResourceTransportPending(input.State, request.TargetKingdomID) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom %d resource transport is still pending", request.TargetKingdomID), Localization.New("server.app.kingdom_p_resource_transport.8c0b3a3a", "kingdom {p0} resource transport is still pending", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
	}
	target, exists := input.State.Castles[workflow.TargetCastleID]
	if !exists || target.ID <= 0 || target.KingdomID != request.TargetKingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("resource workflow target castle %d is unavailable", workflow.TargetCastleID), Localization.New("server.app.resource_workflow_target_castle.1e067c57", "resource workflow target castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", workflow.TargetCastleID)}))
	}
	refreshPayload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"AID"`
		KingdomID State.KingdomID `json:"KID"`
	}{target.ID, target.KingdomID})
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(target.ID), 10),
			"kingdom:" + strconv.FormatInt(int64(target.KingdomID), 10),
		},
		Summary: fmt.Sprintf("Refresh %s after its completed kingdom resource transport", castleLabel(target)), SummaryDescriptor: Localization.New("server.app.refresh_p_after_its.6b9ad103", "Refresh {p0} after its completed kingdom resource transport", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(target))}),
		Steps: []Intent.Step{
			contextCommandStep("Refresh kingdom resource destination", "grc", refreshPayload, "grc").WithNameDescriptor(Localization.New("server.app.refresh_kingdom_resource_destination.2312062d", "Refresh kingdom resource destination", nil)),
			{Name: "Complete owned kingdom resource workflow", NameDescriptor: Localization.New("server.app.complete_owned_kingdom_resource.284592d4", "Complete owned kingdom resource workflow", nil), Action: "resources.kingdom.complete_workflow", ActionArguments: arguments},
		},
	}, nil
}

func (application *Application) completeKingdomResourceWorkflow(ctx context.Context, arguments json.RawMessage) error {
	var request kingdomResourceSettlementRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	request.Owner = strings.TrimSpace(request.Owner)
	if Outbound.MetadataFromContext(ctx).Actor != "automation:"+request.Owner {
		return Localization.WithError(fmt.Errorf("only automation %s can complete its kingdom resource workflow", request.Owner), Localization.New("server.app.only_automation_p_can.b8b106ef", "only automation {p0} can complete its kingdom resource workflow", Localization.Params{"p0": fmt.Sprintf("%s", request.Owner)}))
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.ResourceWorkflows[request.TargetKingdomID]
		if !exists || workflow.Owner != request.Owner {
			return nil, false, Localization.WithError(fmt.Errorf("%w: kingdom %d resource workflow ownership changed", Intent.ErrPlanStale, request.TargetKingdomID), Localization.New("server.app.intent_plan_became_stale.f8bfa138", "intent plan became stale before dispatch: kingdom {p1} resource workflow ownership changed", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
		if kingdomResourceTransportPending(*gameState, request.TargetKingdomID) {
			return nil, false, Localization.WithError(fmt.Errorf("%w: kingdom %d resource transport is still pending", Intent.ErrPlanStale, request.TargetKingdomID), Localization.New("server.app.intent_plan_became_stale.93daa9e3", "intent plan became stale before dispatch: kingdom {p1} resource transport is still pending", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
		delete(gameState.KingdomTransport.ResourceWorkflows, request.TargetKingdomID)
		return []string{"kingdom-transport", "resources"}, true, nil
	})
	return err
}

func normalizeKingdomResourceGoods(request kingdomResourceShipmentRequest) ([]kingdomResourceShipmentGood, error) {
	if len(request.Goods) > 0 && (request.ResourceID != 0 || request.Amount != 0) {
		return nil, Localization.WithError(fmt.Errorf("use either goods or the legacy resourceId and amount fields, not both"), Localization.New("server.app.use_either_goods_or.9ad3f6d4", "use either goods or the legacy resourceId and amount fields, not both", nil))
	}
	goods := append([]kingdomResourceShipmentGood(nil), request.Goods...)
	if len(goods) == 0 {
		goods = []kingdomResourceShipmentGood{{ResourceID: request.ResourceID, Amount: request.Amount}}
	}
	if len(goods) > 20 {
		return nil, Localization.WithError(fmt.Errorf("goods may contain at most 20 resources"), Localization.New("server.app.goods_may_contain_at.fa6a3d97", "goods may contain at most 20 resources", nil))
	}
	merged := map[State.ResourceID]int64{}
	for _, good := range goods {
		if good.ResourceID <= 0 || good.Amount <= 0 {
			return nil, Localization.WithError(fmt.Errorf("every shipment good requires a positive resourceId and amount"), Localization.New("server.app.every_shipment_good_requires.8c99acfa", "every shipment good requires a positive resourceId and amount", nil))
		}
		if merged[good.ResourceID] > int64(^uint64(0)>>1)-good.Amount {
			return nil, Localization.WithError(fmt.Errorf("shipment amount for resource %d is too large", good.ResourceID), Localization.New("server.app.shipment_amount_for_resource.e089c988", "shipment amount for resource {p0} is too large", Localization.Params{"p0": fmt.Sprintf("%d", good.ResourceID)}))
		}
		merged[good.ResourceID] += good.Amount
	}
	result := make([]kingdomResourceShipmentGood, 0, len(merged))
	for resourceID, amount := range merged {
		result = append(result, kingdomResourceShipmentGood{ResourceID: resourceID, Amount: amount})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ResourceID < result[right].ResourceID })
	return result, nil
}

func planKingdomResourceSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomResourceSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	step, currencyID, timeSkipLabel, err := kingdomResourceSkipStep(input, request, true)
	if err != nil {
		return Intent.Plan{}, err
	}
	guardArguments, _ := json.Marshal(kingdomResourcePendingGuard{TargetKingdomID: request.TargetKingdomID})
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
			"currency:" + strconv.FormatInt(int64(currencyID), 10),
		},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d resource transport", timeSkipLabel, request.TargetKingdomID), SummaryDescriptor: Localization.New("server.app.apply_a_p_to.eb83aa5e", "Apply a {p0} to kingdom {p1} resource transport", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipLabel), "p1": fmt.Sprintf("%d", request.TargetKingdomID)}),
		Steps: []Intent.Step{
			// An earlier MSK may have reached the game after its caller was
			// cancelled. Re-read and reduce the authoritative transport list
			// before allowing another skip onto the wire.
			kingdomTransportContextStep(),
			Intent.RebuildOnResume(Intent.Step{
				Name: "Verify refreshed kingdom resource transport", NameDescriptor: Localization.New("server.app.verify_refreshed_kingdom_resource.a39de696", "Verify refreshed kingdom resource transport", nil), Action: "resources.kingdom.verify_pending",
				ActionArguments: guardArguments,
			}),
			step,
			timeSkipConsumeStep(input, currencyID),
		},
	}, nil
}

func kingdomResourceSkipStep(
	input Intent.PlanningContext,
	request kingdomResourceSkipRequest,
	requirePending bool,
) (Intent.Step, State.CurrencyID, string, error) {
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("targetKingdomId and timeSkipId are required"), Localization.New("server.app.targetkingdomid_and_timeskipid_are.d0c2a0d6", "targetKingdomId and timeSkipId are required", nil))
	}
	if requirePending {
		pending := false
		for _, transport := range input.State.KingdomTransport.Pending {
			if transport.KingdomID == request.TargetKingdomID && transport.RemainingSec > 0 {
				pending = true
				break
			}
		}
		if !pending {
			return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf(
				"%w: kingdom %d has no pending resource transport",
				Intent.ErrPlanStale, request.TargetKingdomID,
			), Localization.New("server.app.intent_plan_became_stale.39a1a735", "intent plan became stale before dispatch: kingdom {p1} has no pending resource transport", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Step{}, 0, "", err
	}
	timeSkipLabel := officialTimeSkipLabel(input.GameData, int64(currencyID), request.TimeSkipID)
	if request.MinimumRemaining < 0 {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("minimumRemaining cannot be negative"), Localization.New("server.app.minimumremaining_cannot_be_negative.1793e608", "minimumRemaining cannot be negative", nil))
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("no %s is available", timeSkipLabel), Localization.New("server.app.no_p_is_available.d9c730b3", "no {p0} is available", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipLabel)}))
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "2",
	})
	step := commandStep("Skip kingdom resource transport time", "msk", payload, "msk", Localization.New("server.app.skip_kingdom_resource_transport.2e90fce7", "Skip kingdom resource transport time", nil))
	// Captures show 182 when the referenced transport disappeared before MSK
	// was handled. It is a stale transport plan, not an inventory failure.
	step.StaleCodes = []int{182}
	return step, currencyID, timeSkipLabel, nil
}

func officialResourceJSONKey(store *GameData.Store, id State.ResourceID) (string, error) {
	if store == nil || id <= 0 {
		return "", Localization.WithError(fmt.Errorf("resourceId must reference the loaded official catalog"), Localization.New("server.app.resourceid_must_reference_the.1909178a", "resourceId must reference the loaded official catalog", nil))
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return "", err
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(id), 10))
	if !exists {
		return "", Localization.WithError(fmt.Errorf("resource %d is not in the current official catalog", id), Localization.New("server.app.resource_p_is_not.0b385b1d", "resource {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return "", err
	}
	jsonKey, _ := record.String("JSONKey")
	jsonKey = strings.TrimSpace(jsonKey)
	if jsonKey == "" {
		return "", Localization.WithError(fmt.Errorf("resource %d has no official wire key", id), Localization.New("server.app.resource_p_has_no.36d43b9d", "resource {p0} has no official wire key", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
	}
	return jsonKey, nil
}

func officialResourceIDByJSONKey(store *GameData.Store, jsonKey string) (State.ResourceID, error) {
	if store == nil || strings.TrimSpace(jsonKey) == "" {
		return 0, Localization.WithError(fmt.Errorf("resource key must reference the loaded official catalog"), Localization.New("server.app.resource_key_must_reference.e9ee1420", "resource key must reference the loaded official catalog", nil))
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return 0, err
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, _ := record.String("JSONKey")
		if !strings.EqualFold(strings.TrimSpace(candidate), strings.TrimSpace(jsonKey)) {
			continue
		}
		id, found := record.Int64("resourceID")
		if found && id > 0 {
			return State.ResourceID(id), nil
		}
	}
	return 0, Localization.WithError(fmt.Errorf("official resource %s is unavailable", strings.TrimSpace(jsonKey)), Localization.New("server.app.official_resource_p_is.71532eda", "official resource {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%s", strings.TrimSpace(jsonKey))}))
}

func officialResourceDisplayName(store *GameData.Store, id State.ResourceID, fallback string) string {
	if store == nil || id <= 0 {
		return fallback
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return fallback
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(id), 10))
	if !exists {
		return fallback
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return fallback
	}
	for _, field := range []string{"_display_name", "name", "JSONKey"} {
		if value, found := record.String(field); found {
			if name := userFacingGameName(value); name != "" {
				return name
			}
		}
	}
	return fallback
}

func officialCurrencyID(store *GameData.Store, jsonKey string) (State.CurrencyID, error) {
	if store == nil {
		return 0, Localization.WithError(fmt.Errorf("official currency catalog is unavailable"), Localization.New("server.app.official_currency_catalog_is.ea85019a", "official currency catalog is unavailable", nil))
	}
	catalog, err := store.Catalog("currencies")
	if err != nil {
		return 0, err
	}
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidate, _ := record.String("JSONKey")
		if !strings.EqualFold(strings.TrimSpace(candidate), jsonKey) {
			continue
		}
		id, _ := record.Int64("currencyID")
		if id > 0 {
			return State.CurrencyID(id), nil
		}
	}
	return 0, Localization.WithError(fmt.Errorf("currency %s is not in the current official catalog", jsonKey), Localization.New("server.app.currency_p_is_not.f421b875", "currency {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%s", jsonKey)}))
}

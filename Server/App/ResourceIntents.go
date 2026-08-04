package App

import (
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
	kingdomResourceDeliveryRatio           = 0.90
	kingdomResourceTransportInitialSeconds = 3_600
	autoFoodBalanceResourceWorkflowOwner   = "autoFoodBalance"
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
		commandStep("Refresh kingdom transports", "kpi", json.RawMessage(`{}`), "kpi"),
	}
	summary := "Refresh kingdom-resource logistics state"

	marketCastle, marketRequired, err := resourceLogisticsMarketCastle(input)
	if err != nil {
		return Intent.Plan{}, err
	}
	if marketRequired {
		claims = append(claims, "castle-focus")
		originalCastle, hadOriginalFocus := resourceLogisticsFocusedCastle(input.State)
		if !marketCastle.Focused {
			steps = append(steps, castleFocusStep(marketCastle))
		}
		steps = append(steps,
			commandStep("Refresh caravan boosters", "boi", json.RawMessage(`{}`), "boi"),
			commandStep("Refresh market capacity", "cmi", json.RawMessage(`{"S":1,"KID":-1}`), "cmi"),
		)
		if hadOriginalFocus && originalCastle.ID != marketCastle.ID {
			steps = append(steps, castleFocusStep(originalCastle))
		}
		summary = "Refresh market and kingdom-resource logistics state"
	}

	return Intent.Plan{Claims: claims, Summary: summary, Steps: steps}, nil
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
		return State.CastleState{}, false, fmt.Errorf("official game data is unavailable")
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
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if !targetExists || request.TargetCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("target castle %d is not in the current player state", request.TargetCastleID)
	}
	if source.ID == target.ID {
		return Intent.Plan{}, fmt.Errorf("resource shipments require distinct source and target castles")
	}

	if source.KingdomID == target.KingdomID {
		goods, err := normalizeKingdomResourceGoods(request)
		if err != nil {
			return Intent.Plan{}, err
		}
		if len(goods) != 1 {
			return Intent.Plan{}, fmt.Errorf("same-kingdom market shipments require exactly one resource")
		}
		marketArguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": source.ID, "targetCastleId": target.ID,
			"resourceId": goods[0].ResourceID, "amount": goods[0].Amount,
			"workflowOwner": request.WorkflowOwner, "enforceTargetCapacity": request.EnforceTargetCap,
			"horseTravelBoostId": request.HorseTravelBoostID,
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
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || request.SourceCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if !targetExists || request.TargetCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("target castle %d is not in the current player state", request.TargetCastleID)
	}
	if source.ID == target.ID || source.KingdomID != target.KingdomID {
		return Intent.Plan{}, fmt.Errorf("market shipments require distinct castles in the same kingdom")
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
		return Intent.Plan{}, fmt.Errorf("source castle %d has no Marketplace building", source.ID)
	}
	resourceKey, err := officialResourceJSONKey(input.GameData, request.ResourceID)
	if err != nil {
		return Intent.Plan{}, err
	}
	resourceName := officialResourceDisplayName(input.GameData, request.ResourceID, resourceKey)
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("amount must be positive")
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, err
	}
	horseTravelBoostID, err := resolveCastleHorseTravelBoostID(
		input.GameData, source, request.HorseTravelBoostID,
	)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("resolve market horse travel boost: %w", err)
	}
	if source.Resources[request.ResourceID].Amount < float64(request.Amount) {
		return Intent.Plan{}, fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceName)
	}
	goods := []kingdomResourceShipmentGood{{ResourceID: request.ResourceID, Amount: request.Amount}}
	if request.EnforceTargetCap {
		if err := validateResourceTargetCapacity(input.State, target.ID, goods, 1, time.Now().UTC()); err != nil {
			return Intent.Plan{}, err
		}
	}
	market, observed := input.State.Market.Castles[source.ID]
	if !observed || input.State.Market.ObservedAt.IsZero() ||
		State.AvailableMarketBarrowsAt(input.State, market, time.Now().UTC()) <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d has no observed available market barrows", source.ID)
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
	steps := []Intent.Step{commandStep("Start market shipment", "crm", payload, "crm")}
	if request.EnforceTargetCap {
		guardArguments, _ := json.Marshal(resourceTargetCapacityGuard{
			TargetCastleID: target.ID, Goods: goods, DeliveryRatio: 1,
		})
		steps = append([]Intent.Step{Intent.RebuildOnResume(Intent.Step{
			Name: "Verify destination resource storage", Action: "resources.verify_target_capacity",
			ActionArguments: guardArguments,
		})}, steps...)
	}
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"castle:" + strconv.FormatInt(int64(target.ID), 10),
		},
		Summary: fmt.Sprintf("Ship %d %s from %s to %s", request.Amount, resourceName, castleLabel(source), castleLabel(target)),
		Steps:   steps,
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
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if request.TargetKingdomID < 0 || request.TargetKingdomID == source.KingdomID {
		return Intent.Plan{}, fmt.Errorf("targetKingdomId must identify a different owned kingdom")
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
		return Intent.Plan{}, fmt.Errorf("kingdom %d has no castle in the current player state", request.TargetKingdomID)
	}
	var target State.CastleState
	if request.TargetCastleID > 0 {
		var targetExists bool
		target, targetExists = input.State.Castles[request.TargetCastleID]
		if !targetExists || target.KingdomID != request.TargetKingdomID {
			return Intent.Plan{}, fmt.Errorf("target castle %d is not in kingdom %d", request.TargetCastleID, request.TargetKingdomID)
		}
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[request.TargetKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked {
		return Intent.Plan{}, fmt.Errorf("kingdom transport to %d is not observed as unlocked", request.TargetKingdomID)
	}
	if kingdomResourceTransportBusy(input.State, request.TargetKingdomID) {
		if strings.TrimSpace(request.WorkflowOwner) != "" {
			return Intent.Plan{}, fmt.Errorf(
				"%w: kingdom %d already has a pending or settling resource transport",
				Intent.ErrPlanStale, request.TargetKingdomID,
			)
		}
		return Intent.Plan{}, fmt.Errorf("kingdom %d already has a pending or settling resource transport", request.TargetKingdomID)
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
			return Intent.Plan{}, fmt.Errorf("source castle %d has insufficient %s", source.ID, resourceName)
		}
		wireGoods = append(wireGoods, []any{resourceKey, good.Amount})
		summaryGoods = append(summaryGoods, fmt.Sprintf("%d %s", good.Amount, resourceName))
	}
	if request.EnforceTargetCap {
		if target.ID <= 0 {
			return Intent.Plan{}, fmt.Errorf("targetCastleId is required when destination storage enforcement is enabled")
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
	if request.TargetCastleID > 0 {
		summary = fmt.Sprintf("Ship %s from %s to %s by kingdom transport", strings.Join(summaryGoods, " and "), castleLabel(source), castleLabel(target))
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
		contextCommandStep("Refresh kingdom resource donor", "grc", resourceRefreshPayload, "grc"),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Verify kingdom resource transport and donor balance", Action: "kingdom.transport.verify_available",
			ActionArguments: guardArguments,
		}),
		commandStep("Start kingdom resource shipment", "kgt", payload, "kgt"),
		{Name: "Consume confirmed donor resources", Action: "resources.kingdom.consume_source", ActionArguments: consumeArguments},
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
		if request.SettleAfterSkip &&
			officialTimeSkipMinutes(input.GameData, int64(currencyID), request.TimeSkipID)*60 <
				kingdomResourceTransportInitialSeconds {
			return Intent.Plan{}, fmt.Errorf("settleAfterTimeSkip requires a time skip covering the full kingdom transport")
		}
	}
	if request.SettleAfterSkip {
		workflowOwner := strings.TrimSpace(request.WorkflowOwner)
		if strings.TrimSpace(request.TimeSkipID) == "" {
			return Intent.Plan{}, fmt.Errorf("settleAfterTimeSkip requires timeSkipId")
		}
		if workflowOwner == "" || target.ID <= 0 {
			return Intent.Plan{}, fmt.Errorf("settleAfterTimeSkip requires workflowOwner and targetCastleId")
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
			contextCommandStep("Refresh skipped kingdom resource destination", "grc", targetRefreshPayload, "grc"),
			Intent.Step{
				Name:   "Complete immediately skipped kingdom resource workflow",
				Action: "resources.kingdom.complete_workflow", ActionArguments: settlementArguments,
			},
		)
		if !request.EnforceTargetCap {
			claims = append(claims, "castle:"+strconv.FormatInt(int64(target.ID), 10))
		}
	}
	return Intent.Plan{Claims: claims, Summary: summary, Steps: steps}, nil
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
		return fmt.Errorf("Auto Food cannot send resources to or from Berimond castles")
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
		return fmt.Errorf("kingdom transport state is unavailable")
	}
	gameState := application.State.Snapshot()
	switch guard.TransportKind {
	case "resource":
		if kingdomResourceTransportBusy(gameState, guard.TargetKingdomID) {
			return fmt.Errorf(
				"%w: kingdom %d has a pending or settling resource transport",
				Intent.ErrPlanStale, guard.TargetKingdomID,
			)
		}
		if guard.SourceCastleID > 0 || len(guard.Goods) > 0 {
			source, found := gameState.Castles[guard.SourceCastleID]
			if !found || guard.SourceCastleID <= 0 {
				return fmt.Errorf("%w: kingdom resource donor %d is unavailable", Intent.ErrPlanStale, guard.SourceCastleID)
			}
			goods, err := normalizeKingdomResourceGoods(kingdomResourceShipmentRequest{Goods: guard.Goods})
			if err != nil {
				return err
			}
			for _, good := range goods {
				if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
					return fmt.Errorf(
						"%w: kingdom resource donor %d has insufficient resource %d after refresh",
						Intent.ErrPlanStale, source.ID, good.ResourceID,
					)
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
			return fmt.Errorf(
				"%w: kingdom %d has a pending or settling troop transport",
				Intent.ErrPlanStale, guard.TargetKingdomID,
			)
		}
	default:
		return fmt.Errorf("unsupported kingdom transport kind %q", guard.TransportKind)
	}
	return nil
}

func (application *Application) verifyResourceTargetCapacity(_ context.Context, arguments json.RawMessage) error {
	var guard resourceTargetCapacityGuard
	if err := decodeIntentArguments(arguments, &guard); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("resource target state is unavailable")
	}
	if err := validateResourceTargetCapacity(
		application.State.Snapshot(), guard.TargetCastleID, guard.Goods, guard.DeliveryRatio, time.Now().UTC(),
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
		return fmt.Errorf("resource target castle %d is unavailable", targetCastleID)
	}
	if deliveryRatio <= 0 || deliveryRatio > 1 {
		return fmt.Errorf("resource delivery ratio %.4f is invalid", deliveryRatio)
	}
	for _, good := range goods {
		balance, observed := target.Resources[good.ResourceID]
		if !observed || balance.Capacity == nil {
			return fmt.Errorf("resource %d storage capacity is unavailable at target castle %d", good.ResourceID, target.ID)
		}
		incoming := incomingMarketResourceAt(gameState, target, good.ResourceID, now)
		free := math.Max(0, *balance.Capacity-balance.Amount-incoming)
		delivery := float64(good.Amount) * deliveryRatio
		if delivery > free+0.000001 {
			return fmt.Errorf(
				"resource %d delivery %.0f exceeds %.0f free storage at target castle %d",
				good.ResourceID, delivery, free, target.ID,
			)
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
	for _, movement := range gameState.Movements {
		if movement.Direction != 0 || movement.KingdomID != target.KingdomID ||
			movement.TargetX != target.X || movement.TargetY != target.Y {
			continue
		}
		if movement.ArrivesAt != nil && !movement.ArrivesAt.After(now) {
			continue
		}
		for _, good := range movement.MarketGoods {
			if good.ResourceID == resourceID {
				amount += good.Amount
			}
		}
	}
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
		return fmt.Errorf("kingdom resource state is unavailable")
	}
	metadata := Outbound.MetadataFromContext(ctx)
	workflowOwner := strings.TrimSpace(request.WorkflowOwner)
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, found := gameState.Castles[request.SourceCastleID]
		if !found {
			return nil, false, fmt.Errorf("confirmed kingdom resource donor %d is unavailable", request.SourceCastleID)
		}
		for _, good := range goods {
			if source.Resources[good.ResourceID].Amount < float64(good.Amount) {
				return nil, false, fmt.Errorf(
					"confirmed kingdom resource donor %d has insufficient resource %d in state",
					source.ID, good.ResourceID,
				)
			}
		}
		for _, good := range goods {
			balance := source.Resources[good.ResourceID]
			balance.Amount -= float64(good.Amount)
			source.Resources[good.ResourceID] = balance
		}
		gameState.Castles[source.ID] = source
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
		return Intent.Plan{}, fmt.Errorf("owner and targetKingdomId are required")
	}
	workflow, exists := input.State.KingdomTransport.ResourceWorkflows[request.TargetKingdomID]
	if !exists || workflow.Owner != request.Owner {
		return Intent.Plan{}, fmt.Errorf("kingdom %d has no %s resource workflow", request.TargetKingdomID, request.Owner)
	}
	if kingdomResourceTransportPending(input.State, request.TargetKingdomID) {
		return Intent.Plan{}, fmt.Errorf("kingdom %d resource transport is still pending", request.TargetKingdomID)
	}
	target, exists := input.State.Castles[workflow.TargetCastleID]
	if !exists || target.ID <= 0 || target.KingdomID != request.TargetKingdomID {
		return Intent.Plan{}, fmt.Errorf("resource workflow target castle %d is unavailable", workflow.TargetCastleID)
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
		Summary: fmt.Sprintf("Refresh %s after its completed kingdom resource transport", castleLabel(target)),
		Steps: []Intent.Step{
			contextCommandStep("Refresh kingdom resource destination", "grc", refreshPayload, "grc"),
			{Name: "Complete owned kingdom resource workflow", Action: "resources.kingdom.complete_workflow", ActionArguments: arguments},
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
		return fmt.Errorf("only automation %s can complete its kingdom resource workflow", request.Owner)
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.ResourceWorkflows[request.TargetKingdomID]
		if !exists || workflow.Owner != request.Owner {
			return nil, false, fmt.Errorf("%w: kingdom %d resource workflow ownership changed", Intent.ErrPlanStale, request.TargetKingdomID)
		}
		if kingdomResourceTransportPending(*gameState, request.TargetKingdomID) {
			return nil, false, fmt.Errorf("%w: kingdom %d resource transport is still pending", Intent.ErrPlanStale, request.TargetKingdomID)
		}
		delete(gameState.KingdomTransport.ResourceWorkflows, request.TargetKingdomID)
		return []string{"kingdom-transport", "resources"}, true, nil
	})
	return err
}

func normalizeKingdomResourceGoods(request kingdomResourceShipmentRequest) ([]kingdomResourceShipmentGood, error) {
	if len(request.Goods) > 0 && (request.ResourceID != 0 || request.Amount != 0) {
		return nil, fmt.Errorf("use either goods or the legacy resourceId and amount fields, not both")
	}
	goods := append([]kingdomResourceShipmentGood(nil), request.Goods...)
	if len(goods) == 0 {
		goods = []kingdomResourceShipmentGood{{ResourceID: request.ResourceID, Amount: request.Amount}}
	}
	if len(goods) > 20 {
		return nil, fmt.Errorf("goods may contain at most 20 resources")
	}
	merged := map[State.ResourceID]int64{}
	for _, good := range goods {
		if good.ResourceID <= 0 || good.Amount <= 0 {
			return nil, fmt.Errorf("every shipment good requires a positive resourceId and amount")
		}
		if merged[good.ResourceID] > int64(^uint64(0)>>1)-good.Amount {
			return nil, fmt.Errorf("shipment amount for resource %d is too large", good.ResourceID)
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
	return Intent.Plan{
		Claims: []string{
			"resource-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
			"currency:" + strconv.FormatInt(int64(currencyID), 10),
		},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d resource transport", timeSkipLabel, request.TargetKingdomID),
		Steps:   []Intent.Step{step, timeSkipConsumeStep(input, currencyID)},
	}, nil
}

func kingdomResourceSkipStep(
	input Intent.PlanningContext,
	request kingdomResourceSkipRequest,
	requirePending bool,
) (Intent.Step, State.CurrencyID, string, error) {
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Step{}, 0, "", fmt.Errorf("targetKingdomId and timeSkipId are required")
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
			return Intent.Step{}, 0, "", fmt.Errorf("kingdom %d has no pending resource transport", request.TargetKingdomID)
		}
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Step{}, 0, "", err
	}
	timeSkipLabel := officialTimeSkipLabel(input.GameData, int64(currencyID), request.TimeSkipID)
	if request.MinimumRemaining < 0 {
		return Intent.Step{}, 0, "", fmt.Errorf("minimumRemaining cannot be negative")
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
		return Intent.Step{}, 0, "", fmt.Errorf("no %s is available", timeSkipLabel)
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "2",
	})
	return commandStep("Skip kingdom resource transport time", "msk", payload, "msk"), currencyID, timeSkipLabel, nil
}

func officialResourceJSONKey(store *GameData.Store, id State.ResourceID) (string, error) {
	if store == nil || id <= 0 {
		return "", fmt.Errorf("resourceId must reference the loaded official catalog")
	}
	catalog, err := store.Catalog("resources")
	if err != nil {
		return "", err
	}
	raw, exists := catalog.Find(strconv.FormatInt(int64(id), 10))
	if !exists {
		return "", fmt.Errorf("resource %d is not in the current official catalog", id)
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return "", err
	}
	jsonKey, _ := record.String("JSONKey")
	jsonKey = strings.TrimSpace(jsonKey)
	if jsonKey == "" {
		return "", fmt.Errorf("resource %d has no official wire key", id)
	}
	return jsonKey, nil
}

func officialResourceIDByJSONKey(store *GameData.Store, jsonKey string) (State.ResourceID, error) {
	if store == nil || strings.TrimSpace(jsonKey) == "" {
		return 0, fmt.Errorf("resource key must reference the loaded official catalog")
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
	return 0, fmt.Errorf("official resource %s is unavailable", strings.TrimSpace(jsonKey))
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
		return 0, fmt.Errorf("official currency catalog is unavailable")
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
	return 0, fmt.Errorf("currency %s is not in the current official catalog", jsonKey)
}

package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	defaultProductionSessionKey = 73
	productionBaseQueueCapacity = 2
)

type productionQueueCapacityGuard struct {
	CastleID               State.CastleID `json:"castleId"`
	LineID                 int            `json:"lineId"`
	DefinitionID           int64          `json:"definitionId"`
	ExpectedFreeSlots      int            `json:"expectedFreeSlots"`
	FillAvailable          bool           `json:"fillAvailable"`
	ScheduledDefinitionID  int64          `json:"scheduledDefinitionId,omitempty"`
	ScheduleValidUntil     *time.Time     `json:"scheduleValidUntil,omitempty"`
	TitleGatedDefinitionID int64          `json:"titleGatedDefinitionId,omitempty"`
	RequiredGloryTitleID   int64          `json:"requiredGloryTitleId,omitempty"`
	TitleLossFallback      bool           `json:"titleLossFallback,omitempty"`
	QueueObservedAt        time.Time      `json:"queueObservedAt"`
	RequireNewerQueue      bool           `json:"requireNewerQueue,omitempty"`
}

func planProductionEnqueue(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID               State.CastleID `json:"castleId"`
		LineID                 int            `json:"lineId"`
		DefinitionID           int64          `json:"definitionId"`
		Amount                 int64          `json:"amount,omitempty"`
		FillAvailable          bool           `json:"fillAvailable,omitempty"`
		ScheduledDefinitionID  int64          `json:"scheduledDefinitionId,omitempty"`
		ScheduleValidUntil     *time.Time     `json:"scheduleValidUntil,omitempty"`
		TitleGatedDefinitionID int64          `json:"titleGatedDefinitionId,omitempty"`
		RequiredGloryTitleID   int64          `json:"requiredGloryTitleId,omitempty"`
		TitleLossFallback      bool           `json:"titleLossFallback,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if State.CastleFocusKnownUnavailable(input.State, castle) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: castle %d cannot be focused in the current kingdom session", Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.f50ee7dc", "intent plan became stale before dispatch: castle {p1} cannot be focused in the current kingdom session", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	if request.LineID != 0 && request.LineID != 1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production line %d is not a recruitment or tool line", request.LineID), Localization.New("server.app.production_line_p_is.bb47fbe3", "production line {p0} is not a recruitment or tool line", Localization.Params{"p0": fmt.Sprintf("%d", request.LineID)}))
	}
	if request.ScheduledDefinitionID != 0 || request.ScheduleValidUntil != nil {
		if request.ScheduledDefinitionID <= 0 || request.ScheduledDefinitionID != request.DefinitionID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("scheduled production definition must match definitionId %d", request.DefinitionID), Localization.New("server.app.scheduled_production_definition_must.275029c3", "scheduled production definition must match definitionId {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.DefinitionID)}))
		}
		if request.ScheduleValidUntil == nil || request.ScheduleValidUntil.IsZero() {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("scheduled production requires scheduleValidUntil"), Localization.New("server.app.scheduled_production_requires_schedulevaliduntil.275faea8", "scheduled production requires scheduleValidUntil", nil))
		}
		if request.FillAvailable {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("scheduled production must enqueue one stack before reevaluating the schedule"), Localization.New("server.app.scheduled_production_must_enqueue.c8ceac3b", "scheduled production must enqueue one stack before reevaluating the schedule", nil))
		}
	}
	queue, ok := castle.Production[request.LineID]
	if !ok || State.ProductionQueueNeedsRefresh(input.State, queue, time.Now().UTC()) ||
		State.ProductionQueuePredatesCastleSnapshot(castle, queue) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: production line %d needs a current slot observation for castle %d",
			Intent.ErrPlanStale, request.LineID, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.060bad99", "intent plan became stale before dispatch: production line {p1} needs a current slot observation for castle {p2}", Localization.Params{"p1": fmt.Sprintf("%d", request.LineID), "p2": fmt.Sprintf("%d", request.CastleID)}))
	}
	collection := "units"
	if request.LineID == 1 {
		collection = "tools"
	}
	if err := requireOfficialDefinition(input.GameData, collection, request.DefinitionID); err != nil {
		return Intent.Plan{}, err
	}
	if request.LineID == 0 && request.TitleGatedDefinitionID <= 0 &&
		request.RequiredGloryTitleID <= 0 && !request.TitleLossFallback && input.GameData != nil {
		if unlock, titleGated := input.GameData.GloryTitleUnlockForUnit(request.DefinitionID); titleGated {
			request.TitleGatedDefinitionID = unlock.UnitID
			request.RequiredGloryTitleID = unlock.RequiredTitleID
		}
	}
	if err := validateProductionGloryTitle(
		input.State,
		input.GameData,
		request.DefinitionID,
		request.TitleGatedDefinitionID,
		request.RequiredGloryTitleID,
		request.TitleLossFallback,
	); err != nil {
		return Intent.Plan{}, err
	}
	if !productionDefinitionAvailable(castle, request.LineID, request.DefinitionID) {
		return Intent.Plan{}, fmt.Errorf(
			"%w: %s %d is not currently available for production at %s",
			Intent.ErrPlanStale, strings.TrimSuffix(collection, "s"), request.DefinitionID, castleLabel(castle),
		)
	}
	definitionLabel := productionDefinitionLabel(input.GameData, input.Language, collection, request.DefinitionID)
	// Queue capacity represents the QS slots, not the active production stack.
	occupied := len(queue.Queued)
	queueCapacity := productionQueueCapacity(input.State, request.LineID, queue, input.GameData)
	if queueCapacity <= 0 || occupied >= queueCapacity {
		if request.FillAvailable {
			return Intent.Plan{Summary: fmt.Sprintf("Production line %d is already full at %s", request.LineID, castleLabel(castle)), SummaryDescriptor: Localization.New("server.app.production_line_p_is.54538668", "Production line {p0} is already full at {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.LineID), "p1": fmt.Sprintf("%s", castleLabel(castle))})}, nil
		}
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production line %d is full", request.LineID), Localization.New("server.app.production_line_p_is.e110449d", "production line {p0} is full", Localization.Params{"p0": fmt.Sprintf("%d", request.LineID)}))
	}
	if request.Amount <= 0 {
		request.Amount = observedProductionStack(input.State, queue, request.DefinitionID)
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("production stack size is unknown; create one %s stack in-game so CitadelOps can learn the live amount", collection), Localization.New("server.app.production_stack_size_is.983fb867", "production stack size is unknown; create one {p0} stack in-game so CitadelOps can learn the live amount", Localization.Params{"p0": fmt.Sprintf("%s", collection)}))
	}
	sessionKey := input.State.CommandContext.ProductionSessionKey
	if sessionKey <= 0 {
		sessionKey = defaultProductionSessionKey
	}
	payload, _ := json.Marshal(struct {
		LineID       int             `json:"LID"`
		DefinitionID int64           `json:"WID"`
		Amount       int64           `json:"AMT"`
		PublicOrder  int             `json:"PO"`
		Power        int             `json:"PWR"`
		SessionKey   int             `json:"SK"`
		KingdomID    State.KingdomID `json:"SID"`
		CastleID     State.CastleID  `json:"AID"`
	}{request.LineID, request.DefinitionID, request.Amount, -1, 0, sessionKey, castle.KingdomID, request.CastleID})
	stackCount := 1
	if request.FillAvailable {
		stackCount = queueCapacity - occupied
	}
	steps := castleContextSteps(input, castle)
	recruitment := request.LineID == recruitmentProductionLineID
	requireNewerQueue := len(steps) > 0
	for index := range steps {
		steps[index].StaleCodes = []int{175}
	}
	helpArguments, _ := json.Marshal(recruitmentBUPAllianceHelpRequest{CastleID: request.CastleID})
	for stack := 0; stack < stackCount; stack++ {
		guardArguments, _ := json.Marshal(productionQueueCapacityGuard{
			CastleID: request.CastleID, LineID: request.LineID, DefinitionID: request.DefinitionID,
			ExpectedFreeSlots: stackCount - stack, FillAvailable: request.FillAvailable,
			ScheduledDefinitionID: request.ScheduledDefinitionID, ScheduleValidUntil: request.ScheduleValidUntil,
			TitleGatedDefinitionID: request.TitleGatedDefinitionID,
			RequiredGloryTitleID:   request.RequiredGloryTitleID,
			TitleLossFallback:      request.TitleLossFallback,
			QueueObservedAt:        queue.ObservedAt,
			RequireNewerQueue:      requireNewerQueue,
		})
		steps = append(steps, Intent.RebuildOnResume(Intent.Step{
			Name: "Revalidate production queue and player title", NameDescriptor: Localization.New("server.app.revalidate_production_queue_and.d06aa141", "Revalidate production queue and player title", nil), Action: "production.enqueue.verify_capacity", ActionArguments: guardArguments,
		}))
		enqueueStep := commandStep("Enqueue production stack", "bup", payload, "bup", Localization.New("server.app.enqueue_production_stack.f40f70ec", "Enqueue production stack", nil))
		enqueueStep.StaleCodes = []int{175}
		enqueueStep.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, enqueueStep)
	}
	if recruitment {
		// Recruitment AHR is deliberately at-least-once. Every newly committed
		// BUP batch gets one native request after its final enqueue, even when an
		// older request or a previous focus epoch appears to cover the castle.
		steps = appendRecruitmentBUPAllianceHelpSteps(steps, helpArguments)
	}
	summary := fmt.Sprintf("Queue %d %s at %s", request.Amount, definitionLabel, castleLabel(castle))
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.queue_p_p_at.8430a9e8", "Queue {p0, number} {p1} at {p2}", Localization.Params{"p0": request.Amount, "p1": fmt.Sprintf("%s", definitionLabel), "p2": fmt.Sprintf("%s", castleLabel(castle))})
	if stackCount > 1 {
		summary = fmt.Sprintf("Queue %d stacks of %d %s at %s", stackCount, request.Amount, definitionLabel, castleLabel(castle))
		summaryLocalizationMessage = Localization.New("server.app.queue_p_stacks_of.3e5fa3c2", "Queue {p0, number} stacks of {p1, number} {p2} at {p3}", Localization.Params{"p0": stackCount, "p1": request.Amount, "p2": fmt.Sprintf("%s", definitionLabel), "p3": fmt.Sprintf("%s", castleLabel(castle))})
	}
	claims := []string{
		"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
		"production-line:" + strconv.Itoa(request.LineID), "account-resources",
	}
	if request.LineID == recruitmentProductionLineID {
		claims = append(claims, "alliance-help")
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: steps,
	}, nil
}

func appendRecruitmentBUPAllianceHelpSteps(steps []Intent.Step, arguments json.RawMessage) []Intent.Step {
	return append(steps, Intent.Step{
		Name: "Request alliance help for recruitment BUPs", NameDescriptor: Localization.New("server.app.request_alliance_help_for.886434a3", "Request alliance help for recruitment BUPs", nil),
		Resolver: "production.enqueue.alliance_help.build", ResolverArguments: arguments,
		AwaitOpcodes: []string{"ahh", "ahr"}, TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
}

func (application *Application) verifyProductionQueueCapacity(_ context.Context, arguments json.RawMessage) error {
	return application.verifyProductionQueueCapacityAt(arguments, time.Now().UTC())
}

func (application *Application) verifyProductionQueueCapacityAt(arguments json.RawMessage, now time.Time) error {
	var request productionQueueCapacityGuard
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.ScheduledDefinitionID != 0 || request.ScheduleValidUntil != nil {
		if request.ScheduledDefinitionID <= 0 || request.ScheduleValidUntil == nil || request.ScheduleValidUntil.IsZero() {
			return Localization.WithError(fmt.Errorf("%w: scheduled production guard is incomplete", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.9bb1df28", "intent plan became stale before dispatch: scheduled production guard is incomplete", nil))
		}
		if !now.Before(request.ScheduleValidUntil.UTC()) {
			return fmt.Errorf(
				"%w: schedule for production definition %d ended at %s",
				Intent.ErrPlanStale, request.ScheduledDefinitionID, request.ScheduleValidUntil.UTC().Format(time.RFC3339),
			)
		}
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("production state is unavailable"), Localization.New("server.app.production_state_is_unavailable.46c36af4", "production state is unavailable", nil))
	}
	gameState := application.State.ReadOnlyView()
	var gameData *GameData.Store
	if application.GameData != nil {
		gameData, _ = application.GameData.Current()
	}
	if err := validateProductionGloryTitle(
		gameState,
		gameData,
		request.DefinitionID,
		request.TitleGatedDefinitionID,
		request.RequiredGloryTitleID,
		request.TitleLossFallback,
	); err != nil {
		return err
	}
	castle, exists := gameState.Castles[request.CastleID]
	if !exists || !castle.Focused {
		return Localization.WithError(fmt.Errorf("%w: castle %d is no longer focused", Intent.ErrPlanStale, request.CastleID), Localization.New("server.app.intent_plan_became_stale.5b97c8ab", "intent plan became stale before dispatch: castle {p1} is no longer focused", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	if State.CastleFocusKnownUnavailable(gameState, castle) {
		return Localization.WithError(fmt.Errorf(
			"%w: castle %d cannot be focused in the current kingdom session", Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.f50ee7dc", "intent plan became stale before dispatch: castle {p1} cannot be focused in the current kingdom session", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	queue, exists := castle.Production[request.LineID]
	if !exists || State.ProductionQueueNeedsRefresh(gameState, queue, now) ||
		State.ProductionQueuePredatesCastleSnapshot(castle, queue) {
		return Localization.WithError(fmt.Errorf("%w: production line %d needs a current slot observation", Intent.ErrPlanStale, request.LineID), Localization.New("server.app.intent_plan_became_stale.c137896e", "intent plan became stale before dispatch: production line {p1} needs a current slot observation", Localization.Params{"p1": fmt.Sprintf("%d", request.LineID)}))
	}
	if request.RequireNewerQueue &&
		(request.QueueObservedAt.IsZero() || !queue.ObservedAt.After(request.QueueObservedAt)) {
		return Localization.WithError(fmt.Errorf(
			"%w: production line %d did not refresh after castle focus",
			Intent.ErrPlanStale, request.LineID,
		), Localization.New("server.app.intent_plan_became_stale.6e2a671e", "intent plan became stale before dispatch: production line {p1} did not refresh after castle focus", Localization.Params{"p1": fmt.Sprintf("%d", request.LineID)}))
	}
	if !productionDefinitionAvailable(castle, request.LineID, request.DefinitionID) {
		return Localization.WithError(fmt.Errorf(
			"%w: production definition %d is no longer available at castle %d",
			Intent.ErrPlanStale, request.DefinitionID, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.03239d94", "intent plan became stale before dispatch: production definition {p1} is no longer available at castle {p2}", Localization.Params{"p1": fmt.Sprintf("%d", request.DefinitionID), "p2": fmt.Sprintf("%d", request.CastleID)}))
	}
	available := productionQueueCapacity(gameState, request.LineID, queue, gameData) - len(queue.Queued)
	if available < request.ExpectedFreeSlots || request.FillAvailable && available != request.ExpectedFreeSlots {
		return fmt.Errorf(
			"%w: production line %d free slots changed from %d to %d",
			Intent.ErrPlanStale, request.LineID, request.ExpectedFreeSlots, max(0, available),
		)
	}
	return nil
}

func validateProductionGloryTitle(
	gameState State.GameState,
	gameData *GameData.Store,
	definitionID int64,
	titleGatedDefinitionID int64,
	requiredGloryTitleID int64,
	titleLossFallback bool,
) error {
	if titleGatedDefinitionID <= 0 && requiredGloryTitleID <= 0 && !titleLossFallback {
		if gameData == nil {
			return nil
		}
		unlock, titleGated := gameData.GloryTitleUnlockForUnit(definitionID)
		if !titleGated {
			return nil
		}
		titleGatedDefinitionID = unlock.UnitID
		requiredGloryTitleID = unlock.RequiredTitleID
	}
	if gameData == nil || titleGatedDefinitionID <= 0 || requiredGloryTitleID <= 0 {
		return Localization.WithError(fmt.Errorf("%w: glory-title production guard is incomplete", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.fd1837d4", "intent plan became stale before dispatch: glory-title production guard is incomplete", nil))
	}
	unlock, found := gameData.GloryTitleUnlockForUnit(titleGatedDefinitionID)
	if !found || unlock.RequiredTitleID != requiredGloryTitleID {
		return Localization.WithError(fmt.Errorf("%w: official glory-title unit mapping changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4721e27a", "intent plan became stale before dispatch: official glory-title unit mapping changed", nil))
	}
	currentTitleID, current := gameState.Player.CurrentGloryTitle(gameState.Session.ConnectionGeneration)
	if !current {
		return Localization.WithError(fmt.Errorf("%w: current player glory title has not been observed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.dad05d00", "intent plan became stale before dispatch: current player glory title has not been observed", nil))
	}
	titleEligible := gameData.GloryTitleIncludes(currentTitleID, requiredGloryTitleID)
	if titleLossFallback {
		if unlock.Level10UnitID <= 0 || definitionID != unlock.Level10UnitID {
			return Localization.WithError(fmt.Errorf("%w: level 10 glory-title fallback mapping changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.cafcfb34", "intent plan became stale before dispatch: level 10 glory-title fallback mapping changed", nil))
		}
		if titleEligible {
			return Localization.WithError(fmt.Errorf("%w: required glory title was restored before the fallback recruit", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.0f921bf1", "intent plan became stale before dispatch: required glory title was restored before the fallback recruit", nil))
		}
		return nil
	}
	if definitionID != titleGatedDefinitionID {
		return Localization.WithError(fmt.Errorf("%w: title-gated recruit definition changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.33c81ef6", "intent plan became stale before dispatch: title-gated recruit definition changed", nil))
	}
	if !titleEligible {
		return Localization.WithError(fmt.Errorf("%w: required glory title was lost before the level 11 recruit", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.39efc286", "intent plan became stale before dispatch: required glory title was lost before the level 11 recruit", nil))
	}
	return nil
}

func productionDefinitionAvailable(castle State.CastleState, lineID int, definitionID int64) bool {
	if castle.QueueableObservedAt.IsZero() {
		return true
	}
	collection := "units"
	if lineID == 1 {
		collection = "tools"
	}
	for _, definition := range castle.QueueableProduction[lineID] {
		if definition.ID == definitionID && (definition.Collection == "" || definition.Collection == collection) {
			return true
		}
	}
	return false
}

func productionQueueCapacity(state State.GameState, lineID int, queue State.ProductionQueue, gameData *GameData.Store) int {
	// The observed slot count is authoritative: the server reports every slot
	// the player owns, including slots granted by capacity effects the VIP
	// model below knows nothing about. Clamping to the VIP expectation used
	// to discard those effect slots; the base+VIP expectation now serves only
	// as the fallback before the first queue snapshot arrives. If a stale
	// observation ever overshoots, the enqueue verify-capacity guard
	// revalidates against live state before dispatch.
	if queue.Capacity > 0 {
		return queue.Capacity
	}
	expected, _ := productionVIPQueueCapacity(state, lineID, gameData)
	return expected
}

func productionVIPQueueCapacity(state State.GameState, lineID int, gameData *GameData.Store) (int, bool) {
	if gameData == nil || state.Player.VIP.Level <= 0 {
		return productionBaseQueueCapacity, false
	}
	catalog, err := gameData.Catalog("viplevels")
	if err != nil {
		return productionBaseQueueCapacity, false
	}
	raw, found := catalog.Find(strconv.Itoa(state.Player.VIP.Level))
	if !found {
		return productionBaseQueueCapacity, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return productionBaseQueueCapacity, false
	}
	field := "recruitmentBonusSlots"
	if lineID == 1 {
		field = "productionBonusSlots"
	}
	bonus, exists := record.Int64(field)
	if !exists || bonus < 0 {
		return productionBaseQueueCapacity, false
	}
	return productionBaseQueueCapacity + int(bonus), true
}

func planHospitalHeal(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planHospitalOperation(input, arguments, false)
}

func planHospitalDiscard(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planHospitalOperation(input, arguments, true)
}

func planHospitalOperation(input Intent.PlanningContext, arguments json.RawMessage, discard bool) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
		UnitID   State.UnitID   `json:"unitId"`
		Amount   int64          `json:"amount"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if State.CastleFocusKnownUnavailable(input.State, castle) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: castle %d cannot be focused in the current kingdom session", Intent.ErrPlanStale, request.CastleID,
		), Localization.New("server.app.intent_plan_became_stale.f50ee7dc", "intent plan became stale before dispatch: castle {p1} cannot be focused in the current kingdom session", Localization.Params{"p1": fmt.Sprintf("%d", request.CastleID)}))
	}
	unitLabel := productionDefinitionLabel(input.GameData, input.Language, "units", int64(request.UnitID))
	wounded := castle.Units.Hospital[request.UnitID]
	if request.UnitID <= 0 || wounded <= 0 {
		return Intent.Plan{}, fmt.Errorf("%s is not wounded at %s", unitLabel, castleLabel(castle))
	}
	if request.Amount <= 0 || request.Amount > wounded {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("amount must be between 1 and the wounded count %d", wounded), Localization.New("server.app.amount_must_be_between.57179501", "amount must be between 1 and the wounded count {p0}", Localization.Params{"p0": wounded}))
	}
	if err := requireOfficialDefinition(input.GameData, "units", int64(request.UnitID)); err != nil {
		return Intent.Plan{}, err
	}
	if !discard {
		rubyCost, known := officialNumber(input.GameData, "units", int64(request.UnitID), "healingCostC2")
		if known && rubyCost > 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("%s requires rubies to heal; use hospital.discard or heal it manually", unitLabel), Localization.New("server.app.p_requires_rubies_to.a543b4b7", "{p0} requires rubies to heal; use hospital.discard or heal it manually", Localization.Params{"p0": fmt.Sprintf("%s", unitLabel)}))
		}
	}
	payload, _ := json.Marshal(map[string]any{"U": request.UnitID, "A": request.Amount})
	opcode := "hru"
	label := "Heal wounded units"
	var labelLocalizationMessage *Localization.Message = Localization.New("server.app.heal_wounded_units.df85ef2b", "Heal wounded units", nil)
	if discard {
		opcode = "hdu"
		label = "Discard wounded units"
		labelLocalizationMessage = Localization.New("server.app.discard_wounded_units.f563039b", "Discard wounded units", nil)
	}
	// Healing is focus-sensitive. Always refresh the castle after claims are
	// acquired because another operation may have displaced a previously
	// focused castle while this plan waited for admission.
	focusStep := castleFocusStep(castle)
	focusStep.StaleCodes = []int{175}
	operationStep := commandStep(label, opcode, payload, opcode, Localization.Clone(labelLocalizationMessage))
	operationStep.StaleCodes = []int{175}
	steps := []Intent.Step{focusStep, operationStep}
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"hospital", "account-resources",
		},
		Summary: fmt.Sprintf(
			"%s: %d %s at %s",
			label, request.Amount, unitLabel, castleLabel(castle),
		), SummaryDescriptor: Localization.New("server.app.p_p_p_at.2403c8fc", "{p0}: {p1} {p2} at {p3}", Localization.Params{"p0": fmt.Sprintf("%s", label), "p1": request.Amount, "p2": fmt.Sprintf("%s", unitLabel), "p3": fmt.Sprintf("%s", castleLabel(castle))}),
		Steps: steps,
	}, nil
}

// observedProductionStack picks the per-stack amount FillAvailable sends for
// one unit definition. The game never reports the entitled batch size
// directly (subscriptions carry only type + remaining time), so the size is
// learned from what is visible in the queue — floored by the per-definition
// LearnedStacks high-water mark so a spell of smaller stacks cannot ratchet
// the batch size down. The learned floor only applies while the subscription
// set it was recorded under still matches; after a lapse the floor is
// ignored and live stacks rule again. Batch caps are per-unit, so only
// stacks of the SAME definition inform the amount; a unit with no history at
// all falls back to mimicking whatever the line currently runs (the only
// signal available on a cold start).
func observedProductionStack(gameState State.GameState, queue State.ProductionQueue, definitionID int64) int64 {
	var amount int64
	if queue.LearnedStackScope == gameState.SubscriptionScope() {
		amount = queue.LearnedStacks[definitionID]
	}
	var anyDefinition int64
	consider := func(itemDefinition, itemAmount int64) {
		if itemAmount > anyDefinition {
			anyDefinition = itemAmount
		}
		if itemDefinition == definitionID && itemAmount > amount {
			amount = itemAmount
		}
	}
	if queue.Active != nil {
		consider(int64(queue.Active.Definition.ID), queue.Active.Amount)
	}
	for _, item := range queue.Queued {
		consider(int64(item.Definition.ID), item.Amount)
	}
	if amount > 0 {
		return amount
	}
	return anyDefinition
}

func requireOfficialDefinition(store *GameData.Store, collection string, id int64) error {
	if store == nil || id <= 0 {
		return Localization.WithError(fmt.Errorf("%s definition %d requires the loaded official catalog", collection, id), Localization.New("server.app.p_definition_p_requires.530cbd49", "{p0} definition {p1} requires the loaded official catalog", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": fmt.Sprintf("%d", id)}))
	}
	catalogName := collection
	if collection == "tools" {
		catalogName = "units"
	}
	catalog, err := store.Catalog(catalogName)
	if err != nil {
		return err
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return Localization.WithError(fmt.Errorf("%s definition %d is not in the current official catalog", collection, id), Localization.New("server.app.p_definition_p_is.ba15e94c", "{p0} definition {p1} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": fmt.Sprintf("%d", id)}))
	}
	if collection == "units" || collection == "tools" {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return Localization.WithError(fmt.Errorf("decode %s definition %d: %w", collection, id, decodeErr), Localization.ErrorContext(Localization.New("server.app.decode_p_definition_p.c80a806a", "decode {p0} definition {p1}", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": fmt.Sprintf("%d", id)}), decodeErr))
		}
		isTool := GameData.IsToolRecord(record)
		if collection == "tools" && !isTool || collection == "units" && isTool {
			return Localization.WithError(fmt.Errorf("definition %d is not an official %s item", id, collection), Localization.New("server.app.definition_p_is_not.aeb71aa2", "definition {p0} is not an official {p1} item", Localization.Params{"p0": fmt.Sprintf("%d", id), "p1": fmt.Sprintf("%s", collection)}))
		}
	}
	return nil
}

func officialNumber(store *GameData.Store, collection string, id int64, field string) (float64, bool) {
	if store == nil {
		return 0, false
	}
	catalog, err := store.Catalog(collection)
	if err != nil {
		return 0, false
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return 0, false
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return 0, false
	}
	return record.Float64(field)
}

package App

import (
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
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if request.LineID != 0 && request.LineID != 1 {
		return Intent.Plan{}, fmt.Errorf("production line %d is not a recruitment or tool line", request.LineID)
	}
	if request.ScheduledDefinitionID != 0 || request.ScheduleValidUntil != nil {
		if request.ScheduledDefinitionID <= 0 || request.ScheduledDefinitionID != request.DefinitionID {
			return Intent.Plan{}, fmt.Errorf("scheduled production definition must match definitionId %d", request.DefinitionID)
		}
		if request.ScheduleValidUntil == nil || request.ScheduleValidUntil.IsZero() {
			return Intent.Plan{}, fmt.Errorf("scheduled production requires scheduleValidUntil")
		}
		if request.FillAvailable {
			return Intent.Plan{}, fmt.Errorf("scheduled production must enqueue one stack before reevaluating the schedule")
		}
	}
	queue, ok := castle.Production[request.LineID]
	if !ok || queue.ObservedAt.IsZero() {
		return Intent.Plan{}, fmt.Errorf("production line %d has not been observed for castle %d", request.LineID, request.CastleID)
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
			return Intent.Plan{Summary: fmt.Sprintf("Production line %d is already full at %s", request.LineID, castleLabel(castle))}, nil
		}
		return Intent.Plan{}, fmt.Errorf("production line %d is full", request.LineID)
	}
	if request.Amount <= 0 {
		request.Amount = observedProductionStack(input.State, queue, request.DefinitionID)
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("production stack size is unknown; create one %s stack in-game so CitadelOps can learn the live amount", collection)
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
	for stack := 0; stack < stackCount; stack++ {
		guardArguments, _ := json.Marshal(productionQueueCapacityGuard{
			CastleID: request.CastleID, LineID: request.LineID, DefinitionID: request.DefinitionID,
			ExpectedFreeSlots: stackCount - stack, FillAvailable: request.FillAvailable,
			ScheduledDefinitionID: request.ScheduledDefinitionID, ScheduleValidUntil: request.ScheduleValidUntil,
			TitleGatedDefinitionID: request.TitleGatedDefinitionID,
			RequiredGloryTitleID:   request.RequiredGloryTitleID,
			TitleLossFallback:      request.TitleLossFallback,
		})
		steps = append(steps, Intent.RebuildOnResume(Intent.Step{
			Name: "Revalidate production queue and player title", Action: "production.enqueue.verify_capacity", ActionArguments: guardArguments,
		}))
		steps = append(steps, commandStep("Enqueue production stack", "bup", payload, "bup"))
	}
	summary := fmt.Sprintf("Queue %d %s at %s", request.Amount, definitionLabel, castleLabel(castle))
	if stackCount > 1 {
		summary = fmt.Sprintf("Queue %d stacks of %d %s at %s", stackCount, request.Amount, definitionLabel, castleLabel(castle))
	}
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"production-line:" + strconv.Itoa(request.LineID), "account-resources",
		},
		Summary: summary,
		Steps:   steps,
	}, nil
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
			return fmt.Errorf("%w: scheduled production guard is incomplete", Intent.ErrPlanStale)
		}
		if !now.Before(request.ScheduleValidUntil.UTC()) {
			return fmt.Errorf(
				"%w: schedule for production definition %d ended at %s",
				Intent.ErrPlanStale, request.ScheduledDefinitionID, request.ScheduleValidUntil.UTC().Format(time.RFC3339),
			)
		}
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("production state is unavailable")
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
		return fmt.Errorf("%w: castle %d is no longer focused", Intent.ErrPlanStale, request.CastleID)
	}
	queue, exists := castle.Production[request.LineID]
	if !exists || queue.ObservedAt.IsZero() {
		return fmt.Errorf("%w: production line %d is no longer observed", Intent.ErrPlanStale, request.LineID)
	}
	if !productionDefinitionAvailable(castle, request.LineID, request.DefinitionID) {
		return fmt.Errorf(
			"%w: production definition %d is no longer available at castle %d",
			Intent.ErrPlanStale, request.DefinitionID, request.CastleID,
		)
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
		return fmt.Errorf("%w: glory-title production guard is incomplete", Intent.ErrPlanStale)
	}
	unlock, found := gameData.GloryTitleUnlockForUnit(titleGatedDefinitionID)
	if !found || unlock.RequiredTitleID != requiredGloryTitleID {
		return fmt.Errorf("%w: official glory-title unit mapping changed", Intent.ErrPlanStale)
	}
	currentTitleID, current := gameState.Player.CurrentGloryTitle(gameState.Session.ConnectionGeneration)
	if !current {
		return fmt.Errorf("%w: current player glory title has not been observed", Intent.ErrPlanStale)
	}
	titleEligible := gameData.GloryTitleIncludes(currentTitleID, requiredGloryTitleID)
	if titleLossFallback {
		if unlock.Level10UnitID <= 0 || definitionID != unlock.Level10UnitID {
			return fmt.Errorf("%w: level 10 glory-title fallback mapping changed", Intent.ErrPlanStale)
		}
		if titleEligible {
			return fmt.Errorf("%w: required glory title was restored before the fallback recruit", Intent.ErrPlanStale)
		}
		return nil
	}
	if definitionID != titleGatedDefinitionID {
		return fmt.Errorf("%w: title-gated recruit definition changed", Intent.ErrPlanStale)
	}
	if !titleEligible {
		return fmt.Errorf("%w: required glory title was lost before the level 11 recruit", Intent.ErrPlanStale)
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
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	unitLabel := productionDefinitionLabel(input.GameData, input.Language, "units", int64(request.UnitID))
	wounded := castle.Units.Hospital[request.UnitID]
	if request.UnitID <= 0 || wounded <= 0 {
		return Intent.Plan{}, fmt.Errorf("%s is not wounded at %s", unitLabel, castleLabel(castle))
	}
	if request.Amount <= 0 || request.Amount > wounded {
		return Intent.Plan{}, fmt.Errorf("amount must be between 1 and the wounded count %d", wounded)
	}
	if err := requireOfficialDefinition(input.GameData, "units", int64(request.UnitID)); err != nil {
		return Intent.Plan{}, err
	}
	if !discard {
		rubyCost, known := officialNumber(input.GameData, "units", int64(request.UnitID), "healingCostC2")
		if known && rubyCost > 0 {
			return Intent.Plan{}, fmt.Errorf("%s requires rubies to heal; use hospital.discard or heal it manually", unitLabel)
		}
	}
	payload, _ := json.Marshal(map[string]any{"U": request.UnitID, "A": request.Amount})
	opcode := "hru"
	label := "Heal wounded units"
	if discard {
		opcode = "hdu"
		label = "Discard wounded units"
	}
	// Healing is focus-sensitive. Always refresh the castle after claims are
	// acquired because another operation may have displaced a previously
	// focused castle while this plan waited for admission.
	steps := []Intent.Step{castleFocusStep(castle)}
	steps = append(steps, commandStep(label, opcode, payload, opcode))
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"hospital", "account-resources",
		},
		Summary: fmt.Sprintf(
			"%s: %d %s at %s",
			label, request.Amount, unitLabel, castleLabel(castle),
		),
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
		return fmt.Errorf("%s definition %d requires the loaded official catalog", collection, id)
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
		return fmt.Errorf("%s definition %d is not in the current official catalog", collection, id)
	}
	if collection == "units" || collection == "tools" {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return fmt.Errorf("decode %s definition %d: %w", collection, id, decodeErr)
		}
		isTool := GameData.IsToolRecord(record)
		if collection == "tools" && !isTool || collection == "units" && isTool {
			return fmt.Errorf("definition %d is not an official %s item", id, collection)
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

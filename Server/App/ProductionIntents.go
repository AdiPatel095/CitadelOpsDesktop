package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
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
	CastleID              State.CastleID `json:"castleId"`
	LineID                int            `json:"lineId"`
	ExpectedFreeSlots     int            `json:"expectedFreeSlots"`
	FillAvailable         bool           `json:"fillAvailable"`
	ScheduledDefinitionID int64          `json:"scheduledDefinitionId,omitempty"`
	ScheduleValidUntil    *time.Time     `json:"scheduleValidUntil,omitempty"`
}

func planProductionEnqueue(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID              State.CastleID `json:"castleId"`
		LineID                int            `json:"lineId"`
		DefinitionID          int64          `json:"definitionId"`
		Amount                int64          `json:"amount,omitempty"`
		FillAvailable         bool           `json:"fillAvailable,omitempty"`
		ScheduledDefinitionID int64          `json:"scheduledDefinitionId,omitempty"`
		ScheduleValidUntil    *time.Time     `json:"scheduleValidUntil,omitempty"`
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
		request.Amount = observedProductionStack(queue)
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
	guardArguments, _ := json.Marshal(productionQueueCapacityGuard{
		CastleID: request.CastleID, LineID: request.LineID,
		ExpectedFreeSlots: stackCount, FillAvailable: request.FillAvailable,
		ScheduledDefinitionID: request.ScheduledDefinitionID, ScheduleValidUntil: request.ScheduleValidUntil,
	})
	steps = append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Revalidate production queue capacity", Action: "production.enqueue.verify_capacity", ActionArguments: guardArguments,
	}))
	for stack := 0; stack < stackCount; stack++ {
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
	gameState := application.State.Snapshot()
	castle, exists := gameState.Castles[request.CastleID]
	if !exists || !castle.Focused {
		return fmt.Errorf("%w: castle %d is no longer focused", Intent.ErrPlanStale, request.CastleID)
	}
	queue, exists := castle.Production[request.LineID]
	if !exists || queue.ObservedAt.IsZero() {
		return fmt.Errorf("%w: production line %d is no longer observed", Intent.ErrPlanStale, request.LineID)
	}
	var gameData *GameData.Store
	if application.GameData != nil {
		gameData, _ = application.GameData.Current()
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

func productionQueueCapacity(state State.GameState, lineID int, queue State.ProductionQueue, gameData *GameData.Store) int {
	expected, known := productionVIPQueueCapacity(state, lineID, gameData)
	if queue.Capacity <= 0 {
		return expected
	}
	if !known || queue.Capacity < expected {
		return queue.Capacity
	}
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

func observedProductionStack(queue State.ProductionQueue) int64 {
	var amount int64
	if queue.Active != nil && queue.Active.Amount > amount {
		amount = queue.Active.Amount
	}
	for _, item := range queue.Queued {
		if item.Amount > amount {
			amount = item.Amount
		}
	}
	return amount
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

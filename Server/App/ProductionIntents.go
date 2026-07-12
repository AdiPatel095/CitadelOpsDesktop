package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func planProductionEnqueue(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID     State.CastleID `json:"castleId"`
		LineID       int            `json:"lineId"`
		DefinitionID int64          `json:"definitionId"`
		Amount       int64          `json:"amount,omitempty"`
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
	queue, ok := castle.Production[request.LineID]
	if !ok || queue.ObservedAt.IsZero() {
		return Intent.Plan{}, fmt.Errorf("production line %d has not been observed for castle %d", request.LineID, request.CastleID)
	}
	if queue.Capacity <= 0 || len(queue.Queued) >= queue.Capacity {
		return Intent.Plan{}, fmt.Errorf("production line %d is full", request.LineID)
	}
	collection := "units"
	if request.LineID == 1 {
		collection = "tools"
	}
	if err := requireOfficialDefinition(input.GameData, collection, request.DefinitionID); err != nil {
		return Intent.Plan{}, err
	}
	if request.Amount <= 0 {
		request.Amount = observedProductionStack(queue)
	}
	if request.Amount <= 0 {
		return Intent.Plan{}, fmt.Errorf("production stack size is unknown; create one %s stack in-game so CitadelOps can learn the live amount", collection)
	}
	if input.State.CommandContext.ProductionSessionKey <= 0 {
		return Intent.Plan{}, fmt.Errorf("production session key is unknown; enqueue one stack in-game so CitadelOps can learn the current command context")
	}
	payload, _ := json.Marshal(struct {
		LineID       int            `json:"LID"`
		DefinitionID int64          `json:"WID"`
		Amount       int64          `json:"AMT"`
		PublicOrder  int            `json:"PO"`
		Power        int            `json:"PWR"`
		SessionKey   int            `json:"SK"`
		SlotID       int            `json:"SID"`
		CastleID     State.CastleID `json:"AID"`
	}{request.LineID, request.DefinitionID, request.Amount, -1, 0, input.State.CommandContext.ProductionSessionKey, 0, request.CastleID})
	steps := castleContextSteps(castle)
	steps = append(steps, commandStep("Enqueue production stack", "bup", payload, "bup"))
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"production-line:" + strconv.Itoa(request.LineID), "account-resources",
		},
		Summary: fmt.Sprintf("Queue %d of %s definition %d at %s", request.Amount, collection, request.DefinitionID, castleLabel(castle)),
		Steps:   steps,
	}, nil
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
	wounded := castle.Units.Hospital[request.UnitID]
	if request.UnitID <= 0 || wounded <= 0 {
		return Intent.Plan{}, fmt.Errorf("unit %d is not wounded at castle %d", request.UnitID, request.CastleID)
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
			return Intent.Plan{}, fmt.Errorf("unit %d requires rubies to heal; use hospital.discard or heal it manually", request.UnitID)
		}
	}
	payload, _ := json.Marshal(map[string]any{"U": request.UnitID, "A": request.Amount})
	opcode := "hru"
	label := "Heal wounded units"
	if discard {
		opcode = "hdu"
		label = "Discard wounded units"
	}
	steps := castleContextSteps(castle)
	steps = append(steps, commandStep(label, opcode, payload, opcode))
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"hospital", "account-resources",
		},
		Summary: fmt.Sprintf("%s: %d of unit %d at %s", label, request.Amount, request.UnitID, castleLabel(castle)),
		Steps:   steps,
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
	catalog, err := store.Catalog(collection)
	if err != nil {
		return err
	}
	if _, exists := catalog.Find(strconv.FormatInt(id, 10)); !exists {
		return fmt.Errorf("%s definition %d is not in the current official catalog", collection, id)
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

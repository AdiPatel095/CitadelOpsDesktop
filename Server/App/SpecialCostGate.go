package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type specialCostWatermark struct {
	observation State.PlayerResourceObservation
	balance     int64
}
type specialCostDispatchGate struct {
	mu         sync.Mutex
	pending    map[Intent.BalanceKey]map[string]pendingCoinDebit
	watermarks map[Intent.BalanceKey]specialCostWatermark
}

func newSpecialCostDispatchGate() *specialCostDispatchGate {
	return &specialCostDispatchGate{pending: map[Intent.BalanceKey]map[string]pendingCoinDebit{}, watermarks: map[Intent.BalanceKey]specialCostWatermark{}}
}
func specialProductionRequest(step Intent.Step) (unit, amount int64, line int, castle State.CastleID, err error) {
	var body struct {
		Unit   int64          `json:"WID"`
		Amount int64          `json:"AMT"`
		Line   *int           `json:"LID"`
		Castle State.CastleID `json:"AID"`
	}
	err = json.Unmarshal(step.Payload, &body)
	if err != nil || body.Unit <= 0 || body.Amount <= 0 || body.Line == nil || (*body.Line != 0 && *body.Line != 1) || body.Castle <= 0 {
		return 0, 0, 0, 0, &Automation.ProductionCostBlock{Field: "payload"}
	}
	return body.Unit, body.Amount, *body.Line, body.Castle, nil
}
func (gate *specialCostDispatchGate) budget(state State.GameState, key Intent.BalanceKey) (int64, State.PlayerResourceObservation, bool) {
	amount, observation, known := Intent.ObservedBalance(state, key)
	if !known {
		return amount, observation, false
	}
	watermark := gate.watermarks[key]
	if watermark.observation.ConnectionGeneration > observation.ConnectionGeneration ||
		(watermark.observation.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Before(watermark.observation.ObservedAt)) ||
		(key.Kind != Intent.BalanceCastleResource && watermark.observation.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Equal(watermark.observation.ObservedAt) && !watermark.observation.ObservedAt.IsZero() && amount != watermark.balance) {
		return amount, observation, false
	}
	gate.watermarks[key] = specialCostWatermark{observation, amount}
	for token, debit := range gate.pending[key] {
		if observation.ConnectionGeneration != debit.connectionGeneration {
			if observation.ConnectionGeneration > 0 && observation.ObservedAt.After(debit.reservedAt) {
				delete(gate.pending[key], token)
			}
			continue
		}
		after := debit.completedAt
		if after.IsZero() {
			after = debit.indeterminateAt
		}
		if !after.IsZero() && observation.ObservedAt.After(after) {
			delete(gate.pending[key], token)
		}
	}
	return amount, observation, true
}
func (gate *specialCostDispatchGate) Validate(ctx context.Context, input Intent.PlanningContext, step Intent.Step) error {
	if !strings.EqualFold(step.Opcode, "bup") {
		return nil
	}
	unit, amount, line, castle, err := specialProductionRequest(step)
	if err != nil {
		return productionCostError(input, unit, err)
	}
	costs, err := resolveProductionCosts(input.GameData, input.Language, line, unit, castle)
	if err != nil {
		return productionCostError(input, unit, err)
	}
	token := coinDispatchKey(ctx, step)
	gate.mu.Lock()
	defer gate.mu.Unlock()
	debits := make(map[Intent.BalanceKey]pendingCoinDebit)
	for _, cost := range costs {
		if cost.Key.Kind == Intent.BalancePlayerResource {
			continue
		} // C1 is owned by coinDispatchGate.
		observed, observation, known := gate.budget(input.State, cost.Key)
		needed, costErr := checkedCeilProduct(cost.PerUnit, amount)
		if costErr != nil {
			return productionCostError(input, unit, &Automation.ProductionCostBlock{Field: "amount"})
		}
		pending := int64(0)
		for other, debit := range gate.pending[cost.Key] {
			if other != token {
				if debit.amount > math.MaxInt64-pending {
					pending = math.MaxInt64
					break
				}
				pending += debit.amount
			} else if debit.amount != needed {
				return productionCostError(input, unit, &Automation.ProductionCostBlock{Field: "amount"})
			}
		}
		if !known || needed > max(int64(0), observed-pending) {
			castleName := input.State.Castles[castle].Name
			if castleName == "" {
				castleName = fmt.Sprint(castle)
			}
			shortage := &Intent.BalanceUnavailableError{Key: cost.Key, Required: needed, Observed: observed, Pending: pending, Known: known, Name: cost.Name, NameKey: cost.NameKey, CastleName: castleName}
			return Localization.WithError(shortage, shortage.LocalizationMessage())
		}
		debits[cost.Key] = pendingCoinDebit{amount: needed, observedBalance: observed, observedAt: observation.ObservedAt, connectionGeneration: observation.ConnectionGeneration, reservedAt: time.Now().UTC()}
	}
	// Commit reservations only after every balance passes, in the same mutex.
	for key, debit := range debits {
		if gate.pending[key] == nil {
			gate.pending[key] = map[string]pendingCoinDebit{}
		}
		if _, exists := gate.pending[key][token]; !exists {
			gate.pending[key][token] = debit
		}
	}
	return nil
}
func (gate *specialCostDispatchGate) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	token := coinDispatchKey(ctx, step)
	for _, debits := range gate.pending {
		delete(debits, token)
	}
}
func (gate *specialCostDispatchGate) Indeterminate(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	token := coinDispatchKey(ctx, step)
	for _, debits := range gate.pending {
		if debit, exists := debits[token]; exists {
			debit.indeterminateAt = time.Now().UTC()
			debits[token] = debit
		}
	}
}
func castleCostResponseCorrelated(input Intent.PlanningContext, key Intent.BalanceKey, response Protocol.CommittedFrame) bool {
	root, err := response.Frame.PayloadRoot()
	if err != nil {
		return false
	}
	raw := root["grc"]
	if len(raw) == 0 && strings.EqualFold(response.Frame.Opcode, "grc") {
		raw = response.Frame.Payload
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return false
	}
	castleID, _ := record.Int64("AID")
	if castleID <= 0 || input.State.Castles[State.CastleID(castleID)].ID <= 0 {
		castleID = 0
		for id, castle := range input.State.Castles {
			if castle.Focused {
				castleID = int64(id)
				break
			}
		}
	}
	if State.CastleID(castleID) != key.CastleID || input.GameData == nil {
		return false
	}
	// An omitted/malformed resource does not prove this debit was committed.
	catalog, err := input.GameData.Catalog("resources")
	if err != nil {
		return false
	}
	rawDefinition, found := catalog.Find(fmt.Sprint(key.ID))
	if !found {
		return false
	}
	definition, err := GameData.DecodeRecord(rawDefinition)
	if err != nil {
		return false
	}
	jsonKey, _ := definition.String("JSONKey")
	amount, numeric := record.Float64(jsonKey)
	return numeric && amount >= 0 && !math.IsNaN(amount) && !math.IsInf(amount, 0) && amount < math.Exp2(63)
}
func (gate *specialCostDispatchGate) Completed(ctx context.Context, input Intent.PlanningContext, step Intent.Step, response Protocol.CommittedFrame) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	token := coinDispatchKey(ctx, step)
	refresh := false
	for key, debits := range gate.pending {
		debit, exists := debits[token]
		if !exists {
			continue
		}
		debit.completedAt = time.Now().UTC()
		debits[token] = debit
		balance, observation, known := Intent.ObservedBalance(input.State, key)
		correlated := response.ReduceError == "" && !response.Frame.ReceivedAt.IsZero() && known && observation.ConnectionGeneration == debit.connectionGeneration
		if key.Kind == Intent.BalanceCastleResource {
			correlated = correlated && castleCostResponseCorrelated(input, key, response)
		} else {
			correlated = correlated && observation.ObservedAt.Equal(response.Frame.ReceivedAt)
		}
		if correlated {
			watermark := gate.watermarks[key]
			if observation.ConnectionGeneration > watermark.observation.ConnectionGeneration || !observation.ObservedAt.Before(watermark.observation.ObservedAt) {
				gate.watermarks[key] = specialCostWatermark{observation, balance}
			}
			delete(debits, token)
		} else if key.Kind == Intent.BalanceCurrency {
			refresh = true
		}
	}
	return refresh
}

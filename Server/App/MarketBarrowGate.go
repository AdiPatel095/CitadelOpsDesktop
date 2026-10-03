package App

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"
)

func marketBarrowsRequired(input Intent.PlanningContext, source State.CastleID, amount int64) (int, error) {
	row, found := input.State.Market.Castles[source]
	if !found || input.GameData == nil || !input.State.Market.CaravanLevelLoaded || amount <= 0 {
		return 0, marketBarrowPlanningUnavailable(source)
	}
	effects := make([]GameData.MarketEffect, 0, len(row.AreaEffects))
	for _, effect := range row.AreaEffects {
		effects = append(effects, GameData.MarketEffect{EffectID: effect.EffectID, Values: effect.Values})
	}
	capacity, err := input.GameData.MarketCapacity(input.State.Market.CaravanLevel, effects)
	if err != nil || capacity.CapacityPerBarrow <= 0 {
		return 0, marketBarrowPlanningUnavailable(source)
	}
	perBarrow := int64(capacity.CapacityPerBarrow)
	required := amount / perBarrow
	if amount%perBarrow != 0 {
		required++
	}
	return int(required), nil
}

// Capacity lookup retains its existing diagnostic; callers return the typed barrow wait.
func marketBarrowPlanningUnavailable(source State.CastleID) error {
	return Localization.WithError(fmt.Errorf("source castle %d has no observed available market barrows", source), Localization.New("server.app.source_castle_p_has.f0ba5631", "source castle {p0} has no observed available market barrows", Localization.Params{"p0": fmt.Sprint(source)}))
}

func marketBarrowAvailabilityError(input Intent.PlanningContext, source State.CastleID, required, observed, pending int64, known bool) error {
	name := input.State.Castles[source].Name
	if name == "" {
		name = fmt.Sprint(source)
	}
	shortage := &Intent.BalanceUnavailableError{Key: Intent.MarketBarrowBalanceKey(source), Required: required, Observed: observed, Pending: pending, Known: known, CastleName: name}
	return Localization.WithError(shortage, shortage.LocalizationMessage())
}

type pendingMarketBarrows struct {
	source     State.CastleID
	barrows    int64
	reservedAt time.Time
	outcomeAt  time.Time
}

type marketBarrowDispatchGate struct {
	mu      sync.Mutex
	pending map[string]pendingMarketBarrows
}

func newMarketBarrowDispatchGate() *marketBarrowDispatchGate {
	return &marketBarrowDispatchGate{pending: map[string]pendingMarketBarrows{}}
}

func marketBarrowRequest(step Intent.Step) (State.CastleID, int64, bool) {
	var request struct {
		Source State.CastleID      `json:"SID"`
		Goods  [][]json.RawMessage `json:"G"`
	}
	payload := step.Payload
	if len(payload) == 0 {
		payload = step.Command.Payload
	}
	if json.Unmarshal(payload, &request) != nil || request.Source <= 0 || len(request.Goods) != 1 || len(request.Goods[0]) < 2 {
		return request.Source, 0, false
	}
	var amount int64
	if json.Unmarshal(request.Goods[0][1], &amount) != nil || amount <= 0 {
		return request.Source, 0, false
	}
	return request.Source, amount, true
}

func (gate *marketBarrowDispatchGate) reconcile(state State.GameState, now time.Time) {
	for key, debit := range gate.pending {
		if !now.Before(debit.reservedAt.Add(State.MarketBarrowLeaseRetention)) ||
			(!debit.outcomeAt.IsZero() && state.Market.Castles[debit.source].ObservedAt.After(debit.outcomeAt)) {
			delete(gate.pending, key)
		}
	}
}

func (gate *marketBarrowDispatchGate) Validate(ctx context.Context, input Intent.PlanningContext, step Intent.Step) error {
	if !strings.EqualFold(step.Opcode, "crm") {
		return nil
	}
	now := time.Now().UTC()
	source, amount, valid := marketBarrowRequest(step)
	required, capacityErr := marketBarrowsRequired(input, source, amount)
	_, owned := input.State.Castles[source]
	known := valid && owned && capacityErr == nil && State.MarketBarrowSourceStatusAt(&input.State, source, now).Ready
	available := int64(State.AvailableMarketBarrowsAt(&input.State, input.State.Market.Castles[source], now))
	key := coinDispatchKey(ctx, step)
	gate.mu.Lock()
	defer gate.mu.Unlock()
	gate.reconcile(input.State, now)
	pending := int64(0)
	for other, debit := range gate.pending {
		if other != key && debit.source == source {
			if debit.barrows > math.MaxInt64-pending {
				pending = math.MaxInt64
				break
			}
			pending += debit.barrows
		}
	}
	debit, exists := gate.pending[key]
	if exists && (debit.source != source || debit.barrows != int64(required)) {
		known = false
	}
	if !known || int64(required) > max(int64(0), available-pending) {
		return marketBarrowAvailabilityError(input, source, int64(required), available, pending, known)
	}
	if !exists {
		gate.pending[key] = pendingMarketBarrows{source: source, barrows: int64(required), reservedAt: now}
	}
	return nil
}

func (gate *marketBarrowDispatchGate) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	delete(gate.pending, coinDispatchKey(ctx, step))
}

func (gate *marketBarrowDispatchGate) Indeterminate(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	key := coinDispatchKey(ctx, step)
	if debit, exists := gate.pending[key]; exists {
		debit.outcomeAt = time.Now().UTC()
		gate.pending[key] = debit
	}
}

func (gate *marketBarrowDispatchGate) Completed(ctx context.Context, input Intent.PlanningContext, step Intent.Step, response Protocol.CommittedFrame) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	key := coinDispatchKey(ctx, step)
	debit, exists := gate.pending[key]
	if !exists {
		return false
	}
	correlated := false
	if response.ReduceError == "" && !response.Frame.ReceivedAt.IsZero() {
		input.State.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
			home := movement.SourceCastleID
			if movement.Direction == 1 {
				home = movement.TargetCastleID
			}
			if home == debit.source && movement.MarketBarrows > 0 && State.MovementOwnedByCurrentPlayer(&input.State, movement) && movement.ObservedAt.Equal(response.Frame.ReceivedAt) {
				correlated = true
				return false
			}
			return true
		})
	}
	if correlated {
		delete(gate.pending, key)
	} else {
		debit.outcomeAt = time.Now().UTC()
		gate.pending[key] = debit
	}
	// Market reconciliation must never request GBD or spend coins.
	return false
}

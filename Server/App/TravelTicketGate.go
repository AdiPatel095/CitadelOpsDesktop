package App

import (
	"context"
	"encoding/json"
	"sync"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type travelTicketDispatchGate struct {
	mu               sync.Mutex
	pending          map[string]pendingCoinDebit
	watermark        State.PlayerResourceObservation
	watermarkBalance int64
}

func newTravelTicketDispatchGate() *travelTicketDispatchGate {
	return &travelTicketDispatchGate{pending: map[string]pendingCoinDebit{}}
}
func ticketTravel(payload json.RawMessage) bool {
	var fields struct{ PTT int }
	return json.Unmarshal(payload, &fields) == nil && fields.PTT == 1
}
func (gate *travelTicketDispatchGate) budget(state State.GameState) (int64, int64, bool) {
	observed, known := Intent.ObservedCurrency(state, Intent.TravelTicketCurrencyID)
	if !known {
		return 0, 0, false
	}
	observation := state.Player.CurrencyObservations[Intent.TravelTicketCurrencyID]
	if gate.watermark.ConnectionGeneration > observation.ConnectionGeneration ||
		(gate.watermark.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Before(gate.watermark.ObservedAt)) ||
		(gate.watermark.ConnectionGeneration == observation.ConnectionGeneration && observation.ObservedAt.Equal(gate.watermark.ObservedAt) && !gate.watermark.ObservedAt.IsZero() && observed != gate.watermarkBalance) {
		return observed, 0, false
	}
	gate.watermark = observation
	gate.watermarkBalance = observed
	gate.reconcile(observation, observed)
	pending := int64(len(gate.pending))
	return observed, pending, true
}
func (gate *travelTicketDispatchGate) AvailableCurrency(state State.GameState, id State.CurrencyID) (int64, int64, bool) {
	if id != Intent.TravelTicketCurrencyID {
		observed, known := Intent.ObservedCurrency(state, id)
		return observed, 0, known
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	return gate.budget(state)
}
func (gate *travelTicketDispatchGate) Validate(ctx context.Context, input Intent.PlanningContext, step Intent.Step) error {
	if !ticketTravel(step.Payload) {
		return nil
	}
	gate.mu.Lock()
	defer gate.mu.Unlock()
	observed, pending, known := gate.budget(input.State)
	key := coinDispatchKey(ctx, step)
	_, exists := gate.pending[key]
	if exists {
		pending--
	}
	if !known || observed-pending < 1 {
		err := &Intent.CurrencyUnavailableError{CurrencyID: Intent.TravelTicketCurrencyID, Required: 1, Observed: observed, Pending: pending, Known: known}
		return Localization.WithError(err, err.LocalizationMessage())
	}
	if !exists {
		observation := input.State.Player.CurrencyObservations[Intent.TravelTicketCurrencyID]
		gate.pending[key] = pendingCoinDebit{amount: 1, observedBalance: observed, observedAt: observation.ObservedAt, connectionGeneration: observation.ConnectionGeneration, reservedAt: time.Now().UTC()}
	}
	return nil
}
func (gate *travelTicketDispatchGate) DefinitiveFailure(ctx context.Context, step Intent.Step) {
	gate.mu.Lock()
	delete(gate.pending, coinDispatchKey(ctx, step))
	gate.mu.Unlock()
}
func (gate *travelTicketDispatchGate) Indeterminate(ctx context.Context, step Intent.Step) {
	gate.DefinitiveFailure(ctx, step)
}
func (gate *travelTicketDispatchGate) Completed(ctx context.Context, input Intent.PlanningContext, step Intent.Step, response Protocol.CommittedFrame) bool {
	gate.mu.Lock()
	defer gate.mu.Unlock()
	key := coinDispatchKey(ctx, step)
	debit, exists := gate.pending[key]
	if !exists {
		return false
	}
	debit.completedAt = time.Now().UTC()
	gate.pending[key] = debit
	observation, observed := input.State.Player.CurrencyObservations[Intent.TravelTicketCurrencyID]
	balance, known := Intent.ObservedCurrency(input.State, Intent.TravelTicketCurrencyID)
	correlated := response.ReduceError == "" && !response.Frame.ReceivedAt.IsZero() && observed && known &&
		observation.ConnectionGeneration == debit.connectionGeneration && observation.ObservedAt.Equal(response.Frame.ReceivedAt)
	if correlated {
		if observation.ConnectionGeneration > gate.watermark.ConnectionGeneration || !observation.ObservedAt.Before(gate.watermark.ObservedAt) {
			gate.watermark = observation
			gate.watermarkBalance = balance
		}
		delete(gate.pending, key)
	}
	return !correlated
}
func (gate *travelTicketDispatchGate) reconcile(observation State.PlayerResourceObservation, balance int64) {
	for key, debit := range gate.pending {
		if observation.ConnectionGeneration != debit.connectionGeneration {
			if observation.ConnectionGeneration > 0 && observation.ObservedAt.After(debit.reservedAt) {
				delete(gate.pending, key)
			}
			continue
		}
		// Successful commands wait for a snapshot ordered after completion.
		// Uncertain commands wait for a new snapshot ordered after the uncertain
		// outcome. A balance refresh between reservation and acknowledgement can
		// never release either debit.
		reconcileAfter := debit.completedAt
		if reconcileAfter.IsZero() {
			reconcileAfter = debit.indeterminateAt
		}
		if !reconcileAfter.IsZero() && observation.ObservedAt.After(reconcileAfter) {
			_ = balance // the authoritative post-dispatch snapshot is the new budget
			delete(gate.pending, key)
		}
	}
}

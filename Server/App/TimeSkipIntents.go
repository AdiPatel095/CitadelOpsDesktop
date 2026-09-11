package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	timeSkipConsumeAction      = "inventory.time_skip.consume"
	timeSkipReserveGuardAction = "inventory.time_skip.reserve_guard"
)

type timeSkipConsumeRequest struct {
	CurrencyID     State.CurrencyID `json:"currencyId"`
	ExpectedBefore float64          `json:"expectedBefore"`
}

type timeSkipReserveGuardRequest struct {
	CurrencyID       State.CurrencyID `json:"currencyId"`
	MinimumRemaining int64            `json:"minimumRemaining"`
}

func timeSkipConsumeStep(input Intent.PlanningContext, currencyID State.CurrencyID) Intent.Step {
	return timeSkipConsumeStepAtBalance(currencyID, input.State.Player.Currencies[currencyID])
}

func timeSkipConsumeStepAtBalance(currencyID State.CurrencyID, expectedBefore float64) Intent.Step {
	arguments, _ := json.Marshal(timeSkipConsumeRequest{
		CurrencyID: currencyID, ExpectedBefore: expectedBefore,
	})
	return Intent.Step{
		Name:   "Reconcile confirmed time-skip inventory",
		Action: timeSkipConsumeAction, ActionArguments: arguments,
	}
}

func (application *Application) guardTimeSkipReserve(_ context.Context, arguments json.RawMessage) error {
	var request timeSkipReserveGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("time-skip inventory state is unavailable")
	}
	if request.CurrencyID <= 0 || request.MinimumRemaining < 0 {
		return fmt.Errorf("time-skip reserve guard has invalid currency data")
	}
	balance := application.State.ReadOnlyView().Player.Currencies[request.CurrencyID]
	if math.IsNaN(balance) || math.IsInf(balance, 0) || math.Floor(balance) <= float64(request.MinimumRemaining) {
		return fmt.Errorf(
			"%w: currency %d is no longer available above its configured time-skip reserve",
			Intent.ErrPlanStale, request.CurrencyID,
		)
	}
	return nil
}

func (application *Application) consumeTimeSkip(_ context.Context, arguments json.RawMessage) error {
	var request timeSkipConsumeRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("time-skip inventory state is unavailable")
	}
	if request.CurrencyID <= 0 || request.ExpectedBefore < 1 {
		return fmt.Errorf("confirmed time skip has invalid currency data")
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(gameState *State.GameState) ([]string, bool, error) {
		current := gameState.Player.Currencies[request.CurrencyID]
		expectedAfter := request.ExpectedBefore - 1
		if current <= expectedAfter {
			return nil, false, nil
		}
		gameState.Player.Currencies[request.CurrencyID] = max(float64(0), current-1)
		return []string{"currencies"}, true, nil
	})
	return err
}

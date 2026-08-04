package App

import (
	"context"
	"encoding/json"
	"fmt"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const timeSkipConsumeAction = "inventory.time_skip.consume"

type timeSkipConsumeRequest struct {
	CurrencyID     State.CurrencyID `json:"currencyId"`
	ExpectedBefore float64          `json:"expectedBefore"`
}

func timeSkipConsumeStep(input Intent.PlanningContext, currencyID State.CurrencyID) Intent.Step {
	arguments, _ := json.Marshal(timeSkipConsumeRequest{
		CurrencyID: currencyID, ExpectedBefore: input.State.Player.Currencies[currencyID],
	})
	return Intent.Step{
		Name:   "Reconcile confirmed time-skip inventory",
		Action: timeSkipConsumeAction, ActionArguments: arguments,
	}
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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

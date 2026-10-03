package Intent

import (
	"errors"
	"fmt"
	"math"
	"strings"

	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

const TravelTicketCurrencyID = State.CurrencyID(22)

var ErrCurrencyUnavailable = errors.New("currency unavailable for dispatch")

// CurrencyAvailabilityProvider includes process-local, unsettled dispatch debits.
// It never grants authority to a persisted or previous-session balance.
type CurrencyAvailabilityProvider interface {
	AvailableCurrency(State.GameState, State.CurrencyID) (observed, pending int64, known bool)
}

func ObservedCurrency(state State.GameState, id State.CurrencyID) (int64, bool) {
	observation, exists := state.Player.CurrencyObservations[id]
	balance, present := state.Player.Currencies[id]
	if !exists || !present || state.Session.ConnectionGeneration == 0 || observation.ObservedAt.IsZero() ||
		observation.ConnectionGeneration != state.Session.ConnectionGeneration || balance < 0 || math.IsNaN(balance) || math.IsInf(balance, 0) || balance >= math.Exp2(63) {
		return 0, false
	}
	return int64(math.Floor(balance)), true
}

func TravelTicketBudget(state State.GameState, provider CurrencyAvailabilityProvider) (observed, pending int64, known bool) {
	if provider != nil {
		return provider.AvailableCurrency(state, TravelTicketCurrencyID)
	}
	observed, known = ObservedCurrency(state, TravelTicketCurrencyID)
	return
}

type CurrencyUnavailableError struct {
	CurrencyID                  State.CurrencyID
	Required, Observed, Pending int64
	Known                       bool
}

func (err *CurrencyUnavailableError) Error() string {
	detail := fmt.Sprintf("not enough travel tickets for dispatch: %d needed; %d available from %d observed after %d pending (currency %d)", err.Required, max(int64(0), err.Observed-err.Pending), err.Observed, err.Pending, err.CurrencyID)
	if !err.Known {
		detail += ": current-session balance unavailable"
	}
	return detail
}
func (err *CurrencyUnavailableError) Unwrap() error { return ErrCurrencyUnavailable }
func (err *CurrencyUnavailableError) LocalizationMessage() *Localization.Message {
	if !err.Known {
		return Localization.New("server.travel_tickets.unavailable", "Travel ticket balance is unavailable; waiting for fresh game data", nil)
	}
	return Localization.New("server.travel_tickets.short", "Not enough travel tickets: {needed} needed, {available} available", Localization.Params{"needed": err.Required, "available": max(int64(0), err.Observed-err.Pending)})
}

func RequireTravelTickets(input PlanningContext, needed int64) error {
	observed, pending, known := TravelTicketBudget(input.State, input.CurrencyAvailability)
	if !known || needed > max(int64(0), observed-pending) {
		err := &CurrencyUnavailableError{CurrencyID: TravelTicketCurrencyID, Required: needed, Observed: observed, Pending: pending, Known: known}
		return Localization.WithError(err, err.LocalizationMessage())
	}
	return nil
}

const SupportCoinHorseSource = "auto support coin-horse reserve"

func (err *CoinUnavailableError) LocalizationMessage() *Localization.Message {
	if !strings.Contains(err.Source, SupportCoinHorseSource) {
		return nil
	}
	if err.BalanceUnavailable {
		message := Localization.New("server.support.coin_horse_unavailable", "Waiting for a coin horse: coin balance is unavailable; waiting for fresh game data", nil)
		return Localization.Bind(message, message.Fallback)
	}
	available := max(int64(0), err.Observed-err.Pending)
	return Localization.Bind(Localization.New("server.support.coin_horse_short", "Waiting: not enough coins for a coin horse ({needed} needed, {available} available)", Localization.Params{"needed": err.Required, "available": available}), fmt.Sprintf("Waiting: not enough coins for a coin horse (%d needed, %d available)", err.Required, available))
}

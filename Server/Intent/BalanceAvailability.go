package Intent

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"

	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

type BalanceKind string

const (
	BalanceCurrency       BalanceKind = "currency"
	BalancePlayerResource BalanceKind = "player_resource"
	BalanceCastleResource BalanceKind = "castle_resource"
)

type BalanceKey struct {
	Kind     BalanceKind
	ID       int64
	CastleID State.CastleID
}

func CurrencyBalanceKey(id State.CurrencyID) BalanceKey {
	return BalanceKey{Kind: BalanceCurrency, ID: int64(id)}
}
func PlayerResourceBalanceKey(id State.ResourceID) BalanceKey {
	return BalanceKey{Kind: BalancePlayerResource, ID: int64(id)}
}
func CastleResourceBalanceKey(castle State.CastleID, resource State.ResourceID) BalanceKey {
	return BalanceKey{Kind: BalanceCastleResource, ID: int64(resource), CastleID: castle}
}
func (key BalanceKey) Valid() bool {
	if key.ID <= 0 {
		return false
	}
	switch key.Kind {
	case BalanceCurrency, BalancePlayerResource:
		return key.CastleID == 0
	case BalanceCastleResource:
		return key.CastleID > 0
	}
	return false
}
func (key BalanceKey) String() string {
	text := string(key.Kind) + ":" + strconv.FormatInt(key.ID, 10)
	if key.Kind == BalanceCastleResource {
		text += "@" + strconv.FormatInt(int64(key.CastleID), 10)
	}
	return text
}
func ParseBalanceKey(text string) (BalanceKey, error) {
	kind, ids, ok := strings.Cut(text, ":")
	key := BalanceKey{Kind: BalanceKind(kind)}
	id, castle, hasCastle := strings.Cut(ids, "@")
	var err error
	key.ID, err = strconv.ParseInt(id, 10, 64)
	if !ok || err != nil {
		return BalanceKey{}, fmt.Errorf("invalid balance key %q", text)
	}
	if hasCastle {
		value, parseErr := strconv.ParseInt(castle, 10, 64)
		if parseErr != nil {
			return BalanceKey{}, fmt.Errorf("invalid balance key %q", text)
		}
		key.CastleID = State.CastleID(value)
	}
	if !key.Valid() || key.String() != text {
		return BalanceKey{}, fmt.Errorf("invalid balance key %q", text)
	}
	return key, nil
}
func ObservedBalance(state State.GameState, key BalanceKey) (amount int64, observation State.PlayerResourceObservation, known bool) {
	if !key.Valid() {
		return
	}
	if key.Kind == BalanceCurrency {
		amount, known = ObservedCurrency(state, State.CurrencyID(key.ID))
		observation = state.Player.CurrencyObservations[State.CurrencyID(key.ID)]
		return
	}
	var balance float64
	var present bool
	switch key.Kind {
	case BalancePlayerResource:
		observation, known = state.Player.ResourceObservations[State.ResourceID(key.ID)]
		balance, present = state.Player.Resources[State.ResourceID(key.ID)]
		known = known && state.Session.ConnectionGeneration > 0 && !observation.ObservedAt.IsZero() && observation.ConnectionGeneration == state.Session.ConnectionGeneration
	case BalanceCastleResource:
		castle, exists := state.Castles[key.CastleID]
		value, found := castle.Resources[State.ResourceID(key.ID)]
		balance, present = value.Amount, found
		observation = State.PlayerResourceObservation{ObservedAt: castle.ContextSnapshotObservedAt, ConnectionGeneration: state.Session.ConnectionGeneration}
		known = exists && state.Session.ConnectionGeneration > 0 && !state.Session.ChangedAt.IsZero() && !observation.ObservedAt.IsZero() && !observation.ObservedAt.Before(state.Session.ChangedAt)
	}
	known = known && present && balance >= 0 && !math.IsNaN(balance) && !math.IsInf(balance, 0) && balance < math.Exp2(63)
	if known {
		amount = int64(math.Floor(balance))
	}
	return
}

var ErrBalanceUnavailable = errors.New("not enough balance for dispatch")

type BalanceUnavailableError struct {
	Key                         BalanceKey
	Required, Observed, Pending int64
	Known                       bool
	Name, NameKey, CastleName   string
}

func (err *BalanceUnavailableError) Error() string {
	text := fmt.Sprintf("%s: %s %d needed; %d available from %d observed after %d pending", ErrBalanceUnavailable, err.Key, err.Required, max(int64(0), err.Observed-err.Pending), err.Observed, err.Pending)
	if !err.Known {
		text += ": current-session balance unavailable"
	}
	return text
}
func (err *BalanceUnavailableError) Unwrap() error { return ErrBalanceUnavailable }
func (err *BalanceUnavailableError) Detail() string {
	if err.Key.Kind == BalanceCastleResource {
		if !err.Known {
			return fmt.Sprintf("Waiting: %s in %s is unavailable; waiting for a fresh castle snapshot", err.Name, err.CastleName)
		}
		return fmt.Sprintf("Waiting: not enough %s in %s (%d needed, %d available)", err.Name, err.CastleName, err.Required, max(int64(0), err.Observed-err.Pending))
	}
	if !err.Known {
		return fmt.Sprintf("Waiting: the %s balance is unavailable; waiting for fresh game data", err.Name)
	}
	return fmt.Sprintf("Waiting: not enough %s (%d needed, %d available)", err.Name, err.Required, max(int64(0), err.Observed-err.Pending))
}
func (err *BalanceUnavailableError) LocalizationMessage() *Localization.Message {
	params := Localization.Params{"balance": err.Name, "needed": err.Required, "available": max(int64(0), err.Observed-err.Pending), "castle": err.CastleName}
	var message *Localization.Message
	if err.Key.Kind == BalanceCastleResource {
		if err.Known {
			message = Localization.New("server.balance.castle_short", "Waiting: not enough {balance} in {castle} ({needed} needed, {available} available)", params)
		} else {
			message = Localization.New("server.balance.castle_unavailable", "Waiting: {balance} in {castle} is unavailable; waiting for a fresh castle snapshot", params)
		}
	} else if err.Known {
		message = Localization.New("server.balance.short", "Waiting: not enough {balance} ({needed} needed, {available} available)", params)
	} else {
		message = Localization.New("server.balance.unavailable", "Waiting: the {balance} balance is unavailable; waiting for fresh game data", params)
	}
	return Localization.Bind(message.WithGameParam("balance", err.NameKey, err.Name), err.Detail())
}

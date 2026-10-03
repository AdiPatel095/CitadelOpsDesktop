package Intent

import (
	"errors"
	"fmt"
	"math"
	"testing"
	"time"

	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
)

func TestBalanceKeyCanonicalRoundTrip(t *testing.T) {
	for _, key := range []BalanceKey{CurrencyBalanceKey(69), PlayerResourceBalanceKey(3), CastleResourceBalanceKey(77118, 1)} {
		got, err := ParseBalanceKey(key.String())
		if err != nil || got != key || !key.Valid() {
			t.Fatalf("%v -> %v: %v", key, got, err)
		}
	}
	for _, text := range []string{"", "currency:0", "currency:-1", "currency:+1", "currency:01", "currency:1@0", "currency:1@4", "currency:1 ", " currency:1", "currency:1:2", "player_resource:1@3", "castle_resource:1", "castle_resource:1@0", "castle_resource:1@-2", "castle_resource:1@03", "castle_resource:1@3@4", "foo:1", "currency:9223372036854775808", "castle_resource:1@9223372036854775808"} {
		if key, err := ParseBalanceKey(text); err == nil || key.Valid() {
			t.Errorf("accepted %q: %v", text, key)
		}
	}
}
func balanceTestState() State.GameState {
	state := State.NewGameState()
	now := time.Now().UTC()
	state.Session.ConnectionGeneration = 7
	state.Session.ChangedAt = now.Add(-time.Minute)
	state.Player.Currencies[69] = 300.9
	state.Player.CurrencyObservations[69] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	state.Player.Resources[3] = 300.9
	state.Player.ResourceObservations[3] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
	state.Castles[77118] = State.CastleState{ID: 77118, ContextSnapshotObservedAt: now, Resources: map[State.ResourceID]State.ResourceBalance{1: {Amount: 300.9}}}
	return state
}
func TestObservedBalanceAuthority(t *testing.T) {
	keys := []BalanceKey{CurrencyBalanceKey(69), PlayerResourceBalanceKey(3), CastleResourceBalanceKey(77118, 1)}
	for _, key := range keys {
		t.Run(key.String(), func(t *testing.T) {
			state := balanceTestState()
			amount, observation, known := ObservedBalance(state, key)
			if !known || amount != 300 || observation.ConnectionGeneration != 7 || observation.ObservedAt.IsZero() {
				t.Fatalf("valid balance %d %v %v", amount, observation, known)
			}
			for _, value := range []float64{-1, math.NaN(), math.Inf(1), math.Exp2(63)} {
				state = balanceTestState()
				switch key.Kind {
				case BalanceCurrency:
					state.Player.Currencies[69] = value
				case BalancePlayerResource:
					state.Player.Resources[3] = value
				case BalanceCastleResource:
					c := state.Castles[77118]
					c.Resources[1] = State.ResourceBalance{Amount: value}
					state.Castles[77118] = c
				}
				if _, _, known := ObservedBalance(state, key); known {
					t.Errorf("accepted %v", value)
				}
			}
			for _, change := range []string{"missing", "zero_stamp", "wrong_generation", "zero_generation"} {
				state = balanceTestState()
				switch change {
				case "zero_generation":
					state.Session.ConnectionGeneration = 0
				default:
					switch key.Kind {
					case BalanceCurrency:
						if change == "missing" {
							delete(state.Player.Currencies, 69)
						} else {
							o := state.Player.CurrencyObservations[69]
							if change == "zero_stamp" {
								o.ObservedAt = time.Time{}
							} else {
								o.ConnectionGeneration = 6
							}
							state.Player.CurrencyObservations[69] = o
						}
					case BalancePlayerResource:
						if change == "missing" {
							delete(state.Player.Resources, 3)
						} else {
							o := state.Player.ResourceObservations[3]
							if change == "zero_stamp" {
								o.ObservedAt = time.Time{}
							} else {
								o.ConnectionGeneration = 6
							}
							state.Player.ResourceObservations[3] = o
						}
					case BalanceCastleResource:
						c := state.Castles[77118]
						if change == "missing" {
							delete(c.Resources, 1)
						} else if change == "zero_stamp" {
							c.ContextSnapshotObservedAt = time.Time{}
						} else {
							c.ContextSnapshotObservedAt = state.Session.ChangedAt.Add(-time.Second)
						}
						state.Castles[77118] = c
					}
				}
				if _, _, known := ObservedBalance(state, key); known {
					t.Errorf("accepted %s", change)
				}
			}
		})
	}
	state := balanceTestState()
	state.Session.ChangedAt = time.Time{}
	if _, _, known := ObservedBalance(state, keys[2]); known {
		t.Fatal("castle authority without ChangedAt")
	}
	state = balanceTestState()
	delete(state.Castles, 77118)
	if _, _, known := ObservedBalance(state, keys[2]); known {
		t.Fatal("missing castle")
	}
	state = balanceTestState()
	delete(state.Player.CurrencyObservations, 69)
	if _, _, known := ObservedBalance(state, keys[0]); known {
		t.Fatal("unobserved currency")
	}
	state = balanceTestState()
	delete(state.Player.ResourceObservations, 3)
	if _, _, known := ObservedBalance(state, keys[1]); known {
		t.Fatal("unobserved resource")
	}
	a, k := ObservedCurrency(state, 69)
	b, _, l := ObservedBalance(state, keys[0])
	if a != b || k != l {
		t.Fatal("currency contract drift")
	}
	if _, _, known := ObservedBalance(state, BalanceKey{}); known {
		t.Fatal("zero key")
	}
}
func TestBalanceUnavailableGoldenAndPresentation(t *testing.T) {
	for _, key := range []BalanceKey{CurrencyBalanceKey(69), CastleResourceBalanceKey(77118, 1)} {
		for _, known := range []bool{true, false} {
			shortage := &BalanceUnavailableError{Key: key, Required: 2, Observed: 1, Pending: 3, Known: known, Name: "Arrows", NameKey: "currency_name_arrows", CastleName: "Sample keep"}
			expected := fmt.Sprintf("not enough balance for dispatch: %s 2 needed; 0 available from 1 observed after 3 pending", key)
			if !known {
				expected += ": current-session balance unavailable"
			}
			if shortage.Error() != expected {
				t.Fatal(shortage.Error())
			}
			wrapped := fmt.Errorf("wrapped: %w", shortage)
			if !errors.Is(wrapped, ErrBalanceUnavailable) || errors.Is(wrapped, ErrCurrencyUnavailable) || errors.Is(wrapped, ErrCoinUnavailable) {
				t.Fatal("sentinel overlap")
			}
			for _, actor := range []string{"ui", "automation:autoRecruit"} {
				receipt := (&Engine{}).withFailure(Receipt{Actor: actor, Status: StatusFailed}, Localization.WithError(wrapped, shortage.LocalizationMessage()))
				f := receipt.Failure
				if f == nil || f.Kind != FailureAvailability || f.Severity != FailureSeverityWarning || f.Toast != (actor == "ui") || f.Explanation != shortage.Detail() || f.ExplanationDescriptor == nil || f.RecoveryDescriptor.Key != "server.balance.recovery" || f.SafetyLock != nil {
					t.Fatalf("presentation: %+v", receipt)
				}
			}
			shortage.NameKey = ""
			if shortage.LocalizationMessage() != nil {
				t.Fatal("descriptor without name authority")
			}
		}
	}
}

func TestCIT119FeatherWaitUnchanged(t *testing.T) {
	for _, known := range []bool{true, false} {
		shortage := &CurrencyUnavailableError{CurrencyID: 22, Required: 1, Known: known}
		receipt := (&Engine{}).withFailure(Receipt{Actor: "automation:autoBird", Status: StatusFailed}, shortage)
		key := "server.travel_tickets.short"
		detail := "Not enough travel tickets: 1 needed, 0 available"
		if !known {
			key = "server.travel_tickets.unavailable"
			detail = "Travel ticket balance is unavailable; waiting for fresh game data"
		}
		f := receipt.Failure
		if f == nil || f.Explanation != detail || f.ExplanationDescriptor.Key != key || f.RecoveryDescriptor.Key != "server.travel_tickets.recovery" || f.Toast || f.SafetyLock != nil {
			t.Fatalf("ticket presentation changed %+v", f)
		}
	}
}

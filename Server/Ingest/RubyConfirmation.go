package Ingest

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
	"time"
)

func decodeRubyConfirmation(raw json.RawMessage, generation uint64, observedAt time.Time) State.RubyConfirmationState {
	result := State.RubyConfirmationState{Generation: generation}
	var value struct {
		Amount *int64 `json:"CC2T"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Amount != nil && State.ValidRubyConfirmationAmount(*value.Amount) {
		result.Amount, result.Known, result.ObservedAt = *value.Amount, true, observedAt.UTC()
	}
	return result
}

func reduceRubyConfirmation(_ context.Context, frame Protocol.Frame, state *State.GameState, _ *GameData.Store) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	next := decodeRubyConfirmation(frame.Payload, state.Session.Generation, frame.ReceivedAt)
	wake := rubyConfirmationWake(state, next)
	changed := next != state.Player.RubyConfirmation
	state.Player.RubyConfirmation = next
	if wake {
		return []string{"ruby-confirmation"}, changed, nil
	}
	return nil, changed, nil
}

func rubyConfirmationWake(state *State.GameState, next State.RubyConfirmationState) bool {
	previous := state.Player.RubyConfirmation
	if previous.Amount != next.Amount || previous.Known != next.Known || previous.Generation != next.Generation {
		return true
	}
	for _, record := range state.EventScores.Inventory.GlobalEffectPurchases {
		effect, found := state.EventScores.Inventory.GlobalEffects[record.GlobalEffectID]
		if found && State.SameEventOccurrence(record.OccurrenceEndsAt, effect.EndsAt) && record.Outcome == State.GlobalEffectPurchaseConfirmationRequired && next.Known && next.ObservedAt.After(record.ResultObservedAt) {
			return true
		}
	}
	return false
}

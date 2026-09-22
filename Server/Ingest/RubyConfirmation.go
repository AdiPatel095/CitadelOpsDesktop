package Ingest

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
	"encoding/json"
)

func decodeRubyConfirmation(raw json.RawMessage, generation uint64) State.RubyConfirmationState {
	result := State.RubyConfirmationState{Generation: generation}
	var value struct {
		Amount *int64 `json:"CC2T"`
	}
	if json.Unmarshal(raw, &value) == nil && value.Amount != nil && (*value.Amount == -1 || *value.Amount > 0) {
		result.Amount, result.Known = *value.Amount, true
	}
	return result
}

func reduceRubyConfirmation(_ context.Context, frame Protocol.Frame, state *State.GameState, _ *GameData.Store) ([]string, bool, error) {
	if !frameSucceeded(frame) {
		return nil, false, nil
	}
	next := decodeRubyConfirmation(frame.Payload, state.Session.Generation)
	changed := next != state.Player.RubyConfirmation
	state.Player.RubyConfirmation = next
	return []string{"ruby-confirmation"}, changed, nil
}

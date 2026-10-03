package Ingest

import (
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
	"context"
)

func reduceMarketBarrowLeases(_ context.Context, frame Protocol.Frame, gs *State.GameState, _ *GameData.Store) ([]string, bool, error) {
	if State.RecordMarketBarrowLeases(gs, frame.ReceivedAt) {
		return []string{"market"}, true, nil
	}
	return nil, false, nil
}

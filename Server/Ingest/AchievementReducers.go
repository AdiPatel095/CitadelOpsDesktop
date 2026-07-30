package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type achievementProgressWire struct {
	AchievementID wireInt64   `json:"AID"`
	Progress      []wireInt64 `json:"P"`
}

func reduceAchievementSnapshot(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if !frameSucceeded(frame) || len(frame.Payload) == 0 {
		return nil, false, nil
	}
	changed, err := applyAchievementSnapshot(frame.Payload, frame.ReceivedAt, gameState)
	return []string{"player", "achievements"}, changed, err
}

func applyAchievementSnapshot(raw json.RawMessage, observedAt time.Time, gameState *State.GameState) (bool, error) {
	var payload struct {
		Points   wireInt64                 `json:"AVP"`
		Running  []achievementProgressWire `json:"RA"`
		Finished []wireInt64               `json:"FA"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return false, fmt.Errorf("decode achievement snapshot: %w", err)
	}

	completed := make(map[int64]bool, len(payload.Finished))
	for _, achievementID := range payload.Finished {
		if achievementID > 0 {
			completed[int64(achievementID)] = true
		}
	}
	progress := make(map[int64][]int64, len(payload.Running))
	for _, running := range payload.Running {
		if running.AchievementID <= 0 {
			continue
		}
		values := make([]int64, len(running.Progress))
		for index, value := range running.Progress {
			values[index] = int64(value)
		}
		progress[int64(running.AchievementID)] = values
	}

	next := State.AchievementState{
		Points: int64(payload.Points), Completed: completed, Progress: progress, ObservedAt: observedAt,
	}
	if reflect.DeepEqual(gameState.Player.Achievements, next) {
		return false, nil
	}
	gameState.Player.Achievements = next
	return true, nil
}

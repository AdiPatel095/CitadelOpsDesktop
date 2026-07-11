package gamestate

import (
	"CitadelDesktop/Server/Models/Movement"
	"time"
)

// MovementUpdatePayload builds the JSON-safe map for WebSocket movementUpdate messages.
func MovementUpdatePayload() map[string]interface{} {
	gs := GetGameState()
	status := gs.Movement.StatusSnapshot(time.Now().Unix())
	movements := status.ActiveMovements
	if movements == nil {
		movements = []movement.GAMMovement{}
	}
	commanders := status.CommanderStatuses
	if commanders == nil {
		commanders = []movement.CommanderStatusRow{}
	}
	return map[string]interface{}{
		"activeMovements":    movements,
		"commanderStatuses":  commanders,
		"snapshotReady":      status.SnapshotReady,
		"snapshotFresh":      status.SnapshotFresh,
		"lastSnapshotUnix":   status.LastSnapshotUnix,
		"freshnessWindowSec": movement.CommanderSnapshotFreshnessSeconds,
	}
}

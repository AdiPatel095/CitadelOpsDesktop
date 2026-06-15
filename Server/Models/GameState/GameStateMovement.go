package gamestate

import (
	"CitadelDesktop/Server/Models/Movement"
)

// MovementUpdatePayload builds the JSON-safe map for WebSocket movementUpdate messages.
func MovementUpdatePayload() map[string]interface{} {
	gs := GetGameState()
	movements := gs.Movement.ActiveMovements
	if movements == nil {
		movements = []movement.GAMMovement{}
	}
	return map[string]interface{}{
		"activeMovements": movements,
	}
}

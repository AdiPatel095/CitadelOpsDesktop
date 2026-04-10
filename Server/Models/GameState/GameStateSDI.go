package gamestate

import "CitadelDesktop/Server/Models/Movement"

// SetLastSDI updates the last SDI context for a source castle.
func (gs *GameState) SetLastSDI(sourceCastleID int, lid int, receivedUnixNs int64) {
	if gs.Movement.LastSDI == nil {
		gs.Movement.LastSDI = make(map[int]movement.SDIContext)
	}
	gs.Movement.LastSDI[sourceCastleID] = movement.SDIContext{
		LID:          lid,
		ReceivedUnix: receivedUnixNs,
	}
}

// GetLastSDI returns the last SDI context for a source castle.
func (gs *GameState) GetLastSDI(sourceCastleID int) (movement.SDIContext, bool) {
	if gs.Movement.LastSDI == nil {
		return movement.SDIContext{}, false
	}
	ctx, ok := gs.Movement.LastSDI[sourceCastleID]
	return ctx, ok
}

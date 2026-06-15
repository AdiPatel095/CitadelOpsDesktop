package gamestate

// EvaluateCommanderWireBusy is wired by GameParser to classify live GAM legs (outbound, siege, return).
var EvaluateCommanderWireBusy func(gs *GameState, commanderWireID int) bool

// IsCommanderWireIDBusy reports whether wire LID is on an in-flight attack (outbound, at target, or return).
// Busy state is derived entirely from live GAM legs via EvaluateCommanderWireBusy (wired by GameParser).
func (gs *GameState) IsCommanderWireIDBusy(commanderWireID int) bool {
	if gs == nil || commanderWireID < 0 {
		return false
	}
	if EvaluateCommanderWireBusy != nil {
		return EvaluateCommanderWireBusy(gs, commanderWireID)
	}
	return false
}

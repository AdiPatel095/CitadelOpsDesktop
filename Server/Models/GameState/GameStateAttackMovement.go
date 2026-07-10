package gamestate

// EvaluateCommanderWireBusy is wired by GameParser to classify live GAM legs (outbound, siege, return).
var EvaluateCommanderWireBusy func(gs *GameState, commanderWireID int) bool

// IsCommanderWireIDBusy reports whether wire LID is unavailable for another attack. It fails closed
// until GameParser wires the evaluator and a fresh GAM snapshot proves the commander free.
func (gs *GameState) IsCommanderWireIDBusy(commanderWireID int) bool {
	if gs == nil || commanderWireID < 0 {
		return true
	}
	if EvaluateCommanderWireBusy != nil {
		return EvaluateCommanderWireBusy(gs, commanderWireID)
	}
	return true
}

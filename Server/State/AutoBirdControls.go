package State

import (
	"strconv"
	"time"
)

func AutoBirdControlID(id CastleID) string {
	return "autoBirdControl:" + strconv.FormatInt(int64(id), 10)
}
func (state GameState) AutoBirdControl(id CastleID) StationingOperation {
	return state.Stationing[AutoBirdControlID(id)]
}
func (state GameState) AutoBirdPaused(id CastleID, now time.Time) bool {
	control := state.AutoBirdControl(id)
	return control.Paused && (control.PausedUntil == nil || control.PausedUntil.After(now))
}

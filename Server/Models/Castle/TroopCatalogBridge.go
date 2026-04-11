package castle

import gamedata "CitadelDesktop/Server/GameData"

// TroopInfo and troop/tool ID maps live in GameData (fixed catalog).
type TroopInfo = gamedata.TroopInfo

var (
	TroopIDs = gamedata.TroopIDs
	ToolIDs  = gamedata.ToolIDs
)

func IsTroop(unitID int) bool { return gamedata.IsTroop(unitID) }
func IsTool(unitID int) bool  { return gamedata.IsTool(unitID) }

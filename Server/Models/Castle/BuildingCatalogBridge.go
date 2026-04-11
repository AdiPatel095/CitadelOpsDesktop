package castle

import gamedata "CitadelDesktop/Server/GameData"

// BuildingInfo and BuildingIDMap live in GameData (fixed catalog).
type BuildingInfo = gamedata.BuildingInfo

var BuildingIDMap = gamedata.BuildingIDMap

func GetBuildingInfo(id int) BuildingInfo { return gamedata.GetBuildingInfo(id) }

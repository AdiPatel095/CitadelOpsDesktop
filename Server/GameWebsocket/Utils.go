package GameWebsocket

import (
	"CitadelDesktop/Server/Models"
)

// GetCommanderID returns the actual Commander ID (GUID) from the global model array.
// Returns 0 if the index is out of bounds.
func GetCommanderID(targetIndex int) float64 {
	if targetIndex >= 0 && targetIndex < len(Models.CommActualArray) {
		return Models.CommActualArray[targetIndex].ID
	}
	return 0
}

func GetCastellanID(targetIndex int) float64 {
	switch targetIndex {
	case 0:
		return Models.CastActualArray.MainCastleCast.ID
	case 1:
		return Models.CastActualArray.Outpost1Cast.ID
	case 2:
		return Models.CastActualArray.Outpost2Cast.ID
	case 3:
		return Models.CastActualArray.Outpost3Cast.ID
	case 4:
		return Models.CastActualArray.IceCastleCast.ID
	case 5:
		return Models.CastActualArray.DesertCastleCast.ID
	case 6:
		return Models.CastActualArray.DungeonCastleCast.ID
	case 7:
		return Models.CastActualArray.StormCastleCast.ID
	default:
		return 0
	}
}

// GetCastellanStat returns the CastStatModel for the given castle index (0-7).
// Returns an empty model if index is invalid.
func GetCastellanStat(targetIndex int) Models.CastStatModel {
	switch targetIndex {
	case 0:
		return Models.CastStatArray.MainCastleCast
	case 1:
		return Models.CastStatArray.Outpost1Cast
	case 2:
		return Models.CastStatArray.Outpost2Cast
	case 3:
		return Models.CastStatArray.Outpost3Cast
	case 4:
		return Models.CastStatArray.IceCastleCast
	case 5:
		return Models.CastStatArray.DesertCastleCast
	case 6:
		return Models.CastStatArray.DungeonCastleCast
	case 7:
		return Models.CastStatArray.StormCastleCast
	default:
		return Models.CastStatModel{}
	}
}

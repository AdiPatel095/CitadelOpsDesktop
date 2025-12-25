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
	// targetIndex represents the logical castle index (0=Main, 1=Op1, etc.)
	// Find the CastActualModel in the slice with this CastlePosition using the mapped ID

	targetAid := getAidFromIndex(targetIndex)
	for _, cast := range Models.CastActualArray {
		if cast.CastleID == targetAid {
			return cast.ID
		}
	}
	return 0
}

// GetCastellanStat returns the CastStatModel for the given castle index (0-7).
// Returns an empty model if index is invalid.
func GetCastellanStat(targetIndex int) Models.CastStatModel {
	// targetIndex is 0..7
	// We Iterate slice to find the one with CastlePosition == targetIndex

	for _, cast := range Models.CastStatArray {
		if cast.CastlePosition == targetIndex {
			return cast
		}
	}
	return Models.CastStatModel{}
}

func getAidFromIndex(index int) float64 {
	switch index {
	case 0:
		return Models.MainCastleResources.Aid
	case 1:
		return Models.Outpost1Resources.Aid
	case 2:
		return Models.Outpost2Resources.Aid
	case 3:
		return Models.Outpost3Resources.Aid
	case 4:
		return Models.IceCastleResources.Aid
	case 5:
		return Models.DesertCastleResources.Aid
	case 6:
		return Models.DungeonCastleResources.Aid
	case 7:
		return Models.StormCastleResources.Aid
	default:
		return -1
	}
}

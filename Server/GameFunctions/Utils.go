package GameFunctions

import (
	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/ResponseRegistry"
	"fmt"
)

// GetCommanderID returns the actual Commander ID (GUID) from the global model array.
// Returns 0 if the index is out of bounds.
func GetCommanderID(targetIndex int) float64 {
	gs := Models.GetGameState()
	if targetIndex >= 0 && targetIndex < len(gs.CommActualArray) {
		return gs.CommActualArray[targetIndex].ID
	}
	return 0
}

func GetCastellanID(targetIndex int) float64 {
	// targetIndex represents the logical castle index (0=Main, 1=Op1, etc.)
	// Find the CastActualModel in the slice with this CastlePosition using the mapped ID
	gs := Models.GetGameState()
	targetAid := getAidFromIndex(targetIndex)
	for _, cast := range gs.CastActualArray {
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
	gs := Models.GetGameState()
	switch index {
	case 0:
		return gs.MainCastle.Aid
	case 1:
		return gs.Outpost1.Aid
	case 2:
		return gs.Outpost2.Aid
	case 3:
		return gs.Outpost3.Aid
	case 4:
		return gs.IceCastle.Aid
	case 5:
		return gs.DesertCastle.Aid
	case 6:
		return gs.DungeonCastle.Aid
	case 7:
		return gs.StormCastle.Aid
	case 8:
		return gs.BeriWorldCastle.Aid
	default:
		return -1
	}
}

// FetchAllianceInfo sends the AIN command to fetch full alliance info using the stored AID
func FetchAllianceInfo() {
	aid := Models.GetGameState().Alliance.AID
	if aid == 0 {
		return
	}
	payload := fmt.Sprintf(`%%xt%%EmpireEx_21%%ain%%1%%{"AID":%d}%%`, aid)
	ResponseRegistry.OutgoingMessages <- []byte(payload)
}

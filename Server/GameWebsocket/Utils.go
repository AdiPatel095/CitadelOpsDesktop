package GameWebsocket

import (
	"CitadelDesktop/Server/Channels"
	"CitadelDesktop/Server/Models"
	equip "CitadelDesktop/Server/Models/Equipment"
	"fmt"
)

// GetCommanderID returns the actual Commander ID (GUID) from the global model array.
// Returns 0 if the index is out of bounds.
func GetCommanderID(targetIndex int) float64 {
	gs := Models.GetGameState()
	if targetIndex >= 0 && targetIndex < len(gs.Equipment.CommActualArray) {
		return gs.Equipment.CommActualArray[targetIndex].ID
	}
	return 0
}

func GetCastellanID(targetIndex int) float64 {
	// targetIndex represents the logical castle index (0=Main, 1=Op1, etc.)
	// Find the CastActualModel in the slice with this CastlePosition using the mapped ID
	gs := Models.GetGameState()
	targetAid := getAidFromIndex(targetIndex)
	for _, cast := range gs.Equipment.CastActualArray {
		if cast.CastleID == targetAid {
			return cast.ID
		}
	}
	return 0
}

// GetCastellanStat returns the CastStatModel for the given castle index (0-10).
// Returns an empty model if index is invalid.
func GetCastellanStat(targetIndex int) Models.CastStatModel {
	for _, cast := range equip.CastStatArray {
		if cast.CastlePosition == targetIndex {
			return cast
		}
	}
	return Models.CastStatModel{}
}

func getAidFromIndex(index int) float64 {
	gs := Models.GetGameState()
	c := &gs.Castle
	switch index {
	case 0:
		return c.MainCastle.Aid
	case 1:
		return c.Outpost1.Aid
	case 2:
		return c.Outpost2.Aid
	case 3:
		return c.Outpost3.Aid
	case 4:
		return c.IceCastle.Aid
	case 5:
		return c.DesertCastle.Aid
	case 6:
		return c.DungeonCastle.Aid
	case 7:
		return c.StormCastle.Aid
	case 8:
		return c.BeriWorldCastle.Aid
	case 9:
		return c.Metropolis.Aid
	case 10:
		return c.Capital.Aid
	default:
		return -1
	}
}


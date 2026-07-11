package featureview

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/Models"
)

// FetchAllianceInfo sends the AIN command to fetch full alliance info using the stored AID.
func FetchAllianceInfo() {
	aid := Models.GetGameState().Alliance.AID
	if aid == 0 {
		return
	}
	GameCommands.QueueFeatureRefresh(Automation.OwnerManual, GameCommands.AINPayload(aid), nil)
}

package GameCommands

import (
	"CitadelDesktop/Server/Logging"

	castle "CitadelDesktop/Server/Models/Castle"
)

// BeriWorldKingdomID is the Berimond ("Beri") event-world kingdom id (KID 10).
const BeriWorldKingdomID = castle.BeriWorldKingdomID

// QueueAutoBeriWorldOutgoing enqueues a wire payload for the game client and mirrors it to the Auto Beri World log tab.
func QueueAutoBeriWorldOutgoing(payload string) {
	Logging.AppendAutoBeriWorldSendPayload(payload)
	QueueOutgoingPayload(payload)
}

// SendBeriFucTroopSpaceCheck sends **fuc** for the Beri castle CID.
func SendBeriFucTroopSpaceCheck(beriCastleCID int) {
	QueueAutoBeriWorldOutgoing(FUCPayload(beriCastleCID))
}

// SendBeriSeiFollowup sends **sei** after **fuc**.
func SendBeriSeiFollowup() {
	QueueAutoBeriWorldOutgoing(SEIPayload())
}

// SendBeriDclRefresh sends **dcl** with CD=0 after the troop-space sequence.
func SendBeriDclRefresh() {
	QueueAutoBeriWorldOutgoing(DCLRefreshPayload())
}

// SendBeriKutTransfer sends **kut** with troops bound for Beri world (TKID 10).
func SendBeriKutTransfer(scid, kutCID, unitWID, amount int) {
	troops := KUTTroopArrayJSON(unitWID, amount)
	QueueAutoBeriWorldOutgoing(KUTPayload(scid, kutCID, 0, BeriWorldKingdomID, troops))
}

// SendBeriMskSpeedup sends the fixed Beri march speed-up (**msk**).
func SendBeriMskSpeedup() {
	QueueAutoBeriWorldOutgoing(MSKPayload())
}

package GameCommands

import "CitadelDesktop/Server/Logging"

// QueueAutoTCIOutgoing enqueues a wire payload for the game client and mirrors it to the Auto TCI log tab.
func QueueAutoTCIOutgoing(payload string) {
	Logging.AppendAutoTCISendPayload(payload)
	QueueOutgoingPayload(payload)
}

// SendGIIAutoTCI requests construction-item inventory (**gii**).
func SendGIIAutoTCI() {
	QueueAutoTCIOutgoing(GIIPayload())
}

// SendRPCAutoTCI equips a construction item (**rpc**).
func SendRPCAutoTCI(oid, cid, slotID, m, kid, aid int) {
	QueueAutoTCIOutgoing(RPCPayload(oid, cid, slotID, m, kid, aid))
}

// SendUBCAutoTCI upgrades an equipped construction item (**ubc**).
func SendUBCAutoTCI(oid, suc, slotID, kid, aid, cid int) {
	QueueAutoTCIOutgoing(UBCPayload(oid, suc, slotID, kid, aid, cid))
}

// SendCastleFocusAutoTCI focuses the client on a castle (**jaa** or **jca**).
func SendCastleFocusAutoTCI(kingdomID, castleID, mapX, mapY int) {
	QueueAutoTCIOutgoing(CastleFocusCommand(kingdomID, castleID, mapX, mapY))
}

// SendAECAutoTCI opens the construction-item menu (**aec**) before trivial-shop commands.
func SendAECAutoTCI() {
	QueueAutoTCIOutgoing(AECPayload())
}

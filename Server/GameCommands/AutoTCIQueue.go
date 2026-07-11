package GameCommands

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Logging"
)

// QueueAutoTCIOutgoing enqueues a wire payload for the game client and mirrors it to the Auto TCI log tab.
func QueueAutoTCIOutgoing(payload string, leases ...commandLease) {
	Logging.AppendAutoTCISendPayload(payload)
	QueueFeaturePayload(Automation.OwnerAutoTCI, payload, firstCommandLease(leases))
}

// SendGIIAutoTCI requests construction-item inventory (**gii**).
func SendGIIAutoTCI(leases ...commandLease) {
	QueueAutoTCIOutgoing(GIIPayload(), leases...)
}

// SendRPCAutoTCI equips a construction item (**rpc**).
func SendRPCAutoTCI(oid, cid, slotID, m, kid, aid int, leases ...commandLease) {
	QueueAutoTCIOutgoing(RPCPayload(oid, cid, slotID, m, kid, aid), leases...)
}

// SendUBCAutoTCI upgrades an equipped construction item (**ubc**).
func SendUBCAutoTCI(oid, suc, slotID, kid, aid, cid int, leases ...commandLease) {
	QueueAutoTCIOutgoing(UBCPayload(oid, suc, slotID, kid, aid, cid), leases...)
}

// SendCastleFocusAutoTCI focuses the client on a castle (**jaa** or **jca**).
func SendCastleFocusAutoTCI(kingdomID, castleID, mapX, mapY int, leases ...commandLease) {
	QueueAutoTCIOutgoing(CastleFocusCommand(kingdomID, castleID, mapX, mapY), leases...)
}

// SendAECAutoTCI opens the construction-item menu (**aec**) before trivial-shop commands.
func SendAECAutoTCI(leases ...commandLease) {
	QueueAutoTCIOutgoing(AECPayload(), leases...)
}

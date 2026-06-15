package GameCommands

import "CitadelDesktop/Server/Logging"

// Default trivial construction-item (shop) **sbp** field values from live EmpireEx_21 captures
// (see SBPPayload / SendSBP). If the game changes offers, update or override with PL from gbc.
const (
	AutoTCITrivialSBPBuildType  = 0
	AutoTCITrivialSBPTypeID     = 116
	AutoTCITrivialSBPPC2        = -1
	AutoTCITrivialSBPBuildAux   = 0
	AutoTCITrivialSBPPower      = 0
	AutoTCITrivialSBPPO         = -1
	AutoTCITrivialGbcKidUnknown = 0
	AutoTCITrivialSbpAidGlobal  = -1
)

// AutoTCISendGbcTrivialCIPurchaseList opens the **gbc** trivial-CI product list for a castle.
// The wire field CID is the castle instance id (player castle AID), not a constructionItemID
// (see SendGBC and AGENTS.md gbc rules). Call before **sbp** when the list is empty.
func AutoTCISendGbcTrivialCIPurchaseList(castleInstanceAID, mapKingdomID int) {
	if castleInstanceAID <= 0 {
		Logging.AutoTCILog("buy gbc", "skip: invalid AID")
		return
	}
	Logging.AutoTCILogf("buy gbc", "CID(AID)=%d KID=%d", castleInstanceAID, mapKingdomID)
	SendAECAutoTCI()
	QueueAutoTCIOutgoing(GBCPayload(castleInstanceAID, mapKingdomID))
}

// AutoTCISendSbpTrivialCIPurchase sends **sbp** to buy from the list opened with
// [AutoTCISendGbcTrivialCIPurchaseList] (or an equivalent gbc that populated offers).
// productID and amount should come from gbc/PL and CidTrivialProduct when possible.
func AutoTCISendSbpTrivialCIPurchase(productID, amount, mapKingdomID, castleInstanceAID int) {
	if productID <= 0 || amount <= 0 {
		Logging.AutoTCILogf("buy sbp", "skip: invalid pid=%d amt=%d", productID, amount)
		return
	}
	Logging.AutoTCILogf("buy sbp", "PID=%d AMT=%d KID=%d AID=%d (TID=%d BT=%d)",
		productID, amount, mapKingdomID, castleInstanceAID, AutoTCITrivialSBPTypeID, AutoTCITrivialSBPBuildType)
	QueueAutoTCIOutgoing(SBPPayload(
		productID,
		AutoTCITrivialSBPBuildType,
		AutoTCITrivialSBPTypeID,
		amount,
		mapKingdomID,
		castleInstanceAID,
		AutoTCITrivialSBPPC2,
		AutoTCITrivialSBPBuildAux,
		AutoTCITrivialSBPPower,
		AutoTCITrivialSBPPO,
	))
}

// AutoTCISendSbpTrivialCIPurchaseWithPayload queues **sbp** with every field set explicitly. Use when
// you have a full product row (or non-default TID/BT) from a capture or gbc PL.
func AutoTCISendSbpTrivialCIPurchaseWithPayload(
	productID, buildType, typeID, amount,
	mapKingdomID, castleInstanceAID,
	pc2, buildAux, pwr, po int,
) {
	if productID <= 0 || amount <= 0 {
		Logging.AutoTCILogf("buy sbp", "skip (full): invalid pid=%d amt=%d", productID, amount)
		return
	}
	Logging.AutoTCILogf("buy sbp", "PID=%d BT=%d TID=%d AMT=%d KID=%d AID=%d",
		productID, buildType, typeID, amount, mapKingdomID, castleInstanceAID)
	QueueAutoTCIOutgoing(SBPPayload(
		productID,
		buildType,
		typeID,
		amount,
		mapKingdomID,
		castleInstanceAID,
		pc2,
		buildAux,
		pwr,
		po,
	))
}

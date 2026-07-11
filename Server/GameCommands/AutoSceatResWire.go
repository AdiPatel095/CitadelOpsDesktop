package GameCommands

import (
	"encoding/json"
	"fmt"
)

// CRINPayload requests all sovereign crafting queues and effective research entitlements.
func CRINPayload() string {
	return empireExFrame("crin", "{}")
}

// SendCRIN requests the current crafting snapshot.
func SendCRIN() {
	QueueOutgoingPayload(CRINPayload())
}

// CRSTPayload starts or queues one official crafting recipe.
func CRSTPayload(kingdomID, castleAID, buildingOID, power, recipeID int) string {
	return empireExFrame("crst", fmt.Sprintf(
		`{"KID":%d,"AID":%d,"OID":%d,"PWR":%d,"CRID":%d}`,
		kingdomID, castleAID, buildingOID, power, recipeID,
	))
}

// SendCRST starts or queues one crafting recipe.
func SendCRST(kingdomID, castleAID, buildingOID, power, recipeID int) {
	QueueOutgoingPayload(CRSTPayload(kingdomID, castleAID, buildingOID, power, recipeID))
}

// CRUNPayload rents one production or queue slot for seven days. slot is one-based.
func CRUNPayload(kingdomID, castleAID, buildingOID, slot int, slotType string) string {
	return empireExFrame("crun", fmt.Sprintf(
		`{"KID":%d,"AID":%d,"OID":%d,"S":[%d],"ST":%q}`,
		kingdomID, castleAID, buildingOID, slot, slotType,
	))
}

// SendCRUN rents a crafting slot.
func SendCRUN(kingdomID, castleAID, buildingOID, slot int, slotType string) {
	QueueOutgoingPayload(CRUNPayload(kingdomID, castleAID, buildingOID, slot, slotType))
}

// CRSKPayload completes one crafting slot for the official remaining-time ruby price. slot is zero-based.
func CRSKPayload(kingdomID, castleAID, buildingOID, slot int, slotType string, priceRubies int) string {
	return empireExFrame("crsk", fmt.Sprintf(
		`{"KID":%d,"AID":%d,"OID":%d,"S":%d,"ST":%q,"PC2":%d}`,
		kingdomID, castleAID, buildingOID, slot, slotType, priceRubies,
	))
}

// SendCRSK completes one crafting slot with rubies.
func SendCRSK(kingdomID, castleAID, buildingOID, slot int, slotType string, priceRubies int) {
	QueueOutgoingPayload(CRSKPayload(kingdomID, castleAID, buildingOID, slot, slotType, priceRubies))
}

// CMIPayload requests market resources plus total and available barrows for every kingdom.
func CMIPayload() string {
	return empireExFrame("cmi", `{"S":1,"KID":-1}`)
}

// SendCMI requests current market state for all owned castles.
func SendCMI() {
	QueueOutgoingPayload(CMIPayload())
}

// BOIPayload requests account booster state, including permanent caravan-overloader level ID 11.
func BOIPayload() string {
	return empireExFrame("boi", "{}")
}

// SendBOI requests current premium and permanent booster state.
func SendBOI() {
	QueueOutgoingPayload(BOIPayload())
}

// CRMPayload starts a same-kingdom market shipment without a paid horse or scheduled delay.
func CRMPayload(sourceKingdomID, sourceCastleAID, targetX, targetY int, resourceCode string, amount int) string {
	body, _ := json.Marshal(struct {
		KingdomID int             `json:"KID"`
		SourceID  int             `json:"SID"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		HorseWID  int             `json:"HBW"`
		Goods     [][]interface{} `json:"G"`
		PaidTime  int             `json:"PTT"`
		DelaySec  int             `json:"SD"`
	}{
		KingdomID: sourceKingdomID,
		SourceID:  sourceCastleAID,
		TargetX:   targetX,
		TargetY:   targetY,
		HorseWID:  -1,
		Goods:     [][]interface{}{{resourceCode, amount}},
	})
	return empireExFrame("crm", string(body))
}

// SendCRM starts a same-kingdom market shipment.
func SendCRM(sourceKingdomID, sourceCastleAID, targetX, targetY int, resourceCode string, amount int) {
	QueueOutgoingPayload(CRMPayload(sourceKingdomID, sourceCastleAID, targetX, targetY, resourceCode, amount))
}

// KPIPayload requests kingdom-resource transport unlocks and pending shipments.
func KPIPayload() string {
	return empireExFrame("kpi", "{}")
}

// SendKPI requests current kingdom transport state.
func SendKPI() {
	QueueOutgoingPayload(KPIPayload())
}

// KGTPayload sends one kingdom resource from a source castle to a target kingdom.
func KGTPayload(sourceCastleAID, sourceKingdomID, targetKingdomID int, resourceCode string, amount int) string {
	body, _ := json.Marshal(map[string]interface{}{
		"SCID": sourceCastleAID,
		"SKID": sourceKingdomID,
		"TKID": targetKingdomID,
		"G":    [][]interface{}{{resourceCode, amount}},
	})
	return empireExFrame("kgt", string(body))
}

// SendKGT starts one kingdom-resource shipment.
func SendKGT(sourceCastleAID, sourceKingdomID, targetKingdomID int, resourceCode string, amount int) {
	QueueOutgoingPayload(KGTPayload(sourceCastleAID, sourceKingdomID, targetKingdomID, resourceCode, amount))
}

// KingdomResourceMSKPayload applies a selected time skip to resource transport (TT 2).
func KingdomResourceMSKPayload(timeSkipID string, targetKingdomID int) string {
	return empireExFrame("msk", fmt.Sprintf(`{"MST":%q,"KID":%q,"TT":"2"}`, timeSkipID, fmt.Sprint(targetKingdomID)))
}

// SendKingdomResourceMSK skips time on an in-flight kingdom resource shipment.
func SendKingdomResourceMSK(timeSkipID string, targetKingdomID int) {
	QueueOutgoingPayload(KingdomResourceMSKPayload(timeSkipID, targetKingdomID))
}

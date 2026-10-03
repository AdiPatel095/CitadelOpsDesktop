package Intent

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// recordStormPackageCapEvidence runs before guardRejection can terminate the
// operation. Rebuild every wire object from allowed fields, never copy frames.
func recordStormPackageCapEvidence(ctx context.Context, command Protocol.Command, response Protocol.Frame, state State.GameState, sentAt time.Time) error {
	if command.Opcode != "sbp" || response.ResponseCode == nil || *response.ResponseCode != 237 {
		return nil
	}
	var request struct {
		PID      int64 `json:"PID"`
		BT       int64 `json:"BT"`
		TID      int64 `json:"TID"`
		AMT      int64 `json:"AMT"`
		KID      int64 `json:"KID"`
		AID      int64 `json:"AID"`
		PC2      int64 `json:"PC2"`
		BA       int64 `json:"BA"`
		PWR      int64 `json:"PWR"`
		Position int64 `json:"_PO"`
	}
	if json.Unmarshal(command.Payload, &request) != nil || request.PID <= 0 || request.AMT <= 0 ||
		request.KID != GameData.StormKingdomID || request.BT != GameData.StormLunaShopBuildType ||
		request.TID != GameData.StormLunaShopTableID || request.AID != GameData.StormLunaShopCastleID {
		return nil
	}
	castleID := state.Inventory.ConstructionOffersCastleID
	if state.Inventory.ConstructionOffersKingdomID != GameData.StormKingdomID || castleID <= 0 {
		return nil
	}
	type product struct {
		PID State.PackageID `json:"PID"`
		AMT int64           `json:"AMT"`
	}
	products := make([]product, 0, len(state.Inventory.ConstructionOffers))
	for id, amount := range state.Inventory.ConstructionOffers {
		products = append(products, product{id, amount})
	}
	sort.Slice(products, func(i, j int) bool { return products[i].PID < products[j].PID })
	var reply struct {
		PID *int64 `json:"PID,omitempty"`
		AMT *int64 `json:"AMT,omitempty"`
	}
	// Unknown/private response fields are deliberately excluded. An empty or
	// malformed error payload still retains the authoritative response code.
	_ = json.Unmarshal(response.Payload, &reply)
	return RecordOperationEvidence(ctx, "sbp_cap_rejection", map[string]any{
		"occurrenceKey": State.StormShopOccurrenceKey(castleID, request.TID),
		"generation":    state.Session.Generation, "connectionGeneration": state.Session.ConnectionGeneration,
		"gbc": map[string]any{"PL": products}, "gbcObservedAt": state.Inventory.ConstructionOffersObservedAt,
		"sbpRequest": request, "sbpResponse": reply, "responseCode": *response.ResponseCode,
		"sentAt": sentAt, "receivedAt": response.ReceivedAt,
	})
}

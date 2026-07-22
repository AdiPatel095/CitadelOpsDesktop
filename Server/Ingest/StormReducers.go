package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func reduceStormShopCommand(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	gameData *GameData.Store,
) ([]string, bool, error) {
	if frame.Direction != Protocol.DirectionOutbound || len(frame.Payload) == 0 || gameData == nil {
		return nil, false, nil
	}
	var payload struct {
		ProductID wireInt64 `json:"PID"`
		BuildType wireInt64 `json:"BT"`
		TableID   wireInt64 `json:"TID"`
		Amount    wireInt64 `json:"AMT"`
		KingdomID wireInt64 `json:"KID"`
		CastleID  wireInt64 `json:"AID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Storm shop purchase context: %w", err)
	}
	if payload.KingdomID != GameData.StormKingdomID || payload.ProductID <= 0 || payload.Amount <= 0 ||
		payload.BuildType != GameData.StormLunaShopBuildType || payload.TableID != GameData.StormLunaShopTableID ||
		payload.CastleID != GameData.StormLunaShopCastleID {
		return nil, false, nil
	}
	item, found := gameData.StormShopPackage(int64(payload.ProductID))
	if !found || int64(payload.Amount) > math.MaxInt64/item.AquamarinePrice {
		return nil, false, nil
	}
	castleID, castle, focused := focusedCastle(gameState)
	if !focused || castle.KingdomID != GameData.StormKingdomID {
		return nil, false, nil
	}
	observedAt := frame.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	productID := State.PackageID(payload.ProductID)
	amount := int64(payload.Amount)
	cost := amount * item.AquamarinePrice
	if gameState.Storm.LunaShopTableID == int64(payload.TableID) &&
		gameState.Storm.LunaShopProductID == productID && gameState.Storm.LunaShopObservedAt.Equal(observedAt) &&
		gameState.Storm.LunaShopPending && gameState.Storm.LunaShopPendingCastleID == castleID &&
		gameState.Storm.LunaShopPendingAmount == amount && gameState.Storm.LunaShopPendingAquamarineCost == cost {
		return nil, false, nil
	}
	gameState.Storm.LunaShopTableID = int64(payload.TableID)
	gameState.Storm.LunaShopProductID = productID
	gameState.Storm.LunaShopObservedAt = observedAt
	gameState.Storm.LunaShopPending = true
	gameState.Storm.LunaShopPendingCastleID = castleID
	gameState.Storm.LunaShopPendingAmount = amount
	gameState.Storm.LunaShopPendingAquamarineCost = cost
	return []string{"storm"}, true, nil
}

func reduceStormShopResponse(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.Direction != Protocol.DirectionInbound || !gameState.Storm.LunaShopPending ||
		(!frame.ReceivedAt.IsZero() && !gameState.Storm.LunaShopObservedAt.IsZero() &&
			frame.ReceivedAt.Before(gameState.Storm.LunaShopObservedAt)) {
		return nil, false, nil
	}
	castleID := gameState.Storm.LunaShopPendingCastleID
	cost := gameState.Storm.LunaShopPendingAquamarineCost
	resourceChanged := false
	var payload struct {
		Available *float64        `json:"AS"`
		Resources json.RawMessage `json:"grc"`
	}
	if len(frame.Payload) > 0 {
		if err := json.Unmarshal(frame.Payload, &payload); err != nil {
			return nil, false, fmt.Errorf("decode Storm shop response: %w", err)
		}
	}
	castle, exists := gameState.Castles[castleID]
	if exists {
		ensureCastleMaps(&castle)
		balance := castle.Resources[State.ResourceID(GameData.StormAquamarineID)]
		switch {
		case frame.ResponseCode != nil && *frame.ResponseCode == 55 && payload.Available != nil:
			if balance.Amount != *payload.Available {
				balance.Amount = *payload.Available
				resourceChanged = true
			}
		case frameSucceeded(frame) && len(payload.Resources) == 0 && cost > 0:
			next := math.Max(0, balance.Amount-float64(cost))
			if balance.Amount != next {
				balance.Amount = next
				resourceChanged = true
			}
		}
		if resourceChanged {
			castle.Resources[State.ResourceID(GameData.StormAquamarineID)] = balance
			gameState.Castles[castleID] = castle
		}
	}
	gameState.Storm.LunaShopPending = false
	gameState.Storm.LunaShopPendingCastleID = 0
	gameState.Storm.LunaShopPendingAmount = 0
	gameState.Storm.LunaShopPendingAquamarineCost = 0
	domains := []string{"storm"}
	if resourceChanged {
		domains = append(domains, "castles", "resources")
	}
	return domains, true, nil
}

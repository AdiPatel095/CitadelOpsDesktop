package Ingest

import (
	"context"
	"encoding/json"
	"fmt"
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
		TableID   wireInt64 `json:"TID"`
		KingdomID wireInt64 `json:"KID"`
	}
	if err := json.Unmarshal(frame.Payload, &payload); err != nil {
		return nil, false, fmt.Errorf("decode Storm shop purchase context: %w", err)
	}
	if payload.KingdomID != GameData.StormKingdomID || payload.ProductID <= 0 || payload.TableID <= 0 {
		return nil, false, nil
	}
	if _, found := gameData.StormShopPackage(int64(payload.ProductID)); !found {
		return nil, false, nil
	}
	observedAt := frame.ReceivedAt.UTC()
	if observedAt.IsZero() {
		observedAt = time.Now().UTC()
	}
	productID := State.PackageID(payload.ProductID)
	if gameState.Storm.LunaShopTableID == int64(payload.TableID) &&
		gameState.Storm.LunaShopProductID == productID && gameState.Storm.LunaShopObservedAt.Equal(observedAt) {
		return nil, false, nil
	}
	gameState.Storm.LunaShopTableID = int64(payload.TableID)
	gameState.Storm.LunaShopProductID = productID
	gameState.Storm.LunaShopObservedAt = observedAt
	return []string{"storm"}, true, nil
}

package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type beriToolInventoryRefreshRequest struct {
	CastleID State.CastleID `json:"castleId"`
}

type beriToolPurchaseRequest struct {
	CastleID  State.CastleID  `json:"castleId"`
	PackageID State.PackageID `json:"packageId"`
	ToolID    State.UnitID    `json:"toolId"`
	Amount    int64           `json:"amount"`
	Minimum   int64           `json:"minimum"`
}

func planBeriToolInventoryRefresh(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request beriToolInventoryRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if request.CastleID <= 0 || !exists || castle.KingdomID != beriKingdomID {
		return Intent.Plan{}, fmt.Errorf(
			"%w: castleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		)
	}
	castleID := strconv.FormatInt(int64(castle.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "attack-inventory:" + castleID},
		Summary: fmt.Sprintf("Refresh Berimond armorer tool inventory for castle %d", castle.ID),
		Steps:   []Intent.Step{attackCastleContextStep(castle)},
	}, nil
}

func planBeriToolPurchase(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, castle, item, err := beriToolPurchaseContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolvedArguments, _ := json.Marshal(request)
	historyPayload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, castle.KingdomID})
	purchasePayload, _ := json.Marshal(struct {
		ProductID State.PackageID `json:"PID"`
		BuildType int64           `json:"BT"`
		TableID   int64           `json:"TID"`
		Amount    int64           `json:"AMT"`
		KingdomID State.KingdomID `json:"KID"`
		CastleID  int64           `json:"AID"`
		Premium   int64           `json:"PC2"`
		BuyAll    int64           `json:"BA"`
		Power     int64           `json:"PWR"`
		Position  int64           `json:"_PO"`
	}{
		request.PackageID, 0, GameData.BerimondArmorerShopTableID, request.Amount,
		castle.KingdomID, -1, -1, 0, 0, -1,
	})
	steps := []Intent.Step{
		attackCastleContextStep(castle),
		shopCommandStep("Refresh Berimond armorer", "gbc", historyPayload, 0),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Recheck Berimond armorer tool purchase", Action: "beri.tools.purchase.guard",
			ActionArguments: resolvedArguments,
		}),
		shopCommandStep("Purchase "+item.Name+" from the Berimond armorer", "sbp", purchasePayload, 0),
		attackCastleRefreshStep("Refresh Berimond tool inventory after purchase", castle),
	}
	castleID := strconv.FormatInt(int64(castle.ID), 10)
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:table:" + strconv.FormatInt(GameData.BerimondArmorerShopTableID, 10),
			"account-resources", "castle-focus", "castle:" + castleID,
			"attack-inventory:" + castleID, "unit:" + strconv.FormatInt(int64(request.ToolID), 10),
		},
		Summary: fmt.Sprintf(
			"Buy a batch of %d %s for %d coins toward the Berimond minimum %d",
			request.Amount*item.ToolAmount, item.Name, request.Amount*item.CoinPrice, request.Minimum,
		),
		Steps: steps,
	}, nil
}

func beriToolPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (beriToolPurchaseRequest, State.CastleState, GameData.BerimondArmorerToolPackage, error) {
	var request beriToolPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{}, err
	}
	if request.CastleID <= 0 || request.PackageID <= 0 || request.ToolID <= 0 ||
		request.Amount <= 0 || request.Minimum <= 0 {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{},
			fmt.Errorf("Berimond tool purchase requires castle, package, tool, amount, and minimum")
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{},
			fmt.Errorf("official game data is unavailable")
	}
	item, supported := input.GameData.BerimondArmorerAttackTool(int64(request.ToolID))
	if !supported || item.PackageID != int64(request.PackageID) {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{},
			fmt.Errorf("package %d is not the supported Berimond armorer package for tool %d", request.PackageID, request.ToolID)
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return request, State.CastleState{}, item,
			fmt.Errorf("%w: the Berimond camp is no longer owned", Intent.ErrPlanStale)
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return request, castle, item,
			fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale)
	}
	if item.MinLevel > input.State.Player.Level {
		return request, castle, item,
			fmt.Errorf("%s requires player level %d", item.Name, item.MinLevel)
	}
	available := castle.Units.Stationed[request.ToolID]
	deficit := request.Minimum - available
	if deficit <= 0 {
		return request, castle, item,
			fmt.Errorf("%w: tool %d already meets its configured minimum", Intent.ErrPlanStale, request.ToolID)
	}
	neededPurchases := deficit / item.ToolAmount
	if deficit%item.ToolAmount != 0 {
		neededPurchases++
	}
	expectedPurchases := min(neededPurchases, int64(GameData.BerimondArmorerMaxPurchaseAmount))
	if request.Amount != expectedPurchases {
		return request, castle, item,
			fmt.Errorf(
				"%w: tool %d now requires a batch of %d package purchase(s), not %d",
				Intent.ErrPlanStale, request.ToolID, expectedPurchases, request.Amount,
			)
	}
	if request.Amount > math.MaxInt64/item.CoinPrice {
		return request, castle, item, fmt.Errorf("Berimond tool purchase amount is too large")
	}
	requiredCoins := request.Amount * item.CoinPrice
	availableCoins := int64(math.Floor(input.State.Player.Resources[State.ResourceID(1)]))
	if availableCoins < requiredCoins {
		return request, castle, item,
			fmt.Errorf(
				"%w: Berimond tool purchase requires %d coins but only %d are available",
				Intent.ErrPlanStale, requiredCoins, availableCoins,
			)
	}
	return request, castle, item, nil
}

func (application *Application) guardBeriToolPurchase(_ context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("Berimond armorer state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, castle, _, err := beriToolPurchaseContext(Intent.PlanningContext{
		State: application.State.Snapshot(), GameData: gameData,
	}, arguments)
	if err != nil {
		return err
	}
	if !castle.Focused {
		return fmt.Errorf("%w: the Berimond armorer castle is no longer focused", Intent.ErrPlanStale)
	}
	return nil
}

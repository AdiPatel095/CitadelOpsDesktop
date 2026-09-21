package App

import (
	"CitadelDesktop/Server/Localization"
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: castleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.92f30ade", "intent plan became stale before dispatch: castleId no longer identifies an owned Berimond camp", nil))
	}
	castleID := strconv.FormatInt(int64(castle.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "attack-inventory:" + castleID},
		Summary: fmt.Sprintf("Refresh Berimond armorer tool inventory for castle %d", castle.ID), SummaryDescriptor: Localization.New("server.app.refresh_berimond_armorer_tool.c1c2a537", "Refresh Berimond armorer tool inventory for castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}),
		Steps: []Intent.Step{attackCastleContextStep(castle)},
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
		shopCommandStep("Refresh Berimond armorer", "gbc", historyPayload, 0).WithNameDescriptor(Localization.New("server.app.refresh_berimond_armorer.4675d28b", "Refresh Berimond armorer", nil)),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Recheck Berimond armorer tool purchase", NameDescriptor: Localization.New("server.app.recheck_berimond_armorer_tool.abf3a716", "Recheck Berimond armorer tool purchase", nil), Action: "beri.tools.purchase.guard",
			ActionArguments: resolvedArguments,
		}),
		shopCommandStep("Purchase "+item.Name+" from the Berimond armorer", "sbp", purchasePayload, 0),
		attackCastleRefreshStep("Refresh Berimond tool inventory after purchase", castle).WithNameDescriptor(Localization.New("server.app.refresh_berimond_tool_inventory.4175233d", "Refresh Berimond tool inventory after purchase", nil)),
	}
	steps[3].CoinCost = &Intent.CoinCostRequirement{
		Amount: request.Amount * item.CoinPrice, Source: "official Berimond armorer package price",
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
		), SummaryDescriptor: gameNameDescriptor(Localization.New("server.app.buy_a_batch_of.8ecc9a26", "Buy a batch of {p0} {p1} for {p2} coins toward the Berimond minimum {p3}", Localization.Params{"p0": request.Amount * item.ToolAmount, "p1": fmt.Sprintf("%s", item.Name), "p2": request.Amount * item.CoinPrice, "p3": request.Minimum}), input, "p1", "units", item.ToolID, item.Name),
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
			Localization.WithError(fmt.Errorf("Berimond tool purchase requires castle, package, tool, amount, and minimum"), Localization.New("server.app.berimond_tool_purchase_requires.a6e30ce2", "Berimond tool purchase requires castle, package, tool, amount, and minimum", nil))
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{},
			Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	item, supported := input.GameData.BerimondArmorerAttackTool(int64(request.ToolID))
	if !supported || item.PackageID != int64(request.PackageID) {
		return request, State.CastleState{}, GameData.BerimondArmorerToolPackage{},
			Localization.WithError(fmt.Errorf("package %d is not the supported Berimond armorer package for tool %d", request.PackageID, request.ToolID), Localization.New("server.app.package_p_is_not.acf34738", "package {p0} is not the supported Berimond armorer package for tool {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": fmt.Sprintf("%d", request.ToolID)}))
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return request, State.CastleState{}, item,
			Localization.WithError(fmt.Errorf("%w: the Berimond camp is no longer owned", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.31bb78b0", "intent plan became stale before dispatch: the Berimond camp is no longer owned", nil))
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return request, castle, item,
			Localization.WithError(fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4e00f6a", "intent plan became stale before dispatch: the Battle for Berimond is no longer unlocked", nil))
	}
	if item.MinLevel > input.State.Player.Level {
		return request, castle, item,
			Localization.WithError(fmt.Errorf("%s requires player level %d", item.Name, item.MinLevel), gameNameDescriptor(Localization.New("server.app.p_requires_player_level.a958c0e9", "{p0} requires player level {p1}", Localization.Params{"p0": fmt.Sprintf("%s", item.Name), "p1": item.MinLevel}), input, "p0", "units", item.ToolID, item.Name))
	}
	available := castle.Units.Stationed[request.ToolID]
	deficit := request.Minimum - available
	if deficit <= 0 {
		return request, castle, item,
			Localization.WithError(fmt.Errorf("%w: tool %d already meets its configured minimum", Intent.ErrPlanStale, request.ToolID), Localization.New("server.app.intent_plan_became_stale.187edad8", "intent plan became stale before dispatch: tool {p1} already meets its configured minimum", Localization.Params{"p1": fmt.Sprintf("%d", request.ToolID)}))
	}
	neededPurchases := deficit / item.ToolAmount
	if deficit%item.ToolAmount != 0 {
		neededPurchases++
	}
	expectedPurchases := min(neededPurchases, int64(GameData.BerimondArmorerMaxPurchaseAmount))
	if request.Amount != expectedPurchases {
		return request, castle, item,
			Localization.WithError(fmt.Errorf(
				"%w: tool %d now requires a batch of %d package purchase(s), not %d",
				Intent.ErrPlanStale, request.ToolID, expectedPurchases, request.Amount,
			), Localization.New("server.app.intent_plan_became_stale.502b6ea4", "intent plan became stale before dispatch: tool {p1} now requires a batch of {p2, number} package purchase(s), not {p3, number}", Localization.Params{"p1": fmt.Sprintf("%d", request.ToolID), "p2": expectedPurchases, "p3": request.Amount}))
	}
	if request.Amount > math.MaxInt64/item.CoinPrice {
		return request, castle, item, Localization.WithError(fmt.Errorf("Berimond tool purchase amount is too large"), Localization.New("server.app.berimond_tool_purchase_amount.d071c423", "Berimond tool purchase amount is too large", nil))
	}
	requiredCoins := request.Amount * item.CoinPrice
	coinID, coinErr := officialResourceIDByJSONKey(input.GameData, coinResourceKey)
	if coinErr != nil {
		return request, castle, item, Localization.WithError(fmt.Errorf("coin affordability unavailable: %w", coinErr), Localization.ErrorContext(Localization.New("server.app.coin_affordability_unavailable.a3c889be", "coin affordability unavailable", nil), coinErr))
	}
	availableCoins := int64(math.Floor(input.State.Player.Resources[State.ResourceID(coinID)]))
	if availableCoins < requiredCoins {
		return request, castle, item, &Intent.CoinUnavailableError{
			Required: requiredCoins, Observed: availableCoins, Source: "official Berimond armorer package price",
		}
	}
	return request, castle, item, nil
}

func (application *Application) guardBeriToolPurchase(_ context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Berimond armorer state is unavailable"), Localization.New("server.app.berimond_armorer_state_is.52d4d1cf", "Berimond armorer state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, castle, _, err := beriToolPurchaseContext(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, arguments)
	if err != nil {
		return err
	}
	if !castle.Focused {
		return Localization.WithError(fmt.Errorf("%w: the Berimond armorer castle is no longer focused", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.e4043592", "intent plan became stale before dispatch: the Berimond armorer castle is no longer focused", nil))
	}
	return nil
}

package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type autoBuyerHistoryRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
}

type autoBuyerPackagePurchaseRequest struct {
	SourceCastleID           State.CastleID  `json:"sourceCastleId"`
	ShopID                   string          `json:"shopId"`
	PackageID                State.PackageID `json:"packageId"`
	Amount                   int64           `json:"amount"`
	TargetPurchasesPerReset  int64           `json:"targetPurchasesPerReset"`
	MinimumBalanceReserve    int64           `json:"minimumBalanceReserve"`
	AllowRubyPackages        bool            `json:"allowRubyPackages"`
	MaximumRubySpendPerReset int64           `json:"maximumRubySpendPerReset"`
	MinimumRubyReserve       int64           `json:"minimumRubyReserve"`
	ExpectedPurchasedBefore  int64           `json:"expectedPurchasedBefore"`
	ExpectedBalanceBefore    int64           `json:"expectedBalanceBefore"`
}

type autoBuyerSpecialistPurchaseRequest struct {
	SpecialistID               int   `json:"specialistId"`
	MinimumDays                int   `json:"minimumDays"`
	MaximumRubyCostPerPurchase int64 `json:"maximumRubyCostPerPurchase"`
	MinimumRubyReserve         int64 `json:"minimumRubyReserve"`
	ExpectedExpiresAtUnix      int64 `json:"expectedExpiresAtUnix"`
	ExpectedPurchaseCount      int   `json:"expectedPurchaseCount"`
	ExpectedRubyBalance        int64 `json:"expectedRubyBalance"`
	HistoryRefreshSec          int   `json:"historyRefreshSec"`
}

type autoBuyerFeastPurchaseRequest struct {
	FeastID                    int64          `json:"feastId"`
	MinimumRemainingHours      int            `json:"minimumRemainingHours"`
	SourceCastleID             State.CastleID `json:"sourceCastleId"`
	MinimumFoodReserve         int64          `json:"minimumFoodReserve"`
	AllowRubies                bool           `json:"allowRubies"`
	MaximumRubyCostPerPurchase int64          `json:"maximumRubyCostPerPurchase"`
	MinimumRubyReserve         int64          `json:"minimumRubyReserve"`
	ExpectedActiveFeastID      int64          `json:"expectedActiveFeastId"`
	ExpectedExpiresAtUnix      int64          `json:"expectedExpiresAtUnix"`
	ExpectedBalanceBefore      int64          `json:"expectedBalanceBefore"`
	HistoryRefreshSec          int            `json:"historyRefreshSec"`
}

func (application *Application) registerAutoBuyerIntents() error {
	definitions := []Intent.Definition{
		{
			Name: "autoBuyer.package.history", Description: "Refresh server purchase counters used to detect shop stock resets",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{"sourceCastleId":123}`), Planner: planAutoBuyerPackageHistory,
		},
		{
			Name: "autoBuyer.boosters.refresh", Description: "Refresh specialist and feast timers before evaluating renewal floors",
			Effect: Intent.EffectRead, ArgumentsExample: json.RawMessage(`{}`), Planner: planAutoBuyerBoostersRefresh,
		},
		{
			Name: "autoBuyer.package.purchase", Description: "Purchase a bounded official package after a fresh stock, event, price, and reserve guard",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"sourceCastleId":123,"shopId":"master-blacksmith","packageId":456,"amount":1,"targetPurchasesPerReset":1}`), Planner: planAutoBuyerPackagePurchase,
		},
		{
			Name: "autoBuyer.specialist.purchase", Description: "Renew one supported specialist by one official seven-day period within a ruby ceiling",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"specialistId":0,"minimumDays":14,"maximumRubyCostPerPurchase":625}`), Planner: planAutoBuyerSpecialistPurchase,
		},
		{
			Name: "autoBuyer.feast.purchase", Description: "Start or extend one official feast within configured food or ruby reserves",
			Effect: Intent.EffectWrite, ArgumentsExample: json.RawMessage(`{"feastId":0,"minimumRemainingHours":12,"sourceCastleId":123}`), Planner: planAutoBuyerFeastPurchase,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	actions := map[string]Intent.Action{
		"auto_buyer.package.guard":     application.guardAutoBuyerPackagePurchase,
		"auto_buyer.package.verify":    application.verifyAutoBuyerPackagePurchase,
		"auto_buyer.specialist.guard":  application.guardAutoBuyerSpecialistPurchase,
		"auto_buyer.specialist.verify": application.verifyAutoBuyerSpecialistPurchase,
		"auto_buyer.feast.guard":       application.guardAutoBuyerFeastPurchase,
		"auto_buyer.feast.verify":      application.verifyAutoBuyerFeastPurchase,
	}
	for name, action := range actions {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	return nil
}

func planAutoBuyerPackageHistory(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request autoBuyerHistoryRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, err := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(map[string]any{"CID": source.ID, "KID": source.KingdomID})
	step := shopCommandStep("Refresh Auto Buyer package counters", "gbc", payload, 0)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "shop:purchase-history"},
		Summary: fmt.Sprintf("Refresh Auto Buyer stock counters from %s", castleLabel(source)), Steps: []Intent.Step{step},
	}, nil
}

func planAutoBuyerBoostersRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	step := shopCommandStep("Refresh specialist and feast timers", "boi", json.RawMessage(`{}`), 0)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{"shop", "market:boosters"}, Summary: "Refresh Auto Buyer specialist and feast timers",
		Steps: []Intent.Step{step},
	}, nil
}

func planAutoBuyerPackagePurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, product, err := autoBuyerPackagePurchaseContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	historyPayload, _ := json.Marshal(map[string]any{"CID": source.ID, "KID": source.KingdomID})
	historyBefore := shopCommandStep("Refresh package counter before purchase", "gbc", historyPayload, 0)
	historyBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	historyAfter := shopCommandStep("Verify package counter after purchase", "gbc", historyPayload, 0)
	historyAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	purchaseCastleID := int64(-1)
	purchaseKingdomID := State.KingdomID(0)
	if product.Price.Scope == GameData.AutoBuyerPriceCastleResource {
		purchaseCastleID = int64(source.ID)
		purchaseKingdomID = source.KingdomID
	}
	payload, _ := json.Marshal(struct {
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
	}{State.PackageID(product.PackageID), 0, product.TableID, request.Amount, purchaseKingdomID, purchaseCastleID, -1, 0, 0, -1})
	steps := []Intent.Step{historyBefore}
	if product.Price.Scope == GameData.AutoBuyerPriceCastleResource {
		resourcePayload, _ := json.Marshal(map[string]any{"AID": source.ID, "KID": source.KingdomID})
		resourceRefresh := commandStep("Refresh package source resources before purchase", "grc", resourcePayload, "grc")
		resourceRefresh.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, resourceRefresh)
	}
	steps = append(steps,
		Intent.RebuildOnResume(Intent.Step{Name: "Recheck Auto Buyer package purchase", Action: "auto_buyer.package.guard", ActionArguments: resolved}),
		shopCommandStep("Purchase "+product.Name, "sbp", payload, 0),
		historyAfter,
		Intent.RebuildOnResume(Intent.Step{Name: "Verify Auto Buyer package purchase", Action: "auto_buyer.package.verify", ActionArguments: resolved}),
	)
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:table:" + strconv.FormatInt(product.TableID, 10), "shop:purchase-history", "account-resources",
		},
		Summary: fmt.Sprintf("Buy %d x %s for %d %s", request.Amount, product.Name, request.Amount*product.Price.Amount, product.Price.Name),
		Steps:   steps,
	}, nil
}

func planAutoBuyerSpecialistPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, specialist, err := autoBuyerSpecialistPurchaseContext(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	payload := json.RawMessage(`{"PO":-1}`)
	if specialist.Opcode == "ovs" {
		payload, _ = json.Marshal(map[string]any{"T": specialist.ResourceType, "PO": -1})
	}
	refreshBefore := shopCommandStep("Refresh specialist timer before renewal", "boi", json.RawMessage(`{}`), 0)
	refreshBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	refreshAfter := shopCommandStep("Refresh specialist timer after renewal", "boi", json.RawMessage(`{}`), 0)
	refreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"shop", "market:boosters", "account-resources", "specialist:" + strconv.Itoa(request.SpecialistID)},
		Summary: fmt.Sprintf("Renew %s by 7 days within a %d-ruby ceiling", specialist.Name, request.MaximumRubyCostPerPurchase),
		Steps: []Intent.Step{
			refreshBefore,
			Intent.RebuildOnResume(Intent.Step{Name: "Recheck specialist renewal", Action: "auto_buyer.specialist.guard", ActionArguments: resolved}),
			shopCommandStep("Renew "+specialist.Name, specialist.Opcode, payload, 0),
			refreshAfter,
			Intent.RebuildOnResume(Intent.Step{Name: "Verify specialist renewal", Action: "auto_buyer.specialist.verify", ActionArguments: resolved}),
		},
	}, nil
}

func planAutoBuyerFeastPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, feast, err := autoBuyerFeastPurchaseContext(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	resolved, _ := json.Marshal(request)
	payload, _ := json.Marshal(map[string]any{
		"T": feast.ID, "CID": source.ID, "KID": source.KingdomID, "PO": -1, "PWR": 0,
	})
	resourcePayload, _ := json.Marshal(map[string]any{"AID": source.ID, "KID": source.KingdomID})
	resourceRefreshBefore := commandStep("Refresh feast source resources before purchase", "grc", resourcePayload, "grc")
	resourceRefreshBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	boosterRefreshBefore := shopCommandStep("Refresh feast timer before purchase", "boi", json.RawMessage(`{}`), 0)
	boosterRefreshBefore.ResponseBarrier = Intent.ResponseBarrierCommitted
	resourceRefreshAfter := commandStep("Refresh feast source resources after purchase", "grc", resourcePayload, "grc")
	resourceRefreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	boosterRefreshAfter := shopCommandStep("Refresh feast timer after purchase", "boi", json.RawMessage(`{}`), 0)
	boosterRefreshAfter.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{
			"shop", "market:boosters", "account-resources", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Start or extend %s for %d %s", feast.Name, feast.Price.Amount, feast.Price.Name),
		Steps: []Intent.Step{
			resourceRefreshBefore,
			boosterRefreshBefore,
			Intent.RebuildOnResume(Intent.Step{Name: "Recheck feast purchase", Action: "auto_buyer.feast.guard", ActionArguments: resolved}),
			shopCommandStep("Start or extend "+feast.Name, "bfs", payload, 0),
			resourceRefreshAfter, boosterRefreshAfter,
			Intent.RebuildOnResume(Intent.Step{Name: "Verify feast purchase", Action: "auto_buyer.feast.verify", ActionArguments: resolved}),
		},
	}, nil
}

func autoBuyerPackagePurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requireFresh bool,
) (autoBuyerPackagePurchaseRequest, State.CastleState, GameData.AutoBuyerPackage, error) {
	var request autoBuyerPackagePurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, err
	}
	request.ShopID = strings.TrimSpace(request.ShopID)
	if request.PackageID <= 0 || request.Amount <= 0 || request.TargetPurchasesPerReset <= 0 ||
		request.MinimumBalanceReserve < 0 || request.MaximumRubySpendPerReset < 0 || request.MinimumRubyReserve < 0 ||
		request.ExpectedPurchasedBefore < 0 || request.ExpectedBalanceBefore < 0 {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, fmt.Errorf("Auto Buyer package request contains an invalid product, amount, target, balance, reserve, or ceiling")
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, fmt.Errorf("official game data is unavailable")
	}
	source, err := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerPackage{}, err
	}
	product, found := input.GameData.AutoBuyerPackage(request.ShopID, int64(request.PackageID))
	if !found {
		return request, source, GameData.AutoBuyerPackage{}, fmt.Errorf("package %d is not in the supported %s Auto Buyer catalog", request.PackageID, request.ShopID)
	}
	if request.TargetPurchasesPerReset > product.Stock {
		return request, source, product, fmt.Errorf("package target exceeds official stock %d", product.Stock)
	}
	if !autoBuyerIntentLevelEligible(input.State.Player, product.MinLevel, product.MaxLevel, product.MinLegendLevel, product.MaxLegendLevel) {
		return request, source, product, fmt.Errorf("package %d is not available at the current player level", request.PackageID)
	}
	if requireFresh {
		if input.State.Inventory.ConstructionOffersCastleID != source.ID ||
			input.State.Inventory.ConstructionOffersKingdomID != source.KingdomID ||
			input.State.Inventory.ConstructionOffersObservedAt.IsZero() ||
			now.Sub(input.State.Inventory.ConstructionOffersObservedAt) > 2*time.Minute {
			return request, source, product, fmt.Errorf("%w: package purchase counters are not fresh for castle %d", Intent.ErrPlanStale, source.ID)
		}
		if product.RequiresEvent {
			if _, active := input.State.ActiveShopForPackage(request.PackageID, now); !active {
				return request, source, product, fmt.Errorf("%w: the event shop for package %d is not active", Intent.ErrPlanStale, request.PackageID)
			}
		}
		purchased := input.State.Inventory.ConstructionOffers[request.PackageID]
		if purchased != request.ExpectedPurchasedBefore {
			return request, source, product, fmt.Errorf("%w: package %d purchase count changed from %d to %d", Intent.ErrPlanStale, request.PackageID, request.ExpectedPurchasedBefore, purchased)
		}
		remaining := min(request.TargetPurchasesPerReset, product.Stock) - purchased
		remaining = min(remaining, product.Stock-purchased)
		if product.MaxBuyPerClick > 0 {
			remaining = min(remaining, product.MaxBuyPerClick)
		}
		if request.Amount > remaining || remaining <= 0 {
			return request, source, product, fmt.Errorf("%w: package %d now has only %d configured purchases remaining", Intent.ErrPlanStale, request.PackageID, max(int64(0), remaining))
		}
		balance, available := autoBuyerIntentPriceBalance(input.State, source, product.Price)
		if !available {
			return request, source, product, fmt.Errorf("%s balance is unavailable", product.Price.Name)
		}
		reserve := request.MinimumBalanceReserve
		if product.Price.Premium {
			if !request.AllowRubyPackages || request.MaximumRubySpendPerReset <= 0 {
				return request, source, product, fmt.Errorf("ruby package purchases are not explicitly enabled")
			}
			reserve = max(reserve, request.MinimumRubyReserve)
			if purchased > math.MaxInt64/product.Price.Amount || request.Amount > math.MaxInt64/product.Price.Amount {
				return request, source, product, fmt.Errorf("package ruby amount is too large")
			}
			if (purchased+request.Amount)*product.Price.Amount > request.MaximumRubySpendPerReset {
				return request, source, product, fmt.Errorf("package purchase exceeds the configured per-reset ruby ceiling")
			}
		}
		if request.Amount > math.MaxInt64/product.Price.Amount {
			return request, source, product, fmt.Errorf("package purchase amount is too large")
		}
		required := request.Amount * product.Price.Amount
		if balance-reserve < required {
			return request, source, product, fmt.Errorf("%w: package requires %d %s above reserve but only %d is spendable", Intent.ErrPlanStale, required, product.Price.Name, max(int64(0), balance-reserve))
		}
	}
	return request, source, product, nil
}

func autoBuyerSpecialistPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (autoBuyerSpecialistPurchaseRequest, GameData.AutoBuyerSpecialist, error) {
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, GameData.AutoBuyerSpecialist{}, err
	}
	specialist, found := GameData.AutoBuyerSpecialistByID(request.SpecialistID)
	if !found || request.MinimumDays < 14 || request.MinimumDays > 365 || request.MinimumRubyReserve < 0 ||
		request.MaximumRubyCostPerPurchase < specialist.BaseRubyCost || request.HistoryRefreshSec < 60 || request.HistoryRefreshSec > 3600 ||
		request.ExpectedRubyBalance < 0 {
		return request, specialist, fmt.Errorf("specialist renewal settings are invalid or do not cover the safe official ruby maximum")
	}
	if input.GameData == nil {
		return request, specialist, fmt.Errorf("official game data is unavailable")
	}
	if input.State.Market.BoostersObservedAt.IsZero() || now.Sub(input.State.Market.BoostersObservedAt) >= time.Duration(request.HistoryRefreshSec)*time.Second {
		return request, specialist, fmt.Errorf("%w: specialist timers are stale", Intent.ErrPlanStale)
	}
	booster := input.State.Market.Boosters[request.SpecialistID]
	if !autoBuyerIntentExpiryMatches(booster.ExpiresAt, request.ExpectedExpiresAtUnix) || booster.ContinuousPurchaseCount != request.ExpectedPurchaseCount {
		return request, specialist, fmt.Errorf("%w: %s timer or rebuy state changed", Intent.ErrPlanStale, specialist.Name)
	}
	remaining := autoBuyerIntentRemaining(booster.ExpiresAt, now)
	if remaining >= int64(request.MinimumDays)*24*60*60 {
		return request, specialist, fmt.Errorf("%w: %s already meets its configured floor", Intent.ErrPlanStale, specialist.Name)
	}
	rubies := int64(math.Floor(playerResourceByOfficialKey(input.State, input.GameData, "C2")))
	if rubies-request.MinimumRubyReserve < specialist.BaseRubyCost {
		return request, specialist, fmt.Errorf("%w: %s requires up to %d rubies above reserve", Intent.ErrPlanStale, specialist.Name, specialist.BaseRubyCost)
	}
	return request, specialist, nil
}

func autoBuyerFeastPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (autoBuyerFeastPurchaseRequest, State.CastleState, GameData.AutoBuyerFeast, error) {
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, err
	}
	if request.MinimumRemainingHours <= 0 || request.MinimumRemainingHours > 24*30 || request.MinimumFoodReserve < 0 ||
		request.MinimumRubyReserve < 0 || request.MaximumRubyCostPerPurchase < 0 || request.HistoryRefreshSec < 60 ||
		request.HistoryRefreshSec > 3600 || request.ExpectedBalanceBefore < 0 {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("feast floor, balance, reserve, or ruby ceiling is invalid")
	}
	if input.GameData == nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, fmt.Errorf("official game data is unavailable")
	}
	source, err := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return request, State.CastleState{}, GameData.AutoBuyerFeast{}, err
	}
	feast, found := input.GameData.AutoBuyerFeast(request.FeastID)
	if !found {
		return request, source, GameData.AutoBuyerFeast{}, fmt.Errorf("feast %d is not in the supported official catalog", request.FeastID)
	}
	if !autoBuyerIntentLevelEligible(input.State.Player, feast.MinLevel, feast.MaxLevel, 0, 0) {
		return request, source, feast, fmt.Errorf("%s is not available at the current player level", feast.Name)
	}
	if input.State.Market.BoostersObservedAt.IsZero() || now.Sub(input.State.Market.BoostersObservedAt) >= time.Duration(request.HistoryRefreshSec)*time.Second {
		return request, source, feast, fmt.Errorf("%w: feast timer is stale", Intent.ErrPlanStale)
	}
	current := input.State.Market.Feast
	if current.ID != request.ExpectedActiveFeastID || !autoBuyerIntentExpiryMatches(current.ExpiresAt, request.ExpectedExpiresAtUnix) {
		return request, source, feast, fmt.Errorf("%w: active feast changed", Intent.ErrPlanStale)
	}
	if current.ActiveAt(now) && current.ID != feast.ID {
		return request, source, feast, fmt.Errorf("%w: feast %d is already active", Intent.ErrPlanStale, current.ID)
	}
	if autoBuyerIntentRemaining(current.ExpiresAt, now) >= int64(request.MinimumRemainingHours)*60*60 {
		return request, source, feast, fmt.Errorf("%w: %s already meets its configured floor", Intent.ErrPlanStale, feast.Name)
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, feast.Price)
	if !available {
		return request, source, feast, fmt.Errorf("%s balance is unavailable", feast.Price.Name)
	}
	reserve := request.MinimumFoodReserve
	if feast.Price.Premium {
		reserve = request.MinimumRubyReserve
		if !request.AllowRubies || request.MaximumRubyCostPerPurchase < feast.Price.Amount {
			return request, source, feast, fmt.Errorf("%s is not permitted within the configured ruby ceiling", feast.Name)
		}
	}
	if balance-reserve < feast.Price.Amount {
		return request, source, feast, fmt.Errorf("%w: %s requires %d %s above reserve", Intent.ErrPlanStale, feast.Name, feast.Price.Amount, feast.Price.Name)
	}
	return request, source, feast, nil
}

func (application *Application) guardAutoBuyerPackagePurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	_, _, _, err = autoBuyerPackagePurchaseContext(input, arguments, time.Now().UTC(), true)
	return err
}

func (application *Application) verifyAutoBuyerPackagePurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	var request autoBuyerPackagePurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	product, found := input.GameData.AutoBuyerPackage(strings.TrimSpace(request.ShopID), int64(request.PackageID))
	if !found {
		return fmt.Errorf("package %d disappeared from the official Auto Buyer catalog", request.PackageID)
	}
	purchased := input.State.Inventory.ConstructionOffers[request.PackageID]
	if purchased < request.ExpectedPurchasedBefore+request.Amount {
		return fmt.Errorf("package %d purchase was not confirmed by the server counter", request.PackageID)
	}
	source, sourceErr := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if sourceErr != nil {
		return sourceErr
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, product.Price)
	if !available {
		return fmt.Errorf("%s balance is unavailable after package purchase", product.Price.Name)
	}
	reserve := request.MinimumBalanceReserve
	if product.Price.Premium {
		reserve = max(reserve, request.MinimumRubyReserve)
	}
	if balance < reserve {
		return fmt.Errorf("package purchase left %d %s below configured reserve %d", balance, product.Price.Name, reserve)
	}
	maximumCost := request.Amount * product.Price.Amount
	if request.ExpectedBalanceBefore > balance && request.ExpectedBalanceBefore-balance > maximumCost {
		return fmt.Errorf("package purchase consumed more %s than the official guarded cost", product.Price.Name)
	}
	return nil
}

func (application *Application) guardAutoBuyerSpecialistPurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	_, _, err = autoBuyerSpecialistPurchaseContext(input, arguments, time.Now().UTC())
	return err
}

func (application *Application) verifyAutoBuyerSpecialistPurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	var request autoBuyerSpecialistPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	specialist, found := GameData.AutoBuyerSpecialistByID(request.SpecialistID)
	if !found {
		return fmt.Errorf("specialist %d is no longer supported", request.SpecialistID)
	}
	booster := input.State.Market.Boosters[request.SpecialistID]
	oldExpiry := time.Unix(request.ExpectedExpiresAtUnix, 0).UTC()
	baseline := time.Now().UTC()
	if oldExpiry.After(baseline) {
		baseline = oldExpiry
	}
	minimumExpiry := baseline.Add(time.Duration(specialist.DurationSec)*time.Second - time.Minute)
	if booster.ExpiresAt.Before(minimumExpiry) {
		return fmt.Errorf("%s renewal was not confirmed by the refreshed specialist timer", specialist.Name)
	}
	rubies := int64(math.Floor(playerResourceByOfficialKey(input.State, input.GameData, "C2")))
	if rubies < request.MinimumRubyReserve {
		return fmt.Errorf("%s renewal left rubies below the configured reserve", specialist.Name)
	}
	if request.ExpectedRubyBalance > rubies && request.ExpectedRubyBalance-rubies > request.MaximumRubyCostPerPurchase {
		return fmt.Errorf("%s renewal exceeded the configured ruby ceiling", specialist.Name)
	}
	return nil
}

func (application *Application) guardAutoBuyerFeastPurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	_, _, _, err = autoBuyerFeastPurchaseContext(input, arguments, time.Now().UTC())
	return err
}

func (application *Application) verifyAutoBuyerFeastPurchase(_ context.Context, arguments json.RawMessage) error {
	input, err := application.autoBuyerPlanningContext()
	if err != nil {
		return err
	}
	var request autoBuyerFeastPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	feast, found := input.GameData.AutoBuyerFeast(request.FeastID)
	if !found {
		return fmt.Errorf("feast %d disappeared from the official catalog", request.FeastID)
	}
	current := input.State.Market.Feast
	oldExpiry := time.Unix(request.ExpectedExpiresAtUnix, 0).UTC()
	baseline := time.Now().UTC()
	if oldExpiry.After(baseline) {
		baseline = oldExpiry
	}
	minimumExpiry := baseline.Add(time.Duration(feast.DurationSec)*time.Second - time.Minute)
	if current.ID != feast.ID || current.ExpiresAt.Before(minimumExpiry) {
		return fmt.Errorf("%s purchase was not confirmed by the refreshed feast timer", feast.Name)
	}
	source, sourceErr := autoBuyerIntentSourceCastle(input.State, request.SourceCastleID)
	if sourceErr != nil {
		return sourceErr
	}
	balance, available := autoBuyerIntentPriceBalance(input.State, source, feast.Price)
	if !available {
		return fmt.Errorf("%s balance is unavailable after feast purchase", feast.Price.Name)
	}
	reserve := request.MinimumFoodReserve
	ceiling := int64(0)
	if feast.Price.Premium {
		reserve = request.MinimumRubyReserve
		ceiling = request.MaximumRubyCostPerPurchase
	}
	if balance < reserve {
		return fmt.Errorf("%s purchase left %s below the configured reserve", feast.Name, feast.Price.Name)
	}
	if request.ExpectedBalanceBefore > balance && request.ExpectedBalanceBefore-balance > feast.Price.Amount {
		return fmt.Errorf("%s purchase consumed more %s than the official guarded cost", feast.Name, feast.Price.Name)
	}
	if ceiling > 0 && request.ExpectedBalanceBefore > balance && request.ExpectedBalanceBefore-balance > ceiling {
		return fmt.Errorf("%s purchase exceeded the configured ruby ceiling", feast.Name)
	}
	return nil
}

func (application *Application) autoBuyerPlanningContext() (Intent.PlanningContext, error) {
	if application == nil || application.State == nil || application.GameData == nil {
		return Intent.PlanningContext{}, fmt.Errorf("Auto Buyer state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Intent.PlanningContext{}, fmt.Errorf("official game data is unavailable")
	}
	return Intent.PlanningContext{State: application.State.Snapshot(), GameData: gameData}, nil
}

func autoBuyerIntentSourceCastle(gameState State.GameState, castleID State.CastleID) (State.CastleState, error) {
	castle, found := gameState.Castles[castleID]
	if castleID <= 0 || !found || castle.KingdomID != 0 || castle.SlotType != 1 {
		return State.CastleState{}, fmt.Errorf("%w: castle %d is not the owned Great Empire main castle", Intent.ErrPlanStale, castleID)
	}
	return castle, nil
}

func autoBuyerIntentPriceBalance(gameState State.GameState, source State.CastleState, price GameData.AutoBuyerPrice) (int64, bool) {
	switch price.Scope {
	case GameData.AutoBuyerPricePlayerResource:
		balance, found := gameState.Player.Resources[State.ResourceID(price.ResourceID)]
		return int64(math.Floor(balance)), found && price.ResourceID > 0
	case GameData.AutoBuyerPriceCastleResource:
		balance, found := source.Resources[State.ResourceID(price.ResourceID)]
		return int64(math.Floor(balance.Amount)), found && price.ResourceID > 0
	case GameData.AutoBuyerPriceCurrency:
		balance, found := gameState.Player.Currencies[State.CurrencyID(price.CurrencyID)]
		return int64(math.Floor(balance)), found && price.CurrencyID > 0
	default:
		return 0, false
	}
}

func autoBuyerIntentLevelEligible(player State.PlayerState, minLevel, maxLevel, minLegend, maxLegend int64) bool {
	if minLevel > 0 && int64(player.Level) < minLevel || maxLevel > 0 && int64(player.Level) > maxLevel {
		return false
	}
	if minLegend > 0 && int64(player.LegendLevel) < minLegend || maxLegend > 0 && int64(player.LegendLevel) > maxLegend {
		return false
	}
	return true
}

func autoBuyerIntentRemaining(expiresAt time.Time, now time.Time) int64 {
	if expiresAt.IsZero() || !expiresAt.After(now) {
		return 0
	}
	return int64(expiresAt.Sub(now) / time.Second)
}

func autoBuyerIntentUnix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func autoBuyerIntentExpiryMatches(actual time.Time, expectedUnix int64) bool {
	if expectedUnix == 0 {
		return actual.IsZero()
	}
	if actual.IsZero() {
		return false
	}
	delta := actual.Unix() - expectedUnix
	return delta >= -5 && delta <= 5
}

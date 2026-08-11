package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoBuyerSection                 = "automation.autoBuyer"
	autoBuyerDefaultCheckIntervalSec = 60
	autoBuyerDefaultRefreshSec       = 900
	autoBuyerMinimumSpecialistDays   = 14
)

type AutoBuyerPolicy struct{}

type autoBuyerSettings struct {
	Version            int                       `json:"version"`
	CheckIntervalSec   int                       `json:"checkIntervalSec"`
	HistoryRefreshSec  int                       `json:"historyRefreshSec"`
	SourceCastleID     State.CastleID            `json:"sourceCastleId"`
	MinimumRubyReserve int64                     `json:"minimumRubyReserve"`
	AllowRubyPackages  bool                      `json:"allowRubyPackages"`
	Packages           []autoBuyerPackageRule    `json:"packages"`
	Specialists        []autoBuyerSpecialistRule `json:"specialists"`
	Feast              autoBuyerFeastSettings    `json:"feast"`
}

type autoBuyerPackageRule struct {
	Enabled                  bool            `json:"enabled"`
	ShopID                   string          `json:"shopId"`
	PackageID                State.PackageID `json:"packageId"`
	TargetPurchasesPerReset  int64           `json:"targetPurchasesPerReset"`
	MinimumBalanceReserve    int64           `json:"minimumBalanceReserve"`
	MaximumRubySpendPerReset int64           `json:"maximumRubySpendPerReset"`
}

type autoBuyerSpecialistRule struct {
	Enabled                    bool  `json:"enabled"`
	ID                         int   `json:"id"`
	MinimumDays                int   `json:"minimumDays"`
	MaximumRubyCostPerPurchase int64 `json:"maximumRubyCostPerPurchase"`
}

type autoBuyerFeastSettings struct {
	Enabled                    bool           `json:"enabled"`
	FeastID                    int64          `json:"feastId"`
	MinimumRemainingHours      int            `json:"minimumRemainingHours"`
	SourceCastleID             State.CastleID `json:"sourceCastleId"`
	MinimumFoodReserve         int64          `json:"minimumFoodReserve"`
	AllowRubies                bool           `json:"allowRubies"`
	MaximumRubyCostPerPurchase int64          `json:"maximumRubyCostPerPurchase"`
}

func NewAutoBuyerPolicy() *AutoBuyerPolicy { return &AutoBuyerPolicy{} }

func (*AutoBuyerPolicy) ID() string         { return "autoBuyer" }
func (*AutoBuyerPolicy) EnabledKey() string { return "auto_buyer" }

func (*AutoBuyerPolicy) WakeDomains() []string {
	return []string{
		"boosters", "market", "construction-offers", "currencies", "events",
		"event-scores", "inventory", "resources", "castles",
	}
}

func (*AutoBuyerPolicy) WakeSections() []string { return []string{autoBuyerSection} }

func (*AutoBuyerPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := autoBuyerSettings{
		Version: 1, CheckIntervalSec: autoBuyerDefaultCheckIntervalSec,
		HistoryRefreshSec: autoBuyerDefaultRefreshSec,
	}
	if !decodeSection(snapshot.Configuration, autoBuyerSection, &settings) {
		return autoBuyerWaiting(snapshot.Now, "Auto Buyer settings have not been saved", nil), nil
	}
	if settings.Version != 1 {
		return autoBuyerWaiting(snapshot.Now, fmt.Sprintf("Unsupported Auto Buyer settings version %d", settings.Version), nil), nil
	}
	if settings.CheckIntervalSec <= 0 {
		settings.CheckIntervalSec = autoBuyerDefaultCheckIntervalSec
	}
	if settings.HistoryRefreshSec <= 0 {
		settings.HistoryRefreshSec = autoBuyerDefaultRefreshSec
	}
	if settings.CheckIntervalSec < 10 || settings.CheckIntervalSec > 3600 ||
		settings.HistoryRefreshSec < 60 || settings.HistoryRefreshSec > 3600 ||
		settings.MinimumRubyReserve < 0 {
		return autoBuyerWaiting(snapshot.Now, "Auto Buyer cadence and ruby reserve settings are invalid", nil), nil
	}
	if snapshot.GameData == nil {
		return autoBuyerWaiting(snapshot.Now, "Official game data is unavailable", nil), nil
	}
	if _, err := snapshot.GameData.AutoBuyerCatalog(); err != nil {
		return autoBuyerWaiting(snapshot.Now, "The supported Auto Buyer catalog is unavailable", nil), nil
	}

	enabledPackages, enabledSpecialists := 0, 0
	for _, rule := range settings.Packages {
		if rule.Enabled {
			enabledPackages++
		}
	}
	for _, rule := range settings.Specialists {
		if rule.Enabled {
			enabledSpecialists++
		}
	}
	metrics := map[string]float64{
		"enabledPackages":    float64(enabledPackages),
		"enabledSpecialists": float64(enabledSpecialists),
		"feastEnabled":       boolMetric(settings.Feast.Enabled),
	}
	if enabledPackages == 0 && enabledSpecialists == 0 && !settings.Feast.Enabled {
		return autoBuyerIdle(snapshot.Now, settings.CheckIntervalSec, "No Auto Buyer goals are enabled", metrics), nil
	}

	if detail := validateAutoBuyerRules(snapshot.GameData, settings); detail != "" {
		return autoBuyerWaiting(snapshot.Now, detail, metrics), nil
	}

	var source State.CastleState
	var sourceFound bool
	if enabledPackages > 0 || settings.Feast.Enabled {
		sourceCastleID := settings.SourceCastleID
		if enabledPackages == 0 && settings.Feast.SourceCastleID > 0 {
			sourceCastleID = settings.Feast.SourceCastleID
		}
		source, sourceFound = autoBuyerSourceCastle(snapshot.State, sourceCastleID)
		if !sourceFound {
			return autoBuyerWaiting(snapshot.Now, "Choose an owned Great Empire main castle for Auto Buyer", metrics), nil
		}
		metrics["sourceCastleId"] = float64(source.ID)
	}

	refreshAge := time.Duration(settings.HistoryRefreshSec) * time.Second
	if enabledPackages > 0 && (snapshot.State.Inventory.ConstructionOffersCastleID != source.ID ||
		snapshot.State.Inventory.ConstructionOffersKingdomID != source.KingdomID ||
		snapshot.State.Inventory.ConstructionOffersObservedAt.IsZero() ||
		snapshot.Now.Sub(snapshot.State.Inventory.ConstructionOffersObservedAt) >= refreshAge) {
		return autoBuyerRequestDecision(snapshot.Now, metrics, "Refresh shop stock and reset counters", "autoBuyer.package.history", map[string]any{
			"sourceCastleId": source.ID,
		}), nil
	}

	if enabledSpecialists > 0 || settings.Feast.Enabled {
		if snapshot.State.Market.BoostersObservedAt.IsZero() ||
			snapshot.Now.Sub(snapshot.State.Market.BoostersObservedAt) >= refreshAge {
			return autoBuyerRequestDecision(snapshot.Now, metrics, "Refresh specialist and feast timers", "autoBuyer.boosters.refresh", map[string]any{}), nil
		}
	}

	blockedDetail := ""
	if decision, detail := evaluateAutoBuyerSpecialists(snapshot, settings, metrics); decision != nil {
		return *decision, nil
	} else if detail != "" {
		metrics["specialistBlocked"] = 1
		blockedDetail = detail
	}

	if decision, detail := evaluateAutoBuyerFeast(snapshot, settings, source, metrics); decision != nil {
		return *decision, nil
	} else if detail != "" {
		metrics["feastBlocked"] = 1
		if blockedDetail == "" {
			blockedDetail = detail
		}
	}

	if decision, detail := evaluateAutoBuyerPackages(snapshot, settings, source, metrics); decision != nil {
		return *decision, nil
	} else if detail != "" {
		if blockedDetail == "" {
			blockedDetail = detail
		}
	}
	if blockedDetail != "" {
		return autoBuyerWaiting(snapshot.Now, blockedDetail, metrics), nil
	}

	return autoBuyerIdle(snapshot.Now, settings.CheckIntervalSec, "All configured purchase floors and reset goals are currently satisfied", metrics), nil
}

func validateAutoBuyerRules(store *GameData.Store, settings autoBuyerSettings) string {
	seenPackages := map[string]struct{}{}
	for _, rule := range settings.Packages {
		if !rule.Enabled {
			continue
		}
		shopID := strings.TrimSpace(rule.ShopID)
		key := fmt.Sprintf("%s:%d", shopID, rule.PackageID)
		if _, duplicate := seenPackages[key]; duplicate {
			return fmt.Sprintf("Package %d is configured more than once for %s", rule.PackageID, shopID)
		}
		seenPackages[key] = struct{}{}
		product, found := store.AutoBuyerPackage(shopID, int64(rule.PackageID))
		if !found {
			return fmt.Sprintf("Package %d is not in the supported %s Auto Buyer catalog", rule.PackageID, shopID)
		}
		if rule.TargetPurchasesPerReset <= 0 || rule.TargetPurchasesPerReset > product.Stock {
			return fmt.Sprintf("%s target must be between 1 and its stock limit %d", product.Name, product.Stock)
		}
		if rule.MinimumBalanceReserve < 0 || rule.MaximumRubySpendPerReset < 0 {
			return fmt.Sprintf("%s reserve and ruby ceiling cannot be negative", product.Name)
		}
		if product.Price.Premium && rule.MaximumRubySpendPerReset < product.Price.Amount {
			return fmt.Sprintf("%s needs an explicit ruby ceiling of at least %d", product.Name, product.Price.Amount)
		}
	}

	seenSpecialists := map[int]struct{}{}
	for _, rule := range settings.Specialists {
		if !rule.Enabled {
			continue
		}
		if _, duplicate := seenSpecialists[rule.ID]; duplicate {
			return fmt.Sprintf("Specialist %d is configured more than once", rule.ID)
		}
		seenSpecialists[rule.ID] = struct{}{}
		specialist, found := GameData.AutoBuyerSpecialistByID(rule.ID)
		if !found {
			return fmt.Sprintf("Specialist %d is not supported for automatic renewal", rule.ID)
		}
		if rule.MinimumDays < autoBuyerMinimumSpecialistDays || rule.MinimumDays > 365 {
			return fmt.Sprintf("%s floor must be between %d and 365 days", specialist.Name, autoBuyerMinimumSpecialistDays)
		}
		if rule.MaximumRubyCostPerPurchase < specialist.BaseRubyCost {
			return fmt.Sprintf("%s ruby ceiling must cover its safe maximum cost of %d", specialist.Name, specialist.BaseRubyCost)
		}
	}

	if settings.Feast.Enabled {
		feast, found := store.AutoBuyerFeast(settings.Feast.FeastID)
		if !found {
			return fmt.Sprintf("Feast %d is not in the supported official catalog", settings.Feast.FeastID)
		}
		if settings.Feast.MinimumRemainingHours <= 0 || settings.Feast.MinimumRemainingHours > 24*30 ||
			settings.Feast.MinimumFoodReserve < 0 || settings.Feast.MaximumRubyCostPerPurchase < 0 {
			return fmt.Sprintf("%s duration, reserve, or ruby ceiling is invalid", feast.Name)
		}
		if feast.Price.Premium && (!settings.Feast.AllowRubies || settings.Feast.MaximumRubyCostPerPurchase < feast.Price.Amount) {
			return fmt.Sprintf("%s needs explicit ruby permission and a ceiling of at least %d", feast.Name, feast.Price.Amount)
		}
	}
	return ""
}

func evaluateAutoBuyerSpecialists(
	snapshot Snapshot,
	settings autoBuyerSettings,
	metrics map[string]float64,
) (*Decision, string) {
	firstBlocked := ""
	for _, rule := range settings.Specialists {
		if !rule.Enabled {
			continue
		}
		specialist, _ := GameData.AutoBuyerSpecialistByID(rule.ID)
		booster := snapshot.State.Market.Boosters[rule.ID]
		remaining := autoBuyerBoosterRemaining(booster, snapshot.Now)
		floor := int64(rule.MinimumDays) * 24 * 60 * 60
		metrics[fmt.Sprintf("specialist.%d.remainingSec", rule.ID)] = float64(remaining)
		if remaining >= floor {
			continue
		}
		rubies := int64(math.Floor(playerResourceAmount(snapshot, "C2")))
		if rubies-settings.MinimumRubyReserve < specialist.BaseRubyCost {
			if firstBlocked == "" {
				firstBlocked = fmt.Sprintf("Waiting for %d rubies above reserve to renew %s", specialist.BaseRubyCost, specialist.Name)
			}
			continue
		}
		arguments := map[string]any{
			"specialistId": rule.ID, "minimumDays": rule.MinimumDays,
			"maximumRubyCostPerPurchase": rule.MaximumRubyCostPerPurchase,
			"minimumRubyReserve":         settings.MinimumRubyReserve,
			"expectedExpiresAtUnix":      autoBuyerUnix(booster.ExpiresAt),
			"expectedPurchaseCount":      booster.ContinuousPurchaseCount,
			"expectedRubyBalance":        rubies,
			"historyRefreshSec":          settings.HistoryRefreshSec,
		}
		decision := autoBuyerRequestDecision(snapshot.Now, metrics,
			fmt.Sprintf("Renew %s by 7 days toward the %d-day floor", specialist.Name, rule.MinimumDays),
			"autoBuyer.specialist.purchase", arguments)
		return &decision, ""
	}
	return nil, firstBlocked
}

func evaluateAutoBuyerFeast(
	snapshot Snapshot,
	settings autoBuyerSettings,
	defaultSource State.CastleState,
	metrics map[string]float64,
) (*Decision, string) {
	if !settings.Feast.Enabled {
		return nil, ""
	}
	feast, _ := snapshot.GameData.AutoBuyerFeast(settings.Feast.FeastID)
	source := defaultSource
	if settings.Feast.SourceCastleID > 0 && settings.Feast.SourceCastleID != defaultSource.ID {
		var found bool
		source, found = autoBuyerSourceCastle(snapshot.State, settings.Feast.SourceCastleID)
		if !found {
			return nil, "Choose an owned Great Empire main castle for the feast"
		}
	}
	if !autoBuyerLevelEligible(snapshot.State.Player, feast.MinLevel, feast.MaxLevel, 0, 0) {
		return nil, fmt.Sprintf("%s is not available at the current player level", feast.Name)
	}
	current := snapshot.State.Market.Feast
	remaining := autoBuyerFeastRemaining(current, snapshot.Now)
	metrics["feastRemainingSec"] = float64(remaining)
	if current.ActiveAt(snapshot.Now) && current.ID != feast.ID {
		return nil, fmt.Sprintf("Waiting for active feast %d to end before starting %s", current.ID, feast.Name)
	}
	floor := int64(settings.Feast.MinimumRemainingHours) * 60 * 60
	if remaining >= floor {
		return nil, ""
	}
	balance, available := autoBuyerPriceBalance(snapshot.State, source, feast.Price)
	if !available {
		return nil, fmt.Sprintf("%s balance is unavailable", feast.Price.Name)
	}
	reserve := settings.Feast.MinimumFoodReserve
	if feast.Price.Premium {
		reserve = settings.MinimumRubyReserve
		if !settings.Feast.AllowRubies || feast.Price.Amount > settings.Feast.MaximumRubyCostPerPurchase {
			return nil, fmt.Sprintf("%s exceeds the configured ruby ceiling", feast.Name)
		}
	}
	if balance-reserve < feast.Price.Amount {
		return nil, fmt.Sprintf("Waiting for %d %s above reserve to start or extend %s", feast.Price.Amount, feast.Price.Name, feast.Name)
	}
	arguments := map[string]any{
		"feastId": feast.ID, "minimumRemainingHours": settings.Feast.MinimumRemainingHours,
		"sourceCastleId": source.ID, "minimumFoodReserve": settings.Feast.MinimumFoodReserve,
		"allowRubies":                settings.Feast.AllowRubies,
		"maximumRubyCostPerPurchase": settings.Feast.MaximumRubyCostPerPurchase,
		"minimumRubyReserve":         settings.MinimumRubyReserve,
		"expectedActiveFeastId":      current.ID,
		"expectedExpiresAtUnix":      autoBuyerUnix(current.ExpiresAt),
		"expectedBalanceBefore":      balance,
		"historyRefreshSec":          settings.HistoryRefreshSec,
	}
	decision := autoBuyerRequestDecision(snapshot.Now, metrics,
		fmt.Sprintf("Start or extend %s toward the %d-hour floor", feast.Name, settings.Feast.MinimumRemainingHours),
		"autoBuyer.feast.purchase", arguments)
	return &decision, ""
}

func evaluateAutoBuyerPackages(
	snapshot Snapshot,
	settings autoBuyerSettings,
	source State.CastleState,
	metrics map[string]float64,
) (*Decision, string) {
	firstBlocked := ""
	for _, rule := range settings.Packages {
		if !rule.Enabled {
			continue
		}
		product, _ := snapshot.GameData.AutoBuyerPackage(strings.TrimSpace(rule.ShopID), int64(rule.PackageID))
		if !autoBuyerLevelEligible(snapshot.State.Player, product.MinLevel, product.MaxLevel, product.MinLegendLevel, product.MaxLegendLevel) {
			if firstBlocked == "" {
				firstBlocked = fmt.Sprintf("%s is not available at the current player level", product.Name)
			}
			continue
		}
		if product.RequiresEvent {
			if _, active := snapshot.State.ActiveShopForPackage(rule.PackageID, snapshot.Now); !active {
				if firstBlocked == "" {
					firstBlocked = fmt.Sprintf("Waiting for the event shop that sells %s", product.Name)
				}
				continue
			}
		}
		purchased := snapshot.State.Inventory.ConstructionOffers[rule.PackageID]
		target := min(rule.TargetPurchasesPerReset, product.Stock)
		if purchased >= target || purchased >= product.Stock {
			continue
		}
		amount := min(target-purchased, product.Stock-purchased)
		if product.MaxBuyPerClick > 0 {
			amount = min(amount, product.MaxBuyPerClick)
		}
		balance, available := autoBuyerPriceBalance(snapshot.State, source, product.Price)
		if !available {
			if firstBlocked == "" {
				firstBlocked = fmt.Sprintf("%s balance is unavailable", product.Price.Name)
			}
			continue
		}
		reserve := rule.MinimumBalanceReserve
		if product.Price.Premium {
			if !settings.AllowRubyPackages {
				if firstBlocked == "" {
					firstBlocked = fmt.Sprintf("Ruby shop purchases are disabled for %s", product.Name)
				}
				continue
			}
			reserve = max(reserve, settings.MinimumRubyReserve)
			spentBudget := purchased * product.Price.Amount
			remainingBudget := rule.MaximumRubySpendPerReset - spentBudget
			if remainingBudget <= 0 {
				if firstBlocked == "" {
					firstBlocked = fmt.Sprintf("%s reached its ruby ceiling for this stock reset", product.Name)
				}
				continue
			}
			amount = min(amount, remainingBudget/product.Price.Amount)
		}
		spendable := balance - reserve
		if spendable > 0 {
			amount = min(amount, spendable/product.Price.Amount)
		} else {
			amount = 0
		}
		if amount <= 0 {
			if firstBlocked == "" {
				firstBlocked = fmt.Sprintf("Waiting for %d %s above reserve to buy %s", product.Price.Amount, product.Price.Name, product.Name)
			}
			continue
		}
		arguments := map[string]any{
			"sourceCastleId": source.ID, "shopId": product.ShopID, "packageId": product.PackageID,
			"amount": amount, "targetPurchasesPerReset": rule.TargetPurchasesPerReset,
			"minimumBalanceReserve":    rule.MinimumBalanceReserve,
			"allowRubyPackages":        settings.AllowRubyPackages,
			"maximumRubySpendPerReset": rule.MaximumRubySpendPerReset,
			"minimumRubyReserve":       settings.MinimumRubyReserve,
			"expectedPurchasedBefore":  purchased,
			"expectedBalanceBefore":    balance,
		}
		decision := autoBuyerRequestDecision(snapshot.Now, metrics,
			fmt.Sprintf("Buy %d x %s for %d %s", amount, product.Name, amount*product.Price.Amount, product.Price.Name),
			"autoBuyer.package.purchase", arguments)
		return &decision, ""
	}
	return nil, firstBlocked
}

func autoBuyerSourceCastle(gameState State.GameState, configured State.CastleID) (State.CastleState, bool) {
	if configured > 0 {
		castle, found := gameState.Castles[configured]
		return castle, found && castle.KingdomID == 0 && castle.SlotType == 1
	}
	var selected State.CastleState
	for _, castle := range gameState.Castles {
		if castle.KingdomID != 0 || castle.SlotType != 1 || selected.ID > 0 && castle.ID >= selected.ID {
			continue
		}
		selected = castle
	}
	return selected, selected.ID > 0
}

func autoBuyerLevelEligible(player State.PlayerState, minLevel, maxLevel, minLegend, maxLegend int64) bool {
	if minLevel > 0 && int64(player.Level) < minLevel || maxLevel > 0 && int64(player.Level) > maxLevel {
		return false
	}
	if minLegend > 0 && int64(player.LegendLevel) < minLegend || maxLegend > 0 && int64(player.LegendLevel) > maxLegend {
		return false
	}
	return true
}

func autoBuyerPriceBalance(gameState State.GameState, source State.CastleState, price GameData.AutoBuyerPrice) (int64, bool) {
	switch price.Scope {
	case GameData.AutoBuyerPricePlayerResource:
		return int64(math.Floor(gameState.Player.Resources[State.ResourceID(price.ResourceID)])), price.ResourceID > 0
	case GameData.AutoBuyerPriceCastleResource:
		balance, found := source.Resources[State.ResourceID(price.ResourceID)]
		return int64(math.Floor(balance.Amount)), found && price.ResourceID > 0
	case GameData.AutoBuyerPriceCurrency:
		_, found := gameState.Player.Currencies[State.CurrencyID(price.CurrencyID)]
		return int64(math.Floor(gameState.Player.Currencies[State.CurrencyID(price.CurrencyID)])), found && price.CurrencyID > 0
	default:
		return 0, false
	}
}

func autoBuyerBoosterRemaining(booster State.MarketBoosterState, now time.Time) int64 {
	if booster.Permanent {
		return math.MaxInt32
	}
	if booster.ExpiresAt.IsZero() || !booster.ExpiresAt.After(now) {
		return 0
	}
	return int64(booster.ExpiresAt.Sub(now) / time.Second)
}

func autoBuyerFeastRemaining(feast State.MarketFeastState, now time.Time) int64 {
	if !feast.ActiveAt(now) {
		return 0
	}
	return int64(feast.ExpiresAt.Sub(now) / time.Second)
}

func autoBuyerUnix(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.Unix()
}

func autoBuyerRequestDecision(now time.Time, metrics map[string]float64, detail, name string, arguments any) Decision {
	raw, _ := json.Marshal(arguments)
	return Decision{
		Status: "ready", Detail: detail, Metrics: metrics, NextCheckAt: now.Add(2 * time.Second),
		Request: &Intent.Request{Name: name, Arguments: raw}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}

func autoBuyerWaiting(now time.Time, detail string, metrics map[string]float64) Decision {
	return Decision{Status: "waiting", Detail: detail, Metrics: metrics, NextCheckAt: now.Add(30 * time.Second)}
}

func autoBuyerIdle(now time.Time, intervalSec int, detail string, metrics map[string]float64) Decision {
	return Decision{
		Status: "idle", Detail: detail, Metrics: metrics,
		NextCheckAt: now.Add(policyInterval(intervalSec, autoBuyerDefaultCheckIntervalSec)),
	}
}

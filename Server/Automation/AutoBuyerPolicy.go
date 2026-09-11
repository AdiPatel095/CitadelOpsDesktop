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
	autoBuyerDefaultCheckIntervalSec = 30 * 60
	autoBuyerDefaultRefreshSec       = 60 * 60
	autoBuyerMinimumSpecialistDays   = 14
	autoBuyerFeastPurchasePacing     = 30 * time.Second
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
	// Resource, currency, inventory, and castle balance churn is deliberately
	// sampled on CheckIntervalSec. Shop/reset observations and event routes are
	// the only state changes that can invalidate the current passive decision
	// before its configured 30-minute-or-longer cadence.
	return []string{"boosters", "market", "construction-offers", "events", "event-scores"}
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
	if settings.CheckIntervalSec < autoBuyerDefaultCheckIntervalSec {
		settings.CheckIntervalSec = autoBuyerDefaultCheckIntervalSec
	}
	if settings.HistoryRefreshSec < autoBuyerDefaultRefreshSec {
		settings.HistoryRefreshSec = autoBuyerDefaultRefreshSec
	}
	if settings.CheckIntervalSec > 3600 || settings.HistoryRefreshSec > 3600 ||
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
	availablePackageGoals, unavailableEventShopGoals := autoBuyerAvailablePackageGoals(snapshot, settings)
	metrics["availablePackageGoals"] = float64(availablePackageGoals)
	metrics["ignoredUnavailableEventShops"] = float64(unavailableEventShopGoals)

	refreshAge := time.Duration(settings.HistoryRefreshSec) * time.Second
	specialistContextStale := enabledSpecialists > 0 && !autoBuyerObservationFresh(
		snapshot.State.Market.BoostersObservedAt, snapshot.Now, snapshot.State.Session.ChangedAt, refreshAge,
	)
	feastContextStale := settings.Feast.Enabled && !snapshot.State.Market.Feast.FreshAt(
		snapshot.Now, snapshot.State.Session.ChangedAt, refreshAge,
	)
	if settings.Feast.Enabled {
		feast, _ := snapshot.GameData.AutoBuyerFeast(settings.Feast.FeastID)
		feastContextStale = feastContextStale || !feast.Price.Premium && !autoBuyerObservationFresh(
			snapshot.State.Market.FeastCostReductionObservedAt,
			snapshot.Now,
			snapshot.State.Session.ChangedAt,
			refreshAge,
		)
	}
	if specialistContextStale || feastContextStale {
		decision := autoBuyerRequestDecision(snapshot.Now, metrics, "Refresh specialist and feast context", "autoBuyer.boosters.refresh", map[string]any{
			"feastContext": settings.Feast.Enabled,
		})
		if settings.Feast.Enabled {
			decision.ReevaluateOnStale = false
			decision.NextCheckAt = snapshot.Now.Add(30 * time.Second)
		}
		return decision, nil
	}

	blockedDetail := ""
	if decision, detail := evaluateAutoBuyerSpecialists(snapshot, settings, metrics); decision != nil {
		return *decision, nil
	} else if detail != "" {
		metrics["specialistBlocked"] = 1
		blockedDetail = detail
	}

	var feastSource State.CastleState
	if settings.Feast.Enabled {
		feastSourceID := settings.Feast.SourceCastleID
		if feastSourceID <= 0 {
			feastSourceID = settings.SourceCastleID
		}
		var feastSourceFound bool
		feastSource, feastSourceFound = autoBuyerSourceCastle(snapshot.State, feastSourceID)
		if !feastSourceFound {
			metrics["feastBlocked"] = 1
			if blockedDetail == "" {
				blockedDetail = "Choose an owned Great Empire main castle for the feast"
			}
		} else {
			metrics["feastSourceCastleId"] = float64(feastSource.ID)
			if availablePackageGoals == 0 {
				metrics["sourceCastleId"] = float64(feastSource.ID)
			}
			if decision, detail := evaluateAutoBuyerFeast(snapshot, settings, feastSource, metrics); decision != nil {
				return *decision, nil
			} else if detail != "" {
				metrics["feastBlocked"] = 1
				if blockedDetail == "" {
					blockedDetail = detail
				}
			}
		}
	}

	if availablePackageGoals > 0 {
		packageSource, sourceFound := autoBuyerSourceCastle(snapshot.State, settings.SourceCastleID)
		if !sourceFound {
			if blockedDetail == "" {
				blockedDetail = "Choose an owned Great Empire main castle for Auto Buyer packages"
			}
		} else {
			metrics["sourceCastleId"] = float64(packageSource.ID)
			_, packageHistoryObservedAt, packageHistoryFound := snapshot.State.ConstructionOffersFor(packageSource.ID, packageSource.KingdomID)
			if !packageHistoryFound || packageHistoryObservedAt.IsZero() ||
				snapshot.Now.Sub(packageHistoryObservedAt) >= refreshAge {
				return autoBuyerRequestDecision(snapshot.Now, metrics, "Refresh shop stock and reset counters", "autoBuyer.package.history", map[string]any{
					"sourceCastleId": packageSource.ID,
				}), nil
			}
			if decision, detail := evaluateAutoBuyerPackages(snapshot, settings, packageSource, metrics); decision != nil {
				return *decision, nil
			} else if detail != "" && blockedDetail == "" {
				blockedDetail = detail
			}
		}
	}
	if blockedDetail != "" {
		return autoBuyerWaiting(snapshot.Now, blockedDetail, metrics), nil
	}
	if unavailableEventShopGoals > 0 && availablePackageGoals == 0 && enabledSpecialists == 0 && !settings.Feast.Enabled {
		return Decision{
			Status: "idle",
			Detail: fmt.Sprintf(
				"Ignoring %d configured event-shop goal(s) while their specific shops are unavailable",
				unavailableEventShopGoals,
			),
			NextCheckAt: limitedEventOpeningAfter(snapshot.Now), Metrics: metrics,
		}, nil
	}

	return autoBuyerIdle(snapshot.Now, settings.CheckIntervalSec, "All configured purchase floors and reset goals are currently satisfied", metrics), nil
}

func autoBuyerAvailablePackageGoals(snapshot Snapshot, settings autoBuyerSettings) (available int, unavailableEventShops int) {
	for _, rule := range settings.Packages {
		if !rule.Enabled {
			continue
		}
		product, found := snapshot.GameData.AutoBuyerPackage(strings.TrimSpace(rule.ShopID), int64(rule.PackageID))
		if !found || !autoBuyerLevelEligible(
			snapshot.State.Player, product.MinLevel, product.MaxLevel, product.MinLegendLevel, product.MaxLegendLevel,
		) {
			continue
		}
		if product.RequiresEvent {
			if _, active := snapshot.State.ActiveShopForPackage(rule.PackageID, snapshot.Now); !active {
				unavailableEventShops++
				continue
			}
		}
		available++
	}
	return available, unavailableEventShops
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
		if !feast.AutomaticPurchase.Supported {
			return feast.AutomaticPurchase.Reason
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
	if !feast.AutomaticPurchase.Supported {
		return nil, feast.AutomaticPurchase.Reason
	}
	if snapshot.State.Market.FeastPurchasePending {
		metrics["feastReconciliationPending"] = 1
		return nil, "A previous feast purchase is awaiting authoritative game reconciliation; Auto Buyer will not repeat it"
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
	if !snapshot.State.Market.FeastLastPurchaseAt.IsZero() &&
		!snapshot.State.Market.FeastLastPurchaseAt.After(snapshot.Now) {
		nextPurchaseAt := snapshot.State.Market.FeastLastPurchaseAt.Add(autoBuyerFeastPurchasePacing)
		if snapshot.Now.Before(nextPurchaseAt) {
			return &Decision{
				Status: "waiting", Detail: "Waiting briefly before extending the active feast again",
				NextCheckAt: nextPurchaseAt, Metrics: metrics,
			}, ""
		}
	}
	balance, available := autoBuyerPriceBalance(snapshot.State, source, feast.Price)
	if !available {
		return nil, fmt.Sprintf("%s balance is unavailable", feast.Price.Name)
	}
	effectiveCost := feast.Price.Amount
	if !feast.Price.Premium {
		refreshAge := time.Duration(settings.HistoryRefreshSec) * time.Second
		if !autoBuyerObservationFresh(
			snapshot.State.Market.FeastCostReductionObservedAt,
			snapshot.Now,
			snapshot.State.Session.ChangedAt,
			refreshAge,
		) {
			return nil, "Waiting for a fresh feast cost reduction before purchasing"
		}
		reductionPercent := snapshot.State.Market.FeastCostReductionPercent
		if reductionPercent < 0 || reductionPercent > 100 {
			return nil, "The current feast cost reduction is invalid"
		}
		effectiveCost = feast.EffectiveCost(reductionPercent)
		metrics["feastCostReductionPercent"] = float64(reductionPercent)
	}
	metrics["feastEffectiveCost"] = float64(effectiveCost)
	reserve := settings.Feast.MinimumFoodReserve
	if feast.Price.Premium {
		reserve = settings.MinimumRubyReserve
		if !settings.Feast.AllowRubies || effectiveCost > settings.Feast.MaximumRubyCostPerPurchase {
			return nil, fmt.Sprintf("%s exceeds the configured ruby ceiling", feast.Name)
		}
	}
	if balance-reserve < effectiveCost {
		return nil, fmt.Sprintf("Waiting for %d %s above reserve to start or extend %s", effectiveCost, feast.Price.Name, feast.Name)
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
		"expectedEffectiveCost":      effectiveCost,
		"attemptAfter":               snapshot.Now,
		"historyRefreshSec":          settings.HistoryRefreshSec,
	}
	decision := autoBuyerRequestDecision(snapshot.Now, metrics,
		fmt.Sprintf("Start or extend %s for %d %s toward the %d-hour floor", feast.Name, effectiveCost, feast.Price.Name, settings.Feast.MinimumRemainingHours),
		"autoBuyer.feast.purchase", arguments)
	decision.FailureFallback = &Intent.Request{
		Name: "autoBuyer.feast.reconcile", Arguments: append(json.RawMessage(nil), decision.Request.Arguments...),
	}
	decision.FailureDetail = "Reconciled feast state after an incomplete or uncertain purchase"
	decision.ReevaluateOnSuccess = false
	decision.NextCheckAt = snapshot.Now.Add(autoBuyerFeastPurchasePacing)
	return &decision, ""
}

func autoBuyerObservationFresh(observedAt, now, sessionChangedAt time.Time, maxAge time.Duration) bool {
	return !observedAt.IsZero() && !observedAt.After(now) &&
		(sessionChangedAt.IsZero() || !observedAt.Before(sessionChangedAt)) &&
		maxAge > 0 && now.Sub(observedAt) < maxAge
}

func evaluateAutoBuyerPackages(
	snapshot Snapshot,
	settings autoBuyerSettings,
	source State.CastleState,
	metrics map[string]float64,
) (*Decision, string) {
	firstBlocked := ""
	offers, _, _ := snapshot.State.ConstructionOffersFor(source.ID, source.KingdomID)
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
				continue
			}
		}
		purchased := offers[rule.PackageID]
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
		balance, found := gameState.Player.Resources[State.ResourceID(price.ResourceID)]
		amount, valid := autoBuyerBalanceAmount(balance)
		return amount, found && price.ResourceID > 0 && valid
	case GameData.AutoBuyerPriceCastleResource:
		balance, found := source.Resources[State.ResourceID(price.ResourceID)]
		amount, valid := autoBuyerBalanceAmount(balance.Amount)
		return amount, found && price.ResourceID > 0 && valid
	case GameData.AutoBuyerPriceCurrency:
		balance, found := gameState.Player.Currencies[State.CurrencyID(price.CurrencyID)]
		amount, valid := autoBuyerBalanceAmount(balance)
		return amount, found && price.CurrencyID > 0 && valid
	default:
		return 0, false
	}
}

func autoBuyerBalanceAmount(value float64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= float64(math.MaxInt64) {
		return 0, false
	}
	return int64(math.Floor(value)), true
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

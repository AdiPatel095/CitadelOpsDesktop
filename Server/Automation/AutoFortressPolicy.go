package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"sync"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoFortressSection                 = "automation.autoFortress"
	autoFortressDefaultCheckIntervalSec = 5
	autoFortressDefaultMapRefreshSec    = 1800
	autoFortressAttackDialogFreshness   = 30 * time.Second
	autoFortressUnitFreshness           = 5 * time.Minute
	autoFortressPurchaseHistoryAge      = 5 * time.Minute
	autoFortressMaximumSpeedPercent     = 100
)

type AutoFortressPolicy struct {
	mu                    sync.Mutex
	lastFullScanRequested map[State.KingdomID]time.Time
}

type autoFortressSettings struct {
	Version                    int                            `json:"version"`
	CheckIntervalSec           int                            `json:"checkIntervalSec"`
	MapRefreshIntervalSec      int                            `json:"mapRefreshIntervalSec"`
	DailyAttackLimit           int64                          `json:"dailyAttackLimit"`
	HorseTravelBoostID         int                            `json:"horseTravelBoostId"`
	MinimumCommanderSpeedBonus float64                        `json:"minimumCommanderSpeedBonus"`
	DirewolfPurchaseLimit      int64                          `json:"direwolfPurchaseLimit"`
	MinimumTabletReserve       int64                          `json:"minimumTabletReserve"`
	Kingdoms                   map[string]autoFortressKingdom `json:"kingdoms"`
}

type autoFortressKingdom struct {
	Enabled bool `json:"enabled"`
}

type autoFortressTarget struct {
	Source State.CastleState
	Target State.MapObservation
}

type autoFortressKingdomStats struct {
	Known     int
	Ready     int
	NextReady time.Time
}

func NewAutoFortressPolicy() *AutoFortressPolicy {
	return &AutoFortressPolicy{lastFullScanRequested: map[State.KingdomID]time.Time{}}
}

func (*AutoFortressPolicy) ID() string         { return "autoFortress" }
func (*AutoFortressPolicy) EnabledKey() string { return "auto_fortress" }

func (*AutoFortressPolicy) WakeDomains() []string {
	return []string{"attack_dialog", "attacks", "castles", "commanders", "construction-offers", "event-scores", "events", "kingdom-transport", "map-fortress", "movements", "tower-cooldowns", "units"}
}

func (*AutoFortressPolicy) WakeSections() []string {
	return []string{autoFortressSection, commanderFeatureSection}
}

func (policy *AutoFortressPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := defaultAutoFortressSettings()
	if !decodeSection(snapshot.Configuration, autoFortressSection, &settings) {
		return autoFortressWaiting(snapshot, "Auto Fortress settings have not been saved", nil), nil
	}
	if snapshot.PolicyConfigurationChanged {
		policy.mu.Lock()
		policy.lastFullScanRequested = map[State.KingdomID]time.Time{}
		policy.mu.Unlock()
	}
	if detail := validateAutoFortressSettings(settings); detail != "" {
		return autoFortressWaiting(snapshot, detail, nil), nil
	}
	if snapshot.GameData == nil {
		return autoFortressWaiting(snapshot, "Official game data is unavailable", nil), nil
	}
	definitions, err := snapshot.GameData.KingdomFortressDefinitions()
	if err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil), nil
	}
	if _, err := snapshot.GameData.FortressDirewolf(); err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil), nil
	}
	speedContract, err := snapshot.GameData.FortressRelicSpeed()
	if err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil), nil
	}
	if speedContract.RelicMaximumPercent != autoFortressMaximumSpeedPercent {
		return autoFortressWaiting(snapshot, "Official fortress commander speed contract changed; Auto Fortress is paused", nil), nil
	}
	definitionByKingdom := make(map[State.KingdomID]GameData.KingdomFortressDefinition, len(definitions))
	for _, definition := range definitions {
		definitionByKingdom[State.KingdomID(definition.KingdomID)] = definition
	}

	sources := autoFortressSources(snapshot.State, settings, definitionByKingdom)
	metrics := map[string]float64{
		"enabledKingdoms": float64(len(sources)), "direwolfPurchaseLimit": float64(settings.DirewolfPurchaseLimit),
		"minimumCommanderSpeedBonus": settings.MinimumCommanderSpeedBonus,
	}
	if len(sources) == 0 {
		return autoFortressWaiting(snapshot, "Enable at least one available outer-kingdom main castle", metrics), nil
	}

	if cooldown, found := pendingFortressCooldownRefresh(snapshot.State); found {
		source, sourceFound := sourceForKingdom(sources, cooldown.KingdomID)
		if sourceFound {
			return autoFortressRequest(snapshot, metrics, fmt.Sprintf("Refresh five-day fortress cooldown at %d:%d", cooldown.X, cooldown.Y), "fortress.target.refresh", map[string]any{
				"sourceCastleId": source.ID, "kingdomId": cooldown.KingdomID, "targetX": cooldown.X, "targetY": cooldown.Y,
			}), nil
		}
	}

	main, mainFound := autoBuyerSourceCastle(snapshot.State, 0)
	purchaseBlocked := ""
	if settings.DirewolfPurchaseLimit > 0 && mainFound {
		if decision, detail := evaluateAutoFortressPurchase(snapshot, settings, main, metrics); decision != nil {
			return *decision, nil
		} else {
			purchaseBlocked = detail
		}
	}

	candidates, nextCooldown, kingdomStats := autoFortressTargets(snapshot, sources)
	knownFortresses := 0
	for kingdomID, stats := range kingdomStats {
		knownFortresses += stats.Known
		metrics[fmt.Sprintf("knownFortressesKingdom%d", kingdomID)] = float64(stats.Known)
		metrics[fmt.Sprintf("readyFortressesKingdom%d", kingdomID)] = float64(stats.Ready)
		if !stats.NextReady.IsZero() {
			metrics[fmt.Sprintf("nextReadyAtKingdom%dUnix", kingdomID)] = float64(stats.NextReady.Unix())
		}
	}
	metrics["knownFortresses"] = float64(knownFortresses)
	metrics["readyFortresses"] = float64(len(candidates))
	if !nextCooldown.IsZero() {
		metrics["nextReadyAtUnix"] = float64(nextCooldown.Unix())
	}
	if len(candidates) == 0 {
		refreshInterval := time.Duration(settings.MapRefreshIntervalSec) * time.Second
		for _, source := range sources {
			if policy.fullScanDue(source.KingdomID, snapshot.Now, refreshInterval) {
				policy.markFullScanRequested(source.KingdomID, snapshot.Now)
				return autoFortressRequest(snapshot, metrics, fmt.Sprintf("Discover every fortress across %s", castleName(source)), "fortress.map.scan", map[string]any{
					"sourceCastleId": source.ID, "kingdomId": source.KingdomID,
				}), nil
			}
		}

		detail := "No known fortress is currently available"
		if !nextCooldown.IsZero() {
			detail = fmt.Sprintf(
				"Next known fortress is expected at %s; a targeted cooldown check will run at availability",
				nextCooldown.UTC().Format("Jan 2 15:04:05 UTC"),
			)
		}
		if purchaseBlocked != "" {
			detail += "; supply: " + purchaseBlocked
		}
		next := time.Time{}
		if !nextCooldown.IsZero() {
			next = nextCooldown
		}
		for _, source := range sources {
			fullScanAt := policy.nextFullScanAt(source.KingdomID, snapshot.Now, refreshInterval)
			if next.IsZero() || fullScanAt.Before(next) {
				next = fullScanAt
			}
		}
		if next.IsZero() || !next.After(snapshot.Now) {
			next = snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30))
		}
		return Decision{Status: "idle", Detail: detail, NextCheckAt: next, Metrics: metrics}, nil
	}

	commanderIDs, restricted := commanderFeatureCandidates(snapshot.State, snapshot.Configuration, "autoFortress")
	var candidate autoFortressTarget
	var commanderID State.CommanderID
	var speed float64
	commanderFound := false
	for _, nextCandidate := range candidates {
		commanderID, speed, commanderFound = fastestFortressCommander(snapshot, nextCandidate, commanderIDs, restricted, speedContract)
		if commanderFound {
			candidate = nextCandidate
			break
		}
	}
	if !commanderFound {
		return autoFortressWaiting(snapshot, "No available assigned commander has Relic 2.0 equipment and the maxed 100% fortress speed bonus", metrics), nil
	}
	metrics["commanderSpeedBonus"] = speed
	capacity, err := autoFortressCapacity(snapshot, candidate, commanderID)
	if err != nil {
		if decision, refresh := generalSkillsRefreshDecision(err, snapshot.Now, metrics); refresh {
			return decision, nil
		}
		return autoFortressWaiting(snapshot, "Cannot calculate the one-wave fortress formation: "+err.Error(), metrics), nil
	}
	required := capacity.Capacity.Left + capacity.Capacity.Right
	metrics["requiredDirewolves"] = float64(required)
	available := max(int64(0), candidate.Source.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
	if available < required {
		if decision, detail := autoFortressTransportDecision(snapshot, candidate.Source, main, mainFound, required-available, metrics); decision != nil {
			return *decision, nil
		} else {
			if detail == "" {
				detail = fmt.Sprintf("%s needs %d more Direwolves for one full flank wave", castleName(candidate.Source), required-available)
			}
			if purchaseBlocked != "" {
				detail += "; supply: " + purchaseBlocked
			}
			return autoFortressWaiting(snapshot, detail, metrics), nil
		}
	}

	if _, blocked := dailyAttackLimitAllowance(snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics); blocked != nil {
		return *blocked, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": candidate.Source.ID, "kingdomId": candidate.Target.KingdomID,
		"targetX": candidate.Target.X, "targetY": candidate.Target.Y,
		"commanderIds": []State.CommanderID{commanderID}, "horseTravelBoostId": settings.HorseTravelBoostID,
		"dailyAttackLimit": settings.DailyAttackLimit, "minimumCommanderSpeedBonus": settings.MinimumCommanderSpeedBonus,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch fastest Direwolf wave at fortress %d:%d", candidate.Target.X, candidate.Target.Y),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: "fortress.attack", Arguments: arguments}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func defaultAutoFortressSettings() autoFortressSettings {
	return autoFortressSettings{
		Version: 1, CheckIntervalSec: autoFortressDefaultCheckIntervalSec, MapRefreshIntervalSec: autoFortressDefaultMapRefreshSec,
		HorseTravelBoostID: 1009, MinimumCommanderSpeedBonus: autoFortressMaximumSpeedPercent,
		Kingdoms: map[string]autoFortressKingdom{},
	}
}

func validateAutoFortressSettings(settings autoFortressSettings) string {
	if settings.Version != 1 {
		return fmt.Sprintf("Unsupported Auto Fortress settings version %d", settings.Version)
	}
	if settings.CheckIntervalSec < 1 || settings.CheckIntervalSec > 3600 ||
		settings.MapRefreshIntervalSec < 60 || settings.MapRefreshIntervalSec > 3600 {
		return "Auto Fortress scan and check cadence settings are invalid"
	}
	if settings.DailyAttackLimit < 0 || settings.DirewolfPurchaseLimit < 0 || settings.DirewolfPurchaseLimit%100 != 0 ||
		settings.DirewolfPurchaseLimit > 100_000 || settings.MinimumTabletReserve < 0 {
		return "Direwolf purchase limit must be 0 to 100,000 in exact increments of 100, with a non-negative tablet reserve"
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) || settings.MinimumCommanderSpeedBonus != autoFortressMaximumSpeedPercent {
		return "Auto Fortress requires the maxed 100% fortress commander speed bonus and a supported travel boost"
	}
	return ""
}

func autoFortressSources(gameState State.GameState, settings autoFortressSettings, definitions map[State.KingdomID]GameData.KingdomFortressDefinition) []State.CastleState {
	sources := make([]State.CastleState, 0, 3)
	for key, kingdom := range settings.Kingdoms {
		if !kingdom.Enabled {
			continue
		}
		value, err := strconv.Atoi(key)
		kingdomID := State.KingdomID(value)
		if err != nil || kingdomID < 1 || kingdomID > 3 {
			continue
		}
		definition, supported := definitions[kingdomID]
		if !supported || definition.GlobalCooldownSec != int64(24*time.Hour/time.Second) || definition.PersonalCooldownSec != int64(120*time.Hour/time.Second) {
			continue
		}
		for _, castle := range gameState.Castles {
			if castle.KingdomID == kingdomID && castle.SlotType == 12 {
				sources = append(sources, castle)
				break
			}
		}
	}
	sort.Slice(sources, func(left, right int) bool { return sources[left].KingdomID < sources[right].KingdomID })
	return sources
}

func sourceForKingdom(sources []State.CastleState, kingdomID State.KingdomID) (State.CastleState, bool) {
	for _, source := range sources {
		if source.KingdomID == kingdomID {
			return source, true
		}
	}
	return State.CastleState{}, false
}

func (policy *AutoFortressPolicy) markFullScanRequested(kingdomID State.KingdomID, now time.Time) {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	policy.lastFullScanRequested[kingdomID] = now
}

func (policy *AutoFortressPolicy) fullScanDue(kingdomID State.KingdomID, now time.Time, interval time.Duration) bool {
	return !policy.nextFullScanAt(kingdomID, now, interval).After(now)
}

func (policy *AutoFortressPolicy) nextFullScanAt(kingdomID State.KingdomID, now time.Time, interval time.Duration) time.Time {
	policy.mu.Lock()
	requested := policy.lastFullScanRequested[kingdomID]
	policy.mu.Unlock()
	if requested.IsZero() || now.Before(requested) {
		return now
	}
	return requested.Add(interval)
}

func autoFortressTargets(snapshot Snapshot, sources []State.CastleState) ([]autoFortressTarget, time.Time, map[State.KingdomID]autoFortressKingdomStats) {
	candidates := make([]autoFortressTarget, 0)
	statsByKingdom := make(map[State.KingdomID]autoFortressKingdomStats, len(sources))
	var nextCooldown time.Time
	for _, source := range sources {
		snapshot.State.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionFortress, func(_ string, target State.MapObservation) bool {
			if target.TypeID != State.MapTypeKingdomFortress {
				return true
			}
			stats := statsByKingdom[source.KingdomID]
			stats.Known++
			remaining := autoFortressCooldownRemaining(snapshot.State, target, snapshot.Now)
			if remaining > 0 {
				readyAt := snapshot.Now.Add(time.Duration(remaining) * time.Second)
				if nextCooldown.IsZero() || readyAt.Before(nextCooldown) {
					nextCooldown = readyAt
				}
				if stats.NextReady.IsZero() || readyAt.Before(stats.NextReady) {
					stats.NextReady = readyAt
				}
				statsByKingdom[source.KingdomID] = stats
				return true
			}
			if autoFortressTargetInFlight(snapshot.State, target, snapshot.Now) {
				statsByKingdom[source.KingdomID] = stats
				return true
			}
			stats.Ready++
			statsByKingdom[source.KingdomID] = stats
			candidates = append(candidates, autoFortressTarget{Source: source, Target: target})
			return true
		})
		if _, exists := statsByKingdom[source.KingdomID]; !exists {
			statsByKingdom[source.KingdomID] = autoFortressKingdomStats{}
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftDistance := fortressDistanceSquared(candidates[left])
		rightDistance := fortressDistanceSquared(candidates[right])
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if candidates[left].Target.KingdomID != candidates[right].Target.KingdomID {
			return candidates[left].Target.KingdomID < candidates[right].Target.KingdomID
		}
		if candidates[left].Target.Y != candidates[right].Target.Y {
			return candidates[left].Target.Y < candidates[right].Target.Y
		}
		return candidates[left].Target.X < candidates[right].Target.X
	})
	return candidates, nextCooldown, statsByKingdom
}

func fortressDistanceSquared(candidate autoFortressTarget) int {
	x, y := candidate.Target.X-candidate.Source.X, candidate.Target.Y-candidate.Source.Y
	return x*x + y*y
}

func autoFortressTargetInFlight(gameState State.GameState, target State.MapObservation, now time.Time) bool {
	if State.AttackFeatureTargetPendingAt(gameState, State.AttackFeatureAutoFortress, target.KingdomID, target.TypeID, target.X, target.Y, now) {
		return true
	}
	blocked := false
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.KingdomID == target.KingdomID && towerMovementActiveAt(movement, now) && ((movement.Direction == 0 && movement.TargetTypeID == State.MapTypeKingdomFortress && movement.TargetX == target.X && movement.TargetY == target.Y) ||
			(movement.Direction == 1 && movement.SourceTypeID == State.MapTypeKingdomFortress && movement.SourceX == target.X && movement.SourceY == target.Y)) {
			blocked = true
			return false
		}
		return true
	})
	return blocked
}

func autoFortressCooldownRemaining(gameState State.GameState, target State.MapObservation, now time.Time) int {
	remaining := towerCooldownRemaining(target, now)
	key := towerTargetKey(target.KingdomID, target.X, target.Y)
	if cooldown, found := gameState.LookupTowerCooldown(key); found && cooldown.TargetTypeID == State.MapTypeKingdomFortress {
		until := cooldown.LastSuccessfulBattleAt.Add(120 * time.Hour)
		if until.After(now) {
			remaining = max(remaining, int(math.Ceil(until.Sub(now).Seconds())))
		}
	}
	return remaining
}

func pendingFortressCooldownRefresh(gameState State.GameState) (State.TowerCooldownState, bool) {
	var selected State.TowerCooldownState
	gameState.RangeTowerCooldowns(func(_ string, cooldown State.TowerCooldownState) bool {
		if cooldown.TargetTypeID != State.MapTypeKingdomFortress || !cooldown.PendingCooldownRefresh {
			return true
		}
		if selected.LastSuccessfulBattleAt.IsZero() || cooldown.LastSuccessfulBattleAt.Before(selected.LastSuccessfulBattleAt) {
			selected = cooldown
		}
		return true
	})
	return selected, !selected.LastSuccessfulBattleAt.IsZero()
}

func fastestFortressCommander(
	snapshot Snapshot,
	candidate autoFortressTarget,
	configured []State.CommanderID,
	restricted bool,
	speedContract GameData.FortressRelicSpeedContract,
) (State.CommanderID, float64, bool) {
	if !restricted {
		configured = make([]State.CommanderID, 0, len(snapshot.State.Commanders))
		for commanderID := range snapshot.State.Commanders {
			configured = append(configured, commanderID)
		}
	}
	sort.Slice(configured, func(left, right int) bool { return configured[left] < configured[right] })
	selected, best := State.CommanderID(0), -1.0
	found := false
	for _, commanderID := range configured {
		commander, exists := snapshot.State.Commanders[commanderID]
		relicSpeed, hasRelicSpeed := EquipmentDomain.CommanderRelic2EffectTotal(snapshot.State, commanderID, speedContract.RelicEffectID)
		if !exists || !commander.Available || State.CommanderHasActiveMovementAt(snapshot.State, commanderID, snapshot.Now) ||
			!hasRelicSpeed || relicSpeed+0.0001 < speedContract.RelicMaximumPercent {
			continue
		}
		result, err := (AttackCapacity.Resolver{}).ResolveTravelSpeed(snapshot.State, snapshot.GameData, AttackCapacity.Request{
			SourceCastleID: candidate.Source.ID, CommanderID: commanderID,
			Target: AttackCapacity.TargetContext{
				Map:   &AttackCapacity.MapTarget{KingdomID: candidate.Target.KingdomID, TypeID: candidate.Target.TypeID, X: candidate.Target.X, Y: candidate.Target.Y, Level: candidate.Target.Level},
				Level: candidate.Target.Level, CastleTypeID: candidate.Target.TypeID, PvP: false,
			},
		})
		if err != nil || result.AppliedPercent+0.0001 < speedContract.RelicMaximumPercent {
			continue
		}
		if !found || result.AppliedPercent > best || result.AppliedPercent == best && commanderID < selected {
			selected, best, found = commanderID, result.AppliedPercent, true
		}
	}
	return selected, best, found
}

func autoFortressCapacity(snapshot Snapshot, candidate autoFortressTarget, commanderID State.CommanderID) (AttackCapacity.Result, error) {
	return (AttackCapacity.Resolver{}).Resolve(snapshot.State, snapshot.GameData, AttackCapacity.Request{
		SourceCastleID: candidate.Source.ID, CommanderID: commanderID,
		UseAttackDialogEffects: fortressDialogMatches(
			snapshot.State.AttackDialog, candidate, snapshot.Now,
		),
		Target: AttackCapacity.TargetContext{
			ID:    fmt.Sprintf("fortress:%d:%d:%d", candidate.Target.KingdomID, candidate.Target.X, candidate.Target.Y),
			Map:   &AttackCapacity.MapTarget{KingdomID: candidate.Target.KingdomID, TypeID: candidate.Target.TypeID, X: candidate.Target.X, Y: candidate.Target.Y, Level: candidate.Target.Level},
			Level: candidate.Target.Level, CastleTypeID: candidate.Target.TypeID, PvP: false,
		},
	})
}

func fortressDialogMatches(dialog State.AttackDialogState, candidate autoFortressTarget, now time.Time) bool {
	if dialog.ObservedAt.IsZero() || now.Before(dialog.ObservedAt) ||
		now.Sub(dialog.ObservedAt) > autoFortressAttackDialogFreshness {
		return false
	}
	return dialog.SourceCastleID == candidate.Source.ID && dialog.KingdomID == candidate.Target.KingdomID &&
		dialog.Target.TypeID == State.MapTypeKingdomFortress && dialog.Target.X == candidate.Target.X &&
		dialog.Target.Y == candidate.Target.Y
}

func autoFortressTransportDecision(snapshot Snapshot, target State.CastleState, main State.CastleState, mainFound bool, shortfall int64, metrics map[string]float64) (*Decision, string) {
	if !mainFound {
		return nil, "The Great Empire main castle is unavailable for Direwolf supply"
	}
	if target.UnitsObservedAt.IsZero() || snapshot.Now.Sub(target.UnitsObservedAt) > autoFortressUnitFreshness {
		decision := autoFortressRequest(snapshot, metrics, "Refresh destination Direwolves at "+castleName(target), "game.focus_castle", map[string]any{"castleId": target.ID, "refresh": true})
		return &decision, ""
	}
	if main.UnitsObservedAt.IsZero() || snapshot.Now.Sub(main.UnitsObservedAt) > autoFortressUnitFreshness {
		decision := autoFortressRequest(snapshot, metrics, "Refresh purchased Direwolves at "+castleName(main), "game.focus_castle", map[string]any{"castleId": main.ID, "refresh": true})
		return &decision, ""
	}
	if snapshot.State.KingdomTransport.ObservedAt.IsZero() || snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt) > autoFortressUnitFreshness {
		decision := autoFortressRequest(snapshot, metrics, "Refresh kingdom troop transport availability", "troops.kingdom.refresh", map[string]any{})
		return &decision, ""
	}
	unlock, observed := snapshot.State.KingdomTransport.Unlocks[target.KingdomID]
	if !observed || !unlock.Unlocked {
		return nil, fmt.Sprintf("Kingdom troop transport to %s is not unlocked", castleName(target))
	}
	for _, pending := range snapshot.State.KingdomTransport.PendingUnits {
		if pending.KingdomID == target.KingdomID {
			return nil, fmt.Sprintf("Waiting for the Direwolf troop transport to %s", castleName(target))
		}
	}
	available := max(int64(0), main.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
	if available <= 0 {
		return nil, "No purchased Direwolves are stationed at the Great Empire main castle"
	}
	amount := min(shortfall, available)
	decision := autoFortressRequest(snapshot, metrics, fmt.Sprintf("Transport %d Direwolves to %s", amount, castleName(target)), "troops.kingdom.ship", map[string]any{
		"sourceCastleId": main.ID, "targetCastleId": target.ID, "targetKingdomId": target.KingdomID,
		"units": []map[string]any{{"unitId": GameData.DirewolfUnitID, "amount": amount}},
	})
	return &decision, ""
}

func evaluateAutoFortressPurchase(snapshot Snapshot, settings autoFortressSettings, main State.CastleState, metrics map[string]float64) (*Decision, string) {
	packages, err := snapshot.GameData.FortressDirewolfPackages()
	if err != nil {
		return nil, err.Error()
	}
	active := make([]GameData.AutoBuyerPackage, 0, len(packages))
	for _, product := range packages {
		if _, found := snapshot.State.ActiveShopForPackage(State.PackageID(product.PackageID), snapshot.Now); found {
			active = append(active, product)
		}
	}
	if len(active) == 0 {
		return nil, "Nomad Direwolf shop is not active"
	}
	offers, observedAt, found := snapshot.State.ConstructionOffersFor(main.ID, main.KingdomID)
	if !found || observedAt.IsZero() || snapshot.Now.Sub(observedAt) >= autoFortressPurchaseHistoryAge {
		decision := autoFortressRequest(snapshot, metrics, "Refresh Nomad Direwolf stock counters", "autoBuyer.package.history", map[string]any{"sourceCastleId": main.ID})
		return &decision, ""
	}
	remainingGoal := settings.DirewolfPurchaseLimit
	purchasedUnits := int64(0)
	for _, product := range active {
		allocatedUnits := min(remainingGoal, product.Stock*product.UnitAmount)
		if allocatedUnits <= 0 {
			break
		}
		targetPurchases := allocatedUnits / product.UnitAmount
		purchased := offers[State.PackageID(product.PackageID)]
		purchasedUnits += min(purchased, targetPurchases) * product.UnitAmount
		remainingGoal -= allocatedUnits
		if purchased >= targetPurchases {
			continue
		}
		amount := targetPurchases - purchased
		if product.MaxBuyPerClick > 0 {
			amount = min(amount, product.MaxBuyPerClick)
		}
		balance, available := autoBuyerPriceBalance(snapshot.State, main, product.Price)
		if !available {
			return nil, product.Price.Name + " balance is unavailable"
		}
		spendable := balance - settings.MinimumTabletReserve
		if spendable < product.Price.Amount {
			return nil, fmt.Sprintf("Waiting for %d %s above reserve to buy the next 100 Direwolves", product.Price.Amount, product.Price.Name)
		}
		amount = min(amount, spendable/product.Price.Amount)
		if amount <= 0 {
			return nil, "The configured Nomad tablet reserve blocks the next Direwolf package"
		}
		metrics["direwolvesPurchasedThisSession"] = float64(purchasedUnits)
		decision := autoBuyerRequestDecision(snapshot.Now, metrics, fmt.Sprintf("Buy %d Direwolves from the cheapest remaining Nomad tier", amount*product.UnitAmount), "autoBuyer.package.purchase", map[string]any{
			"sourceCastleId": main.ID, "shopId": product.ShopID, "packageId": product.PackageID,
			"amount": amount, "targetPurchasesPerReset": targetPurchases,
			"minimumBalanceReserve": settings.MinimumTabletReserve, "allowRubyPackages": false,
			"maximumRubySpendPerReset": 0, "minimumRubyReserve": 0,
			"expectedPurchasedBefore": purchased, "expectedBalanceBefore": balance,
		})
		return &decision, ""
	}
	metrics["direwolvesPurchasedThisSession"] = float64(purchasedUnits)
	return nil, "Direwolf session limit is satisfied"
}

func autoFortressRequest(snapshot Snapshot, metrics map[string]float64, detail, name string, arguments any) Decision {
	raw, _ := json.Marshal(arguments)
	return Decision{
		Status: "ready", Detail: detail, NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: name, Arguments: raw}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}

func autoFortressWaiting(snapshot Snapshot, detail string, metrics map[string]float64) Decision {
	seconds := snapshotFortressCheckInterval(snapshot)
	if seconds < 1 || seconds > 3600 {
		seconds = 30
	}
	return Decision{Status: "waiting", Detail: detail, NextCheckAt: snapshot.Now.Add(time.Duration(seconds) * time.Second), Metrics: metrics}
}

func snapshotFortressCheckInterval(snapshot Snapshot) int {
	settings := defaultAutoFortressSettings()
	if decodeSection(snapshot.Configuration, autoFortressSection, &settings) {
		return max(1, settings.CheckIntervalSec)
	}
	return 30
}

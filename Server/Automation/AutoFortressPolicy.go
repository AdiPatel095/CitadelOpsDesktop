package Automation

import (
	"CitadelDesktop/Server/Localization"
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
	mu                         sync.Mutex
	lastFullScanRequested      map[State.KingdomID]time.Time
	lastSupplyRefreshRequested map[State.KingdomID]time.Time
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
	UseTimeSkips               bool                           `json:"useTimeSkips"`
	TimeSkipReserve            map[string]int64               `json:"timeSkipReserve"`
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
	return &AutoFortressPolicy{
		lastFullScanRequested:      map[State.KingdomID]time.Time{},
		lastSupplyRefreshRequested: map[State.KingdomID]time.Time{},
	}
}

func (*AutoFortressPolicy) ID() string         { return "autoFortress" }
func (*AutoFortressPolicy) EnabledKey() string { return "auto_fortress" }

func (*AutoFortressPolicy) WakeDomains() []string {
	return []string{"attack_dialog", "attacks", "castles", "commanders", "construction-offers", "currencies", "event-scores", "events", "kingdom-transport", "map-fortress", "movements", "tower-cooldowns", "units"}
}

func (*AutoFortressPolicy) WakeSections() []string {
	return []string{autoFortressSection, commanderFeatureSection}
}

func (policy *AutoFortressPolicy) Evaluate(_ context.Context, snapshot Snapshot) (result Decision, resultErr error) {
	settings := defaultAutoFortressSettings()
	if !decodeSection(snapshot.Configuration, autoFortressSection, &settings) {
		return autoFortressWaiting(snapshot, "Auto Fortress settings have not been saved", nil, Localization.New("server.automation.auto_fortress_settings_have.34b778f4", "Auto Fortress settings have not been saved", nil)), nil
	}
	if snapshot.PolicyConfigurationChanged {
		policy.mu.Lock()
		policy.lastFullScanRequested = map[State.KingdomID]time.Time{}
		policy.lastSupplyRefreshRequested = map[State.KingdomID]time.Time{}
		policy.mu.Unlock()
	}
	if detail := validateAutoFortressSettings(settings); detail != "" {
		return autoFortressWaiting(snapshot, detail, nil), nil
	}
	if snapshot.GameData == nil {
		return autoFortressWaiting(snapshot, "Official game data is unavailable", nil, Localization.New("server.automation.official_game_data_is.c5e55e7e", "Official game data is unavailable", nil)), nil
	}
	definitions, err := snapshot.GameData.KingdomFortressDefinitions()
	if err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil, Localization.FromError(err)), nil
	}
	if _, err := snapshot.GameData.FortressDirewolf(); err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil, Localization.FromError(err)), nil
	}
	speedContract, err := snapshot.GameData.FortressRelicSpeed()
	if err != nil {
		return autoFortressWaiting(snapshot, err.Error(), nil, Localization.FromError(err)), nil
	}
	if speedContract.RelicMaximumPercent != autoFortressMaximumSpeedPercent {
		return autoFortressWaiting(snapshot, "Official fortress commander speed contract changed; Auto Fortress is paused", nil, Localization.New("server.automation.official_fortress_commander_speed.891da2be", "Official fortress commander speed contract changed; Auto Fortress is paused", nil)), nil
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
	details := map[string]string{}
	detailDescriptors := map[string]*Localization.Message{}
	defer func() {
		if result.Details != nil {
			result.DetailsDescriptors = detailDescriptors
		}
	}()
	main, mainFound := autoBuyerSourceCastle(snapshot.State, 0)
	if decision := policy.autoFortressSupplyDecision(snapshot, settings, sources, main, mainFound, metrics, details, detailDescriptors); decision != nil {
		decision.Details = details
		return *decision, nil
	}
	if len(sources) == 0 {
		decision := autoFortressWaiting(snapshot, "Enable at least one available outer-kingdom main castle", metrics, Localization.New("server.automation.enable_at_least_one.a8095e21", "Enable at least one available outer-kingdom main castle", nil))
		decision.Details = details
		return decision, nil
	}
	purchaseBlocked := ""
	if settings.DirewolfPurchaseLimit > 0 && mainFound {
		if decision, detail := evaluateAutoFortressPurchase(snapshot, settings, main, metrics); decision != nil {
			return *decision, nil
		} else {
			purchaseBlocked = detail
		}
	}

	if cooldown, found := pendingFortressCooldownRefresh(snapshot.State); found {
		source, sourceFound := sourceForKingdom(sources, cooldown.KingdomID)
		if sourceFound {
			decision := autoFortressRequest(snapshot, metrics, fmt.Sprintf("Refresh five-day fortress cooldown at %d:%d", cooldown.X, cooldown.Y), "fortress.target.refresh", map[string]any{
				"sourceCastleId": source.ID, "kingdomId": cooldown.KingdomID, "targetX": cooldown.X, "targetY": cooldown.Y,
			}, Localization.New("server.automation.fortress_refresh_cooldown", "Refresh five-day fortress cooldown at {x}:{y}", Localization.Params{"x": fmt.Sprint(cooldown.X), "y": fmt.Sprint(cooldown.Y)}))
			decision.Details = details
			return decision, nil
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
				decision := autoFortressRequest(snapshot, metrics, fmt.Sprintf("Discover every fortress across %s", castleName(source)), "fortress.map.scan", map[string]any{
					"sourceCastleId": source.ID, "kingdomId": source.KingdomID,
				}, castleDecisionDescriptor("fortress_discover", source, nil))
				decision.Details = details
				return decision, nil
			}
		}

		detail := "No known fortress is currently available"
		var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.no_known_fortress_is.3b2b0041", "No known fortress is currently available", nil)
		if !nextCooldown.IsZero() {
			detail = fmt.Sprintf(
				"Next known fortress is expected at %s; a targeted cooldown check will run at availability",
				nextCooldown.UTC().Format("Jan 2 15:04:05 UTC"),
			)
			detailLocalizationMessage = nil
		}
		if purchaseBlocked != "" {
			detail += "; supply: " + purchaseBlocked
			detailLocalizationMessage = nil
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
		return Decision{Status: "idle", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), NextCheckAt: next, Metrics: metrics, Details: details}, nil
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
		decision := autoFortressWaiting(snapshot, "No available assigned commander has Relic 2.0 equipment and the maxed 100% fortress speed bonus", metrics, Localization.New("server.automation.no_available_assigned_commander.973da452", "No available assigned commander has Relic 2.0 equipment and the maxed 100% fortress speed bonus", nil))
		decision.Details = details
		return decision, nil
	}
	metrics["commanderSpeedBonus"] = speed
	capacity, err := autoFortressCapacity(snapshot, candidate, commanderID)
	if err != nil {
		if decision, refresh := generalSkillsRefreshDecision(err, snapshot.Now, metrics); refresh {
			decision.Details = details
			return decision, nil
		}
		decision := autoFortressWaiting(snapshot, "Cannot calculate the one-wave fortress formation: "+err.Error(), metrics, Localization.ErrorContext(Localization.New("server.automation.cannot_calculate_the_one.d7048c25", "Cannot calculate the one-wave fortress formation", nil), err))
		decision.Details = details
		return decision, nil
	}
	required := capacity.Capacity.Left + capacity.Capacity.Right
	metrics["requiredDirewolves"] = float64(required)
	available := max(int64(0), candidate.Source.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
	if available < required {
		detail := fmt.Sprintf("%s needs %d more arrived Direwolves for one full flank wave", castleName(candidate.Source), required-available)
		var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.p_needs_p_more.8b5c5bac", "{p0} needs {p1, number} more arrived Direwolves for one full flank wave", Localization.Params{"p0": fmt.Sprintf("%s", castleName(candidate.Source)), "p1": required - available})
		if purchaseBlocked != "" {
			detail += "; supply: " + purchaseBlocked
			detailLocalizationMessage = nil
		}
		decision := autoFortressWaiting(snapshot, detail, metrics, Localization.Clone(detailLocalizationMessage))
		decision.Details = details
		return decision, nil
	}

	if _, blocked := dailyAttackLimitAllowance(snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics); blocked != nil {
		blocked.Details = details
		return *blocked, nil
	}
	arguments, _ := json.Marshal(map[string]any{
		"sourceCastleId": candidate.Source.ID, "kingdomId": candidate.Target.KingdomID,
		"targetX": candidate.Target.X, "targetY": candidate.Target.Y,
		"commanderIds": []State.CommanderID{commanderID}, "horseTravelBoostId": settings.HorseTravelBoostID,
		"dailyAttackLimit": settings.DailyAttackLimit, "minimumCommanderSpeedBonus": settings.MinimumCommanderSpeedBonus,
	})
	return Decision{
		Status: "ready", Detail: fmt.Sprintf("Launch fastest Direwolf wave at fortress %d:%d", candidate.Target.X, candidate.Target.Y), DetailDescriptor: Localization.New("server.automation.launch_fastest_direwolf_wave.f080e364", "Launch fastest Direwolf wave at fortress {p0}:{p1}", Localization.Params{"p0": fmt.Sprintf("%d", candidate.Target.X), "p1": fmt.Sprintf("%d", candidate.Target.Y)}),
		NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics, Details: details,
		Request: &Intent.Request{Name: "fortress.attack", Arguments: arguments}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}, nil
}

func defaultAutoFortressSettings() autoFortressSettings {
	return autoFortressSettings{
		Version: 1, CheckIntervalSec: autoFortressDefaultCheckIntervalSec, MapRefreshIntervalSec: autoFortressDefaultMapRefreshSec,
		HorseTravelBoostID: 1009, MinimumCommanderSpeedBonus: autoFortressMaximumSpeedPercent,
		TimeSkipReserve: map[string]int64{}, Kingdoms: map[string]autoFortressKingdom{},
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
	if invalidTimeSkipReserve(settings.TimeSkipReserve) {
		return "Auto Fortress time-skip reserves must be non-negative whole numbers"
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

const autoFortressTransportOwner = "autoFortress"

type autoFortressSupplyDestination struct {
	castle     State.CastleState
	stationed  int64
	inbound    int64
	committed  int64
	allocation int64
	pending    bool
}

func (policy *AutoFortressPolicy) autoFortressSupplyDecision(
	snapshot Snapshot,
	settings autoFortressSettings,
	sources []State.CastleState,
	main State.CastleState,
	mainFound bool,
	metrics map[string]float64,
	details map[string]string,
	descriptorMaps ...map[string]*Localization.Message,
) *Decision {
	detailDescriptors := detailDescriptorMap(descriptorMaps)
	configuredKingdoms := make([]int, 0, len(settings.Kingdoms))
	for key, configured := range settings.Kingdoms {
		if !configured.Enabled {
			continue
		}
		kingdomID, _ := strconv.Atoi(key)
		configuredKingdoms = append(configuredKingdoms, kingdomID)
		metrics[fmt.Sprintf("stationedDirewolvesKingdom%d", kingdomID)] = 0
		metrics[fmt.Sprintf("inboundDirewolvesKingdom%d", kingdomID)] = 0
		metrics[fmt.Sprintf("allocatedDirewolvesKingdom%d", kingdomID)] = 0
		metrics[fmt.Sprintf("outstandingDirewolvesKingdom%d", kingdomID)] = 0
		if _, found := sourceForKingdom(sources, State.KingdomID(kingdomID)); !found {
			details[fmt.Sprintf("supplyKingdom%d", kingdomID)] = "Main castle is unavailable or the kingdom is not unlocked"
			detailDescriptors[fmt.Sprintf("supplyKingdom%d", kingdomID)] = Localization.New("server.automation.main_castle_is_unavailable.e1216480", "Main castle is unavailable or the kingdom is not unlocked", nil)
		}
	}
	sort.Ints(configuredKingdoms)

	workflowKingdoms := make([]int, 0, len(snapshot.State.KingdomTransport.TroopWorkflows))
	for kingdomID, workflow := range snapshot.State.KingdomTransport.TroopWorkflows {
		if workflow.Owner == autoFortressTransportOwner {
			workflowKingdoms = append(workflowKingdoms, int(kingdomID))
		}
	}
	sort.Ints(workflowKingdoms)
	var workflowAction *Decision
	donorUnreconciled := false
	for _, value := range workflowKingdoms {
		kingdomID := State.KingdomID(value)
		workflow := snapshot.State.KingdomTransport.TroopWorkflows[kingdomID]
		if workflow.SourceReconciledAt.IsZero() && !workflow.SourceDebitedLocally {
			donorUnreconciled = true
		}
		target, found := snapshot.State.Castles[workflow.TargetCastleID]
		if !found {
			details[fmt.Sprintf("supplyKingdom%d", kingdomID)] = "Owned Direwolf transfer target is unavailable; reconciliation is paused"
			detailDescriptors[fmt.Sprintf("supplyKingdom%d", kingdomID)] = Localization.New("server.automation.owned_direwolf_transfer_target.cd1bf957", "Owned Direwolf transfer target is unavailable; reconciliation is paused", nil)
			if workflow.SourceReconciledAt.IsZero() && !workflow.SourceDebitedLocally && workflow.Status != "armed" && workflow.Status != "ownership_uncertain" && workflow.TransportObservedAt.After(workflow.ArmedAt) && mainFound {
				if main.UnitsObservedAt.IsZero() || !main.UnitsObservedAt.After(workflow.TransportObservedAt) {
					decision := autoFortressRequest(snapshot, metrics, "Refresh Direwolf donor after ambiguous dispatch", "game.focus_castle", map[string]any{"castleId": main.ID, "refresh": true}, Localization.New("server.automation.refresh_direwolf_donor_after.62624b9c", "Refresh Direwolf donor after ambiguous dispatch", nil))
					if workflowAction == nil {
						workflowAction = &decision
					}
				} else {
					decision := autoFortressRequest(snapshot, metrics, "Reconcile Direwolf donor after ambiguous dispatch", "troops.kingdom.reconcile_donor", map[string]any{
						"owner": workflow.Owner, "workflowId": workflow.ID, "targetKingdomId": workflow.KingdomID,
					}, Localization.New("server.automation.reconcile_direwolf_donor_after.3e8f7c6a", "Reconcile Direwolf donor after ambiguous dispatch", nil))
					if workflowAction == nil {
						workflowAction = &decision
					}
				}
			}
			continue
		}
		stationed := max(int64(0), target.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
		inbound, _ := autoFortressInboundDirewolves(snapshot.State, kingdomID)
		metrics[fmt.Sprintf("stationedDirewolvesKingdom%d", kingdomID)] = float64(stationed)
		metrics[fmt.Sprintf("inboundDirewolvesKingdom%d", kingdomID)] = float64(inbound)
		metrics[fmt.Sprintf("allocatedDirewolvesKingdom%d", kingdomID)] = float64(stationed + inbound)
		metrics[fmt.Sprintf("outstandingDirewolvesKingdom%d", kingdomID)] = 0
		enabled := settings.Kingdoms[strconv.Itoa(int(kingdomID))].Enabled
		if decision := policy.autoFortressWorkflowDecision(snapshot, settings, main, mainFound, target, workflow, enabled, metrics, details, detailDescriptors); decision != nil && workflowAction == nil {
			workflowAction = decision
		}
	}
	if !mainFound {
		for _, source := range sources {
			details[fmt.Sprintf("supplyKingdom%d", source.KingdomID)] = "Great Empire main castle is unavailable for Direwolf supply"
			detailDescriptors[fmt.Sprintf("supplyKingdom%d", source.KingdomID)] = Localization.New("server.automation.great_empire_main_castle.82eee831", "Great Empire main castle is unavailable for Direwolf supply", nil)
		}
		return workflowAction
	}
	var maintenance *Decision
	if main.UnitsObservedAt.IsZero() || snapshot.Now.Sub(main.UnitsObservedAt) > autoFortressUnitFreshness {
		decision := autoFortressRequest(snapshot, metrics, "Refresh purchased Direwolves at "+castleName(main), "game.focus_castle", map[string]any{"castleId": main.ID, "refresh": true}, castleDecisionDescriptor("fortress_refresh_purchased", main, nil))
		maintenance = &decision
	}
	if snapshot.State.KingdomTransport.ObservedAt.IsZero() || snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt) > autoFortressUnitFreshness {
		decision := autoFortressRequest(snapshot, metrics, "Refresh kingdom troop transport availability", "troops.kingdom.refresh", map[string]any{}, Localization.New("server.automation.refresh_kingdom_troop_transport.832f1194", "Refresh kingdom troop transport availability", nil))
		if maintenance == nil {
			maintenance = &decision
		}
	}

	destinations := make([]autoFortressSupplyDestination, 0, len(sources))
	for _, source := range sources {
		key := fmt.Sprintf("supplyKingdom%d", source.KingdomID)
		unlock, observed := snapshot.State.KingdomTransport.Unlocks[source.KingdomID]
		if !observed || !unlock.Unlocked {
			details[key] = "Kingdom troop transport is not unlocked"
			detailDescriptors[key] = Localization.New("server.automation.kingdom_troop_transport_is.f888fc32", "Kingdom troop transport is not unlocked", nil)
			continue
		}
		if source.UnitsObservedAt.IsZero() || snapshot.Now.Sub(source.UnitsObservedAt) > autoFortressUnitFreshness {
			details[key] = "Destination Direwolf inventory needs a fresh observation"
			detailDescriptors[key] = Localization.New("server.automation.destination_direwolf_inventory_needs.2ab6a00f", "Destination Direwolf inventory needs a fresh observation", nil)
			if maintenance == nil && policy.supplyRefreshDue(source.KingdomID, snapshot.Now) {
				policy.markSupplyRefreshRequested(source.KingdomID, snapshot.Now)
				decision := autoFortressRequest(snapshot, metrics, "Refresh destination Direwolves at "+castleName(source), "game.focus_castle", map[string]any{"castleId": source.ID, "refresh": true}, castleDecisionDescriptor("fortress_refresh_destination", source, nil))
				maintenance = &decision
			}
			continue
		}
		stationed := max(int64(0), source.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
		inbound, pending := autoFortressInboundDirewolves(snapshot.State, source.KingdomID)
		destinations = append(destinations, autoFortressSupplyDestination{
			castle: source, stationed: stationed, inbound: inbound, committed: stationed + inbound, pending: pending,
		})
		metrics[fmt.Sprintf("stationedDirewolvesKingdom%d", source.KingdomID)] = float64(stationed)
		metrics[fmt.Sprintf("inboundDirewolvesKingdom%d", source.KingdomID)] = float64(inbound)
		if _, exists := details[key]; !exists {
			details[key] = "Direwolf supply is ready"
			detailDescriptors[key] = Localization.New("server.automation.direwolf_supply_is_ready.cafc9a1f", "Direwolf supply is ready", nil)
		}
	}

	donor := max(int64(0), main.Units.Stationed[State.UnitID(GameData.DirewolfUnitID)])
	if donorUnreconciled || main.UnitsObservedAt.IsZero() || snapshot.Now.Before(main.UnitsObservedAt) || snapshot.Now.Sub(main.UnitsObservedAt) > autoFortressUnitFreshness ||
		snapshot.State.KingdomTransport.ObservedAt.IsZero() || snapshot.Now.Before(snapshot.State.KingdomTransport.ObservedAt) || snapshot.Now.Sub(snapshot.State.KingdomTransport.ObservedAt) > autoFortressUnitFreshness {
		donor = 0
	}
	metrics["availableDonorDirewolves"] = float64(donor)
	autoFortressBalanceAllocations(destinations, donor)
	var shipmentAction *Decision
	for index := range destinations {
		destination := &destinations[index]
		kingdomID := destination.castle.KingdomID
		metrics[fmt.Sprintf("allocatedDirewolvesKingdom%d", kingdomID)] = float64(destination.committed + destination.allocation)
		metrics[fmt.Sprintf("outstandingDirewolvesKingdom%d", kingdomID)] = float64(destination.allocation)
		if destination.allocation <= 0 || shipmentAction != nil {
			continue
		}
		if destination.pending {
			details[fmt.Sprintf("supplyKingdom%d", kingdomID)] = "Waiting for the confirmed inbound Direwolves to arrive"
			detailDescriptors[fmt.Sprintf("supplyKingdom%d", kingdomID)] = Localization.New("server.automation.waiting_for_the_confirmed.dd90e254", "Waiting for the confirmed inbound Direwolves to arrive", nil)
			continue
		}
		if workflow, exists := snapshot.State.KingdomTransport.TroopWorkflows[kingdomID]; exists {
			details[fmt.Sprintf("supplyKingdom%d", kingdomID)] = "Reconciling the owned Direwolf transfer before another shipment"
			detailDescriptors[fmt.Sprintf("supplyKingdom%d", kingdomID)] = Localization.New("server.automation.reconciling_the_owned_direwolf.f8feecaa", "Reconciling the owned Direwolf transfer before another shipment", nil)
			_ = workflow
			continue
		}
		amount := destination.allocation
		workflowID := fmt.Sprintf("auto-fortress-%d-%d", kingdomID, snapshot.Now.UnixNano())
		decision := autoFortressRequest(snapshot, metrics, fmt.Sprintf("Allocate %d Direwolves to %s", amount, castleName(destination.castle)), "troops.kingdom.ship", map[string]any{
			"sourceCastleId": main.ID, "targetCastleId": destination.castle.ID, "targetKingdomId": kingdomID,
			"owner": autoFortressTransportOwner, "workflowId": workflowID,
			"units": []map[string]any{{"unitId": GameData.DirewolfUnitID, "amount": amount}},
		}, castleDecisionDescriptor("fortress_allocate", destination.castle, Localization.Params{"troops": amount}))
		details[fmt.Sprintf("supplyKingdom%d", kingdomID)] = fmt.Sprintf("Allocating %d Direwolves from the Great Empire", amount)
		detailDescriptors[fmt.Sprintf("supplyKingdom%d", kingdomID)] = Localization.New("server.automation.allocating_amount_number_direwolves.82b58fd8", "Allocating {amount, number} Direwolves from the Great Empire", Localization.Params{"amount": amount})
		shipmentAction = &decision
	}
	if workflowAction != nil {
		workflowAction.Metrics = metrics
		return workflowAction
	}
	if maintenance != nil {
		maintenance.Metrics = metrics
		return maintenance
	}
	if shipmentAction != nil {
		shipmentAction.Metrics = metrics
		return shipmentAction
	}
	return maintenance
}

func autoFortressInboundDirewolves(gameState State.GameState, kingdomID State.KingdomID) (int64, bool) {
	total := int64(0)
	pending := false
	for _, transport := range gameState.KingdomTransport.PendingUnits {
		if transport.KingdomID != kingdomID {
			continue
		}
		pending = true
		for _, unit := range transport.Units {
			if unit.UnitID == State.UnitID(GameData.DirewolfUnitID) && unit.Amount > 0 {
				total += unit.Amount
			}
		}
	}
	return total, pending
}

func autoFortressBalanceAllocations(destinations []autoFortressSupplyDestination, available int64) {
	if available <= 0 || len(destinations) == 0 {
		return
	}
	sort.Slice(destinations, func(left, right int) bool {
		if destinations[left].committed != destinations[right].committed {
			return destinations[left].committed < destinations[right].committed
		}
		return destinations[left].castle.KingdomID < destinations[right].castle.KingdomID
	})
	levels := make([]int64, len(destinations))
	for index := range destinations {
		levels[index] = destinations[index].committed
	}
	for width := 1; width < len(destinations) && available > 0; width++ {
		delta := levels[width] - levels[width-1]
		cost := delta * int64(width)
		if cost > available {
			break
		}
		for index := 0; index < width; index++ {
			levels[index] += delta
		}
		available -= cost
	}
	if available > 0 {
		minimum := levels[0]
		width := 0
		for width < len(levels) && levels[width] == minimum {
			width++
		}
		share, remainder := available/int64(width), available%int64(width)
		for index := 0; index < width; index++ {
			levels[index] += share
			if int64(index) < remainder {
				levels[index]++
			}
		}
	}
	for index := range destinations {
		destinations[index].allocation = max(int64(0), levels[index]-destinations[index].committed)
	}
}

func (policy *AutoFortressPolicy) autoFortressWorkflowDecision(snapshot Snapshot, settings autoFortressSettings, main State.CastleState, mainFound bool, target State.CastleState, workflow State.KingdomTroopTransportWorkflow, enabled bool, metrics map[string]float64, details map[string]string, descriptorMaps ...map[string]*Localization.Message) *Decision {
	detailDescriptors := detailDescriptorMap(descriptorMaps)
	key := fmt.Sprintf("supplyKingdom%d", target.KingdomID)
	if workflow.Status == "armed" || workflow.Status == "ownership_uncertain" {
		details[key] = "Reconciling an uncertain Direwolf shipment before any replay"
		detailDescriptors[key] = Localization.New("server.automation.reconciling_an_uncertain_direwolf.77eb81f1", "Reconciling an uncertain Direwolf shipment before any replay", nil)
		if policy.supplyRefreshDue(workflow.KingdomID, snapshot.Now) {
			policy.markSupplyRefreshRequested(workflow.KingdomID, snapshot.Now)
			decision := autoFortressRequest(snapshot, metrics, "Refresh uncertain owned Direwolf shipment", "troops.kingdom.refresh", map[string]any{}, Localization.New("server.automation.refresh_uncertain_owned_direwolf.1d0c6481", "Refresh uncertain owned Direwolf shipment", nil))
			return &decision
		}
		return nil
	}
	if workflow.SourceReconciledAt.IsZero() && !workflow.SourceDebitedLocally {
		if !mainFound {
			details[key] = "Owned transfer donor inventory is unresolved; the Great Empire main castle is unavailable"
			detailDescriptors[key] = Localization.New("server.automation.owned_transfer_donor_inventory.fcf7bdd6", "Owned transfer donor inventory is unresolved; the Great Empire main castle is unavailable", nil)
			return nil
		}
		if workflow.SessionGeneration == 0 || workflow.SessionGeneration != snapshot.State.Session.ConnectionGeneration || workflow.TransportObservedAt.IsZero() {
			details[key] = "Refreshing current-session transport authority before donor reconciliation"
			detailDescriptors[key] = Localization.New("server.automation.refreshing_current_session_transport.c18be7a0", "Refreshing current-session transport authority before donor reconciliation", nil)
			if policy.supplyRefreshDue(workflow.KingdomID, snapshot.Now) {
				policy.markSupplyRefreshRequested(workflow.KingdomID, snapshot.Now)
				decision := autoFortressRequest(snapshot, metrics, "Refresh uncertain owned Direwolf shipment", "troops.kingdom.refresh", map[string]any{}, Localization.New("server.automation.refresh_uncertain_owned_direwolf.1d0c6481", "Refresh uncertain owned Direwolf shipment", nil))
				return &decision
			}
			return nil
		}
		if main.UnitsObservedAt.IsZero() || !main.UnitsObservedAt.After(workflow.TransportObservedAt) {
			details[key] = "Refreshing the donor after an ambiguous Direwolf dispatch"
			detailDescriptors[key] = Localization.New("server.automation.refreshing_the_donor_after.5a3e11cb", "Refreshing the donor after an ambiguous Direwolf dispatch", nil)
			decision := autoFortressRequest(snapshot, metrics, "Refresh Direwolf donor after ambiguous dispatch", "game.focus_castle", map[string]any{"castleId": main.ID, "refresh": true}, Localization.New("server.automation.refresh_direwolf_donor_after.62624b9c", "Refresh Direwolf donor after ambiguous dispatch", nil))
			return &decision
		}
		details[key] = "Confirming the authoritative donor inventory after shipment"
		detailDescriptors[key] = Localization.New("server.automation.confirming_the_authoritative_donor.e39f7da3", "Confirming the authoritative donor inventory after shipment", nil)
		decision := autoFortressRequest(snapshot, metrics, "Reconcile Direwolf donor after ambiguous dispatch", "troops.kingdom.reconcile_donor", map[string]any{
			"owner": workflow.Owner, "workflowId": workflow.ID, "targetKingdomId": workflow.KingdomID,
		}, Localization.New("server.automation.reconcile_direwolf_donor_after.3e8f7c6a", "Reconcile Direwolf donor after ambiguous dispatch", nil))
		return &decision
	}
	if !workflow.SkipRequestedAt.IsZero() {
		if workflow.Status == "skip_inventory_pending" || workflow.Status == "skip_uncertain" {
			observation := snapshot.State.Player.CurrencyObservations[workflow.SkipCurrencyID]
			needsRefresh := !observation.ObservedAt.After(workflow.SkipRequestedAt) || observation.ConnectionGeneration != snapshot.State.Session.ConnectionGeneration ||
				workflow.Status == "skip_uncertain" && !observation.ObservedAt.After(workflow.SkipInventoryObservedAt)
			if needsRefresh {
				details[key] = "Time-skip timer advanced; refreshing official skip inventory before another spend"
				detailDescriptors[key] = Localization.New("server.automation.time_skip_timer_advanced.adabd66d", "Time-skip timer advanced; refreshing official skip inventory before another spend", nil)
				if policy.supplyRefreshDue(workflow.KingdomID, snapshot.Now) {
					policy.markSupplyRefreshRequested(workflow.KingdomID, snapshot.Now)
					decision := autoFortressRequest(snapshot, metrics, "Refresh official time-skip inventory", "account.inventory.refresh", map[string]any{}, Localization.New("server.automation.refresh_official_time_skip.dbd0cda5", "Refresh official time-skip inventory", nil))
					return &decision
				}
				return nil
			}
			details[key] = "Reconciling official time-skip inventory consumption"
			detailDescriptors[key] = Localization.New("server.automation.reconciling_official_time_skip.56aa4a31", "Reconciling official time-skip inventory consumption", nil)
			decision := autoFortressRequest(snapshot, metrics, "Reconcile confirmed time-skip inventory", "troops.kingdom.skip.reconcile_inventory", map[string]any{
				"owner": workflow.Owner, "workflowId": workflow.ID, "targetKingdomId": workflow.KingdomID,
			}, Localization.New("server.automation.reconcile_confirmed_time_skip.241f2dc6", "Reconcile confirmed time-skip inventory", nil))
			return &decision
		}
		if !workflow.TransportObservedAt.After(workflow.SkipRequestedAt) ||
			!workflow.SkipTimerObservedAt.IsZero() && !workflow.TransportObservedAt.After(workflow.SkipTimerObservedAt) {
			details[key] = "Reconciling an uncertain time-skip reply before another spend"
			detailDescriptors[key] = Localization.New("server.automation.reconciling_an_uncertain_time.9c7b87c7", "Reconciling an uncertain time-skip reply before another spend", nil)
			if policy.supplyRefreshDue(workflow.KingdomID, snapshot.Now) {
				policy.markSupplyRefreshRequested(workflow.KingdomID, snapshot.Now)
				decision := autoFortressRequest(snapshot, metrics, "Refresh owned Direwolf transfer after uncertain skip", "troops.kingdom.refresh", map[string]any{}, Localization.New("server.automation.refresh_owned_direwolf_transfer.9eb2aca1", "Refresh owned Direwolf transfer after uncertain skip", nil))
				return &decision
			}
			return nil
		}
		details[key] = "Reconciling the owned transfer timer after a time skip"
		detailDescriptors[key] = Localization.New("server.automation.reconciling_the_owned_transfer.1d5fe14f", "Reconciling the owned transfer timer after a time skip", nil)
		decision := autoFortressRequest(snapshot, metrics, "Reconcile owned Direwolf transfer timer", "troops.kingdom.skip.reconcile_timer", map[string]any{
			"owner": workflow.Owner, "workflowId": workflow.ID, "targetKingdomId": workflow.KingdomID,
		}, Localization.New("server.automation.reconcile_owned_direwolf_transfer.5a7b3c44", "Reconcile owned Direwolf transfer timer", nil))
		return &decision
	}
	if workflow.Status == "awaiting_destination_refresh" || workflow.Status == "ownership_absent" {
		if !target.UnitsObservedAt.After(workflow.TransportObservedAt) {
			details[key] = "Transfer arrived; refreshing destination inventory before reuse"
			detailDescriptors[key] = Localization.New("server.automation.transfer_arrived_refreshing_destination.ac73401a", "Transfer arrived; refreshing destination inventory before reuse", nil)
			decision := autoFortressRequest(snapshot, metrics, "Refresh arrived Direwolves at "+castleName(target), "game.focus_castle", map[string]any{"castleId": target.ID, "refresh": true}, castleDecisionDescriptor("fortress_refresh_arrived", target, nil))
			return &decision
		}
		details[key] = "Settling the completed Direwolf transfer"
		detailDescriptors[key] = Localization.New("server.automation.settling_the_completed_direwolf.98b622a0", "Settling the completed Direwolf transfer", nil)
		decision := autoFortressRequest(snapshot, metrics, "Settle completed Direwolf transfer to "+castleName(target), "troops.kingdom.settle", map[string]any{
			"owner": workflow.Owner, "workflowId": workflow.ID, "targetKingdomId": workflow.KingdomID,
		}, castleDecisionDescriptor("fortress_settle_transfer", target, nil))
		return &decision
	}
	if workflow.Status != "pending" {
		details[key] = "Owned Direwolf shipment requires reconciliation"
		detailDescriptors[key] = Localization.New("server.automation.owned_direwolf_shipment_requires.2f7767f1", "Owned Direwolf shipment requires reconciliation", nil)
		return nil
	}
	details[key] = "Confirmed Direwolves are inbound"
	detailDescriptors[key] = Localization.New("server.automation.confirmed_direwolves_are_inbound.deadb392", "Confirmed Direwolves are inbound", nil)
	if !enabled {
		details[key] = "Destination is disabled; the confirmed Direwolves will arrive without a new skip or shipment"
		detailDescriptors[key] = Localization.New("server.automation.destination_is_disabled_the.151d672a", "Destination is disabled; the confirmed Direwolves will arrive without a new skip or shipment", nil)
		return nil
	}
	if !settings.UseTimeSkips {
		return nil
	}
	if workflow.TransportObservedAt.IsZero() || snapshot.Now.Before(workflow.TransportObservedAt) ||
		snapshot.Now.Sub(workflow.TransportObservedAt) > autoFortressUnitFreshness {
		details[key] = "Refreshing the owned Direwolf transfer before considering a time skip"
		detailDescriptors[key] = Localization.New("server.automation.refreshing_the_owned_direwolf.de3bd653", "Refreshing the owned Direwolf transfer before considering a time skip", nil)
		decision := autoFortressRequest(snapshot, metrics, "Refresh owned Direwolf transfer before a time skip", "troops.kingdom.refresh", map[string]any{}, Localization.New("server.automation.refresh_owned_direwolf_transfer.e05ddbb2", "Refresh owned Direwolf transfer before a time skip", nil))
		return &decision
	}
	remaining, found := autoFortressAgedOwnedPendingRemaining(snapshot.State, workflow, snapshot.Now)
	if !found || remaining <= 0 {
		return nil
	}
	option := autoFortressTimeSkipOption(snapshot, settings.TimeSkipReserve, remaining)
	if option.WireKey == "" {
		if autoFortressSkipInventoryNeedsRefresh(snapshot, settings.TimeSkipReserve) {
			details[key] = "Official time-skip inventory is stale; refreshing before reconsidering a spend"
			detailDescriptors[key] = Localization.New("server.automation.official_time_skip_inventory.c62574f9", "Official time-skip inventory is stale; refreshing before reconsidering a spend", nil)
			if policy.supplyRefreshDue(workflow.KingdomID, snapshot.Now) {
				policy.markSupplyRefreshRequested(workflow.KingdomID, snapshot.Now)
				decision := autoFortressRequest(snapshot, metrics, "Refresh stale official time-skip inventory", "account.inventory.refresh", map[string]any{}, Localization.New("server.automation.refresh_stale_official_time.7895c01e", "Refresh stale official time-skip inventory", nil))
				return &decision
			}
			return nil
		}
		details[key] = "No useful official time skip is available above the configured reserves; waiting naturally"
		detailDescriptors[key] = Localization.New("server.automation.no_useful_official_time.ae9f9f45", "No useful official time skip is available above the configured reserves; waiting naturally", nil)
		return nil
	}
	reserve := max(int64(0), settings.TimeSkipReserve[option.WireKey])
	details[key] = fmt.Sprintf("Applying %s with minimal duration waste", option.WireKey)
	detailDescriptors[key] = Localization.New("server.automation.applying_skip_with_minimal.ccc2f689", "Applying {skip} with minimal duration waste", Localization.Params{"skip": option.WireKey})
	decision := autoFortressRequest(snapshot, metrics, "Apply "+option.WireKey+" to the owned Direwolf transfer", "troops.kingdom.skip", map[string]any{
		"targetKingdomId": workflow.KingdomID, "timeSkipId": option.WireKey, "minimumRemaining": reserve,
		"owner": workflow.Owner, "workflowId": workflow.ID, "expectedRemaining": remaining, "expectedDurationSec": option.Seconds,
	}, Localization.New("server.automation.fortress_transfer_apply_skip", "Apply {skip} to the owned Direwolf transfer", Localization.Params{"skip": option.WireKey}))
	return &decision
}

func autoFortressOwnedPendingRemaining(gameState State.GameState, workflow State.KingdomTroopTransportWorkflow) (int, bool) {
	for _, pending := range gameState.KingdomTransport.PendingUnits {
		if pending.KingdomID != workflow.KingdomID {
			continue
		}
		if len(pending.Units) != len(workflow.Units) {
			continue
		}
		amounts := map[State.UnitID]int64{}
		for _, unit := range pending.Units {
			amounts[unit.UnitID] += unit.Amount
		}
		exact := true
		for _, unit := range workflow.Units {
			if amounts[unit.UnitID] != unit.Amount {
				exact = false
				break
			}
		}
		if exact {
			return pending.RemainingSec, true
		}
	}
	return 0, false
}

func autoFortressAgedOwnedPendingRemaining(gameState State.GameState, workflow State.KingdomTroopTransportWorkflow, now time.Time) (int, bool) {
	remaining, found := autoFortressOwnedPendingRemaining(gameState, workflow)
	if !found || workflow.TransportObservedAt.IsZero() || !now.After(workflow.TransportObservedAt) {
		return remaining, found
	}
	return max(0, remaining-int(now.Sub(workflow.TransportObservedAt)/time.Second)), true
}

func autoFortressTimeSkipOption(snapshot Snapshot, reserves map[string]int64, remaining int) GameData.TimeSkipOption {
	options, err := snapshot.GameData.OfficialTimeSkips()
	if err != nil {
		return GameData.TimeSkipOption{}
	}
	available := make([]GameData.TimeSkipOption, 0, len(options))
	for _, option := range options {
		balance := snapshot.State.Player.Currencies[State.CurrencyID(option.CurrencyID)]
		observation := snapshot.State.Player.CurrencyObservations[State.CurrencyID(option.CurrencyID)]
		if observation.ConnectionGeneration == 0 || observation.ConnectionGeneration != snapshot.State.Session.ConnectionGeneration ||
			observation.ObservedAt.IsZero() || snapshot.Now.Before(observation.ObservedAt) || snapshot.Now.Sub(observation.ObservedAt) > autoFortressUnitFreshness ||
			balance < float64(max(int64(0), reserves[option.WireKey]))+1 {
			continue
		}
		available = append(available, option)
	}
	if len(available) == 0 {
		return GameData.TimeSkipOption{}
	}
	sort.Slice(available, func(left, right int) bool {
		leftUnder, rightUnder := available[left].Seconds <= int64(remaining), available[right].Seconds <= int64(remaining)
		if leftUnder != rightUnder {
			return leftUnder
		}
		if leftUnder && available[left].Seconds != available[right].Seconds {
			return available[left].Seconds > available[right].Seconds
		}
		if !leftUnder && available[left].Seconds != available[right].Seconds {
			return available[left].Seconds < available[right].Seconds
		}
		return available[left].WireKey < available[right].WireKey
	})
	selected := available[0]
	if selected.Seconds > int64(remaining) && !autoFortressSkipUseful(selected.Seconds, remaining) {
		return GameData.TimeSkipOption{}
	}
	return selected
}

func autoFortressSkipUseful(duration int64, remaining int) bool {
	if duration <= 0 || remaining <= 0 {
		return false
	}
	if duration <= int64(remaining) {
		return true
	}
	maximumWaste := max(int64(60), min(int64(15*time.Minute/time.Second), int64(remaining)/4))
	return duration-int64(remaining) <= maximumWaste
}

func autoFortressSkipInventoryNeedsRefresh(snapshot Snapshot, reserves map[string]int64) bool {
	options, err := snapshot.GameData.OfficialTimeSkips()
	if err != nil {
		return false
	}
	for _, option := range options {
		currencyID := State.CurrencyID(option.CurrencyID)
		if snapshot.State.Player.Currencies[currencyID] < float64(max(int64(0), reserves[option.WireKey]))+1 {
			continue
		}
		observation := snapshot.State.Player.CurrencyObservations[currencyID]
		if observation.ConnectionGeneration == 0 || observation.ConnectionGeneration != snapshot.State.Session.ConnectionGeneration ||
			observation.ObservedAt.IsZero() || snapshot.Now.Before(observation.ObservedAt) || snapshot.Now.Sub(observation.ObservedAt) > autoFortressUnitFreshness {
			return true
		}
	}
	return false
}

func (policy *AutoFortressPolicy) supplyRefreshDue(kingdomID State.KingdomID, now time.Time) bool {
	policy.mu.Lock()
	defer policy.mu.Unlock()
	last := policy.lastSupplyRefreshRequested[kingdomID]
	return last.IsZero() || now.Sub(last) >= 30*time.Second
}

func (policy *AutoFortressPolicy) markSupplyRefreshRequested(kingdomID State.KingdomID, now time.Time) {
	policy.mu.Lock()
	policy.lastSupplyRefreshRequested[kingdomID] = now
	policy.mu.Unlock()
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
		decision := autoFortressRequest(snapshot, metrics, "Refresh Nomad Direwolf stock counters", "autoBuyer.package.history", map[string]any{"sourceCastleId": main.ID}, Localization.New("server.automation.refresh_nomad_direwolf_stock.3e1531f4", "Refresh Nomad Direwolf stock counters", nil))
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
		}, Localization.New("server.automation.fortress_buy_direwolves", "Buy {troops, number} Direwolves from the cheapest remaining Nomad tier", Localization.Params{"troops": amount * product.UnitAmount}))
		return &decision, ""
	}
	metrics["direwolvesPurchasedThisSession"] = float64(purchasedUnits)
	return nil, "Direwolf session limit is satisfied"
}

func autoFortressRequest(snapshot Snapshot, metrics map[string]float64, detail, name string, arguments any, descriptors ...*Localization.Message) Decision {
	raw, _ := json.Marshal(arguments)
	return Decision{
		Status: "ready", Detail: detail, DetailDescriptor: Localization.First(descriptors), NextCheckAt: snapshot.Now.Add(2 * time.Second), Metrics: metrics,
		Request: &Intent.Request{Name: name, Arguments: raw}, ReevaluateOnSuccess: true, ReevaluateOnStale: true,
	}
}

func autoFortressWaiting(snapshot Snapshot, detail string, metrics map[string]float64, descriptors ...*Localization.Message) Decision {
	seconds := snapshotFortressCheckInterval(snapshot)
	if seconds < 1 || seconds > 3600 {
		seconds = 30
	}
	return Decision{Status: "waiting", Detail: detail, DetailDescriptor: Localization.First(descriptors), NextCheckAt: snapshot.Now.Add(time.Duration(seconds) * time.Second), Metrics: metrics}
}

func snapshotFortressCheckInterval(snapshot Snapshot) int {
	settings := defaultAutoFortressSettings()
	if decodeSection(snapshot.Configuration, autoFortressSection, &settings) {
		return max(1, settings.CheckIntervalSec)
	}
	return 30
}

// Optional maps keep internal test callers compatible while the policy supplies
// one shared presentation map alongside the raw detail map.
func detailDescriptorMap(maps []map[string]*Localization.Message) map[string]*Localization.Message {
	if len(maps) > 0 && maps[0] != nil {
		return maps[0]
	}
	return map[string]*Localization.Message{}
}

package Automation

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	kingdomTowerMapTypeID                 = 2
	autoTowerTargetVerificationAge        = 30 * time.Second
	autoTowerCapacityObservationFreshness = time.Hour
	autoTowerBaronAdvisorTypeID           = 4
	autoTowerBaronSubscriptionTypeID      = 4
	autoTowerBaronTokenCurrencyID         = State.CurrencyID(79)
	autoTowerAdvisorMinimumAttackCount    = 2
	autoTowerAdvisorMaximumAttackCount    = 9999
	autoTowerAdvisorCooldownSeconds       = int64(3 * time.Hour / time.Second)
)

type AutoTowerPolicy struct{}

type autoTowerSettings struct {
	CheckIntervalSec      int                        `json:"checkIntervalSec"`
	MapRefreshIntervalSec int                        `json:"mapRefreshIntervalSec"`
	DailyAttackLimit      int64                      `json:"dailyAttackLimit"`
	HorseTravelBoostID    int                        `json:"horseTravelBoostId"`
	UseAdvisor            bool                       `json:"useAdvisor"`
	AutoActivateAdvisor   bool                       `json:"autoActivateAdvisor"`
	MaximumDailyTimeSkips int64                      `json:"maximumDailyTimeSkips"`
	Castles               map[string]autoTowerCastle `json:"castles"`
}

type autoTowerCastle struct {
	Enabled    bool         `json:"enabled"`
	Radius     int          `json:"radius"`
	UnitID     State.UnitID `json:"unitId"`
	MaidenOnly bool         `json:"maidenOnly"`
}

type towerQueueCandidate struct {
	Castle State.CastleState
	Plan   autoTowerCastle
	Entry  State.TowerQueueEntry
}

func NewAutoTowerPolicy() *AutoTowerPolicy { return &AutoTowerPolicy{} }

func (*AutoTowerPolicy) ID() string { return "autoTowers" }

func (*AutoTowerPolicy) EnabledKey() string { return "auto_towers" }

func (*AutoTowerPolicy) WakeDomains() []string {
	return []string{
		"advisor", "attacks", "building-layout", "commanders", "currencies", "map-tower", "movements",
		"subscriptions", "tower-cooldowns", "tower-queue", "units",
	}
}

func (*AutoTowerPolicy) WakeSections() []string {
	return []string{"automation.autoTowers", commanderFeatureSection}
}

func (*AutoTowerPolicy) ConfigurationDerivedStateSections() []string {
	return []string{"automation.autoTowers"}
}

func (*AutoTowerPolicy) ConfigurationDerivedStateComponents() State.ComponentSet {
	return State.Components(State.ComponentTowerQueue)
}

func (*AutoTowerPolicy) ResetConfigurationDerivedState(gameState *State.GameState) ([]string, bool) {
	if gameState == nil || len(gameState.TowerQueue.EntriesByCastle) == 0 &&
		len(gameState.TowerQueue.LastScannedAt) == 0 && len(gameState.TowerQueue.LastAttemptedAt) == 0 &&
		len(gameState.TowerQueue.ConfirmedLaunchesByCastle) == 0 {
		return nil, false
	}
	gameState.ReplaceTowerQueue(State.TowerQueueState{
		EntriesByCastle: map[State.CastleID][]State.TowerQueueEntry{},
		LastScannedAt:   map[State.CastleID]time.Time{}, LastAttemptedAt: map[State.CastleID]time.Time{},
		ConfirmedLaunchesByCastle: map[State.CastleID]int64{},
		CursorVersion:             gameState.TowerQueue.CursorVersion,
		CapacityByCastle:          gameState.TowerQueue.CapacityByCastle,
	})
	return []string{"tower-queue"}, true
}

func (*AutoTowerPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {

	settings := autoTowerSettings{CheckIntervalSec: 30, MapRefreshIntervalSec: 1800, HorseTravelBoostID: -1, Castles: map[string]autoTowerCastle{}}
	settingsConfigured := decodeSection(snapshot.Configuration, "automation.autoTowers", &settings)
	if !settingsConfigured || len(settings.Castles) == 0 {
		return Decision{
			Status: "waiting", Detail: "No tower castles are configured", DetailDescriptor: Localization.New("server.automation.no_tower_castles_are.bb947c54", "No tower castles are configured", nil),
			EventDriven: true,
		}, nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return Decision{Status: "waiting", Detail: "Choose a supported horse travel boost", DetailDescriptor: Localization.New("server.automation.choose_a_supported_horse.0d7016a8", "Choose a supported horse travel boost", nil), EventDriven: true}, nil
	}
	if settings.MaximumDailyTimeSkips < 0 {
		return Decision{Status: "waiting", Detail: "Maximum daily Advisor Time Skips cannot be negative", DetailDescriptor: Localization.New("server.automation.maximum_daily_advisor_time.0db54277", "Maximum daily Advisor Time Skips cannot be negative", nil), EventDriven: true}, nil
	}
	if settings.UseAdvisor && settings.MaximumDailyTimeSkips == 0 {
		return Decision{
			Status: "waiting", Detail: "Set a positive maximum daily Time Skip limit before using the Baron Advisor", DetailDescriptor: Localization.New("server.automation.set_a_positive_maximum.202e2322", "Set a positive maximum daily Time Skip limit before using the Baron Advisor", nil),
			EventDriven: true,
		}, nil
	}
	commanderIDs, commandersRestricted := commanderFeatureCandidates(
		snapshot.State,
		snapshot.Configuration,
		"autoTowers",
	)
	if commandersRestricted && len(commanderIDs) == 0 {
		return Decision{
			Status: "waiting", Detail: "No commanders are assigned to Auto Towers", DetailDescriptor: Localization.New("server.automation.no_commanders_are_assigned.e4c6d04f", "No commanders are assigned to Auto Towers", nil),
			EventDriven: true,
		}, nil
	}
	if cooldownTarget, found := pendingTowerCooldownRefresh(snapshot.State); found {
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": cooldownTarget.KingdomID,
			"x1":        cooldownTarget.X, "y1": cooldownTarget.Y,
			"x2": cooldownTarget.X, "y2": cooldownTarget.Y,
		})
		return Decision{
			Status: "ready",
			Detail: fmt.Sprintf("Refresh cooldown after tower battle at %d:%d", cooldownTarget.X, cooldownTarget.Y), DetailDescriptor: Localization.New("server.automation.refresh_cooldown_after_tower.9b84a25a", "Refresh cooldown after tower battle at {p0}:{p1}", Localization.Params{"p0": cooldownTarget.X, "p1": cooldownTarget.Y}),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "map.query", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, nil
	}
	filteredSettings, unsupportedHorseCastles, horseDecision, err := filterAutoTowerHorseTravelBoostCastles(snapshot, settings)
	if err != nil {
		return Decision{}, err
	}
	if horseDecision != nil {
		return *horseDecision, nil
	}
	settings = filteredSettings
	nextCastleSchedule := nextAutoTowerScheduleOpening(snapshot, settings)

	if castle, plan, found := nextTowerQueueScan(snapshot, settings, towerMapRefreshInterval(settings.MapRefreshIntervalSec)); found {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": castle.ID, "radius": clampTowerRadius(plan.Radius),
			"scanStartedAt": snapshot.Now,
		})
		return Decision{
			Status: "ready",
			Detail: fmt.Sprintf("Refresh complete tower map around %s", castleName(castle)), DetailDescriptor: Localization.New("server.automation.refresh_complete_tower_map.2dcabea9", "Refresh complete tower map around {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "tower.queue.scan", Arguments: arguments},
			ScheduleKey:         towerCastleScheduleKey(castle.ID),
			ReevaluateOnSuccess: true,
		}, nil
	}

	candidates, activeCount, configured := queuedTowerCandidates(snapshot, settings)
	metrics := map[string]float64{
		"activeTowers": float64(activeCount), "queuedTowers": float64(len(candidates)),
	}
	maximumAdvisorAttacks := 1
	if settings.UseAdvisor {
		dailyTimeSkipAllowance, blocked := autoTowerAdvisorDailyTimeSkipAllowance(
			snapshot, settings.MaximumDailyTimeSkips, policyInterval(settings.CheckIntervalSec, 30), metrics,
		)
		if blocked != nil {
			return *blocked, nil
		}
		inventoryTimeSkips := oneCommandDungeonSkipCount(snapshot.State, nil, autoTowerAdvisorCooldownSeconds)
		plannedTimeSkips := min(
			int64(autoTowerAdvisorMaximumAttackCount-1), dailyTimeSkipAllowance, inventoryTimeSkips,
		)
		metrics["advisorTimeSkipInventoryCapacity"] = float64(inventoryTimeSkips)
		metrics["plannedAdvisorTimeSkips"] = float64(plannedTimeSkips)
		if plannedTimeSkips < 1 {
			return Decision{
				Status: "waiting", Detail: "Baron Advisor chaining needs at least one Time Skip that covers the three-hour tower cooldown", DetailDescriptor: Localization.New("server.automation.baron_advisor_chaining_needs.d6347f08", "Baron Advisor chaining needs at least one Time Skip that covers the three-hour tower cooldown", nil),
				EventDriven: true, Metrics: metrics,
			}, nil
		}
		maximumAdvisorAttacks = 1 + int(plannedTimeSkips)
	}
	if unsupportedHorseCastles > 0 {
		metrics["unsupportedHorseCastles"] = float64(unsupportedHorseCastles)
	}
	var selected towerQueueCandidate
	var selectedCommanderID State.CommanderID
	selectedAdvisorAttackCount := 0
	var firstCapacityError error
	firstTroopShortage := ""
	for _, candidate := range candidates {
		commanderID, commanderAvailable := nextAutoTowerCommander(
			snapshot.State, commanderIDs, commandersRestricted, candidate.Plan.MaidenOnly, snapshot.Now,
		)
		if !commanderAvailable {
			detail := "No commander is currently available"
			var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.no_commander_is_currently.25dd6b1e", "No commander is currently available", nil)
			if commandersRestricted {
				detail = "No assigned Auto Towers commander is currently available"
				detailLocalizationMessage = Localization.New("server.automation.no_assigned_auto_towers.35e91608", "No assigned Auto Towers commander is currently available", nil)
			}
			if candidate.Plan.MaidenOnly {
				detail = "No available commander supports the required maiden relic"
				detailLocalizationMessage = Localization.New("server.automation.no_available_commander_supports.629a4be8", "No available commander supports the required maiden relic", nil)
				if commandersRestricted {
					detail = "No available assigned Auto Towers commander supports the required maiden relic"
					detailLocalizationMessage = Localization.New("server.automation.no_available_assigned_auto.58ec76f5", "No available assigned Auto Towers commander supports the required maiden relic", nil)
				}
			}
			return Decision{
				Status: "waiting", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
				EventDriven: true, Metrics: metrics,
			}, nil
		}
		if snapshot.GameData != nil {
			required, err := autoTowerCapacityRequirement(snapshot, candidate, commanderID)
			if err != nil {
				if firstCapacityError == nil {
					firstCapacityError = err
				}
				continue
			}
			required += autoTowerCapacityCorrection(snapshot.State, candidate.Castle.ID, snapshot.Now)
			available := max(int64(0), candidate.Castle.Units.Stationed[candidate.Plan.UnitID])
			attackCount := 1
			if settings.UseAdvisor {
				attackCount = maximumAdvisorAttacks
				if required > 0 {
					attackCount = min(attackCount, int(available/required))
				}
			}
			repeatedRequired, valid := autoTowerRepeatedUnitRequirement(required, attackCount)
			if settings.UseAdvisor && attackCount < autoTowerAdvisorMinimumAttackCount {
				repeatedRequired, _ = autoTowerRepeatedUnitRequirement(required, autoTowerAdvisorMinimumAttackCount)
			}
			if !valid || settings.UseAdvisor && attackCount < autoTowerAdvisorMinimumAttackCount || available < repeatedRequired {
				if firstTroopShortage == "" {
					firstTroopShortage = fmt.Sprintf(
						"%s has %d of unit %d; its next %d-hit tower launch requires %d",
						castleName(candidate.Castle), available, candidate.Plan.UnitID, max(1, attackCount), repeatedRequired,
					)
				}
				continue
			}
			if settings.UseAdvisor {
				selectedAdvisorAttackCount = attackCount
			}
		} else if settings.UseAdvisor {
			selectedAdvisorAttackCount = maximumAdvisorAttacks
		}
		selected = candidate
		selectedCommanderID = commanderID
		break
	}
	if selected.Castle.ID > 0 {
		if settings.UseAdvisor {
			metrics["plannedAdvisorAttacks"] = float64(selectedAdvisorAttackCount)
		}
		target, _ := snapshot.State.LookupMapObservation(selected.Entry.KingdomID, fmt.Sprintf("%d:%d", selected.Entry.TargetX, selected.Entry.TargetY))
		if target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) >= autoTowerTargetVerificationAge {
			arguments, _ := json.Marshal(map[string]any{
				"sourceCastleId":   selected.Castle.ID,
				"kingdomId":        selected.Entry.KingdomID,
				"targetX":          selected.Entry.TargetX,
				"targetY":          selected.Entry.TargetY,
				"refreshStartedAt": snapshot.Now,
			})
			return Decision{
				Status: "ready",
				Detail: fmt.Sprintf("Refresh queued tower target %d:%d; rotate it back if unchanged", selected.Entry.TargetX, selected.Entry.TargetY), DetailDescriptor: Localization.New("server.automation.refresh_queued_tower_target.a45c4ae9", "Refresh queued tower target {p0}:{p1}; rotate it back if unchanged", Localization.Params{"p0": selected.Entry.TargetX, "p1": selected.Entry.TargetY}),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Metrics:             metrics,
				Request:             &Intent.Request{Name: "tower.queue.target.refresh", Arguments: arguments},
				ScheduleKey:         towerCastleScheduleKey(selected.Castle.ID),
				ReevaluateOnSuccess: true,
			}, nil
		}
		if _, blocked := dailyAttackLimitAllowance(
			snapshot, settings.DailyAttackLimit, policyInterval(settings.CheckIntervalSec, 30), metrics,
		); blocked != nil {
			return *blocked, nil
		}
		if settings.UseAdvisor && !autoTowerBaronAdvisorActive(snapshot.State) {
			if snapshot.GameData == nil {
				return Decision{
					Status: "waiting", Detail: "Official game data is unavailable; the Baron Advisor token will not be activated yet", DetailDescriptor: Localization.New("server.automation.official_game_data_is.8eb23e76", "Official game data is unavailable; the Baron Advisor token will not be activated yet", nil),
					NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)), Metrics: metrics,
				}, nil
			}
			if settings.AutoActivateAdvisor && snapshot.State.Player.Currencies[autoTowerBaronTokenCurrencyID] >= 1 {
				arguments, _ := json.Marshal(map[string]any{"confirmedTokenSpend": true})
				return Decision{
					Status: "ready",
					Detail: fmt.Sprintf("Activate the Baron Advisor for ready tower %d:%d with one available token", selected.Entry.TargetX, selected.Entry.TargetY), DetailDescriptor: Localization.New("server.automation.activate_the_baron_advisor.22417a5c", "Activate the Baron Advisor for ready tower {p0}:{p1} with one available token", Localization.Params{"p0": selected.Entry.TargetX, "p1": selected.Entry.TargetY}),
					NextCheckAt:         snapshot.Now.Add(2 * time.Second),
					Metrics:             metrics,
					Request:             &Intent.Request{Name: "tower.advisor.activate", Arguments: arguments},
					ScheduleKey:         towerCastleScheduleKey(selected.Castle.ID),
					ReevaluateOnSuccess: true,
					ReevaluateOnStale:   true,
				}, nil
			}
			detail := "Baron Advisor mode is selected, but the Advisor is not active"
			var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.baron_advisor_mode_is.c255b8a9", "Baron Advisor mode is selected, but the Advisor is not active", nil)
			if settings.AutoActivateAdvisor {
				detail += fmt.Sprintf(" and no Baron Advisor token (currency %d) is available", autoTowerBaronTokenCurrencyID)
				detailLocalizationMessage = nil
			}
			return Decision{
				Status: "waiting", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), EventDriven: true, Metrics: metrics,
				ScheduleKey: towerCastleScheduleKey(selected.Castle.ID),
			}, nil
		}
		attackArguments := map[string]any{
			"sourceCastleId": selected.Castle.ID, "kingdomId": selected.Entry.KingdomID,
			"targetX": selected.Entry.TargetX, "targetY": selected.Entry.TargetY,
			"unitId": selected.Plan.UnitID, "maidenOnly": selected.Plan.MaidenOnly,
			"horseTravelBoostId": settings.HorseTravelBoostID, "dailyAttackLimit": settings.DailyAttackLimit,
			"commanderIds": []State.CommanderID{selectedCommanderID},
		}
		if settings.UseAdvisor {
			attackArguments["advisorMode"] = true
			attackArguments["advisorAttackCount"] = selectedAdvisorAttackCount
			attackArguments["maximumDailyTimeSkips"] = settings.MaximumDailyTimeSkips
		}
		arguments, _ := json.Marshal(attackArguments)
		return Decision{
			Status:              "ready",
			Detail:              autoTowerLaunchDetail(settings, selected, selectedAdvisorAttackCount),
			DetailDescriptor:    autoTowerLaunchDescriptor(settings, selected, selectedAdvisorAttackCount),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Metrics:             metrics,
			Request:             &Intent.Request{Name: "tower.attack", Arguments: arguments},
			ScheduleKey:         towerCastleScheduleKey(selected.Castle.ID),
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	if len(candidates) > 0 {
		detail := "No queued tower candidate is ready"
		var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.no_queued_tower_candidate.fb5fec86", "No queued tower candidate is ready", nil)
		if firstTroopShortage != "" {
			detail = "Waiting for tower troops: " + firstTroopShortage
			detailLocalizationMessage = nil
		} else if firstCapacityError != nil {
			if decision, refresh := generalSkillsRefreshDecision(firstCapacityError, snapshot.Now, metrics); refresh {
				return decision, nil
			}
			detail = "Cannot calculate tower troop requirements: " + firstCapacityError.Error()
			detailLocalizationMessage = nil
		}
		return Decision{
			Status: "waiting", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
			Metrics:     metrics,
		}, nil
	}

	detail := "No enabled castle has a queued tower target to launch"
	var detailLocalizationMessage *Localization.Message = Localization.New("server.automation.no_enabled_castle_has.2a725a0e", "No enabled castle has a queued tower target to launch", nil)
	if configured == 0 {
		detail = "No enabled castle has a troop configured"
		detailLocalizationMessage = Localization.New("server.automation.no_enabled_castle_has.72b05111", "No enabled castle has a troop configured", nil)
	} else if activeCount > 0 {
		detail = "No additional tower target is ready; active tower movements continue independently"
		detailLocalizationMessage = Localization.New("server.automation.no_additional_tower_target.f48b64fe", "No additional tower target is ready; active tower movements continue independently", nil)
	}
	if unsupportedHorseCastles > 0 {
		detail += fmt.Sprintf("; %d configured castle(s) do not support the selected horse travel boost", unsupportedHorseCastles)
		detailLocalizationMessage = nil
	}
	nextCheck := snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30))
	if !nextCastleSchedule.IsZero() && nextCastleSchedule.Before(nextCheck) {
		nextCheck = nextCastleSchedule
	}
	return Decision{
		Status: "idle", Detail: detail, DetailDescriptor: Localization.Clone(detailLocalizationMessage), NextCheckAt: nextCheck,
		Metrics: metrics,
	}, nil
}

func autoTowerAdvisorDailyTimeSkipAllowance(
	snapshot Snapshot,
	maximum int64,
	interval time.Duration,
	metrics map[string]float64,
) (int64, *Decision) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	if metrics != nil {
		metrics["maximumDailyAdvisorTimeSkips"] = float64(maximum)
	}
	if maximum <= 0 {
		return 0, &Decision{
			Status: "waiting", Detail: "Set a positive maximum daily Time Skip limit before using the Baron Advisor", DetailDescriptor: Localization.New("server.automation.set_a_positive_maximum.202e2322", "Set a positive maximum daily Time Skip limit before using the Baron Advisor", nil),
			EventDriven: true, Metrics: metrics,
		}
	}
	attacks := snapshot.State.DailyAttacks
	if attacks.ObservedAt.IsZero() || attacks.SessionStartedAt.IsZero() {
		return 0, &Decision{
			Status: "waiting", Detail: "Waiting for the authoritative server daily reset before using Advisor Time Skips", DetailDescriptor: Localization.New("server.automation.waiting_for_the_authoritative.fe492a87", "Waiting for the authoritative server daily reset before using Advisor Time Skips", nil),
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	used, exact := State.TowerAdvisorTimeSkipsUsedSince(snapshot.State, attacks.SessionStartedAt, snapshot.Now)
	if !exact {
		return 0, &Decision{
			Status: "waiting", Detail: "Cannot establish exact Auto Towers Advisor Time Skip usage for the current server day", DetailDescriptor: Localization.New("server.automation.cannot_establish_exact_auto.f1bde5b1", "Cannot establish exact Auto Towers Advisor Time Skip usage for the current server day", nil),
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	remaining := max(int64(0), maximum-used)
	if metrics != nil {
		metrics["advisorTimeSkipsUsedToday"] = float64(used)
		metrics["advisorTimeSkipsRemainingToday"] = float64(remaining)
	}
	if remaining == 0 {
		return 0, &Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"Daily Auto Towers Advisor Time Skip limit reached: %d / %d; chaining resumes when the server daily attack count resets",
				used, maximum,
			), DetailDescriptor: Localization.New("server.automation.daily_auto_towers_advisor.9d171427", "Daily Auto Towers Advisor Time Skip limit reached: {p0} / {p1}; chaining resumes when the server daily attack count resets", Localization.Params{"p0": used, "p1": maximum}),
			NextCheckAt: snapshot.Now.Add(interval), Metrics: metrics,
		}
	}
	return remaining, nil
}

func filterAutoTowerHorseTravelBoostCastles(
	snapshot Snapshot,
	settings autoTowerSettings,
) (autoTowerSettings, int, *Decision, error) {
	if settings.HorseTravelBoostID == 0 || settings.HorseTravelBoostID == -1 {
		return settings, 0, nil, nil
	}
	if snapshot.GameData == nil {
		return settings, 0, &Decision{
			Status: "waiting", Detail: "Waiting for official game data to resolve the selected horse travel boost", DetailDescriptor: Localization.New("server.automation.waiting_for_official_game.7059ce31", "Waiting for official game data to resolve the selected horse travel boost", nil),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
		}, nil
	}
	filtered := make(map[string]autoTowerCastle, len(settings.Castles))
	configured := 0
	unsupported := 0
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		plan := settings.Castles[castleKey]
		if !plan.Enabled || plan.UnitID <= 0 {
			continue
		}
		castleID, _ := strconv.ParseInt(castleKey, 10, 64)
		castle, exists := snapshot.State.Castles[State.CastleID(castleID)]
		if !exists {
			continue
		}
		configured++
		if horseTravelBoostLayoutNeedsRefresh(
			castle, snapshot.Now, towerMapRefreshInterval(settings.MapRefreshIntervalSec),
		) {
			if allowed, _ := scheduleAllows(snapshot.Configuration, "autoTowers:"+castleKey, snapshot.Now); !allowed {
				filtered[castleKey] = plan
				continue
			}
			arguments, _ := json.Marshal(map[string]any{"castleId": castle.ID, "refresh": true})
			return settings, unsupported, &Decision{
				Status: "ready",
				Detail: fmt.Sprintf("Refresh travel-building state at %s", castleName(castle)), DetailDescriptor: Localization.New("server.automation.refresh_travel_building_state.7d626275", "Refresh travel-building state at {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ScheduleKey:         towerCastleScheduleKey(castle.ID),
				ReevaluateOnSuccess: true,
			}, nil
		}
		err := resolvePolicyHorseTravelBoost(snapshot.GameData, castle, settings.HorseTravelBoostID)
		switch {
		case err == nil:
			filtered[castleKey] = plan
		case errors.Is(err, GameData.ErrHorseTravelBoostLayoutUnobserved):
			arguments, _ := json.Marshal(map[string]any{"castleId": castle.ID, "refresh": true})
			return settings, unsupported, &Decision{
				Status: "ready",
				Detail: fmt.Sprintf("Refresh travel-building state at %s", castleName(castle)), DetailDescriptor: Localization.New("server.automation.refresh_travel_building_state.7d626275", "Refresh travel-building state at {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleName(castle))}),
				NextCheckAt:         snapshot.Now.Add(2 * time.Second),
				Request:             &Intent.Request{Name: "game.focus_castle", Arguments: arguments},
				ScheduleKey:         towerCastleScheduleKey(castle.ID),
				ReevaluateOnSuccess: true,
			}, nil
		case errors.Is(err, GameData.ErrHorseTravelBoostUnavailable):
			unsupported++
		default:
			return settings, unsupported, nil, fmt.Errorf(
				"resolve selected horse travel boost at %s: %w", castleName(castle), err,
			)
		}
	}
	settings.Castles = filtered
	if configured > 0 && len(filtered) == 0 {
		return settings, unsupported, &Decision{
			Status: "waiting",
			Detail: fmt.Sprintf(
				"No configured Auto Towers castle supports the selected horse travel boost; %d castle(s) were skipped",
				unsupported,
			), DetailDescriptor: Localization.New("server.automation.no_configured_auto_towers.a3e456bf", "No configured Auto Towers castle supports the selected horse travel boost; {p0} castle(s) were skipped", Localization.Params{"p0": unsupported}),
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
			Metrics:     map[string]float64{"unsupportedHorseCastles": float64(unsupported)},
		}, nil
	}
	return settings, unsupported, nil, nil
}

func nextAutoTowerScheduleOpening(snapshot Snapshot, settings autoTowerSettings) time.Time {
	var earliest time.Time
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		plan := settings.Castles[castleKey]
		if !plan.Enabled || plan.UnitID <= 0 {
			continue
		}
		castleID, _ := strconv.ParseInt(castleKey, 10, 64)
		if _, exists := snapshot.State.Castles[State.CastleID(castleID)]; !exists {
			continue
		}
		allowed, next := scheduleAllows(snapshot.Configuration, "autoTowers:"+castleKey, snapshot.Now)
		if !allowed && !next.IsZero() && (earliest.IsZero() || next.Before(earliest)) {
			earliest = next
		}
	}
	return earliest
}

func queuedTowerCandidates(snapshot Snapshot, settings autoTowerSettings) ([]towerQueueCandidate, int, int) {
	candidates := make([]towerQueueCandidate, 0)
	reserved := activeTowerTargetKeys(snapshot.State, snapshot.Now)
	activeCount := 0
	configured := 0
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		plan := settings.Castles[castleKey]
		if !plan.Enabled || plan.UnitID <= 0 {
			continue
		}
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castle, exists := snapshot.State.Castles[State.CastleID(castleIDValue)]
		if !exists {
			continue
		}
		configured++
		if allowed, _ := scheduleAllows(snapshot.Configuration, "autoTowers:"+castleKey, snapshot.Now); !allowed {
			continue
		}
		active := activeTowerMovements(snapshot.State, castle.ID, snapshot.Now)
		activeCount += active
		radius := clampTowerRadius(plan.Radius)
		maximumDistanceSquared := radius * radius
		for _, entry := range snapshot.State.TowerQueue.EntriesByCastle[castle.ID] {
			if towerQueueEntryDistanceSquared(castle, entry) > maximumDistanceSquared {
				continue
			}
			if entry.DeferredUntil != nil && entry.DeferredUntil.After(snapshot.Now) {
				continue
			}
			key := towerTargetKey(entry.KingdomID, entry.TargetX, entry.TargetY)
			if _, locked := reserved[key]; locked || towerCooldownRefreshPending(snapshot.State, key) {
				continue
			}
			if State.AttackFeatureTargetPendingAt(
				snapshot.State, State.AttackFeatureAutoTowers, entry.KingdomID, kingdomTowerMapTypeID,
				entry.TargetX, entry.TargetY, snapshot.Now,
			) {
				continue
			}
			target, exists := snapshot.State.LookupMapObservation(entry.KingdomID, fmt.Sprintf("%d:%d", entry.TargetX, entry.TargetY))
			if !exists || target.TypeID != kingdomTowerMapTypeID ||
				towerCooldownRemaining(target, snapshot.Now) > 0 {
				continue
			}
			candidates = append(candidates, towerQueueCandidate{Castle: castle, Plan: plan, Entry: entry})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		return towerQueueCandidateLess(snapshot.State.TowerQueue, candidates[left], candidates[right])
	})
	return candidates, activeCount, configured
}

func towerQueueCandidateLess(queue State.TowerQueueState, left, right towerQueueCandidate) bool {
	leftLaunches := queue.ConfirmedLaunchesByCastle[left.Castle.ID]
	rightLaunches := queue.ConfirmedLaunchesByCastle[right.Castle.ID]
	if leftLaunches != rightLaunches {
		return leftLaunches < rightLaunches
	}
	leftAttemptedAt := queue.LastAttemptedAt[left.Castle.ID]
	rightAttemptedAt := queue.LastAttemptedAt[right.Castle.ID]
	if leftAttemptedAt.IsZero() != rightAttemptedAt.IsZero() {
		return leftAttemptedAt.IsZero()
	}
	if !leftAttemptedAt.Equal(rightAttemptedAt) {
		return leftAttemptedAt.Before(rightAttemptedAt)
	}
	if left.Castle.ID != right.Castle.ID {
		return left.Castle.ID < right.Castle.ID
	}
	if !left.Entry.QueuedAt.Equal(right.Entry.QueuedAt) {
		return left.Entry.QueuedAt.Before(right.Entry.QueuedAt)
	}
	leftDistance := towerQueueEntryDistanceSquared(left.Castle, left.Entry)
	rightDistance := towerQueueEntryDistanceSquared(right.Castle, right.Entry)
	if leftDistance != rightDistance {
		return leftDistance < rightDistance
	}
	if left.Entry.TargetY != right.Entry.TargetY {
		return left.Entry.TargetY < right.Entry.TargetY
	}
	return left.Entry.TargetX < right.Entry.TargetX
}

func nextTowerQueueCandidate(candidates []towerQueueCandidate) (towerQueueCandidate, bool) {
	if len(candidates) == 0 {
		return towerQueueCandidate{}, false
	}
	return candidates[0], true
}

func nextAutoTowerCommander(
	gameState State.GameState,
	candidates []State.CommanderID,
	restricted bool,
	maidenOnly bool,
	now time.Time,
) (State.CommanderID, bool) {
	if !restricted {
		candidates = make([]State.CommanderID, 0, len(gameState.Commanders))
		for commanderID := range gameState.Commanders {
			if commanderID >= 0 {
				candidates = append(candidates, commanderID)
			}
		}
		sort.Slice(candidates, func(left, right int) bool { return candidates[left] < candidates[right] })
	}
	for _, commanderID := range candidates {
		commander, exists := gameState.Commanders[commanderID]
		if !exists || !commander.Available || State.CommanderHasActiveMovementAt(gameState, commanderID, now) ||
			maidenOnly && !autoTowerCommanderSupportsMaiden(gameState, commanderID) {
			continue
		}
		return commanderID, true
	}
	return 0, false
}

func autoTowerCommanderSupportsMaiden(gameState State.GameState, commanderID State.CommanderID) bool {
	if commanderID <= 0 {
		return false
	}
	for _, equipment := range gameState.Inventory.Equipment {
		if equipment.WearerKind != "commander" || equipment.WearerID != int64(commanderID) ||
			equipment.RarityID != 5 && equipment.RarityID != 15 {
			continue
		}
		for _, effect := range equipment.Effects {
			if effect.WireID != 121 || len(effect.Values) == 0 {
				continue
			}
			value := effect.Values[len(effect.Values)-1]
			if value >= 300 && value <= 1050 {
				return true
			}
		}
	}
	return false
}

func autoTowerCapacityRequirement(
	snapshot Snapshot,
	candidate towerQueueCandidate,
	commanderID State.CommanderID,
) (int64, error) {
	target, exists := snapshot.State.LookupMapObservation(candidate.Entry.KingdomID, fmt.Sprintf("%d:%d", candidate.Entry.TargetX, candidate.Entry.TargetY))
	if !exists {
		return 0, Localization.WithError(fmt.Errorf("tower %d:%d is no longer in map state", candidate.Entry.TargetX, candidate.Entry.TargetY), Localization.New("server.automation.tower_p_p_is.a0c98b1c", "tower {p0}:{p1} is no longer in map state", Localization.Params{"p0": fmt.Sprintf("%d", candidate.Entry.TargetX), "p1": fmt.Sprintf("%d", candidate.Entry.TargetY)}))
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(snapshot.State, snapshot.GameData, AttackCapacity.Request{
		SourceCastleID: candidate.Castle.ID, CommanderID: commanderID,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("tower:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: target.Level, VictoryCount: target.TowerVictoryCount,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return 0, err
	}
	return capacity.Capacity.Left + capacity.Capacity.Right, nil
}

func autoTowerCapacityCorrection(gameState State.GameState, castleID State.CastleID, now time.Time) int64 {
	observation, exists := gameState.TowerQueue.CapacityByCastle[castleID]
	if !exists || observation.AdditionalUnits <= 0 || observation.ObservedAt.IsZero() ||
		now.Before(observation.ObservedAt) || now.Sub(observation.ObservedAt) > autoTowerCapacityObservationFreshness {
		return 0
	}
	return observation.AdditionalUnits
}

func nextTowerQueueScan(snapshot Snapshot, settings autoTowerSettings, refreshInterval time.Duration) (State.CastleState, autoTowerCastle, bool) {
	type candidate struct {
		castle  State.CastleState
		plan    autoTowerCastle
		scanned time.Time
	}
	candidates := make([]candidate, 0)
	for _, castleKey := range sortedNumericKeys(settings.Castles) {
		plan := settings.Castles[castleKey]
		if !plan.Enabled || plan.UnitID <= 0 {
			continue
		}
		if allowed, _ := scheduleAllows(snapshot.Configuration, "autoTowers:"+castleKey, snapshot.Now); !allowed {
			continue
		}
		castleIDValue, _ := strconv.ParseInt(castleKey, 10, 64)
		castle, exists := snapshot.State.Castles[State.CastleID(castleIDValue)]
		if !exists {
			continue
		}
		scanned := snapshot.State.TowerQueue.LastScannedAt[castle.ID]
		if !scanned.IsZero() && snapshot.Now.Sub(scanned) < refreshInterval {
			continue
		}
		candidates = append(candidates, candidate{castle: castle, plan: plan, scanned: scanned})
	}
	sort.Slice(candidates, func(left, right int) bool {
		if candidates[left].scanned.IsZero() != candidates[right].scanned.IsZero() {
			return candidates[left].scanned.IsZero()
		}
		if !candidates[left].scanned.Equal(candidates[right].scanned) {
			return candidates[left].scanned.Before(candidates[right].scanned)
		}
		return candidates[left].castle.ID < candidates[right].castle.ID
	})
	if len(candidates) == 0 {
		return State.CastleState{}, autoTowerCastle{}, false
	}
	return candidates[0].castle, candidates[0].plan, true
}

func clampTowerRadius(value int) int {
	if value < 1 {
		return 10
	}
	if value > 50 {
		return 50
	}
	return value
}

func towerCastleScheduleKey(castleID State.CastleID) string {
	return "autoTowers:" + strconv.FormatInt(int64(castleID), 10)
}

func towerMapRefreshInterval(value int) time.Duration {
	if value < 1800 || value > 3600 {
		value = 1800
	}
	return time.Duration(value) * time.Second
}

func towerQueueEntryDistanceSquared(castle State.CastleState, entry State.TowerQueueEntry) int {
	x := entry.TargetX - castle.X
	y := entry.TargetY - castle.Y
	return x*x + y*y
}

func activeTowerMovements(gameState State.GameState, castleID State.CastleID, now time.Time) int {
	count := 0
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if !towerMovementActiveAt(movement, now) {
			return true
		}
		if movement.Direction == 0 && movement.SourceCastleID == castleID && movement.TargetTypeID == kingdomTowerMapTypeID {
			count++
		}
		if movement.Direction == 1 && movement.TargetCastleID == castleID && movement.SourceTypeID == kingdomTowerMapTypeID {
			count++
		}
		return true
	})
	return count
}

func activeTowerTargetKeys(gameState State.GameState, now time.Time) map[string]struct{} {
	locked := map[string]struct{}{}
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if !towerMovementActiveAt(movement, now) {
			return true
		}
		switch {
		case movement.Direction == 0 && movement.TargetTypeID == kingdomTowerMapTypeID:
			locked[towerTargetKey(movement.KingdomID, movement.TargetX, movement.TargetY)] = struct{}{}
		case movement.Direction == 1 && movement.SourceTypeID == kingdomTowerMapTypeID:
			locked[towerTargetKey(movement.KingdomID, movement.SourceX, movement.SourceY)] = struct{}{}
		}
		return true
	})
	return locked
}

func towerMovementActiveAt(movement State.MovementState, now time.Time) bool {
	var completion *time.Time
	if movement.Direction == 0 {
		completion = movement.ArrivesAt
	} else if movement.Direction == 1 {
		completion = movement.ReturnsAt
	}
	return completion == nil || completion.IsZero() || completion.After(now)
}

func pendingTowerCooldownRefresh(gameState State.GameState) (State.TowerCooldownState, bool) {
	pending := make([]State.TowerCooldownState, 0)
	gameState.RangeTowerCooldowns(func(_ string, cooldown State.TowerCooldownState) bool {
		if cooldown.PendingCooldownRefresh && (cooldown.TargetTypeID == 0 || cooldown.TargetTypeID == kingdomTowerMapTypeID) {
			pending = append(pending, cooldown)
		}
		return true
	})
	sort.Slice(pending, func(left, right int) bool {
		if !pending[left].LastSuccessfulBattleAt.Equal(pending[right].LastSuccessfulBattleAt) {
			return pending[left].LastSuccessfulBattleAt.Before(pending[right].LastSuccessfulBattleAt)
		}
		if pending[left].KingdomID != pending[right].KingdomID {
			return pending[left].KingdomID < pending[right].KingdomID
		}
		if pending[left].Y != pending[right].Y {
			return pending[left].Y < pending[right].Y
		}
		return pending[left].X < pending[right].X
	})
	if len(pending) == 0 {
		return State.TowerCooldownState{}, false
	}
	return pending[0], true
}

func towerCooldownRefreshPending(gameState State.GameState, key string) bool {
	cooldown, found := gameState.LookupTowerCooldown(key)
	return found && cooldown.PendingCooldownRefresh
}

func towerCooldownRemaining(target State.MapObservation, now time.Time) int {
	if target.TowerCooldownRemaining <= 0 || target.ObservedAt.IsZero() {
		return 0
	}
	elapsed := int(now.Sub(target.ObservedAt) / time.Second)
	return max(0, target.TowerCooldownRemaining-elapsed)
}

func towerTargetKey(kingdomID State.KingdomID, x, y int) string {
	return fmt.Sprintf("%d:%d:%d", kingdomID, x, y)
}

func autoTowerBaronAdvisorActive(gameState State.GameState) bool {
	subscription, exists := gameState.Subscriptions[autoTowerBaronSubscriptionTypeID]
	return exists && subscription.TypeID == autoTowerBaronSubscriptionTypeID && subscription.RemainingSec > 0
}

func autoTowerRepeatedUnitRequirement(perAttack int64, count int) (int64, bool) {
	if perAttack <= 0 || count <= 0 || perAttack > math.MaxInt64/int64(count) {
		return math.MaxInt64, false
	}
	return perAttack * int64(count), true
}

func autoTowerLaunchDetail(settings autoTowerSettings, selected towerQueueCandidate, attackCount int) string {
	if settings.UseAdvisor {
		return fmt.Sprintf(
			"Chain %d Baron Advisor hits using %d Time Skips on tower %d:%d from %s",
			attackCount, attackCount-1, selected.Entry.TargetX, selected.Entry.TargetY, castleName(selected.Castle),
		)
	}
	return fmt.Sprintf(
		"Launch queued tower target %d:%d from %s",
		selected.Entry.TargetX, selected.Entry.TargetY, castleName(selected.Castle),
	)
}

func autoTowerLaunchDescriptor(settings autoTowerSettings, selected towerQueueCandidate, attackCount int) *Localization.Message {
	params := Localization.Params{"x": fmt.Sprint(selected.Entry.TargetX), "y": fmt.Sprint(selected.Entry.TargetY)}
	variant := "queued_tower"
	if settings.UseAdvisor {
		variant = "advisor_tower"
		params["attacks"] = attackCount
		params["skips"] = attackCount - 1
	}
	return castleDecisionDescriptor(variant, selected.Castle, params)
}

package Automation

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	kingdomTowerMapTypeID                 = 2
	autoTowerTargetVerificationAge        = 30 * time.Second
	autoTowerCapacityObservationFreshness = time.Hour
)

type AutoTowerPolicy struct{}

type autoTowerSettings struct {
	CheckIntervalSec      int                        `json:"checkIntervalSec"`
	MapRefreshIntervalSec int                        `json:"mapRefreshIntervalSec"`
	DailyAttackLimit      int64                      `json:"dailyAttackLimit"`
	HorseTravelBoostID    int                        `json:"horseTravelBoostId"`
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
	return []string{"attacks", "commanders", "map", "movements", "tower-cooldowns", "tower-queue", "units"}
}

func (*AutoTowerPolicy) WakeSections() []string {
	return []string{"automation.autoTowers", commanderFeatureSection}
}

func (*AutoTowerPolicy) Evaluate(_ context.Context, snapshot Snapshot) (Decision, error) {
	settings := autoTowerSettings{CheckIntervalSec: 30, MapRefreshIntervalSec: 1800, HorseTravelBoostID: -1, Castles: map[string]autoTowerCastle{}}
	if !decodeSection(snapshot.Configuration, "automation.autoTowers", &settings) || len(settings.Castles) == 0 {
		return Decision{
			Status: "waiting", Detail: "No tower castles are configured",
			NextCheckAt: snapshot.Now.Add(30 * time.Second),
		}, nil
	}
	if !validHorseTravelBoostID(settings.HorseTravelBoostID) {
		return Decision{Status: "waiting", Detail: "Choose a supported horse travel boost", NextCheckAt: snapshot.Now.Add(30 * time.Second)}, nil
	}
	commanderIDs, commandersRestricted := commanderFeatureCandidates(
		snapshot.State,
		snapshot.Configuration,
		"autoTowers",
	)
	if commandersRestricted && len(commanderIDs) == 0 {
		return Decision{
			Status: "waiting", Detail: "No commanders are assigned to Auto Towers",
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
		}, nil
	}
	nextCastleSchedule := nextAutoTowerScheduleOpening(snapshot, settings)
	if cooldownTarget, found := pendingTowerCooldownRefresh(snapshot.State); found {
		arguments, _ := json.Marshal(map[string]any{
			"kingdomId": cooldownTarget.KingdomID,
			"x1":        cooldownTarget.X, "y1": cooldownTarget.Y,
			"x2": cooldownTarget.X, "y2": cooldownTarget.Y,
		})
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Refresh cooldown after tower battle at %d:%d", cooldownTarget.X, cooldownTarget.Y),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Request:             &Intent.Request{Name: "map.query", Arguments: arguments},
			ReevaluateOnSuccess: true,
		}, nil
	}

	if castle, plan, found := nextTowerQueueScan(snapshot, settings, towerMapRefreshInterval(settings.MapRefreshIntervalSec)); found {
		arguments, _ := json.Marshal(map[string]any{
			"sourceCastleId": castle.ID, "radius": clampTowerRadius(plan.Radius),
			"scanStartedAt": snapshot.Now,
		})
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Refresh complete tower map around %s", castleName(castle)),
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
	var selected towerQueueCandidate
	var selectedCommanderID State.CommanderID
	var firstCapacityError error
	firstTroopShortage := ""
	commanderEligibleCandidates := 0
	for _, candidate := range candidates {
		commanderID, commanderAvailable := nextAutoTowerCommander(
			snapshot.State, commanderIDs, commandersRestricted, candidate.Plan.MaidenOnly, snapshot.Now,
		)
		if !commanderAvailable {
			continue
		}
		commanderEligibleCandidates++
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
			if available < required {
				if firstTroopShortage == "" {
					firstTroopShortage = fmt.Sprintf(
						"%s has %d of unit %d; its next full attack requires %d",
						castleName(candidate.Castle), available, candidate.Plan.UnitID, required,
					)
				}
				continue
			}
		}
		selected = candidate
		selectedCommanderID = commanderID
		break
	}
	if selected.Castle.ID > 0 {
		target := snapshot.State.Map[selected.Entry.KingdomID][fmt.Sprintf("%d:%d", selected.Entry.TargetX, selected.Entry.TargetY)]
		if target.ObservedAt.IsZero() || snapshot.Now.Sub(target.ObservedAt) >= autoTowerTargetVerificationAge {
			arguments, _ := json.Marshal(map[string]any{
				"sourceCastleId":   selected.Castle.ID,
				"kingdomId":        selected.Entry.KingdomID,
				"targetX":          selected.Entry.TargetX,
				"targetY":          selected.Entry.TargetY,
				"refreshStartedAt": snapshot.Now,
			})
			return Decision{
				Status:              "ready",
				Detail:              fmt.Sprintf("Refresh queued tower target %d:%d; rotate it back if unchanged", selected.Entry.TargetX, selected.Entry.TargetY),
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
		attackArguments := map[string]any{
			"sourceCastleId": selected.Castle.ID, "kingdomId": selected.Entry.KingdomID,
			"targetX": selected.Entry.TargetX, "targetY": selected.Entry.TargetY,
			"unitId": selected.Plan.UnitID, "maidenOnly": selected.Plan.MaidenOnly,
			"horseTravelBoostId": settings.HorseTravelBoostID, "dailyAttackLimit": settings.DailyAttackLimit,
			"commanderIds": []State.CommanderID{selectedCommanderID},
		}
		arguments, _ := json.Marshal(attackArguments)
		return Decision{
			Status:              "ready",
			Detail:              fmt.Sprintf("Launch queued tower target %d:%d from %s", selected.Entry.TargetX, selected.Entry.TargetY, castleName(selected.Castle)),
			NextCheckAt:         snapshot.Now.Add(2 * time.Second),
			Metrics:             metrics,
			Request:             &Intent.Request{Name: "tower.attack", Arguments: arguments},
			ScheduleKey:         towerCastleScheduleKey(selected.Castle.ID),
			ReevaluateOnSuccess: true,
			ReevaluateOnStale:   true,
		}, nil
	}
	if len(candidates) > 0 {
		detail := "No commander is currently available"
		if commandersRestricted {
			detail = "No assigned Auto Towers commander is currently available"
		}
		if commanderEligibleCandidates > 0 && firstTroopShortage != "" {
			detail = "Waiting for tower troops: " + firstTroopShortage
		} else if commanderEligibleCandidates > 0 && firstCapacityError != nil {
			detail = "Cannot calculate tower troop requirements: " + firstCapacityError.Error()
		} else if commanderEligibleCandidates == 0 {
			for _, candidate := range candidates {
				if !candidate.Plan.MaidenOnly {
					continue
				}
				detail = "No available commander supports the required maiden relic"
				if commandersRestricted {
					detail = "No available assigned Auto Towers commander supports the required maiden relic"
				}
				break
			}
		}
		return Decision{
			Status: "waiting", Detail: detail,
			NextCheckAt: snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30)),
			Metrics:     metrics,
		}, nil
	}

	detail := "No enabled castle has a queued tower target to launch"
	if configured == 0 {
		detail = "No enabled castle has a troop configured"
	} else if activeCount > 0 {
		detail = "No additional tower target is ready; active tower movements continue independently"
	}
	nextCheck := snapshot.Now.Add(policyInterval(settings.CheckIntervalSec, 30))
	if !nextCastleSchedule.IsZero() && nextCastleSchedule.Before(nextCheck) {
		nextCheck = nextCastleSchedule
	}
	return Decision{
		Status: "idle", Detail: detail, NextCheckAt: nextCheck,
		Metrics: metrics,
	}, nil
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
		for _, entry := range snapshot.State.TowerQueue.EntriesByCastle[castle.ID] {
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
			target, exists := snapshot.State.Map[entry.KingdomID][fmt.Sprintf("%d:%d", entry.TargetX, entry.TargetY)]
			if !exists || target.TypeID != kingdomTowerMapTypeID ||
				towerCooldownRemaining(target, snapshot.Now) > 0 {
				continue
			}
			candidates = append(candidates, towerQueueCandidate{Castle: castle, Plan: plan, Entry: entry})
		}
	}
	sort.Slice(candidates, func(left, right int) bool {
		leftDistance := towerQueueEntryDistanceSquared(candidates[left].Castle, candidates[left].Entry)
		rightDistance := towerQueueEntryDistanceSquared(candidates[right].Castle, candidates[right].Entry)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if candidates[left].Castle.ID != candidates[right].Castle.ID {
			return candidates[left].Castle.ID < candidates[right].Castle.ID
		}
		if candidates[left].Entry.TargetY != candidates[right].Entry.TargetY {
			return candidates[left].Entry.TargetY < candidates[right].Entry.TargetY
		}
		return candidates[left].Entry.TargetX < candidates[right].Entry.TargetX
	})
	available := make([]towerQueueCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		key := towerTargetKey(candidate.Entry.KingdomID, candidate.Entry.TargetX, candidate.Entry.TargetY)
		if _, locked := reserved[key]; locked {
			continue
		}
		reserved[key] = struct{}{}
		available = append(available, candidate)
	}
	sort.SliceStable(available, func(left, right int) bool {
		leftAttemptedAt := snapshot.State.TowerQueue.LastAttemptedAt[available[left].Castle.ID]
		rightAttemptedAt := snapshot.State.TowerQueue.LastAttemptedAt[available[right].Castle.ID]
		if leftAttemptedAt.IsZero() != rightAttemptedAt.IsZero() {
			return leftAttemptedAt.IsZero()
		}
		if !leftAttemptedAt.Equal(rightAttemptedAt) {
			return leftAttemptedAt.Before(rightAttemptedAt)
		}
		leftQueuedAt := available[left].Entry.QueuedAt
		rightQueuedAt := available[right].Entry.QueuedAt
		if !leftQueuedAt.Equal(rightQueuedAt) {
			return leftQueuedAt.Before(rightQueuedAt)
		}
		leftDistance := towerQueueEntryDistanceSquared(available[left].Castle, available[left].Entry)
		rightDistance := towerQueueEntryDistanceSquared(available[right].Castle, available[right].Entry)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if available[left].Castle.ID != available[right].Castle.ID {
			return available[left].Castle.ID < available[right].Castle.ID
		}
		if available[left].Entry.TargetY != available[right].Entry.TargetY {
			return available[left].Entry.TargetY < available[right].Entry.TargetY
		}
		return available[left].Entry.TargetX < available[right].Entry.TargetX
	})
	return available, activeCount, configured
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
	target, exists := snapshot.State.Map[candidate.Entry.KingdomID][fmt.Sprintf("%d:%d", candidate.Entry.TargetX, candidate.Entry.TargetY)]
	if !exists {
		return 0, fmt.Errorf("tower %d:%d is no longer in map state", candidate.Entry.TargetX, candidate.Entry.TargetY)
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
	for _, movement := range gameState.Movements {
		if !towerMovementActiveAt(movement, now) {
			continue
		}
		if movement.Direction == 0 && movement.SourceCastleID == castleID && movement.TargetTypeID == kingdomTowerMapTypeID {
			count++
		}
		if movement.Direction == 1 && movement.TargetCastleID == castleID && movement.SourceTypeID == kingdomTowerMapTypeID {
			count++
		}
	}
	return count
}

func activeTowerTargetKeys(gameState State.GameState, now time.Time) map[string]struct{} {
	locked := map[string]struct{}{}
	for _, movement := range gameState.Movements {
		if !towerMovementActiveAt(movement, now) {
			continue
		}
		switch {
		case movement.Direction == 0 && movement.TargetTypeID == kingdomTowerMapTypeID:
			locked[towerTargetKey(movement.KingdomID, movement.TargetX, movement.TargetY)] = struct{}{}
		case movement.Direction == 1 && movement.SourceTypeID == kingdomTowerMapTypeID:
			locked[towerTargetKey(movement.KingdomID, movement.SourceX, movement.SourceY)] = struct{}{}
		}
	}
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
	for _, cooldown := range gameState.TowerCooldowns {
		if cooldown.PendingCooldownRefresh {
			pending = append(pending, cooldown)
		}
	}
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
	return gameState.TowerCooldowns[key].PendingCooldownRefresh
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

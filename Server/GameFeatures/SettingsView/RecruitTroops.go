package settingsview

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"sync"
	"time"
)

const (
	recruitBaseQueuedStackCapacity = 2
	recruitDefaultStackAmount      = 100
	recruitActionDelay             = 50 * time.Millisecond
)

type recruitQueueExpectation struct {
	Capacity int
	VIPLevel int
	Known    bool
	Source   string
}

var (
	recruitTroopsCancel          context.CancelFunc
	recruitTroopsMu              sync.Mutex
	recruitTargetResolverRefresh = make(chan struct{}, 1)
)

// IsRecruitTroopsRunning returns true if the RecruitTroops goroutine is currently active
func IsRecruitTroopsRunning() bool {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()
	return recruitTroopsCancel != nil
}

// StartRecruitTroops starts the recruit troops goroutine.
func StartRecruitTroops() {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()

	// If already running, don't start another
	if recruitTroopsCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	recruitTroopsCancel = cancel

	go runRecruitTargetResolver(ctx)
	go runRecruitTroops(ctx)
}

// StopRecruitTroops stops the recruit troops goroutine if running.
func StopRecruitTroops() {
	recruitTroopsMu.Lock()
	defer recruitTroopsMu.Unlock()

	if recruitTroopsCancel != nil {
		recruitTroopsCancel()
		recruitTroopsCancel = nil
		Models.GetGameState().ClearAutoRecruitActiveUnits()
		if ResponseRegistry.SendRecruitTroopsStatusFunc != nil {
			go ResponseRegistry.SendRecruitTroopsStatusFunc(false)
		}
	}
}

// NotifyRecruitTroopsSettingsChanged refreshes the resolved active unit plan after settings or schedules change.
func NotifyRecruitTroopsSettingsChanged() {
	refreshRecruitTargetPlan(time.Now())
	select {
	case recruitTargetResolverRefresh <- struct{}{}:
	default:
	}
}

func recruitSlotOptionInt(slot stsettings.FeatureScheduleSlot, key string) (int, bool) {
	if slot.Options == nil {
		return 0, false
	}
	switch value := slot.Options[key].(type) {
	case float64:
		return int(value), true
	case float32:
		return int(value), true
	case int:
		return value, true
	case int64:
		return int(value), true
	case json.Number:
		i, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return int(i), true
	case string:
		i, err := strconv.Atoi(value)
		return i, err == nil
	default:
		return 0, false
	}
}

func recruitOccupiedQueueStacks(castleInfo *Models.PlayerCastleInfo) int {
	if castleInfo == nil || castleInfo.SlotProductionByLID == nil {
		return 0
	}
	queue := castleInfo.SlotProductionByLID[0]
	if queue == nil {
		return 0
	}
	return len(queue.Queued)
}

func recruitParsedQueueCapacity(castleInfo *Models.PlayerCastleInfo) int {
	if castleInfo == nil || castleInfo.SlotProductionByLID == nil {
		return 0
	}
	queue := castleInfo.SlotProductionByLID[0]
	if queue == nil {
		return 0
	}
	return queue.QueueCapacity
}

func recruitExpectedQueueCapacity(gs *Models.GameState) recruitQueueExpectation {
	vipLevel := 0
	if gs != nil && gs.VIP.Level > 0 {
		vipLevel = gs.VIP.Level
		if bonusSlots, ok := GameParser.VIPRecruitmentBonusSlots(vipLevel); ok {
			return recruitQueueExpectation{
				Capacity: recruitBaseQueuedStackCapacity + bonusSlots,
				VIPLevel: vipLevel,
				Known:    true,
				Source:   "vip",
			}
		}
	}
	return recruitQueueExpectation{
		Capacity: recruitBaseQueuedStackCapacity,
		VIPLevel: vipLevel,
		Known:    false,
		Source:   "base",
	}
}

func recruitEffectiveQueueCapacity(expected recruitQueueExpectation, castleInfo *Models.PlayerCastleInfo) int {
	capacity := expected.Capacity
	if capacity <= 0 {
		capacity = recruitBaseQueuedStackCapacity
	}
	parsedCapacity := recruitParsedQueueCapacity(castleInfo)
	if parsedCapacity <= 0 {
		return capacity
	}
	if !expected.Known || parsedCapacity < capacity {
		return parsedCapacity
	}
	return capacity
}

type recruitStackAmountPlan struct {
	Amount                int
	Breakdown             GameParser.BarracksRecruitStackCapacityBreakdown
	SubscriptionBonus     int
	SubscriptionTypeIDs   []int
	CalculatedStackAmount int
	ObservedTUA           int
	Source                string
}

func recruitStackAmountForUnit(gs *Models.GameState, castleInfo *Models.PlayerCastleInfo, troopID int) recruitStackAmountPlan {
	breakdown := GameParser.BarracksRecruitStackCapacityDetails(castleInfo)
	stackAmount := breakdown.TotalStackSize
	source := "barracks"
	if stackAmount <= 0 {
		stackAmount = recruitDefaultStackAmount
		source = "default"
	}
	var activeSubscriptionTypeIDs []int
	if gs != nil {
		activeSubscriptionTypeIDs = gs.ActiveSubscriptionTypeIDs()
	}
	subscriptionBonus := GameParser.ActiveSubscriptionRecruitmentSlotCapacityBonus(activeSubscriptionTypeIDs)
	if subscriptionBonus > 0 {
		stackAmount += subscriptionBonus
		source += "+subscription"
	}
	calculatedStackAmount := stackAmount
	if castleInfo == nil || castleInfo.SlotProductionByLID == nil {
		return recruitStackAmountPlan{
			Amount:                stackAmount,
			Breakdown:             breakdown,
			SubscriptionBonus:     subscriptionBonus,
			SubscriptionTypeIDs:   activeSubscriptionTypeIDs,
			CalculatedStackAmount: calculatedStackAmount,
			Source:                source,
		}
	}
	queue := castleInfo.SlotProductionByLID[0]
	if queue == nil {
		return recruitStackAmountPlan{
			Amount:                stackAmount,
			Breakdown:             breakdown,
			SubscriptionBonus:     subscriptionBonus,
			SubscriptionTypeIDs:   activeSubscriptionTypeIDs,
			CalculatedStackAmount: calculatedStackAmount,
			Source:                source,
		}
	}
	observedTUA := 0
	for _, slot := range queue.Queued {
		if slot.WID == troopID && slot.TUA > stackAmount {
			observedTUA = slot.TUA
			stackAmount = slot.TUA
			source = "observed"
		}
	}
	return recruitStackAmountPlan{
		Amount:                stackAmount,
		Breakdown:             breakdown,
		SubscriptionBonus:     subscriptionBonus,
		SubscriptionTypeIDs:   activeSubscriptionTypeIDs,
		CalculatedStackAmount: calculatedStackAmount,
		ObservedTUA:           observedTUA,
		Source:                source,
	}
}

func recruitConfiguredUnit(units map[int]int) int {
	if len(units) == 0 {
		return 0
	}
	unitIDs := make([]int, 0, len(units))
	for unitID := range units {
		if unitID > 0 {
			unitIDs = append(unitIDs, unitID)
		}
	}
	sort.Ints(unitIDs)
	if len(unitIDs) == 0 {
		return 0
	}
	return unitIDs[0]
}

func recruitUnitFromScheduleSlot(slot stsettings.FeatureScheduleSlot, active bool) int {
	if !active {
		return 0
	}
	unitID, ok := recruitSlotOptionInt(slot, "unitID")
	if !ok || unitID <= 0 {
		return 0
	}
	return unitID
}

func orderedRecruitCastleIDs(gs *Models.GameState, unitsByCastle map[int]int) []int {
	ids := make([]int, 0, len(unitsByCastle))
	seen := make(map[int]bool, len(unitsByCastle))
	appendCastle := func(c *Models.PlayerCastleInfo) {
		if c == nil {
			return
		}
		castleID := int(c.Aid)
		if castleID <= 0 || seen[castleID] {
			return
		}
		if _, ok := unitsByCastle[castleID]; !ok {
			return
		}
		ids = append(ids, castleID)
		seen[castleID] = true
	}

	if gs != nil {
		c := &gs.Castle
		appendCastle(&c.MainCastle)
		appendCastle(&c.Outpost1)
		appendCastle(&c.Outpost2)
		appendCastle(&c.Outpost3)
		appendCastle(&c.IceCastle)
		appendCastle(&c.DesertCastle)
		appendCastle(&c.DungeonCastle)
		appendCastle(&c.StormCastle)
		appendCastle(&c.Metropolis)
		appendCastle(&c.Capital)
	}

	var remaining []int
	for castleID := range unitsByCastle {
		if castleID > 0 && !seen[castleID] {
			remaining = append(remaining, castleID)
		}
	}
	sort.Ints(remaining)
	return append(ids, remaining...)
}

func recruitSleepDuration(state *stsettings.SettingsState, featureID string, now time.Time, fallback time.Duration) time.Duration {
	if state == nil || state.FeatureScheduleAllows(featureID, now) {
		return fallback
	}
	if next, ok := state.NextFeatureScheduleStart(featureID, now); ok {
		until := time.Until(next)
		if until > 0 && until < fallback {
			return until
		}
	}
	return fallback
}

func recruitPause(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		return true
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func resolveActiveRecruitUnits(state *stsettings.SettingsState, settings stsettings.RecruitTroopsConfig, now time.Time) map[int]int {
	settings = settings.Normalize()
	unitsByCastle := make(map[int]int)

	if settings.Mode == stsettings.RecruitTroopsModeGlobal {
		if !state.FeatureScheduleAllows("autoRecruit", now) {
			return unitsByCastle
		}

		unitID := recruitConfiguredUnit(settings.GlobalTargets)
		slot, active := state.ActiveFeatureScheduleSlot("autoRecruit", now)
		if scheduledUnitID := recruitUnitFromScheduleSlot(slot, active); scheduledUnitID > 0 {
			unitID = scheduledUnitID
		}
		if unitID <= 0 {
			return unitsByCastle
		}
		for castleID, enabled := range settings.EnabledCastles {
			if enabled {
				unitsByCastle[castleID] = unitID
			}
		}
		return unitsByCastle
	}

	for castleID, enabled := range settings.EnabledCastles {
		if !enabled {
			continue
		}
		scheduleID := fmt.Sprintf("autoRecruit:%d", castleID)
		if !state.FeatureScheduleAllows(scheduleID, now) {
			continue
		}
		unitID := recruitConfiguredUnit(settings.Targets[castleID])
		slot, active := state.ActiveFeatureScheduleSlot(scheduleID, now)
		if scheduledUnitID := recruitUnitFromScheduleSlot(slot, active); scheduledUnitID > 0 {
			unitID = scheduledUnitID
		}
		if unitID > 0 {
			unitsByCastle[castleID] = unitID
		}
	}
	return unitsByCastle
}

func refreshRecruitTargetPlan(now time.Time) map[int]int {
	state := Models.GetSettingsState()
	settings := state.RecruitTroopsList.Normalize()
	state.RecruitTroopsList = settings
	unitsByCastle := resolveActiveRecruitUnits(state, settings, now)
	Models.GetGameState().SetAutoRecruitActiveUnits(unitsByCastle, now.UnixMilli())
	return unitsByCastle
}

func runRecruitTargetResolver(ctx context.Context) {
	refreshRecruitTargetPlan(time.Now())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshRecruitTargetPlan(time.Now())
		case <-recruitTargetResolverRefresh:
			refreshRecruitTargetPlan(time.Now())
		}
	}
}

// runRecruitTroops is the main loop for Auto-Recruiting
func runRecruitTroops(ctx context.Context) {
	Logging.AutoRecruitLog("loop_start", "")
	defer Logging.AutoRecruitLog("loop_stop", "")

	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		state := Models.GetSettingsState()
		settings := state.RecruitTroopsList.Normalize()
		state.RecruitTroopsList = settings
		sleepDuration := time.Duration(settings.CheckIntervalSec) * time.Second

		// If disconnected, handle reload
		if !ResponseRegistry.LoginStatus {
			Logging.AutoRecruitLog("login", "disconnected; waiting for login")
		LoginWaitLoop:
			for {
				select {
				case <-ctx.Done():
					return
				case <-time.After(5 * time.Second):
					if ResponseRegistry.LoginStatus {
						break LoginWaitLoop
					}
				}
			}
		}

		Logging.AutoRecruitLog("cycle", "start")

		now := time.Now()
		expectedQueue := recruitExpectedQueueCapacity(gs)
		Logging.AutoRecruitLogf("capacity", "expected=%d vipLevel=%d source=%s", expectedQueue.Capacity, expectedQueue.VIPLevel, expectedQueue.Source)
		refreshRecruitTargetPlan(now)
		unitsByCastle := gs.AutoRecruitActiveUnits()
		if len(unitsByCastle) == 0 {
			Logging.AutoRecruitLog("plan", "no active units configured for enabled castles")
		}
		if len(unitsByCastle) == 0 && settings.Mode == stsettings.RecruitTroopsModeGlobal && !state.FeatureScheduleAllows("autoRecruit", now) {
			Logging.AutoRecruitLog("schedule", "shared schedule inactive")
			sleepDuration = recruitSleepDuration(state, "autoRecruit", now, sleepDuration)
		} else if len(unitsByCastle) > 0 {
			resourceReservations := make(map[string]float64)
			for _, castleID := range orderedRecruitCastleIDs(gs, unitsByCastle) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				unitID := unitsByCastle[castleID]
				if unitID <= 0 {
					continue
				}
				castleInfo := gs.GetCastleByID(castleID)
				if castleInfo == nil {
					Logging.AutoRecruitLogf("castle_skip", "castle=%d missing from game state", castleID)
					continue
				}

				// Find location details for the `FetchCastleTroops` call
				var kingdomID, x, y int
				foundLoc := false
				for _, loc := range gs.Alliance.PlayerCastleLocations {
					if loc.CastleID == castleID {
						kingdomID = loc.KingdomID
						x = loc.X
						y = loc.Y
						foundLoc = true
						break
					}
				}

				if !foundLoc {
					Logging.AutoRecruitLogf("castle_skip", "castle=%d missing map location", castleID)
					continue
				}

				// Focus castle; inbound **jaa** updates troops/buildings/focus in MessageRouter.
				Logging.AutoRecruitLogf("focus", "castle=%d kid=%d x=%d y=%d", castleID, kingdomID, x, y)
				if GameParser.FetchCastleTroops(kingdomID, castleID, x, y) == nil {
					Logging.AutoRecruitLogf("castle_skip", "castle=%d focus failed; queue state not trusted", castleID)
					continue
				}
				if !recruitPause(ctx, recruitActionDelay) {
					return
				}

				occupiedQueueStacks := recruitOccupiedQueueStacks(castleInfo)
				queueCapacity := recruitEffectiveQueueCapacity(expectedQueue, castleInfo)
				if queueCapacity != expectedQueue.Capacity {
					Logging.AutoRecruitLogf("capacity", "castle=%d parsed=%d expected=%d", castleID, queueCapacity, expectedQueue.Capacity)
				}
				if occupiedQueueStacks >= queueCapacity {
					Logging.AutoRecruitLogf("queue_full", "castle=%d occupied=%d capacity=%d", castleID, occupiedQueueStacks, queueCapacity)
					continue
				}

				stackPlan := recruitStackAmountForUnit(gs, castleInfo, unitID)
				Logging.AutoRecruitLogf(
					"stack_capacity",
					"castle=%d unit=%d amount=%d source=%s barracksOID=%d wid=%d level=%d base=%d ciBonus=%d subBonus=%d calcTotal=%d total=%d boosts=%d activeSubs=%v observedTUA=%d",
					castleID,
					unitID,
					stackPlan.Amount,
					stackPlan.Source,
					stackPlan.Breakdown.BuildingOID,
					stackPlan.Breakdown.BuildingWID,
					stackPlan.Breakdown.BuildingLevel,
					stackPlan.Breakdown.BaseStackSize,
					stackPlan.Breakdown.ConstructionBonus,
					stackPlan.SubscriptionBonus,
					stackPlan.CalculatedStackAmount,
					stackPlan.Breakdown.TotalStackSize,
					len(stackPlan.Breakdown.Boosts),
					stackPlan.SubscriptionTypeIDs,
					stackPlan.ObservedTUA,
				)
				costReductionPercent := GameParser.BarracksRecruitCostReductionPercent(
					castleInfo,
					stackPlan.Breakdown.BuildingOID,
					stackPlan.Breakdown.BuildingWID,
				)
				for occupiedQueueStacks < queueCapacity {
					select {
					case <-ctx.Done():
						return
					default:
					}

					costCheck := GameParser.RecruitUnitResourceCostCheck(gs, castleInfo, unitID, stackPlan.Amount, costReductionPercent, resourceReservations)
					if costCheck.UnknownUnitCost {
						Logging.AutoRecruitLogf("resource_unknown", "castle=%d unit=%d amount=%d cost data unavailable; attempting queue", castleID, unitID, stackPlan.Amount)
					} else if !costCheck.CanAfford() {
						Logging.AutoRecruitLogf(
							"resource_skip",
							"castle=%d unit=%d amount=%d costReduction=%d missing=%s",
							castleID,
							unitID,
							stackPlan.Amount,
							costReductionPercent,
							costCheck.MissingSummary(),
						)
						break
					}

					Logging.AutoRecruitLogf("queue", "castle=%d unit=%d amount=%d occupied=%d capacity=%d", castleID, unitID, stackPlan.Amount, occupiedQueueStacks, queueCapacity)
					// SK must match live session (captured from browser bup); TODO: read from game state when available.
					const recruitSessionKey = 73
					payload := GameCommands.BUPPayload(0, unitID, stackPlan.Amount, -1, 0, recruitSessionKey, 0, castleID)
					Logging.AppendAutoRecruitSendPayload(payload)
					GameCommands.QueueOutgoingPayload(payload)
					GameParser.ReserveRecruitResourceCosts(resourceReservations, costCheck)
					occupiedQueueStacks++
					if !recruitPause(ctx, recruitActionDelay) {
						return
					}
				}
			}
		}

		Logging.AutoRecruitLogf("sleep", "next cycle in %s", sleepDuration)
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
			// Waking up for next cycle
		}
	}
}

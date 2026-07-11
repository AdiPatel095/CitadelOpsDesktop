package settingsview

import (
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
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
	autoToolQueueLID                = 1
	autoToolBaseQueuedStackCapacity = 2
	autoToolActionDelay             = 50 * time.Millisecond
)

var (
	autoToolCancel                context.CancelFunc
	autoToolMu                    sync.Mutex
	autoToolTargetResolverRefresh = make(chan struct{}, 1)
)

// IsAutoToolRunning returns true if the Auto Tool goroutine is currently active.
func IsAutoToolRunning() bool {
	autoToolMu.Lock()
	defer autoToolMu.Unlock()
	return autoToolCancel != nil
}

// StartAutoTool starts the Auto Tool goroutine.
func StartAutoTool() {
	autoToolMu.Lock()
	defer autoToolMu.Unlock()

	if autoToolCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	autoToolCancel = cancel

	go runAutoToolTargetResolver(ctx)
	go runAutoTool(ctx)
}

// StopAutoTool stops the Auto Tool goroutine if running.
func StopAutoTool() {
	autoToolMu.Lock()
	defer autoToolMu.Unlock()

	if autoToolCancel != nil {
		autoToolCancel()
		autoToolCancel = nil
		Models.GetGameState().ClearAutoToolActiveTools()
		if ResponseRegistry.SendAutoToolStatusFunc != nil {
			go ResponseRegistry.SendAutoToolStatusFunc(false)
		}
	}
}

// NotifyAutoToolSettingsChanged refreshes the resolved active tool plan after settings or schedules change.
func NotifyAutoToolSettingsChanged() {
	refreshAutoToolTargetPlan(time.Now())
	select {
	case autoToolTargetResolverRefresh <- struct{}{}:
	default:
	}
}

func autoToolSlotOptionInt(slot stsettings.FeatureScheduleSlot, key string) (int, bool) {
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

func autoToolQueue(castleInfo *Models.PlayerCastleInfo) *castle.BarracksProductionQueue {
	if castleInfo == nil || castleInfo.SlotProductionByLID == nil {
		return nil
	}
	return castleInfo.SlotProductionByLID[autoToolQueueLID]
}

func autoToolOccupiedQueueStacks(castleInfo *Models.PlayerCastleInfo) int {
	queue := autoToolQueue(castleInfo)
	if queue == nil {
		return 0
	}
	return len(queue.Queued)
}

func autoToolQueueCapacity(castleInfo *Models.PlayerCastleInfo) int {
	queue := autoToolQueue(castleInfo)
	if queue != nil && queue.QueueCapacity > 0 {
		return queue.QueueCapacity
	}
	return autoToolBaseQueuedStackCapacity
}

type autoToolStackAmountPlan struct {
	Amount    int
	Breakdown GameParser.ToolProductionStackCapacityBreakdown
	Source    string
}

func autoToolStackAmountForCastle(castleInfo *Models.PlayerCastleInfo, toolID int) autoToolStackAmountPlan {
	breakdown := GameParser.ToolProductionStackCapacityDetailsForTool(castleInfo, toolID)
	if breakdown.TotalStackSize <= 0 {
		return autoToolStackAmountPlan{Breakdown: breakdown}
	}
	return autoToolStackAmountPlan{
		Amount:    breakdown.TotalStackSize,
		Breakdown: breakdown,
		Source:    breakdown.Source,
	}
}

func autoToolConfiguredTool(tools map[int]int) int {
	if len(tools) == 0 {
		return 0
	}
	toolIDs := make([]int, 0, len(tools))
	for toolID := range tools {
		if toolID > 0 {
			toolIDs = append(toolIDs, toolID)
		}
	}
	sort.Ints(toolIDs)
	if len(toolIDs) == 0 {
		return 0
	}
	return toolIDs[0]
}

func autoToolFromScheduleSlot(slot stsettings.FeatureScheduleSlot, active bool) int {
	if !active {
		return 0
	}
	toolID, ok := autoToolSlotOptionInt(slot, "toolID")
	if !ok || toolID <= 0 {
		return 0
	}
	return toolID
}

func orderedAutoToolCastleIDs(gs *Models.GameState, toolsByCastle map[int]int) []int {
	ids := make([]int, 0, len(toolsByCastle))
	seen := make(map[int]bool, len(toolsByCastle))
	appendCastle := func(c *Models.PlayerCastleInfo) {
		if c == nil {
			return
		}
		castleID := int(c.Aid)
		if castleID <= 0 || seen[castleID] {
			return
		}
		if _, ok := toolsByCastle[castleID]; !ok {
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
	for castleID := range toolsByCastle {
		if castleID > 0 && !seen[castleID] {
			remaining = append(remaining, castleID)
		}
	}
	sort.Ints(remaining)
	return append(ids, remaining...)
}

func autoToolSleepDuration(state *stsettings.SettingsState, featureID string, now time.Time, fallback time.Duration) time.Duration {
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

func autoToolPause(ctx context.Context, duration time.Duration) bool {
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

func resolveActiveAutoTools(state *stsettings.SettingsState, settings stsettings.AutoToolConfig, now time.Time) map[int]int {
	settings = settings.Normalize()
	toolsByCastle := make(map[int]int)

	if settings.Mode == stsettings.AutoToolModeGlobal {
		if !state.FeatureScheduleAllows("autoTool", now) {
			return toolsByCastle
		}

		toolID := autoToolConfiguredTool(settings.GlobalTargets)
		slot, active := state.ActiveFeatureScheduleSlot("autoTool", now)
		if scheduledToolID := autoToolFromScheduleSlot(slot, active); scheduledToolID > 0 {
			toolID = scheduledToolID
		}
		if toolID <= 0 {
			return toolsByCastle
		}
		for castleID, enabled := range settings.EnabledCastles {
			if enabled {
				toolsByCastle[castleID] = toolID
			}
		}
		return toolsByCastle
	}

	for castleID, enabled := range settings.EnabledCastles {
		if !enabled {
			continue
		}
		scheduleID := fmt.Sprintf("autoTool:%d", castleID)
		if !state.FeatureScheduleAllows(scheduleID, now) {
			continue
		}
		toolID := autoToolConfiguredTool(settings.Targets[castleID])
		slot, active := state.ActiveFeatureScheduleSlot(scheduleID, now)
		if scheduledToolID := autoToolFromScheduleSlot(slot, active); scheduledToolID > 0 {
			toolID = scheduledToolID
		}
		if toolID > 0 {
			toolsByCastle[castleID] = toolID
		}
	}
	return toolsByCastle
}

func refreshAutoToolTargetPlan(now time.Time) map[int]int {
	state := Models.GetSettingsState()
	settings := state.AutoToolList.Normalize()
	state.AutoToolList = settings
	toolsByCastle := resolveActiveAutoTools(state, settings, now)
	Models.GetGameState().SetAutoToolActiveTools(toolsByCastle, now.UnixMilli())
	return toolsByCastle
}

func runAutoToolTargetResolver(ctx context.Context) {
	refreshAutoToolTargetPlan(time.Now())
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshAutoToolTargetPlan(time.Now())
		case <-autoToolTargetResolverRefresh:
			refreshAutoToolTargetPlan(time.Now())
		}
	}
}

func runAutoTool(ctx context.Context) {
	Logging.AutoToolLog("loop_start", "")
	defer Logging.AutoToolLog("loop_stop", "")

	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		state := Models.GetSettingsState()
		settings := state.AutoToolList.Normalize()
		state.AutoToolList = settings
		sleepDuration := time.Duration(settings.CheckIntervalSec) * time.Second

		if !ResponseRegistry.IsGameWebSocketReady() {
			Logging.AutoToolLogf("login", "game websocket not ready; skipping cycle, next cycle in %s", sleepDuration)
			select {
			case <-ctx.Done():
				return
			case <-time.After(sleepDuration):
			}
			continue
		}

		Logging.AutoToolLog("cycle", "start")

		now := time.Now()
		refreshAutoToolTargetPlan(now)
		toolsByCastle := gs.AutoToolActiveTools()
		if len(toolsByCastle) == 0 {
			Logging.AutoToolLog("plan", "no active tools configured for enabled castles")
		}
		if len(toolsByCastle) == 0 && settings.Mode == stsettings.AutoToolModeGlobal && !state.FeatureScheduleAllows("autoTool", now) {
			Logging.AutoToolLog("schedule", "shared schedule inactive")
			sleepDuration = autoToolSleepDuration(state, "autoTool", now, sleepDuration)
		} else if len(toolsByCastle) > 0 {
			resourceReservations := make(map[string]float64)
			for _, castleID := range orderedAutoToolCastleIDs(gs, toolsByCastle) {
				select {
				case <-ctx.Done():
					return
				default:
				}

				toolID := toolsByCastle[castleID]
				if toolID <= 0 {
					continue
				}
				castleInfo := gs.GetCastleByID(castleID)
				if castleInfo == nil {
					Logging.AutoToolLogf("castle_skip", "castle=%d missing from game state", castleID)
					continue
				}

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
					Logging.AutoToolLogf("castle_skip", "castle=%d missing map location", castleID)
					continue
				}

				lease, ok := GameFocus.Acquire(ctx, GameFocus.Request{
					Owner:   GameFocus.OwnerAutoTool,
					Reason:  fmt.Sprintf("castle=%d", castleID),
					MaxHold: 30 * time.Second,
				})
				if !ok {
					return
				}
				leaseRevoked := false
				func() {
					defer lease.Release()

					Logging.AutoToolLogf("focus", "castle=%d kid=%d x=%d y=%d", castleID, kingdomID, x, y)
					if GameParser.FetchCastleTroopsWithLease(lease, kingdomID, castleID, x, y) == nil {
						if !lease.Active() {
							leaseRevoked = true
							Logging.AutoToolLogf("lease_revoked", "castle=%d", castleID)
							return
						}
						Logging.AutoToolLogf("castle_skip", "castle=%d focus failed; queue state not trusted", castleID)
						return
					}
					if !autoToolPause(ctx, autoToolActionDelay) {
						return
					}
					if !lease.Active() {
						leaseRevoked = true
						Logging.AutoToolLogf("lease_revoked", "castle=%d", castleID)
						return
					}
					if refreshed := gs.GetCastleByID(castleID); refreshed != nil {
						castleInfo = refreshed
					}

					occupiedQueueStacks := autoToolOccupiedQueueStacks(castleInfo)
					queueCapacity := autoToolQueueCapacity(castleInfo)
					if occupiedQueueStacks >= queueCapacity {
						Logging.AutoToolLogf("queue_full", "castle=%d occupied=%d capacity=%d", castleID, occupiedQueueStacks, queueCapacity)
						return
					}

					stackPlan := autoToolStackAmountForCastle(castleInfo, toolID)
					if stackPlan.Amount <= 0 {
						Logging.AutoToolLogf("castle_skip", "castle=%d missing workshop stack capacity", castleID)
						return
					}
					Logging.AutoToolLogf(
						"stack_capacity",
						"castle=%d tool=%d amount=%d source=%s workshopOID=%d wid=%d level=%d",
						castleID,
						toolID,
						stackPlan.Amount,
						stackPlan.Source,
						stackPlan.Breakdown.BuildingOID,
						stackPlan.Breakdown.BuildingWID,
						stackPlan.Breakdown.BuildingLevel,
					)
					for occupiedQueueStacks < queueCapacity {
						select {
						case <-ctx.Done():
							return
						default:
						}

						costCheck := GameParser.ToolProductionResourceCostCheck(gs, castleInfo, toolID, stackPlan.Amount, resourceReservations)
						if !costCheck.CanAfford() {
							Logging.AutoToolLogf(
								"resource_skip",
								"castle=%d tool=%d amount=%d missing=%s",
								castleID,
								toolID,
								stackPlan.Amount,
								costCheck.MissingSummary(),
							)
							break
						}

						if !lease.Active() {
							leaseRevoked = true
							Logging.AutoToolLogf("lease_revoked", "castle=%d send skipped", castleID)
							return
						}
						Logging.AutoToolLogf("queue", "castle=%d tool=%d amount=%d occupied=%d capacity=%d", castleID, toolID, stackPlan.Amount, occupiedQueueStacks, queueCapacity)
						// SK must match live session (captured from browser bup); TODO: read from game state when available.
						const autoToolSessionKey = 73
						payload := GameCommands.BUPPayload(autoToolQueueLID, toolID, stackPlan.Amount, -1, 0, autoToolSessionKey, 0, castleID)
						Logging.AppendAutoToolSendPayload(payload)
						GameCommands.QueueOutgoingPayload(payload)
						GameParser.ReserveToolResourceCosts(resourceReservations, costCheck)
						occupiedQueueStacks++
						if !autoToolPause(ctx, autoToolActionDelay) {
							return
						}
					}
				}()
				if leaseRevoked {
					break
				}
			}
		}

		Logging.AutoToolLogf("sleep", "next cycle in %s", sleepDuration)
		select {
		case <-ctx.Done():
			return
		case <-time.After(sleepDuration):
		}
	}
}

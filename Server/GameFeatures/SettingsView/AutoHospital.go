package settingsview

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	autoHospitalQueueLID         = 2
	autoHospitalBaseStackAmount  = 10
	autoHospitalMaxStackAmount   = 15
	autoHospitalMaxQueueCapacity = 5
	autoHospitalActionDelay      = 50 * time.Millisecond
	autoHospitalFeatureID        = "autoHospital"
)

type autoHospitalWoundedUnit struct {
	UnitID int
	Amount int
}

type autoHospitalStackAmountPlan struct {
	Amount              int
	SubscriptionBonus   int
	SubscriptionTypeIDs []int
	Source              string
}

var (
	autoHospitalCancel context.CancelFunc
	autoHospitalMu     sync.Mutex
	autoHospitalWake   = make(chan struct{}, 1)
)

// IsAutoHospitalRunning returns true if the Auto Hospital goroutine is currently active.
func IsAutoHospitalRunning() bool {
	autoHospitalMu.Lock()
	defer autoHospitalMu.Unlock()
	return autoHospitalCancel != nil
}

// StartAutoHospital starts the Auto Hospital goroutine.
func StartAutoHospital() {
	autoHospitalMu.Lock()
	defer autoHospitalMu.Unlock()

	if autoHospitalCancel != nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	autoHospitalCancel = cancel

	go runAutoHospital(ctx)
}

// StopAutoHospital stops the Auto Hospital goroutine if running.
func StopAutoHospital() {
	autoHospitalMu.Lock()
	defer autoHospitalMu.Unlock()

	if autoHospitalCancel != nil {
		autoHospitalCancel()
		Automation.CancelOwner(Automation.OwnerAutoHospital)
		autoHospitalCancel = nil
		if ResponseRegistry.SendAutoHospitalStatusFunc != nil {
			go ResponseRegistry.SendAutoHospitalStatusFunc(false)
		}
	}
}

// NotifyAutoHospitalSettingsChanged wakes the loop after interval or schedule updates.
func NotifyAutoHospitalSettingsChanged() {
	select {
	case autoHospitalWake <- struct{}{}:
	default:
	}
}

func autoHospitalQueue(castleInfo *Models.PlayerCastleInfo) *castle.BarracksProductionQueue {
	if castleInfo == nil || castleInfo.SlotProductionByLID == nil {
		return nil
	}
	return castleInfo.SlotProductionByLID[autoHospitalQueueLID]
}

func autoHospitalOccupiedQueueStacks(castleInfo *Models.PlayerCastleInfo) int {
	queue := autoHospitalQueue(castleInfo)
	if queue == nil {
		return 0
	}
	occupied := len(queue.Queued)
	if queue.Active != nil {
		occupied++
	}
	return occupied
}

func autoHospitalParsedQueueCapacity(castleInfo *Models.PlayerCastleInfo) int {
	queue := autoHospitalQueue(castleInfo)
	if queue == nil {
		return 0
	}
	return queue.QueueCapacity
}

func autoHospitalQueueCapacity(castleInfo *Models.PlayerCastleInfo) (int, GameParser.HospitalQueueCapacityBreakdown, string) {
	breakdown := GameParser.HospitalQueueCapacityDetails(castleInfo)
	capacity := breakdown.HospitalSlots
	source := breakdown.Source
	if capacity <= 0 {
		capacity = autoHospitalParsedQueueCapacity(castleInfo)
		source = "parsed"
	}
	if capacity > autoHospitalMaxQueueCapacity {
		capacity = autoHospitalMaxQueueCapacity
	}
	if capacity < 0 {
		capacity = 0
	}
	return capacity, breakdown, source
}

func autoHospitalStackAmount(gs *Models.GameState) autoHospitalStackAmountPlan {
	var activeSubscriptionTypeIDs []int
	if gs != nil {
		activeSubscriptionTypeIDs = gs.ActiveSubscriptionTypeIDs()
	}
	bonus := GameParser.ActiveSubscriptionHospitalSlotCapacityBonus(activeSubscriptionTypeIDs)
	amount := autoHospitalBaseStackAmount + bonus
	source := "base"
	if bonus > 0 {
		source = "base+subscription"
	}
	if amount > autoHospitalMaxStackAmount {
		amount = autoHospitalMaxStackAmount
	}
	return autoHospitalStackAmountPlan{
		Amount:              amount,
		SubscriptionBonus:   bonus,
		SubscriptionTypeIDs: activeSubscriptionTypeIDs,
		Source:              source,
	}
}

func autoHospitalWoundedUnits(castleInfo *Models.PlayerCastleInfo) []autoHospitalWoundedUnit {
	if castleInfo == nil || len(castleInfo.Troops.TroopsHI) == 0 {
		return nil
	}
	units := make([]autoHospitalWoundedUnit, 0, len(castleInfo.Troops.TroopsHI))
	for unitID, amount := range castleInfo.Troops.TroopsHI {
		if unitID <= 0 || amount <= 0 {
			continue
		}
		units = append(units, autoHospitalWoundedUnit{UnitID: unitID, Amount: amount})
	}
	sort.Slice(units, func(i, j int) bool {
		if units[i].Amount != units[j].Amount {
			return units[i].Amount > units[j].Amount
		}
		return units[i].UnitID < units[j].UnitID
	})
	return units
}

func autoHospitalDiscardRubyHealingUnits(ctx context.Context, lease *GameFocus.Lease, castleID int, wounded []autoHospitalWoundedUnit) ([]autoHospitalWoundedUnit, bool) {
	if len(wounded) == 0 {
		return wounded, true
	}
	healable := make([]autoHospitalWoundedUnit, 0, len(wounded))
	for _, unit := range wounded {
		cost := GameParser.HospitalHealingCostDetails(unit.UnitID)
		if cost.Known && cost.RubyCost > 0 {
			Logging.AutoHospitalLogf(
				"discard_ruby",
				"castle=%d unit=%d amount=%d rubyCost=%.0f coinCost=%.0f",
				castleID,
				unit.UnitID,
				unit.Amount,
				cost.RubyCost,
				cost.CoinCost,
			)
			payload := GameCommands.HDUPayload(unit.UnitID, unit.Amount)
			if !lease.Active() {
				Logging.AutoHospitalLogf("lease_revoked", "castle=%d discard skipped", castleID)
				return nil, false
			}
			Logging.AppendAutoHospitalSendPayload(payload)
			GameCommands.QueueFeaturePayload(Automation.OwnerAutoHospital, payload, lease)
			if !autoHospitalPause(ctx, autoHospitalActionDelay) {
				return nil, false
			}
			continue
		}
		if !cost.Known {
			Logging.AutoHospitalLogf("cost_unknown", "castle=%d unit=%d amount=%d; treating as healable", castleID, unit.UnitID, unit.Amount)
		}
		healable = append(healable, unit)
	}
	return healable, true
}

func orderedAutoHospitalCastleIDs(gs *Models.GameState) []int {
	if gs == nil {
		return nil
	}
	ids := []int{}
	seen := map[int]bool{}
	appendCastle := func(c *Models.PlayerCastleInfo) {
		if c == nil {
			return
		}
		castleID := int(c.Aid)
		if castleID <= 0 || seen[castleID] {
			return
		}
		ids = append(ids, castleID)
		seen[castleID] = true
	}

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

	var remaining []int
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID > 0 && !seen[loc.CastleID] {
			remaining = append(remaining, loc.CastleID)
			seen[loc.CastleID] = true
		}
	}
	sort.Ints(remaining)
	return append(ids, remaining...)
}

func autoHospitalCastleFocusArgs(gs *Models.GameState, castleInfo *Models.PlayerCastleInfo) (kingdomID, x, y int, ok bool) {
	if gs == nil || castleInfo == nil {
		return 0, 0, 0, false
	}
	castleID := int(castleInfo.Aid)
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleID {
			return loc.KingdomID, loc.X, loc.Y, true
		}
	}
	if castleInfo.MapX != 0 || castleInfo.MapY != 0 {
		return castleInfo.MapKingdomID, castleInfo.MapX, castleInfo.MapY, true
	}
	if castleInfo.Troops.X != 0 || castleInfo.Troops.Y != 0 {
		return castleInfo.Troops.KingdomID, castleInfo.Troops.X, castleInfo.Troops.Y, true
	}
	return 0, 0, 0, false
}

func autoHospitalPause(ctx context.Context, duration time.Duration) bool {
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

func autoHospitalSleepDuration(state *stsettings.SettingsState, now time.Time, fallback time.Duration) time.Duration {
	if state == nil || state.FeatureScheduleAllows(autoHospitalFeatureID, now) {
		return fallback
	}
	if next, ok := state.NextFeatureScheduleStart(autoHospitalFeatureID, now); ok {
		until := time.Until(next)
		if until > 0 && until < fallback {
			return until
		}
	}
	return fallback
}

func runAutoHospital(ctx context.Context) {
	Logging.AutoHospitalLog("loop_start", "")
	defer Logging.AutoHospitalLog("loop_stop", "")

	gs := Models.GetGameState()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		state := Models.GetSettingsState()
		settings := state.AutoHospital.Normalize()
		state.AutoHospital = settings
		sleepDuration := time.Duration(settings.CheckIntervalSec) * time.Second

		if !ResponseRegistry.IsGameWebSocketReady() {
			Logging.AutoHospitalLogf("login", "game websocket not ready; skipping cycle, next cycle in %s", sleepDuration)
			select {
			case <-ctx.Done():
				return
			case <-autoHospitalWake:
			case <-time.After(sleepDuration):
			}
			continue
		}

		now := time.Now()
		if !state.FeatureScheduleAllows(autoHospitalFeatureID, now) {
			Logging.AutoHospitalLog("schedule", "shared schedule inactive")
			sleepDuration = autoHospitalSleepDuration(state, now, sleepDuration)
			Logging.AutoHospitalLogf("sleep", "next cycle in %s", sleepDuration)
			select {
			case <-ctx.Done():
				return
			case <-autoHospitalWake:
			case <-time.After(sleepDuration):
			}
			continue
		}

		Logging.AutoHospitalLog("cycle", "start")
		stackPlan := autoHospitalStackAmount(gs)
		Logging.AutoHospitalLogf("stack_capacity", "amount=%d source=%s subBonus=%d activeSubs=%v",
			stackPlan.Amount, stackPlan.Source, stackPlan.SubscriptionBonus, stackPlan.SubscriptionTypeIDs)

		for _, castleID := range orderedAutoHospitalCastleIDs(gs) {
			select {
			case <-ctx.Done():
				return
			default:
			}

			castleInfo := gs.GetCastleByID(castleID)
			if castleInfo == nil {
				Logging.AutoHospitalLogf("castle_skip", "castle=%d missing from game state", castleID)
				continue
			}
			kingdomID, x, y, foundLoc := autoHospitalCastleFocusArgs(gs, castleInfo)
			if !foundLoc {
				Logging.AutoHospitalLogf("castle_skip", "castle=%d missing map location", castleID)
				continue
			}

			lease, ok := GameFocus.Acquire(ctx, GameFocus.Request{
				Owner:   GameFocus.OwnerAutoHospital,
				Reason:  fmt.Sprintf("castle=%d", castleID),
				MaxHold: 30 * time.Second,
				Claims: []GameFocus.Claim{
					GameFocus.CastleClaim(castleID, "hospital-queue"),
					GameFocus.ExclusiveClaim(Automation.ClaimAccountResources),
				},
			})
			if !ok {
				return
			}
			leaseRevoked := false
			func() {
				defer lease.Release()

				Logging.AutoHospitalLogf("focus", "castle=%d kid=%d x=%d y=%d", castleID, kingdomID, x, y)
				if GameParser.FetchCastleTroopsWithLease(lease, kingdomID, castleID, x, y) == nil {
					if !lease.Active() {
						leaseRevoked = true
						Logging.AutoHospitalLogf("lease_revoked", "castle=%d", castleID)
						return
					}
					Logging.AutoHospitalLogf("castle_skip", "castle=%d focus failed; queue state not trusted", castleID)
					return
				}
				if !autoHospitalPause(ctx, autoHospitalActionDelay) {
					return
				}
				if !lease.Active() {
					leaseRevoked = true
					Logging.AutoHospitalLogf("lease_revoked", "castle=%d", castleID)
					return
				}
				if refreshed := gs.GetCastleByID(castleID); refreshed != nil {
					castleInfo = refreshed
				}

				queueCapacity, capacityBreakdown, capacitySource := autoHospitalQueueCapacity(castleInfo)
				if queueCapacity <= 0 {
					Logging.AutoHospitalLogf("castle_skip", "castle=%d missing hospital queue capacity", castleID)
					return
				}
				occupiedQueueStacks := autoHospitalOccupiedQueueStacks(castleInfo)
				if occupiedQueueStacks >= queueCapacity {
					Logging.AutoHospitalLogf("queue_full", "castle=%d occupied=%d capacity=%d", castleID, occupiedQueueStacks, queueCapacity)
					return
				}

				wounded := autoHospitalWoundedUnits(castleInfo)
				if len(wounded) == 0 {
					Logging.AutoHospitalLogf("castle_skip", "castle=%d no wounded units in HI", castleID)
					return
				}
				filteredWounded, ok := autoHospitalDiscardRubyHealingUnits(ctx, lease, castleID, wounded)
				if !ok {
					leaseRevoked = !lease.Active()
					return
				}
				wounded = filteredWounded
				if len(wounded) == 0 {
					Logging.AutoHospitalLogf("castle_skip", "castle=%d no coin-healable wounded units in HI", castleID)
					return
				}
				Logging.AutoHospitalLogf(
					"queue_capacity",
					"castle=%d occupied=%d capacity=%d source=%s hospitalOID=%d wid=%d level=%d",
					castleID,
					occupiedQueueStacks,
					queueCapacity,
					capacitySource,
					capacityBreakdown.BuildingOID,
					capacityBreakdown.BuildingWID,
					capacityBreakdown.BuildingLevel,
				)

				for occupiedQueueStacks < queueCapacity && len(wounded) > 0 {
					select {
					case <-ctx.Done():
						return
					default:
					}

					unit := wounded[0]
					amount := stackPlan.Amount
					if unit.Amount < amount {
						amount = unit.Amount
					}
					if amount <= 0 {
						wounded = wounded[1:]
						continue
					}

					payload := GameCommands.HRUPayload(unit.UnitID, amount)
					if !lease.Active() {
						leaseRevoked = true
						Logging.AutoHospitalLogf("lease_revoked", "castle=%d heal skipped", castleID)
						return
					}
					Logging.AutoHospitalLogf("queue", "castle=%d unit=%d amount=%d available=%d occupied=%d capacity=%d",
						castleID, unit.UnitID, amount, unit.Amount, occupiedQueueStacks, queueCapacity)
					Logging.AppendAutoHospitalSendPayload(payload)
					GameCommands.QueueFeaturePayload(Automation.OwnerAutoHospital, payload, lease)

					wounded[0].Amount -= amount
					if wounded[0].Amount <= 0 {
						wounded = wounded[1:]
					}
					occupiedQueueStacks++
					if !autoHospitalPause(ctx, autoHospitalActionDelay) {
						return
					}
				}
			}()
			if leaseRevoked {
				break
			}
		}

		Logging.AutoHospitalLogf("sleep", "next cycle in %s", sleepDuration)
		select {
		case <-ctx.Done():
			return
		case <-autoHospitalWake:
		case <-time.After(sleepDuration):
		}
	}
}

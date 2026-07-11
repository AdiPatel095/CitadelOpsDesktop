package featureview

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameFocus"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	autotcisched "CitadelDesktop/Server/Models/Autotcisched"
	"CitadelDesktop/Server/Models/Castle"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/ResponseRegistry"
	"CitadelDesktop/Server/UTCTime"
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	autoTCIUpgradeTimerMaxSec      = 5 * 60
	autoTCIUBCRareBooster          = 2000
	autoTCIUBCEpicBooster          = 2001
	autoTCIUBCLegendaryBooster     = 2002
	autoTCIRebuyLeadSec            = 120
	autoTCIJAAFocusWait            = 8 * time.Second
	upgradeDedupWindow             = 20 * time.Second
	autoTCIEquipDedupWindow        = 25 * time.Second
	maxAutoTCIEquipsPerRound       = 20
	autoTCIBuyRebuyGoroutineDedup  = 40 * time.Second
	autoTCIBuyMissingGroupDedup    = 3 * time.Minute
	autoTCIBuyPostSbpPause         = 2 * time.Second
	autoTCIPostPurchaseSettlePause = 120 * time.Millisecond
	autoTCIPostPurchaseGIIWait     = 6 * time.Second
	autoTCIMinSleep                = 10 * time.Second
	autoTCISessionRetry            = 30 * time.Second
	autoTCIIdleRetry               = 30 * time.Minute
	autoTCIPendingRetry            = 15 * time.Minute
)

var (
	autoTCICancel       context.CancelFunc
	autoTCIMu           sync.Mutex
	autoTCINextWakeUpMs int64

	upgradeLastSent  sync.Map // dedup key -> time.Time
	equipLastSent    sync.Map
	buyGoroutineLast sync.Map
)

func acquireAutoTCILease(ctx context.Context, reason string, maxHold time.Duration, claims ...GameFocus.Claim) (*GameFocus.Lease, bool) {
	claims = append([]GameFocus.Claim{GameFocus.ExclusiveClaim(Automation.ClaimTCIInventory)}, claims...)
	return GameFocus.Acquire(ctx, GameFocus.Request{
		Owner:   GameFocus.OwnerAutoTCI,
		Reason:  reason,
		MaxHold: maxHold,
		Claims:  claims,
	})
}

func autoTCILeaseActive(lease *GameFocus.Lease, event, detail string) bool {
	if lease == nil || lease.Active() {
		return true
	}
	Logging.AutoTCILog(event, detail)
	return false
}

// AutoTCIPresenceStatus is the outcome of checking one settings target against stash + equipped slots.
type AutoTCIPresenceStatus string

const (
	autoTCIPresenceEquipped         AutoTCIPresenceStatus = "equipped"
	autoTCIPresenceNeedEquip        AutoTCIPresenceStatus = "need_equip"
	autoTCIPresenceNeedBuy          AutoTCIPresenceStatus = "need_buy"
	autoTCIPresenceBlockedNoProduct AutoTCIPresenceStatus = "blocked_no_product"
	autoTCIPresenceBlockedNoOID     AutoTCIPresenceStatus = "blocked_no_oid"
	autoTCIPresenceMissingCastle    AutoTCIPresenceStatus = "missing_castle"
	autoTCIPresenceMissingCIData    AutoTCIPresenceStatus = "missing_ci_data"
	autoTCIPresenceUnknownGroup     AutoTCIPresenceStatus = "unknown_group"
	autoTCIPresenceInvalidCeiling   AutoTCIPresenceStatus = "invalid_ceiling"
)

// AutoTCITargetPresence is one configured (castle, catalog group) row after the presence audit.
type AutoTCITargetPresence struct {
	CastleID         int
	SettingsKey      int
	GroupID          int
	MinLevel         int
	Ceiling          int
	FirstTierWireCID int
	StashCount       int
	Status           AutoTCIPresenceStatus
	Entry            ConstructionItemCatalogEntry
	HostOID          int
	PickHint         string
}

// IsAutoTCIRunning reports whether the AutoTCI background loop is active.
func IsAutoTCIRunning() bool {
	autoTCIMu.Lock()
	defer autoTCIMu.Unlock()
	return autoTCICancel != nil
}

// StartAutoTCI starts the AutoTCI goroutine (temporary construction item automation).
func StartAutoTCI() {
	autoTCIMu.Lock()
	defer autoTCIMu.Unlock()
	if autoTCICancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoTCICancel = cancel
	go runAutoTCI(ctx)
}

// StopAutoTCI stops AutoTCI if running.
func StopAutoTCI() {
	autoTCIMu.Lock()
	defer autoTCIMu.Unlock()
	if autoTCICancel != nil {
		autoTCICancel()
		Automation.CancelOwner(Automation.OwnerAutoTCI)
		autoTCICancel = nil
		autoTCINextWakeUpMs = 0
	}
}

// GetAutoTCINextWakeUp returns unix ms of the next loop wake, or 0 while a round is running.
func GetAutoTCINextWakeUp() int64 {
	autoTCIMu.Lock()
	defer autoTCIMu.Unlock()
	return autoTCINextWakeUpMs
}

func setAutoTCINextWake(t time.Time) {
	if !IsAutoTCIRunning() {
		return
	}
	autoTCIMu.Lock()
	autoTCINextWakeUpMs = t.UnixMilli()
	autoTCIMu.Unlock()
	if ResponseRegistry.SendAutoTCIStatusFunc != nil {
		go ResponseRegistry.SendAutoTCIStatusFunc(true)
	}
}

func clearAutoTCINextWake() {
	autoTCIMu.Lock()
	autoTCINextWakeUpMs = 0
	autoTCIMu.Unlock()
}

func runAutoTCI(ctx context.Context) {
	Logging.AutoTCILog("loop_start", "")
	defer Logging.AutoTCILog("loop_stop", "")

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		st := Models.GetSettingsState()
		if st == nil || !st.AutoTCIEnabled || st.AutoTCIList.Targets == nil || len(st.AutoTCIList.Targets) == 0 {
			time.Sleep(autoTCISessionRetry)
			continue
		}

		now := utctime.Now()
		if sleepUntil, blocked := featureScheduleBlockedUntil(st, "autoTCI", now, autoTCIIdleRetry, autoTCIMinSleep); blocked {
			setAutoTCINextWake(sleepUntil)
			Logging.AutoTCILogf("schedule", "inactive; next check at %s", sleepUntil.Format(time.RFC3339))
			select {
			case <-ctx.Done():
				return
			case <-time.After(utctime.Until(sleepUntil)):
			}
			clearAutoTCINextWake()
			continue
		}

		if !EnsureGameSessionOrReload(ctx) {
			time.Sleep(autoTCISessionRetry)
			continue
		}

		catalog, err := ConstructionItemsCatalog()
		if err != nil {
			Logging.AutoTCILogf("round", "catalog: %v", err)
			time.Sleep(autoTCISessionRetry)
			continue
		}

		gs := Models.GetGameState()
		if gs == nil {
			time.Sleep(autoTCISessionRetry)
			continue
		}

		sleepUntil, pending := runAutoTCIRound(ctx, gs, st, catalog)
		if pending {
			retry := utctime.Now().Add(autoTCIPendingRetry)
			if sleepUntil.IsZero() || retry.Before(sleepUntil) {
				sleepUntil = retry
			}
		}
		if sleepUntil.IsZero() {
			sleepUntil = utctime.Now().Add(autoTCIIdleRetry)
		}
		if sleepUntil.Before(utctime.Now().Add(autoTCIMinSleep)) {
			sleepUntil = utctime.Now().Add(autoTCIMinSleep)
		}

		select {
		case <-ctx.Done():
			return
		default:
		}
		if !IsAutoTCIRunning() {
			return
		}

		setAutoTCINextWake(sleepUntil)
		Logging.AutoTCILogf("sleep_until", "%s pending=%v", sleepUntil.Format(time.RFC3339), pending)

		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(sleepUntil)):
		}
		clearAutoTCINextWake()
	}
}

func runAutoTCIRound(ctx context.Context, gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry) (sleepUntil time.Time, hasPendingWork bool) {
	now := utctime.Now()
	nowMs := now.UnixMilli()

	// Phase 1: jaa/jca every configured castle — fresh gca.CI only.
	if !focusAllAutoTCICastles(ctx, gs, st) {
		return sleepUntil, true
	}
	if gs2 := Models.GetGameState(); gs2 != nil {
		gs = gs2
	}

	// Phase 2: upgrades from fresh CI; earliest RS wake wins.
	var scheduleSlots []autotcisched.SlotRecord
	upgradeWake, ranUpgrade := runAutoTCIUpgradePhase(ctx, gs, st, catalog, now, nowMs, &scheduleSlots)
	if !upgradeWake.IsZero() {
		sleepUntil = upgradeWake
	}
	if ranUpgrade {
		t := utctime.Now().Add(autoTCIMinSleep)
		if sleepUntil.IsZero() || t.Before(sleepUntil) {
			sleepUntil = t
		}
	}

	// Phase 3a: main castle — gii + bulk purchase all stash deficits (sbp only works on main).
	// Phase 3b: per-castle equip round (rpc only; no purchase).
	stashPending := runAutoTCIStashEquipPhase(ctx, gs, st, catalog)
	if stashPending {
		hasPendingWork = true
	}

	Logging.AutoTCILogf("round", "done slots=%d pending=%v next=%v", len(scheduleSlots), hasPendingWork, sleepUntil.Format(time.RFC3339))
	return sleepUntil, hasPendingWork
}

// focusAllAutoTCICastles sends jaa/jca for each castle in settings (loads gca.CI); no gii/equip/buy.
func focusAllAutoTCICastles(ctx context.Context, gs *Models.GameState, st *Models.SettingsState) bool {
	if gs == nil || st == nil {
		return false
	}
	keys := sortedTargetCastleIDs(st.AutoTCIList.Targets)
	Logging.AutoTCILogf("jaa sweep", "start castles=%d", len(keys))
	var focused, skipped int
	for _, castleID := range keys {
		select {
		case <-ctx.Done():
			return false
		default:
		}
		if !ResponseRegistry.LoginStatus {
			return false
		}
		perCastle := st.AutoTCIList.Targets[castleID]
		if len(perCastle) == 0 {
			Logging.AutoTCILogf("jaa sweep", "skip key=%d (no targets)", castleID)
			skipped++
			continue
		}
		c, castleAID, ok := resolveCastleForAutoTCISettings(gs, castleID)
		if !ok || c == nil || castleAID <= 0 {
			Logging.AutoTCILogf("jaa sweep", "skip key=%d (no AID / map coords)", castleID)
			skipped++
			continue
		}
		kid, _, _, mapOK := resolveAutoTCICastleMapCoords(gs, castleAID, c)
		Logging.AutoTCILogf("jaa sweep", "focus key=%d AID=%d KID=%d mapOK=%v items=%d",
			castleID, castleAID, kid, mapOK, len(perCastle))
		lease, leaseOK := acquireAutoTCILease(ctx, fmt.Sprintf("jaa sweep castle=%d", castleAID), 20*time.Second)
		if !leaseOK {
			return false
		}
		focusOK := false
		func() {
			defer lease.Release()
			focusOK = ensureClientFocusedOnCastleWithLease(lease, gs, c, castleAID, true)
		}()
		if !focusOK {
			if lease.Revoked() {
				Logging.AutoTCILogf("lease revoked", "jaa sweep AID=%d", castleAID)
				return false
			}
			Logging.AutoTCILogf("jaa sweep", "focus failed AID=%d", castleAID)
			skipped++
			continue
		}
		if c2 := gs.GetCastleByID(castleAID); c2 != nil {
			Logging.AutoTCILogf("jaa sweep", "loaded AID=%d CI_buildings=%d", castleAID, len(c2.ConstructionByBuilding))
		}
		focused++
	}
	Logging.AutoTCILogf("jaa sweep", "done focused=%d skipped=%d", focused, skipped)
	return focused > 0 || skipped == 0
}

// runAutoTCIUpgradePhase scans equipped slots, fires due ubc, returns earliest scheduled wake.
func runAutoTCIUpgradePhase(
	ctx context.Context,
	gs *Models.GameState,
	st *Models.SettingsState,
	catalog []ConstructionItemCatalogEntry,
	now time.Time,
	nowMs int64,
	scheduleSlots *[]autotcisched.SlotRecord,
) (sleepUntil time.Time, ranUpgrade bool) {
	if gs == nil || st == nil {
		return sleepUntil, false
	}
	var immediate [][3]int
	for _, castleID := range sortedTargetCastleIDs(st.AutoTCIList.Targets) {
		perCastle := st.AutoTCIList.Targets[castleID]
		if len(perCastle) == 0 {
			continue
		}
		c, castleAID, ok := resolveCastleForAutoTCISettings(gs, castleID)
		if !ok || c == nil {
			continue
		}
		due := collectAutoTCIUpgradeScheduleSlots(
			castleAID, c, perCastle, catalog, now, nowMs, scheduleSlots,
		)
		immediate = append(immediate, due...)
	}
	if len(immediate) > 0 {
		Logging.AutoTCILogf("upgrade", "immediate ubc slots=%d", len(immediate))
	}
	for i := range immediate {
		executeOrRetryAutoTCIForSlot(ctx, immediate[i][0], immediate[i][1], immediate[i][2])
		ranUpgrade = true
	}
	if nextMs := autotcisched.NextScheduleWakeMillis(&autotcisched.File{Slots: *scheduleSlots}, nowMs); nextMs > 0 {
		sleepUntil = time.UnixMilli(nextMs)
	} else if len(*scheduleSlots) > 0 {
		sleepUntil = utctime.Now().Add(autoTCIMinSleep)
	}
	return sleepUntil, ranUpgrade
}

// collectAutoTCIUpgradeScheduleSlots records RS wake rows and slots due for ubc now.
func collectAutoTCIUpgradeScheduleSlots(
	castleID int,
	c *castle.PlayerCastleInfo,
	perCastle map[int]stsettings.AutoTCILevelTarget,
	catalog []ConstructionItemCatalogEntry,
	now time.Time,
	nowMs int64,
	scheduleSlots *[]autotcisched.SlotRecord,
) (immediateUbc [][3]int) {
	if c == nil {
		return nil
	}
	for _, b := range c.ConstructionByBuilding {
		for j := range b.Slots {
			sl := b.Slots[j]
			entry := catalogEntryForWireCID(catalog, sl.CID)
			if entry == nil {
				continue
			}
			minLevel, ceiling, hasAuto := levelRangeForAutoTCIEntry(perCastle, entry)
			if !hasAuto {
				continue
			}
			nextTier, _, haveNext := nextTierAfterUpgrade(entry, sl.CID)
			canUbc := haveNext && nextTier <= ceiling && nextTier >= minLevel
			if !canUbc {
				continue
			}
			if sl.RemainingSec != nil && *sl.RemainingSec == 0 {
				immediateUbc = append(immediateUbc, [3]int{castleID, b.OID, sl.S})
				continue
			}
			if sl.RemainingSec == nil || *sl.RemainingSec <= 0 {
				continue
			}
			rs := *sl.RemainingSec
			exMs := now.Add(time.Duration(rs) * time.Second).UnixMilli()
			ubcMs := exMs - autotcisched.UbcWindowMillis
			if ubcMs < 0 {
				ubcMs = 0
			}
			loginMs := exMs - autotcisched.LoginLeadMillis
			if loginMs < 0 {
				loginMs = 0
			}
			*scheduleSlots = append(*scheduleSlots, autotcisched.SlotRecord{
				CastleID: castleID, OID: b.OID, SlotS: sl.S,
				WireCID: sl.CID, RSSeconds: rs,
				ExpiresAtMillis: exMs, LoginWakeAtMillis: loginMs, FireAtMillis: ubcMs,
				ObservedAtMillis: nowMs,
			})
			if rs <= autoTCIUpgradeTimerMaxSec || ubcMs <= nowMs {
				immediateUbc = append(immediateUbc, [3]int{castleID, b.OID, sl.S})
			}
		}
	}
	return immediateUbc
}

// runAutoTCIStashEquipPhase runs after jaa + ubc: bulk buy on main, then a separate equip round.
func runAutoTCIStashEquipPhase(ctx context.Context, gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry) (hasPendingWork bool) {
	if gs == nil || st == nil {
		return false
	}
	presence := evaluateAutoTCIPresence(gs, catalog, st.AutoTCIList.Targets)
	logAutoTCIPresenceAudit(presence)
	if !autoTCINeedsStashPhase(gs, st, catalog, presence) {
		Logging.AutoTCILog("stash", "skip — all targets equipped (upgrades handled in phase 2)")
		return false
	}

	runAutoTCIMainBulkPurchasePhase(ctx, gs, st, catalog)
	presence = evaluateAutoTCIPresence(gs, catalog, st.AutoTCIList.Targets)
	logAutoTCIPresenceAudit(presence)

	Logging.AutoTCILog("equip-round", "start — rpc per castle (purchases already done on main)")
	tryAutoTCIEquipFromPresence(ctx, gs, catalog, presence)

	for _, row := range presence {
		switch row.Status {
		case autoTCIPresenceNeedEquip, autoTCIPresenceNeedBuy,
			autoTCIPresenceBlockedNoOID, autoTCIPresenceBlockedNoProduct:
			hasPendingWork = true
		}
	}
	return hasPendingWork
}

// runAutoTCIMainBulkPurchasePhase focuses main, gii once, buys every deficit from main, gii again.
// Trivial-shop sbp only succeeds on the main castle — outpost AIDs return wire code 203.
func runAutoTCIMainBulkPurchasePhase(ctx context.Context, gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry) {
	if gs == nil || st == nil {
		return
	}
	Logging.AutoTCILog("purchase-bulk", "start — focus main, gii, buy all deficits (before equip round)")
	lease, ok := acquireAutoTCILease(ctx, "purchase bulk main refresh", 30*time.Second)
	if !ok {
		return
	}
	mainRefreshOK := false
	func() {
		defer lease.Release()
		if !ensureAutoTCIFocusedOnMainCastleWithLease(lease, gs) {
			Logging.AutoTCILog("purchase-bulk", "defer — could not focus main castle")
			return
		}
		refreshAutoTCIStashOnceWithLease(lease)
		mainRefreshOK = lease.Active()
	}()
	if !mainRefreshOK {
		return
	}
	presence := evaluateAutoTCIPresence(gs, catalog, st.AutoTCIList.Targets)
	logAutoTCIPresenceAudit(presence)

	autoTCIPurchaseStashDeficits(ctx, gs, presence)
	runAutoTCIMainRebuyPurchases(ctx, gs, st, catalog)

	refreshAutoTCIStashOnce(ctx)
	Logging.AutoTCILog("purchase-bulk", "done — final gii on main; equip round follows")
}

func runAutoTCIMainRebuyPurchases(ctx context.Context, gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry) {
	if gs == nil || st == nil {
		return
	}
	mainC, mainAID, mainOK := resolveAutoTCIMainCastle(gs)
	if !mainOK {
		return
	}
	for _, slot := range collectDueRebuySlots(gs, st, catalog) {
		castleID, oid, slotS := slot[0], slot[1], slot[2]
		c := gs.GetCastleByID(castleID)
		if c == nil {
			continue
		}
		per := st.AutoTCIList.Targets[castleID]
		for _, b := range c.ConstructionByBuilding {
			if b.OID != oid {
				continue
			}
			for j := range b.Slots {
				sl := b.Slots[j]
				if sl.S != slotS {
					continue
				}
				entry := catalogEntryForWireCID(catalog, sl.CID)
				if entry == nil {
					continue
				}
				first := firstTierWireCIDForEntry(entry)
				if first <= 0 {
					continue
				}
				tinfo, havePid := GameParser.TrivialCIPurchaseInfoForCid(first)
				if !havePid {
					continue
				}
				_, ceiling, ok := levelRangeForAutoTCIEntry(per, entry)
				if !ok {
					continue
				}
				nextTier, _, haveNext := nextTierAfterUpgrade(entry, sl.CID)
				if haveNext && nextTier <= ceiling {
					continue
				}
				en := *entry
				Logging.AutoTCILogf("purchase-bulk", "rebuy mainAID=%d castle=%d firstCID=%d", mainAID, castleID, first)
				runAutoTCIMainPurchaseUnit(ctx, mainC, mainAID, &en, first, tinfo, "rebuy-ceil", false)
				return
			}
		}
	}
}

func autoTCINeedsStashPhase(gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry, presence []AutoTCITargetPresence) bool {
	for _, row := range presence {
		switch row.Status {
		case autoTCIPresenceEquipped, autoTCIPresenceInvalidCeiling, autoTCIPresenceUnknownGroup:
			continue
		default:
			return true
		}
	}
	return len(collectDueRebuySlots(gs, st, catalog)) > 0
}

func collectDueRebuySlots(gs *Models.GameState, st *Models.SettingsState, catalog []ConstructionItemCatalogEntry) [][3]int {
	if gs == nil || st == nil {
		return nil
	}
	var out [][3]int
	now := utctime.Now()
	for _, castleID := range sortedTargetCastleIDs(st.AutoTCIList.Targets) {
		perCastle := st.AutoTCIList.Targets[castleID]
		c, _, ok := resolveCastleForAutoTCISettings(gs, castleID)
		if !ok || c == nil || len(perCastle) == 0 {
			continue
		}
		for _, b := range c.ConstructionByBuilding {
			for j := range b.Slots {
				sl := b.Slots[j]
				if sl.RemainingSec == nil || *sl.RemainingSec <= 0 || *sl.RemainingSec > autoTCIRebuyLeadSec {
					continue
				}
				entry := catalogEntryForWireCID(catalog, sl.CID)
				if entry == nil {
					continue
				}
				minLevel, ceiling, ok := levelRangeForAutoTCIEntry(perCastle, entry)
				if !ok {
					continue
				}
				nextTier, _, haveNext := nextTierAfterUpgrade(entry, sl.CID)
				if haveNext && nextTier <= ceiling {
					continue
				}
				first := firstTierWireCIDForEntry(entry)
				if first <= 0 || ciStashCountInLevelRange(gs, entry, minLevel, ceiling) >= 1 {
					continue
				}
				rs := *sl.RemainingSec
				exMs := now.Add(time.Duration(rs) * time.Second).UnixMilli()
				ubcMs := exMs - autotcisched.UbcWindowMillis
				if ubcMs < 0 {
					ubcMs = 0
				}
				if rs <= autoTCIUpgradeTimerMaxSec || ubcMs <= now.UnixMilli() {
					out = append(out, [3]int{castleID, b.OID, sl.S})
				}
			}
		}
	}
	return out
}

// autoTCIPurchaseDeficit is one first-tier wire CID shortfall after post-gii presence audit.
type autoTCIPurchaseDeficit struct {
	FirstTierWireCID int
	Demand           int
	Have             int
	Deficit          int
	SampleRow        AutoTCITargetPresence
	CanBuy           bool
}

// planAutoTCIPurchaseDeficits counts unequipped targets per first-tier CID vs account stash (post-gii).
func planAutoTCIPurchaseDeficits(gs *Models.GameState, presence []AutoTCITargetPresence) []autoTCIPurchaseDeficit {
	if gs == nil {
		return nil
	}
	needByCID := map[int]int{}
	rowByCID := map[int]AutoTCITargetPresence{}
	for _, row := range presence {
		switch row.Status {
		case autoTCIPresenceEquipped, autoTCIPresenceInvalidCeiling, autoTCIPresenceUnknownGroup:
			continue
		case autoTCIPresenceNeedBuy, autoTCIPresenceNeedEquip,
			autoTCIPresenceBlockedNoOID, autoTCIPresenceMissingCIData, autoTCIPresenceBlockedNoProduct:
		default:
			continue
		}
		buyCID := firstTierWireCIDForEntry(&row.Entry)
		if buyCID <= 0 {
			continue
		}
		needByCID[buyCID]++
		if _, ok := rowByCID[buyCID]; !ok {
			rowByCID[buyCID] = row
		}
	}
	var plans []autoTCIPurchaseDeficit
	for firstCID, demand := range needByCID {
		sample := rowByCID[firstCID]
		have := ciStashCountInLevelRange(gs, &sample.Entry, sample.MinLevel, sample.Ceiling)
		deficit := demand - have
		if deficit <= 0 {
			continue
		}
		row := rowByCID[firstCID]
		_, canBuy := GameParser.TrivialCIPurchaseInfoForCid(firstCID)
		plans = append(plans, autoTCIPurchaseDeficit{
			FirstTierWireCID: firstCID,
			Demand:           demand,
			Have:             have,
			Deficit:          deficit,
			SampleRow:        row,
			CanBuy:           canBuy,
		})
	}
	sort.Slice(plans, func(i, j int) bool {
		return plans[i].FirstTierWireCID < plans[j].FirstTierWireCID
	})
	return plans
}

// autoTCIPurchaseStashDeficits buys when account stash is short for unequipped targets (uses post-gii counts).
func autoTCIPurchaseStashDeficits(ctx context.Context, gs *Models.GameState, presence []AutoTCITargetPresence) {
	if gs == nil {
		return
	}
	plans := planAutoTCIPurchaseDeficits(gs, presence)
	if len(plans) == 0 {
		Logging.AutoTCILog("purchase-plan", "no stash deficits after gii")
		return
	}
	for _, plan := range plans {
		if !plan.CanBuy {
			Logging.AutoTCILogf("purchase-plan", "skip firstCID=%d demand=%d have=%d deficit=%d (no trivial product)",
				plan.FirstTierWireCID, plan.Demand, plan.Have, plan.Deficit)
			continue
		}
		Logging.AutoTCILogf("purchase-plan", "firstCID=%d demand=%d have=%d deficit=%d",
			plan.FirstTierWireCID, plan.Demand, plan.Have, plan.Deficit)
	}
	mainC, mainAID, mainOK := resolveAutoTCIMainCastle(gs)
	if !mainOK {
		Logging.AutoTCILog("purchase-batch", "skip — no MainCastle AID")
		return
	}
	for _, plan := range plans {
		if !plan.CanBuy {
			continue
		}
		row := plan.SampleRow
		tinfo, ok := GameParser.TrivialCIPurchaseInfoForCid(plan.FirstTierWireCID)
		if !ok {
			continue
		}
		entry := row.Entry
		for i := 0; i < plan.Deficit; i++ {
			have := ciStashCountForFirstTierWireCID(gs, plan.FirstTierWireCID)
			Logging.AutoTCILogf("purchase-batch", "deficit firstCID=%d demand=%d have=%d buy=%d/%d mainAID=%d",
				plan.FirstTierWireCID, plan.Demand, have, i+1, plan.Deficit, mainAID)
			if !runAutoTCIMainPurchaseUnit(ctx, mainC, mainAID, &entry, plan.FirstTierWireCID, tinfo, "purchase-batch", false) {
				break
			}
			if gs2 := Models.GetGameState(); gs2 != nil {
				gs = gs2
			}
		}
	}
}

func refreshAutoTCIStashOnce(ctx context.Context) {
	lease, ok := acquireAutoTCILease(ctx, "gii refresh", 20*time.Second)
	if !ok {
		return
	}
	defer lease.Release()
	refreshAutoTCIStashOnceWithLease(lease)
}

func refreshAutoTCIStashOnceWithLease(lease *GameFocus.Lease) {
	if !ResponseRegistry.LoginStatus || !IsAutoTCIRunning() {
		return
	}
	Logging.AutoTCILog("gii", "account stash refresh")
	if !autoTCILeaseActive(lease, "lease revoked", "aec skipped") {
		return
	}
	GameCommands.SendAECAutoTCI(lease)
	w := ResponseRegistry.Global.RegisterWaiter("gii", 8*time.Second)
	if !autoTCILeaseActive(lease, "lease revoked", "gii skipped") {
		w.Cleanup()
		return
	}
	GameCommands.SendGIIAutoTCI(lease)
	_, err := w.WaitWithTimeout()
	w.Cleanup()
	time.Sleep(80 * time.Millisecond)
	if err != nil {
		Logging.AutoTCILogf("gii", "stash refresh: %v", err)
	}
}

func evaluateAutoTCIPresence(gs *Models.GameState, catalog []ConstructionItemCatalogEntry, targets map[int]map[int]stsettings.AutoTCILevelTarget) []AutoTCITargetPresence {
	if gs == nil || len(targets) == 0 {
		return nil
	}
	byID := make(map[int]ConstructionItemCatalogEntry, len(catalog))
	for i := range catalog {
		byID[catalog[i].ID] = catalog[i]
	}
	var out []AutoTCITargetPresence
	for _, castleID := range sortedTargetCastleIDs(targets) {
		perCastle := targets[castleID]
		if len(perCastle) == 0 {
			continue
		}
		c, _, _ := resolveCastleForAutoTCISettings(gs, castleID)
		for _, settingsKey := range sortedTargetKeysFromLevelMap(perCastle) {
			row := AutoTCITargetPresence{CastleID: castleID, SettingsKey: settingsKey}
			entry, ok := resolveAutoTCIEntry(byID, catalog, settingsKey)
			if !ok {
				row.Status = autoTCIPresenceUnknownGroup
				out = append(out, row)
				continue
			}
			row.Entry = entry
			row.GroupID = entry.ID
			minLevel, ceiling, hasCeiling := levelRangeForAutoTCIEntry(perCastle, &entry)
			if !hasCeiling {
				row.Status = autoTCIPresenceInvalidCeiling
				out = append(out, row)
				continue
			}
			row.MinLevel = minLevel
			row.Ceiling = ceiling
			if c == nil {
				row.Status = autoTCIPresenceMissingCastle
				out = append(out, row)
				continue
			}
			if castleHasConstructionGroup(c, catalog, &entry) {
				row.Status = autoTCIPresenceEquipped
				out = append(out, row)
				continue
			}
			equipCID, stash := bestEquipWireCIDInLevelRange(gs, &entry, minLevel, ceiling)
			if equipCID <= 0 {
				equipCID = firstTierWireCIDForEntry(&entry)
			}
			row.FirstTierWireCID = equipCID
			if equipCID <= 0 {
				row.Status = autoTCIPresenceUnknownGroup
				out = append(out, row)
				continue
			}
			if stash <= 0 {
				stash = ciStashCountInLevelRange(gs, &entry, minLevel, ceiling)
			}
			row.StashCount = stash
			if row.StashCount >= 1 {
				row.Status = autoTCIPresenceNeedEquip
				if len(c.ConstructionByBuilding) == 0 {
					row.Status = autoTCIPresenceMissingCIData
				} else {
					oid, hint := pickConstructionHostOIDForEquip(c, catalog, &entry)
					row.HostOID, row.PickHint = oid, hint
					if oid <= 0 {
						row.Status = autoTCIPresenceBlockedNoOID
					}
				}
				out = append(out, row)
				continue
			}
			buyCID := firstTierWireCIDForEntry(&entry)
			if _, havePid := GameParser.TrivialCIPurchaseInfoForCid(buyCID); !havePid {
				row.Status = autoTCIPresenceBlockedNoProduct
				out = append(out, row)
				continue
			}
			row.Status = autoTCIPresenceNeedBuy
			out = append(out, row)
		}
	}
	return out
}

func logAutoTCIPresenceAudit(rows []AutoTCITargetPresence) {
	if len(rows) == 0 {
		Logging.AutoTCILog("presence", "audit targets=0 (no settings or castles)")
		return
	}
	var equipped, needEquip, needBuy, blockedProduct, blockedOID, other int
	for _, r := range rows {
		switch r.Status {
		case autoTCIPresenceEquipped:
			equipped++
		case autoTCIPresenceNeedEquip:
			needEquip++
		case autoTCIPresenceNeedBuy:
			needBuy++
		case autoTCIPresenceBlockedNoProduct:
			blockedProduct++
		case autoTCIPresenceBlockedNoOID:
			blockedOID++
		default:
			other++
		}
	}
	Logging.AutoTCILogf("presence", "audit targets=%d equipped=%d need_equip=%d need_buy=%d blocked_product=%d blocked_oid=%d other=%d",
		len(rows), equipped, needEquip, needBuy, blockedProduct, blockedOID, other)
	for _, r := range rows {
		switch r.Status {
		case autoTCIPresenceNeedEquip, autoTCIPresenceNeedBuy,
			autoTCIPresenceBlockedNoOID, autoTCIPresenceMissingCastle, autoTCIPresenceMissingCIData:
			if r.Status == autoTCIPresenceBlockedNoOID && r.PickHint != "" {
				Logging.AutoTCILogf("presence", "castle=%d %q stash=%d status=%s firstCID=%d hint=%s",
					r.CastleID, autoTCILogLabel(r), r.StashCount, r.Status, r.FirstTierWireCID, r.PickHint)
			} else {
				Logging.AutoTCILogf("presence", "castle=%d %q stash=%d status=%s firstCID=%d",
					r.CastleID, autoTCILogLabel(r), r.StashCount, r.Status, r.FirstTierWireCID)
			}
		case autoTCIPresenceBlockedNoProduct:
			Logging.AutoTCILogf("presence", "castle=%d %q no CidTrivialProduct for firstCID=%d",
				r.CastleID, autoTCILogLabel(r), r.FirstTierWireCID)
		}
	}
}

func tryAutoTCIEquipFromPresence(ctx context.Context, gs *Models.GameState, catalog []ConstructionItemCatalogEntry, presence []AutoTCITargetPresence) {
	if gs == nil || !ResponseRegistry.LoginStatus || len(presence) == 0 {
		return
	}
	virtualStash := map[int]int{}
	for _, row := range presence {
		if row.FirstTierWireCID <= 0 {
			continue
		}
		if _, seeded := virtualStash[row.FirstTierWireCID]; !seeded {
			virtualStash[row.FirstTierWireCID] = ciStashCountForFirstTierWireCID(gs, row.FirstTierWireCID)
		}
	}
	equips := 0
	for _, row := range presence {
		if row.Status != autoTCIPresenceNeedEquip {
			continue
		}
		if virtualStash[row.FirstTierWireCID] <= 0 {
			Logging.AutoTCILogf("equip skip", "castle=%d %q firstCID=%d — virtual stash empty (buy shortfall)",
				row.CastleID, autoTCILogLabel(row), row.FirstTierWireCID)
			continue
		}
		if equips >= maxAutoTCIEquipsPerRound {
			break
		}
		c := gs.GetCastleByID(row.CastleID)
		if c == nil || row.FirstTierWireCID <= 0 {
			continue
		}
		if catalogEntryForWireCID(catalog, row.FirstTierWireCID) == nil {
			Logging.AutoTCILogf("equip skip", "CID %d not in catalog (group %d)", row.FirstTierWireCID, row.GroupID)
			continue
		}
		lease, leaseOK := acquireAutoTCILease(ctx, fmt.Sprintf("rpc castle=%d", row.CastleID), 20*time.Second)
		if !leaseOK {
			return
		}
		focusOK := false
		func() {
			defer lease.Release()
			if !ensureClientFocusedOnCastleWithLease(lease, gs, c, row.CastleID) {
				return
			}
			focusOK = true
			c = gs.GetCastleByID(row.CastleID)
			if c == nil {
				return
			}
			oid, pickHint := pickConstructionHostOIDForEquip(c, catalog, &row.Entry)
			if oid <= 0 {
				Logging.AutoTCILogf("equip skip", "no OID for group %d firstCID=%d (%s)", row.GroupID, row.FirstTierWireCID, pickHint)
				return
			}
			aid := autoTCICastleAID(c, row.CastleID)
			if aid <= 0 {
				return
			}
			dk := presenceRowDedupKey("eq", row)
			if t, ok := equipLastSent.Load(dk); ok {
				if utctime.Since(t.(time.Time)) < autoTCIEquipDedupWindow {
					return
				}
			}
			kid := autoTCICastleKID(gs, c, aid)
			bName := buildingNameForOID(c, oid)
			Logging.AutoTCILogf("equip", "rpc AID=%d OID=%d (%s) KID=%d firstCID=%d group=%d stash=%d hint=%s",
				aid, oid, bName, kid, row.FirstTierWireCID, row.GroupID, row.StashCount, pickHint)
			if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("rpc skipped AID=%d", aid)) {
				return
			}
			GameCommands.SendRPCAutoTCI(oid, row.FirstTierWireCID, 0, 0, kid, aid, lease)
			equipLastSent.Store(dk, utctime.Now())
			virtualStash[row.FirstTierWireCID]--
			equips++
		}()
		if !focusOK {
			if lease.Revoked() {
				Logging.AutoTCILogf("lease revoked", "equip castle=%d", row.CastleID)
				return
			}
			Logging.AutoTCILogf("equip", "focus failed castle=%d", row.CastleID)
			continue
		}
	}
}

// runAutoTCIMainPurchaseUnit buys one trivial-shop unit from the main castle (gbc/sbp require main focus).
// refreshGiiAfter: bulk phase passes false and relies on one final gii; standalone rebuy may pass true.
func runAutoTCIMainPurchaseUnit(ctx context.Context, c *castle.PlayerCastleInfo, mainAID int, entry *ConstructionItemCatalogEntry, firstCID int, tinfo GameParser.TrivialCIPurchaseInfo, reason string, refreshGiiAfter bool) bool {
	gs := Models.GetGameState()
	if c == nil || gs == nil || mainAID <= 0 || !ResponseRegistry.LoginStatus || !IsAutoTCIRunning() {
		return false
	}
	lease, ok := acquireAutoTCILease(
		ctx,
		fmt.Sprintf("%s main purchase", reason),
		45*time.Second,
		GameFocus.ExclusiveClaim(Automation.ClaimAccountResources),
	)
	if !ok {
		return false
	}
	defer lease.Release()
	if !ensureAutoTCIFocusedOnMainCastleWithLease(lease, gs) {
		if !lease.Active() {
			Logging.AutoTCILogf("lease revoked", "%s mainAID=%d", reason, mainAID)
			return false
		}
		Logging.AutoTCILogf(reason, "focus main failed AID=%d", mainAID)
		return false
	}
	kid := autoTCICastleKID(gs, c, mainAID)

	w := ResponseRegistry.Global.RegisterWaiter("gbc", 6*time.Second)
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s gbc skipped", reason)) {
		w.Cleanup()
		return false
	}
	Logging.AutoTCILogf("buy gbc", "CID(AID)=%d KID=%d", mainAID, kid)
	GameCommands.SendAECAutoTCI(lease)
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s gbc skipped", reason)) {
		w.Cleanup()
		return false
	}
	GameCommands.QueueAutoTCIOutgoing(GameCommands.GBCPayload(mainAID, kid), lease)
	_, err := w.WaitWithTimeout()
	if err != nil {
		Logging.AutoTCILogf(reason, "gbc wait: %v", err)
	}
	w.Cleanup()
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s after gbc", reason)) {
		return false
	}

	gs2 := Models.GetGameState()
	if gs2 == nil {
		return false
	}
	needAmt := tinfo.Amt
	if needAmt <= 0 {
		needAmt = 1
	}
	amt := needAmt
	if a, ok := gs2.GbcCIPLFindAMT(tinfo.PID); ok && a > 0 && a < needAmt {
		amt = a
	}
	if amt <= 0 {
		Logging.AutoTCILogf(reason, "skip sbp: invalid amt for PID=%d firstCID=%d", tinfo.PID, firstCID)
		return false
	}
	Logging.AutoTCILogf(reason, "sbp PID=%d AMT=%d (need=%d, firstCID=%d, mainAID=%d)", tinfo.PID, amt, needAmt, firstCID, mainAID)
	wSbp := ResponseRegistry.Global.RegisterWaiter("sbp", 6*time.Second)
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s sbp skipped", reason)) {
		wSbp.Cleanup()
		return false
	}
	Logging.AutoTCILogf("buy sbp", "PID=%d AMT=%d KID=%d AID=%d (TID=%d BT=%d)",
		tinfo.PID, amt, kid, mainAID, GameCommands.AutoTCITrivialSBPTypeID, GameCommands.AutoTCITrivialSBPBuildType)
	GameCommands.QueueAutoTCIOutgoing(GameCommands.SBPPayload(
		tinfo.PID,
		GameCommands.AutoTCITrivialSBPBuildType,
		GameCommands.AutoTCITrivialSBPTypeID,
		amt,
		kid,
		mainAID,
		GameCommands.AutoTCITrivialSBPPC2,
		GameCommands.AutoTCITrivialSBPBuildAux,
		GameCommands.AutoTCITrivialSBPPower,
		GameCommands.AutoTCITrivialSBPPO,
	), lease)
	sbpResp, sbpErr := wSbp.WaitWithTimeout()
	wSbp.Cleanup()
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s after sbp", reason)) {
		return false
	}
	if sbpErr != nil {
		Logging.AutoTCILogf(reason, "sbp wait: %v", sbpErr)
		return false
	}
	if !autoTCISbpWireOK(sbpResp) {
		code := autoTCIWireStatusCode(sbpResp)
		Logging.AutoTCILogf(reason, "sbp rejected code=%s (mainAID=%d PID=%d)", code, mainAID, tinfo.PID)
		return false
	}
	// sbp success includes an inbound gii that ReplaceCIInventoryCountsFromMap applies — do not MergeCIInventoryDelta (+1 would double-count).
	time.Sleep(autoTCIPostPurchaseSettlePause)
	if refreshGiiAfter {
		wG := ResponseRegistry.Global.RegisterWaiter("gii", autoTCIPostPurchaseGIIWait)
		if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("%s gii skipped", reason)) {
			wG.Cleanup()
			return false
		}
		GameCommands.SendGIIAutoTCI(lease)
		if _, err := wG.WaitWithTimeout(); err != nil {
			Logging.AutoTCILogf(reason, "gii after purchase: %v", err)
		}
		wG.Cleanup()
		time.Sleep(autoTCIPostPurchaseSettlePause)
	}
	Logging.AutoTCILogf(reason, "purchase complete mainAID=%d group=%d firstCID=%d", mainAID, entry.ID, firstCID)
	return true
}

func resolveAutoTCIMainCastle(gs *Models.GameState) (c *castle.PlayerCastleInfo, aid int, ok bool) {
	if gs == nil {
		return nil, 0, false
	}
	c = &gs.Castle.MainCastle
	aid = int(c.Aid)
	if aid <= 0 {
		return nil, 0, false
	}
	return c, aid, true
}

func ensureAutoTCIFocusedOnMainCastle(gs *Models.GameState) bool {
	c, aid, ok := resolveAutoTCIMainCastle(gs)
	if !ok {
		Logging.AutoTCILog("focus main", "skip: MainCastle AID unknown")
		return false
	}
	return ensureClientFocusedOnCastle(gs, c, aid, true)
}

func ensureAutoTCIFocusedOnMainCastleWithLease(lease *GameFocus.Lease, gs *Models.GameState) bool {
	c, aid, ok := resolveAutoTCIMainCastle(gs)
	if !ok {
		Logging.AutoTCILog("focus main", "skip: MainCastle AID unknown")
		return false
	}
	return ensureClientFocusedOnCastleWithLease(lease, gs, c, aid, true)
}

func autoTCIWireStatusCode(parts []string) string {
	if len(parts) <= 4 {
		return "?"
	}
	return parts[4]
}

func autoTCISbpWireOK(parts []string) bool {
	return autoTCIWireStatusCode(parts) == "0"
}

func executeRebuyAutoTCIForSlot(castleID, oid, slotS int) {
	if !ResponseRegistry.LoginStatus || !IsAutoTCIRunning() {
		return
	}
	gs := Models.GetGameState()
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return
	}
	catalog, err := ConstructionItemsCatalog()
	if err != nil {
		return
	}
	st := Models.GetSettingsState()
	if st == nil {
		return
	}
	per := st.AutoTCIList.Targets[castleID]
	if len(per) == 0 {
		return
	}
	dedup := fmt.Sprintf("rebuy:%d:%d:%d", castleID, oid, slotS)
	if t, ok := buyGoroutineLast.Load(dedup); ok {
		if utctime.Since(t.(time.Time)) < autoTCIBuyRebuyGoroutineDedup {
			return
		}
	}
	buyGoroutineLast.Store(dedup, utctime.Now())
	for _, b := range c.ConstructionByBuilding {
		if b.OID != oid {
			continue
		}
		for j := range b.Slots {
			sl := b.Slots[j]
			if sl.S != slotS {
				continue
			}
			if sl.RemainingSec == nil || *sl.RemainingSec > autoTCIRebuyLeadSec {
				return
			}
			entry := catalogEntryForWireCID(catalog, sl.CID)
			if entry == nil {
				return
			}
			_, ceiling, ok := levelRangeForAutoTCIEntry(per, entry)
			if !ok {
				return
			}
			nextTier, _, haveNext := nextTierAfterUpgrade(entry, sl.CID)
			if haveNext && nextTier <= ceiling {
				return
			}
			first := firstTierWireCIDForEntry(entry)
			if first <= 0 {
				return
			}
			tinfo, havePid := GameParser.TrivialCIPurchaseInfoForCid(first)
			if !havePid {
				Logging.AutoTCILogf("rebuy", "no packages/items.json trivial-shop PID for firstCID=%d (or add DataDir CidTrivialProduct.json)", first)
				return
			}
			mainC, mainAID, mainOK := resolveAutoTCIMainCastle(gs)
			if !mainOK {
				Logging.AutoTCILog("rebuy", "skip — no MainCastle AID")
				return
			}
			en := *entry
			runAutoTCIMainPurchaseUnit(context.Background(), mainC, mainAID, &en, first, tinfo, "rebuy-ceil", true)
			return
		}
	}
}

func executeOrRetryAutoTCIForSlot(ctx context.Context, castleID, oid, slotS int) {
	if !ResponseRegistry.LoginStatus || !IsAutoTCIRunning() {
		return
	}
	st := Models.GetSettingsState()
	if st == nil || !st.AutoTCIEnabled {
		return
	}
	perCastle, ok := st.AutoTCIList.Targets[castleID]
	if !ok || len(perCastle) == 0 {
		return
	}
	gs := Models.GetGameState()
	if gs == nil {
		return
	}
	c := gs.GetCastleByID(castleID)
	if c == nil {
		return
	}
	catalog, err := ConstructionItemsCatalog()
	if err != nil {
		return
	}
	for _, b := range c.ConstructionByBuilding {
		if b.OID != oid {
			continue
		}
		for j := range b.Slots {
			sl := b.Slots[j]
			if sl.S != slotS || !slotReadyForUbcUpgrade(sl) {
				continue
			}
			entry := catalogEntryForWireCID(catalog, sl.CID)
			if entry == nil {
				return
			}
			minLevel, ceiling, hasAuto := levelRangeForAutoTCIEntry(perCastle, entry)
			if !hasAuto {
				return
			}
			nextTier, nextCID, haveNext := nextTierAfterUpgrade(entry, sl.CID)
			if !haveNext || nextTier > ceiling || nextTier < minLevel {
				return
			}
			tryExecuteSingleAutoTCIUpgrade(ctx, gs, c, castleID, b, sl, nextTier, nextCID, ceiling)
			return
		}
	}
}

func tryExecuteSingleAutoTCIUpgrade(ctx context.Context, gs *Models.GameState, c *castle.PlayerCastleInfo, castleID int, b Models.GCAConstructionBuilding, sl Models.GCAConstructionSlot, nextTier, nextCID, ceiling int) {
	castleAID := autoTCICastleAID(c, castleID)
	if castleAID <= 0 {
		return
	}
	suc, ok := autoTCIUBCSUCForTargetTier(nextTier)
	if !ok {
		Logging.AutoTCILogf("ubc skip", "AID=%d OID=%d slot=%d currentCID=%d nextTier=%d nextCID=%d has no SUC mapping",
			castleAID, b.OID, sl.S, sl.CID, nextTier, nextCID)
		return
	}
	dk := fmt.Sprintf("%d:%d:%d", castleAID, b.OID, sl.CID)
	if t, ok := upgradeLastSent.Load(dk); ok {
		if utctime.Since(t.(time.Time)) < upgradeDedupWindow {
			return
		}
	}
	lease, leaseOK := acquireAutoTCILease(ctx, fmt.Sprintf("ubc castle=%d", castleAID), 20*time.Second)
	if !leaseOK {
		return
	}
	defer lease.Release()
	if !ensureClientFocusedOnCastleWithLease(lease, gs, c, castleAID) {
		return
	}
	kid := autoTCICastleKID(gs, c, castleAID)
	aid := castleAID
	Logging.AutoTCILogf("ubc upgrade",
		"AID=%d OID=%d slot=%d KID=%d currentCID=%d nextCID=%d (next tier %d, cap %d) SUC=%d RS=%d",
		castleAID, b.OID, sl.S, kid, sl.CID, nextCID, nextTier, ceiling, suc, derefInt(sl.RemainingSec))
	if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("ubc skipped AID=%d", castleAID)) {
		return
	}
	GameCommands.SendUBCAutoTCI(b.OID, suc, sl.S, kid, aid, sl.CID, lease)
	upgradeLastSent.Store(dk, utctime.Now())
}

func autoTCIUBCSUCForTargetTier(targetTier int) (int, bool) {
	switch targetTier {
	case 2:
		return autoTCIUBCRareBooster, true
	case 3:
		return autoTCIUBCEpicBooster, true
	case 4:
		return autoTCIUBCLegendaryBooster, true
	default:
		return 0, false
	}
}

func catalogEntryForWireCID(catalog []ConstructionItemCatalogEntry, wireCID int) *ConstructionItemCatalogEntry {
	if wireCID <= 0 {
		return nil
	}
	for i := range catalog {
		for _, g := range catalog[i].GroupIDs {
			if g == wireCID {
				return &catalog[i]
			}
		}
	}
	return nil
}

func resolveAutoTCIEntry(byID map[int]ConstructionItemCatalogEntry, catalog []ConstructionItemCatalogEntry, key int) (ConstructionItemCatalogEntry, bool) {
	if e, ok := byID[key]; ok {
		return e, true
	}
	if ce := catalogEntryForWireCID(catalog, key); ce != nil {
		return *ce, true
	}
	return ConstructionItemCatalogEntry{}, false
}

func levelTargetForAutoTCIEntry(per map[int]stsettings.AutoTCILevelTarget, entry *ConstructionItemCatalogEntry) (stsettings.AutoTCILevelTarget, bool) {
	if entry == nil || len(per) == 0 {
		return stsettings.AutoTCILevelTarget{}, false
	}
	if t, ok := per[entry.ID]; ok && t.MaxLevel >= 1 {
		return t.Normalize(), true
	}
	for _, wid := range entry.GroupIDs {
		if t, ok := per[wid]; ok && t.MaxLevel >= 1 {
			return t.Normalize(), true
		}
	}
	return stsettings.AutoTCILevelTarget{}, false
}

func levelRangeForAutoTCIEntry(per map[int]stsettings.AutoTCILevelTarget, entry *ConstructionItemCatalogEntry) (minLevel, maxLevel int, ok bool) {
	t, ok := levelTargetForAutoTCIEntry(per, entry)
	if !ok {
		return 0, 0, false
	}
	n := t.Normalize()
	return n.MinLevel, n.MaxLevel, true
}

func tierLevelForWireCID(entry *ConstructionItemCatalogEntry, wireCID int) int {
	if entry == nil || wireCID <= 0 {
		return 0
	}
	for _, gt := range entry.GroupTiers {
		if gt.WireCID == wireCID {
			return gt.Level
		}
	}
	if lvl, ok := GameParser.ConstructionItemLevelByCID(wireCID); ok {
		return lvl
	}
	return 0
}

func wireCIDForTierLevel(entry *ConstructionItemCatalogEntry, tier1Based int) int {
	if entry == nil || tier1Based < 1 {
		return 0
	}
	for _, gt := range entry.GroupTiers {
		if gt.Level == tier1Based {
			return gt.WireCID
		}
	}
	sorted := sortCIDsByConstructionLevel(entry.GroupIDs)
	if tier1Based-1 < len(sorted) {
		return sorted[tier1Based-1]
	}
	return 0
}

func ciStashCountInLevelRange(gs *Models.GameState, entry *ConstructionItemCatalogEntry, minLevel, maxLevel int) int {
	if gs == nil || entry == nil {
		return 0
	}
	total := 0
	for tier := minLevel; tier <= maxLevel; tier++ {
		cid := wireCIDForTierLevel(entry, tier)
		if cid <= 0 {
			continue
		}
		total += ciStashCountForFirstTierWireCID(gs, cid)
	}
	return total
}

// bestEquipWireCIDInLevelRange picks the lowest tier in [minLevel, maxLevel] with stash >= 1
// so a lower tier can be equipped and ubc-upgraded (utilize upgrade timers) before using a max-tier item.
func bestEquipWireCIDInLevelRange(gs *Models.GameState, entry *ConstructionItemCatalogEntry, minLevel, maxLevel int) (wireCID, stash int) {
	if gs == nil || entry == nil {
		return 0, 0
	}
	for tier := minLevel; tier <= maxLevel; tier++ {
		cid := wireCIDForTierLevel(entry, tier)
		if cid <= 0 {
			continue
		}
		n := ciStashCountForFirstTierWireCID(gs, cid)
		if n > 0 {
			return cid, n
		}
	}
	return 0, 0
}

func firstTierWireCIDForEntry(entry *ConstructionItemCatalogEntry) int {
	if entry == nil || len(entry.GroupIDs) == 0 {
		return 0
	}
	if len(entry.GroupTiers) > 0 {
		return entry.GroupTiers[0].WireCID
	}
	return sortCIDsByConstructionLevel(entry.GroupIDs)[0]
}

func castleHasConstructionGroup(c *castle.PlayerCastleInfo, catalog []ConstructionItemCatalogEntry, entry *ConstructionItemCatalogEntry) bool {
	if c == nil || entry == nil {
		return false
	}
	for _, b := range c.ConstructionByBuilding {
		for j := range b.Slots {
			ce := catalogEntryForWireCID(catalog, b.Slots[j].CID)
			if ce != nil && ce.ID == entry.ID {
				return true
			}
		}
	}
	return false
}

func ciStashCountForFirstTierWireCID(gs *Models.GameState, firstWireCID int) int {
	if gs == nil || firstWireCID <= 0 {
		return 0
	}
	n := gs.CIInventoryCountForCID(firstWireCID)
	if t, ok := GameParser.TrivialCIPurchaseInfoForCid(firstWireCID); ok && t.PID > 0 {
		if m := gs.CIInventoryCountForCID(t.PID); m > n {
			n = m
		}
	}
	return n
}

func pickConstructionHostOIDForEquip(c *castle.PlayerCastleInfo, catalog []ConstructionItemCatalogEntry, entry *ConstructionItemCatalogEntry) (oid int, hint string) {
	if c == nil || entry == nil || len(c.ConstructionByBuilding) == 0 {
		return 0, "no_ci_rows"
	}
	firstCID := firstTierWireCIDForEntry(entry)
	allowed, filterActive, filterSource := allowedHostOIDsForEquip(c, firstCID)
	for _, b := range c.ConstructionByBuilding {
		if b.OID <= 0 {
			continue
		}
		for _, sl := range b.Slots {
			if wireCIDBelongsToCatalogEntry(catalog, entry, sl.CID) {
				return b.OID, "existing_slot_same_group"
			}
		}
	}
	if filterActive && len(allowed) == 0 {
		return 0, "no_building_for_" + filterSource
	}
	matchesAllowed := func(oid int) bool {
		if !filterActive {
			return true
		}
		_, ok := allowed[oid]
		return ok
	}
	var empty []int
	for _, b := range c.ConstructionByBuilding {
		if b.OID <= 0 || len(b.Slots) != 0 || !matchesAllowed(b.OID) {
			continue
		}
		empty = append(empty, b.OID)
	}
	sort.Ints(empty)
	if len(empty) > 0 {
		return empty[0], "empty_cil_row"
	}
	if filterActive {
		ciOIDs := constructionCIBuildingOIDs(c)
		var unused []int
		for oid := range allowed {
			if _, inCI := ciOIDs[oid]; inCI {
				continue
			}
			unused = append(unused, oid)
		}
		sort.Ints(unused)
		if len(unused) > 0 {
			return unused[0], "wod_host_no_ci_row"
		}
	}
	var eligible []int
	for _, b := range c.ConstructionByBuilding {
		if b.OID <= 0 || !matchesAllowed(b.OID) {
			continue
		}
		eligible = append(eligible, b.OID)
	}
	sort.Ints(eligible)
	if len(eligible) == 1 {
		return eligible[0], "sole_non_foreign_ci_host"
	}
	if len(eligible) > 1 {
		return eligible[0], "min_oid_non_foreign_ci_host"
	}
	if filterActive {
		return 0, "no_oid_for_" + filterSource
	}
	return 0, "no_eligible_oid"
}

func allowedHostOIDsForEquip(c *castle.PlayerCastleInfo, firstWireCID int) (allowed map[int]struct{}, filterActive bool, source string) {
	if firstWireCID <= 0 {
		return nil, false, "none"
	}
	if wods, ok := GameParser.BuildingWodIDsForConstructionWireCID(firstWireCID); ok && len(wods) > 0 {
		meta, _ := GameParser.ConstructionItemMetaByCID(firstWireCID)
		src := "group_wod"
		if meta.ConstructionItemGroupID > 0 {
			src = fmt.Sprintf("group_%d_wod", meta.ConstructionItemGroupID)
		}
		return hostOIDsForAllowedWodIDs(c, wods), true, src
	}
	keywords := constructionItemHostBuildingKeywords(firstWireCID)
	if len(keywords) > 0 {
		return hostOIDsForBuildingKeywords(c, keywords), true, "keyword_fallback"
	}
	return nil, false, "none"
}

func hostOIDsForAllowedWodIDs(c *castle.PlayerCastleInfo, allowedWods map[int]struct{}) map[int]struct{} {
	out := make(map[int]struct{})
	if c == nil || len(allowedWods) == 0 {
		return out
	}
	for _, b := range c.AllBuildingRows() {
		if b.OID <= 0 {
			continue
		}
		if _, ok := allowedWods[b.BuildingID]; ok {
			out[b.OID] = struct{}{}
		}
	}
	return out
}

func buildingNameForOID(c *castle.PlayerCastleInfo, oid int) string {
	if c == nil || oid <= 0 {
		return "?"
	}
	for _, b := range c.AllBuildingRows() {
		if b.OID == oid {
			if b.Name != "" {
				return b.Name
			}
			return fmt.Sprintf("wodID=%d", b.BuildingID)
		}
	}
	return "?"
}

func hostOIDsForBuildingKeywords(c *castle.PlayerCastleInfo, keywords []string) map[int]struct{} {
	out := make(map[int]struct{})
	if c == nil {
		return out
	}
	for _, b := range c.AllBuildingRows() {
		if b.OID <= 0 {
			continue
		}
		if !buildingNameMatchesHostKeywords(b.Name, keywords) {
			continue
		}
		out[b.OID] = struct{}{}
	}
	return out
}

func buildingNameMatchesHostKeywords(buildingName string, keywords []string) bool {
	if len(keywords) == 0 {
		return true
	}
	bn := strings.ToLower(strings.TrimSpace(buildingName))
	if bn == "" || bn == "unknown" {
		return false
	}
	for _, kw := range keywords {
		if strings.Contains(bn, strings.ToLower(kw)) {
			return true
		}
	}
	return false
}

func constructionItemHostBuildingKeywords(firstWireCID int) []string {
	m, ok := GameParser.ConstructionItemMetaByCID(firstWireCID)
	if !ok {
		return nil
	}
	switch m.LockRemoval {
	case "SOLDIER_RECRUITMENT":
		return []string{"barracks"}
	case "TOOL_PRODUCTION":
		return []string{"workshop"}
	}
	if m.SlotType == 2 {
		return relicHostBuildingKeywords(m.Name, m.Comment2)
	}
	return inferHostBuildingKeywords(m.Name, m.Comment2)
}

func relicHostBuildingKeywords(name, comment2 string) []string {
	blob := strings.ToLower(name + " " + comment2)
	switch {
	case strings.Contains(blob, "woodcutter"):
		return []string{"relic woodcutter", "woodcutter"}
	case strings.Contains(blob, "quarry"):
		return []string{"relic quarry", "quarry", "stone quarry"}
	case strings.Contains(blob, "mine"):
		return []string{"relic mine", "mine", "iron mine"}
	case strings.Contains(blob, "farmstead"):
		return []string{"relic farmstead", "farmstead", "farmhouse"}
	case strings.Contains(blob, "mill"):
		return []string{"relic mill", "mill"}
	}
	return inferHostBuildingKeywords(name, comment2)
}

func inferHostBuildingKeywords(name, comment2 string) []string {
	blob := strings.ToLower(name + " " + comment2)
	type pair struct{ needle, host string }
	rules := []pair{
		{"barrel workshop", "barrel workshop"},
		{"bakery", "bakery"},
		{"barracks", "barracks"},
		{"workshop", "workshop"},
		{"woodcutter", "woodcutter"},
		{"quarry", "quarry"},
		{"mine", "mine"},
		{"farmstead", "farmstead"},
		{"farmhouse", "farmhouse"},
		{"mill", "mill"},
		{"hospital", "hospital"},
		{"stonemason", "stonemason"},
		{"keep", "keep"},
		{"stables", "stables"},
		{"storehouse", "storehouse"},
		{"storage", "storage"},
		{"dwelling", "dwelling"},
	}
	var out []string
	seen := make(map[string]struct{})
	for _, r := range rules {
		if !strings.Contains(blob, r.needle) {
			continue
		}
		if _, ok := seen[r.host]; ok {
			continue
		}
		seen[r.host] = struct{}{}
		out = append(out, r.host)
	}
	return out
}

func wireCIDBelongsToCatalogEntry(catalog []ConstructionItemCatalogEntry, entry *ConstructionItemCatalogEntry, wireCID int) bool {
	if entry == nil || wireCID <= 0 {
		return false
	}
	ce := catalogEntryForWireCID(catalog, wireCID)
	return ce != nil && ce.ID == entry.ID
}

func constructionCIBuildingOIDs(c *castle.PlayerCastleInfo) map[int]struct{} {
	out := make(map[int]struct{})
	if c == nil {
		return out
	}
	for _, b := range c.ConstructionByBuilding {
		if b.OID > 0 {
			out[b.OID] = struct{}{}
		}
	}
	return out
}

func ensureClientFocusedOnCastle(gs *Models.GameState, c *castle.PlayerCastleInfo, castleAID int, forceRefresh ...bool) bool {
	return ensureClientFocusedOnCastleWithLease(nil, gs, c, castleAID, forceRefresh...)
}

func ensureClientFocusedOnCastleWithLease(lease *GameFocus.Lease, gs *Models.GameState, c *castle.PlayerCastleInfo, castleAID int, forceRefresh ...bool) bool {
	if gs == nil || c == nil || castleAID <= 0 {
		return false
	}
	force := len(forceRefresh) > 0 && forceRefresh[0]
	if int(c.Aid) > 0 && int(c.Aid) != castleAID {
		castleAID = int(c.Aid)
	}
	if !force && gs.CastleFocus.CastleAID == castleAID && len(c.ConstructionByBuilding) > 0 {
		Logging.AutoTCILogf("focus", "reuse cached CI AID=%d (no jaa sent)", castleAID)
		return true
	}
	kid, x, y, ok := resolveAutoTCICastleMapCoords(gs, castleAID, c)
	if !ok {
		Logging.AutoTCILogf("focus", "skip: no map coords AID=%d", castleAID)
		return false
	}
	tryFocus := func(kingdomID int) bool {
		prev := GameParser.JAAProcessedSnapshot()
		Logging.AutoTCILogf("focus", "send jaa/jca AID=%d KID=%d X=%d Y=%d", castleAID, kingdomID, x, y)
		if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("focus skipped AID=%d", castleAID)) {
			return false
		}
		GameCommands.SendCastleFocusAutoTCI(kingdomID, castleAID, x, y, lease)
		if !GameParser.AwaitJAAProcessedAfter(prev, autoTCIJAAFocusWait) {
			return false
		}
		if !autoTCILeaseActive(lease, "lease revoked", fmt.Sprintf("focus wait AID=%d", castleAID)) {
			return false
		}
		return gs.CastleFocus.CastleAID == castleAID
	}
	if tryFocus(kid) {
		return true
	}
	if kid == 1 || kid == 2 {
		swapped := 2
		if kid == 2 {
			swapped = 1
		}
		if tryFocus(swapped) {
			return true
		}
	}
	Logging.AutoTCILogf("focus", "jaa timeout AID=%d KID=%d", castleAID, kid)
	return false
}

// resolveCastleForAutoTCISettings finds a castle slot or alliance stub for a settings AID key.
func resolveCastleForAutoTCISettings(gs *Models.GameState, settingsKey int) (c *castle.PlayerCastleInfo, castleAID int, ok bool) {
	if gs == nil || settingsKey <= 0 {
		return nil, 0, false
	}
	if c = gs.GetCastleByID(settingsKey); c != nil {
		return c, autoTCICastleAID(c, settingsKey), true
	}
	for _, slot := range []*castle.PlayerCastleInfo{
		&gs.Castle.MainCastle,
		&gs.Castle.Outpost1,
		&gs.Castle.Outpost2,
		&gs.Castle.Outpost3,
		&gs.Castle.IceCastle,
		&gs.Castle.DesertCastle,
		&gs.Castle.DungeonCastle,
		&gs.Castle.StormCastle,
		&gs.Castle.BeriWorldCastle,
		&gs.Castle.Metropolis,
		&gs.Castle.Capital,
	} {
		if slot == nil || int(slot.Aid) != settingsKey {
			continue
		}
		return slot, settingsKey, true
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID != settingsKey {
			continue
		}
		stub := &castle.PlayerCastleInfo{
			Aid:          float64(settingsKey),
			MapKingdomID: loc.KingdomID,
			MapX:         loc.X,
			MapY:         loc.Y,
		}
		return stub, settingsKey, loc.X != 0 || loc.Y != 0 || loc.KingdomID != 0
	}
	return nil, 0, false
}

// autoTCICastleAID is the player castle instance id (JAA gca / gbc CID / rpc AID), not a slot index.
func autoTCICastleAID(c *castle.PlayerCastleInfo, settingsKey int) int {
	if c == nil {
		return settingsKey
	}
	if aid := int(c.Aid); aid > 0 {
		return aid
	}
	return settingsKey
}

// resolveAutoTCICastleMapCoords returns kingdom + map tile for JAA/JCA focus (alliance list first, like AutoBird).
func resolveAutoTCICastleMapCoords(gs *Models.GameState, castleAID int, c *castle.PlayerCastleInfo) (kingdomID, x, y int, ok bool) {
	if gs == nil || c == nil || castleAID <= 0 {
		return 0, 0, 0, false
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleAID && (loc.X != 0 || loc.Y != 0) {
			return loc.KingdomID, loc.X, loc.Y, true
		}
	}
	if c.MapX != 0 || c.MapY != 0 {
		return c.MapKingdomID, c.MapX, c.MapY, true
	}
	t := c.Troops
	if t.X != 0 || t.Y != 0 {
		return t.KingdomID, t.X, t.Y, true
	}
	x, y, found := gs.ResolveCastleMapCoords(castleAID, c.MapKingdomID)
	if !found {
		return 0, 0, 0, false
	}
	kid := c.MapKingdomID
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID != castleAID {
			continue
		}
		if loc.X == x && loc.Y == y {
			return loc.KingdomID, x, y, true
		}
		if kid == 0 {
			kid = loc.KingdomID
		}
	}
	if kid != 0 {
		return kid, x, y, true
	}
	for _, loc := range gs.Alliance.PlayerCastleLocations {
		if loc.CastleID == castleAID {
			return loc.KingdomID, x, y, true
		}
	}
	return c.MapKingdomID, x, y, true
}

func autoTCICastleKID(gs *Models.GameState, c *castle.PlayerCastleInfo, castleAID int) int {
	if kid, _, _, ok := resolveAutoTCICastleMapCoords(gs, castleAID, c); ok && kid != 0 {
		return kid
	}
	if c != nil && c.MapKingdomID != 0 {
		return c.MapKingdomID
	}
	if c != nil && c.Troops.KingdomID != 0 {
		return c.Troops.KingdomID
	}
	return 0
}

func sortCIDsByConstructionLevel(cids []int) []int {
	out := append([]int(nil), cids...)
	sort.Slice(out, func(i, j int) bool {
		li, oi := GameParser.ConstructionItemLevelByCID(out[i])
		lj, oj := GameParser.ConstructionItemLevelByCID(out[j])
		if oi && oj && li != lj {
			return li < lj
		}
		if oi != oj {
			return oi && !oj
		}
		return out[i] < out[j]
	})
	return out
}

func nextTierIndexAfterUpgrade(entry *ConstructionItemCatalogEntry, currentCID int) (nextTier1Based int, ok bool) {
	nextTier, _, ok := nextTierAfterUpgrade(entry, currentCID)
	return nextTier, ok
}

func nextTierAfterUpgrade(entry *ConstructionItemCatalogEntry, currentCID int) (nextTier1Based, nextWireCID int, ok bool) {
	if entry == nil || currentCID <= 0 {
		return 0, 0, false
	}
	sorted := sortCIDsByConstructionLevel(entry.GroupIDs)
	cur := -1
	for i, id := range sorted {
		if id == currentCID {
			cur = i
			break
		}
	}
	if cur < 0 || cur+1 >= len(sorted) {
		return 0, 0, false
	}
	nextWireCID = sorted[cur+1]
	if lvl := tierLevelForWireCID(entry, nextWireCID); lvl > 0 {
		return lvl, nextWireCID, true
	}
	return cur + 2, nextWireCID, true
}

func slotReadyForUbcUpgrade(sl Models.GCAConstructionSlot) bool {
	if sl.RemainingSec == nil {
		return false
	}
	rs := *sl.RemainingSec
	return rs == 0 || (rs > 0 && rs <= autoTCIUpgradeTimerMaxSec)
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func sortedTargetCastleIDs(targets map[int]map[int]stsettings.AutoTCILevelTarget) []int {
	if len(targets) == 0 {
		return nil
	}
	ids := make([]int, 0, len(targets))
	for id := range targets {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	return ids
}

func sortedTargetKeysFromLevelMap(m map[int]stsettings.AutoTCILevelTarget) []int {
	if len(m) == 0 {
		return nil
	}
	keys := make([]int, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func autoTCILogLabel(r AutoTCITargetPresence) string {
	label := strings.TrimSpace(r.Entry.Label)
	if label == "" {
		return fmt.Sprintf("group=%d", r.GroupID)
	}
	fx := strings.TrimSpace(r.Entry.Effects)
	if fx == "" {
		return label
	}
	return label + " | " + fx
}

func presenceRowDedupKey(prefix string, r AutoTCITargetPresence) string {
	oid := r.HostOID
	if oid <= 0 {
		oid = 0
	}
	return fmt.Sprintf("%s:%d:%d:%d", prefix, r.CastleID, r.GroupID, oid)
}

func init() {
	ResponseRegistry.GameSessionInactiveHandler = onGameSessionInactiveForAutoTCI
}

func onGameSessionInactiveForAutoTCI() {
	if gs := Models.GetGameState(); gs != nil {
		gs.ReplaceCIInventoryCountsFromMap(nil)
	}
}

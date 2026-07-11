package settingsview

import (
	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameCommands"
	"CitadelDesktop/Server/GameParser"
	"CitadelDesktop/Server/Logging"
	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
	resources "CitadelDesktop/Server/Models/Resources"
	stsettings "CitadelDesktop/Server/Models/Settings"
	"CitadelDesktop/Server/ResponseRegistry"
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	autoSceatResFeatureID         = "autoSceatRes"
	autoSceatResCraftingWait      = 5 * time.Second
	autoSceatResTransportWait     = 5 * time.Second
	autoSceatResMarketWait        = 5 * time.Second
	autoSceatResRefreshSettle     = 900 * time.Millisecond
	autoSceatResActionDelay       = 100 * time.Millisecond
	autoSceatResKingdomDelivery   = 0.90
	AutoSceatResRentalActiveCoins = 5_000_000
)

var autoSceatResQueueRentalCoins = map[int]float64{
	1: 500_000,
	2: 3_000_000,
	3: 6_500_000,
}

var autoSceatResTimeSkipSeconds = map[string]int{
	"MS1": 60,
	"MS2": 300,
	"MS3": 600,
	"MS4": 1800,
	"MS5": 3600,
	"MS6": 18000,
	"MS7": 86400,
}

var autoSceatResResourceCodes = map[string]string{
	"wood": "W", "stone": "S", "coal": "C", "oil": "O", "glass": "G", "iron": "I",
}

var (
	autoSceatResMu      sync.Mutex
	autoSceatResCancel  context.CancelFunc
	autoSceatResRefresh = make(chan struct{}, 1)
)

// IsAutoSceatResRunning reports whether the automation loop is active.
func IsAutoSceatResRunning() bool {
	autoSceatResMu.Lock()
	defer autoSceatResMu.Unlock()
	return autoSceatResCancel != nil
}

// StartAutoSceatRes starts the crafting and resource-logistics loop.
func StartAutoSceatRes() {
	autoSceatResMu.Lock()
	defer autoSceatResMu.Unlock()
	if autoSceatResCancel != nil {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoSceatResCancel = cancel
	go runAutoSceatRes(ctx)
}

// StopAutoSceatRes stops the automation loop.
func StopAutoSceatRes() {
	autoSceatResMu.Lock()
	defer autoSceatResMu.Unlock()
	if autoSceatResCancel == nil {
		return
	}
	autoSceatResCancel()
	Automation.CancelOwner(Automation.OwnerAutoSceatRes)
	autoSceatResCancel = nil
	if ResponseRegistry.SendAutoSceatResStatusFunc != nil {
		go ResponseRegistry.SendAutoSceatResStatusFunc(false)
	}
}

// NotifyAutoSceatResSettingsChanged wakes a sleeping loop so new settings apply promptly.
func NotifyAutoSceatResSettingsChanged() {
	select {
	case autoSceatResRefresh <- struct{}{}:
	default:
	}
}

func autoSceatResPause(ctx context.Context, duration time.Duration) bool {
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

func autoSceatResSleep(ctx context.Context, duration time.Duration) bool {
	if duration <= 0 {
		duration = time.Second
	}
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-autoSceatResRefresh:
		return true
	case <-timer.C:
		return true
	}
}

func autoSceatResScheduledSleep(state *stsettings.SettingsState, now time.Time, fallback time.Duration) time.Duration {
	if state == nil || state.FeatureScheduleAllows(autoSceatResFeatureID, now) {
		return fallback
	}
	if next, ok := state.NextFeatureScheduleStart(autoSceatResFeatureID, now); ok {
		until := time.Until(next)
		if until > 0 && until < fallback {
			return until
		}
	}
	return fallback
}

func autoSceatResQueuePayload(payload string, leases ...*Automation.Lease) bool {
	Logging.AppendAutoSceatResSendPayload(payload)
	var lease *Automation.Lease
	if len(leases) > 0 {
		lease = leases[0]
	}
	return GameCommands.QueueFeaturePayload(Automation.OwnerAutoSceatRes, payload, lease)
}

func autoSceatResWaitForGeneration(ctx context.Context, previous uint64, current func() uint64, timeout time.Duration) bool {
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		if current() > previous {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-deadline.C:
			return false
		case <-ticker.C:
		}
	}
}

func autoSceatResSendCraftingAndWait(ctx context.Context, payload string) bool {
	err := Automation.RunWork(ctx, Automation.WorkItem{
		Request: Automation.Request{
			Owner:    Automation.OwnerAutoSceatRes,
			Priority: Automation.PriorityAutoSceat,
			Reason:   "crafting command",
			Claims: []Automation.Claim{
				Automation.ExclusiveClaim(Automation.ClaimCrafting),
				Automation.ExclusiveClaim(Automation.ClaimAccountResources),
			},
			MaxHold: autoSceatResCraftingWait + time.Second,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			previous := GameParser.CraftingUpdateGeneration()
			if !autoSceatResQueuePayload(payload, lease) {
				return Automation.ErrWorkCancelled
			}
			if !autoSceatResWaitForGeneration(workCtx, previous, GameParser.CraftingUpdateGeneration, autoSceatResCraftingWait) {
				return Automation.ErrWorkCancelled
			}
			return nil
		},
	})
	return err == nil
}

func autoSceatResSendTransportAndWait(ctx context.Context, payload string) bool {
	err := Automation.RunWork(ctx, Automation.WorkItem{
		Request: Automation.Request{
			Owner:    Automation.OwnerAutoSceatRes,
			Priority: Automation.PriorityAutoSceat,
			Reason:   "kingdom transport command",
			Claims: []Automation.Claim{
				Automation.ExclusiveClaim(Automation.ClaimTransport),
				Automation.ExclusiveClaim(Automation.ClaimAccountResources),
			},
			MaxHold: autoSceatResTransportWait + time.Second,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			previous := GameParser.KingdomTransportUpdateGeneration()
			if !autoSceatResQueuePayload(payload, lease) {
				return Automation.ErrWorkCancelled
			}
			if !autoSceatResWaitForGeneration(workCtx, previous, GameParser.KingdomTransportUpdateGeneration, autoSceatResTransportWait) {
				return Automation.ErrWorkCancelled
			}
			return nil
		},
	})
	return err == nil
}

func autoSceatResSendMarketAndWait(ctx context.Context, payload string) (int, bool) {
	remaining := 0
	err := Automation.RunWork(ctx, Automation.WorkItem{
		Request: Automation.Request{
			Owner:    Automation.OwnerAutoSceatRes,
			Priority: Automation.PriorityAutoSceat,
			Reason:   "market transport command",
			Claims: []Automation.Claim{
				Automation.ExclusiveClaim(Automation.ClaimTransport),
				Automation.ExclusiveClaim(Automation.ClaimAccountResources),
			},
			MaxHold: autoSceatResMarketWait + time.Second,
		},
		Run: func(workCtx context.Context, lease *Automation.Lease) error {
			waiter := ResponseRegistry.Global.RegisterWaiter("crm", autoSceatResMarketWait)
			defer waiter.Cleanup()
			if !autoSceatResQueuePayload(payload, lease) {
				return Automation.ErrWorkCancelled
			}
			response, waitErr := waiter.WaitWithContext(workCtx)
			if waitErr != nil {
				return waitErr
			}
			code, codeOK := GameParser.EmpireExResponseCode(response)
			if !codeOK || code != 0 {
				return fmt.Errorf("crm transport rejected")
			}
			responsePayload, payloadOK := GameParser.Payload(response)
			if payloadOK {
				remaining = GameParser.ExtractMaxTTSecondsFromGAMLikeJSON(responsePayload)
			}
			return nil
		},
	})
	return remaining, err == nil
}

func autoSceatResRefreshState(ctx context.Context, includeTransport bool) {
	autoSceatResQueuePayload(GameCommands.DCLRefreshPayload())
	previousCrafting := GameParser.CraftingUpdateGeneration()
	autoSceatResQueuePayload(GameCommands.CRINPayload())
	if includeTransport {
		previousTransport := GameParser.KingdomTransportUpdateGeneration()
		previousMarket := GameParser.MarketInfoUpdateGeneration()
		previousBooster := GameParser.MarketBoosterUpdateGeneration()
		autoSceatResQueuePayload(GameCommands.KPIPayload())
		autoSceatResQueuePayload(GameCommands.BOIPayload())
		autoSceatResQueuePayload(GameCommands.CMIPayload())
		_ = autoSceatResWaitForGeneration(ctx, previousTransport, GameParser.KingdomTransportUpdateGeneration, autoSceatResTransportWait)
		_ = autoSceatResWaitForGeneration(ctx, previousBooster, GameParser.MarketBoosterUpdateGeneration, autoSceatResTransportWait)
		_ = autoSceatResWaitForGeneration(ctx, previousMarket, GameParser.MarketInfoUpdateGeneration, autoSceatResTransportWait)
	}
	if !autoSceatResWaitForGeneration(ctx, previousCrafting, GameParser.CraftingUpdateGeneration, autoSceatResCraftingWait) {
		Logging.AutoSceatResLog("snapshot", "crafting refresh timed out; retaining last valid snapshot")
	}
	_ = autoSceatResPause(ctx, autoSceatResRefreshSettle)
}

type autoSceatResReservations map[string]float64

func autoSceatResReservationKey(castleID int, resource string) string {
	if autoSceatResResourceCodes[resource] != "" {
		return fmt.Sprintf("castle:%d:%s", castleID, resource)
	}
	return "global:" + resource
}

func autoSceatResLocalAmount(info *Models.PlayerCastleInfo, resource string) float64 {
	if info == nil {
		return 0
	}
	switch resource {
	case "wood":
		return info.Amount.WoodAmount
	case "stone":
		return info.Amount.StoneAmount
	case "coal":
		return info.Amount.CoalAmount
	case "oil":
		return info.Amount.OilAmount
	case "glass":
		return info.Amount.GlassAmount
	case "iron":
		return info.Amount.IronAmount
	default:
		return 0
	}
}

func autoSceatResLocalStorage(info *Models.PlayerCastleInfo, resource string) float64 {
	if info == nil {
		return 0
	}
	switch resource {
	case "wood":
		return info.Storage.WoodMax
	case "stone":
		return info.Storage.StoneMax
	case "coal":
		return info.Storage.CoalMax
	case "oil":
		return info.Storage.OilMax
	case "glass":
		return info.Storage.GlassMax
	case "iron":
		return info.Storage.IronMax
	default:
		return 0
	}
}

func autoSceatResGlobalAmount(global resources.PlayerGlobalResources, resource string) float64 {
	switch resource {
	case "rubies":
		return global.Rubies
	case "coins":
		return global.Coins
	case "sceatToken":
		return global.Sceat
	case "refinedLumber":
		return global.RefinedLumber
	case "refinedStone":
		return global.RefinedStone
	case "legendaryToken":
		return global.LegendaryToken
	case "legendaryMaterial":
		return global.UpgrToken
	case "steel":
		return global.Steel
	case "dragonGlass":
		return global.DragonGlass
	case "dragonCharm":
		return global.DragonCharm
	case "dragonScaleSplinters":
		return global.DrgSpl
	case "dragonScaleTile":
		return global.DrgScale
	case "dragonGlassArrows":
		return global.DrgGlassArrow
	case "dragonScaleArmor":
		return global.DrgScaleArmor
	case "dragonScaleArrows":
		return global.DrgScaleArrow
	case "twinFlameAxes":
		return global.TwinFlameAxes
	case "component1":
		return global.Component1
	case "component2":
		return global.Component2
	case "component3":
		return global.Component3
	case "component4":
		return global.Component4
	case "component5":
		return global.Component5
	case "component6":
		return global.Component6
	case "component7":
		return global.Component7
	case "component8":
		return global.Component8
	default:
		return 0
	}
}

type autoSceatResCostCheck struct {
	MissingLocal  map[string]float64
	MissingGlobal map[string]float64
	RubyBlocked   bool
}

func (check autoSceatResCostCheck) canAfford() bool {
	return !check.RubyBlocked && len(check.MissingLocal) == 0 && len(check.MissingGlobal) == 0
}

func autoSceatResCheckCosts(gs *Models.GameState, info *Models.PlayerCastleInfo, recipe AutoSceatRecipeCatalogEntry, cfg stsettings.AutoSceatResConfig, reservations autoSceatResReservations) autoSceatResCostCheck {
	check := autoSceatResCostCheck{MissingLocal: make(map[string]float64), MissingGlobal: make(map[string]float64)}
	castleID := 0
	if info != nil {
		castleID = int(info.Aid)
	}
	for resource, cost := range recipe.Costs {
		if cost <= 0 {
			continue
		}
		if resource == "rubies" && !cfg.AllowRubyRecipes {
			check.RubyBlocked = true
			continue
		}
		reserved := reservations[autoSceatResReservationKey(castleID, resource)]
		if autoSceatResResourceCodes[resource] != "" {
			available := autoSceatResLocalAmount(info, resource) - reserved
			if available < cost {
				check.MissingLocal[resource] = cost - math.Max(available, 0)
			}
			continue
		}
		available := autoSceatResGlobalAmount(gs.GlobalResources, resource) - reserved
		if resource == "coins" {
			available -= cfg.MinimumCoinReserve
		}
		if resource == "rubies" {
			available -= cfg.MinimumRubyReserve
		}
		if available < cost {
			check.MissingGlobal[resource] = cost - math.Max(available, 0)
		}
	}
	return check
}

func autoSceatResReserveRecipe(info *Models.PlayerCastleInfo, recipe AutoSceatRecipeCatalogEntry, reservations autoSceatResReservations) {
	castleID := int(info.Aid)
	for resource, cost := range recipe.Costs {
		reservations[autoSceatResReservationKey(castleID, resource)] += cost
	}
}

func autoSceatResBuilding(info *Models.PlayerCastleInfo, queueID int) (castle.CraftingBuildingSnapshot, bool) {
	if info == nil {
		return castle.CraftingBuildingSnapshot{}, false
	}
	for _, building := range info.CraftingQueues {
		if building.CQID == queueID {
			return building, true
		}
	}
	return castle.CraftingBuildingSnapshot{}, false
}

func autoSceatResOccupied(building castle.CraftingBuildingSnapshot) int {
	return len(building.PS.CRID) + len(building.QS.CRID)
}

func autoSceatResCapacity(building castle.CraftingBuildingSnapshot) int {
	return 2 + len(building.PS.RUT) + len(building.QS.RUT)
}

func autoSceatResRecipeAllowed(info *Models.PlayerCastleInfo, building castle.CraftingBuildingSnapshot, recipe AutoSceatRecipeCatalogEntry) bool {
	if info == nil {
		return false
	}
	return autoSceatRecipeAvailable(recipe, building, info.CraftingEntitlements)
}

func autoSceatResMissingSummary(check autoSceatResCostCheck) string {
	var parts []string
	if check.RubyBlocked {
		parts = append(parts, "ruby spending disabled")
	}
	keys := make([]string, 0, len(check.MissingLocal)+len(check.MissingGlobal))
	for key := range check.MissingLocal {
		keys = append(keys, key)
	}
	for key := range check.MissingGlobal {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		amount := check.MissingLocal[key]
		if amount == 0 {
			amount = check.MissingGlobal[key]
		}
		parts = append(parts, fmt.Sprintf("%s %.0f", key, amount))
	}
	return strings.Join(parts, ", ")
}

func autoSceatResRentNextSlot(ctx context.Context, cfg stsettings.AutoSceatResConfig, info *Models.PlayerCastleInfo, building castle.CraftingBuildingSnapshot, plan stsettings.AutoSceatBuildingPlan, reservations autoSceatResReservations) bool {
	activeRented := len(building.PS.RUT)
	queueRented := len(building.QS.RUT)
	desiredActive := 0
	if plan.AutoRentActiveSlot {
		desiredActive = 1
	}
	desiredQueues := plan.AutoRentQueueSlots
	slotType := ""
	slot := 0
	cost := float64(0)
	if activeRented < desiredActive {
		slotType, slot, cost = "production", activeRented+1, AutoSceatResRentalActiveCoins
	} else if queueRented < desiredQueues {
		slotType, slot = "queue", queueRented+1
		cost = autoSceatResQueueRentalCoins[slot]
	}
	if slotType == "" || cost <= 0 {
		return false
	}
	coinKey := autoSceatResReservationKey(0, "coins")
	availableCoins := Models.GetGameState().GlobalResources.Coins - reservations[coinKey] - cfg.MinimumCoinReserve
	if availableCoins < cost {
		Logging.AutoSceatResLogf("rent_skip", "castle=%d queue=%d %s slot=%d needs %.0f coins (available above reserve %.0f)", int(info.Aid), building.CQID, slotType, slot, cost, math.Max(availableCoins, 0))
		return false
	}

	payload := GameCommands.CRUNPayload(building.KID, building.AID, building.OID, slot, slotType)
	Logging.AutoSceatResLogf("rent", "castle=%d queue=%d %s slot=%d cost=%.0f", building.AID, building.CQID, slotType, slot, cost)
	if !autoSceatResSendCraftingAndWait(ctx, payload) {
		Logging.AutoSceatResLogf("rent_timeout", "castle=%d queue=%d", building.AID, building.CQID)
		return false
	}
	reservations[coinKey] += cost
	return true
}

func autoSceatResAdvancePlan(cfg *stsettings.AutoSceatResConfig, castleID, queueID int) {
	castlePlan := cfg.Castles[castleID]
	buildingPlan := castlePlan.Buildings[queueID]
	if cycleLength := buildingPlan.CycleLength(); cycleLength > 0 {
		buildingPlan.Cursor = (buildingPlan.Cursor + 1) % cycleLength
	}
	castlePlan.Buildings[queueID] = buildingPlan
	cfg.Castles[castleID] = castlePlan
}

func autoSceatResPendingForKingdom(state gamestate.KingdomTransportState, kingdomID int) (gamestate.KingdomResourceTransport, bool) {
	for _, pending := range state.Pending {
		if pending.KingdomID == kingdomID {
			return pending, true
		}
	}
	return gamestate.KingdomResourceTransport{}, false
}

func autoSceatResUnlockAllows(state gamestate.KingdomTransportState, kingdomID int) bool {
	for _, unlock := range state.Unlocks {
		if unlock.KingdomID == kingdomID {
			return unlock.Unlocked > 0
		}
	}
	return false
}

func autoSceatResIncomingAmount(pending gamestate.KingdomResourceTransport, resource string) float64 {
	code := autoSceatResResourceCodes[resource]
	for _, good := range pending.Goods {
		if good.Resource == code {
			return good.Amount
		}
	}
	return 0
}

func autoSceatResProtectedDemand(cfg stsettings.AutoSceatResConfig, node AutoSceatStorageNode, resource string) float64 {
	castlePlan, ok := cfg.Castles[node.CastleID]
	if !ok {
		return 0
	}
	protected := float64(0)
	for queueID, plan := range castlePlan.Buildings {
		if !plan.Enabled {
			continue
		}
		recipeID, ok := plan.RecipeAtCursor()
		if !ok {
			continue
		}
		recipe, found := AutoSceatRecipeByID(recipeID)
		if !found || recipe.QueueTypeID != queueID {
			continue
		}
		protected += recipe.Costs[resource]
	}
	return protected
}

func autoSceatResSelectTransportSource(cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, target AutoSceatStorageNode, resource string, reservations autoSceatResReservations) (AutoSceatStorageNode, float64, bool) {
	bestAvailable := float64(0)
	best := AutoSceatStorageNode{}
	preferredKingdom := map[string]int{"coal": 2, "oil": 1, "glass": 3}[resource]
	preferredFound := false
	for _, node := range catalog.Nodes {
		if node.CastleID == target.CastleID || node.KingdomID == target.KingdomID {
			continue
		}
		// Any owned storage node may source a kingdom transfer; Storm remains explicitly opt-in.
		if node.StormBuffer && !cfg.UseStormBuffer {
			continue
		}
		info := Models.GetGameState().GetCastleByID(node.CastleID)
		if info == nil {
			continue
		}
		onHand := autoSceatResLocalAmount(info, resource)
		storageReserve := autoSceatResLocalStorage(info, resource) * float64(cfg.SourceReservePercent) / 100
		protected := math.Max(storageReserve, autoSceatResProtectedDemand(cfg, node, resource))
		available := onHand - protected - reservations[autoSceatResReservationKey(node.CastleID, resource)]
		preferred := preferredKingdom > 0 && node.KingdomID == preferredKingdom
		if preferred && !preferredFound && available > 0 {
			preferredFound = true
			bestAvailable, best = available, node
			continue
		}
		if preferredFound && !preferred {
			continue
		}
		if available > bestAvailable {
			bestAvailable, best = available, node
		}
	}
	return best, bestAvailable, bestAvailable > 0
}

func autoSceatResMarketIncoming(state gamestate.MarketTransportState, target AutoSceatStorageNode, resource string) float64 {
	sessionIncoming := float64(0)
	for _, pending := range state.Pending {
		if pending.TargetCastleID == target.CastleID && pending.Resource == resource {
			sessionIncoming += pending.Amount
		}
	}

	resourceCode := autoSceatResResourceCodes[resource]
	if resourceCode == "" {
		return sessionIncoming
	}
	gs := Models.GetGameState()
	targetX, targetY, found := gs.ResolveCastleMapCoords(target.CastleID, target.KingdomID)
	if !found {
		return sessionIncoming
	}
	movements, _, _, _ := gs.Movement.Snapshot()
	nowUnix := time.Now().Unix()
	activeIncoming := float64(0)
	for _, movement := range movements {
		if movement.D != 0 || movement.KID != target.KingdomID || movement.TT <= 0 || movement.EffectivePT(nowUnix) >= movement.TT {
			continue
		}
		if movement.TargetX != targetX || movement.TargetY != targetY {
			continue
		}
		for _, good := range movement.MarketGoods {
			if good.Resource == resourceCode {
				activeIncoming += good.Amount
			}
		}
	}
	// A newly accepted crm is represented in both collections. Use the larger view so it is not
	// double-counted while still retaining the in-session fallback until the next gam snapshot.
	return math.Max(sessionIncoming, activeIncoming)
}

func autoSceatResSelectMarketSource(cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, target AutoSceatStorageNode, resource string, reservations autoSceatResReservations) (AutoSceatStorageNode, float64, bool) {
	best := AutoSceatStorageNode{}
	bestAvailable := float64(0)
	for _, node := range catalog.Nodes {
		if node.CastleID == target.CastleID || node.KingdomID != target.KingdomID || node.Market == nil || !node.Market.Loaded {
			continue
		}
		if node.Market.AvailableBarrows <= 0 || node.Market.AvailableShipmentCapacity <= 0 {
			continue
		}
		info := Models.GetGameState().GetCastleByID(node.CastleID)
		if info == nil {
			continue
		}
		onHand := autoSceatResLocalAmount(info, resource)
		storageReserve := autoSceatResLocalStorage(info, resource) * float64(cfg.SourceReservePercent) / 100
		protected := math.Max(storageReserve, autoSceatResProtectedDemand(cfg, node, resource))
		available := onHand - protected - reservations[autoSceatResReservationKey(node.CastleID, resource)]
		available = math.Min(available, float64(node.Market.AvailableShipmentCapacity))
		if available > bestAvailable {
			best, bestAvailable = node, available
		}
	}
	return best, bestAvailable, bestAvailable > 0
}

func autoSceatResMarketDistance(source, target *Models.PlayerCastleInfo) float64 {
	if source == nil || target == nil {
		return 0
	}
	dx := float64(source.MapX - target.MapX)
	dy := float64(source.MapY - target.MapY)
	return math.Round(math.Hypot(dx, dy)*10) / 10
}

func autoSceatResMarketCoinCost(distance float64, barrows int) float64 {
	if distance <= 0 || barrows <= 0 {
		return 0
	}
	return math.Ceil(3 * float64(barrows) * math.Log(distance+1) / math.Log(2.3))
}

func autoSceatResTryMarketTransport(ctx context.Context, cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, target AutoSceatStorageNode, missing map[string]float64, reservations autoSceatResReservations, transportActionTaken *bool) bool {
	if !cfg.AutoKingdomTransport || len(missing) == 0 || *transportActionTaken {
		return false
	}
	targetInfo := Models.GetGameState().GetCastleByID(target.CastleID)
	if targetInfo == nil {
		return false
	}
	marketState := Models.GetGameState().MarketTransportSnapshot()
	resourcesToTry := make([]string, 0, len(missing))
	for resource := range missing {
		if autoSceatResResourceCodes[resource] != "" {
			resourcesToTry = append(resourcesToTry, resource)
		}
	}
	sort.Strings(resourcesToTry)
	for _, resource := range resourcesToTry {
		incoming := autoSceatResMarketIncoming(marketState, target, resource)
		shortfall := math.Max(0, missing[resource]-incoming)
		if shortfall <= 0 {
			Logging.AutoSceatResLogf("market_wait", "castle=%d %s in-flight market shipment covers %.0f", target.CastleID, resource, missing[resource])
			return true
		}
		source, available, found := autoSceatResSelectMarketSource(cfg, catalog, target, resource, reservations)
		if !found || source.Market == nil {
			continue
		}
		destinationFree := autoSceatResLocalStorage(targetInfo, resource) - autoSceatResLocalAmount(targetInfo, resource) - incoming
		if destinationFree <= 0 {
			continue
		}
		amount := shortfall
		minimum := float64(cfg.MinimumShipmentSize)
		if amount < minimum {
			amount = minimum
		}
		amount = math.Floor(math.Min(amount, math.Min(available, destinationFree)))
		if amount <= 0 || (minimum > 0 && amount < minimum) {
			continue
		}
		barrows := int(math.Ceil(amount / float64(source.Market.CapacityPerBarrow)))
		sourceInfo := Models.GetGameState().GetCastleByID(source.CastleID)
		coinCost := autoSceatResMarketCoinCost(autoSceatResMarketDistance(sourceInfo, targetInfo), barrows)
		coinKey := autoSceatResReservationKey(0, "coins")
		availableCoins := Models.GetGameState().GlobalResources.Coins - reservations[coinKey] - cfg.MinimumCoinReserve
		if coinCost > availableCoins {
			Logging.AutoSceatResLogf("market_skip", "source=%d target=%d needs %.0f coins above reserve; available %.0f", source.CastleID, target.CastleID, coinCost, math.Max(availableCoins, 0))
			continue
		}

		payload := GameCommands.CRMPayload(source.KingdomID, source.CastleID, targetInfo.MapX, targetInfo.MapY, autoSceatResResourceCodes[resource], int(amount))
		Logging.AutoSceatResLogf("market", "source=%d target=%d kid=%d %s=%.0f barrows=%d/%d capacity=%d each", source.CastleID, target.CastleID, target.KingdomID, resource, amount, barrows, source.Market.AvailableBarrows, source.Market.CapacityPerBarrow)
		travelSec, accepted := autoSceatResSendMarketAndWait(ctx, payload)
		if !accepted {
			Logging.AutoSceatResLogf("market_rejected", "source=%d target=%d resource=%s", source.CastleID, target.CastleID, resource)
			return false
		}
		if travelSec <= 0 {
			travelSec = int(math.Max(300, float64(cfg.CheckIntervalSec)))
		}
		Models.GetGameState().RecordMarketShipment(gamestate.MarketPendingShipment{
			SourceCastleID: source.CastleID,
			TargetCastleID: target.CastleID,
			Resource:       resource,
			Amount:         amount,
			ArrivesAtUnix:  time.Now().Unix() + int64(travelSec),
		})
		reservations[autoSceatResReservationKey(source.CastleID, resource)] += amount
		reservations[coinKey] += coinCost
		*transportActionTaken = true
		autoSceatResQueuePayload(GameCommands.CMIPayload())
		return true
	}
	return false
}

func autoSceatResTimeSkipAmount(global resources.PlayerGlobalResources, id string) float64 {
	switch id {
	case "MS1":
		return global.Min1
	case "MS2":
		return global.Min5
	case "MS3":
		return global.Min10
	case "MS4":
		return global.Min30
	case "MS5":
		return global.Hr1
	case "MS6":
		return global.Hr5
	case "MS7":
		return global.Hr24
	default:
		return 0
	}
}

func autoSceatResChooseTimeSkip(cfg stsettings.AutoSceatResConfig, remainingSec int, reservations autoSceatResReservations) string {
	if !cfg.UseKingdomTimeSkips || remainingSec <= 0 {
		return ""
	}
	type candidate struct {
		id      string
		seconds int
	}
	var covering, partial []candidate
	for _, id := range cfg.AllowedTimeSkips {
		seconds := autoSceatResTimeSkipSeconds[id]
		if seconds <= 0 {
			continue
		}
		reserve := float64(cfg.TimeSkipReserve[id])
		available := autoSceatResTimeSkipAmount(Models.GetGameState().GlobalResources, id) - reservations["skip:"+id]
		if available <= reserve {
			continue
		}
		entry := candidate{id: id, seconds: seconds}
		if seconds >= remainingSec {
			covering = append(covering, entry)
		} else {
			partial = append(partial, entry)
		}
	}
	if len(covering) > 0 {
		sort.Slice(covering, func(i, j int) bool { return covering[i].seconds < covering[j].seconds })
		return covering[0].id
	}
	if len(partial) > 0 {
		sort.Slice(partial, func(i, j int) bool { return partial[i].seconds > partial[j].seconds })
		return partial[0].id
	}
	return ""
}

func autoSceatResTryPendingTimeSkip(ctx context.Context, cfg stsettings.AutoSceatResConfig, reservations autoSceatResReservations, transportActionTaken *bool) bool {
	if !cfg.AutoKingdomTransport || !cfg.UseKingdomTimeSkips || *transportActionTaken {
		return false
	}
	pending := append([]gamestate.KingdomResourceTransport(nil), Models.GetGameState().KingdomTransportSnapshot().Pending...)
	sort.Slice(pending, func(i, j int) bool { return pending[i].RemainingSec < pending[j].RemainingSec })
	for _, shipment := range pending {
		skipID := autoSceatResChooseTimeSkip(cfg, shipment.RemainingSec, reservations)
		if skipID == "" {
			continue
		}
		Logging.AutoSceatResLogf("transport_skip_time", "kingdom=%d skip=%s remaining=%ds", shipment.KingdomID, skipID, shipment.RemainingSec)
		if !autoSceatResSendTransportAndWait(ctx, GameCommands.KingdomResourceMSKPayload(skipID, shipment.KingdomID)) {
			return false
		}
		reservations["skip:"+skipID]++
		*transportActionTaken = true
		return true
	}
	return false
}

func autoSceatResTryTransport(ctx context.Context, cfg stsettings.AutoSceatResConfig, targetCastleID int, missing map[string]float64, reservations autoSceatResReservations, transportActionTaken *bool) bool {
	if !cfg.AutoKingdomTransport || len(missing) == 0 || *transportActionTaken {
		return false
	}
	catalog, err := BuildAutoSceatResCatalog()
	if err != nil {
		Logging.AutoSceatResLogf("transport_skip", "catalog: %v", err)
		return false
	}
	var target AutoSceatStorageNode
	for _, node := range catalog.Nodes {
		if node.CastleID == targetCastleID {
			target = node
			break
		}
	}
	if target.CastleID == 0 || !target.CanCraft {
		return false
	}
	if autoSceatResTryMarketTransport(ctx, cfg, catalog, target, missing, reservations, transportActionTaken) {
		return true
	}
	transportState := Models.GetGameState().KingdomTransportSnapshot()
	if pending, found := autoSceatResPendingForKingdom(transportState, target.KingdomID); found {
		for resource, shortfall := range missing {
			if autoSceatResIncomingAmount(pending, resource) >= shortfall {
				Logging.AutoSceatResLogf("transport_wait", "castle=%d %s shipment already covers %.0f", targetCastleID, resource, shortfall)
				return true
			}
		}
		Logging.AutoSceatResLogf("transport_wait", "kingdom=%d already has a pending shipment", target.KingdomID)
		return true
	}
	if !autoSceatResUnlockAllows(transportState, target.KingdomID) {
		Logging.AutoSceatResLogf("transport_skip", "kingdom=%d transport is not unlocked", target.KingdomID)
		return false
	}

	resourcesToTry := make([]string, 0, len(missing))
	for resource := range missing {
		if autoSceatResResourceCodes[resource] != "" {
			resourcesToTry = append(resourcesToTry, resource)
		}
	}
	sort.Strings(resourcesToTry)
	for _, resource := range resourcesToTry {
		shortfall := missing[resource]
		source, available, found := autoSceatResSelectTransportSource(cfg, catalog, target, resource, reservations)
		if !found {
			continue
		}
		amount := math.Ceil(shortfall / autoSceatResKingdomDelivery)
		if amount < float64(cfg.MinimumShipmentSize) && available >= float64(cfg.MinimumShipmentSize) {
			amount = float64(cfg.MinimumShipmentSize)
		}
		amount = math.Min(amount, math.Floor(available))
		if amount <= 0 || (cfg.MinimumShipmentSize > 0 && amount < float64(cfg.MinimumShipmentSize)) || amount*autoSceatResKingdomDelivery < math.Min(shortfall, autoSceatResKingdomDelivery) {
			continue
		}
		payload := GameCommands.KGTPayload(source.CastleID, source.KingdomID, target.KingdomID, autoSceatResResourceCodes[resource], int(amount))
		Logging.AutoSceatResLogf("transport", "source=%d(kid=%d) target=%d(kid=%d) %s=%.0f protected-next-queue applied", source.CastleID, source.KingdomID, target.CastleID, target.KingdomID, resource, amount)
		if !autoSceatResSendTransportAndWait(ctx, payload) {
			Logging.AutoSceatResLogf("transport_timeout", "target kingdom=%d", target.KingdomID)
			return false
		}
		reservations[autoSceatResReservationKey(source.CastleID, resource)] += amount
		*transportActionTaken = true

		state := Models.GetGameState().KingdomTransportSnapshot()
		if pending, ok := autoSceatResPendingForKingdom(state, target.KingdomID); ok {
			if skipID := autoSceatResChooseTimeSkip(cfg, pending.RemainingSec, reservations); skipID != "" {
				Logging.AutoSceatResLogf("transport_skip_time", "kingdom=%d skip=%s remaining=%ds", target.KingdomID, skipID, pending.RemainingSec)
				if autoSceatResSendTransportAndWait(ctx, GameCommands.KingdomResourceMSKPayload(skipID, target.KingdomID)) {
					reservations["skip:"+skipID]++
				}
			}
		}
		return true
	}
	return false
}

func autoSceatResTryMarketOverflow(ctx context.Context, cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, reservations autoSceatResReservations, transportActionTaken *bool) bool {
	if !cfg.AutoKingdomTransport || *transportActionTaken {
		return false
	}
	type candidate struct {
		source   AutoSceatStorageNode
		target   AutoSceatStorageNode
		resource string
		amount   float64
	}
	best := candidate{}
	marketState := Models.GetGameState().MarketTransportSnapshot()
	minimum := float64(cfg.MinimumShipmentSize)
	for _, source := range catalog.Nodes {
		if source.Market == nil || !source.Market.Loaded || source.Market.AvailableBarrows <= 0 || source.Market.AvailableShipmentCapacity <= 0 {
			continue
		}
		sourceInfo := Models.GetGameState().GetCastleByID(source.CastleID)
		if sourceInfo == nil {
			continue
		}
		for _, resource := range []string{"coal", "oil", "glass", "iron"} {
			storage := autoSceatResLocalStorage(sourceInfo, resource)
			if storage <= 0 {
				continue
			}
			threshold := storage * float64(cfg.OverflowThresholdPercent) / 100
			leaveBehind := math.Max(threshold, autoSceatResProtectedDemand(cfg, source, resource))
			overflow := autoSceatResLocalAmount(sourceInfo, resource) - leaveBehind - reservations[autoSceatResReservationKey(source.CastleID, resource)]
			overflow = math.Min(overflow, float64(source.Market.AvailableShipmentCapacity))
			if overflow <= 0 {
				continue
			}
			for _, target := range catalog.Nodes {
				if target.CastleID == source.CastleID || target.KingdomID != source.KingdomID {
					continue
				}
				targetInfo := Models.GetGameState().GetCastleByID(target.CastleID)
				if targetInfo == nil {
					continue
				}
				incoming := autoSceatResMarketIncoming(marketState, target, resource)
				free := autoSceatResLocalStorage(targetInfo, resource) - autoSceatResLocalAmount(targetInfo, resource) - incoming
				amount := math.Floor(math.Min(overflow, free))
				if amount <= 0 || (minimum > 0 && amount < minimum) {
					continue
				}
				if amount > best.amount {
					best = candidate{source: source, target: target, resource: resource, amount: amount}
				}
			}
		}
	}
	if best.amount <= 0 || best.source.Market == nil {
		return false
	}
	sourceInfo := Models.GetGameState().GetCastleByID(best.source.CastleID)
	targetInfo := Models.GetGameState().GetCastleByID(best.target.CastleID)
	if sourceInfo == nil || targetInfo == nil {
		return false
	}
	barrows := int(math.Ceil(best.amount / float64(best.source.Market.CapacityPerBarrow)))
	coinCost := autoSceatResMarketCoinCost(autoSceatResMarketDistance(sourceInfo, targetInfo), barrows)
	coinKey := autoSceatResReservationKey(0, "coins")
	availableCoins := Models.GetGameState().GlobalResources.Coins - reservations[coinKey] - cfg.MinimumCoinReserve
	if coinCost > availableCoins {
		return false
	}
	payload := GameCommands.CRMPayload(best.source.KingdomID, best.source.CastleID, targetInfo.MapX, targetInfo.MapY, autoSceatResResourceCodes[best.resource], int(best.amount))
	Logging.AutoSceatResLogf("market_overflow", "source=%d target=%d kid=%d %s=%.0f threshold=%d%% barrows=%d", best.source.CastleID, best.target.CastleID, best.source.KingdomID, best.resource, best.amount, cfg.OverflowThresholdPercent, barrows)
	*transportActionTaken = true
	travelSec, accepted := autoSceatResSendMarketAndWait(ctx, payload)
	if !accepted {
		return false
	}
	if travelSec <= 0 {
		travelSec = int(math.Max(300, float64(cfg.CheckIntervalSec)))
	}
	Models.GetGameState().RecordMarketShipment(gamestate.MarketPendingShipment{
		SourceCastleID: best.source.CastleID,
		TargetCastleID: best.target.CastleID,
		Resource:       best.resource,
		Amount:         best.amount,
		ArrivesAtUnix:  time.Now().Unix() + int64(travelSec),
	})
	reservations[autoSceatResReservationKey(best.source.CastleID, best.resource)] += best.amount
	reservations[coinKey] += coinCost
	autoSceatResQueuePayload(GameCommands.CMIPayload())
	return true
}

func autoSceatResTryStormOverflow(ctx context.Context, cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, reservations autoSceatResReservations, transportActionTaken *bool) bool {
	if !cfg.AutoKingdomTransport || !cfg.UseStormBuffer || *transportActionTaken {
		return false
	}
	var storm AutoSceatStorageNode
	for _, node := range catalog.Nodes {
		if node.StormBuffer {
			storm = node
			break
		}
	}
	if storm.CastleID == 0 {
		return false
	}
	state := Models.GetGameState().KingdomTransportSnapshot()
	if _, pending := autoSceatResPendingForKingdom(state, storm.KingdomID); pending || !autoSceatResUnlockAllows(state, storm.KingdomID) {
		return false
	}

	type overflowCandidate struct {
		node     AutoSceatStorageNode
		resource string
		amount   float64
	}
	best := overflowCandidate{}
	for _, node := range catalog.Nodes {
		if node.KingdomID == storm.KingdomID {
			continue
		}
		info := Models.GetGameState().GetCastleByID(node.CastleID)
		if info == nil {
			continue
		}
		for _, resource := range []string{"coal", "oil", "glass", "iron"} {
			storage := autoSceatResLocalStorage(info, resource)
			if storage <= 0 {
				continue
			}
			threshold := storage * float64(cfg.OverflowThresholdPercent) / 100
			protected := autoSceatResProtectedDemand(cfg, node, resource)
			leaveBehind := math.Max(threshold, protected)
			overflow := autoSceatResLocalAmount(info, resource) - leaveBehind - reservations[autoSceatResReservationKey(node.CastleID, resource)]
			stormFree := storm.Storage[resource] - storm.Resources[resource]
			maxSendForDestination := math.Floor(stormFree / autoSceatResKingdomDelivery)
			overflow = math.Min(overflow, maxSendForDestination)
			if overflow >= float64(cfg.MinimumShipmentSize) && overflow > best.amount {
				best = overflowCandidate{node: node, resource: resource, amount: overflow}
			}
		}
	}
	if best.amount <= 0 {
		return false
	}

	amount := int(math.Floor(best.amount))
	payload := GameCommands.KGTPayload(best.node.CastleID, best.node.KingdomID, storm.KingdomID, autoSceatResResourceCodes[best.resource], amount)
	Logging.AutoSceatResLogf("storm_buffer", "source=%d(kid=%d) storm=%d %s=%d threshold=%d%%", best.node.CastleID, best.node.KingdomID, storm.CastleID, best.resource, amount, cfg.OverflowThresholdPercent)
	*transportActionTaken = true
	if !autoSceatResSendTransportAndWait(ctx, payload) {
		Logging.AutoSceatResLogf("storm_timeout", "source=%d resource=%s", best.node.CastleID, best.resource)
		return false
	}
	reservations[autoSceatResReservationKey(best.node.CastleID, best.resource)] += float64(amount)

	state = Models.GetGameState().KingdomTransportSnapshot()
	if pending, ok := autoSceatResPendingForKingdom(state, storm.KingdomID); ok {
		if skipID := autoSceatResChooseTimeSkip(cfg, pending.RemainingSec, reservations); skipID != "" {
			Logging.AutoSceatResLogf("storm_skip_time", "skip=%s remaining=%ds", skipID, pending.RemainingSec)
			if autoSceatResSendTransportAndWait(ctx, GameCommands.KingdomResourceMSKPayload(skipID, storm.KingdomID)) {
				reservations["skip:"+skipID]++
			}
		}
	}
	return true
}

func autoSceatResAdjustedSkipCost(recipe AutoSceatRecipeCatalogEntry, remainingSec int) int {
	if recipe.SkipCostRubies <= 0 || recipe.DurationSec <= 0 || remainingSec <= 0 {
		return 0
	}
	remaining := math.Min(float64(remainingSec), float64(recipe.DurationSec))
	return int(math.Ceil(remaining / float64(recipe.DurationSec) * float64(recipe.SkipCostRubies)))
}

func autoSceatResOverflowPressureResources(cfg stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, reservations autoSceatResReservations) map[string]bool {
	pressured := make(map[string]bool)
	minimumExcess := math.Max(1, float64(cfg.MinimumShipmentSize))
	for _, node := range catalog.Nodes {
		info := Models.GetGameState().GetCastleByID(node.CastleID)
		if info == nil {
			continue
		}
		for _, resource := range []string{"coal", "oil", "glass", "iron"} {
			storage := autoSceatResLocalStorage(info, resource)
			if storage <= 0 {
				continue
			}
			threshold := storage * float64(cfg.OverflowThresholdPercent) / 100
			excess := autoSceatResLocalAmount(info, resource) - threshold - reservations[autoSceatResReservationKey(node.CastleID, resource)]
			if excess >= minimumExcess {
				pressured[resource] = true
			}
		}
	}
	return pressured
}

func autoSceatResPressureResource(recipe AutoSceatRecipeCatalogEntry, pressured map[string]bool) string {
	for _, resource := range []string{"iron", "coal", "oil", "glass"} {
		if pressured[resource] && recipe.Costs[resource] > 0 {
			return resource
		}
	}
	return ""
}

type autoSceatResRubySkipCandidate struct {
	castleID         int
	queueID          int
	building         castle.CraftingBuildingSnapshot
	slot             int
	activeRecipeID   int
	nextRecipeID     int
	pressureResource string
	remainingSec     int
	priceRubies      int
}

func autoSceatResTryRubyOverflowSkip(ctx context.Context, cfg *stsettings.AutoSceatResConfig, catalog AutoSceatResCatalog, reservations autoSceatResReservations, transportActionTaken bool) bool {
	if cfg == nil || !cfg.UseRubyOverflowSkip || !cfg.AutoKingdomTransport || transportActionTaken {
		return false
	}
	pressured := autoSceatResOverflowPressureResources(*cfg, catalog, reservations)
	if len(pressured) == 0 {
		return false
	}

	var mainNode AutoSceatStorageNode
	for _, node := range catalog.Nodes {
		if node.KingdomID == 0 && node.Role == "Main Castle" {
			mainNode = node
			break
		}
	}
	if mainNode.CastleID <= 0 {
		return false
	}
	mainInfo := Models.GetGameState().GetCastleByID(mainNode.CastleID)
	castlePlan, planned := cfg.Castles[mainNode.CastleID]
	if mainInfo == nil || !planned {
		return false
	}

	queueIDs := make([]int, 0, len(castlePlan.Buildings))
	for queueID := range castlePlan.Buildings {
		queueIDs = append(queueIDs, queueID)
	}
	sort.Ints(queueIDs)
	nowUnix := time.Now().Unix()
	best := autoSceatResRubySkipCandidate{}
	for _, queueID := range queueIDs {
		plan := castlePlan.Buildings[queueID]
		if !plan.Enabled || plan.CycleLength() == 0 {
			continue
		}
		building, found := autoSceatResBuilding(mainInfo, queueID)
		if !found || autoSceatResOccupied(building) < autoSceatResCapacity(building) {
			continue
		}
		nextRecipeID, found := plan.RecipeAtCursor()
		if !found {
			continue
		}
		nextRecipe, found := AutoSceatRecipeByID(nextRecipeID)
		if !found || nextRecipe.QueueTypeID != queueID || strings.EqualFold(nextRecipe.Type, "Ruby") || nextRecipe.Costs["rubies"] > 0 {
			continue
		}
		pressureResource := autoSceatResPressureResource(nextRecipe, pressured)
		if pressureResource == "" || !autoSceatResRecipeAllowed(mainInfo, building, nextRecipe) {
			continue
		}
		if !autoSceatResCheckCosts(Models.GetGameState(), mainInfo, nextRecipe, *cfg, reservations).canAfford() {
			continue
		}

		for slot, activeRecipeID := range building.PS.CRID {
			if slot >= len(building.PS.RCT) {
				continue
			}
			activeRecipe, found := AutoSceatRecipeByID(activeRecipeID)
			if !found || strings.EqualFold(activeRecipe.Type, "Ruby") {
				continue
			}
			remainingSec := building.PS.RCT[slot]
			if building.ObservedUnix > 0 && nowUnix > building.ObservedUnix {
				remainingSec -= int(nowUnix - building.ObservedUnix)
			}
			priceRubies := autoSceatResAdjustedSkipCost(activeRecipe, remainingSec)
			if priceRubies <= 0 || (best.priceRubies > 0 && priceRubies >= best.priceRubies) {
				continue
			}
			best = autoSceatResRubySkipCandidate{
				castleID:         mainNode.CastleID,
				queueID:          queueID,
				building:         building,
				slot:             slot,
				activeRecipeID:   activeRecipeID,
				nextRecipeID:     nextRecipeID,
				pressureResource: pressureResource,
				remainingSec:     remainingSec,
				priceRubies:      priceRubies,
			}
		}
	}
	if best.priceRubies <= 0 {
		return false
	}

	rubyKey := autoSceatResReservationKey(0, "rubies")
	availableRubies := Models.GetGameState().GlobalResources.Rubies - reservations[rubyKey] - cfg.MinimumRubyReserve
	if float64(best.priceRubies) > availableRubies {
		Logging.AutoSceatResLogf("ruby_overflow_reserve", "castle=%d queue=%d needs=%d available-above-reserve=%.0f", best.castleID, best.queueID, best.priceRubies, math.Max(availableRubies, 0))
		return false
	}

	before := autoSceatResOccupied(best.building)
	payload := GameCommands.CRSKPayload(best.building.KID, best.building.AID, best.building.OID, best.slot, "production", best.priceRubies)
	Logging.AutoSceatResLogf("ruby_overflow_skip", "castle=%d queue=%d slot=%d active=%d next=%d pressure=%s remaining=%ds rubies=%d", best.castleID, best.queueID, best.slot, best.activeRecipeID, best.nextRecipeID, best.pressureResource, best.remainingSec, best.priceRubies)
	if !autoSceatResSendCraftingAndWait(ctx, payload) {
		Logging.AutoSceatResLogf("ruby_overflow_timeout", "castle=%d queue=%d slot=%d", best.castleID, best.queueID, best.slot)
		return false
	}
	refreshed := Models.GetGameState().GetCastleByID(best.castleID)
	updatedBuilding, found := autoSceatResBuilding(refreshed, best.queueID)
	if !found || autoSceatResOccupied(updatedBuilding) >= before {
		Logging.AutoSceatResLogf("ruby_overflow_rejected", "castle=%d queue=%d slot=%d", best.castleID, best.queueID, best.slot)
		return false
	}
	reservations[rubyKey] += float64(best.priceRubies)
	followupTransportActionTaken := true
	autoSceatResProcessBuilding(ctx, cfg, refreshed, best.queueID, reservations, &followupTransportActionTaken)
	return true
}

func autoSceatResProcessBuilding(ctx context.Context, cfg *stsettings.AutoSceatResConfig, info *Models.PlayerCastleInfo, queueID int, reservations autoSceatResReservations, transportActionTaken *bool) {
	castleID := int(info.Aid)
	castlePlan := cfg.Castles[castleID]
	plan := castlePlan.Buildings[queueID]
	if !plan.Enabled || plan.CycleLength() == 0 {
		return
	}
	maxAttempts := plan.CycleLength() + 8
	for attempt := 0; attempt < maxAttempts; attempt++ {
		currentInfo := Models.GetGameState().GetCastleByID(castleID)
		building, found := autoSceatResBuilding(currentInfo, queueID)
		if !found {
			Logging.AutoSceatResLogf("building_skip", "castle=%d queue=%d not present", castleID, queueID)
			return
		}
		if autoSceatResOccupied(building) >= autoSceatResCapacity(building) {
			recipeID, ok := plan.RecipeAtCursor()
			if !ok {
				return
			}
			recipe, ok := AutoSceatRecipeByID(recipeID)
			if !ok || !autoSceatResRecipeAllowed(currentInfo, building, recipe) {
				Logging.AutoSceatResLogf("recipe_unavailable", "castle=%d queue=%d recipe=%d", castleID, queueID, recipeID)
				autoSceatResAdvancePlan(cfg, castleID, queueID)
				plan = cfg.Castles[castleID].Buildings[queueID]
				continue
			}
			costCheck := autoSceatResCheckCosts(Models.GetGameState(), currentInfo, recipe, *cfg, reservations)
			if !costCheck.canAfford() {
				Logging.AutoSceatResLogf("queue_full", "castle=%d queue=%d; rental deferred because next recipe lacks %s", castleID, queueID, autoSceatResMissingSummary(costCheck))
				return
			}
			if !autoSceatResRentNextSlot(ctx, *cfg, currentInfo, building, plan, reservations) {
				return
			}
			if !autoSceatResPause(ctx, autoSceatResActionDelay) {
				return
			}
			continue
		}

		recipeID, ok := plan.RecipeAtCursor()
		if !ok {
			return
		}
		recipe, ok := AutoSceatRecipeByID(recipeID)
		if !ok || recipe.QueueTypeID != queueID || !autoSceatResRecipeAllowed(currentInfo, building, recipe) {
			Logging.AutoSceatResLogf("recipe_unavailable", "castle=%d queue=%d recipe=%d (research/building requirement)", castleID, queueID, recipeID)
			autoSceatResAdvancePlan(cfg, castleID, queueID)
			plan = cfg.Castles[castleID].Buildings[queueID]
			continue
		}
		costCheck := autoSceatResCheckCosts(Models.GetGameState(), currentInfo, recipe, *cfg, reservations)
		if !costCheck.canAfford() {
			if costCheck.RubyBlocked || len(costCheck.MissingGlobal) > 0 {
				Logging.AutoSceatResLogf("resource_skip", "castle=%d queue=%d recipe=%d missing=%s", castleID, queueID, recipeID, autoSceatResMissingSummary(costCheck))
				return
			}
			if autoSceatResTryTransport(ctx, *cfg, castleID, costCheck.MissingLocal, reservations, transportActionTaken) {
				return
			}
			Logging.AutoSceatResLogf("resource_skip", "castle=%d queue=%d recipe=%d missing=%s", castleID, queueID, recipeID, autoSceatResMissingSummary(costCheck))
			return
		}

		before := autoSceatResOccupied(building)
		payload := GameCommands.CRSTPayload(building.KID, building.AID, building.OID, 0, recipeID)
		Logging.AutoSceatResLogf("queue", "castle=%d queue=%d recipe=%d level=%d output=%s %.0f occupied=%d/%d", castleID, queueID, recipeID, recipe.Level, recipe.Output.Name, recipe.Output.Amount, before, autoSceatResCapacity(building))
		if !autoSceatResSendCraftingAndWait(ctx, payload) {
			Logging.AutoSceatResLogf("queue_timeout", "castle=%d queue=%d recipe=%d", castleID, queueID, recipeID)
			return
		}
		refreshed := Models.GetGameState().GetCastleByID(castleID)
		updatedBuilding, found := autoSceatResBuilding(refreshed, queueID)
		if !found || autoSceatResOccupied(updatedBuilding) <= before {
			Logging.AutoSceatResLogf("queue_rejected", "castle=%d queue=%d recipe=%d", castleID, queueID, recipeID)
			return
		}
		autoSceatResReserveRecipe(currentInfo, recipe, reservations)
		autoSceatResAdvancePlan(cfg, castleID, queueID)
		plan = cfg.Castles[castleID].Buildings[queueID]
		if !autoSceatResPause(ctx, autoSceatResActionDelay) {
			return
		}
	}
}

func autoSceatResOrderedCraftingNodes(catalog AutoSceatResCatalog, cfg stsettings.AutoSceatResConfig) []AutoSceatStorageNode {
	var nodes []AutoSceatStorageNode
	for _, node := range catalog.Nodes {
		if !node.CanCraft {
			continue
		}
		if castlePlan, ok := cfg.Castles[node.CastleID]; ok && len(castlePlan.Buildings) > 0 {
			nodes = append(nodes, node)
		}
	}
	sort.SliceStable(nodes, func(i, j int) bool {
		if nodes[i].KingdomID == nodes[j].KingdomID {
			return nodes[i].CastleID < nodes[j].CastleID
		}
		return nodes[i].KingdomID < nodes[j].KingdomID
	})
	return nodes
}

func runAutoSceatRes(ctx context.Context) {
	Logging.AutoSceatResLog("loop_start", "")
	defer Logging.AutoSceatResLog("loop_stop", "")
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		state := Models.GetSettingsState()
		cfg := state.AutoSceatRes.Normalize()
		state.AutoSceatRes = cfg
		sleepDuration := time.Duration(cfg.CheckIntervalSec) * time.Second
		now := time.Now()
		if !state.FeatureScheduleAllows(autoSceatResFeatureID, now) {
			sleepDuration = autoSceatResScheduledSleep(state, now, sleepDuration)
			Logging.AutoSceatResLogf("schedule", "inactive; next check in %s", sleepDuration.Round(time.Second))
			if !autoSceatResSleep(ctx, sleepDuration) {
				return
			}
			continue
		}
		if !ResponseRegistry.IsGameWebSocketReady() {
			Logging.AutoSceatResLogf("login", "game websocket not ready; next check in %s", sleepDuration)
			if !autoSceatResSleep(ctx, sleepDuration) {
				return
			}
			continue
		}

		Logging.AutoSceatResLog("cycle", "start")
		autoSceatResRefreshState(ctx, cfg.AutoKingdomTransport)
		catalog, err := BuildAutoSceatResCatalog()
		if err != nil {
			Logging.AutoSceatResLogf("catalog", "load failed: %v", err)
			if !autoSceatResSleep(ctx, sleepDuration) {
				return
			}
			continue
		}

		reservations := make(autoSceatResReservations)
		transportActionTaken := false
		for _, node := range autoSceatResOrderedCraftingNodes(catalog, cfg) {
			info := Models.GetGameState().GetCastleByID(node.CastleID)
			if info == nil {
				continue
			}
			castlePlan := cfg.Castles[node.CastleID]
			queueIDs := make([]int, 0, len(castlePlan.Buildings))
			for queueID := range castlePlan.Buildings {
				queueIDs = append(queueIDs, queueID)
			}
			sort.Ints(queueIDs)
			for _, queueID := range queueIDs {
				autoSceatResProcessBuilding(ctx, &cfg, info, queueID, reservations, &transportActionTaken)
			}
		}
		_ = autoSceatResTryPendingTimeSkip(ctx, cfg, reservations, &transportActionTaken)
		_ = autoSceatResTryMarketOverflow(ctx, cfg, catalog, reservations, &transportActionTaken)
		_ = autoSceatResTryStormOverflow(ctx, cfg, catalog, reservations, &transportActionTaken)
		_ = autoSceatResTryRubyOverflowSkip(ctx, &cfg, catalog, reservations, transportActionTaken)

		state.AutoSceatRes = cfg.Normalize()
		if err := stsettings.WriteAutoSceatResConfig(state.AutoSceatRes); err != nil {
			Logging.AutoSceatResLogf("settings", "cursor write failed: %v", err)
		}
		Logging.AutoSceatResLogf("sleep", "next cycle in %s", sleepDuration)
		if !autoSceatResSleep(ctx, sleepDuration) {
			return
		}
	}
}

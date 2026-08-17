package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	stormIntentKingdomID           State.KingdomID = GameData.StormKingdomID
	stormIntentIslandMapTypeID                     = 24
	stormIntentFortMapTypeID                       = 25
	stormIntentMaximumSupport                      = 8
	stormMapWindowSize                             = 101
	stormMapCenterCoordinate                       = 650
	stormMapInitialHalfSpan                        = 50
	stormMapRingStride                             = stormMapWindowSize - 1
	stormMapEdgeBuffer                             = 25
	stormMapMaximumWindowCount                     = 400
	stormMapMaximumCoordinate                      = stormMapWindowSize*stormMapMaximumWindowCount - 1
	stormMapWindowDelayMillis                      = 150
	stormMapBurstResponseTimeout                   = 15 * time.Second
	stormMapMinimumAttemptInterval                 = 2 * time.Hour
)

type stormCastleUnlockRequest struct {
	PrebuiltCastleID int64 `json:"prebuiltCastleId"`
}

type stormCastleUnlockGuardRequest struct {
	PrebuiltCastleID int64     `json:"prebuiltCastleId"`
	RefreshStartedAt time.Time `json:"refreshStartedAt"`
}

type stormMapScanRequest struct {
	SourceCastleID State.CastleID         `json:"sourceCastleId"`
	FullMap        bool                   `json:"fullMap,omitempty"`
	Cooperative    bool                   `json:"cooperative,omitempty"`
	LeaseID        string                 `json:"leaseId,omitempty"`
	Windows        []State.StormMapBounds `json:"windows,omitempty"`
	Targeted       bool                   `json:"targeted,omitempty"`
	Bounds         State.StormMapBounds   `json:"bounds"`
	Radius         int                    `json:"radius,omitempty"`
	ScanStartedAt  time.Time              `json:"scanStartedAt"`
}

type stormMapBurstSender interface {
	CorrelatesResponses() bool
	Namespace() string
	Send(context.Context, []byte) error
	WaitForAutomationUnlocked(context.Context) error
}

type stormMapBurstObserver interface {
	ForgetCommitted(uint64)
	WaitCommitted(context.Context, uint64) (Protocol.CommittedFrame, error)
	WatchWireResponse(string, string) (<-chan Protocol.CommittedFrame, func())
}

type stormMapBurstSlot struct {
	window towerMapWindow
	token  string
	wire   []byte
	frames <-chan Protocol.CommittedFrame
	cancel func()
}

type stormMapBurstResult struct {
	index int
	frame Protocol.CommittedFrame
}

type stormDefenseUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type stormAttackRequest struct {
	SourceCastleID      State.CastleID       `json:"sourceCastleId"`
	KingdomID           State.KingdomID      `json:"kingdomId"`
	TargetTypeID        int                  `json:"targetTypeId"`
	TargetX             int                  `json:"targetX"`
	TargetY             int                  `json:"targetY"`
	StormIsleID         int64                `json:"stormIsleId"`
	VictoryCount        int64                `json:"victoryCount,omitempty"`
	MinimumVictoryCount int64                `json:"minimumVictoryCount,omitempty"`
	Preset              AttackPresets.Preset `json:"preset"`
	CommanderIDs        []State.CommanderID  `json:"commanderIds,omitempty"`
	HorseTravelBoostID  int                  `json:"horseTravelBoostId"`
	DailyAttackLimit    int64                `json:"dailyAttackLimit"`
	DefenseUnits        []stormDefenseUnit   `json:"defenseUnits,omitempty"`
}

type resolvedStormAttackRequest struct {
	stormAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type stormTargetConsumeRequest struct {
	SourceCastleID State.CastleID  `json:"sourceCastleId"`
	KingdomID      State.KingdomID `json:"kingdomId"`
	TargetTypeID   int             `json:"targetTypeId"`
	TargetX        int             `json:"targetX"`
	TargetY        int             `json:"targetY"`
	IslandObjectID int64           `json:"islandObjectId,omitempty"`
	LeaveBehind    int64           `json:"leaveBehind,omitempty"`
}

type stormIslandReturnUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type stormIslandReturnRequest struct {
	SourceCastleID State.CastleID          `json:"sourceCastleId"`
	KingdomID      State.KingdomID         `json:"kingdomId"`
	IslandX        int                     `json:"islandX"`
	IslandY        int                     `json:"islandY"`
	IslandObjectID int64                   `json:"islandObjectId"`
	ReportID       int64                   `json:"reportId"`
	Units          []stormIslandReturnUnit `json:"units"`
}

type stormShopPurchaseLineRequest struct {
	ProductID State.PackageID `json:"productId"`
	Amount    int64           `json:"amount"`
}

type stormShopPurchaseRequest struct {
	CastleID          State.CastleID                 `json:"castleId"`
	ProductID         State.PackageID                `json:"productId,omitempty"`
	Amount            int64                          `json:"amount,omitempty"`
	Purchases         []stormShopPurchaseLineRequest `json:"purchases,omitempty"`
	AquamarineReserve int64                          `json:"aquamarineReserve"`
}

type stormShopPurchaseLine struct {
	request stormShopPurchaseLineRequest
	item    GameData.StormShopPackage
}

func (application *Application) registerStormIntents() error {
	if err := application.registerStormBlueprintIntents(); err != nil {
		return err
	}
	for name, action := range map[string]Intent.Action{
		"storm.castle.unlock.verify":   application.verifyStormCastleUnlock,
		"storm.scan.begin":             application.beginStormScan,
		"storm.scan.burst":             application.burstStormMapScan,
		"storm.scan.capture":           application.captureStormScan,
		"storm.attack.guard":           application.guardStormAttack,
		"storm.target.consume":         application.consumeStormTarget,
		"storm.island.return.guard":    application.guardStormIslandReturn,
		"storm.island.return.complete": application.completeStormIslandReturn,
		"storm.shop.guard":             application.guardStormShopPurchase,
	} {
		if err := application.Intents.RegisterAction(name, action); err != nil {
			return err
		}
	}
	if err := application.Intents.RegisterStepResolver("storm.attack.build", application.resolveStormAttackStep); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "storm.castle.refresh", Description: "Refresh the authoritative owned-castle directory after a Storm unlock", Effect: Intent.EffectRead,
			Planner: planStormCastleRefresh,
		},
		{
			Name: "storm.castle.unlock", Description: "Open one explicitly selected official prebuilt Storm castle", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"prebuiltCastleId":16}`), Planner: planStormCastleUnlock,
		},
		{
			Name: "storm.map.scan", Description: "Focus the Storm castle and refresh an adaptive center-out map snapshot", Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"fullMap":true,"bounds":{"x1":600,"y1":600,"x2":700,"y2":700}}`), Planner: planStormMapScan,
		},
		{
			Name: "storm.attack", Description: "Launch a guarded CitadelOps preset against an eligible Storm fort or resource island", Effect: Intent.EffectLaunch,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"kingdomId":4,"targetTypeId":25,"targetX":500,"targetY":500,"stormIsleId":10,"minimumVictoryCount":5,"preset":{"id":"storm-fort","name":"Storm fort","waves":[]}}`),
			AttackModule:     &Intent.AttackModuleDefinition{ID: "autoStorm", Label: "Auto Storm", Description: "Storm fort and resource-island attacks", DefaultWeight: 50},
			Planner:          planStormAttack,
		},
		{
			Name: "storm.island.return", Description: "Return report-confirmed surviving troops from an occupied Storm island to the Storm castle", Effect: Intent.EffectLaunch,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"kingdomId":4,"islandX":500,"islandY":500,"islandObjectId":15532314,"reportId":42,"units":[{"unitId":10,"amount":99}]}`),
			Planner:          planStormIslandReturn,
		},
		{
			Name: "storm.shop.purchase", Description: "Buy one or more official Luna trade-boat packages while preserving an Aquamarine reserve", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"purchases":[{"productId":244,"amount":2}],"aquamarineReserve":50000}`), Planner: planStormShopPurchase,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planStormCastleRefresh(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	if input.State.Player.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("the current player identity is unavailable")
	}
	payload, _ := json.Marshal(struct {
		PlayerID State.PlayerID `json:"PID"`
	}{input.State.Player.ID})
	step := commandStep("Refresh the owned-castle directory", "gcl", payload, "gcl")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"castle-directory", "kingdom:" + strconv.FormatInt(int64(stormIntentKingdomID), 10)},
		Summary: "Refresh the owned-castle directory for Storm",
		Steps:   []Intent.Step{step},
	}, nil
}

func planStormCastleUnlock(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request stormCastleUnlockRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	option, err := stormCastleUnlockOption(input, request.PrebuiltCastleID, time.Time{})
	if err != nil {
		return Intent.Plan{}, err
	}
	refreshStartedAt := time.Now().UTC()
	guardArguments, _ := json.Marshal(stormCastleUnlockGuardRequest{
		PrebuiltCastleID: request.PrebuiltCastleID, RefreshStartedAt: refreshStartedAt,
	})
	payload, _ := json.Marshal(struct {
		ID        int64           `json:"ID"`
		Direction int             `json:"D"`
		Premium   int             `json:"PWR"`
		Secondary int             `json:"OC2"`
		KingdomID State.KingdomID `json:"SID"`
	}{
		ID: option.ID, Direction: 0, Premium: 0,
		Secondary: boolInt(option.CostPremium > 0), KingdomID: stormIntentKingdomID,
	})
	castleListPayload, _ := json.Marshal(struct {
		PlayerID State.PlayerID `json:"PID"`
	}{input.State.Player.ID})
	unlockStep := commandStep("Open the selected Storm castle", "ksc", payload, "ksc")
	unlockStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	castleListStep := commandStep("Refresh the new Storm castle", "gcl", castleListPayload, "gcl")
	castleListStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	kingdomStep := kingdomTransportContextStep()
	kingdomStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{
			"account-resources", "castle-directory",
			"kingdom:" + strconv.FormatInt(int64(stormIntentKingdomID), 10),
		},
		Summary: fmt.Sprintf("Open official Storm castle %d (%s)", option.ID, stormCastleOptionCost(option)),
		Steps: []Intent.Step{
			kingdomStep,
			Intent.RebuildOnResume(Intent.Step{
				Name: "Verify refreshed Storm castle availability", Action: "storm.castle.unlock.verify",
				ActionArguments: guardArguments,
			}),
			unlockStep,
			castleListStep,
			contextCommandStep("Refresh Storm kingdom state", "kpi", json.RawMessage(`{}`), "kpi"),
		},
	}, nil
}

func stormCastleUnlockOption(
	input Intent.PlanningContext,
	prebuiltCastleID int64,
	refreshedAfter time.Time,
) (GameData.StormCastleOption, error) {
	if input.GameData == nil {
		return GameData.StormCastleOption{}, fmt.Errorf("official game data is unavailable")
	}
	if input.State.Player.ID <= 0 {
		return GameData.StormCastleOption{}, fmt.Errorf("the current player identity is unavailable")
	}
	if _, exists := ownedCastleInKingdom(input.State, stormIntentKingdomID); exists {
		return GameData.StormCastleOption{}, fmt.Errorf("%w: an owned Storm castle already exists", Intent.ErrPlanStale)
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[stormIntentKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed {
		return GameData.StormCastleOption{}, fmt.Errorf("%w: Storm kingdom availability has not been observed", Intent.ErrPlanStale)
	}
	if !refreshedAfter.IsZero() && input.State.KingdomTransport.ObservedAt.Before(refreshedAfter) {
		return GameData.StormCastleOption{}, fmt.Errorf("the Storm kingdom list was not refreshed before opening the castle")
	}
	if unlock.Unlocked || unlock.Created {
		return GameData.StormCastleOption{}, fmt.Errorf("%w: the Storm kingdom is already unlocked; refresh the castle directory instead", Intent.ErrPlanStale)
	}
	option, found := input.GameData.StormCastleOption(prebuiltCastleID, input.State.Player.Level)
	if !found {
		return GameData.StormCastleOption{}, fmt.Errorf("prebuiltCastleId must identify an unlocked official Storm castle option")
	}
	return option, nil
}

func (application *Application) verifyStormCastleUnlock(_ context.Context, arguments json.RawMessage) error {
	var request stormCastleUnlockGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.PrebuiltCastleID <= 0 || request.RefreshStartedAt.IsZero() {
		return fmt.Errorf("Storm castle unlock verification requires an option and refresh time")
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("Storm castle unlock state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	if _, err := stormCastleUnlockOption(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, request.PrebuiltCastleID, request.RefreshStartedAt); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func stormCastleOptionCost(option GameData.StormCastleOption) string {
	parts := make([]string, 0, 5)
	for _, cost := range []struct {
		amount int64
		name   string
	}{
		{option.CostWood, "wood"}, {option.CostStone, "stone"}, {option.CostFood, "food"},
		{option.CostCoins, "coins"}, {option.CostPremium, "rubies"},
	} {
		if cost.amount > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", cost.amount, cost.name))
		}
	}
	if len(parts) == 0 {
		return "no catalog cost"
	}
	return strings.Join(parts, ", ")
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func planStormMapScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := stormMapScanContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	lastAttemptAt := time.Time{}
	if stormMapStateMatches(input.State, input.State.Storm.Map, source.ID) {
		lastAttemptAt = input.State.Storm.Map.LastAttemptAt
	} else if input.State.Storm.Map.SourceCastleID == 0 {
		lastAttemptAt = input.State.Storm.LastScannedAt[source.ID]
	}
	if request.FullMap && !request.Cooperative && !lastAttemptAt.IsZero() && time.Now().UTC().Before(lastAttemptAt.Add(stormMapMinimumAttemptInterval)) {
		return Intent.Plan{}, fmt.Errorf("full Storm map sweeps are limited to one attempt every two hours")
	}
	request.ScanStartedAt = time.Now().UTC()
	normalizedArguments, _ := json.Marshal(request)
	windows := towerMapScanWindows(source, request.Radius)
	if request.Cooperative {
		windows = stormCooperativeScanWindows(request.Windows)
	} else if request.FullMap || request.Targeted {
		windows = stormMapScanWindows(request.Bounds)
	}
	steps := make([]Intent.Step, 0, len(windows)+3)
	if !source.Focused {
		focus := castleFocusStep(source)
		focus.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, focus)
	}
	if request.FullMap {
		steps = append(steps, Intent.Step{
			Name: "Record Storm map sweep attempt", Action: "storm.scan.begin", ActionArguments: normalizedArguments,
		})
		steps = append(steps, Intent.Step{
			Name: "Burst Storm map windows", Action: "storm.scan.burst", ActionArguments: normalizedArguments,
		})
	} else {
		for index, window := range windows {
			payload, _ := json.Marshal(struct {
				KingdomID State.KingdomID `json:"KID"`
				X1        int             `json:"AX1"`
				Y1        int             `json:"AY1"`
				X2        int             `json:"AX2"`
				Y2        int             `json:"AY2"`
			}{source.KingdomID, window.X1, window.Y1, window.X2, window.Y2})
			step := commandStep(
				fmt.Sprintf("Refresh Storm map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa",
			)
			if index > 0 {
				step.DelayMillis = stormMapWindowDelayMillis
			}
			steps = append(steps, step)
		}
	}
	if !request.FullMap {
		steps = append(steps, Intent.Step{Name: "Build Storm map state", Action: "storm.scan.capture", ActionArguments: normalizedArguments})
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	summary := fmt.Sprintf("Refresh Storm forts and resource islands around %s", castleLabel(source))
	if request.FullMap {
		if request.Cooperative {
			summary = fmt.Sprintf("Refresh %d leased windows for shared Storm coverage", len(request.Windows))
		} else {
			summary = fmt.Sprintf("Refresh the Storm map outward from %d:%d", stormMapCenterCoordinate, stormMapCenterCoordinate)
		}
	} else if request.Targeted {
		summary = fmt.Sprintf("Refresh Storm target at %d:%d", request.Bounds.X1, request.Bounds.Y1)
	}
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: summary, Steps: steps,
	}, nil
}

func stormCooperativeScanWindows(bounds []State.StormMapBounds) []towerMapWindow {
	windows := make([]towerMapWindow, 0, len(bounds))
	for _, value := range bounds {
		windows = append(windows, towerMapWindow{X1: value.X1, Y1: value.Y1, X2: value.X2, Y2: value.Y2})
	}
	return windows
}

func stormMapScanWindows(bounds State.StormMapBounds) []towerMapWindow {
	if !bounds.IsValid() || bounds.X2 > stormMapMaximumCoordinate || bounds.Y2 > stormMapMaximumCoordinate {
		return nil
	}
	width := bounds.X2 - bounds.X1 + 1
	height := bounds.Y2 - bounds.Y1 + 1
	windowColumns := (width + stormMapWindowSize - 1) / stormMapWindowSize
	windowRows := (height + stormMapWindowSize - 1) / stormMapWindowSize
	if windowColumns <= 0 || windowRows <= 0 || windowColumns > stormMapMaximumWindowCount/windowRows {
		return nil
	}
	windows := make([]towerMapWindow, 0, windowColumns*windowRows)
	for y := bounds.Y1; y <= bounds.Y2; y += stormMapWindowSize {
		for x := bounds.X1; x <= bounds.X2; x += stormMapWindowSize {
			windows = append(windows, towerMapWindow{
				X1: x,
				Y1: y,
				X2: min(bounds.X2, x+stormMapWindowSize-1),
				Y2: min(bounds.Y2, y+stormMapWindowSize-1),
			})
		}
	}
	return windows
}

func stormMapConcentricBounds(ring int) State.StormMapBounds {
	offset := max(0, ring) * stormMapRingStride
	return State.StormMapBounds{
		X1: stormMapCenterCoordinate - stormMapInitialHalfSpan - offset,
		Y1: stormMapCenterCoordinate - stormMapInitialHalfSpan - offset,
		X2: stormMapCenterCoordinate + stormMapInitialHalfSpan + offset,
		Y2: stormMapCenterCoordinate + stormMapInitialHalfSpan + offset,
	}
}

func stormMapConcentricRingWindows(ring int) []towerMapWindow {
	if ring < 0 {
		return nil
	}
	bounds := stormMapConcentricBounds(ring)
	allWindows := stormMapScanWindows(bounds)
	if len(allWindows) == 0 || len(allWindows) > stormMapMaximumWindowCount {
		return nil
	}
	windows := make([]towerMapWindow, 0, max(1, ring*8))
	for yOffset := -ring; yOffset <= ring; yOffset++ {
		for xOffset := -ring; xOffset <= ring; xOffset++ {
			if max(absStormMapOffset(xOffset), absStormMapOffset(yOffset)) != ring {
				continue
			}
			x1, x2 := stormMapRingAxisBounds(xOffset)
			y1, y2 := stormMapRingAxisBounds(yOffset)
			windows = append(windows, towerMapWindow{
				X1: x1,
				Y1: y1,
				X2: x2,
				Y2: y2,
			})
		}
	}
	return windows
}

func stormMapRingAxisBounds(offset int) (int, int) {
	centerStart := stormMapCenterCoordinate - stormMapInitialHalfSpan
	centerEnd := stormMapCenterCoordinate + stormMapInitialHalfSpan
	if offset < 0 {
		start := centerStart + offset*stormMapRingStride
		return start, start + stormMapRingStride - 1
	}
	if offset > 0 {
		start := centerEnd + 1 + (offset-1)*stormMapRingStride
		return start, start + stormMapRingStride - 1
	}
	return centerStart, centerEnd
}

func absStormMapOffset(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func stormMapSweepNeedsExpansion(state State.GameState, bounds State.StormMapBounds, startedAt time.Time) bool {
	touchesEdge := false
	state.RangeStormMapObservations(func(_ string, observation State.MapObservation) bool {
		if !observation.ObservedAt.IsZero() && observation.ObservedAt.Before(startedAt) {
			return true
		}
		if stormMapTargetTouchesEdge(observation, bounds) {
			touchesEdge = true
			return false
		}
		return true
	})
	return touchesEdge
}

func stormMapTargetTouchesEdge(observation State.MapObservation, bounds State.StormMapBounds) bool {
	if observation.TypeID != stormIntentIslandMapTypeID && observation.TypeID != stormIntentFortMapTypeID {
		return false
	}
	return observation.X <= bounds.X1+stormMapEdgeBuffer ||
		observation.X >= bounds.X2-stormMapEdgeBuffer ||
		observation.Y <= bounds.Y1+stormMapEdgeBuffer ||
		observation.Y >= bounds.Y2-stormMapEdgeBuffer
}

func planStormAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, definition, err := stormAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	var selection *craCommanderSelectionRequest
	if request.CommanderIDs != nil {
		if len(request.CommanderIDs) == 0 {
			return Intent.Plan{}, fmt.Errorf("no commanders are assigned to Auto Storm")
		}
		selection = &craCommanderSelectionRequest{Candidates: request.CommanderIDs, Count: 1, Strategy: "lowest_id"}
	}
	resolution, err := resolveCRACommanders(input.State, selection, craCommanderSelectionOptions{DefaultCount: 1, RequireAvailable: true})
	if err != nil {
		if errors.Is(err, errCRACommanderUnavailable) {
			detail := "no commander is currently available"
			if request.CommanderIDs != nil {
				detail = "no assigned Auto Storm commander is currently available"
			}
			return Intent.Plan{Summary: "Skip Storm attack: " + detail}, nil
		}
		return Intent.Plan{}, err
	}
	commanderID := resolution.Selected[0]
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	resolvedArguments, _ := json.Marshal(resolvedStormAttackRequest{stormAttackRequest: request, CommanderID: commanderID})
	leaveBehind := int64(0)
	if request.TargetTypeID == stormIntentIslandMapTypeID && len(request.DefenseUnits) == 0 {
		leaveBehind = 1
	}
	consumeArguments, _ := json.Marshal(stormTargetConsumeRequest{
		SourceCastleID: source.ID,
		KingdomID:      target.KingdomID,
		TargetTypeID:   target.TypeID,
		TargetX:        target.X,
		TargetY:        target.Y,
		IslandObjectID: target.ObjectID,
		LeaveBehind:    leaveBehind,
	})
	steps := make([]Intent.Step, 0, 6)
	steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	steps = append(steps,
		deferredCRACommandStep("Build and launch capacity-adjusted Storm attack", "storm.attack.build", resolvedArguments, contextPayload),
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoStorm, SourceCastleID: source.ID, CommanderID: commanderID,
			KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
		Intent.Step{Name: "Consume Storm target", Action: "storm.target.consume", ActionArguments: consumeArguments},
	)
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		fmt.Sprintf("storm-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{commanderID})...)
	return Intent.Plan{
		Claims:    claims,
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoStorm", Affinity: "castle:" + castleID},
		Summary:   fmt.Sprintf("Attack Storm %s at %d:%d with %s", definition.Kind, target.X, target.Y, request.Preset.Name),
		Steps:     steps,
	}, nil
}

func planStormIslandReturn(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, castle, operation, err := stormIslandReturnContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	unitIDs := make([]State.UnitID, 0, len(request.Units))
	amounts := make(map[State.UnitID]int64, len(request.Units))
	for _, unit := range request.Units {
		unitIDs = append(unitIDs, unit.UnitID)
		amounts[unit.UnitID] = unit.Amount
	}
	sort.Slice(unitIDs, func(left, right int) bool { return unitIDs[left] < unitIDs[right] })
	wireUnits := make([][2]int64, 0, len(unitIDs))
	var returnTotal int64
	for _, unitID := range unitIDs {
		amount := amounts[unitID]
		wireUnits = append(wireUnits, [2]int64{int64(unitID), amount})
		returnTotal += amount
	}
	route, _ := json.Marshal(struct {
		TargetX int `json:"TX"`
		TargetY int `json:"TY"`
		SourceX int `json:"SX"`
		SourceY int `json:"SY"`
	}{castle.X, castle.Y, request.IslandX, request.IslandY})
	dispatch, _ := json.Marshal(struct {
		SourceID int64      `json:"SID"`
		TargetX  int        `json:"TX"`
		TargetY  int        `json:"TY"`
		LeaderID int        `json:"LID"`
		Wait     int        `json:"WT"`
		Booster  int        `json:"HBW"`
		Premium  int        `json:"BPC"`
		Travel   int        `json:"PTT"`
		Delay    int        `json:"SD"`
		Units    [][2]int64 `json:"A"`
	}{request.IslandObjectID, castle.X, castle.Y, stationLeaderID, 0, -1, 1, 1, 0, wireUnits})
	steps := castleContextSteps(input, castle)
	steps = append(steps,
		contextCommandStep("Preview island return route", "sdi", route, "sdi"),
		Intent.Step{Name: "Verify report-confirmed island survivors", Action: "storm.island.return.guard", ActionArguments: arguments},
		commandStep("Return surviving island troops to Storm castle", "cds", dispatch, "cds"),
		Intent.Step{Name: "Complete island troop return", Action: "storm.island.return.complete", ActionArguments: arguments},
	)
	key := State.StormIslandReturnKey(request.KingdomID, request.IslandX, request.IslandY)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"storm-island-return:" + key,
		},
		Summary: fmt.Sprintf(
			"Return %d surviving troops from Storm island %d:%d after report %d, leaving %d occupier(s)",
			returnTotal, request.IslandX, request.IslandY, request.ReportID, operation.LeaveBehind,
		),
		Steps: steps,
	}, nil
}

func planStormShopPurchase(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, castle, purchases, err := stormShopPurchaseContext(input, arguments, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	historyPayload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, castle.KingdomID})
	steps := []Intent.Step{
		castleFocusStep(castle),
		shopCommandStep("Refresh Luna package purchase counters", "gbc", historyPayload, 0),
		{Name: "Verify Storm Aquamarine reserve", Action: "storm.shop.guard", ActionArguments: arguments},
	}
	purchaseLabels := make([]string, 0, len(purchases))
	totalCost := int64(0)
	for _, purchase := range purchases {
		payload, _ := json.Marshal(struct {
			ProductID State.PackageID `json:"PID"`
			BuildType int64           `json:"BT"`
			TableID   int64           `json:"TID"`
			Amount    int64           `json:"AMT"`
			KingdomID State.KingdomID `json:"KID"`
			CastleID  int64           `json:"AID"`
			Premium   int64           `json:"PC2"`
			BuyAll    int64           `json:"BA"`
			Power     int64           `json:"PWR"`
			Position  int64           `json:"_PO"`
		}{
			purchase.request.ProductID, GameData.StormLunaShopBuildType, GameData.StormLunaShopTableID,
			purchase.request.Amount, stormIntentKingdomID, GameData.StormLunaShopCastleID, -1, 0, 0, -1,
		})
		itemName := userFacingGameName(purchase.item.Name)
		if itemName == "" {
			itemName = fmt.Sprintf("Luna package %d", purchase.request.ProductID)
		}
		purchaseLabels = append(purchaseLabels, fmt.Sprintf("%d x %s", purchase.request.Amount, itemName))
		totalCost += purchase.request.Amount * purchase.item.AquamarinePrice
		steps = append(steps, shopCommandStep("Purchase "+itemName+" from Luna", "sbp", payload, 0))
	}
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:table:" + strconv.FormatInt(GameData.StormLunaShopTableID, 10), "account-resources", "castle-focus",
			"castle:" + strconv.FormatInt(int64(request.CastleID), 10),
		},
		Summary: fmt.Sprintf(
			"Buy %s from Luna for %d Aquamarine at %s",
			stormShopFriendlyList(purchaseLabels), totalCost, castleLabel(castle),
		),
		Steps: steps,
	}, nil
}

func stormShopFriendlyList(values []string) string {
	if len(values) == 0 {
		return "Luna packages"
	}
	if len(values) == 1 {
		return values[0]
	}
	return strings.Join(values[:len(values)-1], ", ") + " and " + values[len(values)-1]
}

func stormMapScanContext(input Intent.PlanningContext, arguments json.RawMessage) (stormMapScanRequest, State.CastleState, error) {
	var request stormMapScanRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return stormMapScanRequest{}, State.CastleState{}, err
	}
	if request.SourceCastleID <= 0 {
		return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("Storm map scan requires a source castle")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != stormIntentKingdomID {
		return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("Storm source castle %d is unavailable in kingdom %d", request.SourceCastleID, stormIntentKingdomID)
	}
	if request.FullMap && request.Cooperative {
		if strings.TrimSpace(request.LeaseID) == "" || len(request.Windows) == 0 || len(request.Windows) > stormMapMaximumWindowCount {
			return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("cooperative Storm scan requires one bounded shared lease")
		}
		covered := State.StormMapBounds{}
		for index, window := range request.Windows {
			if !window.IsValid() || window.X2 > stormMapMaximumCoordinate || window.Y2 > stormMapMaximumCoordinate ||
				len(stormMapScanWindows(window)) != 1 {
				return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("cooperative Storm scan contains an invalid map window")
			}
			if index == 0 {
				covered = window
			} else {
				covered.X1 = min(covered.X1, window.X1)
				covered.Y1 = min(covered.Y1, window.Y1)
				covered.X2 = max(covered.X2, window.X2)
				covered.Y2 = max(covered.Y2, window.Y2)
			}
		}
		if request.Bounds != covered {
			return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("cooperative Storm scan bounds do not match its leased windows")
		}
	} else if request.FullMap {
		initialBounds := stormMapConcentricBounds(0)
		if request.Bounds != initialBounds {
			return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf(
				"full Storm map scan must start at %d:%d with bounds %d:%d through %d:%d",
				stormMapCenterCoordinate, stormMapCenterCoordinate,
				initialBounds.X1, initialBounds.Y1, initialBounds.X2, initialBounds.Y2,
			)
		}
	} else if request.Targeted {
		windows := stormMapScanWindows(request.Bounds)
		if !request.Bounds.IsValid() || len(windows) != 1 || request.Bounds.X1 != request.Bounds.X2 || request.Bounds.Y1 != request.Bounds.Y2 {
			return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("targeted Storm map refresh requires one exact map coordinate")
		}
	} else if request.Radius < 1 || request.Radius > 50 {
		return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf("Storm map scan radius must be between 1 and 50")
	}
	return request, source, nil
}

func stormAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (stormAttackRequest, State.CastleState, State.MapObservation, GameData.StormIsleDefinition, error) {
	var request stormAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, err
	}
	if request.SourceCastleID <= 0 || request.KingdomID != stormIntentKingdomID ||
		(request.TargetTypeID != stormIntentIslandMapTypeID && request.TargetTypeID != stormIntentFortMapTypeID) || request.StormIsleID <= 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm attack requires a Storm source, target type, coordinates, and isle id")
	}
	if input.GameData == nil {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("official game data is unavailable")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != stormIntentKingdomID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID)
	}
	key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
	target, exists := input.State.LookupMapObservation(request.KingdomID, key)
	mapStateCurrent := stormMapStateMatches(input.State, input.State.Storm.Map, source.ID)
	if mapStateCurrent && !input.State.Storm.Map.LastAttemptAt.IsZero() &&
		(input.State.Storm.Map.LastCompletedAt.IsZero() || input.State.Storm.Map.LastCompletedAt.Before(input.State.Storm.Map.LastAttemptAt)) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("the latest full Storm map sweep has not completed")
	}
	if !input.State.Storm.Map.LastCompletedAt.IsZero() && mapStateCurrent {
		scannedTarget, scanned := input.State.LookupStormTarget(key)
		if !scanned {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm target %d:%d is no longer in the authoritative map state", request.TargetX, request.TargetY)
		}
		if !exists || target.ObservedAt.Before(scannedTarget.ObservedAt) {
			target = scannedTarget
			exists = true
		}
	}
	if !exists || target.TypeID != request.TargetTypeID || target.StormIsleID != request.StormIsleID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm target %d:%d is no longer the selected isle", request.TargetX, request.TargetY)
	}
	definition, found := input.GameData.StormIsleView(request.StormIsleID)
	if !found || definition.Kind == GameData.StormIsleKindFort && request.TargetTypeID != stormIntentFortMapTypeID ||
		definition.Kind == GameData.StormIsleKindIsland && request.TargetTypeID != stormIntentIslandMapTypeID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm isle %d does not match target type %d", request.StormIsleID, request.TargetTypeID)
	}
	now := time.Now().UTC()
	if request.TargetTypeID == stormIntentIslandMapTypeID && stormIslandUnavailable(target, now) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm resource island %d:%d is already occupied", request.TargetX, request.TargetY)
	}
	if stormTargetCooldownRemaining(target, now) > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm target %d:%d is still on cooldown", request.TargetX, request.TargetY)
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && stormTargetExpired(target, definition, now) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm resource island %d:%d has expired from the learned map state", request.TargetX, request.TargetY)
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && target.ObjectID <= 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm resource island %d:%d has no object id for the post-capture troop return", request.TargetX, request.TargetY)
	}
	if request.MinimumVictoryCount < 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("minimum attacks remaining cannot be negative")
	}
	awaitingReadyVerification := request.TargetTypeID == stormIntentFortMapTypeID && target.StormCooldownRemaining > 0 &&
		!stormTargetReadyAt(target).After(now)
	if request.TargetTypeID == stormIntentFortMapTypeID && request.MinimumVictoryCount > 0 && !awaitingReadyVerification {
		remaining, known := GameData.StormFortAttacksRemaining(definition, target.StormVictoryCount)
		if !known {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf(
				"Storm fort %d:%d has no authoritative attack limit", request.TargetX, request.TargetY,
			)
		}
		if remaining < request.MinimumVictoryCount {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf(
				"Storm fort %d:%d has %d attacks remaining, below the required minimum %d",
				request.TargetX, request.TargetY, remaining, request.MinimumVictoryCount,
			)
		}
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && request.MinimumVictoryCount > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("minimum attacks remaining only applies to Storm forts")
	}
	if request.TargetTypeID == stormIntentFortMapTypeID && len(request.DefenseUnits) > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("defense units can only accompany a resource-island occupation")
	}
	if err := validateStormDefenseUnits(request.DefenseUnits); err != nil {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, err
	}
	return request, source, target, definition, nil
}

func stormIslandReturnContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (stormIslandReturnRequest, State.CastleState, State.StormIslandReturnState, error) {
	var request stormIslandReturnRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, err
	}
	if request.SourceCastleID <= 0 || request.KingdomID != stormIntentKingdomID || request.IslandObjectID <= 0 ||
		request.ReportID <= 0 || len(request.Units) == 0 {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return requires the Storm castle, island identity, report, and surviving units")
	}
	castle, exists := input.State.Castles[request.SourceCastleID]
	if !exists || castle.KingdomID != stormIntentKingdomID {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID)
	}
	key := State.StormIslandReturnKey(request.KingdomID, request.IslandX, request.IslandY)
	operation, exists := input.State.Storm.IslandReturns[key]
	if !exists || operation.Status != State.StormIslandReturnReady {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island %d:%d has no report-confirmed troop return ready", request.IslandX, request.IslandY)
	}
	if operation.SourceCastleID != request.SourceCastleID || operation.KingdomID != request.KingdomID ||
		operation.TargetX != request.IslandX || operation.TargetY != request.IslandY ||
		operation.IslandObjectID != request.IslandObjectID || operation.ReportID != request.ReportID {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return identity changed after report %d", request.ReportID)
	}
	if input.GameData == nil {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("official game data is unavailable")
	}
	unitsCatalog, err := input.GameData.Catalog("units")
	if err != nil {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, err
	}
	requested := make(map[State.UnitID]int64, len(request.Units))
	for _, unit := range request.Units {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return unit ids and amounts must be positive")
		}
		if _, duplicate := requested[unit.UnitID]; duplicate {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return unit %d is duplicated", unit.UnitID)
		}
		raw, found := unitsCatalog.Find(strconv.FormatInt(int64(unit.UnitID), 10))
		if !found {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("unit %d is not in the official unit catalog", unit.UnitID)
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("decode unit %d: %w", unit.UnitID, decodeErr)
		}
		if GameData.IsToolRecord(record) {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("definition %d is a tool, not an island troop", unit.UnitID)
		}
		requested[unit.UnitID] = unit.Amount
	}
	expected := operation.UnitsToReturn()
	if len(requested) != len(expected) {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return no longer matches report %d survivors", request.ReportID)
	}
	for unitID, amount := range expected {
		if requested[unitID] != amount {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, fmt.Errorf("Storm island return no longer matches report %d survivors", request.ReportID)
		}
	}
	return request, castle, operation, nil
}

func stormShopPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	requireCurrentCounters bool,
) (stormShopPurchaseRequest, State.CastleState, []stormShopPurchaseLine, error) {
	var request stormShopPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, err
	}
	if request.CastleID <= 0 || request.AquamarineReserve < 0 {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm shop purchase requires a castle and a non-negative reserve")
	}
	requested := append([]stormShopPurchaseLineRequest(nil), request.Purchases...)
	if len(requested) == 0 && request.ProductID > 0 && request.Amount > 0 {
		requested = append(requested, stormShopPurchaseLineRequest{ProductID: request.ProductID, Amount: request.Amount})
	}
	normalized := make([]stormShopPurchaseLineRequest, 0, len(requested))
	lineByProduct := map[State.PackageID]int{}
	for _, line := range requested {
		if line.ProductID <= 0 || line.Amount <= 0 {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("each Storm shop purchase requires a product and positive amount")
		}
		if index, exists := lineByProduct[line.ProductID]; exists {
			if normalized[index].Amount > math.MaxInt64-line.Amount {
				return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm shop amount is too large")
			}
			normalized[index].Amount += line.Amount
			continue
		}
		lineByProduct[line.ProductID] = len(normalized)
		normalized = append(normalized, line)
	}
	if len(normalized) == 0 {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm shop purchase requires at least one package")
	}
	if input.GameData == nil {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("official game data is unavailable")
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != stormIntentKingdomID {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("castle %d is not the current Storm castle", request.CastleID)
	}
	offers, observedAt, countersFound := input.State.ConstructionOffersFor(castle.ID, castle.KingdomID)
	if requireCurrentCounters && (!countersFound || observedAt.IsZero()) {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Luna package purchase counters are not current for Storm castle %d", castle.ID)
	}
	purchases := make([]stormShopPurchaseLine, 0, len(normalized))
	totalCost := int64(0)
	for _, line := range normalized {
		item, found := input.GameData.StormShopPackage(int64(line.ProductID))
		if !found {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("package %d is not sold by Luna's trade boat", line.ProductID)
		}
		if line.Amount > (math.MaxInt64-totalCost)/item.AquamarinePrice {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm shop amount is too large")
		}
		totalCost += line.Amount * item.AquamarinePrice
		if requireCurrentCounters && item.Stock > 0 {
			purchased := offers[line.ProductID]
			if line.Amount > max(int64(0), item.Stock-purchased) {
				return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("package %d has only %d purchases remaining", line.ProductID, max(int64(0), item.Stock-purchased))
			}
		}
		purchases = append(purchases, stormShopPurchaseLine{request: line, item: item})
	}
	if totalCost > math.MaxInt64-request.AquamarineReserve {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm shop amount is too large")
	}
	required := totalCost + request.AquamarineReserve
	available := int64(math.Floor(castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount))
	if available < required {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, fmt.Errorf("Storm castle has %d Aquamarine; purchases and reserve require %d", available, required)
	}
	request.Purchases = normalized
	return request, castle, purchases, nil
}

func validateStormDefenseUnits(units []stormDefenseUnit) error {
	if len(units) > stormIntentMaximumSupport {
		return fmt.Errorf("Storm island defense may contain at most %d unit stacks", stormIntentMaximumSupport)
	}
	seen := map[State.UnitID]struct{}{}
	for _, unit := range units {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return fmt.Errorf("Storm island defense unit ids and amounts must be positive")
		}
		if _, duplicate := seen[unit.UnitID]; duplicate {
			return fmt.Errorf("Storm island defense unit %d is duplicated", unit.UnitID)
		}
		seen[unit.UnitID] = struct{}{}
	}
	return nil
}

func (application *Application) beginStormScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := stormMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	if !request.FullMap {
		return nil
	}
	if request.Cooperative {
		// The process-owned lease is the attempt record. Keeping contributor,
		// lease, and partial-coverage metadata out of account GameState prevents
		// both duplication and cross-account attribution leaks.
		return nil
	}
	startedAt := stormScanStartedAt(request)
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStorm), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("Storm source castle %d is no longer focused", request.SourceCastleID)
		}
		if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) {
			gameState.ReplaceStormMap(State.StormMapState{
				ServerURL:      gameState.Session.ServerURL,
				PlayerID:       gameState.Player.ID,
				SourceCastleID: request.SourceCastleID,
				Targets:        map[string]State.MapObservation{},
			})
		}
		if gameState.Storm.Map.LastAttemptAt.Equal(startedAt) {
			return nil, false, nil
		}
		gameState.Storm.Map.ServerURL = gameState.Session.ServerURL
		gameState.Storm.Map.PlayerID = gameState.Player.ID
		gameState.Storm.Map.SourceCastleID = request.SourceCastleID
		gameState.Storm.Map.NextBounds = request.Bounds
		gameState.Storm.Map.LastAttemptAt = startedAt
		return []string{"storm"}, true, nil
	})
	return err
}

func (application *Application) burstStormMapScan(ctx context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.Session == nil || application.Ingest == nil {
		return fmt.Errorf("Storm map burst dependencies are unavailable")
	}
	request, source, err := stormMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	if !request.FullMap {
		return fmt.Errorf("Storm map burst requires a full-map scan")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	burstContext, cancelBurst := context.WithTimeout(ctx, stormMapBurstResponseTimeout)
	defer cancelBurst()

	startedAt := stormScanStartedAt(request)
	if request.Cooperative {
		windows := stormCooperativeScanWindows(request.Windows)
		timeout := stormMapBurstRemaining(burstContext)
		if timeout <= 0 {
			return fmt.Errorf("cooperative Storm scan exceeded the %s response deadline: %w", stormMapBurstResponseTimeout, burstContext.Err())
		}
		if err := runStormMapGAABurst(
			burstContext, application.Session, application.Ingest, source.KingdomID, windows, timeout,
		); err != nil {
			if application.WorldMaps != nil {
				application.WorldMaps.ReleaseStormScan(application.AccountKey, request.LeaseID)
			}
			return fmt.Errorf("scan leased Storm map windows: %w", err)
		}
		return application.captureStormScanRequest(request)
	}
	for ring := 0; ; ring++ {
		windows := stormMapConcentricRingWindows(ring)
		if len(windows) == 0 {
			return fmt.Errorf(
				"Storm map targets still touch the %d-coordinate safety margin at the maximum concentric scan bounds",
				stormMapEdgeBuffer,
			)
		}
		timeout := stormMapBurstRemaining(burstContext)
		if timeout <= 0 {
			return fmt.Errorf("Storm map concentric sweep exceeded the %s response deadline: %w", stormMapBurstResponseTimeout, burstContext.Err())
		}
		if err := runStormMapGAABurst(
			burstContext, application.Session, application.Ingest, source.KingdomID, windows, timeout,
		); err != nil {
			return fmt.Errorf("scan Storm map ring %d: %w", ring, err)
		}
		request.Bounds = stormMapConcentricBounds(ring)
		if !stormMapSweepNeedsExpansion(application.State.ReadOnlyView(), request.Bounds, startedAt) {
			return application.captureStormScanRequest(request)
		}
	}
}

func stormMapBurstRemaining(ctx context.Context) time.Duration {
	if deadline, exists := ctx.Deadline(); exists {
		return time.Until(deadline)
	}
	return stormMapBurstResponseTimeout
}

func runStormMapGAABurst(
	ctx context.Context,
	sender stormMapBurstSender,
	observer stormMapBurstObserver,
	kingdomID State.KingdomID,
	windows []towerMapWindow,
	timeout time.Duration,
) error {
	if sender == nil || observer == nil {
		return fmt.Errorf("Storm map burst sender and response observer are required")
	}
	if !sender.CorrelatesResponses() {
		return fmt.Errorf("Storm map burst requires correlated websocket responses")
	}
	if len(windows) == 0 {
		return fmt.Errorf("Storm map burst has no windows")
	}
	if timeout <= 0 {
		return fmt.Errorf("Storm map burst timeout must be positive")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	burstContext, cancelBurst := context.WithTimeout(ctx, timeout)
	defer cancelBurst()

	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		operationID = "storm-map"
	}
	tokenRoot := fmt.Sprintf("%s/storm-gaa/%d", operationID, time.Now().UTC().UnixNano())
	slots := make([]stormMapBurstSlot, len(windows))
	defer func() {
		for _, slot := range slots {
			if slot.cancel != nil {
				slot.cancel()
			}
		}
	}()
	for index, window := range windows {
		payload, err := json.Marshal(struct {
			KingdomID State.KingdomID `json:"KID"`
			X1        int             `json:"AX1"`
			Y1        int             `json:"AY1"`
			X2        int             `json:"AX2"`
			Y2        int             `json:"AY2"`
		}{kingdomID, window.X1, window.Y1, window.X2, window.Y2})
		if err != nil {
			return fmt.Errorf("encode Storm map window %d/%d: %w", index+1, len(windows), err)
		}
		wire, err := Protocol.Encode(Protocol.Command{
			Namespace: sender.Namespace(), Opcode: "gaa", Payload: payload,
		})
		if err != nil {
			return fmt.Errorf("build Storm map window %d/%d: %w", index+1, len(windows), err)
		}
		token := fmt.Sprintf("%s/%d", tokenRoot, index+1)
		frames, cancel := observer.WatchWireResponse("gaa", token)
		slots[index] = stormMapBurstSlot{
			window: window, token: token, wire: wire, frames: frames, cancel: cancel,
		}
	}

	results := make(chan stormMapBurstResult, len(slots))
	for index := range slots {
		slot := slots[index]
		go func() {
			select {
			case frame := <-slot.frames:
				results <- stormMapBurstResult{index: index, frame: frame}
			case <-burstContext.Done():
			}
		}()
	}

	timeoutMillis := int(timeout / time.Millisecond)
	baseMetadata := Outbound.MetadataFromContext(burstContext)
	for index, slot := range slots {
		metadata := baseMetadata
		metadata.ResponseToken = slot.token
		metadata.ResponseOpcodes = []string{"gaa"}
		metadata.ResponseTimeoutMillis = timeoutMillis
		sendContext := Outbound.WithMetadata(burstContext, metadata)
		for {
			err := sender.Send(sendContext, slot.wire)
			if err == nil {
				break
			}
			if !errors.Is(err, Outbound.ErrAutomationLocked) || Outbound.IsIndeterminate(err) {
				return fmt.Errorf("send Storm map window %d/%d: %w", index+1, len(slots), err)
			}
			if err := sender.WaitForAutomationUnlocked(burstContext); err != nil {
				return fmt.Errorf(
					"Storm map GAA burst timed out while paused before window %d/%d: %w",
					index+1, len(slots), err,
				)
			}
		}
	}

	received := make([]Protocol.CommittedFrame, len(slots))
	seen := make([]bool, len(slots))
	pendingCommits := map[uint64]struct{}{}
	defer func() {
		for ingressID := range pendingCommits {
			observer.ForgetCommitted(ingressID)
		}
	}()
	for responseCount := 0; responseCount < len(slots); {
		select {
		case <-burstContext.Done():
			return fmt.Errorf(
				"Storm map GAA burst received %d/%d responses before the %s deadline: %w",
				responseCount, len(slots), timeout, burstContext.Err(),
			)
		case result := <-results:
			if result.index < 0 || result.index >= len(slots) || seen[result.index] {
				return fmt.Errorf("Storm map GAA response aggregation received a duplicate or invalid slot")
			}
			if result.frame.Frame.ResponseToken != slots[result.index].token {
				return fmt.Errorf("Storm map GAA response token changed for window %d/%d", result.index+1, len(slots))
			}
			seen[result.index] = true
			received[result.index] = result.frame
			pendingCommits[result.frame.IngressID] = struct{}{}
			responseCount++
		}
	}
	for index, response := range received {
		committed, err := observer.WaitCommitted(burstContext, response.IngressID)
		delete(pendingCommits, response.IngressID)
		if err != nil {
			return fmt.Errorf("commit Storm map window %d/%d: %w", index+1, len(slots), err)
		}
		if committed.Frame.ResponseCode == nil {
			return fmt.Errorf("Storm map window %d/%d response did not include a result code", index+1, len(slots))
		}
		if *committed.Frame.ResponseCode != 0 {
			return fmt.Errorf(
				"Storm map window %d/%d response code %d was not successful",
				index+1, len(slots), *committed.Frame.ResponseCode,
			)
		}
		if committed.ReduceError != "" {
			return fmt.Errorf(
				"Storm map window %d/%d response state reduction failed: %s",
				index+1, len(slots), committed.ReduceError,
			)
		}
	}
	return nil
}

func (application *Application) captureStormScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := stormMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	return application.captureStormScanRequest(request)
}

func (application *Application) captureStormScanRequest(request stormMapScanRequest) error {
	if request.Cooperative {
		if application.WorldMaps == nil || strings.TrimSpace(application.AccountKey) == "" {
			return fmt.Errorf("shared Storm scan coordinator is unavailable")
		}
		startedAt := stormScanStartedAt(request)
		completedAt := time.Now().UTC()
		state := application.State.ReadOnlyView()
		worldID := strings.TrimSpace(state.Account.WorldID)
		if worldID == "" {
			worldID = strings.TrimSpace(state.Session.ServerURL)
		}
		worldEvent, err := application.WorldMaps.CompleteStormScan(
			application.AccountKey, worldID, stormIntentKingdomID, request.LeaseID,
			request.Windows, startedAt, completedAt,
		)
		if err != nil {
			return err
		}
		// Adopt immediately instead of waiting for supervisor fan-out. No
		// contributor identity is retained in the generation or emitted patch.
		application.State.AdoptWorldMap(worldEvent)
		_, err = application.State.ApplyComponents(State.Components(State.ComponentStorm), func(gameState *State.GameState) ([]string, bool, error) {
			changed := false
			if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) {
				// Cooperative scans skip the burst-path initialization, so a
				// hosted account's map identity would stay zero forever — and
				// every later targeted pre-attack refresh would be refused
				// with "Storm map identity changed". Bind the identity to
				// this session the first time shared coverage lands (and
				// again after a server, player, or source-castle change).
				gameState.ReplaceStormMap(State.StormMapState{
					ServerURL:      gameState.Session.ServerURL,
					PlayerID:       gameState.Player.ID,
					SourceCastleID: request.SourceCastleID,
					Targets:        map[string]State.MapObservation{},
				})
				changed = true
			}
			lastScannedAt := gameState.MutableStormLastScannedAt()
			if !lastScannedAt[request.SourceCastleID].Equal(completedAt) {
				lastScannedAt[request.SourceCastleID] = completedAt
				changed = true
			}
			if !changed {
				return nil, false, nil
			}
			return []string{"storm", "storm-scan"}, true, nil
		})
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(
		State.ComponentStorm, State.ComponentWorldMap,
	), func(gameState *State.GameState) ([]string, bool, error) {
		if _, exists := gameState.Castles[request.SourceCastleID]; !exists {
			return nil, false, fmt.Errorf("Storm source castle %d is no longer available", request.SourceCastleID)
		}
		lastScannedAt := gameState.MutableStormLastScannedAt()
		startedAt := stormScanStartedAt(request)
		completedAt := time.Now().UTC()
		if !request.FullMap {
			changed := !lastScannedAt[request.SourceCastleID].Equal(completedAt)
			lastScannedAt[request.SourceCastleID] = completedAt
			if request.Targeted {
				if stormMapIdentityUnbound(gameState.Storm.Map) {
					// Cooperative-only accounts may reach their first targeted
					// refresh before any local capture bound the identity
					// (shared coverage persists across restarts, so the scan
					// lane can stay idle for hours). An unbound identity is
					// not a change — bind it now and continue.
					gameState.Storm.Map.ServerURL = gameState.Session.ServerURL
					gameState.Storm.Map.PlayerID = gameState.Player.ID
					gameState.Storm.Map.SourceCastleID = request.SourceCastleID
					changed = true
				} else if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) {
					return nil, false, fmt.Errorf("Storm map identity changed before targeted refresh capture")
				}
				gameState.RangeStormTargets(func(key string, tracked State.MapObservation) bool {
					if !request.Bounds.Contains(tracked.X, tracked.Y) {
						return true
					}
					observation, exists := gameState.LookupMapObservation(stormIntentKingdomID, key)
					if !exists || observation.ObservedAt.Before(startedAt) ||
						(observation.TypeID != stormIntentIslandMapTypeID && observation.TypeID != stormIntentFortMapTypeID) {
						if gameState.DeleteStormTarget(key) {
							changed = true
						}
					}
					return true
				})
			}
			return []string{"storm", "map-storm"}, changed, nil
		}
		if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) ||
			!gameState.Storm.Map.LastAttemptAt.Equal(startedAt) {
			return nil, false, fmt.Errorf("Storm map sweep identity changed before capture")
		}
		if !gameState.Storm.Map.LastCompletedAt.IsZero() && !gameState.Storm.Map.LastCompletedAt.Before(startedAt) {
			return nil, false, nil
		}

		targets := map[string]State.MapObservation{}
		staleKeys := []string{}
		gameState.RangeMapObservationsByKind(stormIntentKingdomID, State.MapProjectionStorm, func(key string, observation State.MapObservation) bool {
			if !request.Bounds.Contains(observation.X, observation.Y) {
				return true
			}
			if observation.ObservedAt.Before(startedAt) {
				if observation.TypeID == stormIntentIslandMapTypeID || observation.TypeID == stormIntentFortMapTypeID {
					staleKeys = append(staleKeys, key)
				}
				return true
			}
			if observation.TypeID == stormIntentIslandMapTypeID || observation.TypeID == stormIntentFortMapTypeID {
				targets[key] = observation
			}
			return true
		})
		for _, key := range staleKeys {
			gameState.DeleteMapObservation(stormIntentKingdomID, key)
		}

		gameState.ReplaceStormMap(State.StormMapState{
			ServerURL:       gameState.Session.ServerURL,
			PlayerID:        gameState.Player.ID,
			SourceCastleID:  request.SourceCastleID,
			CoveredBounds:   request.Bounds,
			NextBounds:      request.Bounds,
			LastAttemptAt:   startedAt,
			LastCompletedAt: completedAt,
			WindowCount:     len(stormMapScanWindows(request.Bounds)),
			Targets:         targets,
		})
		lastScannedAt[request.SourceCastleID] = completedAt
		return []string{"storm", "map-storm"}, true, nil
	})
	return err
}

func stormScanStartedAt(request stormMapScanRequest) time.Time {
	startedAt := request.ScanStartedAt.UTC()
	if startedAt.IsZero() {
		startedAt = time.Now().UTC()
	}
	return startedAt
}

func stormMapStateMatches(gameState State.GameState, mapState State.StormMapState, sourceCastleID State.CastleID) bool {
	return mapState.ServerURL == gameState.Session.ServerURL && mapState.PlayerID == gameState.Player.ID &&
		mapState.SourceCastleID == sourceCastleID
}

// stormMapIdentityUnbound reports a map that has never been bound to any
// session — the state of a cooperative-only (hosted) account before its
// first local capture.
func stormMapIdentityUnbound(mapState State.StormMapState) bool {
	return mapState.ServerURL == "" && mapState.PlayerID == 0 && mapState.SourceCastleID == 0
}

func (application *Application) guardStormAttack(_ context.Context, arguments json.RawMessage) error {
	var request resolvedStormAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, source, target, _, err := stormAttackContext(
		Intent.PlanningContext{State: state, GameData: gameData}, mustMarshalStormAttackRequest(request.stormAttackRequest),
	)
	if err != nil {
		return err
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID)
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != target.TypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y ||
		dialog.Target.StormIsleID > 0 && dialog.Target.StormIsleID != target.StormIsleID {
		return fmt.Errorf("current attack dialog does not match Storm target %d:%d", target.X, target.Y)
	}
	return nil
}

func (application *Application) resolveStormAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request resolvedStormAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	attackRequest, source, target, definition, err := stormAttackContext(input, mustMarshalStormAttackRequest(request.stormAttackRequest))
	if err != nil {
		return Intent.Step{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Intent.Step{}, fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID)
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("storm:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: definition.Level, VictoryCount: target.StormVictoryCount,
			},
			Level: definition.Level, CastleTypeID: target.TypeID, PvP: definition.Kind == GameData.StormIsleKindIsland,
		},
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("resolve Storm attack capacity: %w", err)
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(attackRequest.Preset, capacity))
	if definition.Kind == GameData.StormIsleKindIsland {
		setup.CourtyardSupport.Troops = nil
	}
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Storm preset %q: %w", attackRequest.Preset.Name, err)
	}
	body := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&body, input.GameData, source, attackRequest.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve Storm horse travel boost: %w", err)
	}
	if definition.Kind == GameData.StormIsleKindIsland {
		body.SupportTroops = stormSupportTroops(attackRequest.DefenseUnits)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Storm CRA payload: %w", err)
	}
	if err := validateCRAInventoryPayloads([]json.RawMessage{payload}, source); err != nil {
		return Intent.Step{}, err
	}
	return commandStep(fmt.Sprintf("Attack Storm %s at %d:%d", definition.Kind, target.X, target.Y), "cra", payload, "cra"), nil
}

func stormSupportTroops(units []stormDefenseUnit) []attackPair {
	ordered := append([]stormDefenseUnit(nil), units...)
	sort.Slice(ordered, func(left, right int) bool { return ordered[left].UnitID < ordered[right].UnitID })
	empty := attackPair{-1, 0}
	result := make([]attackPair, stormIntentMaximumSupport)
	for index := range result {
		result[index] = empty
	}
	for index, unit := range ordered {
		result[index] = attackPair{int64(unit.UnitID), unit.Amount}
	}
	return result
}

func (application *Application) consumeStormTarget(_ context.Context, arguments json.RawMessage) error {
	var request stormTargetConsumeRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || request.KingdomID != stormIntentKingdomID ||
		(request.TargetTypeID != stormIntentIslandMapTypeID && request.TargetTypeID != stormIntentFortMapTypeID) {
		return fmt.Errorf("Storm target consumption requires a valid source and target identity")
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID &&
		(request.IslandObjectID <= 0 || request.LeaveBehind < 0 || request.LeaveBehind > 1) {
		return fmt.Errorf("Storm island consumption requires an object id and valid occupation count")
	}
	_, err := application.State.ApplyComponents(State.Components(
		State.ComponentStorm,
	), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || source.KingdomID != stormIntentKingdomID {
			return nil, false, fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID)
		}
		key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
		changed := gameState.DeleteStormTarget(key)
		if request.TargetTypeID == stormIntentIslandMapTypeID {
			returnKey := State.StormIslandReturnKey(request.KingdomID, request.TargetX, request.TargetY)
			gameState.MutableStormIslandReturns()[returnKey] = State.StormIslandReturnState{
				KingdomID:      request.KingdomID,
				SourceCastleID: request.SourceCastleID,
				TargetX:        request.TargetX,
				TargetY:        request.TargetY,
				IslandObjectID: request.IslandObjectID,
				Status:         State.StormIslandReturnAwaitingReport,
				LeaveBehind:    request.LeaveBehind,
				Survivors:      map[State.UnitID]int64{},
				LaunchedAt:     time.Now().UTC(),
			}
			changed = true
		}
		if !changed {
			return nil, false, nil
		}
		return []string{"storm"}, true, nil
	})
	return err
}

func (application *Application) guardStormIslandReturn(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, _, _, err := stormIslandReturnContext(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, arguments)
	return err
}

func (application *Application) completeStormIslandReturn(_ context.Context, arguments json.RawMessage) error {
	var request stormIslandReturnRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	key := State.StormIslandReturnKey(request.KingdomID, request.IslandX, request.IslandY)
	_, err := application.State.ApplyComponents(State.Components(State.ComponentStorm), func(gameState *State.GameState) ([]string, bool, error) {
		returns := gameState.MutableStormIslandReturns()
		operation, exists := returns[key]
		if !exists {
			return nil, false, nil
		}
		if operation.Status != State.StormIslandReturnReady || operation.SourceCastleID != request.SourceCastleID ||
			operation.IslandObjectID != request.IslandObjectID || operation.ReportID != request.ReportID {
			return nil, false, fmt.Errorf("Storm island return identity changed before completion")
		}
		delete(returns, key)
		return []string{"storm"}, true, nil
	})
	return err
}

func (application *Application) guardStormShopPurchase(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, _, _, err := stormShopPurchaseContext(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, arguments, true)
	return err
}

func stormTargetCooldownRemaining(target State.MapObservation, now time.Time) int {
	if target.TypeID == stormIntentIslandMapTypeID && target.OwnerID <= 0 {
		return 0
	}
	readyAt := stormTargetReadyAt(target)
	if readyAt.IsZero() || !readyAt.After(now) {
		return 0
	}
	return int((readyAt.Sub(now) + time.Second - 1) / time.Second)
}

func stormTargetReadyAt(target State.MapObservation) time.Time {
	return target.StormReadyAt()
}

func stormIslandUnavailable(target State.MapObservation, now time.Time) bool {
	if target.OwnerID <= 0 {
		return false
	}
	readyAt := stormTargetReadyAt(target)
	return readyAt.IsZero() || readyAt.After(now)
}

func stormTargetExpired(target State.MapObservation, definition GameData.StormIsleDefinition, now time.Time) bool {
	expiresAt := target.StormExpiresAt(definition.GlobalCooldownSec)
	return !expiresAt.IsZero() && !expiresAt.After(now)
}

func stormAttackDialogUnavailable(target State.AttackDialogTarget) bool {
	switch target.TypeID {
	case stormIntentFortMapTypeID:
		return target.StormCooldownRemaining > 0
	case stormIntentIslandMapTypeID:
		return target.OwnerID > 0
	default:
		return target.StormCooldownRemaining > 0
	}
}

func mustMarshalStormAttackRequest(request stormAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

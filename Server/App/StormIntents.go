package App

import (
	"CitadelDesktop/Server/Localization"
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

type mapGAASender interface {
	CorrelatesResponses() bool
	Namespace() string
	Send(context.Context, []byte) error
	WaitForAutomationUnlocked(context.Context) error
}

type mapGAAObserver interface {
	ForgetCommitted(uint64)
	WaitCommitted(context.Context, uint64) (Protocol.CommittedFrame, error)
	WatchWireResponse(string, string) (<-chan Protocol.CommittedFrame, func())
}

type stormMapBurstSlot struct {
	token string
	wire  []byte
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
	MinimumTroops       int64                `json:"minimumTroops,omitempty"`
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
			Name: "storm.castle.refresh", Description: "Refresh the authoritative owned-castle directory after a Storm unlock", DescriptionDescriptor: Localization.New("server.intent.description.4e866c7b", "Refresh the authoritative owned-castle directory after a Storm unlock", nil), Effect: Intent.EffectRead,
			Planner: planStormCastleRefresh,
		},
		{
			Name: "storm.castle.unlock", Description: "Open one explicitly selected official prebuilt Storm castle", DescriptionDescriptor: Localization.New("server.intent.description.f2fc2eb1", "Open one explicitly selected official prebuilt Storm castle", nil), Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"prebuiltCastleId":16}`), Planner: planStormCastleUnlock,
		},
		{
			Name: "storm.map.scan", Description: "Focus the Storm castle and refresh an adaptive center-out map snapshot", DescriptionDescriptor: Localization.New("server.intent.description.0709c508", "Focus the Storm castle and refresh an adaptive center-out map snapshot", nil), Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"fullMap":true,"bounds":{"x1":600,"y1":600,"x2":700,"y2":700}}`), Planner: planStormMapScan,
		},
		{
			Name: "storm.attack", Description: "Launch a guarded CitadelOps preset against an eligible Storm fort or resource island", DescriptionDescriptor: Localization.New("server.intent.description.fe841621", "Launch a guarded CitadelOps preset against an eligible Storm fort or resource island", nil), Effect: Intent.EffectLaunch,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"kingdomId":4,"targetTypeId":25,"targetX":500,"targetY":500,"stormIsleId":10,"minimumVictoryCount":5,"preset":{"id":"storm-fort","name":"Storm fort","waves":[]}}`),
			AttackModule:     &Intent.AttackModuleDefinition{ID: "autoStorm", Label: "Auto Storm", Description: "Storm fort and resource-island attacks", DefaultWeight: 50},
			Planner:          planStormAttack,
		},
		{
			Name: "storm.island.return", Description: "Return report-confirmed surviving troops from an occupied Storm island to the Storm castle", DescriptionDescriptor: Localization.New("server.intent.description.5c5db776", "Return report-confirmed surviving troops from an occupied Storm island to the Storm castle", nil), Effect: Intent.EffectLaunch,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"kingdomId":4,"islandX":500,"islandY":500,"islandObjectId":15532314,"reportId":42,"units":[{"unitId":10,"amount":99}]}`),
			Planner:          planStormIslandReturn,
		},
		{
			Name: "storm.shop.purchase", Description: "Buy one or more official Luna trade-boat packages while preserving an Aquamarine reserve", DescriptionDescriptor: Localization.New("server.intent.description.ebf593c0", "Buy one or more official Luna trade-boat packages while preserving an Aquamarine reserve", nil), Effect: Intent.EffectWrite,
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the current player identity is unavailable"), Localization.New("server.app.the_current_player_identity.362340dd", "the current player identity is unavailable", nil))
	}
	payload, _ := json.Marshal(struct {
		PlayerID State.PlayerID `json:"PID"`
	}{input.State.Player.ID})
	step := commandStep("Refresh the owned-castle directory", "gcl", payload, "gcl", Localization.New("server.app.refresh_the_owned_castle.3d4f20ea", "Refresh the owned-castle directory", nil))
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims:  []string{"castle-directory", "kingdom:" + strconv.FormatInt(int64(stormIntentKingdomID), 10)},
		Summary: "Refresh the owned-castle directory for Storm", SummaryDescriptor: Localization.New("server.app.refresh_the_owned_castle.9ab9f250", "Refresh the owned-castle directory for Storm", nil),
		Steps: []Intent.Step{step},
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
	unlockStep := commandStep("Open the selected Storm castle", "ksc", payload, "ksc", Localization.New("server.app.open_the_selected_storm.11b54358", "Open the selected Storm castle", nil))
	unlockStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	castleListStep := commandStep("Refresh the new Storm castle", "gcl", castleListPayload, "gcl", Localization.New("server.app.refresh_the_new_storm.2a8e1eb3", "Refresh the new Storm castle", nil))
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
				Name: "Verify refreshed Storm castle availability", NameDescriptor: Localization.New("server.app.verify_refreshed_storm_castle.a19f700d", "Verify refreshed Storm castle availability", nil), Action: "storm.castle.unlock.verify",
				ActionArguments: guardArguments,
			}),
			unlockStep,
			castleListStep,
			contextCommandStep("Refresh Storm kingdom state", "kpi", json.RawMessage(`{}`), "kpi").WithNameDescriptor(Localization.New("server.app.refresh_storm_kingdom_state.0012e929", "Refresh Storm kingdom state", nil)),
		},
	}, nil
}

func stormCastleUnlockOption(
	input Intent.PlanningContext,
	prebuiltCastleID int64,
	refreshedAfter time.Time,
) (GameData.StormCastleOption, error) {
	if input.GameData == nil {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if input.State.Player.ID <= 0 {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("the current player identity is unavailable"), Localization.New("server.app.the_current_player_identity.362340dd", "the current player identity is unavailable", nil))
	}
	if _, exists := ownedCastleInKingdom(input.State, stormIntentKingdomID); exists {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("%w: an owned Storm castle already exists", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.0639ab4f", "intent plan became stale before dispatch: an owned Storm castle already exists", nil))
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[stormIntentKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("%w: Storm kingdom availability has not been observed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.66548733", "intent plan became stale before dispatch: Storm kingdom availability has not been observed", nil))
	}
	if !refreshedAfter.IsZero() && input.State.KingdomTransport.ObservedAt.Before(refreshedAfter) {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("the Storm kingdom list was not refreshed before opening the castle"), Localization.New("server.app.the_storm_kingdom_list.a7947fae", "the Storm kingdom list was not refreshed before opening the castle", nil))
	}
	if unlock.Unlocked || unlock.Created {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("%w: the Storm kingdom is already unlocked; refresh the castle directory instead", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4d56eef2", "intent plan became stale before dispatch: the Storm kingdom is already unlocked; refresh the castle directory instead", nil))
	}
	option, found := input.GameData.StormCastleOption(prebuiltCastleID, input.State.Player.Level)
	if !found {
		return GameData.StormCastleOption{}, Localization.WithError(fmt.Errorf("prebuiltCastleId must identify an unlocked official Storm castle option"), Localization.New("server.app.prebuiltcastleid_must_identify_an.8b7dc413", "prebuiltCastleId must identify an unlocked official Storm castle option", nil))
	}
	return option, nil
}

func (application *Application) verifyStormCastleUnlock(_ context.Context, arguments json.RawMessage) error {
	var request stormCastleUnlockGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.PrebuiltCastleID <= 0 || request.RefreshStartedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Storm castle unlock verification requires an option and refresh time"), Localization.New("server.app.storm_castle_unlock_verification.89086442", "Storm castle unlock verification requires an option and refresh time", nil))
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Storm castle unlock state is unavailable"), Localization.New("server.app.storm_castle_unlock_state.a8f854f3", "Storm castle unlock state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("full Storm map sweeps are limited to one attempt every two hours"), Localization.New("server.app.full_storm_map_sweeps.c8a3f1f7", "full Storm map sweeps are limited to one attempt every two hours", nil))
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
			Name: "Record Storm map sweep attempt", NameDescriptor: Localization.New("server.app.record_storm_map_sweep.7eca9431", "Record Storm map sweep attempt", nil), Action: "storm.scan.begin", ActionArguments: normalizedArguments,
		})
		steps = append(steps, Intent.Step{
			Name: "Burst Storm map windows", NameDescriptor: Localization.New("server.app.burst_storm_map_windows.4f33423f", "Burst Storm map windows", nil), Action: "storm.scan.burst", ActionArguments: normalizedArguments,
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
				fmt.Sprintf("Refresh Storm map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa", Localization.New("server.app.refresh_storm_map_window.3789ed39", "Refresh Storm map window {p0, number}/{p1, number}", Localization.Params{"p0": index + 1, "p1": len(windows)}),
			)
			if index > 0 {
				step.DelayMillis = stormMapWindowDelayMillis
			}
			steps = append(steps, step)
		}
	}
	if !request.FullMap {
		steps = append(steps, Intent.Step{Name: "Build Storm map state", NameDescriptor: Localization.New("server.app.build_storm_map_state.26b5098d", "Build Storm map state", nil), Action: "storm.scan.capture", ActionArguments: normalizedArguments})
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	summary := fmt.Sprintf("Refresh Storm forts and resource islands around %s", castleLabel(source))
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.refresh_storm_forts_and.6cd1e8d2", "Refresh Storm forts and resource islands around {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))})
	if request.FullMap {
		if request.Cooperative {
			summary = fmt.Sprintf("Refresh %d leased windows for shared Storm coverage", len(request.Windows))
			summaryLocalizationMessage = Localization.New("server.app.refresh_p_leased_windows.f111eaff", "Refresh {p0, number} leased windows for shared Storm coverage", Localization.Params{"p0": len(request.Windows)})
		} else {
			summary = fmt.Sprintf("Refresh the Storm map outward from %d:%d", stormMapCenterCoordinate, stormMapCenterCoordinate)
			summaryLocalizationMessage = Localization.New("server.app.refresh_the_storm_map.4d9c8bba", "Refresh the Storm map outward from {p0, number}:{p1, number}", Localization.Params{"p0": stormMapCenterCoordinate, "p1": stormMapCenterCoordinate})
		}
	} else if request.Targeted {
		summary = fmt.Sprintf("Refresh Storm target at %d:%d", request.Bounds.X1, request.Bounds.Y1)
		summaryLocalizationMessage = Localization.New("server.app.refresh_storm_target_at.f836deec", "Refresh Storm target at {p0, number}:{p1, number}", Localization.Params{"p0": request.Bounds.X1, "p1": request.Bounds.Y1})
	}
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage), Steps: steps,
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("no commanders are assigned to Auto Storm"), Localization.New("server.app.no_commanders_are_assigned.a7a2e2e1", "no commanders are assigned to Auto Storm", nil))
		}
		selection = &craCommanderSelectionRequest{Candidates: request.CommanderIDs, Count: 1, Strategy: "lowest_id"}
	}
	resolution, err := resolveCRACommanders(input.State, selection, craCommanderSelectionOptions{Holds: input.CommanderHolds, DefaultCount: 1, RequireAvailable: true})
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
		deferredCRACommandStep("Build and launch capacity-adjusted Storm attack", "storm.attack.build", resolvedArguments, contextPayload, Localization.New("server.app.build_and_launch_capacity.7801ee75", "Build and launch capacity-adjusted Storm attack", nil)),
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoStorm, SourceCastleID: source.ID, CommanderID: commanderID,
			KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
		Intent.Step{Name: "Consume Storm target", NameDescriptor: Localization.New("server.app.consume_storm_target.fde8cf55", "Consume Storm target", nil), Action: "storm.target.consume", ActionArguments: consumeArguments},
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
		Summary:   fmt.Sprintf("Attack Storm %s at %d:%d with %s", definition.Kind, target.X, target.Y, request.Preset.Name), SummaryDescriptor: Localization.New("server.app.attack_storm_p_at.4421a131", "Attack Storm {p0} at {p1}:{p2} with {p3}", Localization.Params{"p0": fmt.Sprintf("%s", definition.Kind), "p1": target.X, "p2": target.Y, "p3": fmt.Sprintf("%s", request.Preset.Name)}),
		Steps: steps,
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
		contextCommandStep("Preview island return route", "sdi", route, "sdi").WithNameDescriptor(Localization.New("server.app.preview_island_return_route.280f06d9", "Preview island return route", nil)),
		Intent.Step{Name: "Verify report-confirmed island survivors", NameDescriptor: Localization.New("server.app.verify_report_confirmed_island.add352f9", "Verify report-confirmed island survivors", nil), Action: "storm.island.return.guard", ActionArguments: arguments},
		commandStep("Return surviving island troops to Storm castle", "cds", dispatch, "cds", Localization.New("server.app.return_surviving_island_troops.44a6c20a", "Return surviving island troops to Storm castle", nil)),
		Intent.Step{Name: "Complete island troop return", NameDescriptor: Localization.New("server.app.complete_island_troop_return.ab63dd3f", "Complete island troop return", nil), Action: "storm.island.return.complete", ActionArguments: arguments},
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
		), SummaryDescriptor: Localization.New("server.app.return_p_surviving_troops.3580d3a6", "Return {p0} surviving troops from Storm island {p1}:{p2} after report {p3}, leaving {p4} occupier(s)", Localization.Params{"p0": returnTotal, "p1": request.IslandX, "p2": request.IslandY, "p3": fmt.Sprintf("%d", request.ReportID), "p4": operation.LeaveBehind}),
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
		shopCommandStep("Refresh Luna package purchase counters", "gbc", historyPayload, 0).WithNameDescriptor(Localization.New("server.app.refresh_luna_package_purchase.858325f5", "Refresh Luna package purchase counters", nil)),
		{Name: "Verify Storm Aquamarine reserve", NameDescriptor: Localization.New("server.app.verify_storm_aquamarine_reserve.aca840c3", "Verify Storm Aquamarine reserve", nil), Action: "storm.shop.guard", ActionArguments: arguments},
	}
	purchaseLabels := make([]string, 0, len(purchases))
	purchaseMessages := make([]*Localization.Message, 0, len(purchases))
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
		purchaseMessages = append(purchaseMessages, Localization.New("server.storm.purchase_list_item", "{amount, number} x Luna package {packageID}", Localization.Params{"amount": purchase.request.Amount, "packageID": strconv.FormatInt(int64(purchase.request.ProductID), 10)}))
		totalCost += purchase.request.Amount * purchase.item.AquamarinePrice
		steps = append(steps, shopCommandStep("Purchase "+itemName+" from Luna", "sbp", payload, 0).WithNameDescriptor(Localization.New("server.storm.purchase_step", "Purchase package {packageID} from Luna", Localization.Params{"packageID": strconv.FormatInt(int64(purchase.request.ProductID), 10)})))
	}
	summary := fmt.Sprintf("Buy %s from Luna for %d Aquamarine at %s", stormShopFriendlyList(purchaseLabels), totalCost, castleLabel(castle))
	return Intent.Plan{
		Claims: []string{
			"shop", "shop:table:" + strconv.FormatInt(GameData.StormLunaShopTableID, 10), "account-resources", "castle-focus",
			"castle:" + strconv.FormatInt(int64(request.CastleID), 10),
		},
		Summary:           summary,
		SummaryDescriptor: Localization.WithLists(Localization.New("server.storm.purchase_plan", "Buy {purchases} from Luna for {cost, number} Aquamarine at {castle}", Localization.Params{"cost": totalCost}), summary, map[string][]*Localization.Message{"purchases": purchaseMessages, "castle": {stormCastleIdentity(castle)}}),
		Steps:             steps,
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
		return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("Storm map scan requires a source castle"), Localization.New("server.app.storm_map_scan_requires.d5384a40", "Storm map scan requires a source castle", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != stormIntentKingdomID {
		return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("Storm source castle %d is unavailable in kingdom %d", request.SourceCastleID, stormIntentKingdomID), Localization.New("server.app.storm_source_castle_p.a33f4b7f", "Storm source castle {p0} is unavailable in kingdom {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID), "p1": fmt.Sprintf("%d", stormIntentKingdomID)}))
	}
	if request.FullMap && request.Cooperative {
		if strings.TrimSpace(request.LeaseID) == "" || len(request.Windows) == 0 || len(request.Windows) > stormMapMaximumWindowCount {
			return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("cooperative Storm scan requires one bounded shared lease"), Localization.New("server.app.cooperative_storm_scan_requires.4d849085", "cooperative Storm scan requires one bounded shared lease", nil))
		}
		covered := State.StormMapBounds{}
		for index, window := range request.Windows {
			if !window.IsValid() || window.X2 > stormMapMaximumCoordinate || window.Y2 > stormMapMaximumCoordinate ||
				len(stormMapScanWindows(window)) != 1 {
				return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("cooperative Storm scan contains an invalid map window"), Localization.New("server.app.cooperative_storm_scan_contains.5a68882c", "cooperative Storm scan contains an invalid map window", nil))
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
			return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("cooperative Storm scan bounds do not match its leased windows"), Localization.New("server.app.cooperative_storm_scan_bounds.601b3ae0", "cooperative Storm scan bounds do not match its leased windows", nil))
		}
	} else if request.FullMap {
		initialBounds := stormMapConcentricBounds(0)
		if request.Bounds != initialBounds {
			return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf(
				"full Storm map scan must start at %d:%d with bounds %d:%d through %d:%d",
				stormMapCenterCoordinate, stormMapCenterCoordinate,
				initialBounds.X1, initialBounds.Y1, initialBounds.X2, initialBounds.Y2,
			), Localization.New("server.app.full_storm_map_scan.569a2706", "full Storm map scan must start at {p0}:{p1} with bounds {p2}:{p3} through {p4}:{p5}", Localization.Params{"p0": stormMapCenterCoordinate, "p1": stormMapCenterCoordinate, "p2": initialBounds.X1, "p3": initialBounds.Y1, "p4": initialBounds.X2, "p5": initialBounds.Y2}))
		}
	} else if request.Targeted {
		windows := stormMapScanWindows(request.Bounds)
		if !request.Bounds.IsValid() || len(windows) != 1 || request.Bounds.X1 != request.Bounds.X2 || request.Bounds.Y1 != request.Bounds.Y2 {
			return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("targeted Storm map refresh requires one exact map coordinate"), Localization.New("server.app.targeted_storm_map_refresh.c493c536", "targeted Storm map refresh requires one exact map coordinate", nil))
		}
	} else if request.Radius < 1 || request.Radius > 50 {
		return stormMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("Storm map scan radius must be between 1 and 50"), Localization.New("server.app.storm_map_scan_radius.70d71b20", "Storm map scan radius must be between 1 and 50", nil))
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
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm attack requires a Storm source, target type, coordinates, and isle id"), Localization.New("server.app.storm_attack_requires_a.422156d7", "Storm attack requires a Storm source, target type, coordinates, and isle id", nil))
	}
	if input.GameData == nil {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != stormIntentKingdomID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.storm_source_castle_p.d6407401", "Storm source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
	target, exists := input.State.LookupMapObservation(request.KingdomID, key)
	mapStateCurrent := stormMapStateMatches(input.State, input.State.Storm.Map, source.ID)
	if mapStateCurrent && !input.State.Storm.Map.LastAttemptAt.IsZero() &&
		(input.State.Storm.Map.LastCompletedAt.IsZero() || input.State.Storm.Map.LastCompletedAt.Before(input.State.Storm.Map.LastAttemptAt)) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("the latest full Storm map sweep has not completed"), Localization.New("server.app.the_latest_full_storm.c1de6c66", "the latest full Storm map sweep has not completed", nil))
	}
	if !input.State.Storm.Map.LastCompletedAt.IsZero() && mapStateCurrent {
		scannedTarget, scanned := input.State.LookupStormTarget(key)
		if !scanned {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm target %d:%d is no longer in the authoritative map state", request.TargetX, request.TargetY), Localization.New("server.app.storm_target_p_p.a82fe9b2", "Storm target {p0}:{p1} is no longer in the authoritative map state", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
		}
		if !exists || target.ObservedAt.Before(scannedTarget.ObservedAt) {
			target = scannedTarget
			exists = true
		}
	}
	if !exists || target.TypeID != request.TargetTypeID || target.StormIsleID != request.StormIsleID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm target %d:%d is no longer the selected isle", request.TargetX, request.TargetY), Localization.New("server.app.storm_target_p_p.1f1d53ad", "Storm target {p0}:{p1} is no longer the selected isle", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	definition, found := input.GameData.StormIsleView(request.StormIsleID)
	if !found || definition.Kind == GameData.StormIsleKindFort && request.TargetTypeID != stormIntentFortMapTypeID ||
		definition.Kind == GameData.StormIsleKindIsland && request.TargetTypeID != stormIntentIslandMapTypeID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm isle %d does not match target type %d", request.StormIsleID, request.TargetTypeID), Localization.New("server.app.storm_isle_p_does.0dd2ad1a", "Storm isle {p0} does not match target type {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.StormIsleID), "p1": fmt.Sprintf("%d", request.TargetTypeID)}))
	}
	now := time.Now().UTC()
	if request.TargetTypeID == stormIntentIslandMapTypeID && stormIslandUnavailable(target, now) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm resource island %d:%d is already occupied", request.TargetX, request.TargetY), Localization.New("server.app.storm_resource_island_p.2177c4ca", "Storm resource island {p0}:{p1} is already occupied", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if stormTargetCooldownRemaining(target, now) > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm target %d:%d is still on cooldown", request.TargetX, request.TargetY), Localization.New("server.app.storm_target_p_p.a567def1", "Storm target {p0}:{p1} is still on cooldown", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && stormTargetExpired(target, definition, now) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm resource island %d:%d has expired from the learned map state", request.TargetX, request.TargetY), Localization.New("server.app.storm_resource_island_p.c5f4db8c", "Storm resource island {p0}:{p1} has expired from the learned map state", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && target.ObjectID <= 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("Storm resource island %d:%d has no object id for the post-capture troop return", request.TargetX, request.TargetY), Localization.New("server.app.storm_resource_island_p.7fe91a50", "Storm resource island {p0}:{p1} has no object id for the post-capture troop return", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if request.MinimumVictoryCount < 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("minimum attacks remaining cannot be negative"), Localization.New("server.app.minimum_attacks_remaining_cannot.65c669d6", "minimum attacks remaining cannot be negative", nil))
	}
	if request.MinimumTroops < 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("minimum troops cannot be negative"), Localization.New("server.app.minimum_troops_cannot_be.e04fd2a4", "minimum troops cannot be negative", nil))
	}
	awaitingReadyVerification := request.TargetTypeID == stormIntentFortMapTypeID && target.StormCooldownRemaining > 0 &&
		!stormTargetReadyAt(target).After(now)
	if request.TargetTypeID == stormIntentFortMapTypeID && request.MinimumVictoryCount > 0 && !awaitingReadyVerification {
		remaining, known := GameData.StormFortAttacksRemaining(definition, target.StormVictoryCount)
		if !known {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf(
				"Storm fort %d:%d has no authoritative attack limit", request.TargetX, request.TargetY,
			), Localization.New("server.app.storm_fort_p_p.d260fee9", "Storm fort {p0}:{p1} has no authoritative attack limit", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
		}
		if remaining < request.MinimumVictoryCount {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf(
				"Storm fort %d:%d has %d attacks remaining, below the required minimum %d",
				request.TargetX, request.TargetY, remaining, request.MinimumVictoryCount,
			), Localization.New("server.app.storm_fort_p_p.82dedac1", "Storm fort {p0}:{p1} has {p2} attacks remaining, below the required minimum {p3}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY, "p2": remaining, "p3": request.MinimumVictoryCount}))
		}
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && request.MinimumVictoryCount > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("minimum attacks remaining only applies to Storm forts"), Localization.New("server.app.minimum_attacks_remaining_only.10d7b0c0", "minimum attacks remaining only applies to Storm forts", nil))
	}
	if request.TargetTypeID == stormIntentFortMapTypeID && len(request.DefenseUnits) > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, Localization.WithError(fmt.Errorf("defense units can only accompany a resource-island occupation"), Localization.New("server.app.defense_units_can_only.13f28a6c", "defense units can only accompany a resource-island occupation", nil))
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
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return requires the Storm castle, island identity, report, and surviving units"), Localization.New("server.app.storm_island_return_requires.8030781a", "Storm island return requires the Storm castle, island identity, report, and surviving units", nil))
	}
	castle, exists := input.State.Castles[request.SourceCastleID]
	if !exists || castle.KingdomID != stormIntentKingdomID {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.storm_source_castle_p.d6407401", "Storm source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	key := State.StormIslandReturnKey(request.KingdomID, request.IslandX, request.IslandY)
	operation, exists := input.State.Storm.IslandReturns[key]
	if !exists || operation.Status != State.StormIslandReturnReady {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island %d:%d has no report-confirmed troop return ready", request.IslandX, request.IslandY), Localization.New("server.app.storm_island_p_p.8e2d28a1", "Storm island {p0}:{p1} has no report-confirmed troop return ready", Localization.Params{"p0": request.IslandX, "p1": request.IslandY}))
	}
	if operation.SourceCastleID != request.SourceCastleID || operation.KingdomID != request.KingdomID ||
		operation.TargetX != request.IslandX || operation.TargetY != request.IslandY ||
		operation.IslandObjectID != request.IslandObjectID || operation.ReportID != request.ReportID {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return identity changed after report %d", request.ReportID), Localization.New("server.app.storm_island_return_identity.0b6ccc30", "Storm island return identity changed after report {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.ReportID)}))
	}
	if input.GameData == nil {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	unitsCatalog, err := input.GameData.Catalog("units")
	if err != nil {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, err
	}
	requested := make(map[State.UnitID]int64, len(request.Units))
	for _, unit := range request.Units {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return unit ids and amounts must be positive"), Localization.New("server.app.storm_island_return_unit.104477a3", "Storm island return unit ids and amounts must be positive", nil))
		}
		if _, duplicate := requested[unit.UnitID]; duplicate {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return unit %d is duplicated", unit.UnitID), Localization.New("server.app.storm_island_return_unit.7df2d282", "Storm island return unit {p0} is duplicated", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		raw, found := unitsCatalog.Find(strconv.FormatInt(int64(unit.UnitID), 10))
		if !found {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("unit %d is not in the official unit catalog", unit.UnitID), Localization.New("server.app.unit_p_is_not.57d42843", "unit {p0} is not in the official unit catalog", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("decode unit %d: %w", unit.UnitID, decodeErr), Localization.ErrorContext(Localization.New("server.app.decode_unit_p.90388c2e", "decode unit {p0}", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}), decodeErr))
		}
		if GameData.IsToolRecord(record) {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("definition %d is a tool, not an island troop", unit.UnitID), Localization.New("server.app.definition_p_is_a.285acbd8", "definition {p0} is a tool, not an island troop", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		requested[unit.UnitID] = unit.Amount
	}
	expected := operation.UnitsToReturn()
	if len(requested) != len(expected) {
		return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return no longer matches report %d survivors", request.ReportID), Localization.New("server.app.storm_island_return_no.7c6fc497", "Storm island return no longer matches report {p0} survivors", Localization.Params{"p0": fmt.Sprintf("%d", request.ReportID)}))
	}
	for unitID, amount := range expected {
		if requested[unitID] != amount {
			return stormIslandReturnRequest{}, State.CastleState{}, State.StormIslandReturnState{}, Localization.WithError(fmt.Errorf("Storm island return no longer matches report %d survivors", request.ReportID), Localization.New("server.app.storm_island_return_no.7c6fc497", "Storm island return no longer matches report {p0} survivors", Localization.Params{"p0": fmt.Sprintf("%d", request.ReportID)}))
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
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm shop purchase requires a castle and a non-negative reserve"), Localization.New("server.app.storm_shop_purchase_requires.ec4a979b", "Storm shop purchase requires a castle and a non-negative reserve", nil))
	}
	requested := append([]stormShopPurchaseLineRequest(nil), request.Purchases...)
	if len(requested) == 0 && request.ProductID > 0 && request.Amount > 0 {
		requested = append(requested, stormShopPurchaseLineRequest{ProductID: request.ProductID, Amount: request.Amount})
	}
	normalized := make([]stormShopPurchaseLineRequest, 0, len(requested))
	lineByProduct := map[State.PackageID]int{}
	for _, line := range requested {
		if line.ProductID <= 0 || line.Amount <= 0 {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("each Storm shop purchase requires a product and positive amount"), Localization.New("server.app.each_storm_shop_purchase.5a2b503f", "each Storm shop purchase requires a product and positive amount", nil))
		}
		if index, exists := lineByProduct[line.ProductID]; exists {
			if normalized[index].Amount > math.MaxInt64-line.Amount {
				return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm shop amount is too large"), Localization.New("server.app.storm_shop_amount_is.16ee5f1d", "Storm shop amount is too large", nil))
			}
			normalized[index].Amount += line.Amount
			continue
		}
		lineByProduct[line.ProductID] = len(normalized)
		normalized = append(normalized, line)
	}
	if len(normalized) == 0 {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm shop purchase requires at least one package"), Localization.New("server.app.storm_shop_purchase_requires.f72ce911", "Storm shop purchase requires at least one package", nil))
	}
	if input.GameData == nil {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != stormIntentKingdomID {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("castle %d is not the current Storm castle", request.CastleID), Localization.New("server.app.castle_p_is_not.11aa6462", "castle {p0} is not the current Storm castle", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	offers, observedAt, countersFound := input.State.ConstructionOffersFor(castle.ID, castle.KingdomID)
	if requireCurrentCounters && (!countersFound || observedAt.IsZero()) {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Luna package purchase counters are not current for Storm castle %d", castle.ID), Localization.New("server.app.luna_package_purchase_counters.1f117d7c", "Luna package purchase counters are not current for Storm castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
	}
	purchases := make([]stormShopPurchaseLine, 0, len(normalized))
	totalCost := int64(0)
	for _, line := range normalized {
		item, found := input.GameData.StormShopPackage(int64(line.ProductID))
		if !found {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("package %d is not sold by Luna's trade boat", line.ProductID), Localization.New("server.app.package_p_is_not.7e02298e", "package {p0} is not sold by Luna's trade boat", Localization.Params{"p0": fmt.Sprintf("%d", line.ProductID)}))
		}
		if line.Amount > (math.MaxInt64-totalCost)/item.AquamarinePrice {
			return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm shop amount is too large"), Localization.New("server.app.storm_shop_amount_is.16ee5f1d", "Storm shop amount is too large", nil))
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
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm shop amount is too large"), Localization.New("server.app.storm_shop_amount_is.16ee5f1d", "Storm shop amount is too large", nil))
	}
	required := totalCost + request.AquamarineReserve
	available := int64(math.Floor(castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount))
	if available < required {
		return stormShopPurchaseRequest{}, State.CastleState{}, nil, Localization.WithError(fmt.Errorf("Storm castle has %d Aquamarine; purchases and reserve require %d", available, required), Localization.New("server.app.storm_castle_has_p.63b05463", "Storm castle has {p0} Aquamarine; purchases and reserve require {p1}", Localization.Params{"p0": available, "p1": required}))
	}
	request.Purchases = normalized
	return request, castle, purchases, nil
}

func validateStormDefenseUnits(units []stormDefenseUnit) error {
	if len(units) > stormIntentMaximumSupport {
		return Localization.WithError(fmt.Errorf("Storm island defense may contain at most %d unit stacks", stormIntentMaximumSupport), Localization.New("server.app.storm_island_defense_may.ab10a489", "Storm island defense may contain at most {p0} unit stacks", Localization.Params{"p0": stormIntentMaximumSupport}))
	}
	seen := map[State.UnitID]struct{}{}
	for _, unit := range units {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return Localization.WithError(fmt.Errorf("Storm island defense unit ids and amounts must be positive"), Localization.New("server.app.storm_island_defense_unit.bcabe39a", "Storm island defense unit ids and amounts must be positive", nil))
		}
		if _, duplicate := seen[unit.UnitID]; duplicate {
			return Localization.WithError(fmt.Errorf("Storm island defense unit %d is duplicated", unit.UnitID), Localization.New("server.app.storm_island_defense_unit.0cd00d65", "Storm island defense unit {p0} is duplicated", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
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
			return nil, false, Localization.WithError(fmt.Errorf("Storm source castle %d is no longer focused", request.SourceCastleID), Localization.New("server.app.storm_source_castle_p.b68ea826", "Storm source castle {p0} is no longer focused", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
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
		return Localization.WithError(fmt.Errorf("Storm map burst dependencies are unavailable"), Localization.New("server.app.storm_map_burst_dependencies.37a6a75c", "Storm map burst dependencies are unavailable", nil))
	}
	request, source, err := stormMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	if !request.FullMap {
		return Localization.WithError(fmt.Errorf("Storm map burst requires a full-map scan"), Localization.New("server.app.storm_map_burst_requires.5e4e8676", "Storm map burst requires a full-map scan", nil))
	}
	if ctx == nil {
		ctx = context.Background()
	}

	startedAt := stormScanStartedAt(request)
	var language *GameData.LanguageStore
	if application.GameData != nil {
		language, _ = application.GameData.Language()
	}
	if request.Cooperative {
		windows := stormCooperativeScanWindows(request.Windows)
		if err := runStormMapGAABurst(
			ctx, application.Session, application.Ingest, language, source.KingdomID, windows, stormMapBurstResponseTimeout,
		); err != nil {
			if application.WorldMaps != nil {
				application.WorldMaps.ReleaseStormScan(application.AccountKey, request.LeaseID)
			}
			return Localization.WithError(fmt.Errorf("scan leased Storm map windows: %w", err), Localization.ErrorContext(Localization.New("server.app.scan_leased_storm_map.b07fb31a", "scan leased Storm map windows", nil), err))
		}
		return application.captureStormScanRequest(request)
	}
	for ring := 0; ; ring++ {
		windows := stormMapConcentricRingWindows(ring)
		if len(windows) == 0 {
			return Localization.WithError(fmt.Errorf(
				"Storm map targets still touch the %d-coordinate safety margin at the maximum concentric scan bounds",
				stormMapEdgeBuffer,
			), Localization.New("server.app.storm_map_targets_still.56ef946f", "Storm map targets still touch the {p0}-coordinate safety margin at the maximum concentric scan bounds", Localization.Params{"p0": stormMapEdgeBuffer}))
		}
		if err := runStormMapGAABurst(
			ctx, application.Session, application.Ingest, language, source.KingdomID, windows, stormMapBurstResponseTimeout,
		); err != nil {
			return Localization.WithError(fmt.Errorf("scan Storm map ring %d: %w", ring, err), Localization.ErrorContext(Localization.New("server.app.scan_storm_map_ring.28204107", "scan Storm map ring {p0}", Localization.Params{"p0": ring}), err))
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

func stormMapBurstDeadline(responseTimeout time.Duration, windowCount int) time.Duration {
	return responseTimeout * time.Duration(windowCount+1)
}

func runStormMapGAABurst(
	ctx context.Context,
	sender mapGAASender,
	observer mapGAAObserver,
	language *GameData.LanguageStore,
	kingdomID State.KingdomID,
	windows []towerMapWindow,
	responseTimeout time.Duration,
) error {
	if sender == nil || observer == nil {
		return Localization.WithError(fmt.Errorf("Storm map burst sender and response observer are required"), Localization.New("server.app.storm_map_burst_sender.65363131", "Storm map burst sender and response observer are required", nil))
	}
	if !sender.CorrelatesResponses() {
		return Localization.WithError(fmt.Errorf("Storm map burst requires correlated websocket responses"), Localization.New("server.app.storm_map_burst_requires.4594017f", "Storm map burst requires correlated websocket responses", nil))
	}
	if len(windows) == 0 {
		return Localization.WithError(fmt.Errorf("Storm map burst has no windows"), Localization.New("server.app.storm_map_burst_has.a29563ed", "Storm map burst has no windows", nil))
	}
	if responseTimeout <= 0 {
		return Localization.WithError(fmt.Errorf("Storm map burst timeout must be positive"), Localization.New("server.app.storm_map_burst_timeout.cfe89c73", "Storm map burst timeout must be positive", nil))
	}
	if ctx == nil {
		ctx = context.Background()
	}
	overallTimeout := stormMapBurstDeadline(responseTimeout, len(windows))
	if overallTimeout <= 0 {
		return Localization.WithError(fmt.Errorf("Storm map burst timeout exceeds the supported duration"), Localization.New("server.app.storm_map_burst_timeout.32c6df20", "Storm map burst timeout exceeds the supported duration", nil))
	}
	burstContext, cancelBurst := context.WithTimeout(ctx, overallTimeout)
	defer cancelBurst()

	operationID := strings.TrimSpace(Outbound.MetadataFromContext(ctx).OperationID)
	if operationID == "" {
		operationID = "storm-map"
	}
	tokenRoot := fmt.Sprintf("%s/storm-gaa/%d", operationID, time.Now().UTC().UnixNano())
	slots := make([]stormMapBurstSlot, len(windows))
	for index, window := range windows {
		payload, err := json.Marshal(struct {
			KingdomID State.KingdomID `json:"KID"`
			X1        int             `json:"AX1"`
			Y1        int             `json:"AY1"`
			X2        int             `json:"AX2"`
			Y2        int             `json:"AY2"`
		}{kingdomID, window.X1, window.Y1, window.X2, window.Y2})
		if err != nil {
			return Localization.WithError(fmt.Errorf("encode Storm map window %d/%d: %w", index+1, len(windows), err), Localization.ErrorContext(Localization.New("server.app.encode_storm_map_window.32081358", "encode Storm map window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(windows)}), err))
		}
		wire, err := Protocol.Encode(Protocol.Command{
			Namespace: sender.Namespace(), Opcode: "gaa", Payload: payload,
		})
		if err != nil {
			return Localization.WithError(fmt.Errorf("build Storm map window %d/%d: %w", index+1, len(windows), err), Localization.ErrorContext(Localization.New("server.app.build_storm_map_window.ea495115", "build Storm map window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(windows)}), err))
		}
		token := fmt.Sprintf("%s/%d", tokenRoot, index+1)
		slots[index] = stormMapBurstSlot{token: token, wire: wire}
	}

	baseMetadata := Outbound.MetadataFromContext(burstContext)
	for index, slot := range slots {
		if err := func() error {
			remaining := stormMapBurstRemaining(burstContext)
			if remaining <= 0 {
				deadlineErr := burstContext.Err()
				if deadlineErr == nil {
					deadlineErr = context.DeadlineExceeded
				}
				return Localization.WithError(fmt.Errorf(
					"Storm map GAA burst received %d/%d responses before the %s deadline: %w",
					index, len(slots), overallTimeout, deadlineErr,
				), Localization.ErrorContext(Localization.New("server.app.storm_map_gaa_burst.701aacfe", "Storm map GAA burst received {p0}/{p1} responses before the {p2} deadline", Localization.Params{"p0": index, "p1": len(slots), "p2": fmt.Sprintf("%s", overallTimeout)}), deadlineErr))
			}
			responseBudget := min(responseTimeout, remaining)
			responseContext, cancelResponse := context.WithTimeout(burstContext, responseBudget)
			defer cancelResponse()
			frames, cancelWatch := observer.WatchWireResponse("gaa", slot.token)
			defer cancelWatch()

			metadata := baseMetadata
			metadata.ResponseToken = slot.token
			metadata.ResponseOpcodes = []string{"gaa"}
			metadata.ResponseTimeoutMillis = max(1, int(responseBudget/time.Millisecond))
			sendContext := Outbound.WithMetadata(responseContext, metadata)
			for {
				err := sender.Send(sendContext, slot.wire)
				if err == nil {
					break
				}
				if !errors.Is(err, Outbound.ErrAutomationLocked) || Outbound.IsIndeterminate(err) {
					return Localization.WithError(fmt.Errorf("send Storm map window %d/%d: %w", index+1, len(slots), err), Localization.ErrorContext(Localization.New("server.app.send_storm_map_window.4dfbe2d7", "send Storm map window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(slots)}), err))
				}
				if err := sender.WaitForAutomationUnlocked(responseContext); err != nil {
					return Localization.WithError(fmt.Errorf(
						"Storm map GAA burst timed out while paused before window %d/%d: %w",
						index+1, len(slots), err,
					), Localization.ErrorContext(Localization.New("server.app.storm_map_gaa_burst.d3c76bc9", "Storm map GAA burst timed out while paused before window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(slots)}), err))
				}
			}

			var response Protocol.CommittedFrame
			select {
			case <-responseContext.Done():
				return fmt.Errorf(
					"Storm map GAA burst received %d/%d responses before window %d exceeded the %s deadline: %w",
					index, len(slots), index+1, responseBudget, responseContext.Err(),
				)
			case response = <-frames:
			}
			if response.Frame.ResponseToken != slot.token {
				observer.ForgetCommitted(response.IngressID)
				return Localization.WithError(fmt.Errorf("Storm map GAA response token changed for window %d/%d", index+1, len(slots)), Localization.New("server.app.storm_map_gaa_response.8f0b99e2", "Storm map GAA response token changed for window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(slots)}))
			}
			committed, err := observer.WaitCommitted(responseContext, response.IngressID)
			if err != nil {
				observer.ForgetCommitted(response.IngressID)
				return Localization.WithError(fmt.Errorf("commit Storm map window %d/%d: %w", index+1, len(slots), err), Localization.ErrorContext(Localization.New("server.app.commit_storm_map_window.2813968d", "commit Storm map window {p0}/{p1}", Localization.Params{"p0": index + 1, "p1": len(slots)}), err))
			}
			if committed.Frame.ResponseCode == nil {
				return Localization.WithError(fmt.Errorf("Storm map window %d/%d response did not include a result code", index+1, len(slots)), Localization.New("server.app.storm_map_window_p.4d27f3ae", "Storm map window {p0}/{p1} response did not include a result code", Localization.Params{"p0": index + 1, "p1": len(slots)}))
			}
			if *committed.Frame.ResponseCode != 0 {
				return fmt.Errorf(
					"Storm map window %d/%d: %w",
					index+1, len(slots),
					Intent.NewResponseCodeError(language, committed.Frame.Opcode, *committed.Frame.ResponseCode),
				)
			}
			if committed.ReduceError != "" {
				return Localization.WithError(fmt.Errorf(
					"Storm map window %d/%d response state reduction failed: %s",
					index+1, len(slots), committed.ReduceError,
				), Localization.New("server.app.storm_map_window_p.653420a1", "Storm map window {p0}/{p1} response state reduction failed: {p2}", Localization.Params{"p0": index + 1, "p1": len(slots), "p2": fmt.Sprintf("%s", committed.ReduceError)}))
			}
			return nil
		}(); err != nil {
			return err
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
			return Localization.WithError(fmt.Errorf("shared Storm scan coordinator is unavailable"), Localization.New("server.app.shared_storm_scan_coordinator.115746d4", "shared Storm scan coordinator is unavailable", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("Storm source castle %d is no longer available", request.SourceCastleID), Localization.New("server.app.storm_source_castle_p.d0cc7af4", "Storm source castle {p0} is no longer available", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
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
					return nil, false, Localization.WithError(fmt.Errorf("Storm map identity changed before targeted refresh capture"), Localization.New("server.app.storm_map_identity_changed.8d553c3f", "Storm map identity changed before targeted refresh capture", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("Storm map sweep identity changed before capture"), Localization.New("server.app.storm_map_sweep_identity.3e757e6b", "Storm map sweep identity changed before capture", nil))
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
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, source, target, _, err := stormAttackContext(
		Intent.PlanningContext{State: state, GameData: gameData}, mustMarshalStormAttackRequest(request.stormAttackRequest),
	)
	if err != nil {
		return err
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Localization.WithError(fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID), Localization.New("server.app.intent_plan_became_stale.f815ae9b", "intent plan became stale before dispatch: commander {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", request.CommanderID)}))
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != target.TypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y ||
		dialog.Target.StormIsleID > 0 && dialog.Target.StormIsleID != target.StormIsleID {
		return Localization.WithError(fmt.Errorf("current attack dialog does not match Storm target %d:%d", target.X, target.Y), Localization.New("server.app.current_attack_dialog_does.89322709", "current attack dialog does not match Storm target {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}))
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID), Localization.New("server.app.intent_plan_became_stale.f815ae9b", "intent plan became stale before dispatch: commander {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", request.CommanderID)}))
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve Storm attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_storm_attack_capacity.b9731e82", "resolve Storm attack capacity", nil), err))
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(attackRequest.Preset, capacity))
	if definition.Kind == GameData.StormIsleKindIsland {
		setup.CourtyardSupport.Troops = nil
	}
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Storm preset %q: %w", attackRequest.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_storm_preset_p.a5e2af16", "build Storm preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", attackRequest.Preset.Name)}), err))
	}
	body := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&body, input.GameData, source, attackRequest.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve Storm horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_storm_horse_travel.e47b07d3", "resolve Storm horse travel boost", nil), err))
	}
	if definition.Kind == GameData.StormIsleKindIsland {
		body.SupportTroops = stormSupportTroops(attackRequest.DefenseUnits)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Storm CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_storm_cra_payload.601ca33e", "build Storm CRA payload", nil), err))
	}
	if err := validateCRAInventoryPayloads([]json.RawMessage{payload}, source); err != nil {
		return Intent.Step{}, err
	}
	if err := validateStormAttackTroopReserve(body, source, input.GameData, attackRequest.MinimumTroops); err != nil {
		return Intent.Step{}, err
	}
	return commandStep(fmt.Sprintf("Attack Storm %s at %d:%d", definition.Kind, target.X, target.Y), "cra", payload, "cra", Localization.New("server.app.attack_storm_p_at.118ef7c4", "Attack Storm {p0} at {p1}:{p2}", Localization.Params{"p0": fmt.Sprintf("%s", definition.Kind), "p1": target.X, "p2": target.Y})), nil
}

func validateStormAttackTroopReserve(
	body attackBody,
	source State.CastleState,
	gameData *GameData.Store,
	minimumTroops int64,
) error {
	if minimumTroops <= 0 {
		return nil
	}
	stationedTroops := int64(0)
	for unitID, amount := range source.Units.Stationed {
		if amount <= 0 {
			continue
		}
		isTool, found := gameData.UnitIsTool(int64(unitID))
		if !found || isTool {
			continue
		}
		stationedTroops = saturatingTroopAdd(stationedTroops, amount)
	}
	launchedTroops := int64(0)
	for _, wave := range body.Waves {
		for _, flank := range []attackFlank{wave.Left, wave.Middle, wave.Right} {
			for _, pair := range flank.Units {
				launchedTroops = saturatingTroopAdd(launchedTroops, pair[1])
			}
		}
	}
	for _, pair := range body.SupportTroops {
		launchedTroops = saturatingTroopAdd(launchedTroops, pair[1])
	}
	remainingTroops := max(int64(0), stationedTroops-launchedTroops)
	if remainingTroops < minimumTroops {
		return Localization.WithError(fmt.Errorf(
			"%w: Storm attack would leave %d stationed troops, below the configured minimum %d",
			Intent.ErrPlanStale, remainingTroops, minimumTroops,
		), Localization.New("server.app.intent_plan_became_stale.e2b230e0", "intent plan became stale before dispatch: Storm attack would leave {p1, number} stationed troops, below the configured minimum {p2, number}", Localization.Params{"p1": remainingTroops, "p2": minimumTroops}))
	}
	return nil
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
		return Localization.WithError(fmt.Errorf("Storm target consumption requires a valid source and target identity"), Localization.New("server.app.storm_target_consumption_requires.3d37c986", "Storm target consumption requires a valid source and target identity", nil))
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID &&
		(request.IslandObjectID <= 0 || request.LeaveBehind < 0 || request.LeaveBehind > 1) {
		return Localization.WithError(fmt.Errorf("Storm island consumption requires an object id and valid occupation count"), Localization.New("server.app.storm_island_consumption_requires.7299f9bb", "Storm island consumption requires an object id and valid occupation count", nil))
	}
	_, err := application.State.ApplyComponents(State.Components(
		State.ComponentStorm,
	), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || source.KingdomID != stormIntentKingdomID {
			return nil, false, Localization.WithError(fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.storm_source_castle_p.d6407401", "Storm source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
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
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("Storm island return identity changed before completion"), Localization.New("server.app.storm_island_return_identity.e8bb0b68", "Storm island return identity changed before completion", nil))
		}
		delete(returns, key)
		return []string{"storm"}, true, nil
	})
	return err
}

func (application *Application) guardStormShopPurchase(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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

func stormCastleIdentity(castle State.CastleState) *Localization.Message {
	if castle.Name != "" {
		return Localization.New("server.storm.castle_name", "{name}", Localization.Params{"name": castle.Name})
	}
	return Localization.New("server.storm.castle_id", "castle {id}", Localization.Params{"id": strconv.FormatInt(int64(castle.ID), 10)})
}

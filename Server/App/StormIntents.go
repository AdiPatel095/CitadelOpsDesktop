package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	stormIntentKingdomID           State.KingdomID = GameData.StormKingdomID
	stormIntentIslandMapTypeID                     = 24
	stormIntentFortMapTypeID                       = 25
	stormIntentMaximumSupport                      = 8
	stormMapWindowSize                             = 101
	stormMapMaximumWindowCount                     = 400
	stormMapMaximumCoordinate                      = stormMapWindowSize*stormMapMaximumWindowCount - 1
	stormMapWindowDelayMillis                      = 150
	stormMapMinimumAttemptInterval                 = 6 * time.Hour
)

type stormMapScanRequest struct {
	SourceCastleID State.CastleID       `json:"sourceCastleId"`
	FullMap        bool                 `json:"fullMap,omitempty"`
	Bounds         State.StormMapBounds `json:"bounds"`
	Radius         int                  `json:"radius,omitempty"`
	ScanStartedAt  time.Time            `json:"scanStartedAt"`
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

type stormShopPurchaseRequest struct {
	CastleID          State.CastleID  `json:"castleId"`
	ProductID         State.PackageID `json:"productId"`
	TableID           int64           `json:"tableId"`
	Amount            int64           `json:"amount"`
	AquamarineReserve int64           `json:"aquamarineReserve"`
}

func (application *Application) registerStormIntents() error {
	for name, action := range map[string]Intent.Action{
		"storm.scan.begin":             application.beginStormScan,
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
			Name: "storm.map.scan", Description: "Focus the Storm castle and refresh a complete tiled map snapshot", Effect: Intent.EffectRead,
			ArgumentsExample: json.RawMessage(`{"sourceCastleId":5358,"fullMap":true,"bounds":{"x1":0,"y1":0,"x2":605,"y2":605}}`), Planner: planStormMapScan,
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
			Name: "storm.shop.purchase", Description: "Buy an official Luna trade-boat package while preserving an Aquamarine reserve", Effect: Intent.EffectWrite,
			ArgumentsExample: json.RawMessage(`{"castleId":5358,"productId":244,"tableId":1,"amount":1,"aquamarineReserve":50000}`), Planner: planStormShopPurchase,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
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
	if request.FullMap && !lastAttemptAt.IsZero() && time.Now().UTC().Before(lastAttemptAt.Add(stormMapMinimumAttemptInterval)) {
		return Intent.Plan{}, fmt.Errorf("full Storm map sweeps are limited to one attempt every six hours")
	}
	request.ScanStartedAt = time.Now().UTC()
	normalizedArguments, _ := json.Marshal(request)
	windows := towerMapScanWindows(source, request.Radius)
	if request.FullMap {
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
	}
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
	steps = append(steps, Intent.Step{Name: "Build Storm map state", Action: "storm.scan.capture", ActionArguments: normalizedArguments})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	summary := fmt.Sprintf("Refresh Storm forts and resource islands around %s", castleLabel(source))
	if request.FullMap {
		summary = fmt.Sprintf("Refresh the complete Storm map in %d windows", len(windows))
	}
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: summary, Steps: steps,
	}, nil
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

func planStormAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, definition, err := stormAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
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
	steps = append(steps,
		deferredCRACommandStep("Build and launch capacity-adjusted Storm attack", "storm.attack.build", resolvedArguments, contextPayload),
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
	steps := castleContextSteps(castle)
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

func planStormShopPurchase(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, _, _, err := stormShopPurchaseContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	normalized, _ := json.Marshal(map[string]any{
		"productId": request.ProductID, "tableId": request.TableID, "amount": request.Amount,
		"kingdomId": stormIntentKingdomID, "castleId": request.CastleID,
		"buildType": 0, "premiumCost": -1, "buyAll": 0, "power": 0, "position": -1,
	})
	plan, err := planShopPackagePurchase(ctx, input, normalized)
	if err != nil {
		return Intent.Plan{}, err
	}
	plan.Steps = append([]Intent.Step{{Name: "Verify Storm Aquamarine reserve", Action: "storm.shop.guard", ActionArguments: arguments}}, plan.Steps...)
	plan.Claims = append(plan.Claims, "castle:"+strconv.FormatInt(int64(request.CastleID), 10))
	plan.Summary = fmt.Sprintf("Purchase Luna trade-boat package %d with Storm Aquamarine", request.ProductID)
	return plan, nil
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
	if request.FullMap {
		windows := stormMapScanWindows(request.Bounds)
		if !request.Bounds.IsValid() || request.Bounds.X1 != 0 || request.Bounds.Y1 != 0 ||
			!request.Bounds.Contains(source.X, source.Y) || len(windows) == 0 || len(windows) > stormMapMaximumWindowCount {
			return stormMapScanRequest{}, State.CastleState{}, fmt.Errorf(
				"full Storm map scan requires origin-based bounds containing the source castle and no more than %d windows",
				stormMapMaximumWindowCount,
			)
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
	target, exists := input.State.Map[request.KingdomID][key]
	mapStateCurrent := stormMapStateMatches(input.State, input.State.Storm.Map, source.ID)
	if mapStateCurrent && !input.State.Storm.Map.LastAttemptAt.IsZero() &&
		(input.State.Storm.Map.LastCompletedAt.IsZero() || input.State.Storm.Map.LastCompletedAt.Before(input.State.Storm.Map.LastAttemptAt)) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("the latest full Storm map sweep has not completed")
	}
	if !input.State.Storm.Map.LastCompletedAt.IsZero() && mapStateCurrent {
		scannedTarget, scanned := input.State.Storm.Map.Targets[key]
		if !scanned || scannedTarget.TypeID != request.TargetTypeID || scannedTarget.StormIsleID != request.StormIsleID ||
			!exists || target.TypeID != scannedTarget.TypeID || target.StormIsleID != scannedTarget.StormIsleID {
			return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm target %d:%d is no longer in the authoritative map state", request.TargetX, request.TargetY)
		}
	}
	if !exists || target.TypeID != request.TargetTypeID || target.StormIsleID != request.StormIsleID {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm target %d:%d is no longer the selected isle", request.TargetX, request.TargetY)
	}
	definition, found := input.GameData.StormIsle(request.StormIsleID)
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
	if request.TargetTypeID == stormIntentIslandMapTypeID && stormTargetExpired(target, now) {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm resource island %d:%d has expired from the learned map state", request.TargetX, request.TargetY)
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && target.ObjectID <= 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("Storm resource island %d:%d has no object id for the post-capture troop return", request.TargetX, request.TargetY)
	}
	if request.MinimumVictoryCount < 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("minimumVictoryCount cannot be negative")
	}
	awaitingReadyVerification := request.TargetTypeID == stormIntentFortMapTypeID && target.StormCooldownRemaining > 0 &&
		!stormTargetReadyAt(target).After(now)
	if request.TargetTypeID == stormIntentFortMapTypeID && target.StormVictoryCount < request.MinimumVictoryCount && !awaitingReadyVerification {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf(
			"Storm fort %d:%d has %d wins, below the required minimum %d",
			request.TargetX, request.TargetY, target.StormVictoryCount, request.MinimumVictoryCount,
		)
	}
	if request.TargetTypeID == stormIntentIslandMapTypeID && request.MinimumVictoryCount > 0 {
		return stormAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.StormIsleDefinition{}, fmt.Errorf("minimumVictoryCount only applies to Storm forts")
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
) (stormShopPurchaseRequest, State.CastleState, GameData.StormShopPackage, error) {
	var request stormShopPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, err
	}
	if request.CastleID <= 0 || request.ProductID <= 0 || request.TableID <= 0 || request.Amount <= 0 || request.AquamarineReserve < 0 {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("Storm shop purchase requires castle, product, table, positive amount, and a non-negative reserve")
	}
	if input.GameData == nil {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("official game data is unavailable")
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != stormIntentKingdomID {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("castle %d is not the current Storm castle", request.CastleID)
	}
	item, found := input.GameData.StormShopPackage(int64(request.ProductID))
	if !found {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("package %d is not sold by Luna's trade boat", request.ProductID)
	}
	if input.State.Inventory.ConstructionOffersCastleID != castle.ID ||
		input.State.Inventory.ConstructionOffersKingdomID != castle.KingdomID ||
		input.State.Inventory.ConstructionOffersObservedAt.IsZero() {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("Luna package purchase counters are not current for Storm castle %d", castle.ID)
	}
	if request.Amount > math.MaxInt64/item.AquamarinePrice {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("Storm shop amount is too large")
	}
	required := request.Amount*item.AquamarinePrice + request.AquamarineReserve
	available := int64(math.Floor(castle.Resources[State.ResourceID(GameData.StormAquamarineID)].Amount))
	if available < required {
		return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("Storm castle has %d Aquamarine; purchase and reserve require %d", available, required)
	}
	if item.Stock > 0 {
		purchased := input.State.Inventory.ConstructionOffers[request.ProductID]
		if purchased+request.Amount > item.Stock {
			return stormShopPurchaseRequest{}, State.CastleState{}, GameData.StormShopPackage{}, fmt.Errorf("package %d has only %d purchases remaining", request.ProductID, max(int64(0), item.Stock-purchased))
		}
	}
	return request, castle, item, nil
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
	request, _, err := stormMapScanContext(Intent.PlanningContext{State: application.State.Snapshot()}, arguments)
	if err != nil {
		return err
	}
	if !request.FullMap {
		return nil
	}
	startedAt := stormScanStartedAt(request)
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("Storm source castle %d is no longer focused", request.SourceCastleID)
		}
		if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) {
			gameState.Storm.Map = State.StormMapState{
				ServerURL:      gameState.Session.ServerURL,
				PlayerID:       gameState.Player.ID,
				SourceCastleID: request.SourceCastleID,
				Targets:        map[string]State.MapObservation{},
			}
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

func (application *Application) captureStormScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := stormMapScanContext(Intent.PlanningContext{State: application.State.Snapshot()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("Storm source castle %d is no longer focused", request.SourceCastleID)
		}
		if gameState.Storm.LastScannedAt == nil {
			gameState.Storm.LastScannedAt = map[State.CastleID]time.Time{}
		}
		startedAt := stormScanStartedAt(request)
		completedAt := time.Now().UTC()
		if !request.FullMap {
			if gameState.Storm.LastScannedAt[request.SourceCastleID].Equal(completedAt) {
				return nil, false, nil
			}
			gameState.Storm.LastScannedAt[request.SourceCastleID] = completedAt
			return []string{"storm", "map"}, true, nil
		}
		if !stormMapStateMatches(*gameState, gameState.Storm.Map, request.SourceCastleID) ||
			!gameState.Storm.Map.LastAttemptAt.Equal(startedAt) {
			return nil, false, fmt.Errorf("Storm map sweep identity changed before capture")
		}
		if !gameState.Storm.Map.LastCompletedAt.IsZero() && !gameState.Storm.Map.LastCompletedAt.Before(startedAt) {
			return nil, false, nil
		}

		targets := map[string]State.MapObservation{}
		observations := gameState.Map[stormIntentKingdomID]
		eastEdgeObserved := false
		southEdgeObserved := false
		eastEdgeStart := max(request.Bounds.X1, request.Bounds.X2-stormMapWindowSize+1)
		southEdgeStart := max(request.Bounds.Y1, request.Bounds.Y2-stormMapWindowSize+1)
		for key, observation := range observations {
			if !request.Bounds.Contains(observation.X, observation.Y) {
				continue
			}
			if observation.ObservedAt.Before(startedAt) {
				if observation.TypeID == stormIntentIslandMapTypeID || observation.TypeID == stormIntentFortMapTypeID {
					delete(observations, key)
				}
				continue
			}
			if observation.X >= eastEdgeStart {
				eastEdgeObserved = true
			}
			if observation.Y >= southEdgeStart {
				southEdgeObserved = true
			}
			if observation.TypeID == stormIntentIslandMapTypeID || observation.TypeID == stormIntentFortMapTypeID {
				targets[key] = observation
			}
		}

		gameState.Storm.Map = State.StormMapState{
			ServerURL:       gameState.Session.ServerURL,
			PlayerID:        gameState.Player.ID,
			SourceCastleID:  request.SourceCastleID,
			CoveredBounds:   request.Bounds,
			NextBounds:      stormNextMapBounds(request.Bounds, eastEdgeObserved, southEdgeObserved),
			LastAttemptAt:   startedAt,
			LastCompletedAt: completedAt,
			WindowCount:     len(stormMapScanWindows(request.Bounds)),
			Targets:         targets,
		}
		gameState.Storm.LastScannedAt[request.SourceCastleID] = completedAt
		return []string{"storm", "map"}, true, nil
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

func stormNextMapBounds(bounds State.StormMapBounds, eastEdgeObserved bool, southEdgeObserved bool) State.StormMapBounds {
	next := bounds
	if eastEdgeObserved {
		candidate := next
		candidate.X2 += stormMapWindowSize
		if windows := stormMapScanWindows(candidate); len(windows) > 0 && len(windows) <= stormMapMaximumWindowCount {
			next = candidate
		}
	}
	if southEdgeObserved {
		candidate := next
		candidate.Y2 += stormMapWindowSize
		if windows := stormMapScanWindows(candidate); len(windows) > 0 && len(windows) <= stormMapMaximumWindowCount {
			next = candidate
		}
	}
	return next
}

func (application *Application) guardStormAttack(_ context.Context, arguments json.RawMessage) error {
	var request resolvedStormAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
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
		return fmt.Errorf("commander %d is no longer available", request.CommanderID)
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
		return Intent.Step{}, fmt.Errorf("commander %d is no longer available", request.CommanderID)
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
	setup := limitAttackSetupToCapacity(invasionAttackSetup(attackRequest.Preset), capacity.Capacity)
	waves, err := buildAttackSetupWaves(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Storm preset %q: %w", attackRequest.Preset.Name, err)
	}
	body := invasionAttackBody(source, target, request.CommanderID, waves, attackRequest.HorseTravelBoostID)
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || source.KingdomID != stormIntentKingdomID {
			return nil, false, fmt.Errorf("Storm source castle %d is unavailable", request.SourceCastleID)
		}
		observations := gameState.Map[request.KingdomID]
		key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
		changed := false
		if _, exists := observations[key]; exists {
			delete(observations, key)
			changed = true
		}
		if _, exists := gameState.Storm.Map.Targets[key]; exists {
			delete(gameState.Storm.Map.Targets, key)
			changed = true
		}
		if request.TargetTypeID == stormIntentIslandMapTypeID {
			if gameState.Storm.IslandReturns == nil {
				gameState.Storm.IslandReturns = map[string]State.StormIslandReturnState{}
			}
			returnKey := State.StormIslandReturnKey(request.KingdomID, request.TargetX, request.TargetY)
			gameState.Storm.IslandReturns[returnKey] = State.StormIslandReturnState{
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
		return []string{"map", "storm"}, true, nil
	})
	return err
}

func (application *Application) guardStormIslandReturn(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, _, _, err := stormIslandReturnContext(Intent.PlanningContext{
		State: application.State.Snapshot(), GameData: gameData,
	}, arguments)
	return err
}

func (application *Application) completeStormIslandReturn(_ context.Context, arguments json.RawMessage) error {
	var request stormIslandReturnRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	key := State.StormIslandReturnKey(request.KingdomID, request.IslandX, request.IslandY)
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		operation, exists := gameState.Storm.IslandReturns[key]
		if !exists {
			return nil, false, nil
		}
		if operation.Status != State.StormIslandReturnReady || operation.SourceCastleID != request.SourceCastleID ||
			operation.IslandObjectID != request.IslandObjectID || operation.ReportID != request.ReportID {
			return nil, false, fmt.Errorf("Storm island return identity changed before completion")
		}
		delete(gameState.Storm.IslandReturns, key)
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
		State: application.State.Snapshot(), GameData: gameData,
	}, arguments)
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
	if !target.StormReadyAt.IsZero() {
		return target.StormReadyAt
	}
	if target.ObservedAt.IsZero() {
		return time.Time{}
	}
	readyAt := target.ObservedAt
	if target.StormCooldownRemaining > 0 &&
		(target.TypeID == stormIntentFortMapTypeID || target.TypeID == stormIntentIslandMapTypeID && target.OwnerID > 0) {
		readyAt = readyAt.Add(time.Duration(target.StormCooldownRemaining) * time.Second)
	}
	return readyAt
}

func stormIslandUnavailable(target State.MapObservation, now time.Time) bool {
	if target.OwnerID <= 0 {
		return false
	}
	readyAt := stormTargetReadyAt(target)
	return readyAt.IsZero() || readyAt.After(now)
}

func stormTargetExpired(target State.MapObservation, now time.Time) bool {
	expiresAt := target.StormExpiresAt
	if expiresAt.IsZero() && target.TypeID == stormIntentIslandMapTypeID && target.OwnerID <= 0 &&
		target.StormCooldownRemaining > 0 && !target.ObservedAt.IsZero() {
		expiresAt = target.ObservedAt.Add(time.Duration(target.StormCooldownRemaining) * time.Second)
	}
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

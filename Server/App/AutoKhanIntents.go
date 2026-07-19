package App

import (
	"context"
	"encoding/json"
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
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/State"
)

const (
	khanEventID          = 72
	khanCampTypeID       = 2
	khanCampVictoryCount = 845
	khanTargetRadius     = 6
)

type khanAttackRequest struct {
	RunID                  string                   `json:"runId"`
	EventEndsAt            time.Time                `json:"eventEndsAt"`
	SourceCastleID         State.CastleID           `json:"sourceCastleId"`
	MainCastleID           State.CastleID           `json:"mainCastleId"`
	KingdomID              State.KingdomID          `json:"kingdomId"`
	TargetX                int                      `json:"targetX"`
	TargetY                int                      `json:"targetY"`
	VictoryCount           int64                    `json:"victoryCount"`
	Preset                 AttackPresets.Preset     `json:"preset"`
	CommanderID            State.CommanderID        `json:"commanderId"`
	HorseTravelBoostID     int                      `json:"horseTravelBoostId"`
	DefensePreset          KhanDomain.DefensePreset `json:"defensePreset"`
	OpenGateProtection     bool                     `json:"openGateProtection"`
	OffensiveUnitThreshold int64                    `json:"offensiveUnitThreshold"`
}

type khanLaunchCapture struct {
	RunID          string            `json:"runId"`
	EventEndsAt    time.Time         `json:"eventEndsAt"`
	SourceCastleID State.CastleID    `json:"sourceCastleId"`
	MainCastleID   State.CastleID    `json:"mainCastleId"`
	KingdomID      State.KingdomID   `json:"kingdomId"`
	TargetX        int               `json:"targetX"`
	TargetY        int               `json:"targetY"`
	CommanderID    State.CommanderID `json:"commanderId"`
}

type khanProtectionRequest struct {
	RunID                  string                   `json:"runId"`
	CastleID               State.CastleID           `json:"castleId"`
	DefensePreset          KhanDomain.DefensePreset `json:"defensePreset"`
	OffensiveUnitThreshold int64                    `json:"offensiveUnitThreshold"`
}

type khanPointLimitRequest struct {
	CastleID       State.CastleID `json:"castleId"`
	PointThreshold int64          `json:"pointThreshold"`
}

type khanDefenseToolPurchaseRequest struct {
	CastleID      State.CastleID           `json:"castleId"`
	PackageID     State.PackageID          `json:"packageId"`
	ToolID        State.UnitID             `json:"toolId"`
	Amount        int64                    `json:"amount"`
	ShopTableID   int64                    `json:"shopTableId"`
	DefensePreset KhanDomain.DefensePreset `json:"defensePreset"`
}

func planKhanAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := khanAttackContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	steps := generalSkillsContextSteps(input.State, request.CommanderID, time.Now().UTC())
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps, deferredCRACommandStep(
		fmt.Sprintf("Build and launch Khan camp attack with commander %d", request.CommanderID),
		"khan.attack.build", arguments, contextPayload,
	))
	capture, _ := json.Marshal(khanLaunchCapture{
		RunID: request.RunID, EventEndsAt: request.EventEndsAt, SourceCastleID: source.ID,
		MainCastleID: request.MainCastleID, KingdomID: target.KingdomID, TargetX: target.X,
		TargetY: target.Y, CommanderID: request.CommanderID,
	})
	steps = append(steps, Intent.Step{
		Name: "Capture authoritative Khan camp movement", Action: "khan.attack.capture", ActionArguments: capture,
	})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		"khan-protection",
		fmt.Sprintf("khan-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{request.CommanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoKhan", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack Khan camp %d:%d with commander %d", target.X, target.Y, request.CommanderID),
		Steps:   steps,
	}, nil
}

func (application *Application) resolveKhanAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	request, source, target, err := khanAttackContext(input, arguments, time.Now().UTC(), true)
	if err != nil {
		return Intent.Step{}, err
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("khan-camp:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: target.Level, VictoryCount: target.TowerVictoryCount,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("resolve Khan camp attack capacity: %w", err)
	}
	setup := limitAttackSetupToCapacity(invasionAttackSetup(request.Preset), capacity.Capacity)
	waves, err := buildAttackSetupWaves(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Khan attack preset %q: %w", request.Preset.Name, err)
	}
	body, err := json.Marshal(invasionAttackBody(source, target, request.CommanderID, waves, request.HorseTravelBoostID))
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Khan camp CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack Khan camp at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func khanAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requireDialog bool,
) (khanAttackRequest, State.CastleState, State.MapObservation, error) {
	var request khanAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	request.RunID = strings.TrimSpace(request.RunID)
	if request.RunID == "" || request.SourceCastleID <= 0 || request.MainCastleID <= 0 || request.CommanderID < 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan attack requires run, castle, and commander ids")
	}
	if input.GameData == nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("official game data is unavailable")
	}
	score, active := input.State.EventScores.ByEvent[khanEventID]
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() || invasionRemainingSeconds(score, now) == 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("the Nomad event is no longer active")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 || source.KingdomID != request.KingdomID {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan attack source must be an owned Great Empire castle")
	}
	main, exists := input.State.Castles[request.MainCastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan defense target must be the Great Empire main castle")
	}
	target, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || target.TypeID != khanCampTypeID || target.TowerVictoryCount != khanCampVictoryCount ||
		target.TowerVictoryCount != request.VictoryCount ||
		khanAbsoluteInt(target.X-main.X) > khanTargetRadius || khanAbsoluteInt(target.Y-main.Y) > khanTargetRadius {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan camp %d:%d changed or is unavailable", request.TargetX, request.TargetY)
	}
	if cooldown, found := input.State.TowerCooldowns[fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)]; found && cooldown.PendingCooldownRefresh {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan camp is awaiting its post-battle cooldown refresh")
	}
	if appDungeonCooldownRemaining(input.State, target, now) > 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("Khan camp is on cooldown")
	}
	if err := khanLaunchSafety(input.State, input.GameData, request, main, now); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	if requireDialog {
		dialog := input.State.AttackDialog
		if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
			dialog.Target.TypeID != khanCampTypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y ||
			dialog.Target.TowerCooldownRemaining > 0 {
			return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("authoritative attack dialog no longer matches the ready Khan camp")
		}
	}
	return request, source, target, nil
}

func khanLaunchSafety(
	gameState State.GameState,
	gameDataStore *GameData.Store,
	request khanAttackRequest,
	main State.CastleState,
	now time.Time,
) error {
	if State.HasIncomingPlayerAttack(gameState, now) {
		return fmt.Errorf("Auto Khan yielded to an incoming player attack for Auto Station")
	}
	if khanAutoStationActive(gameState, now) {
		return fmt.Errorf("Auto Khan yielded while Auto Station is moving troops")
	}
	if main.Defense.OpenGateUntil != nil && main.Defense.OpenGateUntil.After(now) {
		return fmt.Errorf("main castle gates are open until %s", main.Defense.OpenGateUntil.UTC().Format(time.RFC3339))
	}
	if gameState.Khan.Protection.Active {
		return fmt.Errorf("Auto Khan protection is locked: %s", gameState.Khan.Protection.Reason)
	}
	if main.Defense.ObservedAt.IsZero() || main.Defense.InventoryObservedAt.IsZero() {
		return fmt.Errorf("main castle defense must be refreshed before a Khan attack")
	}
	if !gameState.Khan.LastTauntResolvedAt.IsZero() && !main.Defense.ObservedAt.After(gameState.Khan.LastTauntResolvedAt) {
		return fmt.Errorf("main castle defense must be refreshed after the latest Khan taunt")
	}
	if !KhanDomain.Matches(main, request.DefensePreset) {
		return fmt.Errorf("main castle no longer matches defense preset %q", request.DefensePreset.Name)
	}
	if request.OpenGateProtection {
		if request.SourceCastleID != request.MainCastleID || request.OffensiveUnitThreshold <= 0 {
			return fmt.Errorf("offensive wall protection requires the main castle as source and a positive threshold")
		}
		if gameDataStore == nil {
			return fmt.Errorf("official game data is unavailable")
		}
		risk, err := KhanDomain.OffensiveWallUnits(main, gameDataStore, request.DefensePreset)
		if err != nil {
			return err
		}
		if risk.OffensiveUnits >= request.OffensiveUnitThreshold {
			return fmt.Errorf("defense would place %d offensive units on the wall (threshold %d)", risk.OffensiveUnits, request.OffensiveUnitThreshold)
		}
	}
	return nil
}

func (application *Application) captureKhanLaunch(_ context.Context, arguments json.RawMessage) error {
	var request khanLaunchCapture
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	var safetyError string
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		var selected State.MovementState
		for _, movement := range gameState.Movements {
			if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
				movement.KingdomID != request.KingdomID || movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
				movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
				continue
			}
			if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
				movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
				selected = movement
			}
		}
		if selected.ID == 0 {
			return nil, false, fmt.Errorf("CRA response did not return commander %d's Khan movement", request.CommanderID)
		}
		if gameState.Khan.RunID != request.RunID {
			taunts := gameState.Khan.Taunts
			observed, resolved, lastResolved := gameState.Khan.TauntsObserved, gameState.Khan.TauntsResolved, gameState.Khan.LastTauntResolvedAt
			gameState.Khan = State.KhanState{
				RunID: request.RunID, EventEndsAt: request.EventEndsAt, SourceCastleID: request.SourceCastleID,
				MainCastleID: request.MainCastleID, KingdomID: request.KingdomID, TargetX: request.TargetX,
				TargetY: request.TargetY, Launches: []State.KhanLaunchState{}, Taunts: taunts,
				TauntsObserved: observed, TauntsResolved: resolved, LastTauntResolvedAt: lastResolved,
			}
		}
		for _, launch := range gameState.Khan.Launches {
			if launch.MovementID == selected.ID {
				return nil, false, nil
			}
		}
		launch := State.KhanLaunchState{
			CommanderID: request.CommanderID, MovementID: selected.ID, ArrivesAt: selected.ArrivesAt.UTC(),
		}
		if count := len(gameState.Khan.Launches); count > 0 {
			previous := gameState.Khan.Launches[count-1]
			if launch.ArrivesAt.Before(previous.ArrivesAt) {
				safetyError = fmt.Sprintf(
					"commander %d arrives at %s before commander %d at %s",
					launch.CommanderID, launch.ArrivesAt.Format(time.RFC3339Nano),
					previous.CommanderID, previous.ArrivesAt.Format(time.RFC3339Nano),
				)
				gameState.Khan.SafetyError = safetyError
			}
		}
		gameState.Khan.Launches = append(gameState.Khan.Launches, launch)
		if len(gameState.Khan.Launches) > 256 {
			gameState.Khan.Launches = append([]State.KhanLaunchState(nil), gameState.Khan.Launches[len(gameState.Khan.Launches)-256:]...)
		}
		gameState.Khan.AttacksLaunched++
		gameState.Khan.LastAttackLaunchedAt = time.Now().UTC()
		return []string{"khan", "movements"}, true, nil
	})
	if err != nil {
		return err
	}
	if safetyError != "" {
		return fmt.Errorf("unsafe Khan chain arrival order: %s", safetyError)
	}
	return nil
}

func planKhanOpenGate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.OffensiveUnitThreshold <= 0 {
		return Intent.Plan{}, fmt.Errorf("Khan gate protection requires the Great Empire main castle and a positive threshold")
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
		Cooldown  int             `json:"CD"`
	}{castle.ID, castle.KingdomID, 0})
	steps := defenseRefreshSteps(castle)
	steps = append(steps,
		Intent.Step{Name: "Verify offensive wall threshold", Action: "khan.protection.guard", ActionArguments: arguments},
		commandStep("Open main castle gates for six hours", "mos", payload, "mos"),
		Intent.Step{Name: "Activate Auto Khan soft lock", Action: "khan.protection.activate", ActionArguments: arguments},
	)
	claims := append(defenseClaims(castle.ID), "khan-protection")
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Open gates and lock Auto Khan after %d offensive wall units", request.OffensiveUnitThreshold),
		Steps:   steps,
	}, nil
}

func planKhanPointLimitProtection(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request khanPointLimitRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.PointThreshold <= 0 {
		return Intent.Plan{}, fmt.Errorf("Khan point protection requires the Great Empire main castle and a positive point threshold")
	}
	score, active := input.State.EventScores.ByEvent[khanEventID]
	if !active || score.PlayerScore < request.PointThreshold {
		return Intent.Plan{}, fmt.Errorf("Nomad points are below the configured threshold")
	}
	now := time.Now().UTC()
	movementIDs := khanPointLimitMovementIDs(input.State, now)
	steps := make([]Intent.Step, 0, len(movementIDs)+1)
	claims := []string{"khan-protection", "castle:" + strconv.FormatInt(int64(castle.ID), 10)}
	for _, movementID := range movementIDs {
		payload, _ := json.Marshal(struct {
			MovementID State.MovementID `json:"MID"`
		}{movementID})
		steps = append(steps, commandStep(fmt.Sprintf("Recall Khan movement %d", movementID), "mcm", payload, "mcm"))
		claims = append(claims, "movement:"+strconv.FormatInt(int64(movementID), 10))
	}
	if castle.Defense.OpenGateUntil == nil || !castle.Defense.OpenGateUntil.After(now) {
		payload, _ := json.Marshal(struct {
			CastleID  State.CastleID  `json:"CID"`
			KingdomID State.KingdomID `json:"KID"`
			Cooldown  int             `json:"CD"`
		}{castle.ID, castle.KingdomID, 0})
		steps = append(steps, commandStep("Open main castle gates for six hours", "mos", payload, "mos"))
	}
	if len(steps) == 0 {
		return Intent.Plan{}, fmt.Errorf("Khan point protection is already active")
	}
	return Intent.Plan{
		Claims: claims,
		Summary: fmt.Sprintf(
			"Stop Auto Khan at %d Nomad points, recall %d movement(s), and open gates",
			request.PointThreshold, len(movementIDs),
		),
		Steps: steps,
	}, nil
}

func planKhanDefenseToolReplenish(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, main, item, purchaseCastleID, purchaseKingdomID, err := khanDefenseToolPurchaseContext(
		input, arguments, time.Now().UTC(),
	)
	if err != nil {
		return Intent.Plan{}, err
	}
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
	}{request.PackageID, 0, request.ShopTableID, request.Amount, purchaseKingdomID, purchaseCastleID, -1, 0, 0, -1})
	steps := []Intent.Step{
		{Name: "Recheck non-ruby defense-tool purchase", Action: "khan.defense_tools.guard", ActionArguments: arguments},
		shopCommandStep(fmt.Sprintf("Purchase defense tool %d", request.ToolID), "sbp", payload, 0),
		{Name: "Record defense-tool shop cadence", Action: "khan.defense_tools.purchased", ActionArguments: arguments},
	}
	steps = append(steps, defenseRefreshSteps(main)...)
	claims := append(defenseClaims(main.ID),
		"shop", "shop:table:"+strconv.FormatInt(request.ShopTableID, 10), "account-resources", "khan-defense-tools",
	)
	return Intent.Plan{
		Claims: claims,
		Summary: fmt.Sprintf(
			"Replenish tool %d with %d package purchase(s) for %d %s",
			request.ToolID, request.Amount, request.Amount*item.Price, item.PriceName,
		),
		Steps: steps,
	}, nil
}

func khanDefenseToolPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (khanDefenseToolPurchaseRequest, State.CastleState, GameData.DefenseToolShopPackage, int64, State.KingdomID, error) {
	var request khanDefenseToolPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0, err
	}
	if request.CastleID <= 0 || request.PackageID <= 0 || request.ToolID <= 0 || request.Amount <= 0 || request.ShopTableID <= 0 {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0,
			fmt.Errorf("Khan defense-tool replenishment requires castle, package, tool, shop table, and positive amount")
	}
	main, exists := input.State.Castles[request.CastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0,
			fmt.Errorf("Khan defense tools can only replenish the Great Empire main castle")
	}
	if input.GameData == nil {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0, fmt.Errorf("official game data is unavailable")
	}
	if next := input.State.Khan.LastDefenseToolPurchaseAt.Add(30 * time.Second); next.After(now) {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0,
			fmt.Errorf("Khan defense tools can next be replenished at %s", next.UTC().Format(time.RFC3339))
	}
	item, found := input.GameData.DefenseToolShopPackage(int64(request.PackageID))
	if !found || item.ToolID != int64(request.ToolID) {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0,
			fmt.Errorf("package %d is not a supported non-ruby package for tool %d", request.PackageID, request.ToolID)
	}
	route, active := input.State.ActiveShopForPackage(request.PackageID, now)
	if !active && item.PriceScope == GameData.DefenseToolPriceCastleResource && item.PriceID == GameData.StormAquamarineID &&
		input.State.Storm.LunaShopTableID > 0 && input.State.Storm.LunaShopTableID == request.ShopTableID {
		route = State.EventShopRoute{EventID: request.ShopTableID}
		active = true
	}
	if !active || route.EventID != request.ShopTableID {
		return request, main, item, 0, 0,
			fmt.Errorf("package %d is not advertised by active shop table %d", request.PackageID, request.ShopTableID)
	}
	deficit := KhanDomain.DefenseToolDeficits(main, request.DefensePreset)[request.ToolID]
	if deficit <= 0 {
		return request, main, item, 0, 0, fmt.Errorf("defense tool %d is no longer short", request.ToolID)
	}
	neededPurchases := (deficit + item.ToolAmount - 1) / item.ToolAmount
	if request.Amount > neededPurchases {
		return request, main, item, 0, 0,
			fmt.Errorf("amount %d exceeds the %d purchase(s) required for tool %d", request.Amount, neededPurchases, request.ToolID)
	}
	if item.MaxBuyPerClick > 0 && request.Amount > item.MaxBuyPerClick {
		return request, main, item, 0, 0, fmt.Errorf("amount exceeds package %d per-click maximum %d", request.PackageID, item.MaxBuyPerClick)
	}
	if item.Stock > 0 {
		remaining := max(int64(0), item.Stock-input.State.Inventory.ConstructionOffers[request.PackageID])
		if request.Amount > remaining {
			return request, main, item, 0, 0, fmt.Errorf("package %d has only %d purchase(s) remaining", request.PackageID, remaining)
		}
	}
	if request.Amount > math.MaxInt64/item.Price {
		return request, main, item, 0, 0, fmt.Errorf("defense-tool purchase amount is too large")
	}
	balance, purchaseCastleID, purchaseKingdomID, available := khanDefenseToolPurchaseBalance(input.State, main, item)
	if !available {
		return request, main, item, 0, 0, fmt.Errorf("%s balance is unavailable", item.PriceName)
	}
	required := request.Amount * item.Price
	if balance < required {
		return request, main, item, 0, 0,
			fmt.Errorf("package %d requires %d %s but only %d is available", request.PackageID, required, item.PriceName, balance)
	}
	return request, main, item, purchaseCastleID, purchaseKingdomID, nil
}

func khanDefenseToolPurchaseBalance(
	gameState State.GameState,
	main State.CastleState,
	item GameData.DefenseToolShopPackage,
) (int64, int64, State.KingdomID, bool) {
	switch item.PriceScope {
	case GameData.DefenseToolPricePlayerResource:
		return int64(math.Floor(gameState.Player.Resources[State.ResourceID(item.PriceID)])), -1, 0, true
	case GameData.DefenseToolPriceCurrency:
		return int64(math.Floor(gameState.Player.Currencies[State.CurrencyID(item.PriceID)])), -1, 0, true
	case GameData.DefenseToolPriceCastleResource:
		if item.PriceID != GameData.StormAquamarineID {
			return int64(math.Floor(main.Resources[State.ResourceID(item.PriceID)].Amount)), int64(main.ID), main.KingdomID, true
		}
		castles := make([]State.CastleState, 0)
		for _, castle := range gameState.Castles {
			if castle.KingdomID == GameData.StormKingdomID {
				castles = append(castles, castle)
			}
		}
		sort.Slice(castles, func(left, right int) bool {
			if castles[left].SlotType != castles[right].SlotType {
				return castles[left].SlotType == 1
			}
			return castles[left].ID < castles[right].ID
		})
		if len(castles) > 0 {
			castle := castles[0]
			return int64(math.Floor(castle.Resources[State.ResourceID(item.PriceID)].Amount)), int64(castle.ID), castle.KingdomID, true
		}
	}
	return 0, 0, 0, false
}

func (application *Application) guardKhanDefenseToolPurchase(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, _, _, _, _, err := khanDefenseToolPurchaseContext(
		Intent.PlanningContext{State: application.State.Snapshot(), GameData: gameData}, arguments, time.Now().UTC(),
	)
	return err
}

func (application *Application) markKhanDefenseToolPurchase(_ context.Context, arguments json.RawMessage) error {
	var request khanDefenseToolPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Khan.LastDefenseToolPurchaseAt = now
		return []string{"khan", "inventory", "resources", "currencies"}, true, nil
	})
	return err
}

func khanPointLimitMovementIDs(gameState State.GameState, now time.Time) []State.MovementID {
	seen := map[State.MovementID]struct{}{}
	result := make([]State.MovementID, 0, len(gameState.Khan.Launches))
	for _, launch := range gameState.Khan.Launches {
		movement, found := gameState.Movements[launch.MovementID]
		if !found || movement.Direction != 0 || !khanMovementActive(movement, now) {
			continue
		}
		if movement.OwnerPlayerID != 0 && movement.OwnerPlayerID != gameState.Player.ID {
			continue
		}
		if _, owned := gameState.Castles[movement.SourceCastleID]; !owned {
			continue
		}
		if _, duplicate := seen[movement.ID]; duplicate {
			continue
		}
		seen[movement.ID] = struct{}{}
		result = append(result, movement.ID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func (application *Application) guardKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	state := application.State.Snapshot()
	if State.HasIncomingPlayerAttack(state, time.Now().UTC()) || khanAutoStationActive(state, time.Now().UTC()) {
		return fmt.Errorf("Auto Khan yielded gate protection to Auto Station")
	}
	if castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(time.Now().UTC()) {
		return fmt.Errorf("main castle gates are already open")
	}
	if risk.OffensiveUnits < request.OffensiveUnitThreshold {
		return fmt.Errorf("offensive wall risk fell to %d below threshold %d", risk.OffensiveUnits, request.OffensiveUnitThreshold)
	}
	return nil
}

func (application *Application) activateKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if castle.Defense.OpenGateUntil == nil || !castle.Defense.OpenGateUntil.After(now) {
		return fmt.Errorf("the game did not confirm an active open-gate period")
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Khan.Protection = State.KhanProtectionState{
			Active: true, CastleID: request.CastleID, OffensiveWallUnits: risk.OffensiveUnits,
			OffensiveUnitThreshold: request.OffensiveUnitThreshold, TriggeredAt: now,
			GateOpenUntil: castle.Defense.OpenGateUntil.UTC(),
			Reason:        fmt.Sprintf("Add defense units to continue; %d offensive units reached the %d-unit wall threshold", risk.OffensiveUnits, request.OffensiveUnitThreshold),
		}
		return []string{"khan", "defense"}, true, nil
	})
	return err
}

func planKhanProtectionClear(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if !input.State.Khan.Protection.Active {
		return Intent.Plan{}, fmt.Errorf("Auto Khan protection is not active")
	}
	return Intent.Plan{
		Claims: []string{"khan-protection"}, Summary: "Clear recovered Auto Khan protection lock",
		Steps: []Intent.Step{{Name: "Verify defense recovery", Action: "khan.protection.clear", ActionArguments: arguments}},
	}, nil
}

func (application *Application) clearKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	protection := application.State.Snapshot().Khan.Protection
	if !protection.Active {
		return nil
	}
	if protection.GateOpenUntil.After(now) || castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(now) {
		return fmt.Errorf("the six-hour open-gate protection is still active")
	}
	if !castle.Defense.ObservedAt.After(protection.GateOpenUntil) {
		return fmt.Errorf("main castle defense has not been refreshed since protection expired")
	}
	if risk.OffensiveUnits >= request.OffensiveUnitThreshold {
		return fmt.Errorf("add defense units to continue; %d offensive units still reach the wall", risk.OffensiveUnits)
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		if !gameState.Khan.Protection.Active {
			return nil, false, nil
		}
		gameState.Khan.Protection = State.KhanProtectionState{}
		return []string{"khan", "defense"}, true, nil
	})
	return err
}

func (application *Application) khanProtectionContext(
	arguments json.RawMessage,
) (khanProtectionRequest, State.CastleState, KhanDomain.WallRisk, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, err
	}
	state := application.State.Snapshot()
	castle, exists := state.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.OffensiveUnitThreshold <= 0 {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, fmt.Errorf("Khan protection target is invalid")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, fmt.Errorf("official game data is unavailable")
	}
	risk, err := KhanDomain.OffensiveWallUnits(castle, gameData, request.DefensePreset)
	if err != nil {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, err
	}
	return request, castle, risk, nil
}

func khanAutoStationActive(gameState State.GameState, now time.Time) bool {
	for _, operation := range gameState.Stationing {
		if operation.Purpose != "autoStation" {
			continue
		}
		if operation.SafeAfter != nil && now.Before(operation.SafeAfter.Add(5*time.Second)) {
			return true
		}
		if movement, exists := gameState.Movements[operation.MovementID]; exists && khanMovementActive(movement, now) {
			return true
		}
		for _, movement := range gameState.Movements {
			if movement.SourceCastleID == operation.SourceCastleID && movement.TargetCastleID == operation.TargetCastleID && khanMovementActive(movement, now) {
				return true
			}
		}
	}
	return false
}

func khanMovementActive(movement State.MovementState, now time.Time) bool {
	if movement.Direction == 0 {
		return movement.ArrivesAt == nil || movement.ArrivesAt.After(now)
	}
	return movement.ReturnsAt == nil || movement.ReturnsAt.After(now)
}

func khanAbsoluteInt(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

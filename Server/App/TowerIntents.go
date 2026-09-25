package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	kingdomTowerMapTypeID              = 2
	towerAttackDialogPlanningFreshness = 30 * time.Second
	baronAdvisorTypeID                 = 4
	baronAdvisorSubscriptionTypeID     = 4
	baronAdvisorTokenCurrencyID        = State.CurrencyID(79)
	baronAdvisorMinimumAttackCount     = 2
	baronAdvisorMaximumAttackCount     = 9999
)

type towerLaunchRequest struct {
	SourceCastleID        State.CastleID      `json:"sourceCastleId"`
	KingdomID             State.KingdomID     `json:"kingdomId"`
	TargetX               int                 `json:"targetX"`
	TargetY               int                 `json:"targetY"`
	UnitID                State.UnitID        `json:"unitId"`
	MaidenOnly            bool                `json:"maidenOnly"`
	CommanderIDs          []State.CommanderID `json:"commanderIds"`
	HorseTravelBoostID    int                 `json:"horseTravelBoostId"`
	DailyAttackLimit      int64               `json:"dailyAttackLimit"`
	AdvisorMode           bool                `json:"advisorMode"`
	AdvisorAttackCount    int                 `json:"advisorAttackCount,omitempty"`
	MaximumDailyTimeSkips int64               `json:"maximumDailyTimeSkips,omitempty"`
}

type towerResolvedAttackRequest struct {
	towerLaunchRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type towerAdvisorActivationRequest struct {
	ConfirmedTokenSpend bool `json:"confirmedTokenSpend"`
}

func planTowerAdvisorActivation(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request towerAdvisorActivationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if !request.ConfirmedTokenSpend {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Baron Advisor activation consumes one dedicated token; confirmedTokenSpend=true is required"), Localization.New("server.app.baron_advisor_activation_consumes.6c2d27b2", "Baron Advisor activation consumes one dedicated token; confirmedTokenSpend=true is required", nil))
	}
	if baronAdvisorActive(input.State) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the Baron Advisor is already active"), Localization.New("server.app.the_baron_advisor_is.7f8f22ee", "the Baron Advisor is already active", nil))
	}
	if input.State.Player.Currencies[baronAdvisorTokenCurrencyID] < 1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Baron Advisor activation requires one token (currency %d)", baronAdvisorTokenCurrencyID), Localization.New("server.app.baron_advisor_activation_requires.ae64d21e", "Baron Advisor activation requires one token (currency {p0})", Localization.Params{"p0": fmt.Sprintf("%d", baronAdvisorTokenCurrencyID)}))
	}
	return Intent.Plan{
		Claims:  []string{"advisor:baron:activation", "account-resources", "subscriptions"},
		Summary: "Activate the Baron Advisor with one dedicated token", SummaryDescriptor: Localization.New("server.app.activate_the_baron_advisor.ca848745", "Activate the Baron Advisor with one dedicated token", nil),
		Steps: []Intent.Step{
			commandStep("Consume one Baron Advisor token", "aa", json.RawMessage(`{"AAT":4}`), "aa", Localization.New("server.app.consume_one_baron_advisor.2c9505cf", "Consume one Baron Advisor token", nil)),
			commandStep("Refresh Baron Advisor subscription", "sie", json.RawMessage(`{}`), "sie", Localization.New("server.app.refresh_baron_advisor_subscription.ea7577ab", "Refresh Baron Advisor subscription", nil)),
		},
	}, nil
}

func planTowerContext(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := towerLaunchContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, request.KingdomID})
	setup, err := craSetupContextSteps(contextPayload)
	if err != nil {
		return Intent.Plan{}, err
	}
	steps := make([]Intent.Step, 0, len(setup)+1)
	if !source.Focused {
		steps = append(steps, attackCastleContextStep(source))
	}
	steps = append(steps, setup...)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			towerTargetClaim(target),
		},
		Summary: fmt.Sprintf("Refresh tower attack context at %d:%d", target.X, target.Y), SummaryDescriptor: Localization.New("server.app.refresh_tower_attack_context.fa29ccaa", "Refresh tower attack context at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}),
		Steps: steps,
	}, nil
}

func planTowerAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := towerLaunchContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	queueEntry, _ := json.Marshal(towerQueueEntryRequest{
		SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
	})
	deferTargetStep := Intent.Step{
		Name: "Rotate tower target behind ready targets", NameDescriptor: Localization.New("server.app.rotate_tower_target_behind.fb928a7d", "Rotate tower target behind ready targets", nil), Action: "tower.queue.defer", ActionArguments: queueEntry,
	}
	deferredSkipPlan := func(summary string) Intent.Plan {
		return Intent.Plan{
			Claims: []string{
				"castle:" + strconv.FormatInt(int64(source.ID), 10), towerTargetClaim(target),
			},
			Summary: summary,
			Steps:   []Intent.Step{deferTargetStep},
		}
	}
	if State.AttackFeatureTargetPendingAt(
		input.State, State.AttackFeatureAutoTowers, target.KingdomID, kingdomTowerMapTypeID,
		target.X, target.Y, now,
	) {
		return deferredSkipPlan(fmt.Sprintf(
			"Skip tower attack: kingdom tower at %d:%d has a prior Auto Towers attack awaiting settlement",
			target.X, target.Y,
		)), nil
	}
	if towerCooldownRemaining(target, input.State.UpdatedAt, now) > 0 {
		return deferredSkipPlan(fmt.Sprintf(
			"Skip tower attack: kingdom tower at %d:%d is on cooldown", target.X, target.Y,
		)), nil
	}
	if request.AdvisorMode && !baronAdvisorActive(input.State) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the Baron Advisor is not active", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c60d6837", "intent plan became stale before dispatch: the Baron Advisor is not active", nil))
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	if request.AdvisorMode {
		if _, detail, blocked, err := towerAdvisorTimeSkipLimitStatus(
			input.State, request.MaximumDailyTimeSkips, int64(request.AdvisorAttackCount-1), now,
		); err != nil {
			return Intent.Plan{}, err
		} else if blocked {
			return Intent.Plan{Summary: detail}, nil
		}
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	commander, err := towerCommander(input.State, input.CommanderHolds, request.MaidenOnly, request.CommanderIDs)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	useAttackDialogEffects := towerAttackDialogFreshForTarget(input.State.AttackDialog, source, target, now)
	capacity, currentSource, _, err := resolveTowerAttackCapacity(input, request, commander, useAttackDialogEffects)
	if err != nil {
		return Intent.Plan{}, err
	}
	required, err := towerTotalRequiredUnits(capacity.Capacity.Left+capacity.Capacity.Right, request)
	if err != nil {
		return Intent.Plan{}, err
	}
	if err := requireTowerAttackUnits(currentSource, request.UnitID, required); err != nil {
		return deferredSkipPlan("Skip tower attack: " + err.Error()), nil
	}
	resolvedArguments, _ := json.Marshal(towerResolvedAttackRequest{towerLaunchRequest: request, CommanderID: commander})
	contextPayload, _ := json.Marshal(struct {
		SourceX              int               `json:"SX"`
		SourceY              int               `json:"SY"`
		TargetX              int               `json:"TX"`
		TargetY              int               `json:"TY"`
		KingdomID            State.KingdomID   `json:"KID"`
		CommanderID          State.CommanderID `json:"LID"`
		TowerCapacityCapture json.RawMessage   `json:"towerCapacityCapture"`
	}{source.X, source.Y, target.X, target.Y, request.KingdomID, commander, resolvedArguments})
	steps := make([]Intent.Step, 0, 5)
	steps = append(steps, deferTargetStep)
	steps = append(steps, generalSkillsContextSteps(input.State, commander, now)...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	steps = append(steps,
		deferredCRACommandStep("Build and launch tower attack", "tower.attack.build", resolvedArguments, contextPayload, Localization.New("server.app.build_and_launch_tower.99d8d615", "Build and launch tower attack", nil)),
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoTowers, SourceCastleID: source.ID, CommanderID: commander,
			KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
			AdvisorTimeSkipsUsed: int64(max(0, request.AdvisorAttackCount-1)),
		}),
	)
	steps = append(steps, Intent.Step{Name: "Consume tower queue target", NameDescriptor: Localization.New("server.app.consume_tower_queue_target.80b6e3d7", "Consume tower queue target", nil), Action: "tower.queue.consume", ActionArguments: queueEntry})
	claims := towerAttackClaims(source, target, commander, true)
	if request.AdvisorMode {
		claims = append(claims, "tower-advisor-time-skips")
	}
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoTowers",
			Affinity: "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: towerAttackSummary(request, source, target),
		Steps:   steps,
	}, nil
}

func planTowerLaunch(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planTowerAttack(ctx, input, arguments)
}

func (application *Application) resolveTowerAttackStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request towerResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	return buildTowerAttackStep(input, request.towerLaunchRequest, request.CommanderID)
}

func (application *Application) guardTowerAttackInventory(_ context.Context, arguments json.RawMessage) error {
	var request towerResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	input := Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: gameData}
	capacity, source, _, err := resolveTowerAttackCapacity(input, request.towerLaunchRequest, request.CommanderID, false)
	if err != nil {
		return err
	}
	required, err := towerTotalRequiredUnits(capacity.Capacity.Left+capacity.Capacity.Right, request.towerLaunchRequest)
	if err != nil {
		return err
	}
	return requireFreshTowerAttackUnits(source, request.UnitID, required)
}

func (application *Application) captureTowerCapacity(_ context.Context, arguments json.RawMessage) error {
	var request towerResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	now := time.Now().UTC()
	input := Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: gameData}
	_, source, target, err := towerLaunchContext(input, mustMarshalTowerLaunchRequest(request.towerLaunchRequest))
	if err != nil {
		return err
	}
	if !towerAttackDialogFreshForTarget(input.State.AttackDialog, source, target, now) {
		return Localization.WithError(fmt.Errorf(
			"%w: current attack-dialog context does not match tower %d:%d",
			Intent.ErrPlanStale, target.X, target.Y,
		), Localization.New("server.app.intent_plan_became_stale.4f404336", "intent plan became stale before dispatch: current attack-dialog context does not match tower {p1}:{p2}", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	if towerCooldownRemaining(target, input.State.UpdatedAt, now) > 0 ||
		input.State.AttackDialog.Target.TowerCooldownRemaining > 0 {
		return Localization.WithError(fmt.Errorf(
			"%w: kingdom tower at %d:%d is on cooldown",
			Intent.ErrPlanStale, target.X, target.Y,
		), Localization.New("server.app.intent_plan_became_stale.483b6d63", "intent plan became stale before dispatch: kingdom tower at {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	observation, err := towerCapacityObservation(input, request.towerLaunchRequest, request.CommanderID)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentTowerQueue), func(gameState *State.GameState) ([]string, bool, error) {
		currentSource, sourceExists := gameState.Castles[source.ID]
		currentTarget, targetExists := gameState.LookupMapObservation(target.KingdomID, fmt.Sprintf("%d:%d", target.X, target.Y))
		if !sourceExists || !targetExists ||
			!towerAttackDialogFreshForTarget(gameState.AttackDialog, currentSource, currentTarget, now) {
			return nil, false, Localization.WithError(fmt.Errorf(
				"%w: current attack-dialog context does not match tower %d:%d",
				Intent.ErrPlanStale, target.X, target.Y,
			), Localization.New("server.app.intent_plan_became_stale.4f404336", "intent plan became stale before dispatch: current attack-dialog context does not match tower {p1}:{p2}", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
		}
		if !gameState.SetTowerQueueCapacity(source.ID, observation) {
			return nil, false, nil
		}
		return []string{"tower-queue"}, true, nil
	})
	return err
}

func towerCapacityObservation(
	input Intent.PlanningContext,
	request towerLaunchRequest,
	commander State.CommanderID,
) (State.TowerCapacityObservation, error) {
	baseline, _, _, err := resolveTowerAttackCapacity(input, request, commander, false)
	if err != nil {
		return State.TowerCapacityObservation{}, err
	}
	fresh, _, _, err := resolveTowerAttackCapacity(input, request, commander, true)
	if err != nil {
		return State.TowerCapacityObservation{}, err
	}
	baselineUnits := baseline.Capacity.Left + baseline.Capacity.Right
	freshUnits := fresh.Capacity.Left + fresh.Capacity.Right
	return State.TowerCapacityObservation{
		AdditionalUnits: max(int64(0), freshUnits-baselineUnits),
		FullFlankUnits:  freshUnits,
		ObservedAt:      input.State.AttackDialog.ObservedAt,
	}, nil
}

func buildTowerAttackStep(input Intent.PlanningContext, request towerLaunchRequest, commander State.CommanderID) (Intent.Step, error) {
	_, source, target, err := towerLaunchContext(input, mustMarshalTowerLaunchRequest(request))
	if err != nil {
		return Intent.Step{}, err
	}
	if towerCooldownRemaining(target, input.State.UpdatedAt, time.Now().UTC()) > 0 {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: kingdom tower at %d:%d is on cooldown", Intent.ErrPlanStale, target.X, target.Y,
		), Localization.New("server.app.intent_plan_became_stale.483b6d63", "intent plan became stale before dispatch: kingdom tower at {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	dialog := input.State.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != kingdomTowerMapTypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y {
		return Intent.Step{}, Localization.WithError(fmt.Errorf(
			"%w: current attack-dialog context does not match tower %d:%d",
			Intent.ErrPlanStale, target.X, target.Y,
		), Localization.New("server.app.intent_plan_became_stale.4f404336", "intent plan became stale before dispatch: current attack-dialog context does not match tower {p1}:{p2}", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	capacity, source, target, err := resolveTowerAttackCapacity(input, request, commander, true)
	if err != nil {
		return Intent.Step{}, err
	}
	if request.AdvisorMode && !baronAdvisorActive(input.State) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: the Baron Advisor is no longer active", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4a29a9ab", "intent plan became stale before dispatch: the Baron Advisor is no longer active", nil))
	}
	if request.AdvisorMode {
		if _, detail, blocked, err := towerAdvisorTimeSkipLimitStatus(
			input.State, request.MaximumDailyTimeSkips, int64(request.AdvisorAttackCount-1), time.Now().UTC(),
		); err != nil {
			return Intent.Step{}, err
		} else if blocked {
			return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: %s", Intent.ErrPlanStale, detail), Localization.New("server.app.intent_plan_became_stale.9e9732a4", "intent plan became stale before dispatch: {p1}", Localization.Params{"p1": fmt.Sprintf("%s", detail)}))
		}
	}
	required, err := towerTotalRequiredUnits(capacity.Capacity.Left+capacity.Capacity.Right, request)
	if err != nil {
		return Intent.Step{}, err
	}
	if err := requireFreshTowerAttackUnits(source, request.UnitID, required); err != nil {
		return Intent.Step{}, err
	}
	attack := towerAttackBody(
		source, target, commander, request.UnitID, capacity.Capacity.Left, capacity.Capacity.Right,
	)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve tower horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_tower_horse_travel.61847d73", "resolve tower horse travel boost", nil), err))
	}
	var wireBody any = attack
	if request.AdvisorMode {
		wireBody = advisorAttackBody{
			attackBody: attack, AttackCount: request.AdvisorAttackCount, Mode: 0, AdvisorType: baronAdvisorTypeID,
		}
	}
	body, err := json.Marshal(wireBody)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build tower CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_tower_cra_payload.de81c069", "build tower CRA payload", nil), err))
	}
	return commandStep(fmt.Sprintf("Attack tower at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_tower_at_p.0545c4a6", "Attack tower at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y})), nil
}

func resolveTowerAttackCapacity(
	input Intent.PlanningContext,
	request towerLaunchRequest,
	commander State.CommanderID,
	useAttackDialogEffects bool,
) (AttackCapacity.Result, State.CastleState, State.MapObservation, error) {
	_, source, target, err := towerLaunchContext(input, mustMarshalTowerLaunchRequest(request))
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, err
	}
	commanderState, exists := input.State.Commanders[commander]
	if !exists || !commanderState.Available {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
			"%w: commander %d is no longer available", Intent.ErrPlanStale, commander,
		), Localization.New("server.app.intent_plan_became_stale.e3879423", "intent plan became stale before dispatch: commander {p1, number} is no longer available", Localization.Params{"p1": commander}))
	}
	if input.GameData == nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commander,
		UseAttackDialogEffects: useAttackDialogEffects,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("tower:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y, ObjectID: target.ObjectID,
				Level: target.Level, VictoryCount: target.TowerVictoryCount,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("resolve tower attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_tower_attack_capacity.070a7787", "resolve tower attack capacity", nil), err))
	}
	return capacity, source, target, nil
}

func towerAttackDialogFreshForTarget(
	dialog State.AttackDialogState,
	source State.CastleState,
	target State.MapObservation,
	now time.Time,
) bool {
	if dialog.ObservedAt.IsZero() || now.Before(dialog.ObservedAt) ||
		now.Sub(dialog.ObservedAt) > towerAttackDialogPlanningFreshness {
		return false
	}
	return dialog.SourceCastleID == source.ID && dialog.KingdomID == target.KingdomID &&
		dialog.Target.TypeID == kingdomTowerMapTypeID && dialog.Target.X == target.X && dialog.Target.Y == target.Y
}

func mustMarshalTowerLaunchRequest(request towerLaunchRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

func towerAttackClaims(source State.CastleState, target State.MapObservation, commander State.CommanderID, focus bool) []string {
	claims := []string{
		"attack-context", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		"attack-inventory:" + strconv.FormatInt(int64(source.ID), 10), towerTargetClaim(target),
	}
	if focus {
		claims = append(claims, "castle-focus")
	}
	return append(claims, craCommanderClaims([]State.CommanderID{commander})...)
}

func towerLaunchContext(input Intent.PlanningContext, arguments json.RawMessage) (towerLaunchRequest, State.CastleState, State.MapObservation, error) {
	var request towerLaunchRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if request.AdvisorMode {
		if request.AdvisorAttackCount < baronAdvisorMinimumAttackCount || request.AdvisorAttackCount > baronAdvisorMaximumAttackCount {
			return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
				"Baron Advisor attackCount must be between %d and %d",
				baronAdvisorMinimumAttackCount, baronAdvisorMaximumAttackCount,
			), Localization.New("server.app.baron_advisor_attackcount_must.b4925ab3", "Baron Advisor attackCount must be between {p0} and {p1}", Localization.Params{"p0": baronAdvisorMinimumAttackCount, "p1": baronAdvisorMaximumAttackCount}))
		}
		if request.MaximumDailyTimeSkips <= 0 || int64(request.AdvisorAttackCount-1) > request.MaximumDailyTimeSkips {
			return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
				"Baron Advisor attack count exceeds the configured daily Time Skip limit",
			), Localization.New("server.app.baron_advisor_attack_count.22b05372", "Baron Advisor attack count exceeds the configured daily Time Skip limit", nil))
		}
	} else if request.AdvisorAttackCount != 0 || request.MaximumDailyTimeSkips != 0 {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("Advisor attack options require advisorMode=true"), Localization.New("server.app.advisor_attack_options_require.41b42a15", "Advisor attack options require advisorMode=true", nil))
	}
	if request.SourceCastleID <= 0 || request.UnitID <= 0 {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("tower source castle and unit are required"), Localization.New("server.app.tower_source_castle_and.49183710", "tower source castle and unit are required", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("tower source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.tower_source_castle_p.5df4eaf1", "tower source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if request.KingdomID != source.KingdomID {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("tower target must be in source castle kingdom %d", source.KingdomID), Localization.New("server.app.tower_target_must_be.81b1a340", "tower target must be in source castle kingdom {p0}", Localization.Params{"p0": fmt.Sprintf("%d", source.KingdomID)}))
	}
	target, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.TypeID != kingdomTowerMapTypeID {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("kingdom tower at %d:%d is not in the current map state", request.TargetX, request.TargetY), Localization.New("server.app.kingdom_tower_at_p.78b21994", "kingdom tower at {p0}:{p1} is not in the current map state", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	return request, source, target, nil
}

func towerTotalRequiredUnits(perAttack int64, request towerLaunchRequest) (int64, error) {
	count := int64(1)
	if request.AdvisorMode {
		count = int64(request.AdvisorAttackCount)
	}
	if perAttack <= 0 || count <= 0 || perAttack > math.MaxInt64/count {
		return 0, Localization.WithError(fmt.Errorf("tower attack troop requirement is invalid"), Localization.New("server.app.tower_attack_troop_requirement.3eec3870", "tower attack troop requirement is invalid", nil))
	}
	return perAttack * count, nil
}

func towerAttackSummary(request towerLaunchRequest, source State.CastleState, target State.MapObservation) string {
	if request.AdvisorMode {
		return fmt.Sprintf(
			"Chain %d Baron Advisor tower hits using %d Time Skips at %d:%d from %s",
			request.AdvisorAttackCount, request.AdvisorAttackCount-1, target.X, target.Y, castleLabel(source),
		)
	}
	return fmt.Sprintf("Attack kingdom tower at %d:%d from %s", target.X, target.Y, castleLabel(source))
}

func towerAdvisorTimeSkipLimitStatus(
	gameState State.GameState,
	maximum int64,
	planned int64,
	now time.Time,
) (int64, string, bool, error) {
	if maximum <= 0 {
		return 0, "", false, Localization.WithError(fmt.Errorf("maximumDailyTimeSkips must be positive for Advisor mode"), Localization.New("server.app.maximumdailytimeskips_must_be_positive.fc10e544", "maximumDailyTimeSkips must be positive for Advisor mode", nil))
	}
	if planned <= 0 || planned >= int64(baronAdvisorMaximumAttackCount) {
		return 0, "", false, Localization.WithError(fmt.Errorf("planned Baron Advisor Time Skip usage is invalid"), Localization.New("server.app.planned_baron_advisor_time.3ecf610c", "planned Baron Advisor Time Skip usage is invalid", nil))
	}
	attacks := gameState.DailyAttacks
	if attacks.ObservedAt.IsZero() || attacks.SessionStartedAt.IsZero() {
		return 0, "Waiting for the authoritative server daily reset before using Advisor Time Skips", true, nil
	}
	used, exact := State.TowerAdvisorTimeSkipsUsedSince(gameState, attacks.SessionStartedAt, now)
	if !exact {
		return 0, "Cannot establish exact Auto Towers Advisor Time Skip usage for the current server day", true, nil
	}
	if used >= maximum {
		return used, fmt.Sprintf(
			"Daily Auto Towers Advisor Time Skip limit reached: %d / %d; chaining resumes when the server daily attack count resets",
			used, maximum,
		), true, nil
	}
	if planned > maximum-used {
		return used, fmt.Sprintf(
			"Advisor chain needs %d Time Skips with %d / %d already used in the current server day",
			planned, used, maximum,
		), true, nil
	}
	return used, "", false, nil
}

func baronAdvisorActive(gameState State.GameState) bool {
	subscription, exists := gameState.Subscriptions[baronAdvisorSubscriptionTypeID]
	return exists && subscription.TypeID == baronAdvisorSubscriptionTypeID && subscription.RemainingSec > 0
}

func towerCommander(
	gameState State.GameState,
	holds Intent.CommanderHoldRegistry,
	maidenOnly bool,
	configured []State.CommanderID,
) (State.CommanderID, error) {
	if configured != nil && len(configured) == 0 {
		return 0, Localization.WithError(fmt.Errorf("no commanders are assigned to Auto Towers"), Localization.New("server.app.no_commanders_are_assigned.eacda54e", "no commanders are assigned to Auto Towers", nil))
	}
	candidates := allCommanderIDs(gameState)
	if configured != nil {
		candidates = append([]State.CommanderID(nil), configured...)
	}
	if maidenOnly {
		maidenCandidates := maidenCandidateCommanders(gameState)
		eligible := make(map[State.CommanderID]struct{}, len(maidenCandidates))
		for _, commanderID := range maidenCandidates {
			eligible[commanderID] = struct{}{}
		}
		filtered := make([]State.CommanderID, 0, len(candidates))
		for _, commanderID := range candidates {
			if _, supported := eligible[commanderID]; supported {
				filtered = append(filtered, commanderID)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		if maidenOnly && configured != nil {
			return 0, Localization.WithError(fmt.Errorf("no assigned commander supports the required maiden relic"), Localization.New("server.app.no_assigned_commander_supports.e36fa973", "no assigned commander supports the required maiden relic", nil))
		}
		if maidenOnly {
			return 0, Localization.WithError(fmt.Errorf("no commander supports the required maiden relic"), Localization.New("server.app.no_commander_supports_the.0ea54c02", "no commander supports the required maiden relic", nil))
		}
		if configured != nil {
			return 0, Localization.WithError(fmt.Errorf("no assigned Auto Towers commander is in the current roster"), Localization.New("server.app.no_assigned_auto_towers.4dd7e92d", "no assigned Auto Towers commander is in the current roster", nil))
		}
		return 0, Localization.WithError(fmt.Errorf("no commander is in the current roster"), Localization.New("server.app.no_commander_is_in.dc103653", "no commander is in the current roster", nil))
	}
	resolution, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{Candidates: candidates, Count: 1, Strategy: "lowest_id"}, craCommanderSelectionOptions{
		Holds:        holds,
		DefaultCount: 1, RequireAvailable: true,
	})
	if err != nil {
		if maidenOnly {
			if configured != nil {
				return 0, Localization.WithError(fmt.Errorf("no available assigned maiden-supported commander: %w", err), Localization.ErrorContext(Localization.New("server.app.no_available_assigned_maiden.11a97110", "no available assigned maiden-supported commander", nil), err))
			}
			return 0, Localization.WithError(fmt.Errorf("no available maiden-supported commander: %w", err), Localization.ErrorContext(Localization.New("server.app.no_available_maiden_supported.1e0afba5", "no available maiden-supported commander", nil), err))
		}
		if configured != nil {
			return 0, Localization.WithError(fmt.Errorf("no available assigned Auto Towers commander: %w", err), Localization.ErrorContext(Localization.New("server.app.no_available_assigned_auto.e384b7e9", "no available assigned Auto Towers commander", nil), err))
		}
		return 0, Localization.WithError(fmt.Errorf("no available commander: %w", err), Localization.ErrorContext(Localization.New("server.app.no_available_commander.09af8daf", "no available commander", nil), err))
	}
	return resolution.Selected[0], nil
}

func requireTowerAttackUnits(source State.CastleState, unitID State.UnitID, required int64) error {
	available := max(0, source.Units.Stationed[unitID])
	if required > 0 && available >= required {
		return nil
	}
	return fmt.Errorf(
		"%s has %d available of unit %d; full tower flanks require %d",
		castleLabel(source), available, unitID, required,
	)
}

func requireFreshTowerAttackUnits(source State.CastleState, unitID State.UnitID, required int64) error {
	if err := requireTowerAttackUnits(source, unitID, required); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func towerCooldownRemaining(target State.MapObservation, stateUpdatedAt time.Time, now time.Time) int {
	if target.TowerCooldownRemaining <= 0 {
		return 0
	}
	observedAt := target.ObservedAt
	if observedAt.IsZero() {
		observedAt = stateUpdatedAt
	}
	if observedAt.IsZero() {
		return target.TowerCooldownRemaining
	}
	remaining := target.TowerCooldownRemaining - int(now.Sub(observedAt)/time.Second)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func towerAttackBody(
	source State.CastleState,
	target State.MapObservation,
	commander State.CommanderID,
	unitID State.UnitID,
	left, right int64,
) attackBody {
	empty := attackPair{-1, 0}
	wave := attackWave{
		Left:  attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{{int64(unitID), left}, empty}},
		Right: attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{{int64(unitID), right}, empty}},
		Middle: attackFlank{
			Tools: []attackPair{empty, empty, empty},
			Units: []attackPair{empty, empty, empty, empty, empty, empty},
		},
	}
	return attackBody{
		SourceX: source.X, SourceY: source.Y, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commander, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: []attackWave{wave}, Books: []any{},
		AttackSupportTools: emptyAttackSupportTools(),
		SupportTroops:      emptyAttackSupportTroops(),
	}
}

func towerTargetClaim(target State.MapObservation) string {
	return fmt.Sprintf("tower-target:%d:%d:%d", target.KingdomID, target.X, target.Y)
}

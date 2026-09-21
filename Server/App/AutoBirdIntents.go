package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	autoBirdNoTargetRetry   = 5 * time.Minute
	autoBirdNoTroopsRetry   = 5 * time.Minute
	autoBirdFreshStateRetry = 30 * time.Second
	autoBirdMovementRetry   = time.Minute
	autoBirdTargetFreshness = 5 * time.Minute
	autoBirdUnitsFreshness  = 30 * time.Second
)

type autoBirdCycleRequest struct {
	ControlRevision      time.Time            `json:"controlRevision,omitempty"`
	SourceCastleID       State.CastleID       `json:"sourceCastleId"`
	TrackingID           string               `json:"trackingId"`
	PresetID             string               `json:"presetId,omitempty"`
	PresetValidUntil     time.Time            `json:"presetValidUntil,omitempty"`
	MinimumRPTDays       int                  `json:"minimumRPTDays"`
	MinimumDelayHours    int                  `json:"minimumDelayHours"`
	MaximumDelayHours    int                  `json:"maximumDelayHours"`
	MinimumSend          int64                `json:"minimumSend"`
	Reserves             []stationUnitRequest `json:"reserves,omitempty"`
	AllianceRefreshAt    time.Time            `json:"allianceRefreshAt,omitempty"`
	UnitsRefreshAt       time.Time            `json:"unitsRefreshAt,omitempty"`
	DispatchStartedAt    time.Time            `json:"dispatchStartedAt,omitempty"`
	ExpectedTargetCastle State.CastleID       `json:"expectedTargetCastleId,omitempty"`
}

func (application *Application) registerAutoBirdIntents() error {
	if err := application.Intents.RegisterAction("auto_bird.castle.control", application.controlAutoBirdCastle); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("auto_bird.batch.guard", application.guardAutoBirdBatch); err != nil {
		return err
	}

	if err := application.Intents.RegisterAction("auto_bird.tracking.clear", application.clearAutoBirdTracking); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("auto_bird.target.capture", application.captureAutoBirdTarget); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("auto_bird.manifest.capture", application.captureAutoBirdManifest); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("auto_bird.dispatch.guard", application.guardAutoBirdDispatch); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("auto_bird.movement.capture", application.captureAutoBirdMovement); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("auto_bird.dispatch.build", application.resolveAutoBirdDispatchStep); err != nil {
		return err
	}
	for _, definition := range []Intent.Definition{
		{Name: "auto_bird.castle_control", Description: "Pause, resume, or rescan one castle without editing its settings", DescriptionDescriptor: Localization.New("server.intent.description.9050558d", "Pause, resume, or rescan one castle without editing its settings", nil), Effect: Intent.EffectWrite, Planner: planAutoBirdCastleControl},
		{
			Name: "auto_bird.clear_tracking", Description: "Clear persisted Auto Bird cycle tracking without changing movements, settings, or Auto Station", DescriptionDescriptor: Localization.New("server.intent.description.38cff871", "Clear persisted Auto Bird cycle tracking without changing movements, settings, or Auto Station", nil), Effect: Intent.EffectWrite,
			Planner: planAutoBirdClearTracking,
		},
		{
			Name: "auto_bird.discover", Description: "Refresh and select one castle's protected alliance bird target", DescriptionDescriptor: Localization.New("server.intent.description.174530fe", "Refresh and select one castle's protected alliance bird target", nil), Effect: Intent.EffectRead,
			Planner: planAutoBirdDiscover,
		},
		{
			Name: "auto_bird.prepare", Description: "Refresh one castle's complete troop inventory for its selected bird target", DescriptionDescriptor: Localization.New("server.intent.description.3b678a13", "Refresh one castle's complete troop inventory for its selected bird target", nil), Effect: Intent.EffectRead,
			Planner: planAutoBirdPrepare,
		},
		{
			Name: "auto_bird.dispatch", Description: "Dispatch one prepared Auto Bird movement and record its return schedule", DescriptionDescriptor: Localization.New("server.intent.description.9b42017d", "Dispatch one prepared Auto Bird movement and record its return schedule", nil), Effect: Intent.EffectLaunch,
			Planner: planAutoBirdDispatch,
		},
		{
			Name: "auto_bird.reconcile", Description: "Refresh and reconcile one Auto Bird castle's launched movement", DescriptionDescriptor: Localization.New("server.intent.description.c9223724", "Refresh and reconcile one Auto Bird castle's launched movement", nil), Effect: Intent.EffectRead,
			Planner: planAutoBirdReconcile,
		},
	} {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return nil
}

func planAutoBirdClearTracking(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	tracked := 0
	for trackingID, operation := range input.State.Stationing {
		if operation.Purpose != "autoBird" && !strings.HasPrefix(trackingID, "autoBird:") {
			continue
		}
		tracked++
	}
	summary := "Clear Auto Bird tracking; no tracked cycles are currently stored"
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.clear_auto_bird_tracking.d70d288d", "Clear Auto Bird tracking; no tracked cycles are currently stored", nil)
	if tracked == 1 {
		summary = "Clear 1 persisted Auto Bird cycle"
		summaryLocalizationMessage = Localization.New("server.app.clear_persisted_auto_bird.f699dcdc", "Clear 1 persisted Auto Bird cycle", nil)
	} else if tracked > 1 {
		summary = fmt.Sprintf("Clear %d persisted Auto Bird cycles", tracked)
		summaryLocalizationMessage = Localization.New("server.app.clear_p_persisted_auto.77113fd0", "Clear {p0, number} persisted Auto Bird cycles", Localization.Params{"p0": tracked})
	}
	return Intent.Plan{
		Claims:  []string{autoBirdCycleClaim(0)},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: []Intent.Step{{
			Name: "Clear persisted Auto Bird cycle tracking", Action: "auto_bird.tracking.clear",
			ActionArguments: json.RawMessage(`{}`),
		}},
	}, nil
}

func (application *Application) clearAutoBirdTracking(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		changed := false
		for trackingID, operation := range gameState.Stationing {
			if operation.Purpose != "autoBird" && !strings.HasPrefix(trackingID, "autoBird:") {
				continue
			}
			delete(gameState.Stationing, trackingID)
			changed = true
		}
		return []string{"stationing"}, changed, nil
	})
	return err
}

func planAutoBirdDiscover(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	if autoBirdPresetWindowExpired(request, now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the selected Auto Bird preset period has ended", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.574a3499", "intent plan became stale before dispatch: the selected Auto Bird preset period has ended", nil))
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Auto Bird target discovery is disabled while Protection Mode is preparing or active"), Localization.New("server.app.auto_bird_target_discovery.64d2df8d", "Auto Bird target discovery is disabled while Protection Mode is preparing or active", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if input.State.Alliance.ID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the current player's alliance is not known"), Localization.New("server.app.the_current_player_s.e618bdb1", "the current player's alliance is not known", nil))
	}
	request.AllianceRefreshAt = now
	actionArguments, _ := json.Marshal(request)
	alliancePayload, _ := json.Marshal(struct {
		AllianceID State.AllianceID `json:"AID"`
	}{AllianceID: input.State.Alliance.ID})
	allianceStep := commandStep("Refresh Auto Bird alliance targets", "ain", alliancePayload, "ain", Localization.New("server.app.refresh_auto_bird_alliance.48abcc24", "Refresh Auto Bird alliance targets", nil))
	allianceStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{
			"alliance-directory",
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			autoBirdCycleClaim(source.ID),
		},
		Summary: fmt.Sprintf("Discover a fresh Auto Bird target for %s", castleLabel(source)), SummaryDescriptor: Localization.New("server.app.discover_a_fresh_auto.d7472659", "Discover a fresh Auto Bird target for {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))}),
		Steps: []Intent.Step{
			allianceStep,
			{
				Name:   "Capture fresh Auto Bird target",
				Action: "auto_bird.target.capture", ActionArguments: actionArguments,
			},
		},
	}, nil
}

func planAutoBirdPrepare(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	if autoBirdPresetWindowExpired(request, now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the selected Auto Bird preset period has ended", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.574a3499", "intent plan became stale before dispatch: the selected Auto Bird preset period has ended", nil))
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Auto Bird troop preparation is disabled while Protection Mode is preparing or active"), Localization.New("server.app.auto_bird_troop_preparation.856f00e6", "Auto Bird troop preparation is disabled while Protection Mode is preparing or active", nil))
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.PresetID != request.PresetID ||
		operation.Phase != State.StationingPhaseTargetReady &&
			operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: castle %d has no selected Auto Bird target",
			Intent.ErrPlanStale, request.SourceCastleID,
		), Localization.New("server.app.intent_plan_became_stale.33addb98", "intent plan became stale before dispatch: castle {p1} has no selected Auto Bird target", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	target, exists := allianceHolding(input.State.Alliance, operation.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) || target.KingdomID != source.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: selected Auto Bird target %d is no longer valid",
			Intent.ErrPlanStale, operation.TargetCastleID,
		), Localization.New("server.app.intent_plan_became_stale.5a8b3759", "intent plan became stale before dispatch: selected Auto Bird target {p1} is no longer valid", Localization.Params{"p1": fmt.Sprintf("%d", operation.TargetCastleID)}))
	}
	request.UnitsRefreshAt = now
	request.ExpectedTargetCastle = target.CastleID
	actionArguments, _ := json.Marshal(request)
	return Intent.Plan{
		Claims: []string{
			"castle-focus",
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			"alliance-holding:" + strconv.FormatInt(int64(target.CastleID), 10),
			autoBirdCycleClaim(source.ID),
		},
		Summary: fmt.Sprintf("Refresh every stationable troop at %s for Auto Bird", castleLabel(source)), SummaryDescriptor: Localization.New("server.app.refresh_every_stationable_troop.fc7b6f16", "Refresh every stationable troop at {p0} for Auto Bird", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))}),
		Steps: []Intent.Step{
			stationCastleContextStep(source),
			{
				Name:   "Capture fresh Auto Bird troop manifest",
				Action: "auto_bird.manifest.capture", ActionArguments: actionArguments,
			},
		},
	}, nil
}

func planAutoBirdDispatch(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	now := time.Now().UTC()
	if autoBirdPresetWindowExpired(request, now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the selected Auto Bird preset period has ended", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.574a3499", "intent plan became stale before dispatch: the selected Auto Bird preset period has ended", nil))
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Auto Bird dispatch is disabled while Protection Mode is preparing or active"), Localization.New("server.app.auto_bird_dispatch_is.cd0455b9", "Auto Bird dispatch is disabled while Protection Mode is preparing or active", nil))
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.PresetID != request.PresetID ||
		operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: castle %d has no prepared Auto Bird dispatch", Intent.ErrPlanStale, request.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.cfbc7992", "intent plan became stale before dispatch: castle {p1} has no prepared Auto Bird dispatch", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	target, exists := allianceHolding(input.State.Alliance, operation.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: prepared Auto Bird target %d is no longer available", Intent.ErrPlanStale, operation.TargetCastleID), Localization.New("server.app.intent_plan_became_stale.569cf86c", "intent plan became stale before dispatch: prepared Auto Bird target {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", operation.TargetCastleID)}))
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: prepared Auto Bird target moved to another kingdom", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.ccd6d533", "intent plan became stale before dispatch: prepared Auto Bird target moved to another kingdom", nil))
	}
	if operation.DelayHours < 1 || operation.DelayHours > 12 || len(operation.Units) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: castle %d has an incomplete Auto Bird preparation", Intent.ErrPlanStale, source.ID), Localization.New("server.app.intent_plan_became_stale.13877f22", "intent plan became stale before dispatch: castle {p1} has an incomplete Auto Bird preparation", Localization.Params{"p1": fmt.Sprintf("%d", source.ID)}))
	}
	request.DispatchStartedAt = now
	request.ExpectedTargetCastle = target.CastleID
	resolverArguments, _ := json.Marshal(request)
	// Refresh the source after acquiring castle-focus and retain that claim through
	// the guard and CDS. A different feature may have changed focus after prepare.
	steps := []Intent.Step{stationCastleContextStep(source), {
		Name:   "Verify prepared Auto Bird castle context",
		Action: "auto_bird.dispatch.guard", ActionArguments: resolverArguments,
	}}
	steps = append(steps, stationRouteContextSteps(source, target)...)
	steps = append(steps, Intent.Step{
		Name: "Dispatch freshly resolved Auto Bird troops", NameDescriptor: Localization.New("server.app.dispatch_freshly_resolved_auto.c3c18cbc", "Dispatch freshly resolved Auto Bird troops", nil), Resolver: "auto_bird.dispatch.build",
		ResolverArguments: resolverArguments, AwaitOpcode: "cds", TimeoutMillis: 10_000,
		SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted,
	})
	steps = append(steps, Intent.Step{
		Name: "Commit successful Auto Bird dispatch", NameDescriptor: Localization.New("server.app.commit_successful_auto_bird.211d25b8", "Commit successful Auto Bird dispatch", nil),
		Action: "auto_bird.movement.capture", ActionArguments: resolverArguments,
	})
	movementStep := contextCommandStep("Refresh launched Auto Bird movement", "gam", json.RawMessage(`{}`), "gam")
	movementStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	steps = append(steps, movementStep, Intent.Step{
		Name: "Reconcile Auto Bird travel and expected return", NameDescriptor: Localization.New("server.app.reconcile_auto_bird_travel.5f3b84c0", "Reconcile Auto Bird travel and expected return", nil),
		Action: "auto_bird.movement.capture", ActionArguments: resolverArguments,
	})
	return Intent.Plan{
		Claims: []string{
			"castle-focus",
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			"alliance-holding:" + strconv.FormatInt(int64(target.CastleID), 10),
			autoBirdCycleClaim(source.ID),
			"game:movements",
		},
		Summary: fmt.Sprintf("Dispatch every eligible troop from %s and record its return", castleLabel(source)), SummaryDescriptor: Localization.New("server.app.dispatch_every_eligible_troop.6e795ba3", "Dispatch every eligible troop from {p0} and record its return", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))}),
		Steps: steps,
	}, nil
}

func planAutoBirdReconcile(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.Phase != State.StationingPhaseAway {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: castle %d has no Auto Bird movement to reconcile", Intent.ErrPlanStale, request.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.e5df75df", "intent plan became stale before dispatch: castle {p1} has no Auto Bird movement to reconcile", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	actionArguments, _ := json.Marshal(request)
	movementStep := contextCommandStep("Refresh Auto Bird movements", "gam", json.RawMessage(`{}`), "gam")
	movementStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(request.SourceCastleID), 10),
			autoBirdCycleClaim(request.SourceCastleID),
			"game:movements",
		},
		Summary: fmt.Sprintf("Reconcile Auto Bird movement from castle %d", request.SourceCastleID), SummaryDescriptor: Localization.New("server.app.reconcile_auto_bird_movement.d153a72a", "Reconcile Auto Bird movement from castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}),
		Steps: []Intent.Step{
			movementStep,
			{
				Name:   "Record reconciled Auto Bird travel and expected return",
				Action: "auto_bird.movement.capture", ActionArguments: actionArguments,
			},
		},
	}, nil
}

func autoBirdCycleClaim(castleID State.CastleID) string {
	if castleID <= 0 {
		return "auto-bird-cycle"
	}
	return "auto-bird-cycle:" + strconv.FormatInt(int64(castleID), 10)
}

func decodeAutoBirdCycleRequest(arguments json.RawMessage) (autoBirdCycleRequest, error) {
	var request autoBirdCycleRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return autoBirdCycleRequest{}, err
	}
	if request.SourceCastleID <= 0 {
		return autoBirdCycleRequest{}, Localization.WithError(fmt.Errorf("sourceCastleId must be positive"), Localization.New("server.app.sourcecastleid_must_be_positive.3446acca", "sourceCastleId must be positive", nil))
	}
	request.TrackingID = strings.TrimSpace(request.TrackingID)
	request.PresetID = strings.TrimSpace(request.PresetID)
	if !request.PresetValidUntil.IsZero() {
		request.PresetValidUntil = request.PresetValidUntil.UTC()
	}
	if request.TrackingID == "" {
		request.TrackingID = "autoBird:" + strconv.FormatInt(int64(request.SourceCastleID), 10)
	}
	if request.MinimumDelayHours <= 0 {
		request.MinimumDelayHours = 6
	}
	if request.MaximumDelayHours <= 0 {
		request.MaximumDelayHours = 12
	}
	if request.MinimumDelayHours < 1 || request.MinimumDelayHours > 12 ||
		request.MaximumDelayHours < request.MinimumDelayHours || request.MaximumDelayHours > 12 {
		return autoBirdCycleRequest{}, Localization.WithError(fmt.Errorf("Auto Bird delay range must be between 1 and 12 hours"), Localization.New("server.app.auto_bird_delay_range.e58e8597", "Auto Bird delay range must be between 1 and 12 hours", nil))
	}
	if request.MinimumSend < 0 {
		return autoBirdCycleRequest{}, Localization.WithError(fmt.Errorf("minimumSend cannot be negative"), Localization.New("server.app.minimumsend_cannot_be_negative.6ef57a7b", "minimumSend cannot be negative", nil))
	}
	seen := make(map[State.UnitID]struct{}, len(request.Reserves))
	for _, reserve := range request.Reserves {
		if reserve.UnitID <= 0 || reserve.Amount <= 0 {
			return autoBirdCycleRequest{}, Localization.WithError(fmt.Errorf("Auto Bird reserve unit ids and amounts must be positive"), Localization.New("server.app.auto_bird_reserve_unit.f16a16a1", "Auto Bird reserve unit ids and amounts must be positive", nil))
		}
		if _, duplicate := seen[reserve.UnitID]; duplicate {
			return autoBirdCycleRequest{}, Localization.WithError(fmt.Errorf("Auto Bird reserve unit %d appears more than once", reserve.UnitID), Localization.New("server.app.auto_bird_reserve_unit.48498d5d", "Auto Bird reserve unit {p0} appears more than once", Localization.Params{"p0": fmt.Sprintf("%d", reserve.UnitID)}))
		}
		seen[reserve.UnitID] = struct{}{}
	}
	return request, nil
}

func (application *Application) captureAutoBirdTarget(
	_ context.Context,
	arguments json.RawMessage,
) error {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return err
	}
	delayHours := randomAutoBirdDelayHours(request.MinimumDelayHours, request.MaximumDelayHours)
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		if err := validateAutoBirdControl(*gameState, request, now); err != nil {
			return nil, false, err
		}
		current := gameState.Stationing[request.TrackingID]
		next := discoveredAutoBirdOperation(*gameState, current, request, delayHours, now)
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	return err
}

func discoveredAutoBirdOperation(
	gameState State.GameState,
	current State.StationingOperation,
	request autoBirdCycleRequest,
	delayHours int,
	now time.Time,
) State.StationingOperation {
	next := State.StationingOperation{
		ID: request.TrackingID, Purpose: "autoBird", SourceCastleID: request.SourceCastleID,
		PresetID: request.PresetID,
		Units:    map[State.UnitID]int64{}, CreatedAt: current.CreatedAt, UpdatedAt: now,
		AllianceObservedAt: gameState.Alliance.ObservedAt,
	}
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}
	wait := func(detail string, retryAt time.Time, descriptors ...*Localization.Message) State.StationingOperation {
		next.Phase = State.StationingPhaseWaiting
		next.StatusDetail = detail
		next.StatusDetailDescriptor = nil
		if len(descriptors) > 0 {
			next.StatusDetailDescriptor = Localization.Bind(descriptors[0], detail)
		}
		retryAt = retryAt.UTC()
		next.NextAttemptAt = &retryAt
		return next
	}
	if gameState.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := gameState.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		return wait("Protection Mode is preparing or active", retryAt, Localization.New("server.app.protection_mode_is_preparing.60761c5d", "Protection Mode is preparing or active", nil))
	}
	if autoBirdPresetWindowExpired(request, now) {
		return wait("The selected Auto Bird preset period ended before target capture", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.the_selected_auto_bird.fad86dde", "The selected Auto Bird preset period ended before target capture", nil))
	}
	source, exists := gameState.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return wait("Source castle is not present in the current player state", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.source_castle_is_not.ecce665b", "Source castle is not present in the current player state", nil))
	}
	if request.AllianceRefreshAt.IsZero() || gameState.Alliance.ObservedAt.Before(request.AllianceRefreshAt) {
		return wait("AIN did not commit a fresh alliance roster for this castle cycle", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.ain_did_not_commit.b90d52ff", "AIN did not commit a fresh alliance roster for this castle cycle", nil))
	}
	target, found := Automation.SelectAutoBirdHolding(gameState.Alliance, source, request.MinimumRPTDays)
	if !found {
		return wait("No protected alliance bird target is available for this castle", now.Add(autoBirdNoTargetRetry), Localization.New("server.app.no_protected_alliance_bird.d520e790", "No protected alliance bird target is available for this castle", nil))
	}
	next.TargetCastleID = target.CastleID
	next.Phase = State.StationingPhaseTargetReady
	next.DelayHours = delayHours
	next.WaitSeconds = delayHours * 3600
	next.StatusDetail = fmt.Sprintf(
		"Fresh AIN selected target %d with a %d-hour random wait; JAA inventory is next",
		target.CastleID, delayHours,
	)
	next.StatusDetailDescriptor = Localization.New("server.app.fresh_ain_selected_target.a83a2e8d", "Fresh AIN selected target {p0} with a {p1}-hour random wait; JAA inventory is next", Localization.Params{"p0": fmt.Sprintf("%d", target.CastleID), "p1": delayHours})
	return next
}

func (application *Application) captureAutoBirdManifest(
	_ context.Context,
	arguments json.RawMessage,
) error {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return err
	}
	var gameData *GameData.Store
	if application.GameData != nil {
		gameData, _ = application.GameData.Current()
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		if err := validateAutoBirdControl(*gameState, request, now); err != nil {
			return nil, false, err
		}
		current := gameState.Stationing[request.TrackingID]
		protectDirewolves := application.autoBirdDirewolvesProtected(*gameState, request.SourceCastleID, now)
		next := preparedAutoBirdManifest(*gameState, gameData, current, request, now, protectDirewolves)
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	return err
}

func preparedAutoBirdManifest(
	gameState State.GameState,
	gameData *GameData.Store,
	current State.StationingOperation,
	request autoBirdCycleRequest,
	now time.Time,
	protectDirewolves bool,
) State.StationingOperation {
	if current.Purpose != "autoBird" ||
		current.SourceCastleID != request.SourceCastleID ||
		current.PresetID != request.PresetID ||
		current.Phase != State.StationingPhaseTargetReady &&
			current.Phase != State.StationingPhaseDispatchReady {
		return current
	}
	next := current
	next.Phase = State.StationingPhaseTargetReady
	next.Units = map[State.UnitID]int64{}
	next.MovementID = 0
	next.MovementIDs = nil
	next.DispatchedAt = nil
	next.ExpectedReturnAt = nil
	next.NextAttemptAt = nil
	next.SuccessCooldownUntil = nil
	next.TravelSeconds = 0
	next.UpdatedAt = now
	wait := func(detail string, retryAt time.Time, descriptors ...*Localization.Message) State.StationingOperation {
		next.Phase = State.StationingPhaseWaiting
		next.StatusDetail = detail
		next.StatusDetailDescriptor = nil
		if len(descriptors) > 0 {
			next.StatusDetailDescriptor = Localization.Bind(descriptors[0], detail)
		}
		retryAt = retryAt.UTC()
		next.NextAttemptAt = &retryAt
		return next
	}
	if gameState.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := gameState.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		return wait("Protection Mode is preparing or active", retryAt, Localization.New("server.app.protection_mode_is_preparing.60761c5d", "Protection Mode is preparing or active", nil))
	}
	if autoBirdPresetWindowExpired(request, now) {
		return wait("The selected Auto Bird preset period ended before troop preparation", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.the_selected_auto_bird.8d5eee92", "The selected Auto Bird preset period ended before troop preparation", nil))
	}
	source, exists := gameState.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return wait("Source castle is not present in the current player state", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.source_castle_is_not.ecce665b", "Source castle is not present in the current player state", nil))
	}
	next.UnitsObservedAt = source.UnitsObservedAt
	if request.UnitsRefreshAt.IsZero() || source.UnitsObservedAt.Before(request.UnitsRefreshAt) {
		return wait("JAA did not commit a fresh troop inventory for this castle cycle", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.jaa_did_not_commit.e4ca5d85", "JAA did not commit a fresh troop inventory for this castle cycle", nil))
	}
	target, targetExists := allianceHolding(gameState.Alliance, current.TargetCastleID)
	if !targetExists || !stationHoldingType(target.SlotType) ||
		target.KingdomID != source.KingdomID ||
		request.ExpectedTargetCastle != 0 && request.ExpectedTargetCastle != target.CastleID {
		return wait("The AIN-selected bird target is no longer valid", now.Add(autoBirdFreshStateRetry), Localization.New("server.app.the_ain_selected_bird.2d648c94", "The AIN-selected bird target is no longer valid", nil))
	}
	manifest, total, manifestErr := autoBirdStationManifest(gameData, source, request.Reserves, protectDirewolves)
	if manifestErr != nil {
		return wait("Could not read eligible troops from the fresh JAA: "+manifestErr.Error(), now.Add(autoBirdNoTroopsRetry))
	}
	if len(manifest) == 0 {
		return wait("Fresh JAA contains no eligible troops after tools and reserves are excluded", now.Add(autoBirdNoTroopsRetry), Localization.New("server.app.fresh_jaa_contains_no.efb3c591", "Fresh JAA contains no eligible troops after tools and reserves are excluded", nil))
	}
	if request.MinimumSend > 0 && total < request.MinimumSend {
		return wait(
			fmt.Sprintf("Fresh JAA contains %d eligible troops; minimum send is %d", total, request.MinimumSend),
			now.Add(autoBirdNoTroopsRetry), Localization.New("server.app.fresh_jaa_contains_p.15b4a3e5", "Fresh JAA contains {p0} eligible troops; minimum send is {p1}", Localization.Params{"p0": total, "p1": request.MinimumSend}),
		)
	}
	next.Phase = State.StationingPhaseDispatchReady
	next.Units = manifest
	next.StatusDetail = fmt.Sprintf(
		"Fresh JAA prepared %d troops for target %d with the recorded %d-hour random wait",
		total, target.CastleID, next.DelayHours,
	)
	next.StatusDetailDescriptor = Localization.New("server.app.fresh_jaa_prepared_p.12933a9d", "Fresh JAA prepared {p0} troops for target {p1} with the recorded {p2}-hour random wait", Localization.Params{"p0": total, "p1": fmt.Sprintf("%d", target.CastleID), "p2": next.DelayHours})
	return next
}

func (application *Application) guardAutoBirdDispatch(
	_ context.Context,
	arguments json.RawMessage,
) error {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("%w: Auto Bird state is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.10888440", "intent plan became stale before dispatch: Auto Bird state is unavailable", nil))
	}
	snapshot := application.State.ReadOnlyView()
	if err := validateAutoBirdControl(snapshot, request, now); err != nil {
		return err
	}
	if snapshot.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := snapshot.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		application.deferAutoBirdDispatch(request, "Protection Mode became active before Auto Bird dispatch", retryAt)
		return Localization.WithError(fmt.Errorf("%w: Protection Mode became active before Auto Bird dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d740cbe2", "intent plan became stale before dispatch: Protection Mode became active before Auto Bird dispatch", nil))
	}
	if autoBirdPresetWindowExpired(request, now) {
		application.deferAutoBirdDispatch(request, "The selected Auto Bird preset period ended before dispatch", now.Add(time.Second))
		return Localization.WithError(fmt.Errorf("%w: the selected Auto Bird preset period ended before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.92da81e3", "intent plan became stale before dispatch: the selected Auto Bird preset period ended before dispatch", nil))
	}
	contextReady := false
	_, applyErr := application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		operation, exists := gameState.Stationing[request.TrackingID]
		if !exists || operation.Purpose != "autoBird" ||
			operation.SourceCastleID != request.SourceCastleID ||
			operation.PresetID != request.PresetID ||
			operation.Phase != State.StationingPhaseDispatchReady {
			return nil, false, nil
		}
		source, sourceExists := gameState.Castles[request.SourceCastleID]
		contextReady = sourceExists && source.Focused &&
			!operation.AllianceObservedAt.IsZero() &&
			now.Sub(operation.AllianceObservedAt) <= autoBirdTargetFreshness &&
			!operation.UnitsObservedAt.IsZero() &&
			!source.UnitsObservedAt.Before(operation.UnitsObservedAt) &&
			now.Sub(source.UnitsObservedAt) <= autoBirdUnitsFreshness
		if contextReady {
			return nil, false, nil
		}
		next := operation
		next.Phase = State.StationingPhaseTargetReady
		next.Units = map[State.UnitID]int64{}
		next.UnitsObservedAt = time.Time{}
		next.NextAttemptAt = nil
		next.StatusDetail = "Prepared context expired or focus changed; refresh AIN or JAA before dispatch"
		next.StatusDetailDescriptor = Localization.New("server.app.prepared_context_expired_or.60b7cf1d", "Prepared context expired or focus changed; refresh AIN or JAA before dispatch", nil)
		next.UpdatedAt = now
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	if applyErr != nil {
		return applyErr
	}
	if !contextReady {
		return Localization.WithError(fmt.Errorf("%w: castle %d needs a fresh JAA before Auto Bird dispatch", Intent.ErrPlanStale, request.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.5dfd670c", "intent plan became stale before dispatch: castle {p1} needs a fresh JAA before Auto Bird dispatch", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	return nil
}

func (application *Application) resolveAutoBirdDispatchStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return Intent.Step{}, err
	}
	now := time.Now().UTC()
	if err := validateAutoBirdControl(input.State, request, now); err != nil {
		return Intent.Step{}, err
	}
	hold := func(detail string, retry time.Duration) (Intent.Step, error) {
		application.deferAutoBirdDispatch(request, detail, now.Add(retry))
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: %s", Intent.ErrPlanStale, detail), Localization.New("server.app.intent_plan_became_stale.9e9732a4", "intent plan became stale before dispatch: {p1}", Localization.Params{"p1": fmt.Sprintf("%s", detail)}))
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := input.State.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		application.deferAutoBirdDispatch(request, "Protection Mode became active before Auto Bird dispatch", retryAt)
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: Protection Mode became active before Auto Bird dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d740cbe2", "intent plan became stale before dispatch: Protection Mode became active before Auto Bird dispatch", nil))
	}
	if autoBirdPresetWindowExpired(request, now) {
		return hold("the selected Auto Bird preset period ended before dispatch", time.Second)
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.PresetID != request.PresetID ||
		operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: castle %d is no longer prepared for Auto Bird", Intent.ErrPlanStale, request.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.9be8b6b5", "intent plan became stale before dispatch: castle {p1} is no longer prepared for Auto Bird", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return hold(
			fmt.Sprintf("source castle %d is no longer available", request.SourceCastleID),
			autoBirdFreshStateRetry,
		)
	}
	if !source.Focused || operation.UnitsObservedAt.IsZero() ||
		source.UnitsObservedAt.Before(operation.UnitsObservedAt) {
		return hold(
			fmt.Sprintf("castle %d lost its prepared JAA context before Auto Bird dispatch", source.ID),
			autoBirdFreshStateRetry,
		)
	}
	target, exists := allianceHolding(input.State.Alliance, operation.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) || target.KingdomID != source.KingdomID ||
		request.ExpectedTargetCastle != 0 && request.ExpectedTargetCastle != target.CastleID {
		return hold(
			fmt.Sprintf("prepared Auto Bird target %d is no longer valid", operation.TargetCastleID),
			autoBirdFreshStateRetry,
		)
	}
	protectDirewolves := application.autoBirdDirewolvesProtected(input.State, source.ID, now)
	manifest, total, manifestErr := autoBirdStationManifest(input.GameData, source, request.Reserves, protectDirewolves)
	if manifestErr != nil {
		return hold(
			"could not read eligible troops from the dispatch JAA: "+manifestErr.Error(),
			autoBirdNoTroopsRetry,
		)
	}
	if len(manifest) == 0 {
		return hold(
			fmt.Sprintf("fresh dispatch JAA has no eligible troops at castle %d", source.ID),
			autoBirdNoTroopsRetry,
		)
	}
	if request.MinimumSend > 0 && total < request.MinimumSend {
		return hold(
			fmt.Sprintf(
				"fresh dispatch JAA has %d eligible troops at castle %d; minimum send is %d",
				total, source.ID, request.MinimumSend,
			),
			autoBirdNoTroopsRetry,
		)
	}
	step := supportDispatchStep("Dispatch Auto Bird troops", source, target, operation.DelayHours, manifest,
		Intent.Step{Name: "Track accepted Auto Bird batch", NameDescriptor: Localization.New("server.app.track_accepted_auto_bird.4524d09b", "Track accepted Auto Bird batch", nil), Action: "auto_bird.movement.capture", ActionArguments: arguments})
	guard := func(step *Intent.Step) {
		if step.Opcode != "cds" {
			return
		}
		// commandStep stores the wire payload on Command before normalization.
		guardArguments, _ := json.Marshal(autoBirdBatchGuardRequest{Cycle: request, Payload: step.Command.Payload})
		step.PreDispatchAction = "auto_bird.batch.guard"
		step.PreDispatchArguments = guardArguments
		step.FinalDispatchAction = "auto_bird.batch.guard"
		step.FinalDispatchArguments = guardArguments
	}
	if len(step.Batch) == 0 {
		guard(&step)
	} else {
		for i := range step.Batch {
			guard(&step.Batch[i])
		}
	}
	return step, nil
}

func (application *Application) deferAutoBirdDispatch(
	request autoBirdCycleRequest,
	detail string,
	retryAt time.Time,
) {
	if application == nil || application.State == nil {
		return
	}
	_, _ = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.Stationing[request.TrackingID]
		if !exists || current.Purpose != "autoBird" ||
			current.SourceCastleID != request.SourceCastleID ||
			current.Phase != State.StationingPhaseDispatchReady {
			return nil, false, nil
		}
		next := current
		next.Phase = State.StationingPhaseWaiting
		next.Units = map[State.UnitID]int64{}
		next.MovementID = 0
		next.MovementIDs = nil
		next.DispatchedAt = nil
		next.ExpectedReturnAt = nil
		next.SuccessCooldownUntil = nil
		retryAt = retryAt.UTC()
		next.NextAttemptAt = &retryAt
		next.StatusDetail = detail
		next.UpdatedAt = time.Now().UTC()
		if source, found := gameState.Castles[request.SourceCastleID]; found {
			next.UnitsObservedAt = source.UnitsObservedAt
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
}

func autoBirdStationManifest(
	gameData *GameData.Store,
	source State.CastleState,
	reserves []stationUnitRequest,
	protectDirewolves bool,
) (map[State.UnitID]int64, int64, error) {
	if gameData == nil {
		return nil, 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	unitsCatalog, err := gameData.Catalog("units")
	if err != nil {
		return nil, 0, err
	}
	reserved := make(map[State.UnitID]int64, len(reserves))
	for _, item := range reserves {
		if item.UnitID > 0 && item.Amount > 0 {
			reserved[item.UnitID] = item.Amount
		}
	}
	amounts := make(map[State.UnitID]int64, len(source.Units.Stationed))
	var total int64
	for unitID, stationed := range source.Units.Stationed {
		if protectDirewolves && unitID == GameData.DirewolfUnitID {
			continue
		}
		if unitID <= 0 || stationed <= 0 || reserved[unitID] >= stationed {
			continue
		}
		raw, found := unitsCatalog.Find(strconv.FormatInt(int64(unitID), 10))
		if !found {
			continue
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil || GameData.IsToolRecord(record) {
			continue
		}
		amount := stationed - reserved[unitID]
		amounts[unitID] = amount
		total += amount
	}
	return amounts, total, nil
}

func (application *Application) autoBirdDirewolvesProtected(
	state State.GameState,
	castleID State.CastleID,
	now time.Time,
) bool {
	if application == nil || application.Configuration == nil {
		return false
	}
	castle, exists := state.Castles[castleID]
	if !exists || castle.SlotType != 12 || castle.KingdomID < 1 || castle.KingdomID > 3 {
		return false
	}
	configuration := application.Configuration.Snapshot()
	return Automation.FeatureEnabledAt(configuration, "auto_fortress", now) &&
		autoFortressKingdomEnabledInSnapshot(configuration, castle.KingdomID)
}

func randomAutoBirdDelayHours(minimum, maximum int) int {
	if maximum <= minimum {
		return minimum
	}
	return minimum + rand.IntN(maximum-minimum+1)
}

func autoBirdPresetWindowExpired(request autoBirdCycleRequest, now time.Time) bool {
	return !request.PresetValidUntil.IsZero() && !now.Before(request.PresetValidUntil)
}

func (application *Application) captureAutoBirdMovement(
	_ context.Context,
	arguments json.RawMessage,
) error {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		// An acknowledgement from an older cycle must not replace a requested
		// rescan. The movement reducer still retains the actual game movement.
		control := gameState.AutoBirdControl(request.SourceCastleID)
		if !control.UpdatedAt.Equal(request.ControlRevision) {
			return nil, false, nil
		}

		current, exists := gameState.Stationing[request.TrackingID]
		if !exists || current.Purpose != "autoBird" ||
			current.SourceCastleID != request.SourceCastleID ||
			current.Phase != State.StationingPhaseDispatchReady &&
				current.Phase != State.StationingPhaseAway {
			return nil, false, nil
		}
		now := time.Now().UTC()
		next := current
		next.Phase = State.StationingPhaseAway
		next.UpdatedAt = now
		if next.DispatchedAt == nil {
			dispatchedAt := request.DispatchStartedAt
			if dispatchedAt.IsZero() {
				dispatchedAt = now
			}
			dispatchedAt = dispatchedAt.UTC()
			next.DispatchedAt = &dispatchedAt
		}
		movements := findAutoBirdMovements(*gameState, next)
		if len(movements) > 0 && control.RescanRequested {
			control.RescanRequested = false
			gameState.Stationing[State.AutoBirdControlID(request.SourceCastleID)] = control
		}

		found := len(movements) > 0
		if !found {
			retryDelay := autoBirdMovementRetry
			if next.DispatchedAt != nil {
				switch elapsed := now.Sub(*next.DispatchedAt); {
				case elapsed >= 30*time.Minute:
					retryDelay = 30 * time.Minute
				case elapsed >= 5*time.Minute:
					retryDelay = 5 * time.Minute
				}
			}
			retryAt := now.Add(retryDelay)
			next.NextAttemptAt = &retryAt
			next.StatusDetail = fmt.Sprintf(
				"Dispatch succeeded; retry movement timing capture in %s without relaunching",
				retryDelay,
			)
			next.StatusDetailDescriptor = Localization.New("server.app.dispatch_succeeded_retry_movement.90bb2a75", "Dispatch succeeded; retry movement timing capture in {p0} without relaunching", Localization.Params{"p0": fmt.Sprintf("%s", retryDelay)})
		} else {
			next.MovementIDs = nil
			next.Units = map[State.UnitID]int64{}
			var expectedReturn time.Time
			for _, movement := range movements {
				next.MovementIDs = append(next.MovementIDs, movement.ID)
				next.MovementID = movement.ID // legacy client summary
				for id, amount := range movement.Units {
					next.Units[id] += amount
				}
				wait := movement.WaitSeconds
				if wait <= 0 {
					wait = next.DelayHours * 3600
				}
				next.WaitSeconds = max(next.WaitSeconds, wait)
				next.TravelSeconds = max(next.TravelSeconds, movement.TravelSeconds)
				if returned := autoBirdMovementReturnAt(movement, next, now); returned.After(expectedReturn) {
					expectedReturn = returned
				}
			}
			if !expectedReturn.IsZero() {
				expectedReturn = expectedReturn.UTC()
				next.ExpectedReturnAt = &expectedReturn
				next.NextAttemptAt = &expectedReturn
				next.SuccessCooldownUntil = &expectedReturn
			}
			next.StatusDetail = fmt.Sprintf("Auto Bird tracks %d support movement(s); last expected return %s", len(movements), formatAutoBirdTime(expectedReturn))
			next.StatusDetailDescriptor = Localization.New("server.app.auto_bird_tracks_p.fc97896c", "Auto Bird tracks {p0} support movement(s); last expected return {p1}", Localization.Params{"p0": len(movements), "p1": fmt.Sprintf("%s", formatAutoBirdTime(expectedReturn))})
		}
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	return err
}

func findAutoBirdMovements(
	gameState State.GameState,
	operation State.StationingOperation,
) []State.MovementState {
	var movements []State.MovementState
	target, targetKnown := allianceHolding(gameState.Alliance, operation.TargetCastleID)
	source, sourceKnown := gameState.Castles[operation.SourceCastleID]
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.OwnerPlayerID > 0 && gameState.Player.ID > 0 &&
			movement.OwnerPlayerID != gameState.Player.ID {
			return true
		}
		outboundSource := movement.SourceCastleID == operation.SourceCastleID ||
			sourceKnown && movement.SourceX == source.X && movement.SourceY == source.Y
		outboundTarget := movement.TargetCastleID == operation.TargetCastleID ||
			targetKnown && movement.TargetX == target.X && movement.TargetY == target.Y
		returningSource := movement.SourceCastleID == operation.TargetCastleID ||
			targetKnown && movement.SourceX == target.X && movement.SourceY == target.Y
		returningTarget := movement.TargetCastleID == operation.SourceCastleID ||
			sourceKnown && movement.TargetX == source.X && movement.TargetY == source.Y
		outbound := movement.Direction == 0 &&
			outboundSource &&
			outboundTarget
		returning := movement.Direction == 1 &&
			returningSource &&
			returningTarget
		if !outbound && !returning {
			return true
		}
		if operation.DispatchedAt != nil && !movement.ObservedAt.IsZero() &&
			movement.ObservedAt.Before(operation.DispatchedAt.Add(-time.Second)) {
			return true
		}
		if operation.DispatchedAt != nil && !movement.StartedAt.IsZero() && movement.Direction == 0 && movement.StartedAt.Before(operation.DispatchedAt.Add(-time.Second)) {
			return true
		}
		movements = append(movements, movement)
		return true
	})
	sort.Slice(movements, func(i, j int) bool { return movements[i].ID < movements[j].ID })
	return movements
}

func autoBirdMovementReturnAt(
	movement State.MovementState,
	operation State.StationingOperation,
	now time.Time,
) time.Time {
	if releaseAt := State.StationMovementReleaseAt(movement); releaseAt != nil && !releaseAt.IsZero() {
		if movement.Direction == 0 && movement.WaitSeconds <= 0 && movement.ArrivesAt != nil {
			return movement.ArrivesAt.UTC().Add(
				time.Duration(operation.WaitSeconds+max(0, movement.TravelSeconds)) * time.Second,
			)
		}
		return releaseAt.UTC()
	}
	travelSeconds := max(0, movement.TravelSeconds)
	waitSeconds := movement.WaitSeconds
	if waitSeconds <= 0 {
		waitSeconds = operation.WaitSeconds
	}
	startedAt := movement.StartedAt
	if startedAt.IsZero() {
		startedAt = now
	}
	if movement.Direction == 1 {
		return startedAt.UTC().Add(time.Duration(travelSeconds) * time.Second)
	}
	return startedAt.UTC().Add(time.Duration(travelSeconds+waitSeconds+travelSeconds) * time.Second)
}

func formatAutoBirdTime(value time.Time) string {
	if value.IsZero() {
		return "unknown"
	}
	return value.UTC().Format(time.RFC3339)
}

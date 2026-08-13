package App

import (
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
	SourceCastleID       State.CastleID       `json:"sourceCastleId"`
	TrackingID           string               `json:"trackingId"`
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
		{
			Name: "auto_bird.clear_tracking", Description: "Clear persisted Auto Bird cycle tracking without changing movements, settings, or Auto Station", Effect: Intent.EffectWrite,
			Planner: planAutoBirdClearTracking,
		},
		{
			Name: "auto_bird.discover", Description: "Refresh and select one castle's protected alliance bird target", Effect: Intent.EffectRead,
			Planner: planAutoBirdDiscover,
		},
		{
			Name: "auto_bird.prepare", Description: "Refresh one castle's complete troop inventory for its selected bird target", Effect: Intent.EffectRead,
			Planner: planAutoBirdPrepare,
		},
		{
			Name: "auto_bird.dispatch", Description: "Dispatch one prepared Auto Bird movement and record its return schedule", Effect: Intent.EffectLaunch,
			Planner: planAutoBirdDispatch,
		},
		{
			Name: "auto_bird.reconcile", Description: "Refresh and reconcile one Auto Bird castle's launched movement", Effect: Intent.EffectRead,
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
	if tracked == 1 {
		summary = "Clear 1 persisted Auto Bird cycle"
	} else if tracked > 1 {
		summary = fmt.Sprintf("Clear %d persisted Auto Bird cycles", tracked)
	}
	return Intent.Plan{
		Claims:  []string{autoBirdCycleClaim(0)},
		Summary: summary,
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, fmt.Errorf("Auto Bird target discovery is disabled while Protection Mode is preparing or active")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if input.State.Alliance.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("the current player's alliance is not known")
	}
	request.AllianceRefreshAt = now
	actionArguments, _ := json.Marshal(request)
	alliancePayload, _ := json.Marshal(struct {
		AllianceID State.AllianceID `json:"AID"`
	}{AllianceID: input.State.Alliance.ID})
	allianceStep := commandStep("Refresh Auto Bird alliance targets", "ain", alliancePayload, "ain")
	allianceStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	return Intent.Plan{
		Claims: []string{
			"alliance-directory",
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			autoBirdCycleClaim(source.ID),
		},
		Summary: fmt.Sprintf("Discover a fresh Auto Bird target for %s", castleLabel(source)),
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
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, fmt.Errorf("Auto Bird troop preparation is disabled while Protection Mode is preparing or active")
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.Phase != State.StationingPhaseTargetReady &&
			operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Plan{}, fmt.Errorf(
			"%w: castle %d has no selected Auto Bird target",
			Intent.ErrPlanStale, request.SourceCastleID,
		)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	target, exists := allianceHolding(input.State.Alliance, operation.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) || target.KingdomID != source.KingdomID {
		return Intent.Plan{}, fmt.Errorf(
			"%w: selected Auto Bird target %d is no longer valid",
			Intent.ErrPlanStale, operation.TargetCastleID,
		)
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
		Summary: fmt.Sprintf("Refresh every stationable troop at %s for Auto Bird", castleLabel(source)),
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
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, fmt.Errorf("Auto Bird dispatch is disabled while Protection Mode is preparing or active")
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Plan{}, fmt.Errorf("%w: castle %d has no prepared Auto Bird dispatch", Intent.ErrPlanStale, request.SourceCastleID)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	target, exists := allianceHolding(input.State.Alliance, operation.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Plan{}, fmt.Errorf("%w: prepared Auto Bird target %d is no longer available", Intent.ErrPlanStale, operation.TargetCastleID)
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Plan{}, fmt.Errorf("%w: prepared Auto Bird target moved to another kingdom", Intent.ErrPlanStale)
	}
	if operation.DelayHours < 1 || operation.DelayHours > 12 || len(operation.Units) == 0 {
		return Intent.Plan{}, fmt.Errorf("%w: castle %d has an incomplete Auto Bird preparation", Intent.ErrPlanStale, source.ID)
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
		Name: "Dispatch freshly resolved Auto Bird troops", Resolver: "auto_bird.dispatch.build",
		ResolverArguments: resolverArguments, AwaitOpcode: "cds", TimeoutMillis: 10_000,
		SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted,
	})
	steps = append(steps, Intent.Step{
		Name:   "Commit successful Auto Bird dispatch",
		Action: "auto_bird.movement.capture", ActionArguments: resolverArguments,
	})
	movementStep := contextCommandStep("Refresh launched Auto Bird movement", "gam", json.RawMessage(`{}`), "gam")
	movementStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	steps = append(steps, movementStep, Intent.Step{
		Name:   "Reconcile Auto Bird travel and expected return",
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
		Summary: fmt.Sprintf("Dispatch every eligible troop from %s and record its return", castleLabel(source)),
		Steps:   steps,
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
		return Intent.Plan{}, fmt.Errorf("%w: castle %d has no Auto Bird movement to reconcile", Intent.ErrPlanStale, request.SourceCastleID)
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
		Summary: fmt.Sprintf("Reconcile Auto Bird movement from castle %d", request.SourceCastleID),
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
		return autoBirdCycleRequest{}, fmt.Errorf("sourceCastleId must be positive")
	}
	request.TrackingID = strings.TrimSpace(request.TrackingID)
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
		return autoBirdCycleRequest{}, fmt.Errorf("Auto Bird delay range must be between 1 and 12 hours")
	}
	if request.MinimumSend < 0 {
		return autoBirdCycleRequest{}, fmt.Errorf("minimumSend cannot be negative")
	}
	seen := make(map[State.UnitID]struct{}, len(request.Reserves))
	for _, reserve := range request.Reserves {
		if reserve.UnitID <= 0 || reserve.Amount <= 0 {
			return autoBirdCycleRequest{}, fmt.Errorf("Auto Bird reserve unit ids and amounts must be positive")
		}
		if _, duplicate := seen[reserve.UnitID]; duplicate {
			return autoBirdCycleRequest{}, fmt.Errorf("Auto Bird reserve unit %d appears more than once", reserve.UnitID)
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
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
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
		Units: map[State.UnitID]int64{}, CreatedAt: current.CreatedAt, UpdatedAt: now,
		AllianceObservedAt: gameState.Alliance.ObservedAt,
	}
	if next.CreatedAt.IsZero() {
		next.CreatedAt = now
	}
	wait := func(detail string, retryAt time.Time) State.StationingOperation {
		next.Phase = State.StationingPhaseWaiting
		next.StatusDetail = detail
		retryAt = retryAt.UTC()
		next.NextAttemptAt = &retryAt
		return next
	}
	if gameState.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := gameState.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		return wait("Protection Mode is preparing or active", retryAt)
	}
	source, exists := gameState.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return wait("Source castle is not present in the current player state", now.Add(autoBirdFreshStateRetry))
	}
	if request.AllianceRefreshAt.IsZero() || gameState.Alliance.ObservedAt.Before(request.AllianceRefreshAt) {
		return wait("AIN did not commit a fresh alliance roster for this castle cycle", now.Add(autoBirdFreshStateRetry))
	}
	target, found := Automation.SelectAutoBirdHolding(gameState.Alliance, source, request.MinimumRPTDays)
	if !found {
		return wait("No protected alliance bird target is available for this castle", now.Add(autoBirdNoTargetRetry))
	}
	next.TargetCastleID = target.CastleID
	next.Phase = State.StationingPhaseTargetReady
	next.DelayHours = delayHours
	next.WaitSeconds = delayHours * 3600
	next.StatusDetail = fmt.Sprintf(
		"Fresh AIN selected target %d with a %d-hour random wait; JAA inventory is next",
		target.CastleID, delayHours,
	)
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
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		current := gameState.Stationing[request.TrackingID]
		next := preparedAutoBirdManifest(*gameState, gameData, current, request, now)
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
) State.StationingOperation {
	if current.Purpose != "autoBird" ||
		current.SourceCastleID != request.SourceCastleID ||
		current.Phase != State.StationingPhaseTargetReady &&
			current.Phase != State.StationingPhaseDispatchReady {
		return current
	}
	next := current
	next.Phase = State.StationingPhaseTargetReady
	next.Units = map[State.UnitID]int64{}
	next.MovementID = 0
	next.DispatchedAt = nil
	next.ExpectedReturnAt = nil
	next.NextAttemptAt = nil
	next.SuccessCooldownUntil = nil
	next.TravelSeconds = 0
	next.UpdatedAt = now
	wait := func(detail string, retryAt time.Time) State.StationingOperation {
		next.Phase = State.StationingPhaseWaiting
		next.StatusDetail = detail
		retryAt = retryAt.UTC()
		next.NextAttemptAt = &retryAt
		return next
	}
	if gameState.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := gameState.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		return wait("Protection Mode is preparing or active", retryAt)
	}
	source, exists := gameState.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return wait("Source castle is not present in the current player state", now.Add(autoBirdFreshStateRetry))
	}
	next.UnitsObservedAt = source.UnitsObservedAt
	if request.UnitsRefreshAt.IsZero() || source.UnitsObservedAt.Before(request.UnitsRefreshAt) {
		return wait("JAA did not commit a fresh troop inventory for this castle cycle", now.Add(autoBirdFreshStateRetry))
	}
	target, targetExists := allianceHolding(gameState.Alliance, current.TargetCastleID)
	if !targetExists || !stationHoldingType(target.SlotType) ||
		target.KingdomID != source.KingdomID ||
		request.ExpectedTargetCastle != 0 && request.ExpectedTargetCastle != target.CastleID {
		return wait("The AIN-selected bird target is no longer valid", now.Add(autoBirdFreshStateRetry))
	}
	manifest, total, manifestErr := autoBirdStationManifest(gameData, source, request.Reserves)
	if manifestErr != nil {
		return wait("Could not read eligible troops from the fresh JAA: "+manifestErr.Error(), now.Add(autoBirdNoTroopsRetry))
	}
	if len(manifest) == 0 {
		return wait("Fresh JAA contains no eligible troops after tools and reserves are excluded", now.Add(autoBirdNoTroopsRetry))
	}
	if request.MinimumSend > 0 && total < request.MinimumSend {
		return wait(
			fmt.Sprintf("Fresh JAA contains %d eligible troops; minimum send is %d", total, request.MinimumSend),
			now.Add(autoBirdNoTroopsRetry),
		)
	}
	next.Phase = State.StationingPhaseDispatchReady
	next.Units = manifest
	next.StatusDetail = fmt.Sprintf(
		"Fresh JAA prepared %d troops for target %d with the recorded %d-hour random wait",
		total, target.CastleID, next.DelayHours,
	)
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
		return fmt.Errorf("%w: Auto Bird state is unavailable", Intent.ErrPlanStale)
	}
	snapshot := application.State.ReadOnlyView()
	if snapshot.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := snapshot.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		application.deferAutoBirdDispatch(request, "Protection Mode became active before Auto Bird dispatch", retryAt)
		return fmt.Errorf("%w: Protection Mode became active before Auto Bird dispatch", Intent.ErrPlanStale)
	}
	contextReady := false
	_, applyErr := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		operation, exists := gameState.Stationing[request.TrackingID]
		if !exists || operation.Purpose != "autoBird" ||
			operation.SourceCastleID != request.SourceCastleID ||
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
		next.UpdatedAt = now
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	if applyErr != nil {
		return applyErr
	}
	if !contextReady {
		return fmt.Errorf("%w: castle %d needs a fresh JAA before Auto Bird dispatch", Intent.ErrPlanStale, request.SourceCastleID)
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
	hold := func(detail string, retry time.Duration) (Intent.Step, error) {
		application.deferAutoBirdDispatch(request, detail, now.Add(retry))
		return Intent.Step{}, fmt.Errorf("%w: %s", Intent.ErrPlanStale, detail)
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(now) {
		retryAt := input.State.Player.ProtectionMode.Until().Add(time.Second)
		if !retryAt.After(now) {
			retryAt = now.Add(autoBirdFreshStateRetry)
		}
		application.deferAutoBirdDispatch(request, "Protection Mode became active before Auto Bird dispatch", retryAt)
		return Intent.Step{}, fmt.Errorf("%w: Protection Mode became active before Auto Bird dispatch", Intent.ErrPlanStale)
	}
	operation, exists := input.State.Stationing[request.TrackingID]
	if !exists || operation.Purpose != "autoBird" ||
		operation.SourceCastleID != request.SourceCastleID ||
		operation.Phase != State.StationingPhaseDispatchReady {
		return Intent.Step{}, fmt.Errorf("%w: castle %d is no longer prepared for Auto Bird", Intent.ErrPlanStale, request.SourceCastleID)
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
	manifest, total, manifestErr := autoBirdStationManifest(input.GameData, source, request.Reserves)
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
	unitIDs := make([]int64, 0, len(manifest))
	for unitID := range manifest {
		unitIDs = append(unitIDs, int64(unitID))
	}
	sort.Slice(unitIDs, func(left, right int) bool { return unitIDs[left] < unitIDs[right] })
	wireUnits := make([][2]int64, 0, len(unitIDs))
	for _, unitID := range unitIDs {
		wireUnits = append(wireUnits, [2]int64{unitID, manifest[State.UnitID(unitID)]})
	}
	dispatch, _ := json.Marshal(struct {
		SourceID State.CastleID `json:"SID"`
		TargetX  int            `json:"TX"`
		TargetY  int            `json:"TY"`
		LeaderID int            `json:"LID"`
		Wait     int            `json:"WT"`
		Booster  int            `json:"HBW"`
		Premium  int            `json:"BPC"`
		Travel   int            `json:"PTT"`
		Delay    int            `json:"SD"`
		Units    [][2]int64     `json:"A"`
	}{
		source.ID, target.X, target.Y, stationLeaderID, operation.DelayHours,
		-1, 1, 1, 0, wireUnits,
	})
	step := commandStep("Dispatch Auto Bird troops", "cds", dispatch, "cds")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
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
	_, _ = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
) (map[State.UnitID]int64, int64, error) {
	if gameData == nil {
		return nil, 0, fmt.Errorf("official game data is unavailable")
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

func randomAutoBirdDelayHours(minimum, maximum int) int {
	if maximum <= minimum {
		return minimum
	}
	return minimum + rand.IntN(maximum-minimum+1)
}

func (application *Application) captureAutoBirdMovement(
	_ context.Context,
	arguments json.RawMessage,
) error {
	request, err := decodeAutoBirdCycleRequest(arguments)
	if err != nil {
		return err
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
		movement, found := findAutoBirdMovement(*gameState, next)
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
		} else {
			next.MovementID = movement.ID
			if len(movement.Units) > 0 {
				next.Units = cloneStationUnits(movement.Units)
			}
			next.WaitSeconds = movement.WaitSeconds
			if next.WaitSeconds <= 0 {
				next.WaitSeconds = next.DelayHours * 3600
			}
			next.TravelSeconds = max(0, movement.TravelSeconds)
			expectedReturn := autoBirdMovementReturnAt(movement, next, now)
			if !expectedReturn.IsZero() {
				expectedReturn = expectedReturn.UTC()
				next.ExpectedReturnAt = &expectedReturn
				next.NextAttemptAt = &expectedReturn
				next.SuccessCooldownUntil = &expectedReturn
			}
			next.StatusDetail = fmt.Sprintf(
				"Auto Bird movement %d has %d seconds travel, %d seconds wait, and expected return %s",
				movement.ID, next.TravelSeconds, next.WaitSeconds, formatAutoBirdTime(expectedReturn),
			)
		}
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	return err
}

func findAutoBirdMovement(
	gameState State.GameState,
	operation State.StationingOperation,
) (State.MovementState, bool) {
	var best State.MovementState
	target, targetKnown := allianceHolding(gameState.Alliance, operation.TargetCastleID)
	source, sourceKnown := gameState.Castles[operation.SourceCastleID]
	for _, movement := range gameState.Movements {
		if movement.OwnerPlayerID > 0 && gameState.Player.ID > 0 &&
			movement.OwnerPlayerID != gameState.Player.ID {
			continue
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
			continue
		}
		if operation.DispatchedAt != nil && !movement.ObservedAt.IsZero() &&
			movement.ObservedAt.Before(operation.DispatchedAt.Add(-time.Second)) {
			continue
		}
		if best.ID == 0 || movement.ObservedAt.After(best.ObservedAt) ||
			movement.ObservedAt.Equal(best.ObservedAt) && movement.ID > best.ID {
			best = movement
		}
	}
	return best, best.ID > 0
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

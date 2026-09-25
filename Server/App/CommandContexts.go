package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func commandStep(name string, opcode string, payload json.RawMessage, awaitOpcode string, descriptors ...*Localization.Message) Intent.Step {
	return Intent.Step{
		Name: name, NameDescriptor: Localization.First(descriptors), Opcode: opcode, AwaitOpcode: awaitOpcode, TimeoutMillis: 10_000, SuccessCodes: []int{0},
		Command: Protocol.Command{Opcode: opcode, Payload: payload},
	}
}

func contextCommandStep(name string, opcode string, payload json.RawMessage, awaitOpcode string) Intent.Step {
	return Intent.RebuildOnResume(commandStep(name, opcode, payload, awaitOpcode))
}

func closeGameUIStep() Intent.Step {
	return Intent.RebuildOnResume(Intent.Step{Name: "Close game UI", NameDescriptor: Localization.New("server.app.close_game_ui.12ff0827", "Close game UI", nil), Action: "game.ui.close"})
}

func generalSkillsContextSteps(gameState State.GameState, commanderID State.CommanderID, evaluatedAt time.Time) []Intent.Step {
	commander, exists := gameState.Commanders[commanderID]
	if !exists || commander.GeneralID <= 0 {
		return nil
	}
	general, observed := gameState.Generals[commander.GeneralID]
	if observed && !general.ObservedAt.IsZero() && evaluatedAt.Sub(general.ObservedAt) < 5*time.Minute {
		return nil
	}
	return []Intent.Step{contextCommandStep(
		"Refresh commander general attack limits", "gie", json.RawMessage(`{}`), "gie",
	).WithNameDescriptor(Localization.New("server.app.refresh_commander_general_attack.1ffe2e87", "Refresh commander general attack limits", nil))}
}

func castleContextSteps(input Intent.PlanningContext, castle State.CastleState) []Intent.Step {
	if !castle.Focused ||
		(input.ProtocolContext.FocusedCastleID > 0 && input.ProtocolContext.FocusedCastleID != castle.ID) {
		return []Intent.Step{castleFocusStep(castle)}
	}
	if input.ProtocolContext.FocusedCastleID == castle.ID &&
		input.ProtocolContext.FocusSubcontext == State.FocusSubcontextCastle {
		return nil
	}
	// Direct planner callers that do not provide a protocol view retain the
	// legacy state-only behavior. Live engine planning always supplies it.
	if input.ProtocolContext.FocusEpoch == 0 && input.ProtocolContext.FocusedCastleID == 0 &&
		input.ProtocolContext.FocusSubcontext == State.FocusSubcontextUnknown {
		return nil
	}
	return []Intent.Step{castleRefreshStep("Re-enter focused castle from map context", castle).WithNameDescriptor(Localization.New("server.app.re_enter_focused_castle.312684ff", "Re-enter focused castle from map context", nil))}
}

func castleFocusStep(castle State.CastleState) Intent.Step {
	payload, _ := json.Marshal(struct {
		X         int             `json:"PX"`
		Y         int             `json:"PY"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.X, castle.Y, castle.KingdomID})
	step := contextCommandStep("Focus castle", "jaa", payload, "jaa").WithNameDescriptor(Localization.New("server.app.focus_castle.50e5148a", "Focus castle", nil))
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func stationCastleContextStep(castle State.CastleState) Intent.Step {
	if !castle.Focused {
		return castleFocusStep(castle)
	}
	return castleRefreshStep("Refresh station source castle", castle).WithNameDescriptor(Localization.New("server.app.refresh_station_source_castle.620aa1ad", "Refresh station source castle", nil))
}

func castleRefreshStep(name string, castle State.CastleState) Intent.Step {
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, castle.KingdomID})
	step := contextCommandStep(name, "jca", payload, "jaa")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

// JAA switches to a different castle. JCA re-enters the already-focused
// castle from world-map context and also returns a fresh JAA snapshot.
func attackCastleContextStep(castle State.CastleState) Intent.Step {
	if !castle.Focused {
		return castleFocusStep(castle)
	}
	return attackCastleRefreshStep("Refocus attack source castle", castle).WithNameDescriptor(Localization.New("server.app.refocus_attack_source_castle.da91500d", "Refocus attack source castle", nil))
}

func attackCastleRefreshStep(name string, castle State.CastleState) Intent.Step {
	return castleRefreshStep(name, castle)
}

func constructionMenuStep() Intent.Step {
	return Intent.RebuildOnResume(Intent.Step{
		Name: "Open construction-item menu", NameDescriptor: Localization.New("server.app.open_construction_item_menu.f5b3691f", "Open construction-item menu", nil), Opcode: "aec", AwaitOpcode: "aec",
		TimeoutMillis: 10_000, SuccessCodes: []int{0},
		Command: Protocol.Command{Opcode: "aec", Payload: json.RawMessage(`{}`)},
	})
}

func constructionShopContextSteps(castle State.CastleState) []Intent.Step {
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.ID, castle.KingdomID})
	return []Intent.Step{
		constructionMenuStep(),
		contextCommandStep("Refresh construction-item offers", "gbc", payload, "gbc").WithNameDescriptor(Localization.New("server.app.refresh_construction_item_offers.fd46e62d", "Refresh construction-item offers", nil)),
	}
}

// constructionSpaceLeftStep asks the server how much construction-item
// inventory room is left (C2S "csp"). The official client sends it from the
// buy slider right before a purchase; the answer is the authoritative
// fullness oracle for the purchase guard (State.ConstructionItemInventorySpaceLeft).
func constructionSpaceLeftStep() Intent.Step {
	return contextCommandStep("Refresh construction-item inventory space", "csp", json.RawMessage(`{}`), "csp").WithNameDescriptor(Localization.New("server.app.refresh_construction_item_inventory.35c78e7b", "Refresh construction-item inventory space", nil))
}

func stationRouteContextSteps(source State.CastleState, target State.AllianceHolding) []Intent.Step {
	payload, _ := json.Marshal(struct {
		TargetX int `json:"TX"`
		TargetY int `json:"TY"`
		SourceX int `json:"SX"`
		SourceY int `json:"SY"`
	}{target.X, target.Y, source.X, source.Y})
	return []Intent.Step{contextCommandStep("Preview station route", "sdi", payload, "sdi").WithNameDescriptor(Localization.New("server.app.preview_station_route.321ca54d", "Preview station route", nil))}
}

// craSetupContextSteps refreshes the same pre-attack state the game uses
// before a CRA. The game enters its world-map context before establishing the
// attack dialog and loading saved formations. All three steps are rebuilt
// after an interruption.
func craSetupContextSteps(craPayload json.RawMessage) ([]Intent.Step, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(craPayload, &fields); err != nil {
		return nil, Localization.WithError(fmt.Errorf("decode CRA setup context: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_cra_setup_context.d0d8634e", "decode CRA setup context", nil), err))
	}
	for _, key := range []string{"SX", "SY", "TX", "TY", "KID"} {
		if len(fields[key]) == 0 {
			return nil, Localization.WithError(fmt.Errorf("CRA setup context requires %s", key), Localization.New("server.app.cra_setup_context_requires.0df9435a", "CRA setup context requires {p0}", Localization.Params{"p0": fmt.Sprintf("%s", key)}))
		}
	}
	var attackDialog struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}
	if err := json.Unmarshal(craPayload, &attackDialog); err != nil {
		return nil, Localization.WithError(fmt.Errorf("decode CRA attack-dialog coordinates: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_cra_attack_dialog.d4682994", "decode CRA attack-dialog coordinates", nil), err))
	}
	attackDialogPayload, _ := json.Marshal(attackDialog)
	return []Intent.Step{
		closeGameUIStep(),
		contextCommandStep("Refresh world-map context", "gbl", json.RawMessage(`{}`), "gbl").WithNameDescriptor(Localization.New("server.app.refresh_world_map_context.c10486d7", "Refresh world-map context", nil)),
		contextCommandStep("Refresh attack-dialog context", "adi", attackDialogPayload, "adi").WithNameDescriptor(Localization.New("server.app.refresh_attack_dialog_context.9d287627", "Refresh attack-dialog context", nil)),
		contextCommandStep("Refresh saved attack presets", "gas", json.RawMessage(`{}`), "gas").WithNameDescriptor(Localization.New("server.app.refresh_saved_attack_presets.e3ddde7b", "Refresh saved attack presets", nil)),
	}, nil
}

func deferredCRACommandStep(name, resolver string, arguments, routePayload json.RawMessage, descriptors ...*Localization.Message) Intent.Step {
	return Intent.Step{
		Name: name, NameDescriptor: Localization.First(descriptors), Resolver: resolver, ResolverArguments: arguments,
		AwaitOpcode: "cra", TimeoutMillis: 10_000, SuccessCodes: []int{0},
		CommandDependencies: &Intent.CommandDependencyRequest{Opcode: "cra", Payload: routePayload},
	}
}

type craSendGuardRequest struct {
	SourceX                int                `json:"sourceX"`
	SourceY                int                `json:"sourceY"`
	TargetX                int                `json:"targetX"`
	TargetY                int                `json:"targetY"`
	KingdomID              State.KingdomID    `json:"kingdomId"`
	CommanderID            *State.CommanderID `json:"commanderId,omitempty"`
	DialogObservedAt       time.Time          `json:"dialogObservedAt"`
	MovementsObservedAfter time.Time          `json:"movementsObservedAfter,omitempty"`
}

func (application *Application) resolveCRACommandDependencies(
	_ context.Context,
	input Intent.PlanningContext,
	step Intent.Step,
) (Intent.CommandDependencyPlan, error) {
	payload := step.Command.Payload
	if len(payload) == 0 {
		payload = step.Payload
	}
	var fields struct {
		SourceX              int                `json:"SX"`
		SourceY              int                `json:"SY"`
		TargetX              int                `json:"TX"`
		TargetY              int                `json:"TY"`
		KingdomID            State.KingdomID    `json:"KID"`
		TargetTypeID         int                `json:"_citadelTargetTypeId"`
		NomadSequentialGuard json.RawMessage    `json:"_citadelNomadSequentialArrivalGuard"`
		InvasionGuard        json.RawMessage    `json:"_citadelInvasionGuard"`
		CommanderID          *State.CommanderID `json:"LID"`
		TowerCapacityCapture json.RawMessage    `json:"towerCapacityCapture"`
		FortressVerification json.RawMessage    `json:"_citadelFortressVerification"`
		ContextMode          string             `json:"_citadelContextMode"`
	}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("decode CRA send guard: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_cra_send_guard.fdbb4d52", "decode CRA send guard", nil), err))
	}
	routeKey := fmt.Sprintf(
		"%d:%d:%d:%d:%d", fields.KingdomID, fields.SourceX, fields.SourceY, fields.TargetX, fields.TargetY,
	)
	if fields.ContextMode != "" {
		if fields.ContextMode != beriCRAContextMode || fields.KingdomID != beriKingdomID {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf(
				"unsupported CRA context mode %q for kingdom %d", fields.ContextMode, fields.KingdomID,
			), Localization.New("server.app.unsupported_cra_context_mode.4fa1e26b", "unsupported CRA context mode {p0} for kingdom {p1}", Localization.Params{"p0": fmt.Sprintf("%q", fields.ContextMode), "p1": fmt.Sprintf("%d", fields.KingdomID)}))
		}
		return Intent.CommandDependencyPlan{Key: routeKey}, nil
	}
	if fields.CommanderID == nil && len(step.ResolverArguments) > 0 {
		var resolved struct {
			CommanderID *State.CommanderID `json:"commanderId"`
		}
		if json.Unmarshal(step.ResolverArguments, &resolved) == nil {
			fields.CommanderID = resolved.CommanderID
		}
	}
	if State.AttackFeatureTargetPendingAt(
		input.State, State.AttackFeatureAutoTowers, fields.KingdomID, kingdomTowerMapTypeID,
		fields.TargetX, fields.TargetY, time.Now().UTC(),
	) {
		return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf(
			"%w: tower target %d:%d has a prior Auto Towers attack awaiting settlement",
			Intent.ErrPlanStale, fields.TargetX, fields.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.49bd0e9e", "intent plan became stale before dispatch: tower target {p1}:{p2} has a prior Auto Towers attack awaiting settlement", Localization.Params{"p1": fmt.Sprintf("%d", fields.TargetX), "p2": fmt.Sprintf("%d", fields.TargetY)}))
	}
	setup, err := craSetupContextSteps(payload)
	if err != nil {
		return Intent.CommandDependencyPlan{}, err
	}
	if fields.TargetTypeID == State.MapTypeKingdomFortress {
		if len(fields.FortressVerification) == 0 {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("fortress CRA route is missing its exact-target verification"), Localization.New("server.app.fortress_cra_route_is.7ae90443", "fortress CRA route is missing its exact-target verification", nil))
		}
		var verification fortressTargetVerificationRequest
		if err := decodeIntentArguments(fields.FortressVerification, &verification); err != nil ||
			verification.SourceCastleID <= 0 || verification.KingdomID != fields.KingdomID ||
			verification.TargetX != fields.TargetX || verification.TargetY != fields.TargetY {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("fortress CRA route has invalid exact-target verification"), Localization.New("server.app.fortress_cra_route_has.c69f86b7", "fortress CRA route has invalid exact-target verification", nil))
		}
		for index := range setup {
			if setup[index].Opcode == "adi" {
				setup[index].Name = "Refresh fortress attack-dialog context"
				setup[index].Opcode = "abi"
				setup[index].AwaitOpcode = "abi"
				setup[index].Command.Opcode = "abi"
				setup[index].FinalDispatchAction = "fortress.target.verification.guard"
				setup[index].FinalDispatchArguments = append(json.RawMessage(nil), fields.FortressVerification...)
			}
		}
	}
	if fields.TargetTypeID == nomadIntentCampTypeID || fields.TargetTypeID == samuraiIntentCampTypeID {
		if len(fields.NomadSequentialGuard) == 0 {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("Nomad/Samurai CRA route is missing its sequential-arrival guard"), Localization.New("server.app.nomad_samurai_cra_route.3267cc42", "Nomad/Samurai CRA route is missing its sequential-arrival guard", nil))
		}
		var guard nomadSequentialArrivalGuardRequest
		if err := decodeIntentArguments(fields.NomadSequentialGuard, &guard); err != nil ||
			guard.EventID <= 0 || guard.KingdomID != fields.KingdomID || guard.TargetTypeID != fields.TargetTypeID ||
			guard.TargetX != fields.TargetX || guard.TargetY != fields.TargetY {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("Nomad/Samurai CRA route has an invalid sequential-arrival guard"), Localization.New("server.app.nomad_samurai_cra_route.4ad90e53", "Nomad/Samurai CRA route has an invalid sequential-arrival guard", nil))
		}
		for index := range setup {
			if setup[index].Opcode == "adi" {
				setup[index].FinalDispatchAction = "nomad.attack.sequential_arrival.guard"
				setup[index].FinalDispatchArguments = append(json.RawMessage(nil), fields.NomadSequentialGuard...)
			}
		}
	}
	if fields.TargetTypeID == State.MapTypeForeignLord || fields.TargetTypeID == State.MapTypeBloodcrow {
		if len(fields.InvasionGuard) == 0 {
			return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf("invasion CRA route is missing its occurrence-bound launch guard"), Localization.New("server.app.invasion_cra_route_is.975d776e", "invasion CRA route is missing its occurrence-bound launch guard", nil))
		}
		probeReservationArguments, err := invasionTargetOnlyReservationArguments(payload)
		if err != nil {
			return Intent.CommandDependencyPlan{}, err
		}
		guardedSetup := make([]Intent.Step, 0, len(setup)+1)
		for _, dependency := range setup {
			if dependency.Opcode == "adi" {
				dependency.StaleCodes = []int{95}
				dependency.PreDispatchAction = "invasion.target.reserve"
				dependency.PreDispatchArguments = append(json.RawMessage(nil), probeReservationArguments...)
				dependency.DefinitiveSendFailureAction = "invasion.target.release"
				dependency.DefinitiveSendFailureArguments = append(json.RawMessage(nil), probeReservationArguments...)
				dependency.DefinitiveResponseFailureAction = "invasion.target.release"
				dependency.DefinitiveResponseFailureArguments = append(json.RawMessage(nil), probeReservationArguments...)
				dependency.StaleResponseAction = "invasion.target.cooldown"
				dependency.StaleResponseArguments = append(json.RawMessage(nil), probeReservationArguments...)
				dependency.ResponseProjectionFailureIndeterminate = true
				guardedSetup = append(guardedSetup, dependency, Intent.Step{
					Name: "Release confirmed invasion attack-dialog probe", NameDescriptor: Localization.New("server.app.release_confirmed_invasion_attack.5fc341da", "Release confirmed invasion attack-dialog probe", nil),
					Action: "invasion.target.release", ActionArguments: append(json.RawMessage(nil), probeReservationArguments...),
				})
				continue
			}
			guardedSetup = append(guardedSetup, dependency)
		}
		setup = guardedSetup
	}
	guardedAt := time.Now().UTC()
	target, towerTarget := input.State.LookupMapObservation(fields.KingdomID, fmt.Sprintf("%d:%d", fields.TargetX, fields.TargetY))
	towerTarget = towerTarget && target.TypeID == kingdomTowerMapTypeID
	if target.TypeID == State.MapTypeKingdomFortress && State.AttackFeatureTargetPendingAt(
		input.State, State.AttackFeatureAutoFortress, fields.KingdomID, State.MapTypeKingdomFortress,
		fields.TargetX, fields.TargetY, time.Now().UTC(),
	) {
		return Intent.CommandDependencyPlan{}, Localization.WithError(fmt.Errorf(
			"%w: fortress target %d:%d has a prior Auto Fortress attack awaiting settlement",
			Intent.ErrPlanStale, fields.TargetX, fields.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.3f7be492", "intent plan became stale before dispatch: fortress target {p1}:{p2} has a prior Auto Fortress attack awaiting settlement", Localization.Params{"p1": fmt.Sprintf("%d", fields.TargetX), "p2": fmt.Sprintf("%d", fields.TargetY)}))
	}
	var movementsObservedAfter time.Time
	if fields.CommanderID != nil {
		movementsObservedAfter = guardedAt
		movementStep := contextCommandStep("Refresh commander movements before CRA launch", "gam", json.RawMessage(`{}`), "gam").WithNameDescriptor(Localization.New("server.app.refresh_commander_movements_before.1d53861a", "Refresh commander movements before CRA launch", nil))
		movementStep.ResponseBarrier = Intent.ResponseBarrierCommitted
		setup = append([]Intent.Step{movementStep}, setup...)
	}
	if towerTarget {
		if len(fields.TowerCapacityCapture) > 0 {
			setup = append(setup, Intent.Step{
				Name: "Capture fresh tower capacity", NameDescriptor: Localization.New("server.app.capture_fresh_tower_capacity.7af429bb", "Capture fresh tower capacity", nil), Action: "tower.capacity.capture",
				ActionArguments: append(json.RawMessage(nil), fields.TowerCapacityCapture...),
			})
		}
	}
	guardArguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: fields.SourceX, SourceY: fields.SourceY, TargetX: fields.TargetX, TargetY: fields.TargetY,
		KingdomID: fields.KingdomID, CommanderID: fields.CommanderID,
		DialogObservedAt: guardedAt, MovementsObservedAfter: movementsObservedAfter,
	})
	guards := []Intent.Step{{
		Name: "Verify authoritative CRA target", NameDescriptor: Localization.New("server.app.verify_authoritative_cra_target.551988c7", "Verify authoritative CRA target", nil), Action: "attack.cra.send.guard", ActionArguments: guardArguments,
	}}
	if fields.TargetTypeID == State.MapTypeForeignLord || fields.TargetTypeID == State.MapTypeBloodcrow {
		guards = append(guards, Intent.Step{
			Name: "Verify active invasion occurrence and score boundary", NameDescriptor: Localization.New("server.app.verify_active_invasion_occurrence.71084c1f", "Verify active invasion occurrence and score boundary", nil), Action: "invasion.attack.guard",
			ActionArguments: append(json.RawMessage(nil), fields.InvasionGuard...),
		})
	}
	return Intent.CommandDependencyPlan{
		Key:   routeKey,
		Steps: append(setup, guards...),
	}, nil
}

func (application *Application) guardCRASend(_ context.Context, arguments json.RawMessage) error {
	var request craSendGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	dialog := state.AttackDialog
	if dialog.ObservedAt.IsZero() || dialog.ObservedAt.Before(request.DialogObservedAt) {
		return Localization.WithError(fmt.Errorf("CRA attack dialog was not refreshed at send time"), Localization.New("server.app.cra_attack_dialog_was.cb5317f9", "CRA attack dialog was not refreshed at send time", nil))
	}
	source, exists := state.Castles[dialog.SourceCastleID]
	if !exists || source.X != request.SourceX || source.Y != request.SourceY ||
		dialog.KingdomID != request.KingdomID || dialog.Target.TypeID <= 0 ||
		dialog.Target.X != request.TargetX || dialog.Target.Y != request.TargetY {
		return Localization.WithError(fmt.Errorf("authoritative attack dialog does not match CRA route %d:%d to %d:%d",
			request.SourceX, request.SourceY, request.TargetX, request.TargetY), Localization.New("server.app.authoritative_attack_dialog_does.fc06eca5", "authoritative attack dialog does not match CRA route {p0}:{p1} to {p2}:{p3}", Localization.Params{"p0": request.SourceX, "p1": request.SourceY, "p2": request.TargetX, "p3": request.TargetY}))
	}
	if dialog.Target.TowerCooldownRemaining > 0 || dialog.Target.EventCampCooldownRemaining > 0 ||
		stormAttackDialogUnavailable(dialog.Target) {
		if dialog.Target.TypeID == khanCampTypeID || dialog.Target.TypeID == nomadIntentCampTypeID ||
			dialog.Target.TypeID == samuraiIntentCampTypeID {
			return Localization.WithError(fmt.Errorf("%w: CRA target %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.96837c8a", "intent plan became stale before dispatch: CRA target {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		return Localization.WithError(fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY), Localization.New("server.app.cra_target_p_p.f4664247", "CRA target {p0}:{p1} is on cooldown", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if request.CommanderID != nil {
		if request.MovementsObservedAfter.IsZero() || state.MovementSnapshot.ObservedAt.IsZero() ||
			state.MovementSnapshot.ObservedAt.Before(request.MovementsObservedAfter) {
			return Localization.WithError(fmt.Errorf("CRA launch does not have a fresh movement snapshot"), Localization.New("server.app.cra_launch_does_not.4e14a98c", "CRA launch does not have a fresh movement snapshot", nil))
		}
		commander, found := state.Commanders[*request.CommanderID]
		if !found || !commander.Available ||
			State.CommanderHasActiveMovementAt(state, *request.CommanderID, time.Now().UTC()) ||
			State.InvasionCommanderReserved(state, *request.CommanderID) {
			return Localization.WithError(fmt.Errorf("%w: CRA commander %d is no longer available", Intent.ErrPlanStale, *request.CommanderID), Localization.New("server.app.intent_plan_became_stale.97faff4d", "intent plan became stale before dispatch: CRA commander {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", *request.CommanderID)}))
		}
	}
	key := fmt.Sprintf("%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	switch dialog.Target.TypeID {
	case kingdomTowerMapTypeID, State.MapTypeKingdomFortress:
		if request.CommanderID == nil {
			return Localization.WithError(fmt.Errorf("CRA tower launch does not identify a commander"), Localization.New("server.app.cra_tower_launch_does.d14c6003", "CRA tower launch does not identify a commander", nil))
		}
		if cooldown, found := state.LookupTowerCooldown(key); found && cooldown.PendingCooldownRefresh {
			return Localization.WithError(fmt.Errorf("CRA target %d:%d is awaiting a post-victory cooldown refresh", request.TargetX, request.TargetY), Localization.New("server.app.cra_target_p_p.330c40d5", "CRA target {p0}:{p1} is awaiting a post-victory cooldown refresh", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
		}
	case nomadIntentCampTypeID, samuraiIntentCampTypeID:
		if cooldown, found := state.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
			return Localization.WithError(fmt.Errorf("%w: CRA target %d:%d is awaiting a post-victory cooldown refresh", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.039d8a1c", "intent plan became stale before dispatch: CRA target {p1}:{p2} is awaiting a post-victory cooldown refresh", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
	case khanCampTypeID:
		if cooldown, found := state.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA target %d:%d is awaiting a post-victory cooldown refresh",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.039d8a1c", "intent plan became stale before dispatch: CRA target {p1}:{p2} is awaiting a post-victory cooldown refresh", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
	case State.MapTypeForeignLord, State.MapTypeBloodcrow:
		if !dialog.Target.InvasionAvailabilityKnown || dialog.Target.ObjectID <= 0 {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA invasion target %d:%d does not have confirmed attack availability",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.3297d0bc", "intent plan became stale before dispatch: CRA invasion target {p1}:{p2} does not have confirmed attack availability", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		if dialog.Target.InvasionProtected || state.Invasion.TargetUnavailable(request.KingdomID, request.TargetX, request.TargetY) {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA invasion target %d:%d is hidden or protected",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.00d45735", "intent plan became stale before dispatch: CRA invasion target {p1}:{p2} is hidden or protected", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		if _, reserved := state.Invasion.TargetReservation(request.KingdomID, request.TargetX, request.TargetY); reserved {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA invasion target %d:%d has an unresolved launch",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.e3a9f3c9", "intent plan became stale before dispatch: CRA invasion target {p1}:{p2} has an unresolved launch", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		if State.AttackFeatureTargetPendingAt(
			state, State.AttackFeatureAutoInvasion, request.KingdomID, dialog.Target.TypeID,
			request.TargetX, request.TargetY, time.Now().UTC(),
		) {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA invasion target %d:%d has a prior attack awaiting settlement",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.d4513ec1", "intent plan became stale before dispatch: CRA invasion target {p1}:{p2} has a prior attack awaiting settlement", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		if State.AnyActiveMovementAtMapTarget(state, State.MapTargetKey{
			KingdomID: request.KingdomID, TypeID: dialog.Target.TypeID,
			X: request.TargetX, Y: request.TargetY,
		}, time.Now().UTC()) {
			return Localization.WithError(fmt.Errorf(
				"%w: CRA invasion target %d:%d already has an active movement",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.0009ef24", "intent plan became stale before dispatch: CRA invasion target {p1}:{p2} already has an active movement", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
	}
	if target, found := state.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)); found &&
		target.TypeID == dialog.Target.TypeID {
		now := time.Now().UTC()
		switch target.TypeID {
		case kingdomTowerMapTypeID:
			if appDungeonCooldownRemaining(state, target, now) > 0 {
				return Localization.WithError(fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY), Localization.New("server.app.cra_target_p_p.f4664247", "CRA target {p0}:{p1} is on cooldown", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
			}
		case State.MapTypeKingdomFortress:
			if fortressCooldownRemaining(state, target, now) > 0 {
				return Localization.WithError(fmt.Errorf("CRA fortress target %d:%d is on cooldown", request.TargetX, request.TargetY), Localization.New("server.app.cra_fortress_target_p.197a3bb5", "CRA fortress target {p0}:{p1} is on cooldown", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
			}
		case nomadIntentCampTypeID, samuraiIntentCampTypeID:
			if nomadAppCooldownRemaining(state, target, now) > 0 {
				return Localization.WithError(fmt.Errorf("%w: CRA target %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.96837c8a", "intent plan became stale before dispatch: CRA target {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
			}
		case khanCampTypeID:
			if appDungeonCooldownRemaining(state, target, now) > 0 {
				return Localization.WithError(fmt.Errorf("%w: CRA target %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.96837c8a", "intent plan became stale before dispatch: CRA target {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
			}
		case stormIntentIslandMapTypeID, stormIntentFortMapTypeID:
			if stormTargetCooldownRemaining(target, now) > 0 {
				return Localization.WithError(fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY), Localization.New("server.app.cra_target_p_p.f4664247", "CRA target {p0}:{p1} is on cooldown", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
			}
		}
	}
	return nil
}

func equipmentUpgradeContextStep() Intent.Step {
	return contextCommandStep("Open equipment upgrade menu", "gnr", json.RawMessage(`{}`), "gnr").WithNameDescriptor(Localization.New("server.app.open_equipment_upgrade_menu.4289f94b", "Open equipment upgrade menu", nil))
}

func kingdomTransportContextStep() Intent.Step {
	return contextCommandStep("Refresh kingdom transports", "kpi", json.RawMessage(`{}`), "kpi").WithNameDescriptor(Localization.New("server.app.refresh_kingdom_transports.ad5d0438", "Refresh kingdom transports", nil))
}

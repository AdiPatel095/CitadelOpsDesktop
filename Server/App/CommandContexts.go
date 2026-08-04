package App

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func commandStep(name string, opcode string, payload json.RawMessage, awaitOpcode string) Intent.Step {
	return Intent.Step{
		Name: name, Opcode: opcode, AwaitOpcode: awaitOpcode, TimeoutMillis: 10_000, SuccessCodes: []int{0},
		Command: Protocol.Command{Opcode: opcode, Payload: payload},
	}
}

func contextCommandStep(name string, opcode string, payload json.RawMessage, awaitOpcode string) Intent.Step {
	return Intent.RebuildOnResume(commandStep(name, opcode, payload, awaitOpcode))
}

func closeGameUIStep() Intent.Step {
	return Intent.RebuildOnResume(Intent.Step{Name: "Close game UI", Action: "game.ui.close"})
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
	)}
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
	return []Intent.Step{castleRefreshStep("Re-enter focused castle from map context", castle)}
}

func castleFocusStep(castle State.CastleState) Intent.Step {
	payload, _ := json.Marshal(struct {
		X         int             `json:"PX"`
		Y         int             `json:"PY"`
		KingdomID State.KingdomID `json:"KID"`
	}{castle.X, castle.Y, castle.KingdomID})
	step := contextCommandStep("Focus castle", "jaa", payload, "jaa")
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func stationCastleContextStep(castle State.CastleState) Intent.Step {
	if !castle.Focused {
		return castleFocusStep(castle)
	}
	return castleRefreshStep("Refresh station source castle", castle)
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
	return attackCastleRefreshStep("Refocus attack source castle", castle)
}

func attackCastleRefreshStep(name string, castle State.CastleState) Intent.Step {
	return castleRefreshStep(name, castle)
}

func constructionMenuStep() Intent.Step {
	return Intent.RebuildOnResume(Intent.Step{
		Name: "Open construction-item menu", Opcode: "aec", AwaitOpcode: "aec",
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
		contextCommandStep("Refresh construction-item offers", "gbc", payload, "gbc"),
	}
}

func stationRouteContextSteps(source State.CastleState, target State.AllianceHolding) []Intent.Step {
	payload, _ := json.Marshal(struct {
		TargetX int `json:"TX"`
		TargetY int `json:"TY"`
		SourceX int `json:"SX"`
		SourceY int `json:"SY"`
	}{target.X, target.Y, source.X, source.Y})
	return []Intent.Step{contextCommandStep("Preview station route", "sdi", payload, "sdi")}
}

// craSetupContextSteps refreshes the same pre-attack state the game uses
// before a CRA. The game enters its world-map context before establishing the
// attack dialog and loading saved formations. All three steps are rebuilt
// after an interruption.
func craSetupContextSteps(craPayload json.RawMessage) ([]Intent.Step, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(craPayload, &fields); err != nil {
		return nil, fmt.Errorf("decode CRA setup context: %w", err)
	}
	for _, key := range []string{"SX", "SY", "TX", "TY", "KID"} {
		if len(fields[key]) == 0 {
			return nil, fmt.Errorf("CRA setup context requires %s", key)
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
		return nil, fmt.Errorf("decode CRA attack-dialog coordinates: %w", err)
	}
	attackDialogPayload, _ := json.Marshal(attackDialog)
	return []Intent.Step{
		closeGameUIStep(),
		contextCommandStep("Refresh world-map context", "gbl", json.RawMessage(`{}`), "gbl"),
		contextCommandStep("Refresh attack-dialog context", "adi", attackDialogPayload, "adi"),
		contextCommandStep("Refresh saved attack presets", "gas", json.RawMessage(`{}`), "gas"),
	}, nil
}

func deferredCRACommandStep(name, resolver string, arguments, routePayload json.RawMessage) Intent.Step {
	return Intent.Step{
		Name: name, Resolver: resolver, ResolverArguments: arguments,
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
		CommanderID          *State.CommanderID `json:"LID"`
		TowerCapacityCapture json.RawMessage    `json:"towerCapacityCapture"`
		ContextMode          string             `json:"_citadelContextMode"`
	}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Intent.CommandDependencyPlan{}, fmt.Errorf("decode CRA send guard: %w", err)
	}
	routeKey := fmt.Sprintf(
		"%d:%d:%d:%d:%d", fields.KingdomID, fields.SourceX, fields.SourceY, fields.TargetX, fields.TargetY,
	)
	if fields.ContextMode != "" {
		if fields.ContextMode != beriCRAContextMode || fields.KingdomID != beriKingdomID {
			return Intent.CommandDependencyPlan{}, fmt.Errorf(
				"unsupported CRA context mode %q for kingdom %d", fields.ContextMode, fields.KingdomID,
			)
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
		return Intent.CommandDependencyPlan{}, fmt.Errorf(
			"%w: tower target %d:%d has a prior Auto Towers attack awaiting settlement",
			Intent.ErrPlanStale, fields.TargetX, fields.TargetY,
		)
	}
	setup, err := craSetupContextSteps(payload)
	if err != nil {
		return Intent.CommandDependencyPlan{}, err
	}
	guardedAt := time.Now().UTC()
	target, towerTarget := input.State.Map[fields.KingdomID][fmt.Sprintf("%d:%d", fields.TargetX, fields.TargetY)]
	towerTarget = towerTarget && target.TypeID == kingdomTowerMapTypeID
	var movementsObservedAfter time.Time
	if fields.CommanderID != nil {
		movementsObservedAfter = guardedAt
		movementStep := contextCommandStep("Refresh commander movements before CRA launch", "gam", json.RawMessage(`{}`), "gam")
		movementStep.ResponseBarrier = Intent.ResponseBarrierCommitted
		setup = append([]Intent.Step{movementStep}, setup...)
	}
	if towerTarget {
		if len(fields.TowerCapacityCapture) > 0 {
			setup = append(setup, Intent.Step{
				Name: "Capture fresh tower capacity", Action: "tower.capacity.capture",
				ActionArguments: append(json.RawMessage(nil), fields.TowerCapacityCapture...),
			})
		}
	}
	guardArguments, _ := json.Marshal(craSendGuardRequest{
		SourceX: fields.SourceX, SourceY: fields.SourceY, TargetX: fields.TargetX, TargetY: fields.TargetY,
		KingdomID: fields.KingdomID, CommanderID: fields.CommanderID,
		DialogObservedAt: guardedAt, MovementsObservedAfter: movementsObservedAfter,
	})
	return Intent.CommandDependencyPlan{
		Key: routeKey,
		Steps: append(setup, Intent.Step{
			Name: "Verify authoritative CRA target", Action: "attack.cra.send.guard", ActionArguments: guardArguments,
		}),
	}, nil
}

func (application *Application) guardCRASend(_ context.Context, arguments json.RawMessage) error {
	var request craSendGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	dialog := state.AttackDialog
	if dialog.ObservedAt.IsZero() || dialog.ObservedAt.Before(request.DialogObservedAt) {
		return fmt.Errorf("CRA attack dialog was not refreshed at send time")
	}
	source, exists := state.Castles[dialog.SourceCastleID]
	if !exists || source.X != request.SourceX || source.Y != request.SourceY ||
		dialog.KingdomID != request.KingdomID || dialog.Target.TypeID <= 0 ||
		dialog.Target.X != request.TargetX || dialog.Target.Y != request.TargetY {
		return fmt.Errorf("authoritative attack dialog does not match CRA route %d:%d to %d:%d",
			request.SourceX, request.SourceY, request.TargetX, request.TargetY)
	}
	if dialog.Target.TowerCooldownRemaining > 0 || dialog.Target.EventCampCooldownRemaining > 0 ||
		stormAttackDialogUnavailable(dialog.Target) {
		if dialog.Target.TypeID == khanCampTypeID {
			return fmt.Errorf("%w: CRA target %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY)
		}
		return fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY)
	}
	if request.CommanderID != nil {
		if request.MovementsObservedAfter.IsZero() || state.MovementSnapshot.ObservedAt.IsZero() ||
			state.MovementSnapshot.ObservedAt.Before(request.MovementsObservedAfter) {
			return fmt.Errorf("CRA launch does not have a fresh movement snapshot")
		}
		commander, found := state.Commanders[*request.CommanderID]
		if !found || !commander.Available ||
			State.CommanderHasActiveMovementAt(state, *request.CommanderID, time.Now().UTC()) {
			return fmt.Errorf("%w: CRA commander %d is no longer available", Intent.ErrPlanStale, *request.CommanderID)
		}
	}
	key := fmt.Sprintf("%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	switch dialog.Target.TypeID {
	case kingdomTowerMapTypeID:
		if request.CommanderID == nil {
			return fmt.Errorf("CRA tower launch does not identify a commander")
		}
		if cooldown, found := state.TowerCooldowns[key]; found && cooldown.PendingCooldownRefresh {
			return fmt.Errorf("CRA target %d:%d is awaiting a post-victory cooldown refresh", request.TargetX, request.TargetY)
		}
	case nomadIntentCampTypeID, samuraiIntentCampTypeID:
		if cooldown, found := state.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
			return fmt.Errorf("CRA target %d:%d is awaiting a post-victory cooldown refresh", request.TargetX, request.TargetY)
		}
	case khanCampTypeID:
		if cooldown, found := state.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
			return fmt.Errorf(
				"%w: CRA target %d:%d is awaiting a post-victory cooldown refresh",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			)
		}
	}
	if target, found := state.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]; found &&
		target.TypeID == dialog.Target.TypeID {
		now := time.Now().UTC()
		switch target.TypeID {
		case kingdomTowerMapTypeID:
			if appDungeonCooldownRemaining(state, target, now) > 0 {
				return fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY)
			}
		case nomadIntentCampTypeID, samuraiIntentCampTypeID:
			if nomadAppCooldownRemaining(state, target, now) > 0 {
				return fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY)
			}
		case khanCampTypeID:
			if appDungeonCooldownRemaining(state, target, now) > 0 {
				return fmt.Errorf("%w: CRA target %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY)
			}
		case stormIntentIslandMapTypeID, stormIntentFortMapTypeID:
			if stormTargetCooldownRemaining(target, now) > 0 {
				return fmt.Errorf("CRA target %d:%d is on cooldown", request.TargetX, request.TargetY)
			}
		}
	}
	return nil
}

func equipmentUpgradeContextStep() Intent.Step {
	return contextCommandStep("Open equipment upgrade menu", "gnr", json.RawMessage(`{}`), "gnr")
}

func kingdomTransportContextStep() Intent.Step {
	return contextCommandStep("Refresh kingdom transports", "kpi", json.RawMessage(`{}`), "kpi")
}

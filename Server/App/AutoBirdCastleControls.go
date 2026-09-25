package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type autoBirdCastleControlRequest struct {
	SourceCastleID  State.CastleID `json:"sourceCastleId"`
	Action          string         `json:"action"`
	DurationMinutes int            `json:"durationMinutes,omitempty"`
}

func decodeAutoBirdCastleControl(arguments json.RawMessage) (autoBirdCastleControlRequest, error) {
	var request autoBirdCastleControlRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, err
	}
	if request.SourceCastleID <= 0 {
		return request, Localization.WithError(fmt.Errorf("an owned castle is required"), Localization.New("server.app.an_owned_castle_is.266fdc86", "an owned castle is required", nil))
	}
	if request.Action != "pause" && request.Action != "resume" && request.Action != "resend" {
		return request, Localization.WithError(fmt.Errorf("choose pause, resume, or resend"), Localization.New("server.app.choose_pause_resume_or.ef523e3e", "choose pause, resume, or resend", nil))
	}
	if request.DurationMinutes < 0 || request.DurationMinutes > 10080 || request.Action != "pause" && request.DurationMinutes != 0 {
		return request, Localization.WithError(fmt.Errorf("pause duration must be 1 minute through 7 days, or zero for an indefinite pause"), Localization.New("server.app.pause_duration_must_be.52abf45a", "pause duration must be 1 minute through 7 days, or zero for an indefinite pause", nil))
	}
	return request, nil
}

func planAutoBirdCastleControl(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := decodeAutoBirdCastleControl(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, ok := input.State.Castles[request.SourceCastleID]; !ok {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not owned", request.SourceCastleID), Localization.New("server.app.castle_p_is_not.b7f65b1d", "castle {p0} is not owned", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	return Intent.Plan{
		// Controls can interrupt a cycle between commands; every dispatch rechecks
		// the control revision. They must not wait for that cycle's claim.
		Claims:  []string{fmt.Sprintf("auto-bird-control:%d", request.SourceCastleID)},
		Summary: fmt.Sprintf("%s Auto Bird for castle %d", request.Action, request.SourceCastleID), SummaryDescriptor: Localization.New("server.app.p_auto_bird_for.3128212d", "{p0} Auto Bird for castle {p1}", Localization.Params{"p0": fmt.Sprintf("%s", request.Action), "p1": fmt.Sprintf("%d", request.SourceCastleID)}),
		Steps: []Intent.Step{{Name: "Update castle Auto Bird control", NameDescriptor: Localization.New("server.app.update_castle_auto_bird.419c9665", "Update castle Auto Bird control", nil), Action: "auto_bird.castle.control", ActionArguments: arguments}},
	}, nil
}

func (application *Application) controlAutoBirdCastle(_ context.Context, arguments json.RawMessage) error {
	request, err := decodeAutoBirdCastleControl(arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(state *State.GameState) ([]string, bool, error) {
		if _, ok := state.Castles[request.SourceCastleID]; !ok {
			return nil, false, Localization.WithError(fmt.Errorf("castle %d is not owned", request.SourceCastleID), Localization.New("server.app.castle_p_is_not.b7f65b1d", "castle {p0} is not owned", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
		}
		id := State.AutoBirdControlID(request.SourceCastleID)
		control := state.AutoBirdControl(request.SourceCastleID)
		now := time.Now().UTC()
		if !now.After(control.UpdatedAt) {
			now = control.UpdatedAt.Add(time.Nanosecond)
		}
		control.ID, control.Purpose, control.SourceCastleID = id, "autoBirdControl", request.SourceCastleID
		control.UpdatedAt = now
		if control.CreatedAt.IsZero() {
			control.CreatedAt = now
		}
		switch request.Action {
		case "pause":
			control.Paused, control.PausedUntil = true, nil
			if request.DurationMinutes > 0 {
				until := now.Add(time.Duration(request.DurationMinutes) * time.Minute)
				control.PausedUntil = &until
			}
		case "resume":
			control.Paused, control.PausedUntil = false, nil
		case "resend":
			delete(state.Stationing, fmt.Sprintf("autoBird:%d", request.SourceCastleID))
			control.RescanRequested = true
		}
		state.Stationing[id] = control
		return []string{"stationing"}, true, nil
	})
	return err
}

func validateAutoBirdControl(state State.GameState, request autoBirdCycleRequest, now time.Time) error {
	if autoBirdPresetWindowExpired(request, now) {
		return Localization.WithError(fmt.Errorf("%w: the selected Auto Bird preset period has ended", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.574a3499", "intent plan became stale before dispatch: the selected Auto Bird preset period has ended", nil))
	}

	if state.AutoBirdPaused(request.SourceCastleID, now) {
		return Localization.WithError(fmt.Errorf("%w: Auto Bird is paused for castle %d", Intent.ErrPlanStale, request.SourceCastleID), Localization.New("server.app.intent_plan_became_stale.8a2b22ad", "intent plan became stale before dispatch: Auto Bird is paused for castle {p1}", Localization.Params{"p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if !state.AutoBirdControl(request.SourceCastleID).UpdatedAt.Equal(request.ControlRevision) {
		return Localization.WithError(fmt.Errorf("%w: Auto Bird castle control changed; prepare a fresh cycle", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.5317f17d", "intent plan became stale before dispatch: Auto Bird castle control changed; prepare a fresh cycle", nil))
	}
	return nil
}

type autoBirdBatchGuardRequest struct {
	TargetOwner State.PlayerID       `json:"targetOwner"`
	Cycle       autoBirdCycleRequest `json:"cycle"`
	Payload     json.RawMessage      `json:"payload"`
}

func (application *Application) guardAutoBirdBatch(ctx context.Context, arguments json.RawMessage) error {
	var request autoBirdBatchGuardRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if application.Configuration == nil || !Automation.AutoBirdDispatchAllowed(application.Configuration.Snapshot(), request.Cycle.PresetID, time.Now()) {
		return fmt.Errorf("%w: Auto Bird is disabled", Intent.ErrPlanStale)
	}
	state := application.State.Snapshot()
	if err := validateStationSession(state, "autoBird", request.Cycle.ConnectionGeneration, time.Now()); err != nil {
		return err
	}
	op, ok := state.Stationing[request.Cycle.TrackingID]
	if !ok || op.Purpose != "autoBird" || op.SourceCastleID != request.Cycle.SourceCastleID || op.TargetCastleID != request.Cycle.ExpectedTargetCastle || op.PresetID != request.Cycle.PresetID {
		return fmt.Errorf("%w: Auto Bird cycle changed before dispatch", Intent.ErrPlanStale)
	}
	target, _ := allianceHolding(state.Alliance, request.Cycle.ExpectedTargetCastle)
	if request.TargetOwner <= 0 || target.PlayerID != request.TargetOwner {
		return fmt.Errorf("%w: station target owner changed", Intent.ErrPlanStale)
	}
	if err := validateStationAuthority(state, request.Cycle.SourceCastleID, request.Cycle.ExpectedTargetCastle, request.Cycle.DispatchStartedAt, request.Cycle.MinimumRPTDays, time.Now()); err != nil {
		return err
	}
	if err := validateStationPayload(state, request.Cycle.ExpectedTargetCastle, request.Payload); err != nil {
		return err
	}
	if err := validateAutoBirdControl(application.State.Snapshot(), request.Cycle, time.Now().UTC()); err != nil {
		return err
	}
	var payload struct {
		SID State.CastleID `json:"SID"`
		A   [][2]int64     `json:"A"`
	}
	if err := json.Unmarshal(request.Payload, &payload); err != nil {
		return err
	}
	if application.autoBirdDirewolvesProtected(application.State.Snapshot(), payload.SID, time.Now().UTC()) {
		for _, unit := range payload.A {
			if State.UnitID(unit[0]) == GameData.DirewolfUnitID {
				return Localization.WithError(fmt.Errorf("%w: Auto Fortress now reserves every Direwolf at castle %d; rebuild the Auto Bird manifest", Intent.ErrPlanStale, payload.SID), Localization.New("server.app.intent_plan_became_stale.e3ba5725", "intent plan became stale before dispatch: Auto Fortress now reserves every Direwolf at castle {p1}; rebuild the Auto Bird manifest", Localization.Params{"p1": fmt.Sprintf("%d", payload.SID)}))
			}
		}
	}
	return application.guardSupportBatch(ctx, request.Payload)
}

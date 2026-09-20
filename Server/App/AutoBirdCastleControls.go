package App

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

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
		return request, fmt.Errorf("an owned castle is required")
	}
	if request.Action != "pause" && request.Action != "resume" && request.Action != "resend" {
		return request, fmt.Errorf("choose pause, resume, or resend")
	}
	if request.DurationMinutes < 0 || request.DurationMinutes > 10080 || request.Action != "pause" && request.DurationMinutes != 0 {
		return request, fmt.Errorf("pause duration must be 1 minute through 7 days, or zero for an indefinite pause")
	}
	return request, nil
}

func planAutoBirdCastleControl(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, err := decodeAutoBirdCastleControl(arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, ok := input.State.Castles[request.SourceCastleID]; !ok {
		return Intent.Plan{}, fmt.Errorf("castle %d is not owned", request.SourceCastleID)
	}
	return Intent.Plan{
		// Controls can interrupt a cycle between commands; every dispatch rechecks
		// the control revision. They must not wait for that cycle's claim.
		Claims:  []string{fmt.Sprintf("auto-bird-control:%d", request.SourceCastleID)},
		Summary: fmt.Sprintf("%s Auto Bird for castle %d", request.Action, request.SourceCastleID),
		Steps:   []Intent.Step{{Name: "Update castle Auto Bird control", Action: "auto_bird.castle.control", ActionArguments: arguments}},
	}, nil
}

func (application *Application) controlAutoBirdCastle(_ context.Context, arguments json.RawMessage) error {
	request, err := decodeAutoBirdCastleControl(arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentStationing), func(state *State.GameState) ([]string, bool, error) {
		if _, ok := state.Castles[request.SourceCastleID]; !ok {
			return nil, false, fmt.Errorf("castle %d is not owned", request.SourceCastleID)
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
		return fmt.Errorf("%w: the selected Auto Bird preset period has ended", Intent.ErrPlanStale)
	}

	if state.AutoBirdPaused(request.SourceCastleID, now) {
		return fmt.Errorf("%w: Auto Bird is paused for castle %d", Intent.ErrPlanStale, request.SourceCastleID)
	}
	if !state.AutoBirdControl(request.SourceCastleID).UpdatedAt.Equal(request.ControlRevision) {
		return fmt.Errorf("%w: Auto Bird castle control changed; prepare a fresh cycle", Intent.ErrPlanStale)
	}
	return nil
}

type autoBirdBatchGuardRequest struct {
	Cycle   autoBirdCycleRequest `json:"cycle"`
	Payload json.RawMessage      `json:"payload"`
}

func (application *Application) guardAutoBirdBatch(ctx context.Context, arguments json.RawMessage) error {
	var request autoBirdBatchGuardRequest
	if err := json.Unmarshal(arguments, &request); err != nil {
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
				return fmt.Errorf("%w: Auto Fortress now reserves every Direwolf at castle %d; rebuild the Auto Bird manifest", Intent.ErrPlanStale, payload.SID)
			}
		}
	}
	return application.guardSupportBatch(ctx, request.Payload)
}

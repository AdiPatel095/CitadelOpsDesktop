package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const maximumRiftMaidenRunAttacks = 9999

type riftMaidenRunStartRequest struct {
	AttackCount        int                 `json:"attackCount"`
	UnitID             State.UnitID        `json:"unitWodID"`
	HorseTravelBoostID int                 `json:"horseTravelBoostId"`
	CommanderIDs       []State.CommanderID `json:"commanderIds"`
}

type riftMaidenRunMutation struct {
	Run State.RiftMaidenRunState `json:"run"`
}

type riftMaidenRunCancelRequest struct {
	RunID string `json:"runId"`
}

func planRiftMaidenRunStart(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request riftMaidenRunStartRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.AttackCount < 1 || request.AttackCount > maximumRiftMaidenRunAttacks {
		return Intent.Plan{}, fmt.Errorf("attackCount must be between 1 and %d", maximumRiftMaidenRunAttacks)
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, fmt.Errorf("unitWodID must identify a probe unit in the main castle")
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, err
	}
	if current := input.State.Rift.MaidenRun; current != nil && current.Status == "running" {
		return Intent.Plan{}, fmt.Errorf(
			"Rift Maiden run %s is already active at %d of %d probes; cancel it before starting another",
			current.ID, current.AttacksLaunched, current.RequestedAttacks,
		)
	}
	source, err := sourceCastle(input.State, 0)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	units, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, exists := units.Find(fmt.Sprint(request.UnitID)); !exists {
		return Intent.Plan{}, fmt.Errorf("unit %d is not in the official catalog", request.UnitID)
	}
	if _, _, err := resolveCastleHorseTravelBoostFields(input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, fmt.Errorf("resolve Rift probe horse travel boost: %w", err)
	}
	if available := source.Units.Stationed[request.UnitID]; available < maidenProbeCountPerFlank*3 {
		return Intent.Plan{}, fmt.Errorf(
			"main castle has %d of unit %d; at least %d are required",
			available, request.UnitID, maidenProbeCountPerFlank*3,
		)
	}
	target, ok := riftTargetForKingdom(input.State, source.KingdomID)
	if !ok {
		return Intent.Plan{}, fmt.Errorf("the Rift map tile is unknown; refresh the surrounding map first")
	}
	eligible := maidenCandidateCommanders(input.State)
	if len(request.CommanderIDs) > 0 {
		allowed := make(map[State.CommanderID]struct{}, len(request.CommanderIDs))
		for _, commanderID := range request.CommanderIDs {
			if commanderID > 0 {
				allowed[commanderID] = struct{}{}
			}
		}
		eligible = slicesMatchingCommanders(eligible, allowed)
	}
	if len(eligible) == 0 {
		return Intent.Plan{}, fmt.Errorf("no assigned commander has a shield-maiden relic in the supported effect range")
	}
	sort.Slice(eligible, func(left, right int) bool { return eligible[left] < eligible[right] })
	now := time.Now().UTC()
	run := State.RiftMaidenRunState{
		ID: fmt.Sprintf("rift-maiden-%d", now.UnixNano()), Status: "running",
		RequestedAttacks: request.AttackCount, UnitID: request.UnitID,
		HorseTravelBoostID: request.HorseTravelBoostID,
		CommanderIDs:       append([]State.CommanderID(nil), eligible...),
		LaunchIDs:          []State.MovementID{},
		SourceCastleID:     source.ID, SourceX: source.X, SourceY: source.Y,
		KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
		StartedAt: now, UpdatedAt: now,
	}
	actionArguments, _ := json.Marshal(riftMaidenRunMutation{Run: run})
	return Intent.Plan{
		Claims:  []string{"rift-launch:maiden-wave"},
		Summary: fmt.Sprintf("Start a %d-probe Rift Maiden run", request.AttackCount),
		Steps: []Intent.Step{{
			Name: "Start Rift Maiden probe run", Action: "rift.maiden_run.start", ActionArguments: actionArguments,
		}},
	}, nil
}

func planRiftMaidenRunCancel(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request riftMaidenRunCancelRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.RunID = strings.TrimSpace(request.RunID)
	current := input.State.Rift.MaidenRun
	if current == nil || current.Status != "running" {
		return Intent.Plan{}, fmt.Errorf("no Rift Maiden probe run is active")
	}
	if request.RunID == "" {
		request.RunID = current.ID
	}
	if request.RunID != current.ID {
		return Intent.Plan{}, fmt.Errorf("Rift Maiden run %s is no longer active", request.RunID)
	}
	actionArguments, _ := json.Marshal(request)
	return Intent.Plan{
		Claims:  []string{"rift-launch:maiden-wave"},
		Summary: fmt.Sprintf("Cancel Rift Maiden run at %d of %d probes", current.AttacksLaunched, current.RequestedAttacks),
		Steps: []Intent.Step{{
			Name: "Cancel Rift Maiden probe run", Action: "rift.maiden_run.cancel", ActionArguments: actionArguments,
		}},
	}, nil
}

func (application *Application) startRiftMaidenRun(_ context.Context, arguments json.RawMessage) error {
	var request riftMaidenRunMutation
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if strings.TrimSpace(request.Run.ID) == "" || request.Run.RequestedAttacks < 1 ||
		request.Run.RequestedAttacks > maximumRiftMaidenRunAttacks || request.Run.UnitID <= 0 ||
		request.Run.SourceCastleID <= 0 || len(request.Run.CommanderIDs) == 0 {
		return fmt.Errorf("Rift Maiden run state is invalid")
	}
	_, err := application.State.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		if current := gameState.Rift.MaidenRun; current != nil && current.Status == "running" {
			if current.ID == request.Run.ID {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("%w: another Rift Maiden run is already active", Intent.ErrPlanStale)
		}
		run := request.Run
		run.CommanderIDs = append([]State.CommanderID(nil), request.Run.CommanderIDs...)
		run.LaunchIDs = append([]State.MovementID(nil), request.Run.LaunchIDs...)
		gameState.Rift.MaidenRun = &run
		return []string{"rift"}, true, nil
	})
	return err
}

func (application *Application) cancelRiftMaidenRun(_ context.Context, arguments json.RawMessage) error {
	var request riftMaidenRunCancelRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	request.RunID = strings.TrimSpace(request.RunID)
	_, err := application.State.ApplyWithoutMapMutation(func(gameState *State.GameState) ([]string, bool, error) {
		run := gameState.Rift.MaidenRun
		if run == nil || run.Status != "running" || run.ID != request.RunID {
			return nil, false, fmt.Errorf("%w: Rift Maiden run %s is no longer active", Intent.ErrPlanStale, request.RunID)
		}
		run.Status = "cancelled"
		run.UpdatedAt = time.Now().UTC()
		return []string{"rift"}, true, nil
	})
	return err
}

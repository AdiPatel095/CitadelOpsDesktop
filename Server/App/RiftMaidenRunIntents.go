package App

import (
	"CitadelDesktop/Server/Localization"
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("attackCount must be between 1 and %d", maximumRiftMaidenRunAttacks), Localization.New("server.app.attackcount_must_be_between.8c81b709", "attackCount must be between 1 and {p0}", Localization.Params{"p0": fmt.Sprintf("%d", maximumRiftMaidenRunAttacks)}))
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unitWodID must identify a probe unit in the main castle"), Localization.New("server.app.unitwodid_must_identify_a.a810416b", "unitWodID must identify a probe unit in the main castle", nil))
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, err
	}
	if current := input.State.Rift.MaidenRun; current != nil && current.Status == "running" {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"Rift Maiden run %s is already active at %d of %d probes; cancel it before starting another",
			current.ID, current.AttacksLaunched, current.RequestedAttacks,
		), Localization.New("server.app.rift_maiden_run_p.c759c881", "Rift Maiden run {p0} is already active at {p1} of {p2} probes; cancel it before starting another", Localization.Params{"p0": fmt.Sprintf("%s", current.ID), "p1": current.AttacksLaunched, "p2": current.RequestedAttacks}))
	}
	source, err := sourceCastle(input.State, 0)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	units, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, exists := units.Find(fmt.Sprint(request.UnitID)); !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unit %d is not in the official catalog", request.UnitID), Localization.New("server.app.unit_p_is_not.3618fe57", "unit {p0} is not in the official catalog", Localization.Params{"p0": fmt.Sprintf("%d", request.UnitID)}))
	}
	if _, _, err := resolveCastleHorseTravelBoostFields(input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("resolve Rift probe horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_rift_probe_horse.e8670ea8", "resolve Rift probe horse travel boost", nil), err))
	}
	if available := source.Units.Stationed[request.UnitID]; available < maidenProbeCountPerFlank*3 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"main castle has %d of unit %d; at least %d are required",
			available, request.UnitID, maidenProbeCountPerFlank*3,
		), Localization.New("server.app.main_castle_has_p.2a5228f9", "main castle has {p0} of unit {p1}; at least {p2} are required", Localization.Params{"p0": available, "p1": fmt.Sprintf("%d", request.UnitID), "p2": fmt.Sprintf("%d", maidenProbeCountPerFlank*3)}))
	}
	target, ok := riftTargetForKingdom(input.State, source.KingdomID)
	if !ok {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the Rift map tile is unknown; refresh the surrounding map first"), Localization.New("server.app.the_rift_map_tile.5b5a905b", "the Rift map tile is unknown; refresh the surrounding map first", nil))
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("no assigned commander has a shield-maiden relic in the supported effect range"), Localization.New("server.app.no_assigned_commander_has.a76e7d84", "no assigned commander has a shield-maiden relic in the supported effect range", nil))
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
		Summary: fmt.Sprintf("Start a %d-probe Rift Maiden run", request.AttackCount), SummaryDescriptor: Localization.New("server.app.start_a_p_probe.4c883f24", "Start a {p0}-probe Rift Maiden run", Localization.Params{"p0": request.AttackCount}),
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("no Rift Maiden probe run is active"), Localization.New("server.app.no_rift_maiden_probe.59688836", "no Rift Maiden probe run is active", nil))
	}
	if request.RunID == "" {
		request.RunID = current.ID
	}
	if request.RunID != current.ID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift Maiden run %s is no longer active", request.RunID), Localization.New("server.app.rift_maiden_run_p.ece8e8ac", "Rift Maiden run {p0} is no longer active", Localization.Params{"p0": fmt.Sprintf("%s", request.RunID)}))
	}
	actionArguments, _ := json.Marshal(request)
	return Intent.Plan{
		Claims:  []string{"rift-launch:maiden-wave"},
		Summary: fmt.Sprintf("Cancel Rift Maiden run at %d of %d probes", current.AttacksLaunched, current.RequestedAttacks), SummaryDescriptor: Localization.New("server.app.cancel_rift_maiden_run.232a1af5", "Cancel Rift Maiden run at {p0} of {p1} probes", Localization.Params{"p0": current.AttacksLaunched, "p1": current.RequestedAttacks}),
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
		return Localization.WithError(fmt.Errorf("Rift Maiden run state is invalid"), Localization.New("server.app.rift_maiden_run_state.48dddd8f", "Rift Maiden run state is invalid", nil))
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentRift), func(gameState *State.GameState) ([]string, bool, error) {
		if current := gameState.Rift.MaidenRun; current != nil && current.Status == "running" {
			if current.ID == request.Run.ID {
				return nil, false, nil
			}
			return nil, false, Localization.WithError(fmt.Errorf("%w: another Rift Maiden run is already active", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4fa42490", "intent plan became stale before dispatch: another Rift Maiden run is already active", nil))
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
	_, err := application.State.ApplyComponents(State.Components(State.ComponentRift), func(gameState *State.GameState) ([]string, bool, error) {
		run := gameState.Rift.MaidenRun
		if run == nil || run.Status != "running" || run.ID != request.RunID {
			return nil, false, Localization.WithError(fmt.Errorf("%w: Rift Maiden run %s is no longer active", Intent.ErrPlanStale, request.RunID), Localization.New("server.app.intent_plan_became_stale.fd051f40", "intent plan became stale before dispatch: Rift Maiden run {p1} is no longer active", Localization.Params{"p1": fmt.Sprintf("%s", request.RunID)}))
		}
		run.Status = "cancelled"
		run.UpdatedAt = time.Now().UTC()
		return []string{"rift"}, true, nil
	})
	return err
}

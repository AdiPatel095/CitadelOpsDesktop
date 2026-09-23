package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const dungeonMinuteSkipDispatchGuard = "nomad.cooldown.minute_skip.dispatch_guard"

type dungeonMinuteSkipRequest struct {
	KingdomID        State.KingdomID       `json:"kingdomId"`
	TargetTypeID     int                   `json:"targetTypeId"`
	TargetX          int                   `json:"targetX"`
	TargetY          int                   `json:"targetY"`
	EventCampID      int64                 `json:"eventCampId,omitempty"`
	MinimumRemaining map[string]int64      `json:"minimumRemaining"`
	KhanReportIDs    []int64               `json:"khanReportIds,omitempty"`
	KhanGuard        *khanLaneGuardRequest `json:"khanGuard,omitempty"`
}

type dungeonMinuteSkipVerification struct {
	dungeonMinuteSkipRequest
	StartedAt        time.Time `json:"startedAt"`
	InitialRemaining int       `json:"initialRemaining"`
	MSDWireKey       string    `json:"msdWireKey,omitempty"`
	MSDMinutes       int       `json:"msdMinutes,omitempty"`
}

type khanCooldownReportResolveRequest struct {
	KingdomID  State.KingdomID      `json:"kingdomId"`
	TargetX    int                  `json:"targetX"`
	TargetY    int                  `json:"targetY"`
	ReportIDs  []int64              `json:"reportIds"`
	CooldownAt time.Time            `json:"cooldownObservedAt"`
	KhanGuard  khanLaneGuardRequest `json:"khanGuard"`
}

func planDungeonMinuteSkip(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, observation, remaining, option, err := validatedDungeonMinuteSkip(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	verification, _ := json.Marshal(dungeonMinuteSkipVerification{
		dungeonMinuteSkipRequest: request, StartedAt: time.Now().UTC(), InitialRemaining: remaining,
		MSDWireKey: option.WireKey, MSDMinutes: option.Minutes,
	})
	claims := []string{
		dungeonMinuteSkipClaim(observation), "account-resources",
		"currency:" + strconv.FormatInt(int64(option.CurrencyID), 10),
	}
	if request.KhanGuard != nil {
		claims = append(claims, "khan-lane:cooldown")
	}
	return Intent.Plan{
		Claims: claims,
		Summary: fmt.Sprintf(
			"Apply a %d-minute time skip to %d-second dungeon cooldown at %d:%d",
			option.Minutes, remaining, request.TargetX, request.TargetY,
		), SummaryDescriptor: Localization.New("server.app.apply_a_p_minute.5aa8f1c2", "Apply a {p0}-minute time skip to {p1}-second dungeon cooldown at {p2}:{p3}", Localization.Params{"p0": option.Minutes, "p1": remaining, "p2": request.TargetX, "p3": request.TargetY}),
		Steps: []Intent.Step{
			{
				Name: "Build authoritative dungeon time skip", NameDescriptor: Localization.New("server.app.build_authoritative_dungeon_time.a4ee1fe2", "Build authoritative dungeon time skip", nil), Resolver: "nomad.cooldown.minute_skip.build",
				ResolverArguments: verification, AwaitOpcode: "msd", TimeoutMillis: 10_000, SuccessCodes: []int{0},
			},
			timeSkipConsumeStep(input, option.CurrencyID),
			{Name: "Verify dungeon cooldown advanced", NameDescriptor: Localization.New("server.app.verify_dungeon_cooldown_advanced.c58fabfe", "Verify dungeon cooldown advanced", nil), Action: "nomad.cooldown.minute_skip.verify", ActionArguments: verification},
		},
	}, nil
}

func resolveDungeonMinuteSkipStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var verification dungeonMinuteSkipVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return Intent.Step{}, err
	}
	requestArguments, _ := json.Marshal(verification.dungeonMinuteSkipRequest)
	request, _, _, option, err := validatedDungeonMinuteSkip(input, requestArguments, time.Now().UTC())
	if err != nil {
		if errors.Is(err, Intent.ErrPlanStale) {
			return Intent.Step{}, err
		}
		return Intent.Step{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	if verification.MSDWireKey != "" && verification.MSDMinutes > 0 {
		option, err = exactAvailableDungeonTimeSkip(
			input.State, input.GameData, verification.MSDWireKey, verification.MSDMinutes, request.MinimumRemaining,
		)
		if err != nil {
			return Intent.Step{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
		}
	}
	payload, _ := json.Marshal(struct {
		MinuteSkip string `json:"MST"`
		KingdomID  string `json:"KID"`
		X          int    `json:"X"`
		Y          int    `json:"Y"`
		MapID      int    `json:"MID"`
		NodeID     int    `json:"NID"`
	}{
		MinuteSkip: option.WireKey, KingdomID: strconv.FormatInt(int64(request.KingdomID), 10),
		X: request.TargetX, Y: request.TargetY, MapID: -1, NodeID: -1,
	})
	step := commandStep(fmt.Sprintf("Apply %s to dungeon cooldown", option.WireKey), "msd", payload, "msd", Localization.New("server.app.apply_p_to_dungeon.588f99be", "Apply {p0} to dungeon cooldown", Localization.Params{"p0": fmt.Sprintf("%s", option.WireKey)}))
	step.PreDispatchAction = dungeonMinuteSkipDispatchGuard
	step.PreDispatchArguments = append(json.RawMessage(nil), arguments...)
	step.FinalDispatchAction = dungeonMinuteSkipDispatchGuard
	step.FinalDispatchArguments = append(json.RawMessage(nil), arguments...)
	return step, nil
}

func (application *Application) guardDungeonMinuteSkipDispatch(
	ctx context.Context,
	arguments json.RawMessage,
) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("game state or official game data is unavailable"), Localization.New("server.app.game_state_or_official.39f551f4", "game state or official game data is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, err := resolveDungeonMinuteSkipStep(ctx, Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, arguments)
	return err
}

func (application *Application) verifyDungeonMinuteSkip(_ context.Context, arguments json.RawMessage) error {
	var verification dungeonMinuteSkipVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	observation, exists := state.LookupMapObservation(verification.KingdomID, fmt.Sprintf("%d:%d", verification.TargetX, verification.TargetY))
	if !exists || observation.TypeID != verification.TargetTypeID ||
		verification.EventCampID > 0 && observation.EventCampID != verification.EventCampID ||
		observation.ObservedAt.Before(verification.StartedAt) {
		return Localization.WithError(fmt.Errorf("time skip did not return a fresh row for dungeon %d:%d", verification.TargetX, verification.TargetY), Localization.New("server.app.time_skip_did_not.d76c7200", "time skip did not return a fresh row for dungeon {p0}:{p1}", Localization.Params{"p0": verification.TargetX, "p1": verification.TargetY}))
	}
	remaining := appDungeonCooldownRemaining(state, observation, time.Now().UTC())
	if remaining >= verification.InitialRemaining {
		return Localization.WithError(fmt.Errorf(
			"dungeon %d:%d cooldown did not advance: %d seconds remain from %d",
			verification.TargetX, verification.TargetY, remaining, verification.InitialRemaining,
		), Localization.New("server.app.dungeon_p_p_cooldown.87d55c5d", "dungeon {p0}:{p1} cooldown did not advance: {p2} seconds remain from {p3}", Localization.Params{"p0": verification.TargetX, "p1": verification.TargetY, "p2": remaining, "p3": verification.InitialRemaining}))
	}
	if len(verification.KhanReportIDs) == 0 {
		return nil
	}
	return application.completeKhanCooldownReports(verification, observation, remaining)
}

func validatedDungeonMinuteSkip(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (dungeonMinuteSkipRequest, State.MapObservation, int, buildingTimeSkipOption, error) {
	var request dungeonMinuteSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
	}
	if request.TargetTypeID != kingdomTowerMapTypeID && request.TargetTypeID != nomadIntentCampTypeID &&
		request.TargetTypeID != samuraiIntentCampTypeID && request.TargetTypeID != khanCampTypeID {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
			"dungeon time skips support tower, Nomad, Samurai, and Khan targets only",
		), Localization.New("server.app.dungeon_time_skips_support.7f4699ab", "dungeon time skips support tower, Nomad, Samurai, and Khan targets only", nil))
	}
	observation, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || observation.TypeID != request.TargetTypeID ||
		request.EventCampID > 0 && observation.EventCampID != request.EventCampID {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
			"%w: dungeon %d:%d does not match the current map row", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.10613db7", "intent plan became stale before dispatch: dungeon {p1}:{p2} does not match the current map row", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if err := validateDungeonCooldownFreshness(input.State, request, observation); err != nil {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
	}
	if request.KhanGuard != nil {
		if request.TargetTypeID != khanCampTypeID {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
				"guarded Khan cooldown skips require a type-35 target",
			), Localization.New("server.app.guarded_khan_cooldown_skips.a27be025", "guarded Khan cooldown skips require a type-35 target", nil))
		}
		if err := validateKhanLaneGuard(input.State, input.GameData, *request.KhanGuard, now); err != nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
		}
	}
	if len(request.KhanReportIDs) > 0 {
		if request.TargetTypeID != khanCampTypeID || request.KhanGuard == nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
				"report-linked Khan cooldown skips require a type-35 target and safety guard",
			), Localization.New("server.app.report_linked_khan_cooldown.73f13418", "report-linked Khan cooldown skips require a type-35 target and safety guard", nil))
		}
		if err := validateKhanCooldownReports(input.State, request, observation); err != nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
		}
	}
	remaining := appDungeonCooldownRemaining(input.State, observation, now)
	if remaining <= 0 {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
			"%w: dungeon %d:%d is no longer on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.868d55e4", "intent plan became stale before dispatch: dungeon {p1}:{p2} is no longer on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	option, err := fastestAvailableDungeonTimeSkip(input.State, input.GameData, remaining, request.MinimumRemaining)
	if err != nil {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
	}
	return request, observation, remaining, option, nil
}

func validateDungeonCooldownFreshness(
	gameState State.GameState,
	request dungeonMinuteSkipRequest,
	observation State.MapObservation,
) error {
	key := fmt.Sprintf("%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	if request.TargetTypeID == kingdomTowerMapTypeID {
		if cooldown, found := gameState.LookupTowerCooldown(key); found &&
			(cooldown.TargetTypeID > 0 && cooldown.TargetTypeID != request.TargetTypeID ||
				cooldown.PendingCooldownRefresh || cooldown.LastSuccessfulBattleAt.After(observation.ObservedAt)) {
			return Localization.WithError(fmt.Errorf(
				"%w: tower %d:%d is awaiting a fresh post-victory cooldown row",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.b56377b9", "intent plan became stale before dispatch: tower {p1}:{p2} is awaiting a fresh post-victory cooldown row", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		return nil
	}
	if request.TargetTypeID == nomadIntentCampTypeID || request.TargetTypeID == samuraiIntentCampTypeID ||
		request.TargetTypeID == khanCampTypeID {
		if cooldown, found := gameState.NomadCamps.Cooldowns[key]; found &&
			(cooldown.PendingCooldownRefresh || cooldown.LastSuccessfulBattleAt.After(observation.ObservedAt)) {
			return Localization.WithError(fmt.Errorf(
				"%w: camp %d:%d is awaiting a fresh post-victory cooldown row",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.42dffc1b", "intent plan became stale before dispatch: camp {p1}:{p2} is awaiting a fresh post-victory cooldown row", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
	}
	return nil
}

func validateKhanCooldownReports(
	gameState State.GameState,
	request dungeonMinuteSkipRequest,
	observation State.MapObservation,
) error {
	seen := make(map[int64]struct{}, len(request.KhanReportIDs))
	for _, reportID := range request.KhanReportIDs {
		if reportID <= 0 {
			return Localization.WithError(fmt.Errorf("Khan cooldown report id must be positive"), Localization.New("server.app.khan_cooldown_report_id.402c9f25", "Khan cooldown report id must be positive", nil))
		}
		if _, duplicate := seen[reportID]; duplicate {
			return Localization.WithError(fmt.Errorf("Khan cooldown report %d was included more than once", reportID), Localization.New("server.app.khan_cooldown_report_p.6d16e752", "Khan cooldown report {p0} was included more than once", Localization.Params{"p0": fmt.Sprintf("%d", reportID)}))
		}
		seen[reportID] = struct{}{}
		report, found := gameState.Khan.CooldownReports[reportID]
		if !found || !report.ResolvedAt.IsZero() || report.KingdomID != request.KingdomID ||
			report.X != request.TargetX || report.Y != request.TargetY {
			return Localization.WithError(fmt.Errorf("%w: Khan cooldown report %d is no longer pending for this target", Intent.ErrPlanStale, reportID), Localization.New("server.app.intent_plan_became_stale.d32fb5ca", "intent plan became stale before dispatch: Khan cooldown report {p1} is no longer pending for this target", Localization.Params{"p1": fmt.Sprintf("%d", reportID)}))
		}
		if report.CooldownObservedAt.IsZero() || report.CooldownObservedAt.After(observation.ObservedAt) ||
			report.LandedAt.After(report.CooldownObservedAt) {
			return Localization.WithError(fmt.Errorf("%w: Khan cooldown report %d does not have a fresh target re-ping", Intent.ErrPlanStale, reportID), Localization.New("server.app.intent_plan_became_stale.232f3c03", "intent plan became stale before dispatch: Khan cooldown report {p1} does not have a fresh target re-ping", Localization.Params{"p1": fmt.Sprintf("%d", reportID)}))
		}
	}
	for reportID, report := range gameState.Khan.CooldownReports {
		if !report.ResolvedAt.IsZero() || report.KingdomID != request.KingdomID ||
			report.X != request.TargetX || report.Y != request.TargetY {
			continue
		}
		if report.LandedAt.After(observation.ObservedAt) ||
			report.CooldownObservedAt.IsZero() ||
			report.CooldownObservedAt.Before(report.LandedAt) {
			return Localization.WithError(fmt.Errorf(
				"%w: Khan cooldown report %d requires a newer target re-ping",
				Intent.ErrPlanStale, reportID,
			), Localization.New("server.app.intent_plan_became_stale.f7de5e4e", "intent plan became stale before dispatch: Khan cooldown report {p1} requires a newer target re-ping", Localization.Params{"p1": fmt.Sprintf("%d", reportID)}))
		}
		if _, included := seen[reportID]; !included {
			return Localization.WithError(fmt.Errorf(
				"%w: Khan cooldown report %d is missing from this MSD",
				Intent.ErrPlanStale, reportID,
			), Localization.New("server.app.intent_plan_became_stale.d0906560", "intent plan became stale before dispatch: Khan cooldown report {p1} is missing from this MSD", Localization.Params{"p1": fmt.Sprintf("%d", reportID)}))
		}
	}
	return nil
}

func (application *Application) completeKhanCooldownReports(
	verification dungeonMinuteSkipVerification,
	observation State.MapObservation,
	remainingAfter int,
) error {
	appliedAt := observation.ObservedAt.UTC()
	if appliedAt.IsZero() {
		appliedAt = time.Now().UTC()
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		changed := false
		for _, reportID := range verification.KhanReportIDs {
			report, found := gameState.Khan.CooldownReports[reportID]
			if !found || !report.ResolvedAt.IsZero() {
				continue
			}
			if report.KingdomID != verification.KingdomID ||
				report.X != verification.TargetX || report.Y != verification.TargetY {
				return nil, false, Localization.WithError(fmt.Errorf("Khan cooldown report %d changed targets", reportID), Localization.New("server.app.khan_cooldown_report_p.a6562ce2", "Khan cooldown report {p0} changed targets", Localization.Params{"p0": fmt.Sprintf("%d", reportID)}))
			}
			alreadyAttached := false
			for _, applied := range report.MSDs {
				if applied.AppliedAt.Equal(appliedAt) && applied.WireKey == verification.MSDWireKey {
					alreadyAttached = true
					break
				}
			}
			if alreadyAttached {
				continue
			}
			report.MSDs = append(report.MSDs, State.KhanCooldownMSDState{
				WireKey: verification.MSDWireKey, Minutes: verification.MSDMinutes,
				CooldownBefore: verification.InitialRemaining, CooldownAfter: remainingAfter,
				AppliedAt: appliedAt,
			})
			report.CooldownRemaining = remainingAfter
			report.CooldownObservedAt = appliedAt
			if remainingAfter <= 0 {
				report.ResolvedAt = appliedAt
			}
			gameState.Khan.CooldownReports[reportID] = report
			changed = true
		}
		if !changed {
			return nil, false, nil
		}
		gameState.Khan.CooldownsSkipped++
		gameState.Khan.LastCooldownSkippedAt = appliedAt
		return []string{"khan", "nomad-camps"}, true, nil
	})
	return err
}

func planKhanCooldownReportResolve(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request khanCooldownReportResolveRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	observation, err := validateKhanCooldownReportResolution(input, request, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims: []string{dungeonMinuteSkipClaim(observation)},
		Summary: fmt.Sprintf(
			"Resolve %d Khan cooldown report(s) already clear at %d:%d",
			len(request.ReportIDs), request.TargetX, request.TargetY,
		), SummaryDescriptor: Localization.New("server.app.resolve_p_khan_cooldown.22baf706", "Resolve {p0} Khan cooldown report(s) already clear at {p1}:{p2}", Localization.Params{"p0": fmt.Sprintf("%d", len(request.ReportIDs)), "p1": request.TargetX, "p2": request.TargetY}),
		Steps: []Intent.Step{{
			Name: "Resolve Khan cooldown reports without another time skip", NameDescriptor: Localization.New("server.app.resolve_khan_cooldown_reports.e77b8328", "Resolve Khan cooldown reports without another time skip", nil),
			Action: "khan.cooldown.reports.resolve", ActionArguments: arguments,
		}},
	}, nil
}

func (application *Application) resolveKhanCooldownReports(
	_ context.Context,
	arguments json.RawMessage,
) error {
	var request khanCooldownReportResolveRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	input := Intent.PlanningContext{State: application.State.ReadOnlyView(), GameData: gameData}
	observation, err := validateKhanCooldownReportResolution(input, request, time.Now().UTC())
	if err != nil {
		return err
	}
	resolvedAt := observation.ObservedAt.UTC()
	_, err = application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		changed := false
		for _, reportID := range request.ReportIDs {
			report, found := gameState.Khan.CooldownReports[reportID]
			if !found || !report.ResolvedAt.IsZero() {
				continue
			}
			report.ResolvedAt = resolvedAt
			gameState.Khan.CooldownReports[reportID] = report
			changed = true
		}
		return []string{"khan"}, changed, nil
	})
	return err
}

func validateKhanCooldownReportResolution(
	input Intent.PlanningContext,
	request khanCooldownReportResolveRequest,
	now time.Time,
) (State.MapObservation, error) {
	if len(request.ReportIDs) == 0 || request.CooldownAt.IsZero() {
		return State.MapObservation{}, Localization.WithError(fmt.Errorf("Khan cooldown resolution requires reports and a fresh re-ping"), Localization.New("server.app.khan_cooldown_resolution_requires.e6a08635", "Khan cooldown resolution requires reports and a fresh re-ping", nil))
	}
	if err := validateKhanLaneGuard(input.State, input.GameData, request.KhanGuard, now); err != nil {
		return State.MapObservation{}, err
	}
	observation, found := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !found || observation.TypeID != khanCampTypeID || observation.ObservedAt.Before(request.CooldownAt) ||
		appDungeonCooldownRemaining(input.State, observation, now) > 0 {
		return State.MapObservation{}, Localization.WithError(fmt.Errorf("%w: the Khan target is not authoritatively clear", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d1e32263", "intent plan became stale before dispatch: the Khan target is not authoritatively clear", nil))
	}
	validation := dungeonMinuteSkipRequest{
		KingdomID: request.KingdomID, TargetTypeID: khanCampTypeID,
		TargetX: request.TargetX, TargetY: request.TargetY, KhanReportIDs: request.ReportIDs,
	}
	if err := validateKhanCooldownReports(input.State, validation, observation); err != nil {
		return State.MapObservation{}, err
	}
	return observation, nil
}

func fastestAvailableDungeonTimeSkip(
	gameState State.GameState,
	gameData *GameData.Store,
	remainingSec int,
	minimumRemaining map[string]int64,
) (buildingTimeSkipOption, error) {
	minutes := []int{1, 5, 10, 30, 60, 300, 1440}
	available := make([]buildingTimeSkipOption, 0, len(minutes))
	for _, minute := range minutes {
		option, err := officialBuildingTimeSkipOption(gameData, minute)
		if err != nil {
			return buildingTimeSkipOption{}, err
		}
		reserve := timeSkipReserve(minimumRemaining, option.WireKey)
		if reserve < 0 {
			return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("%s time-skip reserve cannot be negative", option.WireKey), Localization.New("server.app.p_time_skip_reserve.62d66da1", "{p0} time-skip reserve cannot be negative", Localization.Params{"p0": fmt.Sprintf("%s", option.WireKey)}))
		}
		balance := int64(math.Floor(gameState.Player.Currencies[option.CurrencyID]))
		if balance > reserve {
			available = append(available, option)
		}
	}
	for _, option := range available {
		if option.Minutes*60 >= remainingSec {
			return option, nil
		}
	}
	if len(available) > 0 {
		return available[len(available)-1], nil
	}
	return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("no dungeon time skip is available above the configured reserves"), Localization.New("server.app.no_dungeon_time_skip.892407f2", "no dungeon time skip is available above the configured reserves", nil))
}

func exactAvailableDungeonTimeSkip(
	gameState State.GameState,
	gameData *GameData.Store,
	wireKey string,
	minutes int,
	minimumRemaining map[string]int64,
) (buildingTimeSkipOption, error) {
	option, err := officialBuildingTimeSkipOption(gameData, minutes)
	if err != nil {
		return buildingTimeSkipOption{}, err
	}
	if !strings.EqualFold(option.WireKey, strings.TrimSpace(wireKey)) {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf(
			"planned dungeon time skip changed from %s to %s",
			strings.TrimSpace(wireKey), option.WireKey,
		), Localization.New("server.app.planned_dungeon_time_skip.96597b72", "planned dungeon time skip changed from {p0} to {p1}", Localization.Params{"p0": fmt.Sprintf("%s", strings.TrimSpace(wireKey)), "p1": fmt.Sprintf("%s", option.WireKey)}))
	}
	reserve := timeSkipReserve(minimumRemaining, option.WireKey)
	if reserve < 0 {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("%s time-skip reserve cannot be negative", option.WireKey), Localization.New("server.app.p_time_skip_reserve.62d66da1", "{p0} time-skip reserve cannot be negative", Localization.Params{"p0": fmt.Sprintf("%s", option.WireKey)}))
	}
	balance := int64(math.Floor(gameState.Player.Currencies[option.CurrencyID]))
	if balance <= reserve {
		return buildingTimeSkipOption{}, Localization.WithError(fmt.Errorf("%s is no longer available above its configured reserve", option.WireKey), Localization.New("server.app.p_is_no_longer.6fc096f6", "{p0} is no longer available above its configured reserve", Localization.Params{"p0": fmt.Sprintf("%s", option.WireKey)}))
	}
	return option, nil
}

func timeSkipReserve(reserves map[string]int64, wireKey string) int64 {
	for key, value := range reserves {
		if strings.EqualFold(strings.TrimSpace(key), wireKey) {
			return value
		}
	}
	return 0
}

func appDungeonCooldownRemaining(gameState State.GameState, observation State.MapObservation, now time.Time) int {
	if observation.TypeID == kingdomTowerMapTypeID {
		remaining, observedAt := observation.TowerCooldownRemaining, observation.ObservedAt
		key := fmt.Sprintf("%d:%d:%d", observation.KingdomID, observation.X, observation.Y)
		if cooldown, found := gameState.LookupTowerCooldown(key); found && cooldown.CooldownObservedAt.After(observedAt) {
			remaining, observedAt = cooldown.CooldownRemaining, cooldown.CooldownObservedAt
		}
		return elapsedCooldownRemaining(remaining, observedAt, now)
	}
	return nomadAppCooldownRemaining(gameState, observation, now)
}

func elapsedCooldownRemaining(remaining int, observedAt, now time.Time) int {
	if remaining <= 0 {
		return 0
	}
	if !observedAt.IsZero() && now.After(observedAt) {
		remaining -= int(now.Sub(observedAt) / time.Second)
	}
	return max(0, remaining)
}

func dungeonMinuteSkipClaim(observation State.MapObservation) string {
	if observation.TypeID == kingdomTowerMapTypeID {
		return towerTargetClaim(observation)
	}
	return fmt.Sprintf("nomad-target:%d:%d:%d", observation.KingdomID, observation.X, observation.Y)
}

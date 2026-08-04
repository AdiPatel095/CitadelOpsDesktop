package App

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

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
		),
		Steps: []Intent.Step{
			{
				Name: "Build authoritative dungeon time skip", Resolver: "nomad.cooldown.minute_skip.build",
				ResolverArguments: verification, AwaitOpcode: "msd", TimeoutMillis: 10_000, SuccessCodes: []int{0},
			},
			timeSkipConsumeStep(input, option.CurrencyID),
			{Name: "Verify dungeon cooldown advanced", Action: "nomad.cooldown.minute_skip.verify", ActionArguments: verification},
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
		return Intent.Step{}, err
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
	return commandStep(fmt.Sprintf("Apply %s to dungeon cooldown", option.WireKey), "msd", payload, "msd"), nil
}

func (application *Application) verifyDungeonMinuteSkip(_ context.Context, arguments json.RawMessage) error {
	var verification dungeonMinuteSkipVerification
	if err := decodeIntentArguments(arguments, &verification); err != nil {
		return err
	}
	state := application.State.Snapshot()
	observation, exists := state.Map[verification.KingdomID][fmt.Sprintf("%d:%d", verification.TargetX, verification.TargetY)]
	if !exists || observation.TypeID != verification.TargetTypeID ||
		verification.EventCampID > 0 && observation.EventCampID != verification.EventCampID ||
		observation.ObservedAt.Before(verification.StartedAt) {
		return fmt.Errorf("time skip did not return a fresh row for dungeon %d:%d", verification.TargetX, verification.TargetY)
	}
	remaining := appDungeonCooldownRemaining(state, observation, time.Now().UTC())
	if remaining >= verification.InitialRemaining {
		return fmt.Errorf(
			"dungeon %d:%d cooldown did not advance: %d seconds remain from %d",
			verification.TargetX, verification.TargetY, remaining, verification.InitialRemaining,
		)
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
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
			"dungeon time skips support tower, Nomad, Samurai, and Khan targets only",
		)
	}
	observation, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || observation.TypeID != request.TargetTypeID ||
		request.EventCampID > 0 && observation.EventCampID != request.EventCampID {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
			"dungeon %d:%d does not match the current map row", request.TargetX, request.TargetY,
		)
	}
	if request.KhanGuard != nil {
		if request.TargetTypeID != khanCampTypeID {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
				"guarded Khan cooldown skips require a type-35 target",
			)
		}
		if err := validateKhanLaneGuard(input.State, input.GameData, *request.KhanGuard, now); err != nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
		}
	}
	if len(request.KhanReportIDs) > 0 {
		if request.TargetTypeID != khanCampTypeID || request.KhanGuard == nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
				"report-linked Khan cooldown skips require a type-35 target and safety guard",
			)
		}
		if err := validateKhanCooldownReports(input.State, request, observation); err != nil {
			return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
		}
	}
	remaining := appDungeonCooldownRemaining(input.State, observation, now)
	if remaining <= 0 {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
			"dungeon %d:%d is no longer on cooldown", request.TargetX, request.TargetY,
		)
	}
	option, err := fastestAvailableDungeonTimeSkip(input.State, input.GameData, remaining, request.MinimumRemaining)
	if err != nil {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, err
	}
	return request, observation, remaining, option, nil
}

func validateKhanCooldownReports(
	gameState State.GameState,
	request dungeonMinuteSkipRequest,
	observation State.MapObservation,
) error {
	seen := make(map[int64]struct{}, len(request.KhanReportIDs))
	for _, reportID := range request.KhanReportIDs {
		if reportID <= 0 {
			return fmt.Errorf("Khan cooldown report id must be positive")
		}
		if _, duplicate := seen[reportID]; duplicate {
			return fmt.Errorf("Khan cooldown report %d was included more than once", reportID)
		}
		seen[reportID] = struct{}{}
		report, found := gameState.Khan.CooldownReports[reportID]
		if !found || !report.ResolvedAt.IsZero() || report.KingdomID != request.KingdomID ||
			report.X != request.TargetX || report.Y != request.TargetY {
			return fmt.Errorf("%w: Khan cooldown report %d is no longer pending for this target", Intent.ErrPlanStale, reportID)
		}
		if report.CooldownObservedAt.IsZero() || report.CooldownObservedAt.After(observation.ObservedAt) ||
			report.LandedAt.After(report.CooldownObservedAt) {
			return fmt.Errorf("%w: Khan cooldown report %d does not have a fresh target re-ping", Intent.ErrPlanStale, reportID)
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
			return fmt.Errorf(
				"%w: Khan cooldown report %d requires a newer target re-ping",
				Intent.ErrPlanStale, reportID,
			)
		}
		if _, included := seen[reportID]; !included {
			return fmt.Errorf(
				"%w: Khan cooldown report %d is missing from this MSD",
				Intent.ErrPlanStale, reportID,
			)
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		changed := false
		for _, reportID := range verification.KhanReportIDs {
			report, found := gameState.Khan.CooldownReports[reportID]
			if !found || !report.ResolvedAt.IsZero() {
				continue
			}
			if report.KingdomID != verification.KingdomID ||
				report.X != verification.TargetX || report.Y != verification.TargetY {
				return nil, false, fmt.Errorf("Khan cooldown report %d changed targets", reportID)
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
		),
		Steps: []Intent.Step{{
			Name:   "Resolve Khan cooldown reports without another time skip",
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
		return fmt.Errorf("official game data is unavailable")
	}
	input := Intent.PlanningContext{State: application.State.Snapshot(), GameData: gameData}
	observation, err := validateKhanCooldownReportResolution(input, request, time.Now().UTC())
	if err != nil {
		return err
	}
	resolvedAt := observation.ObservedAt.UTC()
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
		return State.MapObservation{}, fmt.Errorf("Khan cooldown resolution requires reports and a fresh re-ping")
	}
	if err := validateKhanLaneGuard(input.State, input.GameData, request.KhanGuard, now); err != nil {
		return State.MapObservation{}, err
	}
	observation, found := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !found || observation.TypeID != khanCampTypeID || observation.ObservedAt.Before(request.CooldownAt) ||
		appDungeonCooldownRemaining(input.State, observation, now) > 0 {
		return State.MapObservation{}, fmt.Errorf("%w: the Khan target is not authoritatively clear", Intent.ErrPlanStale)
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
			return buildingTimeSkipOption{}, fmt.Errorf("%s time-skip reserve cannot be negative", option.WireKey)
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
	return buildingTimeSkipOption{}, fmt.Errorf("no dungeon time skip is available above the configured reserves")
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
		return buildingTimeSkipOption{}, fmt.Errorf(
			"planned dungeon time skip changed from %s to %s",
			strings.TrimSpace(wireKey), option.WireKey,
		)
	}
	reserve := timeSkipReserve(minimumRemaining, option.WireKey)
	if reserve < 0 {
		return buildingTimeSkipOption{}, fmt.Errorf("%s time-skip reserve cannot be negative", option.WireKey)
	}
	balance := int64(math.Floor(gameState.Player.Currencies[option.CurrencyID]))
	if balance <= reserve {
		return buildingTimeSkipOption{}, fmt.Errorf("%s is no longer available above its configured reserve", option.WireKey)
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
		if cooldown, found := gameState.TowerCooldowns[key]; found && cooldown.CooldownObservedAt.After(observedAt) {
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

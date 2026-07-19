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
	KingdomID        State.KingdomID  `json:"kingdomId"`
	TargetTypeID     int              `json:"targetTypeId"`
	TargetX          int              `json:"targetX"`
	TargetY          int              `json:"targetY"`
	EventCampID      int64            `json:"eventCampId,omitempty"`
	MinimumRemaining map[string]int64 `json:"minimumRemaining"`
}

type dungeonMinuteSkipVerification struct {
	dungeonMinuteSkipRequest
	StartedAt        time.Time `json:"startedAt"`
	InitialRemaining int       `json:"initialRemaining"`
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
	})
	return Intent.Plan{
		Claims: []string{dungeonMinuteSkipClaim(observation), "account-resources"},
		Summary: fmt.Sprintf(
			"Apply a %d-minute time skip to %d-second dungeon cooldown at %d:%d",
			option.Minutes, remaining, request.TargetX, request.TargetY,
		),
		Steps: []Intent.Step{
			{
				Name: "Build authoritative dungeon time skip", Resolver: "nomad.cooldown.minute_skip.build",
				ResolverArguments: arguments, AwaitOpcode: "msd", TimeoutMillis: 10_000, SuccessCodes: []int{0},
			},
			{Name: "Verify dungeon cooldown advanced", Action: "nomad.cooldown.minute_skip.verify", ActionArguments: verification},
		},
	}, nil
}

func resolveDungeonMinuteSkipStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	request, _, _, option, err := validatedDungeonMinuteSkip(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Step{}, err
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
	return nil
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
		request.TargetTypeID != samuraiIntentCampTypeID {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
			"dungeon time skips support tower, Nomad, and Samurai targets only",
		)
	}
	observation, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || observation.TypeID != request.TargetTypeID ||
		request.EventCampID > 0 && observation.EventCampID != request.EventCampID {
		return dungeonMinuteSkipRequest{}, State.MapObservation{}, 0, buildingTimeSkipOption{}, fmt.Errorf(
			"dungeon %d:%d does not match the current map row", request.TargetX, request.TargetY,
		)
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

package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type nomadRBCTestAttackRequest struct {
	RunID              string               `json:"runId"`
	BatchID            string               `json:"batchId"`
	SourceCastleID     State.CastleID       `json:"sourceCastleId"`
	KingdomID          State.KingdomID      `json:"kingdomId"`
	TargetX            int                  `json:"targetX"`
	TargetY            int                  `json:"targetY"`
	VictoryCount       int64                `json:"victoryCount"`
	ExpectedAttacks    int                  `json:"expectedAttacks"`
	Preset             AttackPresets.Preset `json:"preset"`
	CommanderIDs       []State.CommanderID  `json:"commanderIds"`
	HorseTravelBoostID int                  `json:"horseTravelBoostId"`
	DailyAttackLimit   int64                `json:"dailyAttackLimit"`
}

type resolvedNomadRBCTestAttackRequest struct {
	nomadRBCTestAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type nomadRBCTestLaunchCapture struct {
	RunID       string            `json:"runId"`
	BatchID     string            `json:"batchId"`
	CommanderID State.CommanderID `json:"commanderId"`
}

func planNomadRBCTestAttack(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, source, target, err := nomadRBCTestAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	if current := input.State.NomadCamps.RBCTest; current != nil && current.RunID == request.RunID && current.SafetyError != "" {
		return Intent.Plan{}, fmt.Errorf("RBC trial %s is blocked: %s", request.RunID, current.SafetyError)
	}
	resolution, err := resolveCRACommanders(
		input.State,
		&craCommanderSelectionRequest{Candidates: request.CommanderIDs, Count: request.ExpectedAttacks, Strategy: "lowest_id"},
		craCommanderSelectionOptions{DefaultCount: request.ExpectedAttacks, RequireAvailable: true},
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	if len(resolution.Selected) != request.ExpectedAttacks {
		return Intent.Plan{}, fmt.Errorf("RBC trial requires exactly %d available commanders", request.ExpectedAttacks)
	}
	request.CommanderIDs = orderNomadChainCommanders(input, source, target, resolution.Selected)
	if _, err := buildAttackSetupForCommanders(invasionAttackSetup(request.Preset), source, input.GameData, len(request.CommanderIDs)); err != nil {
		return Intent.Plan{}, fmt.Errorf("validate RBC trial preset inventory: %w", err)
	}

	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	normalized, _ := json.Marshal(request)
	steps := make([]Intent.Step, 0, len(request.CommanderIDs)*4+8)
	for _, commanderID := range request.CommanderIDs {
		steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	}
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps,
		Intent.Step{Name: "Initialize or continue opportunistic RBC trial", Action: "nomad.rbc_test.begin", ActionArguments: normalized},
		Intent.Step{Name: "Verify RBC trial preset inventory", Action: "nomad.rbc_test.inventory.guard", ActionArguments: normalized},
	)
	for _, commanderID := range request.CommanderIDs {
		resolvedArguments, _ := json.Marshal(resolvedNomadRBCTestAttackRequest{
			nomadRBCTestAttackRequest: request, CommanderID: commanderID,
		})
		capture, _ := json.Marshal(nomadRBCTestLaunchCapture{
			RunID: request.RunID, BatchID: request.BatchID, CommanderID: commanderID,
		})
		steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
		steps = append(steps,
			deferredCRACommandStep(
				fmt.Sprintf("Build and launch RBC trial attack with commander %d", commanderID),
				"nomad.rbc_test.attack.build", resolvedArguments, contextPayload,
			),
			Intent.Step{Name: "Capture authoritative RBC trial arrival", Action: "nomad.rbc_test.launch.capture", ActionArguments: capture},
		)
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		towerTargetClaim(target),
	}
	claims = append(claims, craCommanderClaims(request.CommanderIDs)...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoNomad", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Launch a %d-hit Auto Camp trial into RBC %d:%d", len(request.CommanderIDs), target.X, target.Y),
		Steps:   steps,
	}, nil
}

func nomadRBCTestAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (nomadRBCTestAttackRequest, State.CastleState, State.MapObservation, error) {
	var request nomadRBCTestAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	request.RunID = strings.TrimSpace(request.RunID)
	request.BatchID = strings.TrimSpace(request.BatchID)
	if request.RunID == "" || request.BatchID == "" || request.SourceCastleID <= 0 || request.ExpectedAttacks < 1 ||
		len(request.CommanderIDs) < request.ExpectedAttacks {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"RBC trial requires run and batch ids, a source castle, and at least one commander",
		)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != request.KingdomID {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("RBC trial source castle is unavailable")
	}
	target, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || target.TypeID != kingdomTowerMapTypeID || target.TowerVictoryCount != request.VictoryCount {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"RBC trial target %d:%d changed or is not a kingdom tower", request.TargetX, request.TargetY,
		)
	}
	key := fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)
	if cooldown, found := input.State.TowerCooldowns[key]; found && cooldown.PendingCooldownRefresh {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf(
			"RBC trial target %d:%d is awaiting an authoritative cooldown refresh", target.X, target.Y,
		)
	}
	if appDungeonCooldownRemaining(input.State, target, time.Now().UTC()) > 0 {
		return nomadRBCTestAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("RBC trial target %d:%d is on cooldown", target.X, target.Y)
	}
	return request, source, target, nil
}

func orderNomadChainCommanders(
	input Intent.PlanningContext,
	source State.CastleState,
	target State.MapObservation,
	commanderIDs []State.CommanderID,
) []State.CommanderID {
	type rankedCommander struct {
		id    State.CommanderID
		speed float64
	}
	ranked := make([]rankedCommander, 0, len(commanderIDs))
	for _, commanderID := range commanderIDs {
		victoryCount := target.TowerVictoryCount
		if target.TypeID == nomadIntentCampTypeID || target.TypeID == samuraiIntentCampTypeID {
			victoryCount = target.EventCampVictoryCount
		}
		result, err := (AttackCapacity.Resolver{}).ResolveTravelSpeed(input.State, input.GameData, AttackCapacity.Request{
			SourceCastleID: source.ID, CommanderID: commanderID,
			Target: AttackCapacity.TargetContext{
				Map: &AttackCapacity.MapTarget{
					KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
					ObjectID: target.ObjectID, Level: target.Level, VictoryCount: victoryCount,
				},
				Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
			},
		})
		speed := float64(0)
		if err == nil {
			speed = result.AppliedPercent
		}
		ranked = append(ranked, rankedCommander{id: commanderID, speed: speed})
	}
	sort.SliceStable(ranked, func(left, right int) bool {
		if ranked[left].speed != ranked[right].speed {
			return ranked[left].speed > ranked[right].speed
		}
		return ranked[left].id < ranked[right].id
	})
	result := make([]State.CommanderID, len(ranked))
	for index := range ranked {
		result[index] = ranked[index].id
	}
	return result
}

func (application *Application) resolveNomadRBCTestAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request resolvedNomadRBCTestAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	_, source, target, err := nomadRBCTestAttackContext(input, mustMarshalNomadRBCTestAttackRequest(request.nomadRBCTestAttackRequest))
	if err != nil {
		return Intent.Step{}, err
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: target.Level, VictoryCount: target.TowerVictoryCount,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("resolve RBC trial attack capacity: %w", err)
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(request.Preset, capacity))
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build RBC trial preset %q: %w", request.Preset.Name, err)
	}
	body, err := json.Marshal(invasionAttackBody(source, target, request.CommanderID, built, request.HorseTravelBoostID))
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build RBC trial CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack RBC trial target at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func (application *Application) beginNomadRBCTest(_ context.Context, arguments json.RawMessage) error {
	var request nomadRBCTestAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		if current := gameState.NomadCamps.RBCTest; current != nil && current.RunID == request.RunID {
			return nil, false, nil
		}
		gameState.NomadCamps.RBCTest = &State.NomadRBCTestState{
			RunID: request.RunID, SourceCastleID: request.SourceCastleID, KingdomID: request.KingdomID,
			TargetX: request.TargetX, TargetY: request.TargetY,
			Launches: []State.NomadRBCTestLaunch{}, StartedAt: time.Now().UTC(),
		}
		return []string{"nomad-camps"}, true, nil
	})
	return err
}

func (application *Application) guardNomadRBCTestInventory(_ context.Context, arguments json.RawMessage) error {
	var request nomadRBCTestAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	source, exists := state.Castles[request.SourceCastleID]
	if !exists {
		return fmt.Errorf("RBC trial source castle %d is unavailable", request.SourceCastleID)
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, err := buildAttackSetupForCommanders(invasionAttackSetup(request.Preset), source, gameData, request.ExpectedAttacks)
	return err
}

func (application *Application) guardNomadRBCTestAttack(_ context.Context, arguments json.RawMessage) error {
	var request resolvedNomadRBCTestAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, source, target, err := nomadRBCTestAttackContext(
		Intent.PlanningContext{State: state, GameData: gameData}, mustMarshalNomadRBCTestAttackRequest(request.nomadRBCTestAttackRequest),
	)
	if err != nil {
		return err
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != kingdomTowerMapTypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y ||
		dialog.Target.TowerVictoryCount != target.TowerVictoryCount || dialog.Target.TowerCooldownRemaining > 0 {
		return fmt.Errorf("authoritative ADI row no longer matches ready RBC trial target %d:%d", target.X, target.Y)
	}
	return nil
}

func (application *Application) captureNomadRBCTestLaunch(_ context.Context, arguments json.RawMessage) error {
	var request nomadRBCTestLaunchCapture
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	var safetyError string
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		test := gameState.NomadCamps.RBCTest
		if test == nil || test.RunID != request.RunID {
			return nil, false, fmt.Errorf("RBC trial %s is not active", request.RunID)
		}
		for _, launch := range test.Launches {
			if launch.BatchID == request.BatchID && launch.CommanderID == request.CommanderID {
				return nil, false, nil
			}
		}
		var selected State.MovementState
		found := false
		for _, movement := range gameState.Movements {
			if movement.Direction != 0 || movement.SourceCastleID != test.SourceCastleID || movement.KingdomID != test.KingdomID ||
				movement.TargetX != test.TargetX || movement.TargetY != test.TargetY || movement.CommanderID == nil ||
				*movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
				continue
			}
			if !found || movement.ArrivesAt.After(*selected.ArrivesAt) {
				selected, found = movement, true
			}
		}
		if !found {
			return nil, false, fmt.Errorf("CRA response did not return commander %d's RBC trial movement", request.CommanderID)
		}
		launch := State.NomadRBCTestLaunch{
			BatchID: request.BatchID, CommanderID: request.CommanderID,
			MovementID: selected.ID, ArrivesAt: selected.ArrivesAt.UTC(),
		}
		var previous *State.NomadRBCTestLaunch
		for index := len(test.Launches) - 1; index >= 0; index-- {
			if test.Launches[index].BatchID == request.BatchID {
				candidate := test.Launches[index]
				previous = &candidate
				break
			}
		}
		if previous != nil {
			if launch.ArrivesAt.Before(previous.ArrivesAt) {
				safetyError = fmt.Sprintf(
					"commander %d arrives at %s before commander %d at %s",
					launch.CommanderID, launch.ArrivesAt.Format(time.RFC3339Nano),
					previous.CommanderID, previous.ArrivesAt.Format(time.RFC3339Nano),
				)
				test.SafetyError = safetyError
			}
		}
		test.Launches = append(test.Launches, launch)
		if len(test.Launches) > 256 {
			test.Launches = append([]State.NomadRBCTestLaunch(nil), test.Launches[len(test.Launches)-256:]...)
		}
		test.AttacksLaunched++
		test.ExpectedAttacks = test.AttacksLaunched
		test.LastChainLaunchedAt = time.Now().UTC()
		return []string{"nomad-camps", "movements"}, true, nil
	})
	if err != nil {
		return err
	}
	if safetyError != "" {
		return fmt.Errorf("unsafe RBC trial arrival order: %s", safetyError)
	}
	return nil
}

func mustMarshalNomadRBCTestAttackRequest(request nomadRBCTestAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

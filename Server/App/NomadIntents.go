package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	nomadIntentEventID      = 72
	nomadIntentCampTypeID   = 27
	samuraiIntentEventID    = 80
	samuraiIntentCampTypeID = 29
	nomadIntentCampCount    = 4
	nomadIntentRadius       = 50
)

type nomadMapScanRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
	Radius         int            `json:"radius"`
	ScanStartedAt  time.Time      `json:"scanStartedAt"`
}

type nomadTargetRequest struct {
	SourceCastleID State.CastleID  `json:"sourceCastleId"`
	EventID        int64           `json:"eventId"`
	DifficultyID   int64           `json:"difficultyId"`
	KingdomID      State.KingdomID `json:"kingdomId"`
	TargetTypeID   int             `json:"targetTypeId"`
	TargetX        int             `json:"targetX"`
	TargetY        int             `json:"targetY"`
	EventCampID    int64           `json:"eventCampId"`
}

type nomadTargetLockRequest struct {
	nomadTargetRequest
	VictoryCount int64     `json:"victoryCount"`
	DefenseScore int64     `json:"defenseScore"`
	EventEndsAt  time.Time `json:"eventEndsAt"`
}

type nomadCampAttackRequest struct {
	nomadTargetRequest
	Mode                string               `json:"mode"`
	ScoreTarget         int64                `json:"scoreTarget"`
	MinimumRemainingSec int64                `json:"minimumRemainingSec"`
	VictoryCount        int64                `json:"victoryCount"`
	Preset              AttackPresets.Preset `json:"preset"`
	CommanderIDs        []State.CommanderID  `json:"commanderIds"`
	HorseTravelBoostID  int                  `json:"horseTravelBoostId"`
	DailyAttackLimit    int64                `json:"dailyAttackLimit"`
}

type resolvedNomadCampAttackRequest struct {
	nomadCampAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

type nomadChainArrivalGuard struct {
	SourceCastleID    State.CastleID    `json:"sourceCastleId"`
	KingdomID         State.KingdomID   `json:"kingdomId"`
	TargetX           int               `json:"targetX"`
	TargetY           int               `json:"targetY"`
	PreviousCommander State.CommanderID `json:"previousCommanderId"`
	CurrentCommander  State.CommanderID `json:"currentCommanderId"`
}

type nomadCooldownSkipRequest struct {
	nomadTargetRequest
	MaximumRubyCost    int64 `json:"maximumRubyCost"`
	MinimumRubyReserve int64 `json:"minimumRubyReserve"`
}

type nomadCooldownVerification struct {
	nomadCooldownSkipRequest
	ResetStartedAt time.Time `json:"resetStartedAt"`
}

type appNomadCamp struct {
	Observation State.MapObservation
	Definition  GameData.EventCampDefinition
}

func planNomadMapScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := nomadMapScanContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	windows := towerMapScanWindows(source, request.Radius)
	steps := make([]Intent.Step, 0, len(windows)+2)
	if !source.Focused {
		steps = append(steps, castleFocusStep(source))
	}
	for index, window := range windows {
		payload, _ := json.Marshal(struct {
			KingdomID State.KingdomID `json:"KID"`
			X1        int             `json:"AX1"`
			Y1        int             `json:"AY1"`
			X2        int             `json:"AX2"`
			Y2        int             `json:"AY2"`
		}{source.KingdomID, window.X1, window.Y1, window.X2, window.Y2})
		steps = append(steps, commandStep(
			fmt.Sprintf("Refresh Nomad/Samurai map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa",
		))
	}
	steps = append(steps, Intent.Step{Name: "Record Nomad/Samurai map scan", Action: "nomad.scan.capture", ActionArguments: arguments})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: fmt.Sprintf("Refresh the four regular event camps around %s", castleLabel(source)), Steps: steps,
	}, nil
}

func planNomadDifficulty(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request invasionDifficultyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	if _, supported := nomadMapTypeForEvent(request.EventID); !supported {
		return Intent.Plan{}, fmt.Errorf("event %d is not supported by Auto Nomad/Samurai", request.EventID)
	}
	difficulty, valid := input.GameData.ScalableEvent(request.EventID, request.DifficultyID)
	if !valid {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not valid for event %d", request.DifficultyID, request.EventID)
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !input.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not unlocked by this player's achievements", request.DifficultyID)
	}
	score, active := input.State.EventScores.ByEvent[request.EventID]
	if !active {
		return Intent.Plan{}, fmt.Errorf("event %d is no longer active", request.EventID)
	}
	if score.DifficultyID > 0 {
		return Intent.Plan{}, fmt.Errorf("event %d already selected difficulty %d", request.EventID, score.DifficultyID)
	}
	payload, _ := json.Marshal(struct {
		EventID      int64 `json:"EID"`
		DifficultyID int64 `json:"EDID"`
		PremiumUsed  int   `json:"C2U"`
	}{request.EventID, request.DifficultyID, 0})
	return Intent.Plan{
		Claims:  []string{"event-difficulty"},
		Summary: fmt.Sprintf("Start regular-camp event %d at difficulty %d", request.EventID, request.DifficultyID),
		Steps: []Intent.Step{
			commandStep("Select Nomad/Samurai event difficulty", "sede", payload, "sede"),
			{Name: "Reset the previous regular-camp run", Action: "nomad.run.reset", ActionArguments: arguments},
		},
	}, nil
}

func planNomadTargetLock(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request nomadTargetLockRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	_, camps, maximumVictoryCount, err := validatedNomadCampSet(input, request.nomadTargetRequest)
	if err != nil {
		return Intent.Plan{}, err
	}
	for _, camp := range camps {
		if camp.Observation.EventCampVictoryCount < maximumVictoryCount {
			return Intent.Plan{}, fmt.Errorf("all four regular camps must be maxed before a target can be locked")
		}
	}
	weakest := weakestAppNomadCamp(camps)
	if weakest.Observation.X != request.TargetX || weakest.Observation.Y != request.TargetY ||
		weakest.Observation.EventCampID != request.EventCampID {
		return Intent.Plan{}, fmt.Errorf("camp %d:%d is not the weakest maxed camp in the current four-camp set", request.TargetX, request.TargetY)
	}
	request.VictoryCount = weakest.Observation.EventCampVictoryCount
	request.DefenseScore = appNomadDefenseScore(weakest)
	normalized, _ := json.Marshal(request)
	return Intent.Plan{
		Claims:  []string{nomadTargetClaim(request.nomadTargetRequest)},
		Summary: fmt.Sprintf("Lock weakest maxed camp at %d:%d", request.TargetX, request.TargetY),
		Steps:   []Intent.Step{{Name: "Lock regular event camp", Action: "nomad.target.capture", ActionArguments: normalized}},
	}, nil
}

func planNomadCampAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, _, _, err := nomadCampAttackContext(input, arguments)
	if err != nil {
		if errors.Is(err, Intent.ErrPlanStale) {
			return Intent.Plan{Summary: "Nomad/Samurai camp progression changed; reevaluate the current camp state"}, nil
		}
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	commanderCount := len(request.CommanderIDs)
	if request.Mode == "level" {
		commanderCount = 1
	}
	selection := &craCommanderSelectionRequest{Candidates: request.CommanderIDs, Count: commanderCount, Strategy: "lowest_id"}
	resolution, err := resolveCRACommanders(input.State, selection, craCommanderSelectionOptions{
		DefaultCount: commanderCount, RequireAvailable: true,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	resolution.Selected = orderNomadChainCommanders(input, source, target, resolution.Selected)
	request.CommanderIDs = append([]State.CommanderID(nil), resolution.Selected...)
	if _, err := buildAttackSetupForCommanders(invasionAttackSetup(request.Preset), source, input.GameData, len(resolution.Selected)); err != nil {
		return Intent.Plan{}, fmt.Errorf("validate chained preset inventory: %w", err)
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	steps := make([]Intent.Step, 0, len(resolution.Selected)*2+8)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() || time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep("Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl"))
	}
	for _, commanderID := range resolution.Selected {
		steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	}
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Verify chained preset inventory", Action: "nomad.attack.inventory.guard",
		ActionArguments: mustMarshalNomadAttackRequest(request),
	}))
	for index, commanderID := range resolution.Selected {
		resolvedArguments, _ := json.Marshal(resolvedNomadCampAttackRequest{nomadCampAttackRequest: request, CommanderID: commanderID})
		steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
		steps = append(steps, deferredCRACommandStep(
			fmt.Sprintf("Build and launch camp attack with commander %d", commanderID),
			"nomad.attack.build", resolvedArguments, contextPayload,
		))
		steps = append(steps, Intent.Step{
			Name: "Capture confirmed Nomad/Samurai camp movement", Action: "nomad.attack.capture",
			ActionArguments: resolvedArguments,
		})
		if request.Mode == "chain" && index > 0 {
			arrivalGuard, _ := json.Marshal(nomadChainArrivalGuard{
				SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
				PreviousCommander: resolution.Selected[index-1], CurrentCommander: commanderID,
			})
			steps = append(steps, Intent.Step{
				Name: "Verify authoritative chain arrival order", Action: "nomad.attack.arrival.guard", ActionArguments: arrivalGuard,
			})
		}
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		nomadTargetClaim(request.nomadTargetRequest),
	}
	claims = append(claims, craCommanderClaims(resolution.Selected)...)
	summary := fmt.Sprintf("Level camp %d:%d with commander %d", target.X, target.Y, resolution.Selected[0])
	if request.Mode == "chain" {
		summary = fmt.Sprintf("Chain %d attacks into locked camp %d:%d", len(resolution.Selected), target.X, target.Y)
	}
	return Intent.Plan{
		Claims:    claims,
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoNomad", Affinity: "castle:" + castleID},
		Summary:   summary, Steps: steps,
	}, nil
}

func planNomadCooldownSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request nomadCooldownSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	definition, remaining, err := validateNomadCooldownSkip(input.State, input.GameData, request, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	resetStartedAt := time.Now().UTC()
	verification, _ := json.Marshal(nomadCooldownVerification{nomadCooldownSkipRequest: request, ResetStartedAt: resetStartedAt})
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X         int             `json:"X"`
		Y         int             `json:"Y"`
		MapID     int             `json:"MID"`
		NodeID    int             `json:"NID"`
	}{request.KingdomID, request.TargetX, request.TargetY, -1, -1})
	return Intent.Plan{
		Claims:  []string{nomadTargetClaim(request.nomadTargetRequest), "account-resources"},
		Summary: fmt.Sprintf("Reset %d-second cooldown on locked camp %d:%d for at most %d rubies", remaining, request.TargetX, request.TargetY, definition.SkipCost),
		Steps: []Intent.Step{
			{Name: "Verify locked camp cooldown and ruby reserve", Action: "nomad.cooldown.guard", ActionArguments: arguments},
			commandStep("Reset locked camp cooldown", "sdc", payload, "sdc"),
			{Name: "Verify returned zero-cooldown camp row", Action: "nomad.cooldown.verify", ActionArguments: verification},
		},
	}, nil
}

func nomadMapScanContext(input Intent.PlanningContext, arguments json.RawMessage) (nomadMapScanRequest, State.CastleState, error) {
	var request nomadMapScanRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return nomadMapScanRequest{}, State.CastleState{}, err
	}
	if request.SourceCastleID <= 0 || request.Radius < 1 || request.Radius > nomadIntentRadius {
		return nomadMapScanRequest{}, State.CastleState{}, fmt.Errorf("Nomad/Samurai map scan requires a source castle and radius between 1 and %d", nomadIntentRadius)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return nomadMapScanRequest{}, State.CastleState{}, fmt.Errorf("source castle %d is unavailable", request.SourceCastleID)
	}
	if source.KingdomID != 0 {
		return nomadMapScanRequest{}, State.CastleState{}, fmt.Errorf("Nomad/Samurai source castle must be in the Great Empire")
	}
	return request, source, nil
}

func nomadCampAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (nomadCampAttackRequest, State.CastleState, State.MapObservation, GameData.EventCampDefinition, int64, error) {
	var request nomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, err
	}
	request.Mode = strings.ToLower(strings.TrimSpace(request.Mode))
	if request.Mode != "level" && request.Mode != "chain" {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("camp attack mode must be level or chain")
	}
	if request.ScoreTarget <= 0 || request.MinimumRemainingSec < 0 || len(request.CommanderIDs) == 0 {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("camp attack requires a score target and at least one commander")
	}
	source, camps, maximumVictoryCount, err := validatedNomadCampSet(input, request.nomadTargetRequest)
	if err != nil {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, err
	}
	var selected appNomadCamp
	found := false
	for _, camp := range camps {
		if camp.Observation.X == request.TargetX && camp.Observation.Y == request.TargetY && camp.Observation.EventCampID == request.EventCampID {
			selected, found = camp, true
			break
		}
	}
	if !found || selected.Observation.EventCampVictoryCount != request.VictoryCount {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf(
			"%w: camp %d:%d changed progression state", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	key := fmt.Sprintf("%d:%d:%d", selected.Observation.KingdomID, selected.Observation.X, selected.Observation.Y)
	if cooldown, found := input.State.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf(
			"camp %d:%d is awaiting an authoritative cooldown refresh", request.TargetX, request.TargetY,
		)
	}
	if nomadAppCooldownRemaining(input.State, selected.Observation, time.Now().UTC()) > 0 {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("camp %d:%d is on cooldown", request.TargetX, request.TargetY)
	}
	if request.Mode == "level" {
		if selected.Observation.EventCampVictoryCount >= maximumVictoryCount {
			return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("level mode cannot attack an already maxed camp")
		}
	} else {
		for _, camp := range camps {
			if camp.Observation.EventCampVictoryCount < maximumVictoryCount {
				return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("chain mode requires all four camps to be maxed")
			}
		}
		if !nomadLockMatches(input.State.NomadCamps.LockedTarget, request.nomadTargetRequest) {
			return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("chain mode target does not match the locked camp")
		}
	}
	score, active := input.State.EventScores.ByEvent[request.EventID]
	if !active || score.DifficultyID != request.DifficultyID {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("event %d difficulty %d is no longer active", request.EventID, request.DifficultyID)
	}
	if score.PlayerScore >= request.ScoreTarget {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("event score target reached: %d / %d", score.PlayerScore, request.ScoreTarget)
	}
	if remaining := invasionRemainingSeconds(score, time.Now().UTC()); remaining >= 0 && remaining <= request.MinimumRemainingSec {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, fmt.Errorf("event has only %d seconds remaining", remaining)
	}
	return request, source, selected.Observation, selected.Definition, maximumVictoryCount, nil
}

func validatedNomadCampSet(
	input Intent.PlanningContext,
	request nomadTargetRequest,
) (State.CastleState, []appNomadCamp, int64, error) {
	if request.SourceCastleID <= 0 || request.EventID <= 0 || request.DifficultyID <= 0 || request.TargetTypeID <= 0 || request.EventCampID <= 0 {
		return State.CastleState{}, nil, 0, fmt.Errorf("regular camp target requires source, event, difficulty, type, and scaling-camp id")
	}
	expectedTypeID, supported := nomadMapTypeForEvent(request.EventID)
	if !supported || expectedTypeID != request.TargetTypeID {
		return State.CastleState{}, nil, 0, fmt.Errorf("event %d does not use regular camp type %d", request.EventID, request.TargetTypeID)
	}
	if input.GameData == nil {
		return State.CastleState{}, nil, 0, fmt.Errorf("official game data is unavailable")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 || request.KingdomID != source.KingdomID {
		return State.CastleState{}, nil, 0, fmt.Errorf("regular camp source and target must be in the Great Empire")
	}
	progression := input.GameData.EventCampProgression(request.EventID, request.DifficultyID, request.TargetTypeID)
	if len(progression) == 0 {
		return State.CastleState{}, nil, 0, fmt.Errorf("official regular-camp progression is unavailable")
	}
	maximumVictoryCount := progression[len(progression)-1].VictoryCount
	lastScan := input.State.NomadCamps.LastScannedAt[source.ID]
	if lastScan.IsZero() {
		return State.CastleState{}, nil, 0, fmt.Errorf("the four regular camps have not been scanned")
	}
	camps := make([]appNomadCamp, 0, nomadIntentCampCount)
	for _, observation := range input.State.Map[source.KingdomID] {
		if observation.TypeID != request.TargetTypeID || observation.EventCampID <= 0 || observation.ObservedAt.Before(lastScan) ||
			appNomadDistanceSquared(source, observation) > nomadIntentRadius*nomadIntentRadius {
			continue
		}
		definition, found := input.GameData.EventCamp(observation.EventCampID)
		if !found || definition.EventID != request.EventID || definition.DifficultyID != request.DifficultyID ||
			definition.AreaTypeID != request.TargetTypeID || definition.VictoryCount != observation.EventCampVictoryCount {
			continue
		}
		camps = append(camps, appNomadCamp{Observation: observation, Definition: definition})
	}
	sort.Slice(camps, func(left, right int) bool {
		leftDistance := appNomadDistanceSquared(source, camps[left].Observation)
		rightDistance := appNomadDistanceSquared(source, camps[right].Observation)
		if leftDistance != rightDistance {
			return leftDistance < rightDistance
		}
		if camps[left].Observation.Y != camps[right].Observation.Y {
			return camps[left].Observation.Y < camps[right].Observation.Y
		}
		return camps[left].Observation.X < camps[right].Observation.X
	})
	if len(camps) < nomadIntentCampCount {
		return State.CastleState{}, nil, 0, fmt.Errorf("found %d of the required %d regular camps", len(camps), nomadIntentCampCount)
	}
	return source, camps[:nomadIntentCampCount], maximumVictoryCount, nil
}

func (application *Application) resolveNomadCampAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request resolvedNomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	_, source, target, definition, _, err := nomadCampAttackContext(input, mustMarshalNomadAttackRequest(request.nomadCampAttackRequest))
	if err != nil {
		return Intent.Step{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Intent.Step{}, fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("event-camp:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.EventCampID, Level: definition.CampLevel, VictoryCount: target.EventCampVictoryCount,
			},
			Level: definition.CampLevel, CastleTypeID: target.TypeID, PvP: false, LegendaryFight: false,
		},
	})
	if err != nil {
		return Intent.Step{}, fmt.Errorf("resolve Nomad/Samurai camp attack capacity: %w", err)
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(request.Preset, capacity))
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build camp preset %q: %w", request.Preset.Name, err)
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve camp horse travel boost: %w", err)
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build camp CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack locked camp at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func (application *Application) captureNomadCampLaunch(_ context.Context, arguments json.RawMessage) error {
	var request resolvedNomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		var selected State.MovementState
		for _, movement := range gameState.Movements {
			if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
				movement.KingdomID != request.KingdomID || movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
				movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
				continue
			}
			if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
				movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
				selected = movement
			}
		}
		if selected.ID == 0 {
			return nil, false, fmt.Errorf("CRA response did not return commander %d's Nomad/Samurai movement", request.CommanderID)
		}
		launchedAt := selected.ObservedAt
		if launchedAt.IsZero() {
			launchedAt = time.Now().UTC()
		}
		changed := State.RecordEventAttackLaunch(gameState, request.EventID, State.EventAttackRecord{
			MovementID: selected.ID, Kind: State.EventActivityCamp, KingdomID: request.KingdomID,
			TargetTypeID: request.TargetTypeID, TargetX: request.TargetX, TargetY: request.TargetY,
			LaunchedAt: launchedAt.UTC(), ArrivesAt: selected.ArrivesAt.UTC(),
		})
		return []string{"event-scores", "movements"}, changed, nil
	})
	return err
}

func (application *Application) captureNomadScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := nomadMapScanContext(Intent.PlanningContext{State: application.State.Snapshot()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("source castle %d is no longer focused", request.SourceCastleID)
		}
		if gameState.NomadCamps.LastScannedAt == nil {
			gameState.NomadCamps.LastScannedAt = map[State.CastleID]time.Time{}
		}
		scannedAt := request.ScanStartedAt.UTC()
		if scannedAt.IsZero() {
			scannedAt = time.Now().UTC()
		}
		if gameState.NomadCamps.LastScannedAt[request.SourceCastleID].Equal(scannedAt) {
			return nil, false, nil
		}
		gameState.NomadCamps.LastScannedAt[request.SourceCastleID] = scannedAt
		return []string{"nomad-camps"}, true, nil
	})
	return err
}

func (application *Application) resetNomadRun(_ context.Context, arguments json.RawMessage) error {
	var request invasionDifficultyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		changed := gameState.NomadCamps.LockedTarget != nil || len(gameState.NomadCamps.Cooldowns) > 0 || len(gameState.NomadCamps.LastScannedAt) > 0
		gameState.NomadCamps.LockedTarget = nil
		gameState.NomadCamps.Cooldowns = map[string]State.NomadCampCooldownState{}
		gameState.NomadCamps.LastScannedAt = map[State.CastleID]time.Time{}
		if score, found := gameState.EventScores.ByEvent[request.EventID]; found && score.DifficultyID != request.DifficultyID {
			score.DifficultyID = request.DifficultyID
			gameState.EventScores.ByEvent[request.EventID] = score
			changed = true
		}
		return []string{"nomad-camps", "event-scores"}, changed, nil
	})
	return err
}

func (application *Application) captureNomadTarget(_ context.Context, arguments json.RawMessage) error {
	var request nomadTargetLockRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		observation, exists := gameState.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
		if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID ||
			observation.EventCampVictoryCount != request.VictoryCount {
			return nil, false, fmt.Errorf("camp %d:%d changed before it could be locked", request.TargetX, request.TargetY)
		}
		next := State.NomadCampTargetState{
			SourceCastleID: request.SourceCastleID, EventID: request.EventID, DifficultyID: request.DifficultyID,
			KingdomID: request.KingdomID, TypeID: request.TargetTypeID, X: request.TargetX, Y: request.TargetY,
			EventCampID: request.EventCampID, VictoryCount: request.VictoryCount, DefenseScore: request.DefenseScore,
			EventEndsAt: request.EventEndsAt.UTC(), LockedAt: time.Now().UTC(),
		}
		if gameState.NomadCamps.LockedTarget != nil && *gameState.NomadCamps.LockedTarget == next {
			return nil, false, nil
		}
		gameState.NomadCamps.LockedTarget = &next
		return []string{"nomad-camps"}, true, nil
	})
	return err
}

func (application *Application) guardNomadCampAttack(_ context.Context, arguments json.RawMessage) error {
	var request resolvedNomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	currentData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, source, target, _, _, err := nomadCampAttackContext(
		Intent.PlanningContext{State: state, GameData: currentData}, mustMarshalNomadAttackRequest(request.nomadCampAttackRequest),
	)
	if err != nil {
		return err
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID || dialog.Target.TypeID != target.TypeID ||
		dialog.Target.X != target.X || dialog.Target.Y != target.Y || dialog.Target.EventCampID != target.EventCampID ||
		dialog.Target.EventCampVictoryCount != target.EventCampVictoryCount || dialog.Target.EventCampCooldownRemaining > 0 {
		return fmt.Errorf("authoritative ADI row no longer matches ready camp %d:%d", target.X, target.Y)
	}
	return nil
}

func (application *Application) guardNomadAttackInventory(_ context.Context, arguments json.RawMessage) error {
	var request nomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	source, exists := state.Castles[request.SourceCastleID]
	if !exists {
		return fmt.Errorf("source castle %d is unavailable", request.SourceCastleID)
	}
	currentData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, err := buildAttackSetupForCommanders(invasionAttackSetup(request.Preset), source, currentData, len(request.CommanderIDs))
	return err
}

func (application *Application) guardNomadChainArrival(_ context.Context, arguments json.RawMessage) error {
	var request nomadChainArrivalGuard
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || request.PreviousCommander < 0 || request.CurrentCommander < 0 ||
		request.PreviousCommander == request.CurrentCommander {
		return fmt.Errorf("invalid Nomad chain arrival guard")
	}
	gameState := application.State.Snapshot()
	previous, previousFound := latestNomadChainMovement(gameState, request, request.PreviousCommander)
	current, currentFound := latestNomadChainMovement(gameState, request, request.CurrentCommander)
	if !previousFound || !currentFound {
		return fmt.Errorf(
			"server did not return both chained movements for commanders %d and %d",
			request.PreviousCommander, request.CurrentCommander,
		)
	}
	if current.ArrivesAt.Before(*previous.ArrivesAt) {
		return fmt.Errorf(
			"commander %d arrives at %s before commander %d at %s",
			request.CurrentCommander, current.ArrivesAt.Format(time.RFC3339Nano),
			request.PreviousCommander, previous.ArrivesAt.Format(time.RFC3339Nano),
		)
	}
	return nil
}

func latestNomadChainMovement(
	gameState State.GameState,
	request nomadChainArrivalGuard,
	commanderID State.CommanderID,
) (State.MovementState, bool) {
	var selected State.MovementState
	found := false
	for _, movement := range gameState.Movements {
		if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID || movement.KingdomID != request.KingdomID ||
			movement.TargetX != request.TargetX || movement.TargetY != request.TargetY || movement.CommanderID == nil ||
			*movement.CommanderID != commanderID || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
			continue
		}
		if !found || movement.ArrivesAt.After(*selected.ArrivesAt) {
			selected, found = movement, true
		}
	}
	return selected, found
}

func (application *Application) guardNomadCooldownSkip(_ context.Context, arguments json.RawMessage) error {
	var request nomadCooldownSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	currentData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	_, _, err := validateNomadCooldownSkip(application.State.Snapshot(), currentData, request, time.Now().UTC())
	return err
}

func (application *Application) verifyNomadCooldownSkip(_ context.Context, arguments json.RawMessage) error {
	var request nomadCooldownVerification
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	observation, exists := state.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID ||
		observation.ObservedAt.Before(request.ResetStartedAt) || observation.EventCampCooldownRemaining != 0 {
		return fmt.Errorf("cooldown reset did not return an authoritative zero-cooldown row for camp %d:%d", request.TargetX, request.TargetY)
	}
	return nil
}

func validateNomadCooldownSkip(
	gameState State.GameState,
	gameData *GameData.Store,
	request nomadCooldownSkipRequest,
	now time.Time,
) (GameData.EventCampDefinition, int, error) {
	if request.MaximumRubyCost <= 0 || request.MinimumRubyReserve < 0 {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("cooldown reset requires a positive ruby cap and non-negative reserve")
	}
	if !nomadLockMatches(gameState.NomadCamps.LockedTarget, request.nomadTargetRequest) {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("cooldown reset target does not match the locked camp")
	}
	observation, exists := gameState.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("locked camp %d:%d is unavailable", request.TargetX, request.TargetY)
	}
	definition, found := gameData.EventCamp(request.EventCampID)
	if !found || definition.EventID != request.EventID || definition.DifficultyID != request.DifficultyID ||
		definition.AreaTypeID != request.TargetTypeID || definition.CooldownSec <= 0 || definition.SkipCost <= 0 {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("locked camp has no official skippable cooldown definition")
	}
	remaining := nomadAppCooldownRemaining(gameState, observation, now)
	if remaining <= 0 {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("locked camp is no longer on cooldown")
	}
	if definition.SkipCost > request.MaximumRubyCost {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf("official reset may cost %d rubies, above configured cap %d", definition.SkipCost, request.MaximumRubyCost)
	}
	rubies := playerResourceByOfficialKey(gameState, gameData, "C2")
	if rubies-float64(request.MinimumRubyReserve) < float64(definition.SkipCost) {
		return GameData.EventCampDefinition{}, 0, fmt.Errorf(
			"cooldown reset reserves %d rubies and may cost %d; %.0f are observed",
			request.MinimumRubyReserve, definition.SkipCost, rubies,
		)
	}
	return definition, remaining, nil
}

func nomadLockMatches(locked *State.NomadCampTargetState, request nomadTargetRequest) bool {
	return locked != nil && locked.SourceCastleID == request.SourceCastleID && locked.EventID == request.EventID &&
		locked.DifficultyID == request.DifficultyID && locked.KingdomID == request.KingdomID &&
		locked.TypeID == request.TargetTypeID && locked.X == request.TargetX && locked.Y == request.TargetY &&
		locked.EventCampID == request.EventCampID
}

func nomadMapTypeForEvent(eventID int64) (int, bool) {
	switch eventID {
	case nomadIntentEventID:
		return nomadIntentCampTypeID, true
	case samuraiIntentEventID:
		return samuraiIntentCampTypeID, true
	default:
		return 0, false
	}
}

func weakestAppNomadCamp(camps []appNomadCamp) appNomadCamp {
	result := camps[0]
	for _, camp := range camps[1:] {
		left, right := appNomadDefenseScore(camp), appNomadDefenseScore(result)
		if left < right || left == right && (camp.Observation.Y < result.Observation.Y ||
			camp.Observation.Y == result.Observation.Y && camp.Observation.X < result.Observation.X) {
			result = camp
		}
	}
	return result
}

func appNomadDefenseScore(camp appNomadCamp) int64 {
	observation := camp.Observation
	return camp.Definition.MaximumDefenseTroopCount*1000 + observation.EventCampBaseWallBonus +
		observation.EventCampBaseGateBonus + observation.EventCampBaseMoatBonus
}

func appNomadDistanceSquared(source State.CastleState, target State.MapObservation) int {
	x, y := target.X-source.X, target.Y-source.Y
	return x*x + y*y
}

func nomadAppCooldownRemaining(gameState State.GameState, observation State.MapObservation, now time.Time) int {
	remaining, observedAt := observation.EventCampCooldownRemaining, observation.ObservedAt
	key := fmt.Sprintf("%d:%d:%d", observation.KingdomID, observation.X, observation.Y)
	if cooldown, found := gameState.NomadCamps.Cooldowns[key]; found && cooldown.CooldownObservedAt.After(observedAt) {
		remaining, observedAt = cooldown.CooldownRemaining, cooldown.CooldownObservedAt
	}
	if remaining <= 0 {
		return 0
	}
	if !observedAt.IsZero() && now.After(observedAt) {
		remaining -= int(now.Sub(observedAt) / time.Second)
	}
	return max(0, remaining)
}

func nomadTargetClaim(request nomadTargetRequest) string {
	return fmt.Sprintf("nomad-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
}

func mustMarshalNomadAttackRequest(request nomadCampAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

package App

import (
	"context"
	"encoding/json"
	"fmt"
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
	advisorMaxAttackCount   = 9999
	advisorUniversalTokenID = State.CurrencyID(76)
	advisorPegasusTicketID  = State.CurrencyID(22)
	advisorDefaultCoinCost  = int64(500)
)

type advisorActivationRequest struct {
	EventID             int64 `json:"eventId"`
	ConfirmedTokenSpend bool  `json:"confirmedTokenSpend"`
}

type advisorAttackRequest struct {
	nomadTargetRequest
	Preset                AttackPresets.Preset `json:"preset"`
	CommanderID           State.CommanderID    `json:"commanderId"`
	AttackCount           int                  `json:"attackCount"`
	MinimumRemainingSec   int64                `json:"minimumRemainingSec"`
	CoinCostPerAttack     int64                `json:"coinCostPerAttack"`
	MinimumCoinReserve    int64                `json:"minimumCoinReserve"`
	RubyCostPerAttack     int64                `json:"rubyCostPerAttack"`
	MinimumRubyReserve    int64                `json:"minimumRubyReserve"`
	MinimumFeatherReserve int64                `json:"minimumFeatherReserve"`
	TimeSkipReserve       map[string]int64     `json:"timeSkipReserve"`
	HorseTravelBoostID    int                  `json:"horseTravelBoostId"`
}

type advisorAttackBody struct {
	attackBody
	AttackCount int `json:"AAC"`
	Mode        int `json:"AASM"`
	AdvisorType int `json:"AAT"`
}

func planAdvisorActivation(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request advisorActivationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if !request.ConfirmedTokenSpend {
		return Intent.Plan{}, fmt.Errorf("advisor activation consumes a paid event token; confirmedTokenSpend=true is required")
	}
	score, found := input.State.EventScores.ByEvent[request.EventID]
	if !found || (request.EventID != nomadIntentEventID && request.EventID != samuraiIntentEventID) {
		return Intent.Plan{}, fmt.Errorf("event %d is not an active Nomad or Samurai event", request.EventID)
	}
	if score.DifficultyID <= 0 || invasionRemainingSeconds(score, time.Now().UTC()) <= 0 {
		return Intent.Plan{}, fmt.Errorf("event %d must have an active difficulty before advisor activation", request.EventID)
	}
	if score.AdvisorActive {
		return Intent.Plan{}, fmt.Errorf("the advisor is already active for event %d", request.EventID)
	}
	tokenID := score.AdvisorCurrencyID
	if tokenID == 0 {
		tokenID = advisorEventTokenCurrency(request.EventID)
	}
	eventTokens := input.State.Player.Currencies[tokenID]
	universalTokens := input.State.Player.Currencies[advisorUniversalTokenID]
	if !score.AdvisorFree && eventTokens < 1 && universalTokens < 1 {
		return Intent.Plan{}, fmt.Errorf(
			"advisor activation requires one event token (currency %d) or universal advisor token; neither is available",
			tokenID,
		)
	}
	payload := json.RawMessage(`{"AAT":1}`)
	return Intent.Plan{
		Claims:  []string{"advisor:activation", "account-resources", "event:" + strconv.FormatInt(request.EventID, 10)},
		Summary: fmt.Sprintf("Activate the advisor for event %d with one paid token", request.EventID),
		Steps:   []Intent.Step{commandStep("Consume one advisor token and activate the event advisor", "aa", payload, "aa")},
	}, nil
}

func planAdvisorOverview(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	if _, found := activeAppAdvisorEvent(input.State); !found {
		return Intent.Plan{}, fmt.Errorf("no active Nomad or Samurai event is available")
	}
	return Intent.Plan{
		Claims: []string{"advisor:overview"}, Summary: "Refresh advisor run totals",
		Steps: []Intent.Step{commandStep("Refresh advisor overview", "aao", json.RawMessage(`{"AAT":1}`), "aao")},
	}, nil
}

func planAdvisorAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, _, err := advisorAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Intent.Plan{}, fmt.Errorf("commander %d is not available", request.CommanderID)
	}
	if _, err := buildAttackSetup(invasionAttackSetup(request.Preset), source, input.GameData); err != nil {
		return Intent.Plan{}, fmt.Errorf("validate advisor preset %q: %w", request.Preset.Name, err)
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	steps := make([]Intent.Step, 0, 5)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() || time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep("Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl"))
	}
	steps = append(steps, generalSkillsContextSteps(input.State, request.CommanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps, deferredCRACommandStep("Build and launch advisor attack", "advisor.attack.build", arguments, contextPayload))
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
			"advisor:event:" + strconv.FormatInt(request.EventID, 10),
			"commander:" + strconv.FormatInt(int64(request.CommanderID), 10),
			"leader:commander:" + strconv.FormatInt(int64(request.CommanderID), 10),
		},
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoAdvisor", Affinity: "castle:" + castleID},
		Summary:   fmt.Sprintf("Launch %d advisor attacks against camp %d:%d", request.AttackCount, target.X, target.Y),
		Steps:     steps,
	}, nil
}

func (application *Application) resolveAdvisorAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	request, source, target, definition, err := advisorAttackContext(input, arguments)
	if err != nil {
		return Intent.Step{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Intent.Step{}, fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	dialog := input.State.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID || dialog.Target.TypeID != target.TypeID ||
		dialog.Target.X != target.X || dialog.Target.Y != target.Y || dialog.Target.EventCampID != target.EventCampID {
		return Intent.Step{}, fmt.Errorf("authoritative ADI row no longer matches advisor camp %d:%d", target.X, target.Y)
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
		return Intent.Step{}, fmt.Errorf("resolve advisor attack capacity: %w", err)
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(request.Preset, capacity))
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build advisor preset %q: %w", request.Preset.Name, err)
	}
	body := advisorAttackBody{
		attackBody:  invasionAttackBody(source, target, request.CommanderID, built),
		AttackCount: request.AttackCount, Mode: 0, AdvisorType: 1,
	}
	if err := applyCastleHorseTravelBoost(&body.attackBody, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve advisor horse travel boost: %w", err)
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build advisor CRA payload: %w", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Intent.Step{}, fmt.Errorf("decode advisor CRA inventory: %w", err)
	}
	if err := validateRepeatedAttackInventory(fields, source, request.AttackCount); err != nil {
		return Intent.Step{}, fmt.Errorf("advisor inventory preflight: %w", err)
	}
	return commandStep(fmt.Sprintf("Launch advisor at %d:%d for %d attacks", target.X, target.Y, request.AttackCount), "cra", payload, "cra"), nil
}

func advisorAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (advisorAttackRequest, State.CastleState, State.MapObservation, GameData.EventCampDefinition, error) {
	var request advisorAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, err
	}
	if request.AttackCount < 1 || request.AttackCount > advisorMaxAttackCount {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("advisor attackCount must be between 1 and %d", advisorMaxAttackCount)
	}
	if request.MinimumRemainingSec < 0 || request.MinimumCoinReserve < 0 || request.RubyCostPerAttack < 0 ||
		request.MinimumRubyReserve < 0 || request.MinimumFeatherReserve < 0 {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("advisor timing and reserves are invalid")
	}
	if request.CoinCostPerAttack <= 0 {
		request.CoinCostPerAttack = advisorDefaultCoinCost
	}
	if advisorUsesRubyHorse(request.HorseTravelBoostID) && request.RubyCostPerAttack <= 0 {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("advisor ruby horse requires a positive rubyCostPerAttack")
	}
	for key, reserve := range request.TimeSkipReserve {
		if !advisorTimeSkipKey(key) || reserve < 0 {
			return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("invalid advisor time-skip reserve %q", key)
		}
	}
	if input.GameData == nil {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("official game data is unavailable")
	}
	score, active := input.State.EventScores.ByEvent[request.EventID]
	if !active || score.DifficultyID != request.DifficultyID || !score.AdvisorActive {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("advisor is not active for event %d difficulty %d", request.EventID, request.DifficultyID)
	}
	remaining := invasionRemainingSeconds(score, time.Now().UTC())
	usableSeconds := remaining - request.MinimumRemainingSec
	if remaining <= 0 || usableSeconds < State.AdvisorEstimatedCycleSeconds {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("event has only %d usable seconds remaining", max(int64(0), usableSeconds))
	}
	if maximum := int(usableSeconds / State.AdvisorEstimatedCycleSeconds); request.AttackCount > maximum {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
			"%d advisor attacks exceed the event-time capacity of %d", request.AttackCount, maximum,
		)
	}
	if run := input.State.Advisor.Run; run != nil && advisorRunMatchesScore(*run, score) {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
			"event %d already has an advisor run in %s state", request.EventID, run.Status,
		)
	}
	source, camps, _, err := validatedNomadCampSet(input, request.nomadTargetRequest)
	if err != nil {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, err
	}
	var selected appNomadCamp
	found := false
	for _, camp := range camps {
		if camp.Observation.X == request.TargetX && camp.Observation.Y == request.TargetY && camp.Observation.EventCampID == request.EventCampID {
			selected, found = camp, true
			break
		}
	}
	if !found {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("advisor camp %d:%d is unavailable", request.TargetX, request.TargetY)
	}
	coins := int64(playerResourceByOfficialKey(input.State, input.GameData, "C1"))
	requiredCoins := request.CoinCostPerAttack * int64(request.AttackCount)
	if coins-request.MinimumCoinReserve < requiredCoins {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
			"advisor reserves %d coins and budgets %d per attack; %d attacks need %d with %d observed",
			request.MinimumCoinReserve, request.CoinCostPerAttack, request.AttackCount, requiredCoins, coins,
		)
	}
	if advisorUsesRubyHorse(request.HorseTravelBoostID) {
		rubies := int64(playerResourceByOfficialKey(input.State, input.GameData, "C2"))
		requiredRubies := request.RubyCostPerAttack * int64(request.AttackCount)
		if rubies-request.MinimumRubyReserve < requiredRubies {
			return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
				"advisor reserves %d rubies and budgets %d per attack; %d attacks need %d with %d observed",
				request.MinimumRubyReserve, request.RubyCostPerAttack, request.AttackCount, requiredRubies, rubies,
			)
		}
	}
	if _, premiumTravel := horseTravelBoostFields(request.HorseTravelBoostID); premiumTravel == 1 {
		available := int64(input.State.Player.Currencies[advisorPegasusTicketID]) - request.MinimumFeatherReserve
		if available < int64(request.AttackCount) {
			return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
				"advisor needs one travel feather per attack; %d are available above reserve for %d attacks",
				max(int64(0), available), request.AttackCount,
			)
		}
	}
	if needed := int64(max(0, request.AttackCount-1)); selected.Definition.CooldownSec > 0 &&
		advisorAvailableTimeSkips(input.State, request.TimeSkipReserve, selected.Definition.CooldownSec) < needed {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf(
			"advisor needs %d cooldown skips above reserve for %d attacks", needed, request.AttackCount,
		)
	}
	return request, source, selected.Observation, selected.Definition, nil
}

func advisorAvailableTimeSkips(gameState State.GameState, reserves map[string]int64, cooldownSec int64) int64 {
	options := []struct {
		key      string
		currency State.CurrencyID
		seconds  int64
	}{
		{"MS1", 1001, 60}, {"MS2", 1002, 300}, {"MS3", 1003, 600}, {"MS4", 1004, 1800},
		{"MS5", 1005, 3600}, {"MS6", 1006, 18000}, {"MS7", 1007, 86400},
	}
	total := int64(0)
	for _, option := range options {
		if option.seconds < cooldownSec {
			continue
		}
		available := int64(gameState.Player.Currencies[option.currency]) - advisorTimeSkipReserve(reserves, option.key)
		if available > 0 {
			total += available
		}
	}
	return total
}

func advisorTimeSkipReserve(reserves map[string]int64, key string) int64 {
	for candidate, value := range reserves {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return value
		}
	}
	return 0
}

func advisorUsesRubyHorse(horseTravelBoostID int) bool {
	return horseTravelBoostID == 1008 || horseTravelBoostID == 1009
}

func advisorRunMatchesScore(run State.AdvisorRunState, score State.ScalableEventScore) bool {
	if run.EventID != score.EventID {
		return false
	}
	currentEnd := time.Time{}
	if !score.ObservedAt.IsZero() && score.RemainingSec > 0 {
		currentEnd = score.ObservedAt.Add(time.Duration(score.RemainingSec) * time.Second).UTC()
	}
	if run.EventEndsAt.IsZero() || currentEnd.IsZero() {
		return run.Status == "running" || !run.UpdatedAt.IsZero() && run.UpdatedAt.After(score.ObservedAt.Add(-time.Minute))
	}
	delta := run.EventEndsAt.Sub(currentEnd)
	return delta >= -10*time.Minute && delta <= 10*time.Minute
}

func advisorTimeSkipKey(value string) bool {
	switch strings.ToUpper(strings.TrimSpace(value)) {
	case "MS1", "MS2", "MS3", "MS4", "MS5", "MS6", "MS7":
		return true
	default:
		return false
	}
}

func advisorEventTokenCurrency(eventID int64) State.CurrencyID {
	if eventID == nomadIntentEventID {
		return 77
	}
	if eventID == samuraiIntentEventID {
		return 78
	}
	return 0
}

func activeAppAdvisorEvent(gameState State.GameState) (State.ScalableEventScore, bool) {
	if score, found := gameState.ActiveScalableEventScore(); found &&
		(score.EventID == nomadIntentEventID || score.EventID == samuraiIntentEventID) {
		return score, true
	}
	for _, eventID := range []int64{nomadIntentEventID, samuraiIntentEventID} {
		if score, found := gameState.EventScores.ByEvent[eventID]; found && score.RemainingSec > 0 {
			return score, true
		}
	}
	return State.ScalableEventScore{}, false
}

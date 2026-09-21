package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"math"
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("advisor activation consumes a paid event token; confirmedTokenSpend=true is required"), Localization.New("server.app.advisor_activation_consumes_a.a5bf9a48", "advisor activation consumes a paid event token; confirmedTokenSpend=true is required", nil))
	}
	score, found := input.State.LookupScalableEventScore(request.EventID)
	if !found || (request.EventID != nomadIntentEventID && request.EventID != samuraiIntentEventID) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d is not an active Nomad or Samurai event", request.EventID), Localization.New("server.app.event_p_is_not.e64e3c57", "event {p0} is not an active Nomad or Samurai event", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	if score.DifficultyID <= 0 || invasionRemainingSeconds(score, time.Now().UTC()) <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d must have an active difficulty before advisor activation", request.EventID), Localization.New("server.app.event_p_must_have.64ff2a7b", "event {p0} must have an active difficulty before advisor activation", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	if score.AdvisorActive {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the advisor is already active for event %d", request.EventID), Localization.New("server.app.the_advisor_is_already.aa1f7495", "the advisor is already active for event {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	tokenID := score.AdvisorCurrencyID
	if tokenID == 0 {
		tokenID = advisorEventTokenCurrency(request.EventID)
	}
	eventTokens := input.State.Player.Currencies[tokenID]
	universalTokens := input.State.Player.Currencies[advisorUniversalTokenID]
	if !score.AdvisorFree && eventTokens < 1 && universalTokens < 1 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"advisor activation requires one event token (currency %d) or universal advisor token; neither is available",
			tokenID,
		), Localization.New("server.app.advisor_activation_requires_one.1f6da378", "advisor activation requires one event token (currency {p0}) or universal advisor token; neither is available", Localization.Params{"p0": fmt.Sprintf("%d", tokenID)}))
	}
	payload := json.RawMessage(`{"AAT":1}`)
	return Intent.Plan{
		Claims:  []string{"advisor:activation", "account-resources", "event:" + strconv.FormatInt(request.EventID, 10)},
		Summary: fmt.Sprintf("Activate the advisor for event %d with one paid token", request.EventID), SummaryDescriptor: Localization.New("server.app.activate_the_advisor_for.65aff4b4", "Activate the advisor for event {p0} with one paid token", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}),
		Steps: []Intent.Step{commandStep("Consume one advisor token and activate the event advisor", "aa", payload, "aa", Localization.New("server.app.consume_one_advisor_token.10947b5d", "Consume one advisor token and activate the event advisor", nil))},
	}, nil
}

func planAdvisorOverview(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	if _, found := activeAppAdvisorEvent(input.State); !found {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("no active Nomad or Samurai event is available"), Localization.New("server.app.no_active_nomad_or.af38ad15", "no active Nomad or Samurai event is available", nil))
	}
	return Intent.Plan{
		Claims: []string{"advisor:overview"}, Summary: "Refresh advisor run totals", SummaryDescriptor: Localization.New("server.app.refresh_advisor_run_totals.2df528ac", "Refresh advisor run totals", nil),
		Steps: []Intent.Step{commandStep("Refresh advisor overview", "aao", json.RawMessage(`{"AAT":1}`), "aao", Localization.New("server.app.refresh_advisor_overview.ea1be3a0", "Refresh advisor overview", nil))},
	}, nil
}

func planAdvisorAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, _, err := advisorAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is not available", request.CommanderID), Localization.New("server.app.commander_p_is_not.b3a8ca45", "commander {p0} is not available", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
	}
	if _, err := buildAttackSetupForCommanders(
		invasionAttackSetup(request.Preset), source, input.GameData, request.AttackCount,
	); err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("validate advisor preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.validate_advisor_preset_p.af66c224", "validate advisor preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
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
	steps = append(steps, deferredCRACommandStep("Build and launch advisor attack", "advisor.attack.build", arguments, contextPayload, Localization.New("server.app.build_and_launch_advisor.a88b1386", "Build and launch advisor attack", nil)))
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
			"advisor:event:" + strconv.FormatInt(request.EventID, 10),
			"commander:" + strconv.FormatInt(int64(request.CommanderID), 10),
			"leader:commander:" + strconv.FormatInt(int64(request.CommanderID), 10),
		},
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoAdvisor", Affinity: "castle:" + castleID},
		Summary:   fmt.Sprintf("Launch %d advisor attacks against camp %d:%d", request.AttackCount, target.X, target.Y), SummaryDescriptor: Localization.New("server.app.launch_p_advisor_attacks.4d6f1bff", "Launch {p0} advisor attacks against camp {p1}:{p2}", Localization.Params{"p0": request.AttackCount, "p1": target.X, "p2": target.Y}),
		Steps: steps,
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("commander %d is no longer available", request.CommanderID), Localization.New("server.app.commander_p_is_no.546e373b", "commander {p0} is no longer available", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
	}
	dialog := input.State.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID || dialog.Target.TypeID != target.TypeID ||
		dialog.Target.X != target.X || dialog.Target.Y != target.Y || dialog.Target.EventCampID != target.EventCampID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("authoritative ADI row no longer matches advisor camp %d:%d", target.X, target.Y), Localization.New("server.app.authoritative_adi_row_no.be9ba1ce", "authoritative ADI row no longer matches advisor camp {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}))
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve advisor attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_advisor_attack_capacity.9a93a104", "resolve advisor attack capacity", nil), err))
	}
	setup := invasionAttackSetup(AttackPresets.LimitToCapacity(request.Preset, capacity))
	built, err := buildAttackSetupForCommanders(setup, source, input.GameData, request.AttackCount)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build advisor preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_advisor_preset_p.7d363ad7", "build advisor preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	body := advisorAttackBody{
		attackBody:  invasionAttackBody(source, target, request.CommanderID, built),
		AttackCount: request.AttackCount, Mode: 0, AdvisorType: 1,
	}
	if err := applyCastleHorseTravelBoost(&body.attackBody, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve advisor horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_advisor_horse_travel.0c805cee", "resolve advisor horse travel boost", nil), err))
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build advisor CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_advisor_cra_payload.3d231f94", "build advisor CRA payload", nil), err))
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("decode advisor CRA inventory: %w", err), Localization.ErrorContext(Localization.New("server.app.decode_advisor_cra_inventory.378542f0", "decode advisor CRA inventory", nil), err))
	}
	if err := validateRepeatedAttackInventory(fields, source, request.AttackCount); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("advisor inventory preflight: %w", err), Localization.ErrorContext(Localization.New("server.app.advisor_inventory_preflight.4871b06a", "advisor inventory preflight", nil), err))
	}
	step := commandStep(fmt.Sprintf("Launch advisor at %d:%d for %d attacks", target.X, target.Y, request.AttackCount), "cra", payload, "cra", Localization.New("server.app.launch_advisor_at_p.c169358d", "Launch advisor at {p0}:{p1} for {p2, number} attacks", Localization.Params{"p0": target.X, "p1": target.Y, "p2": request.AttackCount}))
	if request.CoinCostPerAttack > 0 && int64(request.AttackCount) > math.MaxInt64/request.CoinCostPerAttack {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("advisor coin budget overflowed"), Localization.New("server.app.advisor_coin_budget_overflowed.baaf304d", "advisor coin budget overflowed", nil))
	}
	step.CoinCost = &Intent.CoinCostRequirement{
		Amount: request.CoinCostPerAttack * int64(request.AttackCount), Reserve: request.MinimumCoinReserve,
		Source: "configured advisor coin budget", UpperBound: true,
	}
	return step, nil
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
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor attackCount must be between 1 and %d", advisorMaxAttackCount), Localization.New("server.app.advisor_attackcount_must_be.76766872", "advisor attackCount must be between 1 and {p0}", Localization.Params{"p0": advisorMaxAttackCount}))
	}
	if request.MinimumRemainingSec < 0 || request.MinimumCoinReserve < 0 || request.RubyCostPerAttack < 0 ||
		request.MinimumRubyReserve < 0 || request.MinimumFeatherReserve < 0 {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor timing and reserves are invalid"), Localization.New("server.app.advisor_timing_and_reserves.30f18dba", "advisor timing and reserves are invalid", nil))
	}
	if request.CoinCostPerAttack <= 0 {
		request.CoinCostPerAttack = advisorDefaultCoinCost
	}
	if advisorUsesRubyHorse(request.HorseTravelBoostID) && request.RubyCostPerAttack <= 0 {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor ruby horse requires a positive rubyCostPerAttack"), Localization.New("server.app.advisor_ruby_horse_requires.fea705e1", "advisor ruby horse requires a positive rubyCostPerAttack", nil))
	}
	for key, reserve := range request.TimeSkipReserve {
		if !advisorTimeSkipKey(key) || reserve < 0 {
			return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("invalid advisor time-skip reserve %q", key), Localization.New("server.app.invalid_advisor_time_skip.5705c16c", "invalid advisor time-skip reserve {p0}", Localization.Params{"p0": fmt.Sprintf("%q", key)}))
		}
	}
	if input.GameData == nil {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	score, active := input.State.LookupScalableEventScore(request.EventID)
	if !active || score.DifficultyID != request.DifficultyID || !score.AdvisorActive {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor is not active for event %d difficulty %d", request.EventID, request.DifficultyID), Localization.New("server.app.advisor_is_not_active.18ae9c8b", "advisor is not active for event {p0} difficulty {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%d", request.DifficultyID)}))
	}
	remaining := invasionRemainingSeconds(score, time.Now().UTC())
	usableSeconds := remaining - request.MinimumRemainingSec
	if remaining <= 0 || usableSeconds < State.AdvisorEstimatedCycleSeconds {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, fmt.Errorf("event has only %d usable seconds remaining", max(int64(0), usableSeconds))
	}
	if maximum := int(usableSeconds / State.AdvisorEstimatedCycleSeconds); request.AttackCount > maximum {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf(
			"%d advisor attacks exceed the event-time capacity of %d", request.AttackCount, maximum,
		), Localization.New("server.app.p_advisor_attacks_exceed.646c75d6", "{p0} advisor attacks exceed the event-time capacity of {p1}", Localization.Params{"p0": request.AttackCount, "p1": maximum}))
	}
	if run := input.State.Advisor.Run; run != nil && advisorRunMatchesScore(*run, score) {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf(
			"event %d already has an advisor run in %s state", request.EventID, run.Status,
		), Localization.New("server.app.event_p_already_has.8e34163f", "event {p0} already has an advisor run in {p1} state", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%s", run.Status)}))
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
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor camp %d:%d is unavailable", request.TargetX, request.TargetY), Localization.New("server.app.advisor_camp_p_p.71aeb5af", "advisor camp {p0}:{p1} is unavailable", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	coins := int64(playerResourceByOfficialKey(input.State, input.GameData, "C1"))
	if request.CoinCostPerAttack > math.MaxInt64/int64(request.AttackCount) {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf("advisor coin budget overflowed"), Localization.New("server.app.advisor_coin_budget_overflowed.baaf304d", "advisor coin budget overflowed", nil))
	}
	requiredCoins := request.CoinCostPerAttack * int64(request.AttackCount)
	if coins-request.MinimumCoinReserve < requiredCoins {
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, &Intent.CoinUnavailableError{
			Required: requiredCoins, Reserve: request.MinimumCoinReserve, Observed: coins, Source: "configured advisor coin budget",
		}
	}
	if advisorUsesRubyHorse(request.HorseTravelBoostID) {
		rubies := int64(playerResourceByOfficialKey(input.State, input.GameData, "C2"))
		requiredRubies := request.RubyCostPerAttack * int64(request.AttackCount)
		if rubies-request.MinimumRubyReserve < requiredRubies {
			return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf(
				"advisor reserves %d rubies and budgets %d per attack; %d attacks need %d with %d observed",
				request.MinimumRubyReserve, request.RubyCostPerAttack, request.AttackCount, requiredRubies, rubies,
			), Localization.New("server.app.advisor_reserves_p_rubies.7a86da3c", "advisor reserves {p0} rubies and budgets {p1} per attack; {p2} attacks need {p3} with {p4} observed", Localization.Params{"p0": request.MinimumRubyReserve, "p1": request.RubyCostPerAttack, "p2": request.AttackCount, "p3": requiredRubies, "p4": rubies}))
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
		return request, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, Localization.WithError(fmt.Errorf(
			"advisor needs %d cooldown skips above reserve for %d attacks", needed, request.AttackCount,
		), Localization.New("server.app.advisor_needs_p_cooldown.6085759a", "advisor needs {p0} cooldown skips above reserve for {p1} attacks", Localization.Params{"p0": needed, "p1": request.AttackCount}))
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
		if score, found := gameState.LookupScalableEventScore(eventID); found && score.RemainingSec > 0 {
			return score, true
		}
	}
	return State.ScalableEventScore{}, false
}

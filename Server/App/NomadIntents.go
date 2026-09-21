package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
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
	SkipCooldowns       bool                 `json:"skipCooldowns"`
	TimeSkipReserve     map[string]int64     `json:"timeSkipReserve,omitempty"`
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

type nomadSequentialArrivalGuardRequest struct {
	EventID      int64           `json:"eventId"`
	KingdomID    State.KingdomID `json:"kingdomId"`
	TargetTypeID int             `json:"targetTypeId"`
	TargetX      int             `json:"targetX"`
	TargetY      int             `json:"targetY"`
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
			fmt.Sprintf("Refresh Nomad/Samurai map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa", Localization.New("server.app.refresh_nomad_samurai_map.7f3ec84e", "Refresh Nomad/Samurai map window {p0, number}/{p1, number}", Localization.Params{"p0": index + 1, "p1": len(windows)}),
		))
	}
	steps = append(steps, Intent.Step{Name: "Record Nomad/Samurai map scan", NameDescriptor: Localization.New("server.app.record_nomad_samurai_map.e1addc5e", "Record Nomad/Samurai map scan", nil), Action: "nomad.scan.capture", ActionArguments: arguments})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: fmt.Sprintf("Refresh the four regular event camps around %s", castleLabel(source)), SummaryDescriptor: Localization.New("server.app.refresh_the_four_regular.c603fbbf", "Refresh the four regular event camps around {p0}", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))}), Steps: steps,
	}, nil
}

func planNomadDifficulty(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request invasionDifficultyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if _, supported := nomadMapTypeForEvent(request.EventID); !supported {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d is not supported by Auto Nomad/Samurai", request.EventID), Localization.New("server.app.event_p_is_not.b7a63435", "event {p0} is not supported by Auto Nomad/Samurai", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	difficulty, valid := input.GameData.ScalableEvent(request.EventID, request.DifficultyID)
	if !valid {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("difficulty %d is not valid for event %d", request.DifficultyID, request.EventID), Localization.New("server.app.difficulty_p_is_not.4a0f6ff3", "difficulty {p0} is not valid for event {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.DifficultyID), "p1": fmt.Sprintf("%d", request.EventID)}))
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !input.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("difficulty %d is not unlocked by this player's achievements", request.DifficultyID), Localization.New("server.app.difficulty_p_is_not.2461af52", "difficulty {p0} is not unlocked by this player's achievements", Localization.Params{"p0": fmt.Sprintf("%d", request.DifficultyID)}))
	}
	score, active := input.State.LookupScalableEventScore(request.EventID)
	if !active {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d is no longer active", request.EventID), Localization.New("server.app.event_p_is_no.8ff46255", "event {p0} is no longer active", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID)}))
	}
	if score.DifficultyID > 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("event %d already selected difficulty %d", request.EventID, score.DifficultyID), Localization.New("server.app.event_p_already_selected.43ceaa5c", "event {p0} already selected difficulty {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%d", score.DifficultyID)}))
	}
	payload, _ := json.Marshal(struct {
		EventID      int64 `json:"EID"`
		DifficultyID int64 `json:"EDID"`
		PremiumUsed  int   `json:"C2U"`
	}{request.EventID, request.DifficultyID, 0})
	return Intent.Plan{
		Claims:  []string{"event-difficulty"},
		Summary: fmt.Sprintf("Start regular-camp event %d at difficulty %d", request.EventID, request.DifficultyID), SummaryDescriptor: Localization.New("server.app.start_regular_camp_event.2e7b446a", "Start regular-camp event {p0} at difficulty {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%d", request.DifficultyID)}),
		Steps: []Intent.Step{
			commandStep("Select Nomad/Samurai event difficulty", "sede", payload, "sede", Localization.New("server.app.select_nomad_samurai_event.e4c385ff", "Select Nomad/Samurai event difficulty", nil)),
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("all four regular camps must be maxed before a target can be locked"), Localization.New("server.app.all_four_regular_camps.216b17ec", "all four regular camps must be maxed before a target can be locked", nil))
		}
	}
	weakest := weakestAppNomadCamp(camps)
	if weakest.Observation.X != request.TargetX || weakest.Observation.Y != request.TargetY ||
		weakest.Observation.EventCampID != request.EventCampID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("camp %d:%d is not the weakest maxed camp in the current four-camp set", request.TargetX, request.TargetY), Localization.New("server.app.camp_p_p_is.61477d10", "camp {p0}:{p1} is not the weakest maxed camp in the current four-camp set", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	request.VictoryCount = weakest.Observation.EventCampVictoryCount
	request.DefenseScore = appNomadDefenseScore(weakest)
	normalized, _ := json.Marshal(request)
	return Intent.Plan{
		Claims:  []string{nomadTargetClaim(request.nomadTargetRequest)},
		Summary: fmt.Sprintf("Lock weakest maxed camp at %d:%d", request.TargetX, request.TargetY), SummaryDescriptor: Localization.New("server.app.lock_weakest_maxed_camp.31f97bb8", "Lock weakest maxed camp at {p0}:{p1}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}),
		Steps: []Intent.Step{{Name: "Lock regular event camp", Action: "nomad.target.capture", ActionArguments: normalized}},
	}, nil
}

func planNomadCampAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, definition, _, err := nomadCampAttackContext(input, arguments)
	if err != nil {
		if errors.Is(err, Intent.ErrPlanStale) {
			return Intent.Plan{Summary: "Nomad/Samurai camp progression changed; reevaluate the current camp state", SummaryDescriptor: Localization.New("server.app.nomad_samurai_camp_progression.6e0c81ac", "Nomad/Samurai camp progression changed; reevaluate the current camp state", nil)}, nil
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
		Holds:        input.CommanderHolds,
		DefaultCount: commanderCount, RequireAvailable: true,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	resolution.Selected = orderNomadChainCommanders(input, source, target, resolution.Selected)
	request.CommanderIDs = append([]State.CommanderID(nil), resolution.Selected...)
	resolvedPresets, err := validateNomadCampPresetInventory(
		input.State, input.GameData, source, target, definition, request.Preset, resolution.Selected,
	)
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("validate capacity-adjusted preset inventory: %w", err), Localization.ErrorContext(Localization.New("server.app.validate_capacity_adjusted_preset.0941f59b", "validate capacity-adjusted preset inventory", nil), err))
	}
	if request.Mode == "chain" {
		if len(resolution.Selected) > 1 && !request.SkipCooldowns {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("chained Nomad/Samurai attacks require cooldown time skips for every committed attack"), Localization.New("server.app.chained_nomad_samurai_attacks.ff0fae7e", "chained Nomad/Samurai attacks require cooldown time skips for every committed attack", nil))
		}
		if request.SkipCooldowns {
			if err := validateNomadChainCooldownSkipCapacity(
				input, definition.CooldownSec, request.TimeSkipReserve, len(resolution.Selected),
			); err != nil {
				return Intent.Plan{}, err
			}
		}
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX         int             `json:"SX"`
		SourceY         int             `json:"SY"`
		TargetX         int             `json:"TX"`
		TargetY         int             `json:"TY"`
		KingdomID       State.KingdomID `json:"KID"`
		TargetTypeID    int             `json:"_citadelTargetTypeId"`
		SequentialGuard json.RawMessage `json:"_citadelNomadSequentialArrivalGuard"`
	}{
		source.X, source.Y, target.X, target.Y, target.KingdomID, target.TypeID,
		mustMarshalNomadSequentialArrivalGuard(request.EventID, target),
	})
	steps := make([]Intent.Step, 0, len(resolution.Selected)*2+8)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() || time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep("Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl"))
	}
	for _, commanderID := range resolution.Selected {
		steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	}
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps, Intent.RebuildOnResume(Intent.Step{
		Name: "Verify chained preset inventory", NameDescriptor: Localization.New("server.app.verify_chained_preset_inventory.3e38c61e", "Verify chained preset inventory", nil), Action: "nomad.attack.inventory.guard",
		ActionArguments: mustMarshalNomadAttackRequest(request),
	}))
	for index, commanderID := range resolution.Selected {
		resolvedRequest := request
		resolvedRequest.Preset = resolvedPresets[commanderID]
		resolvedArguments, _ := json.Marshal(resolvedNomadCampAttackRequest{nomadCampAttackRequest: resolvedRequest, CommanderID: commanderID})
		steps = append(steps, deferredCRACommandStep(
			fmt.Sprintf("Build and launch camp attack with commander %d", commanderID),
			"nomad.attack.build", resolvedArguments, contextPayload, Localization.New("server.app.build_and_launch_camp.055c0e3b", "Build and launch camp attack with commander {p0}", Localization.Params{"p0": fmt.Sprintf("%d", commanderID)}),
		))
		steps = append(steps, Intent.Step{
			Name: "Capture confirmed Nomad/Samurai camp movement", NameDescriptor: Localization.New("server.app.capture_confirmed_nomad_samurai.d5171580", "Capture confirmed Nomad/Samurai camp movement", nil), Action: "nomad.attack.capture",
			ActionArguments: resolvedArguments,
		})
		if request.Mode == "chain" && index > 0 {
			arrivalGuard, _ := json.Marshal(nomadChainArrivalGuard{
				SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
				PreviousCommander: resolution.Selected[index-1], CurrentCommander: commanderID,
			})
			steps = append(steps, Intent.Step{
				Name: "Verify authoritative chain arrival order", NameDescriptor: Localization.New("server.app.verify_authoritative_chain_arrival.5a93dd4f", "Verify authoritative chain arrival order", nil), Action: "nomad.attack.arrival.guard", ActionArguments: arrivalGuard,
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
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.level_camp_p_p.3dc6b270", "Level camp {p0}:{p1} with commander {p2, number}", Localization.Params{"p0": target.X, "p1": target.Y, "p2": resolution.Selected[0]})
	if request.Mode == "chain" {
		summary = fmt.Sprintf("Chain %d attacks into locked camp %d:%d", len(resolution.Selected), target.X, target.Y)
		summaryLocalizationMessage = Localization.New("server.app.chain_p_attacks_into.a4ae8560", "Chain {p0, number} attacks into locked camp {p1}:{p2}", Localization.Params{"p0": len(resolution.Selected), "p1": target.X, "p2": target.Y})
	}
	return Intent.Plan{
		Claims:    claims,
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoNomad", Affinity: "castle:" + castleID},
		Summary:   summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage), Steps: steps,
	}, nil
}

func validateNomadChainCooldownSkipCapacity(
	input Intent.PlanningContext,
	cooldownSec int64,
	reserves map[string]int64,
	commitmentCount int,
) error {
	if commitmentCount <= 0 {
		return nil
	}
	if cooldownSec <= 0 {
		return Localization.WithError(fmt.Errorf("official camp cooldown duration is unavailable for a chained attack"), Localization.New("server.app.official_camp_cooldown_duration.9a3820fe", "official camp cooldown duration is unavailable for a chained attack", nil))
	}
	minutes := []int{1, 5, 10, 30, 60, 300, 1440}
	options := make([]buildingTimeSkipOption, 0, len(minutes))
	available := map[State.CurrencyID]int64{}
	for _, minute := range minutes {
		option, err := officialBuildingTimeSkipOption(input.GameData, minute)
		if err != nil {
			return Localization.WithError(fmt.Errorf("validate official cooldown time skip: %w", err), Localization.ErrorContext(Localization.New("server.app.validate_official_cooldown_time.0c1b81f8", "validate official cooldown time skip", nil), err))
		}
		reserve := timeSkipReserve(reserves, option.WireKey)
		if reserve < 0 {
			return Localization.WithError(fmt.Errorf("%s time-skip reserve cannot be negative", option.WireKey), Localization.New("server.app.p_time_skip_reserve.62d66da1", "{p0} time-skip reserve cannot be negative", Localization.Params{"p0": fmt.Sprintf("%s", option.WireKey)}))
		}
		balance := input.State.Player.Currencies[option.CurrencyID]
		available[option.CurrencyID] = max(int64(0), int64(math.Floor(balance))-reserve)
		options = append(options, option)
	}

	for commitment := 0; commitment < commitmentCount; commitment++ {
		remaining := cooldownSec
		for remaining > 0 {
			selected := -1
			for index, option := range options {
				if available[option.CurrencyID] > 0 && int64(option.Minutes)*60 >= remaining {
					selected = index
					break
				}
			}
			if selected < 0 {
				for index := len(options) - 1; index >= 0; index-- {
					if available[options[index].CurrencyID] > 0 {
						selected = index
						break
					}
				}
			}
			if selected < 0 {
				return Localization.WithError(fmt.Errorf(
					"available time skips cannot cover committed attack %d of %d while preserving configured reserves",
					commitment+1, commitmentCount,
				), Localization.New("server.app.available_time_skips_cannot.59dbc203", "available time skips cannot cover committed attack {p0} of {p1} while preserving configured reserves", Localization.Params{"p0": commitment + 1, "p1": commitmentCount}))
			}
			option := options[selected]
			available[option.CurrencyID]--
			remaining -= int64(option.Minutes) * 60
		}
	}
	return nil
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
	reset := commandStep("Reset locked camp cooldown", "sdc", payload, "sdc", Localization.New("server.app.reset_locked_camp_cooldown.46270e85", "Reset locked camp cooldown", nil))
	reset.FinalDispatchAction = "nomad.cooldown.guard"
	reset.FinalDispatchArguments = append(json.RawMessage(nil), arguments...)
	return Intent.Plan{
		Claims:  []string{nomadTargetClaim(request.nomadTargetRequest), "account-resources"},
		Summary: fmt.Sprintf("Reset %d-second cooldown on locked camp %d:%d for at most %d rubies", remaining, request.TargetX, request.TargetY, definition.SkipCost), SummaryDescriptor: Localization.New("server.app.reset_p_second_cooldown.8f9ac01b", "Reset {p0}-second cooldown on locked camp {p1}:{p2} for at most {p3} rubies", Localization.Params{"p0": remaining, "p1": request.TargetX, "p2": request.TargetY, "p3": definition.SkipCost}),
		Steps: []Intent.Step{
			{Name: "Verify locked camp cooldown and ruby reserve", Action: "nomad.cooldown.guard", ActionArguments: arguments},
			reset,
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
		return nomadMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("Nomad/Samurai map scan requires a source castle and radius between 1 and %d", nomadIntentRadius), Localization.New("server.app.nomad_samurai_map_scan.93f32885", "Nomad/Samurai map scan requires a source castle and radius between 1 and {p0}", Localization.Params{"p0": nomadIntentRadius}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return nomadMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("source castle %d is unavailable", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.984a66b1", "source castle {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if source.KingdomID != 0 {
		return nomadMapScanRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("Nomad/Samurai source castle must be in the Great Empire"), Localization.New("server.app.nomad_samurai_source_castle.1cf33ea1", "Nomad/Samurai source castle must be in the Great Empire", nil))
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
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("camp attack mode must be level or chain"), Localization.New("server.app.camp_attack_mode_must.3eb54bd9", "camp attack mode must be level or chain", nil))
	}
	if request.ScoreTarget <= 0 || request.MinimumRemainingSec < 0 || len(request.CommanderIDs) == 0 {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("camp attack requires a score target and at least one commander"), Localization.New("server.app.camp_attack_requires_a.2bdbf45a", "camp attack requires a score target and at least one commander", nil))
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
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf(
			"%w: camp %d:%d changed progression state", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.6809d03f", "intent plan became stale before dispatch: camp {p1}:{p2} changed progression state", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	key := fmt.Sprintf("%d:%d:%d", selected.Observation.KingdomID, selected.Observation.X, selected.Observation.Y)
	if cooldown, found := input.State.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf(
			"%w: camp %d:%d is awaiting an authoritative cooldown refresh", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.d54cd184", "intent plan became stale before dispatch: camp {p1}:{p2} is awaiting an authoritative cooldown refresh", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if nomadAppCooldownRemaining(input.State, selected.Observation, time.Now().UTC()) > 0 {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("%w: camp %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.f2baea1d", "intent plan became stale before dispatch: camp {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if request.Mode == "level" {
		if selected.Observation.EventCampVictoryCount >= maximumVictoryCount {
			return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("level mode cannot attack an already maxed camp"), Localization.New("server.app.level_mode_cannot_attack.35f43259", "level mode cannot attack an already maxed camp", nil))
		}
	} else {
		for _, camp := range camps {
			if camp.Observation.EventCampVictoryCount < maximumVictoryCount {
				return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("chain mode requires all four camps to be maxed"), Localization.New("server.app.chain_mode_requires_all.16c369c1", "chain mode requires all four camps to be maxed", nil))
			}
		}
		if !nomadLockMatches(input.State.NomadCamps.LockedTarget, request.nomadTargetRequest) {
			return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("chain mode target does not match the locked camp"), Localization.New("server.app.chain_mode_target_does.60944d3e", "chain mode target does not match the locked camp", nil))
		}
	}
	score, active := input.State.LookupScalableEventScore(request.EventID)
	if !active || score.DifficultyID != request.DifficultyID {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("event %d difficulty %d is no longer active", request.EventID, request.DifficultyID), Localization.New("server.app.event_p_difficulty_p.3d00c6cd", "event {p0} difficulty {p1} is no longer active", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%d", request.DifficultyID)}))
	}
	if score.PlayerScore >= request.ScoreTarget {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("event score target reached: %d / %d", score.PlayerScore, request.ScoreTarget), Localization.New("server.app.event_score_target_reached.f3265d79", "event score target reached: {p0} / {p1}", Localization.Params{"p0": score.PlayerScore, "p1": request.ScoreTarget}))
	}
	if remaining := invasionRemainingSeconds(score, time.Now().UTC()); remaining >= 0 && remaining <= request.MinimumRemainingSec {
		return nomadCampAttackRequest{}, State.CastleState{}, State.MapObservation{}, GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("event has only %d seconds remaining", remaining), Localization.New("server.app.event_has_only_p.adec6e47", "event has only {p0} seconds remaining", Localization.Params{"p0": remaining}))
	}
	return request, source, selected.Observation, selected.Definition, maximumVictoryCount, nil
}

func validatedNomadCampSet(
	input Intent.PlanningContext,
	request nomadTargetRequest,
) (State.CastleState, []appNomadCamp, int64, error) {
	if request.SourceCastleID <= 0 || request.EventID <= 0 || request.DifficultyID <= 0 || request.TargetTypeID <= 0 || request.EventCampID <= 0 {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("regular camp target requires source, event, difficulty, type, and scaling-camp id"), Localization.New("server.app.regular_camp_target_requires.52348ac4", "regular camp target requires source, event, difficulty, type, and scaling-camp id", nil))
	}
	expectedTypeID, supported := nomadMapTypeForEvent(request.EventID)
	if !supported || expectedTypeID != request.TargetTypeID {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("event %d does not use regular camp type %d", request.EventID, request.TargetTypeID), Localization.New("server.app.event_p_does_not.982753dd", "event {p0} does not use regular camp type {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.EventID), "p1": fmt.Sprintf("%d", request.TargetTypeID)}))
	}
	if input.GameData == nil {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 || request.KingdomID != source.KingdomID {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("regular camp source and target must be in the Great Empire"), Localization.New("server.app.regular_camp_source_and.43351df4", "regular camp source and target must be in the Great Empire", nil))
	}
	progression := input.GameData.EventCampProgression(request.EventID, request.DifficultyID, request.TargetTypeID)
	if len(progression) == 0 {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("official regular-camp progression is unavailable"), Localization.New("server.app.official_regular_camp_progression.e094ecd5", "official regular-camp progression is unavailable", nil))
	}
	maximumVictoryCount := progression[len(progression)-1].VictoryCount
	lastScan := input.State.NomadCamps.LastScannedAt[source.ID]
	if lastScan.IsZero() {
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("the four regular camps have not been scanned"), Localization.New("server.app.the_four_regular_camps.967219de", "the four regular camps have not been scanned", nil))
	}
	camps := make([]appNomadCamp, 0, nomadIntentCampCount)
	input.State.RangeMapObservationsByKind(source.KingdomID, State.MapProjectionEventCamp, func(_ string, observation State.MapObservation) bool {
		if observation.TypeID != request.TargetTypeID || observation.EventCampID <= 0 || observation.ObservedAt.Before(lastScan) ||
			appNomadDistanceSquared(source, observation) > nomadIntentRadius*nomadIntentRadius {
			return true
		}
		definition, found := input.GameData.EventCamp(observation.EventCampID)
		if !found || definition.EventID != request.EventID || definition.DifficultyID != request.DifficultyID ||
			definition.AreaTypeID != request.TargetTypeID || definition.VictoryCount != observation.EventCampVictoryCount {
			return true
		}
		camps = append(camps, appNomadCamp{Observation: observation, Definition: definition})
		return true
	})
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
		return State.CastleState{}, nil, 0, Localization.WithError(fmt.Errorf("found %d of the required %d regular camps", len(camps), nomadIntentCampCount), Localization.New("server.app.found_p_of_the.8517f0b3", "found {p0} of the required {p1} regular camps", Localization.Params{"p0": len(camps), "p1": nomadIntentCampCount}))
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
		return Intent.Step{}, Localization.WithError(fmt.Errorf("commander %d is no longer available", request.CommanderID), Localization.New("server.app.commander_p_is_no.546e373b", "commander {p0} is no longer available", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
	}
	limitedPreset, err := capacityLimitedNomadCampPreset(
		input.State, input.GameData, source, target, definition, request.Preset, request.CommanderID, true,
	)
	if err != nil {
		return Intent.Step{}, err
	}
	setup := invasionAttackSetup(limitedPreset)
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build camp preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_camp_preset_p.792dbb97", "build camp preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve camp horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_camp_horse_travel.cbe6226c", "resolve camp horse travel boost", nil), err))
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build camp CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_camp_cra_payload.22a17f7e", "build camp CRA payload", nil), err))
	}
	step := commandStep(fmt.Sprintf("Attack locked camp at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_locked_camp_at.6f79cc4d", "Attack locked camp at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}))
	step.PreDispatchAction = "nomad.attack.guard"
	step.PreDispatchArguments = append(json.RawMessage(nil), arguments...)
	step.FinalDispatchAction = "nomad.attack.guard"
	step.FinalDispatchArguments = append(json.RawMessage(nil), arguments...)
	return step, nil
}

func (application *Application) captureNomadCampLaunch(_ context.Context, arguments json.RawMessage) error {
	var request resolvedNomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentEventScores), func(gameState *State.GameState) ([]string, bool, error) {
		var selected State.MovementState
		gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
			if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID ||
				movement.KingdomID != request.KingdomID || movement.TargetX != request.TargetX || movement.TargetY != request.TargetY ||
				movement.CommanderID == nil || *movement.CommanderID != request.CommanderID || movement.ArrivesAt == nil {
				return true
			}
			if selected.ID == 0 || movement.ObservedAt.After(selected.ObservedAt) ||
				movement.ObservedAt.Equal(selected.ObservedAt) && movement.ID > selected.ID {
				selected = movement
			}
			return true
		})
		if selected.ID == 0 {
			return nil, false, Localization.WithError(fmt.Errorf("CRA response did not return commander %d's Nomad/Samurai movement", request.CommanderID), Localization.New("server.app.cra_response_did_not.b21109db", "CRA response did not return commander {p0}'s Nomad/Samurai movement", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
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
	request, _, err := nomadMapScanContext(Intent.PlanningContext{State: application.State.ReadOnlyView()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentNomadCamps), func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, Localization.WithError(fmt.Errorf("source castle %d is no longer focused", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.b7a54840", "source castle {p0} is no longer focused", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
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
	_, err := application.State.ApplyComponents(State.Components(
		State.ComponentNomadCamps, State.ComponentEventScores,
	), func(gameState *State.GameState) ([]string, bool, error) {
		changed := gameState.NomadCamps.LockedTarget != nil || len(gameState.NomadCamps.Cooldowns) > 0 || len(gameState.NomadCamps.LastScannedAt) > 0
		gameState.NomadCamps.LockedTarget = nil
		gameState.NomadCamps.Cooldowns = map[string]State.NomadCampCooldownState{}
		gameState.NomadCamps.LastScannedAt = map[State.CastleID]time.Time{}
		if score, found := gameState.LookupScalableEventScore(request.EventID); found && score.DifficultyID != request.DifficultyID {
			score.DifficultyID = request.DifficultyID
			gameState.SetScalableEventScore(request.EventID, score)
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
	_, err := application.State.ApplyComponents(State.Components(State.ComponentNomadCamps), func(gameState *State.GameState) ([]string, bool, error) {
		observation, exists := gameState.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
		if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID ||
			observation.EventCampVictoryCount != request.VictoryCount {
			return nil, false, Localization.WithError(fmt.Errorf("camp %d:%d changed before it could be locked", request.TargetX, request.TargetY), Localization.New("server.app.camp_p_p_changed.a09f3f27", "camp {p0}:{p1} changed before it could be locked", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
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
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("game state is unavailable"), Localization.New("server.app.game_state_is_unavailable.cfae30c6", "game state is unavailable", nil))
	}
	state := application.State.ReadOnlyView()
	if err := guardDailyAttackLimitAtDispatch(state, request.DailyAttackLimit); err != nil {
		return err
	}
	currentData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, source, target, _, _, err := nomadCampAttackContext(
		Intent.PlanningContext{State: state, GameData: currentData}, mustMarshalNomadAttackRequest(request.nomadCampAttackRequest),
	)
	if err != nil {
		return err
	}
	if err := guardNomadSequentialArrivalAt(state, mustMarshalNomadSequentialArrivalGuard(request.EventID, target), time.Now().UTC()); err != nil {
		return err
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return Localization.WithError(fmt.Errorf("commander %d is no longer available", request.CommanderID), Localization.New("server.app.commander_p_is_no.546e373b", "commander {p0} is no longer available", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID || dialog.Target.TypeID != target.TypeID ||
		dialog.Target.X != target.X || dialog.Target.Y != target.Y || dialog.Target.EventCampID != target.EventCampID ||
		dialog.Target.EventCampVictoryCount != target.EventCampVictoryCount {
		return Localization.WithError(fmt.Errorf("authoritative ADI row no longer matches ready camp %d:%d", target.X, target.Y), Localization.New("server.app.authoritative_adi_row_no.c0bfc60a", "authoritative ADI row no longer matches ready camp {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y}))
	}
	if dialog.Target.EventCampCooldownRemaining > 0 {
		return Localization.WithError(fmt.Errorf("%w: authoritative ADI row shows camp %d:%d on cooldown", Intent.ErrPlanStale, target.X, target.Y), Localization.New("server.app.intent_plan_became_stale.3dde6992", "intent plan became stale before dispatch: authoritative ADI row shows camp {p1}:{p2} on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", target.X), "p2": fmt.Sprintf("%d", target.Y)}))
	}
	return nil
}

func mustMarshalNomadSequentialArrivalGuard(eventID int64, target State.MapObservation) json.RawMessage {
	arguments, _ := json.Marshal(nomadSequentialArrivalGuardRequest{
		EventID: eventID, KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
	})
	return arguments
}

func (application *Application) guardNomadSequentialArrival(_ context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("game state is unavailable"), Localization.New("server.app.game_state_is_unavailable.cfae30c6", "game state is unavailable", nil))
	}
	return guardNomadSequentialArrivalAt(application.State.ReadOnlyView(), arguments, time.Now().UTC())
}

func guardNomadSequentialArrivalAt(gameState State.GameState, arguments json.RawMessage, now time.Time) error {
	var request nomadSequentialArrivalGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.TargetTypeID != nomadIntentCampTypeID && request.TargetTypeID != samuraiIntentCampTypeID {
		return Localization.WithError(fmt.Errorf("Nomad sequential-arrival guard requires a Nomad or Samurai target type"), Localization.New("server.app.nomad_sequential_arrival_guard.b32ba1dd", "Nomad sequential-arrival guard requires a Nomad or Samurai target type", nil))
	}
	key := fmt.Sprintf("%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	if cooldown, found := gameState.NomadCamps.Cooldowns[key]; found && cooldown.PendingCooldownRefresh {
		return Localization.WithError(fmt.Errorf(
			"%w: camp %d:%d is awaiting an authoritative cooldown refresh",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.d54cd184", "intent plan became stale before dispatch: camp {p1}:{p2} is awaiting an authoritative cooldown refresh", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if target, found := gameState.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)); found && target.TypeID == request.TargetTypeID && nomadAppCooldownRemaining(gameState, target, now) > 0 {
		return Localization.WithError(fmt.Errorf("%w: camp %d:%d is on cooldown", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.f2baea1d", "intent plan became stale before dispatch: camp {p1}:{p2} is on cooldown", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if block, found := State.NomadSequentialArrivalBlockAt(
		gameState, request.EventID, request.KingdomID, request.TargetTypeID, request.TargetX, request.TargetY, now,
	); found {
		if block.Unknown {
			return Localization.WithError(fmt.Errorf(
				"%w: camp %d:%d has an earlier Auto Nomad/Samurai launch with unknown arrival timing",
				Intent.ErrPlanStale, request.TargetX, request.TargetY,
			), Localization.New("server.app.intent_plan_became_stale.8bde4bef", "intent plan became stale before dispatch: camp {p1}:{p2} has an earlier Auto Nomad/Samurai launch with unknown arrival timing", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
		}
		return fmt.Errorf(
			"%w: camp %d:%d has an earlier Auto Nomad/Samurai arrival at %s awaiting settlement",
			Intent.ErrPlanStale, request.TargetX, request.TargetY, block.ArrivesAt.Format(time.RFC3339Nano),
		)
	}
	return nil
}

func (application *Application) guardNomadAttackInventory(_ context.Context, arguments json.RawMessage) error {
	var request nomadCampAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	currentData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, source, target, definition, _, err := nomadCampAttackContext(
		Intent.PlanningContext{State: state, GameData: currentData}, arguments,
	)
	if err != nil {
		return err
	}
	_, err = validateNomadCampPresetInventory(
		state, currentData, source, target, definition, request.Preset, request.CommanderIDs,
	)
	return err
}

func validateNomadCampPresetInventory(
	gameState State.GameState,
	gameData *GameData.Store,
	source State.CastleState,
	target State.MapObservation,
	definition GameData.EventCampDefinition,
	preset AttackPresets.Preset,
	commanderIDs []State.CommanderID,
) (map[State.CommanderID]AttackPresets.Preset, error) {
	if len(commanderIDs) == 0 {
		return nil, Localization.WithError(fmt.Errorf("camp attack requires at least one commander"), Localization.New("server.app.camp_attack_requires_at.b88490e1", "camp attack requires at least one commander", nil))
	}
	remaining := make(map[State.UnitID]int64, len(source.Units.Stationed))
	for itemID, amount := range source.Units.Stationed {
		remaining[itemID] = amount
	}
	resolvedPresets := make(map[State.CommanderID]AttackPresets.Preset, len(commanderIDs))
	useAttackDialog := matchingNomadAttackDialog(gameState, source, target)
	targetLabel := "Nomad camps"
	if definition.EventID == samuraiIntentEventID {
		targetLabel = "Samurai camps"
	}
	for _, commanderID := range commanderIDs {
		limited, err := capacityLimitedNomadCampPreset(
			gameState, gameData, source, target, definition, preset, commanderID, useAttackDialog,
		)
		if err != nil {
			return nil, err
		}
		if err := AttackPresets.ValidateToolCompatibility(limited, gameData, AttackPresets.ToolTarget{
			KingdomID: int64(target.KingdomID), TypeID: target.TypeID,
			EventID: definition.EventID, Label: targetLabel,
		}); err != nil {
			return nil, err
		}
		resolved, shortage, err := AttackPresets.CheckInventory(limited, remaining, gameData, 1)
		if err != nil {
			return nil, Localization.WithError(fmt.Errorf("resolve camp preset %q troop families for commander %d: %w", preset.Name, commanderID, err), Localization.ErrorContext(Localization.New("server.app.resolve_camp_preset_p.9c683cfe", "resolve camp preset {p0} troop families for commander {p1}", Localization.Params{"p0": fmt.Sprintf("%q", preset.Name), "p1": fmt.Sprintf("%d", commanderID)}), err))
		}
		if shortage != nil {
			return nil, Localization.WithError(fmt.Errorf(
				"castle %d has %d of item %d; %d commander(s) require %d after camp capacity limits",
				source.ID, shortage.Available, shortage.ItemID, len(commanderIDs), shortage.Required,
			), Localization.New("server.app.castle_p_has_p.13db3365", "castle {p0} has {p1} of item {p2}; {p3} commander(s) require {p4} after camp capacity limits", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": shortage.Available, "p2": fmt.Sprintf("%d", shortage.ItemID), "p3": fmt.Sprintf("%d", len(commanderIDs)), "p4": shortage.Required}))
		}
		if _, err := buildAttackSetup(invasionAttackSetup(resolved), source, gameData); err != nil {
			return nil, Localization.WithError(fmt.Errorf("build camp preset %q for commander %d: %w", preset.Name, commanderID, err), Localization.ErrorContext(Localization.New("server.app.build_camp_preset_p.bc414d5d", "build camp preset {p0} for commander {p1}", Localization.Params{"p0": fmt.Sprintf("%q", preset.Name), "p1": fmt.Sprintf("%d", commanderID)}), err))
		}
		required, _ := AttackPresets.Requirements(resolved)
		for itemID, amount := range required {
			remaining[itemID] = max(int64(0), remaining[itemID]-amount)
		}
		resolvedPresets[commanderID] = resolved
	}
	return resolvedPresets, nil
}

func capacityLimitedNomadCampPreset(
	gameState State.GameState,
	gameData *GameData.Store,
	source State.CastleState,
	target State.MapObservation,
	definition GameData.EventCampDefinition,
	preset AttackPresets.Preset,
	commanderID State.CommanderID,
	useAttackDialog bool,
) (AttackPresets.Preset, error) {
	capacity, err := (AttackCapacity.Resolver{}).Resolve(gameState, gameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: useAttackDialog,
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
		return AttackPresets.Preset{}, Localization.WithError(fmt.Errorf("resolve Nomad/Samurai camp attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_nomad_samurai_camp.45e1ec2d", "resolve Nomad/Samurai camp attack capacity", nil), err))
	}
	return AttackPresets.LimitToCapacity(preset, capacity), nil
}

func matchingNomadAttackDialog(gameState State.GameState, source State.CastleState, target State.MapObservation) bool {
	dialog := gameState.AttackDialog
	return dialog.SourceCastleID == source.ID && dialog.KingdomID == target.KingdomID &&
		dialog.Target.TypeID == target.TypeID && dialog.Target.X == target.X && dialog.Target.Y == target.Y &&
		dialog.Target.EventCampID == target.EventCampID &&
		dialog.Target.EventCampVictoryCount == target.EventCampVictoryCount
}

func (application *Application) guardNomadChainArrival(_ context.Context, arguments json.RawMessage) error {
	var request nomadChainArrivalGuard
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || request.PreviousCommander < 0 || request.CurrentCommander < 0 ||
		request.PreviousCommander == request.CurrentCommander {
		return Localization.WithError(fmt.Errorf("invalid Nomad chain arrival guard"), Localization.New("server.app.invalid_nomad_chain_arrival.c367f255", "invalid Nomad chain arrival guard", nil))
	}
	gameState := application.State.ReadOnlyView()
	previous, previousFound := latestNomadChainMovement(gameState, request, request.PreviousCommander)
	current, currentFound := latestNomadChainMovement(gameState, request, request.CurrentCommander)
	if !previousFound || !currentFound {
		return Localization.WithError(fmt.Errorf(
			"server did not return both chained movements for commanders %d and %d",
			request.PreviousCommander, request.CurrentCommander,
		), Localization.New("server.app.server_did_not_return.701f722f", "server did not return both chained movements for commanders {p0} and {p1}", Localization.Params{"p0": request.PreviousCommander, "p1": request.CurrentCommander}))
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
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.Direction != 0 || movement.SourceCastleID != request.SourceCastleID || movement.KingdomID != request.KingdomID ||
			movement.TargetX != request.TargetX || movement.TargetY != request.TargetY || movement.CommanderID == nil ||
			*movement.CommanderID != commanderID || movement.ArrivesAt == nil || movement.ArrivesAt.IsZero() {
			return true
		}
		if !found || movement.ArrivesAt.After(*selected.ArrivesAt) {
			selected, found = movement, true
		}
		return true
	})
	return selected, found
}

func (application *Application) guardNomadCooldownSkip(_ context.Context, arguments json.RawMessage) error {
	var request nomadCooldownSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	currentData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	_, _, err := validateNomadCooldownSkip(application.State.ReadOnlyView(), currentData, request, time.Now().UTC())
	return err
}

func (application *Application) verifyNomadCooldownSkip(_ context.Context, arguments json.RawMessage) error {
	var request nomadCooldownVerification
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	observation, exists := state.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID ||
		observation.ObservedAt.Before(request.ResetStartedAt) || observation.EventCampCooldownRemaining != 0 {
		return Localization.WithError(fmt.Errorf("cooldown reset did not return an authoritative zero-cooldown row for camp %d:%d", request.TargetX, request.TargetY), Localization.New("server.app.cooldown_reset_did_not.b57c86d6", "cooldown reset did not return an authoritative zero-cooldown row for camp {p0}:{p1}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
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
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("cooldown reset requires a positive ruby cap and non-negative reserve"), Localization.New("server.app.cooldown_reset_requires_a.6ce3db7a", "cooldown reset requires a positive ruby cap and non-negative reserve", nil))
	}
	if !nomadLockMatches(gameState.NomadCamps.LockedTarget, request.nomadTargetRequest) {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("cooldown reset target does not match the locked camp"), Localization.New("server.app.cooldown_reset_target_does.93d7717d", "cooldown reset target does not match the locked camp", nil))
	}
	observation, exists := gameState.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || observation.TypeID != request.TargetTypeID || observation.EventCampID != request.EventCampID {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("%w: locked camp %d:%d is unavailable", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.70912fa3", "intent plan became stale before dispatch: locked camp {p1}:{p2} is unavailable", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	key := fmt.Sprintf("%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY)
	if cooldown, found := gameState.NomadCamps.Cooldowns[key]; found &&
		(cooldown.PendingCooldownRefresh || cooldown.LastSuccessfulBattleAt.After(observation.ObservedAt)) {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf(
			"%w: locked camp %d:%d is awaiting a fresh post-victory cooldown row",
			Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.c43da7d2", "intent plan became stale before dispatch: locked camp {p1}:{p2} is awaiting a fresh post-victory cooldown row", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	definition, found := gameData.EventCamp(request.EventCampID)
	if !found || definition.EventID != request.EventID || definition.DifficultyID != request.DifficultyID ||
		definition.AreaTypeID != request.TargetTypeID || definition.CooldownSec <= 0 || definition.SkipCost <= 0 {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("locked camp has no official skippable cooldown definition"), Localization.New("server.app.locked_camp_has_no.d72d06d7", "locked camp has no official skippable cooldown definition", nil))
	}
	remaining := nomadAppCooldownRemaining(gameState, observation, now)
	if remaining <= 0 {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("%w: locked camp is no longer on cooldown", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.4c809ffd", "intent plan became stale before dispatch: locked camp is no longer on cooldown", nil))
	}
	if definition.SkipCost > request.MaximumRubyCost {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf("official reset may cost %d rubies, above configured cap %d", definition.SkipCost, request.MaximumRubyCost), Localization.New("server.app.official_reset_may_cost.df975cfa", "official reset may cost {p0} rubies, above configured cap {p1}", Localization.Params{"p0": definition.SkipCost, "p1": request.MaximumRubyCost}))
	}
	rubies := playerResourceByOfficialKey(gameState, gameData, "C2")
	if rubies-float64(request.MinimumRubyReserve) < float64(definition.SkipCost) {
		return GameData.EventCampDefinition{}, 0, Localization.WithError(fmt.Errorf(
			"cooldown reset reserves %d rubies and may cost %d; %.0f are observed",
			request.MinimumRubyReserve, definition.SkipCost, rubies,
		), Localization.New("server.app.cooldown_reset_reserves_p.48e97ef3", "cooldown reset reserves {p0} rubies and may cost {p1}; {p2} are observed", Localization.Params{"p0": request.MinimumRubyReserve, "p1": definition.SkipCost, "p2": rubies}))
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

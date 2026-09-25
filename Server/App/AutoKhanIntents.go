package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
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
	KhanDomain "CitadelDesktop/Server/Khan"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const (
	khanEventID                    = 72
	khanCampTypeID                 = 35
	khanMapNPCID                   = -801
	khanTauntResponseTimeoutMillis = 10_000
)

type khanAttackRequest struct {
	RunID                    string                   `json:"runId"`
	EventEndsAt              time.Time                `json:"eventEndsAt"`
	SourceCastleID           State.CastleID           `json:"sourceCastleId"`
	MainCastleID             State.CastleID           `json:"mainCastleId"`
	KingdomID                State.KingdomID          `json:"kingdomId"`
	TargetX                  int                      `json:"targetX"`
	TargetY                  int                      `json:"targetY"`
	Preset                   AttackPresets.Preset     `json:"preset"`
	CommanderID              State.CommanderID        `json:"commanderId"`
	HorseTravelBoostID       int                      `json:"horseTravelBoostId"`
	DailyAttackLimit         int64                    `json:"dailyAttackLimit"`
	NomadPointThreshold      int64                    `json:"nomadPointThreshold"`
	MaxRageChain             int64                    `json:"maxRageChain"`
	RequireActiveRageBooster bool                     `json:"requireActiveRageBooster"`
	DefensePreset            KhanDomain.DefensePreset `json:"defensePreset"`
	OpenGateProtection       bool                     `json:"openGateProtection"`
	OffensiveUnitThreshold   int64                    `json:"offensiveUnitThreshold"`
}

type khanLaneGuardRequest struct {
	MainCastleID           State.CastleID           `json:"mainCastleId"`
	DefensePreset          KhanDomain.DefensePreset `json:"defensePreset"`
	OpenGateProtection     bool                     `json:"openGateProtection"`
	OffensiveUnitThreshold int64                    `json:"offensiveUnitThreshold"`
	NomadPointThreshold    int64                    `json:"nomadPointThreshold"`
}

type khanLaneGuardActionRequest struct {
	KhanGuard khanLaneGuardRequest `json:"khanGuard"`
}

type khanTauntRequest struct {
	EventID          int64                `json:"eventId"`
	EventEndsAt      time.Time            `json:"eventEndsAt"`
	MainCastleID     State.CastleID       `json:"mainCastleId"`
	TargetX          int                  `json:"targetX"`
	TargetY          int                  `json:"targetY"`
	RageCampID       int64                `json:"rageCampId"`
	RageCampRevision uint64               `json:"rageCampRevision"`
	PlayerRageCap    int64                `json:"playerRageCap"`
	PlayerTotalRage  int64                `json:"playerTotalRage"`
	RageObservedAt   time.Time            `json:"rageObservedAt"`
	KhanGuard        khanLaneGuardRequest `json:"khanGuard"`
}

type khanLaunchCapture struct {
	RunID          string            `json:"runId"`
	EventEndsAt    time.Time         `json:"eventEndsAt"`
	SourceCastleID State.CastleID    `json:"sourceCastleId"`
	MainCastleID   State.CastleID    `json:"mainCastleId"`
	KingdomID      State.KingdomID   `json:"kingdomId"`
	TargetX        int               `json:"targetX"`
	TargetY        int               `json:"targetY"`
	CommanderID    State.CommanderID `json:"commanderId"`
}

type khanProtectionRequest struct {
	RunID                  string                   `json:"runId"`
	CastleID               State.CastleID           `json:"castleId"`
	DefensePreset          KhanDomain.DefensePreset `json:"defensePreset"`
	OffensiveUnitThreshold int64                    `json:"offensiveUnitThreshold"`
}

type khanPointLimitRequest struct {
	CastleID       State.CastleID `json:"castleId"`
	PointThreshold int64          `json:"pointThreshold"`
}

type khanDefenseToolPurchaseRequest struct {
	CastleID      State.CastleID           `json:"castleId"`
	PackageID     State.PackageID          `json:"packageId"`
	ToolID        State.UnitID             `json:"toolId"`
	Amount        int64                    `json:"amount"`
	ShopTableID   int64                    `json:"shopTableId"`
	DefensePreset KhanDomain.DefensePreset `json:"defensePreset"`
}

func planKhanMapJump(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	score, active := input.State.LookupScalableEventScore(khanEventID)
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the Nomad event and Khan camp are not active"), Localization.New("server.app.the_nomad_event_and.01b4f683", "the Nomad event and Khan camp are not active", nil))
	}
	payload, _ := json.Marshal(struct {
		TargetTypeID int             `json:"T"`
		KingdomID    State.KingdomID `json:"KID"`
		MinimumLevel int             `json:"LMIN"`
		MaximumLevel int             `json:"LMAX"`
		NPCID        int             `json:"NID"`
	}{
		TargetTypeID: khanCampTypeID, KingdomID: 0,
		MinimumLevel: -1, MaximumLevel: -1, NPCID: khanMapNPCID,
	})
	return Intent.Plan{
		Claims:  []string{"castle-focus", "map:0"},
		Summary: "Jump world map to the active Khan camp", SummaryDescriptor: Localization.New("server.app.jump_world_map_to.fefec393", "Jump world map to the active Khan camp", nil),
		Steps: []Intent.Step{commandStep("Jump to Khan camp", "fnm", payload, "fnm", Localization.New("server.app.jump_to_khan_camp.48220631", "Jump to Khan camp", nil))},
	}, nil
}

func planKhanTaunt(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request khanTauntRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if err := validateKhanTauntContext(input.State, input.GameData, request, time.Now().UTC()); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		// The taunt only asks the event for a retaliation, so it claims its own
		// lane instead of the whole main castle. A castle-wide claim would queue
		// the full-rage window behind every chain attack launched from it.
		Claims:  []string{"khan-lane:taunt"},
		Summary: "Trigger the full-rage Khan retaliation", SummaryDescriptor: Localization.New("server.app.trigger_the_full_rage.3832322d", "Trigger the full-rage Khan retaliation", nil),
		Steps: []Intent.Step{
			{
				Name: "Revalidate and dispatch Khan retaliation", NameDescriptor: Localization.New("server.app.revalidate_and_dispatch_khan.1719c552", "Revalidate and dispatch Khan retaliation", nil), Resolver: "khan.taunt.build",
				ResolverArguments: arguments, AwaitOpcode: "gam",
				TimeoutMillis: khanTauntResponseTimeoutMillis, SuccessCodes: []int{0},
				ResponseBarrier: Intent.ResponseBarrierCommitted,
				ResponseIdentity: Outbound.ResponseIdentity{
					PlayerID: int64(input.State.Player.ID), CastleID: int64(request.MainCastleID),
				},
			},
			{Name: "Record accepted Khan retaliation", NameDescriptor: Localization.New("server.app.record_accepted_khan_retaliation.d352f5fa", "Record accepted Khan retaliation", nil), Action: "khan.taunt.accepted", ActionArguments: arguments},
		},
	}, nil
}

func validateKhanTauntContext(
	gameState State.GameState,
	gameData *GameData.Store,
	request khanTauntRequest,
	now time.Time,
) error {
	if request.EventID != khanEventID || request.EventEndsAt.IsZero() || request.MainCastleID <= 0 ||
		request.RageCampID <= 0 || request.RageCampRevision == 0 || request.PlayerRageCap <= 0 || request.RageObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Khan taunt requires the active event occurrence, main castle, camp, and rage observation"), Localization.New("server.app.khan_taunt_requires_the.6833c0ad", "Khan taunt requires the active event occurrence, main castle, camp, and rage observation", nil))
	}
	score, active := gameState.LookupScalableEventScore(khanEventID)
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() || invasionRemainingSeconds(score, now) == 0 {
		return Localization.WithError(fmt.Errorf("%w: the Nomad event is no longer active", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.ee4f8de2", "intent plan became stale before dispatch: the Nomad event is no longer active", nil))
	}
	occurrence, occurrenceFound := gameState.LookupEventOccurrence(khanEventID)
	if !occurrenceFound || !State.SameEventOccurrence(request.EventEndsAt, occurrence.EndsAt) {
		return Localization.WithError(fmt.Errorf("%w: the active Khan event occurrence changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.d1216559", "intent plan became stale before dispatch: the active Khan event occurrence changed", nil))
	}
	main, exists := gameState.Castles[request.MainCastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return Localization.WithError(fmt.Errorf("%w: Khan taunt target must be the Great Empire main castle", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.75399db3", "intent plan became stale before dispatch: Khan taunt target must be the Great Empire main castle", nil))
	}
	target, exists := gameState.LookupMapObservation(0, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.TypeID != khanCampTypeID {
		return Localization.WithError(fmt.Errorf("%w: the active type-35 Khan camp changed or is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.e64259f7", "intent plan became stale before dispatch: the active type-35 Khan camp changed or is unavailable", nil))
	}
	if gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	camp, found := gameData.EventCamp(request.RageCampID)
	if !found || camp.EventID != khanEventID || camp.AreaTypeID != khanCampTypeID ||
		camp.PlayerRageCap <= 0 || camp.PlayerRageCap != gameState.Khan.PlayerRageCap {
		return Localization.WithError(fmt.Errorf("%w: the authoritative Khan rage cap is unavailable or changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c812467f", "intent plan became stale before dispatch: the authoritative Khan rage cap is unavailable or changed", nil))
	}
	khan := gameState.Khan
	if khan.RageCampID != request.RageCampID ||
		khan.RageCampRevision != request.RageCampRevision ||
		khan.RageBalanceCampRevision != request.RageCampRevision ||
		khan.PlayerRageCap != request.PlayerRageCap ||
		khan.PlayerTotalRage != request.PlayerTotalRage ||
		!khan.RageObservedAt.Equal(request.RageObservedAt) ||
		khan.PlayerRage < khan.PlayerRageCap ||
		!khan.FullRageTauntDue(occurrence) {
		return Localization.WithError(fmt.Errorf("%w: the Khan rage bar is no longer ready for this taunt", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.57f732b0", "intent plan became stale before dispatch: the Khan rage bar is no longer ready for this taunt", nil))
	}
	if request.KhanGuard.MainCastleID != request.MainCastleID {
		return Localization.WithError(fmt.Errorf("Khan taunt safety guard does not match the main castle"), Localization.New("server.app.khan_taunt_safety_guard.c2950c65", "Khan taunt safety guard does not match the main castle", nil))
	}
	if err := validateKhanLaneGuard(gameState, gameData, request.KhanGuard, now); err != nil {
		return err
	}
	return nil
}

func resolveKhanTauntStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request khanTauntRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	if err := validateKhanTauntContext(input.State, input.GameData, request, time.Now().UTC()); err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		AllianceVisible int   `json:"AV"`
		EventID         int64 `json:"EID"`
	}{AllianceVisible: 0, EventID: khanEventID})
	return Intent.Step{
		Name: "Dispatch Khan retaliation", NameDescriptor: Localization.New("server.app.dispatch_khan_retaliation.394a3d81", "Dispatch Khan retaliation", nil), Opcode: "lta", Payload: payload,
		Command:     Protocol.Command{Opcode: "lta", Payload: payload},
		AwaitOpcode: "gam", TimeoutMillis: khanTauntResponseTimeoutMillis,
		SuccessCodes: []int{0}, ResponseBarrier: Intent.ResponseBarrierCommitted,
		ResponseIdentity: Outbound.ResponseIdentity{
			PlayerID: int64(input.State.Player.ID), CastleID: int64(request.MainCastleID),
		},
		FinalDispatchAction: "khan.taunt.guard", FinalDispatchArguments: append(json.RawMessage(nil), arguments...),
	}, nil
}

func (application *Application) guardKhanTauntFinalDispatch(_ context.Context, arguments json.RawMessage) error {
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Auto Khan state is unavailable"), Localization.New("server.app.auto_khan_state_is.45acd25c", "Auto Khan state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready || gameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	var request khanTauntRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	return validateKhanTauntContext(application.State.ReadOnlyView(), gameData, request, time.Now().UTC())
}

func planKhanAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := khanAttackContext(input, arguments, time.Now().UTC(), false)
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	steps := generalSkillsContextSteps(input.State, request.CommanderID, time.Now().UTC())
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	steps = append(steps, deferredCRACommandStep(
		fmt.Sprintf("Build and launch Khan camp attack with commander %d", request.CommanderID),
		"khan.attack.build", arguments, contextPayload, Localization.New("server.app.build_and_launch_khan.d288daa1", "Build and launch Khan camp attack with commander {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}),
	))
	capture, _ := json.Marshal(khanLaunchCapture{
		RunID: request.RunID, EventEndsAt: request.EventEndsAt, SourceCastleID: source.ID,
		MainCastleID: request.MainCastleID, KingdomID: target.KingdomID, TargetX: target.X,
		TargetY: target.Y, CommanderID: request.CommanderID,
	})
	steps = append(steps, Intent.Step{
		Name: "Capture authoritative Khan camp movement", NameDescriptor: Localization.New("server.app.capture_authoritative_khan_camp.23385b02", "Capture authoritative Khan camp movement", nil), Action: "khan.attack.capture", ActionArguments: capture,
	})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"attack-context", "attack-inventory:" + castleID,
		"khan-lane:attack",
		fmt.Sprintf("khan-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	if source.ID != request.MainCastleID {
		// Chaining from the main castle leaves focus where the rest of the loop
		// already needs it. Only a separate attack castle moves focus away, and
		// that is the one case worth holding the session-wide focus lock for.
		claims = append(claims, "castle-focus")
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{request.CommanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoKhan", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack Khan camp %d:%d with commander %d", target.X, target.Y, request.CommanderID), SummaryDescriptor: Localization.New("server.app.attack_khan_camp_p.ca3a3c1b", "Attack Khan camp {p0}:{p1} with commander {p2}", Localization.Params{"p0": target.X, "p1": target.Y, "p2": fmt.Sprintf("%d", request.CommanderID)}),
		Steps: steps,
	}, nil
}

func (application *Application) resolveKhanAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	if err := application.requireAutoKhanRageBooster(Intent.Plan{
		Admission: &Intent.Admission{Class: Intent.AdmissionAttackLaunch, Module: "autoKhan"},
	}, time.Now().UTC()); err != nil {
		return Intent.Step{}, err
	}
	request, source, target, err := khanAttackContext(input, arguments, time.Now().UTC(), true)
	if err != nil {
		return Intent.Step{}, err
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("khan-camp:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				Level: target.Level,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve Khan camp attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_khan_camp_attack.fbc01024", "resolve Khan camp attack capacity", nil), err))
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	if err := khanAttackPresetAvailability(limitedPreset, source, input.GameData); err != nil {
		return Intent.Step{}, err
	}
	setup := invasionAttackSetup(limitedPreset)
	built, err := buildAttackSetup(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Khan attack preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_khan_attack_preset.38a91cb9", "build Khan attack preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve Khan horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_khan_horse_travel.6f0b1539", "resolve Khan horse travel boost", nil), err))
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Khan camp CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_khan_camp_cra.89f82b71", "build Khan camp CRA payload", nil), err))
	}
	return commandStep(fmt.Sprintf("Attack Khan camp at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_khan_camp_at.3c3c8f92", "Attack Khan camp at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y})), nil
}

func khanAttackPresetAvailability(
	preset AttackPresets.Preset,
	source State.CastleState,
	gameData *GameData.Store,
) error {
	_, shortage, err := AttackPresets.CheckInventory(preset, source.Units.Stationed, gameData, 1)
	if err != nil {
		return Localization.WithError(fmt.Errorf("%w: resolve Khan preset troop families: %v", Intent.ErrPlanStale, err), Localization.Join(Localization.New("server.app.khan_stale_preset_families", "intent plan became stale before dispatch: resolve Khan preset troop families", nil), Localization.FromError(err)))
	}
	if shortage != nil {
		return Localization.WithError(fmt.Errorf(
			"%w: Khan CRA launch cursor paused because preset needs %d of item %d and castle %d has %d",
			Intent.ErrPlanStale, shortage.Required, shortage.ItemID, source.ID, shortage.Available,
		), Localization.New("server.app.intent_plan_became_stale.90d0f9a9", "intent plan became stale before dispatch: Khan CRA launch cursor paused because preset needs {p1, number} of item {p2} and castle {p3} has {p4, number}", Localization.Params{"p1": shortage.Required, "p2": fmt.Sprintf("%d", shortage.ItemID), "p3": fmt.Sprintf("%d", source.ID), "p4": shortage.Available}))
	}
	return nil
}

func khanAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	requireDialog bool,
) (khanAttackRequest, State.CastleState, State.MapObservation, error) {
	var request khanAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	request.RunID = strings.TrimSpace(request.RunID)
	if request.RunID == "" || request.SourceCastleID <= 0 || request.MainCastleID <= 0 || request.CommanderID < 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("Khan attack requires run, castle, and commander ids"), Localization.New("server.app.khan_attack_requires_run.9aa7620b", "Khan attack requires run, castle, and commander ids", nil))
	}
	if input.GameData == nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	score, active := input.State.LookupScalableEventScore(khanEventID)
	if !active || score.RemainingSec <= 0 || score.ObservedAt.IsZero() || invasionRemainingSeconds(score, now) == 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("the Nomad event is no longer active"), Localization.New("server.app.the_nomad_event_is.8accf93e", "the Nomad event is no longer active", nil))
	}
	if request.MaxRageChain < 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("maximum Khan rage chain cannot be negative"), Localization.New("server.app.maximum_khan_rage_chain.7850fab9", "maximum Khan rage chain cannot be negative", nil))
	}
	occurrence, occurrenceFound := input.State.LookupEventOccurrence(khanEventID)
	if request.EventEndsAt.IsZero() || !occurrenceFound ||
		!State.SameEventOccurrence(request.EventEndsAt, occurrence.EndsAt) {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("the active Khan event occurrence changed"), Localization.New("server.app.the_active_khan_event.e214f37f", "the active Khan event occurrence changed", nil))
	}
	if request.MaxRageChain > 0 {
		taunts := input.State.KhanDefenseLaunchesForOccurrence(khanEventID, occurrence)
		if taunts >= request.MaxRageChain {
			return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
				"maximum Khan rage chain reached: %d / %d taunts", taunts, request.MaxRageChain,
			), Localization.New("server.app.maximum_khan_rage_chain.0bd9777a", "maximum Khan rage chain reached: {p0} / {p1} taunts", Localization.Params{"p0": taunts, "p1": request.MaxRageChain}))
		}
	}
	if request.RequireActiveRageBooster {
		booster, found := input.State.Market.Boosters[GameData.KhanRagePointsBoosterID]
		if !found || !booster.ActiveAt(now) {
			return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
				"Auto Khan requires an active Rage points booster (boi ID %d)",
				GameData.KhanRagePointsBoosterID,
			), Localization.New("server.app.auto_khan_requires_an.aab027f4", "Auto Khan requires an active Rage points booster (boi ID {p0})", Localization.Params{"p0": fmt.Sprintf("%d", GameData.KhanRagePointsBoosterID)}))
		}
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 || source.KingdomID != request.KingdomID {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("Khan attack source must be an owned Great Empire castle"), Localization.New("server.app.khan_attack_source_must.09015513", "Khan attack source must be an owned Great Empire castle", nil))
	}
	main, exists := input.State.Castles[request.MainCastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("Khan defense target must be the Great Empire main castle"), Localization.New("server.app.khan_defense_target_must.32dc49ba", "Khan defense target must be the Great Empire main castle", nil))
	}
	target, exists := input.State.LookupMapObservation(request.KingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.TypeID != khanCampTypeID {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("Khan camp %d:%d changed or is unavailable", request.TargetX, request.TargetY), Localization.New("server.app.khan_camp_p_p.a87d9a52", "Khan camp {p0}:{p1} changed or is unavailable", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if cooldown, found := input.State.NomadCamps.Cooldowns[fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)]; found && cooldown.PendingCooldownRefresh {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
			"%w: Khan camp is awaiting its post-battle cooldown refresh", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.03b26504", "intent plan became stale before dispatch: Khan camp is awaiting its post-battle cooldown refresh", nil))
	}
	if appDungeonCooldownRemaining(input.State, target, now) > 0 {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("%w: Khan camp is on cooldown", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.36358e90", "intent plan became stale before dispatch: Khan camp is on cooldown", nil))
	}
	if err := khanLaunchSafety(input.State, input.GameData, request, main, now); err != nil {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("commander %d is no longer available", request.CommanderID), Localization.New("server.app.commander_p_is_no.546e373b", "commander {p0} is no longer available", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
	}
	if requireDialog {
		dialog := input.State.AttackDialog
		if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
			dialog.Target.TypeID != khanCampTypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y {
			return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("authoritative attack dialog no longer matches the ready Khan camp"), Localization.New("server.app.authoritative_attack_dialog_no.96f89c17", "authoritative attack dialog no longer matches the ready Khan camp", nil))
		}
		if dialog.Target.EventCampCooldownRemaining > 0 {
			return khanAttackRequest{}, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf(
				"%w: authoritative Khan camp attack dialog is on cooldown", Intent.ErrPlanStale,
			), Localization.New("server.app.intent_plan_became_stale.a204b656", "intent plan became stale before dispatch: authoritative Khan camp attack dialog is on cooldown", nil))
		}
	}
	return request, source, target, nil
}

func khanLaunchSafety(
	gameState State.GameState,
	gameDataStore *GameData.Store,
	request khanAttackRequest,
	main State.CastleState,
	now time.Time,
) error {
	if err := validateKhanLaneGuard(gameState, gameDataStore, khanLaneGuardRequest{
		MainCastleID: main.ID, DefensePreset: request.DefensePreset,
		OpenGateProtection: request.OpenGateProtection, OffensiveUnitThreshold: request.OffensiveUnitThreshold,
		NomadPointThreshold: request.NomadPointThreshold,
	}, now); err != nil {
		return err
	}
	if main.Defense.ObservedAt.IsZero() || main.Defense.InventoryObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("main castle defense must be refreshed before a Khan attack"), Localization.New("server.app.main_castle_defense_must.9393020f", "main castle defense must be refreshed before a Khan attack", nil))
	}
	if !gameState.Khan.LastTauntResolvedAt.IsZero() && !main.Defense.ObservedAt.After(gameState.Khan.LastTauntResolvedAt) {
		return Localization.WithError(fmt.Errorf("main castle defense must be refreshed after the latest Khan taunt"), Localization.New("server.app.main_castle_defense_must.10ba3b6b", "main castle defense must be refreshed after the latest Khan taunt", nil))
	}
	if !KhanDomain.Matches(main, request.DefensePreset) {
		return Localization.WithError(fmt.Errorf("main castle no longer matches defense preset %q", request.DefensePreset.Name), Localization.New("server.app.main_castle_no_longer.d01131af", "main castle no longer matches defense preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.DefensePreset.Name)}))
	}
	if request.OpenGateProtection {
		if request.SourceCastleID != request.MainCastleID || request.OffensiveUnitThreshold <= 0 {
			return Localization.WithError(fmt.Errorf("offensive wall protection requires the main castle as source and a positive threshold"), Localization.New("server.app.offensive_wall_protection_requires.81463822", "offensive wall protection requires the main castle as source and a positive threshold", nil))
		}
		if gameDataStore == nil {
			return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
		}
		risk, err := KhanDomain.OffensiveWallUnits(main, gameDataStore, request.DefensePreset)
		if err != nil {
			return err
		}
		if risk.OffensiveUnits >= request.OffensiveUnitThreshold {
			return Localization.WithError(fmt.Errorf("defense would place %d offensive units on the wall (threshold %d)", risk.OffensiveUnits, request.OffensiveUnitThreshold), Localization.New("server.app.defense_would_place_p.702011fe", "defense would place {p0} offensive units on the wall (threshold {p1})", Localization.Params{"p0": risk.OffensiveUnits, "p1": request.OffensiveUnitThreshold}))
		}
	}
	return nil
}

func validateKhanLaneGuard(
	gameState State.GameState,
	gameDataStore *GameData.Store,
	request khanLaneGuardRequest,
	now time.Time,
) error {
	main, exists := gameState.Castles[request.MainCastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return Localization.WithError(fmt.Errorf("Auto Khan requires the Great Empire main castle"), Localization.New("server.app.auto_khan_requires_the.6a6d9358", "Auto Khan requires the Great Empire main castle", nil))
	}
	if State.HasIncomingPlayerAttack(gameState, now) {
		return Localization.WithError(fmt.Errorf("Auto Khan yielded to an incoming player attack for Auto Station"), Localization.New("server.app.auto_khan_yielded_to.a67a2a93", "Auto Khan yielded to an incoming player attack for Auto Station", nil))
	}
	if State.KhanAutoStationYieldActiveAt(gameState, now) {
		return Localization.WithError(fmt.Errorf("Auto Khan yielded while Auto Station is moving troops"), Localization.New("server.app.auto_khan_yielded_while.824ea287", "Auto Khan yielded while Auto Station is moving troops", nil))
	}
	if main.Defense.OpenGateUntil != nil && main.Defense.OpenGateUntil.After(now) {
		return Localization.WithError(fmt.Errorf("main castle gates are open until %s", main.Defense.OpenGateUntil.UTC().Format(time.RFC3339)), Localization.New("server.app.khan_gates_open_until", "main castle gates are open until {until}", Localization.Params{"until": main.Defense.OpenGateUntil.UTC().Format(time.RFC3339)}))
	}
	if gameState.Khan.Protection.Active {
		return Localization.WithError(fmt.Errorf("Auto Khan protection is locked: %s", gameState.Khan.Protection.Reason), Localization.Join(Localization.New("server.app.khan_protection.locked", "Auto Khan protection is locked", nil), gameState.Khan.Protection.ReasonDescriptor))
	}
	if request.NomadPointThreshold > 0 {
		score, _ := gameState.LookupScalableEventScore(khanEventID)
		if score.PlayerScore >= request.NomadPointThreshold {
			return Localization.WithError(fmt.Errorf("Nomad point threshold reached: %d / %d", score.PlayerScore, request.NomadPointThreshold), Localization.New("server.app.nomad_point_threshold_reached.1b32b43f", "Nomad point threshold reached: {p0} / {p1}", Localization.Params{"p0": score.PlayerScore, "p1": request.NomadPointThreshold}))
		}
	}
	if !request.OpenGateProtection {
		return nil
	}
	if request.OffensiveUnitThreshold <= 0 {
		return Localization.WithError(fmt.Errorf("open-gate protection requires a positive offensive-unit threshold"), Localization.New("server.app.open_gate_protection_requires.e6865549", "open-gate protection requires a positive offensive-unit threshold", nil))
	}
	if gameDataStore == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	risk, err := KhanDomain.OffensiveWallUnits(main, gameDataStore, request.DefensePreset)
	if err != nil {
		return err
	}
	if risk.OffensiveUnits >= request.OffensiveUnitThreshold {
		return Localization.WithError(fmt.Errorf(
			"defense would place %d offensive units on the wall (threshold %d)",
			risk.OffensiveUnits, request.OffensiveUnitThreshold,
		), Localization.New("server.app.defense_would_place_p.702011fe", "defense would place {p0} offensive units on the wall (threshold {p1})", Localization.Params{"p0": risk.OffensiveUnits, "p1": request.OffensiveUnitThreshold}))
	}
	return nil
}

// recordKhanTauntAcceptance runs only after the correlated GAM reply has been
// committed with response code zero. A Khan movement reducer may already have
// advanced the same cursor from that GAM; TauntCursorIncludes keeps the two
// authoritative acceptance paths idempotent.
func (application *Application) recordKhanTauntAcceptance(_ context.Context, arguments json.RawMessage) error {
	var request khanTauntRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	dispatchedAt := time.Now().UTC()
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		occurrence := State.EventOccurrence{EndsAt: request.EventEndsAt}
		if current, found := gameState.LookupEventOccurrence(khanEventID); found &&
			State.SameEventOccurrence(current.EndsAt, request.EventEndsAt) {
			occurrence.ObservedFrom = current.ObservedFrom
		}
		if gameState.Khan.TauntCursorIncludes(request.PlayerTotalRage, occurrence) {
			return nil, false, nil
		}
		gameState.Khan.TauntsTriggered++
		gameState.Khan.LastTauntTriggeredAt = dispatchedAt
		gameState.Khan.LastTauntTriggeredRage = request.PlayerTotalRage
		gameState.Khan.LastTauntTriggeredEventEndsAt = request.EventEndsAt.UTC()
		return []string{"khan"}, true, nil
	})
	return err
}

// rejectUnconfirmedKhanTauntDispatch keeps journals created by older builds
// fail-closed. Those plans recorded success immediately after writing LTA to
// the socket and therefore cannot prove that the game accepted the command.
func (*Application) rejectUnconfirmedKhanTauntDispatch(_ context.Context, _ json.RawMessage) error {
	return Localization.WithError(fmt.Errorf("%w: Khan taunt dispatch did not await an accepted GAM response", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.24cbddf7", "intent plan became stale before dispatch: Khan taunt dispatch did not await an accepted GAM response", nil))
}

func (application *Application) guardKhanLane(_ context.Context, arguments json.RawMessage) error {
	var request khanLaneGuardActionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.KhanGuard.MainCastleID <= 0 {
		return Localization.WithError(fmt.Errorf("Auto Khan safety guard is required"), Localization.New("server.app.auto_khan_safety_guard.9c31a18a", "Auto Khan safety guard is required", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	return validateKhanLaneGuard(
		application.State.ReadOnlyView(), gameData, request.KhanGuard, time.Now().UTC(),
	)
}

func (application *Application) captureKhanLaunch(_ context.Context, arguments json.RawMessage) error {
	var request khanLaunchCapture
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	var safetyError string
	var safetyDescriptor *Localization.Message
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
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
			return nil, false, Localization.WithError(fmt.Errorf("CRA response did not return commander %d's Khan movement", request.CommanderID), Localization.New("server.app.cra_response_did_not.44b65441", "CRA response did not return commander {p0}'s Khan movement", Localization.Params{"p0": fmt.Sprintf("%d", request.CommanderID)}))
		}
		if gameState.Khan.RunID != request.RunID {
			taunts := gameState.Khan.Taunts
			resolvedTaunts := gameState.Khan.ResolvedTaunts
			observed, resolved, lastResolved := gameState.Khan.TauntsObserved, gameState.Khan.TauntsResolved, gameState.Khan.LastTauntResolvedAt
			cooldownReports := gameState.Khan.CooldownReports
			rageCampID := gameState.Khan.RageCampID
			rageCampRevision := gameState.Khan.RageCampRevision
			rageCampObservedAt := gameState.Khan.RageCampObservedAt
			rageBalanceCampRevision := gameState.Khan.RageBalanceCampRevision
			playerRage, playerRageCap := gameState.Khan.PlayerRage, gameState.Khan.PlayerRageCap
			playerTotalRage, rageObservedAt := gameState.Khan.PlayerTotalRage, gameState.Khan.RageObservedAt
			triggered, lastTriggered := gameState.Khan.TauntsTriggered, gameState.Khan.LastTauntTriggeredAt
			lastTriggeredRage := gameState.Khan.LastTauntTriggeredRage
			lastTriggeredEventEndsAt := gameState.Khan.LastTauntTriggeredEventEndsAt
			gameState.Khan = State.KhanState{
				RunID: request.RunID, EventEndsAt: request.EventEndsAt, SourceCastleID: request.SourceCastleID,
				MainCastleID: request.MainCastleID, KingdomID: request.KingdomID, TargetX: request.TargetX,
				TargetY: request.TargetY, Launches: []State.KhanLaunchState{}, Taunts: taunts, ResolvedTaunts: resolvedTaunts,
				TauntsObserved: observed, TauntsResolved: resolved, LastTauntResolvedAt: lastResolved,
				RageCampID: rageCampID, RageCampRevision: rageCampRevision,
				RageCampObservedAt: rageCampObservedAt, RageBalanceCampRevision: rageBalanceCampRevision,
				PlayerRage: playerRage, PlayerRageCap: playerRageCap,
				PlayerTotalRage: playerTotalRage, RageObservedAt: rageObservedAt,
				TauntsTriggered: triggered, LastTauntTriggeredAt: lastTriggered,
				LastTauntTriggeredRage: lastTriggeredRage, LastTauntTriggeredEventEndsAt: lastTriggeredEventEndsAt,
				TauntCounterVersion: State.KhanTauntCounterVersion,
				CooldownReports:     cooldownReports, CooldownReportVersion: State.KhanCooldownReportVersion,
			}
		}
		for _, launch := range gameState.Khan.Launches {
			if launch.MovementID == selected.ID {
				return nil, false, nil
			}
		}
		launch := State.KhanLaunchState{
			CommanderID: request.CommanderID, MovementID: selected.ID, ArrivesAt: selected.ArrivesAt.UTC(),
		}
		// A completed inversion is historical, not a permanent feature lock. The
		// policy waits until both arrivals have passed before admitting another
		// launch; a subsequently ordered launch therefore clears the old marker.
		gameState.Khan.SafetyError = ""
		gameState.Khan.SafetyErrorDescriptor = nil
		if count := len(gameState.Khan.Launches); count > 0 {
			previous := gameState.Khan.Launches[count-1]
			if State.KhanLaunchOvertakes(previous.ArrivesAt, launch.ArrivesAt) {
				safetyError = fmt.Sprintf(
					"commander %d arrives at %s before commander %d at %s",
					launch.CommanderID, launch.ArrivesAt.Format(time.RFC3339Nano),
					previous.CommanderID, previous.ArrivesAt.Format(time.RFC3339Nano),
				)
				safetyDescriptor = Localization.Bind(State.ArrivalOrderDescriptor(launch.CommanderID, launch.ArrivesAt, previous.CommanderID, previous.ArrivesAt), safetyError)
				gameState.Khan.SafetyError = safetyError
				gameState.Khan.SafetyErrorDescriptor = Localization.Clone(safetyDescriptor)
			}
		}
		gameState.Khan.Launches = append(gameState.Khan.Launches, launch)
		if len(gameState.Khan.Launches) > 256 {
			gameState.Khan.Launches = append([]State.KhanLaunchState(nil), gameState.Khan.Launches[len(gameState.Khan.Launches)-256:]...)
		}
		gameState.Khan.AttacksLaunched++
		launchedAt := selected.ObservedAt
		if launchedAt.IsZero() {
			launchedAt = time.Now().UTC()
		}
		gameState.Khan.LastAttackLaunchedAt = launchedAt.UTC()
		State.RecordEventAttackLaunch(gameState, 72, State.EventAttackRecord{
			MovementID: selected.ID, Kind: State.EventActivityKhan, KingdomID: request.KingdomID,
			TargetTypeID: khanCampTypeID, TargetX: request.TargetX, TargetY: request.TargetY,
			LaunchedAt: launchedAt.UTC(), ArrivesAt: selected.ArrivesAt.UTC(),
		})
		return []string{"khan", "movements", "event-scores"}, true, nil
	})
	if err != nil {
		return err
	}
	if safetyError != "" {
		return Localization.WithError(fmt.Errorf("unsafe Khan chain arrival order: %s", safetyError), Localization.Join(Localization.New("server.app.arrival_order.khan_context", "unsafe Khan chain arrival order", nil), safetyDescriptor))
	}
	return nil
}

func planKhanOpenGate(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.OffensiveUnitThreshold <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Khan gate protection requires the Great Empire main castle and a positive threshold"), Localization.New("server.app.khan_gate_protection_requires.7ea27b21", "Khan gate protection requires the Great Empire main castle and a positive threshold", nil))
	}
	payload, _ := json.Marshal(struct {
		CastleID  State.CastleID  `json:"CID"`
		KingdomID State.KingdomID `json:"KID"`
		Cooldown  int             `json:"CD"`
	}{castle.ID, castle.KingdomID, 0})
	steps := defenseRefreshSteps(castle)
	steps = append(steps,
		Intent.Step{Name: "Verify offensive wall threshold", NameDescriptor: Localization.New("server.app.verify_offensive_wall_threshold.eba92a8c", "Verify offensive wall threshold", nil), Action: "khan.protection.guard", ActionArguments: arguments},
		commandStep("Open main castle gates for six hours", "mos", payload, "mos", Localization.New("server.app.open_main_castle_gates.ee389d37", "Open main castle gates for six hours", nil)),
		Intent.Step{Name: "Activate Auto Khan soft lock", NameDescriptor: Localization.New("server.app.activate_auto_khan_soft.5de464cb", "Activate Auto Khan soft lock", nil), Action: "khan.protection.activate", ActionArguments: arguments},
	)
	claims := append(defenseClaims(castle.ID), "khan-protection", "khan-lane")
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Open gates and lock Auto Khan after %d offensive wall units", request.OffensiveUnitThreshold), SummaryDescriptor: Localization.New("server.app.open_gates_and_lock.48ed5a10", "Open gates and lock Auto Khan after {p0} offensive wall units", Localization.Params{"p0": request.OffensiveUnitThreshold}),
		Steps: steps,
	}, nil
}

func planKhanPointLimitProtection(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	var request khanPointLimitRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, exists := input.State.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.PointThreshold <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Khan point protection requires the Great Empire main castle and a positive point threshold"), Localization.New("server.app.khan_point_protection_requires.024b5b31", "Khan point protection requires the Great Empire main castle and a positive point threshold", nil))
	}
	score, active := input.State.LookupScalableEventScore(khanEventID)
	if !active || score.PlayerScore < request.PointThreshold {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Nomad points are below the configured threshold"), Localization.New("server.app.nomad_points_are_below.d1890d95", "Nomad points are below the configured threshold", nil))
	}
	now := time.Now().UTC()
	movementIDs := khanPointLimitMovementIDs(input.State, now)
	steps := make([]Intent.Step, 0, len(movementIDs)+1)
	claims := []string{"khan-protection", "khan-lane", "castle:" + strconv.FormatInt(int64(castle.ID), 10)}
	for _, movementID := range movementIDs {
		payload, _ := json.Marshal(struct {
			MovementID State.MovementID `json:"MID"`
		}{movementID})
		steps = append(steps, commandStep(fmt.Sprintf("Recall Khan movement %d", movementID), "mcm", payload, "mcm", Localization.New("server.app.recall_khan_movement_p.cf745f10", "Recall Khan movement {p0}", Localization.Params{"p0": fmt.Sprintf("%d", movementID)})))
		claims = append(claims, "movement:"+strconv.FormatInt(int64(movementID), 10))
	}
	if castle.Defense.OpenGateUntil == nil || !castle.Defense.OpenGateUntil.After(now) {
		payload, _ := json.Marshal(struct {
			CastleID  State.CastleID  `json:"CID"`
			KingdomID State.KingdomID `json:"KID"`
			Cooldown  int             `json:"CD"`
		}{castle.ID, castle.KingdomID, 0})
		steps = append(steps, commandStep("Open main castle gates for six hours", "mos", payload, "mos", Localization.New("server.app.open_main_castle_gates.ee389d37", "Open main castle gates for six hours", nil)))
	}
	if len(steps) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Khan point protection is already active"), Localization.New("server.app.khan_point_protection_is.d299f1e5", "Khan point protection is already active", nil))
	}
	return Intent.Plan{
		Claims: claims,
		Summary: fmt.Sprintf(
			"Stop Auto Khan at %d Nomad points, recall %d movement(s), and open gates",
			request.PointThreshold, len(movementIDs),
		), SummaryDescriptor: Localization.New("server.app.stop_auto_khan_at.d81f9afc", "Stop Auto Khan at {p0} Nomad points, recall {p1} movement(s), and open gates", Localization.Params{"p0": request.PointThreshold, "p1": fmt.Sprintf("%d", len(movementIDs))}),
		Steps: steps,
	}, nil
}

func planKhanDefenseToolReplenish(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Plan, error) {
	request, main, item, purchaseCastleID, purchaseKingdomID, err := khanDefenseToolPurchaseContext(
		input, arguments, time.Now().UTC(), false,
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(struct {
		ProductID State.PackageID `json:"PID"`
		BuildType int64           `json:"BT"`
		TableID   int64           `json:"TID"`
		Amount    int64           `json:"AMT"`
		KingdomID State.KingdomID `json:"KID"`
		CastleID  int64           `json:"AID"`
		Premium   int64           `json:"PC2"`
		BuyAll    int64           `json:"BA"`
		Power     int64           `json:"PWR"`
		Position  int64           `json:"_PO"`
	}{request.PackageID, 0, request.ShopTableID, request.Amount, purchaseKingdomID, purchaseCastleID, -1, 0, 0, -1})
	steps := castleContextSteps(input, main)
	if item.Stock > 0 {
		historyPayload, _ := json.Marshal(map[string]any{"CID": main.ID, "KID": main.KingdomID})
		history := shopCommandStep("Refresh defense-tool package counters", "gbc", historyPayload, 0).WithNameDescriptor(Localization.New("server.app.refresh_defense_tool_package.07939425", "Refresh defense-tool package counters", nil))
		history.ResponseBarrier = Intent.ResponseBarrierCommitted
		steps = append(steps, history)
	}
	purchase := shopCommandStep(fmt.Sprintf("Purchase defense tool %d", request.ToolID), "sbp", payload, 0).WithNameDescriptor(Localization.New("server.app.purchase_defense_tool.step", "Purchase defense tool {id}", Localization.Params{"id": strconv.FormatInt(int64(request.ToolID), 10)}))
	purchase.FinalDispatchAction = "khan.defense_tools.guard"
	purchase.FinalDispatchArguments = append(json.RawMessage(nil), arguments...)
	steps = append(steps,
		Intent.Step{Name: "Recheck non-ruby defense-tool purchase", NameDescriptor: Localization.New("server.app.recheck_non_ruby_defense.9cb94b64", "Recheck non-ruby defense-tool purchase", nil), Action: "khan.defense_tools.guard", ActionArguments: arguments},
		purchase,
		Intent.Step{Name: "Record defense-tool shop cadence", NameDescriptor: Localization.New("server.app.record_defense_tool_shop.7631a5b1", "Record defense-tool shop cadence", nil), Action: "khan.defense_tools.purchased", ActionArguments: arguments},
		defenseContextStep(main),
	)
	claims := append(defenseClaims(main.ID),
		"shop", "shop:table:"+strconv.FormatInt(request.ShopTableID, 10), "account-resources",
		"unit:"+strconv.FormatInt(int64(request.ToolID), 10),
	)
	return Intent.Plan{
		Claims: claims,
		Summary: fmt.Sprintf(
			"Replenish tool %d with %d package purchase(s) for %d %s",
			request.ToolID, request.Amount, request.Amount*item.Price, item.PriceName,
		), SummaryDescriptor: defenseToolPriceDescriptor(Localization.New("server.app.replenish_tool_p_with.189b3848", "Replenish tool {p0} with {p1} package purchase(s) for {p2} {p3}", Localization.Params{"p0": fmt.Sprintf("%d", request.ToolID), "p1": request.Amount, "p2": request.Amount * item.Price, "p3": fmt.Sprintf("%s", item.PriceName)}), input, "p3", item),
		Steps: steps,
	}, nil
}

func khanDefenseToolPurchaseContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
	dispatchReady bool,
) (khanDefenseToolPurchaseRequest, State.CastleState, GameData.DefenseToolShopPackage, int64, State.KingdomID, error) {
	var request khanDefenseToolPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0, err
	}
	if request.CastleID <= 0 || request.PackageID <= 0 || request.ToolID <= 0 || request.Amount <= 0 || request.ShopTableID <= 0 {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0,
			Localization.WithError(fmt.Errorf("Khan defense-tool replenishment requires castle, package, tool, shop table, and positive amount"), Localization.New("server.app.khan_defense_tool_replenishment.6778186e", "Khan defense-tool replenishment requires castle, package, tool, shop table, and positive amount", nil))
	}
	main, exists := input.State.Castles[request.CastleID]
	if !exists || main.KingdomID != 0 || main.SlotType != 1 {
		return request, State.CastleState{}, GameData.DefenseToolShopPackage{}, 0, 0,
			Localization.WithError(fmt.Errorf("Khan defense tools can only replenish the Great Empire main castle"), Localization.New("server.app.khan_defense_tools_can.2c8878b4", "Khan defense tools can only replenish the Great Empire main castle", nil))
	}
	if input.GameData == nil {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if next := input.State.Khan.LastDefenseToolPurchaseAt.Add(30 * time.Second); next.After(now) {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0,
			Localization.WithError(fmt.Errorf("Khan defense tools can next be replenished at %s", next.UTC().Format(time.RFC3339)), Localization.New("server.app.khan_defense_tools_next_replenishment", "Khan defense tools can next be replenished at {until}", Localization.Params{"until": next.UTC().Format(time.RFC3339)}))
	}
	item, found := input.GameData.DefenseToolShopPackage(int64(request.PackageID))
	if !found || item.ToolID != int64(request.ToolID) {
		return request, main, GameData.DefenseToolShopPackage{}, 0, 0,
			Localization.WithError(fmt.Errorf("package %d is not a supported non-ruby package for tool %d", request.PackageID, request.ToolID), Localization.New("server.app.package_p_is_not.f4a77aec", "package {p0} is not a supported non-ruby package for tool {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": fmt.Sprintf("%d", request.ToolID)}))
	}
	if !autoBuyerIntentLevelEligible(
		input.State.Player, item.MinLevel, item.MaxLevel, item.MinLegendLevel, item.MaxLegendLevel,
	) {
		return request, main, item, 0, 0,
			Localization.WithError(fmt.Errorf("package %d is not available at the current player level", request.PackageID), Localization.New("server.app.package_p_is_not.09e668ab", "package {p0} is not available at the current player level", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID)}))
	}
	route, active := input.State.ActiveShopForPackage(request.PackageID, now)
	if !active && item.PriceScope == GameData.DefenseToolPriceCastleResource && item.PriceID == GameData.StormAquamarineID &&
		input.State.Storm.LunaShopTableID > 0 && input.State.Storm.LunaShopTableID == request.ShopTableID {
		route = State.EventShopRoute{EventID: request.ShopTableID}
		active = true
	}
	if !active || route.EventID != request.ShopTableID {
		return request, main, item, 0, 0,
			Localization.WithError(fmt.Errorf("package %d is not advertised by active shop table %d", request.PackageID, request.ShopTableID), Localization.New("server.app.package_p_is_not.63168155", "package {p0} is not advertised by active shop table {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": fmt.Sprintf("%d", request.ShopTableID)}))
	}
	if _, advertised := input.State.ActiveShopForPackage(request.PackageID, now); advertised {
		if _, routeErr := validateEventBackedSBP(input, main, eventBackedSBPRequest{
			PackageID: request.PackageID, TableID: request.ShopTableID, Amount: request.Amount,
			Stock: item.Stock, MaxBuyPerClick: item.MaxBuyPerClick,
		}, now, dispatchReady); routeErr != nil {
			return request, main, item, 0, 0, routeErr
		}
	} else if dispatchReady {
		protocol := input.ProtocolContext
		if !main.Focused || protocol.SessionGeneration != input.State.Session.Generation ||
			protocol.ConnectionGeneration != input.State.Session.ConnectionGeneration ||
			protocol.FocusedCastleID != main.ID || protocol.FocusSubcontext != State.FocusSubcontextCastle ||
			protocol.FocusEpoch == 0 {
			return request, main, item, 0, 0,
				Localization.WithError(fmt.Errorf("%w: defense-tool purchase lost current-session main-castle focus", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.ffeece62", "intent plan became stale before dispatch: defense-tool purchase lost current-session main-castle focus", nil))
		}
	}
	deficit := KhanDomain.DefenseToolDeficits(main, request.DefensePreset)[request.ToolID]
	if deficit <= 0 {
		return request, main, item, 0, 0, Localization.WithError(fmt.Errorf("defense tool %d is no longer short", request.ToolID), Localization.New("server.app.defense_tool_p_is.1bd1488a", "defense tool {p0} is no longer short", Localization.Params{"p0": fmt.Sprintf("%d", request.ToolID)}))
	}
	neededPurchases := (deficit + item.ToolAmount - 1) / item.ToolAmount
	if request.Amount > neededPurchases {
		return request, main, item, 0, 0,
			Localization.WithError(fmt.Errorf("amount %d exceeds the %d purchase(s) required for tool %d", request.Amount, neededPurchases, request.ToolID), Localization.New("server.app.amount_p_exceeds_the.bd16bfe2", "amount {p0} exceeds the {p1} purchase(s) required for tool {p2}", Localization.Params{"p0": request.Amount, "p1": neededPurchases, "p2": fmt.Sprintf("%d", request.ToolID)}))
	}
	if item.MaxBuyPerClick > 0 && request.Amount > item.MaxBuyPerClick {
		return request, main, item, 0, 0, Localization.WithError(fmt.Errorf("amount exceeds package %d per-click maximum %d", request.PackageID, item.MaxBuyPerClick), Localization.New("server.app.amount_exceeds_package_p.6f45dfed", "amount exceeds package {p0} per-click maximum {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": item.MaxBuyPerClick}))
	}
	if item.Stock > 0 {
		offers, observedAt, found := input.State.ConstructionOffersFor(main.ID, main.KingdomID)
		if dispatchReady && (!found || observedAt.IsZero() || observedAt.After(now) || now.Sub(observedAt) >= shopPurchaseCounterMaximumAge) {
			return request, main, item, 0, 0,
				Localization.WithError(fmt.Errorf("%w: defense-tool package counters are not fresh for castle %d", Intent.ErrPlanStale, main.ID), Localization.New("server.app.intent_plan_became_stale.480611ee", "intent plan became stale before dispatch: defense-tool package counters are not fresh for castle {p1}", Localization.Params{"p1": fmt.Sprintf("%d", main.ID)}))
		}
		remaining := max(int64(0), item.Stock-offers[request.PackageID])
		if request.Amount > remaining {
			return request, main, item, 0, 0, Localization.WithError(fmt.Errorf("package %d has only %d purchase(s) remaining", request.PackageID, remaining), Localization.New("server.app.package_p_has_only.09f5e3d7", "package {p0} has only {p1} purchase(s) remaining", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": remaining}))
		}
	}
	if request.Amount > math.MaxInt64/item.Price {
		return request, main, item, 0, 0, Localization.WithError(fmt.Errorf("defense-tool purchase amount is too large"), Localization.New("server.app.defense_tool_purchase_amount.3d1593de", "defense-tool purchase amount is too large", nil))
	}
	balance, purchaseCastleID, purchaseKingdomID, available := khanDefenseToolPurchaseBalance(input.State, main, item)
	if !available {
		return request, main, item, 0, 0, Localization.WithError(fmt.Errorf("%s balance is unavailable", item.PriceName), defenseToolPriceDescriptor(Localization.New("server.app.p_balance_is_unavailable.d3e1e9e0", "{p0} balance is unavailable", Localization.Params{"p0": fmt.Sprintf("%s", item.PriceName)}), input, "p0", item))
	}
	required := request.Amount * item.Price
	if balance < required {
		return request, main, item, 0, 0,
			Localization.WithError(fmt.Errorf("package %d requires %d %s but only %d is available", request.PackageID, required, item.PriceName, balance), defenseToolPriceDescriptor(Localization.New("server.app.package_p_requires_p.cb572b3b", "package {p0} requires {p1} {p2} but only {p3} is available", Localization.Params{"p0": fmt.Sprintf("%d", request.PackageID), "p1": required, "p2": fmt.Sprintf("%s", item.PriceName), "p3": balance}), input, "p2", item))
	}
	return request, main, item, purchaseCastleID, purchaseKingdomID, nil
}

func khanDefenseToolPurchaseBalance(
	gameState State.GameState,
	main State.CastleState,
	item GameData.DefenseToolShopPackage,
) (int64, int64, State.KingdomID, bool) {
	switch item.PriceScope {
	case GameData.DefenseToolPricePlayerResource:
		return int64(math.Floor(gameState.Player.Resources[State.ResourceID(item.PriceID)])), -1, 0, true
	case GameData.DefenseToolPriceCurrency:
		return int64(math.Floor(gameState.Player.Currencies[State.CurrencyID(item.PriceID)])), -1, 0, true
	case GameData.DefenseToolPriceCastleResource:
		if item.PriceID != GameData.StormAquamarineID {
			return int64(math.Floor(main.Resources[State.ResourceID(item.PriceID)].Amount)), int64(main.ID), main.KingdomID, true
		}
		castles := make([]State.CastleState, 0)
		for _, castle := range gameState.Castles {
			if castle.KingdomID == GameData.StormKingdomID {
				castles = append(castles, castle)
			}
		}
		sort.Slice(castles, func(left, right int) bool {
			if castles[left].SlotType != castles[right].SlotType {
				return castles[left].SlotType == 1
			}
			return castles[left].ID < castles[right].ID
		})
		if len(castles) > 0 {
			castle := castles[0]
			return int64(math.Floor(castle.Resources[State.ResourceID(item.PriceID)].Amount)), int64(castle.ID), castle.KingdomID, true
		}
	}
	return 0, 0, 0, false
}

func (application *Application) guardKhanDefenseToolPurchase(_ context.Context, arguments json.RawMessage) error {
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	view := application.State.PlanningView()
	_, _, _, _, _, err := khanDefenseToolPurchaseContext(
		Intent.PlanningContext{State: view.State, GameData: gameData, Partitions: view.Partitions, ProtocolContext: view.ProtocolContext},
		arguments, time.Now().UTC(), true,
	)
	return err
}

func (application *Application) markKhanDefenseToolPurchase(_ context.Context, arguments json.RawMessage) error {
	var request khanDefenseToolPurchaseRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	now := time.Now().UTC()
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Khan.LastDefenseToolPurchaseAt = now
		return []string{"khan", "inventory", "resources", "currencies"}, true, nil
	})
	return err
}

func khanPointLimitMovementIDs(gameState State.GameState, now time.Time) []State.MovementID {
	seen := map[State.MovementID]struct{}{}
	result := make([]State.MovementID, 0, len(gameState.Khan.Launches))
	for _, launch := range gameState.Khan.Launches {
		movement, found := gameState.LookupMovement(launch.MovementID)
		if !found || movement.Direction != 0 || !khanMovementActive(movement, now) {
			continue
		}
		if movement.OwnerPlayerID != 0 && movement.OwnerPlayerID != gameState.Player.ID {
			continue
		}
		if _, owned := gameState.Castles[movement.SourceCastleID]; !owned {
			continue
		}
		if _, duplicate := seen[movement.ID]; duplicate {
			continue
		}
		seen[movement.ID] = struct{}{}
		result = append(result, movement.ID)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func (application *Application) guardKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	state := application.State.ReadOnlyView()
	if State.HasIncomingPlayerAttack(state, time.Now().UTC()) || State.KhanAutoStationYieldActiveAt(state, time.Now().UTC()) {
		return Localization.WithError(fmt.Errorf("Auto Khan yielded gate protection to Auto Station"), Localization.New("server.app.auto_khan_yielded_gate.f4a2e7e0", "Auto Khan yielded gate protection to Auto Station", nil))
	}
	if castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(time.Now().UTC()) {
		return Localization.WithError(fmt.Errorf("main castle gates are already open"), Localization.New("server.app.main_castle_gates_are.44b76c7c", "main castle gates are already open", nil))
	}
	if risk.OffensiveUnits < request.OffensiveUnitThreshold {
		return Localization.WithError(fmt.Errorf("offensive wall risk fell to %d below threshold %d", risk.OffensiveUnits, request.OffensiveUnitThreshold), Localization.New("server.app.offensive_wall_risk_fell.5804d6c0", "offensive wall risk fell to {p0} below threshold {p1}", Localization.Params{"p0": risk.OffensiveUnits, "p1": request.OffensiveUnitThreshold}))
	}
	return nil
}

func (application *Application) activateKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if castle.Defense.OpenGateUntil == nil || !castle.Defense.OpenGateUntil.After(now) {
		return Localization.WithError(fmt.Errorf("the game did not confirm an active open-gate period"), Localization.New("server.app.the_game_did_not.dc496f02", "the game did not confirm an active open-gate period", nil))
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		gameState.Khan.Protection = State.KhanProtectionState{
			Active: true, CastleID: request.CastleID, OffensiveWallUnits: risk.OffensiveUnits,
			OffensiveUnitThreshold: request.OffensiveUnitThreshold, TriggeredAt: now,
			GateOpenUntil:    castle.Defense.OpenGateUntil.UTC(),
			Reason:           fmt.Sprintf("Add defense units to continue; %d offensive units reached the %d-unit wall threshold", risk.OffensiveUnits, request.OffensiveUnitThreshold),
			ReasonDescriptor: Localization.New("server.app.khan_protection.add_defense", "Add defense units to continue; {units, number} offensive units reached the {threshold, number}-unit wall threshold", Localization.Params{"units": risk.OffensiveUnits, "threshold": request.OffensiveUnitThreshold}),
		}
		gameState.Khan.Protection.ReasonDescriptor = Localization.Bind(gameState.Khan.Protection.ReasonDescriptor, gameState.Khan.Protection.Reason)
		return []string{"khan", "defense"}, true, nil
	})
	return err
}

func planKhanProtectionClear(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if !input.State.Khan.Protection.Active {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Auto Khan protection is not active"), Localization.New("server.app.auto_khan_protection_is.55fd9b22", "Auto Khan protection is not active", nil))
	}
	return Intent.Plan{
		Claims: []string{"khan-protection", "khan-lane"}, Summary: "Clear recovered Auto Khan protection lock", SummaryDescriptor: Localization.New("server.app.clear_recovered_auto_khan.d138c900", "Clear recovered Auto Khan protection lock", nil),
		Steps: []Intent.Step{{Name: "Verify defense recovery", NameDescriptor: Localization.New("server.app.verify_defense_recovery.60824194", "Verify defense recovery", nil), Action: "khan.protection.clear", ActionArguments: arguments}},
	}, nil
}

func (application *Application) clearKhanProtection(_ context.Context, arguments json.RawMessage) error {
	request, castle, risk, err := application.khanProtectionContext(arguments)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	protection := application.State.ReadOnlyView().Khan.Protection
	if !protection.Active {
		return nil
	}
	if protection.GateOpenUntil.After(now) || castle.Defense.OpenGateUntil != nil && castle.Defense.OpenGateUntil.After(now) {
		return Localization.WithError(fmt.Errorf("the six-hour open-gate protection is still active"), Localization.New("server.app.the_six_hour_open.f685bace", "the six-hour open-gate protection is still active", nil))
	}
	if !castle.Defense.ObservedAt.After(protection.GateOpenUntil) {
		return Localization.WithError(fmt.Errorf("main castle defense has not been refreshed since protection expired"), Localization.New("server.app.main_castle_defense_has.5e3532d7", "main castle defense has not been refreshed since protection expired", nil))
	}
	if risk.OffensiveUnits >= request.OffensiveUnitThreshold {
		return Localization.WithError(fmt.Errorf("add defense units to continue; %d offensive units still reach the wall", risk.OffensiveUnits), Localization.New("server.app.add_defense_units_to.429ca75a", "add defense units to continue; {p0} offensive units still reach the wall", Localization.Params{"p0": risk.OffensiveUnits}))
	}
	_, err = application.State.ApplyComponents(State.Components(State.ComponentKhan), func(gameState *State.GameState) ([]string, bool, error) {
		if !gameState.Khan.Protection.Active {
			return nil, false, nil
		}
		gameState.Khan.Protection = State.KhanProtectionState{}
		return []string{"khan", "defense"}, true, nil
	})
	return err
}

func (application *Application) khanProtectionContext(
	arguments json.RawMessage,
) (khanProtectionRequest, State.CastleState, KhanDomain.WallRisk, error) {
	var request khanProtectionRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, err
	}
	state := application.State.ReadOnlyView()
	castle, exists := state.Castles[request.CastleID]
	if !exists || castle.KingdomID != 0 || castle.SlotType != 1 || request.OffensiveUnitThreshold <= 0 {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, Localization.WithError(fmt.Errorf("Khan protection target is invalid"), Localization.New("server.app.khan_protection_target_is.74128c9a", "Khan protection target is invalid", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	risk, err := KhanDomain.OffensiveWallUnits(castle, gameData, request.DefensePreset)
	if err != nil {
		return khanProtectionRequest{}, State.CastleState{}, KhanDomain.WallRisk{}, err
	}
	return request, castle, risk, nil
}

func khanMovementActive(movement State.MovementState, now time.Time) bool {
	if movement.Direction == 0 {
		return movement.ArrivesAt == nil || movement.ArrivesAt.After(now)
	}
	return movement.ReturnsAt == nil || movement.ReturnsAt.After(now)
}

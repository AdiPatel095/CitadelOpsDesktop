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
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/Scheduling"
	"CitadelDesktop/Server/State"
)

const (
	riftMapTypeID                = 43
	manualAllianceAttackModuleID = "manualAllianceTargets"
	maidenSupportEffectID        = 121
	maidenSupportMinimum         = 300
	maidenSupportMaximum         = 1050
	maidenProbeCountPerFlank     = 11
)

func planSpyLaunch(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID State.CastleID  `json:"sourceCastleId,omitempty"`
		TargetX        int             `json:"targetX"`
		TargetY        int             `json:"targetY"`
		KingdomID      State.KingdomID `json:"kingdomId,omitempty"`
		SpyCount       int             `json:"spyCount,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.TargetX < 0 || request.TargetY < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("target coordinates must be non-negative"), Localization.New("server.app.target_coordinates_must_be.0f93b493", "target coordinates must be non-negative", nil))
	}
	source, err := sourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.SpyCount == 0 {
		request.SpyCount = 1
	}
	if request.SpyCount < 1 || request.SpyCount > 100 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("spyCount must be between 1 and 100"), Localization.New("server.app.spycount_must_be_between.4ccdd7dd", "spyCount must be between 1 and 100", nil))
	}
	payload, _ := json.Marshal(struct {
		SourceID State.CastleID  `json:"SID"`
		TargetX  int             `json:"TX"`
		TargetY  int             `json:"TY"`
		SpyCount int             `json:"SC"`
		SpyType  int             `json:"ST"`
		Success  int             `json:"SE"`
		Booster  int             `json:"HBW"`
		Kingdom  State.KingdomID `json:"KID"`
		Travel   int             `json:"PTT"`
		Delay    int             `json:"SD"`
	}{source.ID, request.TargetX, request.TargetY, request.SpyCount, 0, 100, -1, request.KingdomID, 1, 0})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(source.ID), 10),
			fmt.Sprintf("spy-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY),
		},
		Summary: fmt.Sprintf("Spy on %d:%d with %d agent(s)", request.TargetX, request.TargetY, request.SpyCount), SummaryDescriptor: Localization.New("server.app.spy_on_p_p.2b36db0f", "Spy on {p0}:{p1} with {p2} agent(s)", Localization.Params{"p0": request.TargetX, "p1": request.TargetY, "p2": request.SpyCount}),
		Steps: []Intent.Step{commandStep("Launch spy mission", "csm", payload, "csm", Localization.New("server.app.launch_spy_mission.05b23221", "Launch spy mission", nil))},
	}, nil
}

func planMaidenCommsWave(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		// SourceX and SourceY are retained for compatibility with older clients,
		// but Maiden waves always launch from the authoritative main castle.
		SourceX            *int                          `json:"sourceX,omitempty"`
		SourceY            *int                          `json:"sourceY,omitempty"`
		RunID              string                        `json:"runId,omitempty"`
		UnitID             State.UnitID                  `json:"unitWodID"`
		CommanderIDs       []State.CommanderID           `json:"commanderIds,omitempty"`
		HorseTravelBoostID int                           `json:"horseTravelBoostId"`
		CommanderSelection *craCommanderSelectionRequest `json:"commanderSelection,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return Intent.Plan{}, err
	}
	source, err := sourceCastle(input.State, 0)
	if err != nil {
		return Intent.Plan{}, err
	}
	var maidenRun *State.RiftMaidenRunState
	request.RunID = strings.TrimSpace(request.RunID)
	if request.RunID != "" {
		current := input.State.Rift.MaidenRun
		if current == nil || current.Status != "running" || current.ID != request.RunID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift Maiden run %s is no longer active", request.RunID), Localization.New("server.app.rift_maiden_run_p.ece8e8ac", "Rift Maiden run {p0} is no longer active", Localization.Params{"p0": fmt.Sprintf("%s", request.RunID)}))
		}
		maidenRun = current
		source, err = sourceCastle(input.State, current.SourceCastleID)
		if err != nil {
			return Intent.Plan{}, err
		}
		if source.X != current.SourceX || source.Y != current.SourceY || source.KingdomID != current.KingdomID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the Rift Maiden run's source castle changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.b0293027", "intent plan became stale before dispatch: the Rift Maiden run's source castle changed", nil))
		}
		if request.UnitID != current.UnitID || request.HorseTravelBoostID != current.HorseTravelBoostID {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the Rift Maiden run's probe settings changed", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.22a7e04c", "intent plan became stale before dispatch: the Rift Maiden run's probe settings changed", nil))
		}
		allowed := make(map[State.CommanderID]struct{}, len(current.CommanderIDs))
		for _, commanderID := range current.CommanderIDs {
			allowed[commanderID] = struct{}{}
		}
		if request.CommanderSelection != nil {
			for _, commanderID := range request.CommanderSelection.Candidates {
				if _, ok := allowed[commanderID]; !ok {
					return Intent.Plan{}, Localization.WithError(fmt.Errorf("commander %d is not assigned to Rift Maiden run %s", commanderID, current.ID), Localization.New("server.app.commander_p_is_not.e57130d9", "commander {p0} is not assigned to Rift Maiden run {p1}", Localization.Params{"p0": fmt.Sprintf("%d", commanderID), "p1": fmt.Sprintf("%s", current.ID)}))
				}
			}
		}
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unitWodID must identify a probe unit in the main castle"), Localization.New("server.app.unitwodid_must_identify_a.a810416b", "unitWodID must identify a probe unit in the main castle", nil))
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	units, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	if _, exists := units.Find(strconv.FormatInt(int64(request.UnitID), 10)); !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unit %d is not in the official catalog", request.UnitID), Localization.New("server.app.unit_p_is_not.3618fe57", "unit {p0} is not in the official catalog", Localization.Params{"p0": fmt.Sprintf("%d", request.UnitID)}))
	}
	booster, premiumTravel, err := resolveCastleHorseTravelBoostFields(
		input.GameData, source, request.HorseTravelBoostID,
	)
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("resolve Rift probe horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_rift_probe_horse.e8670ea8", "resolve Rift probe horse travel boost", nil), err))
	}
	availableUnits := source.Units.Stationed[request.UnitID]
	if availableUnits < maidenProbeCountPerFlank*3 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("main castle has %d of unit %d; at least %d are required", availableUnits, request.UnitID, maidenProbeCountPerFlank*3), Localization.New("server.app.main_castle_has_p.2a5228f9", "main castle has {p0} of unit {p1}; at least {p2} are required", Localization.Params{"p0": availableUnits, "p1": fmt.Sprintf("%d", request.UnitID), "p2": fmt.Sprintf("%d", maidenProbeCountPerFlank*3)}))
	}
	target, ok := riftTargetForKingdom(input.State, source.KingdomID)
	if maidenRun != nil {
		target, ok = input.State.LookupMapObservation(maidenRun.KingdomID, fmt.Sprintf("%d:%d", maidenRun.TargetX, maidenRun.TargetY))
		ok = ok && target.TypeID == riftMapTypeID
	}
	if !ok {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("the Rift map tile is unknown; refresh the surrounding map first"), Localization.New("server.app.the_rift_map_tile.5b5a905b", "the Rift map tile is unknown; refresh the surrounding map first", nil))
	}
	eligibleCommanders := maidenCandidateCommanders(input.State)
	if request.CommanderIDs != nil {
		allowed := make(map[State.CommanderID]struct{}, len(request.CommanderIDs))
		for _, commanderID := range request.CommanderIDs {
			allowed[commanderID] = struct{}{}
		}
		eligibleCommanders = slicesMatchingCommanders(eligibleCommanders, allowed)
	}
	maximumByStock := int(availableUnits / (maidenProbeCountPerFlank * 3))
	availableEligible := 0
	eligibleSet := make(map[State.CommanderID]struct{}, len(eligibleCommanders))
	for _, commanderID := range eligibleCommanders {
		eligibleSet[commanderID] = struct{}{}
		if input.State.Commanders[commanderID].Available {
			availableEligible++
		}
	}
	defaultCount := min(availableEligible, maximumByStock)
	if request.CommanderSelection == nil && defaultCount == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("no free commander has a shield-maiden relic in the supported effect range"), Localization.New("server.app.no_free_commander_has.16da28dd", "no free commander has a shield-maiden relic in the supported effect range", nil))
	}
	if request.CommanderSelection != nil {
		requestedCount := request.CommanderSelection.Count
		if requestedCount == 0 {
			requestedCount = 1
		}
		if requestedCount > maximumByStock {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("main castle probe stock supports %d commander(s), not %d", maximumByStock, requestedCount), Localization.New("server.app.main_castle_probe_stock.1ae52fc6", "main castle probe stock supports {p0} commander(s), not {p1}", Localization.Params{"p0": maximumByStock, "p1": requestedCount}))
		}
		if maidenRun != nil && requestedCount > maidenRun.RequestedAttacks-maidenRun.AttacksLaunched {
			return Intent.Plan{}, fmt.Errorf(
				"Rift Maiden run %s has only %d probe(s) remaining",
				maidenRun.ID, max(0, maidenRun.RequestedAttacks-maidenRun.AttacksLaunched),
			)
		}
		defaultCount = 1
	}
	resolution, err := resolveCRACommanders(input.State, request.CommanderSelection, craCommanderSelectionOptions{
		Holds:             input.CommanderHolds,
		DefaultCandidates: eligibleCommanders,
		DefaultCount:      defaultCount,
		Eligible:          eligibleSet,
		RequireAvailable:  true,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	steps, err := buildCRACommandSteps(
		source, resolution.Selected, "Launch Rift probe",
		func(commanderID State.CommanderID) (json.RawMessage, error) {
			body := maidenAttackBody(source.X, source.Y, target, commanderID, request.UnitID)
			body.Booster = booster
			body.PremiumTravel = premiumTravel
			return json.Marshal(body)
		},
		func(commanderID State.CommanderID) Intent.Step {
			return attackFeatureCaptureStep(attackFeatureCaptureRequest{
				FeatureID: State.AttackFeatureRiftMaiden, SourceCastleID: source.ID, CommanderID: commanderID,
				KingdomID: target.KingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
				RunID: request.RunID,
			})
		},
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "attack-inventory:" + castleID,
		"rift-launch:maiden-wave", "castle:" + castleID,
	}
	claimCommanders := resolution.Selected
	if request.CommanderSelection != nil {
		claimCommanders = resolution.Candidates
	}
	claims = append(claims, craCommanderClaims(claimCommanders)...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "riftMaiden",
			Affinity: "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Launch %d shield-maiden Rift probe(s)", len(resolution.Selected)), SummaryDescriptor: Localization.New("server.app.launch_p_shield_maiden.4174b767", "Launch {p0} shield-maiden Rift probe(s)", Localization.Params{"p0": len(resolution.Selected)}),
		Steps: steps,
	}, nil
}

type riftReplayRequest struct {
	LaunchID           string                        `json:"launchId"`
	CommanderID        *int64                        `json:"commanderID,omitempty"`
	CommanderSelection *craCommanderSelectionRequest `json:"commanderSelection,omitempty"`
	SourceCastle       State.CastleID                `json:"sourceCastleId,omitempty"`
	SourceX            *int                          `json:"sourceX,omitempty"`
	SourceY            *int                          `json:"sourceY,omitempty"`
	ArriveAt           int64                         `json:"arriveAtUnix,omitempty"`
	AttackSetup        *attackSetupRequest           `json:"attackSetup,omitempty"`
	HorseTravelBoostID *int                          `json:"horseTravelBoostId,omitempty"`
}

type attackSetupRequest struct {
	Name             string                      `json:"name,omitempty"`
	UseTroopFamilies bool                        `json:"useTroopFamilies,omitempty"`
	Waves            []attackSetupWaveRequest    `json:"waves"`
	CourtyardSupport attackSetupCourtyardSupport `json:"courtyardSupport"`
}

type attackSetupWaveRequest struct {
	Left   attackSetupLaneRequest `json:"L"`
	Middle attackSetupLaneRequest `json:"M"`
	Right  attackSetupLaneRequest `json:"R"`
}

type attackSetupLaneRequest struct {
	Troops []attackSetupSlotRequest `json:"troops"`
	Tools  []attackSetupSlotRequest `json:"tools"`
}

type attackSetupCourtyardSupport struct {
	Troops []attackSetupSlotRequest `json:"troops"`
	Tools  []attackSetupSlotRequest `json:"tools"`
}

type attackSetupSlotRequest struct {
	ItemID   *int64 `json:"itemId"`
	Quantity int64  `json:"quantity"`
}

type allianceTargetAttackRequest struct {
	SourceCastleID     State.CastleID       `json:"sourceCastleId"`
	KingdomID          State.KingdomID      `json:"kingdomId"`
	TargetX            int                  `json:"targetX"`
	TargetY            int                  `json:"targetY"`
	TargetPlayerID     State.PlayerID       `json:"targetPlayerId,omitempty"`
	TargetCastleID     int64                `json:"targetCastleId,omitempty"`
	TargetTypeID       int                  `json:"targetTypeId,omitempty"`
	TargetLevel        int                  `json:"targetLevel"`
	TargetLegendLevel  int                  `json:"targetLegendLevel,omitempty"`
	PreviewCommanderID *State.CommanderID   `json:"previewCommanderId,omitempty"`
	Preset             AttackPresets.Preset `json:"preset"`
}

type resolvedAllianceTargetAttackRequest struct {
	allianceTargetAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

func planAllianceTargetAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := allianceTargetAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	var commanderSelection *craCommanderSelectionRequest
	if request.PreviewCommanderID != nil {
		commanderSelection = &craCommanderSelectionRequest{
			Candidates: []State.CommanderID{*request.PreviewCommanderID}, Count: 1,
		}
	}
	resolution, err := resolveCRACommanders(input.State, commanderSelection, craCommanderSelectionOptions{
		Holds:        input.CommanderHolds,
		DefaultCount: 1, RequireAvailable: true,
	})
	if err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("select a free commander: %w", err), Localization.ErrorContext(Localization.New("server.app.select_a_free_commander.76a6223a", "select a free commander", nil), err))
	}
	commanderID := resolution.Selected[0]
	capacity, err := resolveAllianceTargetAttackCapacity(input, request, commanderID, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	if _, err := buildAttackSetup(invasionAttackSetup(limitedPreset), source, input.GameData); err != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("validate CRA-capped attack preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.validate_cra_capped_attack.988edd48", "validate CRA-capped attack preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	resolved := resolvedAllianceTargetAttackRequest{
		allianceTargetAttackRequest: request,
		CommanderID:                 commanderID,
	}
	resolvedArguments, _ := json.Marshal(resolved)
	contextPayload, _ := json.Marshal(struct {
		SourceX     int               `json:"SX"`
		SourceY     int               `json:"SY"`
		TargetX     int               `json:"TX"`
		TargetY     int               `json:"TY"`
		KingdomID   State.KingdomID   `json:"KID"`
		CommanderID State.CommanderID `json:"LID"`
	}{source.X, source.Y, request.TargetX, request.TargetY, request.KingdomID, commanderID})

	steps := make([]Intent.Step, 0, 4)
	if input.State.Player.LegendLevel > 0 && request.TargetLegendLevel > 0 &&
		(input.State.Player.LegendSkills.ObservedAt.IsZero() ||
			time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute) {
		steps = append(steps, contextCommandStep(
			"Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl",
		))
	}
	steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps, deferredCRACommandStep(
		"Build and launch alliance target attack", "alliance.target.attack.build", resolvedArguments, contextPayload, Localization.New("server.app.build_and_launch_alliance.2600bd7f", "Build and launch alliance target attack", nil),
	))
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		fmt.Sprintf("player-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY),
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{commanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: manualAllianceAttackModuleID, Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack player castle at %d:%d with %s", request.TargetX, request.TargetY, request.Preset.Name), SummaryDescriptor: Localization.New("server.app.attack_player_castle_at.d5d2ca05", "Attack player castle at {p0}:{p1} with {p2}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY, "p2": fmt.Sprintf("%s", request.Preset.Name)}),
		Steps: steps,
	}, nil
}

func allianceTargetAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (allianceTargetAttackRequest, State.CastleState, error) {
	var request allianceTargetAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return allianceTargetAttackRequest{}, State.CastleState{}, err
	}
	if input.State.Player.ProtectionMode.PreparingOrActive(time.Now().UTC()) {
		return allianceTargetAttackRequest{}, State.CastleState{},
			Localization.WithError(fmt.Errorf("player attacks are disabled while Protection Mode is preparing or active"), Localization.New("server.app.player_attacks_are_disabled.fb31aec6", "player attacks are disabled while Protection Mode is preparing or active", nil))
	}
	if request.SourceCastleID <= 0 || request.KingdomID != 0 || request.TargetX < 0 || request.TargetY < 0 {
		return allianceTargetAttackRequest{}, State.CastleState{},
			Localization.WithError(fmt.Errorf("alliance target attack requires a Great Empire source and valid target coordinates"), Localization.New("server.app.alliance_target_attack_requires.606b5ad1", "alliance target attack requires a Great Empire source and valid target coordinates", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != request.KingdomID {
		return allianceTargetAttackRequest{}, State.CastleState{},
			Localization.WithError(fmt.Errorf("source castle %d is not an owned Great Empire castle", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.d2605c80", "source castle {p0} is not an owned Great Empire castle", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if source.X == request.TargetX && source.Y == request.TargetY {
		return allianceTargetAttackRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("source and target castle cannot be the same"), Localization.New("server.app.source_and_target_castle.79947ddf", "source and target castle cannot be the same", nil))
	}
	if source.UnitsObservedAt.IsZero() {
		return allianceTargetAttackRequest{}, State.CastleState{},
			Localization.WithError(fmt.Errorf("source castle %d troop and tool inventory has not been observed", source.ID), Localization.New("server.app.source_castle_p_troop.0e32fbca", "source castle {p0} troop and tool inventory has not been observed", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
	}
	if strings.TrimSpace(request.Preset.Name) == "" {
		return allianceTargetAttackRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("attack preset name is required"), Localization.New("server.app.attack_preset_name_is.448b98c6", "attack preset name is required", nil))
	}
	if request.TargetTypeID <= 0 || request.TargetLevel <= 0 {
		return allianceTargetAttackRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("target castle type and player level are required"), Localization.New("server.app.target_castle_type_and.f7d4200d", "target castle type and player level are required", nil))
	}
	if request.TargetPlayerID > 0 {
		if request.TargetPlayerID == input.State.Player.ID {
			return allianceTargetAttackRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("the target belongs to the current player"), Localization.New("server.app.the_target_belongs_to.f2e72084", "the target belongs to the current player", nil))
		}
		for _, member := range input.State.Alliance.Members {
			if member.PlayerID == request.TargetPlayerID {
				return allianceTargetAttackRequest{}, State.CastleState{}, Localization.WithError(fmt.Errorf("the target is a current alliance member"), Localization.New("server.app.the_target_is_a.57603de6", "the target is a current alliance member", nil))
			}
		}
		if remaining := targetReturnProtectionSeconds(input.State, request.TargetPlayerID, time.Now().UTC()); remaining > 0 {
			return allianceTargetAttackRequest{}, State.CastleState{},
				Localization.WithError(fmt.Errorf("the target has %d seconds of return protection remaining", remaining), Localization.New("server.app.the_target_has_p.dcebde58", "the target has {p0} seconds of return protection remaining", Localization.Params{"p0": remaining}))
		}
	}
	return request, source, nil
}

func targetReturnProtectionSeconds(gameState State.GameState, playerID State.PlayerID, now time.Time) int {
	remaining := 0
	for _, alliance := range gameState.Alliances {
		elapsed := 0
		if !alliance.ObservedAt.IsZero() {
			elapsed = max(0, int(now.Sub(alliance.ObservedAt).Seconds()))
		}
		for _, member := range alliance.Members {
			if member.PlayerID == playerID {
				remaining = max(remaining, member.ReturnProtectionSec-elapsed)
			}
		}
	}
	return max(0, remaining)
}

func (application *Application) resolveAllianceTargetAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var resolved resolvedAllianceTargetAttackRequest
	if err := decodeIntentArguments(arguments, &resolved); err != nil {
		return Intent.Step{}, err
	}
	request, source, err := allianceTargetAttackContext(input, mustMarshalAllianceTargetAttackRequest(resolved.allianceTargetAttackRequest))
	if err != nil {
		return Intent.Step{}, err
	}
	commander, exists := input.State.Commanders[resolved.CommanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(input.State, resolved.CommanderID, time.Now().UTC()) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: commander %d is no longer available", Intent.ErrPlanStale, resolved.CommanderID), Localization.New("server.app.intent_plan_became_stale.f815ae9b", "intent plan became stale before dispatch: commander {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", resolved.CommanderID)}))
	}
	dialog := input.State.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != request.KingdomID ||
		dialog.Target.X != request.TargetX || dialog.Target.Y != request.TargetY || dialog.Target.TypeID <= 0 {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("current attack dialog does not match player target %d:%d", request.TargetX, request.TargetY), Localization.New("server.app.current_attack_dialog_does.3d489a2a", "current attack dialog does not match player target {p0}:{p1}", Localization.Params{"p0": request.TargetX, "p1": request.TargetY}))
	}
	if request.TargetTypeID > 0 && dialog.Target.TypeID != request.TargetTypeID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: target %d:%d changed castle type", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.37c55b55", "intent plan became stale before dispatch: target {p1}:{p2} changed castle type", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if request.TargetCastleID > 0 && dialog.Target.ObjectID > 0 && dialog.Target.ObjectID != request.TargetCastleID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: target %d:%d changed castle identity", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.ed3b7efd", "intent plan became stale before dispatch: target {p1}:{p2} changed castle identity", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if request.TargetPlayerID > 0 && dialog.Target.OwnerID > 0 && dialog.Target.OwnerID != request.TargetPlayerID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: target %d:%d changed owner", Intent.ErrPlanStale, request.TargetX, request.TargetY), Localization.New("server.app.intent_plan_became_stale.c0eb37ca", "intent plan became stale before dispatch: target {p1}:{p2} changed owner", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	capacity, err := resolveAllianceTargetAttackCapacity(input, request, resolved.CommanderID, true)
	if err != nil {
		return Intent.Step{}, err
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	built, err := buildAttackSetup(invasionAttackSetup(limitedPreset), source, input.GameData)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build attack preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_attack_preset_p.5e695b86", "build attack preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	target := State.MapObservation{
		KingdomID: request.KingdomID, X: request.TargetX, Y: request.TargetY,
		TypeID: dialog.Target.TypeID, ObjectID: dialog.Target.ObjectID, OwnerID: dialog.Target.OwnerID,
	}
	body, err := json.Marshal(invasionAttackBody(source, target, resolved.CommanderID, built))
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build player attack payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_player_attack_payload.205b53d5", "build player attack payload", nil), err))
	}
	return commandStep(fmt.Sprintf("Attack player castle at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_player_castle_at.6334a4ea", "Attack player castle at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y})), nil
}

func resolveAllianceTargetAttackCapacity(
	input Intent.PlanningContext,
	request allianceTargetAttackRequest,
	commanderID State.CommanderID,
	useAttackDialogEffects bool,
) (AttackCapacity.Result, error) {
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID:         request.SourceCastleID,
		CommanderID:            commanderID,
		UseAttackDialogEffects: useAttackDialogEffects,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("alliance-target:%d:%d:%d", request.KingdomID, request.TargetX, request.TargetY),
			Map: &AttackCapacity.MapTarget{
				KingdomID: request.KingdomID, TypeID: request.TargetTypeID,
				X: request.TargetX, Y: request.TargetY, ObjectID: request.TargetCastleID, Level: request.TargetLevel,
			},
			Level: request.TargetLevel, CastleTypeID: request.TargetTypeID, PvP: true,
			LegendaryFight: input.State.Player.LegendLevel > 0 && request.TargetLegendLevel > 0,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, Localization.WithError(fmt.Errorf("resolve player attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_player_attack_capacity.6e109f99", "resolve player attack capacity", nil), err))
	}
	return capacity, nil
}

func mustMarshalAllianceTargetAttackRequest(request allianceTargetAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

func (application *Application) planRiftReplay(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request riftReplayRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.CommanderID != nil && request.CommanderSelection != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("commanderID and commanderSelection are mutually exclusive"), Localization.New("server.app.commanderid_and_commanderselection_are.5cb57f03", "commanderID and commanderSelection are mutually exclusive", nil))
	}
	if request.HorseTravelBoostID != nil {
		if err := validateHorseTravelBoostID(*request.HorseTravelBoostID); err != nil {
			return Intent.Plan{}, err
		}
	}
	request.LaunchID = strings.TrimSpace(request.LaunchID)
	if request.LaunchID == "" {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("launchId is required"), Localization.New("server.app.launchid_is_required.b10a5cbb", "launchId is required", nil))
	}
	launch, configured, err := application.riftReplayLaunch(input.State, request.LaunchID)
	if err != nil {
		return Intent.Plan{}, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(launch.Body, &fields) != nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift launch %q has an invalid command body", request.LaunchID), Localization.New("server.app.rift_launch_p_has.b09d26bb", "Rift launch {p0} has an invalid command body", Localization.Params{"p0": fmt.Sprintf("%q", request.LaunchID)}))
	}
	if request.CommanderID != nil {
		fields["LID"], _ = json.Marshal(*request.CommanderID)
	}
	if request.SourceCastle > 0 {
		source, err := sourceCastle(input.State, request.SourceCastle)
		if err != nil {
			return Intent.Plan{}, err
		}
		fields["SX"], _ = json.Marshal(source.X)
		fields["SY"], _ = json.Marshal(source.Y)
	}
	if request.SourceX != nil {
		fields["SX"], _ = json.Marshal(*request.SourceX)
	}
	if request.SourceY != nil {
		fields["SY"], _ = json.Marshal(*request.SourceY)
	}
	now := time.Now().UTC()
	fireAt, normalizedArrival, scheduled, err := riftReplayTiming(launch, request.ArriveAt, now)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolution, err := resolveCRACommanders(input.State, request.CommanderSelection, craCommanderSelectionOptions{
		Holds:             input.CommanderHolds,
		DefaultCandidates: []State.CommanderID{State.CommanderID(rawMapInt(fields, "LID"))},
		DefaultCount:      1,
		RequireAvailable:  !scheduled,
	})
	if err != nil {
		return Intent.Plan{}, err
	}
	claims := []string{"rift-launch:" + request.LaunchID}
	if request.AttackSetup != nil {
		source, err := sourceCastle(input.State, request.SourceCastle)
		if err != nil {
			return Intent.Plan{}, err
		}
		built, err := buildAttackSetupForCommanders(*request.AttackSetup, source, input.GameData, len(resolution.Selected))
		if err != nil {
			return Intent.Plan{}, err
		}
		fields["A"], _ = json.Marshal(built.Waves)
		fields["AST"], _ = json.Marshal(built.SupportTools)
		fields["RW"], _ = json.Marshal(built.SupportTroops)
		fields["ASCT"] = json.RawMessage(`0`)
		fields["SX"], _ = json.Marshal(source.X)
		fields["SY"], _ = json.Marshal(source.Y)
		claims = append(claims,
			"castle:"+strconv.FormatInt(int64(source.ID), 10),
			"attack-inventory:"+strconv.FormatInt(int64(source.ID), 10),
		)
	} else if len(resolution.Selected) > 1 {
		source, err := riftReplaySourceCastle(input.State, request.SourceCastle, fields)
		if err != nil {
			return Intent.Plan{}, err
		}
		if err := validateRepeatedAttackInventory(fields, source, len(resolution.Selected)); err != nil {
			return Intent.Plan{}, err
		}
		claims = append(claims,
			"castle:"+strconv.FormatInt(int64(source.ID), 10),
			"attack-inventory:"+strconv.FormatInt(int64(source.ID), 10),
		)
	}
	var source State.CastleState
	if configured || !scheduled {
		source, err = riftReplaySourceCastle(input.State, request.SourceCastle, fields)
		if err != nil {
			return Intent.Plan{}, err
		}
	}
	if configured && !scheduled {
		if err := validateRepeatedAttackInventory(fields, source, len(resolution.Selected)); err != nil {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("configured Rift launch %q inventory: %w", request.LaunchID, err), Localization.ErrorContext(Localization.New("server.app.configured_rift_launch_p.afef4c45", "configured Rift launch {p0} inventory", Localization.Params{"p0": fmt.Sprintf("%q", request.LaunchID)}), err))
		}
	}
	if scheduled {
		immediateRequest := request
		immediateRequest.ArriveAt = 0
		immediateArguments, _ := json.Marshal(immediateRequest)
		schedule, _ := json.Marshal(Scheduling.Request{
			ID: "rift:" + request.LaunchID, Intent: "rift.launch.replay", Actor: "scheduler:rift",
			Arguments: immediateArguments, ExecuteAt: time.Unix(fireAt, 0).UTC(),
		})
		return Intent.Plan{
			Claims:  []string{"scheduled-operation:rift:" + request.LaunchID},
			Summary: fmt.Sprintf("Schedule Rift launch %s for %s", request.LaunchID, time.Unix(normalizedArrival, 0).Format(time.RFC3339)), SummaryDescriptor: Localization.New("server.app.schedule_rift_launch_p.3e3e36f7", "Schedule Rift launch {p0} for {p1}", Localization.Params{"p0": fmt.Sprintf("%s", request.LaunchID), "p1": fmt.Sprintf("%s", time.Unix(normalizedArrival, 0).Format(time.RFC3339))}),
			Steps: []Intent.Step{{Name: "Schedule Rift replay", Action: "operation.schedule", ActionArguments: schedule}},
		}, nil
	}
	if request.HorseTravelBoostID != nil {
		booster, premiumTravel, err := resolveCastleHorseTravelBoostFields(
			input.GameData, source, *request.HorseTravelBoostID,
		)
		if err != nil {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("resolve Rift replay horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_rift_replay_horse.a4873101", "resolve Rift replay horse travel boost", nil), err))
		}
		fields["HBW"], _ = json.Marshal(booster)
		fields["PTT"], _ = json.Marshal(premiumTravel)
	}
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims = append(claims, "castle-focus", "castle:"+castleID, "attack-inventory:"+castleID)
	steps, err := buildCRACommandSteps(
		source, resolution.Selected, "Replay Rift launch",
		func(commanderID State.CommanderID) (json.RawMessage, error) {
			return craPayloadWithCommander(fields, commanderID)
		},
		func(commanderID State.CommanderID) Intent.Step {
			return attackFeatureCaptureStep(attackFeatureCaptureRequest{
				FeatureID: State.AttackFeatureRiftReplay, SourceCastleID: source.ID, CommanderID: commanderID,
				KingdomID: State.KingdomID(rawMapInt(fields, "KID")), TargetTypeID: riftMapTypeID,
				TargetX: int(rawMapInt(fields, "TX")), TargetY: int(rawMapInt(fields, "TY")),
			})
		},
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	claims = append(claims, "attack-context")
	claims = append(claims, craCommanderClaims(resolution.Candidates)...)
	summary := fmt.Sprintf("Replay Rift launch %s with commander %d", request.LaunchID, resolution.Selected[0])
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.replay_rift_launch_p.f052376f", "Replay Rift launch {p0} with commander {p1, number}", Localization.Params{"p0": fmt.Sprintf("%s", request.LaunchID), "p1": resolution.Selected[0]})
	if len(resolution.Selected) > 1 {
		summary = fmt.Sprintf("Replay Rift launch %s with %d commanders", request.LaunchID, len(resolution.Selected))
		summaryLocalizationMessage = Localization.New("server.app.replay_rift_launch_p.b707aa32", "Replay Rift launch {p0} with {p1, number} commanders", Localization.Params{"p0": fmt.Sprintf("%s", request.LaunchID), "p1": len(resolution.Selected)})
	}
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "riftReplay",
			Affinity: "castle:" + castleID,
		},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: steps,
	}, nil
}

func riftReplayTiming(launch State.RiftLaunch, arriveAt int64, now time.Time) (int64, int64, bool, error) {
	if arriveAt <= now.Unix()+30 {
		return 0, 0, false, nil
	}
	if launch.OneWayTTSeconds <= 0 {
		return 0, 0, false, Localization.WithError(fmt.Errorf("Rift launch %q has no observed one-way travel time", launch.ID), Localization.New("server.app.rift_launch_p_has.4211ad84", "Rift launch {p0} has no observed one-way travel time", Localization.Params{"p0": fmt.Sprintf("%q", launch.ID)}))
	}
	minimumArrival := roundUpUnixMinute(now.Unix() + int64(launch.OneWayTTSeconds))
	normalizedArrival := roundUpUnixMinute(arriveAt)
	if normalizedArrival <= minimumArrival {
		return 0, normalizedArrival, false, nil
	}
	return normalizedArrival - int64(launch.OneWayTTSeconds), normalizedArrival, true, nil
}

type builtAttackSetup struct {
	Waves         []attackWave
	SupportTroops []attackPair
	SupportTools  []int64
}

func buildAttackSetup(setup attackSetupRequest, source State.CastleState, gameData *GameData.Store) (builtAttackSetup, error) {
	return buildAttackSetupForCommanders(setup, source, gameData, 1)
}

func buildAttackSetupForCommanders(
	setup attackSetupRequest,
	source State.CastleState,
	gameData *GameData.Store,
	copies int,
) (builtAttackSetup, error) {
	if copies < 1 {
		return builtAttackSetup{}, Localization.WithError(fmt.Errorf("attack setup requires at least one commander"), Localization.New("server.app.attack_setup_requires_at.53bfcc84", "attack setup requires at least one commander", nil))
	}
	if setup.UseTroopFamilies {
		resolved, err := AttackPresets.ResolveTroopFamilies(
			attackPresetFromSetup(setup), source.Units.Stationed, gameData, copies,
		)
		if err != nil {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("resolve attack troop families: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_attack_troop_families.b612d38c", "resolve attack troop families", nil), err))
		}
		setup = invasionAttackSetup(resolved)
	}
	if len(setup.Waves) < 1 || len(setup.Waves) > AttackPresets.MaximumWaves {
		return builtAttackSetup{}, Localization.WithError(fmt.Errorf("attack setup must contain between 1 and %d waves", AttackPresets.MaximumWaves), Localization.New("server.app.attack_setup_must_contain.684dd56c", "attack setup must contain between 1 and {p0} waves", Localization.Params{"p0": AttackPresets.MaximumWaves}))
	}
	requested := map[State.UnitID]int64{}
	unitTotal := int64(0)
	waves := make([]attackWave, 0, len(setup.Waves))
	for waveIndex, wave := range setup.Waves {
		left, units, err := buildAttackSetupLane(wave.Left, 2, 2, gameData, requested)
		if err != nil {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("wave %d left flank: %w", waveIndex+1, err), Localization.ErrorContext(Localization.New("server.app.wave_p_left_flank.d94b672b", "wave {p0} left flank", Localization.Params{"p0": waveIndex + 1}), err))
		}
		unitTotal += units
		middle, units, err := buildAttackSetupLane(wave.Middle, 6, 3, gameData, requested)
		if err != nil {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("wave %d middle flank: %w", waveIndex+1, err), Localization.ErrorContext(Localization.New("server.app.wave_p_middle_flank.279d128f", "wave {p0} middle flank", Localization.Params{"p0": waveIndex + 1}), err))
		}
		unitTotal += units
		right, units, err := buildAttackSetupLane(wave.Right, 2, 2, gameData, requested)
		if err != nil {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("wave %d right flank: %w", waveIndex+1, err), Localization.ErrorContext(Localization.New("server.app.wave_p_right_flank.eaa7fbf1", "wave {p0} right flank", Localization.Params{"p0": waveIndex + 1}), err))
		}
		unitTotal += units
		waves = append(waves, attackWave{Left: left, Middle: middle, Right: right})
	}
	if unitTotal <= 0 {
		return builtAttackSetup{}, Localization.WithError(fmt.Errorf("attack setup must allocate at least one troop"), Localization.New("server.app.attack_setup_must_allocate.9bb9c58e", "attack setup must allocate at least one troop", nil))
	}
	supportTroops, _, err := buildAttackSetupPairs(
		setup.CourtyardSupport.Troops,
		AttackPresets.CourtyardTroopSlots,
		"units",
		gameData,
		requested,
	)
	if err != nil {
		return builtAttackSetup{}, Localization.WithError(fmt.Errorf("courtyard support troops: %w", err), Localization.ErrorContext(Localization.New("server.app.courtyard_support_troops.2e5158e9", "courtyard support troops", nil), err))
	}
	supportTools, err := buildAttackSupportTools(setup.CourtyardSupport.Tools, gameData, requested)
	if err != nil {
		return builtAttackSetup{}, Localization.WithError(fmt.Errorf("courtyard support tools: %w", err), Localization.ErrorContext(Localization.New("server.app.courtyard_support_tools.5004e58f", "courtyard support tools", nil), err))
	}
	for id, amount := range requested {
		if amount > math.MaxInt64/int64(copies) {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("attack setup quantity for item %d is too large", id), Localization.New("server.app.attack_setup_quantity_for.7083f89f", "attack setup quantity for item {p0} is too large", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
		}
		required := amount * int64(copies)
		available := source.Units.Stationed[id]
		if required > available {
			return builtAttackSetup{}, Localization.WithError(fmt.Errorf("castle %d has %d of item %d; %d commander(s) require %d", source.ID, available, id, copies, required), Localization.New("server.app.castle_p_has_p.310da036", "castle {p0} has {p1} of item {p2}; {p3} commander(s) require {p4}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": available, "p2": fmt.Sprintf("%d", id), "p3": copies, "p4": required}))
		}
	}
	return builtAttackSetup{Waves: waves, SupportTroops: supportTroops, SupportTools: supportTools}, nil
}

func attackPresetFromSetup(setup attackSetupRequest) AttackPresets.Preset {
	convertSlots := func(slots []attackSetupSlotRequest) []AttackPresets.Slot {
		result := make([]AttackPresets.Slot, len(slots))
		for index, slot := range slots {
			result[index] = AttackPresets.Slot{ItemID: slot.ItemID, Quantity: slot.Quantity}
		}
		return result
	}
	convertLane := func(lane attackSetupLaneRequest) AttackPresets.Lane {
		return AttackPresets.Lane{Troops: convertSlots(lane.Troops), Tools: convertSlots(lane.Tools)}
	}
	waves := make([]AttackPresets.Wave, len(setup.Waves))
	for index, wave := range setup.Waves {
		waves[index] = AttackPresets.Wave{
			Left: convertLane(wave.Left), Middle: convertLane(wave.Middle), Right: convertLane(wave.Right),
		}
	}
	return AttackPresets.Preset{
		ID: "runtime", Name: setup.Name, UseTroopFamilies: setup.UseTroopFamilies, Waves: waves,
		CourtyardSupport: AttackPresets.CourtyardSupport{
			Troops: convertSlots(setup.CourtyardSupport.Troops),
			Tools:  convertSlots(setup.CourtyardSupport.Tools),
		},
	}
}

func buildAttackSetupLane(
	lane attackSetupLaneRequest,
	unitCapacity int,
	toolCapacity int,
	gameData *GameData.Store,
	requested map[State.UnitID]int64,
) (attackFlank, int64, error) {
	units, unitTotal, err := buildAttackSetupPairs(lane.Troops, unitCapacity, "units", gameData, requested)
	if err != nil {
		return attackFlank{}, 0, err
	}
	tools, _, err := buildAttackSetupPairs(lane.Tools, toolCapacity, "tools", gameData, requested)
	if err != nil {
		return attackFlank{}, 0, err
	}
	return attackFlank{Units: units, Tools: tools}, unitTotal, nil
}

func buildAttackSetupPairs(
	slots []attackSetupSlotRequest,
	capacity int,
	collection string,
	gameData *GameData.Store,
	requested map[State.UnitID]int64,
) ([]attackPair, int64, error) {
	if len(slots) > capacity {
		return nil, 0, Localization.WithError(fmt.Errorf("%s has %d slots; at most %d are supported", collection, len(slots), capacity), Localization.New("server.app.p_has_p_slots.e2290b41", "{p0} has {p1} slots; at most {p2} are supported", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": len(slots), "p2": capacity}))
	}
	empty := attackPair{-1, 0}
	result := make([]attackPair, capacity)
	for index := range result {
		result[index] = empty
	}
	total := int64(0)
	for index, slot := range slots {
		if slot.Quantity < 0 {
			return nil, 0, Localization.WithError(fmt.Errorf("%s slot %d has a negative quantity", collection, index+1), Localization.New("server.app.p_slot_p_has.06cfe83f", "{p0} slot {p1} has a negative quantity", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": index + 1}))
		}
		if slot.ItemID == nil {
			if slot.Quantity > 0 {
				return nil, 0, Localization.WithError(fmt.Errorf("%s slot %d has a quantity without an item", collection, index+1), Localization.New("server.app.p_slot_p_has.758b1073", "{p0} slot {p1} has a quantity without an item", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": index + 1}))
			}
			continue
		}
		if slot.Quantity == 0 {
			continue
		}
		if *slot.ItemID <= 0 {
			return nil, 0, Localization.WithError(fmt.Errorf("%s slot %d has an invalid item", collection, index+1), Localization.New("server.app.p_slot_p_has.4c56e9a7", "{p0} slot {p1} has an invalid item", Localization.Params{"p0": fmt.Sprintf("%s", collection), "p1": index + 1}))
		}
		if err := requireOfficialDefinition(gameData, collection, *slot.ItemID); err != nil {
			return nil, 0, err
		}
		if collection == "tools" {
			supportTool, err := isSceatAttackSupportTool(gameData, *slot.ItemID)
			if err != nil {
				return nil, 0, err
			}
			if supportTool {
				return nil, 0, Localization.WithError(fmt.Errorf("tool definition %d belongs in a courtyard Sceat support slot", *slot.ItemID), Localization.New("server.app.tool_definition_p_belongs.c90f0ce3", "tool definition {p0} belongs in a courtyard Sceat support slot", Localization.Params{"p0": fmt.Sprintf("%d", *slot.ItemID)}))
			}
		}
		id := State.UnitID(*slot.ItemID)
		requested[id] += slot.Quantity
		total += slot.Quantity
		result[index] = attackPair{*slot.ItemID, slot.Quantity}
	}
	return result, total, nil
}

func buildAttackSupportTools(
	slots []attackSetupSlotRequest,
	gameData *GameData.Store,
	requested map[State.UnitID]int64,
) ([]int64, error) {
	if len(slots) > AttackPresets.CourtyardToolSlots {
		return nil, Localization.WithError(fmt.Errorf("tools has %d slots; at most %d are supported", len(slots), AttackPresets.CourtyardToolSlots), Localization.New("server.app.tools_has_p_slots.41c4f36a", "tools has {p0} slots; at most {p1} are supported", Localization.Params{"p0": len(slots), "p1": AttackPresets.CourtyardToolSlots}))
	}
	result := make([]int64, AttackPresets.CourtyardToolSlots)
	for index := range result {
		result[index] = -1
	}
	for index, slot := range slots {
		if slot.ItemID == nil {
			if slot.Quantity != 0 {
				return nil, Localization.WithError(fmt.Errorf("tool slot %d has a quantity without an item", index+1), Localization.New("server.app.tool_slot_p_has.842f12f6", "tool slot {p0} has a quantity without an item", Localization.Params{"p0": index + 1}))
			}
			continue
		}
		if *slot.ItemID <= 0 || slot.Quantity != 1 {
			return nil, Localization.WithError(fmt.Errorf("tool slot %d must contain exactly one valid item", index+1), Localization.New("server.app.tool_slot_p_must.406d6bd2", "tool slot {p0} must contain exactly one valid item", Localization.Params{"p0": index + 1}))
		}
		if err := requireSceatAttackSupportTool(gameData, *slot.ItemID); err != nil {
			return nil, err
		}
		id := State.UnitID(*slot.ItemID)
		requested[id]++
		result[index] = *slot.ItemID
	}
	return result, nil
}

func requireSceatAttackSupportTool(gameData *GameData.Store, id int64) error {
	if err := requireOfficialDefinition(gameData, "tools", id); err != nil {
		return err
	}
	supportTool, err := isSceatAttackSupportTool(gameData, id)
	if err != nil {
		return err
	}
	if !supportTool {
		return Localization.WithError(fmt.Errorf("tool definition %d is not an official Sceat attack support tool", id), Localization.New("server.app.tool_definition_p_is.45259bd2", "tool definition {p0} is not an official Sceat attack support tool", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
	}
	return nil
}

func isSceatAttackSupportTool(gameData *GameData.Store, id int64) (bool, error) {
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return false, err
	}
	raw, exists := catalog.Find(strconv.FormatInt(id, 10))
	if !exists {
		return false, Localization.WithError(fmt.Errorf("tool definition %d is not in the current official catalog", id), Localization.New("server.app.tool_definition_p_is.e6323455", "tool definition {p0} is not in the current official catalog", Localization.Params{"p0": fmt.Sprintf("%d", id)}))
	}
	record, err := GameData.DecodeRecord(raw)
	if err != nil {
		return false, Localization.WithError(fmt.Errorf("decode tool definition %d: %w", id, err), Localization.ErrorContext(Localization.New("server.app.decode_tool_definition_p.69cf8148", "decode tool definition {p0}", Localization.Params{"p0": fmt.Sprintf("%d", id)}), err))
	}
	itemType, _ := record.String("type")
	return strings.HasPrefix(itemType, "SceatSuppAtt"), nil
}

func planRiftTemplateRename(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LaunchID    string `json:"launchId"`
		DisplayName string `json:"displayName"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.LaunchID = strings.TrimSpace(request.LaunchID)
	request.DisplayName = strings.TrimSpace(request.DisplayName)
	if _, exists := input.State.Rift.Launches[request.LaunchID]; !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift launch %q was not found", request.LaunchID), Localization.New("server.app.rift_launch_p_was.41bd7b2a", "Rift launch {p0} was not found", Localization.Params{"p0": fmt.Sprintf("%q", request.LaunchID)}))
	}
	if len(request.DisplayName) > 80 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift template names may contain at most 80 characters"), Localization.New("server.app.rift_template_names_may.2166d38e", "Rift template names may contain at most 80 characters", nil))
	}
	canonical, _ := json.Marshal(request)
	return Intent.Plan{
		Claims: []string{"rift-launch:" + request.LaunchID}, Summary: "Rename Rift launch " + request.LaunchID,
		Steps: []Intent.Step{{Name: "Rename Rift template", Action: "rift.template.rename", ActionArguments: canonical}},
	}, nil
}

func planRiftTemplateDelete(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		LaunchID string `json:"launchId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.LaunchID = strings.TrimSpace(request.LaunchID)
	if _, exists := input.State.Rift.Launches[request.LaunchID]; !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("Rift launch %q was not found", request.LaunchID), Localization.New("server.app.rift_launch_p_was.41bd7b2a", "Rift launch {p0} was not found", Localization.Params{"p0": fmt.Sprintf("%q", request.LaunchID)}))
	}
	canonical, _ := json.Marshal(request)
	cancel, _ := json.Marshal(map[string]string{"id": "rift:" + request.LaunchID})
	return Intent.Plan{
		Claims:  []string{"rift-launch:" + request.LaunchID, "scheduled-operation:rift:" + request.LaunchID},
		Summary: "Delete Rift launch " + request.LaunchID,
		Steps: []Intent.Step{
			{Name: "Delete Rift template", Action: "rift.template.delete", ActionArguments: canonical},
			{Name: "Cancel scheduled replay", Action: "operation.cancel", ActionArguments: cancel},
		},
	}, nil
}

func (application *Application) renameRiftTemplate(ctx context.Context, arguments json.RawMessage) error {
	var request struct {
		LaunchID    string `json:"launchId"`
		DisplayName string `json:"displayName"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentRift), func(gameState *State.GameState) ([]string, bool, error) {
		launch, exists := gameState.Rift.Launches[request.LaunchID]
		if !exists {
			return nil, false, Localization.WithError(fmt.Errorf("Rift launch %q was not found", request.LaunchID), Localization.New("server.app.rift_launch_p_was.41bd7b2a", "Rift launch {p0} was not found", Localization.Params{"p0": fmt.Sprintf("%q", request.LaunchID)}))
		}
		name := strings.TrimSpace(request.DisplayName)
		if launch.DisplayName == name {
			return nil, false, nil
		}
		launch.DisplayName = name
		gameState.Rift.Launches[request.LaunchID] = launch
		return []string{"rift"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) deleteRiftTemplate(ctx context.Context, arguments json.RawMessage) error {
	var request struct {
		LaunchID string `json:"launchId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentRift), func(gameState *State.GameState) ([]string, bool, error) {
		if _, exists := gameState.Rift.Launches[request.LaunchID]; !exists {
			return nil, false, nil
		}
		if gameState.Rift.DeletedLaunchIDs == nil {
			gameState.Rift.DeletedLaunchIDs = map[string]int64{}
		}
		gameState.Rift.DeletedLaunchIDs[request.LaunchID] = time.Now().UTC().UnixMilli()
		delete(gameState.Rift.Launches, request.LaunchID)
		if gameState.Rift.PendingLaunchID == request.LaunchID {
			gameState.Rift.PendingLaunchID = ""
		}
		return []string{"rift"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func roundUpUnixMinute(value int64) int64 {
	if value <= 0 {
		return 0
	}
	return ((value + 59) / 60) * 60
}

func planDecorationPreset(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID  State.CastleID  `json:"castleId"`
		KingdomID State.KingdomID `json:"kingdomId,omitempty"`
		PresetID  string          `json:"presetId"`
		Items     []struct {
			WID   State.BuildingID `json:"wid"`
			X     int              `json:"x"`
			Y     int              `json:"y"`
			R     int              `json:"r"`
			Layer string           `json:"layer,omitempty"`
		} `json:"items"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d is not in the current player state", request.CastleID), Localization.New("server.app.castle_p_is_not.47524bcb", "castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.CastleID)}))
	}
	if len(request.Items) > 500 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("decoration presets may contain at most 500 placements"), Localization.New("server.app.decoration_presets_may_contain.27d200a4", "decoration presets may contain at most 500 placements", nil))
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	buildings, err := input.GameData.Catalog("buildings")
	if err != nil {
		return Intent.Plan{}, err
	}
	for _, item := range request.Items {
		if item.WID <= 0 || item.X < 0 || item.Y < 0 || item.R < 0 || item.R > 3 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("preset %q contains an invalid placement", request.PresetID), Localization.New("server.app.preset_p_contains_an.196db00f", "preset {p0} contains an invalid placement", Localization.Params{"p0": fmt.Sprintf("%q", request.PresetID)}))
		}
		raw, found := buildings.Find(strconv.FormatInt(int64(item.WID), 10))
		if !found {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("building definition %d is not in the official catalog", item.WID), Localization.New("server.app.building_definition_p_is.fb62e47f", "building definition {p0} is not in the official catalog", Localization.Params{"p0": fmt.Sprintf("%d", item.WID)}))
		}
		record, _ := GameData.DecodeRecord(raw)
		if !officialDecoration(record) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("building definition %d is not an official decoration", item.WID), Localization.New("server.app.building_definition_p_is.e197bb1c", "building definition {p0} is not an official decoration", Localization.Params{"p0": fmt.Sprintf("%d", item.WID)}))
		}
	}
	matched := make([]bool, len(request.Items))
	remove := make([]State.BuildingInstanceID, 0)
	for instanceID, building := range castle.Buildings {
		raw, found := buildings.Find(strconv.FormatInt(int64(building.DefinitionID), 10))
		if !found {
			continue
		}
		record, _ := GameData.DecodeRecord(raw)
		if !officialDecoration(record) {
			continue
		}
		match := -1
		for index, item := range request.Items {
			if !matched[index] && item.WID == building.DefinitionID && item.X == building.GridX && item.Y == building.GridY && item.R == building.Rotation {
				match = index
				break
			}
		}
		if match >= 0 {
			matched[match] = true
		} else {
			remove = append(remove, instanceID)
		}
	}
	sort.Slice(remove, func(left, right int) bool { return remove[left] < remove[right] })
	steps := castleContextSteps(input, castle)
	if len(remove) > 0 || unmatchedCount(matched) > 0 {
		steps = append(steps, Intent.RebuildOnResume(Intent.Step{
			Name: "Refresh decoration storage", NameDescriptor: Localization.New("server.app.refresh_decoration_storage.0518abd6", "Refresh decoration storage", nil), Opcode: "sin", AwaitOpcode: "sin", TimeoutMillis: 10_000,
			SuccessCodes: []int{0}, Command: Protocol.Command{Opcode: "sin", Bare: true},
		}))
	}
	for _, instanceID := range remove {
		payload, _ := json.Marshal(struct {
			CastleID   State.CastleID           `json:"CID"`
			InstanceID State.BuildingInstanceID `json:"OID"`
		}{castle.ID, instanceID})
		steps = append(steps, commandStep(fmt.Sprintf("Store decoration %d", instanceID), "sob", payload, "sob", Localization.New("server.app.store_decoration_p.d7e8519c", "Store decoration {p0}", Localization.Params{"p0": fmt.Sprintf("%d", instanceID)})))
	}
	for index, item := range request.Items {
		if matched[index] {
			continue
		}
		payload, _ := json.Marshal(struct {
			WID   State.BuildingID `json:"WID"`
			X     int              `json:"X"`
			Y     int              `json:"Y"`
			R     int              `json:"R"`
			Power int              `json:"PWR"`
			Order int              `json:"PO"`
			Owner int              `json:"DOID"`
		}{item.WID, item.X, item.Y, item.R, 0, -1, -1})
		steps = append(steps, commandStep(fmt.Sprintf("Place decoration %d", item.WID), "ebu", payload, "ebu", Localization.New("server.app.place_decoration_p.660e60ec", "Place decoration {p0}", Localization.Params{"p0": fmt.Sprintf("%d", item.WID)})))
	}
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10), "decoration-layout"},
		Summary: fmt.Sprintf("Apply decoration preset %s to %s (%d removals, %d placements)", request.PresetID, castleLabel(castle), len(remove), unmatchedCount(matched)), SummaryDescriptor: Localization.New("server.app.apply_decoration_preset_p.fa297770", "Apply decoration preset {p0} to {p1} ({p2} removals, {p3} placements)", Localization.Params{"p0": fmt.Sprintf("%s", request.PresetID), "p1": fmt.Sprintf("%s", castleLabel(castle)), "p2": len(remove), "p3": unmatchedCount(matched)}),
		Steps: steps,
	}, nil
}

func sourceCastle(state State.GameState, requested State.CastleID) (State.CastleState, error) {
	if requested > 0 {
		castle, ok := state.Castles[requested]
		if !ok {
			return State.CastleState{}, Localization.WithError(fmt.Errorf("source castle %d is not owned by the current player", requested), Localization.New("server.app.source_castle_p_is.5c42ab21", "source castle {p0} is not owned by the current player", Localization.Params{"p0": requested}))
		}
		return castle, nil
	}
	for _, castle := range state.Castles {
		if castle.KingdomID == 0 && castle.SlotType == 1 {
			return castle, nil
		}
	}
	return State.CastleState{}, Localization.WithError(fmt.Errorf("the main castle is not known"), Localization.New("server.app.the_main_castle_is.0e1a4da3", "the main castle is not known", nil))
}

func riftTargetForKingdom(state State.GameState, kingdomID State.KingdomID) (State.MapObservation, bool) {
	var target State.MapObservation
	found := false
	state.RangeMapObservationsByKind(kingdomID, State.MapProjectionRift, func(_ string, observation State.MapObservation) bool {
		if observation.TypeID == riftMapTypeID {
			target, found = observation, true
			return false
		}
		return true
	})
	return target, found
}

func eligibleMaidenCommanders(state State.GameState) []State.CommanderID {
	candidates := maidenCandidateCommanders(state)
	result := make([]State.CommanderID, 0, len(candidates))
	for _, commanderID := range candidates {
		if state.Commanders[commanderID].Available {
			result = append(result, commanderID)
		}
	}
	return result
}

func maidenCandidateCommanders(state State.GameState) []State.CommanderID {
	eligible := map[State.CommanderID]struct{}{}
	for _, equipment := range state.Inventory.Equipment {
		if equipment.WearerKind != "commander" || (equipment.RarityID != 5 && equipment.RarityID != 15) {
			continue
		}
		value, found := equipmentWireEffectLastValue(equipment.Effects, maidenSupportEffectID)
		if !found {
			continue
		}
		commanderID := State.CommanderID(equipment.WearerID)
		_, ok := state.Commanders[commanderID]
		if ok && commanderID > 0 && value >= maidenSupportMinimum && value <= maidenSupportMaximum {
			eligible[commanderID] = struct{}{}
		}
	}
	result := make([]State.CommanderID, 0, len(eligible))
	for id := range eligible {
		result = append(result, id)
	}
	sort.Slice(result, func(left, right int) bool { return result[left] < result[right] })
	return result
}

func equipmentWireEffectLastValue(effects State.EquipmentEffects, wireID int64) (float64, bool) {
	for _, effect := range effects {
		if effect.WireID == wireID && len(effect.Values) > 0 {
			return effect.Values[len(effect.Values)-1], true
		}
	}
	return 0, false
}

type attackPair [2]int64

type attackFlank struct {
	Tools []attackPair `json:"T"`
	Units []attackPair `json:"U"`
}

type attackWave struct {
	Left   attackFlank `json:"L"`
	Right  attackFlank `json:"R"`
	Middle attackFlank `json:"M"`
}

type attackBody struct {
	SourceX            int               `json:"SX"`
	SourceY            int               `json:"SY"`
	TargetX            int               `json:"TX"`
	TargetY            int               `json:"TY"`
	Kingdom            State.KingdomID   `json:"KID"`
	Leader             State.CommanderID `json:"LID"`
	WaitHours          int               `json:"WT"`
	Booster            int               `json:"HBW"`
	BoosterCost        int               `json:"BPC"`
	AttackType         int               `json:"ATT"`
	Valid              int               `json:"AV"`
	LootPriority       int               `json:"LP"`
	FormationCost      int               `json:"FC"`
	PremiumTravel      int               `json:"PTT"`
	StartDelay         int               `json:"SD"`
	InstantCapture     int               `json:"ICA"`
	Cooldown           int               `json:"CD"`
	Waves              []attackWave      `json:"A"`
	Books              []any             `json:"BKS"`
	AttackSupportTools []int64           `json:"AST"`
	SupportTroops      []attackPair      `json:"RW"`
	AttackSupportCount int               `json:"ASCT"`
}

func emptyAttackSupportTools() []int64 {
	result := make([]int64, AttackPresets.CourtyardToolSlots)
	for index := range result {
		result[index] = -1
	}
	return result
}

func emptyAttackSupportTroops() []attackPair {
	empty := attackPair{-1, 0}
	result := make([]attackPair, AttackPresets.CourtyardTroopSlots)
	for index := range result {
		result[index] = empty
	}
	return result
}

func maidenAttackBody(
	sourceX, sourceY int,
	target State.MapObservation,
	commanderID State.CommanderID,
	unitID State.UnitID,
) attackBody {
	empty := attackPair{-1, 0}
	pair := attackPair{int64(unitID), maidenProbeCountPerFlank}
	wave := attackWave{
		Left:  attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{pair, empty}},
		Right: attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{pair, empty}},
		Middle: attackFlank{
			Tools: []attackPair{empty, empty, empty},
			Units: []attackPair{pair, empty, empty, empty, empty, empty},
		},
	}
	return attackBody{
		SourceX: sourceX, SourceY: sourceY, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commanderID, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: []attackWave{wave}, Books: []any{},
		AttackSupportTools: emptyAttackSupportTools(),
		SupportTroops:      emptyAttackSupportTroops(),
	}
}

func slicesMatchingCommanders(
	candidates []State.CommanderID,
	allowed map[State.CommanderID]struct{},
) []State.CommanderID {
	result := make([]State.CommanderID, 0, len(candidates))
	for _, commanderID := range candidates {
		if _, exists := allowed[commanderID]; exists {
			result = append(result, commanderID)
		}
	}
	return result
}

func officialDecoration(record GameData.Record) bool {
	ground, _ := record.String("buildingGroundType")
	category, _ := record.String("shopCategory")
	name, _ := record.String("name")
	typeName, _ := record.String("type")
	return ground == "DECO" || category == "DECO" || name == "Deco" || strings.Contains(strings.ToLower(typeName), "deco")
}

func unmatchedCount(matched []bool) int {
	count := 0
	for _, value := range matched {
		if !value {
			count++
		}
	}
	return count
}

func rawMapInt(values map[string]json.RawMessage, key string) int64 {
	var value int64
	_ = json.Unmarshal(values[key], &value)
	return value
}

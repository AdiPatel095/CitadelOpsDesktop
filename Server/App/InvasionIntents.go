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
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type invasionMapScanRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
	Radius         int            `json:"radius"`
	ScanStartedAt  time.Time      `json:"scanStartedAt"`
}

type invasionDifficultyRequest struct {
	EventID      int64 `json:"eventId"`
	DifficultyID int64 `json:"difficultyId"`
}

type invasionAttackRequest struct {
	SourceCastleID      State.CastleID       `json:"sourceCastleId"`
	EventID             int64                `json:"eventId"`
	ScoreTarget         int64                `json:"scoreTarget"`
	MinimumRemainingSec int64                `json:"minimumRemainingSec"`
	TargetTypeID        int                  `json:"targetTypeId"`
	KingdomID           State.KingdomID      `json:"kingdomId"`
	TargetX             int                  `json:"targetX"`
	TargetY             int                  `json:"targetY"`
	TargetObjectID      int64                `json:"targetObjectId,omitempty"`
	FortifyCurrency     string               `json:"fortifyCurrency,omitempty"`
	Preset              AttackPresets.Preset `json:"preset"`
	CommanderIDs        []State.CommanderID  `json:"commanderIds"`
	HorseTravelBoostID  int                  `json:"horseTravelBoostId"`
}

var invasionFortifyOptions = map[string]string{
	"GTO": "Gold tokens",
	"STO": "Silver tokens",
	"KM":  "Khan medals",
	"C2":  "Rubies",
}

type resolvedInvasionAttackRequest struct {
	invasionAttackRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

func planInvasionMapScan(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, err := invasionMapScanContext(input, arguments)
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
			fmt.Sprintf("Refresh invasion map window %d/%d", index+1, len(windows)), "gaa", payload, "gaa",
		))
	}
	steps = append(steps, Intent.Step{
		Name: "Record invasion map scan", Action: "invasion.scan.capture", ActionArguments: append(json.RawMessage(nil), arguments...),
	})
	castleID := strconv.FormatInt(int64(source.ID), 10)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + castleID, "map:" + strconv.FormatInt(int64(source.KingdomID), 10)},
		Summary: fmt.Sprintf("Refresh invasion targets around %s", castleLabel(source)), Steps: steps,
	}, nil
}

func planInvasionDifficulty(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request invasionDifficultyRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	if _, supported := invasionMapTypeForEvent(request.EventID); !supported {
		return Intent.Plan{}, fmt.Errorf("event %d is not supported by Auto Invasion", request.EventID)
	}
	difficulty, valid := input.GameData.ScalableEvent(request.EventID, request.DifficultyID)
	if !valid {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not valid for event %d", request.DifficultyID, request.EventID)
	}
	if difficulty.IsLocked && (difficulty.UnlockAchievementID <= 0 || !input.State.Player.Achievements.Completed[difficulty.UnlockAchievementID]) {
		return Intent.Plan{}, fmt.Errorf("difficulty %d is not unlocked by this player's achievements", request.DifficultyID)
	}
	score, active := input.State.ActiveScalableEventScore()
	if !active || score.EventID != request.EventID {
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
		Summary: fmt.Sprintf("Select difficulty %d for invasion event %d", request.DifficultyID, request.EventID),
		Steps:   []Intent.Step{commandStep("Select invasion event difficulty", "sede", payload, "sede")},
	}, nil
}

func planInvasionAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := invasionAttackContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	var commanderSelection *craCommanderSelectionRequest
	if request.CommanderIDs != nil {
		if len(request.CommanderIDs) == 0 {
			return Intent.Plan{}, fmt.Errorf("no commanders are assigned to Auto Invasion")
		}
		commanderSelection = &craCommanderSelectionRequest{
			Candidates: request.CommanderIDs,
			Count:      1,
			Strategy:   "lowest_id",
		}
	}
	resolution, err := resolveCRACommanders(
		input.State,
		commanderSelection,
		craCommanderSelectionOptions{DefaultCount: 1, RequireAvailable: true},
	)
	if err != nil {
		return Intent.Plan{}, err
	}
	commanderID := resolution.Selected[0]
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, target.KingdomID})
	resolvedArguments, _ := json.Marshal(resolvedInvasionAttackRequest{invasionAttackRequest: request, CommanderID: commanderID})
	consumeArguments, _ := json.Marshal(map[string]any{
		"kingdomId": target.KingdomID, "targetX": target.X, "targetY": target.Y,
	})
	steps := make([]Intent.Step, 0, 5)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() ||
		time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep(
			"Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl",
		))
	}
	steps = append(steps, generalSkillsContextSteps(input.State, commanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	if request.FortifyCurrency != "" && !invasionTargetFortified(input.State, target) {
		fortifyArguments, _ := json.Marshal(request)
		fortifyPayload, _ := json.Marshal(struct {
			X        int    `json:"XPOS"`
			Y        int    `json:"YPOS"`
			Currency string `json:"RCK"`
		}{target.X, target.Y, request.FortifyCurrency})
		steps = append(steps,
			Intent.Step{Name: "Verify invasion fortification currency", Action: "invasion.fortify.guard", ActionArguments: fortifyArguments},
			commandStep("Fortify invasion castle with "+invasionFortifyOptions[request.FortifyCurrency], "rae", fortifyPayload, "rae"),
		)
	}
	steps = append(steps,
		deferredCRACommandStep("Build and launch capacity-adjusted invasion attack", "invasion.attack.build", resolvedArguments, contextPayload),
		Intent.Step{Name: "Consume invasion target", Action: "invasion.target.consume", ActionArguments: consumeArguments},
	)
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		fmt.Sprintf("invasion-target:%d:%d:%d", target.KingdomID, target.X, target.Y),
	}
	if request.FortifyCurrency != "" {
		claims = append(claims, "account-resources")
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{commanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoInvasion", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack invasion castle at %d:%d with %s", target.X, target.Y, request.Preset.Name),
		Steps:   steps,
	}, nil
}

func invasionMapScanContext(input Intent.PlanningContext, arguments json.RawMessage) (invasionMapScanRequest, State.CastleState, error) {
	var request invasionMapScanRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return invasionMapScanRequest{}, State.CastleState{}, err
	}
	if request.SourceCastleID <= 0 || request.Radius < 1 || request.Radius > 50 {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion map scan requires a source castle and radius between 1 and 50")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion source castle %d is unavailable", request.SourceCastleID)
	}
	if source.KingdomID != 0 {
		return invasionMapScanRequest{}, State.CastleState{}, fmt.Errorf("invasion source castle must be in the Great Empire")
	}
	return request, source, nil
}

func invasionAttackContext(input Intent.PlanningContext, arguments json.RawMessage) (invasionAttackRequest, State.CastleState, State.MapObservation, error) {
	var request invasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if request.SourceCastleID <= 0 || request.EventID <= 0 || request.ScoreTarget <= 0 || request.TargetTypeID <= 0 {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion attack requires source, event, score target, and target type")
	}
	request.FortifyCurrency = strings.ToUpper(strings.TrimSpace(request.FortifyCurrency))
	if request.FortifyCurrency != "" {
		if _, valid := invasionFortifyOptions[request.FortifyCurrency]; !valid {
			return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("unsupported invasion fortification currency %q", request.FortifyCurrency)
		}
	}
	expectedTypeID, supported := invasionMapTypeForEvent(request.EventID)
	if !supported || expectedTypeID != request.TargetTypeID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("event %d does not use invasion target type %d", request.EventID, request.TargetTypeID)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion source castle %d is unavailable", request.SourceCastleID)
	}
	if source.KingdomID != 0 || request.KingdomID != source.KingdomID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion attack source and target must be in the Great Empire")
	}
	target, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || target.TypeID != request.TargetTypeID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion target %d:%d is no longer available", request.TargetX, request.TargetY)
	}
	if request.TargetObjectID > 0 && target.ObjectID > 0 && target.ObjectID != request.TargetObjectID {
		return invasionAttackRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invasion target %d:%d changed appearance", request.TargetX, request.TargetY)
	}
	return request, source, target, nil
}

func invasionAttackSetup(preset AttackPresets.Preset) attackSetupRequest {
	waves := make([]attackSetupWaveRequest, 0, len(preset.Waves))
	for _, wave := range preset.Waves {
		waves = append(waves, attackSetupWaveRequest{
			Left: invasionAttackLane(wave.Left), Middle: invasionAttackLane(wave.Middle), Right: invasionAttackLane(wave.Right),
		})
	}
	return attackSetupRequest{Name: preset.Name, Waves: waves}
}

func invasionAttackLane(lane AttackPresets.Lane) attackSetupLaneRequest {
	convert := func(slots []AttackPresets.Slot) []attackSetupSlotRequest {
		result := make([]attackSetupSlotRequest, len(slots))
		for index, slot := range slots {
			result[index] = attackSetupSlotRequest{ItemID: slot.ItemID, Quantity: slot.Quantity}
		}
		return result
	}
	return attackSetupLaneRequest{Troops: convert(lane.Troops), Tools: convert(lane.Tools)}
}

func invasionAttackBody(
	source State.CastleState,
	target State.MapObservation,
	commanderID State.CommanderID,
	waves []attackWave,
	horseTravelBoostIDs ...int,
) attackBody {
	empty := attackPair{-1, 0}
	body := attackBody{
		SourceX: source.X, SourceY: source.Y, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commanderID, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: waves, Books: []any{},
		AttackSupportTools: []int64{-1, -1, -1},
		SupportTroops:      []attackPair{empty, empty, empty, empty, empty, empty, empty, empty},
	}
	horseTravelBoostID := defaultHorseTravelBoostID
	if len(horseTravelBoostIDs) > 0 {
		horseTravelBoostID = horseTravelBoostIDs[0]
	}
	applyHorseTravelBoost(&body, horseTravelBoostID)
	return body
}

func (application *Application) resolveInvasionAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	var request resolvedInvasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	capacity, source, target, err := resolveInvasionAttackCapacity(input, request.invasionAttackRequest, request.CommanderID)
	if err != nil {
		return Intent.Step{}, err
	}
	setup := limitAttackSetupToCapacity(invasionAttackSetup(request.Preset), capacity.Capacity)
	waves, err := buildAttackSetupWaves(setup, source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build invasion preset %q: %w", request.Preset.Name, err)
	}
	body, err := json.Marshal(invasionAttackBody(source, target, request.CommanderID, waves, request.HorseTravelBoostID))
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build invasion CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack invasion castle at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func resolveInvasionAttackCapacity(
	input Intent.PlanningContext,
	request invasionAttackRequest,
	commanderID State.CommanderID,
) (AttackCapacity.Result, State.CastleState, State.MapObservation, error) {
	_, source, target, err := invasionAttackContext(input, mustMarshalInvasionAttackRequest(request))
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, err
	}
	commander, exists := input.State.Commanders[commanderID]
	if !exists || !commander.Available {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("commander %d is no longer available", commanderID)
	}
	level := target.Level
	if level <= 0 {
		level = int(target.ObjectID)
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commanderID, UseAttackDialogEffects: true,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("invasion:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: level,
			},
			Level: level, CastleTypeID: target.TypeID, PvP: true, LegendaryFight: true,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("resolve invasion attack capacity: %w", err)
	}
	return capacity, source, target, nil
}

func limitAttackSetupToCapacity(setup attackSetupRequest, capacity AttackCapacity.LaneCapacity) attackSetupRequest {
	result := setup
	result.Waves = make([]attackSetupWaveRequest, len(setup.Waves))
	for index, wave := range setup.Waves {
		result.Waves[index] = attackSetupWaveRequest{
			Left:   limitAttackSetupLane(wave.Left, capacity.Left),
			Middle: limitAttackSetupLane(wave.Middle, capacity.Front),
			Right:  limitAttackSetupLane(wave.Right, capacity.Right),
		}
	}
	return result
}

func limitAttackSetupLane(lane attackSetupLaneRequest, capacity int64) attackSetupLaneRequest {
	result := attackSetupLaneRequest{
		Troops: append([]attackSetupSlotRequest(nil), lane.Troops...),
		Tools:  append([]attackSetupSlotRequest(nil), lane.Tools...),
	}
	remaining := max(int64(0), capacity)
	for index := range result.Troops {
		quantity := result.Troops[index].Quantity
		if quantity <= 0 {
			continue
		}
		result.Troops[index].Quantity = min(quantity, remaining)
		remaining -= result.Troops[index].Quantity
	}
	return result
}

func (application *Application) captureInvasionScan(_ context.Context, arguments json.RawMessage) error {
	request, _, err := invasionMapScanContext(Intent.PlanningContext{State: application.State.Snapshot()}, arguments)
	if err != nil {
		return err
	}
	_, err = application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, exists := gameState.Castles[request.SourceCastleID]
		if !exists || !source.Focused {
			return nil, false, fmt.Errorf("invasion source castle %d is no longer focused", request.SourceCastleID)
		}
		if gameState.Invasion.LastScannedAt == nil {
			gameState.Invasion.LastScannedAt = map[State.CastleID]time.Time{}
		}
		scannedAt := request.ScanStartedAt.UTC()
		if scannedAt.IsZero() {
			scannedAt = time.Now().UTC()
		}
		if gameState.Invasion.LastScannedAt[request.SourceCastleID].Equal(scannedAt) {
			return nil, false, nil
		}
		gameState.Invasion.LastScannedAt[request.SourceCastleID] = scannedAt
		return []string{"invasion"}, true, nil
	})
	return err
}

func (application *Application) guardInvasionAttack(_ context.Context, arguments json.RawMessage) error {
	var request resolvedInvasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	_, source, target, err := invasionAttackContext(Intent.PlanningContext{State: state}, mustMarshalInvasionAttackRequest(request.invasionAttackRequest))
	if err != nil {
		return err
	}
	score, found := state.ActiveScalableEventScore()
	if !found || score.EventID != request.EventID {
		return fmt.Errorf("invasion event %d is no longer active", request.EventID)
	}
	if score.PlayerScore >= request.ScoreTarget {
		return fmt.Errorf("invasion score target reached: %d / %d", score.PlayerScore, request.ScoreTarget)
	}
	if remaining := invasionRemainingSeconds(score, time.Now().UTC()); remaining >= 0 && remaining <= max(0, request.MinimumRemainingSec) {
		return fmt.Errorf("invasion event has only %d seconds remaining", remaining)
	}
	commander, exists := state.Commanders[request.CommanderID]
	if !exists || !commander.Available {
		return fmt.Errorf("commander %d is no longer available", request.CommanderID)
	}
	dialog := state.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != target.TypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y {
		return fmt.Errorf("current attack dialog does not match invasion target %d:%d", target.X, target.Y)
	}
	return nil
}

func (application *Application) guardInvasionFortify(_ context.Context, arguments json.RawMessage) error {
	var request invasionAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	state := application.State.Snapshot()
	request, _, target, err := invasionAttackContext(Intent.PlanningContext{State: state}, mustMarshalInvasionAttackRequest(request))
	if err != nil {
		return err
	}
	if request.FortifyCurrency == "" {
		return fmt.Errorf("invasion fortification currency is required")
	}
	if invasionTargetFortified(state, target) {
		return fmt.Errorf("invasion target %d:%d is already fortified", target.X, target.Y)
	}
	return nil
}

func invasionTargetFortified(gameState State.GameState, target State.MapObservation) bool {
	_, exists := gameState.Invasion.FortifiedTargets[fmt.Sprintf("%d:%d:%d", target.KingdomID, target.X, target.Y)]
	return exists
}

func (application *Application) consumeInvasionTarget(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		KingdomID State.KingdomID `json:"kingdomId"`
		TargetX   int             `json:"targetX"`
		TargetY   int             `json:"targetY"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		observations := gameState.Map[request.KingdomID]
		key := fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)
		if _, exists := observations[key]; !exists {
			return nil, false, nil
		}
		delete(observations, key)
		return []string{"map", "invasion"}, true, nil
	})
	return err
}

func mustMarshalInvasionAttackRequest(request invasionAttackRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

func invasionMapTypeForEvent(eventID int64) (int, bool) {
	switch eventID {
	case 71:
		return 21, true
	case 103:
		return 34, true
	default:
		return 0, false
	}
}

func invasionRemainingSeconds(score State.ScalableEventScore, now time.Time) int64 {
	if score.RemainingSec <= 0 {
		return -1
	}
	elapsed := int64(0)
	if !score.ObservedAt.IsZero() && now.After(score.ObservedAt) {
		elapsed = int64(now.Sub(score.ObservedAt) / time.Second)
	}
	return max(0, score.RemainingSec-elapsed)
}

package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const kingdomTowerMapTypeID = 2

type towerLaunchRequest struct {
	SourceCastleID     State.CastleID      `json:"sourceCastleId"`
	KingdomID          State.KingdomID     `json:"kingdomId"`
	TargetX            int                 `json:"targetX"`
	TargetY            int                 `json:"targetY"`
	UnitID             State.UnitID        `json:"unitId"`
	MaidenOnly         bool                `json:"maidenOnly"`
	CommanderIDs       []State.CommanderID `json:"commanderIds"`
	HorseTravelBoostID int                 `json:"horseTravelBoostId"`
}

type towerResolvedAttackRequest struct {
	towerLaunchRequest
	CommanderID State.CommanderID `json:"commanderId"`
}

func planTowerContext(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := towerLaunchContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, request.KingdomID})
	setup, err := craSetupContextSteps(contextPayload)
	if err != nil {
		return Intent.Plan{}, err
	}
	steps := make([]Intent.Step, 0, len(setup)+1)
	if !source.Focused {
		steps = append(steps, attackCastleContextStep(source))
	}
	steps = append(steps, setup...)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			towerTargetClaim(target),
		},
		Summary: fmt.Sprintf("Refresh tower attack context at %d:%d", target.X, target.Y),
		Steps:   steps,
	}, nil
}

func planTowerAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := towerLaunchContext(input, arguments)
	if err != nil {
		return Intent.Plan{}, err
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	commander, err := towerCommander(input.State, request.MaidenOnly, request.CommanderIDs)
	if err != nil {
		return Intent.Plan{}, err
	}
	contextPayload, _ := json.Marshal(struct {
		SourceX   int             `json:"SX"`
		SourceY   int             `json:"SY"`
		TargetX   int             `json:"TX"`
		TargetY   int             `json:"TY"`
		KingdomID State.KingdomID `json:"KID"`
	}{source.X, source.Y, target.X, target.Y, request.KingdomID})
	resolvedArguments, _ := json.Marshal(towerResolvedAttackRequest{towerLaunchRequest: request, CommanderID: commander})
	steps := make([]Intent.Step, 0, 4)
	steps = append(steps, generalSkillsContextSteps(input.State, commander, time.Now().UTC())...)
	steps = append(steps,
		attackCastleContextStep(source),
		Intent.Step{Name: "Verify fresh tower troops", Action: "tower.attack.guard", ActionArguments: resolvedArguments},
	)
	steps = append(steps, deferredCRACommandStep(
		"Build and launch tower attack", "tower.attack.build", resolvedArguments, contextPayload,
	))
	queueEntry, _ := json.Marshal(towerQueueEntryRequest{
		SourceCastleID: source.ID, KingdomID: target.KingdomID, TargetX: target.X, TargetY: target.Y,
	})
	steps = append(steps, Intent.Step{Name: "Consume tower queue target", Action: "tower.queue.consume", ActionArguments: queueEntry})
	return Intent.Plan{
		Claims: towerAttackClaims(source, target, commander, true),
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoTowers",
			Affinity: "castle:" + strconv.FormatInt(int64(source.ID), 10),
		},
		Summary: fmt.Sprintf("Attack kingdom tower at %d:%d from %s", target.X, target.Y, castleLabel(source)),
		Steps:   steps,
	}, nil
}

func planTowerLaunch(ctx context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	return planTowerAttack(ctx, input, arguments)
}

func (application *Application) resolveTowerAttackStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request towerResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	return buildTowerAttackStep(input, request.towerLaunchRequest, request.CommanderID)
}

func (application *Application) guardTowerAttackInventory(_ context.Context, arguments json.RawMessage) error {
	var request towerResolvedAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	input := Intent.PlanningContext{State: application.State.Snapshot(), GameData: gameData}
	capacity, source, _, err := resolveTowerAttackCapacity(input, request.towerLaunchRequest, request.CommanderID, false)
	if err != nil {
		return err
	}
	return requireTowerAttackUnits(source, request.UnitID, capacity.Capacity.Left+capacity.Capacity.Right)
}

func buildTowerAttackStep(input Intent.PlanningContext, request towerLaunchRequest, commander State.CommanderID) (Intent.Step, error) {
	_, source, target, err := towerLaunchContext(input, mustMarshalTowerLaunchRequest(request))
	if err != nil {
		return Intent.Step{}, err
	}
	if towerCooldownRemaining(target, input.State.UpdatedAt, time.Now().UTC()) > 0 {
		return Intent.Step{}, fmt.Errorf("kingdom tower at %d:%d is on cooldown", target.X, target.Y)
	}
	dialog := input.State.AttackDialog
	if dialog.SourceCastleID != source.ID || dialog.KingdomID != target.KingdomID ||
		dialog.Target.TypeID != kingdomTowerMapTypeID || dialog.Target.X != target.X || dialog.Target.Y != target.Y {
		return Intent.Step{}, fmt.Errorf("current attack-dialog context does not match tower %d:%d", target.X, target.Y)
	}
	capacity, source, target, err := resolveTowerAttackCapacity(input, request, commander, true)
	if err != nil {
		return Intent.Step{}, err
	}
	required := capacity.Capacity.Left + capacity.Capacity.Right
	if err := requireTowerAttackUnits(source, request.UnitID, required); err != nil {
		return Intent.Step{}, err
	}
	body, err := json.Marshal(towerAttackBody(
		source, target, commander, request.UnitID, capacity.Capacity.Left, capacity.Capacity.Right, request.HorseTravelBoostID,
	))
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build tower CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack tower at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
}

func resolveTowerAttackCapacity(
	input Intent.PlanningContext,
	request towerLaunchRequest,
	commander State.CommanderID,
	useAttackDialogEffects bool,
) (AttackCapacity.Result, State.CastleState, State.MapObservation, error) {
	_, source, target, err := towerLaunchContext(input, mustMarshalTowerLaunchRequest(request))
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, err
	}
	commanderState, exists := input.State.Commanders[commander]
	if !exists || !commanderState.Available {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("commander %d is no longer available", commander)
	}
	if input.GameData == nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("official game data is unavailable")
	}
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: commander,
		UseAttackDialogEffects: useAttackDialogEffects,
		Target: AttackCapacity.TargetContext{
			ID: fmt.Sprintf("tower:%d:%d:%d", target.KingdomID, target.X, target.Y),
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y, ObjectID: target.ObjectID,
				Level: target.Level, VictoryCount: target.TowerVictoryCount,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("resolve tower attack capacity: %w", err)
	}
	return capacity, source, target, nil
}

func mustMarshalTowerLaunchRequest(request towerLaunchRequest) json.RawMessage {
	payload, _ := json.Marshal(request)
	return payload
}

func towerAttackClaims(source State.CastleState, target State.MapObservation, commander State.CommanderID, focus bool) []string {
	claims := []string{
		"attack-context", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		"attack-inventory:" + strconv.FormatInt(int64(source.ID), 10), towerTargetClaim(target),
	}
	if focus {
		claims = append(claims, "castle-focus")
	}
	return append(claims, craCommanderClaims([]State.CommanderID{commander})...)
}

func towerLaunchContext(input Intent.PlanningContext, arguments json.RawMessage) (towerLaunchRequest, State.CastleState, State.MapObservation, error) {
	var request towerLaunchRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, err
	}
	if request.SourceCastleID <= 0 || request.UnitID <= 0 {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("tower source castle and unit are required")
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("tower source castle %d is not in the current player state", request.SourceCastleID)
	}
	if request.KingdomID != source.KingdomID {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("tower target must be in source castle kingdom %d", source.KingdomID)
	}
	target, exists := input.State.Map[request.KingdomID][fmt.Sprintf("%d:%d", request.TargetX, request.TargetY)]
	if !exists || target.TypeID != kingdomTowerMapTypeID {
		return towerLaunchRequest{}, State.CastleState{}, State.MapObservation{}, fmt.Errorf("kingdom tower at %d:%d is not in the current map state", request.TargetX, request.TargetY)
	}
	return request, source, target, nil
}

func towerCommander(
	gameState State.GameState,
	maidenOnly bool,
	configured []State.CommanderID,
) (State.CommanderID, error) {
	if configured != nil && len(configured) == 0 {
		return 0, fmt.Errorf("no commanders are assigned to Auto Towers")
	}
	candidates := allCommanderIDs(gameState)
	if configured != nil {
		candidates = append([]State.CommanderID(nil), configured...)
	}
	if maidenOnly {
		maidenCandidates := maidenCandidateCommanders(gameState)
		eligible := make(map[State.CommanderID]struct{}, len(maidenCandidates))
		for _, commanderID := range maidenCandidates {
			eligible[commanderID] = struct{}{}
		}
		filtered := make([]State.CommanderID, 0, len(candidates))
		for _, commanderID := range candidates {
			if _, supported := eligible[commanderID]; supported {
				filtered = append(filtered, commanderID)
			}
		}
		candidates = filtered
	}
	if len(candidates) == 0 {
		if maidenOnly && configured != nil {
			return 0, fmt.Errorf("no assigned commander supports the required maiden relic")
		}
		if maidenOnly {
			return 0, fmt.Errorf("no commander supports the required maiden relic")
		}
		if configured != nil {
			return 0, fmt.Errorf("no assigned Auto Towers commander is in the current roster")
		}
		return 0, fmt.Errorf("no commander is in the current roster")
	}
	resolution, err := resolveCRACommanders(gameState, &craCommanderSelectionRequest{Candidates: candidates, Count: 1, Strategy: "lowest_id"}, craCommanderSelectionOptions{
		DefaultCount: 1, RequireAvailable: true,
	})
	if err != nil {
		if maidenOnly {
			if configured != nil {
				return 0, fmt.Errorf("no available assigned maiden-supported commander: %w", err)
			}
			return 0, fmt.Errorf("no available maiden-supported commander: %w", err)
		}
		if configured != nil {
			return 0, fmt.Errorf("no available assigned Auto Towers commander: %w", err)
		}
		return 0, fmt.Errorf("no available commander: %w", err)
	}
	return resolution.Selected[0], nil
}

func requireTowerAttackUnits(source State.CastleState, unitID State.UnitID, required int64) error {
	available := max(0, source.Units.Stationed[unitID])
	if required > 0 && available >= required {
		return nil
	}
	return fmt.Errorf(
		"%s has %d available of unit %d; full tower flanks require %d",
		castleLabel(source), available, unitID, required,
	)
}

func towerCooldownRemaining(target State.MapObservation, stateUpdatedAt time.Time, now time.Time) int {
	if target.TowerCooldownRemaining <= 0 {
		return 0
	}
	observedAt := target.ObservedAt
	if observedAt.IsZero() {
		observedAt = stateUpdatedAt
	}
	if observedAt.IsZero() {
		return target.TowerCooldownRemaining
	}
	remaining := target.TowerCooldownRemaining - int(now.Sub(observedAt)/time.Second)
	if remaining < 0 {
		return 0
	}
	return remaining
}

func towerAttackBody(
	source State.CastleState,
	target State.MapObservation,
	commander State.CommanderID,
	unitID State.UnitID,
	left, right int64,
	horseTravelBoostIDs ...int,
) attackBody {
	empty := attackPair{-1, 0}
	wave := attackWave{
		Left:  attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{{int64(unitID), left}, empty}},
		Right: attackFlank{Tools: []attackPair{empty, empty}, Units: []attackPair{{int64(unitID), right}, empty}},
		Middle: attackFlank{
			Tools: []attackPair{empty, empty, empty},
			Units: []attackPair{empty, empty, empty, empty, empty, empty},
		},
	}
	body := attackBody{
		SourceX: source.X, SourceY: source.Y, TargetX: target.X, TargetY: target.Y,
		Kingdom: target.KingdomID, Leader: commander, Booster: -1, Valid: 1,
		PremiumTravel: 1, Cooldown: 99, Waves: []attackWave{wave}, Books: []any{},
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

func towerTargetClaim(target State.MapObservation) string {
	return fmt.Sprintf("tower-target:%d:%d:%d", target.KingdomID, target.X, target.Y)
}

package App

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

const kingdomTroopMaximumStacks = 20

type kingdomTroopShipmentUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type kingdomTroopShipmentRequest struct {
	SourceCastleID                      State.CastleID             `json:"sourceCastleId"`
	TargetCastleID                      State.CastleID             `json:"targetCastleId"`
	TargetKingdomID                     State.KingdomID            `json:"targetKingdomId"`
	MaximumTargetTroops                 int64                      `json:"maximumTargetTroops,omitempty"`
	ExpectedDailyAttackSessionStartedAt *time.Time                 `json:"expectedDailyAttackSessionStartedAt,omitempty"`
	Units                               []kingdomTroopShipmentUnit `json:"units"`
	Owner                               string                     `json:"owner,omitempty"`
	WorkflowID                          string                     `json:"workflowId,omitempty"`
}

type kingdomTroopSkipRequest struct {
	TargetKingdomID     State.KingdomID `json:"targetKingdomId"`
	TimeSkipID          string          `json:"timeSkipId"`
	MinimumRemaining    int64           `json:"minimumRemaining,omitempty"`
	Owner               string          `json:"owner,omitempty"`
	WorkflowID          string          `json:"workflowId,omitempty"`
	ExpectedRemaining   int             `json:"expectedRemaining,omitempty"`
	ExpectedDurationSec int64           `json:"expectedDurationSec,omitempty"`
}

func planKingdomTroopRefresh(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	return Intent.Plan{
		Claims: []string{"troop-transport"}, Summary: "Refresh kingdom troop transports",
		Steps: []Intent.Step{commandStep("Refresh kingdom troop transports", "kpi", json.RawMessage(`{}`), "kpi")},
	}, nil
}

func planKingdomTroopShipment(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || source.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	if !targetExists || target.ID <= 0 || target.KingdomID != request.TargetKingdomID {
		return Intent.Plan{}, fmt.Errorf("target castle %d is not in kingdom %d", request.TargetCastleID, request.TargetKingdomID)
	}
	if source.ID == target.ID || source.KingdomID == target.KingdomID {
		return Intent.Plan{}, fmt.Errorf("kingdom troop transfers require castles in different kingdoms")
	}
	if err := verifyKingdomTroopExpectedDailyAttackSession(input.State, request.ExpectedDailyAttackSessionStartedAt); err != nil {
		return Intent.Plan{}, err
	}
	if err := requireStormTroopSupportMead(input.GameData, target); err != nil {
		return Intent.Plan{}, err
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[target.KingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked {
		return Intent.Plan{}, fmt.Errorf("kingdom troop transport to %d is not observed as unlocked", target.KingdomID)
	}
	if kingdomTroopTransportPending(input.State, target.KingdomID) {
		return Intent.Plan{}, fmt.Errorf("kingdom %d already has a pending or settling troop transport", target.KingdomID)
	}
	units, err := normalizeKingdomTroopShipment(input.GameData, source, request.Units)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.MaximumTargetTroops < 0 {
		return Intent.Plan{}, fmt.Errorf("maximumTargetTroops cannot be negative")
	}
	if request.MaximumTargetTroops > 0 {
		if err := verifyKingdomTroopTargetCap(
			input.GameData, input.State, target, units, request.MaximumTargetTroops,
		); err != nil {
			return Intent.Plan{}, err
		}
	}
	wireUnits := make([][2]int64, 0, len(units))
	summaryUnits := make([]string, 0, len(units))
	claims := []string{
		"castle-focus", "troop-transport", "castle:" + strconv.FormatInt(int64(source.ID), 10),
		"kingdom:" + strconv.FormatInt(int64(target.KingdomID), 10),
	}
	for _, unit := range units {
		wireUnits = append(wireUnits, [2]int64{int64(unit.UnitID), unit.Amount})
		summaryUnits = append(summaryUnits, fmt.Sprintf("%d of unit %d", unit.Amount, unit.UnitID))
		claims = append(claims, "unit:"+strconv.FormatInt(int64(unit.UnitID), 10))
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		WireCastleID   int64           `json:"CID"`
		Units          [][2]int64      `json:"A"`
	}{source.ID, source.KingdomID, target.KingdomID, -1, wireUnits})
	consumeArguments, _ := json.Marshal(request)
	guardArguments, _ := json.Marshal(kingdomTransportAvailabilityGuard{
		TargetKingdomID: target.KingdomID, TransportKind: "troop",
	})
	steps := make([]Intent.Step, 0, 4)
	if !source.Focused {
		steps = append(steps, castleFocusStep(source))
	}
	steps = append(steps,
		kingdomTransportContextStep(),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Verify kingdom troop transport availability", Action: "kingdom.transport.verify_available",
			ActionArguments: guardArguments,
		}),
	)
	if request.MaximumTargetTroops > 0 {
		steps = append(steps, Intent.Step{
			Name: "Verify target troop inventory cap", Action: "troops.kingdom.guard_target_cap",
			ActionArguments: consumeArguments,
		})
	}
	steps = append(steps, commandStep("Start kingdom troop transfer", "kut", payload, "kut"))
	if strings.TrimSpace(request.Owner) != "" {
		if strings.TrimSpace(request.WorkflowID) == "" {
			return Intent.Plan{}, fmt.Errorf("owned kingdom troop transfer requires workflowId")
		}
		commandIndex := len(steps) - 1
		steps[commandIndex].PreDispatchAction = "troops.kingdom.workflow.arm"
		steps[commandIndex].PreDispatchArguments = consumeArguments
		steps[commandIndex].FinalDispatchAction = "troops.kingdom.workflow.dispatch"
		steps[commandIndex].FinalDispatchArguments = consumeArguments
		steps[commandIndex].DefinitiveSendFailureAction = "troops.kingdom.workflow.disarm"
		steps[commandIndex].DefinitiveSendFailureArguments = consumeArguments
		steps[commandIndex].DefinitiveResponseFailureAction = "troops.kingdom.workflow.disarm"
		steps[commandIndex].DefinitiveResponseFailureArguments = consumeArguments
		steps[commandIndex].ResponseProjectionFailureIndeterminate = true
		steps = append(steps, Intent.Step{
			Name: "Confirm owned kingdom troop transfer", Action: "troops.kingdom.workflow.confirm", ActionArguments: consumeArguments,
		})
	}
	steps = append(steps, Intent.Step{Name: "Consume confirmed donor troops", Action: "troops.kingdom.consume_source", ActionArguments: consumeArguments})
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Transfer %s from %s to %s", strings.Join(summaryUnits, ", "), castleLabel(source), castleLabel(target)),
		Steps:   steps,
	}, nil
}

func (application *Application) guardKingdomTroopTargetCap(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.MaximumTargetTroops <= 0 {
		return nil
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	gameState := application.State.ReadOnlyView()
	if err := verifyKingdomTroopExpectedDailyAttackSession(gameState, request.ExpectedDailyAttackSessionStartedAt); err != nil {
		return err
	}
	source, sourceExists := gameState.Castles[request.SourceCastleID]
	target, targetExists := gameState.Castles[request.TargetCastleID]
	if !sourceExists || !targetExists || target.KingdomID != request.TargetKingdomID {
		return fmt.Errorf("kingdom troop transfer castles changed before dispatch")
	}
	units, err := normalizeKingdomTroopShipment(gameData, source, request.Units)
	if err != nil {
		return err
	}
	return verifyKingdomTroopTargetCap(gameData, gameState, target, units, request.MaximumTargetTroops)
}

func verifyKingdomTroopExpectedDailyAttackSession(gameState State.GameState, expected *time.Time) error {
	if expected == nil {
		return nil
	}
	wanted := expected.UTC()
	observed := gameState.DailyAttacks.SessionStartedAt.UTC()
	if wanted.IsZero() || observed.IsZero() || !observed.Equal(wanted) {
		return fmt.Errorf("%w: the daily attack reset changed after the troop cap was calculated", Intent.ErrPlanStale)
	}
	return nil
}

func verifyKingdomTroopTargetCap(
	gameData *GameData.Store,
	gameState State.GameState,
	target State.CastleState,
	units []kingdomTroopShipmentUnit,
	maximum int64,
) error {
	if maximum <= 0 {
		return nil
	}
	current, err := kingdomTroopTargetInventory(gameData, gameState, target)
	if err != nil {
		return err
	}
	incoming := int64(0)
	for _, unit := range units {
		incoming = saturatingTroopAdd(incoming, max(int64(0), unit.Amount))
	}
	if current > maximum-incoming {
		return fmt.Errorf(
			"troop transfer would put %s above its %d-troop import cap (%d committed, %d incoming)",
			castleLabel(target), maximum, current, incoming,
		)
	}
	return nil
}

func kingdomTroopTargetInventory(
	gameData *GameData.Store,
	gameState State.GameState,
	target State.CastleState,
) (int64, error) {
	if gameData == nil {
		return 0, fmt.Errorf("official game data is unavailable")
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return 0, err
	}
	countMap := func(units map[State.UnitID]int64) (int64, error) {
		total := int64(0)
		for unitID, amount := range units {
			if unitID <= 0 || amount <= 0 {
				continue
			}
			raw, found := catalog.Find(strconv.FormatInt(int64(unitID), 10))
			if found {
				record, decodeErr := GameData.DecodeRecord(raw)
				if decodeErr != nil {
					return 0, decodeErr
				}
				if GameData.IsToolRecord(record) {
					continue
				}
			}
			total = saturatingTroopAdd(total, amount)
		}
		return total, nil
	}
	total, err := countMap(target.Units.Stationed)
	if err != nil {
		return 0, err
	}
	traveling, err := countMap(target.Units.Traveling)
	if err != nil {
		return 0, err
	}
	movementTroops := int64(0)
	var movementErr error
	gameState.RangeMovements(func(_ State.MovementID, movement State.MovementState) bool {
		if movement.SourceCastleID != target.ID {
			return true
		}
		amount, countErr := countMap(movement.Units)
		if countErr != nil {
			movementErr = countErr
			return false
		}
		movementTroops = saturatingTroopAdd(movementTroops, amount)
		return true
	})
	if movementErr != nil {
		return 0, movementErr
	}
	total = saturatingTroopAdd(total, max(traveling, movementTroops))
	for _, pending := range gameState.KingdomTransport.PendingUnits {
		if pending.KingdomID != target.KingdomID {
			continue
		}
		units := make(map[State.UnitID]int64, len(pending.Units))
		for _, unit := range pending.Units {
			units[unit.UnitID] = saturatingTroopAdd(units[unit.UnitID], unit.Amount)
		}
		amount, countErr := countMap(units)
		if countErr != nil {
			return 0, countErr
		}
		total = saturatingTroopAdd(total, amount)
	}
	for _, operation := range gameState.Storm.IslandReturns {
		if operation.SourceCastleID != target.ID || operation.Status != State.StormIslandReturnReady {
			continue
		}
		amount, countErr := countMap(operation.UnitsToReturn())
		if countErr != nil {
			return 0, countErr
		}
		total = saturatingTroopAdd(total, amount)
	}
	return total, nil
}

func saturatingTroopAdd(left int64, right int64) int64 {
	if left <= 0 {
		return max(int64(0), right)
	}
	if right <= 0 {
		return left
	}
	if left > int64(^uint64(0)>>1)-right {
		return int64(^uint64(0) >> 1)
	}
	return left + right
}

func requireStormTroopSupportMead(gameData *GameData.Store, target State.CastleState) error {
	if target.KingdomID != State.KingdomID(GameData.StormKingdomID) {
		return nil
	}
	meadID, err := officialResourceIDByJSONKey(gameData, "MEAD")
	if err != nil {
		return fmt.Errorf("verify Storm troop support: %w", err)
	}
	balance, observed := target.Resources[meadID]
	if !observed || target.FoodStateObservedAt.IsZero() {
		return fmt.Errorf("Storm Mead balance is not current; refresh Storm before transferring troops")
	}
	if balance.Amount < GameData.StormTroopSupportMead {
		return fmt.Errorf(
			"Storm has %.0f Mead; at least %.0f Mead is required before receiving troops",
			balance.Amount, float64(GameData.StormTroopSupportMead),
		)
	}
	return nil
}

func (application *Application) consumeKingdomTroopSource(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	amounts := map[State.UnitID]int64{}
	for _, unit := range request.Units {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return fmt.Errorf("confirmed kingdom troop transfer has invalid unit data")
		}
		amounts[unit.UnitID] += unit.Amount
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentCastles, State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, workflowExists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if workflowExists && workflow.ID == request.WorkflowID && workflow.Owner == request.Owner &&
			(workflow.SourceDebitedLocally || !workflow.SourceReconciledAt.IsZero()) {
			return nil, false, nil
		}
		source, found := gameState.MutableCastleParts(request.SourceCastleID, State.CastlePartUnits)
		if !found {
			return nil, false, fmt.Errorf("confirmed kingdom troop donor %d is unavailable", request.SourceCastleID)
		}
		authoritativeAfterResponse := workflowExists && workflow.ID == request.WorkflowID && workflow.Owner == request.Owner &&
			!workflow.TransportObservedAt.IsZero() && source.UnitsObservedAt.After(workflow.TransportObservedAt)
		for unitID, amount := range amounts {
			if !authoritativeAfterResponse && source.Units.Stationed[unitID] < amount {
				return nil, false, fmt.Errorf(
					"confirmed kingdom troop donor %d has only %d of unit %d in state; %d were transferred",
					source.ID, source.Units.Stationed[unitID], unitID, amount,
				)
			}
		}
		if !authoritativeAfterResponse {
			for unitID, amount := range amounts {
				source.Units.Stationed[unitID] -= amount
				if source.Units.Total[unitID] >= amount {
					source.Units.Total[unitID] -= amount
				}
			}
			gameState.SetCastleParts(source.ID, source, State.CastlePartUnits)
		}
		if workflowExists && workflow.ID == request.WorkflowID && workflow.Owner == request.Owner {
			if authoritativeAfterResponse {
				workflow.SourceReconciledAt = source.UnitsObservedAt.UTC()
			} else {
				workflow.SourceDebitedLocally = true
			}
			gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		}
		return []string{"castles", "units", "kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) armKingdomTroopWorkflow(ctx context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if strings.TrimSpace(request.Owner) == "" || strings.TrimSpace(request.WorkflowID) == "" {
		return fmt.Errorf("owned kingdom troop workflow identity is required")
	}
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		if current, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]; exists {
			if current.ID == request.WorkflowID && current.Owner == request.Owner && current.Status == "armed" {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("kingdom %d already has an owned troop workflow", request.TargetKingdomID)
		}
		units := make([]State.KingdomTransportUnit, 0, len(request.Units))
		for _, unit := range request.Units {
			units = append(units, State.KingdomTransportUnit{UnitID: unit.UnitID, Amount: unit.Amount})
		}
		if gameState.KingdomTransport.TroopWorkflows == nil {
			gameState.KingdomTransport.TroopWorkflows = map[State.KingdomID]State.KingdomTroopTransportWorkflow{}
		}
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = State.KingdomTroopTransportWorkflow{
			ID: request.WorkflowID, Owner: request.Owner, Status: "armed", KingdomID: request.TargetKingdomID,
			SourceCastleID: request.SourceCastleID, TargetCastleID: request.TargetCastleID, Units: units,
			ArmedAt: now, SessionGeneration: gameState.Session.ConnectionGeneration,
		}
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) guardKingdomTroopWorkflowDispatch(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameState := application.State.ReadOnlyView()
	workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
	if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.Status != "armed" {
		return fmt.Errorf("%w: owned kingdom troop workflow changed before dispatch", Intent.ErrPlanStale)
	}
	if workflow.SessionGeneration == 0 || workflow.SessionGeneration != gameState.Session.ConnectionGeneration {
		return fmt.Errorf("%w: game session changed before kingdom troop dispatch", Intent.ErrPlanStale)
	}
	if !autoFortressKingdomEnabled(application, request.TargetKingdomID) {
		return fmt.Errorf("%w: Auto Fortress destination was disabled before dispatch", Intent.ErrPlanStale)
	}
	unlock, observed := gameState.KingdomTransport.Unlocks[request.TargetKingdomID]
	if !observed || !unlock.Unlocked || kingdomTroopTransportPending(gameState, request.TargetKingdomID) {
		return fmt.Errorf("%w: kingdom troop transport availability changed before dispatch", Intent.ErrPlanStale)
	}
	source, sourceFound := gameState.Castles[request.SourceCastleID]
	target, targetFound := gameState.Castles[request.TargetCastleID]
	now := time.Now().UTC()
	if !sourceFound || !targetFound || target.KingdomID != request.TargetKingdomID || source.UnitsObservedAt.IsZero() ||
		now.Before(source.UnitsObservedAt) || now.Sub(source.UnitsObservedAt) > 5*time.Minute {
		return fmt.Errorf("%w: kingdom troop castle inventory changed before dispatch", Intent.ErrPlanStale)
	}
	_, err := normalizeKingdomTroopShipment(currentGameData(application), source, request.Units)
	return err
}

func (application *Application) disarmKingdomTroopWorkflow(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || current.ID != request.WorkflowID || current.Owner != request.Owner || current.Status != "armed" {
			return nil, false, nil
		}
		delete(gameState.KingdomTransport.TroopWorkflows, request.TargetKingdomID)
		return []string{"kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) confirmKingdomTroopWorkflow(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	view := application.State.ReadOnlyView()
	workflow, exists := view.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
	if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.Status != "pending" {
		return fmt.Errorf("confirmed kingdom troop response did not project the exact owned shipment")
	}
	return nil
}

func currentGameData(application *Application) *GameData.Store {
	if application == nil || application.GameData == nil {
		return nil
	}
	store, ready := application.GameData.Current()
	if !ready {
		return nil
	}
	return store
}

func autoFortressKingdomEnabled(application *Application, kingdomID State.KingdomID) bool {
	if application == nil || application.Configuration == nil {
		return false
	}
	raw, found := application.Configuration.Section("automation.autoFortress")
	if !found {
		return false
	}
	var settings struct {
		UseTimeSkips bool `json:"useTimeSkips"`
		Kingdoms     map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"kingdoms"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false
	}
	return settings.Kingdoms[strconv.FormatInt(int64(kingdomID), 10)].Enabled
}

func (application *Application) settleKingdomTroopWorkflow(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		Owner           string          `json:"owner"`
		WorkflowID      string          `json:"workflowId"`
		TargetKingdomID State.KingdomID `json:"targetKingdomId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport, State.ComponentCastles), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.Owner != request.Owner || workflow.ID != request.WorkflowID {
			return nil, false, fmt.Errorf("owned kingdom troop workflow changed before settlement")
		}
		if workflow.Status != "awaiting_destination_refresh" {
			return nil, false, fmt.Errorf("owned kingdom troop workflow is not ready to settle")
		}
		if !workflow.SkipRequestedAt.IsZero() {
			return nil, false, fmt.Errorf("owned kingdom troop workflow still has an unresolved time skip")
		}
		if workflow.SourceReconciledAt.IsZero() && !workflow.SourceDebitedLocally {
			return nil, false, fmt.Errorf("owned kingdom troop donor inventory is not reconciled")
		}
		target, exists := gameState.Castles[workflow.TargetCastleID]
		if !exists || target.UnitsObservedAt.IsZero() || !target.UnitsObservedAt.After(workflow.TransportObservedAt) {
			return nil, false, fmt.Errorf("destination inventory was not refreshed after transport completion")
		}
		delete(gameState.KingdomTransport.TroopWorkflows, request.TargetKingdomID)
		return []string{"kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) reconcileKingdomTroopDonor(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		Owner           string          `json:"owner"`
		WorkflowID      string          `json:"workflowId"`
		TargetKingdomID State.KingdomID `json:"targetKingdomId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport, State.ComponentCastles), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.Owner != request.Owner || workflow.ID != request.WorkflowID {
			return nil, false, fmt.Errorf("owned kingdom troop workflow changed before donor reconciliation")
		}
		source, exists := gameState.Castles[workflow.SourceCastleID]
		if !exists || source.UnitsObservedAt.IsZero() || !source.UnitsObservedAt.After(workflow.ArmedAt) {
			return nil, false, fmt.Errorf("donor inventory was not refreshed after the ambiguous troop dispatch")
		}
		workflow.SourceReconciledAt = source.UnitsObservedAt.UTC()
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) armKingdomTroopSkip(ctx context.Context, arguments json.RawMessage) error {
	request, workflow, currencyID, balance, remaining, duration, err := application.validateOwnedKingdomTroopSkip(arguments, true)
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		current, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || current.ID != workflow.ID || current.Owner != workflow.Owner || current.Status != "pending" {
			return nil, false, fmt.Errorf("%w: owned troop workflow changed before skip", Intent.ErrPlanStale)
		}
		if !current.SkipRequestedAt.IsZero() {
			return nil, false, fmt.Errorf("owned troop workflow already has an unresolved time skip")
		}
		current.SkipCurrencyID = currencyID
		current.SkipWireKey = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
		current.SkipBalanceBefore = balance
		current.SkipRemainingBefore = remaining
		current.SkipDurationSec = duration
		current.SkipRequestedAt = now
		current.SkipTimerObservedAt = time.Time{}
		current.SkipInventoryObservedAt = time.Time{}
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = current
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
}

func (application *Application) guardKingdomTroopSkipDispatch(_ context.Context, arguments json.RawMessage) error {
	request, workflow, currencyID, balance, _, duration, err := application.validateOwnedKingdomTroopSkip(arguments, false)
	if err != nil {
		return err
	}
	if workflow.SkipCurrencyID != currencyID || workflow.SkipWireKey != strings.ToUpper(strings.TrimSpace(request.TimeSkipID)) ||
		workflow.SkipBalanceBefore != balance || workflow.SkipDurationSec != duration || workflow.SkipRequestedAt.IsZero() {
		return fmt.Errorf("%w: owned troop time-skip marker changed before dispatch", Intent.ErrPlanStale)
	}
	return nil
}

func (application *Application) validateOwnedKingdomTroopSkip(arguments json.RawMessage, beforeArm bool) (kingdomTroopSkipRequest, State.KingdomTroopTransportWorkflow, State.CurrencyID, float64, int, int64, error) {
	var request kingdomTroopSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.KingdomTroopTransportWorkflow{}, 0, 0, 0, 0, err
	}
	gameState := application.State.ReadOnlyView()
	workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
	if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.Status != "pending" {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: owned troop workflow is no longer pending", Intent.ErrPlanStale)
	}
	if workflow.SessionGeneration == 0 || workflow.SessionGeneration != gameState.Session.ConnectionGeneration {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: owned troop workflow requires current-session reconciliation", Intent.ErrPlanStale)
	}
	useSkips, reserves, enabled := autoFortressSkipSettings(application, request.TargetKingdomID)
	if !enabled || !useSkips {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: Auto Fortress time skips are disabled", Intent.ErrPlanStale)
	}
	now := time.Now().UTC()
	if workflow.TransportObservedAt.IsZero() || now.Before(workflow.TransportObservedAt) ||
		now.Sub(workflow.TransportObservedAt) > 5*time.Minute {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: owned troop transport timer is not current", Intent.ErrPlanStale)
	}
	remaining, exact := agedOwnedPendingRemaining(gameState, workflow, now)
	if !exact || remaining <= 0 || request.ExpectedRemaining <= 0 || remaining > request.ExpectedRemaining || request.ExpectedRemaining-remaining > 10 {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: owned troop transport timer changed before skip", Intent.ErrPlanStale)
	}
	gameData := currentGameData(application)
	currencyID, err := officialCurrencyID(gameData, request.TimeSkipID)
	if err != nil {
		return request, workflow, 0, 0, 0, 0, err
	}
	duration, err := officialKingdomTroopSkipDuration(gameData, currencyID, request.TimeSkipID)
	if err != nil || duration != request.ExpectedDurationSec || !kingdomTroopSkipUseful(duration, remaining) {
		return request, workflow, 0, 0, 0, 0, fmt.Errorf("%w: queued time skip is no longer suitable for the owned transport", Intent.ErrPlanStale)
	}
	balance := gameState.Player.Currencies[currencyID]
	observation := gameState.Player.CurrencyObservations[currencyID]
	if observation.ObservedAt.IsZero() || observation.ConnectionGeneration == 0 ||
		observation.ConnectionGeneration != gameState.Session.ConnectionGeneration || now.Before(observation.ObservedAt) || now.Sub(observation.ObservedAt) > 5*time.Minute {
		return request, workflow, 0, balance, 0, 0, fmt.Errorf("%w: official time-skip inventory is not current", Intent.ErrPlanStale)
	}
	reserve := max(int64(0), reserves[strings.ToUpper(strings.TrimSpace(request.TimeSkipID))])
	if balance < float64(reserve)+1 {
		return request, workflow, 0, balance, 0, 0, fmt.Errorf("%w: time-skip reserve is no longer satisfied", Intent.ErrPlanStale)
	}
	if beforeArm && !workflow.SkipRequestedAt.IsZero() {
		return request, workflow, 0, balance, 0, 0, fmt.Errorf("owned troop workflow already has an unresolved time skip")
	}
	return request, workflow, currencyID, balance, remaining, duration, nil
}

func agedOwnedPendingRemaining(gameState State.GameState, workflow State.KingdomTroopTransportWorkflow, now time.Time) (int, bool) {
	remaining, found := exactOwnedPendingRemaining(gameState, workflow)
	if !found || workflow.TransportObservedAt.IsZero() || !now.After(workflow.TransportObservedAt) {
		return remaining, found
	}
	elapsed := int(now.Sub(workflow.TransportObservedAt) / time.Second)
	return max(0, remaining-elapsed), true
}

func officialKingdomTroopSkipDuration(gameData *GameData.Store, currencyID State.CurrencyID, wireKey string) (int64, error) {
	if gameData == nil {
		return 0, fmt.Errorf("official game data is unavailable")
	}
	options, err := gameData.OfficialTimeSkips()
	if err != nil {
		return 0, err
	}
	wanted := strings.ToUpper(strings.TrimSpace(wireKey))
	for _, option := range options {
		if State.CurrencyID(option.CurrencyID) == currencyID && option.WireKey == wanted && option.Seconds > 0 {
			return option.Seconds, nil
		}
	}
	return 0, fmt.Errorf("official time skip %s is unavailable", wanted)
}

func kingdomTroopSkipUseful(duration int64, remaining int) bool {
	if duration <= 0 || remaining <= 0 {
		return false
	}
	if duration <= int64(remaining) {
		return true
	}
	maximumWaste := max(int64(60), min(int64(15*time.Minute/time.Second), int64(remaining)/4))
	return duration-int64(remaining) <= maximumWaste
}

func exactOwnedPendingRemaining(gameState State.GameState, workflow State.KingdomTroopTransportWorkflow) (int, bool) {
	for _, pending := range gameState.KingdomTransport.PendingUnits {
		if pending.KingdomID == workflow.KingdomID && kingdomTransportUnitsExact(pending.Units, workflow.Units) {
			return pending.RemainingSec, true
		}
	}
	return 0, false
}

func kingdomTransportUnitsExact(left, right []State.KingdomTransportUnit) bool {
	if len(left) != len(right) || len(left) == 0 {
		return false
	}
	amounts := map[State.UnitID]int64{}
	for _, unit := range left {
		amounts[unit.UnitID] += unit.Amount
	}
	if len(amounts) != len(right) {
		return false
	}
	for _, unit := range right {
		if unit.Amount <= 0 || amounts[unit.UnitID] != unit.Amount {
			return false
		}
	}
	return true
}

func autoFortressSkipSettings(application *Application, kingdomID State.KingdomID) (bool, map[string]int64, bool) {
	if application == nil || application.Configuration == nil {
		return false, nil, false
	}
	raw, found := application.Configuration.Section("automation.autoFortress")
	if !found {
		return false, nil, false
	}
	var settings struct {
		UseTimeSkips    bool             `json:"useTimeSkips"`
		TimeSkipReserve map[string]int64 `json:"timeSkipReserve"`
		Kingdoms        map[string]struct {
			Enabled bool `json:"enabled"`
		} `json:"kingdoms"`
	}
	if json.Unmarshal(raw, &settings) != nil {
		return false, nil, false
	}
	return settings.UseTimeSkips, settings.TimeSkipReserve, settings.Kingdoms[strconv.FormatInt(int64(kingdomID), 10)].Enabled
}

func (application *Application) disarmKingdomTroopSkip(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.SkipRequestedAt.IsZero() {
			return nil, false, nil
		}
		clearKingdomTroopSkip(&workflow)
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) verifyKingdomTroopSkipTimer(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.SkipRequestedAt.IsZero() {
			return nil, false, fmt.Errorf("owned troop time-skip marker is unavailable")
		}
		if !workflow.TransportObservedAt.After(workflow.SkipRequestedAt) {
			return nil, false, fmt.Errorf("time-skip response did not project current transport state")
		}
		remaining, pending := exactOwnedPendingRemaining(*gameState, workflow)
		elapsed := int(workflow.TransportObservedAt.Sub(workflow.SkipRequestedAt) / time.Second)
		expectedAfterSkip := max(0, workflow.SkipRemainingBefore-elapsed-int(workflow.SkipDurationSec))
		if workflow.SkipDurationSec <= 0 || pending && remaining > expectedAfterSkip+5 || !pending && expectedAfterSkip > 5 {
			workflow.SkipTimerObservedAt = workflow.TransportObservedAt.UTC()
			gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
			return nil, false, fmt.Errorf("owned transport timer changed only by natural countdown; time-skip progress is unconfirmed")
		}
		workflow.Status = "skip_inventory_pending"
		workflow.RemainingSec = remaining
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport"}, true, nil
	})
	return err
}

func (application *Application) verifyKingdomTroopSkipInventory(_ context.Context, arguments json.RawMessage) error {
	var request kingdomTroopSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	view := application.State.ReadOnlyView()
	workflow, exists := view.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
	if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner ||
		(workflow.Status != "skip_inventory_pending" && workflow.Status != "skip_uncertain") {
		return fmt.Errorf("owned troop time-skip is not awaiting inventory confirmation")
	}
	observation := view.Player.CurrencyObservations[workflow.SkipCurrencyID]
	if !observation.ObservedAt.After(workflow.SkipRequestedAt) || observation.ConnectionGeneration == 0 ||
		observation.ConnectionGeneration != view.Session.ConnectionGeneration {
		return fmt.Errorf("official time-skip inventory was not refreshed after the spend")
	}
	if view.Player.Currencies[workflow.SkipCurrencyID] > workflow.SkipBalanceBefore-1 {
		_, markErr := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
			current, found := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
			if !found || current.ID != request.WorkflowID || current.Owner != request.Owner {
				return nil, false, nil
			}
			current.Status = "skip_uncertain"
			current.SkipInventoryObservedAt = observation.ObservedAt.UTC()
			gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = current
			return []string{"kingdom-transport"}, true, nil
		})
		if markErr != nil {
			return markErr
		}
		return fmt.Errorf("transport timer advanced but official inventory did not confirm time-skip consumption")
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport, State.ComponentPlayer), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner ||
			(workflow.Status != "skip_inventory_pending" && workflow.Status != "skip_uncertain") {
			return nil, false, fmt.Errorf("owned troop time-skip is not awaiting inventory confirmation")
		}
		if _, pending := exactOwnedPendingRemaining(*gameState, workflow); pending {
			workflow.Status = "pending"
		} else {
			workflow.Status = "awaiting_destination_refresh"
		}
		clearKingdomTroopSkip(&workflow)
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport", "currencies"}, true, nil
	})
	return err
}

func clearKingdomTroopSkip(workflow *State.KingdomTroopTransportWorkflow) {
	workflow.SkipCurrencyID = 0
	workflow.SkipWireKey = ""
	workflow.SkipBalanceBefore = 0
	workflow.SkipRemainingBefore = 0
	workflow.SkipDurationSec = 0
	workflow.SkipRequestedAt = time.Time{}
	workflow.SkipTimerObservedAt = time.Time{}
	workflow.SkipInventoryObservedAt = time.Time{}
}

func normalizeKingdomTroopShipment(
	gameData *GameData.Store,
	source State.CastleState,
	requested []kingdomTroopShipmentUnit,
) ([]kingdomTroopShipmentUnit, error) {
	if gameData == nil {
		return nil, fmt.Errorf("official game data is unavailable")
	}
	if len(requested) == 0 || len(requested) > kingdomTroopMaximumStacks {
		return nil, fmt.Errorf("units must contain between 1 and %d troop stacks", kingdomTroopMaximumStacks)
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return nil, err
	}
	merged := map[State.UnitID]int64{}
	for _, unit := range requested {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return nil, fmt.Errorf("every transferred troop requires a positive unitId and amount")
		}
		raw, found := catalog.Find(strconv.FormatInt(int64(unit.UnitID), 10))
		if !found {
			return nil, fmt.Errorf("unit %d is not in the official unit catalog", unit.UnitID)
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if GameData.IsToolRecord(record) {
			return nil, fmt.Errorf("unit %d is a tool and cannot use kingdom troop transport", unit.UnitID)
		}
		if merged[unit.UnitID] > int64(^uint64(0)>>1)-unit.Amount {
			return nil, fmt.Errorf("transfer amount for unit %d is too large", unit.UnitID)
		}
		merged[unit.UnitID] += unit.Amount
	}
	result := make([]kingdomTroopShipmentUnit, 0, len(merged))
	for unitID, amount := range merged {
		if source.Units.Stationed[unitID] < amount {
			return nil, fmt.Errorf("castle %d has %d stationed unit %d; %d requested", source.ID, source.Units.Stationed[unitID], unitID, amount)
		}
		result = append(result, kingdomTroopShipmentUnit{UnitID: unitID, Amount: amount})
	}
	sort.Slice(result, func(left, right int) bool { return result[left].UnitID < result[right].UnitID })
	return result, nil
}

func planKingdomTroopSkip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request kingdomTroopSkipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	step, currencyID, timeSkipLabel, err := kingdomTroopSkipStep(input, request, true)
	if err != nil {
		return Intent.Plan{}, err
	}
	steps := []Intent.Step{step, timeSkipConsumeStep(input, currencyID)}
	if strings.TrimSpace(request.Owner) != "" {
		workflow, exists := input.State.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.Owner != request.Owner || workflow.ID != request.WorkflowID || workflow.Status != "pending" {
			return Intent.Plan{}, fmt.Errorf("owned kingdom troop transport is not pending with the expected workflow")
		}
		if request.ExpectedRemaining <= 0 {
			return Intent.Plan{}, fmt.Errorf("owned kingdom troop skip requires expectedRemaining")
		}
		requestArguments, _ := json.Marshal(request)
		step.PreDispatchAction = "troops.kingdom.skip.arm"
		step.PreDispatchArguments = requestArguments
		step.FinalDispatchAction = "troops.kingdom.skip.dispatch"
		step.FinalDispatchArguments = requestArguments
		step.DefinitiveSendFailureAction = "troops.kingdom.skip.disarm"
		step.DefinitiveSendFailureArguments = requestArguments
		step.DefinitiveResponseFailureAction = "troops.kingdom.skip.disarm"
		step.DefinitiveResponseFailureArguments = requestArguments
		step.ResponseProjectionFailureIndeterminate = true
		_, found := input.State.Castles[workflow.SourceCastleID]
		if !found {
			return Intent.Plan{}, fmt.Errorf("owned kingdom troop donor is unavailable")
		}
		steps = []Intent.Step{
			step,
			{Name: "Verify kingdom troop timer advanced", Action: "troops.kingdom.skip.verify_timer", ActionArguments: requestArguments},
			accountInventoryRefreshStep("Refresh official time-skip inventory"),
			{Name: "Confirm official time-skip consumption", Action: "troops.kingdom.skip.verify_inventory", ActionArguments: requestArguments},
		}
	}
	return Intent.Plan{
		Claims: []string{
			"troop-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
			"currency:" + strconv.FormatInt(int64(currencyID), 10),
		},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d troop transport", timeSkipLabel, request.TargetKingdomID),
		Steps:   steps,
	}, nil
}

func accountInventoryRefreshStep(name string) Intent.Step {
	step := commandStep(name, "gbd", nil, "gbd")
	step.Command = Protocol.Command{Opcode: "gbd", Bare: true}
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	step.CaptureResponse = false
	return step
}

func planAccountInventoryRefresh(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct{}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	return Intent.Plan{
		Claims:  []string{"account-resources", "account-currencies"},
		Summary: "Refresh authoritative account inventory",
		Steps:   []Intent.Step{accountInventoryRefreshStep("Refresh authoritative account inventory")},
	}, nil
}

func kingdomTroopSkipStep(
	input Intent.PlanningContext,
	request kingdomTroopSkipRequest,
	requirePending bool,
) (Intent.Step, State.CurrencyID, string, error) {
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Step{}, 0, "", fmt.Errorf("targetKingdomId and timeSkipId are required")
	}
	if requirePending {
		pending := false
		for _, transport := range input.State.KingdomTransport.PendingUnits {
			if transport.KingdomID == request.TargetKingdomID && transport.RemainingSec > 0 {
				pending = true
				break
			}
		}
		if !pending {
			return Intent.Step{}, 0, "", fmt.Errorf("kingdom %d has no pending troop transport", request.TargetKingdomID)
		}
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Step{}, 0, "", err
	}
	timeSkipLabel := officialTimeSkipLabel(input.GameData, int64(currencyID), request.TimeSkipID)
	if request.MinimumRemaining < 0 {
		return Intent.Step{}, 0, "", fmt.Errorf("minimumRemaining cannot be negative")
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
		return Intent.Step{}, 0, "", fmt.Errorf("no %s is available", timeSkipLabel)
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "1",
	})
	return commandStep("Skip kingdom troop transport time", "msk", payload, "msk"), currencyID, timeSkipLabel, nil
}

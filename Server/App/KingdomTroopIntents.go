package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/Configuration"
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
		Claims: []string{"troop-transport"}, Summary: "Refresh kingdom troop transports", SummaryDescriptor: Localization.New("server.app.refresh_kingdom_troop_transports.ab139211", "Refresh kingdom troop transports", nil),
		Steps: []Intent.Step{commandStep("Refresh kingdom troop transports", "kpi", json.RawMessage(`{}`), "kpi", Localization.New("server.app.refresh_kingdom_troop_transports.ab139211", "Refresh kingdom troop transports", nil))},
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
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if !targetExists || target.ID <= 0 || target.KingdomID != request.TargetKingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("target castle %d is not in kingdom %d", request.TargetCastleID, request.TargetKingdomID), Localization.New("server.app.target_castle_p_is.9e66647b", "target castle {p0} is not in kingdom {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID), "p1": fmt.Sprintf("%d", request.TargetKingdomID)}))
	}
	if source.ID == target.ID || source.KingdomID == target.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom troop transfers require castles in different kingdoms"), Localization.New("server.app.kingdom_troop_transfers_require.fcfd252b", "kingdom troop transfers require castles in different kingdoms", nil))
	}
	if err := verifyKingdomTroopExpectedDailyAttackSession(input.State, request.ExpectedDailyAttackSessionStartedAt); err != nil {
		return Intent.Plan{}, err
	}
	if err := requireStormTroopSupportMead(input.GameData, target); err != nil {
		return Intent.Plan{}, err
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[target.KingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom troop transport to %d is not observed as unlocked", target.KingdomID), Localization.New("server.app.kingdom_troop_transport_to.4cc4aa9e", "kingdom troop transport to {p0} is not observed as unlocked", Localization.Params{"p0": fmt.Sprintf("%d", target.KingdomID)}))
	}
	if kingdomTroopTransportPending(input.State, target.KingdomID) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("kingdom %d already has a pending or settling troop transport", target.KingdomID), Localization.New("server.app.kingdom_p_already_has.d3109204", "kingdom {p0} already has a pending or settling troop transport", Localization.Params{"p0": fmt.Sprintf("%d", target.KingdomID)}))
	}
	units, err := normalizeKingdomTroopShipment(input.GameData, source, request.Units)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.MaximumTargetTroops < 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("maximumTargetTroops cannot be negative"), Localization.New("server.app.maximumtargettroops_cannot_be_negative.0bf12edd", "maximumTargetTroops cannot be negative", nil))
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
			Name: "Verify kingdom troop transport availability", NameDescriptor: Localization.New("server.app.verify_kingdom_troop_transport.a230eeb3", "Verify kingdom troop transport availability", nil), Action: "kingdom.transport.verify_available",
			ActionArguments: guardArguments,
		}),
	)
	if request.MaximumTargetTroops > 0 {
		steps = append(steps, Intent.Step{
			Name: "Verify target troop inventory cap", NameDescriptor: Localization.New("server.app.verify_target_troop_inventory.bbbd4ce6", "Verify target troop inventory cap", nil), Action: "troops.kingdom.guard_target_cap",
			ActionArguments: consumeArguments,
		})
	}
	steps = append(steps, commandStep("Start kingdom troop transfer", "kut", payload, "kut", Localization.New("server.app.start_kingdom_troop_transfer.9fd0d909", "Start kingdom troop transfer", nil)))
	if strings.TrimSpace(request.Owner) != "" {
		if strings.TrimSpace(request.WorkflowID) == "" {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("owned kingdom troop transfer requires workflowId"), Localization.New("server.app.owned_kingdom_troop_transfer.e62d15f9", "owned kingdom troop transfer requires workflowId", nil))
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
			Name: "Confirm owned kingdom troop transfer", NameDescriptor: Localization.New("server.app.confirm_owned_kingdom_troop.c286a9a3", "Confirm owned kingdom troop transfer", nil), Action: "troops.kingdom.workflow.confirm", ActionArguments: consumeArguments,
		})
	}
	steps = append(steps, Intent.Step{Name: "Consume confirmed donor troops", NameDescriptor: Localization.New("server.app.consume_confirmed_donor_troops.ed0c79db", "Consume confirmed donor troops", nil), Action: "troops.kingdom.consume_source", ActionArguments: consumeArguments})
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Transfer %s from %s to %s", strings.Join(summaryUnits, ", "), castleLabel(source), castleLabel(target)), SummaryDescriptor: Localization.New("server.app.transfer_p_from_p.5557592b", "Transfer {p0} from {p1} to {p2}", Localization.Params{"p0": fmt.Sprintf("%s", strings.Join(summaryUnits, ", ")), "p1": fmt.Sprintf("%s", castleLabel(source)), "p2": fmt.Sprintf("%s", castleLabel(target))}),
		Steps: steps,
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
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	gameState := application.State.ReadOnlyView()
	if err := verifyKingdomTroopExpectedDailyAttackSession(gameState, request.ExpectedDailyAttackSessionStartedAt); err != nil {
		return err
	}
	source, sourceExists := gameState.Castles[request.SourceCastleID]
	target, targetExists := gameState.Castles[request.TargetCastleID]
	if !sourceExists || !targetExists || target.KingdomID != request.TargetKingdomID {
		return Localization.WithError(fmt.Errorf("kingdom troop transfer castles changed before dispatch"), Localization.New("server.app.kingdom_troop_transfer_castles.30324f57", "kingdom troop transfer castles changed before dispatch", nil))
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
		return Localization.WithError(fmt.Errorf("%w: the daily attack reset changed after the troop cap was calculated", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.330befee", "intent plan became stale before dispatch: the daily attack reset changed after the troop cap was calculated", nil))
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
		return 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
		return Localization.WithError(fmt.Errorf("verify Storm troop support: %w", err), Localization.ErrorContext(Localization.New("server.app.verify_storm_troop_support.ebbf0163", "verify Storm troop support", nil), err))
	}
	balance, observed := target.Resources[meadID]
	if !observed || target.FoodStateObservedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Storm Mead balance is not current; refresh Storm before transferring troops"), Localization.New("server.app.storm_mead_balance_is.2bba2f2d", "Storm Mead balance is not current; refresh Storm before transferring troops", nil))
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
			return Localization.WithError(fmt.Errorf("confirmed kingdom troop transfer has invalid unit data"), Localization.New("server.app.confirmed_kingdom_troop_transfer.f3c2084b", "confirmed kingdom troop transfer has invalid unit data", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("confirmed kingdom troop donor %d is unavailable", request.SourceCastleID), Localization.New("server.app.confirmed_kingdom_troop_donor.579cc1a9", "confirmed kingdom troop donor {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
		}
		authoritativeAfterResponse := workflowExists && workflow.ID == request.WorkflowID && workflow.Owner == request.Owner &&
			!workflow.TransportObservedAt.IsZero() && source.UnitsObservedAt.After(workflow.TransportObservedAt)
		for unitID, amount := range amounts {
			if !authoritativeAfterResponse && source.Units.Stationed[unitID] < amount {
				return nil, false, Localization.WithError(fmt.Errorf(
					"confirmed kingdom troop donor %d has only %d of unit %d in state; %d were transferred",
					source.ID, source.Units.Stationed[unitID], unitID, amount,
				), Localization.New("server.app.confirmed_kingdom_troop_donor.a770767e", "confirmed kingdom troop donor {p0} has only {p1} of unit {p2} in state; {p3} were transferred", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": fmt.Sprintf("%d", source.Units.Stationed[unitID]), "p2": fmt.Sprintf("%d", unitID), "p3": amount}))
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
		return Localization.WithError(fmt.Errorf("owned kingdom troop workflow identity is required"), Localization.New("server.app.owned_kingdom_troop_workflow.d8dde8a3", "owned kingdom troop workflow identity is required", nil))
	}
	now := time.Now().UTC()
	event, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		if current, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]; exists {
			if current.ID == request.WorkflowID && current.Owner == request.Owner && current.Status == "armed" {
				return nil, false, nil
			}
			return nil, false, Localization.WithError(fmt.Errorf("kingdom %d already has an owned troop workflow", request.TargetKingdomID), Localization.New("server.app.kingdom_p_already_has.88f3bbe7", "kingdom {p0} already has an owned troop workflow", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
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

func (application *Application) guardKingdomTroopWorkflowDispatch(ctx context.Context, arguments json.RawMessage) error {
	var request kingdomTroopShipmentRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	gameState := application.State.ReadOnlyView()
	workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
	if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.Status != "armed" {
		return Localization.WithError(fmt.Errorf("%w: owned kingdom troop workflow changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.85fe9706", "intent plan became stale before dispatch: owned kingdom troop workflow changed before dispatch", nil))
	}
	if workflow.SessionGeneration == 0 || workflow.SessionGeneration != gameState.Session.ConnectionGeneration {
		return Localization.WithError(fmt.Errorf("%w: game session changed before kingdom troop dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a8237aa4", "intent plan became stale before dispatch: game session changed before kingdom troop dispatch", nil))
	}
	if !autoFortressKingdomEnabled(application, request.TargetKingdomID) {
		return Localization.WithError(fmt.Errorf("%w: Auto Fortress destination was disabled before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.2583476b", "intent plan became stale before dispatch: Auto Fortress destination was disabled before dispatch", nil))
	}
	unlock, observed := gameState.KingdomTransport.Unlocks[request.TargetKingdomID]
	if !observed || !unlock.Unlocked || kingdomTroopTransportPending(gameState, request.TargetKingdomID) {
		return Localization.WithError(fmt.Errorf("%w: kingdom troop transport availability changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.f515a900", "intent plan became stale before dispatch: kingdom troop transport availability changed before dispatch", nil))
	}
	source, sourceFound := gameState.Castles[request.SourceCastleID]
	target, targetFound := gameState.Castles[request.TargetCastleID]
	now := time.Now().UTC()
	if !sourceFound || !targetFound || target.KingdomID != request.TargetKingdomID || source.UnitsObservedAt.IsZero() ||
		now.Before(source.UnitsObservedAt) || now.Sub(source.UnitsObservedAt) > 5*time.Minute {
		return Localization.WithError(fmt.Errorf("%w: kingdom troop castle inventory changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.433f0f9e", "intent plan became stale before dispatch: kingdom troop castle inventory changed before dispatch", nil))
	}
	if _, err := normalizeKingdomTroopShipment(currentGameData(application), source, request.Units); err != nil {
		return err
	}
	event, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(current *State.GameState) ([]string, bool, error) {
		workflow, found := current.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !found || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.Status != "armed" {
			return nil, false, Localization.WithError(fmt.Errorf("%w: owned kingdom troop workflow changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.85fe9706", "intent plan became stale before dispatch: owned kingdom troop workflow changed before dispatch", nil))
		}
		workflow.LaunchedAt = now
		current.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		return err
	}
	return application.saveStateEvent(ctx, event)
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
		return Localization.WithError(fmt.Errorf("confirmed kingdom troop response did not project the exact owned shipment"), Localization.New("server.app.confirmed_kingdom_troop_response.2205d210", "confirmed kingdom troop response did not project the exact owned shipment", nil))
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
	return autoFortressKingdomEnabledInSnapshot(application.Configuration.Snapshot(), kingdomID)
}

func autoFortressKingdomEnabledInSnapshot(snapshot Configuration.Snapshot, kingdomID State.KingdomID) bool {
	raw, found := snapshot.Sections["automation.autoFortress"]
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
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop workflow changed before settlement"), Localization.New("server.app.owned_kingdom_troop_workflow.9558da46", "owned kingdom troop workflow changed before settlement", nil))
		}
		if workflow.Status != "awaiting_destination_refresh" && workflow.Status != "ownership_absent" {
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop workflow is not ready to settle"), Localization.New("server.app.owned_kingdom_troop_workflow.eab67af1", "owned kingdom troop workflow is not ready to settle", nil))
		}
		if !workflow.SkipRequestedAt.IsZero() {
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop workflow still has an unresolved time skip"), Localization.New("server.app.owned_kingdom_troop_workflow.60918ee0", "owned kingdom troop workflow still has an unresolved time skip", nil))
		}
		if workflow.SourceReconciledAt.IsZero() && !workflow.SourceDebitedLocally {
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop donor inventory is not reconciled"), Localization.New("server.app.owned_kingdom_troop_donor.006e6878", "owned kingdom troop donor inventory is not reconciled", nil))
		}
		target, exists := gameState.Castles[workflow.TargetCastleID]
		if !exists || target.UnitsObservedAt.IsZero() || !target.UnitsObservedAt.After(workflow.TransportObservedAt) {
			return nil, false, Localization.WithError(fmt.Errorf("destination inventory was not refreshed after transport completion"), Localization.New("server.app.destination_inventory_was_not.2fa31bc5", "destination inventory was not refreshed after transport completion", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop workflow changed before donor reconciliation"), Localization.New("server.app.owned_kingdom_troop_workflow.d0bc5526", "owned kingdom troop workflow changed before donor reconciliation", nil))
		}
		if workflow.SessionGeneration == 0 || workflow.SessionGeneration != gameState.Session.ConnectionGeneration || workflow.TransportObservedAt.IsZero() {
			return nil, false, Localization.WithError(fmt.Errorf("owned kingdom troop workflow lacks current-session transport authority"), Localization.New("server.app.owned_kingdom_troop_workflow.aeb84466", "owned kingdom troop workflow lacks current-session transport authority", nil))
		}
		source, exists := gameState.Castles[workflow.SourceCastleID]
		if !exists || source.UnitsObservedAt.IsZero() || !source.UnitsObservedAt.After(workflow.TransportObservedAt) {
			return nil, false, Localization.WithError(fmt.Errorf("donor inventory was not refreshed after the ambiguous troop dispatch"), Localization.New("server.app.donor_inventory_was_not.a9d164a5", "donor inventory was not refreshed after the ambiguous troop dispatch", nil))
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
			return nil, false, Localization.WithError(fmt.Errorf("%w: owned troop workflow changed before skip", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.313986c5", "intent plan became stale before dispatch: owned troop workflow changed before skip", nil))
		}
		if !current.SkipRequestedAt.IsZero() {
			return nil, false, Localization.WithError(fmt.Errorf("owned troop workflow already has an unresolved time skip"), Localization.New("server.app.owned_troop_workflow_already.fc5bd559", "owned troop workflow already has an unresolved time skip", nil))
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
		return Localization.WithError(fmt.Errorf("%w: owned troop time-skip marker changed before dispatch", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.aba4236d", "intent plan became stale before dispatch: owned troop time-skip marker changed before dispatch", nil))
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
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: owned troop workflow is no longer pending", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.41eb24f6", "intent plan became stale before dispatch: owned troop workflow is no longer pending", nil))
	}
	if workflow.SessionGeneration == 0 || workflow.SessionGeneration != gameState.Session.ConnectionGeneration {
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: owned troop workflow requires current-session reconciliation", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a03bea20", "intent plan became stale before dispatch: owned troop workflow requires current-session reconciliation", nil))
	}
	useSkips, reserves, enabled := autoFortressSkipSettings(application, request.TargetKingdomID)
	if !enabled || !useSkips {
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: Auto Fortress time skips are disabled", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.483981a9", "intent plan became stale before dispatch: Auto Fortress time skips are disabled", nil))
	}
	now := time.Now().UTC()
	if workflow.TransportObservedAt.IsZero() || now.Before(workflow.TransportObservedAt) ||
		now.Sub(workflow.TransportObservedAt) > 5*time.Minute {
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: owned troop transport timer is not current", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.86c8db4c", "intent plan became stale before dispatch: owned troop transport timer is not current", nil))
	}
	remaining, exact := agedOwnedPendingRemaining(gameState, workflow, now)
	if !exact || remaining <= 0 || request.ExpectedRemaining <= 0 || remaining > request.ExpectedRemaining || request.ExpectedRemaining-remaining > 10 {
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: owned troop transport timer changed before skip", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.86875178", "intent plan became stale before dispatch: owned troop transport timer changed before skip", nil))
	}
	gameData := currentGameData(application)
	currencyID, err := officialCurrencyID(gameData, request.TimeSkipID)
	if err != nil {
		return request, workflow, 0, 0, 0, 0, err
	}
	duration, err := officialKingdomTroopSkipDuration(gameData, currencyID, request.TimeSkipID)
	if err != nil || duration != request.ExpectedDurationSec || !kingdomTroopSkipUseful(duration, remaining) {
		return request, workflow, 0, 0, 0, 0, Localization.WithError(fmt.Errorf("%w: queued time skip is no longer suitable for the owned transport", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.58d0b51e", "intent plan became stale before dispatch: queued time skip is no longer suitable for the owned transport", nil))
	}
	balance := gameState.Player.Currencies[currencyID]
	observation := gameState.Player.CurrencyObservations[currencyID]
	if observation.ObservedAt.IsZero() || observation.ConnectionGeneration == 0 ||
		observation.ConnectionGeneration != gameState.Session.ConnectionGeneration || now.Before(observation.ObservedAt) || now.Sub(observation.ObservedAt) > 5*time.Minute {
		return request, workflow, 0, balance, 0, 0, Localization.WithError(fmt.Errorf("%w: official time-skip inventory is not current", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.955212b6", "intent plan became stale before dispatch: official time-skip inventory is not current", nil))
	}
	reserve := max(int64(0), reserves[strings.ToUpper(strings.TrimSpace(request.TimeSkipID))])
	if balance < float64(reserve)+1 {
		return request, workflow, 0, balance, 0, 0, Localization.WithError(fmt.Errorf("%w: time-skip reserve is no longer satisfied", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.27979763", "intent plan became stale before dispatch: time-skip reserve is no longer satisfied", nil))
	}
	if beforeArm && !workflow.SkipRequestedAt.IsZero() {
		return request, workflow, 0, balance, 0, 0, Localization.WithError(fmt.Errorf("owned troop workflow already has an unresolved time skip"), Localization.New("server.app.owned_troop_workflow_already.fc5bd559", "owned troop workflow already has an unresolved time skip", nil))
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
		return 0, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
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
	return 0, Localization.WithError(fmt.Errorf("official time skip %s is unavailable", wanted), Localization.New("server.app.official_time_skip_p.d418b707", "official time skip {p0} is unavailable", Localization.Params{"p0": fmt.Sprintf("%s", wanted)}))
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
	naturalCountdown := false
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner || workflow.SkipRequestedAt.IsZero() {
			return nil, false, Localization.WithError(fmt.Errorf("owned troop time-skip marker is unavailable"), Localization.New("server.app.owned_troop_time_skip.5dfec5a2", "owned troop time-skip marker is unavailable", nil))
		}
		if !workflow.TransportObservedAt.After(workflow.SkipRequestedAt) {
			return nil, false, Localization.WithError(fmt.Errorf("time-skip response did not project current transport state"), Localization.New("server.app.time_skip_response_did.a1a763e4", "time-skip response did not project current transport state", nil))
		}
		remaining, pending := exactOwnedPendingRemaining(*gameState, workflow)
		elapsed := int(workflow.TransportObservedAt.Sub(workflow.SkipRequestedAt) / time.Second)
		expectedAfterSkip := max(0, workflow.SkipRemainingBefore-elapsed-int(workflow.SkipDurationSec))
		if workflow.SkipDurationSec <= 0 || pending && remaining > expectedAfterSkip+5 || !pending && expectedAfterSkip > 5 {
			workflow.SkipTimerObservedAt = workflow.TransportObservedAt.UTC()
			gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
			naturalCountdown = true
			return []string{"kingdom-transport"}, true, nil
		}
		workflow.Status = "skip_inventory_pending"
		workflow.RemainingSec = remaining
		gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID] = workflow
		return []string{"kingdom-transport"}, true, nil
	})
	if err != nil {
		return err
	}
	if naturalCountdown {
		return Localization.WithError(fmt.Errorf("owned transport timer changed only by natural countdown; time-skip progress is unconfirmed"), Localization.New("server.app.owned_transport_timer_changed.71ab80b8", "owned transport timer changed only by natural countdown; time-skip progress is unconfirmed", nil))
	}
	return nil
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
		return Localization.WithError(fmt.Errorf("owned troop time-skip is not awaiting inventory confirmation"), Localization.New("server.app.owned_troop_time_skip.297e36ec", "owned troop time-skip is not awaiting inventory confirmation", nil))
	}
	observation := view.Player.CurrencyObservations[workflow.SkipCurrencyID]
	if !observation.ObservedAt.After(workflow.SkipRequestedAt) || observation.ConnectionGeneration == 0 ||
		observation.ConnectionGeneration != view.Session.ConnectionGeneration {
		return Localization.WithError(fmt.Errorf("official time-skip inventory was not refreshed after the spend"), Localization.New("server.app.official_time_skip_inventory.37fa0053", "official time-skip inventory was not refreshed after the spend", nil))
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
		return Localization.WithError(fmt.Errorf("transport timer advanced but official inventory did not confirm time-skip consumption"), Localization.New("server.app.transport_timer_advanced_but.d8de1ace", "transport timer advanced but official inventory did not confirm time-skip consumption", nil))
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentKingdomTransport, State.ComponentPlayer), func(gameState *State.GameState) ([]string, bool, error) {
		workflow, exists := gameState.KingdomTransport.TroopWorkflows[request.TargetKingdomID]
		if !exists || workflow.ID != request.WorkflowID || workflow.Owner != request.Owner ||
			(workflow.Status != "skip_inventory_pending" && workflow.Status != "skip_uncertain") {
			return nil, false, Localization.WithError(fmt.Errorf("owned troop time-skip is not awaiting inventory confirmation"), Localization.New("server.app.owned_troop_time_skip.297e36ec", "owned troop time-skip is not awaiting inventory confirmation", nil))
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
		return nil, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if len(requested) == 0 || len(requested) > kingdomTroopMaximumStacks {
		return nil, Localization.WithError(fmt.Errorf("units must contain between 1 and %d troop stacks", kingdomTroopMaximumStacks), Localization.New("server.app.units_must_contain_between.a617e2c8", "units must contain between 1 and {p0} troop stacks", Localization.Params{"p0": kingdomTroopMaximumStacks}))
	}
	catalog, err := gameData.Catalog("units")
	if err != nil {
		return nil, err
	}
	merged := map[State.UnitID]int64{}
	for _, unit := range requested {
		if unit.UnitID <= 0 || unit.Amount <= 0 {
			return nil, Localization.WithError(fmt.Errorf("every transferred troop requires a positive unitId and amount"), Localization.New("server.app.every_transferred_troop_requires.df1cc2e5", "every transferred troop requires a positive unitId and amount", nil))
		}
		raw, found := catalog.Find(strconv.FormatInt(int64(unit.UnitID), 10))
		if !found {
			return nil, Localization.WithError(fmt.Errorf("unit %d is not in the official unit catalog", unit.UnitID), Localization.New("server.app.unit_p_is_not.57d42843", "unit {p0} is not in the official unit catalog", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return nil, decodeErr
		}
		if GameData.IsToolRecord(record) {
			return nil, Localization.WithError(fmt.Errorf("unit %d is a tool and cannot use kingdom troop transport", unit.UnitID), Localization.New("server.app.unit_p_is_a.0896e640", "unit {p0} is a tool and cannot use kingdom troop transport", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		if merged[unit.UnitID] > int64(^uint64(0)>>1)-unit.Amount {
			return nil, Localization.WithError(fmt.Errorf("transfer amount for unit %d is too large", unit.UnitID), Localization.New("server.app.transfer_amount_for_unit.47e5c79e", "transfer amount for unit {p0} is too large", Localization.Params{"p0": fmt.Sprintf("%d", unit.UnitID)}))
		}
		merged[unit.UnitID] += unit.Amount
	}
	result := make([]kingdomTroopShipmentUnit, 0, len(merged))
	for unitID, amount := range merged {
		if source.Units.Stationed[unitID] < amount {
			return nil, Localization.WithError(fmt.Errorf("castle %d has %d stationed unit %d; %d requested", source.ID, source.Units.Stationed[unitID], unitID, amount), Localization.New("server.app.castle_p_has_p.7adfb050", "castle {p0} has {p1} stationed unit {p2}; {p3} requested", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": fmt.Sprintf("%d", source.Units.Stationed[unitID]), "p2": fmt.Sprintf("%d", unitID), "p3": amount}))
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("owned kingdom troop transport is not pending with the expected workflow"), Localization.New("server.app.owned_kingdom_troop_transport.e8668660", "owned kingdom troop transport is not pending with the expected workflow", nil))
		}
		if request.ExpectedRemaining <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("owned kingdom troop skip requires expectedRemaining"), Localization.New("server.app.owned_kingdom_troop_skip.8aa016d9", "owned kingdom troop skip requires expectedRemaining", nil))
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
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("owned kingdom troop donor is unavailable"), Localization.New("server.app.owned_kingdom_troop_donor.42207f8a", "owned kingdom troop donor is unavailable", nil))
		}
		steps = []Intent.Step{
			step,
			{Name: "Verify kingdom troop timer advanced", NameDescriptor: Localization.New("server.app.verify_kingdom_troop_timer.8f28dec8", "Verify kingdom troop timer advanced", nil), Action: "troops.kingdom.skip.verify_timer", ActionArguments: requestArguments},
			accountInventoryRefreshStep("Refresh official time-skip inventory").WithNameDescriptor(Localization.New("server.app.refresh_official_time_skip.dbd0cda5", "Refresh official time-skip inventory", nil)),
			{Name: "Confirm official time-skip consumption", NameDescriptor: Localization.New("server.app.confirm_official_time_skip.e9cf4332", "Confirm official time-skip consumption", nil), Action: "troops.kingdom.skip.verify_inventory", ActionArguments: requestArguments},
		}
	}
	return Intent.Plan{
		Claims: []string{
			"troop-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
			"currency:" + strconv.FormatInt(int64(currencyID), 10),
		},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d troop transport", timeSkipLabel, request.TargetKingdomID), SummaryDescriptor: Localization.New("server.app.apply_a_p_to.e2174b30", "Apply a {p0} to kingdom {p1} troop transport", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipLabel), "p1": fmt.Sprintf("%d", request.TargetKingdomID)}),
		Steps: steps,
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
		Summary: "Refresh authoritative account inventory", SummaryDescriptor: Localization.New("server.app.refresh_authoritative_account_inventory.09e11d47", "Refresh authoritative account inventory", nil),
		Steps: []Intent.Step{accountInventoryRefreshStep("Refresh authoritative account inventory").WithNameDescriptor(Localization.New("server.app.refresh_authoritative_account_inventory.09e11d47", "Refresh authoritative account inventory", nil))},
	}, nil
}

func kingdomTroopSkipStep(
	input Intent.PlanningContext,
	request kingdomTroopSkipRequest,
	requirePending bool,
) (Intent.Step, State.CurrencyID, string, error) {
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("targetKingdomId and timeSkipId are required"), Localization.New("server.app.targetkingdomid_and_timeskipid_are.d0c2a0d6", "targetKingdomId and timeSkipId are required", nil))
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
			return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("kingdom %d has no pending troop transport", request.TargetKingdomID), Localization.New("server.app.kingdom_p_has_no.787ee619", "kingdom {p0} has no pending troop transport", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetKingdomID)}))
		}
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Step{}, 0, "", err
	}
	timeSkipLabel := officialTimeSkipLabel(input.GameData, int64(currencyID), request.TimeSkipID)
	if request.MinimumRemaining < 0 {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("minimumRemaining cannot be negative"), Localization.New("server.app.minimumremaining_cannot_be_negative.1793e608", "minimumRemaining cannot be negative", nil))
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
		return Intent.Step{}, 0, "", Localization.WithError(fmt.Errorf("no %s is available", timeSkipLabel), Localization.New("server.app.no_p_is_available.d9c730b3", "no {p0} is available", Localization.Params{"p0": fmt.Sprintf("%s", timeSkipLabel)}))
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "1",
	})
	return commandStep("Skip kingdom troop transport time", "msk", payload, "msk", Localization.New("server.app.skip_kingdom_troop_transport.e84d0a91", "Skip kingdom troop transport time", nil)), currencyID, timeSkipLabel, nil
}

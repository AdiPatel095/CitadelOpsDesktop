package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const (
	beriKingdomID      State.KingdomID = 10
	beriCRAContextMode                 = "beri-tower"
)

type beriCapacityRefreshRequest struct {
	BeriCastleID   State.CastleID `json:"beriCastleId"`
	SourceCastleID State.CastleID `json:"sourceCastleId"`
	RequestedAt    time.Time      `json:"requestedAt,omitempty"`
}

type beriTransferRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId,omitempty"`
	TargetCastleID State.CastleID `json:"targetCastleId,omitempty"`
	// Accepted for queued legacy requests; KUT always uses CID -1.
	LegacyWireCastleID    int64        `json:"wireCastleId,omitempty"`
	ConfigurationRevision uint64       `json:"configurationRevision,omitempty"`
	DonorUnitsObserved    time.Time    `json:"donorUnitsObservedAt,omitempty"`
	CampUnitsObserved     time.Time    `json:"campUnitsObservedAt,omitempty"`
	UnitID                State.UnitID `json:"unitId"`
	Amount                int64        `json:"amount,omitempty"`
	UseTimeSkip           bool         `json:"useTimeSkip,omitempty"`
	TimeSkipID            string       `json:"timeSkipId,omitempty"`
}

type beriTransferGuardRequest struct {
	SourceCastleID        State.CastleID   `json:"sourceCastleId"`
	TargetCastleID        State.CastleID   `json:"targetCastleId"`
	UnitID                State.UnitID     `json:"unitId"`
	Amount                int64            `json:"amount"`
	CapacityObserved      time.Time        `json:"capacityObservedAt"`
	TimeSkipCurrency      State.CurrencyID `json:"timeSkipCurrencyId"`
	ConfigurationRevision uint64           `json:"configurationRevision,omitempty"`
	DonorUnitsObserved    time.Time        `json:"donorUnitsObservedAt,omitempty"`
	CampUnitsObserved     time.Time        `json:"campUnitsObservedAt,omitempty"`
}

type beriCampOpenRequest struct {
	CampID int64 `json:"campId"`
}

type beriCampOpenGuardRequest struct {
	CampID           int64     `json:"campId"`
	RefreshStartedAt time.Time `json:"refreshStartedAt"`
}

type beriTargetFindRequest struct {
	SourceCastleID State.CastleID `json:"sourceCastleId"`
}

type beriTargetFindGuardRequest struct {
	SourceCastleID  State.CastleID `json:"sourceCastleId"`
	SearchStartedAt time.Time      `json:"searchStartedAt"`
}

type beriTowerAttackRequest struct {
	SourceCastleID     State.CastleID       `json:"sourceCastleId"`
	TargetX            int                  `json:"targetX"`
	TargetY            int                  `json:"targetY"`
	TargetTypeID       int                  `json:"targetTypeId"`
	TargetObservedAt   time.Time            `json:"targetObservedAt"`
	CommanderID        State.CommanderID    `json:"commanderId"`
	Preset             AttackPresets.Preset `json:"preset"`
	HorseTravelBoostID int                  `json:"horseTravelBoostId"`
	DailyAttackLimit   int64                `json:"dailyAttackLimit"`
	TargetRefreshAfter time.Time            `json:"targetRefreshedAfter,omitempty"`
}

func planBeriCapacityRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request beriCapacityRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.BeriCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("beriCastleId must identify the active Berimond castle"), Localization.New("server.app.bericastleid_must_identify_the.0ba1c5be", "beriCastleId must identify the active Berimond castle", nil))
	}
	castle, exists := input.State.Castles[request.BeriCastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: beriCastleId no longer identifies the active Berimond castle", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.b176a682", "intent plan became stale before dispatch: beriCastleId no longer identifies the active Berimond castle", nil))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || request.SourceCastleID <= 0 || source.KingdomID != 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: sourceCastleId no longer identifies an owned Great Empire donor", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.43ed39e9", "intent plan became stale before dispatch: sourceCastleId no longer identifies an owned Great Empire donor", nil))
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4e00f6a", "intent plan became stale before dispatch: the Battle for Berimond is no longer unlocked", nil))
	}
	payload, _ := json.Marshal(struct {
		CastleID State.CastleID `json:"CID"`
	}{request.BeriCastleID})
	request.RequestedAt = time.Now().UTC()
	verifyArguments, _ := json.Marshal(request)
	castleID := strconv.FormatInt(int64(request.BeriCastleID), 10)
	sourceID := strconv.FormatInt(int64(request.SourceCastleID), 10)
	return Intent.Plan{
		Claims: []string{
			"castle:" + sourceID, "castle:" + castleID,
			"kingdom:" + strconv.FormatInt(int64(beriKingdomID), 10),
			"beri-capacity:" + castleID,
		},
		Summary: fmt.Sprintf(
			"Refresh Berimond troop capacity for castle %d and donor inventory at castle %d",
			request.BeriCastleID, request.SourceCastleID,
		), SummaryDescriptor: Localization.New("server.app.refresh_berimond_troop_capacity.988f413d", "Refresh Berimond troop capacity for castle {p0} and donor inventory at castle {p1}", Localization.Params{"p0": fmt.Sprintf("%d", request.BeriCastleID), "p1": fmt.Sprintf("%d", request.SourceCastleID)}),
		Steps: []Intent.Step{
			commandStep("Refresh owned-castle troop inventories", "dcl", json.RawMessage(`{"CD":1}`), "dcl", Localization.New("server.app.refresh_owned_castle_troop.fddb7cdb", "Refresh owned-castle troop inventories", nil)),
			commandStep("Refresh Berimond troop capacity", "fuc", payload, "fuc", Localization.New("server.app.refresh_berimond_troop_capacity.94ebee2a", "Refresh Berimond troop capacity", nil)),
			{Name: "Verify refreshed Berimond troop capacity", NameDescriptor: Localization.New("server.app.verify_refreshed_berimond_troop.4ecdd407", "Verify refreshed Berimond troop capacity", nil), Action: "beri.capacity.verify", ActionArguments: verifyArguments},
		},
	}, nil
}

func (application *Application) verifyBeriCapacity(_ context.Context, arguments json.RawMessage) error {
	var request beriCapacityRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.BeriCastleID <= 0 || request.SourceCastleID <= 0 || request.RequestedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Berimond capacity verification requires target and donor castles plus a request time"), Localization.New("server.app.berimond_capacity_verification_requires.1e385ff1", "Berimond capacity verification requires target and donor castles plus a request time", nil))
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("Berimond capacity state is unavailable"), Localization.New("server.app.berimond_capacity_state_is.af5e2f24", "Berimond capacity state is unavailable", nil))
	}
	state := application.State.ReadOnlyView()
	castle, exists := state.Castles[request.BeriCastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return Localization.WithError(fmt.Errorf("%w: the Berimond capacity castle is no longer owned", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.6f8314cf", "intent plan became stale before dispatch: the Berimond capacity castle is no longer owned", nil))
	}
	source, exists := state.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 {
		return Localization.WithError(fmt.Errorf("%w: the selected Berimond donor is no longer an owned Great Empire castle", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.0ce0562f", "intent plan became stale before dispatch: the selected Berimond donor is no longer an owned Great Empire castle", nil))
	}
	if source.UnitsObservedAt.IsZero() || source.UnitsObservedAt.Before(request.RequestedAt) {
		return Localization.WithError(fmt.Errorf("%w: the dcl response did not refresh the selected Berimond donor inventory", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.59b9375e", "intent plan became stale before dispatch: the dcl response did not refresh the selected Berimond donor inventory", nil))
	}
	if state.Beri.ObservedAt.IsZero() || state.Beri.ObservedAt.Before(request.RequestedAt) {
		return Localization.WithError(fmt.Errorf("%w: the fuc response did not refresh Berimond troop capacity", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.41e826ea", "intent plan became stale before dispatch: the fuc response did not refresh Berimond troop capacity", nil))
	}
	if state.Beri.ParsedSourceID > 0 && state.Beri.ParsedSourceID != request.SourceCastleID {
		return Localization.WithError(fmt.Errorf(
			"%w: Berimond capacity was reported for donor %d instead of selected donor %d",
			Intent.ErrPlanStale, state.Beri.ParsedSourceID, request.SourceCastleID,
		), Localization.New("server.app.intent_plan_became_stale.64ea0821", "intent plan became stale before dispatch: Berimond capacity was reported for donor {p1} instead of selected donor {p2}", Localization.Params{"p1": fmt.Sprintf("%d", state.Beri.ParsedSourceID), "p2": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	return nil
}

func planBeriTransfer(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request beriTransferRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, err := sourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	target, exists := input.State.Castles[request.TargetCastleID]
	if request.TargetCastleID <= 0 {
		target, exists = ownedCastleInKingdom(input.State, beriKingdomID)
	}
	if !exists {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: an owned Berimond camp is required before transferring troops", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.94990f0b", "intent plan became stale before dispatch: an owned Berimond camp is required before transferring troops", nil))
	}
	if target.KingdomID != beriKingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: targetCastleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.5a4b2efd", "intent plan became stale before dispatch: targetCastleId no longer identifies an owned Berimond camp", nil))
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("unitId must identify the official troop transferred to Berimond"), Localization.New("server.app.unitid_must_identify_the.1430a935", "unitId must identify the official troop transferred to Berimond", nil))
	}
	if err := validateBeriTransferFoodUnit(input.GameData, request.UnitID); err != nil {
		return Intent.Plan{}, err
	}
	if input.State.Beri.ObservedAt.IsZero() || !input.State.Beri.ConsumedAt.Before(input.State.Beri.ObservedAt) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: Berimond troop capacity has not been refreshed since the last transfer", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.0cde4339", "intent plan became stale before dispatch: Berimond troop capacity has not been refreshed since the last transfer", nil))
	}
	available := input.State.Beri.AvailableTroops
	if exact, exists := input.State.Beri.TroopsByUnit[request.UnitID]; exists {
		available = exact
	}
	if request.Amount <= 0 {
		request.Amount = available
	}
	if request.Amount <= 0 || request.Amount > available {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: amount must be between 1 and the refreshed Berimond capacity %d", Intent.ErrPlanStale, available,
		), Localization.New("server.app.intent_plan_became_stale.f5fdb055", "intent plan became stale before dispatch: amount must be between 1 and the refreshed Berimond capacity {p1, number}", Localization.Params{"p1": available}))
	}
	var skipSteps []Intent.Step
	var currencyID State.CurrencyID
	if request.UseTimeSkip {
		step, selectedCurrencyID, _, err := kingdomTroopSkipStep(input, kingdomTroopSkipRequest{
			TargetKingdomID: beriKingdomID, TimeSkipID: request.TimeSkipID,
		}, false)
		if err != nil {
			return Intent.Plan{}, err
		}
		step.Name = "Immediately apply the selected Berimond troop transport skip"
		currencyID = selectedCurrencyID
		skipSteps = []Intent.Step{step, timeSkipConsumeStep(input, currencyID)}
	}
	guard := beriTransferGuardRequest{
		SourceCastleID: source.ID, TargetCastleID: target.ID, UnitID: request.UnitID, Amount: request.Amount,
		CapacityObserved: input.State.Beri.ObservedAt, TimeSkipCurrency: currencyID,
		ConfigurationRevision: request.ConfigurationRevision,
		DonorUnitsObserved:    request.DonorUnitsObserved, CampUnitsObserved: request.CampUnitsObserved,
	}
	if err := validateBeriTransferState(input, guard); err != nil {
		return Intent.Plan{}, err
	}
	shipment := kingdomTroopShipmentRequest{
		SourceCastleID: source.ID, TargetCastleID: target.ID, TargetKingdomID: beriKingdomID,
		Units: []kingdomTroopShipmentUnit{{UnitID: request.UnitID, Amount: request.Amount}},
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		WireCastleID   int64           `json:"CID"`
		Troops         [][]int64       `json:"A"`
	}{source.ID, source.KingdomID, beriKingdomID, -1, [][]int64{{int64(request.UnitID), request.Amount}}})
	guardArguments, _ := json.Marshal(guard)
	sourceConsumeArguments, _ := json.Marshal(shipment)
	capacityConsumeArguments, _ := json.Marshal(struct {
		ObservedAt time.Time `json:"observedAt"`
	}{input.State.Beri.ObservedAt})
	steps := make([]Intent.Step, 0, 7)
	steps = append(steps,
		kingdomTransportContextStep(),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Verify refreshed Berimond troop transfer", NameDescriptor: Localization.New("server.app.verify_refreshed_berimond_troop.8ec71816", "Verify refreshed Berimond troop transfer", nil), Action: "beri.transfer.verify",
			ActionArguments: guardArguments,
		}),
		commandStep("Transfer troops to Berimond", "kut", payload, "kut", Localization.New("server.app.transfer_troops_to_berimond.142f0aa5", "Transfer troops to Berimond", nil)),
		Intent.Step{
			Name: "Consume confirmed Berimond donor troops", NameDescriptor: Localization.New("server.app.consume_confirmed_berimond_donor.b4eb94b2", "Consume confirmed Berimond donor troops", nil), Action: "troops.kingdom.consume_source",
			ActionArguments: sourceConsumeArguments,
		},
		Intent.Step{Name: "Consume refreshed Berimond capacity", NameDescriptor: Localization.New("server.app.consume_refreshed_berimond_capacity.a749c00b", "Consume refreshed Berimond capacity", nil), Action: "beri.consume_capacity", ActionArguments: capacityConsumeArguments},
	)
	if len(skipSteps) > 0 {
		steps = append(steps, skipSteps...)
	}
	sourceCastleID := strconv.FormatInt(int64(source.ID), 10)
	targetCastleID := strconv.FormatInt(int64(target.ID), 10)
	claims := []string{
		"troop-transport", "castle:" + sourceCastleID, "castle:" + targetCastleID,
		"kingdom:" + strconv.FormatInt(int64(beriKingdomID), 10),
		"beri-capacity:" + targetCastleID,
		"unit:" + strconv.FormatInt(int64(request.UnitID), 10),
	}
	if currencyID > 0 {
		claims = append(claims, "currency:"+strconv.FormatInt(int64(currencyID), 10))
	}
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Transfer %d of unit %d from %s to Berimond", request.Amount, request.UnitID, castleLabel(source)), SummaryDescriptor: Localization.New("server.app.transfer_p_of_unit.6045bb0e", "Transfer {p0} of unit {p1} from {p2} to Berimond", Localization.Params{"p0": request.Amount, "p1": fmt.Sprintf("%d", request.UnitID), "p2": fmt.Sprintf("%s", castleLabel(source))}),
		Steps: steps,
	}, nil
}

func validateBeriTransferFoodUnit(gameData *GameData.Store, unitID State.UnitID) error {
	usesFood, err := gameData.UnitUsesFoodSupply(unitID)
	if err != nil {
		return Localization.WithError(fmt.Errorf("validate Berimond transfer unit %d: %w", unitID, err), Localization.ErrorContext(Localization.New("server.app.validate_berimond_transfer_unit.dfb922e9", "validate Berimond transfer unit {p0}", Localization.Params{"p0": fmt.Sprintf("%d", unitID)}), err))
	}
	if !usesFood {
		return Localization.WithError(fmt.Errorf("Berimond troop transfers require a Food-consuming unit; unit %d consumes Mead or Beef", unitID), Localization.New("server.app.berimond_troop_transfers_require.54c4d843", "Berimond troop transfers require a Food-consuming unit; unit {p0} consumes Mead or Beef", Localization.Params{"p0": fmt.Sprintf("%d", unitID)}))
	}
	return nil
}

func validateBeriTransferState(input Intent.PlanningContext, request beriTransferGuardRequest) error {
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || source.ID <= 0 {
		return Localization.WithError(fmt.Errorf("Berimond troop donor %d is no longer owned", request.SourceCastleID), Localization.New("server.app.berimond_troop_donor_p.f04d8d02", "Berimond troop donor {p0} is no longer owned", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if source.KingdomID != 0 {
		return Localization.WithError(fmt.Errorf("Berimond troop donor %d must be a Great Empire castle", request.SourceCastleID), Localization.New("server.app.berimond_troop_donor_p.5a0298d4", "Berimond troop donor {p0} must be a Great Empire castle", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	if !targetExists || target.ID <= 0 || target.KingdomID != beriKingdomID {
		return Localization.WithError(fmt.Errorf("Berimond troop destination %d is no longer owned", request.TargetCastleID), Localization.New("server.app.berimond_troop_destination_p.dd278836", "Berimond troop destination {p0} is no longer owned", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID)}))
	}
	if (!request.DonorUnitsObserved.IsZero() && !source.UnitsObservedAt.Equal(request.DonorUnitsObserved)) ||
		(!request.CampUnitsObserved.IsZero() && !target.UnitsObservedAt.Equal(request.CampUnitsObserved)) {
		return fmt.Errorf("Berimond donor or camp inventory changed before transfer")
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Localization.WithError(fmt.Errorf("the Battle for Berimond is no longer unlocked"), Localization.New("server.app.the_battle_for_berimond.e2100964", "the Battle for Berimond is no longer unlocked", nil))
	}
	beri := input.State.Beri
	if request.CapacityObserved.IsZero() || !beri.ObservedAt.Equal(request.CapacityObserved) ||
		!beri.ConsumedAt.Before(beri.ObservedAt) {
		return Localization.WithError(fmt.Errorf("Berimond troop capacity changed or was already consumed"), Localization.New("server.app.berimond_troop_capacity_changed.1a0ebb95", "Berimond troop capacity changed or was already consumed", nil))
	}
	available := beri.AvailableTroops
	if exact, exists := beri.TroopsByUnit[request.UnitID]; exists {
		available = exact
	}
	if request.UnitID <= 0 || request.Amount <= 0 || request.Amount > available {
		return Localization.WithError(fmt.Errorf("Berimond capacity for unit %d is %d; %d requested", request.UnitID, available, request.Amount), Localization.New("server.app.berimond_capacity_for_unit.9b392bfc", "Berimond capacity for unit {p0} is {p1}; {p2} requested", Localization.Params{"p0": fmt.Sprintf("%d", request.UnitID), "p1": available, "p2": request.Amount}))
	}
	if err := validateBeriTransferFoodUnit(input.GameData, request.UnitID); err != nil {
		return err
	}
	if _, err := normalizeKingdomTroopShipment(input.GameData, source, []kingdomTroopShipmentUnit{{
		UnitID: request.UnitID, Amount: request.Amount,
	}}); err != nil {
		return err
	}
	if kingdomTroopTransportPending(input.State, beriKingdomID) {
		return Localization.WithError(fmt.Errorf("Berimond already has a pending or settling troop transport"), Localization.New("server.app.berimond_already_has_a.64936ed3", "Berimond already has a pending or settling troop transport", nil))
	}
	if request.TimeSkipCurrency > 0 && input.State.Player.Currencies[request.TimeSkipCurrency] < 1 {
		return Localization.WithError(fmt.Errorf("the selected Berimond transfer time skip is no longer available"), Localization.New("server.app.the_selected_berimond_transfer.4da8ce16", "the selected Berimond transfer time skip is no longer available", nil))
	}
	return nil
}

func (application *Application) verifyBeriTransfer(_ context.Context, arguments json.RawMessage) error {
	var request beriTransferGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Berimond transfer state is unavailable"), Localization.New("server.app.berimond_transfer_state_is.565635c0", "Berimond transfer state is unavailable", nil))
	}
	if request.ConfigurationRevision > 0 && (application.Configuration == nil ||
		application.Configuration.Revision() != request.ConfigurationRevision) {
		return fmt.Errorf("%w: Berimond settings or attack preset changed before transfer", Intent.ErrPlanStale)
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if err := validateBeriTransferState(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, request); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func (application *Application) consumeBeriCapacity(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		ObservedAt time.Time `json:"observedAt"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentBeri), func(gameState *State.GameState) ([]string, bool, error) {
		if request.ObservedAt.IsZero() || !gameState.Beri.ObservedAt.Equal(request.ObservedAt) {
			return nil, false, Localization.WithError(fmt.Errorf("Berimond capacity changed before it could be consumed"), Localization.New("server.app.berimond_capacity_changed_before.c9ae8e93", "Berimond capacity changed before it could be consumed", nil))
		}
		gameState.Beri.AvailableTroops = 0
		gameState.Beri.TroopsByUnit = map[State.UnitID]int64{}
		gameState.Beri.ConsumedAt = time.Now().UTC()
		return []string{"beri", "units"}, true, nil
	})
	return err
}

func planBeriCampOpen(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request beriCampOpenRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	option, err := beriCampOpenOption(input, request.CampID, time.Time{})
	if err != nil {
		return Intent.Plan{}, err
	}
	refreshStartedAt := time.Now().UTC()
	payload, _ := json.Marshal(struct {
		ID        int64           `json:"ID"`
		Premium   int             `json:"PWR"`
		Secondary int             `json:"OC2"`
		KingdomID State.KingdomID `json:"SID"`
	}{ID: option.ID, Premium: 0, Secondary: 0, KingdomID: beriKingdomID})
	mark, _ := json.Marshal(struct {
		RequestedAt time.Time `json:"requestedAt"`
	}{RequestedAt: refreshStartedAt})
	guardArguments, _ := json.Marshal(beriCampOpenGuardRequest{
		CampID: request.CampID, RefreshStartedAt: refreshStartedAt,
	})
	return Intent.Plan{
		Claims: []string{
			"account-resources", "troop-transport",
			"kingdom:" + strconv.FormatInt(int64(beriKingdomID), 10),
		},
		Summary: fmt.Sprintf(
			"Open non-premium Berimond camp %d for %d wood and %d stone",
			option.ID, option.CostWood, option.CostStone,
		), SummaryDescriptor: Localization.New("server.app.open_non_premium_berimond.e3ffa3e2", "Open non-premium Berimond camp {p0} for {p1} wood and {p2} stone", Localization.Params{"p0": fmt.Sprintf("%d", option.ID), "p1": option.CostWood, "p2": option.CostStone}),
		Steps: []Intent.Step{
			kingdomTransportContextStep(),
			Intent.RebuildOnResume(Intent.Step{
				Name: "Verify refreshed Berimond camp availability", NameDescriptor: Localization.New("server.app.verify_refreshed_berimond_camp.b0ffcf36", "Verify refreshed Berimond camp availability", nil), Action: "beri.camp.open.verify",
				ActionArguments: guardArguments,
			}),
			commandStep("Open non-premium Berimond camp", "fsc", payload, "fsc", Localization.New("server.app.open_non_premium_berimond.c60d35de", "Open non-premium Berimond camp", nil)),
			{Name: "Record Berimond camp-open request", NameDescriptor: Localization.New("server.app.record_berimond_camp_open.f1f1f963", "Record Berimond camp-open request", nil), Action: "beri.camp.opened", ActionArguments: mark},
			contextCommandStep("Refresh Berimond kingdom state", "kpi", json.RawMessage(`{}`), "kpi").WithNameDescriptor(Localization.New("server.app.refresh_berimond_kingdom_state.2eeb2ffd", "Refresh Berimond kingdom state", nil)),
		},
	}, nil
}

func beriCampOpenOption(
	input Intent.PlanningContext,
	campID int64,
	refreshedAfter time.Time,
) (GameData.BerimondCampOption, error) {
	if input.GameData == nil {
		return GameData.BerimondCampOption{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	option, found := input.GameData.CheapestNonPremiumBerimondCamp(input.State.Player.Level)
	if !found || campID != option.ID {
		return GameData.BerimondCampOption{},
			Localization.WithError(fmt.Errorf("campId must identify the cheapest unlocked non-premium Berimond camp"), Localization.New("server.app.campid_must_identify_the.5ba00f2f", "campId must identify the cheapest unlocked non-premium Berimond camp", nil))
	}
	if _, exists := ownedCastleInKingdom(input.State, beriKingdomID); exists {
		return GameData.BerimondCampOption{},
			Localization.WithError(fmt.Errorf("%w: an owned Berimond camp already exists", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.85ad226c", "intent plan became stale before dispatch: an owned Berimond camp already exists", nil))
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked || unlock.Created {
		return GameData.BerimondCampOption{},
			Localization.WithError(fmt.Errorf("%w: Berimond must be freshly observed as unlocked without an existing camp", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.48609051", "intent plan became stale before dispatch: Berimond must be freshly observed as unlocked without an existing camp", nil))
	}
	if !refreshedAfter.IsZero() && input.State.KingdomTransport.ObservedAt.Before(refreshedAfter) {
		return GameData.BerimondCampOption{},
			Localization.WithError(fmt.Errorf("the Berimond kingdom list was not refreshed before opening the camp"), Localization.New("server.app.the_berimond_kingdom_list.9dc2786c", "the Berimond kingdom list was not refreshed before opening the camp", nil))
	}
	if !input.State.Beri.CampOpenRequestedAt.IsZero() &&
		time.Since(input.State.Beri.CampOpenRequestedAt) < 5*time.Minute {
		return GameData.BerimondCampOption{},
			Localization.WithError(fmt.Errorf("%w: a Berimond camp-open request is already settling", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.dfd09007", "intent plan became stale before dispatch: a Berimond camp-open request is already settling", nil))
	}
	return option, nil
}

func (application *Application) verifyBeriCampOpen(_ context.Context, arguments json.RawMessage) error {
	var request beriCampOpenGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.CampID <= 0 || request.RefreshStartedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Berimond camp verification requires a camp and refresh time"), Localization.New("server.app.berimond_camp_verification_requires.2b2745db", "Berimond camp verification requires a camp and refresh time", nil))
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return Localization.WithError(fmt.Errorf("Berimond camp state is unavailable"), Localization.New("server.app.berimond_camp_state_is.be86f1e6", "Berimond camp state is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if _, err := beriCampOpenOption(Intent.PlanningContext{
		State: application.State.ReadOnlyView(), GameData: gameData,
	}, request.CampID, request.RefreshStartedAt); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func planBeriTargetFind(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request beriTargetFindRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if request.SourceCastleID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("sourceCastleId must identify an owned Berimond camp"), Localization.New("server.app.sourcecastleid_must_identify_an.e7dd16f8", "sourceCastleId must identify an owned Berimond camp", nil))
	}
	if !exists || source.KingdomID != beriKingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf(
			"%w: sourceCastleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		), Localization.New("server.app.intent_plan_became_stale.3a9eaba3", "intent plan became stale before dispatch: sourceCastleId no longer identifies an owned Berimond camp", nil))
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4e00f6a", "intent plan became stale before dispatch: the Battle for Berimond is no longer unlocked", nil))
	}
	searchStartedAt := time.Now().UTC()
	guardArguments, _ := json.Marshal(beriTargetFindGuardRequest{
		SourceCastleID: source.ID, SearchStartedAt: searchStartedAt,
	})
	steps := make([]Intent.Step, 0, 5)
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps,
		closeGameUIStep(),
		contextCommandStep("Refresh Berimond world-map context", "gbl", json.RawMessage(`{}`), "gbl").WithNameDescriptor(Localization.New("server.app.refresh_berimond_world_map.6896dc80", "Refresh Berimond world-map context", nil)),
		beriFindNextTowerStep(),
		Intent.Step{
			Name: "Verify selected Berimond tower", NameDescriptor: Localization.New("server.app.verify_selected_berimond_tower.aa5e88f9", "Verify selected Berimond tower", nil), Action: "beri.target.verify",
			ActionArguments: guardArguments,
		},
	)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "attack-context", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"map:" + strconv.FormatInt(int64(beriKingdomID), 10),
			"beri-target:" + strconv.FormatInt(int64(beriKingdomID), 10),
		},
		Summary: "Find the next available Berimond tower", SummaryDescriptor: Localization.New("server.app.find_the_next_available.563cbd7b", "Find the next available Berimond tower", nil),
		Steps: steps,
	}, nil
}

func beriFindNextTowerStep() Intent.Step {
	step := contextCommandStep(
		"Find next available Berimond tower", "fnt", json.RawMessage(`{}`), "fnt",
	).WithNameDescriptor(Localization.New("server.app.find_next_available_berimond.09c7c9c7", "Find next available Berimond tower", nil))
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func currentBeriTarget(gameState State.GameState, observedAfter time.Time) (State.MapObservation, error) {
	beri := gameState.Beri
	if beri.TargetObservedAt.IsZero() || !beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) {
		return State.MapObservation{}, Localization.WithError(fmt.Errorf("Berimond did not select a valid tower"), Localization.New("server.app.berimond_did_not_select.a7876f2e", "Berimond did not select a valid tower", nil))
	}
	if !observedAfter.IsZero() && beri.TargetObservedAt.Before(observedAfter) {
		return State.MapObservation{}, Localization.WithError(fmt.Errorf("Berimond did not return a fresh tower selection"), Localization.New("server.app.berimond_did_not_return.30a79f9f", "Berimond did not return a fresh tower selection", nil))
	}
	target, exists := gameState.LookupMapObservation(beriKingdomID, fmt.Sprintf("%d:%d", beri.TargetX, beri.TargetY))
	if !exists || beri.TargetTypeID != AttackCapacity.BerimondTowerMapTypeID ||
		target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.Level <= 0 ||
		target.ObservedAt.Before(beri.TargetObservedAt) {
		return State.MapObservation{}, Localization.WithError(fmt.Errorf("the selected Berimond tower is missing from the refreshed map"), Localization.New("server.app.the_selected_berimond_tower.e27ba32f", "the selected Berimond tower is missing from the refreshed map", nil))
	}
	return target, nil
}

func (application *Application) verifyBeriTargetFound(_ context.Context, arguments json.RawMessage) error {
	var request beriTargetFindGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || request.SearchStartedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("Berimond target verification requires a source castle and search time"), Localization.New("server.app.berimond_target_verification_requires.623924f1", "Berimond target verification requires a source castle and search time", nil))
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("Berimond target state is unavailable"), Localization.New("server.app.berimond_target_state_is.2d54a271", "Berimond target state is unavailable", nil))
	}
	state := application.State.ReadOnlyView()
	source, exists := state.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != beriKingdomID {
		return Localization.WithError(fmt.Errorf("%w: the Berimond attack source is no longer owned", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.a2e496c7", "intent plan became stale before dispatch: the Berimond attack source is no longer owned", nil))
	}
	if !source.Focused {
		return Localization.WithError(fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.5011c69d", "intent plan became stale before dispatch: the Berimond attack source is no longer focused", nil))
	}
	if unlock, observed := state.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Localization.WithError(fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4e00f6a", "intent plan became stale before dispatch: the Battle for Berimond is no longer unlocked", nil))
	}
	if _, err := currentBeriTarget(state, request.SearchStartedAt); err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func planBeriTowerAttack(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	request, source, target, err := beriTowerAttackContext(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Plan{}, err
	}
	if blockedPlan, blocked, err := dailyAttackLimitPlan(input.State, request.DailyAttackLimit); err != nil {
		return Intent.Plan{}, err
	} else if blocked {
		return blockedPlan, nil
	}
	request.TargetRefreshAfter = time.Now().UTC()
	resolvedArguments, _ := json.Marshal(request)
	steps := make([]Intent.Step, 0, 6)
	if input.State.Player.LegendSkills.ObservedAt.IsZero() ||
		time.Since(input.State.Player.LegendSkills.ObservedAt) >= 5*time.Minute {
		steps = append(steps, contextCommandStep(
			"Refresh Hall of Legends attack limits", "skl", json.RawMessage(`{}`), "skl",
		).WithNameDescriptor(Localization.New("server.app.refresh_hall_of_legends.2b74581a", "Refresh Hall of Legends attack limits", nil)))
	}
	steps = append(steps, generalSkillsContextSteps(input.State, request.CommanderID, time.Now().UTC())...)
	steps = append(steps, attackCastleContextStep(source))
	steps = appendDailyAttackLimitGuard(steps, request.DailyAttackLimit)
	targetRefreshPayload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{beriKingdomID, target.X, target.Y, target.X, target.Y})
	targetRefreshStep := contextCommandStep(
		"Refresh selected Berimond tower", "gaa", targetRefreshPayload, "gaa",
	).WithNameDescriptor(Localization.New("server.app.refresh_selected_berimond_tower.a4dafb20", "Refresh selected Berimond tower", nil))
	targetRefreshStep.ResponseBarrier = Intent.ResponseBarrierCommitted
	craDependencyPayload, _ := json.Marshal(struct {
		SourceX     int             `json:"SX"`
		SourceY     int             `json:"SY"`
		TargetX     int             `json:"TX"`
		TargetY     int             `json:"TY"`
		KingdomID   State.KingdomID `json:"KID"`
		ContextMode string          `json:"_citadelContextMode"`
	}{source.X, source.Y, target.X, target.Y, beriKingdomID, beriCRAContextMode})
	steps = append(steps,
		targetRefreshStep,
		Intent.Step{
			Name: "Guard Berimond tower attack", NameDescriptor: Localization.New("server.app.guard_berimond_tower_attack.02b038be", "Guard Berimond tower attack", nil), Action: "beri.tower.attack.guard", ActionArguments: resolvedArguments,
		},
		Intent.Step{
			Name: "Build and launch Berimond tower attack", NameDescriptor: Localization.New("server.app.build_and_launch_berimond.9fc16996", "Build and launch Berimond tower attack", nil), Resolver: "beri.tower.attack.build",
			ResolverArguments: resolvedArguments, AwaitOpcode: "cra", TimeoutMillis: 10_000, SuccessCodes: []int{0},
			CommandDependencies: &Intent.CommandDependencyRequest{
				Opcode: "cra", Payload: craDependencyPayload,
			},
		},
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoBeriWorld, SourceCastleID: source.ID, CommanderID: request.CommanderID,
			KingdomID: beriKingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
		attackCastleRefreshStep("Refresh Berimond source inventory after attack", source).WithNameDescriptor(Localization.New("server.app.refresh_berimond_source_inventory.71930779", "Refresh Berimond source inventory after attack", nil)),
	)
	castleID := strconv.FormatInt(int64(source.ID), 10)
	claims := []string{
		"castle-focus", "attack-context", "castle:" + castleID, "attack-inventory:" + castleID,
		"map:" + strconv.FormatInt(int64(beriKingdomID), 10),
		fmt.Sprintf("beri-target:%d:%d:%d", beriKingdomID, target.X, target.Y),
	}
	claims = append(claims, craCommanderClaims([]State.CommanderID{request.CommanderID})...)
	return Intent.Plan{
		Claims: claims,
		Admission: &Intent.Admission{
			Class: Intent.AdmissionAttackLaunch, Module: "autoBeriWorld", Affinity: "castle:" + castleID,
		},
		Summary: fmt.Sprintf("Attack Berimond tower at %d:%d with %s", target.X, target.Y, request.Preset.Name), SummaryDescriptor: Localization.New("server.app.attack_berimond_tower_at.6496001d", "Attack Berimond tower at {p0}:{p1} with {p2}", Localization.Params{"p0": target.X, "p1": target.Y, "p2": fmt.Sprintf("%s", request.Preset.Name)}),
		Steps: steps,
	}, nil
}

func (application *Application) resolveBeriTowerAttackStep(
	_ context.Context,
	input Intent.PlanningContext,
	arguments json.RawMessage,
) (Intent.Step, error) {
	request, source, target, err := beriTowerAttackContext(input, arguments, time.Now().UTC())
	if err != nil {
		return Intent.Step{}, err
	}
	if !source.Focused {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.5011c69d", "intent plan became stale before dispatch: the Berimond attack source is no longer focused", nil))
	}
	capacity, err := resolveBeriTowerAttackCapacity(input, request, source, target)
	if err != nil {
		return Intent.Step{}, err
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	built, err := buildAttackSetup(invasionAttackSetup(limitedPreset), source, input.GameData)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Berimond preset %q: %w", request.Preset.Name, err), Localization.ErrorContext(Localization.New("server.app.build_berimond_preset_p.4e959192", "build Berimond preset {p0}", Localization.Params{"p0": fmt.Sprintf("%q", request.Preset.Name)}), err))
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("resolve Berimond horse travel boost: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_berimond_horse_travel.eb551fba", "resolve Berimond horse travel boost", nil), err))
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("build Berimond CRA payload: %w", err), Localization.ErrorContext(Localization.New("server.app.build_berimond_cra_payload.ef750d5a", "build Berimond CRA payload", nil), err))
	}
	return commandStep(fmt.Sprintf("Attack Berimond tower at %d:%d", target.X, target.Y), "cra", body, "cra", Localization.New("server.app.attack_berimond_tower_at.a5138f3f", "Attack Berimond tower at {p0}:{p1}", Localization.Params{"p0": target.X, "p1": target.Y})), nil
}

func resolveBeriTowerAttackCapacity(
	input Intent.PlanningContext,
	request beriTowerAttackRequest,
	source State.CastleState,
	target State.MapObservation,
) (AttackCapacity.Result, error) {
	capacity, err := (AttackCapacity.Resolver{}).Resolve(input.State, input.GameData, AttackCapacity.Request{
		SourceCastleID: source.ID, CommanderID: request.CommanderID,
		UseAttackDialogEffects: false,
		Target: AttackCapacity.TargetContext{
			ID:         fmt.Sprintf("berimond-tower:%d:%d:%d", target.KingdomID, target.X, target.Y),
			TargetType: AttackCapacity.TargetTypeBerimondTower,
			Map: &AttackCapacity.MapTarget{
				KingdomID: target.KingdomID, TypeID: target.TypeID, X: target.X, Y: target.Y,
				ObjectID: target.ObjectID, Level: target.Level,
			},
			Level: target.Level, CastleTypeID: target.TypeID, PvP: false, LegendaryFight: false,
		},
	})
	if err != nil {
		return AttackCapacity.Result{}, Localization.WithError(fmt.Errorf("resolve Berimond tower attack capacity: %w", err), Localization.ErrorContext(Localization.New("server.app.resolve_berimond_tower_attack.f86b2b66", "resolve Berimond tower attack capacity", nil), err))
	}
	return capacity, nil
}

func beriTowerAttackContext(
	input Intent.PlanningContext,
	arguments json.RawMessage,
	now time.Time,
) (beriTowerAttackRequest, State.CastleState, State.MapObservation, error) {
	var request beriTowerAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, err
	}
	if input.GameData == nil {
		return request, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, err
	}
	if err := AttackPresets.Validate(request.Preset); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("invalid Berimond attack preset: %w", err), Localization.ErrorContext(Localization.New("server.app.invalid_berimond_attack_preset.606a4eb8", "invalid Berimond attack preset", nil), err))
	}
	if request.TargetTypeID != AttackCapacity.BerimondTowerMapTypeID {
		return request, State.CastleState{}, State.MapObservation{},
			Localization.WithError(fmt.Errorf("Berimond attacks require tower target type %d", AttackCapacity.BerimondTowerMapTypeID), Localization.New("server.app.berimond_attacks_require_tower.91574b9d", "Berimond attacks require tower target type {p0}", Localization.Params{"p0": fmt.Sprintf("%d", AttackCapacity.BerimondTowerMapTypeID)}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != beriKingdomID {
		return request, State.CastleState{}, State.MapObservation{},
			Localization.WithError(fmt.Errorf("%w: Berimond attack source is unavailable", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.691521df", "intent plan became stale before dispatch: Berimond attack source is unavailable", nil))
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return request, State.CastleState{}, State.MapObservation{}, Localization.WithError(fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.c4e00f6a", "intent plan became stale before dispatch: the Battle for Berimond is no longer unlocked", nil))
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(input.State, request.CommanderID, now) {
		return request, State.CastleState{}, State.MapObservation{},
			Localization.WithError(fmt.Errorf("%w: Berimond commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID), Localization.New("server.app.intent_plan_became_stale.726864f6", "intent plan became stale before dispatch: Berimond commander {p1} is no longer available", Localization.Params{"p1": fmt.Sprintf("%d", request.CommanderID)}))
	}
	beri := input.State.Beri
	if request.TargetObservedAt.IsZero() || !beri.TargetObservedAt.Equal(request.TargetObservedAt) ||
		!beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) ||
		beri.TargetX != request.TargetX || beri.TargetY != request.TargetY || beri.TargetTypeID != request.TargetTypeID {
		return request, State.CastleState{}, State.MapObservation{},
			Localization.WithError(fmt.Errorf("%w: Berimond tower selection is no longer current", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.5cc8691e", "intent plan became stale before dispatch: Berimond tower selection is no longer current", nil))
	}
	target, exists := input.State.LookupMapObservation(beriKingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.KingdomID != beriKingdomID || target.X != request.TargetX || target.Y != request.TargetY ||
		target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.TypeID != request.TargetTypeID ||
		target.Level <= 0 ||
		target.ObservedAt.Before(request.TargetObservedAt) ||
		(!request.TargetRefreshAfter.IsZero() && target.ObservedAt.Before(request.TargetRefreshAfter)) {
		return request, State.CastleState{}, State.MapObservation{},
			Localization.WithError(fmt.Errorf("%w: selected Berimond tower is no longer available", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.7e3c358c", "intent plan became stale before dispatch: selected Berimond tower is no longer available", nil))
	}
	return request, source, target, nil
}

func (application *Application) guardBeriTowerAttack(_ context.Context, arguments json.RawMessage) error {
	var request beriTowerAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return Localization.WithError(fmt.Errorf("Berimond attack state is unavailable"), Localization.New("server.app.berimond_attack_state_is.5835c6f0", "Berimond attack state is unavailable", nil))
	}
	state := application.State.ReadOnlyView()
	if !beriTargetConfirmedAfterGAA(state, request) {
		if err := application.invalidateBeriTarget(request); err != nil {
			return err
		}
		return Localization.WithError(fmt.Errorf(
			"%w: GAA no longer returned Berimond tower %d:%d", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		), Localization.New("server.app.intent_plan_became_stale.ac148c1a", "intent plan became stale before dispatch: GAA no longer returned Berimond tower {p1}:{p2}", Localization.Params{"p1": fmt.Sprintf("%d", request.TargetX), "p2": fmt.Sprintf("%d", request.TargetY)}))
	}
	if application.GameData == nil {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	resolvedRequest, source, target, err := beriTowerAttackContext(
		Intent.PlanningContext{State: state, GameData: gameData}, arguments, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if !source.Focused {
		return Localization.WithError(fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale), Localization.New("server.app.intent_plan_became_stale.5011c69d", "intent plan became stale before dispatch: the Berimond attack source is no longer focused", nil))
	}
	input := Intent.PlanningContext{State: state, GameData: gameData}
	capacity, err := resolveBeriTowerAttackCapacity(input, resolvedRequest, source, target)
	if err != nil {
		return fmt.Errorf("%w: %v", Intent.ErrPlanStale, err)
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	if _, err := buildAttackSetup(invasionAttackSetup(limitedPreset), source, gameData); err != nil {
		return fmt.Errorf("%w: Berimond preset inventory changed: %v", Intent.ErrPlanStale, err)
	}
	return nil
}

func beriTargetConfirmedAfterGAA(gameState State.GameState, request beriTowerAttackRequest) bool {
	if request.TargetRefreshAfter.IsZero() {
		return false
	}
	target, exists := gameState.LookupMapObservation(beriKingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	return exists &&
		target.KingdomID == beriKingdomID &&
		target.X == request.TargetX &&
		target.Y == request.TargetY &&
		target.TypeID == AttackCapacity.BerimondTowerMapTypeID &&
		target.TypeID == request.TargetTypeID &&
		target.Level > 0 &&
		!target.ObservedAt.Before(request.TargetRefreshAfter)
}

func (application *Application) invalidateBeriTarget(request beriTowerAttackRequest) error {
	_, err := application.State.ApplyComponents(State.Components(State.ComponentBeri), func(gameState *State.GameState) ([]string, bool, error) {
		beri := gameState.Beri
		if request.TargetObservedAt.IsZero() || !beri.TargetObservedAt.Equal(request.TargetObservedAt) ||
			beri.TargetX != request.TargetX || beri.TargetY != request.TargetY ||
			beri.TargetTypeID != request.TargetTypeID ||
			!beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) {
			return nil, false, nil
		}
		gameState.Beri.TargetInvalidatedAt = time.Now().UTC()
		return []string{"beri"}, true, nil
	})
	return err
}

func (application *Application) markBeriCampOpened(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		RequestedAt time.Time `json:"requestedAt"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.RequestedAt.IsZero() {
		return Localization.WithError(fmt.Errorf("requestedAt is required"), Localization.New("server.app.requestedat_is_required.9f938b4f", "requestedAt is required", nil))
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentBeri), func(gameState *State.GameState) ([]string, bool, error) {
		if !gameState.Beri.CampOpenRequestedAt.Before(request.RequestedAt) {
			return nil, false, nil
		}
		gameState.Beri.CampOpenRequestedAt = request.RequestedAt
		return []string{"beri"}, true, nil
	})
	return err
}

func ownedCastleInKingdom(gameState State.GameState, kingdomID State.KingdomID) (State.CastleState, bool) {
	for _, castle := range gameState.Castles {
		if castle.KingdomID == kingdomID {
			return castle, true
		}
	}
	return State.CastleState{}, false
}

package App

import (
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
	WireCastleID   int64          `json:"wireCastleId,omitempty"`
	UnitID         State.UnitID   `json:"unitId"`
	Amount         int64          `json:"amount,omitempty"`
	UseTimeSkip    bool           `json:"useTimeSkip,omitempty"`
	TimeSkipID     string         `json:"timeSkipId,omitempty"`
}

type beriTransferGuardRequest struct {
	SourceCastleID   State.CastleID   `json:"sourceCastleId"`
	TargetCastleID   State.CastleID   `json:"targetCastleId"`
	UnitID           State.UnitID     `json:"unitId"`
	Amount           int64            `json:"amount"`
	CapacityObserved time.Time        `json:"capacityObservedAt"`
	TimeSkipCurrency State.CurrencyID `json:"timeSkipCurrencyId"`
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
		return Intent.Plan{}, fmt.Errorf("beriCastleId must identify the active Berimond castle")
	}
	castle, exists := input.State.Castles[request.BeriCastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return Intent.Plan{}, fmt.Errorf(
			"%w: beriCastleId no longer identifies the active Berimond castle", Intent.ErrPlanStale,
		)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || request.SourceCastleID <= 0 || source.KingdomID != 0 {
		return Intent.Plan{}, fmt.Errorf(
			"%w: sourceCastleId no longer identifies an owned Great Empire donor", Intent.ErrPlanStale,
		)
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Intent.Plan{}, fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale)
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
		),
		Steps: []Intent.Step{
			commandStep("Refresh owned-castle troop inventories", "dcl", json.RawMessage(`{"CD":1}`), "dcl"),
			commandStep("Refresh Berimond troop capacity", "fuc", payload, "fuc"),
			{Name: "Verify refreshed Berimond troop capacity", Action: "beri.capacity.verify", ActionArguments: verifyArguments},
		},
	}, nil
}

func (application *Application) verifyBeriCapacity(_ context.Context, arguments json.RawMessage) error {
	var request beriCapacityRefreshRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.BeriCastleID <= 0 || request.SourceCastleID <= 0 || request.RequestedAt.IsZero() {
		return fmt.Errorf("Berimond capacity verification requires target and donor castles plus a request time")
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("Berimond capacity state is unavailable")
	}
	state := application.State.ReadOnlyView()
	castle, exists := state.Castles[request.BeriCastleID]
	if !exists || castle.KingdomID != beriKingdomID {
		return fmt.Errorf("%w: the Berimond capacity castle is no longer owned", Intent.ErrPlanStale)
	}
	source, exists := state.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != 0 {
		return fmt.Errorf("%w: the selected Berimond donor is no longer an owned Great Empire castle", Intent.ErrPlanStale)
	}
	if source.UnitsObservedAt.IsZero() || source.UnitsObservedAt.Before(request.RequestedAt) {
		return fmt.Errorf("%w: the dcl response did not refresh the selected Berimond donor inventory", Intent.ErrPlanStale)
	}
	if state.Beri.ObservedAt.IsZero() || state.Beri.ObservedAt.Before(request.RequestedAt) {
		return fmt.Errorf("%w: the fuc response did not refresh Berimond troop capacity", Intent.ErrPlanStale)
	}
	if state.Beri.ParsedSourceID > 0 && state.Beri.ParsedSourceID != request.SourceCastleID {
		return fmt.Errorf(
			"%w: Berimond capacity was reported for donor %d instead of selected donor %d",
			Intent.ErrPlanStale, state.Beri.ParsedSourceID, request.SourceCastleID,
		)
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
		return Intent.Plan{}, fmt.Errorf(
			"%w: an owned Berimond camp is required before transferring troops", Intent.ErrPlanStale,
		)
	}
	if target.KingdomID != beriKingdomID {
		return Intent.Plan{}, fmt.Errorf(
			"%w: targetCastleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		)
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, fmt.Errorf("unitId must identify the official troop transferred to Berimond")
	}
	if err := validateBeriTransferFoodUnit(input.GameData, request.UnitID); err != nil {
		return Intent.Plan{}, err
	}
	if input.State.Beri.ObservedAt.IsZero() || !input.State.Beri.ConsumedAt.Before(input.State.Beri.ObservedAt) {
		return Intent.Plan{}, fmt.Errorf(
			"%w: Berimond troop capacity has not been refreshed since the last transfer", Intent.ErrPlanStale,
		)
	}
	available := input.State.Beri.AvailableTroops
	if exact, exists := input.State.Beri.TroopsByUnit[request.UnitID]; exists {
		available = exact
	}
	if request.Amount <= 0 {
		request.Amount = available
	}
	if request.Amount <= 0 || request.Amount > available {
		return Intent.Plan{}, fmt.Errorf(
			"%w: amount must be between 1 and the refreshed Berimond capacity %d", Intent.ErrPlanStale, available,
		)
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
	}
	if err := validateBeriTransferState(input, guard); err != nil {
		return Intent.Plan{}, err
	}
	shipment := kingdomTroopShipmentRequest{
		SourceCastleID: source.ID, TargetCastleID: target.ID, TargetKingdomID: beriKingdomID,
		Units: []kingdomTroopShipmentUnit{{UnitID: request.UnitID, Amount: request.Amount}},
	}
	wireCastleID := request.WireCastleID
	if wireCastleID == 0 {
		wireCastleID = -1
	}
	payload, _ := json.Marshal(struct {
		SourceCastleID State.CastleID  `json:"SCID"`
		SourceKingdom  State.KingdomID `json:"SKID"`
		TargetKingdom  State.KingdomID `json:"TKID"`
		WireCastleID   int64           `json:"CID"`
		Troops         [][]int64       `json:"A"`
	}{source.ID, source.KingdomID, beriKingdomID, wireCastleID, [][]int64{{int64(request.UnitID), request.Amount}}})
	guardArguments, _ := json.Marshal(guard)
	sourceConsumeArguments, _ := json.Marshal(shipment)
	capacityConsumeArguments, _ := json.Marshal(struct {
		ObservedAt time.Time `json:"observedAt"`
	}{input.State.Beri.ObservedAt})
	steps := make([]Intent.Step, 0, 7)
	steps = append(steps,
		kingdomTransportContextStep(),
		Intent.RebuildOnResume(Intent.Step{
			Name: "Verify refreshed Berimond troop transfer", Action: "beri.transfer.verify",
			ActionArguments: guardArguments,
		}),
		commandStep("Transfer troops to Berimond", "kut", payload, "kut"),
		Intent.Step{
			Name: "Consume confirmed Berimond donor troops", Action: "troops.kingdom.consume_source",
			ActionArguments: sourceConsumeArguments,
		},
		Intent.Step{Name: "Consume refreshed Berimond capacity", Action: "beri.consume_capacity", ActionArguments: capacityConsumeArguments},
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
		Summary: fmt.Sprintf("Transfer %d of unit %d from %s to Berimond", request.Amount, request.UnitID, castleLabel(source)),
		Steps:   steps,
	}, nil
}

func validateBeriTransferFoodUnit(gameData *GameData.Store, unitID State.UnitID) error {
	usesFood, err := gameData.UnitUsesFoodSupply(unitID)
	if err != nil {
		return fmt.Errorf("validate Berimond transfer unit %d: %w", unitID, err)
	}
	if !usesFood {
		return fmt.Errorf("Berimond troop transfers require a Food-consuming unit; unit %d consumes Mead or Beef", unitID)
	}
	return nil
}

func validateBeriTransferState(input Intent.PlanningContext, request beriTransferGuardRequest) error {
	source, sourceExists := input.State.Castles[request.SourceCastleID]
	target, targetExists := input.State.Castles[request.TargetCastleID]
	if !sourceExists || source.ID <= 0 {
		return fmt.Errorf("Berimond troop donor %d is no longer owned", request.SourceCastleID)
	}
	if source.KingdomID != 0 {
		return fmt.Errorf("Berimond troop donor %d must be a Great Empire castle", request.SourceCastleID)
	}
	if !targetExists || target.ID <= 0 || target.KingdomID != beriKingdomID {
		return fmt.Errorf("Berimond troop destination %d is no longer owned", request.TargetCastleID)
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return fmt.Errorf("the Battle for Berimond is no longer unlocked")
	}
	beri := input.State.Beri
	if request.CapacityObserved.IsZero() || !beri.ObservedAt.Equal(request.CapacityObserved) ||
		!beri.ConsumedAt.Before(beri.ObservedAt) {
		return fmt.Errorf("Berimond troop capacity changed or was already consumed")
	}
	available := beri.AvailableTroops
	if exact, exists := beri.TroopsByUnit[request.UnitID]; exists {
		available = exact
	}
	if request.UnitID <= 0 || request.Amount <= 0 || request.Amount > available {
		return fmt.Errorf("Berimond capacity for unit %d is %d; %d requested", request.UnitID, available, request.Amount)
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
		return fmt.Errorf("Berimond already has a pending or settling troop transport")
	}
	if request.TimeSkipCurrency > 0 && input.State.Player.Currencies[request.TimeSkipCurrency] < 1 {
		return fmt.Errorf("the selected Berimond transfer time skip is no longer available")
	}
	return nil
}

func (application *Application) verifyBeriTransfer(_ context.Context, arguments json.RawMessage) error {
	var request beriTransferGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("Berimond transfer state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
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
			return nil, false, fmt.Errorf("Berimond capacity changed before it could be consumed")
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
		),
		Steps: []Intent.Step{
			kingdomTransportContextStep(),
			Intent.RebuildOnResume(Intent.Step{
				Name: "Verify refreshed Berimond camp availability", Action: "beri.camp.open.verify",
				ActionArguments: guardArguments,
			}),
			commandStep("Open non-premium Berimond camp", "fsc", payload, "fsc"),
			{Name: "Record Berimond camp-open request", Action: "beri.camp.opened", ActionArguments: mark},
			contextCommandStep("Refresh Berimond kingdom state", "kpi", json.RawMessage(`{}`), "kpi"),
		},
	}, nil
}

func beriCampOpenOption(
	input Intent.PlanningContext,
	campID int64,
	refreshedAfter time.Time,
) (GameData.BerimondCampOption, error) {
	if input.GameData == nil {
		return GameData.BerimondCampOption{}, fmt.Errorf("official game data is unavailable")
	}
	option, found := input.GameData.CheapestNonPremiumBerimondCamp(input.State.Player.Level)
	if !found || campID != option.ID {
		return GameData.BerimondCampOption{},
			fmt.Errorf("campId must identify the cheapest unlocked non-premium Berimond camp")
	}
	if _, exists := ownedCastleInKingdom(input.State, beriKingdomID); exists {
		return GameData.BerimondCampOption{},
			fmt.Errorf("%w: an owned Berimond camp already exists", Intent.ErrPlanStale)
	}
	unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]
	if input.State.KingdomTransport.ObservedAt.IsZero() || !observed || !unlock.Unlocked || unlock.Created {
		return GameData.BerimondCampOption{},
			fmt.Errorf("%w: Berimond must be freshly observed as unlocked without an existing camp", Intent.ErrPlanStale)
	}
	if !refreshedAfter.IsZero() && input.State.KingdomTransport.ObservedAt.Before(refreshedAfter) {
		return GameData.BerimondCampOption{},
			fmt.Errorf("the Berimond kingdom list was not refreshed before opening the camp")
	}
	if !input.State.Beri.CampOpenRequestedAt.IsZero() &&
		time.Since(input.State.Beri.CampOpenRequestedAt) < 5*time.Minute {
		return GameData.BerimondCampOption{},
			fmt.Errorf("%w: a Berimond camp-open request is already settling", Intent.ErrPlanStale)
	}
	return option, nil
}

func (application *Application) verifyBeriCampOpen(_ context.Context, arguments json.RawMessage) error {
	var request beriCampOpenGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.CampID <= 0 || request.RefreshStartedAt.IsZero() {
		return fmt.Errorf("Berimond camp verification requires a camp and refresh time")
	}
	if application == nil || application.State == nil || application.GameData == nil {
		return fmt.Errorf("Berimond camp state is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
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
		return Intent.Plan{}, fmt.Errorf("sourceCastleId must identify an owned Berimond camp")
	}
	if !exists || source.KingdomID != beriKingdomID {
		return Intent.Plan{}, fmt.Errorf(
			"%w: sourceCastleId no longer identifies an owned Berimond camp", Intent.ErrPlanStale,
		)
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return Intent.Plan{}, fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale)
	}
	searchStartedAt := time.Now().UTC()
	guardArguments, _ := json.Marshal(beriTargetFindGuardRequest{
		SourceCastleID: source.ID, SearchStartedAt: searchStartedAt,
	})
	steps := make([]Intent.Step, 0, 5)
	steps = append(steps, attackCastleContextStep(source))
	steps = append(steps,
		closeGameUIStep(),
		contextCommandStep("Refresh Berimond world-map context", "gbl", json.RawMessage(`{}`), "gbl"),
		beriFindNextTowerStep(),
		Intent.Step{
			Name: "Verify selected Berimond tower", Action: "beri.target.verify",
			ActionArguments: guardArguments,
		},
	)
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "attack-context", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"map:" + strconv.FormatInt(int64(beriKingdomID), 10),
			"beri-target:" + strconv.FormatInt(int64(beriKingdomID), 10),
		},
		Summary: "Find the next available Berimond tower",
		Steps:   steps,
	}, nil
}

func beriFindNextTowerStep() Intent.Step {
	step := contextCommandStep(
		"Find next available Berimond tower", "fnt", json.RawMessage(`{}`), "fnt",
	)
	step.ResponseBarrier = Intent.ResponseBarrierCommitted
	return step
}

func currentBeriTarget(gameState State.GameState, observedAfter time.Time) (State.MapObservation, error) {
	beri := gameState.Beri
	if beri.TargetObservedAt.IsZero() || !beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) {
		return State.MapObservation{}, fmt.Errorf("Berimond did not select a valid tower")
	}
	if !observedAfter.IsZero() && beri.TargetObservedAt.Before(observedAfter) {
		return State.MapObservation{}, fmt.Errorf("Berimond did not return a fresh tower selection")
	}
	target, exists := gameState.LookupMapObservation(beriKingdomID, fmt.Sprintf("%d:%d", beri.TargetX, beri.TargetY))
	if !exists || beri.TargetTypeID != AttackCapacity.BerimondTowerMapTypeID ||
		target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.Level <= 0 ||
		target.ObservedAt.Before(beri.TargetObservedAt) {
		return State.MapObservation{}, fmt.Errorf("the selected Berimond tower is missing from the refreshed map")
	}
	return target, nil
}

func (application *Application) verifyBeriTargetFound(_ context.Context, arguments json.RawMessage) error {
	var request beriTargetFindGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || request.SearchStartedAt.IsZero() {
		return fmt.Errorf("Berimond target verification requires a source castle and search time")
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("Berimond target state is unavailable")
	}
	state := application.State.ReadOnlyView()
	source, exists := state.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != beriKingdomID {
		return fmt.Errorf("%w: the Berimond attack source is no longer owned", Intent.ErrPlanStale)
	}
	if !source.Focused {
		return fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale)
	}
	if unlock, observed := state.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale)
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
		))
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
	)
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
			Name: "Guard Berimond tower attack", Action: "beri.tower.attack.guard", ActionArguments: resolvedArguments,
		},
		Intent.Step{
			Name: "Build and launch Berimond tower attack", Resolver: "beri.tower.attack.build",
			ResolverArguments: resolvedArguments, AwaitOpcode: "cra", TimeoutMillis: 10_000, SuccessCodes: []int{0},
			CommandDependencies: &Intent.CommandDependencyRequest{
				Opcode: "cra", Payload: craDependencyPayload,
			},
		},
		attackFeatureCaptureStep(attackFeatureCaptureRequest{
			FeatureID: State.AttackFeatureAutoBeriWorld, SourceCastleID: source.ID, CommanderID: request.CommanderID,
			KingdomID: beriKingdomID, TargetTypeID: target.TypeID, TargetX: target.X, TargetY: target.Y,
		}),
		attackCastleRefreshStep("Refresh Berimond source inventory after attack", source),
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
		Summary: fmt.Sprintf("Attack Berimond tower at %d:%d with %s", target.X, target.Y, request.Preset.Name),
		Steps:   steps,
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
		return Intent.Step{}, fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale)
	}
	capacity, err := resolveBeriTowerAttackCapacity(input, request, source, target)
	if err != nil {
		return Intent.Step{}, err
	}
	limitedPreset := AttackPresets.LimitToCapacity(request.Preset, capacity)
	built, err := buildAttackSetup(invasionAttackSetup(limitedPreset), source, input.GameData)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Berimond preset %q: %w", request.Preset.Name, err)
	}
	attack := invasionAttackBody(source, target, request.CommanderID, built)
	if err := applyCastleHorseTravelBoost(&attack, input.GameData, source, request.HorseTravelBoostID); err != nil {
		return Intent.Step{}, fmt.Errorf("resolve Berimond horse travel boost: %w", err)
	}
	body, err := json.Marshal(attack)
	if err != nil {
		return Intent.Step{}, fmt.Errorf("build Berimond CRA payload: %w", err)
	}
	return commandStep(fmt.Sprintf("Attack Berimond tower at %d:%d", target.X, target.Y), "cra", body, "cra"), nil
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
		return AttackCapacity.Result{}, fmt.Errorf("resolve Berimond tower attack capacity: %w", err)
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
		return request, State.CastleState{}, State.MapObservation{}, fmt.Errorf("official game data is unavailable")
	}
	if err := validateHorseTravelBoostID(request.HorseTravelBoostID); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, err
	}
	if err := AttackPresets.Validate(request.Preset); err != nil {
		return request, State.CastleState{}, State.MapObservation{}, fmt.Errorf("invalid Berimond attack preset: %w", err)
	}
	if request.TargetTypeID != AttackCapacity.BerimondTowerMapTypeID {
		return request, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("Berimond attacks require tower target type %d", AttackCapacity.BerimondTowerMapTypeID)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.KingdomID != beriKingdomID {
		return request, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("%w: Berimond attack source is unavailable", Intent.ErrPlanStale)
	}
	if unlock, observed := input.State.KingdomTransport.Unlocks[beriKingdomID]; observed && !unlock.Unlocked {
		return request, State.CastleState{}, State.MapObservation{}, fmt.Errorf("%w: the Battle for Berimond is no longer unlocked", Intent.ErrPlanStale)
	}
	commander, exists := input.State.Commanders[request.CommanderID]
	if !exists || !commander.Available || State.CommanderHasActiveMovementAt(input.State, request.CommanderID, now) {
		return request, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("%w: Berimond commander %d is no longer available", Intent.ErrPlanStale, request.CommanderID)
	}
	beri := input.State.Beri
	if request.TargetObservedAt.IsZero() || !beri.TargetObservedAt.Equal(request.TargetObservedAt) ||
		!beri.TargetInvalidatedAt.Before(beri.TargetObservedAt) ||
		beri.TargetX != request.TargetX || beri.TargetY != request.TargetY || beri.TargetTypeID != request.TargetTypeID {
		return request, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("%w: Berimond tower selection is no longer current", Intent.ErrPlanStale)
	}
	target, exists := input.State.LookupMapObservation(beriKingdomID, fmt.Sprintf("%d:%d", request.TargetX, request.TargetY))
	if !exists || target.KingdomID != beriKingdomID || target.X != request.TargetX || target.Y != request.TargetY ||
		target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.TypeID != request.TargetTypeID ||
		target.Level <= 0 ||
		target.ObservedAt.Before(request.TargetObservedAt) ||
		(!request.TargetRefreshAfter.IsZero() && target.ObservedAt.Before(request.TargetRefreshAfter)) {
		return request, State.CastleState{}, State.MapObservation{},
			fmt.Errorf("%w: selected Berimond tower is no longer available", Intent.ErrPlanStale)
	}
	return request, source, target, nil
}

func (application *Application) guardBeriTowerAttack(_ context.Context, arguments json.RawMessage) error {
	var request beriTowerAttackRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if application == nil || application.State == nil {
		return fmt.Errorf("Berimond attack state is unavailable")
	}
	state := application.State.ReadOnlyView()
	if !beriTargetConfirmedAfterGAA(state, request) {
		if err := application.invalidateBeriTarget(request); err != nil {
			return err
		}
		return fmt.Errorf(
			"%w: GAA no longer returned Berimond tower %d:%d", Intent.ErrPlanStale, request.TargetX, request.TargetY,
		)
	}
	if application.GameData == nil {
		return fmt.Errorf("official game data is unavailable")
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		return fmt.Errorf("official game data is unavailable")
	}
	resolvedRequest, source, target, err := beriTowerAttackContext(
		Intent.PlanningContext{State: state, GameData: gameData}, arguments, time.Now().UTC(),
	)
	if err != nil {
		return err
	}
	if !source.Focused {
		return fmt.Errorf("%w: the Berimond attack source is no longer focused", Intent.ErrPlanStale)
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
		return fmt.Errorf("requestedAt is required")
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

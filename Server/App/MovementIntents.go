package App

import (
	"CitadelDesktop/Server/Localization"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const stationLeaderID = -14

type stationUnitRequest struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type stationRequest struct {
	SourceCastleID          State.CastleID       `json:"sourceCastleId"`
	TargetCastleID          State.CastleID       `json:"targetCastleId"`
	DelayHours              int                  `json:"delayHours"`
	Purpose                 string               `json:"purpose,omitempty"`
	TrackingID              string               `json:"trackingId,omitempty"`
	SafeAfterUnix           int64                `json:"safeAfterUnix,omitempty"`
	DispatchStartedAt       time.Time            `json:"dispatchStartedAt,omitempty"`
	FreshManifest           bool                 `json:"freshManifest,omitempty"`
	FreshUnitsObservedAfter time.Time            `json:"freshUnitsObservedAfter,omitempty"`
	MinimumSend             int64                `json:"minimumSend,omitempty"`
	Reserves                []stationUnitRequest `json:"reserves,omitempty"`
	Units                   []stationUnitRequest `json:"units"`
}

func planTroopsStation(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request stationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.Purpose = strings.TrimSpace(request.Purpose)
	request.TrackingID = strings.TrimSpace(request.TrackingID)
	if request.Purpose != "" && request.TrackingID == "" {
		request.TrackingID = request.Purpose + ":" + strconv.FormatInt(int64(request.SourceCastleID), 10)
	}
	now := time.Now().UTC()
	if (request.Purpose == "autoBird" || request.Purpose == "autoStation") &&
		input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("%s stationing is disabled while Protection Mode is preparing or active", request.Purpose), Localization.New("server.app.p_stationing_is_disabled.3de346af", "{p0} stationing is disabled while Protection Mode is preparing or active", Localization.Params{"p0": fmt.Sprintf("%s", request.Purpose)}))
	}
	if operation, exists := input.State.Stationing[request.TrackingID]; exists &&
		(request.Purpose == "autoBird" || request.Purpose == "autoStation") &&
		operation.ActiveInState(input.State, now) {
		return Intent.Plan{
			Summary: fmt.Sprintf("Skip %s stationing from castle %d; its tracked movement is already active", request.Purpose, request.SourceCastleID), SummaryDescriptor: Localization.New("server.app.skip_p_stationing_from.4ffdd147", "Skip {p0} stationing from castle {p1}; its tracked movement is already active", Localization.Params{"p0": fmt.Sprintf("%s", request.Purpose), "p1": fmt.Sprintf("%d", request.SourceCastleID)}),
		}, nil
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	target, exists := allianceHolding(input.State.Alliance, request.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("target castle %d is not a supported alliance holding", request.TargetCastleID), Localization.New("server.app.target_castle_p_is.3502b089", "target castle {p0} is not a supported alliance holding", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID)}))
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("station movements cannot cross kingdoms"), Localization.New("server.app.station_movements_cannot_cross.5e471898", "station movements cannot cross kingdoms", nil))
	}
	if target.CastleID == source.ID || target.X == source.X && target.Y == source.Y {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("source and target holdings must be different"), Localization.New("server.app.source_and_target_holdings.337153dc", "source and target holdings must be different", nil))
	}
	if request.DelayHours < 1 || request.DelayHours > 12 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("delayHours must be between 1 and 12"), Localization.New("server.app.delayhours_must_be_between.a37d53a4", "delayHours must be between 1 and 12", nil))
	}
	if len(request.Units) == 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("at least one unit stack is required"), Localization.New("server.app.at_least_one_unit.b179fc84", "at least one unit stack is required", nil))
	}
	if request.FreshManifest && request.Purpose != "autoBird" {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("fresh station manifests are only supported for Auto Bird"), Localization.New("server.app.fresh_station_manifests_are.c8ab6823", "fresh station manifests are only supported for Auto Bird", nil))
	}
	if input.GameData == nil {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("official game data is unavailable"), Localization.New("server.app.official_game_data_is.ff6f65a7", "official game data is unavailable", nil))
	}
	unitsCatalog, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	amounts := make(map[State.UnitID]int64, len(request.Units))
	for _, item := range request.Units {
		if item.UnitID <= 0 || item.Amount <= 0 {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("station unit ids and amounts must be positive"), Localization.New("server.app.station_unit_ids_and.681552f2", "station unit ids and amounts must be positive", nil))
		}
		if _, duplicate := amounts[item.UnitID]; duplicate {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("unit %d appears more than once", item.UnitID), Localization.New("server.app.unit_p_appears_more.9dc8f22a", "unit {p0} appears more than once", Localization.Params{"p0": fmt.Sprintf("%d", item.UnitID)}))
		}
		raw, found := unitsCatalog.Find(strconv.FormatInt(int64(item.UnitID), 10))
		if !found {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("unit %d is not in the official unit catalog", item.UnitID), Localization.New("server.app.unit_p_is_not.57d42843", "unit {p0} is not in the official unit catalog", Localization.Params{"p0": fmt.Sprintf("%d", item.UnitID)}))
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("decode unit %d: %w", item.UnitID, decodeErr), Localization.ErrorContext(Localization.New("server.app.decode_unit_p.90388c2e", "decode unit {p0}", Localization.Params{"p0": fmt.Sprintf("%d", item.UnitID)}), decodeErr))
		}
		if GameData.IsToolRecord(record) {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("definition %d is a tool, not a stationable troop", item.UnitID), Localization.New("server.app.definition_p_is_a.fea3a358", "definition {p0} is a tool, not a stationable troop", Localization.Params{"p0": fmt.Sprintf("%d", item.UnitID)}))
		}
		if available := source.Units.Stationed[item.UnitID]; !request.FreshManifest && item.Amount > available {
			return Intent.Plan{}, Localization.WithError(fmt.Errorf("castle %d has %d stationed unit %d; %d requested", source.ID, available, item.UnitID, item.Amount), Localization.New("server.app.castle_p_has_p.7adfb050", "castle {p0} has {p1} stationed unit {p2}; {p3} requested", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": available, "p2": fmt.Sprintf("%d", item.UnitID), "p3": item.Amount}))
		}
		amounts[item.UnitID] = item.Amount
	}
	unitIDs := make([]int64, 0, len(amounts))
	for unitID := range amounts {
		unitIDs = append(unitIDs, int64(unitID))
	}
	sort.Slice(unitIDs, func(left, right int) bool { return unitIDs[left] < unitIDs[right] })
	wireUnits := make([][2]int64, 0, len(unitIDs))
	for _, unitID := range unitIDs {
		wireUnits = append(wireUnits, [2]int64{unitID, amounts[State.UnitID(unitID)]})
	}
	if request.FreshManifest {
		request.FreshUnitsObservedAfter = now
	}
	request.DispatchStartedAt = now
	resolverArguments, _ := json.Marshal(request)
	steps := []Intent.Step{stationCastleContextStep(source)}
	steps = append(steps, stationRouteContextSteps(source, target)...)
	steps = append(steps, Intent.Step{
		Name: "Station troops", NameDescriptor: Localization.New("server.app.station_troops.2778f606", "Station troops", nil), Resolver: "troops.station.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "cds", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	if request.Purpose != "" {
		steps = append(steps, Intent.Step{
			Name: "Track station movement", NameDescriptor: Localization.New("server.app.track_station_movement.f347958d", "Track station movement", nil), Action: "movement.track_station", ActionArguments: resolverArguments,
		})
	}
	summary := fmt.Sprintf("Station %d unit stack(s) from %s", len(wireUnits), castleLabel(source))
	var summaryLocalizationMessage *Localization.Message = Localization.New("server.app.station_p_unit_stack.03584da5", "Station {p0, number} unit stack(s) from {p1}", Localization.Params{"p0": len(wireUnits), "p1": fmt.Sprintf("%s", castleLabel(source))})
	if request.FreshManifest {
		summary = fmt.Sprintf("Station all eligible troops from %s using fresh castle inventory", castleLabel(source))
		summaryLocalizationMessage = Localization.New("server.app.station_all_eligible_troops.a5412f74", "Station all eligible troops from {p0} using fresh castle inventory", Localization.Params{"p0": fmt.Sprintf("%s", castleLabel(source))})
	}
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"alliance-holding:" + strconv.FormatInt(int64(target.CastleID), 10),
		},
		Summary: summary, SummaryDescriptor: Localization.Clone(summaryLocalizationMessage),
		Steps: steps,
	}, nil
}

func resolveTroopsStationStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request stationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	automation := request.Purpose == "autoBird" || request.Purpose == "autoStation"
	now := time.Now().UTC()
	if automation && input.State.Player.ProtectionMode.PreparingOrActive(now) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%s stationing is disabled while Protection Mode is preparing or active", request.Purpose), Localization.New("server.app.p_stationing_is_disabled.3de346af", "{p0} stationing is disabled while Protection Mode is preparing or active", Localization.Params{"p0": fmt.Sprintf("%s", request.Purpose)}))
	}
	if operation, exists := input.State.Stationing[request.TrackingID]; exists && automation &&
		operation.ActiveInState(input.State, now) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("%s stationing is already active from castle %d", request.Purpose, request.SourceCastleID), Localization.New("server.app.p_stationing_is_already.a02fbe4c", "{p0} stationing is already active from castle {p1}", Localization.Params{"p0": fmt.Sprintf("%s", request.Purpose), "p1": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
	}
	target, exists := allianceHolding(input.State.Alliance, request.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("target castle %d is not a supported alliance holding", request.TargetCastleID), Localization.New("server.app.target_castle_p_is.3502b089", "target castle {p0} is not a supported alliance holding", Localization.Params{"p0": fmt.Sprintf("%d", request.TargetCastleID)}))
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("station movements cannot cross kingdoms"), Localization.New("server.app.station_movements_cannot_cross.5e471898", "station movements cannot cross kingdoms", nil))
	}

	var amounts map[State.UnitID]int64
	if request.FreshManifest {
		if request.Purpose != "autoBird" {
			return Intent.Step{}, Localization.WithError(fmt.Errorf("fresh station manifests are only supported for Auto Bird"), Localization.New("server.app.fresh_station_manifests_are.c8ab6823", "fresh station manifests are only supported for Auto Bird", nil))
		}
		if source.UnitsObservedAt.IsZero() || source.UnitsObservedAt.Before(request.FreshUnitsObservedAfter) {
			return Intent.Step{}, Localization.WithError(fmt.Errorf(
				"castle %d troop inventory was not refreshed after the Auto Bird launch was planned",
				source.ID,
			), Localization.New("server.app.castle_p_troop_inventory.8bf4d6da", "castle {p0} troop inventory was not refreshed after the Auto Bird launch was planned", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
		}
		var err error
		amounts, err = freshAutoBirdStationAmounts(input.GameData, source, request.Reserves, request.MinimumSend)
		if err != nil {
			return Intent.Step{}, err
		}
	} else {
		amounts = make(map[State.UnitID]int64, len(request.Units))
		for _, item := range request.Units {
			if item.UnitID <= 0 || item.Amount <= 0 {
				return Intent.Step{}, Localization.WithError(fmt.Errorf("station unit ids and amounts must be positive"), Localization.New("server.app.station_unit_ids_and.681552f2", "station unit ids and amounts must be positive", nil))
			}
			if _, duplicate := amounts[item.UnitID]; duplicate {
				return Intent.Step{}, Localization.WithError(fmt.Errorf("unit %d appears more than once", item.UnitID), Localization.New("server.app.unit_p_appears_more.9dc8f22a", "unit {p0} appears more than once", Localization.Params{"p0": fmt.Sprintf("%d", item.UnitID)}))
			}
			available := source.Units.Stationed[item.UnitID]
			amount := item.Amount
			if amount > available {
				if !automation {
					return Intent.Step{}, Localization.WithError(fmt.Errorf(
						"castle %d now has %d stationed unit %d; %d requested",
						source.ID, available, item.UnitID, amount,
					), Localization.New("server.app.castle_p_now_has.a1231d90", "castle {p0} now has {p1} stationed unit {p2}; {p3} requested", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": available, "p2": fmt.Sprintf("%d", item.UnitID), "p3": amount}))
				}
				amount = available
			}
			if amount > 0 {
				amounts[item.UnitID] = amount
			}
		}
	}
	if len(amounts) == 0 {
		return Intent.Step{}, Localization.WithError(fmt.Errorf("no requested troops remain stationed at castle %d", source.ID), Localization.New("server.app.no_requested_troops_remain.c9983e13", "no requested troops remain stationed at castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
	}
	after := Intent.Step{}
	if request.Purpose != "" {
		after = Intent.Step{Name: "Track accepted support batch", NameDescriptor: Localization.New("server.app.track_accepted_support_batch.167c6a02", "Track accepted support batch", nil), Action: "movement.track_station", ActionArguments: arguments}
	}
	return supportDispatchStep("Station troops", source, target, request.DelayHours, amounts, after).WithNameDescriptor(Localization.New("server.app.station_troops.2778f606", "Station troops", nil)), nil
}

func freshAutoBirdStationAmounts(
	gameData *GameData.Store,
	source State.CastleState,
	reserves []stationUnitRequest,
	minimumSend int64,
) (map[State.UnitID]int64, error) {
	amounts, total, err := autoBirdStationManifest(gameData, source, reserves, false)
	if err != nil {
		return nil, err
	}
	if len(amounts) == 0 {
		return nil, Localization.WithError(fmt.Errorf("fresh castle inventory has no eligible troops to station from castle %d", source.ID), Localization.New("server.app.fresh_castle_inventory_has.cfcee72c", "fresh castle inventory has no eligible troops to station from castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID)}))
	}
	if minimumSend > 0 && total < minimumSend {
		return nil, Localization.WithError(fmt.Errorf(
			"fresh castle inventory has %d eligible troops at castle %d; minimum send is %d",
			total, source.ID, minimumSend,
		), Localization.New("server.app.fresh_castle_inventory_has.09952a2a", "fresh castle inventory has {p0} eligible troops at castle {p1}; minimum send is {p2}", Localization.Params{"p0": total, "p1": fmt.Sprintf("%d", source.ID), "p2": minimumSend}))
	}
	return amounts, nil
}

func planMovementRecall(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		MovementID State.MovementID `json:"movementId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	movement, exists := input.State.LookupMovement(request.MovementID)
	if !exists || request.MovementID <= 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("movement %d is not active", request.MovementID), Localization.New("server.app.movement_p_is_not.20d2eafc", "movement {p0} is not active", Localization.Params{"p0": fmt.Sprintf("%d", request.MovementID)}))
	}
	if movement.Direction != 0 {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("movement %d is already returning", request.MovementID), Localization.New("server.app.movement_p_is_already.49eb93e1", "movement {p0} is already returning", Localization.Params{"p0": fmt.Sprintf("%d", request.MovementID)}))
	}
	if movement.OwnerPlayerID != 0 && movement.OwnerPlayerID != input.State.Player.ID {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("movement %d is not owned by the current player", request.MovementID), Localization.New("server.app.movement_p_is_not.31f60177", "movement {p0} is not owned by the current player", Localization.Params{"p0": fmt.Sprintf("%d", request.MovementID)}))
	}
	if _, ownedSource := input.State.Castles[movement.SourceCastleID]; !ownedSource {
		return Intent.Plan{}, Localization.WithError(fmt.Errorf("movement %d did not originate from an owned castle", request.MovementID), Localization.New("server.app.movement_p_did_not.264ebfb6", "movement {p0} did not originate from an owned castle", Localization.Params{"p0": fmt.Sprintf("%d", request.MovementID)}))
	}
	payload, _ := json.Marshal(struct {
		MovementID State.MovementID `json:"MID"`
	}{request.MovementID})
	return Intent.Plan{
		Claims:  []string{"movement:" + strconv.FormatInt(int64(request.MovementID), 10)},
		Summary: fmt.Sprintf("Recall movement %d", request.MovementID), SummaryDescriptor: Localization.New("server.app.recall_movement_p.0fec506e", "Recall movement {p0}", Localization.Params{"p0": fmt.Sprintf("%d", request.MovementID)}),
		Steps: []Intent.Step{commandStep("Recall station movement", "mcm", payload, "mcm", Localization.New("server.app.recall_station_movement.a6b65856", "Recall station movement", nil))},
	}, nil
}

func (application *Application) trackStationMovement(_ context.Context, arguments json.RawMessage) error {
	var request stationRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	units := make(map[State.UnitID]int64, len(request.Units))
	for _, item := range request.Units {
		units[item.UnitID] = item.Amount
	}
	_, err := application.State.ApplyComponents(State.Components(State.ComponentStationing), func(gameState *State.GameState) ([]string, bool, error) {
		now := time.Now().UTC()
		current := gameState.Stationing[request.TrackingID]
		successCooldownUntil := now.Add(time.Duration(request.DelayHours) * time.Hour)
		next := State.StationingOperation{
			ID: request.TrackingID, Purpose: request.Purpose, SourceCastleID: request.SourceCastleID,
			TargetCastleID: request.TargetCastleID, Units: units, SuccessCooldownUntil: &successCooldownUntil,
			CreatedAt: current.CreatedAt, UpdatedAt: now,
		}
		if next.CreatedAt.IsZero() {
			next.CreatedAt = now
		}
		if request.SafeAfterUnix > 0 {
			safeAfter := time.Unix(request.SafeAfterUnix, 0).UTC()
			next.SafeAfter = &safeAfter
		}
		target, _ := allianceHolding(gameState.Alliance, request.TargetCastleID)
		next.Units = map[State.UnitID]int64{}
		gameState.RangeMovements(func(id State.MovementID, movement State.MovementState) bool {
			if !request.DispatchStartedAt.IsZero() && !movement.StartedAt.IsZero() && movement.StartedAt.Before(request.DispatchStartedAt.Add(-time.Second)) {
				return true
			}
			if movement.SourceCastleID != request.SourceCastleID || movement.Direction != 0 ||
				movement.TargetX != target.X || movement.TargetY != target.Y ||
				(!request.FreshManifest && !stationMovementUnitsWithinRequest(movement.Units, units)) {
				return true
			}
			if id > next.MovementID {
				next.MovementID = id
			}
			next.MovementIDs = append(next.MovementIDs, id)
			for unitID, amount := range movement.Units {
				next.Units[unitID] += amount
			}
			if releasesAt := State.StationMovementReleaseAt(movement); releasesAt != nil &&
				releasesAt.After(*next.SuccessCooldownUntil) {
				release := releasesAt.UTC()
				next.SuccessCooldownUntil = &release
			}
			return true
		})
		if len(next.MovementIDs) == 0 {
			next.Units = units
		}
		sort.Slice(next.MovementIDs, func(i, j int) bool { return next.MovementIDs[i] < next.MovementIDs[j] })
		if reflect.DeepEqual(current, next) {
			return nil, false, nil
		}
		gameState.Stationing[request.TrackingID] = next
		return []string{"stationing"}, true, nil
	})
	return err
}

func stationMovementUnitsWithinRequest(actual, requested map[State.UnitID]int64) bool {
	if len(actual) == 0 {
		return false
	}
	for unitID, amount := range actual {
		if amount <= 0 || amount > requested[unitID] {
			return false
		}
	}
	return true
}

func cloneStationUnits(units map[State.UnitID]int64) map[State.UnitID]int64 {
	cloned := make(map[State.UnitID]int64, len(units))
	for unitID, amount := range units {
		cloned[unitID] = amount
	}
	return cloned
}

func allianceHolding(alliance State.AllianceState, castleID State.CastleID) (State.AllianceHolding, bool) {
	for _, holding := range alliance.Holdings {
		if holding.CastleID == castleID {
			return holding, true
		}
	}
	return State.AllianceHolding{}, false
}

func stationHoldingType(slotType int) bool {
	switch slotType {
	case 0, 1, 3, 4, 5, 6, 12, 22:
		return true
	default:
		return false
	}
}

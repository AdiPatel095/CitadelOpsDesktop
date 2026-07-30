package App

import (
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
		return Intent.Plan{}, fmt.Errorf("%s stationing is disabled while Protection Mode is preparing or active", request.Purpose)
	}
	if operation, exists := input.State.Stationing[request.TrackingID]; exists &&
		(request.Purpose == "autoBird" || request.Purpose == "autoStation") &&
		operation.ActiveAt(input.State.Movements, now) {
		return Intent.Plan{
			Summary: fmt.Sprintf("Skip %s stationing from castle %d; its tracked movement is already active", request.Purpose, request.SourceCastleID),
		}, nil
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	target, exists := allianceHolding(input.State.Alliance, request.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Plan{}, fmt.Errorf("target castle %d is not a supported alliance holding", request.TargetCastleID)
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Plan{}, fmt.Errorf("station movements cannot cross kingdoms")
	}
	if target.CastleID == source.ID || target.X == source.X && target.Y == source.Y {
		return Intent.Plan{}, fmt.Errorf("source and target holdings must be different")
	}
	if request.DelayHours < 1 || request.DelayHours > 12 {
		return Intent.Plan{}, fmt.Errorf("delayHours must be between 1 and 12")
	}
	if len(request.Units) == 0 {
		return Intent.Plan{}, fmt.Errorf("at least one unit stack is required")
	}
	if request.FreshManifest && request.Purpose != "autoBird" {
		return Intent.Plan{}, fmt.Errorf("fresh station manifests are only supported for Auto Bird")
	}
	if input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("official game data is unavailable")
	}
	unitsCatalog, err := input.GameData.Catalog("units")
	if err != nil {
		return Intent.Plan{}, err
	}
	amounts := make(map[State.UnitID]int64, len(request.Units))
	for _, item := range request.Units {
		if item.UnitID <= 0 || item.Amount <= 0 {
			return Intent.Plan{}, fmt.Errorf("station unit ids and amounts must be positive")
		}
		if _, duplicate := amounts[item.UnitID]; duplicate {
			return Intent.Plan{}, fmt.Errorf("unit %d appears more than once", item.UnitID)
		}
		raw, found := unitsCatalog.Find(strconv.FormatInt(int64(item.UnitID), 10))
		if !found {
			return Intent.Plan{}, fmt.Errorf("unit %d is not in the official unit catalog", item.UnitID)
		}
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			return Intent.Plan{}, fmt.Errorf("decode unit %d: %w", item.UnitID, decodeErr)
		}
		if GameData.IsToolRecord(record) {
			return Intent.Plan{}, fmt.Errorf("definition %d is a tool, not a stationable troop", item.UnitID)
		}
		if available := source.Units.Stationed[item.UnitID]; !request.FreshManifest && item.Amount > available {
			return Intent.Plan{}, fmt.Errorf("castle %d has %d stationed unit %d; %d requested", source.ID, available, item.UnitID, item.Amount)
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
	resolverArguments, _ := json.Marshal(request)
	steps := []Intent.Step{stationCastleContextStep(source)}
	steps = append(steps, stationRouteContextSteps(source, target)...)
	steps = append(steps, Intent.Step{
		Name: "Station troops", Resolver: "troops.station.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "cds", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	if request.Purpose != "" {
		steps = append(steps, Intent.Step{
			Name: "Track station movement", Action: "movement.track_station", ActionArguments: resolverArguments,
		})
	}
	summary := fmt.Sprintf("Station %d unit stack(s) from %s", len(wireUnits), castleLabel(source))
	if request.FreshManifest {
		summary = fmt.Sprintf("Station all eligible troops from %s using fresh castle inventory", castleLabel(source))
	}
	return Intent.Plan{
		Claims: []string{
			"castle-focus", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"alliance-holding:" + strconv.FormatInt(int64(target.CastleID), 10),
		},
		Summary: summary,
		Steps:   steps,
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
		return Intent.Step{}, fmt.Errorf("%s stationing is disabled while Protection Mode is preparing or active", request.Purpose)
	}
	if operation, exists := input.State.Stationing[request.TrackingID]; exists && automation &&
		operation.ActiveAt(input.State.Movements, now) {
		return Intent.Step{}, fmt.Errorf("%s stationing is already active from castle %d", request.Purpose, request.SourceCastleID)
	}
	source, exists := input.State.Castles[request.SourceCastleID]
	if !exists || source.ID <= 0 {
		return Intent.Step{}, fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	target, exists := allianceHolding(input.State.Alliance, request.TargetCastleID)
	if !exists || !stationHoldingType(target.SlotType) {
		return Intent.Step{}, fmt.Errorf("target castle %d is not a supported alliance holding", request.TargetCastleID)
	}
	if target.KingdomID != source.KingdomID {
		return Intent.Step{}, fmt.Errorf("station movements cannot cross kingdoms")
	}

	var amounts map[State.UnitID]int64
	if request.FreshManifest {
		if request.Purpose != "autoBird" {
			return Intent.Step{}, fmt.Errorf("fresh station manifests are only supported for Auto Bird")
		}
		if source.UnitsObservedAt.IsZero() || source.UnitsObservedAt.Before(request.FreshUnitsObservedAfter) {
			return Intent.Step{}, fmt.Errorf(
				"castle %d troop inventory was not refreshed after the Auto Bird launch was planned",
				source.ID,
			)
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
				return Intent.Step{}, fmt.Errorf("station unit ids and amounts must be positive")
			}
			if _, duplicate := amounts[item.UnitID]; duplicate {
				return Intent.Step{}, fmt.Errorf("unit %d appears more than once", item.UnitID)
			}
			available := source.Units.Stationed[item.UnitID]
			amount := item.Amount
			if amount > available {
				if !automation {
					return Intent.Step{}, fmt.Errorf(
						"castle %d now has %d stationed unit %d; %d requested",
						source.ID, available, item.UnitID, amount,
					)
				}
				amount = available
			}
			if amount > 0 {
				amounts[item.UnitID] = amount
			}
		}
	}
	if len(amounts) == 0 {
		return Intent.Step{}, fmt.Errorf("no requested troops remain stationed at castle %d", source.ID)
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
	dispatch, _ := json.Marshal(struct {
		SourceID State.CastleID `json:"SID"`
		TargetX  int            `json:"TX"`
		TargetY  int            `json:"TY"`
		LeaderID int            `json:"LID"`
		Wait     int            `json:"WT"`
		Booster  int            `json:"HBW"`
		Premium  int            `json:"BPC"`
		Travel   int            `json:"PTT"`
		Delay    int            `json:"SD"`
		Units    [][2]int64     `json:"A"`
	}{source.ID, target.X, target.Y, stationLeaderID, request.DelayHours, -1, 1, 1, 0, wireUnits})
	return commandStep("Station troops", "cds", dispatch, "cds"), nil
}

func freshAutoBirdStationAmounts(
	gameData *GameData.Store,
	source State.CastleState,
	reserves []stationUnitRequest,
	minimumSend int64,
) (map[State.UnitID]int64, error) {
	amounts, total, err := autoBirdStationManifest(gameData, source, reserves)
	if err != nil {
		return nil, err
	}
	if len(amounts) == 0 {
		return nil, fmt.Errorf("fresh castle inventory has no eligible troops to station from castle %d", source.ID)
	}
	if minimumSend > 0 && total < minimumSend {
		return nil, fmt.Errorf(
			"fresh castle inventory has %d eligible troops at castle %d; minimum send is %d",
			total, source.ID, minimumSend,
		)
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
	movement, exists := input.State.Movements[request.MovementID]
	if !exists || request.MovementID <= 0 {
		return Intent.Plan{}, fmt.Errorf("movement %d is not active", request.MovementID)
	}
	if movement.Direction != 0 {
		return Intent.Plan{}, fmt.Errorf("movement %d is already returning", request.MovementID)
	}
	if movement.OwnerPlayerID != 0 && movement.OwnerPlayerID != input.State.Player.ID {
		return Intent.Plan{}, fmt.Errorf("movement %d is not owned by the current player", request.MovementID)
	}
	if _, ownedSource := input.State.Castles[movement.SourceCastleID]; !ownedSource {
		return Intent.Plan{}, fmt.Errorf("movement %d did not originate from an owned castle", request.MovementID)
	}
	payload, _ := json.Marshal(struct {
		MovementID State.MovementID `json:"MID"`
	}{request.MovementID})
	return Intent.Plan{
		Claims:  []string{"movement:" + strconv.FormatInt(int64(request.MovementID), 10)},
		Summary: fmt.Sprintf("Recall movement %d", request.MovementID),
		Steps:   []Intent.Step{commandStep("Recall station movement", "mcm", payload, "mcm")},
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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
		for id, movement := range gameState.Movements {
			if movement.SourceCastleID != request.SourceCastleID || movement.Direction != 0 ||
				movement.TargetX != target.X || movement.TargetY != target.Y ||
				(!request.FreshManifest && !stationMovementUnitsWithinRequest(movement.Units, units)) {
				continue
			}
			if id > next.MovementID {
				next.MovementID = id
				next.Units = cloneStationUnits(movement.Units)
			}
			if releasesAt := State.StationMovementReleaseAt(movement); releasesAt != nil &&
				releasesAt.After(*next.SuccessCooldownUntil) {
				release := releasesAt.UTC()
				next.SuccessCooldownUntil = &release
			}
		}
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

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
	"CitadelDesktop/Server/State"
)

const kingdomTroopMaximumStacks = 20

type kingdomTroopShipmentUnit struct {
	UnitID State.UnitID `json:"unitId"`
	Amount int64        `json:"amount"`
}

type kingdomTroopShipmentRequest struct {
	SourceCastleID      State.CastleID             `json:"sourceCastleId"`
	TargetCastleID      State.CastleID             `json:"targetCastleId"`
	TargetKingdomID     State.KingdomID            `json:"targetKingdomId"`
	MaximumTargetTroops int64                      `json:"maximumTargetTroops,omitempty"`
	Units               []kingdomTroopShipmentUnit `json:"units"`
}

type kingdomTroopSkipRequest struct {
	TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
	TimeSkipID       string          `json:"timeSkipId"`
	MinimumRemaining int64           `json:"minimumRemaining,omitempty"`
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
	steps = append(steps,
		commandStep("Start kingdom troop transfer", "kut", payload, "kut"),
		Intent.Step{Name: "Consume confirmed donor troops", Action: "troops.kingdom.consume_source", ActionArguments: consumeArguments},
	)
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
	gameState := application.State.Snapshot()
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
	for _, movement := range gameState.Movements {
		if movement.SourceCastleID != target.ID {
			continue
		}
		amount, countErr := countMap(movement.Units)
		if countErr != nil {
			return 0, countErr
		}
		movementTroops = saturatingTroopAdd(movementTroops, amount)
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
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
		source, found := gameState.Castles[request.SourceCastleID]
		if !found {
			return nil, false, fmt.Errorf("confirmed kingdom troop donor %d is unavailable", request.SourceCastleID)
		}
		for unitID, amount := range amounts {
			if source.Units.Stationed[unitID] < amount {
				return nil, false, fmt.Errorf(
					"confirmed kingdom troop donor %d has only %d of unit %d in state; %d were transferred",
					source.ID, source.Units.Stationed[unitID], unitID, amount,
				)
			}
		}
		for unitID, amount := range amounts {
			source.Units.Stationed[unitID] -= amount
			if source.Units.Total[unitID] >= amount {
				source.Units.Total[unitID] -= amount
			}
		}
		source.UnitsObservedAt = time.Now().UTC()
		gameState.Castles[source.ID] = source
		return []string{"castles", "units", "kingdom-transport"}, true, nil
	})
	return err
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
	return Intent.Plan{
		Claims: []string{
			"troop-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10),
			"currency:" + strconv.FormatInt(int64(currencyID), 10),
		},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d troop transport", timeSkipLabel, request.TargetKingdomID),
		Steps:   []Intent.Step{step, timeSkipConsumeStep(input, currencyID)},
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

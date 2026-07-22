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
	SourceCastleID  State.CastleID             `json:"sourceCastleId"`
	TargetCastleID  State.CastleID             `json:"targetCastleId"`
	TargetKingdomID State.KingdomID            `json:"targetKingdomId"`
	Units           []kingdomTroopShipmentUnit `json:"units"`
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
		commandStep("Start kingdom troop transfer", "kut", payload, "kut"),
		Intent.Step{Name: "Consume confirmed donor troops", Action: "troops.kingdom.consume_source", ActionArguments: consumeArguments},
	)
	return Intent.Plan{
		Claims:  claims,
		Summary: fmt.Sprintf("Transfer %s from %s to %s", strings.Join(summaryUnits, ", "), castleLabel(source), castleLabel(target)),
		Steps:   steps,
	}, nil
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
	var request struct {
		TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
		TimeSkipID       string          `json:"timeSkipId"`
		MinimumRemaining int64           `json:"minimumRemaining,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	request.TimeSkipID = strings.ToUpper(strings.TrimSpace(request.TimeSkipID))
	if request.TargetKingdomID < 0 || request.TimeSkipID == "" {
		return Intent.Plan{}, fmt.Errorf("targetKingdomId and timeSkipId are required")
	}
	pending := false
	for _, transport := range input.State.KingdomTransport.PendingUnits {
		if transport.KingdomID == request.TargetKingdomID && transport.RemainingSec > 0 {
			pending = true
			break
		}
	}
	if !pending {
		return Intent.Plan{}, fmt.Errorf("kingdom %d has no pending troop transport", request.TargetKingdomID)
	}
	currencyID, err := officialCurrencyID(input.GameData, request.TimeSkipID)
	if err != nil {
		return Intent.Plan{}, err
	}
	timeSkipLabel := officialTimeSkipLabel(input.GameData, int64(currencyID), request.TimeSkipID)
	if request.MinimumRemaining < 0 {
		return Intent.Plan{}, fmt.Errorf("minimumRemaining cannot be negative")
	}
	if input.State.Player.Currencies[currencyID]-1 < float64(request.MinimumRemaining) {
		return Intent.Plan{}, fmt.Errorf("no %s is available", timeSkipLabel)
	}
	payload, _ := json.Marshal(map[string]string{
		"MST": request.TimeSkipID,
		"KID": strconv.FormatInt(int64(request.TargetKingdomID), 10),
		"TT":  "1",
	})
	return Intent.Plan{
		Claims:  []string{"troop-transport", "kingdom:" + strconv.FormatInt(int64(request.TargetKingdomID), 10)},
		Summary: fmt.Sprintf("Apply a %s to kingdom %d troop transport", timeSkipLabel, request.TargetKingdomID),
		Steps:   []Intent.Step{commandStep("Skip kingdom troop transport time", "msk", payload, "msk")},
	}, nil
}

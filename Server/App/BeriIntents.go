package App

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

const beriKingdomID State.KingdomID = 10

func planBeriCapacityRefresh(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		BeriCastleID State.CastleID `json:"beriCastleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.BeriCastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("beriCastleId must identify the active Berimond castle")
	}
	payload, _ := json.Marshal(struct {
		CastleID State.CastleID `json:"CID"`
	}{request.BeriCastleID})
	return Intent.Plan{
		Claims:  []string{"beri-capacity:" + strconv.FormatInt(int64(request.BeriCastleID), 10)},
		Summary: fmt.Sprintf("Refresh Berimond troop capacity for castle %d", request.BeriCastleID),
		Steps:   []Intent.Step{commandStep("Refresh Berimond troop capacity", "fuc", payload, "fuc")},
	}, nil
}

func planBeriTransfer(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId,omitempty"`
		WireCastleID   int64          `json:"wireCastleId,omitempty"`
		UnitID         State.UnitID   `json:"unitId"`
		Amount         int64          `json:"amount,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	source, err := sourceCastle(input.State, request.SourceCastleID)
	if err != nil {
		return Intent.Plan{}, err
	}
	if request.UnitID <= 0 {
		return Intent.Plan{}, fmt.Errorf("unitId must identify the official troop transferred to Berimond")
	}
	if err := requireOfficialDefinition(input.GameData, "units", int64(request.UnitID)); err != nil {
		return Intent.Plan{}, err
	}
	if input.State.Beri.ObservedAt.IsZero() || !input.State.Beri.ConsumedAt.Before(input.State.Beri.ObservedAt) {
		return Intent.Plan{}, fmt.Errorf("Berimond troop capacity has not been refreshed since the last transfer")
	}
	available := input.State.Beri.AvailableTroops
	if exact, exists := input.State.Beri.TroopsByUnit[request.UnitID]; exists {
		available = exact
	}
	if request.Amount <= 0 {
		request.Amount = available
	}
	if request.Amount <= 0 || request.Amount > available {
		return Intent.Plan{}, fmt.Errorf("amount must be between 1 and the refreshed Berimond capacity %d", available)
	}
	if stationed := source.Units.Stationed[request.UnitID]; stationed > 0 && request.Amount > stationed {
		return Intent.Plan{}, fmt.Errorf("source castle %d has %d of unit %d, fewer than requested %d", source.ID, stationed, request.UnitID, request.Amount)
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
	consume, _ := json.Marshal(struct {
		ObservedAt time.Time `json:"observedAt"`
	}{input.State.Beri.ObservedAt})
	return Intent.Plan{
		Claims: []string{
			"beri-transfer", "castle:" + strconv.FormatInt(int64(source.ID), 10),
			"unit:" + strconv.FormatInt(int64(request.UnitID), 10),
		},
		Summary: fmt.Sprintf("Transfer %d of unit %d from %s to Berimond", request.Amount, request.UnitID, castleLabel(source)),
		Steps: []Intent.Step{
			commandStep("Transfer troops to Berimond", "kut", payload, "kut"),
			commandStep("Apply Berimond transfer speed-up", "msk", json.RawMessage(`{"MST":"MS5","KID":"10","TT":"1"}`), "msk"),
			{Name: "Consume refreshed Berimond capacity", Action: "beri.consume_capacity", ActionArguments: consume},
		},
	}, nil
}

func (application *Application) consumeBeriCapacity(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		ObservedAt time.Time `json:"observedAt"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	_, err := application.State.Apply(func(gameState *State.GameState) ([]string, bool, error) {
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

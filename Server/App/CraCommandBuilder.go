package App

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

type craPayloadBuilder func(State.CommanderID) (json.RawMessage, error)
type craPostCommandStepBuilder func(State.CommanderID) Intent.Step

type craInventoryGuardRequest struct {
	SourceCastleID State.CastleID    `json:"sourceCastleId"`
	Payloads       []json.RawMessage `json:"payloads"`
}

func buildCRACommandSteps(
	source State.CastleState,
	commanders []State.CommanderID,
	action string,
	build craPayloadBuilder,
	postCommand ...craPostCommandStepBuilder,
) ([]Intent.Step, error) {
	if len(commanders) == 0 {
		return nil, fmt.Errorf("at least one CRA commander is required")
	}
	payloads := make([]json.RawMessage, len(commanders))
	for index, commanderID := range commanders {
		payload, err := build(commanderID)
		if err != nil {
			return nil, fmt.Errorf("build CRA payload for commander %d: %w", commanderID, err)
		}
		payload, err = ensureCRASupportPayload(payload)
		if err != nil {
			return nil, fmt.Errorf("assemble CRA support wave for commander %d: %w", commanderID, err)
		}
		payloads[index] = payload
	}
	steps := make([]Intent.Step, 0, len(commanders)+2)
	steps = append(steps, attackCastleContextStep(source), craInventoryGuardStep(source.ID, payloads))
	for index, commanderID := range commanders {
		steps = append(steps, commandStep(
			fmt.Sprintf("%s with commander %d", action, commanderID), "cra", payloads[index], "cra",
		))
		for _, buildStep := range postCommand {
			if buildStep != nil {
				steps = append(steps, buildStep(commanderID))
			}
		}
	}
	return steps, nil
}

func craInventoryGuardStep(sourceCastleID State.CastleID, payloads []json.RawMessage) Intent.Step {
	arguments, _ := json.Marshal(craInventoryGuardRequest{SourceCastleID: sourceCastleID, Payloads: payloads})
	return Intent.Step{Name: "Verify fresh attack inventory", Action: "attack.inventory.guard", ActionArguments: arguments}
}

func (application *Application) guardCRAInventory(_ context.Context, arguments json.RawMessage) error {
	var request craInventoryGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || len(request.Payloads) == 0 {
		return fmt.Errorf("fresh CRA inventory guard requires a source castle and payload")
	}
	source, exists := application.State.ReadOnlyView().Castles[request.SourceCastleID]
	if !exists {
		return fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID)
	}
	return validateCRAInventoryPayloads(request.Payloads, source)
}

func craCommanderClaims(candidates []State.CommanderID) []string {
	claims := make([]string, 0, len(candidates)*2)
	for _, commanderID := range candidates {
		wireID := strconv.FormatInt(int64(commanderID), 10)
		claims = append(claims, "commander:"+wireID, "leader:commander:"+wireID)
	}
	return claims
}

func craPayloadWithCommander(fields map[string]json.RawMessage, commanderID State.CommanderID) (json.RawMessage, error) {
	copy := make(map[string]json.RawMessage, len(fields))
	for key, value := range fields {
		copy[key] = append(json.RawMessage(nil), value...)
	}
	copy["LID"], _ = json.Marshal(commanderID)
	return json.Marshal(copy)
}

func ensureCRASupportPayload(payload json.RawMessage) (json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("CRA payload is invalid: %w", err)
	}
	if fields == nil {
		return nil, fmt.Errorf("CRA payload must be an object")
	}
	if missingCRAField(fields, "AST") {
		fields["AST"], _ = json.Marshal(emptyAttackSupportTools())
	}
	if missingCRAField(fields, "RW") {
		fields["RW"], _ = json.Marshal(emptyAttackSupportTroops())
	}
	if missingCRAField(fields, "ASCT") {
		fields["ASCT"] = json.RawMessage(`0`)
	}
	return json.Marshal(fields)
}

func missingCRAField(fields map[string]json.RawMessage, key string) bool {
	raw, exists := fields[key]
	return !exists || len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func riftReplaySourceCastle(
	gameState State.GameState,
	requested State.CastleID,
	fields map[string]json.RawMessage,
) (State.CastleState, error) {
	if requested > 0 {
		castle, err := sourceCastle(gameState, requested)
		if err != nil {
			return State.CastleState{}, err
		}
		var sourceX, sourceY int
		if err := json.Unmarshal(fields["SX"], &sourceX); err != nil || sourceX != castle.X {
			return State.CastleState{}, fmt.Errorf("CRA source X does not match owned castle %d", castle.ID)
		}
		if err := json.Unmarshal(fields["SY"], &sourceY); err != nil || sourceY != castle.Y {
			return State.CastleState{}, fmt.Errorf("CRA source Y does not match owned castle %d", castle.ID)
		}
		return castle, nil
	}
	var sourceX, sourceY int
	if err := json.Unmarshal(fields["SX"], &sourceX); err != nil {
		return State.CastleState{}, fmt.Errorf("captured CRA source X is invalid; sourceCastleId is required")
	}
	if err := json.Unmarshal(fields["SY"], &sourceY); err != nil {
		return State.CastleState{}, fmt.Errorf("captured CRA source Y is invalid; sourceCastleId is required")
	}
	for _, castle := range gameState.Castles {
		if castle.X == sourceX && castle.Y == sourceY {
			return castle, nil
		}
	}
	return State.CastleState{}, fmt.Errorf(
		"captured CRA source %d:%d is not an owned castle; sourceCastleId is required",
		sourceX, sourceY,
	)
}

func validateRepeatedAttackInventory(fields map[string]json.RawMessage, source State.CastleState, copies int) error {
	if copies < 1 {
		return fmt.Errorf("repeated CRA attack requires at least one commander")
	}
	requested := map[State.UnitID]int64{}
	if err := addCRAFieldsInventory(requested, fields); err != nil {
		return err
	}
	for itemID, amount := range requested {
		if amount > math.MaxInt64/int64(copies) {
			return fmt.Errorf("captured CRA quantity for item %d is too large", itemID)
		}
		required := amount * int64(copies)
		available := source.Units.Stationed[itemID]
		if required > available {
			return fmt.Errorf(
				"castle %d has %d of item %d; %d commander(s) require %d",
				source.ID, available, itemID, copies, required,
			)
		}
	}
	return nil
}

func validateCRAInventoryPayloads(payloads []json.RawMessage, source State.CastleState) error {
	requested := map[State.UnitID]int64{}
	for _, payload := range payloads {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			return fmt.Errorf("CRA inventory guard payload is invalid: %w", err)
		}
		if err := addCRAFieldsInventory(requested, fields); err != nil {
			return err
		}
	}
	for itemID, required := range requested {
		available := source.Units.Stationed[itemID]
		if required > available {
			return fmt.Errorf(
				"castle %d has %d available of item %d; attack formation requires %d",
				source.ID, available, itemID, required,
			)
		}
	}
	return nil
}

func addCRAFieldsInventory(requested map[State.UnitID]int64, fields map[string]json.RawMessage) error {
	var waves []attackWave
	if err := json.Unmarshal(fields["A"], &waves); err != nil || len(waves) == 0 {
		return fmt.Errorf("captured CRA formation is invalid")
	}
	for _, wave := range waves {
		for _, flank := range []attackFlank{wave.Left, wave.Middle, wave.Right} {
			if err := addCapturedAttackPairs(requested, flank.Units); err != nil {
				return err
			}
			if err := addCapturedAttackPairs(requested, flank.Tools); err != nil {
				return err
			}
		}
	}
	if raw := fields["RW"]; len(raw) > 0 {
		var supportTroops []attackPair
		if err := json.Unmarshal(raw, &supportTroops); err != nil {
			return fmt.Errorf("captured CRA support troops are invalid")
		}
		if err := addCapturedAttackPairs(requested, supportTroops); err != nil {
			return err
		}
	}
	if raw := fields["AST"]; len(raw) > 0 {
		var supportTools []int64
		if err := json.Unmarshal(raw, &supportTools); err != nil {
			return fmt.Errorf("captured CRA support tools are invalid")
		}
		for _, itemID := range supportTools {
			if itemID <= 0 {
				continue
			}
			if err := addCapturedAttackItem(requested, State.UnitID(itemID), 1); err != nil {
				return err
			}
		}
	}
	return nil
}

func addCapturedAttackPairs(requested map[State.UnitID]int64, pairs []attackPair) error {
	for _, pair := range pairs {
		itemID, amount := pair[0], pair[1]
		if itemID <= 0 && amount == 0 {
			continue
		}
		if itemID <= 0 || amount < 0 {
			return fmt.Errorf("captured CRA formation contains an invalid item allocation")
		}
		if amount == 0 {
			continue
		}
		if err := addCapturedAttackItem(requested, State.UnitID(itemID), amount); err != nil {
			return err
		}
	}
	return nil
}

func addCapturedAttackItem(requested map[State.UnitID]int64, itemID State.UnitID, amount int64) error {
	if amount > math.MaxInt64-requested[itemID] {
		return fmt.Errorf("captured CRA quantity for item %d is too large", itemID)
	}
	requested[itemID] += amount
	return nil
}

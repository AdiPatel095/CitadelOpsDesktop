package App

import (
	"CitadelDesktop/Server/Localization"
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
		return nil, Localization.WithError(fmt.Errorf("at least one CRA commander is required"), Localization.New("server.app.at_least_one_cra.dd0a8c5f", "at least one CRA commander is required", nil))
	}
	payloads := make([]json.RawMessage, len(commanders))
	for index, commanderID := range commanders {
		payload, err := build(commanderID)
		if err != nil {
			return nil, Localization.WithError(fmt.Errorf("build CRA payload for commander %d: %w", commanderID, err), Localization.ErrorContext(Localization.New("server.app.build_cra_payload_for.650e7f51", "build CRA payload for commander {p0}", Localization.Params{"p0": fmt.Sprintf("%d", commanderID)}), err))
		}
		payload, err = ensureCRASupportPayload(payload)
		if err != nil {
			return nil, Localization.WithError(fmt.Errorf("assemble CRA support wave for commander %d: %w", commanderID, err), Localization.ErrorContext(Localization.New("server.app.assemble_cra_support_wave.009e9229", "assemble CRA support wave for commander {p0}", Localization.Params{"p0": fmt.Sprintf("%d", commanderID)}), err))
		}
		payloads[index] = payload
	}
	steps := make([]Intent.Step, 0, len(commanders)+2)
	steps = append(steps, attackCastleContextStep(source), craInventoryGuardStep(source.ID, payloads))
	for index, commanderID := range commanders {
		steps = append(steps, commandStep(
			fmt.Sprintf("%s with commander %d", action, commanderID), "cra", payloads[index], "cra", Localization.New("server.app.p_with_commander_p.9e96f75f", "{p0} with commander {p1}", Localization.Params{"p0": fmt.Sprintf("%s", action), "p1": fmt.Sprintf("%d", commanderID)}),
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
	return Intent.Step{Name: "Verify fresh attack inventory", NameDescriptor: Localization.New("server.app.verify_fresh_attack_inventory.06dd12e9", "Verify fresh attack inventory", nil), Action: "attack.inventory.guard", ActionArguments: arguments}
}

func (application *Application) guardCRAInventory(_ context.Context, arguments json.RawMessage) error {
	var request craInventoryGuardRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	if request.SourceCastleID <= 0 || len(request.Payloads) == 0 {
		return Localization.WithError(fmt.Errorf("fresh CRA inventory guard requires a source castle and payload"), Localization.New("server.app.fresh_cra_inventory_guard.b04f85a5", "fresh CRA inventory guard requires a source castle and payload", nil))
	}
	source, exists := application.State.ReadOnlyView().Castles[request.SourceCastleID]
	if !exists {
		return Localization.WithError(fmt.Errorf("source castle %d is not in the current player state", request.SourceCastleID), Localization.New("server.app.source_castle_p_is.fca7f6bd", "source castle {p0} is not in the current player state", Localization.Params{"p0": fmt.Sprintf("%d", request.SourceCastleID)}))
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
		return nil, Localization.WithError(fmt.Errorf("CRA payload is invalid: %w", err), Localization.ErrorContext(Localization.New("server.app.cra_payload_is_invalid.eaad9084", "CRA payload is invalid", nil), err))
	}
	if fields == nil {
		return nil, Localization.WithError(fmt.Errorf("CRA payload must be an object"), Localization.New("server.app.cra_payload_must_be.a8b577d1", "CRA payload must be an object", nil))
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
			return State.CastleState{}, Localization.WithError(fmt.Errorf("CRA source X does not match owned castle %d", castle.ID), Localization.New("server.app.cra_source_x_does.1c3bc554", "CRA source X does not match owned castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
		}
		if err := json.Unmarshal(fields["SY"], &sourceY); err != nil || sourceY != castle.Y {
			return State.CastleState{}, Localization.WithError(fmt.Errorf("CRA source Y does not match owned castle %d", castle.ID), Localization.New("server.app.cra_source_y_does.b65d37b3", "CRA source Y does not match owned castle {p0}", Localization.Params{"p0": fmt.Sprintf("%d", castle.ID)}))
		}
		return castle, nil
	}
	var sourceX, sourceY int
	if err := json.Unmarshal(fields["SX"], &sourceX); err != nil {
		return State.CastleState{}, Localization.WithError(fmt.Errorf("captured CRA source X is invalid; sourceCastleId is required"), Localization.New("server.app.captured_cra_source_x.7bef6a25", "captured CRA source X is invalid; sourceCastleId is required", nil))
	}
	if err := json.Unmarshal(fields["SY"], &sourceY); err != nil {
		return State.CastleState{}, Localization.WithError(fmt.Errorf("captured CRA source Y is invalid; sourceCastleId is required"), Localization.New("server.app.captured_cra_source_y.a94fbfe0", "captured CRA source Y is invalid; sourceCastleId is required", nil))
	}
	for _, castle := range gameState.Castles {
		if castle.X == sourceX && castle.Y == sourceY {
			return castle, nil
		}
	}
	return State.CastleState{}, Localization.WithError(fmt.Errorf(
		"captured CRA source %d:%d is not an owned castle; sourceCastleId is required",
		sourceX, sourceY,
	), Localization.New("server.app.captured_cra_source_p.01b35121", "captured CRA source {p0}:{p1} is not an owned castle; sourceCastleId is required", Localization.Params{"p0": sourceX, "p1": sourceY}))
}

func validateRepeatedAttackInventory(fields map[string]json.RawMessage, source State.CastleState, copies int) error {
	if copies < 1 {
		return Localization.WithError(fmt.Errorf("repeated CRA attack requires at least one commander"), Localization.New("server.app.repeated_cra_attack_requires.9a8d7c3c", "repeated CRA attack requires at least one commander", nil))
	}
	requested := map[State.UnitID]int64{}
	if err := addCRAFieldsInventory(requested, fields); err != nil {
		return err
	}
	for itemID, amount := range requested {
		if amount > math.MaxInt64/int64(copies) {
			return Localization.WithError(fmt.Errorf("captured CRA quantity for item %d is too large", itemID), Localization.New("server.app.captured_cra_quantity_for.bfadee06", "captured CRA quantity for item {p0} is too large", Localization.Params{"p0": fmt.Sprintf("%d", itemID)}))
		}
		required := amount * int64(copies)
		available := source.Units.Stationed[itemID]
		if required > available {
			return Localization.WithError(fmt.Errorf(
				"castle %d has %d of item %d; %d commander(s) require %d",
				source.ID, available, itemID, copies, required,
			), Localization.New("server.app.castle_p_has_p.310da036", "castle {p0} has {p1} of item {p2}; {p3} commander(s) require {p4}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": available, "p2": fmt.Sprintf("%d", itemID), "p3": copies, "p4": required}))
		}
	}
	return nil
}

func validateCRAInventoryPayloads(payloads []json.RawMessage, source State.CastleState) error {
	requested := map[State.UnitID]int64{}
	for _, payload := range payloads {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(payload, &fields); err != nil {
			return Localization.WithError(fmt.Errorf("CRA inventory guard payload is invalid: %w", err), Localization.ErrorContext(Localization.New("server.app.cra_inventory_guard_payload.91861f7c", "CRA inventory guard payload is invalid", nil), err))
		}
		if err := addCRAFieldsInventory(requested, fields); err != nil {
			return err
		}
	}
	for itemID, required := range requested {
		available := source.Units.Stationed[itemID]
		if required > available {
			return Localization.WithError(fmt.Errorf(
				"castle %d has %d available of item %d; attack formation requires %d",
				source.ID, available, itemID, required,
			), Localization.New("server.app.castle_p_has_p.5abfc142", "castle {p0} has {p1} available of item {p2}; attack formation requires {p3}", Localization.Params{"p0": fmt.Sprintf("%d", source.ID), "p1": available, "p2": fmt.Sprintf("%d", itemID), "p3": required}))
		}
	}
	return nil
}

func addCRAFieldsInventory(requested map[State.UnitID]int64, fields map[string]json.RawMessage) error {
	var waves []attackWave
	if err := json.Unmarshal(fields["A"], &waves); err != nil || len(waves) == 0 {
		return Localization.WithError(fmt.Errorf("captured CRA formation is invalid"), Localization.New("server.app.captured_cra_formation_is.fae97f1a", "captured CRA formation is invalid", nil))
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
			return Localization.WithError(fmt.Errorf("captured CRA support troops are invalid"), Localization.New("server.app.captured_cra_support_troops.15a866b3", "captured CRA support troops are invalid", nil))
		}
		if err := addCapturedAttackPairs(requested, supportTroops); err != nil {
			return err
		}
	}
	if raw := fields["AST"]; len(raw) > 0 {
		var supportTools []int64
		if err := json.Unmarshal(raw, &supportTools); err != nil {
			return Localization.WithError(fmt.Errorf("captured CRA support tools are invalid"), Localization.New("server.app.captured_cra_support_tools.1b78656e", "captured CRA support tools are invalid", nil))
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
			return Localization.WithError(fmt.Errorf("captured CRA formation contains an invalid item allocation"), Localization.New("server.app.captured_cra_formation_contains.6365dc0c", "captured CRA formation contains an invalid item allocation", nil))
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
		return Localization.WithError(fmt.Errorf("captured CRA quantity for item %d is too large", itemID), Localization.New("server.app.captured_cra_quantity_for.bfadee06", "captured CRA quantity for item {p0} is too large", Localization.Params{"p0": fmt.Sprintf("%d", itemID)}))
	}
	requested[itemID] += amount
	return nil
}

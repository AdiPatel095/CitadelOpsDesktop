package App

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/RiftTemplates"
	"CitadelDesktop/Server/State"
)

var configuredRiftCRAFields = map[string]struct{}{
	"SX": {}, "SY": {}, "TX": {}, "TY": {}, "KID": {}, "LID": {},
	"WT": {}, "HBW": {}, "BPC": {}, "ATT": {}, "AV": {}, "LP": {},
	"FC": {}, "PTT": {}, "SD": {}, "ICA": {}, "CD": {}, "ASCT": {},
	"A": {}, "BKS": {}, "AST": {}, "RW": {},
}

// riftReplayLaunch resolves the account-owned catalog before the transient
// runtime capture. A durable tombstone deliberately blocks fallback so an old
// worker snapshot cannot resurrect a deleted replay.
func (application *Application) riftReplayLaunch(
	state State.GameState,
	launchID string,
) (State.RiftLaunch, bool, error) {
	if application != nil && application.Configuration != nil {
		if raw, exists := application.Configuration.Section(RiftTemplates.ConfigurationSection); exists {
			document, err := RiftTemplates.Decode(raw)
			if err != nil {
				return State.RiftLaunch{}, false, fmt.Errorf("Rift template configuration is invalid: %w", err)
			}
			if _, deleted := document.DeletedLaunchIDs[launchID]; deleted {
				return State.RiftLaunch{}, false, fmt.Errorf("Rift launch %q was deleted", launchID)
			}
			if launch, configured := document.Launches[launchID]; configured {
				if err := validateConfiguredRiftLaunch(state, launchID, launch); err != nil {
					return State.RiftLaunch{}, false, err
				}
				return launch, true, nil
			}
		}
	}
	launch, exists := state.Rift.Launches[launchID]
	if !exists || len(launch.Body) == 0 {
		return State.RiftLaunch{}, false, fmt.Errorf("Rift launch %q has no captured 2.0 command body", launchID)
	}
	return launch, false, nil
}

func validateConfiguredRiftLaunch(state State.GameState, launchID string, launch State.RiftLaunch) error {
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(launch.Body, &fields); err != nil || fields == nil {
		return fmt.Errorf("configured Rift launch %q has an invalid command body", launchID)
	}
	for key := range fields {
		if _, supported := configuredRiftCRAFields[key]; !supported {
			return fmt.Errorf("configured Rift launch %q contains unsupported CRA field %q", launchID, key)
		}
	}
	for _, key := range []string{"SX", "SY", "TX", "TY", "KID", "LID"} {
		if _, err := requiredRiftInteger(fields, key); err != nil {
			return fmt.Errorf("configured Rift launch %q: %w", launchID, err)
		}
	}
	for _, key := range []string{"WT", "HBW", "BPC", "ATT", "AV", "LP", "FC", "PTT", "SD", "ICA", "CD", "ASCT"} {
		if raw, exists := fields[key]; exists {
			if _, err := decodeRiftInteger(raw, key); err != nil {
				return fmt.Errorf("configured Rift launch %q: %w", launchID, err)
			}
		}
	}
	if State.CommanderID(rawMapInt(fields, "LID")) != launch.CommanderID ||
		int(rawMapInt(fields, "SX")) != launch.SourceX || int(rawMapInt(fields, "SY")) != launch.SourceY ||
		int(rawMapInt(fields, "TX")) != launch.TargetX || int(rawMapInt(fields, "TY")) != launch.TargetY ||
		State.KingdomID(rawMapInt(fields, "KID")) != launch.KingdomID {
		return fmt.Errorf("configured Rift launch %q command body does not match its captured record", launchID)
	}
	if raw, exists := fields["AV"]; exists {
		attackValid, _ := decodeRiftInteger(raw, "AV")
		if int(attackValid) != launch.AttackValid {
			return fmt.Errorf("configured Rift launch %q attack validity does not match its captured record", launchID)
		}
	}
	waveCount, err := validateConfiguredRiftLayout(fields)
	if err != nil {
		return fmt.Errorf("configured Rift launch %q: %w", launchID, err)
	}
	if waveCount != launch.WaveCount {
		return fmt.Errorf(
			"configured Rift launch %q contains %d waves but its record declares %d",
			launchID, waveCount, launch.WaveCount,
		)
	}
	useTravelFeather := rawMapInt(fields, "HBW") == -1 && rawMapInt(fields, "PTT") == 1
	if useTravelFeather != launch.UseTravelFeather {
		return fmt.Errorf("configured Rift launch %q travel fields do not match its captured record", launchID)
	}
	if _, err := configuredRiftSourceCastle(state, launch); err != nil {
		return fmt.Errorf("configured Rift launch %q: %w", launchID, err)
	}
	target, found := state.LookupMapObservation(
		launch.KingdomID, fmt.Sprintf("%d:%d", launch.TargetX, launch.TargetY),
	)
	if !found || target.TypeID != riftMapTypeID || target.KingdomID != launch.KingdomID ||
		target.X != launch.TargetX || target.Y != launch.TargetY {
		return fmt.Errorf(
			"configured Rift launch %q target %d:%d is not a current known Rift target type %d",
			launchID, launch.TargetX, launch.TargetY, riftMapTypeID,
		)
	}
	return nil
}

func configuredRiftSourceCastle(state State.GameState, launch State.RiftLaunch) (State.CastleState, error) {
	for _, castle := range state.Castles {
		if castle.KingdomID == launch.KingdomID && castle.X == launch.SourceX && castle.Y == launch.SourceY {
			return castle, nil
		}
	}
	return State.CastleState{}, fmt.Errorf(
		"captured source %d:%d in kingdom %d is not an owned castle",
		launch.SourceX, launch.SourceY, launch.KingdomID,
	)
}

func validateConfiguredRiftLayout(fields map[string]json.RawMessage) (int, error) {
	var waves []json.RawMessage
	if raw, exists := fields["A"]; !exists || json.Unmarshal(raw, &waves) != nil ||
		len(waves) < 1 || len(waves) > AttackPresets.MaximumWaves {
		return 0, fmt.Errorf("CRA formation must contain between 1 and %d waves", AttackPresets.MaximumWaves)
	}
	for index, raw := range waves {
		wave, err := decodeRiftObject(raw, "wave "+strconv.Itoa(index+1), "L", "M", "R")
		if err != nil {
			return 0, err
		}
		for _, lane := range []struct {
			key       string
			name      string
			unitSlots int
			toolSlots int
		}{{"L", "left", 2, 2}, {"M", "middle", 6, 3}, {"R", "right", 2, 2}} {
			if rawLane, exists := wave[lane.key]; exists {
				if err := validateConfiguredRiftLane(rawLane, fmt.Sprintf("wave %d %s", index+1, lane.name), lane.unitSlots, lane.toolSlots); err != nil {
					return 0, err
				}
			}
		}
	}
	if raw, exists := fields["AST"]; exists && string(raw) != "null" {
		var tools []int64
		if err := json.Unmarshal(raw, &tools); err != nil || len(tools) > AttackPresets.CourtyardToolSlots {
			return 0, fmt.Errorf("CRA support tools are invalid or exceed %d slots", AttackPresets.CourtyardToolSlots)
		}
		for _, itemID := range tools {
			if itemID == 0 || itemID < -1 {
				return 0, fmt.Errorf("CRA support tools contain an invalid item id")
			}
		}
	}
	if raw, exists := fields["RW"]; exists && string(raw) != "null" {
		if err := validateConfiguredRiftPairs(raw, "CRA support troops", AttackPresets.CourtyardTroopSlots); err != nil {
			return 0, err
		}
	}
	if raw, exists := fields["BKS"]; exists && string(raw) != "null" {
		var books []json.RawMessage
		if err := json.Unmarshal(raw, &books); err != nil || len(books) != 0 {
			return 0, fmt.Errorf("CRA books payload must be an empty array")
		}
	}
	if raw, exists := fields["ASCT"]; exists {
		count, _ := decodeRiftInteger(raw, "ASCT")
		if count < 0 || count > AttackPresets.CourtyardTroopSlots {
			return 0, fmt.Errorf("CRA support troop count is outside supported bounds")
		}
	}
	return len(waves), nil
}

func validateConfiguredRiftLane(raw json.RawMessage, label string, unitSlots, toolSlots int) error {
	lane, err := decodeRiftObject(raw, label+" lane", "U", "T")
	if err != nil {
		return err
	}
	if rawUnits, exists := lane["U"]; exists {
		if err := validateConfiguredRiftPairs(rawUnits, label+" units", unitSlots); err != nil {
			return err
		}
	}
	if rawTools, exists := lane["T"]; exists {
		if err := validateConfiguredRiftPairs(rawTools, label+" tools", toolSlots); err != nil {
			return err
		}
	}
	return nil
}

func validateConfiguredRiftPairs(raw json.RawMessage, label string, maximum int) error {
	var pairs []json.RawMessage
	if err := json.Unmarshal(raw, &pairs); err != nil || len(pairs) > maximum {
		return fmt.Errorf("%s are invalid or exceed %d slots", label, maximum)
	}
	for index, rawPair := range pairs {
		var values []json.RawMessage
		if err := json.Unmarshal(rawPair, &values); err != nil || len(values) != 2 {
			return fmt.Errorf("%s slot %d must contain exactly an item id and quantity", label, index+1)
		}
		itemID, err := decodeRiftInteger(values[0], label+" item id")
		if err != nil {
			return err
		}
		quantity, err := decodeRiftInteger(values[1], label+" quantity")
		if err != nil {
			return err
		}
		if quantity < 0 || itemID == 0 || itemID < -1 || (itemID == -1 && quantity != 0) {
			return fmt.Errorf("%s slot %d contains an invalid allocation", label, index+1)
		}
	}
	return nil
}

func decodeRiftObject(raw json.RawMessage, label string, allowed ...string) (map[string]json.RawMessage, error) {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil || object == nil {
		return nil, fmt.Errorf("%s must be a JSON object", label)
	}
	supported := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		supported[key] = struct{}{}
	}
	for key := range object {
		if _, exists := supported[key]; !exists {
			return nil, fmt.Errorf("%s contains unsupported field %q", label, key)
		}
	}
	return object, nil
}

func requiredRiftInteger(fields map[string]json.RawMessage, key string) (int64, error) {
	raw, exists := fields[key]
	if !exists {
		return 0, fmt.Errorf("CRA field %s is required", key)
	}
	return decodeRiftInteger(raw, key)
}

func decodeRiftInteger(raw json.RawMessage, label string) (int64, error) {
	if len(raw) == 0 || strings.ContainsAny(string(raw), ".eE") {
		return 0, fmt.Errorf("CRA field %s must be an integer", label)
	}
	var value int64
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, fmt.Errorf("CRA field %s must be an integer", label)
	}
	return value, nil
}

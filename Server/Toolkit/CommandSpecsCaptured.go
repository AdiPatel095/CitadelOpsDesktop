package Toolkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"

	"CitadelDesktop/Server/GameCommands"
)

type capturedFieldKind uint8

const (
	capturedInteger capturedFieldKind = iota + 1
	capturedString
	capturedNull
	capturedIntegerArray
	capturedIntegerPairArray
	capturedObject
)

type capturedFieldDefinition struct {
	Name        string
	Description string
	Kind        capturedFieldKind
	Fields      []capturedFieldDefinition
}

type capturedOpcodeDefinition struct {
	Name        string
	Opcode      string
	Description string
	Effect      Effect
	Fields      []capturedFieldDefinition
}

// These captures are deliberately not ordinary game-control tools. LLI can
// contain credentials/tokens; VCK/NCH/PIN are connection-handshake traffic.
var intentionallyUnexposedOutboundExamples = map[string]string{
	"lli": "login payload contains authentication material",
	"nch": "session cryptographic handshake",
	"pin": "connection heartbeat owned by the websocket session",
	"vck": "client-version handshake owned by the websocket session",
}

func registerCapturedOpcodeCommands(builder *commandSpecBuilder) error {
	for _, definition := range capturedOpcodeDefinitions() {
		definition := definition
		if err := builder.add(
			definition.Name,
			definition.Opcode,
			definition.Description,
			definition.Effect,
			capturedObjectSchema(definition.Fields),
			func(raw json.RawMessage) ([]string, error) {
				if err := validateCapturedObject(raw, definition.Fields, definition.Name); err != nil {
					return nil, err
				}
				payload, err := GameCommands.EmpireExObjectPayload(definition.Opcode, raw)
				if err != nil {
					return nil, toolError("command_build_failed", "%s: %v", definition.Name, err)
				}
				return onePayload(payload), nil
			},
		); err != nil {
			return err
		}
	}
	return nil
}

func capturedOpcodeDefinitions() []capturedOpcodeDefinition {
	query := EffectGameQuery
	action := EffectGameAction
	unsafe := EffectDestructive
	external := EffectExternal
	return []capturedOpcodeDefinition{
		capturedDefinition("abpi", unsafe, "Apply the capture-mapped ABPI building/item operation with explicit castle, object, item, and material fields.",
			capturedInt("AID", "Castle instance AID."),
			capturedInt("KID", "Kingdom KID."),
			capturedObjectField("MS", "Captured material-selection object.",
				capturedInt("F", "Food selection amount."),
				capturedInt("HONEY", "Honey selection amount."),
			),
			capturedInt("OID", "Castle object OID."),
			capturedObjectField("PA", "Captured payment-allocation object.",
				capturedInt("MEAD", "Mead payment amount."),
			),
			capturedInt("WID", "Definition WOD ID."),
		),
		capturedDefinition("acm", external, "Send a capture-mapped alliance chat message. External communication requires its own authorization.",
			capturedStringField("M", "Alliance chat message text."),
		),
		capturedDefinition("aha", action, "Submit the capture-mapped AHA kingdom action.",
			capturedInt("KID", "Kingdom KID."),
		),
		capturedDefinition("ahr", action, "Submit the capture-mapped AHR reward/action selection.",
			capturedInt("ID", "Captured action identifier."),
			capturedInt("T", "Captured action type."),
		),
		capturedDefinition("ama", unsafe, "Execute the capture-mapped AMA alliance operation for an explicit alliance AID.",
			capturedInt("AID", "Alliance ID."),
		),
		capturedDefinition("aqs", action, "Submit the capture-mapped AQS event/quest selection.",
			capturedInt("EID", "Event/entity ID."),
		),
		capturedEmptyDefinition("alb", query, "Request the capture-mapped ALB state."),
		capturedEmptyDefinition("all", query, "Request the capture-mapped ALL state."),
		capturedEmptyDefinition("asc", query, "Request the capture-mapped ASC state."),
		capturedEmptyDefinition("bse", query, "Request the capture-mapped BSE state."),
		capturedDefinition("bss", unsafe, "Execute the capture-mapped BSS entity operation.",
			capturedInt("ID", "Captured entity ID."),
		),
		capturedDefinition("cat", unsafe, "Dispatch the capture-mapped CAT troop movement with every observed routing field explicit.",
			capturedPairs("A", "Troops as [unitWodId, amount] pairs."),
			capturedInt("BPC", "Captured premium-cost flag."),
			capturedInt("HBW", "Movement booster field."),
			capturedInt("KID", "Kingdom KID."),
			capturedInt("LID", "Leader/commander identifier."),
			capturedInt("PTT", "Paid travel-time flag."),
			capturedInt("SD", "Captured movement delay flag."),
			capturedInt("SX", "Source X coordinate."),
			capturedInt("SY", "Source Y coordinate."),
			capturedInt("TX", "Target X coordinate."),
			capturedInt("TY", "Target Y coordinate."),
			capturedInt("WT", "Wait time."),
		),
		capturedDefinition("clb", query, "Request the capture-mapped CLB list/page selection.",
			capturedNullField("I", "Captured null filter."),
			capturedInt("ID", "Captured page or entity identifier."),
			capturedStringField("SP", "Captured scope selector."),
		),
		capturedDefinition("core_osd", query, "Request capture-mapped object-state details by OID.",
			capturedInt("OID", "Object definition or instance OID."),
		),
		capturedDefinition("crca", unsafe, "Cancel or mutate a captured sovereign-crafting queue slot.",
			capturedInt("KID", "Kingdom KID."),
			capturedInt("AID", "Castle instance AID."),
			capturedInt("OID", "Crafting building OID."),
			capturedInt("S", "Queue slot index."),
			capturedStringField("ST", "Slot family such as queue."),
		),
		capturedEmptyDefinition("csp", query, "Request the capture-mapped CSP state."),
		capturedDefinition("ctr", action, "Submit the capture-mapped CTR collection/action request.",
			capturedInt("TRQ", "Captured request selector."),
		),
		capturedDefinition("dci", unsafe, "Execute the capture-mapped DCI construction/item transaction.",
			capturedInt("AID", "Castle instance AID."),
			capturedInt("AMT", "Transaction amount."),
			capturedInt("CID", "Captured item/definition ID."),
			capturedInt("KID", "Kingdom KID."),
			capturedInt("LFID", "Captured look/feature identifier."),
		),
		capturedDefinition("dfc", unsafe, "Execute the capture-mapped DFC castle-coordinate operation.",
			capturedInt("AID", "Castle instance AID."),
			capturedInt("CX", "Castle X coordinate."),
			capturedInt("CY", "Castle Y coordinate."),
			capturedInt("KID", "Kingdom KID."),
		),
		capturedDefinition("dge", unsafe, "Execute the capture-mapped DGE inventory transaction.",
			capturedInt("AMT", "Transaction amount."),
			capturedInt("EID", "Entity or equipment ID."),
			capturedInt("OB", "Captured operation flag."),
		),
		capturedDefinition("dms", unsafe, "Delete or dismiss the capture-mapped message by MID.",
			capturedInt("MID", "Message ID."),
		),
		capturedDefinition("dup", unsafe, "Execute the capture-mapped DUP definition upgrade/operation.",
			capturedInt("A", "Captured amount or object identifier."),
			capturedInt("S", "Captured slot selector."),
			capturedInt("WID", "Definition WOD ID."),
		),
		capturedEmptyDefinition("esl", query, "Request the capture-mapped ESL state."),
		capturedDefinition("eup", unsafe, "Upgrade the captured castle object with explicit power and public-order fields.",
			capturedInt("OID", "Castle object OID."),
			capturedInt("PWR", "Captured power field."),
			capturedInt("PO", "Captured public-order field."),
		),
		capturedDefinition("ffi", unsafe, "Execute the capture-mapped FFI operation for explicit feature IDs.",
			capturedInts("FIDS", "Feature IDs."),
		),
		capturedEmptyDefinition("gap", query, "Request the capture-mapped GAP state."),
		capturedEmptyDefinition("gbl", query, "Request the capture-mapped GBL state."),
		capturedEmptyDefinition("gcc", query, "Request the capture-mapped GCC state."),
		capturedEmptyDefinition("gcs", query, "Request the capture-mapped GCS state."),
		capturedEmptyDefinition("gdti", query, "Request the capture-mapped GDTI state."),
		capturedEmptyDefinition("gie", query, "Request the capture-mapped GIE state."),
		capturedEmptyDefinition("gls", query, "Request the capture-mapped GLS state."),
		capturedDefinition("glt", query, "Request the capture-mapped GLT state for a selected GST.",
			capturedInt("GST", "Captured state/list type."),
		),
		capturedEmptyDefinition("gpa", query, "Request the capture-mapped GPA player/account state."),
		capturedEmptyDefinition("gpr", query, "Request the capture-mapped GPR state."),
		capturedDefinition("grc", query, "Request capture-mapped castle/resource state by castle and kingdom.",
			capturedInt("AID", "Castle instance AID."),
			capturedInt("KID", "Kingdom KID."),
		),
		capturedEmptyDefinition("gui", query, "Request the capture-mapped GUI state."),
		capturedDefinition("hfl", unsafe, "Execute the capture-mapped HFL castle operation.",
			capturedInt("AID", "Castle instance AID."),
			capturedInt("HRF", "Captured operation selector."),
			capturedInt("KID", "Kingdom KID."),
		),
		capturedDefinition("hgh", query, "Request the capture-mapped HGH leaderboard/state slice.",
			capturedInt("LID", "List identifier."),
			capturedInt("LT", "List type."),
			capturedStringField("SV", "Captured selector value."),
		),
		capturedEmptyDefinition("irc", query, "Request the capture-mapped IRC state."),
		capturedEmptyDefinition("klh", query, "Request the capture-mapped KLH state."),
		capturedEmptyDefinition("kli", query, "Request the capture-mapped KLI state."),
		capturedDefinition("kss", action, "Apply the capture-mapped KSS kingdom settings flags.",
			capturedInt("KLS", "Captured KLS flag."),
			capturedInt("KLSE", "Captured KLSE flag."),
		),
		capturedEmptyDefinition("laa", query, "Request the capture-mapped LAA state."),
		capturedDefinition("llsw", action, "Switch the capture-mapped leader/loadout selection.",
			capturedInt("LID", "Leader/loadout identifier."),
			capturedInt("LT", "Leader/loadout type."),
			capturedInt("M", "Captured mode."),
			capturedStringField("SI", "Captured selection identifier."),
		),
		capturedDefinition("mbs", unsafe, "Execute the capture-mapped MBS slot operation.",
			capturedInt("SID", "Slot identifier."),
		),
		capturedDefinition("mec", unsafe, "Execute the capture-mapped MEC castle operation.",
			capturedInt("AID", "Castle instance AID."),
		),
		capturedDefinition("mcu", unsafe, "Mutate the capture-mapped queue slot selection.",
			capturedInt("LID", "Line identifier."),
			capturedInt("S", "Queue slot index."),
			capturedStringField("ST", "Slot family such as queue."),
		),
		capturedDefinition("mmr", action, "Apply the capture-mapped MMR message state change.",
			capturedInt("MID", "Message ID."),
		),
		capturedDefinition("mpe", query, "Request the capture-mapped MPE message/page selection.",
			capturedInt("MID", "Message or page identifier; captures include -1."),
		),
		capturedDefinition("msb", unsafe, "Apply a captured time-skip item to a castle object.",
			capturedInt("OID", "Castle object OID."),
			capturedStringField("MST", "Time-skip definition such as MS7."),
		),
		capturedDefinition("odc", unsafe, "Execute the capture-mapped ODC object operation.",
			capturedInt("OID", "Object instance OID."),
		),
		capturedDefinition("oop", unsafe, "Execute the capture-mapped object operation with explicit cost and option fields.",
			capturedInt("C", "Captured operation count/flag."),
			capturedInt("CC2T", "Captured currency-two cost."),
			capturedInts("ODI", "Captured option/detail identifiers."),
			capturedInt("OID", "Object instance OID."),
			capturedStringField("cmdID", "Nested command identifier; captured value is oop."),
		),
		capturedDefinition("pep", unsafe, "Execute the capture-mapped PEP event/entity operation.",
			capturedInt("EID", "Event/entity identifier."),
		),
		capturedDefinition("rcc", unsafe, "Execute the capture-mapped RCC reset/cancel operation.",
			capturedInt("RT", "Captured reset/cancel type."),
		),
		capturedDefinition("rms", unsafe, "Remove the capture-mapped message by MID.",
			capturedInt("MID", "Message ID."),
		),
		capturedDefinition("qsc", action, "Submit the capture-mapped quest selection/collection request.",
			capturedInt("QID", "Quest ID."),
		),
		capturedEmptyDefinition("sie", query, "Request the capture-mapped SIE state."),
		capturedEmptyDefinition("sii", query, "Request the capture-mapped SII state."),
		capturedEmptyDefinition("sli", query, "Request the capture-mapped SLI state."),
		capturedDefinition("spl", query, "Request production-slot state for a selected line LID.",
			capturedInt("LID", "Production line ID."),
		),
		capturedDefinition("sti", query, "Request captured source-to-target travel information.",
			capturedInt("KID", "Kingdom KID."),
			capturedInt("SX", "Source X coordinate."),
			capturedInt("SY", "Source Y coordinate."),
			capturedInt("TX", "Target X coordinate."),
			capturedInt("TY", "Target Y coordinate."),
		),
		capturedEmptyDefinition("tsh", query, "Request the capture-mapped TSH state."),
		capturedDefinition("txc", action, "Collect or apply the capture-mapped tax-cycle result.",
			capturedInt("TR", "Captured tax/result identifier."),
		),
		capturedEmptyDefinition("txi", query, "Request capture-mapped tax information."),
		capturedDefinition("txs", action, "Start the capture-mapped tax cycle with explicit type and duration fields.",
			capturedInt("TT", "Captured tax time/type."),
			capturedInt("TX", "Captured tax option."),
		),
		capturedEmptyDefinition("upt", query, "Request the capture-mapped UPT state."),
		capturedEmptyDefinition("usg", query, "Request the capture-mapped USG state."),
		capturedEmptyDefinition("utc", query, "Request the capture-mapped UTC state."),
		capturedDefinition("vln", unsafe, "Change the capture-mapped player display name. This persistent account mutation requires destructive authorization.",
			capturedStringField("NOM", "New player display name."),
		),
	}
}

func capturedDefinition(name string, effect Effect, description string, fields ...capturedFieldDefinition) capturedOpcodeDefinition {
	return capturedOpcodeDefinition{Name: name, Opcode: name, Description: description, Effect: effect, Fields: fields}
}

func capturedEmptyDefinition(name string, effect Effect, description string) capturedOpcodeDefinition {
	return capturedDefinition(name, effect, description)
}

func capturedInt(name, description string) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedInteger}
}

func capturedStringField(name, description string) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedString}
}

func capturedNullField(name, description string) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedNull}
}

func capturedInts(name, description string) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedIntegerArray}
}

func capturedPairs(name, description string) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedIntegerPairArray}
}

func capturedObjectField(name, description string, fields ...capturedFieldDefinition) capturedFieldDefinition {
	return capturedFieldDefinition{Name: name, Description: description, Kind: capturedObject, Fields: fields}
}

func capturedObjectSchema(fields []capturedFieldDefinition) json.RawMessage {
	properties := make(map[string]interface{}, len(fields))
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		properties[field.Name] = capturedFieldSchema(field)
		required = append(required, field.Name)
	}
	sort.Strings(required)
	return objectSchema(properties, required...)
}

func capturedFieldSchema(field capturedFieldDefinition) map[string]interface{} {
	switch field.Kind {
	case capturedInteger:
		return schemaProperty("integer", field.Description)
	case capturedString:
		return schemaProperty("string", field.Description)
	case capturedNull:
		return schemaProperty("null", field.Description)
	case capturedIntegerArray:
		return map[string]interface{}{
			"type":        "array",
			"description": field.Description,
			"items":       schemaProperty("integer", "Captured integer value."),
		}
	case capturedIntegerPairArray:
		return map[string]interface{}{
			"type":        "array",
			"description": field.Description,
			"items": map[string]interface{}{
				"type":     "array",
				"items":    schemaProperty("integer", "Pair value."),
				"minItems": 2,
				"maxItems": 2,
			},
		}
	case capturedObject:
		properties := make(map[string]interface{}, len(field.Fields))
		required := make([]string, 0, len(field.Fields))
		for _, child := range field.Fields {
			properties[child.Name] = capturedFieldSchema(child)
			required = append(required, child.Name)
		}
		sort.Strings(required)
		return map[string]interface{}{
			"type":                 "object",
			"description":          field.Description,
			"properties":           properties,
			"required":             required,
			"additionalProperties": false,
		}
	default:
		panic(fmt.Sprintf("unsupported captured field kind %d", field.Kind))
	}
}

func validateCapturedObject(raw json.RawMessage, fields []capturedFieldDefinition, command string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return toolError("invalid_arguments", "%s arguments must be a JSON object", command)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &object); err != nil {
		return toolError("invalid_arguments", "%s arguments must be a JSON object: %v", command, err)
	}
	known := make(map[string]capturedFieldDefinition, len(fields))
	for _, field := range fields {
		known[field.Name] = field
		value, ok := object[field.Name]
		if !ok {
			return toolError("invalid_arguments", "%s is missing required wire field %q", command, field.Name)
		}
		if err := validateCapturedField(value, field, command+"."+field.Name); err != nil {
			return err
		}
	}
	for name := range object {
		if _, ok := known[name]; !ok {
			return toolError("invalid_arguments", "%s contains unknown wire field %q", command, name)
		}
	}
	return nil
}

func validateCapturedField(raw json.RawMessage, field capturedFieldDefinition, path string) error {
	switch field.Kind {
	case capturedInteger:
		var value int64
		if err := json.Unmarshal(raw, &value); err != nil {
			return toolError("invalid_arguments", "%s must be an integer", path)
		}
	case capturedString:
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return toolError("invalid_arguments", "%s must be a string", path)
		}
	case capturedNull:
		if !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return toolError("invalid_arguments", "%s must be null", path)
		}
	case capturedIntegerArray:
		var values []int64
		if err := json.Unmarshal(raw, &values); err != nil {
			return toolError("invalid_arguments", "%s must be an integer array", path)
		}
	case capturedIntegerPairArray:
		var pairs [][]int64
		if err := json.Unmarshal(raw, &pairs); err != nil {
			return toolError("invalid_arguments", "%s must be an array of integer pairs", path)
		}
		for index, pair := range pairs {
			if len(pair) != 2 {
				return toolError("invalid_arguments", "%s[%d] must contain exactly two integers", path, index)
			}
		}
	case capturedObject:
		if err := validateCapturedObject(raw, field.Fields, path); err != nil {
			return err
		}
	default:
		return toolError("invalid_arguments", "%s has an unsupported captured field type", path)
	}
	return nil
}

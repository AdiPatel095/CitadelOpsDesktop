package GameCommands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// EmpireExObjectPayload builds an EmpireEx frame for a validated JSON-object body.
// It is the shared builder for capture-mapped opcodes that do not yet warrant a
// dedicated semantic helper. Callers should expose only fixed, reviewed opcodes.
func EmpireExObjectPayload(opcode string, body json.RawMessage) (string, error) {
	opcode = strings.TrimSpace(opcode)
	if !validCapturedOpcode(opcode) {
		return "", fmt.Errorf("invalid EmpireEx opcode %q", opcode)
	}
	body = bytes.TrimSpace(body)
	if len(body) == 0 || !json.Valid(body) || body[0] != '{' {
		return "", fmt.Errorf("EmpireEx %s body must be a JSON object", opcode)
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(body, &object); err != nil {
		return "", fmt.Errorf("decode EmpireEx %s body: %w", opcode, err)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, body); err != nil {
		return "", fmt.Errorf("compact EmpireEx %s body: %w", opcode, err)
	}
	return empireExFrame(opcode, compact.String()), nil
}

func validCapturedOpcode(opcode string) bool {
	if len(opcode) == 0 || len(opcode) > 32 {
		return false
	}
	for _, r := range opcode {
		if (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '_' {
			return false
		}
	}
	return true
}

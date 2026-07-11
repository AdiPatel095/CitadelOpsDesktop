package GameCommands

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestEmpireExObjectPayload(t *testing.T) {
	payload, err := EmpireExObjectPayload("spl", json.RawMessage(`{ "LID": 0 }`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(payload, `%spl%1%{"LID":0}%`) {
		t.Fatalf("payload=%q", payload)
	}
}

func TestEmpireExObjectPayloadRejectsUnsafeShape(t *testing.T) {
	for _, test := range []struct {
		opcode string
		body   string
	}{
		{opcode: "SPL", body: `{}`},
		{opcode: "spl%1", body: `{}`},
		{opcode: "spl", body: `[]`},
		{opcode: "spl", body: `{`},
	} {
		if _, err := EmpireExObjectPayload(test.opcode, json.RawMessage(test.body)); err == nil {
			t.Errorf("EmpireExObjectPayload(%q, %q) succeeded", test.opcode, test.body)
		}
	}
}

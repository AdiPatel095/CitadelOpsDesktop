package Protocol

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestEncodeBareCommand(t *testing.T) {
	payload, err := Encode(Command{Namespace: "EmpireEx_21", Opcode: "sin", Bare: true})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "%xt%EmpireEx_21%sin%1%" {
		t.Fatalf("unexpected bare command: %s", payload)
	}
}

func TestEncodeRoutedLegacyCommand(t *testing.T) {
	payload, err := Encode(Command{
		Namespace: "EmpireEx_21", Opcode: "legacy", Route: "0", Payload: []byte(`[]`), OmitNamespace: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "%xt%legacy%1%0%[]%" {
		t.Fatalf("unexpected routed command: %s", payload)
	}
}

func TestEncodeRejectsOversizedSupport(t *testing.T) {
	for _, count := range []int{10, 11, 20, 26} {
		army := make([][2]int64, count)
		for i := range army {
			army[i] = [2]int64{int64(i + 1), 1000000}
		}
		payload, _ := json.Marshal(map[string]any{"A": army})
		encoded, err := Encode(Command{Opcode: "cds", Payload: payload})
		if count <= 10 && err != nil {
			t.Fatal(err)
		}
		if count > 10 && (err == nil || len(encoded) > 0) {
			t.Fatalf("encoded oversized CDS with %d types", count)
		}
	}
}

func TestDecodeEmpireExNamespaceAndLegacyResponses(t *testing.T) {
	for _, tc := range []struct{ raw, namespace, opcode, route, text, payload, payloadText string }{
		{`%xt%EmpireEx%cra%1%{"note":"50% done"}%`, "EmpireEx", "cra", "cra", "1", `{"note":"50% done"}`, ""},
		{`%xt%EmpireEx_21%cra%1%{}%`, "EmpireEx_21", "cra", "cra", "1", `{}`, ""},
		{`%xt%EmpireEx%sin%1%`, "EmpireEx", "sin", "sin", "1", "", ""},
		{`%xt%EmpireEx%ain%1%{}%`, "EmpireEx", "ain", "ain", "1", `{}`, ""},
		{`%xt%cra%1%0%{"MID":123}%`, "", "cra", "1", "0", `{"MID":123}`, ""},
		{`%xt%cra%1%90%{}%`, "", "cra", "1", "90", `{}`, ""},
		{`%xt%EmpireExFoo%1%90%{}%`, "", "empireexfoo", "1", "90", `{}`, ""},
		{`%xt%EmpireEx%cra%bad%not-json%`, "EmpireEx", "cra", "cra", "bad", "", "not-json"},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			frame, err := Decode(tc.raw, DirectionInbound, time.Time{})
			if err != nil || frame.Namespace != tc.namespace || frame.Opcode != tc.opcode || frame.Route != tc.route || frame.ResponseText != tc.text || string(frame.Payload) != tc.payload || frame.PayloadText != tc.payloadText {
				t.Fatalf("frame=%+v err=%v", frame, err)
			}
			if tc.text == "bad" {
				if frame.ResponseCode != nil {
					t.Fatal("nonnumeric response parsed")
				}
			} else if frame.ResponseCode == nil || strconv.Itoa(*frame.ResponseCode) != tc.text {
				t.Fatalf("response=%v", frame.ResponseCode)
			}
		})
	}
	for _, raw := range []string{"bad", `%xt%EmpireEx%%1%{}%`, `%%EmpireEx%cra%1%{}%`} {
		if _, err := Decode(raw, DirectionInbound, time.Time{}); err == nil {
			t.Fatalf("accepted malformed frame %q", raw)
		}
	}
}

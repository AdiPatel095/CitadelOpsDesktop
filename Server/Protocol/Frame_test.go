package Protocol

import (
	"encoding/json"
	"testing"
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

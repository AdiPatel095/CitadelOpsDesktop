package Protocol

import "testing"

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
	payload, err := Encode(Command{Opcode: "aec", Route: "0", Payload: []byte(`[]`)})
	if err != nil {
		t.Fatal(err)
	}
	if string(payload) != "%xt%aec%1%0%[]%" {
		t.Fatalf("unexpected routed command: %s", payload)
	}
}

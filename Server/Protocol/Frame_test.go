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

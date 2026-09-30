package Intent

import (
	"encoding/json"
	"reflect"
	"testing"

	"CitadelDesktop/Server/Protocol"
)

func TestEstimatedReceiptBytes(t *testing.T) {
	for _, test := range []struct {
		name    string
		receipt Receipt
		want    int
	}{
		{"baseline", Receipt{}, 1024},
		{"plan", Receipt{Plan: &Plan{}}, 1536},
		{"plan summary", Receipt{Plan: &Plan{Summary: "summary"}}, 1543},
		{"step", Receipt{Plan: &Plan{Steps: []Step{{}}}}, 1792},
		{"exchange", Receipt{Exchanges: []CommandExchange{{}}}, 1280},
		{"exchange command", Receipt{Exchanges: []CommandExchange{{Command: Protocol.Command{Payload: json.RawMessage(`{}`)}}}}, 1282},
		{"response", Receipt{Exchanges: []CommandExchange{{Response: &Protocol.Frame{}}}}, 1536},
		{"response payload", Receipt{Exchanges: []CommandExchange{{Response: &Protocol.Frame{Payload: json.RawMessage(`{}`)}}}}, 1538},
		{"response text", Receipt{Exchanges: []CommandExchange{{Response: &Protocol.Frame{PayloadText: "text"}}}}, 1540},
		{"response raw", Receipt{Exchanges: []CommandExchange{{Response: &Protocol.Frame{Raw: "raw"}}}}, 1539},
		{"evidence", Receipt{Evidence: []OperationEvidence{{}}}, 1088},
		{"evidence kind", Receipt{Evidence: []OperationEvidence{{Kind: "kind"}}}, 1092},
		{"evidence data", Receipt{Evidence: []OperationEvidence{{Data: json.RawMessage(`{}`)}}}, 1090},
		{"failure", Receipt{Failure: &FailurePresentation{}}, 1536},
		{"recursive batch", Receipt{Plan: &Plan{Steps: []Step{{Payload: json.RawMessage(`{}`), Batch: []Step{
			{ActionArguments: json.RawMessage(`{}`), Batch: []Step{{Command: Protocol.Command{Payload: json.RawMessage(`{}`)}}}},
			{ResolverArguments: json.RawMessage(`{}`)},
		}}}}}, 2568},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := estimatedReceiptBytes(test.receipt); got != test.want {
				t.Fatalf("estimated bytes = %d, want %d", got, test.want)
			}
			if allocations := testing.AllocsPerRun(10, func() { estimatedReceiptBytes(test.receipt) }); allocations != 0 {
				t.Fatalf("estimate allocated %g times", allocations)
			}
		})
	}
	for _, field := range []string{"ID", "Intent", "Actor", "Error", "RawError"} {
		t.Run(field, func(t *testing.T) {
			var receipt Receipt
			reflect.ValueOf(&receipt).Elem().FieldByName(field).SetString("abc")
			if got := estimatedReceiptBytes(receipt); got != 1027 {
				t.Fatalf("%s estimate = %d, want 1027", field, got)
			}
		})
	}
	for _, field := range []string{"Payload", "ActionArguments", "ResolverArguments", "ExpectedResponsePayload",
		"PreDispatchArguments", "FinalDispatchArguments", "DefinitiveSendFailureArguments", "DefinitiveResponseFailureArguments", "Command.Payload"} {
		t.Run("step "+field, func(t *testing.T) {
			step := Step{}
			if field == "Command.Payload" {
				step.Command.Payload = json.RawMessage(`{}`)
			} else {
				reflect.ValueOf(&step).Elem().FieldByName(field).SetBytes([]byte(`{}`))
			}
			if got := estimatedReceiptBytes(Receipt{Plan: &Plan{Steps: []Step{step}}}); got != 1794 {
				t.Fatalf("%s estimate = %d, want 1794", field, got)
			}
		})
	}
}

package GameParser

import (
	"strings"
	"testing"

	"CitadelDesktop/Server/Automation"
)

func TestMessageRouterCompletesTraceAfterStateObservation(t *testing.T) {
	previous := Automation.CommandTraces
	Automation.CommandTraces = Automation.NewCommandTraceTracker(8)
	defer func() { Automation.CommandTraces = previous }()

	command := Automation.Command{
		ID:      77,
		Owner:   Automation.OwnerToolkit,
		Builder: "test_builder",
		Intent:  "test_intent",
		Payload: []byte(`%xt%EmpireEx_21%zzq%1%{"ID":101}%`),
	}
	Automation.CommandTraces.RecordQueued(command)
	Automation.CommandTraces.RecordSent(command)
	MessageRouter(strings.Split(`%xt%EmpireEx_21%zzq%0%{"OK":true}%`, "%"))

	traces := Automation.CommandTraces.Recent(Automation.CommandTraceFilter{Opcode: "zzq", Limit: 1})
	if len(traces) != 1 {
		t.Fatalf("trace count=%d, want 1", len(traces))
	}
	trace := traces[0]
	if trace.Status != Automation.CommandTraceResponded || trace.ResponseShape != "object{OK:boolean}" {
		t.Fatalf("router response trace=%+v", trace)
	}
	if !containsTraceState(trace.StateChanges, Automation.StateOpcode("zzq")) {
		t.Fatalf("state observation was not captured before trace completion: %v", trace.StateChanges)
	}
}

func containsTraceState(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

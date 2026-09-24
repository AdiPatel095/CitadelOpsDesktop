package Automation

import (
	"CitadelDesktop/Server/Intent"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

type autoStationTraceRecorder struct{ entries []string }

func (*autoStationTraceRecorder) AttackLaunchCountsSince(time.Time, time.Time) (map[string]int, bool) {
	return nil, false
}

func (recorder *autoStationTraceRecorder) RecordFeature(channel, event, detail string) {
	recorder.entries = append(recorder.entries, channel+" "+event+" "+detail)
}

func TestAutoStationDecisionTraceIsBoundedAndPrivate(t *testing.T) {
	recorder := &autoStationTraceRecorder{}
	coordinator := &Coordinator{telemetry: recorder}
	deadline := time.Date(2026, 9, 24, 18, 32, 28, 0, time.UTC)
	decision := Decision{
		Status: "threat", Detail: "private castle SecretName at 1,2; window opens soon",
		NextCheckAt: deadline, Metrics: map[string]float64{"threatCount": 2, "nextImpactUnixMs": 1790274928000},
	}
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	if len(recorder.entries) != 1 || !strings.Contains(recorder.entries[0], "threats=2") ||
		!strings.Contains(recorder.entries[0], "nextCheckAt=2026-09-24T18:32:28Z") {
		t.Fatalf("decision trace = %v", recorder.entries)
	}
	decision.Detail = "Private threat retry"
	decision.Request = &Intent.Request{Name: "map.query"}
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	decision.NextCheckAt = deadline.Add(time.Second)
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	if len(recorder.entries) != 2 || !strings.Contains(recorder.entries[1], "nextCheckAt=2026-09-24T18:32:28Z") {
		t.Fatalf("rolling retry produced noisy trace: %v", recorder.entries)
	}
	decision.Metrics["nextImpactUnixMs"]++
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	if len(recorder.entries) != 3 {
		t.Fatalf("changed impact was not traced: %v", recorder.entries)
	}
	decision.Status = "refreshing"
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	if len(recorder.entries) != 4 || !strings.Contains(recorder.entries[3], "status=refreshing") {
		t.Fatalf("precheck refresh was not traced: %v", recorder.entries)
	}
	decision.Status = "running"
	decision.Request = &Intent.Request{Name: "defense.open_gate", Arguments: json.RawMessage(`{"secret":"private"}`)}
	coordinator.traceAutoStationDecision("autoStation", true, decision)
	coordinator.traceAutoStationDecision("other", true, decision)
	coordinator.traceAutoStationDecision("autoStation", false, Decision{Status: "gated", Detail: "private lock"}, "safety_lock")
	for _, reason := range []string{"troop_gate", "coin_gate", "failure_pause"} {
		coordinator.traceAutoStationDecision("autoStation", true, Decision{Status: "gated", Detail: "private blocker"}, reason)
	}
	if len(recorder.entries) != 9 || !strings.Contains(recorder.entries[4], "intent=defense.open_gate") ||
		!strings.Contains(recorder.entries[5], "reason=safety_lock") {
		t.Fatalf("trace transitions = %v", recorder.entries)
	}
	for index, reason := range []string{"troop_gate", "coin_gate", "failure_pause"} {
		if !strings.Contains(recorder.entries[6+index], "reason="+reason) {
			t.Fatalf("missing %s trace: %v", reason, recorder.entries)
		}
	}
	for _, entry := range recorder.entries {
		if strings.Contains(entry, "SecretName") || strings.Contains(entry, "private") || strings.Contains(entry, "1,2") {
			t.Fatalf("private value leaked into trace: %q", entry)
		}
	}
}

func TestAutoStationReceiptTraceExcludesErrorsAndIdentifiers(t *testing.T) {
	recorder := &autoStationTraceRecorder{}
	coordinator := &Coordinator{telemetry: recorder}
	coordinator.traceAutoStationReceipt(operationResult{
		policyID:        "autoStation",
		receipt:         Intent.Receipt{ID: "private-id", Intent: "alliance.refresh", Status: Intent.StatusFailed, Error: "private-error"},
		failureFallback: &Intent.Receipt{Intent: "defense.open_gate", Status: Intent.StatusSucceeded},
	})
	if len(recorder.entries) != 1 || !strings.Contains(recorder.entries[0], "intent=alliance.refresh status=failed") ||
		!strings.Contains(recorder.entries[0], "fallback=defense.open_gate status=succeeded") ||
		strings.Contains(recorder.entries[0], "private") {
		t.Fatalf("receipt trace = %v", recorder.entries)
	}
}

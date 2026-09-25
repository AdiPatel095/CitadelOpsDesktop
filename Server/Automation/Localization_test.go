package Automation

import (
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Localization"
	"CitadelDesktop/Server/State"
	"encoding/json"
	"testing"
	"time"
)

func TestPresentationDescriptorsDoNotChangeDecisionFingerprints(t *testing.T) {
	original := Decision{Status: "ready", Detail: "Buy two units", Request: &Intent.Request{Name: "buy", Arguments: json.RawMessage(`{"amount":2}`)}}
	localized := original
	localized.DetailDescriptor = Localization.New("buy", "Buy {count} units", Localization.Params{"count": 2})
	localized.FailureDetailDescriptor = Localization.New("failed", "Failed", nil)
	if decisionRequestFingerprint(original) != decisionRequestFingerprint(localized) {
		t.Fatal("presentation changed dispatch identity")
	}
	before, ok := passiveDecisionFingerprint(original)
	after, otherOK := passiveDecisionFingerprint(localized)
	if !ok || !otherOK || before != after {
		t.Fatal("presentation changed passive policy scheduling")
	}
}

func TestActiveSafetyLockDiscardsFreshDecisionDescriptors(t *testing.T) {
	initial := coordinatorReadyState()
	lock := State.AutomationSafetyLock{Lane: "lane", Opcode: "cra", Code: 256, OperationID: "incident", ObservedAt: time.Now().UTC()}
	initial.Automations["lane"] = State.AutomationState{ID: "lane", SafetyLock: lock}
	store := State.NewStore(initial)
	coordinator := NewCoordinator(store, openCoordinatorTestConfiguration(t, "lane"), nil, nil)
	coordinator.recordDecision("lane", false, Decision{Status: "disabled", Detail: "Disabled", DetailDescriptor: Localization.New("disabled", "Disabled", nil)})
	current := store.ReadOnlyView().Automations["lane"]
	if current.Status != "gated" || current.Detail != lock.Detail() || current.LastError != lock.Detail() {
		t.Fatalf("lock presentation changed: %#v", current)
	}
	if current.DetailDescriptor == nil || current.LastErrorDescriptor == nil || current.DetailDescriptor.Key != "server.state.safety_lock.until" || current.LastErrorDescriptor.Key != "server.state.safety_lock.until" {
		t.Fatalf("fresh decision concealed lock: %#v", current)
	}
	coordinator.updateAutomation("lane", func(next State.AutomationState) State.AutomationState {
		next.LastError = "Ready"
		next.LastErrorDescriptor = Localization.New("ready", "Ready", nil)
		return next
	})
	if current = store.ReadOnlyView().Automations["lane"]; current.LastErrorDescriptor == nil || current.LastErrorDescriptor.Key != "server.state.safety_lock.until" {
		t.Fatal("fresh error descriptor concealed lock")
	}
}

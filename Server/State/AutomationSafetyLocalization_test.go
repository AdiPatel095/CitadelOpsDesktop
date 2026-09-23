package State

import (
	"CitadelDesktop/Server/Localization"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestSafetyLockDescriptorPreservesEvidenceAndDeadline(t *testing.T) {
	lock := AutomationSafetyLock{Opcode: "cra", Code: 256, OperationID: "operation{raw}", ObservedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)}
	descriptor := lock.DetailDescriptor()
	if descriptor.Key != "server.state.safety_lock.until" || descriptor.Params["opcode"] != "CRA" || descriptor.Params["code"] != "256" || descriptor.Params["operation"] != nil || descriptor.Params["until"] != "2026-09-20T10:30:00Z" || descriptor.FallbackText != lock.Detail() {
		t.Fatalf("timed evidence changed: %#v", descriptor)
	}
	lock.ObservedAt = time.Time{}
	descriptor = lock.DetailDescriptor()
	if descriptor.Key != "server.state.safety_lock.review" || descriptor.FallbackText != lock.Detail() {
		t.Fatalf("indefinite review instruction lost: %#v", descriptor)
	}
	if _, exists := descriptor.Params["until"]; exists {
		t.Fatal("indefinite lock invented deadline")
	}
}

func TestSafetyLockReasonBindingPersistenceAndClones(t *testing.T) {
	lock := AutomationSafetyLock{Opcode: "cra", Code: 12345, OperationID: "private-operation", Meaning: "Official explanation", MeaningDescriptor: Localization.Bind(Localization.Official("errorCode_12345", "Official explanation"), "Official explanation")}
	message := lock.DetailDescriptor()
	if message == nil || message.Params["code"] != "12345" || message.ListParams["reason"][0].OfficialKey != "errorCode_12345" {
		t.Fatalf("official identity lost: %#v", message)
	}
	raw, _ := json.Marshal(message)
	if strings.Contains(string(raw), "private-operation") {
		t.Fatal("operation ID leaked")
	}
	data, err := json.Marshal(lock)
	if err != nil {
		t.Fatal(err)
	}
	var restored AutomationSafetyLock
	if err = json.Unmarshal(data, &restored); err != nil {
		t.Fatal(err)
	}
	if restored.DetailDescriptor() == nil || restored.DetailDescriptor().FallbackText != lock.Detail() {
		t.Fatal("persisted binding lost")
	}
	state := NewGameState()
	state.Automations["test"] = AutomationState{SafetyLock: lock}
	store := NewStore(state)
	snapshot := store.Snapshot()
	changed := snapshot.Automations["test"]
	changed.SafetyLock.MeaningDescriptor.OfficialKey = "changed"
	if store.Snapshot().Automations["test"].SafetyLock.MeaningDescriptor.OfficialKey != "errorCode_12345" {
		t.Fatal("snapshot aliases descriptor")
	}
	lock.Context = "unknown historical context"
	if lock.DetailDescriptor() != nil {
		t.Fatal("unknown context concealed")
	}
	lock.ContextDescriptor = Localization.Bind(Localization.New("known", "Known reason", nil), "stale context")
	if lock.DetailDescriptor() != nil {
		t.Fatal("stale descriptor accepted")
	}
	lock.ContextDescriptor = Localization.Bind(Localization.New("known", "Known reason", nil), lock.Context)
	if lock.DetailDescriptor() == nil {
		t.Fatal("exact bound context rejected")
	}
	lock.ContextDescriptor.Context = []*Localization.Message{Localization.New("child", "Nested", nil)}
	if lock.DetailDescriptor() != nil {
		t.Fatal("nested reason accepted")
	}
}

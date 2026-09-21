package State

import (
	"testing"
	"time"
)

func TestSafetyLockDescriptorPreservesEvidenceAndDeadline(t *testing.T) {
	lock := AutomationSafetyLock{Opcode: "cra", Code: 256, OperationID: "operation{raw}", ObservedAt: time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)}
	descriptor := lock.DetailDescriptor()
	if descriptor.Key != "server.state.safety_lock.until" || descriptor.Params["opcode"] != "CRA" || descriptor.Params["code"] != 256 || descriptor.Params["operation"] != lock.OperationID || descriptor.Params["until"] != "2026-09-20T10:30:00Z" || descriptor.FallbackText != lock.Detail() {
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

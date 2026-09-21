package Intent

import (
	"CitadelDesktop/Server/State"
	"strings"
	"testing"
	"time"
)

func TestSafetyLockRecoveryMatchesTimedAndReviewRequiredLock(t *testing.T) {
	for _, timed := range []bool{false, true} {
		lock := State.AutomationSafetyLock{Opcode: "cra", Code: 256, OperationID: "operation"}
		if timed {
			lock.ObservedAt = time.Date(2026, 9, 20, 10, 0, 0, 0, time.UTC)
		}
		before := lock
		presentation := (&Engine{}).failurePresentation(Receipt{}, &LaneLockedError{Lock: lock})
		if lock != before || presentation.SafetyLock == nil || *presentation.SafetyLock != before {
			t.Fatal("presentation changed safety policy")
		}
		if presentation.RecoveryDescriptor == nil || presentation.RecoveryDescriptor.Fallback != presentation.Recovery {
			t.Fatal("recovery descriptor diverged from raw recovery")
		}
		automatic := strings.Contains(presentation.Recovery, "automatically")
		review := strings.Contains(presentation.Recovery, "explicitly clearing")
		if automatic != timed || review == timed {
			t.Fatalf("timed=%v inconsistent recovery: %s", timed, presentation.Recovery)
		}
		if timed && presentation.ExplanationDescriptor.Key != "server.state.safety_lock.until" || !timed && presentation.ExplanationDescriptor.Key != "server.state.safety_lock.review" {
			t.Fatal("explanation and recovery disagree")
		}
	}
}

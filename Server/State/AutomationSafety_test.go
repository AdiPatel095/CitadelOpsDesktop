package State

import (
	"testing"
	"time"
)

func TestSafetyExpiryUsesOriginalObservationForEveryRejection(t *testing.T) {
	now := time.Now().UTC()
	for _, pair := range []struct {
		opcode string
		code   int
	}{{"cra", 256}, {"msd", 311}, {"new", 999}, {"ahr", 274}} {
		for _, until := range []time.Time{{}, now.Add(24 * time.Hour)} {
			lock := AutomationSafetyLock{OperationID: "incident", Opcode: pair.opcode, Code: pair.code, ObservedAt: now, Until: until}
			if !lock.Active(now.Add(30*time.Minute-time.Nanosecond)) || lock.Active(now.Add(30*time.Minute)) || !lock.ExpiresAt().Equal(now.Add(30*time.Minute)) {
				t.Fatalf("wrong TTL for %s %d: %#v", pair.opcode, pair.code, lock)
			}
		}
	}
}

func TestWhitelistedSavedLocksAreAlwaysInactive(t *testing.T) {
	now := time.Now().UTC()
	for _, pair := range []struct {
		opcode string
		code   int
	}{{" ADI ", 95}, {"ere", 227}, {"eqe", 227}, {"bup", 87}, {"ahr", 273}} {
		lock := AutomationSafetyLock{OperationID: "legacy", Opcode: pair.opcode, Code: pair.code, ObservedAt: now, Until: now.Add(time.Hour)}
		if lock.Active(now) {
			t.Fatalf("whitelisted %s/%d active", pair.opcode, pair.code)
		}
	}
}

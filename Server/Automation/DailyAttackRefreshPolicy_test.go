package Automation

import (
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestDailyAttackRefreshPolicyWaitsBelowServerMaximum(t *testing.T) {
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	decision, err := NewDailyAttackRefreshPolicy().Evaluate(t.Context(), Snapshot{
		Now: now,
		State: State.GameState{DailyAttacks: State.DailyAttackState{
			Count: 3499, ServerThreshold: 3500, ObservedAt: now,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate below maximum: %v", err)
	}
	if decision.Request != nil || decision.Status != "idle" ||
		!decision.NextCheckAt.Equal(now.Add(dailyAttackRefreshInterval)) {
		t.Fatalf("below-maximum decision = %#v", decision)
	}
}

func TestDailyAttackRefreshPolicySchedulesFromLastServerUpdate(t *testing.T) {
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	observedAt := now.Add(-14 * time.Minute)
	decision, err := NewDailyAttackRefreshPolicy().Evaluate(t.Context(), Snapshot{
		Now: now,
		State: State.GameState{DailyAttacks: State.DailyAttackState{
			Count: 3500, ServerThreshold: 3500, ObservedAt: observedAt,
		}},
	})
	if err != nil {
		t.Fatalf("evaluate fresh maximum: %v", err)
	}
	if decision.Request != nil || decision.Status != "waiting" ||
		!decision.NextCheckAt.Equal(observedAt.Add(dailyAttackRefreshInterval)) {
		t.Fatalf("fresh-maximum decision = %#v", decision)
	}
}

func TestDailyAttackRefreshPolicyPollsStaleMaximum(t *testing.T) {
	now := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	decision, err := NewDailyAttackRefreshPolicy().Evaluate(t.Context(), Snapshot{
		Now: now,
		State: State.GameState{DailyAttacks: State.DailyAttackState{
			Count: 4000, ServerThreshold: 3500, ObservedAt: now.Add(-dailyAttackRefreshInterval),
		}},
	})
	if err != nil {
		t.Fatalf("evaluate stale maximum: %v", err)
	}
	if decision.Request == nil || decision.Request.Name != "daily_attacks.refresh" ||
		decision.Status != "refreshing" || !decision.NextCheckAt.Equal(now.Add(dailyAttackRefreshInterval)) {
		t.Fatalf("stale-maximum decision = %#v", decision)
	}
}

func TestDailyAttackRefreshPolicyIsCoreWithoutPerAttackWake(t *testing.T) {
	policy := NewDailyAttackRefreshPolicy()
	if _, ok := any(policy).(CorePolicy); !ok {
		t.Fatal("daily attack refresh must be a core policy")
	}
	if _, ok := any(policy).(StateWakePolicy); ok {
		t.Fatal("daily attack refresh must not wake and clone state for every attack-count push")
	}
}

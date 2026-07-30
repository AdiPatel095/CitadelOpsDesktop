package Automation

import (
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestDailyAttackLimitUsesAuthoritativeServerCountAndUnblocksAfterReset(t *testing.T) {
	now := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)
	snapshot := Snapshot{State: State.NewGameState(), Now: now}
	metrics := map[string]float64{}

	if allowance, blocked := dailyAttackLimitAllowance(snapshot, 1000, 30*time.Second, metrics); allowance != 0 || blocked == nil || !strings.Contains(blocked.Detail, "Waiting for the server") {
		t.Fatalf("missing count: allowance=%d blocked=%#v", allowance, blocked)
	}

	snapshot.State.DailyAttacks = State.DailyAttackState{Count: 1000, ServerThreshold: 3500, ObservedAt: now}
	if allowance, blocked := dailyAttackLimitAllowance(snapshot, 1000, 30*time.Second, metrics); allowance != 0 || blocked == nil || !strings.Contains(blocked.Detail, "1000 / 1000") {
		t.Fatalf("reached limit: allowance=%d blocked=%#v", allowance, blocked)
	}

	snapshot.State.DailyAttacks = State.DailyAttackState{Count: 997, ServerThreshold: 3500, ObservedAt: now.Add(time.Minute)}
	allowance, blocked := dailyAttackLimitAllowance(snapshot, 1000, 30*time.Second, metrics)
	if blocked != nil || allowance != 3 || capDailyAttackBatch(5, allowance) != 3 {
		t.Fatalf("reset allowance: allowance=%d blocked=%#v metrics=%v", allowance, blocked, metrics)
	}
	if allowance, blocked := dailyAttackLimitAllowance(snapshot, 0, 30*time.Second, metrics); blocked != nil || allowance != -1 || capDailyAttackBatch(5, allowance) != 5 {
		t.Fatalf("disabled limit: allowance=%d blocked=%#v", allowance, blocked)
	}
}

func TestAutoAdvisorDoesNotSubscribeToNormalDailyAttackLimit(t *testing.T) {
	for _, domain := range NewAutoAdvisorPolicy().WakeDomains() {
		if domain == "attacks" {
			t.Fatal("Auto Advisor must remain exempt from the normal CRA daily attack limit")
		}
	}
}

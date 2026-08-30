package App

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestDailyAttackLimitPlanUsesServerCountAndAllowsReset(t *testing.T) {
	now := time.Date(2026, 7, 20, 15, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()

	plan, blocked, err := dailyAttackLimitPlan(gameState, 1000)
	if err != nil || !blocked || !strings.Contains(plan.Summary, "Waiting for the server") || len(plan.Steps) != 0 {
		t.Fatalf("missing server count: plan=%#v blocked=%t err=%v", plan, blocked, err)
	}
	gameState.DailyAttacks = State.DailyAttackState{Count: 1000, ServerThreshold: 3500, ObservedAt: now}
	plan, blocked, err = dailyAttackLimitPlan(gameState, 1000)
	if err != nil || !blocked || !strings.Contains(plan.Summary, "1000 / 1000") {
		t.Fatalf("reached cap: plan=%#v blocked=%t err=%v", plan, blocked, err)
	}
	gameState.DailyAttacks = State.DailyAttackState{Count: 0, ServerThreshold: 3500, ObservedAt: now.Add(time.Hour)}
	if _, blocked, err = dailyAttackLimitPlan(gameState, 1000); err != nil || blocked {
		t.Fatalf("server reset remained blocked: blocked=%t err=%v", blocked, err)
	}
}

func TestDailyAttackLimitGuardRunsOnlyForPositiveNormalAttackCap(t *testing.T) {
	steps := appendDailyAttackLimitGuard(nil, 0)
	if len(steps) != 0 {
		t.Fatalf("disabled cap added a guard: %#v", steps)
	}
	steps = appendDailyAttackLimitGuard(nil, 1000)
	if len(steps) != 1 || steps[0].Action != "attack.daily_limit.guard" {
		t.Fatalf("positive cap guard = %#v", steps)
	}

	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.DailyAttacks = State.DailyAttackState{Count: 1000, ObservedAt: now}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(dailyAttackLimitGuardRequest{Limit: 1000})
	if err := application.guardDailyAttackLimit(t.Context(), arguments); err == nil || !strings.Contains(err.Error(), "1000 / 1000") {
		t.Fatalf("send guard accepted reached cap: %v", err)
	}
}

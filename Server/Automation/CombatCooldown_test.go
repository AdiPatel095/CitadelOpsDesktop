package Automation

import (
	"context"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/State"
)

func TestCombatLanesStandDownDuringTheCooldownWhileDefenseRuns(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 10, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.CombatCooldown = State.CombatCooldownState{
		Until: now.Add(20 * time.Minute), Reason: "the game rejected a commander assignment (CRA 256) at 17:05:00",
	}
	snapshot := Snapshot{State: gameState, Now: now}

	decision, blocked := combatCooldownDecision(snapshot)
	if !blocked {
		t.Fatal("cooldown must block combat lanes while active")
	}
	if decision.Status != "waiting" || !strings.Contains(decision.Detail, "Combat cooldown until 17:30:00") {
		t.Fatalf("decision = %+v", decision)
	}
	if !decision.NextCheckAt.After(gameState.CombatCooldown.Until) {
		t.Fatal("the lane re-checks only after the window lapses")
	}

	// Every combat policy returns the standing-down decision from its head.
	for name, policy := range map[string]interface {
		Evaluate(context.Context, Snapshot) (Decision, error)
	}{
		"autoTowers": &AutoTowerPolicy{},
		"autoNomad":  &AutoNomadPolicy{},
		"autoKhan":   &AutoKhanPolicy{},
	} {
		got, err := policy.Evaluate(context.Background(), snapshot)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if !strings.Contains(got.Detail, "Combat cooldown") {
			t.Fatalf("%s must stand down, got %+v", name, got)
		}
	}

	// Lapsed: combat lanes resume normal evaluation.
	if _, blocked := combatCooldownDecision(Snapshot{State: gameState, Now: gameState.CombatCooldown.Until}); blocked {
		t.Fatal("cooldown must release exactly at its deadline")
	}
}

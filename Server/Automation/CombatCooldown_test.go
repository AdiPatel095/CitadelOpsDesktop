package Automation

import (
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func combatCooldownSnapshot(now time.Time) Snapshot {
	gameState := State.NewGameState()
	gameState.CombatCooldown = State.CombatCooldownState{
		Until: now.Add(20 * time.Minute), Reason: "the game rejected a commander assignment (CRA 256) at 17:05:00",
	}
	return Snapshot{State: gameState, Now: now}
}

func TestCooldownSubstitutesOnlyHostileAttackLaunches(t *testing.T) {
	now := time.Date(2026, 8, 17, 17, 10, 0, 0, time.UTC)
	snapshot := combatCooldownSnapshot(now)

	// Every hostile launch is replaced with the standing-down wait.
	for name := range combatLaunchIntents {
		decision := gateCombatLaunchDecision(snapshot, Decision{
			Status: "ready", Request: &Intent.Request{Name: name}, Metrics: map[string]float64{"x": 1},
		})
		if decision.Request != nil || decision.Status != "waiting" ||
			!strings.Contains(decision.Detail, "Combat cooldown until 17:30:00") {
			t.Fatalf("%s must stand down, got %+v", name, decision)
		}
		if decision.Metrics["x"] != 1 {
			t.Fatalf("%s must keep lane metrics", name)
		}
		if !decision.NextCheckAt.After(snapshot.State.CombatCooldown.Until) {
			t.Fatalf("%s re-checks only after the window", name)
		}
	}

	// The same lanes' non-attack work keeps flowing: khan rage taunts, storm
	// purchases, nomad scans and locks, defense stationing, recalls.
	for _, name := range []string{
		"khan.taunt", "storm.shop.purchase", "storm.map.scan", "nomad.map.scan",
		"nomad.target.lock", "nomad.cooldown.minute_skip", "troops.station",
		"movement.recall", "resource.kingdom.ship", "daily_attacks.refresh",
	} {
		original := Decision{Status: "ready", Request: &Intent.Request{Name: name}}
		if got := gateCombatLaunchDecision(snapshot, original); got.Request == nil || got.Request.Name != name {
			t.Fatalf("%s must pass through during the cooldown, got %+v", name, got)
		}
	}

	// Passive decisions are untouched, and everything flows after expiry.
	passive := Decision{Status: "idle", Detail: "nothing to do"}
	if got := gateCombatLaunchDecision(snapshot, passive); got.Detail != passive.Detail {
		t.Fatal("passive decisions must never be substituted")
	}
	lapsed := snapshot
	lapsed.Now = snapshot.State.CombatCooldown.Until
	attack := Decision{Status: "ready", Request: &Intent.Request{Name: "khan.attack"}}
	if got := gateCombatLaunchDecision(lapsed, attack); got.Request == nil {
		t.Fatal("attacks resume exactly at the deadline")
	}
}

package Automation

// The combat circuit breaker pauses exactly one thing: queueing NEW hostile
// attacks. When the game rejects a commander assignment (CRA 256), every
// automation decision that would launch an attack is substituted with a
// standing-down wait until the window lapses — while the same lanes keep
// doing everything else they do: khan chains its rage taunts, storm keeps
// purchasing and refreshing targets, nomad keeps scanning and locking
// targets, defense stationing and recalls stay untouched, and manual
// dashboard attacks are never gated at all.

import (
	"fmt"
	"time"
)

// combatLaunchIntents is the curated set of hostile attack launches — NOT the
// generic movement-launch effect, which also covers taunts, stationing,
// shipping, and recalls that must keep flowing during a cooldown.
var combatLaunchIntents = map[string]struct{}{
	"tower.attack":            {},
	"tower.launch":            {},
	"storm.attack":            {},
	"khan.attack":             {},
	"nomad.camp.attack":       {},
	"nomad.rbc_test.attack":   {},
	"invasion.attack":         {},
	"alliance.target.attack":  {},
	"beri.tower.attack":       {},
	"advisor.run.launch":      {},
	"rift.maiden_wave.launch": {},
	"rift.launch.replay":      {},
}

// combatCooldownDecision reports the standing-down decision while the
// breaker is tripped; blocked is false in normal operation.
func combatCooldownDecision(snapshot Snapshot) (Decision, bool) {
	cooldown := snapshot.State.CombatCooldown
	if !cooldown.ActiveAt(snapshot.Now) {
		return Decision{}, false
	}
	detail := fmt.Sprintf("Combat cooldown until %s", cooldown.Until.UTC().Format("15:04:05"))
	if cooldown.Reason != "" {
		detail += " — " + cooldown.Reason
	}
	detail += "; attack launches are paused, everything else continues"
	return Decision{
		Status: "waiting", Detail: detail,
		NextCheckAt: cooldown.Until.Add(time.Second),
	}, true
}

// gateCombatLaunchDecision lets every decision through except a hostile
// attack launch during an active cooldown, which it replaces with the
// standing-down wait (metrics preserved so lane telemetry stays continuous).
func gateCombatLaunchDecision(snapshot Snapshot, decision Decision) Decision {
	if decision.Request == nil {
		return decision
	}
	if _, hostile := combatLaunchIntents[decision.Request.Name]; !hostile {
		return decision
	}
	blocked, active := combatCooldownDecision(snapshot)
	if !active {
		return decision
	}
	blocked.Metrics = decision.Metrics
	return blocked
}

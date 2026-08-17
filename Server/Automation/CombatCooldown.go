package Automation

// Shared gate for the attack/combat lanes. When the combat cooldown is
// active (the game rejected a commander assignment with CRA 256), every lane
// that launches attacks waits it out with the same message, while non-combat
// lanes never consult this and keep running.

import (
	"fmt"
	"time"
)

// combatCooldownDecision reports the standing-down decision when the combat
// circuit breaker is tripped; blocked is false in normal operation.
func combatCooldownDecision(snapshot Snapshot) (Decision, bool) {
	cooldown := snapshot.State.CombatCooldown
	if !cooldown.ActiveAt(snapshot.Now) {
		return Decision{}, false
	}
	detail := fmt.Sprintf("Combat cooldown until %s", cooldown.Until.UTC().Format("15:04:05"))
	if cooldown.Reason != "" {
		detail += " — " + cooldown.Reason
	}
	return Decision{
		Status: "waiting", Detail: detail,
		NextCheckAt: cooldown.Until.Add(time.Second),
	}, true
}

package Ingest

// The combat-automation circuit breaker. CRA response code 256 ("the selected
// commander is already assigned to an active movement") is the game refusing
// a launch the client believed was possible. Repeats of that rejection are
// the request pattern observed immediately before a temporary suspension, so
// one occurrence stands the attack lanes down for a fixed window while every
// non-combat lane keeps running (see Automation.combatCooldownDecision).

import (
	"context"
	"fmt"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

// combatCooldownDuration is deliberately long: the point is to look nothing
// like a bot hammering rejected launches.
const combatCooldownDuration = 30 * time.Minute

const craCommanderBusyResponseCode = 256

func reduceCombatCooldownOnCommanderBusy(
	_ context.Context,
	frame Protocol.Frame,
	gameState *State.GameState,
	_ *GameData.Store,
) ([]string, bool, error) {
	if frame.ResponseCode == nil || *frame.ResponseCode != craCommanderBusyResponseCode {
		return nil, false, nil
	}
	observedAt := frame.ReceivedAt
	if observedAt.IsZero() {
		observedAt = time.Now()
	}
	until := observedAt.UTC().Add(combatCooldownDuration)
	if gameState.CombatCooldown.ActiveAt(observedAt.UTC()) && !until.After(gameState.CombatCooldown.Until) {
		// Already standing down at least that long; nothing new to record.
		return nil, false, nil
	}
	gameState.CombatCooldown = State.CombatCooldownState{
		Until: until,
		Reason: fmt.Sprintf(
			"the game rejected a commander assignment (CRA %d) at %s",
			craCommanderBusyResponseCode, observedAt.UTC().Format("15:04:05"),
		),
	}
	return []string{"combat-cooldown"}, true, nil
}

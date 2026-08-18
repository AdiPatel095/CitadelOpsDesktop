package Ingest

import (
	"context"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCommanderBusyRejectionTripsTheCombatCooldown(t *testing.T) {
	gameState := State.NewGameState()
	code := 256
	receivedAt := time.Date(2026, 8, 17, 17, 5, 0, 0, time.UTC)
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "cra", ResponseCode: &code, ReceivedAt: receivedAt,
	}
	domains, changed, err := reduceCombatCooldownOnCommanderBusy(context.Background(), frame, &gameState, nil)
	if err != nil || !changed || len(domains) == 0 {
		t.Fatalf("cooldown reduce changed=%t domains=%v err=%v", changed, domains, err)
	}
	if !gameState.CombatCooldown.ActiveAt(receivedAt) {
		t.Fatal("cooldown must be active immediately")
	}
	if want := receivedAt.Add(combatCooldownDuration); !gameState.CombatCooldown.Until.Equal(want) {
		t.Fatalf("until = %v, want %v", gameState.CombatCooldown.Until, want)
	}
	if gameState.CombatCooldown.ActiveAt(receivedAt.Add(combatCooldownDuration)) {
		t.Fatal("cooldown must lapse after its window")
	}

	// A second 256 five minutes in extends the stand-down.
	later := receivedAt.Add(5 * time.Minute)
	frame.ReceivedAt = later
	if _, changed, _ := reduceCombatCooldownOnCommanderBusy(context.Background(), frame, &gameState, nil); !changed {
		t.Fatal("a later rejection extends the cooldown")
	}
	if want := later.Add(combatCooldownDuration); !gameState.CombatCooldown.Until.Equal(want) {
		t.Fatalf("extended until = %v, want %v", gameState.CombatCooldown.Until, want)
	}
}

func TestOtherResponseCodesLeaveTheCooldownAlone(t *testing.T) {
	gameState := State.NewGameState()
	for _, code := range []int{0, 95, 453} {
		value := code
		frame := Protocol.Frame{
			Direction: Protocol.DirectionInbound, Opcode: "cra", ResponseCode: &value,
			ReceivedAt: time.Date(2026, 8, 17, 17, 5, 0, 0, time.UTC),
		}
		if _, changed, err := reduceCombatCooldownOnCommanderBusy(context.Background(), frame, &gameState, nil); err != nil || changed {
			t.Fatalf("code %d must not trip the cooldown (changed=%t err=%v)", code, changed, err)
		}
	}
	frame := Protocol.Frame{Direction: Protocol.DirectionInbound, Opcode: "cra"}
	if _, changed, _ := reduceCombatCooldownOnCommanderBusy(context.Background(), frame, &gameState, nil); changed {
		t.Fatal("a frame without a response code must not trip the cooldown")
	}
}

package State

import (
	"testing"
	"time"
)

func TestIsIncomingPlayerAttackRequiresCompletePlayerIdentity(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	arrives := now.Add(time.Minute)
	gameState := NewGameState()
	gameState.Player.ID = 7
	gameState.Castles[100] = CastleState{ID: 100, SlotType: 4}
	attack := MovementState{
		ID: 1, Direction: 0, TypeID: 0, OwnerPlayerID: 8, TargetPlayerID: 7,
		SourceTypeID: 1, SourceCastleID: 200, TargetTypeID: 4, TargetCastleID: 100,
		ArrivesAt: &arrives,
	}
	if !IsIncomingPlayerAttack(gameState, attack, now) {
		t.Fatal("fully identified hostile player attack was rejected")
	}

	past := now.Add(-time.Second)
	tests := []struct {
		name   string
		mutate func(*MovementState)
	}{
		{name: "returning movement", mutate: func(movement *MovementState) { movement.Direction = 1 }},
		{name: "non-attack movement", mutate: func(movement *MovementState) { movement.TypeID = 2 }},
		{name: "missing owner", mutate: func(movement *MovementState) { movement.OwnerPlayerID = 0 }},
		{name: "own movement", mutate: func(movement *MovementState) { movement.OwnerPlayerID = 7 }},
		{name: "missing source type", mutate: func(movement *MovementState) { movement.SourceTypeID = 0 }},
		{name: "NPC source type", mutate: func(movement *MovementState) { movement.SourceTypeID = 2 }},
		{name: "non-player source", mutate: func(movement *MovementState) { movement.SourceCastleID = -1 }},
		{name: "missing target type", mutate: func(movement *MovementState) { movement.TargetTypeID = 0 }},
		{name: "mismatched target type", mutate: func(movement *MovementState) { movement.TargetTypeID = 1 }},
		{name: "unowned target", mutate: func(movement *MovementState) { movement.TargetCastleID = 101 }},
		{name: "missing target player", mutate: func(movement *MovementState) { movement.TargetPlayerID = 0 }},
		{name: "different target player", mutate: func(movement *MovementState) { movement.TargetPlayerID = 9 }},
		{name: "missing arrival", mutate: func(movement *MovementState) { movement.ArrivesAt = nil }},
		{name: "landed attack", mutate: func(movement *MovementState) { movement.ArrivesAt = &past }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := attack
			test.mutate(&candidate)
			if IsIncomingPlayerAttack(gameState, candidate, now) {
				t.Fatalf("ambiguous movement was accepted: %#v", candidate)
			}
		})
	}
}

func TestIsIncomingPlayerAttackRejectsKhanAndNPCMovements(t *testing.T) {
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	arrives := now.Add(time.Minute)
	gameState := NewGameState()
	gameState.Player.ID = 7
	gameState.Castles[100] = CastleState{ID: 100, SlotType: 4}

	khanTaunt := MovementState{
		ID: 1, Direction: 0, TypeID: 20, OwnerPlayerID: -801, TargetPlayerID: 7,
		SourceTypeID: 35, SourceCastleID: -1, TargetTypeID: 4, TargetCastleID: 100,
		ArrivesAt: &arrives,
	}
	npcAttackShape := MovementState{
		ID: 2, Direction: 0, TypeID: 0, TargetPlayerID: 7,
		SourceTypeID: 2, SourceCastleID: -2, TargetTypeID: 4, TargetCastleID: 100,
		ArrivesAt: &arrives,
	}
	if IsIncomingPlayerAttack(gameState, khanTaunt, now) {
		t.Fatal("Khan taunt was classified as a player attack")
	}
	if IsIncomingPlayerAttack(gameState, npcAttackShape, now) {
		t.Fatal("NPC-shaped movement was classified as a player attack")
	}
}

func TestIsOutgoingPlayerAttackRequiresOwnedSourceAndRealPlayerTarget(t *testing.T) {
	now := time.Date(2026, 8, 5, 15, 0, 0, 0, time.UTC)
	arrives := now.Add(time.Minute)
	gameState := NewGameState()
	gameState.Player.ID = 7
	gameState.Castles[100] = CastleState{ID: 100, SlotType: 4}
	attack := MovementState{
		ID: 1, Direction: 0, TypeID: 0, OwnerPlayerID: 7, TargetPlayerID: 8,
		SourceTypeID: 4, SourceCastleID: 100, TargetTypeID: 4, TargetCastleID: 200,
		ArrivesAt: &arrives,
	}
	if !IsOutgoingPlayerAttack(gameState, attack, now) {
		t.Fatal("fully identified outgoing PvP attack was rejected")
	}
	for _, mutate := range []func(*MovementState){
		func(movement *MovementState) { movement.OwnerPlayerID = 8 },
		func(movement *MovementState) { movement.TargetPlayerID = -1 },
		func(movement *MovementState) { movement.TargetPlayerID = 7 },
		func(movement *MovementState) { movement.TypeID = 3 },
		func(movement *MovementState) { movement.SourceCastleID = 101 },
		func(movement *MovementState) { movement.TargetCastleID = 0 },
		func(movement *MovementState) { movement.ArrivesAt = nil },
	} {
		candidate := attack
		mutate(&candidate)
		if IsOutgoingPlayerAttack(gameState, candidate, now) {
			t.Fatalf("ambiguous outgoing movement was accepted: %#v", candidate)
		}
	}
}

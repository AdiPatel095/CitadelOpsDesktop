package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanDefensePresetApplyReadsBeforeCombinedWrites(t *testing.T) {
	gameState := defenseIntentState()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = time.Time{}
	castle.Defense.InventoryObservedAt = time.Time{}
	gameState.Castles[10] = castle
	request := defensePresetRequest(castle)
	arguments, _ := json.Marshal(request)

	plan, err := planDefensePresetApply(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 7 {
		t.Fatalf("defense preset steps = %#v", plan.Steps)
	}
	if plan.Steps[0].Command.Opcode != "jaa" || plan.Steps[1].Command.Opcode != "dfc" {
		t.Fatalf("defense preset did not begin with focus and DFC: %#v", plan.Steps[:2])
	}
	if plan.Steps[2].Resolver != "defense.preset.wall.build" || plan.Steps[4].Resolver != "defense.moat.build" {
		t.Fatalf("defense preset write resolvers = %#v", plan.Steps)
	}
	if plan.Steps[6].Action != "defense.preset.verify" {
		t.Fatalf("defense preset verification = %#v", plan.Steps[6])
	}
}

func TestResolveDefensePresetWallValidatesMoatBeforeFirstWrite(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	gameState.Castles[10] = castle
	request := defensePresetResolvedRequest{
		defensePresetApplyRequest:   defensePresetRequest(castle),
		PreviousDefenseObservedAt:   now.Add(-time.Minute),
		PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	request.Moat.LeftToolSlots = append(request.Moat.LeftToolSlots, State.DefenseToolSlot{DefinitionID: -1})
	arguments, _ := json.Marshal(request)

	_, err := resolveDefensePresetWallStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "leftToolSlots must contain exactly 1 slots") {
		t.Fatalf("preset moat preflight error = %v", err)
	}
}

func TestResolveDefensePresetKeepPreservesObservedToolRows(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Keep.PrimaryToolSlots = []State.DefenseToolSlot{
		{DefinitionID: 501, Amount: 2}, {DefinitionID: -1}, {DefinitionID: -1},
	}
	gameState.Castles[10] = castle
	request := defensePresetResolvedRequest{
		defensePresetApplyRequest:   defensePresetRequest(castle),
		PreviousDefenseObservedAt:   now.Add(-time.Minute),
		PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	request.Keep = &defensePresetKeepRequest{MAUCT: 800, UnitTypePercent: 0}
	arguments, _ := json.Marshal(request)

	step, err := resolveDefensePresetKeepStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"CX":1164,"CY":1167,"AID":10,"MAUCT":800,"UC":0,"S":[[501,2],[-1,0],[-1,0]],"STS":[[-1,0],[-1,0],[-1,0]]}`
	if got := string(step.Command.Payload); got != want {
		t.Fatalf("preset DFK payload = %s", got)
	}
}

func defensePresetRequest(castle State.CastleState) defensePresetApplyRequest {
	return defensePresetApplyRequest{
		CastleID: 10, PresetID: "preset-1", PresetName: "Captured defense",
		Wall: defensePresetWallRequest{
			Left: castle.Defense.Wall.Left, Middle: castle.Defense.Wall.Middle, Right: castle.Defense.Wall.Right,
		},
		Moat: defensePresetMoatRequest{
			LeftToolSlots:   castle.Defense.Moat.LeftToolSlots,
			MiddleToolSlots: castle.Defense.Moat.MiddleToolSlots,
			RightToolSlots:  castle.Defense.Moat.RightToolSlots,
		},
	}
}

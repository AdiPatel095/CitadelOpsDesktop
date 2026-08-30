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

func TestPlanDefensePresetApplyRejectsWallSlotOverflow(t *testing.T) {
	gameState := defenseIntentState()
	request := defensePresetRequest(gameState.Castles[10])
	request.Wall.Middle.ToolSlots = append(
		append([]State.DefenseToolSlot{}, request.Wall.Middle.ToolSlots...),
		State.DefenseToolSlot{DefinitionID: -1},
	)
	arguments, _ := json.Marshal(request)

	_, err := planDefensePresetApply(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
	if err == nil || !strings.Contains(err.Error(), "exactly 6 ordered slots") {
		t.Fatalf("defense preset wall slot limit error = %v", err)
	}
}

func TestKhanDefensePresetRechecksSafetyImmediatelyBeforeEachWrite(t *testing.T) {
	gameState := defenseIntentState()
	castle := gameState.Castles[10]
	castle.SlotType = 1
	gameState.Castles[10] = castle
	request := defensePresetRequest(castle)
	request.KhanGuard = &khanLaneGuardRequest{MainCastleID: castle.ID}
	arguments, _ := json.Marshal(request)

	plan, err := planDefensePresetApply(t.Context(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	guardCount := 0
	for index, step := range plan.Steps {
		if step.Action == "khan.lane.guard" {
			guardCount++
			var guard khanLaneGuardActionRequest
			if err := decodeIntentArguments(step.ActionArguments, &guard); err != nil ||
				guard.KhanGuard.MainCastleID != castle.ID {
				t.Fatalf("Khan defense guard arguments = %#v, err=%v", guard, err)
			}
		}
		if step.Resolver == "defense.preset.wall.build" || step.Resolver == "defense.moat.build" {
			if index == 0 || plan.Steps[index-1].Action != "khan.lane.guard" {
				t.Fatalf("Khan defense write lacked an immediate safety guard: %#v", plan.Steps)
			}
		}
	}
	if guardCount != 3 {
		t.Fatalf("Khan defense guard count = %d, steps=%#v", guardCount, plan.Steps)
	}

	openUntil := time.Now().UTC().Add(time.Hour)
	castle.Defense.OpenGateUntil = &openUntil
	gameState.Castles[10] = castle
	if _, err := planDefensePresetApply(t.Context(), Intent.PlanningContext{State: gameState}, arguments); err == nil ||
		!strings.Contains(err.Error(), "gates are open") {
		t.Fatalf("Khan open-gate safety error = %v", err)
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
	if err == nil || !strings.Contains(err.Error(), "leftToolSlots must contain exactly one moat slot") {
		t.Fatalf("preset moat preflight error = %v", err)
	}
}

func TestResolveDefensePresetWallChecksCombinedCourtyardInventoryBeforeFirstWrite(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Inventory[507] = 0
	castle.Defense.Wall.Left.ToolSlots[0] = State.DefenseToolSlot{DefinitionID: 507, Amount: 1}
	gameState.Castles[10] = castle
	primary := []State.DefenseToolSlot{
		{DefinitionID: 507, Amount: 1}, {DefinitionID: -1}, {DefinitionID: -1},
	}
	secondary := emptyDefenseToolSlots()
	request := defensePresetResolvedRequest{
		defensePresetApplyRequest:   defensePresetRequest(castle),
		PreviousDefenseObservedAt:   now.Add(-time.Minute),
		PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	request.Keep = &defensePresetKeepRequest{
		MAUCT: 800, UnitTypePercent: 0,
		PrimaryToolSlots: &primary, SecondaryToolSlots: &secondary,
	}
	arguments, _ := json.Marshal(request)

	_, err := resolveDefensePresetWallStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "requires 2") || !strings.Contains(err.Error(), "has 1 available") {
		t.Fatalf("combined courtyard inventory preflight error = %v", err)
	}
}

func TestResolveDefensePresetKeepPreservesObservedToolRows(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Keep.PrimaryToolSlots = []State.DefenseToolSlot{
		{DefinitionID: 505, Amount: 2}, {DefinitionID: -1}, {DefinitionID: -1},
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
	want := `{"CX":1164,"CY":1167,"AID":10,"MAUCT":800,"UC":0,"S":[[505,2],[-1,0],[-1,0]],"STS":[[-1,0],[-1,0],[-1,0]]}`
	if got := string(step.Command.Payload); got != want {
		t.Fatalf("preset DFK payload = %s", got)
	}
}

func TestResolveDefensePresetKeepAppliesExplicitCourtyardRows(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	gameState.Castles[10] = castle
	primary := []State.DefenseToolSlot{
		{DefinitionID: 505, Amount: 2}, {DefinitionID: -1}, {DefinitionID: -1},
	}
	secondary := []State.DefenseToolSlot{
		{DefinitionID: 506, Amount: 3}, {DefinitionID: -1}, {DefinitionID: -1},
	}
	request := defensePresetResolvedRequest{
		defensePresetApplyRequest:   defensePresetRequest(castle),
		PreviousDefenseObservedAt:   now.Add(-time.Minute),
		PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	request.Keep = &defensePresetKeepRequest{
		MAUCT: 800, UnitTypePercent: 0,
		PrimaryToolSlots: &primary, SecondaryToolSlots: &secondary,
	}
	arguments, _ := json.Marshal(request)

	step, err := resolveDefensePresetKeepStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"CX":1164,"CY":1167,"AID":10,"MAUCT":800,"UC":0,"S":[[505,2],[-1,0],[-1,0]],"STS":[[506,3],[-1,0],[-1,0]]}`
	if got := string(step.Command.Payload); got != want {
		t.Fatalf("preset DFK payload = %s", got)
	}
}

func TestDefensePresetRejectsPartialCourtyardRows(t *testing.T) {
	gameState := defenseIntentState()
	request := defensePresetRequest(gameState.Castles[10])
	primary := emptyDefenseToolSlots()
	request.Keep = &defensePresetKeepRequest{
		MAUCT: 800, UnitTypePercent: 0, PrimaryToolSlots: &primary,
	}
	arguments, _ := json.Marshal(request)

	_, err := planDefensePresetApply(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
	if err == nil || !strings.Contains(err.Error(), "must include both primaryToolSlots and secondaryToolSlots") {
		t.Fatalf("partial courtyard rows error = %v", err)
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

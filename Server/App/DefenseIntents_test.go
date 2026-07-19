package App

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanDefenseRefreshUsesCapturedDFCShape(t *testing.T) {
	gameState := defenseIntentState()
	plan, err := planDefenseRefresh(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"castleId":10}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 || plan.Steps[0].Command.Opcode != "jaa" || plan.Steps[1].Command.Opcode != "dfc" || plan.Steps[2].Action != "defense.verify_refresh" {
		t.Fatalf("defense refresh steps = %#v", plan.Steps)
	}
	if got := string(plan.Steps[1].Command.Payload); got != `{"CX":1164,"CY":1167,"AID":10,"KID":-1}` {
		t.Fatalf("DFC payload = %s", got)
	}
}

func TestResolveDefenseKeepBuildsValidatedDFKPayload(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	gameState.Castles[10] = castle
	request := defenseKeepResolvedRequest{
		defenseKeepUpdateRequest: defenseKeepUpdateRequest{
			CastleID: 10, MAUCT: 973, UnitTypePercent: 50,
			PrimaryToolSlots:   emptyDefenseToolSlots(),
			SecondaryToolSlots: emptyDefenseToolSlots(),
		},
		PreviousDefenseObservedAt: now.Add(-time.Minute), PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	arguments, _ := json.Marshal(request)
	step, err := resolveDefenseKeepStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "dfk" || step.AwaitOpcode != "dfk" {
		t.Fatalf("DFK step = %#v", step)
	}
	if got := string(step.Command.Payload); got != `{"CX":1164,"CY":1167,"AID":10,"MAUCT":973,"UC":50,"S":[[-1,0],[-1,0],[-1,0]],"STS":[[-1,0],[-1,0],[-1,0]]}` {
		t.Fatalf("DFK payload = %s", got)
	}
}

func TestResolveDefenseKeepRejectsStaleSnapshot(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	gameState.Castles[10] = castle
	request := defenseKeepResolvedRequest{
		defenseKeepUpdateRequest: defenseKeepUpdateRequest{
			CastleID: 10, MAUCT: 973, UnitTypePercent: 50,
			PrimaryToolSlots:   emptyDefenseToolSlots(),
			SecondaryToolSlots: emptyDefenseToolSlots(),
		},
		PreviousDefenseObservedAt: now, PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	arguments, _ := json.Marshal(request)
	_, err := resolveDefenseKeepStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "fresh DFC defense snapshot") {
		t.Fatalf("stale defense error = %v", err)
	}
}

func TestDefenseKeepGuardsUnconfirmedSlotChanges(t *testing.T) {
	request := defenseKeepUpdateRequest{
		CastleID: 10, MAUCT: 250, UnitTypePercent: 50,
		PrimaryToolSlots:   []State.DefenseToolSlot{{DefinitionID: 501, Amount: 1}, {DefinitionID: -1}, {DefinitionID: -1}},
		SecondaryToolSlots: emptyDefenseToolSlots(),
	}
	arguments, _ := json.Marshal(request)
	_, err := planDefenseKeepUpdate(context.Background(), Intent.PlanningContext{
		State: defenseIntentState(), GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "DFK S/STS changes are not capture-confirmed") {
		t.Fatalf("DFK slot guard error = %v", err)
	}
}

func TestResolveDefenseWallBuildsCapturedDFWPayloadAndCreditsAssignedTools(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Inventory = map[State.UnitID]int64{501: 0, 503: 0}
	gameState.Castles[10] = castle
	request := defenseWallResolvedRequest{
		defenseWallUpdateRequest: defenseWallUpdateRequest{
			CastleID: 10,
			Left:     castle.Defense.Wall.Left,
			Middle:   castle.Defense.Wall.Middle,
			Right:    castle.Defense.Wall.Right,
		},
		PreviousDefenseObservedAt: now.Add(-time.Minute), PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	arguments, _ := json.Marshal(request)
	step, err := resolveDefenseWallStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if step.Command.Opcode != "dfw" || step.AwaitOpcode != "dfw" {
		t.Fatalf("DFW step = %#v", step)
	}
	want := `{"CX":1164,"CY":1167,"AID":10,"L":{"S":[[-1,0],[-1,0],[-1,0],[501,2],[-1,0]],"UP":23,"UC":20},"M":{"S":[[501,1],[503,2],[-1,0],[-1,0],[-1,0],[-1,0]],"UP":60,"UC":70},"R":{"S":[[-1,0],[-1,0],[-1,0],[-1,0],[-1,0]],"UP":17,"UC":100}}`
	if got := string(step.Command.Payload); got != want {
		t.Fatalf("DFW payload = %s", got)
	}
}

func TestResolveDefenseWallRejectsAmountBeyondFreePlusAssigned(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Inventory = map[State.UnitID]int64{501: 0, 503: 0}
	gameState.Castles[10] = castle
	request := defenseWallResolvedRequest{
		defenseWallUpdateRequest: defenseWallUpdateRequest{
			CastleID: 10,
			Left:     castle.Defense.Wall.Left,
			Middle:   castle.Defense.Wall.Middle,
			Right:    castle.Defense.Wall.Right,
		},
		PreviousDefenseObservedAt: now.Add(-time.Minute), PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	request.Middle.ToolSlots = append([]State.DefenseToolSlot(nil), request.Middle.ToolSlots...)
	request.Middle.ToolSlots[0].Amount = 2
	arguments, _ := json.Marshal(request)
	_, err := resolveDefenseWallStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "requires 4") || !strings.Contains(err.Error(), "has 3 available") {
		t.Fatalf("wall inventory error = %v", err)
	}
}

func TestResolveDefenseMoatBuildsCapturedDFMPayloadAndCreditsAssignedTools(t *testing.T) {
	gameState := defenseIntentState()
	now := time.Now().UTC()
	castle := gameState.Castles[10]
	castle.Defense.ObservedAt = now
	castle.Defense.InventoryObservedAt = now
	castle.Defense.Inventory = map[State.UnitID]int64{502: 0}
	gameState.Castles[10] = castle
	request := defenseMoatResolvedRequest{
		defenseMoatUpdateRequest: defenseMoatUpdateRequest{
			CastleID:        10,
			LeftToolSlots:   castle.Defense.Moat.LeftToolSlots,
			MiddleToolSlots: castle.Defense.Moat.MiddleToolSlots,
			RightToolSlots:  castle.Defense.Moat.RightToolSlots,
		},
		PreviousDefenseObservedAt: now.Add(-time.Minute), PreviousInventoryObservedAt: now.Add(-time.Minute),
	}
	arguments, _ := json.Marshal(request)
	step, err := resolveDefenseMoatStep(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(step.Command.Payload); got != `{"CX":1164,"CY":1167,"AID":10,"LS":[[502,5]],"MS":[[502,2]],"RS":[[-1,0]]}` {
		t.Fatalf("DFM payload = %s", got)
	}
}

func TestDefenseSectionsRejectWrongToolDefinitionOrSlotType(t *testing.T) {
	gameState := defenseIntentState()
	wall := defenseWallUpdateRequest{
		CastleID: 10,
		Left:     gameState.Castles[10].Defense.Wall.Left,
		Middle:   gameState.Castles[10].Defense.Wall.Middle,
		Right:    gameState.Castles[10].Defense.Wall.Right,
	}
	wall.Left.ToolSlots[0] = State.DefenseToolSlot{DefinitionID: 601, Amount: 1}
	arguments, _ := json.Marshal(wall)
	_, err := planDefenseWallUpdate(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "is not a defense tool") {
		t.Fatalf("non-tool wall error = %v", err)
	}

	gameState = defenseIntentState()
	wall = defenseWallUpdateRequest{
		CastleID: 10,
		Left:     gameState.Castles[10].Defense.Wall.Left,
		Middle:   gameState.Castles[10].Defense.Wall.Middle,
		Right:    gameState.Castles[10].Defense.Wall.Right,
	}
	wall.Left.ToolSlots[0] = State.DefenseToolSlot{DefinitionID: 504, Amount: 1}
	arguments, _ = json.Marshal(wall)
	_, err = planDefenseWallUpdate(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "is not a defense tool") {
		t.Fatalf("attack tool wall error = %v", err)
	}

	moat := defenseMoatUpdateRequest{
		CastleID:        10,
		LeftToolSlots:   []State.DefenseToolSlot{{DefinitionID: 501, Amount: 1}},
		MiddleToolSlots: gameState.Castles[10].Defense.Moat.MiddleToolSlots,
		RightToolSlots:  gameState.Castles[10].Defense.Moat.RightToolSlots,
	}
	arguments, _ = json.Marshal(moat)
	_, err = planDefenseMoatUpdate(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: defenseIntentGameData(t),
	}, arguments)
	if err == nil || !strings.Contains(err.Error(), "is not valid for this defense section") {
		t.Fatalf("wrong moat slot type error = %v", err)
	}
}

func defenseIntentState() State.GameState {
	gameState := State.NewGameState()
	observedAt := time.Unix(1_000, 0).UTC()
	gameState.Castles[10] = State.CastleState{
		ID: 10, KingdomID: 0, X: 1164, Y: 1167,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{}, Traveling: map[State.UnitID]int64{}, Hospital: map[State.UnitID]int64{}, SpecialHospital: map[State.UnitID]int64{}, Total: map[State.UnitID]int64{}},
		Defense: State.CastleDefenseState{
			ObservedAt: observedAt, InventoryObservedAt: observedAt,
			Inventory: map[State.UnitID]int64{501: 10, 502: 10, 503: 10},
			Wall: State.DefenseWallState{
				Left: State.DefenseWallSection{
					ToolSlots:   []State.DefenseToolSlot{{DefinitionID: -1}, {DefinitionID: -1}, {DefinitionID: -1}, {DefinitionID: 501, Amount: 2}},
					UnitPercent: 23, UnitTypePercent: 20,
				},
				Middle: State.DefenseWallSection{
					ToolSlots:   []State.DefenseToolSlot{{DefinitionID: 501, Amount: 1}, {DefinitionID: 503, Amount: 2}, {DefinitionID: -1}, {DefinitionID: -1}, {DefinitionID: -1}, {DefinitionID: -1}},
					UnitPercent: 60, UnitTypePercent: 70,
				},
				Right: State.DefenseWallSection{
					ToolSlots: emptyDefenseToolSlotsN(4), UnitPercent: 17, UnitTypePercent: 100,
				},
			},
			Keep: State.DefenseKeepState{
				PrimaryToolSlots: emptyDefenseToolSlots(), SecondaryToolSlots: emptyDefenseToolSlots(),
				MAUCT: 973, UnitTypePercent: 50,
			},
			Moat: State.DefenseMoatState{
				LeftToolSlots:   []State.DefenseToolSlot{{DefinitionID: 502, Amount: 5}},
				MiddleToolSlots: []State.DefenseToolSlot{{DefinitionID: 502, Amount: 2}},
				RightToolSlots:  []State.DefenseToolSlot{{DefinitionID: -1}},
			},
		},
	}
	return gameState
}

func defenseIntentGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[
			{"wodID":501,"name":"DefenseTool","typ":"Defence","slotTypes":[1]},
			{"wodID":502,"name":"MoatTool","typ":"Defence","slotTypes":"4,9"},
			{"wodID":503,"name":"GateTool","typ":"Defence","slotTypes":[2,9]},
			{"wodID":504,"name":"AttackTool","typ":"Attack","slotTypes":[1,2,9]},
			{"wodID":601,"name":"Defender","typ":"Defence","slotTypes":[]}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func emptyDefenseToolSlots() []State.DefenseToolSlot {
	return emptyDefenseToolSlotsN(3)
}

func emptyDefenseToolSlotsN(count int) []State.DefenseToolSlot {
	slots := make([]State.DefenseToolSlot, count)
	for index := range slots {
		slots[index].DefinitionID = -1
	}
	return slots
}

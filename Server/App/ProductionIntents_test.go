package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanProductionEnqueueUsesDefaultSessionKeyBeforeObservation(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[],
		"viplevels":[{"vipLevelID":"10","recruitmentBonusSlots":"3","productionBonusSlots":"3"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID:         77,
		Production: map[int]State.ProductionQueue{0: {LineID: 0, ObservedAt: time.Now().UTC()}},
	}
	gameState.Player.VIP = State.VIPState{Level: 10}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 {
		t.Fatalf("production enqueue steps = %d, want one focus and one enqueue command", len(plan.Steps))
	}
	var payload struct {
		SessionKey int `json:"SK"`
	}
	if err := json.Unmarshal(plan.Steps[len(plan.Steps)-1].Command.Payload, &payload); err != nil {
		t.Fatalf("decode production payload: %v", err)
	}
	if payload.SessionKey != defaultProductionSessionKey {
		t.Fatalf("session key = %d, want %d", payload.SessionKey, defaultProductionSessionKey)
	}
}

func TestPlanProductionEnqueueDoesNotCountActiveStackAgainstQueueSlots(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {
				LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC(),
				Active: &State.QueueItem{Definition: State.DefinitionRef{Collection: "units", ID: 489}, Amount: 110},
				Queued: make([]State.QueueItem, 4),
			},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110}`))
	if err != nil {
		t.Fatalf("plan production enqueue: %v", err)
	}
	if len(plan.Steps) != 2 || plan.Steps[1].Opcode != "bup" {
		t.Fatalf("production enqueue steps = %#v, want one focus and one enqueue command", plan.Steps)
	}
}

func TestPlanProductionEnqueueFillsEveryAvailableQueueSlotWithOneFocus(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC()},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatalf("plan production fill: %v", err)
	}
	if len(plan.Steps) != 6 {
		t.Fatalf("production fill steps = %d, want one focus and five enqueue commands", len(plan.Steps))
	}
	if plan.Steps[0].Opcode != "jaa" {
		t.Fatalf("first production fill opcode = %q, want jaa", plan.Steps[0].Opcode)
	}
	for index, step := range plan.Steps[1:] {
		if step.Opcode != "bup" || step.AwaitOpcode != "bup" {
			t.Fatalf("production fill step %d = %#v, want awaited bup", index+1, step)
		}
	}
}

func TestPlanProductionEnqueueFillOnlyUsesFreeQueueSlots(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":489}],"constructionItems":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[77] = State.CastleState{
		ID: 77,
		Production: map[int]State.ProductionQueue{
			0: {LineID: 0, Capacity: 5, ObservedAt: time.Now().UTC(), Queued: make([]State.QueueItem, 4)},
		},
	}
	plan, err := planProductionEnqueue(context.Background(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, json.RawMessage(`{"castleId":77,"lineId":0,"definitionId":489,"amount":110,"fillAvailable":true}`))
	if err != nil {
		t.Fatalf("plan production fill: %v", err)
	}
	if len(plan.Steps) != 2 || plan.Steps[1].Opcode != "bup" {
		t.Fatalf("production fill steps = %#v, want one focus and one enqueue command", plan.Steps)
	}
}

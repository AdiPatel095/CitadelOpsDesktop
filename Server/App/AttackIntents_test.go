package App

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestRiftReplayUsesOwnedSchedulerForFutureArrival(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", CommanderID: 5, OneWayTTSeconds: 120,
		Body: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"A":[{}]}`),
	}
	arguments, _ := json.Marshal(map[string]any{
		"launchId": "launch", "arriveAtUnix": time.Now().UTC().Add(20 * time.Minute).Unix(),
	})
	plan, err := (&Application{}).planRiftReplay(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "operation.schedule" || plan.Steps[0].Opcode != "" {
		t.Fatalf("unexpected scheduled plan: %#v", plan)
	}
}

func TestRiftReplaySendsImmediateCommand(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", CommanderID: 5,
		Body: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"A":[{}]}`),
	}
	plan, err := (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"launchId":"launch"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Opcode != "cra" || plan.Steps[0].Action != "" {
		t.Fatalf("unexpected immediate plan: %#v", plan)
	}
}

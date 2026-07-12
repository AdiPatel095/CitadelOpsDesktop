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

func TestRiftReplayAppliesValidatedAttackSetup(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[{"wodID":1},{"wodID":2,"slotTypes":[1]}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Castles[1] = State.CastleState{
		ID: 1, X: 7, Y: 8,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{1: 20, 2: 10}},
	}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", CommanderID: 5,
		Body: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"TX":10,"TY":20,"KID":0,"A":[{}],"AST":[9],"RW":[[8,1]]}`),
	}
	arguments := json.RawMessage(`{
		"launchId":"launch","sourceCastleId":1,
		"attackSetup":{"name":"test","waves":[{
			"L":{"troops":[{"itemId":1,"quantity":11}],"tools":[{"itemId":2,"quantity":3}]},
			"M":{"troops":[],"tools":[]},"R":{"troops":[],"tools":[]}
		}]}
	}`)
	plan, err := (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	var body struct {
		SourceX int          `json:"SX"`
		SourceY int          `json:"SY"`
		Waves   []attackWave `json:"A"`
		Tools   []int64      `json:"AST"`
	}
	if err := json.Unmarshal(plan.Steps[0].Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if body.SourceX != 7 || body.SourceY != 8 || len(body.Waves) != 1 {
		t.Fatalf("unexpected custom attack body: %+v", body)
	}
	if body.Waves[0].Left.Units[0] != (attackPair{1, 11}) || body.Waves[0].Left.Tools[0] != (attackPair{2, 3}) {
		t.Fatalf("custom formation was not applied: %+v", body.Waves[0])
	}
	if len(body.Tools) != 3 || body.Tools[0] != -1 {
		t.Fatalf("captured support tools were not cleared: %+v", body.Tools)
	}
}

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

func TestRiftTemplateMutationsPersistImmediately(t *testing.T) {
	dataDir := t.TempDir()
	gameState := State.NewGameState()
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", Body: json.RawMessage(`{"LID":5,"A":[{}]}`),
	}
	application := &Application{DataDir: dataDir, State: State.NewStore(gameState)}

	if err := application.renameRiftTemplate(
		context.Background(), json.RawMessage(`{"launchId":"launch","displayName":"Saved name"}`),
	); err != nil {
		t.Fatal(err)
	}
	loaded, err := State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Rift.Launches["launch"].DisplayName != "Saved name" {
		t.Fatalf("persisted Rift name = %q", loaded.Rift.Launches["launch"].DisplayName)
	}

	if err := application.deleteRiftTemplate(
		context.Background(), json.RawMessage(`{"launchId":"launch"}`),
	); err != nil {
		t.Fatal(err)
	}
	loaded, err = State.LoadSnapshot(dataDir)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := loaded.Rift.Launches["launch"]; exists {
		t.Fatal("deleted Rift template returned from persisted state")
	}
	if loaded.Rift.DeletedLaunchIDs["launch"] <= 0 {
		t.Fatal("deleted Rift template did not persist its deletion marker")
	}
}

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
	if plan.Admission != nil {
		t.Fatalf("future scheduling should not consume attack admission: %#v", plan.Admission)
	}
}

func TestRiftReplaySendsImmediateCommand(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Castles[1] = State.CastleState{ID: 1, X: 1, Y: 2, Focused: true}
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
	if len(plan.Steps) != 4 || plan.Steps[0].Opcode != "jca" || plan.Steps[0].AwaitOpcode != "jaa" ||
		plan.Steps[1].Action != "attack.inventory.guard" || plan.Steps[2].Opcode != "cra" || plan.Steps[2].Action != "" ||
		plan.Steps[3].Action != "attack.analytics.capture" {
		t.Fatalf("unexpected immediate plan: %#v", plan)
	}
	if plan.Admission == nil || plan.Admission.Class != Intent.AdmissionAttackLaunch || plan.Admission.Module != "riftReplay" {
		t.Fatalf("immediate Rift admission = %#v", plan.Admission)
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
		ID: 1, X: 7, Y: 8, Focused: true,
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
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &body); err != nil {
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

func TestAttackSetupAllowsThirtyWaves(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	unitID := int64(1)
	source := State.CastleState{
		ID:    1,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{1: 31}},
	}
	wave := attackSetupWaveRequest{Middle: attackSetupLaneRequest{
		Troops: []attackSetupSlotRequest{{ItemID: &unitID, Quantity: 1}},
	}}
	setup := attackSetupRequest{Waves: make([]attackSetupWaveRequest, 30)}
	for index := range setup.Waves {
		setup.Waves[index] = wave
	}
	if built, err := buildAttackSetupWaves(setup, source, gameData); err != nil || len(built) != 30 {
		t.Fatalf("30-wave setup: len=%d err=%v", len(built), err)
	}
	setup.Waves = append(setup.Waves, wave)
	if _, err := buildAttackSetupWaves(setup, source, gameData); err == nil || !strings.Contains(err.Error(), "between 1 and 30 waves") {
		t.Fatalf("31-wave setup error = %v", err)
	}
}

func TestRiftReplayBuildsOneCommandPerSelectedCommander(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Commanders[9] = State.CommanderState{ID: 9, Available: true}
	gameState.Castles[1] = State.CastleState{
		ID: 1, X: 7, Y: 8, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{1: 10}},
	}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", CommanderID: 5,
		Body: json.RawMessage(`{"LID":5,"SX":7,"SY":8,"TX":10,"TY":20,"KID":0,"A":[{"L":{"U":[[1,5]]}}]}`),
	}
	arguments := json.RawMessage(`{
		"launchId":"launch","sourceCastleId":1,
		"commanderSelection":{"candidates":[7,5,9],"count":2,"strategy":"first_available"}
	}`)
	plan, err := (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 6 || plan.Steps[0].Opcode != "jca" || plan.Steps[0].AwaitOpcode != "jaa" ||
		plan.Steps[1].Action != "attack.inventory.guard" || plan.Steps[2].Opcode != "cra" ||
		plan.Steps[3].Action != "attack.analytics.capture" || plan.Steps[4].Opcode != "cra" ||
		plan.Steps[5].Action != "attack.analytics.capture" {
		t.Fatalf("unexpected CRA setup steps: %#v", plan.Steps)
	}
	if plan.Admission == nil || plan.Admission.Module != "riftReplay" {
		t.Fatalf("Rift replay admission = %#v", plan.Admission)
	}
	for index, want := range []State.CommanderID{5, 9} {
		var body struct {
			CommanderID State.CommanderID `json:"LID"`
		}
		if err := json.Unmarshal(plan.Steps[index*2+2].Command.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.CommanderID != want {
			t.Fatalf("step %d commander = %d, want %d", index, body.CommanderID, want)
		}
	}
	for _, claim := range []string{"commander:7", "commander:5", "commander:9"} {
		if !containsString(plan.Claims, claim) {
			t.Fatalf("claims %#v do not contain %q", plan.Claims, claim)
		}
	}
}

func TestRiftReplayRejectsConflictingCommanderInputs(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", Body: json.RawMessage(`{"LID":5,"A":[{}]}`),
	}
	_, err := (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState},
		json.RawMessage(`{"launchId":"launch","commanderID":5,"commanderSelection":{"candidates":[5]}}`),
	)
	if err == nil || !strings.Contains(err.Error(), "mutually exclusive") {
		t.Fatalf("error = %v", err)
	}
}

func TestRiftReplayValidatesAttackSetupStockAcrossSelectedCommanders(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: true}
	gameState.Commanders[9] = State.CommanderState{ID: 9, Available: true}
	gameState.Castles[1] = State.CastleState{
		ID: 1, X: 7, Y: 8,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{1: 20}},
	}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", Body: json.RawMessage(`{"LID":5,"A":[{}]}`),
	}
	arguments := json.RawMessage(`{
		"launchId":"launch","sourceCastleId":1,
		"commanderSelection":{"candidates":[5,9],"count":2},
		"attackSetup":{"waves":[{
			"L":{"troops":[{"itemId":1,"quantity":11}],"tools":[]},
			"M":{"troops":[],"tools":[]},"R":{"troops":[],"tools":[]}
		}]}
	}`)
	_, err = (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err == nil || !strings.Contains(err.Error(), "2 commander(s) require 22") {
		t.Fatalf("error = %v", err)
	}
}

func TestRiftReplayCanScheduleBusyCommanderCandidates(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[5] = State.CommanderState{ID: 5, Available: false}
	gameState.Rift.Launches["launch"] = State.RiftLaunch{
		ID: "launch", CommanderID: 5, OneWayTTSeconds: 120,
		Body: json.RawMessage(`{"LID":5,"SX":1,"SY":2,"A":[{}]}`),
	}
	arguments, _ := json.Marshal(map[string]any{
		"launchId":           "launch",
		"commanderSelection": map[string]any{"candidates": []int{5}, "count": 1},
		"arriveAtUnix":       time.Now().UTC().Add(20 * time.Minute).Unix(),
	})
	plan, err := (&Application{}).planRiftReplay(
		context.Background(), Intent.PlanningContext{State: gameState}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Action != "operation.schedule" {
		t.Fatalf("unexpected scheduled plan: %#v", plan)
	}
}

func TestMaidenWaveUsesSelectedEligibleCommanders(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":1}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, SlotType: 1, KingdomID: 0, X: 7, Y: 8, Focused: true,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{1: 99}},
	}
	gameState.Map[0] = map[string]State.MapObservation{
		"rift": {KingdomID: 0, X: 10, Y: 20, TypeID: riftMapTypeID},
	}
	for _, commander := range []State.CommanderState{
		{ID: 7, Available: false},
		{ID: 5, Available: true},
		{ID: 9, Available: true},
	} {
		gameState.Commanders[commander.ID] = commander
		gameState.Inventory.Equipment[State.EquipmentInstanceID(commander.ID)] = State.EquipmentInstance{
			ID: State.EquipmentInstanceID(commander.ID), RarityID: 5,
			WearerID: int64(commander.ID), WearerKind: "commander",
			Effects: State.EquipmentEffects{{WireID: maidenSupportEffectID, Values: []float64{500}}},
		}
	}
	arguments := json.RawMessage(`{
		"unitWodID":1,
		"commanderSelection":{"candidates":[7,5,9],"count":2,"strategy":"first_available"}
	}`)
	plan, err := planMaidenCommsWave(
		context.Background(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 6 || plan.Steps[0].Opcode != "jca" || plan.Steps[0].AwaitOpcode != "jaa" ||
		plan.Steps[1].Action != "attack.inventory.guard" || plan.Steps[2].Opcode != "cra" ||
		plan.Steps[3].Action != "attack.analytics.capture" || plan.Steps[4].Opcode != "cra" ||
		plan.Steps[5].Action != "attack.analytics.capture" {
		t.Fatalf("unexpected CRA setup steps: %#v", plan.Steps)
	}
	if plan.Admission == nil || plan.Admission.Module != "riftMaiden" {
		t.Fatalf("maiden admission = %#v", plan.Admission)
	}
	for index, want := range []State.CommanderID{5, 9} {
		var body struct {
			CommanderID State.CommanderID `json:"LID"`
		}
		if err := json.Unmarshal(plan.Steps[index*2+2].Command.Payload, &body); err != nil {
			t.Fatal(err)
		}
		if body.CommanderID != want {
			t.Fatalf("step %d commander = %d, want %d", index, body.CommanderID, want)
		}
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

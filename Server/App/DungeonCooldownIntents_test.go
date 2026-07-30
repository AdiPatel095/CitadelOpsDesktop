package App

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestDungeonMinuteSkipUsesOneMS6ForThreeHourRBCCooldown(t *testing.T) {
	gameData := dungeonMinuteSkipGameData(t)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.Currencies[1006] = 4
	gameState.Map[0] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 0, TypeID: kingdomTowerMapTypeID, X: 101, Y: 102,
			TowerVictoryCount: 845, TowerCooldownRemaining: 10_800, ObservedAt: now,
		},
	}
	arguments := json.RawMessage(`{"kingdomId":0,"targetTypeId":2,"targetX":101,"targetY":102,"minimumRemaining":{"MS6":2}}`)
	input := Intent.PlanningContext{State: gameState, GameData: gameData}
	plan, err := planDungeonMinuteSkip(t.Context(), input, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Resolver != "nomad.cooldown.minute_skip.build" ||
		plan.Steps[0].AwaitOpcode != "msd" || plan.Steps[1].Action != "nomad.cooldown.minute_skip.verify" {
		t.Fatalf("unexpected minute-skip plan: %#v", plan.Steps)
	}
	step, err := resolveDungeonMinuteSkipStep(t.Context(), input, plan.Steps[0].ResolverArguments)
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		MinuteSkip string `json:"MST"`
		KingdomID  string `json:"KID"`
		X          int    `json:"X"`
		Y          int    `json:"Y"`
		MapID      int    `json:"MID"`
		NodeID     int    `json:"NID"`
	}
	if err := json.Unmarshal(step.Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if step.Opcode != "msd" || payload.MinuteSkip != "MS6" || payload.KingdomID != "0" ||
		payload.X != 101 || payload.Y != 102 || payload.MapID != -1 || payload.NodeID != -1 {
		t.Fatalf("unexpected msd command: step=%#v payload=%#v", step, payload)
	}

	gameState.Player.Currencies[1006] = 2
	if _, err := planDungeonMinuteSkip(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments); err == nil {
		t.Fatal("minute skip ignored the configured MS6 reserve")
	}
}

func TestDungeonMinuteSkipAcceptsType35KhanTarget(t *testing.T) {
	gameData := dungeonMinuteSkipGameData(t)
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Player.Currencies[1006] = 1
	gameState.Map[0] = map[string]State.MapObservation{
		"939:1123": {
			KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123,
			EventCampID: 1146, EventCampCooldownRemaining: 194, ObservedAt: now,
		},
	}
	arguments := json.RawMessage(`{
		"kingdomId":0,"targetTypeId":35,"targetX":939,"targetY":1123,
		"eventCampId":1146,"minimumRemaining":{}
	}`)
	plan, err := planDungeonMinuteSkip(t.Context(), Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Resolver != "nomad.cooldown.minute_skip.build" {
		t.Fatalf("unexpected Khan cooldown plan: %#v", plan)
	}
}

func TestKhanCooldownReportsAttachEveryMSDUntilCooldownClears(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Khan.CooldownReports[101] = State.KhanCooldownReportState{
		ReportID: 101, KingdomID: 0, X: 939, Y: 1123,
		LandedAt: now.Add(-time.Second), CooldownRemaining: 700, CooldownObservedAt: now,
	}
	application := &Application{State: State.NewStore(gameState)}
	first := dungeonMinuteSkipVerification{
		dungeonMinuteSkipRequest: dungeonMinuteSkipRequest{
			KingdomID: 0, TargetTypeID: khanCampTypeID, TargetX: 939, TargetY: 1123,
			KhanReportIDs: []int64{101},
		},
		StartedAt: now, InitialRemaining: 700, MSDWireKey: "MS3", MSDMinutes: 10,
	}
	if err := application.completeKhanCooldownReports(first, State.MapObservation{
		KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123, ObservedAt: now.Add(time.Second),
	}, 100); err != nil {
		t.Fatal(err)
	}
	state := application.State.Snapshot()
	report := state.Khan.CooldownReports[101]
	if !report.ResolvedAt.IsZero() || len(report.MSDs) != 1 ||
		report.MSDs[0].WireKey != "MS3" || report.MSDs[0].CooldownBefore != 700 ||
		report.MSDs[0].CooldownAfter != 100 || state.Khan.CooldownsSkipped != 1 {
		t.Fatalf("partially skipped report = %#v, Khan=%#v", report, state.Khan)
	}

	second := first
	second.StartedAt = now.Add(2 * time.Second)
	second.InitialRemaining = 100
	second.MSDWireKey = "MS2"
	second.MSDMinutes = 5
	if err := application.completeKhanCooldownReports(second, State.MapObservation{
		KingdomID: 0, TypeID: khanCampTypeID, X: 939, Y: 1123, ObservedAt: now.Add(3 * time.Second),
	}, 0); err != nil {
		t.Fatal(err)
	}
	state = application.State.Snapshot()
	report = state.Khan.CooldownReports[101]
	if report.ResolvedAt.IsZero() || len(report.MSDs) != 2 ||
		report.MSDs[1].WireKey != "MS2" || report.MSDs[1].CooldownAfter != 0 ||
		state.Khan.CooldownsSkipped != 2 {
		t.Fatalf("resolved report with attached MSDs = %#v, Khan=%#v", report, state.Khan)
	}
}

func dungeonMinuteSkipGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"currencies":[
			{"currencyID":1001,"JSONKey":"MS1"},{"currencyID":1002,"JSONKey":"MS2"},
			{"currencyID":1003,"JSONKey":"MS3"},{"currencyID":1004,"JSONKey":"MS4"},
			{"currencyID":1005,"JSONKey":"MS5"},{"currencyID":1006,"JSONKey":"MS6"},
			{"currencyID":1007,"JSONKey":"MS7"}
		],
		"currencyMinutesSkipValues":[
			{"currencyID":"1001","MinutesSkipValue":"1"},{"currencyID":"1002","MinutesSkipValue":"5"},
			{"currencyID":"1003","MinutesSkipValue":"10"},{"currencyID":"1004","MinutesSkipValue":"30"},
			{"currencyID":"1005","MinutesSkipValue":"60"},{"currencyID":"1006","MinutesSkipValue":"300"},
			{"currencyID":"1007","MinutesSkipValue":"1440"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

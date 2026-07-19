package Ingest

import (
	"encoding/json"
	"testing"
	"time"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestRegularEventCampMapRowParsesProgressionAndDefense(t *testing.T) {
	gameData := nomadReducerGameData(t)
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC)
	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":0,"AI":[[29,101,102,123,9,321,0,0,5001,11,22,33]]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("regular camp map snapshot: changed=%t err=%v", changed, err)
	}
	observation := gameState.Map[0]["101:102"]
	if observation.TypeID != 29 || observation.ObjectID != 5001 || observation.EventCampID != 5001 ||
		observation.EventCampVictoryCount != 9 || observation.EventCampCooldownRemaining != 321 || observation.Level != 90 ||
		observation.EventCampBaseWallBonus != 11 || observation.EventCampBaseGateBonus != 22 ||
		observation.EventCampBaseMoatBonus != 33 || observation.OwnerID != 0 || !observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected regular camp observation: %#v", observation)
	}
}

func TestCooldownResetUsesLockedGreatEmpireCampAndClearsPendingRefresh(t *testing.T) {
	gameData := nomadReducerGameData(t)
	gameState := State.NewGameState()
	resetAt := time.Date(2026, 7, 14, 12, 5, 0, 0, time.UTC)
	gameState.Map[0] = map[string]State.MapObservation{
		"101:102": {KingdomID: 0, X: 101, Y: 102, TypeID: 29, EventCampID: 5001, EventCampCooldownRemaining: 321},
	}
	gameState.Map[1] = map[string]State.MapObservation{
		"101:102": {KingdomID: 1, X: 101, Y: 102, TypeID: 2, TowerVictoryCount: 99},
	}
	gameState.NomadCamps.LockedTarget = &State.NomadCampTargetState{KingdomID: 0, TypeID: 29, X: 101, Y: 102, EventCampID: 5001}
	gameState.NomadCamps.Cooldowns["0:101:102"] = State.NomadCampCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: resetAt.Add(-time.Minute), PendingCooldownRefresh: true,
	}
	code := 0
	_, changed, err := reduceDungeonCooldownSkip(t.Context(), Protocol.Frame{
		Opcode: "sdc", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: resetAt,
		Payload: json.RawMessage(`{"AI":[29,101,102,123,9,0,0,0,5001,11,22,33]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("regular camp cooldown reset: changed=%t err=%v", changed, err)
	}
	updated := gameState.Map[0]["101:102"]
	if updated.EventCampID != 5001 || updated.EventCampCooldownRemaining != 0 || !updated.ObservedAt.Equal(resetAt) {
		t.Fatalf("Great Empire camp was not refreshed: %#v", updated)
	}
	if gameState.Map[1]["101:102"].TypeID != 2 {
		t.Fatalf("same-coordinate camp in another kingdom was overwritten: %#v", gameState.Map[1]["101:102"])
	}
	cooldown := gameState.NomadCamps.Cooldowns["0:101:102"]
	if cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 0 || !cooldown.CooldownObservedAt.Equal(resetAt) {
		t.Fatalf("cooldown state was not authoritatively cleared: %#v", cooldown)
	}
}

func TestDungeonMinuteSkipAcceptsAndClearsRBCTowerRow(t *testing.T) {
	gameState := State.NewGameState()
	resetAt := time.Now().UTC()
	gameState.Map[0] = map[string]State.MapObservation{
		"101:102": {
			KingdomID: 0, X: 101, Y: 102, TypeID: towerMapTypeID,
			TowerVictoryCount: 845, TowerCooldownRemaining: 321,
		},
	}
	gameState.NomadCamps.RBCTest = &State.NomadRBCTestState{
		RunID: "test", KingdomID: 0, TargetX: 101, TargetY: 102,
		ExpectedAttacks: 2, AttacksLaunched: 1,
	}
	gameState.TowerCooldowns["0:101:102"] = State.TowerCooldownState{
		KingdomID: 0, X: 101, Y: 102, LastSuccessfulBattleAt: resetAt.Add(-time.Minute), PendingCooldownRefresh: true,
	}
	code := 0
	_, changed, err := reduceDungeonCooldownSkip(t.Context(), Protocol.Frame{
		Opcode: "msd", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: resetAt,
		Payload: json.RawMessage(`{"KID":0,"AI":[2,101,102,-1,845,0,0]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("RBC cooldown reset: changed=%t err=%v", changed, err)
	}
	updated := gameState.Map[0]["101:102"]
	if updated.TypeID != towerMapTypeID || updated.TowerVictoryCount != 845 || updated.TowerCooldownRemaining != 0 ||
		!updated.ObservedAt.Equal(resetAt) {
		t.Fatalf("unexpected reset RBC row: %#v", updated)
	}
	cooldown := gameState.TowerCooldowns["0:101:102"]
	if cooldown.PendingCooldownRefresh || cooldown.CooldownRemaining != 0 || !cooldown.CooldownObservedAt.Equal(resetAt) {
		t.Fatalf("RBC cooldown state was not authoritatively cleared: %#v", cooldown)
	}
	if gameState.NomadCamps.RBCTest.CooldownsSkipped != 1 || !gameState.NomadCamps.RBCTest.LastCooldownSkippedAt.Equal(resetAt) {
		t.Fatalf("RBC trial did not record the landed-hit cooldown clear: %#v", gameState.NomadCamps.RBCTest)
	}
}

func nomadReducerGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":5001,"eventID":80,"difficultyID":201,"areaType":29,
			"camplevel":90,"countVictory":9,"coolDown":3600,"skipCosts":9950,"maxTroopCapacityDefense":620
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

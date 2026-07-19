package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestAutoInvasionPolicyWaitsForEnoughCapacityAdjustedInventory(t *testing.T) {
	now := time.Date(2026, 7, 15, 20, 0, 0, 0, time.UTC)
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"units":[{"wodID":216}],"buildings":[],"effects":[],"legendskills":[],
		"eventAutoScalingDifficulties":[
			{"difficultyID":8,"eventID":71,"difficultyTypeID":1,"isLocked":0},
			{"difficultyID":108,"eventID":103,"difficultyTypeID":1,"isLocked":0}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Castles[1] = State.CastleState{
		ID: 1, Name: "Main", KingdomID: 0, X: 100, Y: 100,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{216: 1}},
	}
	gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.EventScores.ActiveEventID = foreignLordsEventID
	gameState.EventScores.ByEvent[foreignLordsEventID] = State.ScalableEventScore{
		EventID: foreignLordsEventID, DifficultyID: 8, RemainingSec: 7_200, ObservedAt: now,
	}
	gameState.Invasion.LastScannedAt[1] = now
	gameState.Map[0] = map[string]State.MapObservation{
		"101:100": {
			KingdomID: 0, TypeID: foreignLordsMapTypeID, X: 101, Y: 100,
			ObjectID: 70, Level: 70, ObservedAt: now,
		},
	}
	snapshot := Snapshot{
		State: gameState, GameData: gameData, Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoInvasion": json.RawMessage(`{
				"version":1,"sourceCastleId":1,"presetId":"trial",
				"foreignLordsDifficultyId":8,"bloodcrowDifficultyId":108,
				"scoreTarget":5000000,"minimumRemainingSec":1800,
				"checkIntervalSec":30,"mapRefreshIntervalSec":300
			}`),
			"attacks.presets": json.RawMessage(`{
				"version":1,"presets":[{"id":"trial","name":"Trial","waves":[{
					"L":{"troops":[],"tools":[]},
					"M":{"troops":[{"itemId":216,"quantity":1000}],"tools":[]},
					"R":{"troops":[],"tools":[]}
				}]}]
			}`),
		}},
	}

	decision, err := NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "Waiting for attack inventory") {
		t.Fatalf("invasion shortage decision: %#v err=%v", decision, err)
	}

	castle := snapshot.State.Castles[1]
	castle.Units.Stationed[216] = 2_000
	snapshot.State.Castles[1] = castle
	decision, err = NewAutoInvasionPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "invasion.attack" {
		t.Fatalf("invasion ready decision: %#v err=%v", decision, err)
	}
}

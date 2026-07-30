package Ingest

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCastleResourceValuesTrackFoodConsumptionAndPreserveRates(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	castle := newCastleState(100)
	values := map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(`{
		"F":1000,"MEAD":2000,"BEEF":3000,
		"gpa":{"DF":500,"DFC":120,"FCR":60,"DMEAD":800,"DMEADC":350,"MEADCR":70,"DBEEF":0,"DBEEFC":40,"BEEFCR":100,"MRF":5000}
	}`), &values); err != nil {
		t.Fatal(err)
	}
	applyCastleResourceValues(values, &castle, gameData)
	food := castle.Resources[5]
	if food.ProductionPerHour == nil || *food.ProductionPerHour != 50 || food.ConsumptionPerHour == nil || *food.ConsumptionPerHour != 12 || food.ConsumptionMultiplier == nil || *food.ConsumptionMultiplier != 0.6 || food.Capacity == nil || *food.Capacity != 5000 {
		t.Fatalf("unexpected food balance: %#v", food)
	}
	mead := castle.Resources[12]
	if mead.ProductionPerHour == nil || *mead.ProductionPerHour != 80 || mead.ConsumptionPerHour == nil || *mead.ConsumptionPerHour != 35 || mead.ConsumptionMultiplier == nil || *mead.ConsumptionMultiplier != 0.7 {
		t.Fatalf("unexpected mead balance: %#v", mead)
	}
	beef := castle.Resources[13]
	if beef.ConsumptionPerHour == nil || *beef.ConsumptionPerHour != 4 || beef.ConsumptionMultiplier == nil || *beef.ConsumptionMultiplier != 1 {
		t.Fatalf("unexpected beef balance: %#v", beef)
	}

	values = map[string]json.RawMessage{}
	if err := json.Unmarshal([]byte(`{"F":900}`), &values); err != nil {
		t.Fatal(err)
	}
	applyCastleResourceValues(values, &castle, gameData)
	food = castle.Resources[5]
	if food.Amount != 900 || food.ProductionPerHour == nil || *food.ProductionPerHour != 50 || food.ConsumptionPerHour == nil || *food.ConsumptionPerHour != 12 || food.ConsumptionMultiplier == nil || *food.ConsumptionMultiplier != 0.6 {
		t.Fatalf("resource update discarded economy rates: %#v", food)
	}
}

func TestFocusedCastleProductionReducerAppliesStandaloneGPA(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":5,"JSONKey":"F"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Focused = true
	castle.Resources[5] = State.ResourceBalance{Amount: 100}
	gameState.Castles[100] = castle
	code := 0
	_, changed, err := reduceResponseResources(t.Context(), Protocol.Frame{
		Opcode: "gpa", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"DF":500,"DFC":120,"FCR":60}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("reduce gpa: changed=%t err=%v", changed, err)
	}
	balance := gameState.Castles[100].Resources[5]
	if balance.ProductionPerHour == nil || *balance.ProductionPerHour != 50 || balance.ConsumptionPerHour == nil || *balance.ConsumptionPerHour != 12 || balance.ConsumptionMultiplier == nil || *balance.ConsumptionMultiplier != 0.6 {
		t.Fatalf("unexpected gpa balance: %#v", balance)
	}
}

func TestCastleResourceReducerAppliesStandaloneGRC(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":1}],
		"resources":[{"resourceID":3,"JSONKey":"W"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	castle := newCastleState(100)
	castle.Resources[3] = State.ResourceBalance{Amount: 50_000}
	gameState.Castles[100] = castle
	code := 0
	_, changed, err := reduceResponseResources(t.Context(), Protocol.Frame{
		Opcode: "grc", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"AID":100,"KID":1,"W":14999}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("reduce grc: changed=%t err=%v", changed, err)
	}
	if got := gameState.Castles[100].Resources[3].Amount; got != 14_999 {
		t.Fatalf("standalone grc balance = %.0f, want 14999", got)
	}
}

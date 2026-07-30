package Automation

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriPolicyDeclaresStateDependencies(t *testing.T) {
	if got, want := NewBeriPolicy().WakeDomains(),
		[]string{"beri", "castles", "currencies", "kingdom-transport", "units"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("wake domains = %v, want %v", got, want)
	}
}

func TestBeriPolicyRefreshesThenTransfersExactCapacity(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 1,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 50}},
	}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	gameState.Player.Currencies[1005] = 1
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoBeriWorld": json.RawMessage(`{"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,"minTroopsToTransfer":20,"troopSpaceCheckIntervalSec":30}`),
	}}
	policy := NewBeriPolicy()
	gameData := beriPolicyGameData(t)

	decision, err := policy.Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("capacity refresh does not safely continue the workflow: %#v", decision)
	}

	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
	}
	decision, err = policy.Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: gameData, Now: now.Add(time.Second),
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("transfer decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("transfer does not safely continue the workflow: %#v", decision)
	}
	var arguments map[string]any
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil || arguments["amount"] != float64(25) ||
		arguments["targetCastleId"] != float64(900) {
		t.Fatalf("transfer did not freeze exact capacity: %s", decision.Request.Arguments)
	}
}

func TestBeriPolicyAutoDetectsOwnedBerimondCamp(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"transferTroopId":10,"sourceCastleId":100,"troopSpaceCheckIntervalSec":30}`),
	}}
	decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{State: gameState, Configuration: configuration, Now: now})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CastleID State.CastleID `json:"beriCastleId"`
	}
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil || arguments.CastleID != 900 {
		t.Fatalf("refresh arguments = %s", decision.Request.Arguments)
	}
}

func TestBeriPolicyWaitsForPendingTransportToSettle(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 1,
	}}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100}`),
	}}
	decision, err := NewBeriPolicy().Evaluate(
		t.Context(), Snapshot{State: gameState, Configuration: configuration, Now: now},
	)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("pending transport decision: %#v err=%v", decision, err)
	}
}

func TestBeriPolicyWaitsBeforeSubmittingUnlaunchableTransfer(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 1,
		Units:           State.CastleUnits{Stationed: map[State.UnitID]int64{10: 10}},
		UnitsObservedAt: now,
	}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	gameState.Player.Currencies[1005] = 1
	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100}`),
	}}
	snapshot := Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now.Add(time.Second),
	}
	decision, err := NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("insufficient donor decision: %#v err=%v", decision, err)
	}

	source := snapshot.State.Castles[100]
	source.Units.Stationed[10] = 50
	snapshot.State.Castles[100] = source
	snapshot.State.Player.Currencies[1005] = 0
	decision, err = NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("missing skip decision: %#v err=%v", decision, err)
	}
}

func TestBeriPolicyRejectsConfiguredNonBerimondCastle(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"beriCastleId":100,"transferTroopId":10,"sourceCastleId":100}`),
	}}
	decision, err := NewBeriPolicy().Evaluate(
		t.Context(), Snapshot{State: gameState, Configuration: configuration, Now: now},
	)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("invalid configured camp decision: %#v err=%v", decision, err)
	}
}

func beriPolicyGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

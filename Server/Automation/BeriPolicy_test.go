package Automation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriPolicyDeclaresStateDependencies(t *testing.T) {
	if got, want := NewBeriPolicy().WakeDomains(),
		[]string{"beri", "boosters", "castles", "currencies", "kingdom-transport", "units"}; !reflect.DeepEqual(got, want) {
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
		arguments["targetCastleId"] != float64(900) || arguments["useTimeSkip"] != false {
		t.Fatalf("transfer did not freeze exact capacity: %s", decision.Request.Arguments)
	}
}

func TestBeriPolicyRejectsMeadAndBeefTransferTroops(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],
		"units":[
			{"wodID":10,"foodSupply":1},
			{"wodID":11,"meadSupply":1},
			{"wodID":12,"beefSupply":1}
		],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	for _, unitID := range []int{11, 12} {
		t.Run(fmt.Sprintf("unit %d", unitID), func(t *testing.T) {
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				autoBeriWorldSection: json.RawMessage(`{"transferTroopId":` + fmt.Sprint(unitID) + `}`),
			}}
			decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{
				State: State.NewGameState(), Configuration: configuration, GameData: gameData, Now: now,
			})
			if err != nil {
				t.Fatal(err)
			}
			if decision.Request != nil || decision.Status != "waiting" ||
				!strings.Contains(decision.Detail, "Mead and Beef troops are not eligible") {
				t.Fatalf("unit %d decision = %#v", unitID, decision)
			}
		})
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
	decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
	})
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
		t.Context(), Snapshot{
			State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
		},
	)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("pending transport decision: %#v err=%v", decision, err)
	}
}

func TestBeriPolicyChainsTwelveFiveMinuteSkipsOneAtATimeAfterFallbackRefresh(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 3600,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 25}},
	}}
	gameState.Player.Currencies[1002] = 12
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{
			"useTroopTransportTimeSkips":true,
			"troopTransportTimeSkipId":"ms2"
		}`),
	}}
	policy := NewBeriPolicy()
	snapshot := Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
	}

	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("early fallback decision: %#v err=%v", decision, err)
	}
	if want := now.Add(time.Minute); !decision.NextCheckAt.Equal(want) {
		t.Fatalf("fallback check = %s, want %s", decision.NextCheckAt, want)
	}

	snapshot.Now = now.Add(time.Minute)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.refresh" {
		t.Fatalf("fallback refresh decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("fallback refresh does not recheck committed transport state: %#v", decision)
	}

	snapshot.State.KingdomTransport.ObservedAt = now.Add(61 * time.Second)
	snapshot.State.KingdomTransport.PendingUnits[0].RemainingSec = 3600
	snapshot.Now = now.Add(61 * time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.skip" {
		t.Fatalf("selected fallback skip decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("selected skip does not continue response-by-response: %#v", decision)
	}

	skipCommands := 0
	for {
		if decision.Request == nil || decision.Request.Name != "troops.kingdom.skip" {
			break
		}
		skipCommands++
		var arguments struct {
			TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
			TimeSkipID       string          `json:"timeSkipId"`
			MinimumRemaining int64           `json:"minimumRemaining"`
		}
		if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
			t.Fatal(err)
		}
		if arguments.TargetKingdomID != 10 || arguments.TimeSkipID != "MS2" || arguments.MinimumRemaining != 0 {
			t.Fatalf("skip %d arguments = %#v", skipCommands, arguments)
		}
		snapshot.State.Player.Currencies[1002]--
		snapshot.State.KingdomTransport.ObservedAt =
			snapshot.State.KingdomTransport.ObservedAt.Add(time.Second)
		snapshot.State.KingdomTransport.PendingUnits[0].RemainingSec =
			max(0, snapshot.State.KingdomTransport.PendingUnits[0].RemainingSec-300)
		snapshot.Now = snapshot.State.KingdomTransport.ObservedAt
		decision, err = policy.Evaluate(t.Context(), snapshot)
		if err != nil {
			t.Fatal(err)
		}
	}
	if skipCommands != 12 {
		t.Fatalf("selected five-minute skip commands = %d, want 12; terminal decision=%#v", skipCommands, decision)
	}
	if decision.Request == nil || decision.Request.Name != "troops.kingdom.refresh" {
		t.Fatalf("settled skip chain did not confirm arrival: %#v", decision)
	}
}

func TestBeriPolicyStopsSkipChainWithoutConfirmedTimerReduction(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 600,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 25}},
	}}
	gameState.Player.Currencies[1002] = 2
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{
			"useTroopTransportTimeSkips":true,
			"troopTransportTimeSkipId":"MS2"
		}`),
	}}
	policy := NewBeriPolicy()
	snapshot := Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
	}
	if _, err := policy.Evaluate(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}
	snapshot.Now = now.Add(time.Minute)
	if decision, err := policy.Evaluate(t.Context(), snapshot); err != nil ||
		decision.Request == nil || decision.Request.Name != "troops.kingdom.refresh" {
		t.Fatalf("fallback refresh decision: %#v err=%v", decision, err)
	}
	snapshot.State.KingdomTransport.ObservedAt = now.Add(61 * time.Second)
	snapshot.State.KingdomTransport.PendingUnits[0].RemainingSec = 539
	snapshot.Now = now.Add(61 * time.Second)
	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.skip" {
		t.Fatalf("first skip decision: %#v err=%v", decision, err)
	}

	snapshot.State.Player.Currencies[1002]--
	snapshot.State.KingdomTransport.ObservedAt = now.Add(62 * time.Second)
	snapshot.State.KingdomTransport.PendingUnits[0].RemainingSec = 538
	snapshot.Now = now.Add(62 * time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "did not produce a confirmed timer reduction") {
		t.Fatalf("unconfirmed skip chain decision: %#v err=%v", decision, err)
	}
	if !decision.NextCheckAt.Equal(snapshot.Now.Add(time.Minute)) {
		t.Fatalf("unconfirmed skip retry = %s, want one minute", decision.NextCheckAt)
	}
}

func TestBeriPolicyContinuesSelectedSkipImmediatelyAfterTransfer(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	source := State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 1, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 50}},
	}
	gameState.Castles[source.ID] = source
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	gameState.Player.Currencies[1002] = 12
	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
	}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{
			"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,
			"useTroopTransportTimeSkips":true,"troopTransportTimeSkipId":"MS2"
		}`),
	}}
	policy := NewBeriPolicy()
	snapshot := Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now.Add(time.Second),
	}
	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("transfer decision: %#v err=%v", decision, err)
	}

	snapshot.State.KingdomTransport.ObservedAt = now.Add(2 * time.Second)
	snapshot.State.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 3300,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 25}},
	}}
	snapshot.State.Player.Currencies[1002] = 11
	snapshot.Now = now.Add(2 * time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.skip" ||
		!decision.ReevaluateOnSuccess {
		t.Fatalf("post-transfer skip continuation: %#v err=%v", decision, err)
	}
	if decision.Metrics["timeSkipsNeeded"] != 11 {
		t.Fatalf("post-transfer selected skips needed = %v, want 11", decision.Metrics["timeSkipsNeeded"])
	}
}

func TestBeriPolicyDoesNotRetrySkipWhenFallbackRefreshClearsTransport(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 600,
		Units: []State.KingdomTransportUnit{{UnitID: 10, Amount: 25}},
	}}
	gameState.Player.Currencies[1003] = 2
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{
			"useTroopTransportTimeSkips":true,
			"troopTransportTimeSkipId":"MS3"
		}`),
	}}
	policy := NewBeriPolicy()
	snapshot := Snapshot{
		State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
	}
	if _, err := policy.Evaluate(t.Context(), snapshot); err != nil {
		t.Fatal(err)
	}

	snapshot.Now = now.Add(time.Minute)
	decision, err := policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.refresh" {
		t.Fatalf("fallback refresh decision: %#v err=%v", decision, err)
	}

	snapshot.State.KingdomTransport.ObservedAt = now.Add(61 * time.Second)
	snapshot.State.KingdomTransport.PendingUnits = nil
	snapshot.Now = now.Add(61 * time.Second)
	decision, err = policy.Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request != nil {
		t.Fatalf("settled fallback decision retried a skip: %#v err=%v", decision, err)
	}
}

func TestBeriPolicyRefreshesTransportThatFinishedBeforeFallbackCheck(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.KingdomTransport.ObservedAt = now
	gameState.KingdomTransport.PendingUnits = []State.KingdomUnitTransport{{
		KingdomID: 10, RemainingSec: 30,
	}}
	decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{
		State: gameState, Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			autoBeriWorldSection: json.RawMessage(`{"useTroopTransportTimeSkips":true,"troopTransportTimeSkipId":"MS3"}`),
		}},
		GameData: beriPolicyGameData(t), Now: now.Add(30 * time.Second),
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "troops.kingdom.refresh" {
		t.Fatalf("settled transport refresh decision: %#v err=%v", decision, err)
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
		autoBeriWorldSection: json.RawMessage(`{
			"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,
			"useTroopTransportTimeSkips":true,"troopTransportTimeSkipId":"MS5"
		}`),
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
	decision, err = NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("selected launch skip decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		UseTimeSkip bool   `json:"useTimeSkip"`
		TimeSkipID  string `json:"timeSkipId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil {
		t.Fatal(err)
	}
	if !arguments.UseTimeSkip || arguments.TimeSkipID != "MS5" {
		t.Fatalf("selected launch skip arguments = %#v", arguments)
	}

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
		t.Context(), Snapshot{
			State: gameState, Configuration: configuration, GameData: beriPolicyGameData(t), Now: now,
		},
	)
	if err != nil || decision.Request != nil || decision.Status != "waiting" {
		t.Fatalf("invalid configured camp decision: %#v err=%v", decision, err)
	}
}

func beriPolicyGameData(t *testing.T) *GameData.Store {
	t.Helper()
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10,"foodSupply":1}],
		"currencies":[
			{"currencyID":1002,"JSONKey":"MS2"},
			{"currencyID":1003,"JSONKey":"MS3"},
			{"currencyID":1005,"JSONKey":"MS5"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return gameData
}

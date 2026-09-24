package Automation

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestBeriPolicyDeclaresStateDependencies(t *testing.T) {
	if got, want := NewBeriPolicy().WakeDomains(),
		[]string{"beri", "boosters", "castles", "currencies", "events", "event-scores", "kingdom-transport", "units"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("wake domains = %v, want %v", got, want)
	}
}

func TestBeriPolicyRefreshesThenTransfersExactCapacity(t *testing.T) {
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{
		ID: 100, KingdomID: 0, SlotType: 1, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 50}},
	}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoBeriWorld": json.RawMessage(`{"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,"minTroopsToTransfer":20,"troopSpaceCheckIntervalSec":30}`),
	}}
	policy := NewBeriPolicy()
	gameData := beriPolicyGameData(t)

	decision, err := policy.Evaluate(t.Context(), Snapshot{
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: gameData, Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}
	if !decision.ReevaluateOnSuccess || !decision.ReevaluateOnStale {
		t.Fatalf("capacity refresh does not safely continue the workflow: %#v", decision)
	}
	var refreshArguments struct {
		BeriCastleID   State.CastleID `json:"beriCastleId"`
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if json.Unmarshal(decision.Request.Arguments, &refreshArguments) != nil ||
		refreshArguments.BeriCastleID != 900 || refreshArguments.SourceCastleID != 100 {
		t.Fatalf("capacity refresh did not bind the selected donor: %s", decision.Request.Arguments)
	}

	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
	}
	decision, err = policy.Evaluate(t.Context(), Snapshot{
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: gameData, Now: now.Add(time.Second),
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
	for _, unitID := range []int{11, 12} {
		t.Run(fmt.Sprintf("unit %d", unitID), func(t *testing.T) {
			id := int64(unitID)
			preset := AttackPresets.Preset{Waves: []AttackPresets.Wave{{Middle: AttackPresets.Lane{
				Troops: []AttackPresets.Slot{{ItemID: &id, Quantity: 1}},
			}}}}
			_, _, reason := beriProportionalTransfer(preset,
				map[State.UnitID]int64{State.UnitID(unitID): 10}, nil,
				State.BeriState{AvailableTroops: 10}, gameData)
			if !strings.Contains(reason, "not Food-fed") {
				t.Fatalf("unit %d reason = %q", unitID, reason)
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
	})
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("refresh decision: %#v err=%v", decision, err)
	}
	var arguments struct {
		CastleID       State.CastleID `json:"beriCastleId"`
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if json.Unmarshal(decision.Request.Arguments, &arguments) != nil ||
		arguments.CastleID != 900 || arguments.SourceCastleID != 100 {
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
			State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now.Add(time.Second),
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
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
		State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now.Add(time.Second),
	}
	decision, err := NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("partial donor transfer decision: %#v err=%v", decision, err)
	}
	var partial struct {
		Amount int64 `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &partial); err != nil || partial.Amount != 10 {
		t.Fatalf("partial donor transfer = %s err=%v", decision.Request.Arguments, err)
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

func TestBeriPolicyRefreshesStaleInsufficientSelectedDonor(t *testing.T) {
	now := time.Date(2026, 8, 26, 21, 40, 39, 0, time.UTC)
	for _, test := range []struct {
		name      string
		stationed int64
	}{
		{name: "zero", stationed: 0},
		{name: "insufficient", stationed: 10},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Castles[100] = State.CastleState{
				ID: 100, KingdomID: 0, SlotType: 1,
				Units:           State.CastleUnits{Stationed: map[State.UnitID]int64{10: test.stationed}},
				UnitsObservedAt: now.Add(-time.Minute),
			}
			gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
			gameState.Beri = State.BeriState{
				AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
			}
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				autoBeriWorldSection: json.RawMessage(`{
					"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,
					"troopSpaceCheckIntervalSec":30
				}`),
			}}

			decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{
				State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" ||
				decision.Status != "ready" || !strings.Contains(decision.Detail, "donor troops") {
				t.Fatalf("stale donor decision: %#v err=%v", decision, err)
			}
		})
	}
}

func TestBeriPolicyRefreshesUncurrentSelectedDonorWithCachedCapacity(t *testing.T) {
	now := time.Date(2026, 8, 26, 21, 45, 0, 0, time.UTC)
	for _, test := range []struct {
		name            string
		unitsObservedAt time.Time
	}{
		{name: "stale", unitsObservedAt: now.Add(-time.Minute)},
		{name: "never observed"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Castles[100] = State.CastleState{
				ID: 100, KingdomID: 0, SlotType: 1,
				Units:           State.CastleUnits{Stationed: map[State.UnitID]int64{10: 50}},
				UnitsObservedAt: test.unitsObservedAt,
			}
			gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
			gameState.Beri = State.BeriState{
				AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25}, ObservedAt: now,
			}
			configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
				autoBeriWorldSection: json.RawMessage(`{
					"beriCastleId":900,"transferTroopId":10,"sourceCastleId":100,
					"troopSpaceCheckIntervalSec":30
				}`),
			}}

			decision, err := NewBeriPolicy().Evaluate(t.Context(), Snapshot{
				State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
			})
			if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" ||
				decision.Status != "ready" || !strings.Contains(decision.Detail, "donor troops") {
				t.Fatalf("uncurrent donor decision: %#v err=%v", decision, err)
			}
		})
	}
}

func TestBeriPolicyIgnoresLegacyConfiguredCastle(t *testing.T) {
	now := time.Date(2026, 7, 28, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: 10}
	configuration := Configuration.Snapshot{Sections: map[string]json.RawMessage{
		autoBeriWorldSection: json.RawMessage(`{"beriCastleId":100,"transferTroopId":10,"sourceCastleId":100}`),
	}}
	decision, err := NewBeriPolicy().Evaluate(
		t.Context(), Snapshot{
			State: beriPolicyWithFreshCamp(gameState, now), Configuration: beriPolicyWithPreset(configuration), GameData: beriPolicyGameData(t), Now: now,
		},
	)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.capacity.refresh" {
		t.Fatalf("legacy configured camp blocked discovery: %#v err=%v", decision, err)
	}
	var arguments struct {
		BeriCastleID State.CastleID `json:"beriCastleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.BeriCastleID != 900 {
		t.Fatalf("capacity refresh did not use the owned camp: %s err=%v", decision.Request.Arguments, err)
	}
}

func TestBeriPolicyTransfersProportionalBatchBelowCapacityTrigger(t *testing.T) {
	now := time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC)
	state := State.NewGameState()
	state.Castles[100] = State.CastleState{ID: 100, KingdomID: 0, SlotType: 1, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 2000, 11: 2000}}}
	state.Castles[900] = State.CastleState{ID: 900, KingdomID: 10, UnitsObservedAt: now,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{}}}
	state.Beri = State.BeriState{AvailableTroops: 1000, ObservedAt: now}
	configuration := Configuration.Snapshot{Revision: 7, Sections: map[string]json.RawMessage{
		autoBeriWorldSection:               json.RawMessage(`{"presetId":"mixed","sourceCastleId":100,"minTroopsToTransfer":1000}`),
		AttackPresets.ConfigurationSection: json.RawMessage(`{"version":1,"presets":[{"id":"mixed","name":"Mixed","waves":[{"M":{"troops":[{"itemId":10,"quantity":2}]},"R":{"troops":[{"itemId":11,"quantity":1}]}}]}]}`),
	}}
	gameData := beriCompositionGameData(t)
	snapshot := Snapshot{State: state, Configuration: configuration, GameData: gameData, Now: now.Add(time.Second)}
	decision, err := NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "beri.transfer" {
		t.Fatalf("proportional transfer decision = %#v err=%v", decision, err)
	}
	var request struct {
		UnitID   State.UnitID `json:"unitId"`
		Amount   int64        `json:"amount"`
		Revision uint64       `json:"configurationRevision"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || request.UnitID != 10 || request.Amount != 667 || request.Revision != 7 {
		t.Fatalf("proportional request = %s err=%v", decision.Request.Arguments, err)
	}
	castle := snapshot.State.Castles[900]
	castle.UnitsObservedAt = time.Time{}
	snapshot.State.Castles[900] = castle
	decision, err = NewBeriPolicy().Evaluate(t.Context(), snapshot)
	if err != nil || decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("stale camp inventory decision = %#v err=%v", decision, err)
	}
	var refresh struct {
		CastleID State.CastleID `json:"castleId"`
		Refresh  bool           `json:"refresh"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &refresh); err != nil || refresh.CastleID != 900 || !refresh.Refresh {
		t.Fatalf("stale camp refresh request = %s err=%v", decision.Request.Arguments, err)
	}
}

func beriPolicyWithPreset(configuration Configuration.Snapshot) Configuration.Snapshot {
	var settings map[string]any
	_ = json.Unmarshal(configuration.Sections[autoBeriWorldSection], &settings)
	if settings == nil {
		settings = map[string]any{}
	}
	unitID := int64(10)
	if legacy, ok := settings["transferTroopId"].(float64); ok && legacy > 0 {
		unitID = int64(legacy)
	}
	settings["presetId"] = "beri-test"
	section, _ := json.Marshal(settings)
	oldSections := configuration.Sections
	configuration.Sections = map[string]json.RawMessage{}
	for key, value := range oldSections {
		configuration.Sections[key] = value
	}
	configuration.Sections[autoBeriWorldSection] = section
	configuration.Sections[AttackPresets.ConfigurationSection] = json.RawMessage(fmt.Sprintf(`{"version":1,"presets":[{"id":"beri-test","name":"Beri Test","waves":[{"M":{"troops":[{"itemId":%d,"quantity":25}]}}]}]}`, unitID))
	return configuration
}

func beriPolicyWithFreshCamp(gameState State.GameState, now time.Time) State.GameState {
	for id, castle := range gameState.Castles {
		if castle.KingdomID != 10 {
			continue
		}
		castle.UnitsObservedAt = now
		gameState.Castles[id] = castle
	}
	return gameState
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

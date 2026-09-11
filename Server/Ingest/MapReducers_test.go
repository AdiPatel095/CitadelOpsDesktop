package Ingest

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackCapacity"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestReduceMapSnapshotParsesKingdomTowerVictoryLevelAndCooldown(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 12, 15, 0, 0, 0, time.UTC)
	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":0,"AI":[[2,210,942,-1,845,2759,0],[2,211,942,-1,845,-50,0]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("map snapshot: changed=%t err=%v", changed, err)
	}
	active := gameState.Map[0]["210:942"]
	if active.TowerVictoryCount != 845 || active.Level != 81 || active.TowerCooldownRemaining != 2759 || !active.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected active tower: %#v", active)
	}
	ready := gameState.Map[0]["211:942"]
	if ready.TowerVictoryCount != 845 || ready.Level != 81 || ready.TowerCooldownRemaining != 0 {
		t.Fatalf("unexpected ready tower: %#v", ready)
	}
}

func TestReduceMapSnapshotTracksInvasionLevelAndPeaceMode(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	protectedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: protectedAt,
		Payload: json.RawMessage(`{"KID":0,"AI":[[21,101,102,70,-1,1],[34,103,104,80,-1,1]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("protected invasion map: changed=%t err=%v", changed, err)
	}
	for _, target := range []struct {
		x, y  int
		level int
	}{{101, 102, 70}, {103, 104, 80}} {
		observation := gameState.Map[0][fmt.Sprintf("%d:%d", target.x, target.y)]
		if observation.Level != target.level || observation.ObjectID != int64(target.level) ||
			!gameState.Invasion.TargetUnavailable(0, target.x, target.y) {
			t.Fatalf("protected invasion target %d:%d = %#v, state=%#v", target.x, target.y, observation, gameState.Invasion)
		}
	}
	if !slices.Contains(domains, "map-invasion") || !slices.Contains(domains, "invasion") {
		t.Fatalf("protected invasion domains = %v", domains)
	}

	availableAt := protectedAt.Add(time.Minute)
	domains, changed, err = reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: availableAt,
		Payload: json.RawMessage(`{"KID":0,"AI":[[21,101,102,70,-1,0],[34,103,104,80,-1,0]]}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.Invasion.TargetUnavailable(0, 101, 102) ||
		gameState.Invasion.TargetUnavailable(0, 103, 104) || !slices.Contains(domains, "invasion") {
		t.Fatalf("available invasion map: domains=%v state=%#v changed=%t err=%v", domains, gameState.Invasion, changed, err)
	}

	_, changed, err = reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: availableAt.Add(time.Minute),
		Payload: json.RawMessage(`{"KID":0,"AI":[
			[21,105,106,70,-1,"bad"],[34,107,108,80],[21,109,110,70,-1,0.9],
			[21,111,112,70,-1,"0"],[21,113,114,70,-1,"1"]
		]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("unconfirmed invasion map: changed=%t err=%v", changed, err)
	}
	for _, key := range []string{"105:106", "107:108", "109:110", "111:112", "113:114"} {
		if observation := gameState.Map[0][key]; observation.InvasionAvailabilityKnown {
			t.Fatalf("malformed or missing peace mode became authoritative: %s=%#v", key, observation)
		}
	}

	_, changed, err = reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: availableAt.Add(2 * time.Minute),
		Payload: json.RawMessage(`{"KID":0,"AI":[
			[21.5,101,102,70,-1,1],[21,101.5,102,70,-1,1],[21,101,102.5,70,-1,1],
			["bad",101,102,70,-1,1],[21,9223372036854775808,102,70,-1,1]
		]}`),
	}, &gameState, nil)
	if err == nil || changed {
		t.Fatalf("malformed required GAA fields were accepted: changed=%t err=%v", changed, err)
	}
	if observation, found := gameState.LookupMapObservation(0, "101:102"); !found ||
		observation.TypeID != State.MapTypeForeignLord || observation.InvasionProtected ||
		gameState.Invasion.TargetUnavailable(0, 101, 102) {
		t.Fatalf("malformed GAA row replaced valid target: %#v found=%t", observation, found)
	}
	for _, key := range []string{"0:102", "101:0"} {
		if _, found := gameState.LookupMapObservation(0, key); found {
			t.Fatalf("malformed GAA row created coordinate %s", key)
		}
	}
}

func TestReduceMapSnapshotRejectsInvalidKingdomID(t *testing.T) {
	code := 0
	for _, test := range []struct {
		name    string
		payload json.RawMessage
	}{
		{name: "missing", payload: json.RawMessage(`{"AI":[[21,101,102,70,-1,0]]}`)},
		{name: "null", payload: json.RawMessage(`{"KID":null,"AI":[[21,101,102,70,-1,0]]}`)},
		{name: "string", payload: json.RawMessage(`{"KID":"0","AI":[[21,101,102,70,-1,0]]}`)},
		{name: "fractional", payload: json.RawMessage(`{"KID":0.5,"AI":[[21,101,102,70,-1,0]]}`)},
		{name: "overflow", payload: json.RawMessage(`{"KID":9223372036854775808,"AI":[[21,101,102,70,-1,0]]}`)},
		{name: "negative", payload: json.RawMessage(`{"KID":-1,"AI":[[21,101,102,70,-1,0]]}`)},
	} {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
				Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code,
				ReceivedAt: time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC), Payload: test.payload,
			}, &gameState, nil)
			if err == nil || changed || len(gameState.Map) != 0 || len(gameState.Invasion.UnavailableTargets) != 0 {
				t.Fatalf("invalid GAA kingdom was accepted or mutated state: changed=%t err=%v map=%#v invasion=%#v", changed, err, gameState.Map, gameState.Invasion)
			}
		})
	}
}

func TestReduceMapSnapshotRejectsNullAIWithoutMutatingTargets(t *testing.T) {
	observedAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	prior := State.MapObservation{
		KingdomID: 0, TypeID: State.MapTypeForeignLord, X: 101, Y: 102,
		ObjectID: 70, Level: 70, InvasionAvailabilityKnown: true, ObservedAt: observedAt.Add(-time.Minute),
	}
	gameState := State.NewGameState()
	gameState.Map[0] = map[string]State.MapObservation{"101:102": prior}
	gameState.Invasion.LastScannedAt[1] = observedAt.Add(-time.Minute)
	code := 0
	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":0,"AI":null}`),
	}, &gameState, nil)
	if err == nil || changed {
		t.Fatalf("null GAA array: changed=%t err=%v", changed, err)
	}
	if retained, found := gameState.LookupMapObservation(0, "101:102"); !found || retained != prior {
		t.Fatalf("null GAA mutated prior target: %#v found=%t", retained, found)
	}
	if !gameState.Invasion.LastScannedAt[1].Equal(observedAt.Add(-time.Minute)) {
		t.Fatalf("null GAA advanced scan clock: %s", gameState.Invasion.LastScannedAt[1])
	}
}

func TestReduceHiddenInvasionTargetsPersistsUntilNewerMapReappearance(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 101, 102)] = "STO"
	code := 0
	hiddenAt := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	gameState.Invasion.ReserveTarget(State.InvasionTargetReservation{
		KingdomID: 0, TargetTypeID: State.MapTypeForeignLord, X: 101, Y: 102,
		OperationID: "cooldown-95", ReservedAt: hiddenAt.Add(-time.Second),
	})
	domains, changed, err := reduceHiddenInvasionTargets(t.Context(), Protocol.Frame{
		Opcode: "hac", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: hiddenAt,
		Payload: json.RawMessage(`{"CP":[[101,102],[103,104]]}`),
	}, &gameState, nil)
	if err != nil || !changed || !gameState.Invasion.TargetUnavailable(0, 101, 102) ||
		!gameState.Invasion.TargetUnavailable(0, 103, 104) ||
		len(gameState.Invasion.FortifiedTargets) != 0 ||
		len(gameState.Invasion.TargetReservations) != 1 ||
		!slices.Contains(domains, "invasion") || !slices.Contains(domains, "map-invasion") {
		t.Fatalf("hac reduction: domains=%v state=%#v changed=%t err=%v", domains, gameState.Invasion, changed, err)
	}

	_, changed, err = reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: hiddenAt.Add(time.Minute),
		Payload: json.RawMessage(`{"KID":0,"AI":[[21,101,102,70,-1,0]]}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.Invasion.TargetUnavailable(0, 101, 102) ||
		!gameState.Invasion.TargetUnavailable(0, 103, 104) || len(gameState.Invasion.TargetReservations) != 1 {
		t.Fatalf("selective GAA restoration: state=%#v changed=%t err=%v", gameState.Invasion, changed, err)
	}
	if reservation, found := gameState.Invasion.TargetReservation(0, 101, 102); !found || reservation.OperationID != "cooldown-95" {
		t.Fatalf("GAA alone cleared the no-replay reservation: %#v", reservation)
	}

	_, changed, err = reduceHiddenInvasionTargets(t.Context(), Protocol.Frame{
		Opcode: "hac", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: hiddenAt.Add(30 * time.Second),
		Payload: json.RawMessage(`{"CP":[[101,102]]}`),
	}, &gameState, nil)
	if err != nil || changed || gameState.Invasion.TargetUnavailable(0, 101, 102) {
		t.Fatalf("older HAC overrode newer available GAA: changed=%t err=%v invasion=%#v", changed, err, gameState.Invasion)
	}

	_, changed, err = reduceHiddenInvasionTargets(t.Context(), Protocol.Frame{
		Opcode: "hac", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: hiddenAt.Add(2 * time.Minute),
		Payload: json.RawMessage(`{"CP":[["bad",102],[null,103],[104,"oops"],[-1,105],[101.9,102]]}`),
	}, &gameState, nil)
	if err != nil || changed || gameState.Invasion.TargetUnavailable(0, 0, 102) ||
		gameState.Invasion.TargetUnavailable(0, 0, 103) || gameState.Invasion.TargetUnavailable(0, 104, 0) ||
		gameState.Invasion.TargetUnavailable(0, 101, 102) {
		t.Fatalf("malformed HAC coordinates mutated state: changed=%t err=%v invasion=%#v", changed, err, gameState.Invasion)
	}

	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil || !registry.HasInbound("hac") {
		t.Fatalf("hac reducer registration: registered=%t err=%v", registry.HasInbound("hac"), err)
	}
}

func TestReduceMapSnapshotEmitsOnlyChangedFeatureDomains(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ReceivedAt: time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC),
		Payload:    json.RawMessage(`{"KID":0,"AI":[[2,10,10,-1,1,0,0],[43,20,20,99]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("mixed map snapshot: changed=%t err=%v", changed, err)
	}
	for _, expected := range []string{"map-tower", "map-rift"} {
		if !slices.Contains(domains, expected) {
			t.Fatalf("targeted map domains = %v, missing %s", domains, expected)
		}
	}
	if slices.Contains(domains, "map") || slices.Contains(domains, "map-storm") || slices.Contains(domains, "map-event-camp") {
		t.Fatalf("mixed snapshot emitted unrelated map domains: %v", domains)
	}
}

func TestNestedMapReducersOwnMapAndPlayerIndependently(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	for _, opcode := range []string{"fnm", "fnt", "ssi"} {
		registered := registry.registered(opcode, Protocol.DirectionInbound)
		if len(registered.steps) != 2 {
			t.Fatalf("%s reducer steps = %d, want 2", opcode, len(registered.steps))
		}
		if !registered.steps[0].writes.Has(State.ComponentWorldMap) ||
			registered.steps[0].writes.Has(State.ComponentPlayer) {
			t.Fatalf("%s map writes = %v", opcode, registered.steps[0].writes.List())
		}
		if registered.steps[1].writes != State.Components(State.ComponentPlayer) {
			t.Fatalf("%s player writes = %v", opcode, registered.steps[1].writes.List())
		}
	}
}

func TestCooperativeStormTileEmitsNonWakingProgressDomain(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		ResponseToken: "shared/storm-gaa/20260813/4", ReceivedAt: time.Date(2026, time.August, 13, 12, 0, 0, 0, time.UTC),
		Payload: json.RawMessage(`{"KID":4,"AI":[[25,610,610,1,-1,10,0,0,0]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("cooperative Storm tile: changed=%t err=%v", changed, err)
	}
	if !slices.Contains(domains, "storm-scan-progress") || slices.Contains(domains, "map") ||
		slices.Contains(domains, "map-storm") || slices.Contains(domains, "storm") {
		t.Fatalf("cooperative Storm domains = %v", domains)
	}
}

func TestReduceNestedMapSnapshotParsesCapturedKhanCamp(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[],
		"units":[],
		"eventAutoScalingCamps":[{
			"eventAutoScalingCampID":1145,"eventID":72,"difficultyID":310,"areaType":35,
			"camplevel":105,"coolDown":300,"maxTroopCapacityDefense":4530
		}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 25, 13, 37, 59, 0, time.UTC)
	_, changed, err := reduceNestedMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "fnm", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"X":939,"Y":1123,
			"gaa":{"KID":0,"AI":[[35,939,1123,-1,-1,0,0,0,1,1145,200,200,85]]}
		}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("Khan map jump: changed=%t err=%v", changed, err)
	}
	observation := gameState.Map[0]["939:1123"]
	if observation.TypeID != khanCampMapTypeID || observation.ObjectID != 1145 ||
		observation.EventCampID != 1145 || observation.Level != 105 ||
		observation.EventCampBaseWallBonus != 200 || observation.EventCampBaseGateBonus != 200 ||
		observation.EventCampBaseMoatBonus != 85 || observation.OwnerID != 0 ||
		!observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected Khan camp observation: %#v", observation)
	}
}

func TestReduceNestedMapSnapshotCapturesBerimondFindNextTower(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 28, 15, 0, 0, 0, time.UTC)
	domains, changed, err := reduceNestedMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "fnt", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"X":1472,"Y":29,
			"gaa":{"KID":10,"AI":[[17,1472,29,-410,0,[],-1,55,12070,60]]}
		}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Berimond target: changed=%t err=%v", changed, err)
	}
	if gameState.Beri.TargetX != 1472 || gameState.Beri.TargetY != 29 ||
		gameState.Beri.TargetTypeID != 17 || !gameState.Beri.TargetObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected Berimond target: %#v", gameState.Beri)
	}
	target := gameState.Map[10]["1472:29"]
	if target.TypeID != AttackCapacity.BerimondTowerMapTypeID || target.ObjectID != -410 ||
		target.Level != 55 || !target.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected Berimond tower observation: %#v", target)
	}
	if !slices.Contains(domains, "beri") {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestReduceMapSnapshotInvalidatesUnavailableBerimondTarget(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	selectedAt := time.Date(2026, 7, 29, 13, 40, 0, 0, time.UTC)
	observedAt := selectedAt.Add(time.Minute)
	gameState.Beri.TargetX = 1489
	gameState.Beri.TargetY = 28
	gameState.Beri.TargetTypeID = AttackCapacity.BerimondTowerMapTypeID
	gameState.Beri.TargetObservedAt = selectedAt

	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: selectedAt.Add(30 * time.Second),
		Payload: json.RawMessage(`{"KID":10,"AI":[[17,1489,28,-410,0,[],-1,60,23556,61]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("available Berimond target: changed=%t err=%v", changed, err)
	}
	if !gameState.Beri.TargetInvalidatedAt.IsZero() {
		t.Fatalf("available Berimond target was invalidated: %#v", gameState.Beri)
	}

	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":10,"AI":[[17,1489,28,-410,1,[],-1,60,1,61]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("unavailable Berimond target: changed=%t err=%v", changed, err)
	}
	if !gameState.Beri.TargetInvalidatedAt.Equal(observedAt) {
		t.Fatalf("Berimond target was not invalidated: %#v", gameState.Beri)
	}
	if !slices.Contains(domains, "beri") {
		t.Fatalf("domains = %#v", domains)
	}
}

func TestReduceMapSnapshotParsesCapturedKhanCooldown(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Date(2026, 7, 25, 14, 34, 0, 0, time.UTC)
	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{
			"KID":0,
			"AI":[[35,939,1123,3352,-1,194,360,1825,17,1146,200,200,85]]
		}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("Khan cooldown snapshot: changed=%t err=%v", changed, err)
	}
	observation := gameState.Map[0]["939:1123"]
	if observation.TypeID != khanCampMapTypeID || observation.EventCampID != 1146 ||
		observation.EventCampCooldownRemaining != 194 || !observation.ObservedAt.Equal(observedAt) {
		t.Fatalf("unexpected Khan cooldown observation: %#v", observation)
	}
}

func TestMapObservationMigratesLegacyTowerMapValueAsVictoryCount(t *testing.T) {
	var observation State.MapObservation
	if err := json.Unmarshal([]byte(`{"typeId":2,"towerBaseFlankCapacity":845}`), &observation); err != nil {
		t.Fatal(err)
	}
	if observation.TowerVictoryCount != 845 {
		t.Fatalf("legacy tower victory count = %d", observation.TowerVictoryCount)
	}
}

func TestMapObservationDerivesStormOpportunityTimes(t *testing.T) {
	observedAt := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	var fort State.MapObservation
	if err := json.Unmarshal([]byte(`{
		"typeId":25,"stormIsleId":10,"stormCooldownRemaining":300,"observedAt":"2026-07-15T16:20:00Z"
	}`), &fort); err != nil {
		t.Fatal(err)
	}
	if !fort.StormReadyAt().Equal(observedAt.Add(300 * time.Second)) {
		t.Fatalf("derived fort readyAt = %s", fort.StormReadyAt())
	}
	var island State.MapObservation
	if err := json.Unmarshal([]byte(`{
		"typeId":24,"ownerId":-403,"stormIsleId":4,"stormCooldownRemaining":3600,"observedAt":"2026-07-15T16:20:00Z"
	}`), &island); err != nil {
		t.Fatal(err)
	}
	if !island.StormReadyAt().Equal(observedAt) || !island.StormExpiresAt(0).Equal(observedAt.Add(time.Hour)) {
		t.Fatalf("derived island times = %#v", island)
	}
}

func TestMapObservationOmitsEmptyStormOpportunityLabels(t *testing.T) {
	raw, err := json.Marshal(State.MapObservation{KingdomID: 0, X: 1, Y: 2, TypeID: 2})
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("stormReadyAt")) || bytes.Contains(raw, []byte("stormExpiresAt")) {
		t.Fatalf("empty Storm opportunity labels leaked into a non-Storm map row: %s", raw)
	}
}

func TestReduceMapSnapshotRetainsOfficialStormTimers(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"isles":[
			{"IsleID":4,"type":"VILLAGEWOOD","dungeonlevel":70,"globalCooldown":115200,"occupationTime":14400},
			{"IsleID":6,"type":"VILLAGEAQUAMARINE","dungeonlevel":70,"globalCooldown":115200,"occupationTime":21600},
			{"IsleID":10,"type":"DUNGEON","dungeonlevel":40,"globalCooldown":36000,"localCooldown":14400}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Storm.Map.Targets["104:105"] = State.MapObservation{
		KingdomID: 4, X: 104, Y: 105, TypeID: stormFortMapTypeID, StormIsleID: 99,
		ObservedAt: time.Date(2026, 7, 15, 15, 0, 0, 0, time.UTC),
	}
	code := 0
	observedAt := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	_, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"KID":4,"AI":[
			[24,100,101,3319,-403,0,0,0,4,100],
			[24,102,103,4447,12345,0,0,0,6,120],
			[25,104,105,4,-1,10,300,5,200]
		]}`),
	}, &gameState, gameData)
	if err != nil || !changed {
		t.Fatalf("Storm map snapshot: changed=%t err=%v", changed, err)
	}
	unoccupied := gameState.Map[4]["100:101"]
	if !unoccupied.StormReadyAt().Equal(observedAt) || !unoccupied.StormExpiresAt(115_200).Equal(observedAt.Add(100*time.Second)) {
		t.Fatalf("unoccupied island times = %#v", unoccupied)
	}
	occupied := gameState.Map[4]["102:103"]
	if !occupied.StormReadyAt().Equal(observedAt.Add(120*time.Second)) ||
		!occupied.StormExpiresAt(115_200).Equal(observedAt.Add((120+115_200)*time.Second)) {
		t.Fatalf("occupied island times = %#v", occupied)
	}
	fort := gameState.Map[4]["104:105"]
	if !fort.StormReadyAt().Equal(observedAt.Add(300*time.Second)) || !fort.StormExpiresAt(115_200).IsZero() {
		t.Fatalf("fort times = %#v", fort)
	}
	if tracked := gameState.Storm.Map.Targets["104:105"]; tracked != fort {
		t.Fatalf("tracked Storm fort was not refreshed from the newer map row: %#v", tracked)
	}
}

func TestInvasionFortificationTracksReceiptUntilServerCountersReset(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	_, changed, err := reduceInvasionFortification(t.Context(), Protocol.Frame{
		Opcode: "rae", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"XPOS":1165.0,"YPOS":1166.0,"RCK":"KT"}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.Invasion.FortifiedTargets["0:1165:1166"] != "KT" {
		t.Fatalf("fortification receipt: state=%#v changed=%t err=%v", gameState.Invasion, changed, err)
	}
	_, changed, err = reduceInvasionFortificationCounters(t.Context(), Protocol.Frame{
		Opcode: "rce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"RCSC":3,"RCHC":1}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.Invasion.FortifyResourceCount != 3 || gameState.Invasion.FortifyRubyCount != 1 || len(gameState.Invasion.FortifiedTargets) != 1 {
		t.Fatalf("active fortification counters: state=%#v changed=%t err=%v", gameState.Invasion, changed, err)
	}
	_, changed, err = reduceInvasionFortificationCounters(t.Context(), Protocol.Frame{
		Opcode: "rce", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"RCSC":0,"RCHC":0}`),
	}, &gameState, nil)
	if err != nil || !changed || len(gameState.Invasion.FortifiedTargets) != 0 {
		t.Fatalf("reset fortification counters: state=%#v changed=%t err=%v", gameState.Invasion, changed, err)
	}
}

func TestReduceMapSnapshotDropsRetainedObservationReplacedByUntrackedRow(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	first := time.Date(2026, 8, 18, 12, 0, 0, 0, time.UTC)
	// A Foreign Lord castle (type 21) is observed by a full scan.
	if _, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: first,
		Payload: json.RawMessage(`{"KID":0,"AI":[[21,1124,238,70,0,70,0,0,0,0,0,0,0,0,0,0,0,0,0,0]]}`),
	}, &gameState, nil); err != nil || !changed {
		t.Fatalf("initial invasion observation: changed=%t err=%v", changed, err)
	}
	observed, exists := gameState.LookupMapObservation(0, "1124:238")
	if !exists {
		t.Fatal("foreign lord castle was not retained")
	}
	if observed.Level != 70 || !observed.InvasionAvailabilityKnown || observed.InvasionProtected {
		t.Fatalf("captured trailing-field row did not retain official invasion semantics: %#v", observed)
	}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 1124, 238)] = "STO"
	// The castle is defeated; the coordinate now reports as AREA_TYPE_DYNAMIC
	// (31), which is not a tracked map kind. The stale type-21 observation
	// must go with it — otherwise it stays a phantom attack candidate that
	// every refresh silently re-confirms.
	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: first.Add(time.Minute),
		Payload: json.RawMessage(`{"KID":0,"AI":[[31,1124,238]]}`),
	}, &gameState, nil)
	if err != nil || !changed {
		t.Fatalf("dynamic-area replacement: changed=%t err=%v", changed, err)
	}
	if _, exists := gameState.LookupMapObservation(0, "1124:238"); exists {
		t.Fatal("phantom foreign lord castle survived the dynamic-area row")
	}
	if _, exists := gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 1124, 238)]; exists {
		t.Fatal("dynamic-area replacement retained stale invasion fortification")
	}
	if !slices.Contains(domains, "map-invasion") {
		t.Fatalf("invasion domain did not wake on the deletion: %v", domains)
	}
}

func TestReduceMapSnapshotClearsInvasionFortificationOnRetainedReplacement(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	first := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	if _, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: first,
		Payload: json.RawMessage(`{"KID":0,"AI":[[21,1124,238,70,-1,0]]}`),
	}, &gameState, nil); err != nil || !changed {
		t.Fatalf("initial invasion observation: changed=%t err=%v", changed, err)
	}
	gameState.Invasion.FortifiedTargets[State.InvasionTargetKey(0, 1124, 238)] = "STO"
	domains, changed, err := reduceMapSnapshot(t.Context(), Protocol.Frame{
		Opcode: "gaa", Direction: Protocol.DirectionInbound, ResponseCode: &code, ReceivedAt: first.Add(time.Minute),
		Payload: json.RawMessage(`{"KID":0,"AI":[[2,1124,238,-1,845,0,0]]}`),
	}, &gameState, nil)
	observation, exists := gameState.LookupMapObservation(0, "1124:238")
	if err != nil || !changed || !exists || observation.TypeID != 2 ||
		!slices.Contains(domains, "map-invasion") ||
		len(gameState.Invasion.FortifiedTargets) != 0 {
		t.Fatalf(
			"retained replacement: observation=%#v exists=%t domains=%v invasion=%#v changed=%t err=%v",
			observation, exists, domains, gameState.Invasion, changed, err,
		)
	}
}

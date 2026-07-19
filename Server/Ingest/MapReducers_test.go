package Ingest

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

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

func TestMapObservationMigratesLegacyTowerMapValueAsVictoryCount(t *testing.T) {
	var observation State.MapObservation
	if err := json.Unmarshal([]byte(`{"typeId":2,"towerBaseFlankCapacity":845}`), &observation); err != nil {
		t.Fatal(err)
	}
	if observation.TowerVictoryCount != 845 {
		t.Fatalf("legacy tower victory count = %d", observation.TowerVictoryCount)
	}
}

func TestMapObservationMigratesStormOpportunityLabels(t *testing.T) {
	observedAt := time.Date(2026, 7, 15, 16, 20, 0, 0, time.UTC)
	var fort State.MapObservation
	if err := json.Unmarshal([]byte(`{
		"typeId":25,"stormIsleId":10,"stormCooldownRemaining":300,"observedAt":"2026-07-15T16:20:00Z"
	}`), &fort); err != nil {
		t.Fatal(err)
	}
	if !fort.StormReadyAt.Equal(observedAt.Add(300 * time.Second)) {
		t.Fatalf("migrated fort readyAt = %s", fort.StormReadyAt)
	}
	var island State.MapObservation
	if err := json.Unmarshal([]byte(`{
		"typeId":24,"ownerId":-403,"stormIsleId":4,"stormCooldownRemaining":3600,"observedAt":"2026-07-15T16:20:00Z"
	}`), &island); err != nil {
		t.Fatal(err)
	}
	if !island.StormReadyAt.Equal(observedAt) || !island.StormExpiresAt.Equal(observedAt.Add(time.Hour)) {
		t.Fatalf("migrated island labels = %#v", island)
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

func TestReduceMapSnapshotLabelsStormReadyAndExpiryTimes(t *testing.T) {
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
	if !unoccupied.StormReadyAt.Equal(observedAt) || !unoccupied.StormExpiresAt.Equal(observedAt.Add(100*time.Second)) {
		t.Fatalf("unoccupied island labels = %#v", unoccupied)
	}
	occupied := gameState.Map[4]["102:103"]
	if !occupied.StormReadyAt.Equal(observedAt.Add(120*time.Second)) ||
		!occupied.StormExpiresAt.Equal(observedAt.Add((120+115_200)*time.Second)) {
		t.Fatalf("occupied island labels = %#v", occupied)
	}
	fort := gameState.Map[4]["104:105"]
	if !fort.StormReadyAt.Equal(observedAt.Add(300*time.Second)) || !fort.StormExpiresAt.IsZero() {
		t.Fatalf("fort labels = %#v", fort)
	}
}

func TestInvasionFortificationTracksReceiptUntilServerCountersReset(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	_, changed, err := reduceInvasionFortification(t.Context(), Protocol.Frame{
		Opcode: "rae", Direction: Protocol.DirectionInbound, ResponseCode: &code,
		Payload: json.RawMessage(`{"XPOS":1165.0,"YPOS":1166.0,"RCK":"STO"}`),
	}, &gameState, nil)
	if err != nil || !changed || gameState.Invasion.FortifiedTargets["0:1165:1166"] != "STO" {
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

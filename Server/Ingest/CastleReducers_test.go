package Ingest

import (
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestCastleSnapshotCompletesOnlyCurrentSessionBaseline(t *testing.T) {
	now := time.Date(2026, 7, 21, 19, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	gameState.Session = State.SessionState{
		Generation: 7, LoggedIn: true, SocketReady: true, ChangedAt: now.Add(-time.Second),
	}
	code := 0
	frame := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "jaa", ResponseCode: &code, ReceivedAt: now,
		Payload: json.RawMessage(`{"KID":0,"gca":{"A":[1,10,20,100]}}`),
	}
	domains, changed, err := reduceCastleSnapshot(t.Context(), frame, &gameState, nil)
	if err != nil || !changed || gameState.Session.BaselineGeneration != 7 || !slices.Contains(domains, "session") {
		t.Fatalf("current castle baseline: changed=%t domains=%v session=%+v err=%v", changed, domains, gameState.Session, err)
	}

	gameState.Session.Generation = 8
	gameState.Session.BaselineGeneration = 0
	gameState.Session.ChangedAt = now.Add(time.Second)
	domains, _, err = reduceCastleSnapshot(t.Context(), frame, &gameState, nil)
	if err != nil || gameState.Session.BaselineGeneration != 0 || slices.Contains(domains, "session") {
		t.Fatalf("stale castle snapshot completed newer baseline: domains=%v session=%+v err=%v", domains, gameState.Session, err)
	}
}

func TestCastleSnapshotKeepsConstructionSlots(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 42
	gameState.Castles[100] = newCastleState(100)
	payload := json.RawMessage(`{
		"KID":0,
		"gca":{
			"A":[1,10,20,100,42,0,0,0,0,0,"Main"],
			"O":{"OID":42,"N":"Player"},
			"BG":[[196,4000,1,2,0,0,0,0,0,0,0,0,0,0,8]],
			"BD":[[301,4001,3,4,0,4,8,100,-1,-1,0,0,0,0,1,-1]],
			"T":[[401,5000,0,0,0,0,0,0,0,0,0,0,0,0,1]],
			"scl":{"OIDL":[4000,-1,-2],"SSC":2},
			"CI":[{"OID":4000,"CIL":[{"CID":379,"S":0},{"CID":725,"S":1,"RS":60}]}]
		},
		"abpi":[{"OID":4000,"PA":{"MEAD":45}}],
		"gui":{"I":[[215,10]],"TU":[],"HI":[],"SHI":[]}
	}`)
	code := 0
	observedAt := time.Now().UTC()
	domains, changed, err := reduceCastleSnapshot(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "jaa", ResponseCode: &code,
		Payload: payload, ReceivedAt: observedAt,
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("castle snapshot did not report a change")
	}
	wantDomains := []string{"castles", "player", "alliance", "building-layout", "building-production", "building-construction", "construction-items", "units"}
	if !reflect.DeepEqual(domains, wantDomains) {
		t.Fatalf("castle snapshot domains = %v, want %v", domains, wantDomains)
	}
	castle := gameState.Castles[100]
	if castle.FoodStateObservedAt.IsZero() {
		t.Fatal("food state observation timestamp was not recorded")
	}
	if production := castle.BuildingProduction[4000]; production.PercentByResource["MEAD"] != 45 || !production.ObservedAt.Equal(observedAt) {
		t.Fatalf("building production was not retained: %#v", production)
	}
	if !castle.ConstructionSlotsObservedAt.Equal(observedAt) {
		t.Fatalf("construction slot observation = %v, want %v", castle.ConstructionSlotsObservedAt, observedAt)
	}
	if len(castle.ConstructionSlots) != 1 || len(castle.ConstructionSlots[4000]) != 2 {
		t.Fatalf("unexpected construction slots: %#v", castle.ConstructionSlots)
	}
	if castle.Buildings[4000].Layer != State.BuildingLayerBG || !castle.Buildings[4000].Placed {
		t.Fatalf("building layer was not retained: %#v", castle.Buildings[4000])
	}
	if len(castle.Layout.Objects) != 1 || len(castle.Layout.Ground) != 0 {
		t.Fatalf("unexpected normalized layout: %#v", castle.Layout)
	}
	if _, found := castle.Layout.Objects[4001]; found {
		t.Fatalf("completed disassembly remains in normalized layout: %#v", castle.Layout.Objects[4001])
	}
	if castle.Layout.Fixed[5000].Layer != State.BuildingLayerT {
		t.Fatalf("fixed structures were not retained: %#v", castle.Layout.Fixed)
	}
	if _, merged := castle.Buildings[5000]; merged {
		t.Fatalf("legacy BG+BD building map unexpectedly contains a fixed structure: %#v", castle.Buildings)
	}
	if len(castle.BuildingQueue.Slots) != 3 || castle.BuildingQueue.Slots[0].BuildingID != 4000 ||
		castle.BuildingQueue.Slots[1].Status != State.BuildingQueueSlotAvailable ||
		castle.BuildingQueue.Slots[2].Status != State.BuildingQueueSlotLocked {
		t.Fatalf("building queue was not parsed: %#v", castle.BuildingQueue)
	}
	if remaining := castle.ConstructionSlots[4000][1].RemainingSec; remaining == nil || *remaining != 60 {
		t.Fatalf("remaining seconds were not parsed: %#v", remaining)
	}
}

func TestConstructionSlotsSurviveLaterTransactions(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Castles[100] = newCastleState(100)
	gameState.Castles[100] = func(castle State.CastleState) State.CastleState {
		castle.ConstructionSlots[4000] = []State.ConstructionSlot{{DefinitionID: 379, Slot: 0}}
		return castle
	}(gameState.Castles[100])
	store := State.NewStore(gameState)
	pipeline := NewPipeline(store, nil, NewRegistry())
	if _, err := pipeline.HandleFrame(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "unknown", ReceivedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}
	if slots := store.Snapshot().Castles[100].ConstructionSlots[4000]; len(slots) != 1 {
		t.Fatalf("construction slots were lost after a later transaction: %#v", slots)
	}
}

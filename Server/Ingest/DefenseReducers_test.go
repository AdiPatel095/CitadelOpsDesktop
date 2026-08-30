package Ingest

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestDefenseContextMatchesCapturedFixture(t *testing.T) {
	payload, err := os.ReadFile(filepath.Join("..", "..", "Logs", "RecvCommandsJSON", "dfc.json"))
	if err != nil {
		if os.IsNotExist(err) {
			t.Skipf("captured DFC fixture is not available in this checkout: %v", err)
		}
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	castle := newCastleState(15246649)
	castle.Units.Stationed[9999] = 12
	gameState.Castles[castle.ID] = castle
	code := 0
	observedAt := time.Date(2026, time.July, 10, 12, 0, 0, 0, time.UTC)
	_, changed, err := reduceDefenseContext(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "dfc", ResponseCode: &code,
		Payload: payload, ReceivedAt: observedAt,
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("captured DFC fixture did not update defense state")
	}
	defense := gameState.Castles[castle.ID].Defense
	if len(defense.Wall.Middle.ToolSlots) != 6 || defense.Keep.MAUCT != 250 || defense.Moat.Defense != 191.6 {
		t.Fatalf("captured defense setup = %#v", defense)
	}
	if defense.CastellanID != 1 || defense.GateDefense != 316.1 || len(defense.Inventory) != 44 || defense.Inventory[315] != 2543 {
		t.Fatalf("captured defense context summary = %#v", defense)
	}
	if gameState.Castles[castle.ID].Units.Stationed[9999] != 12 {
		t.Fatalf("captured defense inventory replaced castle units: %#v", gameState.Castles[castle.ID].Units)
	}
}

func TestDefenseContextReducesCastleSetupAndInventory(t *testing.T) {
	gameState := State.NewGameState()
	castleState := newCastleState(100)
	castleState.Units.Stationed[900] = 77
	castleState.Units.Traveling[901] = 3
	gameState.Castles[100] = castleState
	observedAt := time.Date(2026, time.July, 13, 20, 13, 36, 0, time.UTC)
	code := 0
	payload := json.RawMessage(`{
		"A":[1,10,20,100,42,8,7,7,6,7,"Main"],
		"PR":[11,12],"PM":[21,22],
		"dfw":{
			"L":{"S":[[501,4],[-1,0]],"UP":25,"UC":70},
			"M":{"S":[[502,6]],"UP":50,"UC":60},
			"R":{"S":[[503,8]],"UP":25,"UC":30},
			"U":900,"US":875,"D":315.5
		},
		"dfk":{"S":[[504,2],[-1,0],[-1,0]],"STS":[[-1,0],[-1,0],[-1,0]],"MAUCT":973,"UC":50,"U":1010,"UYL":630500,"AUYL":258500},
		"dfm":{"LS":[[505,3]],"MS":[[506,4]],"RS":[[507,5]],"D":165.25},
		"gui":{"I":[[11,40],[501,9]],"TU":[[11,2]],"HI":[],"SHI":[]},
		"L":{"ID":7},"GD":300.0,"MDS":158.0,"RDS":188.0,"HDWL":4
	}`)
	domains, changed, err := reduceDefenseContext(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "dfc", ResponseCode: &code,
		Payload: payload, ReceivedAt: observedAt,
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("defense context did not report a change")
	}
	if want := []string{"castles", "defense"}; !reflect.DeepEqual(domains, want) {
		t.Fatalf("defense domains = %v, want %v", domains, want)
	}
	castle := gameState.Castles[100]
	if !castle.Defense.ObservedAt.Equal(observedAt) || !castle.Defense.InventoryObservedAt.Equal(observedAt) {
		t.Fatalf("observation timestamps were not retained: defense=%v inventory=%v", castle.Defense.ObservedAt, castle.Defense.InventoryObservedAt)
	}
	if castle.Defense.CastellanID != 7 || castle.Defense.GateDefense != 300 || castle.Defense.HDWL != 4 {
		t.Fatalf("defense summary = %#v", castle.Defense)
	}
	if castle.Defense.Wall.Left.UnitPercent != 25 || castle.Defense.Wall.Middle.UnitTypePercent != 60 ||
		castle.Defense.Wall.StationedUnits != 875 || castle.Defense.Wall.Defense != 315.5 {
		t.Fatalf("wall setup = %#v", castle.Defense.Wall)
	}
	if got := castle.Defense.Keep.PrimaryToolSlots; len(got) != 3 || got[0].DefinitionID != 504 || got[1].DefinitionID != -1 {
		t.Fatalf("keep tool slots = %#v", got)
	}
	if castle.Defense.Keep.MAUCT != 973 || castle.Defense.Keep.UnitTypePercent != 50 || castle.Defense.Keep.AUYL != 258500 {
		t.Fatalf("keep setup = %#v", castle.Defense.Keep)
	}
	if castle.Defense.Moat.MiddleToolSlots[0].DefinitionID != 506 || castle.Defense.Moat.Defense != 165.25 {
		t.Fatalf("moat setup = %#v", castle.Defense.Moat)
	}
	if castle.Defense.Inventory[11] != 40 || castle.Defense.Inventory[501] != 9 {
		t.Fatalf("defense inventory = %#v", castle.Defense.Inventory)
	}
	if castle.Units.Stationed[900] != 77 || castle.Units.Traveling[901] != 3 || len(castle.Units.Stationed) != 1 {
		t.Fatalf("defense inventory overwrote the complete castle units: %#v", castle.Units)
	}
	if !reflect.DeepEqual(castle.Defense.RangedUnitIDs, []State.UnitID{11, 12}) ||
		!reflect.DeepEqual(castle.Defense.MeleeUnitIDs, []State.UnitID{21, 22}) {
		t.Fatalf("eligible defense units = ranged %v melee %v", castle.Defense.RangedUnitIDs, castle.Defense.MeleeUnitIDs)
	}
}

func TestDirectDefenseKeepResponseUpdatesFocusedCastleOnly(t *testing.T) {
	gameState := State.NewGameState()
	focused := newCastleState(100)
	focused.Focused = true
	focused.Defense.Wall.Defense = 222
	focused.Defense.ObservedAt = time.Date(2026, time.July, 13, 19, 0, 0, 0, time.UTC)
	gameState.Castles[100] = focused
	gameState.Castles[200] = newCastleState(200)
	observedAt := time.Date(2026, time.July, 13, 20, 0, 0, 0, time.UTC)
	code := 0
	domains, changed, err := reduceDefenseKeep(context.Background(), Protocol.Frame{
		Direction: Protocol.DirectionInbound, Opcode: "dfk", ResponseCode: &code, ReceivedAt: observedAt,
		Payload: json.RawMessage(`{"S":[[-1,0],[-1,0],[-1,0]],"STS":[[-1,0],[-1,0],[-1,0]],"MAUCT":250,"UC":51,"U":657000,"UYL":980500,"AUYL":323500}`),
	}, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || !reflect.DeepEqual(domains, []string{"castles", "defense"}) {
		t.Fatalf("direct DFK result = domains %v changed %v", domains, changed)
	}
	updated := gameState.Castles[100]
	if updated.Defense.Keep.MAUCT != 250 || !updated.Defense.Keep.ObservedAt.Equal(observedAt) {
		t.Fatalf("focused keep = %#v", updated.Defense.Keep)
	}
	if updated.Defense.Wall.Defense != 222 || !updated.Defense.ObservedAt.Equal(focused.Defense.ObservedAt) {
		t.Fatalf("partial DFK overwrote full defense state: %#v", updated.Defense)
	}
	if !gameState.Castles[200].Defense.Keep.ObservedAt.IsZero() {
		t.Fatalf("unfocused castle was updated: %#v", gameState.Castles[200].Defense)
	}
}

func TestCoreRegistryIncludesDefenseReducers(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	for _, opcode := range []string{"dfc", "dfw", "dfk", "dfm"} {
		if !registry.HasInbound(opcode) {
			t.Errorf("missing %s reducer", opcode)
		}
	}
}

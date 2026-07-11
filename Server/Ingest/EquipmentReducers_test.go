package Ingest

import (
	"encoding/json"
	"testing"

	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestApplyLeadersNormalizesEquipmentAndCommanderZero(t *testing.T) {
	gameState := State.NewGameState()
	commanderID := State.CommanderID(0)
	gameState.Movements[12] = State.MovementState{ID: 12, CommanderID: &commanderID}
	gameState.Castles[99] = State.CastleState{ID: 99, Name: "Main"}
	payload := json.RawMessage(`{
		"C":[{"ID":0,"VIS":0,"N":"Rift1","EQ":[[1001,1,2,5,0,[[164,75,[510]]],1375,1086,20,-1,457,1,[1,0,0,[457,9,0,0,[[301,50,[22]]],12]]]]}],
		"B":[{"ID":1,"LICID":99,"N":"","EQ":[[2001,2,1,0,0,[[268,[175]]],1535,1101,3,-1,521,2]]}]
	}`)
	changed, err := applyLeaders(payload, &gameState)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("leader payload did not change state")
	}
	commander, ok := gameState.Commanders[0]
	if !ok {
		t.Fatal("commander id 0 was not retained")
	}
	if commander.Available {
		t.Fatal("commander with an active movement is marked available")
	}
	if commander.Name != "Rift1" || commander.VisiblePosition != 1 || commander.Equipment["1"] != 1001 {
		t.Fatalf("unexpected commander: %+v", commander)
	}
	equipment := gameState.Inventory.Equipment[1001]
	if equipment.DefinitionID != 1375 || equipment.Level != 20 || equipment.SetID != 1086 {
		t.Fatalf("unexpected equipment: %+v", equipment)
	}
	if values := equipment.Effects[164]; len(values) != 2 || values[0] != 75 || values[1] != 510 {
		t.Fatalf("equipment effects = %#v", equipment.Effects)
	}
	gem := gameState.Inventory.Gems[1001]
	if gem.DefinitionID != 457 || gem.Level != 12 || gem.Slot != 1 {
		t.Fatalf("unexpected gem: %+v", gem)
	}
	if values := gem.Effects[301]; len(values) != 2 || values[0] != 50 || values[1] != 22 {
		t.Fatalf("gem effects = %#v", gem.Effects)
	}
	castellan := gameState.Castellans[1]
	if castellan.CastleID != 99 || castellan.Name != "Main" || castellan.Equipment["2"] != 2001 {
		t.Fatalf("unexpected castellan: %+v", castellan)
	}
}

func TestConstructionInventoryUsesConstructionItemIDs(t *testing.T) {
	gameState := State.NewGameState()
	frame := testSuccessfulFrame("gii", `{"CI":[[42,3],[99,0]]}`)
	_, changed, err := reduceConstructionInventory(t.Context(), frame, &gameState, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed || gameState.Inventory.ConstructionItems[42] != 3 {
		t.Fatalf("construction inventory = %#v", gameState.Inventory.ConstructionItems)
	}
}

func testSuccessfulFrame(opcode string, payload string) Protocol.Frame {
	code := 0
	return Protocol.Frame{Opcode: opcode, ResponseCode: &code, Payload: json.RawMessage(payload)}
}

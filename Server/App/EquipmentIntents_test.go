package App

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func TestPlanEquipmentReconfigureUsesCanonicalLeaderAndInstanceIDs(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		currentID := State.EquipmentInstanceID(10 + slot)
		proposedID := State.EquipmentInstanceID(20 + slot)
		leader.Equipment[strconv.Itoa(slot)] = currentID
		gameState.Inventory.Equipment[currentID] = State.EquipmentInstance{ID: currentID, Slot: slot, TypeID: 2, WearerKind: "commander", WearerID: 0}
		gameState.Inventory.Equipment[proposedID] = State.EquipmentInstance{ID: proposedID, Slot: slot, TypeID: 2}
	}
	gameState.Commanders[0] = leader
	gameState.Inventory.Equipment[99] = State.EquipmentInstance{ID: 99, Slot: 1, TypeID: 2}
	gameState.Inventory.Gems[501] = State.GemInstance{ID: 501, EquipmentInstanceID: 99}

	arguments := json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":21,"2":22,"3":23,"4":24},
		"gems":{"1":501}
	}`)
	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Claims) != 2 || plan.Claims[1] != "leader:commander:0" {
		t.Fatalf("claims = %#v", plan.Claims)
	}
	opcodes := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		opcodes = append(opcodes, step.Opcode)
	}
	want := []string{"eeq", "eeq", "eeq", "eeq", "eeq", "ege", "eeq", "eeq", "eeq", "eeq", "eeq", "bge", "ggm", "gei", "gli"}
	if len(opcodes) != len(want) {
		t.Fatalf("opcodes = %#v", opcodes)
	}
	for index := range want {
		if opcodes[index] != want[index] {
			t.Fatalf("opcode %d = %q, want %q (%#v)", index, opcodes[index], want[index], opcodes)
		}
	}
}

func TestPlanEquipmentSellRequiresFreshStorageAndFreezesSelection(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	gameState.Observations["gei"] = State.ProtocolObservation{
		Opcode: "gei", LastDirection: "inbound", LastCode: &code, LastSeenAt: time.Now().UTC(),
	}
	gameState.Inventory.Equipment[10] = State.EquipmentInstance{ID: 10, DefinitionID: 100, Slot: 1, RarityID: 2}
	gameState.Inventory.Equipment[11] = State.EquipmentInstance{ID: 11, DefinitionID: 100, Slot: 1, RarityID: 5}
	plan, err := planEquipmentSell(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"category":"non_relic_equipment"}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Sell 1 item(s) from non_relic_equipment" || len(plan.Steps) != 3 || plan.Steps[0].Opcode != "seq" {
		t.Fatalf("plan = %#v", plan)
	}
}

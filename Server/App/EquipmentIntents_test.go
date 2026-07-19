package App

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
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

func TestPlanEquipmentReconfigureSkipsMatchingEquipment(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, WearerKind: "commander", WearerID: 0}
	}
	gameState.Commanders[0] = leader

	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":101,"2":102,"3":103,"4":104},"gems":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 3 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for index, opcode := range []string{"ggm", "gei", "gli"} {
		if plan.Steps[index].Opcode != opcode {
			t.Fatalf("opcode %d = %q, want %q", index, plan.Steps[index].Opcode, opcode)
		}
	}
}

func TestPlanEquipmentReconfigureDetachesGemWithoutRemountingRetainedEquipment(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{"1": 501}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, WearerKind: "commander", WearerID: 0}
	}
	gameState.Inventory.Gems[501] = State.GemInstance{ID: 501, EquipmentInstanceID: 101, WearerKind: "commander", WearerID: 0}
	gameState.Inventory.Gems[502] = State.GemInstance{ID: 502}
	gameState.Commanders[0] = leader

	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":101,"2":102,"3":103,"4":104},"gems":{"1":502}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for index, opcode := range []string{"ege", "bge", "ggm", "gei", "gli"} {
		if plan.Steps[index].Opcode != opcode {
			t.Fatalf("opcode %d = %q, want %q", index, plan.Steps[index].Opcode, opcode)
		}
	}
}

func TestPlanEquipmentReconfigureTemporarilyClearsRetainedSlotForAnotherGemCarrier(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{"1": 501}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, WearerKind: "commander", WearerID: 0}
	}
	gameState.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2}
	gameState.Inventory.Gems[501] = State.GemInstance{ID: 501, EquipmentInstanceID: 101, WearerKind: "commander", WearerID: 0}
	gameState.Inventory.Gems[502] = State.GemInstance{ID: 502, EquipmentInstanceID: 201}
	gameState.Commanders[0] = leader

	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":101,"2":102,"3":103,"4":104},"gems":{"1":502}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	opcodes := make([]string, 0, len(plan.Steps))
	for _, step := range plan.Steps {
		opcodes = append(opcodes, step.Opcode)
	}
	want := []string{"eeq", "eeq", "ege", "eeq", "eeq", "ege", "eeq", "eeq", "bge", "ggm", "gei", "gli"}
	if len(opcodes) != len(want) {
		t.Fatalf("opcodes = %#v", opcodes)
	}
	for index, opcode := range want {
		if opcodes[index] != opcode {
			t.Fatalf("opcode %d = %q, want %q (%#v)", index, opcodes[index], opcode, opcodes)
		}
	}
}

func TestPlanEquipmentUpgradeHonorsConfiguredDelayFromFirstCommand(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{ID: 101, Level: 1}
	configuration, err := Configuration.Open(t.TempDir(), map[string]json.RawMessage{
		"scheduler": json.RawMessage(`{"upgradeEreDelayMs":75}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{State: State.NewStore(gameState), Configuration: configuration}

	plan, err := application.planEquipmentUpgrade(
		t.Context(),
		Intent.PlanningContext{State: gameState},
		json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":3}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) < 4 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for _, index := range []int{1, 3} {
		if delay := plan.Steps[index].DelayMillis; delay != 75 {
			t.Fatalf("upgrade guard %d delay = %dms, want 75ms", index, delay)
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
	gameState.Inventory.Equipment[6544792251] = State.EquipmentInstance{
		ID: 6544792251, DefinitionID: 6544792251, Slot: 2, RarityID: 2,
	}
	plan, err := planEquipmentSell(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"category":"non_relic_equipment"}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Sell 2 item(s) from non_relic_equipment" || len(plan.Steps) != 4 || plan.Steps[0].Opcode != "seq" || plan.Steps[1].Opcode != "seq" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlanEquipmentSellSellsAllEligibleNonRelicGemStacks(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	gameState.Observations["ggm"] = State.ProtocolObservation{
		Opcode: "ggm", LastDirection: "inbound", LastCode: &code, LastSeenAt: time.Now().UTC(),
	}
	gameState.Inventory.GemStacks[20] = 3
	gameState.Inventory.GemStacks[500] = 2

	plan, err := planEquipmentSell(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{"category":"non_relic_gems"}`))
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Sell 3 item(s) from non_relic_gems" || len(plan.Steps) != 5 {
		t.Fatalf("plan = %#v", plan)
	}
	for index := 0; index < 3; index++ {
		if plan.Steps[index].Opcode != "sge" {
			t.Fatalf("step %d opcode = %q, want sge", index, plan.Steps[index].Opcode)
		}
	}
}

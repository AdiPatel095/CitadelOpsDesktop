package App

import (
	"context"
	"encoding/json"
	"slices"
	"strconv"
	"strings"
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
	gameState.Inventory.Equipment[101] = State.EquipmentInstance{
		ID: 101, Slot: 1, RarityID: 5, Relic: true, RelicKnown: true, Level: 1,
	}
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

func TestPlanEquipmentUpgradeUsesOfficialRarityCapsAndOpcodes(t *testing.T) {
	tests := []struct {
		name       string
		rarityID   int
		slot       int
		relic      bool
		maximum    int
		opcode     string
		payload    string
		hasContext bool
	}{
		{name: "unique", rarityID: 0, slot: 1, maximum: 20, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "common", rarityID: 1, slot: 1, maximum: 3, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "rare", rarityID: 2, slot: 2, maximum: 8, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "epic", rarityID: 3, slot: 3, maximum: 12, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "legendary", rarityID: 4, slot: 4, maximum: 16, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "normal rarity five", rarityID: 5, slot: 1, maximum: 50, opcode: "eqe", payload: `{"C2":0,"EID":101}`},
		{name: "relic", rarityID: 5, slot: 1, relic: true, maximum: 50, opcode: "ere", payload: `{"C2":0,"RIID":101,"EQ":1}`, hasContext: true},
		{name: "relic hero", rarityID: 15, slot: 6, relic: true, maximum: 50, opcode: "ere", payload: `{"C2":0,"RIID":101,"EQ":1}`, hasContext: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Inventory.Equipment[101] = State.EquipmentInstance{
				ID: 101, Slot: test.slot, RarityID: test.rarityID, Relic: test.relic, RelicKnown: true,
				Level: test.maximum - 1,
			}
			configuration, err := Configuration.Open(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			application := &Application{State: State.NewStore(gameState), Configuration: configuration}
			plan, err := application.planEquipmentUpgrade(
				t.Context(),
				Intent.PlanningContext{State: gameState},
				json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":`+strconv.Itoa(test.maximum)+`}`),
			)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Contains(plan.Claims, "account-resources") {
				t.Fatalf("upgrade claims do not reserve spendable resources: %#v", plan.Claims)
			}
			upgradeIndex := 1
			if test.hasContext {
				if plan.Steps[0].Opcode != "gnr" {
					t.Fatalf("relic context opcode = %q, want gnr", plan.Steps[0].Opcode)
				}
				upgradeIndex = 2
			} else if plan.Steps[0].Opcode != "" || plan.Steps[0].Action != "equipment.verify_coin_reserve" {
				t.Fatalf("ordinary equipment unexpectedly opened relic context: %#v", plan.Steps[0])
			}
			upgrade := plan.Steps[upgradeIndex]
			if upgrade.Opcode != test.opcode || upgrade.AwaitOpcode != test.opcode ||
				upgrade.Command.Opcode != test.opcode || string(upgrade.Payload) != test.payload ||
				len(upgrade.SuccessCodes) != 1 || upgrade.SuccessCodes[0] != 0 || len(upgrade.StaleCodes) != 0 ||
				upgrade.ResponseRetry == nil || len(upgrade.ResponseRetry.Codes) != 1 || upgrade.ResponseRetry.Codes[0] != 227 ||
				upgrade.ResponseRetry.GuardAction != "equipment.verify_coin_reserve" || upgrade.ResponseRetry.DelayMillis <= 0 {
				t.Fatalf("upgrade step = %#v", upgrade)
			}
			_, err = application.planEquipmentUpgrade(
				t.Context(),
				Intent.PlanningContext{State: gameState},
				json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":`+strconv.Itoa(test.maximum+1)+`}`),
			)
			if err == nil {
				t.Fatalf("target above rarity cap %d was accepted", test.maximum)
			}
		})
	}
}

func TestPlanEquipmentUpgradeUsesRelicGemWireContract(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Inventory.Gems[501] = State.GemInstance{ID: 501, Level: 1}
	configuration, err := Configuration.Open(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	application := &Application{State: State.NewStore(gameState), Configuration: configuration}
	plan, err := application.planEquipmentUpgrade(
		t.Context(), Intent.PlanningContext{State: gameState},
		json.RawMessage(`{"itemKind":"gem","itemId":501,"targetLevel":2}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(plan.Claims, "account-resources") {
		t.Fatalf("relic gem upgrade claims do not reserve spendable resources: %#v", plan.Claims)
	}
	if len(plan.Steps) < 3 || plan.Steps[0].Opcode != "gnr" || plan.Steps[2].Opcode != "ere" ||
		plan.Steps[2].AwaitOpcode != "ere" || string(plan.Steps[2].Payload) != `{"C2":0,"RIID":501,"EQ":0}` ||
		plan.Steps[2].ResponseRetry == nil || len(plan.Steps[2].ResponseRetry.Codes) != 1 ||
		plan.Steps[2].ResponseRetry.Codes[0] != 227 {
		t.Fatalf("relic gem upgrade plan = %#v", plan)
	}
}

func TestPlanEquipmentUpgradeRejectsUnverifiedTypesAndTravellingWearer(t *testing.T) {
	tests := []struct {
		name string
		item State.EquipmentInstance
	}{
		{name: "unknown rarity", item: State.EquipmentInstance{ID: 101, Slot: 1, RarityID: 6, RelicKnown: true, Level: 1}},
		{name: "missing relic discriminator", item: State.EquipmentInstance{ID: 101, Slot: 1, RarityID: 5, Level: 1}},
		{name: "ordinary hero", item: State.EquipmentInstance{ID: 101, Slot: 6, RarityID: 10, RelicKnown: true, Level: 1}},
		{name: "appearance item", item: State.EquipmentInstance{ID: 101, Slot: 5, RarityID: 0, RelicKnown: true, Level: 1}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Inventory.Equipment[101] = test.item
			configuration, err := Configuration.Open(t.TempDir(), nil)
			if err != nil {
				t.Fatal(err)
			}
			application := &Application{State: State.NewStore(gameState), Configuration: configuration}
			_, err = application.planEquipmentUpgrade(
				t.Context(), Intent.PlanningContext{State: gameState},
				json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":2}`),
			)
			if err == nil || !strings.Contains(err.Error(), "unsupported or unverified enchantment type") {
				t.Fatalf("unsupported equipment error = %v", err)
			}
		})
	}

	t.Run("travelling commander", func(t *testing.T) {
		gameState := State.NewGameState()
		gameState.Commanders[7] = State.CommanderState{ID: 7, Available: false}
		gameState.Inventory.Equipment[101] = State.EquipmentInstance{
			ID: 101, Slot: 1, RarityID: 5, Relic: true, RelicKnown: true, Level: 1,
			WearerKind: "commander", WearerID: 7,
		}
		configuration, err := Configuration.Open(t.TempDir(), nil)
		if err != nil {
			t.Fatal(err)
		}
		application := &Application{State: State.NewStore(gameState), Configuration: configuration}
		_, err = application.planEquipmentUpgrade(
			t.Context(), Intent.PlanningContext{State: gameState},
			json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":2}`),
		)
		if err == nil || !strings.Contains(err.Error(), "cannot be upgraded while commander 7 is travelling") {
			t.Fatalf("travelling wearer error = %v", err)
		}

		commander := gameState.Commanders[7]
		commander.Available = true
		gameState.Commanders[7] = commander
		gameState.Player.ID = 1
		gameState.Castles[100] = State.CastleState{ID: 100}
		arrivesAt := time.Now().UTC().Add(time.Minute)
		commanderID := State.CommanderID(7)
		gameState.Movements[50] = State.MovementState{
			ID: 50, Direction: 0, OwnerPlayerID: 1, SourceCastleID: 100,
			CommanderID: &commanderID, ArrivesAt: &arrivesAt,
		}
		application.State = State.NewStore(gameState)
		_, err = application.planEquipmentUpgrade(
			t.Context(), Intent.PlanningContext{State: gameState},
			json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":2}`),
		)
		if err == nil || !strings.Contains(err.Error(), "cannot be upgraded while commander 7 is travelling") {
			t.Fatalf("active-movement wearer error = %v", err)
		}

		delete(gameState.Movements, 50)
		application.State = State.NewStore(gameState)
		plan, err := application.planEquipmentUpgrade(
			t.Context(), Intent.PlanningContext{State: gameState},
			json.RawMessage(`{"itemKind":"equipment","itemId":101,"targetLevel":2}`),
		)
		if err != nil {
			t.Fatal(err)
		}
		if len(plan.Claims) != 4 || !slices.Contains(plan.Claims, "account-resources") ||
			!slices.Contains(plan.Claims, "leader:commander:7") {
			t.Fatalf("available wearer claims = %#v", plan.Claims)
		}
	})
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
	if plan.Summary != "Sell 2 item(s) from non_relic_equipment" || len(plan.Steps) != 3 ||
		plan.Steps[0].Opcode != "seq" || plan.Steps[1].Opcode != "seq" || plan.Steps[2].Opcode != "gei" {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestEquipmentFreshnessSurvivesOverlappingOutboundRefresh(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	observedAt := time.Now().UTC()
	gameState.Observations["ggm"] = State.ProtocolObservation{
		Opcode: "ggm", LastDirection: "outbound", LastCode: &code, LastSeenAt: observedAt.Add(time.Millisecond),
		LastSuccessfulInboundAt: observedAt,
	}
	if err := requireRecentEquipmentSnapshot(gameState, "ggm"); err != nil {
		t.Fatalf("fresh successful inbound was hidden by outbound refresh: %v", err)
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
	if plan.Summary != "Sell 3 item(s) from non_relic_gems" || len(plan.Steps) != 4 || plan.Steps[3].Opcode != "ggm" {
		t.Fatalf("plan = %#v", plan)
	}
	for index := 0; index < 3; index++ {
		if plan.Steps[index].Opcode != "sge" {
			t.Fatalf("step %d opcode = %q, want sge", index, plan.Steps[index].Opcode)
		}
	}
}

func TestPlanEquipmentSellDoesNotRefreshWhenNothingMatches(t *testing.T) {
	gameState := State.NewGameState()
	code := 0
	gameState.Observations["gei"] = State.ProtocolObservation{
		Opcode: "gei", LastDirection: "inbound", LastCode: &code, LastSeenAt: time.Now().UTC(),
	}
	gameState.Inventory.Equipment[11] = State.EquipmentInstance{ID: 11, DefinitionID: 100, Slot: 1, RarityID: 5}

	plan, err := planEquipmentSell(
		context.Background(),
		Intent.PlanningContext{State: gameState},
		json.RawMessage(`{"category":"non_relic_equipment"}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Summary != "Sell 0 item(s) from non_relic_equipment" || len(plan.Steps) != 0 {
		t.Fatalf("empty sale still emitted game traffic: %#v", plan)
	}
}

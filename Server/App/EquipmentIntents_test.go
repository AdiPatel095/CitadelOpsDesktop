package App

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	EquipmentDomain "CitadelDesktop/Server/Equipment"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Ingest"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Outbound"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

type equipmentTestCommanderHolds struct{ held State.CommanderID }

func (holds equipmentTestCommanderHolds) HoldCommanders([]State.CommanderID, time.Time) {}
func (holds equipmentTestCommanderHolds) CommanderHeldAt(id State.CommanderID, _ time.Time) bool {
	return id == holds.held
}

func TestPlanEquipmentReconfigureUsesCanonicalLeaderAndInstanceIDs(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		currentID := State.EquipmentInstanceID(10 + slot)
		proposedID := State.EquipmentInstanceID(20 + slot)
		leader.Equipment[strconv.Itoa(slot)] = currentID
		gameState.Inventory.Equipment[currentID] = State.EquipmentInstance{ID: currentID, Slot: slot, TypeID: 2, Relic: true, RelicKnown: true, WearerKind: "commander", WearerID: 0}
		gameState.Inventory.Equipment[proposedID] = State.EquipmentInstance{ID: proposedID, Slot: slot, TypeID: 2, Relic: true, RelicKnown: true}
	}
	gameState.Commanders[0] = leader
	gameState.Inventory.Equipment[99] = State.EquipmentInstance{ID: 99, Slot: 1, TypeID: 2, Relic: true, RelicKnown: true}
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
	want := []string{"eeq", "eeq", "eeq", "eeq", "eeq", "ege", "eeq", "eeq", "eeq", "eeq", "eeq", "bge", "ggm", "gei", "gli", ""}
	if len(opcodes) != len(want) {
		t.Fatalf("opcodes = %#v", opcodes)
	}
	for index := range want {
		if opcodes[index] != want[index] {
			t.Fatalf("opcode %d = %q, want %q (%#v)", index, opcodes[index], want[index], opcodes)
		}
	}
	if action := plan.Steps[len(plan.Steps)-1].Action; action != "equipment.reconfigure.verify" {
		t.Fatalf("final action = %q", action)
	}
}

func TestPlanEquipmentReconfigureSkipsMatchingEquipment(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true, WearerKind: "commander", WearerID: 0}
	}
	gameState.Commanders[0] = leader

	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":101,"2":102,"3":103,"4":104},"gems":{}
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 4 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for index, opcode := range []string{"ggm", "gei", "gli"} {
		if plan.Steps[index].Opcode != opcode {
			t.Fatalf("opcode %d = %q, want %q", index, plan.Steps[index].Opcode, opcode)
		}
	}
	if plan.Steps[3].Action != "equipment.reconfigure.verify" {
		t.Fatalf("final action = %q", plan.Steps[3].Action)
	}
}

func TestPlanEquipmentReconfigureDetachesGemWithoutRemountingRetainedEquipment(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{"1": 501}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, Relic: true, RelicKnown: true, WearerKind: "commander", WearerID: 0}
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
	if len(plan.Steps) != 6 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	for index, opcode := range []string{"ege", "bge", "ggm", "gei", "gli"} {
		if plan.Steps[index].Opcode != opcode {
			t.Fatalf("opcode %d = %q, want %q", index, plan.Steps[index].Opcode, opcode)
		}
	}
	if plan.Steps[5].Action != "equipment.reconfigure.verify" {
		t.Fatalf("final action = %q", plan.Steps[5].Action)
	}
}

func TestPlanEquipmentReconfigureTemporarilyClearsRetainedSlotForAnotherGemCarrier(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{"1": 501}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, Relic: true, RelicKnown: true, WearerKind: "commander", WearerID: 0}
	}
	gameState.Inventory.Equipment[201] = State.EquipmentInstance{ID: 201, Slot: 1, TypeID: 2, Relic: true, RelicKnown: true}
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
	want := []string{"eeq", "eeq", "ege", "eeq", "eeq", "ege", "eeq", "eeq", "bge", "ggm", "gei", "gli", ""}
	if len(opcodes) != len(want) {
		t.Fatalf("opcodes = %#v", opcodes)
	}
	for index, opcode := range want {
		if opcodes[index] != opcode {
			t.Fatalf("opcode %d = %q, want %q (%#v)", index, opcodes[index], opcode, opcodes)
		}
	}
}

func TestValidateEquipmentExtractionDispatchAllowsFutureCarrierInSameSlot(t *testing.T) {
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Session = State.SessionState{
		LoggedIn: true, SocketReady: true, ConnectionGeneration: 7, ChangedAt: now.Add(-time.Minute),
	}
	gameState.Player.Resources[2] = 1_000
	gameState.Player.ResourceObservations[2] = State.PlayerResourceObservation{
		ObservedAt: now, ConnectionGeneration: 7,
	}
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{"1": 201}, Gems: map[string]State.GemInstanceID{"1": -201},
	}
	gameState.Inventory.Equipment[201] = State.EquipmentInstance{
		ID: 201, Slot: 1, TypeID: 2, RelicKnown: true, WearerKind: "commander", WearerID: 0,
	}
	gameState.Inventory.Equipment[301] = State.EquipmentInstance{ID: 301, Slot: 1, TypeID: 2, RelicKnown: true}
	gameState.Inventory.Gems[-201] = State.GemInstance{ID: -201, DefinitionID: 494, EquipmentInstanceID: 201}
	gameState.Inventory.Gems[-301] = State.GemInstance{ID: -301, DefinitionID: 490, EquipmentInstanceID: 301}

	manager := equipmentExtractionGameDataManager(t)
	gameData, ready := manager.Current()
	if !ready {
		t.Fatal("official game data is unavailable")
	}
	quote := EquipmentDomain.ExtractionQuote{RubyExtractionCount: 2, MaximumRubySpend: 400}
	request := equipmentReconfigureRequest{
		LeaderKind: "commander", LeaderID: 0, SnapshotFingerprint: "snapshot", MaximumRubySpend: 400,
		Equipment: map[string]State.EquipmentInstanceID{"1": 201}, Gems: map[string]State.GemInstanceID{"1": -301},
	}
	request.QuoteFingerprint = EquipmentDomain.ReconfigurationQuoteFingerprint(
		request.SnapshotFingerprint, request.Equipment, request.Gems, quote,
	)
	arguments := equipmentExtractionDispatch{
		Request: request, InitialQuote: quote, GemID: -201, CarrierID: 201, RubyCost: 200,
		ExpectedRemainingRubySpend: 400, ExpectedConnectionGeneration: 7,
		ExpectedCatalogDigest: gameData.Metadata().DigestSHA256, RubyResourceID: 2,
		PlanningRubyObservedAt: now,
		RemainingPaidExtractions: []equipmentPaidExtraction{
			{GemID: -201, CarrierID: 201, DefinitionID: 494, RubyCost: 200},
			{GemID: -301, CarrierID: 301, DefinitionID: 490, RubyCost: 200},
		},
	}
	stateStore := State.NewStore(gameState)
	if _, err := stateStore.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 7}
		return []string{"player"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	application := &Application{State: stateStore, GameData: manager}
	view := application.State.ReadOnlyView()
	if _, err := validateEquipmentRubyAuthority(view, 2, 400, time.Now().UTC(), time.Time{}); err != nil {
		t.Fatalf("test ruby authority is invalid: state=%+v observation=%+v err=%v", view.Session, view.Player.ResourceObservations[2], err)
	}
	if err := application.validateEquipmentExtractionDispatch(
		arguments, Outbound.Metadata{OperationID: "paid-extraction"}, false,
	); err != nil {
		t.Fatalf("sequential same-slot carriers rejected before first extraction: %v", err)
	}
	replaced := application.State.ReadOnlyView()
	replacedGem := replaced.Inventory.Gems[-201]
	replacedGem.DefinitionID = 490
	if _, err := application.State.ApplyComponents(State.Components(State.ComponentInventory), func(state *State.GameState) ([]string, bool, error) {
		state.SetInventoryGem(-201, replacedGem)
		return []string{"inventory", "gems"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := application.validateEquipmentExtractionDispatch(
		arguments, Outbound.Metadata{OperationID: "paid-extraction"}, false,
	); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("same-price normal gem replacement error = %v, want stale plan", err)
	}
}

func TestPlanEquipmentReconfigureFingerprintIgnoresUnrelatedAndRejectsRelevantChange(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Player.ID = 44
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, RelicKnown: true, WearerKind: "commander", WearerID: 0, Effects: State.EquipmentEffects{{DefinitionID: 9001, Values: []float64{10}}}}
	}
	gameState.Commanders[0] = leader
	fingerprint, err := EquipmentDomain.SnapshotFingerprint(gameState, nil, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(map[string]any{
		"leaderKind": "commander", "leaderId": 0, "combatMode": "pvp", "snapshotFingerprint": fingerprint,
		"equipment": leader.Equipment, "gems": map[string]State.GemInstanceID{},
	})
	unrelated := gameState
	unrelated.Revision++
	unrelated.Player.Level++
	if _, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: unrelated}, arguments); err != nil {
		t.Fatalf("unrelated update invalidated preview: %v", err)
	}
	offModeSocket := gameState
	offModeSocket.Inventory.Gems = maps.Clone(gameState.Inventory.Gems)
	offModeSocket.Inventory.Gems[501] = State.GemInstance{ID: 501, DefinitionID: 77, CompatibleWearerID: 2, CombatMode: "pve", EquipmentInstanceID: 101}
	if _, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: offModeSocket}, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("new off-mode socket error = %v, want stale plan before detach", err)
	}
	relevant := gameState
	relevant.Inventory.Equipment = maps.Clone(gameState.Inventory.Equipment)
	item := relevant.Inventory.Equipment[101]
	item.Level++
	relevant.Inventory.Equipment[101] = item
	if _, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: relevant}, arguments); !errors.Is(err, Intent.ErrPlanStale) {
		t.Fatalf("relevant update error = %v, want stale plan", err)
	}
}

func TestPlanEquipmentReconfigureRejectsReservedCommander(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[7] = State.CommanderState{ID: 7, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{}}
	_, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{
		State: gameState, CommanderHolds: equipmentTestCommanderHolds{held: 7},
	}, json.RawMessage(`{"leaderKind":"commander","leaderId":7,"equipment":{},"gems":{}}`))
	if err == nil || !strings.Contains(err.Error(), "travelling or reserved") {
		t.Fatalf("reserved commander error = %v", err)
	}
}

func TestPlanEquipmentReconfigureRejectsMixedFamiliesBeforeCommands(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
	}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, Slot: slot, TypeID: 2, RelicKnown: true, Relic: slot == 2,
		}
	}
	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":101,"2":102,"3":103,"4":104},"gems":{}
	}`))
	if err == nil || !strings.Contains(err.Error(), "mixes ordinary and relic equipment") {
		t.Fatalf("mixed family error = %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("mixed family request produced command steps: %#v", plan.Steps)
	}
}

func TestPlanEquipmentReconfigureRejectsGemFamilyMismatchBeforeCommands(t *testing.T) {
	tests := []struct {
		name      string
		relicGear bool
		gemID     State.GemInstanceID
	}{
		{name: "normal gem on relic equipment", relicGear: true, gemID: -501},
		{name: "relic gem on ordinary equipment", relicGear: false, gemID: 501},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gameState := State.NewGameState()
			gameState.Commanders[0] = State.CommanderState{
				ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{},
			}
			for slot := 1; slot <= 4; slot++ {
				id := State.EquipmentInstanceID(100 + slot)
				gameState.Inventory.Equipment[id] = State.EquipmentInstance{
					ID: id, Slot: slot, TypeID: 2, RelicKnown: true, Relic: test.relicGear,
				}
			}
			gameState.Inventory.Gems[test.gemID] = State.GemInstance{ID: test.gemID}
			arguments, _ := json.Marshal(map[string]any{
				"leaderKind": "commander", "leaderId": 0,
				"equipment": map[string]State.EquipmentInstanceID{"1": 101, "2": 102, "3": 103, "4": 104},
				"gems":      map[string]State.GemInstanceID{"1": test.gemID},
			})
			plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, arguments)
			if err == nil || !strings.Contains(err.Error(), "ordinary or relic family") {
				t.Fatalf("gem family error = %v", err)
			}
			if len(plan.Steps) != 0 {
				t.Fatalf("gem family mismatch produced command steps: %#v", plan.Steps)
			}
		})
	}
}

func TestPlanEquipmentReconfigureRejectsFamilySwitchWithGemmedAppearanceBeforeCommands(t *testing.T) {
	gameState := State.NewGameState()
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{"5": 105},
		Gems:      map[string]State.GemInstanceID{},
	}
	gameState.Inventory.Equipment[105] = State.EquipmentInstance{
		ID: 105, Slot: 5, TypeID: 2, RelicKnown: true,
		WearerKind: "commander", WearerID: 0,
	}
	gameState.Inventory.Gems[-501] = State.GemInstance{
		ID: -501, EquipmentInstanceID: 105, WearerKind: "commander", WearerID: 0,
	}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(200 + slot)
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{
			ID: id, Slot: slot, TypeID: 2, RelicKnown: true, Relic: true,
		}
	}
	plan, err := planEquipmentReconfigure(context.Background(), Intent.PlanningContext{State: gameState}, json.RawMessage(`{
		"leaderKind":"commander","leaderId":0,
		"equipment":{"1":201,"2":202,"3":203,"4":204},"gems":{}
	}`))
	if err == nil || !strings.Contains(err.Error(), "gemmed appearance item prevents switching") {
		t.Fatalf("gemmed appearance family error = %v", err)
	}
	if len(plan.Steps) != 0 {
		t.Fatalf("gemmed appearance family mismatch produced command steps: %#v", plan.Steps)
	}
}

func TestVerifyEquipmentReconfigureAcceptsNormalGemReidentificationAndRejectsMismatch(t *testing.T) {
	gameState := State.NewGameState()
	leader := State.CommanderState{ID: 0, Available: true, Equipment: map[string]State.EquipmentInstanceID{}, Gems: map[string]State.GemInstanceID{"1": -999}}
	for slot := 1; slot <= 4; slot++ {
		id := State.EquipmentInstanceID(100 + slot)
		leader.Equipment[strconv.Itoa(slot)] = id
		gameState.Inventory.Equipment[id] = State.EquipmentInstance{ID: id, Slot: slot, TypeID: 2, WearerKind: "commander", WearerID: 0}
	}
	gameState.Commanders[0] = leader
	gameState.Inventory.Gems[-999] = State.GemInstance{ID: -999, DefinitionID: 55, EquipmentInstanceID: 101, WearerKind: "commander", WearerID: 0}
	application := &Application{State: State.NewStore(gameState)}
	arguments, _ := json.Marshal(equipmentReconfigureVerification{
		LeaderKind: "commander", LeaderID: 0, Equipment: leader.Equipment,
		Gems: map[string]equipmentReconfigureGemVerification{"1": {InstanceID: -501, DefinitionID: 55, Normal: true}},
	})
	if err := application.verifyEquipmentReconfigure(context.Background(), arguments); err != nil {
		t.Fatalf("normal gem reidentification failed: %v", err)
	}
	_, err := application.State.Apply(func(state *State.GameState) ([]string, bool, error) {
		commander := state.Commanders[0]
		commander.Equipment["1"] = 102
		state.Commanders[0] = commander
		return []string{"equipment"}, true, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := application.verifyEquipmentReconfigure(context.Background(), arguments); err == nil {
		t.Fatal("mismatched authoritative state unexpectedly verified")
	}
}

func equipmentExtractionGameDataManager(t *testing.T) *GameData.Manager {
	t.Helper()
	cacheDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(cacheDir, "Items-vtest.json"), []byte(`{
		"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}],
		"gems":[{"gemID":494,"gemLevelID":0},{"gemID":490,"gemLevelID":0}],
		"gemlevels":[{"gemLevelID":0,"removalCostC2":200}]
	}`), 0o600); err != nil {
		t.Fatal(err)
	}
	manager := GameData.NewManager(GameData.UpdaterConfig{
		CacheDir: cacheDir, VersionURL: "offline://items-version",
	})
	if err := manager.Initialize(t.Context()); err != nil {
		t.Fatal(err)
	}
	return manager
}

func equipmentExtractionGameDataStore(t *testing.T, removalCost int64, digest string) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(fmt.Sprintf(`{
		"versionInfo":{"version":{"@value":"test"}},"buildings":[],"units":[],
		"resources":[{"resourceID":2,"JSONKey":"C2","name":"Rubies"}],
		"gems":[{"gemID":494,"gemLevelID":0},{"gemID":490,"gemLevelID":0}],
		"gemlevels":[{"gemLevelID":0,"removalCostC2":%d}]
	}`, removalCost)), GameData.SourceMetadata{ItemVersion: "test", DigestSHA256: digest})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestEquipmentReconfigureExecutesTwoPaidExtractionsThroughEngine(t *testing.T) {
	application, engine, sender, arguments := newEquipmentExtractionIntegrationHarness(t, nil)
	defer sender.router.Close()
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "equipment-two-paid", Name: "equipment.reconfigure", Actor: "user", Arguments: arguments,
	})
	if receipt.Status != Intent.StatusSucceeded {
		t.Fatalf("equipment reconfigure receipt = %#v", receipt)
	}
	if sender.extractionSends != 2 || sender.rubies != 600 {
		t.Fatalf("paid extraction sends=%d rubies=%d opcodes=%v", sender.extractionSends, sender.rubies, sender.opcodes)
	}
	if receipt.Plan == nil || !slices.Contains(receipt.Plan.Claims, "currency:2") {
		t.Fatalf("paid plan claims = %#v", receipt.Plan)
	}
	foundRubyResource := false
	for _, resource := range receipt.Plan.Resources {
		if resource.Scope == Intent.ResourceScopeAccount && resource.Capability == State.CapabilityEconomy &&
			resource.ResourceKind == "spendable" && resource.ResourceID == "2" {
			foundRubyResource = true
		}
	}
	if !foundRubyResource {
		t.Fatalf("paid plan resources = %#v", receipt.Plan.Resources)
	}
	state := application.State.ReadOnlyView()
	if state.Player.Resources[2] != 600 || state.Commanders[0].Equipment["1"] != 301 {
		t.Fatalf("final state resources=%#v commander=%#v", state.Player.Resources, state.Commanders[0])
	}
	gemID := state.Commanders[0].Gems["1"]
	gem := state.Inventory.Gems[gemID]
	if gemID >= 0 || gem.DefinitionID != 494 || gem.EquipmentInstanceID != 301 {
		t.Fatalf("final normal gem id=%d gem=%#v", gemID, gem)
	}
	for _, carrierID := range []State.EquipmentInstanceID{201, 301} {
		if marker := state.Inventory.Equipment[carrierID].Extraction; marker != nil {
			t.Fatalf("carrier %d retained completed extraction marker %#v", carrierID, marker)
		}
	}
}

func TestEquipmentReconfigureReplansWalletChangeBeforeEquipmentMutation(t *testing.T) {
	application, engine, sender, arguments := newEquipmentExtractionIntegrationHarness(t, nil)
	defer sender.router.Close()
	mutated := false
	engine.SetExecutionGate(func(_ context.Context, _ Intent.Request, _ Intent.Plan, point Intent.ExecutionPoint) error {
		if point != Intent.ExecutionBeforeClaims || mutated {
			return nil
		}
		mutated = true
		now := time.Now().UTC()
		_, err := application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
			state.Player.Resources[2] = 100
			state.Player.ResourceObservations[2] = State.PlayerResourceObservation{
				ObservedAt: now, ConnectionGeneration: state.Session.ConnectionGeneration,
			}
			return []string{"resources"}, true, nil
		})
		return err
	})
	receipt := engine.Submit(t.Context(), Intent.Request{
		ID: "equipment-wallet-replan", Name: "equipment.reconfigure", Actor: "user", Arguments: arguments,
	})
	if receipt.Status == Intent.StatusSucceeded || !strings.Contains(receipt.DiagnosticError(), "cannot cover") {
		t.Fatalf("wallet-change receipt = %#v", receipt)
	}
	if len(sender.opcodes) != 0 || application.State.ReadOnlyView().Commanders[0].Equipment["1"] != 101 {
		t.Fatalf("wallet change dispatched before replan: opcodes=%v state=%#v", sender.opcodes, application.State.ReadOnlyView().Commanders[0])
	}
}

func TestEquipmentReconfigureFinalDispatchRejectsQueueWaitBalanceChange(t *testing.T) {
	entered := make(chan struct{})
	release := make(chan struct{})
	gate := func(_ context.Context, metadata Outbound.Metadata) error {
		if metadata.FinalDispatchValidation == nil {
			return nil
		}
		select {
		case <-entered:
		default:
			close(entered)
			<-release
		}
		return nil
	}
	application, engine, sender, arguments := newEquipmentExtractionIntegrationHarness(t, gate)
	defer sender.router.Close()
	receipts := make(chan Intent.Receipt, 1)
	go func() {
		receipts <- engine.Submit(t.Context(), Intent.Request{
			ID: "equipment-queue-guard", Name: "equipment.reconfigure", Actor: "user", Arguments: arguments,
		})
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("paid extraction did not reach the outbound queue gate")
	}
	now := time.Now().UTC()
	if _, err := application.State.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.Resources[2] = 100
		state.Player.ResourceObservations[2] = State.PlayerResourceObservation{
			ObservedAt: now, ConnectionGeneration: state.Session.ConnectionGeneration,
		}
		return []string{"resources"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	receipt := <-receipts
	if receipt.Status == Intent.StatusSucceeded || !strings.Contains(receipt.DiagnosticError(), "cannot cover") {
		t.Fatalf("queue-wait receipt = %#v", receipt)
	}
	if sender.extractionSends != 0 {
		t.Fatalf("paid extraction crossed transport after queue-wait balance change: %v", sender.opcodes)
	}
	for _, carrierID := range []State.EquipmentInstanceID{201, 301} {
		if marker := application.State.ReadOnlyView().Inventory.Equipment[carrierID].Extraction; marker != nil {
			t.Fatalf("definitively unsent carrier %d retained marker %#v", carrierID, marker)
		}
	}
}

func TestEquipmentReconfigureIndeterminatePaidExtractionCannotReplay(t *testing.T) {
	application, engine, sender, arguments := newEquipmentExtractionIntegrationHarness(t, nil)
	defer sender.router.Close()
	application.DataDir = t.TempDir()
	if err := State.SaveSnapshot(application.DataDir, application.State.ReadOnlyView()); err != nil {
		t.Fatal(err)
	}
	sender.indeterminateExtraction = true
	first := engine.Submit(t.Context(), Intent.Request{
		ID: "equipment-indeterminate", Name: "equipment.reconfigure", Actor: "user", Arguments: arguments,
	})
	if first.Status != Intent.StatusIndeterminate || sender.extractionSends != 1 {
		t.Fatalf("indeterminate receipt=%#v sends=%d", first, sender.extractionSends)
	}
	state := application.State.ReadOnlyView()
	marker := state.Inventory.Equipment[301].Extraction
	if marker == nil || marker.DispatchedAt.IsZero() {
		t.Fatalf("indeterminate extraction marker = %#v", marker)
	}
	reloaded, err := State.LoadSnapshot(application.DataDir)
	if err != nil {
		t.Fatal(err)
	}
	if persisted := reloaded.Inventory.Equipment[301].Extraction; persisted == nil || persisted.OperationID != marker.OperationID || persisted.DispatchedAt.IsZero() {
		t.Fatalf("persisted indeterminate extraction marker = %#v", persisted)
	}
	gameData, ready := application.GameData.Current()
	if !ready {
		t.Fatal("official game data is unavailable")
	}
	targetEquipment := map[string]State.EquipmentInstanceID{"1": 301, "2": 102, "3": 103, "4": 104}
	targetGems := map[string]State.GemInstanceID{"1": -201}
	freshSnapshot, err := EquipmentDomain.SnapshotFingerprint(state, gameData, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	transition, err := EquipmentDomain.BuildReconfigurationTransition(state, state.Commanders[0].Equipment, targetEquipment, targetGems)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := EquipmentDomain.QuoteReconfiguration(gameData, transition)
	if err != nil {
		t.Fatal(err)
	}
	quote.Fingerprint = EquipmentDomain.ReconfigurationQuoteFingerprint(freshSnapshot, targetEquipment, targetGems, quote)
	arguments, err = json.Marshal(equipmentReconfigureRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", SnapshotFingerprint: freshSnapshot,
		Equipment: targetEquipment, Gems: targetGems, QuoteFingerprint: quote.Fingerprint, MaximumRubySpend: quote.MaximumRubySpend,
	})
	if err != nil {
		t.Fatal(err)
	}
	second := engine.Submit(t.Context(), Intent.Request{
		ID: "equipment-indeterminate-replay", Name: "equipment.reconfigure", Actor: "user", Arguments: arguments,
	})
	if second.Status == Intent.StatusSucceeded || !strings.Contains(second.DiagnosticError(), "unresolved prior ruby extraction") || sender.extractionSends != 1 {
		t.Fatalf("replay receipt=%#v sends=%d", second, sender.extractionSends)
	}
}

func TestPlanEquipmentReconfigureRejectsChangedPaidAuthorityBeforeCommands(t *testing.T) {
	application, _, sender, raw := newEquipmentExtractionIntegrationHarness(t, nil)
	defer sender.router.Close()
	gameData, ready := application.GameData.Current()
	if !ready {
		t.Fatal("official game data is unavailable")
	}
	baseState := application.State.ReadOnlyView()
	tests := []struct {
		name   string
		mutate func(*equipmentReconfigureRequest, *State.GameState, **GameData.Store)
		want   string
	}{
		{name: "ruby ceiling", want: "exceeds approved", mutate: func(request *equipmentReconfigureRequest, _ *State.GameState, _ **GameData.Store) {
			request.MaximumRubySpend--
		}},
		{name: "selected alternative", want: "selected alternative", mutate: func(request *equipmentReconfigureRequest, _ *State.GameState, _ **GameData.Store) {
			request.Gems = map[string]State.GemInstanceID{"1": -301}
		}},
		{name: "fresh ruby authority", want: "fresh current-session", mutate: func(_ *equipmentReconfigureRequest, state *State.GameState, _ **GameData.Store) {
			state.Player.ResourceObservations = maps.Clone(state.Player.ResourceObservations)
			observation := state.Player.ResourceObservations[2]
			observation.ObservedAt = time.Now().UTC().Add(-equipmentRubyFreshness - time.Second)
			state.Player.ResourceObservations[2] = observation
		}},
		{name: "official price catalog", want: "equipment changed after this preview", mutate: func(_ *equipmentReconfigureRequest, _ *State.GameState, store **GameData.Store) {
			*store = equipmentExtractionGameDataStore(t, 300, "changed-cost-catalog")
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var request equipmentReconfigureRequest
			if err := json.Unmarshal(raw, &request); err != nil {
				t.Fatal(err)
			}
			state := baseState
			store := gameData
			testCase.mutate(&request, &state, &store)
			arguments, _ := json.Marshal(request)
			plan, err := planEquipmentReconfigure(t.Context(), Intent.PlanningContext{State: state, GameData: store}, arguments)
			if err == nil || !strings.Contains(err.Error(), testCase.want) {
				t.Fatalf("error = %v, want %q", err, testCase.want)
			}
			if len(plan.Steps) != 0 {
				t.Fatalf("rejected paid authority produced commands: %#v", plan.Steps)
			}
		})
	}
}

func TestPlanEquipmentReconfigurePreservesZeroRubyExtractionPath(t *testing.T) {
	application, _, sender, _ := newEquipmentExtractionIntegrationHarness(t, nil)
	defer sender.router.Close()
	state := application.State.ReadOnlyView()
	gameData := equipmentExtractionGameDataStore(t, 0, "zero-cost-catalog")
	targetEquipment := map[string]State.EquipmentInstanceID{"1": 301, "2": 102, "3": 103, "4": 104}
	targetGems := map[string]State.GemInstanceID{"1": -201}
	snapshot, err := EquipmentDomain.SnapshotFingerprint(state, gameData, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	arguments, _ := json.Marshal(equipmentReconfigureRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", SnapshotFingerprint: snapshot,
		Equipment: targetEquipment, Gems: targetGems,
	})
	plan, err := planEquipmentReconfigure(t.Context(), Intent.PlanningContext{State: state, GameData: gameData}, arguments)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(plan.Claims, "currency:2") {
		t.Fatalf("zero-ruby plan claims = %#v", plan.Claims)
	}
	extractions := 0
	for _, step := range plan.Steps {
		if step.Opcode != "ege" {
			continue
		}
		extractions++
		if step.PreDispatchAction != "" || step.FinalDispatchAction != "" {
			t.Fatalf("zero-ruby extraction gained a paid guard: %#v", step)
		}
	}
	if extractions != 2 {
		t.Fatalf("zero-ruby extraction count = %d, want 2", extractions)
	}
}

type equipmentExtractionIntegrationSender struct {
	pipeline                *Ingest.Pipeline
	router                  *Outbound.Router
	equipment               map[State.EquipmentInstanceID]State.EquipmentInstance
	equipped                map[int]State.EquipmentInstanceID
	socketDefinition        map[State.EquipmentInstanceID]State.GemID
	rubies                  int64
	opcodes                 []string
	extractionSends         int
	indeterminateExtraction bool
}

func (sender *equipmentExtractionIntegrationSender) Ready() bool                  { return true }
func (sender *equipmentExtractionIntegrationSender) Namespace() string            { return "EmpireEx_21" }
func (sender *equipmentExtractionIntegrationSender) CorrelatesResponses() bool    { return true }
func (sender *equipmentExtractionIntegrationSender) ConnectionGeneration() uint64 { return 1 }
func (sender *equipmentExtractionIntegrationSender) Send(ctx context.Context, payload []byte) error {
	return sender.router.Send(ctx, payload)
}

func (sender *equipmentExtractionIntegrationSender) dispatch(ctx context.Context, payload []byte) error {
	if err := Outbound.ValidateFinalDispatch(ctx); err != nil {
		return err
	}
	command, err := Protocol.Decode(string(payload), Protocol.DirectionOutbound, time.Now().UTC())
	if err != nil {
		return err
	}
	sender.opcodes = append(sender.opcodes, command.Opcode)
	var body struct {
		EquipmentID State.EquipmentInstanceID `json:"EID"`
		Equip       int                       `json:"E"`
		GemID       State.GemID               `json:"GID"`
	}
	if len(command.Payload) > 0 && json.Unmarshal(command.Payload, &body) != nil {
		return fmt.Errorf("decode %s payload", command.Opcode)
	}
	switch command.Opcode {
	case "eeq":
		item, found := sender.equipment[body.EquipmentID]
		if !found {
			return fmt.Errorf("equipment %d is unavailable", body.EquipmentID)
		}
		if body.Equip == 0 {
			if sender.equipped[item.Slot] == item.ID {
				delete(sender.equipped, item.Slot)
			}
		} else {
			sender.equipped[item.Slot] = item.ID
		}
	case "ege":
		sender.extractionSends++
		if sender.indeterminateExtraction {
			return Outbound.MarkIndeterminate(fmt.Errorf("simulated uncertain paid extraction"))
		}
		delete(sender.socketDefinition, body.EquipmentID)
		sender.rubies -= 200
	case "bge":
		sender.socketDefinition[body.EquipmentID] = body.GemID
	}

	code := 0
	receivedAt := time.Now().UTC()
	metadata := Outbound.MetadataFromContext(ctx)
	response := Protocol.Frame{
		Direction: Protocol.DirectionInbound, Namespace: command.Namespace, Opcode: command.Opcode,
		ResponseCode: &code, ReceivedAt: receivedAt, ResponseToken: metadata.ResponseToken,
		CausationOperationID: metadata.OperationID,
	}
	switch command.Opcode {
	case "eeq", "bge":
		response.Payload, _ = json.Marshal(map[string]any{"gli": sender.leaderPayload()})
	case "ege":
		response.Payload, _ = json.Marshal(map[string]any{
			"gli": sender.leaderPayload(), "gcu": map[string]any{"C2": sender.rubies},
		})
	case "ggm":
		response.Payload = json.RawMessage(`{"GEM":[],"RGEM":[]}`)
	case "gei":
		response.Payload, _ = json.Marshal(map[string]any{"I": sender.storageRows()})
	case "gli":
		response.Payload, _ = json.Marshal(sender.leaderPayload())
	default:
		return fmt.Errorf("unexpected equipment opcode %s", command.Opcode)
	}
	_, err = sender.pipeline.HandleFrame(ctx, response)
	return err
}

func (sender *equipmentExtractionIntegrationSender) leaderPayload() map[string]any {
	rows := make([][]any, 0, len(sender.equipped))
	for _, slot := range []int{1, 2, 3, 4, 6} {
		if id := sender.equipped[slot]; id > 0 {
			rows = append(rows, sender.equipmentRow(id))
		}
	}
	return map[string]any{"C": []any{map[string]any{"ID": 0, "VIS": 0, "N": "Test", "EQ": rows}}, "B": []any{}}
}

func (sender *equipmentExtractionIntegrationSender) storageRows() [][]any {
	rows := make([][]any, 0, len(sender.equipment))
	for _, id := range []State.EquipmentInstanceID{101, 102, 103, 104, 201, 301} {
		item := sender.equipment[id]
		if sender.equipped[item.Slot] != id {
			rows = append(rows, sender.equipmentRow(id))
		}
	}
	return rows
}

func (sender *equipmentExtractionIntegrationSender) equipmentRow(id State.EquipmentInstanceID) []any {
	item := sender.equipment[id]
	return []any{item.ID, item.Slot, item.TypeID, 0, 0, []any{}, item.DefinitionID, 0, 0, -1, sender.socketDefinition[id], 0}
}

func newEquipmentExtractionIntegrationHarness(
	t *testing.T,
	gate Outbound.DispatchGate,
) (*Application, *Intent.Engine, *equipmentExtractionIntegrationSender, json.RawMessage) {
	t.Helper()
	now := time.Now().UTC().Add(-time.Second)
	gameState := State.NewGameState()
	gameState.Account.WorldID = "test-world"
	gameState.Player.ID = 1
	gameState.Session = State.SessionState{
		Generation: 1, BaselineGeneration: 1, ConnectionGeneration: 1,
		LoggedIn: true, SocketReady: true, ChangedAt: now.Add(-time.Minute),
	}
	gameState.Player.Resources[2] = 1_000
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Name: "Test", Available: true,
		Equipment: map[string]State.EquipmentInstanceID{"1": 101, "2": 102, "3": 103, "4": 104},
		Gems:      map[string]State.GemInstanceID{},
	}
	for _, item := range []State.EquipmentInstance{
		{ID: 101, DefinitionID: 101, Slot: 1, TypeID: 2, RelicKnown: true, WearerKind: "commander"},
		{ID: 102, DefinitionID: 102, Slot: 2, TypeID: 2, RelicKnown: true, WearerKind: "commander"},
		{ID: 103, DefinitionID: 103, Slot: 3, TypeID: 2, RelicKnown: true, WearerKind: "commander"},
		{ID: 104, DefinitionID: 104, Slot: 4, TypeID: 2, RelicKnown: true, WearerKind: "commander"},
		{ID: 201, DefinitionID: 201, Slot: 1, TypeID: 2, RelicKnown: true},
		{ID: 301, DefinitionID: 301, Slot: 1, TypeID: 2, RelicKnown: true},
	} {
		gameState.Inventory.Equipment[item.ID] = item
	}
	gameState.Inventory.Gems[-201] = State.GemInstance{
		ID: -201, DefinitionID: 494, Slot: 1, CompatibleWearerID: 2, CombatMode: "pvp", EquipmentInstanceID: 201,
	}
	gameState.Inventory.Gems[-301] = State.GemInstance{
		ID: -301, DefinitionID: 490, Slot: 1, CompatibleWearerID: 2, CombatMode: "pvp", EquipmentInstanceID: 301,
	}
	stateStore := State.NewStore(gameState)
	if _, err := stateStore.ApplyComponents(State.Components(State.ComponentPlayer), func(state *State.GameState) ([]string, bool, error) {
		state.Player.ResourceObservations[2] = State.PlayerResourceObservation{ObservedAt: now, ConnectionGeneration: 1}
		return []string{"resources"}, true, nil
	}); err != nil {
		t.Fatal(err)
	}
	gameData := equipmentExtractionGameDataManager(t)
	registry := Ingest.NewRegistry()
	if err := Ingest.RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	pipeline := Ingest.NewPipeline(stateStore, gameData, registry)
	sender := &equipmentExtractionIntegrationSender{
		pipeline: pipeline, rubies: 1_000,
		equipment:        map[State.EquipmentInstanceID]State.EquipmentInstance{},
		equipped:         map[int]State.EquipmentInstanceID{1: 101, 2: 102, 3: 103, 4: 104},
		socketDefinition: map[State.EquipmentInstanceID]State.GemID{201: 494, 301: 490},
	}
	for id, item := range gameState.Inventory.Equipment {
		sender.equipment[id] = item
	}
	sender.router = Outbound.NewRouter(t.Context(), Outbound.Config{
		Ready: func() bool { return true }, Gate: gate,
		Send: func(ctx context.Context, payload []byte) error { return sender.dispatch(ctx, payload) },
	})
	intentRegistry := Intent.NewRegistry()
	intentRegistry.EnforceResourceDeclarations()
	engine := Intent.NewEngine(intentRegistry, stateStore, gameData, sender, pipeline)
	application := &Application{State: stateStore, GameData: gameData, Ingest: pipeline, Intents: engine}
	if err := application.registerGameIntents(); err != nil {
		t.Fatal(err)
	}
	store, ready := gameData.Current()
	if !ready {
		t.Fatal("official game data is unavailable")
	}
	state := stateStore.ReadOnlyView()
	targetEquipment := map[string]State.EquipmentInstanceID{"1": 301, "2": 102, "3": 103, "4": 104}
	targetGems := map[string]State.GemInstanceID{"1": -201}
	snapshot, err := EquipmentDomain.SnapshotFingerprint(state, store, "commander", 0, "pvp")
	if err != nil {
		t.Fatal(err)
	}
	transition, err := EquipmentDomain.BuildReconfigurationTransition(state, state.Commanders[0].Equipment, targetEquipment, targetGems)
	if err != nil {
		t.Fatal(err)
	}
	quote, err := EquipmentDomain.QuoteReconfiguration(store, transition)
	if err != nil {
		t.Fatal(err)
	}
	quote.Fingerprint = EquipmentDomain.ReconfigurationQuoteFingerprint(snapshot, targetEquipment, targetGems, quote)
	arguments, err := json.Marshal(equipmentReconfigureRequest{
		LeaderKind: "commander", LeaderID: 0, CombatMode: "pvp", SnapshotFingerprint: snapshot,
		Equipment: targetEquipment, Gems: targetGems, QuoteFingerprint: quote.Fingerprint, MaximumRubySpend: quote.MaximumRubySpend,
	})
	if err != nil {
		t.Fatal(err)
	}
	return application, engine, sender, arguments
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

package App

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/AttackPresets"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/Protocol"
	"CitadelDesktop/Server/State"
)

func TestPlanBeriTransferUsesRefreshedAmountSelectedTimeSkipAndCanonicalWireShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":10,"foodSupply":1}],
		"currencies":[{"currencyID":1003,"JSONKey":"MS3"}],
		"currencyMinutesSkipValues":[{"currencyID":"1003","MinutesSkipValue":"10"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	gameState := State.NewGameState()
	castle := State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
	castle.Units.Stationed = map[State.UnitID]int64{10: 50}
	gameState.Castles[100] = castle
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: beriKingdomID}
	gameState.Player.Currencies[1003] = 1
	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25},
		ParsedSourceID: 100, ObservedAt: observedAt,
	}
	plan, err := planBeriTransfer(t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, json.RawMessage(`{
		"sourceCastleId":100,"wireCastleId":-1,"unitId":10,"useTimeSkip":true,"timeSkipId":"MS3"
	}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 7 || plan.Steps[0].Opcode != "kpi" ||
		plan.Steps[1].Action != "beri.transfer.verify" || plan.Steps[1].ResumePolicy != Intent.ResumeRebuild ||
		plan.Steps[2].Opcode != "kut" || plan.Steps[3].Action != "troops.kingdom.consume_source" ||
		plan.Steps[4].Action != "beri.consume_capacity" || plan.Steps[5].Opcode != "msk" ||
		plan.Steps[6].Action != timeSkipConsumeAction {
		t.Fatalf("unexpected Beri plan: %#v", plan.Steps)
	}
	for _, claim := range []string{
		"troop-transport", "castle:100", "castle:900", "kingdom:10",
		"beri-capacity:900", "unit:10", "currency:1003",
	} {
		if !slices.Contains(plan.Claims, claim) {
			t.Fatalf("Beri transfer is missing canonical claim %q: %#v", claim, plan.Claims)
		}
	}
	if slices.Contains(plan.Claims, "castle-focus") {
		t.Fatalf("Beri transfer still claims or changes castle focus: %#v", plan.Claims)
	}
	if slices.Contains(plan.Claims, "beri-transfer") {
		t.Fatalf("Beri transfer retained its unmapped legacy claim: %#v", plan.Claims)
	}
	var payload struct {
		SourceID  int       `json:"SCID"`
		SourceKID int       `json:"SKID"`
		TargetKID int       `json:"TKID"`
		CastleID  int       `json:"CID"`
		Troops    [][]int64 `json:"A"`
	}
	if err := json.Unmarshal(plan.Steps[2].Command.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.SourceID != 100 || payload.SourceKID != 0 || payload.TargetKID != 10 || payload.CastleID != -1 ||
		len(payload.Troops) != 1 || payload.Troops[0][0] != 10 || payload.Troops[0][1] != 25 {
		t.Fatalf("unexpected kut payload: %#v", payload)
	}
	var skipPayload map[string]string
	if err := json.Unmarshal(plan.Steps[5].Command.Payload, &skipPayload); err != nil {
		t.Fatal(err)
	}
	if skipPayload["MST"] != "MS3" || skipPayload["KID"] != "10" || skipPayload["TT"] != "1" {
		t.Fatalf("unexpected selected msk payload: %#v", skipPayload)
	}
	var guard beriTransferGuardRequest
	if err := json.Unmarshal(plan.Steps[1].ActionArguments, &guard); err != nil {
		t.Fatal(err)
	}
	castle.Units.Stationed[10] = 24
	gameState.Castles[castle.ID] = castle
	if err := validateBeriTransferState(Intent.PlanningContext{
		State: gameState, GameData: gameData,
	}, guard); err == nil {
		t.Fatal("Beri transfer accepted insufficient donor inventory")
	}
	castle.Units.Stationed[10] = 50
	gameState.Castles[castle.ID] = castle
	if err := validateBeriTransferState(
		Intent.PlanningContext{State: gameState, GameData: gameData}, guard,
	); err != nil {
		t.Fatalf("Beri transfer rejected available donor inventory: %v", err)
	}
}

func TestBeriTransferPlannerAndGuardRejectMeadAndBeefTroops(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],
		"units":[
			{"wodID":10,"foodSupply":1},
			{"wodID":11,"meadSupply":1},
			{"wodID":12,"beefSupply":1}
		],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"1005","MinutesSkipValue":"60"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	observedAt := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name   string
		unitID State.UnitID
	}{{name: "mead", unitID: 11}, {name: "beef", unitID: 12}} {
		t.Run(test.name, func(t *testing.T) {
			unitID := test.unitID
			gameState := State.NewGameState()
			source := State.CastleState{ID: 100, KingdomID: 0, SlotType: 1}
			source.Units.Stationed = map[State.UnitID]int64{unitID: 50}
			gameState.Castles[source.ID] = source
			gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: beriKingdomID}
			gameState.Player.Currencies[1005] = 1
			gameState.Beri = State.BeriState{
				AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{unitID: 25},
				ObservedAt: observedAt,
			}
			input := Intent.PlanningContext{State: gameState, GameData: gameData}
			arguments, _ := json.Marshal(beriTransferRequest{
				SourceCastleID: source.ID, TargetCastleID: 900, WireCastleID: -1,
				UnitID: unitID, Amount: 25,
			})

			if _, err := planBeriTransfer(t.Context(), input, arguments); err == nil ||
				!strings.Contains(err.Error(), "require a Food-consuming unit") {
				t.Fatalf("unit %d planning error = %v", unitID, err)
			}
			if err := validateBeriTransferState(input, beriTransferGuardRequest{
				SourceCastleID: source.ID, TargetCastleID: 900, UnitID: unitID, Amount: 25,
				CapacityObserved: observedAt, TimeSkipCurrency: 1005,
			}); err == nil || !strings.Contains(err.Error(), "require a Food-consuming unit") {
				t.Fatalf("unit %d guard error = %v", unitID, err)
			}
		})
	}
}

func TestBeriTransferPassesProductionResourceAdmission(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[{"wodID":1}],"units":[{"wodID":10,"foodSupply":1}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"1005","MinutesSkipValue":"60"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	source := State.CastleState{ID: 100, KingdomID: 0, SlotType: 1, Focused: true}
	source.Units.Stationed = map[State.UnitID]int64{10: 50}
	gameState.Castles[source.ID] = source
	gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: beriKingdomID}
	gameState.Player.Currencies[1005] = 1
	gameState.Beri = State.BeriState{
		AvailableTroops: 25, TroopsByUnit: map[State.UnitID]int64{10: 25},
		ParsedSourceID: 100, ObservedAt: time.Now().UTC(),
	}

	registry := Intent.NewRegistry()
	registry.EnforceResourceDeclarations()
	if err := registry.Register(Intent.Definition{
		Name: "beri.transfer", Effect: Intent.EffectLaunch, Planner: planBeriTransfer,
	}); err != nil {
		t.Fatal(err)
	}
	engine := Intent.NewEngine(
		registry, State.NewStore(gameState), beriIntentGameDataProvider{store: gameData}, nil, nil,
	)
	receipt := engine.Submit(t.Context(), Intent.Request{
		Name: "beri.transfer", DryRun: true,
		Arguments: json.RawMessage(`{"sourceCastleId":100,"wireCastleId":-1,"unitId":10}`),
	})
	if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
		t.Fatalf("Beri transfer resource admission failed: %#v", receipt)
	}
	hasTroopTransport := false
	for _, resource := range receipt.Plan.Resources {
		if resource.Capability == "legacy" {
			t.Fatalf("Beri transfer retained a legacy resource: %#v", receipt.Plan.Resources)
		}
		if resource.Capability == "transport" && resource.ResourceKind == "troop-transport" {
			hasTroopTransport = true
		}
	}
	if !hasTroopTransport {
		t.Fatalf("Beri transfer is missing its typed troop-transport resource: %#v", receipt.Plan.Resources)
	}
	for _, step := range receipt.Plan.Steps {
		if step.Opcode == "msk" {
			t.Fatalf("Beri transfer spent a time skip while the toggle was disabled: %#v", receipt.Plan.Steps)
		}
	}
	for _, claim := range receipt.Plan.Claims {
		if strings.HasPrefix(claim, "currency:") {
			t.Fatalf("Beri transfer claimed a time skip while the toggle was disabled: %#v", receipt.Plan.Claims)
		}
	}
}

func TestAllOtherBeriPhasesPassProductionResourceAdmission(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[{"wodID":10,"unitType":"melee"}],
		"prebuiltcastles":[{"preBuiltCastleID":"1","spaceIDs":"10","minLevel":15,"costWood":"900","costStone":"900"}],
		"currencies":[{"currencyID":1005,"JSONKey":"MS5"}],
		"currencyMinutesSkipValues":[{"currencyID":"1005","MinutesSkipValue":"60"}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	itemID := int64(10)
	preset := AttackPresets.Preset{
		ID: "beri", Name: "Berimond",
		Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 100}}},
		}},
	}
	attackArguments, _ := json.Marshal(beriTowerAttackRequest{
		SourceCastleID: 900, TargetX: 321, TargetY: 654, TargetTypeID: 17,
		TargetObservedAt: now, CommanderID: 0, Preset: preset, HorseTravelBoostID: -1,
	})
	cases := []struct {
		name       string
		definition Intent.Definition
		arguments  json.RawMessage
		state      func() State.GameState
	}{
		{
			name: "capacity refresh",
			definition: Intent.Definition{
				Name: "beri.capacity.refresh", Effect: Intent.EffectRead, Planner: planBeriCapacityRefresh,
			},
			arguments: json.RawMessage(`{"beriCastleId":900}`),
			state: func() State.GameState {
				gameState := State.NewGameState()
				gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: beriKingdomID}
				return gameState
			},
		},
		{
			name: "camp open",
			definition: Intent.Definition{
				Name: "beri.camp.open", Effect: Intent.EffectWrite, Planner: planBeriCampOpen,
			},
			arguments: json.RawMessage(`{"campId":1}`),
			state: func() State.GameState {
				gameState := State.NewGameState()
				gameState.Player.Level = 70
				gameState.KingdomTransport.ObservedAt = now
				gameState.KingdomTransport.Unlocks[beriKingdomID] = State.KingdomTransportUnlock{
					KingdomID: beriKingdomID, Unlocked: true,
				}
				return gameState
			},
		},
		{
			name: "target find",
			definition: Intent.Definition{
				Name: "beri.target.find", Effect: Intent.EffectRead, Planner: planBeriTargetFind,
			},
			arguments: json.RawMessage(`{"sourceCastleId":900}`),
			state: func() State.GameState {
				gameState := State.NewGameState()
				gameState.Castles[900] = State.CastleState{ID: 900, KingdomID: beriKingdomID}
				return gameState
			},
		},
		{
			name: "tower attack",
			definition: Intent.Definition{
				Name: "beri.tower.attack", Effect: Intent.EffectLaunch,
				AttackModule: &Intent.AttackModuleDefinition{ID: "autoBeriWorld", Label: "Auto Beri World"},
				Planner:      planBeriTowerAttack,
			},
			arguments: attackArguments,
			state: func() State.GameState {
				gameState := State.NewGameState()
				gameState.Castles[900] = State.CastleState{
					ID: 900, KingdomID: beriKingdomID,
					Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 100}},
				}
				gameState.Commanders[0] = State.CommanderState{ID: 0, Available: true}
				gameState.Beri = State.BeriState{
					TargetX: 321, TargetY: 654, TargetTypeID: 17, TargetObservedAt: now,
				}
				gameState.Map[beriKingdomID] = map[string]State.MapObservation{
					"321:654": {
						KingdomID: beriKingdomID, X: 321, Y: 654, TypeID: 17, Level: 55, ObservedAt: now,
					},
				}
				return gameState
			},
		},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			registry := Intent.NewRegistry()
			registry.EnforceResourceDeclarations()
			if err := registry.Register(test.definition); err != nil {
				t.Fatal(err)
			}
			engine := Intent.NewEngine(
				registry, State.NewStore(test.state()), beriIntentGameDataProvider{store: gameData}, nil, nil,
			)
			receipt := engine.Submit(t.Context(), Intent.Request{
				Name: test.definition.Name, DryRun: true, Arguments: test.arguments,
			})
			if receipt.Status != Intent.StatusPlanned || receipt.Plan == nil {
				t.Fatalf("%s resource admission failed: %#v", test.definition.Name, receipt)
			}
			for _, resource := range receipt.Plan.Resources {
				if resource.Capability == "legacy" {
					t.Fatalf("%s retained a legacy resource: %#v", test.definition.Name, receipt.Plan.Resources)
				}
			}
		})
	}
}

type beriIntentGameDataProvider struct {
	store *GameData.Store
}

func (provider beriIntentGameDataProvider) Current() (*GameData.Store, bool) {
	return provider.store, provider.store != nil
}

func TestPlanBeriCampOpenUsesOfficialNonPremiumWireShape(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"buildings":[],"units":[],
		"prebuiltcastles":[
			{"preBuiltCastleID":"1","spaceIDs":"10","minLevel":15,"costWood":"900","costStone":"900"},
			{"preBuiltCastleID":"3","spaceIDs":"10","minLevel":15,"costC2":"49000"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	gameState := State.NewGameState()
	gameState.Player.Level = 70
	gameState.KingdomTransport.ObservedAt = time.Now().UTC()
	gameState.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{KingdomID: 10, Unlocked: true}
	plan, err := planBeriCampOpen(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{"campId":1}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 5 || plan.Steps[0].Opcode != "kpi" ||
		plan.Steps[1].Action != "beri.camp.open.verify" || plan.Steps[1].ResumePolicy != Intent.ResumeRebuild ||
		plan.Steps[2].Opcode != "fsc" || plan.Steps[3].Action != "beri.camp.opened" ||
		plan.Steps[4].Opcode != "kpi" {
		t.Fatalf("unexpected camp plan: %#v", plan.Steps)
	}
	var payload struct {
		ID        int64 `json:"ID"`
		Premium   int   `json:"PWR"`
		Secondary int   `json:"OC2"`
		KingdomID int   `json:"SID"`
	}
	if json.Unmarshal(plan.Steps[2].Command.Payload, &payload) != nil ||
		payload.ID != 1 || payload.Premium != 0 || payload.Secondary != 0 || payload.KingdomID != 10 {
		t.Fatalf("unexpected fsc payload: %s", plan.Steps[2].Command.Payload)
	}
}

func TestPlanBeriTowerAttackUsesFNTAndOmitsADIAndGAS(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"buildings":[
			{"wodID":294,"name":"FactionStable","level":"5","unlockHorses":"1027,1028,1029"}
		],
		"horses":[
			{"wodID":1027,"group":"Travelbooster","comment1":"FactionStable5","comment2":"Horse"},
			{"wodID":1028,"group":"Travelbooster","comment1":"FactionStable5","comment2":"Warhorse"},
			{"wodID":1029,"group":"Travelbooster","comment1":"FactionStable5","comment2":"Courser"}
		],
		"units":[
			{"wodID":10,"unitType":"melee"},
			{"wodID":11,"slotTypes":[1]}
		],
		"effects":[
			{"effectID":700,"name":"attackUnitAmountReinforcementBonus","effectTypeID":179,"capID":99}
		],
		"effectCaps":[{"capID":99}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	gameState := State.NewGameState()
	gameState.Castles[900] = State.CastleState{
		ID: 900, KingdomID: 10, X: 300, Y: 600,
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{10: 300, 11: 60}},
		Buildings: map[State.BuildingInstanceID]State.Building{
			27: {InstanceID: 27, DefinitionID: 294, Placed: true},
		},
	}
	gameState.Commanders[0] = State.CommanderState{
		ID: 0, Available: true,
		Equipment: map[string]State.EquipmentInstanceID{"1": 1001},
		Gems:      map[string]State.GemInstanceID{},
	}
	gameState.Inventory.Equipment[1001] = State.EquipmentInstance{
		ID: 1001, Effects: State.EquipmentEffects{{DefinitionID: 700, Values: []float64{50}}},
	}
	gameState.Player.LegendSkills.ObservedAt = now
	gameState.KingdomTransport.Unlocks[10] = State.KingdomTransportUnlock{KingdomID: 10, Unlocked: true, Created: true}
	gameState.Beri.TargetX = 321
	gameState.Beri.TargetY = 654
	gameState.Beri.TargetTypeID = 17
	gameState.Beri.TargetObservedAt = now
	gameState.Map[10] = map[string]State.MapObservation{
		"321:654": {KingdomID: 10, X: 321, Y: 654, TypeID: 17, ObjectID: -410, Level: 55, ObservedAt: now},
	}
	findPlan, err := planBeriTargetFind(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{"sourceCastleId":900}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(findPlan.Steps) != 5 || findPlan.Steps[0].Opcode != "jaa" ||
		findPlan.Steps[1].Action != "game.ui.close" || findPlan.Steps[2].Opcode != "gbl" ||
		findPlan.Steps[3].Opcode != "fnt" || string(findPlan.Steps[3].Command.Payload) != "{}" ||
		findPlan.Steps[4].Action != "beri.target.verify" {
		t.Fatalf("unexpected find-next plan: %#v", findPlan.Steps)
	}
	if findPlan.Steps[3].ResumePolicy != Intent.ResumeRebuild ||
		findPlan.Steps[3].ResponseBarrier != Intent.ResponseBarrierCommitted {
		t.Fatalf("FNT is not a committed, resumable engine command: %#v", findPlan.Steps[3])
	}
	for _, claim := range []string{"castle-focus", "attack-context", "castle:900", "map:10", "beri-target:10"} {
		if !slices.Contains(findPlan.Claims, claim) {
			t.Fatalf("find-next plan is missing claim %q: %#v", claim, findPlan.Claims)
		}
	}

	source := gameState.Castles[900]
	source.Focused = true
	gameState.Castles[900] = source
	otherCommanderID := State.CommanderID(1)
	otherAttackArrivesAt := now.Add(time.Minute)
	gameState.Movements[77] = State.MovementState{
		ID: 77, Direction: 0, SourceCastleID: 900, TargetTypeID: 17, KingdomID: 10,
		TargetX: 321, TargetY: 654, CommanderID: &otherCommanderID, ArrivesAt: &otherAttackArrivesAt,
	}
	if _, err := currentBeriTarget(gameState, now.Add(-time.Millisecond)); err != nil {
		t.Fatalf("fresh FNT target was blocked by another commander attacking the same tower: %v", err)
	}
	focusedFindPlan, err := planBeriTargetFind(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: gameData},
		json.RawMessage(`{"sourceCastleId":900}`),
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(focusedFindPlan.Steps) != 5 ||
		focusedFindPlan.Steps[0].Opcode != "jca" ||
		focusedFindPlan.Steps[0].AwaitOpcode != "jaa" ||
		focusedFindPlan.Steps[0].ResponseBarrier != Intent.ResponseBarrierCommitted ||
		focusedFindPlan.Steps[1].Action != "game.ui.close" ||
		focusedFindPlan.Steps[2].Opcode != "gbl" ||
		focusedFindPlan.Steps[3].Opcode != "fnt" ||
		focusedFindPlan.Steps[4].Action != "beri.target.verify" {
		t.Fatalf("focused find-next plan does not re-enter the locked Berimond camp: %#v", focusedFindPlan.Steps)
	}
	itemID, toolID := int64(10), int64(11)
	preset := AttackPresets.Preset{
		ID: "beri", Name: "Berimond",
		Waves: []AttackPresets.Wave{{
			Middle: AttackPresets.Lane{
				Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 300}},
				Tools:  []AttackPresets.Slot{{ItemID: &toolID, Quantity: 60}},
			},
		}},
		CourtyardSupport: AttackPresets.CourtyardSupport{
			Troops: []AttackPresets.Slot{{ItemID: &itemID, Quantity: 100}},
		},
	}
	arguments, _ := json.Marshal(beriTowerAttackRequest{
		SourceCastleID: 900, TargetX: 321, TargetY: 654, TargetTypeID: 17,
		TargetObservedAt: now, CommanderID: 0, Preset: preset, HorseTravelBoostID: 1009,
	})
	plan, err := planBeriTowerAttack(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, arguments,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Admission == nil || plan.Admission.Module != "autoBeriWorld" {
		t.Fatalf("unexpected admission: %#v", plan.Admission)
	}
	var focus, gaa, guards, resolvers, analyticsCaptures int
	var guardArguments json.RawMessage
	var launchDependencies *Intent.CommandDependencyRequest
	var launchIndex int
	for index, step := range plan.Steps {
		if step.Opcode != "" && step.Opcode != "jca" && step.Opcode != "gaa" {
			t.Fatalf("cached Berimond attack includes unexpected %s command: %#v", step.Opcode, plan.Steps)
		}
		if step.Opcode == "adi" || step.Opcode == "gas" {
			t.Fatalf("Berimond attack includes forbidden %s setup: %#v", step.Opcode, plan.Steps)
		}
		if step.Opcode == "jca" {
			focus++
			if step.AwaitOpcode != "jaa" || step.ResponseBarrier != Intent.ResponseBarrierCommitted {
				t.Fatalf("Berimond source refocus is not committed before target refresh: %#v", step)
			}
		}
		if step.Opcode == "gaa" {
			gaa++
			if step.ResponseBarrier != Intent.ResponseBarrierCommitted {
				t.Fatalf("Berimond target refresh does not wait for committed map state: %#v", step)
			}
		}
		if step.Action == "beri.tower.attack.guard" {
			guards++
			guardArguments = step.ActionArguments
			if step.ResumePolicy == Intent.ResumeRebuild {
				t.Fatalf("Berimond attack guard would rerun after a confirmed CRA: %#v", step)
			}
		}
		if step.Resolver == "beri.tower.attack.build" {
			resolvers++
			launchIndex = index
			launchDependencies = step.CommandDependencies
		}
		if step.Action == "attack.analytics.capture" {
			analyticsCaptures++
			var capture attackFeatureCaptureRequest
			if err := json.Unmarshal(step.ActionArguments, &capture); err != nil {
				t.Fatal(err)
			}
			if capture.FeatureID != State.AttackFeatureAutoBeriWorld ||
				capture.SourceCastleID != 900 || capture.CommanderID != 0 ||
				capture.KingdomID != 10 || capture.TargetTypeID != 17 ||
				capture.TargetX != 321 || capture.TargetY != 654 {
				t.Fatalf("unexpected Berimond analytics capture: %#v", capture)
			}
		}
		if step.Action == "beri.target.consume" {
			t.Fatalf("successful CRA discarded the reusable Berimond target: %#v", plan.Steps)
		}
	}
	if focus != 2 || gaa != 1 || guards != 1 || resolvers != 1 || analyticsCaptures != 1 {
		t.Fatalf(
			"unexpected cached attack steps: focus=%d gaa=%d guards=%d resolvers=%d analytics=%d steps=%#v",
			focus, gaa, guards, resolvers, analyticsCaptures, plan.Steps,
		)
	}
	postAttackRefresh := plan.Steps[len(plan.Steps)-1]
	if launchIndex != len(plan.Steps)-3 ||
		plan.Steps[len(plan.Steps)-2].Action != "attack.analytics.capture" ||
		postAttackRefresh.Name != "Refresh Berimond source inventory after attack" ||
		postAttackRefresh.Opcode != "jca" || postAttackRefresh.AwaitOpcode != "jaa" ||
		postAttackRefresh.ResponseBarrier != Intent.ResponseBarrierCommitted ||
		postAttackRefresh.ResumePolicy != Intent.ResumeRebuild {
		t.Fatalf("Berimond attack does not commit a post-launch JAA refresh: %#v", plan.Steps)
	}
	if launchDependencies == nil || launchDependencies.Opcode != "cra" {
		t.Fatalf("Berimond CRA does not declare its dedicated dependency context: %#v", launchDependencies)
	}
	dependencies, err := (&Application{}).resolveCRACommandDependencies(
		t.Context(),
		Intent.PlanningContext{State: gameState, GameData: gameData},
		Intent.Step{Command: Protocol.Command{
			Opcode: launchDependencies.Opcode, Payload: launchDependencies.Payload,
		}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(dependencies.Steps) != 0 {
		t.Fatalf("Berimond CRA dependency context reintroduced generic setup: %#v", dependencies.Steps)
	}
	for _, claim := range []string{
		"castle-focus", "attack-context", "castle:900", "attack-inventory:900",
		"map:10", "beri-target:10:321:654", "commander:0", "leader:commander:0",
	} {
		if !slices.Contains(plan.Claims, claim) {
			t.Fatalf("tower attack is missing claim %q: %#v", claim, plan.Claims)
		}
	}
	if _, _, _, err := beriTowerAttackContext(
		Intent.PlanningContext{State: gameState, GameData: gameData}, guardArguments, time.Now().UTC(),
	); err == nil {
		t.Fatal("tower attack accepted the cached target before its targeted GAA refresh")
	}
	guardStore := State.NewStore(gameState)
	guardApplication := &Application{State: guardStore}
	if err := guardApplication.guardBeriTowerAttack(t.Context(), guardArguments); err == nil {
		t.Fatal("GAA guard accepted a target that was not returned by the targeted refresh")
	}
	invalidated := guardStore.Snapshot().Beri
	if !invalidated.TargetObservedAt.Before(invalidated.TargetInvalidatedAt) {
		t.Fatalf("GAA guard did not invalidate the missing cached target: %#v", invalidated)
	}
	var guardedRequest beriTowerAttackRequest
	if err := json.Unmarshal(guardArguments, &guardedRequest); err != nil {
		t.Fatal(err)
	}
	refreshed := gameState.Map[beriKingdomID]["321:654"]
	refreshed.ObservedAt = guardedRequest.TargetRefreshAfter.Add(time.Millisecond)
	gameState.Map[beriKingdomID]["321:654"] = refreshed
	if _, _, _, err := beriTowerAttackContext(
		Intent.PlanningContext{State: gameState, GameData: gameData}, guardArguments, time.Now().UTC(),
	); err != nil {
		t.Fatalf("tower attack rejected the freshly refreshed target: %v", err)
	}
	resolvedStep, err := (&Application{}).resolveBeriTowerAttackStep(
		t.Context(), Intent.PlanningContext{State: gameState, GameData: gameData}, guardArguments,
	)
	if err != nil {
		t.Fatalf("resolve Berimond attack: %v", err)
	}
	var body attackBody
	if err := json.Unmarshal(resolvedStep.Command.Payload, &body); err != nil {
		t.Fatal(err)
	}
	if len(body.Waves) != 1 || len(body.Waves[0].Middle.Units) == 0 ||
		body.Waves[0].Middle.Units[0] != (attackPair{10, 156}) {
		t.Fatalf("Berimond middle lane was not limited to level-55 capacity: %#v", body.Waves)
	}
	if len(body.Waves[0].Middle.Tools) == 0 ||
		body.Waves[0].Middle.Tools[0] != (attackPair{11, 40}) {
		t.Fatalf("Berimond middle tools were not limited to the PvE section cap: %#v", body.Waves)
	}
	if len(body.SupportTroops) == 0 || body.SupportTroops[0] != (attackPair{10, 50}) {
		t.Fatalf("Berimond support wave was not limited by commander effects: %#v", body.SupportTroops)
	}
	if body.Booster != 1029 || body.PremiumTravel != 0 {
		t.Fatalf("Berimond faction horse fields = HBW %d PTT %d, want HBW 1029 PTT 0", body.Booster, body.PremiumTravel)
	}
}

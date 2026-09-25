package Automation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestStrictStormBuildRunsHarborBeforeStorehouseAndLayoutWork(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	state.Session = State.SessionState{Generation: 1, LoggedIn: true, SocketReady: true}
	state.Player.RubyConfirmation = State.RubyConfirmationState{Amount: -1, Known: true, Generation: 1}
	harbor := strictStormBuilding(50, 45, 0, 0)
	castle.Layout.Fixed[50], castle.Buildings[50] = harbor, harbor
	storehouse := strictStormBuilding(60, 132, 20, 0)
	castle.Layout.Objects[60], castle.Buildings[60] = storehouse, storehouse
	extra := strictStormBuilding(70, 700, 40, 0)
	castle.Layout.Objects[70], castle.Buildings[70] = extra, extra
	state.Castles[castle.ID] = castle
	state.Player.Resources[2] = 50_000
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Fixed:     []Buildings.TargetFixedBuilding{{TargetID: "harbor", DefinitionID: 46}},
		Buildings: []Buildings.TargetBuilding{{TargetID: "store", DefinitionID: 232}},
	})
	settings.Build.AllowPremium = true
	settings.Build.AllowDemolition = true

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.upgrade" {
		t.Fatalf("Harbor phase decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var arguments struct {
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.BuildingInstanceID != harbor.InstanceID {
		t.Fatalf("Harbor upgrade arguments=%s err=%v", decision.Request.Arguments, err)
	}
}

func TestStrictStormStorehouseCapsAtSevenAndPreservesHigher(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	low := strictStormBuilding(60, 202, 10, 0)
	high := strictStormBuilding(61, 1943, 30, 0)
	castle.Layout.Objects[60], castle.Buildings[60] = low, low
	castle.Layout.Objects[61], castle.Buildings[61] = high, high
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "store-a", DefinitionID: 232}, {TargetID: "store-b", DefinitionID: 232},
		},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.upgrade" {
		t.Fatalf("Storehouse phase decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var arguments struct {
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		MaximumLevel       int64                    `json:"maximumLevel"`
	}
	_ = json.Unmarshal(decision.Request.Arguments, &arguments)
	if arguments.BuildingInstanceID != low.InstanceID || arguments.MaximumLevel != 7 {
		t.Fatalf("upgrade arguments=%#v, want lower Storehouse %d capped at 7; higher level must be preserved", arguments, low.InstanceID)
	}
}

func TestStrictStormCapsExpansionStorageUpgradeDecisionAtSeven(t *testing.T) {
	gameData := strictStormGameData(t)
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		t.Fatal(err)
	}
	_, castle := strictStormState(time.Now().UTC())
	storehouse := strictStormBuilding(60, 202, 20, 0)
	castle.Layout.Objects[60], castle.Buildings[60] = storehouse, storehouse
	decision := autoStormIntentDecision(time.Now().UTC(), map[string]float64{}, "capacity", "building.upgrade", map[string]any{
		"castleId": castle.ID, "buildingInstanceId": storehouse.InstanceID,
	})
	stormCapStorehouseUpgradeDecision(decision, castle, catalog)
	var arguments struct {
		MaximumLevel int64 `json:"maximumLevel"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &arguments); err != nil || arguments.MaximumLevel != 7 {
		t.Fatalf("capped expansion storage arguments=%s err=%v", decision.Request.Arguments, err)
	}
}

func TestStrictStormCargoEstablishesAllShipsBeforeUpgrade(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	existing := strictStormBuilding(80, 35, 5, 5)
	castle.Layout.Objects[80], castle.Buildings[80] = existing, existing
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "cargo-one", DefinitionID: 37, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
			{TargetID: "cargo-two", DefinitionID: 36, Placement: &Buildings.TargetPlacement{GridX: 20, GridY: 5}},
		},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.construct" {
		t.Fatalf("Cargo breadth decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var arguments struct {
		DefinitionID State.BuildingID `json:"definitionId"`
	}
	_ = json.Unmarshal(decision.Request.Arguments, &arguments)
	if arguments.DefinitionID != 35 {
		t.Fatalf("Cargo construction definition=%d, want official level-1 root", arguments.DefinitionID)
	}
}

func TestStrictStormMissingDecorationWarnsAndContinuesCargo(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	state.Inventory.ItemsObservedAt[stormDecorationStorageCollection] = now
	state.Inventory.Items[stormDecorationStorageCollection] = map[int64]int64{}
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "missing-deco", DefinitionID: 600, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
			{TargetID: "cargo", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 20, GridY: 5}},
		},
	})
	metrics := map[string]float64{}

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, metrics)
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.construct" {
		t.Fatalf("Mixed target decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	if metrics["stormMissingDecorations"] != 1 {
		t.Fatalf("missing decoration metric=%v", metrics)
	}
}

func TestStrictStormDecorationStorageNeedsFreshSegmentOne(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	state.Inventory.Items[stormDecorationStorageCollection] = map[int64]int64{600: 1}
	state.Inventory.ItemsObservedAt["storage:2"] = now
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{{TargetID: "deco", DefinitionID: 600, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}}},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.storage.refresh" {
		t.Fatalf("stale storage decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	continued := autoStormBuildContinuation(*decision)
	if continued.ReevaluateOnSuccess || continued.NextCheckAt.Before(now.Add(29*time.Second)) {
		t.Fatalf("storage refresh continuation=%#v", continued)
	}
}

func TestStrictStormPartialDecorationInventoryPlacesAvailableMultiplicity(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	state.Inventory.ItemsObservedAt[stormDecorationStorageCollection] = now
	state.Inventory.Items[stormDecorationStorageCollection] = map[int64]int64{600: 1}
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "deco-one", DefinitionID: 600, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
			{TargetID: "deco-two", DefinitionID: 600, Placement: &Buildings.TargetPlacement{GridX: 20, GridY: 5}},
		},
	})
	metrics := map[string]float64{}

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, metrics)
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.place" {
		t.Fatalf("partial decoration decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	if metrics["stormMissingDecorations"] != 1 || strings.Contains(strings.ToLower(decision.Detail), "buy") {
		t.Fatalf("partial decoration metrics/detail=%v %q", metrics, decision.Detail)
	}
}

func TestStrictStormWaitsOnExpansionWithoutFallingThroughToCargo(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	castle.Resources[3] = State.ResourceBalance{Amount: 0, Capacity: strictStormFloat(1_000_000)}
	castle.Resources[4] = State.ResourceBalance{Amount: 0, Capacity: strictStormFloat(1_000_000)}
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Ground: []Buildings.TargetGround{
			{DefinitionID: 900, GridX: 0, GridY: 0}, {DefinitionID: 900, GridX: 100, GridY: 0},
		},
		Buildings: []Buildings.TargetBuilding{{TargetID: "cargo", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}}},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || decision != nil || detail == "" {
		t.Fatalf("expansion wait decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
}

func TestStrictStormUnauthorizedCleanupBlocksBeforeMove(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	extra := strictStormBuilding(70, 700, 40, 0)
	cargo := strictStormBuilding(80, 35, 30, 5)
	castle.Layout.Objects[70], castle.Buildings[70] = extra, extra
	castle.Layout.Objects[80], castle.Buildings[80] = cargo, cargo
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{{TargetID: "cargo", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}}},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || decision != nil || !strings.Contains(detail, "enable demolition") {
		t.Fatalf("cleanup gate decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
}

func TestStrictStormMovesOneRetainedBuildingBeforeConstruction(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	cargo := strictStormBuilding(80, 35, 30, 5)
	castle.Layout.Objects[80], castle.Buildings[80] = cargo, cargo
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "retained", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
			{TargetID: "missing", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 50, GridY: 5}},
		},
	})

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.move" {
		t.Fatalf("retained move decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
	var arguments struct {
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	}
	_ = json.Unmarshal(decision.Request.Arguments, &arguments)
	if arguments.BuildingInstanceID != cargo.InstanceID {
		t.Fatalf("moved building=%d, want retained Cargo %d", arguments.BuildingInstanceID, cargo.InstanceID)
	}
}

func TestStrictStormMalformedTargetBlocksDestructiveCleanup(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	extra := strictStormBuilding(70, 700, 40, 0)
	castle.Layout.Objects[70], castle.Buildings[70] = extra, extra
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{
			{TargetID: "one", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
			{TargetID: "two", DefinitionID: 35, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}},
		},
	})
	settings.Build.AllowDemolition = true

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{State: state, GameData: strictStormGameData(t), Now: now}, settings, castle, map[string]float64{})
	if err != nil || complete || decision != nil || !strings.Contains(detail, "overlaps") {
		t.Fatalf("malformed target decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
}

func TestStrictStormNonExactPresetUsesInventoryAwareDecorationPhase(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	state.Inventory.ItemsObservedAt[stormDecorationStorageCollection] = now
	state.Inventory.Items[stormDecorationStorageCollection] = map[int64]int64{600: 1}
	state.Castles[castle.ID] = castle
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeLayout,
	})
	settings.DecorationPresetCastleID = castle.ID
	settings.DecorationPresetID = "storm-decos"
	configuration := map[string]json.RawMessage{
		"decorations.presets": json.RawMessage(`{"version":1,"castles":{"40":[{"id":"storm-decos","name":"Storm","items":[{"wid":600,"x":5,"y":5,"r":0}]}]}}`),
	}

	decision, complete, detail, err := evaluateStrictAutoStormBuild(Snapshot{
		State: state, GameData: strictStormGameData(t), Now: now,
		Configuration: Configuration.Snapshot{Sections: configuration},
	}, settings, castle, map[string]float64{})
	if err != nil || complete || detail != "" || decision == nil || decision.Request == nil || decision.Request.Name != "building.place" {
		t.Fatalf("preset decoration decision=%#v complete=%t detail=%q err=%v", decision, complete, detail, err)
	}
}

func TestStormCargoGateCannotUpgradeWhileAnyTargetHasNoSource(t *testing.T) {
	diff := Buildings.TargetDiffResult{Targets: []Buildings.TargetMatch{
		{TargetID: "existing", Source: &Buildings.TargetSource{Kind: Buildings.TargetSourceExisting}},
		{TargetID: "missing"},
	}, Actions: []Buildings.TargetAction{{Intent: "building.upgrade"}}}
	if !stormCargoNeedsInitialPlacement(diff) {
		t.Fatal("missing Cargo source did not hold the breadth-before-depth gate")
	}
}

func TestAutoStormMissingOnlyCompletionKeepsBoundedStorageRecheckWithProtectedStorehouse(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	storehouse := strictStormBuilding(60, 232, 20, 0)
	castle.Layout.Objects[60], castle.Buildings[60] = storehouse, storehouse
	state.Castles[castle.ID] = castle
	state.Inventory.ItemsObservedAt[stormDecorationStorageCollection] = now
	settings := strictStormSettings(Buildings.TargetCaptureResult{
		Version: 1, KingdomID: autoStormKingdomID, Mode: Buildings.TargetCaptureModeExact, Exact: true,
		Buildings: []Buildings.TargetBuilding{{TargetID: "deco", DefinitionID: 600, Placement: &Buildings.TargetPlacement{GridX: 5, GridY: 5}}},
	})
	raw, err := json.Marshal(settings)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := NewAutoStormBuildPolicy().Evaluate(t.Context(), Snapshot{
		State: state, GameData: strictStormGameData(t), Now: now,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{autoStormSection: raw}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "complete" || decision.EventDriven || !decision.NextCheckAt.Equal(now.Add(stormDecorationStorageMaxAge)) ||
		decision.Metrics["stormMissingDecorations"] != 1 || decision.Metrics["unmanagedBuildings"] != 1 {
		t.Fatalf("missing-only policy decision=%#v", decision)
	}
}

func strictStormSettings(target Buildings.TargetCaptureResult) autoStormSettings {
	settings := defaultAutoStormSettings()
	settings.Target = &target
	return settings
}

func strictStormState(now time.Time) (State.GameState, State.CastleState) {
	state := State.NewGameState()
	state.Player.Level = 70
	state.Player.Resources[9] = 1_000_000
	state.Player.Resources[2] = 1_000_000
	ground := strictStormBuilding(1, 900, 0, 0)
	ground.Layer = State.BuildingLayerBG
	castle := State.CastleState{
		ID: 40, KingdomID: autoStormKingdomID, Focused: true,
		Resources: map[State.ResourceID]State.ResourceBalance{
			3: {Amount: 1_000_000, Capacity: strictStormFloat(1_000_000)},
			4: {Amount: 1_000_000, Capacity: strictStormFloat(1_000_000)},
		},
		Buildings: map[State.BuildingInstanceID]State.Building{1: ground},
		Layout: State.CastleLayout{
			Ground: map[State.BuildingInstanceID]State.Building{1: ground}, Objects: map[State.BuildingInstanceID]State.Building{},
			Fixed: map[State.BuildingInstanceID]State.Building{}, ObservedAt: now,
		},
		BuildingQueue: State.BuildingConstructionQueue{
			SlotCount: 1, ObservedAt: now,
			Slots: []State.BuildingConstructionQueueSlot{{Index: 0, WireValue: -1, Status: State.BuildingQueueSlotAvailable}},
		},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{}, ConstructionSlotsObservedAt: now,
	}
	state.Castles[castle.ID] = castle
	state.Inventory.Items[stormDecorationStorageCollection] = map[int64]int64{}
	state.Inventory.ConstructionItemsObservedAt = now
	return state, castle
}

func strictStormBuilding(instanceID State.BuildingInstanceID, definitionID State.BuildingID, x, y int) State.Building {
	return State.Building{InstanceID: instanceID, DefinitionID: definitionID, GridX: x, GridY: y, Layer: State.BuildingLayerBD, Placed: true, ConstructionState: State.BuildingStateBuildCompleted}
}

func strictStormFloat(value float64) *float64 { return &value }

func strictStormGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":{"version":"storm-strict-test"},"units":[],
		"resources":[
			{"resourceID":"2","JSONKey":"C2","name":"premium"},
			{"resourceID":"3","JSONKey":"W","name":"wood"},
			{"resourceID":"4","JSONKey":"S","name":"stone"},
			{"resourceID":"9","JSONKey":"C1","name":"aquamarine"}
		],
		"expansions":[{"expansionID":"1","expansionLevel":"1","spaceIDs":"4","costWood":"5000","costStone":"5000"}],
		"buildings":[
			{"wodID":"900","name":"Ground","group":"Ground","width":"100","height":"100"},
			{"wodID":"45","name":"Harbor","group":"FixedPositionBuilding","forcedPosition":"harbor","level":"1","width":"14","height":"7","upgradeWodID":"46","movable":"0","destructable":"0","costWood":"170","costStone":"135"},
			{"wodID":"46","name":"Harbor","group":"FixedPositionBuilding","forcedPosition":"harbor","level":"2","width":"14","height":"7","downgradeWodID":"45","upgradeWodID":"47","movable":"0","destructable":"0","costC2":"12300"},
			{"wodID":"47","name":"Harbor","group":"FixedPositionBuilding","forcedPosition":"harbor","level":"3","width":"14","height":"7","downgradeWodID":"46","movable":"0","destructable":"0","costC2":"24000"},
			{"wodID":"132","name":"Storehouse","group":"Building","type":"Level1","shopCategory":"CIVIL","level":"1","width":"5","height":"10","upgradeWodID":"133","movable":"1","storeable":"1","destructable":"1","costWood":"10","woodStorage":"1000","stoneStorage":"1000"},
			{"wodID":"133","name":"Storehouse","group":"Building","level":"2","width":"5","height":"10","downgradeWodID":"132","upgradeWodID":"134","costWood":"10","woodStorage":"2000","stoneStorage":"2000"},
			{"wodID":"134","name":"Storehouse","group":"Building","level":"3","width":"5","height":"10","downgradeWodID":"133","upgradeWodID":"135","costWood":"10","woodStorage":"3000","stoneStorage":"3000"},
			{"wodID":"135","name":"Storehouse","group":"Building","level":"4","width":"5","height":"10","downgradeWodID":"134","upgradeWodID":"136","costWood":"10","woodStorage":"4000","stoneStorage":"4000"},
			{"wodID":"136","name":"Storehouse","group":"Building","level":"5","width":"5","height":"10","downgradeWodID":"135","upgradeWodID":"202","costWood":"10","woodStorage":"5000","stoneStorage":"5000"},
			{"wodID":"202","name":"Storehouse","group":"Building","level":"6","width":"5","height":"10","downgradeWodID":"136","upgradeWodID":"232","costWood":"10","woodStorage":"6000","stoneStorage":"6000"},
			{"wodID":"232","name":"Storehouse","group":"Building","level":"7","width":"5","height":"10","downgradeWodID":"202","upgradeWodID":"1943","requiredLevel":"45","costWood":"10","woodStorage":"80000","stoneStorage":"80000","aquaStorage":"600000"},
			{"wodID":"1943","name":"Storehouse","group":"Building","level":"8","width":"5","height":"10","downgradeWodID":"232","costWood":"10","woodStorage":"90000","stoneStorage":"90000","aquaStorage":"700000"},
			{"wodID":"35","name":"Cargo","group":"Building","buildingGroundType":"DECO","type":"Level1","shopCategory":"CIVIL","level":"1","width":"5","height":"10","upgradeWodID":"36","movable":"1","storeable":"1","destructable":"1","costC1":"100"},
			{"wodID":"36","name":"Cargo","group":"Building","buildingGroundType":"DECO","level":"2","width":"5","height":"10","downgradeWodID":"35","upgradeWodID":"37","costC1":"100"},
			{"wodID":"37","name":"Cargo","group":"Building","buildingGroundType":"DECO","level":"3","width":"5","height":"10","downgradeWodID":"36","costC1":"100"},
			{"wodID":"600","name":"EventDecoration","group":"Decoration","buildingGroundType":"DECO","level":"1","width":"5","height":"5","movable":"1","storeable":"1","destructable":"1"},
			{"wodID":"700","name":"Woodcutter","group":"Building","type":"Level1","shopCategory":"CIVIL","level":"1","width":"5","height":"5","movable":"1","storeable":"0","destructable":"1","costWood":"10"}
		]
	}`), GameData.SourceMetadata{ItemVersion: "storm-strict-test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestStormRubyBlockedHarborContinuesStorehouseAndNotifies(t *testing.T) {
	now := time.Now().UTC()
	state, castle := strictStormState(now)
	harbor := strictStormBuilding(50, 45, 0, 0)
	castle.Layout.Fixed[50], castle.Buildings[50] = harbor, harbor
	storehouse := strictStormBuilding(60, 132, 20, 0)
	castle.Layout.Objects[60], castle.Buildings[60] = storehouse, storehouse
	state.Castles[castle.ID] = castle
	state.Player.Resources[2] = 50000
	state.Session = State.SessionState{Generation: 1, LoggedIn: true, SocketReady: true}
	state.Player.RubyConfirmation = State.RubyConfirmationState{Amount: 1, Known: true, Generation: 1}
	settings := strictStormSettings(Buildings.TargetCaptureResult{Version: 1, KingdomID: autoStormKingdomID, Fixed: []Buildings.TargetFixedBuilding{{TargetID: "harbor", DefinitionID: 46}}, Buildings: []Buildings.TargetBuilding{{TargetID: "store", DefinitionID: 232}}})
	settings.Build.AllowPremium = true
	snapshot := Snapshot{State: state, GameData: strictStormGameData(t), Now: now}
	decision, complete, detail, err := evaluateStrictAutoStormBuild(snapshot, settings, castle, map[string]float64{})
	if err != nil || complete || decision == nil || decision.Request == nil {
		t.Fatalf("decision=%+v detail=%s err=%v", decision, detail, err)
	}
	var args struct {
		BuildingID State.BuildingInstanceID `json:"buildingInstanceId"`
	}
	_ = json.Unmarshal(decision.Request.Arguments, &args)
	if args.BuildingID != 60 {
		t.Fatalf("args=%s", decision.Request.Arguments)
	}
	attachStormRubyUpgradeNotices(decision, snapshot, castle, settings)
	notice := decision.Details["rubyUpgradeNotice/harbor"]
	if !strings.Contains(notice, "Harbor") || !strings.Contains(notice, "12,300") {
		t.Fatalf("notice=%s", notice)
	}
}

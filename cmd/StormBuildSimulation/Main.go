package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"CitadelDesktop/Server/Automation"
	"CitadelDesktop/Server/Buildings"
	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

const mockBalance = 1_000_000_000_000

type options struct {
	dataDir   string
	itemsPath string
	castleID  int64
}

type simulationReport struct {
	Isolated       bool             `json:"isolated"`
	NetworkCalls   int              `json:"networkCalls"`
	LiveWrites     int              `json:"liveWrites"`
	SourceState    string           `json:"sourceState"`
	OfficialData   string           `json:"officialData"`
	CatalogVersion string           `json:"catalogVersion"`
	CastleID       State.CastleID   `json:"castleId"`
	GeneratedAt    time.Time        `json:"generatedAt"`
	Scenarios      []scenarioResult `json:"scenarios"`
	HarborChecks   []scenarioResult `json:"harborChecks"`
}

type scenarioResult struct {
	Name           string                         `json:"name"`
	Mode           string                         `json:"mode"`
	Target         Buildings.TargetCaptureSummary `json:"target"`
	ResetGround    int                            `json:"resetGround"`
	ResetBuildings int                            `json:"resetBuildings"`
	ResetFixed     int                            `json:"resetFixed"`
	Iterations     int                            `json:"iterations"`
	Complete       bool                           `json:"complete"`
	FinalStatus    string                         `json:"finalStatus"`
	FinalDetail    string                         `json:"finalDetail"`
	ActionCounts   map[string]int                 `json:"actionCounts"`
	TimeSkipUses   map[string]int                 `json:"timeSkipUses"`
	CostTotals     map[string]float64             `json:"costTotals"`
	PremiumSpent   float64                        `json:"premiumSpent"`
	FinalGround    int                            `json:"finalGround"`
	FinalBuildings int                            `json:"finalBuildings"`
	FinalFixed     int                            `json:"finalFixed"`
	Blocker        string                         `json:"blocker,omitempty"`
}

type pendingBuildingOperation struct {
	kind        string
	target      State.BuildingID
	durationSec int64
}

type mockRun struct {
	state    State.GameState
	gameData *GameData.Store
	target   Buildings.TargetCaptureResult
	now      time.Time
	nextID   State.BuildingInstanceID
	pending  map[State.BuildingInstanceID]pendingBuildingOperation
	result   scenarioResult
}

type intentArguments struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	DefinitionID       State.BuildingID         `json:"definitionId"`
	X                  int                      `json:"x"`
	Y                  int                      `json:"y"`
	Rotation           int                      `json:"rotation"`
	Direction          int                      `json:"direction"`
	Minutes            int                      `json:"minutes"`
}

func main() {
	configured := parseOptions()
	rawItems, err := os.ReadFile(configured.itemsPath)
	fatalIf(err, "read official item data")
	version := strings.TrimSuffix(strings.TrimPrefix(filepath.Base(configured.itemsPath), "Items-v"), ".json")
	gameData, err := GameData.DecodeStore(rawItems, GameData.SourceMetadata{
		ItemVersion: version, SourceURL: "mock://official-catalog", LoadedAt: time.Now().UTC(),
	})
	fatalIf(err, "decode official item data")
	state, err := State.LoadSnapshot(configured.dataDir)
	fatalIf(err, "load cloned source state")
	castleID, err := stormCastleID(state, State.CastleID(configured.castleID))
	fatalIf(err, "select Storm castle")

	report := simulationReport{
		Isolated: true, NetworkCalls: 0, LiveWrites: 0,
		SourceState:  filepath.Join(configured.dataDir, "State", "GameState.json"),
		OfficialData: configured.itemsPath, CatalogVersion: version, CastleID: castleID,
		GeneratedAt: time.Now().UTC(), Scenarios: []scenarioResult{}, HarborChecks: []scenarioResult{},
	}
	captures := make(map[string]Buildings.TargetCaptureResult, 3)
	for _, mode := range []string{
		Buildings.TargetCaptureModeFunctional,
		Buildings.TargetCaptureModeLayout,
		Buildings.TargetCaptureModeExact,
	} {
		target, captureErr := Buildings.CaptureTarget(state, gameData, Buildings.TargetCaptureRequest{
			CastleID: castleID, Mode: mode,
		})
		fatalIf(captureErr, "capture "+mode+" target")
		captures[mode] = target
		run, runErr := newMockRun(state, gameData, target, false)
		fatalIf(runErr, "prepare "+mode+" reset")
		report.Scenarios = append(report.Scenarios, run.execute(mode))
	}

	harborTarget, harborErr := targetWithHarbor(captures[Buildings.TargetCaptureModeFunctional], gameData, 3)
	fatalIf(harborErr, "prepare Harbor target")
	withRoot, err := newMockRun(state, gameData, harborTarget, true)
	fatalIf(err, "prepare Harbor-root reset")
	report.HarborChecks = append(report.HarborChecks, withRoot.execute("harbor-level-3-from-observed-root"))
	withoutRoot, err := newMockRun(state, gameData, harborTarget, false)
	fatalIf(err, "prepare Harbor-missing reset")
	report.HarborChecks = append(report.HarborChecks, withoutRoot.execute("harbor-level-3-without-root"))

	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	fatalIf(encoder.Encode(report), "encode simulation report")
}

func parseOptions() options {
	configured := options{}
	flag.StringVar(&configured.dataDir, "data-dir", "Data", "read-only Citadel data directory containing State/GameState.json")
	flag.StringVar(&configured.itemsPath, "items", filepath.Join("Data", "GameData", "Items", "Items-v780.01.json"), "official item JSON")
	flag.Int64Var(&configured.castleID, "castle-id", 0, "Storm castle id; zero auto-detects")
	flag.Parse()
	var err error
	configured.dataDir, err = filepath.Abs(configured.dataDir)
	fatalIf(err, "resolve data directory")
	configured.itemsPath, err = filepath.Abs(configured.itemsPath)
	fatalIf(err, "resolve item data")
	return configured
}

func newMockRun(
	source State.GameState,
	gameData *GameData.Store,
	target Buildings.TargetCaptureResult,
	includeHarborRoot bool,
) (*mockRun, error) {
	state, err := cloneState(source)
	if err != nil {
		return nil, err
	}
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()
	castle, exists := state.Castles[target.CastleID]
	if !exists {
		return nil, fmt.Errorf("castle %d is unavailable", target.CastleID)
	}
	for id, candidate := range state.Castles {
		candidate.Focused = id == target.CastleID
		state.Castles[id] = candidate
	}
	original := castle
	castle.Focused = true
	castle.Layout.Ground = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.Objects = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.Fixed = map[State.BuildingInstanceID]State.Building{}
	castle.Buildings = map[State.BuildingInstanceID]State.Building{}
	castle.Layout.ObservedAt = now
	castle.BuildingQueue = State.BuildingConstructionQueue{
		SlotCount: 1, ObservedAt: now,
		Slots: []State.BuildingConstructionQueueSlot{{Index: 0, Status: State.BuildingQueueSlotAvailable}},
	}
	if ground, found := initialGround(original); found {
		castle.Layout.Ground[ground.InstanceID] = ground
		castle.Buildings[ground.InstanceID] = ground
	}
	if keep, found := initialKeep(original, catalog); found {
		keep.ConstructionState = State.BuildingStateBuildCompleted
		keep.ProgressSec = 0
		castle.Layout.Objects[keep.InstanceID] = keep
		castle.Buildings[keep.InstanceID] = keep
	}
	for _, fixed := range resetFixedBuildings(original, catalog) {
		castle.Layout.Fixed[fixed.InstanceID] = fixed
		castle.Buildings[fixed.InstanceID] = fixed
	}
	if includeHarborRoot {
		harbor, found := harborDefinition(catalog, 1)
		if !found {
			return nil, fmt.Errorf("official Harbor level 1 is unavailable")
		}
		id := nextBuildingID(state)
		building := State.Building{
			InstanceID: id, DefinitionID: State.BuildingID(harbor.ID),
			ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBG, Placed: true,
		}
		castle.Layout.Fixed[id] = building
		castle.Buildings[id] = building
	}
	prepareAbundantBalances(&state, &castle, gameData)
	state.Castles[castle.ID] = castle
	state.Inventory.Items["storage:1"] = map[int64]int64{}
	if target.Mode == Buildings.TargetCaptureModeExact {
		for _, building := range target.Buildings {
			definition, found := catalog.Definition(int64(building.DefinitionID))
			if found && isDecoration(definition) {
				state.Inventory.Items["storage:1"][int64(building.DefinitionID)]++
			}
		}
	}
	state.KingdomTransport.ObservedAt = now
	state.KingdomTransport.Pending = []State.KingdomResourceTransport{}
	state.KingdomTransport.ResourceWorkflows = map[State.KingdomID]State.KingdomResourceTransportWorkflow{}
	if state.KingdomTransport.Unlocks == nil {
		state.KingdomTransport.Unlocks = map[State.KingdomID]State.KingdomTransportUnlock{}
	}
	state.KingdomTransport.Unlocks[castle.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: castle.KingdomID, Unlocked: true,
	}
	return &mockRun{
		state: state, gameData: gameData, target: target, now: now, nextID: nextBuildingID(state),
		pending: map[State.BuildingInstanceID]pendingBuildingOperation{},
		result: scenarioResult{
			Mode: target.Mode, Target: target.Summary,
			ResetGround: len(castle.Layout.Ground), ResetBuildings: len(castle.Layout.Objects), ResetFixed: len(castle.Layout.Fixed),
			ActionCounts: map[string]int{}, TimeSkipUses: map[string]int{}, CostTotals: map[string]float64{},
		},
	}, nil
}

func (run *mockRun) execute(name string) scenarioResult {
	run.result.Name = name
	policy := Automation.NewAutoStormBuildPolicy()
	for iteration := 1; iteration <= 2_000; iteration++ {
		run.result.Iterations = iteration
		configuration, err := mockConfiguration(run.target)
		if err != nil {
			run.fail(err.Error())
			break
		}
		decision, err := policy.Evaluate(context.Background(), Automation.Snapshot{
			State: run.state, Configuration: configuration, GameData: run.gameData, Now: run.now,
		})
		if err != nil {
			run.fail(err.Error())
			break
		}
		run.result.FinalStatus = decision.Status
		run.result.FinalDetail = decision.Detail
		if decision.Request == nil {
			if decision.Status == "complete" {
				run.result.Complete = true
			} else {
				run.result.Blocker = decision.Detail
			}
			break
		}
		run.result.ActionCounts[decision.Request.Name]++
		if err := run.apply(decision.Request.Name, decision.Request.Arguments); err != nil {
			run.fail(err.Error())
			break
		}
		run.now = run.now.Add(time.Second)
	}
	castle := run.state.Castles[run.target.CastleID]
	run.result.FinalGround = len(castle.Layout.Ground)
	run.result.FinalBuildings = len(castle.Layout.Objects)
	run.result.FinalFixed = len(castle.Layout.Fixed)
	if !run.result.Complete && run.result.Blocker == "" {
		run.result.Blocker = "mock iteration limit reached"
	}
	return run.result
}

func (run *mockRun) apply(intent string, raw json.RawMessage) error {
	var arguments intentArguments
	if err := json.Unmarshal(raw, &arguments); err != nil {
		return err
	}
	switch intent {
	case "building.refresh":
		return run.touchCastle(arguments.CastleID)
	case "building.expand":
		return run.applyExpansion(arguments)
	case "building.construct", "building.place":
		return run.applyCreate(intent, arguments)
	case "building.move":
		return run.applyMove(arguments)
	case "building.upgrade":
		return run.applyUpgrade(arguments)
	case "building.store":
		return run.applyStore(arguments)
	case "building.demolish":
		return run.applyDemolition(arguments)
	case "building.skip_time":
		run.result.TimeSkipUses[fmt.Sprintf("%dm", arguments.Minutes)]++
		return run.applyBuildingProgress(arguments.BuildingInstanceID, int64(arguments.Minutes*60))
	case "building.finish_free":
		return run.applyBuildingProgress(arguments.BuildingInstanceID, 1<<62)
	default:
		return fmt.Errorf("mock executor does not support emitted intent %s", intent)
	}
}

func (run *mockRun) applyExpansion(arguments intentArguments) error {
	castle := run.state.Castles[arguments.CastleID]
	x, y, direction := arguments.X, arguments.Y, arguments.Direction
	preview, err := Buildings.PreviewExpansion(run.state, run.gameData, Buildings.ExpansionPreviewRequest{
		CastleID: castle.ID, Payment: Buildings.ExpansionPaymentResources,
		AllowPremium: true, AllowTimeSkips: true, X: &x, Y: &y, Direction: &direction,
	})
	if err != nil {
		return err
	}
	if preview.RecommendedAction == nil || preview.RecommendedAction.Intent != "building.expand" || preview.NextExpansion == nil {
		return fmt.Errorf("expansion %d is not actionable in the mock: %s", preview.CurrentExpansionLevel+1, firstExpansionBlocker(preview))
	}
	var ground Buildings.TargetGround
	found := false
	for _, candidate := range run.target.Ground {
		if candidate.GridX == x && candidate.GridY == y && candidate.Direction == direction {
			ground, found = candidate, true
			break
		}
	}
	if !found {
		return fmt.Errorf("emitted expansion coordinate %d:%d:%d is not in the target", x, y, direction)
	}
	run.consumeCosts(castle.ID, preview.Costs)
	id := run.allocateID()
	building := State.Building{
		InstanceID: id, DefinitionID: ground.DefinitionID, GridX: x, GridY: y, Rotation: direction,
		ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerG, Placed: true,
	}
	castle = run.state.Castles[castle.ID]
	castle.Layout.Ground[id] = building
	castle.Buildings[id] = building
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyCreate(intent string, arguments intentArguments) error {
	definition, found := run.catalogDefinition(arguments.DefinitionID)
	if !found {
		return fmt.Errorf("definition %d is unavailable", arguments.DefinitionID)
	}
	if intent == "building.place" {
		if run.state.Inventory.Items["storage:1"][int64(arguments.DefinitionID)] <= 0 {
			return fmt.Errorf("definition %d is not present in mock storage", arguments.DefinitionID)
		}
		run.state.Inventory.Items["storage:1"][int64(arguments.DefinitionID)]--
	}
	if intent == "building.construct" {
		run.consumeDefinitionCosts(arguments.CastleID, definition)
	}
	id := run.allocateID()
	building := State.Building{
		InstanceID: id, DefinitionID: arguments.DefinitionID,
		GridX: arguments.X, GridY: arguments.Y, Rotation: arguments.Rotation,
		ConstructionState: State.BuildingStateBuildCompleted, Layer: State.BuildingLayerBD, Placed: true,
	}
	castle := run.state.Castles[arguments.CastleID]
	castle.Layout.Objects[id] = building
	castle.Buildings[id] = building
	if intent == "building.construct" {
		if definition.DurationSec > 0 {
			building.ConstructionState = State.BuildingStateBuildInProgress
			castle.Layout.Objects[id] = building
			castle.Buildings[id] = building
			run.pending[id] = pendingBuildingOperation{kind: "construct", durationSec: definition.DurationSec}
			occupyQueue(&castle, id)
		}
	}
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyMove(arguments intentArguments) error {
	castle := run.state.Castles[arguments.CastleID]
	building, location, found := mockBuilding(castle, arguments.BuildingInstanceID)
	if !found {
		return fmt.Errorf("building %d is unavailable for move", arguments.BuildingInstanceID)
	}
	building.GridX, building.GridY, building.Rotation = arguments.X, arguments.Y, arguments.Rotation
	setMockBuilding(&castle, location, building)
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyUpgrade(arguments intentArguments) error {
	castle := run.state.Castles[arguments.CastleID]
	building, location, found := mockBuilding(castle, arguments.BuildingInstanceID)
	if !found {
		return fmt.Errorf("building %d is unavailable for upgrade", arguments.BuildingInstanceID)
	}
	current, found := run.catalogDefinition(building.DefinitionID)
	if !found || current.UpgradeDefinitionID <= 0 {
		return fmt.Errorf("building %d definition %d has no upgrade", building.InstanceID, building.DefinitionID)
	}
	next, found := run.catalogDefinition(State.BuildingID(current.UpgradeDefinitionID))
	if !found {
		return fmt.Errorf("upgrade definition %d is unavailable", current.UpgradeDefinitionID)
	}
	run.consumeDefinitionCosts(castle.ID, next)
	castle = run.state.Castles[castle.ID]
	building, location, _ = mockBuilding(castle, arguments.BuildingInstanceID)
	if next.DurationSec <= 0 {
		building.DefinitionID = State.BuildingID(next.ID)
		building.ConstructionState = State.BuildingStateBuildCompleted
	} else {
		building.ConstructionState = State.BuildingStateUpgradeInProgress
		building.ProgressSec = 0
		run.pending[building.InstanceID] = pendingBuildingOperation{
			kind: "upgrade", target: State.BuildingID(next.ID), durationSec: next.DurationSec,
		}
		occupyQueue(&castle, building.InstanceID)
	}
	setMockBuilding(&castle, location, building)
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyStore(arguments intentArguments) error {
	castle := run.state.Castles[arguments.CastleID]
	building, location, found := mockBuilding(castle, arguments.BuildingInstanceID)
	if !found || location != "objects" {
		return fmt.Errorf("building %d cannot be stored in the mock", arguments.BuildingInstanceID)
	}
	delete(castle.Layout.Objects, building.InstanceID)
	delete(castle.Buildings, building.InstanceID)
	run.state.Inventory.Items["storage:1"][int64(building.DefinitionID)]++
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyDemolition(arguments intentArguments) error {
	castle := run.state.Castles[arguments.CastleID]
	building, location, found := mockBuilding(castle, arguments.BuildingInstanceID)
	if !found || location != "objects" {
		return fmt.Errorf("building %d cannot be demolished in the mock", arguments.BuildingInstanceID)
	}
	definition, found := run.catalogDefinition(building.DefinitionID)
	if !found {
		return fmt.Errorf("building %d definition is unavailable", building.InstanceID)
	}
	if definition.DurationSec <= 0 {
		delete(castle.Layout.Objects, building.InstanceID)
		delete(castle.Buildings, building.InstanceID)
	} else {
		building.ConstructionState = State.BuildingStateDisassembleInProgress
		building.ProgressSec = 0
		run.pending[building.InstanceID] = pendingBuildingOperation{kind: "demolish", durationSec: definition.DurationSec}
		setMockBuilding(&castle, location, building)
		occupyQueue(&castle, building.InstanceID)
	}
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) applyBuildingProgress(buildingID State.BuildingInstanceID, seconds int64) error {
	operation, exists := run.pending[buildingID]
	if !exists {
		return fmt.Errorf("building %d has no pending mock operation", buildingID)
	}
	castle := run.state.Castles[run.target.CastleID]
	building, location, found := mockBuilding(castle, buildingID)
	if !found {
		return fmt.Errorf("pending building %d is unavailable", buildingID)
	}
	building.ProgressSec += seconds
	if building.ProgressSec < operation.durationSec {
		setMockBuilding(&castle, location, building)
		run.storeCastle(castle)
		return nil
	}
	switch operation.kind {
	case "construct":
		building.ConstructionState = State.BuildingStateBuildCompleted
		building.ProgressSec = 0
		setMockBuilding(&castle, location, building)
	case "upgrade":
		building.DefinitionID = operation.target
		building.ConstructionState = State.BuildingStateBuildCompleted
		building.ProgressSec = 0
		setMockBuilding(&castle, location, building)
	case "demolish":
		delete(castle.Layout.Objects, buildingID)
		delete(castle.Buildings, buildingID)
	default:
		return fmt.Errorf("unsupported pending operation %s", operation.kind)
	}
	delete(run.pending, buildingID)
	releaseQueue(&castle, buildingID)
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) consumeDefinitionCosts(castleID State.CastleID, definition GameData.BuildingDefinition) {
	statuses := make([]Buildings.ExpansionCostStatus, 0, len(definition.Costs))
	for _, cost := range definition.Costs {
		statuses = append(statuses, Buildings.ExpansionCostStatus{CostStatus: Buildings.CostStatus{
			Key: cost.Key, Scope: cost.Scope, DefinitionID: cost.DefinitionID,
			Required: cost.Amount, Premium: cost.Premium,
		}})
	}
	run.consumeCosts(castleID, statuses)
}

func (run *mockRun) consumeCosts(castleID State.CastleID, costs []Buildings.ExpansionCostStatus) {
	castle := run.state.Castles[castleID]
	for _, cost := range costs {
		if cost.Required <= 0 {
			continue
		}
		key := cost.Key
		if key == "" {
			key = fmt.Sprintf("%s:%d", cost.Scope, cost.DefinitionID)
		}
		run.result.CostTotals[key] += cost.Required
		if cost.Premium {
			run.result.PremiumSpent += cost.Required
		}
		switch cost.Scope {
		case GameData.BuildingCostCastleResource:
			balance := castle.Resources[State.ResourceID(cost.DefinitionID)]
			balance.Amount -= cost.Required
			castle.Resources[State.ResourceID(cost.DefinitionID)] = balance
		case GameData.BuildingCostPlayerResource:
			run.state.Player.Resources[State.ResourceID(cost.DefinitionID)] -= cost.Required
		case GameData.BuildingCostCurrency:
			run.state.Player.Currencies[State.CurrencyID(cost.DefinitionID)] -= cost.Required
		}
	}
	run.state.Castles[castleID] = castle
}

func (run *mockRun) touchCastle(castleID State.CastleID) error {
	castle, found := run.state.Castles[castleID]
	if !found {
		return fmt.Errorf("castle %d is unavailable", castleID)
	}
	run.storeCastle(castle)
	return nil
}

func (run *mockRun) storeCastle(castle State.CastleState) {
	castle.Layout.ObservedAt = run.now
	castle.BuildingQueue.ObservedAt = run.now
	run.state.Castles[castle.ID] = castle
	run.state.Revision++
}

func (run *mockRun) allocateID() State.BuildingInstanceID {
	id := run.nextID
	run.nextID++
	return id
}

func (run *mockRun) catalogDefinition(id State.BuildingID) (GameData.BuildingDefinition, bool) {
	catalog, err := run.gameData.BuildingCatalog()
	if err != nil {
		return GameData.BuildingDefinition{}, false
	}
	return catalog.Definition(int64(id))
}

func (run *mockRun) fail(message string) {
	run.result.FinalStatus = "blocked"
	run.result.FinalDetail = message
	run.result.Blocker = message
}

func mockConfiguration(target Buildings.TargetCaptureResult) (Configuration.Snapshot, error) {
	settings, err := json.Marshal(map[string]any{
		"version": 1,
		"build": map[string]any{
			"allowPremium": true, "allowDemolition": true, "allowResourceTransport": true, "allowTimeSkips": true,
			"resourceReserves": map[string]float64{}, "sourceResourceReserves": map[string]float64{},
			"timeSkipReserve": map[string]int64{},
		},
		"harbor":           map[string]any{"enabled": false, "targetLevel": 1},
		"checkIntervalSec": 30,
	})
	if err != nil {
		return Configuration.Snapshot{}, err
	}
	id := Buildings.StormBlueprintID(target.Mode)
	document := Buildings.EmptyStormBlueprintDocument()
	document.ActiveID = id
	document.Blueprints[id] = Buildings.StormBlueprint{
		ID: id, Name: Buildings.StormBlueprintName(target.Mode),
		CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Target: target,
	}
	blueprints, err := json.Marshal(document)
	if err != nil {
		return Configuration.Snapshot{}, err
	}
	return Configuration.Snapshot{Sections: map[string]json.RawMessage{
		"automation.autoStorm":                       settings,
		Buildings.StormBlueprintConfigurationSection: blueprints,
	}}, nil
}

func prepareAbundantBalances(state *State.GameState, castle *State.CastleState, gameData *GameData.Store) {
	if castle.Resources == nil {
		castle.Resources = map[State.ResourceID]State.ResourceBalance{}
	}
	if state.Player.Resources == nil {
		state.Player.Resources = map[State.ResourceID]float64{}
	}
	if state.Player.Currencies == nil {
		state.Player.Currencies = map[State.CurrencyID]float64{}
	}
	state.Player.Level = 1_000
	state.Player.LegendLevel = 1_000
	catalog, _ := gameData.BuildingCatalog()
	for _, definition := range catalog.Definitions() {
		for _, cost := range definition.Costs {
			setAbundantCost(state, castle, cost)
		}
	}
	expansions, _ := gameData.ExpansionCatalog()
	for _, definition := range expansions.Definitions() {
		if definition.SceatSkillLocked > 0 {
			state.Player.LegendSkills.SceatSkillIDs = append(state.Player.LegendSkills.SceatSkillIDs, definition.SceatSkillLocked)
		}
		for _, cost := range definition.Costs {
			setAbundantCost(state, castle, cost)
		}
	}
	for id := State.CurrencyID(1001); id <= 1007; id++ {
		state.Player.Currencies[id] = mockBalance
	}
}

func setAbundantCost(state *State.GameState, castle *State.CastleState, cost GameData.BuildingCost) {
	switch cost.Scope {
	case GameData.BuildingCostCastleResource:
		resourceID := State.ResourceID(cost.DefinitionID)
		capacity := float64(mockBalance)
		castle.Resources[resourceID] = State.ResourceBalance{Amount: mockBalance, Capacity: &capacity}
	case GameData.BuildingCostPlayerResource:
		state.Player.Resources[State.ResourceID(cost.DefinitionID)] = mockBalance
	case GameData.BuildingCostCurrency:
		state.Player.Currencies[State.CurrencyID(cost.DefinitionID)] = mockBalance
	}
}

func resetFixedBuildings(castle State.CastleState, catalog *GameData.BuildingCatalog) []State.Building {
	ids := make([]State.BuildingInstanceID, 0, len(castle.Layout.Fixed))
	for id := range castle.Layout.Fixed {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	result := make([]State.Building, 0, len(ids))
	for _, id := range ids {
		building := castle.Layout.Fixed[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if !found || strings.EqualFold(definition.InternalName, "Harbor") {
			continue
		}
		for definition.DowngradeDefinitionID > 0 {
			previous, previousFound := catalog.Definition(definition.DowngradeDefinitionID)
			if !previousFound {
				break
			}
			definition = previous
		}
		building.DefinitionID = State.BuildingID(definition.ID)
		building.ConstructionState = State.BuildingStateBuildCompleted
		building.ProgressSec = 0
		result = append(result, building)
	}
	return result
}

func initialGround(castle State.CastleState) (State.Building, bool) {
	ids := make([]State.BuildingInstanceID, 0, len(castle.Layout.Ground))
	for id := range castle.Layout.Ground {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	if len(ids) == 0 {
		return State.Building{}, false
	}
	ground := castle.Layout.Ground[ids[0]]
	ground.ConstructionState = State.BuildingStateBuildCompleted
	ground.ProgressSec = 0
	return ground, true
}

func initialKeep(castle State.CastleState, catalog *GameData.BuildingCatalog) (State.Building, bool) {
	ids := make([]State.BuildingInstanceID, 0, len(castle.Layout.Objects))
	for id := range castle.Layout.Objects {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	for _, id := range ids {
		building := castle.Layout.Objects[id]
		definition, found := catalog.Definition(int64(building.DefinitionID))
		if found && strings.EqualFold(definition.InternalName, "Keep") {
			return building, true
		}
	}
	return State.Building{}, false
}

func targetWithHarbor(
	target Buildings.TargetCaptureResult,
	gameData *GameData.Store,
	level int64,
) (Buildings.TargetCaptureResult, error) {
	catalog, err := gameData.BuildingCatalog()
	if err != nil {
		return Buildings.TargetCaptureResult{}, err
	}
	harbor, found := harborDefinition(catalog, level)
	if !found {
		return Buildings.TargetCaptureResult{}, fmt.Errorf("official Harbor level %d is unavailable", level)
	}
	target.Fixed = append(target.Fixed, Buildings.TargetFixedBuilding{
		TargetID: "storm-harbor", DefinitionID: State.BuildingID(harbor.ID),
	})
	target.Summary.FixedCount = len(target.Fixed)
	return target, nil
}

func harborDefinition(catalog *GameData.BuildingCatalog, level int64) (GameData.BuildingDefinition, bool) {
	for _, definition := range catalog.Definitions() {
		if strings.EqualFold(definition.InternalName, "Harbor") && definition.Level == level {
			return definition, true
		}
	}
	return GameData.BuildingDefinition{}, false
}

func isDecoration(definition GameData.BuildingDefinition) bool {
	return strings.EqualFold(definition.GroundType, "DECO") || strings.EqualFold(definition.InternalName, "Deco")
}

func mockBuilding(
	castle State.CastleState,
	id State.BuildingInstanceID,
) (State.Building, string, bool) {
	if building, found := castle.Layout.Objects[id]; found {
		return building, "objects", true
	}
	if building, found := castle.Layout.Fixed[id]; found {
		return building, "fixed", true
	}
	return State.Building{}, "", false
}

func setMockBuilding(castle *State.CastleState, location string, building State.Building) {
	if location == "fixed" {
		castle.Layout.Fixed[building.InstanceID] = building
	} else {
		castle.Layout.Objects[building.InstanceID] = building
	}
	castle.Buildings[building.InstanceID] = building
}

func occupyQueue(castle *State.CastleState, buildingID State.BuildingInstanceID) {
	for index := range castle.BuildingQueue.Slots {
		if castle.BuildingQueue.Slots[index].Status == State.BuildingQueueSlotAvailable {
			castle.BuildingQueue.Slots[index].Status = State.BuildingQueueSlotOccupied
			castle.BuildingQueue.Slots[index].BuildingID = buildingID
			return
		}
	}
	castle.BuildingQueue.Slots = append(castle.BuildingQueue.Slots, State.BuildingConstructionQueueSlot{
		Index: len(castle.BuildingQueue.Slots), Status: State.BuildingQueueSlotOccupied, BuildingID: buildingID,
	})
	castle.BuildingQueue.SlotCount = len(castle.BuildingQueue.Slots)
}

func releaseQueue(castle *State.CastleState, buildingID State.BuildingInstanceID) {
	for index := range castle.BuildingQueue.Slots {
		if castle.BuildingQueue.Slots[index].BuildingID == buildingID {
			castle.BuildingQueue.Slots[index].Status = State.BuildingQueueSlotAvailable
			castle.BuildingQueue.Slots[index].BuildingID = 0
		}
	}
}

func nextBuildingID(state State.GameState) State.BuildingInstanceID {
	maximum := State.BuildingInstanceID(0)
	for _, castle := range state.Castles {
		for id := range castle.Buildings {
			if id > maximum {
				maximum = id
			}
		}
		for id := range castle.Layout.Ground {
			if id > maximum {
				maximum = id
			}
		}
		for id := range castle.Layout.Objects {
			if id > maximum {
				maximum = id
			}
		}
		for id := range castle.Layout.Fixed {
			if id > maximum {
				maximum = id
			}
		}
	}
	return maximum + 1
}

func stormCastleID(state State.GameState, requested State.CastleID) (State.CastleID, error) {
	if requested > 0 {
		castle, found := state.Castles[requested]
		if !found || castle.KingdomID != State.KingdomID(GameData.StormKingdomID) {
			return 0, fmt.Errorf("castle %d is not an owned Storm castle", requested)
		}
		return requested, nil
	}
	ids := make([]State.CastleID, 0)
	for id, castle := range state.Castles {
		if castle.KingdomID == State.KingdomID(GameData.StormKingdomID) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	if len(ids) == 0 {
		return 0, fmt.Errorf("no owned Storm castle was found")
	}
	return ids[0], nil
}

func cloneState(source State.GameState) (State.GameState, error) {
	raw, err := json.Marshal(source)
	if err != nil {
		return State.GameState{}, err
	}
	var clone State.GameState
	if err := json.Unmarshal(raw, &clone); err != nil {
		return State.GameState{}, err
	}
	if clone.Inventory.Items == nil {
		clone.Inventory.Items = map[string]map[int64]int64{}
	}
	return clone, nil
}

func firstExpansionBlocker(preview Buildings.ExpansionPreviewResult) string {
	if len(preview.Blockers) > 0 {
		return preview.Blockers[0].Message
	}
	return "no recommended expansion action"
}

func fatalIf(err error, action string) {
	if err != nil {
		fmt.Fprintf(os.Stderr, "%s: %v\n", action, err)
		os.Exit(1)
	}
}

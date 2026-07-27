package Automation

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"CitadelDesktop/Server/Configuration"
	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/State"
)

func TestFoodBalancePolicyPrioritizesHoneyNeededForMead(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	target := foodBalanceCastle(10, 0, 100, 100, now)
	target.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 201}
	target.BuildingProduction[1] = State.BuildingProduction{PercentByResource: map[string]float64{"MEAD": 100}}
	meadProduction := 100.0
	foodProduction := 1_000.0
	honeyProduction := 0.0
	target.Resources[5] = State.ResourceBalance{Amount: 10_000, ProductionPerHour: &foodProduction}
	target.Resources[11] = State.ResourceBalance{Amount: 10, ProductionPerHour: &honeyProduction}
	target.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: &meadProduction, ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source := foodBalanceCastle(20, 0, 120, 100, now)
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	source.Resources[5] = State.ResourceBalance{Amount: 10_000, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[11] = State.ResourceBalance{Amount: 10_000, ProductionPerHour: float64Pointer(0)}
	source.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	state.Castles[target.ID] = target
	state.Castles[source.ID] = source
	state.Market = State.MarketState{
		CaravanLevelLoaded: true, ObservedAt: now,
		Castles: map[State.CastleID]State.MarketCastleState{
			source.ID: {CastleID: source.ID, AvailableBarrows: 10},
		},
	}
	state.KingdomTransport = State.KingdomTransportState{ObservedAt: now, Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{}}
	state.Player.Resources[1] = 1_000_000

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("expected honey market shipment, got %#v", decision)
	}
	var request struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
		WorkflowOwner  string           `json:"workflowOwner"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != source.ID || request.TargetCastleID != target.ID || request.ResourceID != 11 || request.Amount <= 0 || request.WorkflowOwner != "" {
		t.Fatalf("unexpected honey shipment: %#v", request)
	}
}

func TestFoodBalancePolicyWaitsForMarketBarrowReturnBeforeLogisticsRefresh(t *testing.T) {
	now := time.Date(2026, 7, 22, 23, 30, 0, 0, time.UTC)
	returnsAt := now.Add(10 * time.Minute)
	state := State.NewGameState()
	state.Player.ID = 1
	source := foodBalanceCastle(10, 0, 100, 100, now)
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	state.Castles[source.ID] = source
	state.Castles[20] = foodBalanceCastle(20, 0, 120, 100, now)
	state.Market.ObservedAt = now.Add(-10 * time.Minute)
	state.Market.CaravanLevelLoaded = true
	state.KingdomTransport.ObservedAt = now
	state.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: 1, SourceCastleID: source.ID,
		MarketBarrows: 100, ReturnsAt: &returnsAt,
	}

	decision, err := NewFoodBalancePolicy().Evaluate(
		context.Background(), foodBalanceSnapshot(state, foodBalanceGameData(t), now),
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Request != nil || !decision.NextCheckAt.Equal(returnsAt.Add(time.Second)) {
		t.Fatalf("market lease decision = %+v", decision)
	}
}

func TestFoodBalancePolicyRequiresMarketplaceBeforeUsingBarrows(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	target := foodBalanceCastle(10, 0, 100, 100, now)
	target.Resources[5] = State.ResourceBalance{Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	target.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Units.Stationed[101] = 2
	source := foodBalanceCastle(20, 0, 120, 100, now)
	source.Resources[5] = State.ResourceBalance{Amount: 10_000, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	source.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	state.Castles[target.ID] = target
	state.Castles[source.ID] = source
	state.Market = State.MarketState{
		CaravanLevelLoaded: true, ObservedAt: now,
		Castles: map[State.CastleID]State.MarketCastleState{
			source.ID: {CastleID: source.ID, AvailableBarrows: 10},
		},
	}
	state.KingdomTransport = State.KingdomTransportState{ObservedAt: now, Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{}}
	state.Player.Resources[1] = 1_000_000

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("expected no shipment without a marketplace, got %#v", decision)
	}
}

func TestFoodBalanceMarketShipmentPropagatesMarketplaceCatalogFailure(t *testing.T) {
	gameData, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],"resources":[],"units":[],
		"buildings":[{"wodID":137,"name":"Market","marketCarriages":5}]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	target := foodBalanceCastle(10, 0, 100, 100, time.Now().UTC())
	donor := foodBalanceCastle(20, 0, 120, 100, time.Now().UTC())
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID:  {castle: donor},
	}

	_, ready, err := foodBalanceMarketShipment(
		foodBalanceSettings{}, Snapshot{GameData: gameData}, projections,
		foodBalanceRisk{target: projections[target.ID]}, 1,
	)
	if err == nil || ready || !strings.Contains(err.Error(), "constructionItems") {
		t.Fatalf("marketplace catalog result: ready=%v err=%v", ready, err)
	}
}

func TestFoodBalancePolicyUsesKingdomTransportWhenNoMarketDonorExists(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	target := foodBalanceCastle(10, 1, 100, 100, now)
	target.Resources[5] = State.ResourceBalance{Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	target.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Units.Stationed[101] = 2
	source := foodBalanceCastle(20, 0, 120, 100, now)
	source.Resources[5] = State.ResourceBalance{Amount: 10_000, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	source.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	state.Castles[target.ID] = target
	state.Castles[source.ID] = source
	state.Market = State.MarketState{CaravanLevelLoaded: true, ObservedAt: now, Castles: map[State.CastleID]State.MarketCastleState{}}
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Unlocks:    map[State.KingdomID]State.KingdomTransportUnlock{1: {KingdomID: 1, Unlocked: true}},
	}
	state.Player.Resources[1] = 1_000_000

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("expected kingdom food shipment, got %#v", decision)
	}
	var request struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
		WorkflowOwner  string           `json:"workflowOwner"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != source.ID || request.TargetCastleID != target.ID || request.ResourceID != 5 || request.Amount <= 0 || request.WorkflowOwner != autoFoodBalanceTransportOwner {
		t.Fatalf("unexpected kingdom shipment: %#v", request)
	}
}

func TestFoodBalancePolicyUsesSelectedTimeSkipForPendingFoodTransport(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	state.Castles[10] = foodBalanceCastle(10, 0, 100, 100, now)
	state.Castles[20] = foodBalanceCastle(20, 1, 120, 100, now)
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Pending: []State.KingdomResourceTransport{{
			KingdomID: 1, RemainingSec: 350,
			Goods: []State.KingdomTransportGood{{ResourceID: 5, Amount: 1_000}},
		}},
		ResourceWorkflows: map[State.KingdomID]State.KingdomResourceTransportWorkflow{
			1: {Owner: autoFoodBalanceTransportOwner, KingdomID: 1, SourceCastleID: 10, TargetCastleID: 20, LaunchedAt: now},
		},
	}
	state.Player.Currencies[1003] = 2
	state.Player.Currencies[1005] = 2
	snapshot := foodBalanceSnapshot(state, gameData, now)
	snapshot.Configuration.Sections["automation.autoFoodBalance"] = json.RawMessage(`{
		"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,
		"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1,"minimumSourceReserve":1,
		"minimumCoinReserve":0,"autoKingdomTransport":true,"useKingdomTimeSkips":true,
		"allowedTimeSkips":["MS5","MS3"],"timeSkipReserve":{"MS3":1}
	}`)

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.skip" {
		t.Fatalf("expected food transport time skip, got %#v", decision)
	}
	var request struct {
		TargetKingdomID  State.KingdomID `json:"targetKingdomId"`
		TimeSkipID       string          `json:"timeSkipId"`
		MinimumRemaining int64           `json:"minimumRemaining"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TargetKingdomID != 1 || request.TimeSkipID != "MS3" || request.MinimumRemaining != 1 {
		t.Fatalf("unexpected food transport time skip: %#v", request)
	}
}

func TestFoodBalancePolicyWaitsForSettlingKingdomTransport(t *testing.T) {
	now := time.Date(2026, 7, 21, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	target := foodBalanceCastle(10, 1, 100, 100, now)
	target.Resources[5] = State.ResourceBalance{
		Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20),
		ConsumptionMultiplier: float64Pointer(1),
	}
	target.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	target.Resources[12] = State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	target.Resources[13] = State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	target.Units.Stationed[101] = 2
	source := foodBalanceCastle(20, 0, 120, 100, now)
	source.Resources[5] = State.ResourceBalance{
		Amount: 10_000, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	source.Resources[11] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0)}
	source.Resources[12] = State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	source.Resources[13] = State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	state.Castles[target.ID] = target
	state.Castles[source.ID] = source
	state.Market = State.MarketState{CaravanLevelLoaded: true, ObservedAt: now, Castles: map[State.CastleID]State.MarketCastleState{}}
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Unlocks:    map[State.KingdomID]State.KingdomTransportUnlock{1: {KingdomID: 1, Unlocked: true}},
		Pending:    []State.KingdomResourceTransport{{KingdomID: 1, RemainingSec: -1}},
	}
	state.Player.Resources[1] = 1_000_000

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Status != "waiting" || !strings.Contains(decision.Detail, "settle") {
		t.Fatalf("settling kingdom transport decision = %#v", decision)
	}
}

func TestFoodBalancePolicyDoesNotRequireMarketForSingleCastleKingdoms(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	mainCastle := foodBalanceCastle(10, 0, 100, 100, now)
	mainCastle.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	dungeonCastle := foodBalanceCastle(20, 3, 120, 100, now)
	state.Castles[mainCastle.ID] = mainCastle
	state.Castles[dungeonCastle.ID] = dungeonCastle
	state.KingdomTransport.ObservedAt = now

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil {
		t.Fatalf("single-castle kingdoms unexpectedly requested logistics refresh: %#v", decision)
	}
}

func TestFoodBalancePolicyRefreshesUnknownCastleStateBeforeShipping(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	castle := foodBalanceCastle(10, 0, 100, 100, time.Time{})
	state.Castles[castle.ID] = castle
	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("expected state refresh, got %#v", decision)
	}
}

func TestFoodBalanceMinimumOnlyAppliesToKingdomTransport(t *testing.T) {
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	settings := foodBalanceSettings{MinimumShipmentSize: 10_000, SourceSafetyHours: 1}
	rate := GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"}
	target := foodBalanceCastle(10, 0, 100, 100, now)
	target.Resources[5] = State.ResourceBalance{}
	donor := foodBalanceCastle(20, 0, 120, 100, now)
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	donor.Resources[5] = State.ResourceBalance{Amount: 5_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID: {
			castle: donor,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: rate,
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 5, rate: rate}
	state := State.NewGameState()
	state.Market = State.MarketState{
		CaravanLevelLoaded: true,
		Castles: map[State.CastleID]State.MarketCastleState{
			donor.ID: {CastleID: donor.ID, AvailableBarrows: 100},
		},
	}
	state.Player.Resources[1] = 1_000_000

	t.Run("market", func(t *testing.T) {
		decision, ready, err := foodBalanceMarketShipment(settings, Snapshot{
			State: state, GameData: gameData, Now: now,
		}, projections, risk, 100)
		if err != nil {
			t.Fatal(err)
		}
		if !ready || decision.Request == nil {
			t.Fatalf("small market shipment = %#v ready=%t", decision, ready)
		}
		var request struct {
			Amount int64 `json:"amount"`
		}
		if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
			t.Fatal(err)
		}
		if request.Amount != 100 {
			t.Fatalf("market shipment amount = %d, want the actual shortfall", request.Amount)
		}
	})

	t.Run("kingdom", func(t *testing.T) {
		target.KingdomID = 1
		donor.KingdomID = 0
		projections[target.ID] = foodBalanceProjection{castle: target}
		donorProjection := projections[donor.ID]
		donorProjection.castle = donor
		projections[donor.ID] = donorProjection
		risk.target = projections[target.ID]
		state.KingdomTransport.Unlocks[1] = State.KingdomTransportUnlock{KingdomID: 1, Unlocked: true}
		decision, ready := foodBalanceKingdomShipment(settings, Snapshot{
			State: state, GameData: gameData, Now: now,
		}, projections, risk, 100, time.Minute)
		if ready || decision.Request != nil {
			t.Fatalf("undersized kingdom shipment = %#v ready=%t", decision, ready)
		}
	})
}

func foodBalanceSnapshot(state State.GameState, gameData *GameData.Store, now time.Time) Snapshot {
	return Snapshot{
		State: state,
		Configuration: Configuration.Snapshot{Sections: map[string]json.RawMessage{
			"automation.autoFoodBalance": json.RawMessage(`{"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1,"minimumSourceReserve":1,"minimumCoinReserve":0,"autoKingdomTransport":true}`),
		}},
		GameData: gameData, Now: now,
	}
}

func foodBalanceCastle(id State.CastleID, kingdom State.KingdomID, x int, y int, observedAt time.Time) State.CastleState {
	return State.CastleState{
		ID: id, KingdomID: kingdom, X: x, Y: y, FoodStateObservedAt: observedAt,
		Resources:          map[State.ResourceID]State.ResourceBalance{},
		Units:              State.CastleUnits{Stationed: map[State.UnitID]int64{}},
		Buildings:          map[State.BuildingInstanceID]State.Building{},
		BuildingProduction: map[State.BuildingInstanceID]State.BuildingProduction{},
		ConstructionSlots:  map[State.BuildingInstanceID][]State.ConstructionSlot{},
	}
}

func foodBalanceGameData(t *testing.T) *GameData.Store {
	t.Helper()
	store, err := GameData.DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":1,"JSONKey":"C1"},
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"currencies":[
			{"currencyID":1003,"JSONKey":"MS3","Name":"tenMinuteSkip"},
			{"currencyID":1005,"JSONKey":"MS5","Name":"hourSkip"}
		],
		"units":[{"wodID":101,"foodSupply":10}],
		"buildings":[
			{"wodID":137,"name":"Market","marketCarriages":5},
			{"wodID":201,"meadProduction":100000,"honeyRatio":1,"foodRatio":3}
		],
		"constructionItems":[],"levelBoosters":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func float64Pointer(value float64) *float64 { return &value }

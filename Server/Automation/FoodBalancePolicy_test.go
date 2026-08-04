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
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{Amount: 10_000, ProductionPerHour: &foodProduction}, 100_000)
	target.Resources[11] = foodBalanceStorage(State.ResourceBalance{Amount: 10, ProductionPerHour: &honeyProduction}, 10_000)
	target.Resources[12] = State.ResourceBalance{Amount: 0, ProductionPerHour: &meadProduction, ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	target.Resources[13] = State.ResourceBalance{Amount: 0, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1)}
	source := foodBalanceCastle(20, 0, 120, 100, now)
	source.Layout.ObservedAt = now
	source.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137, Placed: true}
	source.Buildings[2] = State.Building{InstanceID: 2, DefinitionID: 226, Placed: true}
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

	snapshot := foodBalanceSnapshot(state, gameData, now)
	var settings map[string]any
	if err := json.Unmarshal(snapshot.Configuration.Sections["automation.autoFoodBalance"], &settings); err != nil {
		t.Fatal(err)
	}
	settings["horseTravelBoostId"] = 1009
	snapshot.Configuration.Sections["automation.autoFoodBalance"], _ = json.Marshal(settings)
	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("expected honey market shipment, got %#v", decision)
	}
	var request struct {
		SourceCastleID   State.CastleID   `json:"sourceCastleId"`
		TargetCastleID   State.CastleID   `json:"targetCastleId"`
		ResourceID       State.ResourceID `json:"resourceId"`
		Amount           int64            `json:"amount"`
		WorkflowOwner    string           `json:"workflowOwner"`
		EnforceTargetCap bool             `json:"enforceTargetCapacity"`
		HorseTravelBoost int              `json:"horseTravelBoostId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != source.ID || request.TargetCastleID != target.ID || request.ResourceID != 11 ||
		request.Amount != 1_010 || request.WorkflowOwner != autoFoodBalanceTransportOwner || !request.EnforceTargetCap ||
		request.HorseTravelBoost != 1009 {
		t.Fatalf("unexpected honey shipment: %#v", request)
	}
}

func TestFoodBalancePolicyRejectsUnsupportedMarketHorseTravelBoost(t *testing.T) {
	now := time.Date(2026, 7, 29, 16, 0, 0, 0, time.UTC)
	snapshot := foodBalanceSnapshot(State.NewGameState(), foodBalanceGameData(t), now)
	snapshot.Configuration.Sections["automation.autoFoodBalance"] = json.RawMessage(`{"horseTravelBoostId":42}`)

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Status != "waiting" || decision.Request != nil ||
		!strings.Contains(decision.Detail, "supported market-barrow horse travel boost") {
		t.Fatalf("unsupported barrow horse decision = %#v", decision)
	}
}

func TestFoodBalancePolicyIgnoresBerimondCastles(t *testing.T) {
	now := time.Date(2026, 7, 30, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	berimondKingdomID := State.KingdomID(GameData.BerimondKingdomID)

	t.Run("target", func(t *testing.T) {
		state := State.NewGameState()
		donor := foodBalanceCompleteCastle(10, 0, 100, 200, now)
		donor.Resources[5] = foodBalanceStorage(State.ResourceBalance{
			Amount: 500_000, ProductionPerHour: float64Pointer(1_000),
			ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
		}, 1_000_000)
		target := foodBalanceCompleteCastle(20, berimondKingdomID, 110, 215, now)
		target.Units.Stationed[101] = 10
		state.Castles[donor.ID] = donor
		state.Castles[target.ID] = target
		state.KingdomTransport.ObservedAt = now
		state.KingdomTransport.Unlocks[berimondKingdomID] = State.KingdomTransportUnlock{
			KingdomID: berimondKingdomID, Unlocked: true,
		}

		decision, err := NewFoodBalancePolicy().Evaluate(
			context.Background(), foodBalanceSnapshot(state, gameData, now),
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Request != nil || decision.Status != "idle" || decision.Metrics["castles"] != 1 {
			t.Fatalf("Berimond target affected Auto Food decision = %#v", decision)
		}
	})

	t.Run("donor", func(t *testing.T) {
		state := State.NewGameState()
		target := foodBalanceCompleteCastle(10, 0, 100, 200, now)
		target.Resources[5] = foodBalanceStorage(State.ResourceBalance{
			Amount: 0, ProductionPerHour: float64Pointer(0),
			ConsumptionPerHour: float64Pointer(100), ConsumptionMultiplier: float64Pointer(1),
		}, 1_000_000)
		donor := foodBalanceCompleteCastle(20, berimondKingdomID, 110, 215, now)
		donor.Resources[5] = foodBalanceStorage(State.ResourceBalance{
			Amount: 500_000, ProductionPerHour: float64Pointer(1_000),
			ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
		}, 1_000_000)
		state.Castles[target.ID] = target
		state.Castles[donor.ID] = donor
		state.KingdomTransport.ObservedAt = now
		state.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
			KingdomID: target.KingdomID, Unlocked: true,
		}

		decision, err := NewFoodBalancePolicy().Evaluate(
			context.Background(), foodBalanceSnapshot(state, gameData, now),
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Request != nil || !strings.Contains(decision.Detail, "No safe") {
			t.Fatalf("Berimond donor affected Auto Food decision = %#v", decision)
		}
	})

	t.Run("refresh and workflow", func(t *testing.T) {
		state := State.NewGameState()
		eligible := foodBalanceCompleteCastle(10, 0, 100, 200, now)
		berimond := foodBalanceCompleteCastle(20, berimondKingdomID, 110, 215, time.Time{})
		state.Castles[eligible.ID] = eligible
		state.Castles[berimond.ID] = berimond
		state.KingdomTransport.ResourceWorkflows[berimondKingdomID] = State.KingdomResourceTransportWorkflow{
			Owner: autoFoodBalanceTransportOwner, KingdomID: berimondKingdomID,
			TargetCastleID: berimond.ID, LaunchedAt: now.Add(-time.Hour),
		}

		decision, err := NewFoodBalancePolicy().Evaluate(
			context.Background(), foodBalanceSnapshot(state, gameData, now),
		)
		if err != nil {
			t.Fatal(err)
		}
		if decision.Request != nil || decision.Status != "idle" || decision.Metrics["castles"] != 1 {
			t.Fatalf("Berimond refresh or workflow affected Auto Food decision = %#v", decision)
		}
	})
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

func TestFoodBalancePolicyContinuesWhileMarketBarrowsAreLeased(t *testing.T) {
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	state.Player.ID = 1
	donor := foodBalanceCompleteCastle(10, 0, 100, 200, now)
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	donor.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		Amount: 300_000, ProductionPerHour: float64Pointer(1_000),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	target := foodBalanceCompleteCastle(20, 0, 110, 215, now)
	target.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(10), ConsumptionMultiplier: float64Pointer(1),
	}, 10_000)
	state.Castles[donor.ID] = donor
	state.Castles[target.ID] = target
	state.Market = State.MarketState{
		CaravanLevelLoaded: true, ObservedAt: now.Add(-10 * time.Minute),
		Castles: map[State.CastleID]State.MarketCastleState{
			donor.ID: {CastleID: donor.ID, TotalBarrows: 100, AvailableBarrows: 100},
		},
	}
	returnsAt := now.Add(10 * time.Minute)
	state.Movements[50] = State.MovementState{
		ID: 50, Direction: 1, OwnerPlayerID: state.Player.ID, SourceCastleID: donor.ID,
		MarketBarrows: 75, ReturnsAt: &returnsAt,
	}
	state.Player.Resources[1] = 1_000_000

	decision, err := NewFoodBalancePolicy().Evaluate(
		context.Background(), foodBalanceSnapshot(state, gameData, now),
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("shipment while market barrows are leased = %#v", decision)
	}
	var request struct {
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TargetCastleID != target.ID || request.ResourceID != 12 {
		t.Fatalf("shipment while market barrows are leased = %#v", request)
	}
}

func TestFoodBalancePolicyRequiresMarketplaceBeforeUsingBarrows(t *testing.T) {
	now := time.Date(2026, 7, 12, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	target := foodBalanceCastle(10, 0, 100, 100, now)
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1)}, 1_000)
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
	donor.Resources[5] = State.ResourceBalance{Amount: 100}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID: {
			castle: donor,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(0)},
			}},
		},
	}

	_, ready, err := foodBalanceMarketShipment(
		foodBalanceSettings{}, Snapshot{GameData: gameData}, projections,
		foodBalanceRisk{
			target: projections[target.ID], resourceID: 5,
			rate: GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"},
		}, 1,
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
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1)}, 1_000)
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

func TestFoodBalancePolicyContinuesPastIncomingFoodToShipMead(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	foodTarget := foodBalanceCastle(10, 2, 100, 100, now)
	foodTarget.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000)
	foodTarget.Resources[11] = State.ResourceBalance{ProductionPerHour: float64Pointer(0)}
	foodTarget.Resources[12] = State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	foodTarget.Resources[13] = State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	storm := foodBalanceCastle(20, 4, 576, 669, now)
	storm.Name = "Storm"
	storm.Resources[5] = State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	storm.Resources[11] = State.ResourceBalance{ProductionPerHour: float64Pointer(0)}
	storm.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(10), ConsumptionMultiplier: float64Pointer(1),
	}, 186_500)
	storm.Resources[13] = State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	donor := foodBalanceCastle(30, 0, 212, 941, now)
	donor.Resources[5] = State.ResourceBalance{
		Amount: 300_000, ProductionPerHour: float64Pointer(1_000),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}
	donor.Resources[11] = State.ResourceBalance{ProductionPerHour: float64Pointer(0)}
	donor.Resources[12] = State.ResourceBalance{
		Amount: 300_000, ProductionPerHour: float64Pointer(1_000),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}
	donor.Resources[13] = State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}
	state.Castles[foodTarget.ID] = foodTarget
	state.Castles[storm.ID] = storm
	state.Castles[donor.ID] = donor
	arrivesAt := now.Add(10 * time.Minute)
	state.Movements[1] = State.MovementState{
		ID: 1, Direction: 0, KingdomID: foodTarget.KingdomID,
		TargetX: foodTarget.X, TargetY: foodTarget.Y, ArrivesAt: &arrivesAt,
		MarketGoods: []State.KingdomTransportGood{{ResourceID: 5, Amount: 1_000}},
	}
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{
			storm.KingdomID: {KingdomID: storm.KingdomID, Unlocked: true},
		},
	}

	decision, err := NewFoodBalancePolicy().Evaluate(
		context.Background(), foodBalanceSnapshot(state, gameData, now),
	)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("Mead shipment after protected Food risk = %#v", decision)
	}
	var request struct {
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TargetCastleID != storm.ID || request.ResourceID != 12 ||
		float64(request.Amount)*foodBalanceKingdomDeliveryRatio > 186_500 {
		t.Fatalf("Storm Mead request = %#v", request)
	}
}

func TestFoodBalancePolicyDoesNotShipFoodToZeroConsumptionCastle(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	storm := foodBalanceCastle(10, 4, 576, 669, now)
	storm.Name = "Storm"
	storm.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}, 80_800)
	storm.Resources[11] = foodBalanceStorage(State.ResourceBalance{ProductionPerHour: float64Pointer(0)}, 0)
	storm.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}, 186_500)
	storm.Resources[13] = foodBalanceStorage(State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}, 0)
	donor := foodBalanceCastle(20, 0, 212, 941, now)
	donor.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 1_000_000, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	donor.Resources[11] = foodBalanceStorage(State.ResourceBalance{ProductionPerHour: float64Pointer(0)}, 1_000_000)
	donor.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	donor.Resources[13] = foodBalanceStorage(State.ResourceBalance{
		ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
		ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	state.Castles[storm.ID] = storm
	state.Castles[donor.ID] = donor
	state.Market = State.MarketState{CaravanLevelLoaded: true, ObservedAt: now, Castles: map[State.CastleID]State.MarketCastleState{}}
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{
			4: {KingdomID: 4, Unlocked: true},
		},
	}

	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), foodBalanceSnapshot(state, gameData, now))
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request != nil || decision.Status != "idle" {
		t.Fatalf("zero-consumption Storm decision = %#v", decision)
	}
}

func TestFoodBalanceKingdomShipmentUsesHighestNetSurplusAndFillsStorage(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	settings := foodBalanceSettings{MinimumShipmentSize: 1, MinimumSourceReserve: 10_000, SourceSafetyHours: 1}
	rate := GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"}
	target := foodBalanceCastle(10, 4, 576, 669, now)
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{Amount: 10_000}, 80_800)
	largeBalance := foodBalanceCastle(20, 0, 212, 941, now)
	largeBalance.Resources[5] = State.ResourceBalance{Amount: 500_000}
	highestNetSurplus := foodBalanceCastle(30, 1, 447, 673, now)
	highestNetSurplus.Resources[5] = State.ResourceBalance{Amount: 300_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		largeBalance.ID: {
			castle: largeBalance,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {
					ResourceID: 5, ResourceJSONKey: "F", TotalConsumptionPerHour: 400_000,
					NetPerHour: float64Pointer(-100_000),
				},
			}},
		},
		highestNetSurplus.ID: {
			castle: highestNetSurplus,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 5, rate: rate}
	state := State.NewGameState()
	state.KingdomTransport = State.KingdomTransportState{
		Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{
			4: {KingdomID: 4, Unlocked: true},
		},
	}

	decision, ready := foodBalanceKingdomShipment(
		settings, Snapshot{State: state, Now: now}, projections, risk, 70_800, time.Minute,
	)
	if !ready || decision.Request == nil {
		t.Fatalf("storage fill decision = %#v ready=%t", decision, ready)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
		Amount         int64          `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != highestNetSurplus.ID {
		t.Fatalf("selected donor = %d, want highest net surplus %d", request.SourceCastleID, highestNetSurplus.ID)
	}
	if request.Amount != 78_666 || float64(request.Amount)*foodBalanceKingdomDeliveryRatio > 70_800 {
		t.Fatalf("kingdom dispatch = %d, want storage-bounded dispatch", request.Amount)
	}
}

func TestFoodBalanceKingdomShipmentSettlesCoveringTimeSkipImmediately(t *testing.T) {
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	settings := foodBalanceSettings{
		MinimumShipmentSize: 1, MinimumSourceReserve: 10_000, SourceSafetyHours: 1,
		AutoKingdomTransport: true, UseKingdomTimeSkips: true,
		AllowedTimeSkips: []string{"MS3", "MS5"}, TimeSkipReserve: map[string]int64{"MS5": 1},
	}
	rate := GameData.FoodConsumptionRate{ResourceID: 12, ResourceJSONKey: "MEAD"}
	target := foodBalanceCastle(10, 4, 576, 669, now)
	donor := foodBalanceCastle(20, 0, 212, 941, now)
	donor.Resources[12] = State.ResourceBalance{Amount: 300_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID: {
			castle: donor,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				12: {ResourceID: 12, ResourceJSONKey: "MEAD", NetPerHour: float64Pointer(1_000)},
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 12, rate: rate}
	state := State.NewGameState()
	state.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	state.Player.Currencies[1003] = 10
	state.Player.Currencies[1005] = 2

	decision, ready := foodBalanceKingdomShipment(
		settings, Snapshot{State: state, GameData: gameData, Now: now},
		projections, risk, 70_800, time.Minute,
	)
	if !ready || decision.Request == nil || !decision.ReevaluateOnSuccess {
		t.Fatalf("immediately skipped shipment = %#v ready=%t", decision, ready)
	}
	var request struct {
		TimeSkipID       string `json:"timeSkipId"`
		MinimumRemaining int64  `json:"minimumRemaining"`
		SettleAfterSkip  bool   `json:"settleAfterTimeSkip"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TimeSkipID != "MS5" || request.MinimumRemaining != 1 || !request.SettleAfterSkip {
		t.Fatalf("immediate time-skip request = %#v", request)
	}

	settings.AllowedTimeSkips = []string{"MS3"}
	decision, ready = foodBalanceKingdomShipment(
		settings, Snapshot{State: state, GameData: gameData, Now: now},
		projections, risk, 70_800, time.Minute,
	)
	if !ready || decision.Request == nil {
		t.Fatalf("travelling shipment = %#v ready=%t", decision, ready)
	}
	var travellingRequest struct {
		TimeSkipID      string `json:"timeSkipId"`
		SettleAfterSkip bool   `json:"settleAfterTimeSkip"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &travellingRequest); err != nil {
		t.Fatal(err)
	}
	if travellingRequest.TimeSkipID != "" || travellingRequest.SettleAfterSkip {
		t.Fatalf("partial skip incorrectly marked instant = %#v", travellingRequest)
	}
}

func TestFoodBalanceShipmentRanksDonorsBeforeChoosingTransport(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	settings := foodBalanceSettings{
		MinimumShipmentSize: 1, MinimumSourceReserve: 10_000, SourceSafetyHours: 1,
		AutoKingdomTransport: true,
	}
	rate := GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"}
	target := foodBalanceCastle(10, 4, 576, 669, now)
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{Amount: 10_000}, 80_800)
	sameKingdom := foodBalanceCastle(20, 4, 580, 670, now)
	sameKingdom.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137}
	sameKingdom.Resources[5] = State.ResourceBalance{Amount: 500_000}
	crossKingdom := foodBalanceCastle(30, 0, 212, 941, now)
	crossKingdom.Resources[5] = State.ResourceBalance{Amount: 300_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		sameKingdom.ID: {
			castle: sameKingdom,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(50_000)},
			}},
		},
		crossKingdom.ID: {
			castle: crossKingdom,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 5, rate: rate}
	state := State.NewGameState()
	state.Market = State.MarketState{
		ObservedAt: now, CaravanLevelLoaded: true,
		Castles: map[State.CastleID]State.MarketCastleState{
			sameKingdom.ID: {CastleID: sameKingdom.ID, AvailableBarrows: 100},
		},
	}
	state.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}
	state.Player.Resources[1] = 1_000_000

	decision, ready, err := foodBalanceShipment(
		settings, Snapshot{State: state, GameData: gameData, Now: now},
		projections, risk, 70_800, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || decision.Request == nil {
		t.Fatalf("globally ranked shipment = %#v ready=%t", decision, ready)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
		Amount         int64          `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != crossKingdom.ID || request.Amount != 78_666 {
		t.Fatalf("globally ranked donor request = %#v", request)
	}
}

func TestFoodBalanceMarketShipmentSkipsHorseIncompatibleDonor(t *testing.T) {
	now := time.Date(2026, 8, 3, 16, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	settings := foodBalanceSettings{
		CheckIntervalSec: 60, MinimumShipmentSize: 1, MinimumSourceReserve: 1, SourceSafetyHours: 1,
		HorseTravelBoostID: 1009,
	}
	target := foodBalanceCastle(10, 0, 100, 100, now)
	unsupported := foodBalanceCastle(20, 0, 110, 100, now)
	unsupported.Layout.ObservedAt = now
	unsupported.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137, Placed: true}
	unsupported.Resources[5] = State.ResourceBalance{Amount: 100_000}
	supported := foodBalanceCastle(30, 0, 120, 100, now)
	supported.Layout.ObservedAt = now
	supported.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137, Placed: true}
	supported.Buildings[2] = State.Building{InstanceID: 2, DefinitionID: 226, Placed: true}
	supported.Resources[5] = State.ResourceBalance{Amount: 100_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		unsupported.ID: {
			castle: unsupported,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
		supported.ID: {
			castle: supported,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(50_000)},
			}},
		},
	}
	risk := foodBalanceRisk{
		target: projections[target.ID], resourceID: 5,
		rate: GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"},
	}
	state := State.NewGameState()
	state.Market = State.MarketState{
		ObservedAt: now, CaravanLevelLoaded: true,
		Castles: map[State.CastleID]State.MarketCastleState{
			unsupported.ID: {CastleID: unsupported.ID, AvailableBarrows: 100},
			supported.ID:   {CastleID: supported.ID, AvailableBarrows: 100},
		},
	}
	state.Player.Resources[1] = 1_000_000

	decision, ready, err := foodBalanceShipment(
		settings, Snapshot{State: state, GameData: gameData, Now: now}, projections, risk, 1_000, time.Minute,
	)
	if err != nil || !ready || decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("compatible donor fallback = %#v ready=%t err=%v", decision, ready, err)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != supported.ID {
		t.Fatalf("market donor = %d, want compatible donor %d", request.SourceCastleID, supported.ID)
	}
}

func TestFoodBalanceMarketShipmentWaitsWithoutHorseCompatibleDonor(t *testing.T) {
	now := time.Date(2026, 8, 3, 16, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	settings := foodBalanceSettings{
		CheckIntervalSec: 60, MinimumShipmentSize: 1, MinimumSourceReserve: 1, SourceSafetyHours: 1,
		HorseTravelBoostID: 1009,
	}
	target := foodBalanceCastle(10, 0, 100, 100, now)
	donor := foodBalanceCastle(20, 0, 110, 100, now)
	donor.Layout.ObservedAt = now
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137, Placed: true}
	donor.Resources[5] = State.ResourceBalance{Amount: 100_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID: {
			castle: donor,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
	}
	risk := foodBalanceRisk{
		target: projections[target.ID], resourceID: 5,
		rate: GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"},
	}
	state := State.NewGameState()
	state.Market = State.MarketState{
		ObservedAt: now, CaravanLevelLoaded: true,
		Castles: map[State.CastleID]State.MarketCastleState{
			donor.ID: {CastleID: donor.ID, AvailableBarrows: 100},
		},
	}
	state.Player.Resources[1] = 1_000_000

	decision, ready, err := foodBalanceShipment(
		settings, Snapshot{State: state, GameData: gameData, Now: now}, projections, risk, 1_000, time.Minute,
	)
	if err != nil || ready || decision.Request != nil || decision.Status != "waiting" ||
		!strings.Contains(decision.Detail, "cannot use the selected horse travel boost") {
		t.Fatalf("incompatible donor wait = %#v ready=%t err=%v", decision, ready, err)
	}
}

func TestFoodBalanceMarketShipmentRefreshesUnobservedHorseLayout(t *testing.T) {
	now := time.Date(2026, 8, 3, 16, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	donor := foodBalanceCastle(20, 0, 110, 100, now)
	donor.Buildings[1] = State.Building{InstanceID: 1, DefinitionID: 137, Placed: true}
	decision, ready, err := foodBalanceMarketShipmentFromDonor(
		foodBalanceSettings{HorseTravelBoostID: 1009},
		Snapshot{GameData: gameData, Now: now},
		foodBalanceRisk{target: foodBalanceProjection{castle: foodBalanceCastle(10, 0, 100, 100, now)}},
		1_000,
		foodBalanceDonor{projection: foodBalanceProjection{castle: donor}},
	)
	if err != nil || !ready || decision.Request == nil || decision.Request.Name != "game.focus_castle" {
		t.Fatalf("unobserved donor refresh = %#v ready=%t err=%v", decision, ready, err)
	}
	var request struct {
		Refresh bool `json:"refresh"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil || !request.Refresh {
		t.Fatalf("donor layout refresh request = %#v err=%v", request, err)
	}
}

func TestFoodBalanceShipmentSkipsHigherNetDonorWithoutEnoughStoredSurplus(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	settings := foodBalanceSettings{
		MinimumShipmentSize: 1, MinimumSourceReserve: 10_000, SourceSafetyHours: 1,
		AutoKingdomTransport: true,
	}
	rate := GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"}
	target := foodBalanceCastle(10, 4, 576, 669, now)
	highestNet := foodBalanceCastle(20, 0, 212, 941, now)
	highestNet.Resources[5] = State.ResourceBalance{Amount: 70_000}
	nextHighest := foodBalanceCastle(30, 1, 447, 673, now)
	nextHighest.Resources[5] = State.ResourceBalance{Amount: 300_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		highestNet.ID: {
			castle: highestNet,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
		nextHighest.ID: {
			castle: nextHighest,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(50_000)},
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 5, rate: rate}
	state := State.NewGameState()
	state.KingdomTransport.Unlocks[target.KingdomID] = State.KingdomTransportUnlock{
		KingdomID: target.KingdomID, Unlocked: true,
	}

	decision, ready, err := foodBalanceShipment(
		settings, Snapshot{State: state, Now: now}, projections, risk, 70_800, time.Minute,
	)
	if err != nil {
		t.Fatal(err)
	}
	if !ready || decision.Request == nil {
		t.Fatalf("fallback donor shipment = %#v ready=%t", decision, ready)
	}
	var request struct {
		SourceCastleID State.CastleID `json:"sourceCastleId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != nextHighest.ID {
		t.Fatalf("fallback donor = %d, want %d", request.SourceCastleID, nextHighest.ID)
	}
}

func TestFoodBalanceKingdomShipmentWaitsWithoutDonorForFullStorageNeed(t *testing.T) {
	now := time.Date(2026, 7, 29, 12, 0, 0, 0, time.UTC)
	settings := foodBalanceSettings{MinimumShipmentSize: 1, MinimumSourceReserve: 10_000, SourceSafetyHours: 1}
	rate := GameData.FoodConsumptionRate{ResourceID: 5, ResourceJSONKey: "F"}
	target := foodBalanceCastle(10, 4, 576, 669, now)
	donor := foodBalanceCastle(20, 0, 212, 941, now)
	donor.Resources[5] = State.ResourceBalance{Amount: 70_000}
	projections := map[State.CastleID]foodBalanceProjection{
		target.ID: {castle: target},
		donor.ID: {
			castle: donor,
			consumed: GameData.CastleFoodConsumption{ByResource: map[State.ResourceID]GameData.FoodConsumptionRate{
				5: {ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100_000)},
			}},
		},
	}
	risk := foodBalanceRisk{target: projections[target.ID], resourceID: 5, rate: rate}
	state := State.NewGameState()
	state.KingdomTransport.Unlocks[4] = State.KingdomTransportUnlock{KingdomID: 4, Unlocked: true}

	decision, ready := foodBalanceKingdomShipment(
		settings, Snapshot{State: state, Now: now}, projections, risk, 70_800, time.Minute,
	)
	if ready || decision.Request != nil {
		t.Fatalf("partial donor decision = %#v ready=%t", decision, ready)
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
	target.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 10, ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(20),
		ConsumptionMultiplier: float64Pointer(1),
	}, 1_000)
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

func TestFoodBalancePolicyContinuesPastPendingKingdomTransferWithoutTimeSkips(t *testing.T) {
	now := time.Date(2026, 7, 29, 13, 0, 0, 0, time.UTC)
	gameData := foodBalanceGameData(t)
	state := State.NewGameState()
	blockedTarget := foodBalanceCompleteCastle(10, 1, 100, 100, now)
	blockedTarget.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(20), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000)
	storm := foodBalanceCompleteCastle(20, 4, 576, 669, now)
	storm.Name = "Storm"
	storm.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		Amount: 0, ProductionPerHour: float64Pointer(0),
		ConsumptionPerHour: float64Pointer(10), ConsumptionMultiplier: float64Pointer(1),
	}, 186_500)
	donor := foodBalanceCompleteCastle(30, 0, 212, 941, now)
	donor.Resources[5] = foodBalanceStorage(State.ResourceBalance{
		Amount: 300_000, ProductionPerHour: float64Pointer(1_000),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	donor.Resources[12] = foodBalanceStorage(State.ResourceBalance{
		Amount: 300_000, ProductionPerHour: float64Pointer(1_000),
		ConsumptionPerHour: float64Pointer(0), ConsumptionMultiplier: float64Pointer(1),
	}, 1_000_000)
	state.Castles[blockedTarget.ID] = blockedTarget
	state.Castles[storm.ID] = storm
	state.Castles[donor.ID] = donor
	state.KingdomTransport = State.KingdomTransportState{
		ObservedAt: now,
		Unlocks: map[State.KingdomID]State.KingdomTransportUnlock{
			blockedTarget.KingdomID: {KingdomID: blockedTarget.KingdomID, Unlocked: true},
			storm.KingdomID:         {KingdomID: storm.KingdomID, Unlocked: true},
		},
		Pending: []State.KingdomResourceTransport{{
			KingdomID: blockedTarget.KingdomID, RemainingSec: 3_600,
			Goods: []State.KingdomTransportGood{{ResourceID: 5, Amount: 1_000}},
		}},
		ResourceWorkflows: map[State.KingdomID]State.KingdomResourceTransportWorkflow{
			blockedTarget.KingdomID: {
				Owner: autoFoodBalanceTransportOwner, KingdomID: blockedTarget.KingdomID,
				SourceCastleID: donor.ID, TargetCastleID: blockedTarget.ID, LaunchedAt: now,
			},
		},
	}

	snapshot := foodBalanceSnapshot(state, gameData, now)
	snapshot.Configuration.Sections["automation.autoFoodBalance"] = json.RawMessage(`{
		"checkIntervalSec":60,"stateRefreshIntervalSec":900,"logisticsRefreshIntervalSec":300,
		"safetyHours":8,"sourceSafetyHours":24,"minimumShipmentSize":1,
		"minimumSourceReserve":1,"minimumCoinReserve":0,"autoKingdomTransport":true,
		"useKingdomTimeSkips":false
	}`)
	decision, err := NewFoodBalancePolicy().Evaluate(context.Background(), snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if decision.Request == nil || decision.Request.Name != "resource.ship" {
		t.Fatalf("shipment past busy kingdom lane = %#v", decision)
	}
	var request struct {
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.TargetCastleID != storm.ID || request.ResourceID != 12 {
		t.Fatalf("shipment past busy kingdom lane = %#v", request)
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
	rate := GameData.FoodConsumptionRate{
		ResourceID: 5, ResourceJSONKey: "F", NetPerHour: float64Pointer(100),
	}
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
			t.Fatalf("market shipment amount = %d, want the remaining storage need", request.Amount)
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

func foodBalanceCompleteCastle(
	id State.CastleID,
	kingdom State.KingdomID,
	x int,
	y int,
	observedAt time.Time,
) State.CastleState {
	castle := foodBalanceCastle(id, kingdom, x, y, observedAt)
	for _, resourceID := range []State.ResourceID{5, 11, 12, 13} {
		castle.Resources[resourceID] = foodBalanceStorage(State.ResourceBalance{
			ProductionPerHour: float64Pointer(0), ConsumptionPerHour: float64Pointer(0),
			ConsumptionMultiplier: float64Pointer(1),
		}, 1_000_000)
	}
	return castle
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
			{"wodID":201,"meadProduction":100000,"honeyRatio":1,"foodRatio":3},
			{"wodID":226,"name":"Stable","level":"3","unlockHorses":"1007,1008,1009"}
		],
		"horses":[
			{"wodID":1007,"group":"Travelbooster"},
			{"wodID":1008,"group":"Travelbooster"},
			{"wodID":1009,"group":"Travelbooster"}
		],
		"constructionItems":[],"levelBoosters":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func float64Pointer(value float64) *float64 { return &value }

func foodBalanceStorage(balance State.ResourceBalance, capacity float64) State.ResourceBalance {
	balance.Capacity = float64Pointer(capacity)
	return balance
}

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
	if decision.Request == nil || decision.Request.Name != "resource.market.ship" {
		t.Fatalf("expected honey market shipment, got %#v", decision)
	}
	var request struct {
		SourceCastleID State.CastleID   `json:"sourceCastleId"`
		TargetCastleID State.CastleID   `json:"targetCastleId"`
		ResourceID     State.ResourceID `json:"resourceId"`
		Amount         int64            `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != source.ID || request.TargetCastleID != target.ID || request.ResourceID != 11 || request.Amount <= 0 {
		t.Fatalf("unexpected honey shipment: %#v", request)
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
	if decision.Request == nil || decision.Request.Name != "resource.kingdom.ship" {
		t.Fatalf("expected kingdom food shipment, got %#v", decision)
	}
	var request struct {
		SourceCastleID  State.CastleID   `json:"sourceCastleId"`
		TargetCastleID  State.CastleID   `json:"targetCastleId"`
		TargetKingdomID State.KingdomID  `json:"targetKingdomId"`
		ResourceID      State.ResourceID `json:"resourceId"`
		Amount          int64            `json:"amount"`
	}
	if err := json.Unmarshal(decision.Request.Arguments, &request); err != nil {
		t.Fatal(err)
	}
	if request.SourceCastleID != source.ID || request.TargetCastleID != target.ID || request.TargetKingdomID != target.KingdomID || request.ResourceID != 5 || request.Amount <= 0 {
		t.Fatalf("unexpected kingdom shipment: %#v", request)
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
		Resources:         map[State.ResourceID]State.ResourceBalance{},
		Units:             State.CastleUnits{Stationed: map[State.UnitID]int64{}},
		Buildings:         map[State.BuildingInstanceID]State.Building{},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{},
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
		"units":[{"wodID":101,"foodSupply":10}],
		"buildings":[
			{"wodID":137,"name":"Market","marketCarriages":5},
			{"wodID":201,"meadProduction":100,"honeyRatio":1,"foodRatio":3}
		],
		"constructionItems":[],"levelBoosters":[],"effects":[]
	}`), GameData.SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func float64Pointer(value float64) *float64 { return &value }

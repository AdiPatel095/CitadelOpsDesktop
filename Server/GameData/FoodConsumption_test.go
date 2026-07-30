package GameData

import (
	"math"
	"strings"
	"testing"

	"CitadelDesktop/Server/State"
)

func TestEstimateFoodConsumptionUsesStationedTroopsAndObservedRates(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"units":[
			{"wodID":101,"foodSupply":3},
			{"wodID":102,"meadSupply":2},
			{"wodID":103,"beefSupply":4}
		],
		"buildings":[{"wodID":201,"Foodreduction":25}],
		"constructionItems":[{"constructionItemID":301,"Meadreduction":10}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	observedMultiplier := 0.5
	observedConsumption := 8.0
	foodProduction := 100.0
	meadProduction := 6.0
	castle := State.CastleState{
		ID: 99,
		Resources: map[State.ResourceID]State.ResourceBalance{
			5:  {ProductionPerHour: &foodProduction, ConsumptionMultiplier: &observedMultiplier, ConsumptionPerHour: &observedConsumption},
			11: {},
			12: {ProductionPerHour: &meadProduction},
			13: {},
		},
		Units: State.CastleUnits{
			Stationed: map[State.UnitID]int64{101: 4, 102: 5, 103: 6},
			Hospital:  map[State.UnitID]int64{101: 100},
		},
		Buildings: map[State.BuildingInstanceID]State.Building{1: {DefinitionID: 201}},
		ConstructionSlots: map[State.BuildingInstanceID][]State.ConstructionSlot{
			1: {{DefinitionID: 301}},
		},
	}
	estimate, err := store.EstimateFoodConsumption(castle)
	if err != nil {
		t.Fatal(err)
	}
	food := estimate.ByResource[5]
	if food.BasePerHour != 12 || food.ConsumptionMultiplier != 0.5 || food.CalculatedPerHour != 6 || food.EffectivePerHour != 8 || food.MultiplierSource != "observed" {
		t.Fatalf("unexpected food consumption: %#v", food)
	}
	if food.NetPerHour == nil || *food.NetPerHour != 92 {
		t.Fatalf("unexpected food net rate: %#v", food.NetPerHour)
	}
	mead := estimate.ByResource[12]
	if mead.BasePerHour != 10 || mead.ConsumptionMultiplier != 0.9 || mead.CalculatedPerHour != 9 || mead.EffectivePerHour != 9 || mead.MultiplierSource != "catalog" {
		t.Fatalf("unexpected mead consumption: %#v", mead)
	}
	if mead.NetPerHour == nil || *mead.NetPerHour != -3 {
		t.Fatalf("unexpected mead net rate: %#v", mead.NetPerHour)
	}
	beef := estimate.ByResource[13]
	if beef.BasePerHour != 24 || beef.ConsumptionMultiplier != 1 || beef.EffectivePerHour != 24 || beef.ObservedPerHour != nil {
		t.Fatalf("unexpected beef consumption: %#v", beef)
	}
}

func TestEstimateFoodConsumptionRejectsUnknownStationedUnits(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"units":[],"buildings":[]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.EstimateFoodConsumption(State.CastleState{
		Units: State.CastleUnits{Stationed: map[State.UnitID]int64{999: 1}},
	})
	if err == nil || !strings.Contains(err.Error(), "999") {
		t.Fatalf("expected missing unit error, got %v", err)
	}
}

func TestEstimateFoodConsumptionClampsCatalogReduction(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"units":[{"wodID":101,"foodSupply":3}],
		"buildings":[{"wodID":201,"Foodreduction":125}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	estimate, err := store.EstimateFoodConsumption(State.CastleState{
		Resources: map[State.ResourceID]State.ResourceBalance{5: {}, 11: {}, 12: {}, 13: {}},
		Units:     State.CastleUnits{Stationed: map[State.UnitID]int64{101: 2}},
		Buildings: map[State.BuildingInstanceID]State.Building{1: {DefinitionID: 201}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if value := estimate.ByResource[5].CalculatedPerHour; math.Abs(value) > 0.000001 {
		t.Fatalf("food consumption = %v, want 0", value)
	}
}

func TestEstimateFoodConsumptionUsesConfiguredBreweryRateForInputs(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"units":[],
		"buildings":[{"wodID":201,"meadProduction":10000,"honeyRatio":1,"foodRatio":3}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	foodProduction := 120.0
	honeyProduction := 50.0
	meadProduction := 200.0
	estimate, err := store.EstimateFoodConsumption(State.CastleState{
		Resources: map[State.ResourceID]State.ResourceBalance{
			5:  {Amount: 150, ProductionPerHour: &foodProduction},
			11: {Amount: 100, ProductionPerHour: &honeyProduction},
			12: {ProductionPerHour: &meadProduction},
			13: {},
		},
		Units:     State.CastleUnits{Stationed: map[State.UnitID]int64{}},
		Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 201}},
		BuildingProduction: map[State.BuildingInstanceID]State.BuildingProduction{
			1: {PercentByResource: map[string]float64{"MEAD": 40}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	food := estimate.ByResource[5]
	if food.ProductionInputPerHour != 120 || food.TotalConsumptionPerHour != 120 || food.NetPerHour == nil || *food.NetPerHour != 0 {
		t.Fatalf("unexpected food production input: %#v", food)
	}
	honey := estimate.ByResource[11]
	if honey.ProductionInputPerHour != 40 || honey.TotalConsumptionPerHour != 40 || honey.NetPerHour == nil || *honey.NetPerHour != 10 {
		t.Fatalf("unexpected honey production input: %#v", honey)
	}
	dependency := estimate.MeadProduction
	if dependency.ProductionPerHour != 200 || dependency.HoneyInputPerHour != 40 || dependency.FoodInputPerHour != 120 {
		t.Fatalf("unexpected mead dependency: %#v", dependency)
	}
	if dependency.SustainableProductionPerHour == nil || *dependency.SustainableProductionPerHour != 200 {
		t.Fatalf("sustainable mead production = %#v, want 200", dependency.SustainableProductionPerHour)
	}
	if dependency.HoneyHoursUntilDepleted != nil {
		t.Fatalf("honey depletion = %#v, want nil", dependency.HoneyHoursUntilDepleted)
	}
	if dependency.FoodHoursUntilDepleted != nil {
		t.Fatalf("food depletion = %#v, want nil", dependency.FoodHoursUntilDepleted)
	}

	meadProduction = 20_000
	estimate, err = store.EstimateFoodConsumption(State.CastleState{
		Resources: map[State.ResourceID]State.ResourceBalance{
			5:  {ProductionPerHour: &foodProduction},
			11: {ProductionPerHour: &honeyProduction},
			12: {ProductionPerHour: &meadProduction},
			13: {},
		},
		Units:     State.CastleUnits{Stationed: map[State.UnitID]int64{}},
		Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 201}},
		BuildingProduction: map[State.BuildingInstanceID]State.BuildingProduction{
			1: {PercentByResource: map[string]float64{"MEAD": 40}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if estimate.MeadProduction.ProductionPerHour != 20_000 || estimate.MeadProduction.HoneyInputPerHour != 40 {
		t.Fatalf("live mead output changed brewery honey use: %#v", estimate.MeadProduction)
	}
}

func TestEstimateFoodConsumptionRequiresObservedBreweryPercentage(t *testing.T) {
	store, err := DecodeStore([]byte(`{
		"versionInfo":[],
		"resources":[
			{"resourceID":5,"JSONKey":"F"},
			{"resourceID":11,"JSONKey":"HONEY"},
			{"resourceID":12,"JSONKey":"MEAD"},
			{"resourceID":13,"JSONKey":"BEEF"}
		],
		"units":[],
		"buildings":[{"wodID":201,"meadProduction":10000,"honeyRatio":1,"foodRatio":3}]
	}`), SourceMetadata{ItemVersion: "test"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.EstimateFoodConsumption(State.CastleState{
		Resources: map[State.ResourceID]State.ResourceBalance{5: {}, 11: {}, 12: {}, 13: {}},
		Units:     State.CastleUnits{Stationed: map[State.UnitID]int64{}},
		Buildings: map[State.BuildingInstanceID]State.Building{1: {InstanceID: 1, DefinitionID: 201}},
	})
	if err == nil || !strings.Contains(err.Error(), "brewery production percentage is unavailable") {
		t.Fatalf("missing brewery percentage error = %v", err)
	}
}

package settingsview

import (
	"testing"

	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
	gamestate "CitadelDesktop/Server/Models/GameState"
)

func TestAutoSceatMarketCapacityUsesOfficialData(t *testing.T) {
	static, err := loadAutoSceatStaticCatalog()
	if err != nil {
		t.Fatal(err)
	}
	info := &Models.PlayerCastleInfo{
		Aid:    1,
		BDRows: []castle.BuildingData{{BuildingID: 253, OID: 99}},
		ConstructionByBuilding: []castle.GCAConstructionBuilding{{
			OID:   99,
			Slots: []castle.GCAConstructionSlot{{CID: 214}},
		}},
	}
	snapshot := gamestate.MarketTransportState{
		CaravanLevel:       21,
		CaravanLevelLoaded: true,
		Castles: []gamestate.MarketCastleState{{
			CastleID:         1,
			TotalBarrows:     125,
			AvailableBarrows: 125,
			AreaEffects: []gamestate.MarketAreaEffect{
				{EffectID: 90, Values: []float64{15}},
				{EffectID: 90, Values: []float64{10}},
			},
		}},
	}

	market := autoSceatMarketState(info, static, snapshot)
	if market == nil {
		t.Fatal("market state is nil")
	}
	if market.BaseBarrows != 85 || market.BuildItemBarrows != 40 || market.CaravanBoostPercent != 147 {
		t.Fatalf("market breakdown = %#v", market)
	}
	if market.CapacityPerBarrow != 310 || market.AvailableShipmentCapacity != 38750 {
		t.Fatalf("capacity = %d/%d, want 310/38750", market.CapacityPerBarrow, market.AvailableShipmentCapacity)
	}
}

func TestAutoSceatAdjustedSkipCostUsesRemainingFraction(t *testing.T) {
	recipe := AutoSceatRecipeCatalogEntry{DurationSec: 3600, SkipCostRubies: 120}
	if got := autoSceatResAdjustedSkipCost(recipe, 2242); got != 75 {
		t.Fatalf("adjusted skip cost = %d, want 75", got)
	}
	if got := autoSceatResAdjustedSkipCost(recipe, 7200); got != 120 {
		t.Fatalf("clamped adjusted skip cost = %d, want 120", got)
	}
	if got := autoSceatResAdjustedSkipCost(recipe, 0); got != 0 {
		t.Fatalf("completed adjusted skip cost = %d, want 0", got)
	}
}

func TestAutoSceatDragonCharmCostsUseCurrencyMetadata(t *testing.T) {
	static, err := loadAutoSceatStaticCatalog()
	if err != nil {
		t.Fatal(err)
	}

	coins := static.Resources["coins"]
	if coins.Name != "Coins" || coins.IconURL != "/game-data/resources/images/Coins.webp" {
		t.Fatalf("coins metadata = %#v", coins)
	}
	sceat := static.Resources["sceatToken"]
	if sceat.Name != "Sceat" || sceat.IconURL != "/game-data/resources/images/Sceat.webp" {
		t.Fatalf("sceat metadata = %#v", sceat)
	}

	recipe, found := AutoSceatRecipeByID(367)
	if !found {
		t.Fatal("dragon charm recipe 367 is missing")
	}
	if recipe.Costs["coins"] != 892500 || recipe.Costs["sceatToken"] != 105 {
		t.Fatalf("dragon charm costs = %#v", recipe.Costs)
	}
	if _, found := recipe.Costs["C1"]; found {
		t.Fatalf("raw C1 leaked into dragon charm costs: %#v", recipe.Costs)
	}
}

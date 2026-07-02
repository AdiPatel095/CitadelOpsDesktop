package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models"
	castle "CitadelDesktop/Server/Models/Castle"
)

func TestToolProductionResourceCostCheckUsesCastleResourcesAndReservations(t *testing.T) {
	gs := &Models.GameState{}
	c := &castle.PlayerCastleInfo{Aid: 1234}
	c.Amount.WoodAmount = 2240
	c.Amount.StoneAmount = 960
	reservations := map[string]float64{}

	check := ToolProductionResourceCostCheck(gs, c, 614, 80, reservations)
	if !check.CanAfford() {
		t.Fatalf("first check cannot afford: %s", check.MissingSummary())
	}
	if len(check.Costs) != 2 {
		t.Fatalf("len(Costs) = %d, want 2", len(check.Costs))
	}

	ReserveToolResourceCosts(reservations, check)
	check = ToolProductionResourceCostCheck(gs, c, 614, 80, reservations)
	if check.CanAfford() {
		t.Fatalf("second check can afford after reservation, want missing resources")
	}
	if len(check.Missing) != 2 {
		t.Fatalf("missing = %+v, want wood and stone", check.Missing)
	}
}

func TestToolProductionResourceCostCheckUsesComponents(t *testing.T) {
	gs := &Models.GameState{}
	gs.GlobalResources.Component1 = 11
	gs.GlobalResources.Component6 = 8
	c := &castle.PlayerCastleInfo{Aid: 1234}

	check := ToolProductionResourceCostCheck(gs, c, 245, 1, nil)
	if check.CanAfford() {
		t.Fatalf("check can afford with component1 shortage, want missing")
	}
	if len(check.Missing) != 1 || check.Missing[0].Key != "component1" {
		t.Fatalf("missing = %+v, want component1", check.Missing)
	}

	gs.GlobalResources.Component1 = 12
	check = ToolProductionResourceCostCheck(gs, c, 245, 1, nil)
	if !check.CanAfford() {
		t.Fatalf("check cannot afford with exact components: %s", check.MissingSummary())
	}
}

func TestToolProductionResourceCostCheckBlocksUnknownToolCost(t *testing.T) {
	check := ToolProductionResourceCostCheck(&Models.GameState{}, &castle.PlayerCastleInfo{Aid: 1234}, 99999999, 1, nil)
	if !check.UnknownToolCost {
		t.Fatalf("UnknownToolCost = false, want true")
	}
	if check.CanAfford() {
		t.Fatalf("unknown tool cost should not be affordable")
	}
}

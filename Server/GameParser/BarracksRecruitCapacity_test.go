package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models"
	"CitadelDesktop/Server/Models/Castle"
)

func TestBarracksStackSizeCatalogIncludesVariableBarracksRows(t *testing.T) {
	tests := []struct {
		wid  int
		want int
	}{
		{wid: 475, want: 5},
		{wid: 3116, want: 250},
		{wid: 631, want: 80},
	}

	for _, tt := range tests {
		got, ok := barracksStackSizeForWID(tt.wid)
		if !ok {
			t.Fatalf("barracksStackSizeForWID(%d) returned ok=false", tt.wid)
		}
		if got != tt.want {
			t.Fatalf("barracksStackSizeForWID(%d) = %d, want %d", tt.wid, got, tt.want)
		}
	}
}

func TestBarracksRecruitStackCapacityDetailsAddsConstructionBoost(t *testing.T) {
	c := &castle.PlayerCastleInfo{
		BDRows: []castle.BuildingData{
			{BuildingID: 3116, OID: 1001, Level: 20},
		},
		ConstructionByBuilding: []castle.GCAConstructionBuilding{
			{
				OID: 1001,
				Slots: []castle.GCAConstructionSlot{
					{CID: 14, Level: 4},
					{CID: 30160, Level: 1},
				},
			},
			{
				OID: 9999,
				Slots: []castle.GCAConstructionSlot{
					{CID: 14, Level: 4},
				},
			},
		},
	}

	got := BarracksRecruitStackCapacityDetails(c)
	if got.BuildingOID != 1001 {
		t.Fatalf("BuildingOID = %d, want 1001", got.BuildingOID)
	}
	if got.BaseStackSize != 250 {
		t.Fatalf("BaseStackSize = %d, want 250", got.BaseStackSize)
	}
	if got.ConstructionBonus != 80 {
		t.Fatalf("ConstructionBonus = %d, want 80", got.ConstructionBonus)
	}
	if got.TotalStackSize != 330 {
		t.Fatalf("TotalStackSize = %d, want 330", got.TotalStackSize)
	}
	if len(got.Boosts) != 1 {
		t.Fatalf("len(Boosts) = %d, want 1", len(got.Boosts))
	}
	if got.Boosts[0].CID != 14 || got.Boosts[0].StackSize != 80 {
		t.Fatalf("Boosts[0] = %+v, want CID 14 stack 80", got.Boosts[0])
	}
}

func TestBarracksConstructionStackBoostCandidatesAreDataDriven(t *testing.T) {
	metaByCID, err := ExportConstructionItemMetaMap()
	if err != nil {
		t.Fatalf("ExportConstructionItemMetaMap() error = %v", err)
	}

	got := map[int]int{}
	for cid, meta := range metaByCID {
		if constructionStackBoostAppliesToBuilding(meta, 3116) {
			got[cid] = meta.StackSize
		}
	}

	want := map[int]int{
		11: 16,
		12: 32,
		13: 48,
		14: 80,
	}
	if len(got) != len(want) {
		t.Fatalf("barracks stack CID count = %d (%v), want %d (%v)", len(got), got, len(want), want)
	}
	for cid, wantStack := range want {
		if got[cid] != wantStack {
			t.Fatalf("CID %d stack = %d, want %d; got all %v", cid, got[cid], wantStack, got)
		}
	}
}

func TestSubscriptionRecruitmentSlotCapacityBonusIsDataDriven(t *testing.T) {
	got, ok := SubscriptionRecruitmentSlotCapacityBonus(1)
	if !ok {
		t.Fatalf("SubscriptionRecruitmentSlotCapacityBonus(1) returned ok=false")
	}
	if got != 40 {
		t.Fatalf("SubscriptionRecruitmentSlotCapacityBonus(1) = %d, want 40", got)
	}

	if got := ActiveSubscriptionRecruitmentSlotCapacityBonus([]int{1, 1, 2}); got != 40 {
		t.Fatalf("ActiveSubscriptionRecruitmentSlotCapacityBonus([1,1,2]) = %d, want 40", got)
	}
}

func TestBarracksRecruitCostReductionPercentUsesEquippedCI(t *testing.T) {
	c := &castle.PlayerCastleInfo{
		ConstructionByBuilding: []castle.GCAConstructionBuilding{
			{
				OID: 1001,
				Slots: []castle.GCAConstructionSlot{
					{CID: 10, Level: 10},
				},
			},
			{
				OID: 9999,
				Slots: []castle.GCAConstructionSlot{
					{CID: 10, Level: 10},
				},
			},
		},
	}

	if got := BarracksRecruitCostReductionPercent(c, 1001, 3116); got != 40 {
		t.Fatalf("BarracksRecruitCostReductionPercent = %d, want 40", got)
	}
}

func TestRecruitUnitResourceCostCheckUsesCoinsReductionAndReservations(t *testing.T) {
	gs := &Models.GameState{}
	gs.GlobalResources.Coins = 6000
	c := &castle.PlayerCastleInfo{Aid: 1234}
	reservations := map[string]float64{}

	check := RecruitUnitResourceCostCheck(gs, c, 216, 10, 40, reservations)
	if !check.CanAfford() {
		t.Fatalf("first check cannot afford: %s", check.MissingSummary())
	}
	if len(check.Costs) != 1 {
		t.Fatalf("len(Costs) = %d, want 1", len(check.Costs))
	}
	if check.Costs[0].Key != "coins" || check.Costs[0].Required != 3072 {
		t.Fatalf("cost = %+v, want coins required 3072", check.Costs[0])
	}

	ReserveRecruitResourceCosts(reservations, check)
	check = RecruitUnitResourceCostCheck(gs, c, 216, 10, 40, reservations)
	if check.CanAfford() {
		t.Fatalf("second check can afford after reservation, want missing coins")
	}
	if len(check.Missing) != 1 || check.Missing[0].Key != "coins" {
		t.Fatalf("missing = %+v, want coins", check.Missing)
	}
}

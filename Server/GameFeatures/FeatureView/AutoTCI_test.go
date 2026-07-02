package featureview

import "testing"

func TestAutoTCIUBCSUCForTargetTier(t *testing.T) {
	tests := []struct {
		targetTier int
		wantSUC    int
		wantOK     bool
	}{
		{targetTier: 1, wantOK: false},
		{targetTier: 2, wantSUC: autoTCIUBCRareBooster, wantOK: true},
		{targetTier: 3, wantSUC: autoTCIUBCEpicBooster, wantOK: true},
		{targetTier: 4, wantSUC: autoTCIUBCLegendaryBooster, wantOK: true},
		{targetTier: 5, wantOK: false},
	}

	for _, tt := range tests {
		gotSUC, gotOK := autoTCIUBCSUCForTargetTier(tt.targetTier)
		if gotOK != tt.wantOK || gotSUC != tt.wantSUC {
			t.Fatalf("autoTCIUBCSUCForTargetTier(%d) = (%d, %v), want (%d, %v)",
				tt.targetTier, gotSUC, gotOK, tt.wantSUC, tt.wantOK)
		}
	}
}

func TestNextTierAfterUpgradeReturnsNextLevelAndCID(t *testing.T) {
	entry := &ConstructionItemCatalogEntry{
		GroupIDs: []int{30173, 30170, 30172, 30171},
	}

	nextTier, nextCID, ok := nextTierAfterUpgrade(entry, 30170)
	if !ok {
		t.Fatal("nextTierAfterUpgrade returned ok=false")
	}
	if nextTier != 2 || nextCID != 30171 {
		t.Fatalf("nextTierAfterUpgrade() = tier %d CID %d, want tier 2 CID 30171", nextTier, nextCID)
	}
}

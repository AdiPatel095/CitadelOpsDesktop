package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models/Castle"
)

func TestToolProductionStackCapacityDetailsForToolUsesSelectedWorkshopKind(t *testing.T) {
	c := &castle.PlayerCastleInfo{
		BDRows: []castle.BuildingData{
			{BuildingID: 256, OID: 1001},
			{BuildingID: 176, OID: 1002},
		},
	}

	got := ToolProductionStackCapacityDetailsForTool(c, 626)
	if got.BuildingWID != 176 {
		t.Fatalf("BuildingWID = %d, want 176", got.BuildingWID)
	}
	if got.TotalStackSize != 20 {
		t.Fatalf("TotalStackSize = %d, want 20", got.TotalStackSize)
	}
}

func TestToolProductionStackCapacityDetailsForToolRejectsUnknownToolKind(t *testing.T) {
	c := &castle.PlayerCastleInfo{
		BDRows: []castle.BuildingData{
			{BuildingID: 256, OID: 1001},
		},
	}

	got := ToolProductionStackCapacityDetailsForTool(c, 999999)
	if got.TotalStackSize != 0 {
		t.Fatalf("TotalStackSize = %d, want 0", got.TotalStackSize)
	}
	if got.Source != "unknown-tool" {
		t.Fatalf("Source = %q, want unknown-tool", got.Source)
	}
}

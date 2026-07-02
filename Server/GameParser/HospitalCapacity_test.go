package GameParser

import (
	"testing"

	"CitadelDesktop/Server/Models/Castle"
)

func TestHospitalQueueCapacityDetailsUsesHospitalSlots(t *testing.T) {
	c := &castle.PlayerCastleInfo{
		BDRows: []castle.BuildingData{
			{BuildingID: 3, OID: 1001, Level: 3},
			{BuildingID: 467, OID: 1002, Level: 9},
		},
	}

	got := HospitalQueueCapacityDetails(c)
	if got.HospitalSlots != 5 {
		t.Fatalf("HospitalSlots = %d, want 5", got.HospitalSlots)
	}
	if got.BuildingOID != 1002 {
		t.Fatalf("BuildingOID = %d, want 1002", got.BuildingOID)
	}
}

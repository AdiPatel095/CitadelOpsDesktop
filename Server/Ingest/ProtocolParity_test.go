package Ingest

import "testing"

func TestLegacy138ProtocolReducerParity(t *testing.T) {
	registry := NewRegistry()
	if err := RegisterCoreReducers(registry); err != nil {
		t.Fatal(err)
	}
	legacyOpcodes := []string{
		"gbd", "gpc", "gei", "gcu", "boi", "cmi", "gmu", "ufa", "ufp", "gdi", "dcl", "sce",
		"kpi", "kgt", "msk", "crm", "sie", "upc", "eqe", "ggm", "gli", "ain", "sne", "cra",
		"gam", "cat", "csm", "mcm", "mrm", "gaa", "ssi", "gbc", "sin", "gii", "jaa", "jca",
		"rpc", "ubc", "spl", "bup", "hru", "hdu", "crin", "crst", "crun", "crsk", "crca", "fuc",
	}
	for _, opcode := range legacyOpcodes {
		if !registry.HasInbound(opcode) {
			t.Errorf("1.3.8 inbound opcode %q has no 2.0 reducer", opcode)
		}
	}
}

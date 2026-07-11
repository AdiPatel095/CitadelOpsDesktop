package Ingest

import "fmt"

func RegisterCoreReducers(registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("ingest registry is required")
	}
	reducers := []struct {
		opcode  string
		reducer Reducer
	}{
		{"gbd", reduceInitialState},
		{"gpi", reducePlayerInfo},
		{"gcu", reduceGlobalResources},
		{"ain", reduceAllianceInfo},
		{"jaa", reduceCastleSnapshot},
		{"jca", reduceCastleSnapshot},
		{"gui", reduceFocusedUnits},
		{"hru", reduceFocusedUnits},
		{"rpc", reduceFocusedConstructionItems},
		{"ubc", reduceFocusedConstructionItems},
		{"gam", newMovementReducer(true)},
		{"cat", newMovementReducer(false)},
		{"cra", newMovementReducer(false)},
		{"cds", newMovementReducer(false)},
		{"mcm", newMovementReducer(false)},
		{"crm", newMovementReducer(false)},
		{"mrm", reduceMovementRemoval},
		{"gaa", reduceMapSnapshot},
	}
	for _, entry := range reducers {
		if err := registry.Register(entry.opcode, entry.reducer); err != nil {
			return err
		}
	}
	return nil
}

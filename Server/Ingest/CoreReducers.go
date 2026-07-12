package Ingest

import "fmt"

func RegisterCoreReducers(registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("ingest registry is required")
	}
	craMovements := newMovementReducer(false)
	reducers := []struct {
		opcode  string
		reducer Reducer
	}{
		{"gbd", reduceInitialState},
		{"gpi", reducePlayerInfo},
		{"gcu", reduceGlobalResources},
		{"ain", reduceAllianceInfo},
		{"gli", reduceLeaders},
		{"gei", reduceEquipmentStorage},
		{"ggm", reduceGemStorage},
		{"gii", reduceConstructionInventory},
		{"jaa", reduceCastleSnapshot},
		{"jca", reduceCastleSnapshot},
		{"gui", reduceFocusedUnits},
		{"hru", reduceFocusedUnits},
		{"hdu", reduceFocusedUnits},
		{"rpc", reduceFocusedConstructionItems},
		{"ubc", reduceFocusedConstructionItems},
		{"spl", reduceProductionSnapshot},
		{"crin", reduceCraftingSnapshot},
		{"crst", reduceCraftingBuilding},
		{"crun", reduceCraftingBuilding},
		{"crsk", reduceCraftingBuilding},
		{"crca", reduceCraftingBuilding},
		{"gam", newMovementReducer(true)},
		{"cat", newMovementReducer(false)},
		{"cra", combineReducers(craMovements, reduceRiftLaunchAck)},
		{"cds", newMovementReducer(false)},
		{"mcm", newMovementReducer(false)},
		{"crm", newMovementReducer(false)},
		{"mrm", reduceMovementRemoval},
		{"gaa", reduceMapSnapshot},
		{"sne", reduceReportNotices},
		{"bsd", reduceSpyReportCapture},
		{"bls", reduceBattleSummaryCapture},
		{"blm", reduceBattleWaveCapture},
		{"bld", reduceBattleDetailCapture},
		{"eeq", reduceEquipmentMutation},
		{"bge", reduceEquipmentMutation},
		{"ege", reduceEquipmentMutation},
		{"ere", reduceEquipmentMutation},
		{"eqe", reduceEquipmentMutation},
		{"gsue", reduceEquipmentMutation},
		{"guse", reduceEquipmentMutation},
		{"seq", reduceEquipmentMutation},
		{"sge", reduceEquipmentMutation},
		{"gnr", reduceEquipmentMutation},
	}
	for _, entry := range reducers {
		if err := registry.Register(entry.opcode, entry.reducer); err != nil {
			return err
		}
	}
	if err := registry.RegisterOutbound("bup", reduceProductionCommandContext); err != nil {
		return err
	}
	if err := registry.RegisterOutbound("cra", reduceRiftLaunchCapture); err != nil {
		return err
	}
	for _, opcode := range []string{"blm", "bld"} {
		if err := registry.RegisterOutbound(opcode, reduceBattleCommandContext); err != nil {
			return err
		}
	}
	return nil
}

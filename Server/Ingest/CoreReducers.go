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
		{"gmu", newPlayerMetricReducer("MP", "gmu")},
		{"ufa", newPlayerMetricReducer("CF", "ufa")},
		{"ufp", newPlayerMetricReducer("CFP", "ufp")},
		{"gdi", reducePlayerSummary},
		{"vip", reduceVIPInfo},
		{"sce", reducePlayerCurrencies},
		{"dcl", reduceCastleDetails},
		{"gpc", reduceQueueableProduction},
		{"boi", reduceMarketBooster},
		{"cmi", reduceMarketInfo},
		{"kpi", reduceKingdomTransport},
		{"kgt", reduceKingdomTransport},
		{"msk", reduceKingdomTransport},
		{"sie", reduceSubscriptions},
		{"upc", reduceSubscriptions},
		{"fuc", reduceBeriCapacity},
		{"ain", reduceAllianceInfo},
		{"gli", reduceLeaders},
		{"gei", reduceEquipmentStorage},
		{"ggm", reduceGemStorage},
		{"gii", reduceConstructionInventory},
		{"jaa", combineReducers(reduceCastleSnapshot, reduceEmbeddedProductionSnapshots, reduceEmbeddedStorageInventory, reduceResponseResources)},
		{"jca", reduceCastleSnapshot},
		{"gui", reduceFocusedUnits},
		{"hru", combineReducers(reduceFocusedUnits, reduceProductionSnapshot, reduceResponseResources)},
		{"hdu", combineReducers(reduceFocusedUnits, reduceProductionSnapshot, reduceResponseResources)},
		{"rpc", combineReducers(reduceFocusedConstructionItems, reduceResponseResources)},
		{"ubc", combineReducers(reduceFocusedConstructionItems, reduceResponseResources)},
		{"spl", combineReducers(reduceProductionSnapshot, reduceResponseResources)},
		{"bup", combineReducers(reduceProductionSnapshot, reduceResponseResources)},
		{"crin", reduceCraftingSnapshot},
		{"crst", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crun", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crsk", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crca", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"gam", newMovementReducer(true)},
		{"cat", newMovementReducer(false)},
		{"csm", newMovementReducer(false)},
		{"cra", combineReducers(craMovements, reduceRiftLaunchAck)},
		{"cds", newMovementReducer(false)},
		{"mcm", newMovementReducer(false)},
		{"crm", combineReducers(newMovementReducer(false), reduceResponseResources)},
		{"mrm", reduceMovementRemoval},
		{"gaa", reduceMapSnapshot},
		{"ssi", reduceNestedMapSnapshot},
		{"sin", reduceStorageInventory},
		{"gbc", reduceConstructionOffers},
		{"sbp", reduceResponseResources},
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

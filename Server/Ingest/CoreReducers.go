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
		{"gai", reduceDailyAttackCount},
		{"aa", reduceAdvisorActivation},
		{"aao", reduceAdvisorOverview},
		{"vli", reduceAchievementSnapshot},
		{"skl", reduceLegendSkills},
		{"skp", reduceLegendSkillPurchase},
		{"skr", reduceLegendSkillReset},
		{"grc", reduceResponseResources},
		{"gpa", reduceResponseResources},
		{"sei", reduceScalableEventSnapshot},
		{"rpr", reduceKhanRagePoints},
		{"pep", reduceEventPoints},
		{"hgh", reduceEventRanking},
		{"dcl", reduceCastleDetails},
		{"dfc", reduceDefenseContext},
		{"dfw", reduceDefenseWall},
		{"dfk", reduceDefenseKeep},
		{"dfm", reduceDefenseMoat},
		{"mos", combineReducers(reduceOpenGate, reduceResponseResources)},
		{"gpc", reduceQueueableProduction},
		{"boi", reduceMarketBooster},
		{"cmi", reduceMarketInfo},
		{"kpi", reduceKingdomTransport},
		{"kgt", reduceKingdomTransport},
		{"kut", reduceKingdomTransport},
		{"msk", reduceKingdomTransport},
		{"sie", reduceSubscriptions},
		{"upc", reduceSubscriptions},
		{"fuc", reduceBeriCapacity},
		{"ain", combineReducers(reduceAllianceInfo, reducePlayerProtectionMode)},
		{"gli", reduceLeaders},
		{"gie", reduceGenerals},
		{"gei", reduceEquipmentStorage},
		{"ggm", reduceGemStorage},
		{"gii", reduceConstructionInventory},
		{"jaa", combineReducers(reduceCastleSnapshot, reduceEmbeddedProductionSnapshots, reduceEmbeddedStorageInventory, reduceResponseResources, reducePlayerProtectionMode)},
		{"jca", combineReducers(reduceCastleSnapshot, reducePlayerProtectionMode)},
		{"gui", reduceFocusedUnits},
		{"hru", combineReducers(reduceFocusedUnits, reduceProductionSnapshot, reduceResponseResources)},
		{"hdu", combineReducers(reduceFocusedUnits, reduceProductionSnapshot, reduceResponseResources)},
		{"rpc", combineReducers(reduceFocusedConstructionItems, reduceResponseResources)},
		{"ubc", combineReducers(reduceFocusedConstructionItems, reduceResponseResources)},
		{"ebe", combineReducers(reduceExpansionMutation, reduceResponseResources)},
		{"etc", reduceBuildingMutation},
		{"ebu", combineReducers(reduceBuildingMutation, reduceEmbeddedStorageInventory, reduceResponseResources)},
		{"emo", reduceBuildingMutation},
		{"eup", combineReducers(reduceBuildingMutation, reduceResponseResources)},
		{"edo", combineReducers(reduceBuildingMutation, reduceResponseResources)},
		{"sob", reduceBuildingMutation},
		{"fco", combineReducers(reduceBuildingMutation, reduceResponseResources)},
		{"msb", combineReducers(reduceBuildingMutation, reduceResponseResources)},
		{"ego", reduceBuildingMutation},
		{"scl", reduceBuildingMutation},
		{"spl", combineReducers(reduceProductionSnapshot, reduceResponseResources)},
		{"bup", combineReducers(reduceProductionSnapshot, reduceResponseResources)},
		{"ahh", reduceAllianceHelpRequest},
		{"ahl", reduceAllianceHelpRequest},
		{"crin", reduceCraftingSnapshot},
		{"crst", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crun", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crsk", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crca", combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"gam", combineReducers(newMovementReducer(true), reduceAdvisorMovement)},
		{"cat", combineReducers(newMovementReducer(false), reduceAdvisorMovement)},
		{"csm", newMovementReducer(false)},
		{"cra", combineReducers(craMovements, reduceRiftLaunchAck, reduceAdvisorMovement)},
		{"cds", newMovementReducer(false)},
		{"mcm", combineReducers(newMovementReducer(false), reduceAdvisorMovement)},
		{"crm", combineReducers(newMovementReducer(false), reduceResponseResources)},
		{"mrm", reduceMovementRemoval},
		{"gaa", combineReducers(reduceMapSnapshot, reducePlayerProtectionMode)},
		{"fnm", reduceNestedMapSnapshot},
		{"rae", reduceInvasionFortification},
		{"rce", reduceInvasionFortificationCounters},
		{"ssi", reduceNestedMapSnapshot},
		{"sdc", combineReducers(reduceDungeonCooldownSkip, reduceResponseResources)},
		{"msd", combineReducers(reduceDungeonCooldownSkip, reduceResponseResources)},
		{"adi", reduceAttackDialog},
		{"gas", reduceAttackPresets},
		{"sin", reduceStorageInventory},
		{"gbc", reduceConstructionOffers},
		{"sbp", combineReducers(reduceResponseResources, reduceStormShopResponse)},
		{"sne", reduceReportNotices},
		{"dms", reduceDeletedReportMessages},
		{"bsd", reduceSpyReportCapture},
		{"bls", combineReducers(reduceBattleSummaryCapture, reduceSuccessfulTowerBattle, reduceSuccessfulNomadCampBattle)},
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
	if err := registry.RegisterOutbound("ahr", reduceAllianceHelpCommand); err != nil {
		return err
	}
	if err := registry.RegisterOutbound("cra", reduceRiftLaunchCapture); err != nil {
		return err
	}
	if err := registry.RegisterOutbound("gbc", reduceConstructionOffersCommand); err != nil {
		return err
	}
	if err := registry.RegisterOutbound("sbp", reduceStormShopCommand); err != nil {
		return err
	}
	for _, opcode := range []string{"blm", "bld"} {
		if err := registry.RegisterOutbound(opcode, reduceBattleCommandContext); err != nil {
			return err
		}
	}
	return nil
}

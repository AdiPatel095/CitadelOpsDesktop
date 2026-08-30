package Ingest

import (
	"fmt"

	"CitadelDesktop/Server/State"
)

func RegisterCoreReducers(registry *Registry) error {
	if registry == nil {
		return fmt.Errorf("ingest registry is required")
	}
	craMovements := newMovementReducer(false)
	components := State.Components
	player := components(State.ComponentPlayer)
	castles := components(State.ComponentCastles)
	production := components(State.ComponentCastles, State.ComponentAllianceHelp)
	resources := components(State.ComponentPlayer, State.ComponentCastles)
	alliance := components(State.ComponentPlayer, State.ComponentAlliance, State.ComponentAlliances)
	leaders := components(State.ComponentCommanders, State.ComponentCastellans, State.ComponentInventory)
	castleSnapshot := components(
		State.ComponentSession, State.ComponentCastles, State.ComponentPlayer, State.ComponentAlliance,
	)
	movements := components(
		State.ComponentMovements, State.ComponentMovementSnapshot, State.ComponentCommanders,
		State.ComponentKhan, State.ComponentEventScores,
	)
	worldMap := components(
		State.ComponentWorldMap, State.ComponentTowerCooldowns, State.ComponentNomadCamps,
		State.ComponentStorm, State.ComponentBeri, State.ComponentKhan,
	)
	reports := components(State.ComponentReports)
	equipment := components(
		State.ComponentCommanders, State.ComponentCastellans, State.ComponentInventory, State.ComponentPlayer,
	)
	reducers := []struct {
		opcode  string
		writes  State.ComponentSet
		reducer Reducer
	}{
		{"gbd", State.AllComponents, reduceInitialState},
		{"gpi", player, reducePlayerInfo},
		{"gcl", castles, reduceCastleList},
		{"ksc", castles, reduceCastleList},
		{"gcu", player, reduceGlobalResources},
		{"gmu", player, newPlayerMetricReducer("MP", "gmu")},
		{"ufa", player, newPlayerMetricReducer("CF", "ufa")},
		{"ufp", player, newPlayerMetricReducer("CFP", "ufp")},
		{"gdi", alliance, reducePlayerSummary},
		{"vip", player, reduceVIPInfo},
		{"sce", player, reducePlayerCurrencies},
		{"gai", components(State.ComponentDailyAttacks), reduceDailyAttackCount},
		{"aa", components(State.ComponentAdvisor), reduceAdvisorActivation},
		{"aao", components(State.ComponentAdvisor), reduceAdvisorOverview},
		{"vli", player, reduceAchievementSnapshot},
		{"skl", player, reduceLegendSkills},
		{"skp", player, reduceLegendSkillPurchase},
		{"skr", player, reduceLegendSkillReset},
		{"grc", resources, reduceResponseResources},
		{"gpa", resources, reduceResponseResources},
		{"sei", components(State.ComponentEventScores, State.ComponentKhan, State.ComponentInvasion), reduceScalableEventSnapshot},
		{"rpr", components(State.ComponentKhan), reduceKhanRagePoints},
		{"pep", components(State.ComponentEventScores), reduceEventPoints},
		{"hgh", components(State.ComponentEventScores), reduceEventRanking},
		{"dcl", castleSnapshot, reduceCastleDetails},
		{"dfc", castles, reduceDefenseContext},
		{"dfw", castles, reduceDefenseWall},
		{"dfk", castles, reduceDefenseKeep},
		{"dfm", castles, reduceDefenseMoat},
		{"gpc", castles, reduceQueueableProduction},
		{"boi", components(State.ComponentMarket), reduceMarketBooster},
		{"cmi", components(State.ComponentMarket, State.ComponentCastles), reduceMarketInfo},
		{"kpi", components(State.ComponentKingdomTransport), reduceKingdomTransport},
		{"kgt", components(State.ComponentKingdomTransport), reduceKingdomTransport},
		{"kut", components(State.ComponentKingdomTransport), reduceKingdomTransport},
		{"msk", components(State.ComponentKingdomTransport), reduceKingdomTransport},
		{"sie", components(State.ComponentSubscriptions), reduceSubscriptions},
		{"upc", components(State.ComponentSubscriptions), reduceSubscriptions},
		{"fuc", components(State.ComponentBeri), reduceBeriCapacity},
		{"gli", leaders, reduceLeaders},
		{"gie", components(State.ComponentGenerals), reduceGenerals},
		{"gei", components(State.ComponentInventory), reduceEquipmentStorage},
		{"ggm", components(State.ComponentInventory), reduceGemStorage},
		{"gii", components(State.ComponentInventory), reduceConstructionInventory},
		{"abpi", castles, reduceBuildingProduction},
		{"gui", castles, reduceFocusedUnits},
		{"etc", castles, reduceBuildingMutation},
		{"emo", castles, reduceBuildingMutation},
		{"sob", castles, reduceBuildingMutation},
		{"ego", castles, reduceBuildingMutation},
		{"scl", castles, reduceBuildingMutation},
		{"ahh", components(State.ComponentAllianceHelp, State.ComponentCastles), reduceAllianceHelpRequest},
		{"ahl", components(State.ComponentAllianceHelp, State.ComponentCastles), reduceAllianceHelpRequest},
		{"ahd", components(State.ComponentAllianceHelp), reduceAllianceHelpDelete},
		{"crin", castles, reduceCraftingSnapshot},
		{"crst", resources, combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crun", resources, combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crsk", resources, combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"crca", resources, combineReducers(reduceCraftingBuilding, reduceResponseResources)},
		{"csm", movements, newMovementReducer(false)},
		{"cds", movements, newMovementReducer(false)},
		{"mrm", movements, reduceMovementRemoval},
		{"rae", components(State.ComponentInvasion), reduceInvasionFortification},
		{"rce", components(State.ComponentInvasion), reduceInvasionFortificationCounters},
		{"adi", worldMap.Union(components(State.ComponentAttackDialog)), reduceAttackDialog},
		{"gas", components(State.ComponentAttackPresets), reduceAttackPresets},
		{"sin", components(State.ComponentInventory), reduceStorageInventory},
		{"gbc", components(State.ComponentInventory), reduceConstructionOffers},
		{"csp", components(State.ComponentInventory), reduceConstructionSpaceLeft},
		{"sne", reports, reduceReportNotices},
		{"dms", reports, reduceDeletedReportMessages},
		{"bsd", reports, reduceSpyReportCapture},
		{"blm", reports, reduceBattleWaveCapture},
		{"bld", reports.Union(components(State.ComponentStorm, State.ComponentEventScores, State.ComponentAttackAnalytics)), reduceBattleDetailCapture},
		{"eeq", equipment, reduceEquipmentMutation},
		{"bge", equipment, reduceEquipmentMutation},
		{"ege", equipment, reduceEquipmentMutation},
		{"ere", equipment, reduceEquipmentMutation},
		{"eqe", equipment, reduceEquipmentMutation},
		{"gsue", equipment, reduceEquipmentMutation},
		{"guse", equipment, reduceEquipmentMutation},
		{"seq", equipment, reduceEquipmentMutation},
		{"sge", equipment, reduceEquipmentMutation},
		{"gnr", equipment, reduceEquipmentMutation},
	}
	for _, entry := range reducers {
		if err := registry.RegisterComponents(entry.opcode, entry.writes, entry.reducer); err != nil {
			return err
		}
	}
	sequenceGroups := []struct {
		opcodes []string
		steps   []reducerStep
	}{
		{[]string{"mos"}, []reducerStep{
			{writes: castles, reducer: reduceOpenGate},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"ovs", "bms", "btx", "bis", "bcs", "brs"}, []reducerStep{
			{writes: resources, reducer: reduceResponseResources},
			{writes: components(State.ComponentMarket), reducer: reduceMarketBooster},
		}},
		{[]string{"bfs"}, []reducerStep{
			{writes: resources, reducer: reduceResponseResources},
			{writes: components(State.ComponentMarket), reducer: reduceMarketFeast},
		}},
		{[]string{"ain"}, []reducerStep{
			{writes: alliance, reducer: reduceAllianceInfo},
			{writes: player, reducer: reducePlayerProtectionMode},
		}},
		{[]string{"jaa"}, []reducerStep{
			{writes: castleSnapshot, reducer: reduceCastleSnapshot},
			{writes: production, reducer: reduceEmbeddedProductionSnapshots},
			{writes: components(State.ComponentInventory), reducer: reduceEmbeddedStorageInventory},
			{writes: resources, reducer: reduceResponseResources},
			{writes: player, reducer: reducePlayerProtectionMode},
		}},
		{[]string{"jca"}, []reducerStep{
			{writes: castleSnapshot, reducer: reduceCastleSnapshot},
			{writes: player, reducer: reducePlayerProtectionMode},
		}},
		{[]string{"hru", "hdu"}, []reducerStep{
			{writes: castles, reducer: reduceFocusedUnits},
			{writes: production, reducer: reduceProductionSnapshot},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"rpc", "ubc"}, []reducerStep{
			{writes: castles, reducer: reduceFocusedConstructionItems},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"ebe"}, []reducerStep{
			{writes: castleSnapshot, reducer: reduceExpansionMutation},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"ebu"}, []reducerStep{
			{writes: castles, reducer: reduceBuildingMutation},
			{writes: components(State.ComponentInventory), reducer: reduceEmbeddedStorageInventory},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"eup", "edo", "fco", "msb"}, []reducerStep{
			{writes: castles, reducer: reduceBuildingMutation},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"spl", "bup"}, []reducerStep{
			{writes: production, reducer: reduceProductionSnapshot},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"sdc", "msd"}, []reducerStep{
			{writes: worldMap, reducer: reduceDungeonCooldownSkip},
			{writes: resources, reducer: reduceResponseResources},
		}},
		{[]string{"sbp"}, []reducerStep{
			{writes: resources, reducer: reduceResponseResources},
			{writes: castles, reducer: reduceShopFocusedUnits},
			{writes: components(State.ComponentStorm, State.ComponentCastles), reducer: reduceStormShopResponse},
		}},
		{[]string{"bls"}, []reducerStep{
			{writes: reports, reducer: reduceBattleSummaryCapture},
			{writes: components(State.ComponentTowerCooldowns, State.ComponentNomadCamps, State.ComponentKhan), reducer: reduceSuccessfulTowerBattle},
			{writes: components(State.ComponentNomadCamps), reducer: reduceSuccessfulNomadCampBattle},
		}},
	}
	for _, group := range sequenceGroups {
		for _, opcode := range group.opcodes {
			if err := registry.registerComponentSequence(opcode, group.steps...); err != nil {
				return err
			}
		}
	}
	for _, opcode := range []string{"fnm", "fnt", "ssi"} {
		if err := registry.registerComponentSequence(opcode,
			reducerStep{writes: worldMap, reducer: reduceNestedMapSnapshot},
			reducerStep{writes: player, reducer: reduceNestedMapPlayerProtection},
		); err != nil {
			return err
		}
	}
	if err := registry.registerComponentSequence("gam",
		reducerStep{writes: movements, reducer: newMovementReducer(true)},
		reducerStep{writes: components(State.ComponentAdvisor, State.ComponentEventScores), reducer: reduceAdvisorMovement},
		reducerStep{writes: player, reducer: reducePlayerTitles},
	); err != nil {
		return err
	}
	for _, opcode := range []string{"cat", "mcm"} {
		if err := registry.registerComponentSequence(opcode,
			reducerStep{writes: movements, reducer: newMovementReducer(false)},
			reducerStep{writes: components(State.ComponentAdvisor, State.ComponentEventScores), reducer: reduceAdvisorMovement},
		); err != nil {
			return err
		}
	}
	if err := registry.registerComponentSequence("cra",
		reducerStep{writes: movements, reducer: craMovements},
		reducerStep{writes: components(State.ComponentRift), reducer: reduceRiftLaunchAck},
		reducerStep{writes: components(State.ComponentAdvisor, State.ComponentEventScores), reducer: reduceAdvisorMovement},
		reducerStep{writes: components(State.ComponentCombatCooldown), reducer: reduceCombatCooldownOnCommanderBusy},
	); err != nil {
		return err
	}
	if err := registry.registerComponentSequence("crm",
		reducerStep{writes: movements, reducer: newMovementReducer(false)},
		reducerStep{writes: resources, reducer: reduceResponseResources},
	); err != nil {
		return err
	}
	if err := registry.registerComponentSequence("gaa",
		reducerStep{writes: worldMap, reducer: reduceMapSnapshot},
		reducerStep{writes: player, reducer: reducePlayerProtectionMode},
	); err != nil {
		return err
	}
	if err := registry.RegisterOutboundComponents(
		"bup", components(State.ComponentCommandContext), reduceProductionCommandContext,
	); err != nil {
		return err
	}
	if err := registry.RegisterOutboundComponents("cra", components(State.ComponentRift), reduceRiftLaunchCapture); err != nil {
		return err
	}
	if err := registry.RegisterOutboundComponents(
		"gbc", components(State.ComponentInventory), reduceConstructionOffersCommand,
	); err != nil {
		return err
	}
	if err := registry.RegisterOutboundComponents("sbp", components(State.ComponentStorm), reduceStormShopCommand); err != nil {
		return err
	}
	for _, opcode := range []string{"blm", "bld"} {
		if err := registry.RegisterOutboundComponents(opcode, reports, reduceBattleCommandContext); err != nil {
			return err
		}
	}
	return nil
}

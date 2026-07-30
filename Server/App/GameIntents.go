package App

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"CitadelDesktop/Server/GameData"
	"CitadelDesktop/Server/Intent"
	"CitadelDesktop/Server/State"
)

func (application *Application) registerGameIntents() error {
	if err := application.registerAutoBirdIntents(); err != nil {
		return err
	}
	if err := application.Intents.RegisterCommandDependencies("cra", application.resolveCRACommandDependencies); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("attack.cra.send.guard", application.guardCRASend); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("attack.daily_limit.guard", application.guardDailyAttackLimit); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("event.ranking.begin", application.beginEventRankingRefresh); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("defense.verify_refresh", application.verifyDefenseRefresh); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("defense.keep.verify", application.verifyDefenseKeep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("defense.wall.verify", application.verifyDefenseWall); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("defense.moat.verify", application.verifyDefenseMoat); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("defense.preset.verify", application.verifyDefensePreset); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("defense.keep.build", resolveDefenseKeepStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("defense.wall.build", resolveDefenseWallStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("defense.moat.build", resolveDefenseMoatStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("defense.preset.wall.build", resolveDefensePresetWallStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("defense.preset.keep.build", resolveDefensePresetKeepStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("attack.inventory.guard", application.guardCRAInventory); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("attack.analytics.capture", application.captureAttackFeatureLaunch); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("alliance.target.attack.build", application.resolveAllianceTargetAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("troops.station.build", resolveTroopsStationStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("movement.track_station", application.trackStationMovement); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("troops.kingdom.consume_source", application.consumeKingdomTroopSource); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("troops.kingdom.guard_target_cap", application.guardKingdomTroopTargetCap); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("resources.kingdom.consume_source", application.consumeKingdomResourceSource); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("resources.kingdom.complete_workflow", application.completeKingdomResourceWorkflow); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("resources.verify_target_capacity", application.verifyResourceTargetCapacity); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("equipment.verify_coin_reserve", application.verifyEquipmentCoinReserve); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("alliance.verify_inspection", application.verifyAllianceInspection); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("alliance.help.mark_requested", application.markAllianceHelpRequested); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("alliance.help.build", application.resolveAllianceHelpRequestStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("production.enqueue.verify_capacity", application.verifyProductionQueueCapacity); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("kingdom.transport.verify_available", application.verifyKingdomTransportAvailable); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.capacity.verify", application.verifyBeriCapacity); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.transfer.verify", application.verifyBeriTransfer); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.consume_capacity", application.consumeBeriCapacity); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.camp.open.verify", application.verifyBeriCampOpen); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.camp.opened", application.markBeriCampOpened); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.target.verify", application.verifyBeriTargetFound); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.tower.attack.guard", application.guardBeriTowerAttack); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("beri.tools.purchase.guard", application.guardBeriToolPurchase); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("beri.tower.attack.build", application.resolveBeriTowerAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("rift.template.rename", application.renameRiftTemplate); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("rift.template.delete", application.deleteRiftTemplate); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.queue.capture", application.captureTowerQueue); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.queue.consume", application.consumeTowerQueueEntry); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.queue.defer", application.deferTowerQueueEntry); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.queue.rotate_stale", application.rotateStaleTowerQueueEntry); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.attack.guard", application.guardTowerAttackInventory); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("tower.capacity.capture", application.captureTowerCapacity); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("tower.attack.build", application.resolveTowerAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.scan.capture", application.captureInvasionScan); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.attack.guard", application.guardInvasionAttack); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.target.guard", application.guardInvasionTarget); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.fortify.guard", application.guardInvasionFortify); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("invasion.attack.build", application.resolveInvasionAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.attack.capture", application.captureInvasionLaunch); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("invasion.target.consume", application.consumeInvasionTarget); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.scan.capture", application.captureNomadScan); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.run.reset", application.resetNomadRun); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.target.capture", application.captureNomadTarget); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.attack.guard", application.guardNomadCampAttack); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.attack.inventory.guard", application.guardNomadAttackInventory); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.attack.arrival.guard", application.guardNomadChainArrival); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.attack.capture", application.captureNomadCampLaunch); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.cooldown.guard", application.guardNomadCooldownSkip); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.cooldown.verify", application.verifyNomadCooldownSkip); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.cooldown.minute_skip.verify", application.verifyDungeonMinuteSkip); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("nomad.cooldown.minute_skip.build", resolveDungeonMinuteSkipStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("nomad.attack.build", application.resolveNomadCampAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("advisor.attack.build", application.resolveAdvisorAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.rbc_test.begin", application.beginNomadRBCTest); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.rbc_test.inventory.guard", application.guardNomadRBCTestInventory); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.rbc_test.attack.guard", application.guardNomadRBCTestAttack); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("nomad.rbc_test.launch.capture", application.captureNomadRBCTestLaunch); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("nomad.rbc_test.attack.build", application.resolveNomadRBCTestAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.attack.capture", application.captureKhanLaunch); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.taunt.dispatched", application.recordKhanTauntDispatch); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.cooldown.reports.resolve", application.resolveKhanCooldownReports); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.lane.guard", application.guardKhanLane); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.protection.guard", application.guardKhanProtection); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.protection.activate", application.activateKhanProtection); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.protection.clear", application.clearKhanProtection); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.defense_tools.guard", application.guardKhanDefenseToolPurchase); err != nil {
		return err
	}
	if err := application.Intents.RegisterAction("khan.defense_tools.purchased", application.markKhanDefenseToolPurchase); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("khan.attack.build", application.resolveKhanAttackStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("construction.equip.build", resolveConstructionEquipStep); err != nil {
		return err
	}
	if err := application.Intents.RegisterStepResolver("construction.upgrade.build", resolveConstructionUpgradeStep); err != nil {
		return err
	}
	definitions := []Intent.Definition{
		{
			Name: "game.refresh_movements", Description: "Request a fresh movement snapshot", Effect: Intent.EffectRead,
			Planner: func(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
				return Intent.Plan{
					Claims: []string{"game:movements"}, Summary: "Refresh movements",
					Steps: []Intent.Step{commandStep("Refresh movements", "gam", json.RawMessage(`{}`), "gam")},
				}, nil
			},
		},
		{
			Name: "troops.station", Description: "Station validated troop stacks at a same-kingdom alliance holding", Effect: Intent.EffectLaunch,
			Planner: planTroopsStation,
		},
		{
			Name: "troops.kingdom.refresh", Description: "Refresh pending kingdom troop transports", Effect: Intent.EffectRead,
			Planner: planKingdomTroopRefresh,
		},
		{
			Name: "troops.kingdom.ship", Description: "Transfer validated troop stacks from an owned donor castle to another kingdom", Effect: Intent.EffectLaunch,
			Planner: planKingdomTroopShipment,
		},
		{
			Name: "troops.kingdom.skip", Description: "Apply an available official time skip to a pending kingdom troop transport", Effect: Intent.EffectWrite,
			Planner: planKingdomTroopSkip,
		},
		{
			Name: "movement.recall", Description: "Recall an active player-owned movement", Effect: Intent.EffectLaunch,
			Planner: planMovementRecall,
		},
		{
			Name: "equipment.refresh", Description: "Refresh loadouts, equipment storage, and gem storage", Effect: Intent.EffectRead,
			Planner: planEquipmentRefresh,
		},
		{
			Name: "equipment.equip", Description: "Equip a validated storage item on a commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentEquip,
		},
		{
			Name: "equipment.unequip", Description: "Unequip one or more validated items from a commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentUnequip,
		},
		{
			Name: "equipment.gem.equip", Description: "Socket a validated relic gem into equipped gear", Effect: Intent.EffectWrite,
			Planner: planGemEquip,
		},
		{
			Name: "equipment.gem.unequip", Description: "Remove a socketed gem from equipped gear", Effect: Intent.EffectWrite,
			Planner: planGemUnequip,
		},
		{
			Name: "equipment.swap", Description: "Swap base equipment and attached gems between two leaders", Effect: Intent.EffectWrite,
			Planner: planEquipmentSwap,
		},
		{
			Name: "equipment.reconfigure", Description: "Apply a validated optimizer loadout to one commander or castellan", Effect: Intent.EffectWrite,
			Planner: planEquipmentReconfigure,
		},
		{
			Name: "equipment.event.apply", Description: "Replace a commander's base equipment with one coherent owned event set", Effect: Intent.EffectWrite,
			Planner: planEquipmentEventApply,
		},
		{
			Name: "equipment.upgrade", Description: "Upgrade equipment or a relic gem to a validated target level", Effect: Intent.EffectWrite,
			Planner: application.planEquipmentUpgrade,
		},
		{
			Name: "equipment.sell", Description: "Sell a deterministic equipment or gem storage selection", Effect: Intent.EffectWrite,
			Planner: planEquipmentSell,
		},
		{
			Name: "game.focus_castle", Description: "Focus one of the player's castles when it is not already focused", Effect: Intent.EffectRead,
			Planner: planCastleFocus,
		},
		{
			Name: "defense.refresh", Description: "Refresh one castle's defense setup and current troop/tool inventory", Effect: Intent.EffectRead,
			Planner: planDefenseRefresh,
		},
		{
			Name: "defense.open_gate", Description: "Open an owned Great Empire castle's gates for six hours", Effect: Intent.EffectWrite,
			Planner: planDefenseOpenGate,
		},
		{
			Name: "defense.keep.update", Description: "Apply and read back validated DFK allocation, keep-tool, and Sceat-support rows", Effect: Intent.EffectWrite,
			Planner: planDefenseKeepUpdate,
		},
		{
			Name: "defense.wall.update", Description: "Apply and read back a validated wall setup using the captured DFW layout", Effect: Intent.EffectWrite,
			Planner: planDefenseWallUpdate,
		},
		{
			Name: "defense.moat.update", Description: "Apply and read back a validated moat setup using the captured DFM layout", Effect: Intent.EffectWrite,
			Planner: planDefenseMoatUpdate,
		},
		{
			Name: "defense.preset.apply", Description: "Refresh, validate, apply, and read back a complete reusable defense preset", Effect: Intent.EffectWrite,
			Planner: planDefensePresetApply,
		},
		{
			Name: "alliance.refresh", Description: "Refresh the current alliance and member state", Effect: Intent.EffectRead,
			Planner: planAllianceRefresh,
		},
		{
			Name: "alliance.inspect", Description: "Fetch a selected alliance into the canonical alliance directory", Effect: Intent.EffectRead,
			Planner: planAllianceInspect,
		},
		{
			Name: "alliance.help.request", Description: "Request alliance help for an eligible production job", Effect: Intent.EffectWrite,
			Planner: planAllianceHelpRequest,
		},
		{
			Name: "map.query", Description: "Query an inclusive rectangular world-map viewport", Effect: Intent.EffectRead,
			Planner: planMapQuery,
		},
		{
			Name: "construction.equip", Description: "Equip an official construction-item definition on a castle building", Effect: Intent.EffectWrite,
			Planner: planConstructionEquip, ReadSet: constructionReadSet,
		},
		{
			Name: "construction.upgrade", Description: "Upgrade the construction item currently equipped in a building slot", Effect: Intent.EffectWrite,
			Planner: planConstructionUpgrade, ReadSet: constructionReadSet,
		},
		{
			Name: "construction.shop", Description: "Request the live construction-item offers for a castle", Effect: Intent.EffectRead,
			Planner: planConstructionShop, ReadSet: constructionReadSet,
		},
		{
			Name: "construction.inventory.refresh", Description: "Refresh the account construction-item inventory", Effect: Intent.EffectRead,
			Planner: planConstructionInventoryRefresh, ReadSet: constructionReadSet,
		},
		{
			Name: "construction.purchase", Description: "Buy an official construction-item package from a live shop offer", Effect: Intent.EffectWrite,
			Planner: planConstructionPurchase, ReadSet: constructionReadSet,
		},
		{
			Name: "crafting.refresh", Description: "Request all sovereign crafting queues and research entitlements", Effect: Intent.EffectRead,
			Planner: func(_ context.Context, _ Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
				return Intent.Plan{
					Claims: []string{"game:crafting"}, Summary: "Refresh crafting queues",
					Steps: []Intent.Step{commandStep("Refresh crafting queues", "crin", json.RawMessage(`{}`), "crin")},
				}, nil
			},
		},
		{
			Name: "crafting.start", Description: "Start or queue one official crafting recipe", Effect: Intent.EffectWrite,
			Planner: planCraftingStart,
		},
		{
			Name: "crafting.rent_slot", Description: "Rent the next configured sovereign crafting slot", Effect: Intent.EffectWrite,
			Planner: planCraftingSlotRental,
		},
		{
			Name: "crafting.skip", Description: "Complete one active sovereign craft at its official remaining-time ruby price", Effect: Intent.EffectWrite,
			Planner: planCraftingSkip,
		},
		{
			Name: "resource.logistics.refresh", Description: "Refresh market, caravan, and kingdom-resource transport state", Effect: Intent.EffectRead,
			Planner: planResourceLogisticsRefresh,
		},
		{
			Name: "resource.ship", Description: "Send resources between owned castles using the transport mode required by their kingdoms", Effect: Intent.EffectLaunch,
			Planner: planResourceShipment,
		},
		{
			Name: "resource.market.ship", Description: "Send a validated same-kingdom market shipment between owned castles", Effect: Intent.EffectLaunch,
			Planner: planMarketResourceShipment,
		},
		{
			Name: "resource.kingdom.ship", Description: "Send a validated resource shipment to another owned kingdom", Effect: Intent.EffectLaunch,
			Planner: planKingdomResourceShipment,
		},
		{
			Name: "resource.kingdom.skip", Description: "Apply an available official time skip to a pending kingdom-resource shipment", Effect: Intent.EffectWrite,
			Planner: planKingdomResourceSkip,
		},
		{
			Name: "resource.kingdom.settle", Description: "Refresh an automation-owned kingdom-resource destination after delivery", Effect: Intent.EffectRead,
			Planner: planKingdomResourceSettlement,
		},
		{
			Name: "production.enqueue", Description: "Enqueue an official troop or tool definition using observed production context", Effect: Intent.EffectWrite,
			Planner: planProductionEnqueue,
		},
		{
			Name: "hospital.heal", Description: "Heal a non-premium wounded unit stack at an owned castle", Effect: Intent.EffectWrite,
			Planner: planHospitalHeal,
		},
		{
			Name: "hospital.discard", Description: "Discard a wounded unit stack at an owned castle", Effect: Intent.EffectWrite,
			Planner: planHospitalDiscard,
		},
		{
			Name: "spy.launch", Description: "Launch a military espionage mission from an owned castle", Effect: Intent.EffectLaunch,
			Planner: planSpyLaunch,
		},
		{
			Name: "alliance.target.attack", Description: "Launch a selected CitadelOps preset against an alliance target", Effect: Intent.EffectLaunch,
			Planner: planAllianceTargetAttack,
		},
		{
			Name: "tower.queue.scan", Description: "Focus one configured castle, refresh its tower map, and capture a fresh target batch", Effect: Intent.EffectRead,
			Planner: planTowerQueueScan,
		},
		{
			Name: "tower.queue.target.refresh", Description: "Refresh one queued tower and rotate it behind other targets when the response remains stale", Effect: Intent.EffectRead,
			Planner: planTowerQueueTargetRefresh,
		},
		{
			Name: "tower.context.refresh", Description: "Refresh selected kingdom-tower attack context and saved formations", Effect: Intent.EffectRead,
			Planner: planTowerContext,
		},
		{
			Name: "tower.attack", Description: "Admit and atomically launch a contextual full-flank kingdom-tower attack", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoTowers", Label: "Auto Towers", Description: "Robber-baron and kingdom tower attacks", DefaultWeight: 50},
			Planner:      planTowerAttack,
		},
		{
			Name: "tower.launch", Description: "Launch a full-flank configured troop attack against a refreshed kingdom tower", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoTowers", Label: "Auto Towers", Description: "Robber-baron and kingdom tower attacks", DefaultWeight: 50},
			Planner:      planTowerLaunch,
		},
		{
			Name: "invasion.difficulty.select", Description: "Select the configured difficulty for an active Foreign Lords or Bloodcrow event without premium spending", Effect: Intent.EffectWrite,
			Planner: planInvasionDifficulty,
		},
		{
			Name: "invasion.map.scan", Description: "Focus the configured castle and refresh nearby invasion-event targets", Effect: Intent.EffectRead,
			Planner: planInvasionMapScan,
		},
		{
			Name: "invasion.attack", Description: "Launch a CitadelOps attack preset against a Foreign Lords or Bloodcrow castle", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoInvasion", Label: "Auto Invasion", Description: "Foreign Lords and Bloodcrow attacks", DefaultWeight: 50},
			Planner:      planInvasionAttack,
		},
		{
			Name: "nomad.difficulty.select", Description: "Start an active Nomad or Samurai event at the configured unlocked difficulty", Effect: Intent.EffectWrite,
			Planner: planNomadDifficulty,
		},
		{
			Name: "nomad.map.scan", Description: "Focus the configured castle and discover its four regular Nomad or Samurai camps", Effect: Intent.EffectRead,
			Planner: planNomadMapScan,
		},
		{
			Name: "nomad.target.lock", Description: "Lock the weakest of four maxed regular Nomad or Samurai camps", Effect: Intent.EffectWrite,
			Planner: planNomadTargetLock,
		},
		{
			Name: "nomad.camp.attack", Description: "Level one camp or chain a preset through all available commanders against the locked camp", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoNomad", Label: "Auto Nomad/Samurai", Description: "Four-camp leveling and locked-camp attack chains", DefaultWeight: 50},
			Planner:      planNomadCampAttack,
		},
		{
			Name: "nomad.rbc_test.attack", Description: "Launch an opportunistic Auto Camp chain against one robber-baron castle", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoNomad", Label: "Auto Nomad/Samurai", Description: "Four-camp leveling and locked-camp attack chains", DefaultWeight: 50},
			Planner:      planNomadRBCTestAttack,
		},
		{
			Name: "nomad.cooldown.skip", Description: "Reset the locked regular camp cooldown within configured ruby limits", Effect: Intent.EffectWrite,
			Planner: planNomadCooldownSkip,
		},
		{
			Name: "nomad.cooldown.minute_skip", Description: "Apply an inventory time skip to a tower, Nomad, or Samurai cooldown", Effect: Intent.EffectWrite,
			Planner: planDungeonMinuteSkip,
		},
		{
			Name: "advisor.activate", Description: "Explicitly consume one available event advisor token after confirmation", Effect: Intent.EffectWrite,
			Planner: planAdvisorActivation,
		},
		{
			Name: "advisor.overview.refresh", Description: "Refresh cumulative advisor gains, costs, losses, wins, and remaining attacks", Effect: Intent.EffectRead,
			Planner: planAdvisorOverview,
		},
		{
			Name: "event.ranking.refresh", Description: "Fetch the active Nomad alliance leaderboard from GGE", Effect: Intent.EffectRead,
			Planner: planEventRankingRefresh,
		},
		{
			Name: "advisor.run.launch", Description: "Launch one guarded server-managed Nomad or Samurai advisor run", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoAdvisor", Label: "Auto Advisor", Description: "Server-managed Nomad and Samurai advisor attack runs", DefaultWeight: 50},
			Planner:      planAdvisorAttack,
		},
		{
			Name: "khan.attack", Description: "Launch one guarded attack against the active Nomad Khan camp", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoKhan", Label: "Auto Khan", Description: "Guarded Khan camp attacks and retaliation chains", DefaultWeight: 50},
			Planner:      planKhanAttack,
		},
		{
			Name: "khan.taunt", Description: "Trigger the Khan retaliation when the player rage bar is full", Effect: Intent.EffectLaunch,
			Planner: planKhanTaunt,
		},
		{
			Name: "khan.cooldown.reports.resolve", Description: "Resolve report-linked Khan cooldown work after a fresh zero-second target re-ping", Effect: Intent.EffectWrite,
			Planner: planKhanCooldownReportResolve,
		},
		{
			Name: "khan.map.jump", Description: "Locate and jump the world map directly to the active Nomad Khan camp", Effect: Intent.EffectRead,
			Planner: planKhanMapJump,
		},
		{
			Name: "khan.open_gate", Description: "Open the main castle gates and activate the six-hour Auto Khan safety lock", Effect: Intent.EffectWrite,
			Planner: planKhanOpenGate,
		},
		{
			Name: "khan.point_limit.protect", Description: "Recall outbound Auto Khan movements and open gates after the Nomad point limit", Effect: Intent.EffectWrite,
			Planner: planKhanPointLimitProtection,
		},
		{
			Name: "khan.defense_tools.replenish", Description: "Buy an active non-ruby shop package for a missing Auto Khan defense tool", Effect: Intent.EffectWrite,
			Planner: planKhanDefenseToolReplenish,
		},
		{
			Name: "khan.protection.clear", Description: "Clear an expired Auto Khan safety lock after defense units recover", Effect: Intent.EffectWrite,
			Planner: planKhanProtectionClear,
		},
		{
			Name: "beri.capacity.refresh", Description: "Refresh the active Berimond castle troop-transfer capacity", Effect: Intent.EffectRead,
			Planner: planBeriCapacityRefresh,
		},
		{
			Name: "beri.transfer", Description: "Transfer a validated troop batch to Berimond and apply its fixed speed-up", Effect: Intent.EffectLaunch,
			Planner: planBeriTransfer,
		},
		{
			Name: "beri.tools.refresh", Description: "Refresh the owned Berimond camp tool inventory", Effect: Intent.EffectRead,
			Planner: planBeriToolInventoryRefresh,
		},
		{
			Name: "beri.tools.purchase", Description: "Buy one game-capped batch of a supported coin attack tool from the Berimond armorer", Effect: Intent.EffectWrite,
			Planner: planBeriToolPurchase,
		},
		{
			Name: "beri.camp.open", Description: "Open the cheapest unlocked non-premium Berimond camp", Effect: Intent.EffectWrite,
			Planner: planBeriCampOpen,
		},
		{
			Name: "beri.target.find", Description: "Use Berimond's find-next command to select the next available tower", Effect: Intent.EffectRead,
			Planner: planBeriTargetFind,
		},
		{
			Name: "beri.tower.attack", Description: "Launch one guarded CitadelOps preset against the selected Berimond tower", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "autoBeriWorld", Label: "Auto Beri World", Description: "Berimond troop transfers and guarded tower attacks", DefaultWeight: 50},
			Planner:      planBeriTowerAttack,
		},
		{
			Name: "rift.maiden_wave.launch", Description: "Launch deterministic Rift probe waves from an optional eligible commander pool", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "riftMaiden", Label: "Rift Maiden Waves", Description: "Shield-maiden probe and wave launches", DefaultWeight: 50},
			Planner:      planMaidenCommsWave,
		},
		{
			Name: "rift.launch.replay", Description: "Replay a captured Rift attack template with one or more selected commanders", Effect: Intent.EffectLaunch,
			AttackModule: &Intent.AttackModuleDefinition{ID: "riftReplay", Label: "Rift Replays", Description: "Captured Rift attack templates", DefaultWeight: 50},
			Planner:      application.planRiftReplay,
		},
		{
			Name: "rift.template.rename", Description: "Rename a captured Rift attack template", Effect: Intent.EffectWrite,
			Planner: planRiftTemplateRename,
		},
		{
			Name: "rift.template.delete", Description: "Delete a captured Rift attack template and cancel its schedule", Effect: Intent.EffectWrite,
			Planner: planRiftTemplateDelete,
		},
		{
			Name: "decoration.apply_preset", Description: "Reconcile one castle's decoration layout with an official-definition preset", Effect: Intent.EffectWrite,
			Planner: planDecorationPreset,
		},
		{
			Name: "report.spy.fetch", Description: "Fetch one spy report from an observed inbox notice", Effect: Intent.EffectRead,
			Planner: planSpyReportFetch,
		},
		{
			Name: "report.spy.share", Description: "Share one captured player-castle spy report with alliance members", Effect: Intent.EffectWrite,
			Planner: planSpyReportShare,
		},
		{
			Name: "report.battle.summary", Description: "Fetch one battle report summary from an observed inbox notice", Effect: Intent.EffectRead,
			Planner: planBattleReportSummary,
		},
		{
			Name: "report.battle.details", Description: "Fetch battle waves, units, and tools using summary-derived report context", Effect: Intent.EffectRead,
			Planner: planBattleReportDetails,
		},
	}
	for _, definition := range definitions {
		if err := application.Intents.Registry().Register(definition); err != nil {
			return err
		}
	}
	return application.registerLegendSkillIntents()
}

func planCraftingStart(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID           State.CastleID           `json:"castleId"`
		BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
		RecipeID           int64                    `json:"recipeId"`
		Power              int                      `json:"power,omitempty"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	if !castle.SupportsSovereignCrafting() {
		return Intent.Plan{}, fmt.Errorf("castle %d is a sovereign-resource storage node, not a crafting castle", request.CastleID)
	}
	building, ok := castle.Crafting.Buildings[request.BuildingInstanceID]
	if !ok || request.BuildingInstanceID <= 0 {
		return Intent.Plan{}, fmt.Errorf("crafting building %d is not in castle %d", request.BuildingInstanceID, request.CastleID)
	}
	if request.RecipeID <= 0 || input.GameData == nil {
		return Intent.Plan{}, fmt.Errorf("recipeId must reference the loaded official catalog")
	}
	catalog, err := input.GameData.Catalog("craftingRecipes")
	if err != nil {
		return Intent.Plan{}, err
	}
	rawRecipe, exists := catalog.Find(strconv.FormatInt(request.RecipeID, 10))
	if !exists {
		return Intent.Plan{}, fmt.Errorf("crafting recipe %d is not in the current official catalog", request.RecipeID)
	}
	recipe, err := GameData.DecodeRecord(rawRecipe)
	if err != nil {
		return Intent.Plan{}, fmt.Errorf("decode crafting recipe %d: %w", request.RecipeID, err)
	}
	queueTypeID, _ := recipe.Int64("queueTypeId")
	if int(queueTypeID) != building.QueueTypeID {
		return Intent.Plan{}, fmt.Errorf("crafting recipe %d belongs to queue %d, not queue %d", request.RecipeID, queueTypeID, building.QueueTypeID)
	}
	if required, _ := recipe.String("requiredCraftingBuildings"); strings.TrimSpace(required) != "" {
		allowed := false
		for _, part := range strings.FieldsFunc(required, func(character rune) bool { return character == ',' || character == '#' }) {
			definitionID, parseErr := strconv.ParseInt(strings.TrimSpace(part), 10, 64)
			if parseErr == nil && State.BuildingID(definitionID) == building.DefinitionID {
				allowed = true
				break
			}
		}
		if !allowed {
			return Intent.Plan{}, fmt.Errorf("crafting recipe %d is not valid for building definition %d", request.RecipeID, building.DefinitionID)
		}
	}
	if err := validateCraftingStartAvailability(input.State, input.GameData, castle, building, request.RecipeID); err != nil {
		return Intent.Plan{}, err
	}
	payload, _ := json.Marshal(struct {
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
		BuildingID State.BuildingInstanceID `json:"OID"`
		Power      int                      `json:"PWR"`
		RecipeID   int64                    `json:"CRID"`
	}{castle.KingdomID, castle.ID, building.InstanceID, request.Power, request.RecipeID})
	return Intent.Plan{
		Claims: []string{
			"castle:" + strconv.FormatInt(int64(castle.ID), 10),
			"crafting-building:" + strconv.FormatInt(int64(building.InstanceID), 10),
			"account-resources",
		},
		Summary: fmt.Sprintf("Queue crafting recipe %d at %s", request.RecipeID, castleLabel(castle)),
		Steps:   []Intent.Step{commandStep("Queue crafting recipe", "crst", payload, "crst")},
	}, nil
}

func planCastleFocus(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	return Intent.Plan{
		Claims: []string{"castle-focus"}, Summary: fmt.Sprintf("Focus %s", castleLabel(castle)),
		Steps: castleContextSteps(castle),
	}, nil
}

func planAllianceRefresh(_ context.Context, input Intent.PlanningContext, _ json.RawMessage) (Intent.Plan, error) {
	if input.State.Alliance.ID <= 0 {
		return Intent.Plan{}, fmt.Errorf("the current player's alliance is not known")
	}
	payload, _ := json.Marshal(struct {
		AllianceID State.AllianceID `json:"AID"`
	}{AllianceID: input.State.Alliance.ID})
	return Intent.Plan{
		Claims: []string{"alliance-directory"}, Summary: "Refresh alliance",
		Steps: []Intent.Step{commandStep("Refresh alliance", "ain", payload, "ain")},
	}, nil
}

func planAllianceInspect(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		AllianceID State.AllianceID `json:"allianceId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.AllianceID <= 0 {
		return Intent.Plan{}, fmt.Errorf("allianceId must be positive")
	}
	payload, _ := json.Marshal(struct {
		AllianceID State.AllianceID `json:"AID"`
	}{AllianceID: request.AllianceID})
	verification, _ := json.Marshal(request)
	return Intent.Plan{
		Claims: []string{"alliance-directory"}, Summary: fmt.Sprintf("Inspect alliance %d", request.AllianceID),
		Steps: []Intent.Step{
			commandStep("Inspect alliance", "ain", payload, "ain"),
			{Name: "Verify inspected alliance", Action: "alliance.verify_inspection", ActionArguments: verification},
		},
	}, nil
}

func (application *Application) verifyAllianceInspection(_ context.Context, arguments json.RawMessage) error {
	var request struct {
		AllianceID State.AllianceID `json:"allianceId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return err
	}
	alliance, found := application.State.Snapshot().Alliances[request.AllianceID]
	if !found || alliance.ID != request.AllianceID {
		return fmt.Errorf("alliance %d did not return a matching live roster", request.AllianceID)
	}
	return nil
}

func planMapQuery(_ context.Context, _ Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		KingdomID State.KingdomID `json:"kingdomId"`
		X1        int             `json:"x1"`
		Y1        int             `json:"y1"`
		X2        int             `json:"x2"`
		Y2        int             `json:"y2"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	if request.X1 > request.X2 || request.Y1 > request.Y2 {
		return Intent.Plan{}, fmt.Errorf("map bounds must be ordered from minimum to maximum")
	}
	if request.X2-request.X1 > 50 || request.Y2-request.Y1 > 50 {
		return Intent.Plan{}, fmt.Errorf("map query dimensions may not exceed 51 by 51 tiles")
	}
	payload, _ := json.Marshal(struct {
		KingdomID State.KingdomID `json:"KID"`
		X1        int             `json:"AX1"`
		Y1        int             `json:"AY1"`
		X2        int             `json:"AX2"`
		Y2        int             `json:"AY2"`
	}{request.KingdomID, request.X1, request.Y1, request.X2, request.Y2})
	return Intent.Plan{
		Claims:  []string{"castle-focus", "map:" + strconv.FormatInt(int64(request.KingdomID), 10)},
		Summary: fmt.Sprintf("Query map %d (%d,%d)-(%d,%d)", request.KingdomID, request.X1, request.Y1, request.X2, request.Y2),
		Steps:   []Intent.Step{commandStep("Query map", "gaa", payload, "gaa")},
	}, nil
}

type constructionEquipRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	DefinitionID       State.ConstructionItemID `json:"constructionItemId"`
	Slot               int                      `json:"slot"`
	Mode               int                      `json:"mode,omitempty"`
}

func planConstructionEquip(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request constructionEquipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, err := validatedConstructionEquipContext(input, request, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	steps := castleContextSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Equip construction item", Resolver: "construction.equip.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "rpc", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	return Intent.Plan{
		Claims:  constructionClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Equip construction item %d on building %d", request.DefinitionID, request.BuildingInstanceID),
		Steps:   steps,
	}, nil
}

func resolveConstructionEquipStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request constructionEquipRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, err := validatedConstructionEquipContext(input, request, true)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		ItemID     State.ConstructionItemID `json:"CID"`
		Slot       int                      `json:"SID"`
		Mode       int                      `json:"M"`
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
	}{request.BuildingInstanceID, request.DefinitionID, request.Slot, request.Mode, castle.KingdomID, castle.ID})
	return commandStep("Equip construction item", "rpc", payload, "rpc"), nil
}

func validatedConstructionEquipContext(input Intent.PlanningContext, request constructionEquipRequest, requireFreeSlot bool) (State.CastleState, error) {
	castle, err := constructionContext(input, request.CastleID, request.BuildingInstanceID)
	if err != nil {
		return State.CastleState{}, err
	}
	if request.DefinitionID <= 0 || input.GameData == nil {
		return State.CastleState{}, fmt.Errorf("constructionItemId must reference the loaded official catalog")
	}
	catalog, err := input.GameData.Catalog("constructionItems")
	if err != nil {
		return State.CastleState{}, err
	}
	rawItem, exists := catalog.Find(strconv.FormatInt(int64(request.DefinitionID), 10))
	if !exists {
		return State.CastleState{}, fmt.Errorf("construction item %d is not in the current official catalog", request.DefinitionID)
	}
	item, err := GameData.DecodeRecord(rawItem)
	if err != nil {
		return State.CastleState{}, err
	}
	if request.Slot < 0 {
		return State.CastleState{}, fmt.Errorf("slot cannot be negative")
	}
	groupID, _ := item.Int64("constructionItemGroupID")
	if groupID > 0 {
		building := castle.Buildings[request.BuildingInstanceID]
		buildingCatalog, catalogErr := input.GameData.BuildingCatalog()
		if catalogErr != nil {
			return State.CastleState{}, catalogErr
		}
		definition, found := buildingCatalog.Definition(int64(building.DefinitionID))
		if !found || !int64ListContains(definition.ConstructionItemGroupIDs, groupID) {
			return State.CastleState{}, fmt.Errorf(
				"building %d does not accept construction-item group %d", request.BuildingInstanceID, groupID,
			)
		}
	}
	if requireFreeSlot && !input.State.Inventory.ConstructionItemsObservedAt.IsZero() &&
		input.State.Inventory.ConstructionItems[request.DefinitionID] <= 0 {
		return State.CastleState{}, fmt.Errorf("construction item %d is not in observed inventory", request.DefinitionID)
	}
	targetSlot := request.Slot
	if slotType, exists := item.Int64("slotTypeID"); exists {
		targetSlot = int(slotType)
	}
	if requireFreeSlot && hasEquippedConstructionItemInSlot(
		castle.ConstructionSlots[request.BuildingInstanceID], catalog, targetSlot,
	) {
		return State.CastleState{}, fmt.Errorf(
			"building %d already has a construction item equipped in slot %d", request.BuildingInstanceID, targetSlot,
		)
	}
	return castle, nil
}

func int64ListContains(values []int64, target int64) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

type constructionUpgradeRequest struct {
	CastleID           State.CastleID           `json:"castleId"`
	BuildingInstanceID State.BuildingInstanceID `json:"buildingInstanceId"`
	DefinitionID       State.ConstructionItemID `json:"constructionItemId"`
	Slot               int                      `json:"slot"`
	OfferCode          int                      `json:"offerCode"`
}

func planConstructionUpgrade(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request constructionUpgradeRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, _, err := validatedConstructionUpgradeContext(input, request, false)
	if err != nil {
		return Intent.Plan{}, err
	}
	resolverArguments, _ := json.Marshal(request)
	steps := castleContextSteps(castle)
	steps = append(steps, Intent.Step{
		Name: "Upgrade construction item", Resolver: "construction.upgrade.build", ResolverArguments: resolverArguments,
		AwaitOpcode: "ubc", TimeoutMillis: 10_000, SuccessCodes: []int{0},
	})
	return Intent.Plan{
		Claims:  constructionClaims(castle.ID, request.BuildingInstanceID),
		Summary: fmt.Sprintf("Upgrade construction item %d on building %d", request.DefinitionID, request.BuildingInstanceID),
		Steps:   steps,
	}, nil
}

func resolveConstructionUpgradeStep(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Step, error) {
	var request constructionUpgradeRequest
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Step{}, err
	}
	castle, equipped, err := validatedConstructionUpgradeContext(input, request, true)
	if err != nil {
		return Intent.Step{}, err
	}
	payload, _ := json.Marshal(struct {
		BuildingID State.BuildingInstanceID `json:"OID"`
		OfferCode  int                      `json:"SUC"`
		Slot       int                      `json:"SID"`
		KingdomID  State.KingdomID          `json:"KID"`
		CastleID   State.CastleID           `json:"AID"`
		ItemID     State.ConstructionItemID `json:"CID"`
	}{request.BuildingInstanceID, request.OfferCode, request.Slot, castle.KingdomID, castle.ID, equipped})
	return commandStep("Upgrade construction item", "ubc", payload, "ubc"), nil
}

func validatedConstructionUpgradeContext(
	input Intent.PlanningContext,
	request constructionUpgradeRequest,
	requireEquipped bool,
) (State.CastleState, State.ConstructionItemID, error) {
	castle, err := constructionContext(input, request.CastleID, request.BuildingInstanceID)
	if err != nil {
		return State.CastleState{}, 0, err
	}
	if request.DefinitionID <= 0 || input.GameData == nil {
		return State.CastleState{}, 0, fmt.Errorf("constructionItemId must reference the equipped official construction item")
	}
	if request.OfferCode <= 0 {
		return State.CastleState{}, 0, fmt.Errorf("offerCode must identify the official target tier")
	}
	catalog, err := input.GameData.Catalog("constructionItems")
	if err != nil {
		return State.CastleState{}, 0, err
	}
	rawCurrent, exists := catalog.Find(strconv.FormatInt(int64(request.DefinitionID), 10))
	if !exists {
		return State.CastleState{}, 0, fmt.Errorf("construction item %d is not in the current official catalog", request.DefinitionID)
	}
	current, err := GameData.DecodeRecord(rawCurrent)
	if err != nil {
		return State.CastleState{}, 0, err
	}
	if !GameData.ConstructionItemIsTemporary(current) {
		return State.CastleState{}, 0, fmt.Errorf("construction item %d is not temporary", request.DefinitionID)
	}
	variantKey := GameData.ConstructionItemVariantKey(current)
	currentLevel, _ := current.Int64("level")
	nextLevel := int64(0)
	for _, raw := range catalog.Rows() {
		record, decodeErr := GameData.DecodeRecord(raw)
		if decodeErr != nil {
			continue
		}
		candidateLevel, _ := record.Int64("level")
		if GameData.ConstructionItemVariantKey(record) == variantKey && candidateLevel > currentLevel && (nextLevel == 0 || candidateLevel < nextLevel) {
			nextLevel = candidateLevel
		}
	}
	expectedCode := map[int64]int{2: 2000, 3: 2001, 4: 2002}[nextLevel]
	if expectedCode == 0 || request.OfferCode != expectedCode {
		return State.CastleState{}, 0, fmt.Errorf("offerCode %d does not match official target level %d", request.OfferCode, nextLevel)
	}
	if requireEquipped {
		for _, slot := range castle.ConstructionSlots[request.BuildingInstanceID] {
			if slot.Slot == request.Slot && slot.DefinitionID == request.DefinitionID {
				return castle, request.DefinitionID, nil
			}
		}
		return State.CastleState{}, 0, fmt.Errorf(
			"building %d does not have construction item %d in slot %d",
			request.BuildingInstanceID, request.DefinitionID, request.Slot,
		)
	}
	return castle, request.DefinitionID, nil
}

func planConstructionShop(_ context.Context, input Intent.PlanningContext, arguments json.RawMessage) (Intent.Plan, error) {
	var request struct {
		CastleID State.CastleID `json:"castleId"`
	}
	if err := decodeIntentArguments(arguments, &request); err != nil {
		return Intent.Plan{}, err
	}
	castle, ok := input.State.Castles[request.CastleID]
	if !ok || request.CastleID <= 0 {
		return Intent.Plan{}, fmt.Errorf("castle %d is not in the current player state", request.CastleID)
	}
	steps := castleContextSteps(castle)
	steps = append(steps, constructionShopContextSteps(castle)...)
	return Intent.Plan{
		Claims:  []string{"castle-focus", "castle:" + strconv.FormatInt(int64(castle.ID), 10), "construction-shop"},
		Summary: fmt.Sprintf("Load construction-item offers for %s", castleLabel(castle)), Steps: steps,
	}, nil
}

func constructionContext(input Intent.PlanningContext, castleID State.CastleID, buildingID State.BuildingInstanceID) (State.CastleState, error) {
	castle, ok := input.State.Castles[castleID]
	if !ok || castleID <= 0 {
		return State.CastleState{}, fmt.Errorf("castle %d is not in the current player state", castleID)
	}
	if _, ok := castle.Buildings[buildingID]; !ok || buildingID <= 0 {
		return State.CastleState{}, fmt.Errorf("building instance %d is not in castle %d", buildingID, castleID)
	}
	return castle, nil
}

func constructionClaims(castleID State.CastleID, buildingID State.BuildingInstanceID) []string {
	return []string{
		"castle-focus", "castle:" + strconv.FormatInt(int64(castleID), 10),
		"building:" + strconv.FormatInt(int64(buildingID), 10),
	}
}

func hasEquippedConstructionItemInSlot(slots []State.ConstructionSlot, catalog *GameData.Catalog, targetSlot int) bool {
	for _, slot := range slots {
		raw, exists := catalog.Find(strconv.FormatInt(int64(slot.DefinitionID), 10))
		if !exists {
			return true
		}
		item, err := GameData.DecodeRecord(raw)
		if err != nil {
			return true
		}
		slotType, exists := item.Int64("slotTypeID")
		if exists && int(slotType) != targetSlot {
			continue
		}
		if GameData.ConstructionItemIsTemporary(item) && slot.RemainingSec != nil && *slot.RemainingSec <= 0 {
			continue
		}
		return true
	}
	return false
}

func castleLabel(castle State.CastleState) string {
	if castle.Name != "" {
		return castle.Name
	}
	return "castle " + strconv.FormatInt(int64(castle.ID), 10)
}

func decodeIntentArguments(raw json.RawMessage, destination any) error {
	if len(raw) == 0 {
		raw = json.RawMessage(`{}`)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return fmt.Errorf("decode intent arguments: %w", err)
	}
	return nil
}

package State

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// This inventory is the commit-B conversion set, against rebased commit A 70bc4ba.
// Existing mutators can have converted read-only source parameters; only that
// source identifier is protected, not the separate mutable receiver.
var gameStateReadOnlyConversions = []struct {
	file, function, identifier string
	index                      int
	receiver                   bool
}{
	{"Automation/Coordinator.go", "Coordinator.cancelRunsForUnavailableSession", "state", 1, false},
	{"Automation/AutoStormTroopCap.go", "PreviewAutoStormTroopCap", "state", 0, false},
	{"Automation/ProductionPolicy.go", "ProductionPolicy.queueCapacity", "state", 0, false},
	{"Automation/ProductionPolicy.go", "ProductionPolicy.targetAmount", "state", 0, false},
	{"Automation/RiftMaidenRunPolicy.go", "RiftMaidenRunPolicy.Active", "state", 0, false},
	{"Automation/AutoInvasionPolicy.go", "activeInvasionAttackCount", "gameState", 0, false},
	{"Automation/AutoInvasionPolicy.go", "activeInvasionTargets", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "activeNomadCampAttackCount", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "activeNomadEventScore", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "activeTowerMovements", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "activeTowerTargetKeys", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "activeTrackedStation", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "autoBirdMovementCastle", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "autoBirdStationActive", "gameState", 0, false},
	{"Automation/AutoBoosterPolicy.go", "autoBoosterRubyBalance", "gameState", 0, false},
	{"Automation/AutoBuyerPolicy.go", "autoBuyerPriceBalance", "gameState", 0, false},
	{"Automation/AutoBuyerPolicy.go", "autoBuyerSourceCastle", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressAgedOwnedPendingRemaining", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressCooldownRemaining", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressInboundDirewolves", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressOwnedPendingRemaining", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressPersonalLockoutSuspected", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressSources", "gameState", 0, false},
	{"Automation/AutoFortressPolicy.go", "autoFortressTargetInFlight", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanCooldownSkipDue", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanDefenseToolBalance", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanDefenseToolShopRoute", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanMainCastle", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanMetrics", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanOutgoingMovementIDs", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "autoKhanTarget", "gameState", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormActiveTargets", "state", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormBuildingTimeSkip", "state", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormCastle", "state", 0, false},
	{"Automation/AutoStormTroopCap.go", "autoStormLegacyAttackDemand", "state", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormMapScanBounds", "_", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormMapStateMatches", "state", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormSelectTimeSkip", "state", 0, false},
	{"Automation/AutoStormPolicy.go", "autoStormTransportTimeSkip", "state", 0, false},
	{"Automation/AutoTowerPolicy.go", "autoTowerBaronAdvisorActive", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "autoTowerCapacityCorrection", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "autoTowerCommanderSupportsMaiden", "gameState", 0, false},
	{"Automation/AutoInvasionPolicy.go", "availableInvasionCandidates", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "availableNomadCommanders", "gameState", 0, false},
	{"Automation/BeriPolicy.go", "beriCastle", "gameState", 0, false},
	{"Automation/BeriAttackPolicy.go", "beriPendingTarget", "gameState", 0, false},
	{"Automation/BeriPolicy.go", "beriSourceCastle", "gameState", 0, false},
	{"Automation/BeriToolPolicy.go", "beriToolCastle", "gameState", 0, false},
	{"Automation/ProductionPolicy.go", "castleSnapshotCurrent", "state", 0, false},
	{"Automation/Coordinator.go", "clearCoinAvailabilityGates", "state", 2, false},
	{"Automation/Coordinator.go", "clearTroopAvailabilityGates", "state", 2, false},
	{"Automation/Coordinator.go", "coinAvailabilityGateChanged", "state", 1, false},
	{"Automation/PolicyHelpers.go", "commanderFeatureCandidates", "gameState", 0, false},
	{"Automation/ConstructionPolicy.go", "constructionShopCastle", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "eligibleAllianceHelpProductionID", "state", 0, false},
	{"Automation/AllianceStationPolicies.go", "expectedAutoBirdReturns", "gameState", 0, false},
	{"Automation/InvasionRecoveryPolicy.go", "firstExhaustedInvasionReservation", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "hasActiveAllianceStationMovement", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "hasAvailableFeatureCommander", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "incomingThreats", "gameState", 0, false},
	{"Automation/AutoInvasionPolicy.go", "invasionCandidatePool", "gameState", 0, false},
	{"Automation/AutoInvasionPolicy.go", "invasionReservationDueForReconciliation", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "kingdomLogisticsRequired", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "kingdomResourceTransportWorkflow", "gameState", 0, false},
	{"Automation/EventAvailability.go", "limitedEventGate", "state", 0, false},
	{"Automation/AutoTowerPolicy.go", "nextAutoTowerCommander", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "nextAvailableFeatureCommander", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "nextGameReportedAutoBirdReturn", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "nomadCampCandidates", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "nomadCampCooldownRemaining", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "oneCommandDungeonSkipCount", "gameState", 0, false},
	{"Automation/OperationalCursors.go", "operationalCursor", "state", 0, false},
	{"Automation/KingdomTransportPolicy.go", "ownedKingdomResourceWorkflows", "gameState", 0, false},
	{"Automation/AutoKhanPolicy.go", "pendingKhanCooldownReports", "gameState", 0, false},
	{"Automation/PolicyHelpers.go", "pendingKingdomResourceTransport", "gameState", 0, false},
	{"Automation/AutoNomadPolicy.go", "pendingNomadCampRefresh", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "pendingTowerCooldownRefresh", "gameState", 0, false},
	{"Automation/Coordinator.go", "policyEnabled", "state", 2, false},
	{"Automation/ProductionPolicy.go", "productionVIPQueueCapacity", "state", 0, false},
	{"Automation/ProductionPolicy.go", "recruitmentStackAmount", "state", 0, false},
	{"Automation/ProductionPolicy.go", "recruitmentSubscriptionStackBonus", "state", 0, false},
	{"Automation/AutoNomadPolicy.go", "responseGatedDungeonCooldownCount", "gameState", 0, false},
	{"Automation/AutoTowerPolicy.go", "towerCooldownRefreshPending", "gameState", 0, false},
	{"Automation/AllianceStationPolicies.go", "trackedStationMovement", "gameState", 0, false},
	{"Automation/Coordinator.go", "troopAvailabilityGateInventoryChanged", "state", 1, false},
	{"Automation/BeriAttackPolicy.go", "unreflectedBeriTowerLaunches", "gameState", 0, false},
	{"Ingest/EventActivityReducers.go", "InvasionReservationReportCandidate", "gameState", 0, false},
	{"Ingest/AdvisorReducers.go", "activeAdvisorEvent", "gameState", 0, false},
	{"Ingest/AdvisorReducers.go", "advisorEventEndsAt", "gameState", 0, false},
	{"Ingest/EventRankingReducers.go", "rankingEventID", "gameState", 0, false},
	{"Ingest/ScopedPartitions.go", "scopedCastleDomainPartitions", "gameState", 0, false},
	{"Ingest/ScopedPartitions.go", "scopedMapKingdoms", "gameState", 1, false},
	{"Ingest/ScopedPartitions.go", "scopedPartitionsForFrame", "gameState", 1, false},
	{"State/Partitions.go", "AccountPartition", "state", 0, false},
	{"State/Partitions.go", "AccountScope", "state", 0, false},
	{"State/InvasionTargets.go", "AnyActiveMovementAtMapTarget", "gameState", 0, false},
	{"State/AttackAnalytics.go", "AttackFeatureTargetPendingAt", "gameState", 0, false},
	{"State/AttackTargetRejections.go", "AttackTargetRejectedAt", "gameState", 0, false},
	{"State/MarketBarrowLeases.go", "AvailableMarketBarrowsAt", "gameState", 0, false},
	{"State/Partitions.go", "BoundAccount", "state", 0, false},
	{"State/KingdomAccess.go", "CastleFocusKnownUnavailable", "gameState", 0, false},
	{"State/Partitions.go", "CastlePartition", "state", 0, false},
	{"State/Partitions.go", "CastleScope", "state", 0, false},
	{"State/CommanderAvailability.go", "CommanderHasActiveMovementAt", "gameState", 0, false},
	{"State/EventScores.go", "GameState.ActiveScalableEventScoreReached", "state", -1, true},
	{"State/EventScores.go", "GameState.ActiveScalableEventScore", "state", -1, true},
	{"State/EventScores.go", "GameState.ActiveShopForPackage", "state", -1, true},
	{"State/EventScores.go", "GameState.AnyEventAvailable", "state", -1, true},
	{"State/AutoBirdControls.go", "GameState.AutoBirdControl", "state", -1, true},
	{"State/AutoBirdControls.go", "GameState.AutoBirdPaused", "state", -1, true},
	{"State/InventoryMutation.go", "GameState.ConstructionOffersFor", "state", -1, true},
	{"State/EventScores.go", "GameState.EventAvailable", "state", -1, true},
	{"State/KhanRage.go", "GameState.KhanDefenseLaunchesForOccurrence", "state", -1, true},
	{"State/ReportMutation.go", "GameState.LookupBattleReportCapture", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.LookupEventActivity", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.LookupEventOccurrence", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.LookupEventRanking", "state", -1, true},
	{"State/WorldMap.go", "GameState.LookupMapObservation", "state", -1, true},
	{"State/MovementMutation.go", "GameState.LookupMovement", "state", -1, true},
	{"State/ReportMutation.go", "GameState.LookupReportNotice", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.LookupScalableEventScore", "state", -1, true},
	{"State/ReportMutation.go", "GameState.LookupSpyReportCapture", "state", -1, true},
	{"State/StormMutation.go", "GameState.LookupStormTarget", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.LookupTowerCooldown", "state", -1, true},
	{"State/WorldMap.go", "GameState.MapKingdomIDs", "state", -1, true},
	{"State/MovementMutation.go", "GameState.MovementCount", "state", -1, true},
	{"State/MovementMutation.go", "GameState.MovementViewMap", "state", -1, true},
	{"State/ReportMutation.go", "GameState.RangeBattleReportCaptures", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.RangeEventActivities", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.RangeEventRankings", "state", -1, true},
	{"State/WorldMap.go", "GameState.RangeMapObservationsByKind", "state", -1, true},
	{"State/WorldMap.go", "GameState.RangeMapObservations", "state", -1, true},
	{"State/MovementMutation.go", "GameState.RangeMovements", "state", -1, true},
	{"State/ReportMutation.go", "GameState.RangeReportNotices", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.RangeScalableEventScores", "state", -1, true},
	{"State/ReportMutation.go", "GameState.RangeSpyReportCaptures", "state", -1, true},
	{"State/StormMutation.go", "GameState.RangeStormMapObservations", "state", -1, true},
	{"State/StormMutation.go", "GameState.RangeStormTargets", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.RangeTowerCooldowns", "state", -1, true},
	{"State/EventScores.go", "GameState.ScalableEventScoreReached", "state", -1, true},
	{"State/SharedStormScan.go", "GameState.SharedStormScanCoverage", "state", -1, true},
	{"State/StormMutation.go", "GameState.StormTargetCount", "state", -1, true},
	{"State/Models.go", "GameState.SubscriptionScope", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.TowerCooldownCount", "state", -1, true},
	{"State/ClientProjection.go", "GameState.clientMapProjection", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.eventScoreChangeIDs", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.eventScoreIDs", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.eventScoreRecord", "state", -1, true},
	{"State/MapOverlay.go", "GameState.hasPrivateMapObservations", "state", -1, true},
	{"State/StormMutation.go", "GameState.hasStormTargetKey", "state", -1, true},
	{"State/MapOverlay.go", "GameState.lookupPrivateMapObservation", "state", -1, true},
	{"State/WorldMap.go", "GameState.mapChanges", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.materializedEventScores", "state", -1, true},
	{"State/WorldMap.go", "GameState.materializedMap", "state", -1, true},
	{"State/MovementMutation.go", "GameState.materializedMovements", "state", -1, true},
	{"State/MapOverlay.go", "GameState.materializedPrivateMap", "state", -1, true},
	{"State/ReportMutation.go", "GameState.materializedReports", "state", -1, true},
	{"State/StormMutation.go", "GameState.materializedStormTargets", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.materializedTowerCooldowns", "state", -1, true},
	{"State/MovementMutation.go", "GameState.movementChangeIDs", "state", -1, true},
	{"State/MovementMutation.go", "GameState.movementViewMap", "state", -1, true},
	{"State/AttackAnalyticsMutation.go", "GameState.prepareAttackAnalyticsMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareAttackDialogMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareAttackPresetMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareAutomationMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareCastellanMutation", "source", 0, false},
	{"State/CastleMutation.go", "GameState.prepareCastleMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareCommanderMutation", "source", 0, false},
	{"State/EventScoreMutation.go", "GameState.prepareEventScoreMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareGeneralMutation", "source", 0, false},
	{"State/InventoryMutation.go", "GameState.prepareInventoryMutation", "source", 0, false},
	{"State/MapOverlay.go", "GameState.prepareMapMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareMarketMutation", "source", 0, false},
	{"State/MovementMutation.go", "GameState.prepareMovementMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareObservationMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.preparePlayerMutation", "source", 0, false},
	{"State/ReportMutation.go", "GameState.prepareReportMutation", "source", 0, false},
	{"State/SelectiveMutation.go", "GameState.prepareStationingMutation", "source", 0, false},
	{"State/StormMutation.go", "GameState.prepareStormMutation", "source", 0, false},
	{"State/TowerCooldownMutation.go", "GameState.prepareTowerCooldownMutation", "source", 0, false},
	{"State/TowerQueueMutation.go", "GameState.prepareTowerQueueMutation", "source", 0, false},
	{"State/MapOverlay.go", "GameState.privateMapKingdomIDs", "state", -1, true},
	{"State/MapOverlay.go", "GameState.privateMapShard", "state", -1, true},
	{"State/EventScoreMutation.go", "GameState.rangeEventScoreRecords", "state", -1, true},
	{"State/MapOverlay.go", "GameState.rangePrivateMapObservationsByKind", "state", -1, true},
	{"State/MapOverlay.go", "GameState.rangePrivateMapObservations", "state", -1, true},
	{"State/MapOverlay.go", "GameState.rangePrivateMapShards", "state", -1, true},
	{"State/ReportMutation.go", "GameState.rangeReportRecords", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.rangeTowerCooldownShards", "state", -1, true},
	{"State/ReportMutation.go", "GameState.reportMessageChangeIDs", "state", -1, true},
	{"State/ReportMutation.go", "GameState.reportMessageIDs", "state", -1, true},
	{"State/ReportMutation.go", "GameState.reportRecord", "state", -1, true},
	{"State/StormMutation.go", "GameState.stormTargetChangeKeys", "state", -1, true},
	{"State/StormMutation.go", "GameState.stormTargetKeys", "state", -1, true},
	{"State/StormMutation.go", "GameState.stormTargetSuppressed", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.towerCooldownChangeKeys", "state", -1, true},
	{"State/TowerCooldownMutation.go", "GameState.towerCooldownShard", "state", -1, true},
	{"State/TowerQueueMutation.go", "GameState.towerQueueCastleIDs", "state", -1, true},
	{"State/TowerQueueMutation.go", "GameState.towerQueueChangeCastleIDs", "state", -1, true},
	{"State/PlayerAttacks.go", "HasIncomingPlayerAttack", "gameState", 0, false},
	{"State/Models.go", "HasOutstandingHospitalAllianceHelpRequest", "state", 0, false},
	{"State/Models.go", "HasOutstandingRecruitmentAllianceHelpRequest", "state", 0, false},
	{"State/InvasionTargets.go", "InvasionCommanderReserved", "gameState", 0, false},
	{"State/InvasionTargets.go", "InvasionReservationMovement", "gameState", 0, false},
	{"State/PlayerAttacks.go", "IsIncomingPlayerAttack", "gameState", 0, false},
	{"State/PlayerAttacks.go", "IsOutgoingPlayerAttack", "gameState", 0, false},
	{"State/StationingLeases.go", "KhanAutoStationYieldActiveAt", "gameState", 0, false},
	{"State/Partitions.go", "KingdomPartition", "state", 0, false},
	{"State/Partitions.go", "KingdomScope", "state", 0, false},
	{"State/MarketBarrowLeases.go", "MarketBarrowLeaseAt", "gameState", 0, false},
	{"State/CommanderAvailability.go", "MovementOwnedByCurrentPlayer", "gameState", 0, false},
	{"State/ClientProjection.go", "NewClientStateSnapshot", "state", 0, false},
	{"State/Store.go", "NewStoreWithWorldMap", "initial", 0, false},
	{"State/Store.go", "NewStore", "initial", 0, false},
	{"State/MarketBarrowLeases.go", "MarketBarrowSourceStatusAt", "gameState", 0, false},
	{"State/NomadSequentialArrival.go", "NomadSequentialArrivalBlockAt", "gameState", 0, false},
	{"State/Models.go", "OutstandingHospitalAllianceHelpRequests", "state", 0, false},
	{"State/Models.go", "OwnAllianceHelpListCurrent", "state", 0, false},
	{"State/Models.go", "OwnAllianceHelpStateCurrent", "state", 0, false},
	{"State/CommandContextRequests.go", "PackageCountersAfterLastPurchase", "gameState", 0, false},
	{"State/CommandContextRequests.go", "PendingCommandRequests", "state", 0, false},
	{"State/Models.go", "PendingOtherAllianceHelpListIDs", "state", 0, false},
	{"State/ProductionQueue.go", "ProductionQueueNeedsRefresh", "state", 0, false},
	{"State/Models.go", "RecruitmentAllianceHelpCovers", "state", 0, false},
	{"State/CommandContextRequests.go", "RecruitmentAllianceHelpItemEligible", "state", 0, false},
	{"State/CommandContextRequests.go", "RecruitmentAllianceHelpRejected", "state", 0, false},
	{"State/Partitions.go", "SessionPartition", "state", 0, false},
	{"State/Partitions.go", "SessionScope", "state", 0, false},
	{"State/MovementMutation.go", "StationingOperation.ActiveInState", "state", 0, false},
	{"State/AttackAnalytics.go", "TowerAdvisorTimeSkipsUsedSince", "gameState", 0, false},
	{"State/StationingLeases.go", "TrackedStationMovementActiveAt", "gameState", 0, false},
	{"State/StationingLeases.go", "TrackedStationMovementReleaseAt", "gameState", 0, false},
	{"State/WorldMap.go", "accountCanAccessSharedKingdom", "state", 0, false},
	{"State/ClientProjection.go", "clientEventScores", "state", 0, false},
	{"State/ClientProjection.go", "clientReports", "state", 0, false},
	{"State/Partitions.go", "defaultPartitionKeys", "state", 0, false},
	{"State/WorldMap.go", "gameStateWorldID", "state", 0, false},
	{"State/Partitions.go", "initialProtocolContext", "state", 0, false},
	{"State/InvasionTargets.go", "invasionMovementCastleEndpointMatches", "gameState", 0, false},
	{"State/ComponentPersistence.go", "loadStormComponent", "state", 3, false},
	{"State/ClientProjection.go", "newClientStormState", "state", 0, false},
	{"State/Partitions.go", "nextProtocolContext", "state", 1, false},
	{"State/NomadSequentialArrival.go", "nomadPendingLaunchSettled", "gameState", 0, false},
	{"State/MapRetention.go", "privateMapRetentionCandidates", "state", 0, false},
	{"State/CommandContextRequests.go", "pruneRecruitmentHelpIneligibility", "state", 1, false},
	{"State/CommandContextRequests.go", "recruitmentQueueContainsAny", "state", 0, false},
}

// Pointer-receiver GameState methods at commit A, excluding read-only ChangeIDs
// and ChangeKeys helpers. Never infer purity from a new method's name.
var baseGameStateMutators = []string{
	"DeleteBattleReportCapture",
	"DeleteCastle",
	"DeleteEventActivity",
	"DeleteEventRanking",
	"DeleteInventoryEquipment",
	"DeleteInventoryGem",
	"DeleteMapObservation",
	"DeleteMovement",
	"DeleteReportNotice",
	"DeleteScalableEventScore",
	"DeleteSpyReportCapture",
	"DeleteStormTarget",
	"DeleteTowerCooldown",
	"DropPendingCommandRequestsBefore",
	"IncrementTowerQueueConfirmedLaunches",
	"InvalidateInventoryItemsCollectionObservation",
	"MarkInventoryEquipmentMutated",
	"MarkInventoryPackagePurchaseDispatched",
	"MutableAttackAnalyticsLaunchIDs",
	"MutableCastle",
	"MutableCastleParts",
	"MutableEventActivity",
	"MutableEventRanking",
	"MutableInventoryConstructionItems",
	"MutableInventoryConstructionOffers",
	"MutableInventoryEquipment",
	"MutableInventoryGemStacks",
	"MutableInventoryGems",
	"MutableInventoryItems",
	"MutablePendingAttackAnalytics",
	"MutableRecentAutoStormLaunches",
	"MutableRecentTowerAdvisorTimeSkips",
	"MutableStormIslandReturns",
	"MutableStormLastScannedAt",
	"MutableTowerQueueEntries",
	"RecordPendingCommandRequest",
	"RefreshStormTargetObservation",
	"ReplaceCastles",
	"ReplaceEventInventory",
	"ReplaceEventScores",
	"ReplaceEventShopRoutes",
	"ReplaceInventoryConstructionItems",
	"ReplaceInventoryConstructionOffers",
	"ReplaceInventoryEquipment",
	"ReplaceInventoryGemStacks",
	"ReplaceInventoryGems",
	"ReplaceMapState",
	"ReplaceMovements",
	"ReplaceReports",
	"ReplaceStormMap",
	"ReplaceTowerCooldowns",
	"ReplaceTowerQueue",
	"SetActiveEventID",
	"SetAttackAnalyticsLaunchIDs",
	"SetAttackTargetRejections",
	"SetBattleReportCapture",
	"SetCastle",
	"SetCastleParts",
	"SetEventActivity",
	"SetEventRanking",
	"SetInventoryConstructionSpaceLeft",
	"SetInventoryEquipment",
	"SetInventoryGem",
	"SetInventoryItemsCollection",
	"SetInventoryItemsCollectionObserved",
	"SetMapObservation",
	"SetMovement",
	"SetPendingAttackAnalytics",
	"SetRecentAutoStormLaunches",
	"SetRecentTowerAdvisorTimeSkips",
	"SetReportNotice",
	"SetScalableEventScore",
	"SetSpyReportCapture",
	"SetStormTarget",
	"SetTowerCooldown",
	"SetTowerQueueCapacity",
	"SetTowerQueueEntries",
	"SetTowerQueueLastAttemptedAt",
	"SetTowerQueueLastScannedAt",
	"TakePendingCommandRequest",
	"UnmarshalJSON",
	"castleChangeParts",
	"compactMapOverlay",
	"initializeEventScores",
	"initializeMovements",
	"initializeReports",
	"initializeStormTargets",
	"initializeTowerCooldowns",
	"markEventScore",
	"markMovement",
	"markReportMessage",
	"markTowerQueueCastle",
	"mutableEventScoreRecord",
	"mutableMapShard",
	"mutableMovementShard",
	"mutableReportRecord",
	"mutableStormTargetShard",
	"mutableTowerCooldownShard",
	"ownedInventoryEquipment",
	"ownedInventoryGems",
	"ownedInventoryItems",
	"prepareAttackAnalyticsMutation",
	"prepareAttackDialogMutation",
	"prepareAttackPresetMutation",
	"prepareAutomationMutation",
	"prepareCastellanMutation",
	"prepareCastleMutation",
	"prepareCommanderMutation",
	"prepareEventScoreMutation",
	"prepareGeneralMutation",
	"prepareInventoryMutation",
	"prepareMapMutation",
	"prepareMarketMutation",
	"prepareMovementMutation",
	"prepareObservationMutation",
	"preparePlayerMutation",
	"prepareReportMutation",
	"prepareStationingMutation",
	"prepareStormMutation",
	"prepareTowerCooldownMutation",
	"prepareTowerQueueMutation",
	"recordMapChange",
	"requireMapWrite",
	"storeEventScoreRecord",
	"storeReportRecord",
}

func gameStateGuardRoot(expression ast.Expr) *ast.Ident {
	switch expression := expression.(type) {
	case *ast.Ident:
		return expression
	case *ast.SelectorExpr:
		return gameStateGuardRoot(expression.X)
	case *ast.IndexExpr:
		return gameStateGuardRoot(expression.X)
	case *ast.StarExpr:
		return gameStateGuardRoot(expression.X)
	case *ast.ParenExpr:
		return gameStateGuardRoot(expression.X)
	}
	return nil
}

func gameStateGuardReceiver(expression ast.Expr) (string, bool) {
	pointer := false
	if star, ok := expression.(*ast.StarExpr); ok {
		expression, pointer = star.X, true
	}
	if name, ok := expression.(*ast.Ident); ok {
		return name.Name, pointer
	}
	return "", pointer
}

func TestGameStateReadOnlyAccessors(t *testing.T) {
	functions := map[string]*ast.FuncDecl{}
	var valueReceivers []string
	for _, directory := range []string{"State", "Ingest", "Automation"} {
		packages, err := parser.ParseDir(token.NewFileSet(), filepath.Join("..", directory), func(info fs.FileInfo) bool { return !strings.HasSuffix(info.Name(), "_test.go") }, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, pkg := range packages {
			for name, file := range pkg.Files {
				for _, declaration := range file.Decls {
					function, ok := declaration.(*ast.FuncDecl)
					if !ok {
						continue
					}
					key := function.Name.Name
					if function.Recv != nil {
						receiver, pointer := gameStateGuardReceiver(function.Recv.List[0].Type)
						key = receiver + "." + key
						if directory == "State" && receiver == "GameState" && !pointer {
							valueReceivers = append(valueReceivers, function.Name.Name)
						}
					}
					functions[directory+"/"+filepath.Base(name)+":"+key] = function
				}
			}
		}
	}
	slices.Sort(valueReceivers)
	if !reflect.DeepEqual(valueReceivers, []string{"MarshalJSON", "clientStateProjection"}) {
		t.Fatalf("value receivers = %v", valueReceivers)
	}
	mutators := map[string]bool{}
	for _, name := range baseGameStateMutators {
		mutators[name] = true
	}
	for _, conversion := range gameStateReadOnlyConversions {
		t.Run(conversion.file+":"+conversion.function+":"+conversion.identifier, func(t *testing.T) {
			function := functions[conversion.file+":"+conversion.function]
			if function == nil {
				t.Fatal("converted function missing")
			}
			var identifier *ast.Ident
			index := 0
			fields := function.Type.Params.List
			if conversion.receiver {
				fields = function.Recv.List
			}
			for _, field := range fields {
				for _, name := range field.Names {
					if conversion.receiver || index == conversion.index {
						identifier = name
						pointer, ok := field.Type.(*ast.StarExpr)
						if !ok {
							t.Fatal("converted identifier is not a pointer")
						}
						typeName := ""
						switch element := pointer.X.(type) {
						case *ast.Ident:
							typeName = element.Name
						case *ast.SelectorExpr:
							typeName = element.Sel.Name
						}
						if typeName != "GameState" {
							t.Fatal("converted type is not GameState")
						}
					}
					index++
				}
			}
			if identifier == nil || identifier.Name != conversion.identifier {
				t.Fatal("converted identifier missing")
			}
			for _, violation := range gameStateReadOnlyViolations(function.Body, identifier, mutators) {
				t.Error(violation)
			}
		})
	}
}

// Shared by the production-source guard and its mutation regression fixtures.
func gameStateReadOnlyViolations(body *ast.BlockStmt, identifier *ast.Ident, mutators map[string]bool) []string {
	var violations []string
	rooted := func(expression ast.Expr) bool {
		root := gameStateGuardRoot(expression)
		return root != nil && root.Obj == identifier.Obj && root.Name == identifier.Name
	}
	// Reject aliases at creation rather than attempting incomplete flow tracking.
	// Scalar field reads and explicit value copies remain allowed; passing the
	// pointer directly to a read-only call does not create a local alias.
	alias := func(expression ast.Expr) bool {
		for {
			parenthesized, ok := expression.(*ast.ParenExpr)
			if !ok {
				break
			}
			expression = parenthesized.X
		}
		if _, ok := expression.(*ast.Ident); ok {
			return rooted(expression)
		}
		address, ok := expression.(*ast.UnaryExpr)
		return ok && address.Op == token.AND && rooted(address.X)
	}
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.AssignStmt:
			for _, left := range node.Lhs {
				if rooted(left) {
					violations = append(violations, "assignment through read-only identifier")
				}
			}
			for index, right := range node.Rhs {
				if index < len(node.Lhs) {
					if name, ok := node.Lhs[index].(*ast.Ident); ok && name.Name == "_" {
						continue
					}
				}
				if alias(right) {
					violations = append(violations, "creates pointer alias of read-only identifier")
				}
			}
		case *ast.ValueSpec:
			for index, value := range node.Values {
				if index < len(node.Names) && node.Names[index].Name == "_" {
					continue
				}
				if alias(value) {
					violations = append(violations, "creates pointer alias of read-only identifier")
				}
			}
		case *ast.IncDecStmt:
			if rooted(node.X) {
				violations = append(violations, "increment through read-only identifier")
			}
		case *ast.CallExpr:
			if method, ok := node.Fun.(*ast.SelectorExpr); ok && rooted(method.X) && mutators[method.Sel.Name] {
				violations = append(violations, "calls base mutator "+method.Sel.Name)
			}
			if builtin, ok := node.Fun.(*ast.Ident); ok && builtin.Obj == nil && (builtin.Name == "delete" || builtin.Name == "clear") && len(node.Args) > 0 && rooted(node.Args[0]) {
				violations = append(violations, "builtin mutation through read-only identifier: "+builtin.Name)
			}
		case *ast.GoStmt:
			ast.Inspect(node, func(node ast.Node) bool {
				if name, ok := node.(*ast.Ident); ok && name.Obj == identifier.Obj && name.Name == identifier.Name {
					violations = append(violations, "goroutine retains read-only identifier")
				}
				return true
			})
		}
		return true
	})
	return violations
}

func TestGameStateReadOnlyGuardMutations(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"delete", "delete(gameState.Castles, 1)", "builtin mutation"},
		{"clear", "clear(gameState.Castles)", "builtin mutation"},
		{"alias", `alias := gameState; alias.Player.Name = "x"`, "pointer alias"},
		{"address", `p := &gameState.Player; p.Name = "x"`, "pointer alias"},
		{"declared alias", `var alias = gameState; alias.Player.Name = "x"`, "pointer alias"},
		{"assigned alias", `var alias *GameState; alias = (gameState); alias.Player.Name = "x"`, "pointer alias"},
		{"declared address", `var p = &(gameState.Player); p.Name = "x"`, "pointer alias"},
		{"parenthesized address", `p := (&gameState.Player); p.Name = "x"`, "pointer alias"},
		{"nested clear", "clear((gameState.Castles))", "builtin mutation"},
		{"field write", `gameState.Player.Name = "x"`, "assignment"},
		{"map write", "gameState.Castles[1] = Castle{}", "assignment"},
		{"increment", "gameState.Player.ID++", "increment"},
		{"mutator", "gameState.DeleteCastle(1)", "base mutator"},
		{"goroutine", "go func(){ _ = gameState }()", "goroutine"},
		{"read-only call", "MovementOwnedByCurrentPlayer(gameState, movement)", ""},
		{"discard pointer", "_ = gameState", ""},
		{"read-only method", "gameState.LookupMovement(1)", ""},
		{"shadowed builtin", "delete := func(any, int) {}; delete(gameState.Castles, 1)", ""},
		{"field read", "name := gameState.Player.Name; _ = name", ""},
		{"owned copy", `owned := *gameState; owned.Player.Name = "x"`, ""},
		{"shadowed identifier", "{ gameState := &GameState{}; gameState.Player.Name = \"x\" }", ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "guard_fixture.go", "package State; func check(gameState *GameState) {"+test.body+"}", 0)
			if err != nil {
				t.Fatal(err)
			}
			function := file.Decls[0].(*ast.FuncDecl)
			violations := gameStateReadOnlyViolations(function.Body, function.Type.Params.List[0].Names[0], map[string]bool{"DeleteCastle": true})
			if test.want == "" {
				if len(violations) != 0 {
					t.Fatalf("read-only fixture rejected: %v", violations)
				}
				return
			}
			for _, violation := range violations {
				if strings.Contains(violation, test.want) {
					return
				}
			}
			t.Fatalf("mutation not rejected with %q: %v", test.want, violations)
		})
	}
}

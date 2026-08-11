export type APIConnectionStatus = 'Disconnected' | 'Connecting' | 'Connected';

export interface ApplicationUpdateV2 {
	currentVersion: string;
	latestVersion?: string;
	available: boolean;
	downloadUrl?: string;
	expectedSha256?: string;
	installSupported: boolean;
	status: 'idle' | 'checking' | 'current' | 'available' | 'downloading' | 'installing' | 'restart-required' | 'error' | 'unavailable';
	stage?: string;
	progress: number;
	error?: string;
	checkedAt?: string;
	restartRequired: boolean;
}

export interface RuntimeDiagnosticsV2 {
	applicationMemoryMb: number;
	browserMemoryMb: number;
	observedAt: string;
}

export interface APIEnvelope<T = unknown> {
  v: 2;
  id?: string;
  type: string;
  revision?: number;
  sequence?: number;
  gap?: boolean;
  payload?: T;
}

export interface SessionStateV2 {
  mode?: 'full' | 'background';
  generation: number;
  baselineGeneration: number;
  connectionGeneration: number;
  status: string;
  loggedIn: boolean;
  socketReady: boolean;
  browserId?: string;
  browserName?: string;
  serverUrl?: string;
  namespace?: string;
  detail?: string;
	cooldownUntil?: string;
	retryAt?: string;
  changedAt: string;
}

export interface AccountBindingStateV2 {
  uid?: number;
  worldId?: string;
  playerId?: number;
  boundAt?: string;
}

export interface BrowserCandidate {
  id: string;
  name: string;
  executablePath: string;
  isDefault?: boolean;
}

export interface BrowserInventory {
  selected: BrowserCandidate | null;
  current: BrowserCandidate | null;
  available: BrowserCandidate[];
  restartRequired: boolean;
  selectionIntent: 'session.select_browser';
}

export interface ConfigurationSnapshot {
	schemaVersion: number;
	revision: number;
	updatedAt: string;
	sections: Record<string, unknown>;
}

export interface SettingsBundleV1 {
	format: 'citadelops-settings';
	formatVersion: 1;
	exportedAt: string;
	appVersion?: string;
	configuration: {
		schemaVersion: number;
		sections: Record<string, unknown>;
	};
	clientPreferences?: Record<string, string>;
}

export interface SettingsImportResult {
	importedSections: number;
	changedSections: number;
	includedClientPreferences: number;
	revision: number;
	updatedAt: string;
}

export interface WorldIntelligenceStatusV1 {
	enabled: boolean;
	contributing: boolean;
	worldId?: string;
	endpoint: string;
	pendingBatches: number;
	lastCapturedAt?: string;
	lastUploadAt?: string;
	lastUploadError?: string;
	collector: boolean;
	collectorPlayerId?: number;
	collectorSlot?: number;
	collectorSlots?: number;
	collectionMode: 'official-catalog';
	pendingCatalogs?: number;
	catalogVersion?: string;
	catalogDatasets?: number;
	lastCatalogAt?: string;
	nextCatalogAt?: string;
	lastCatalogError?: string;
	catalogCollectionInProgress: boolean;
	lastScanAt?: string;
	nextScanAt?: string;
	lastScanError?: string;
	scanInProgress: boolean;
	scannedPlayers?: number;
	publicFieldsOnly: boolean;
	officialSourceOnly: boolean;
}

export interface WorldIntelligenceCatalogDatasetSummaryV1 {
	datasetKey: string;
	datasetLabel: string;
	category: string;
	source: 'ggs-official-items';
	sourceVersion: string;
	sourceUrl: string;
	datasetDigest: string;
	fields: string[];
	rowCount: number;
	capturedAt: string;
	contributorCount: number;
}

export interface WorldIntelligenceCatalogDatasetCatalogV1 {
	source: 'ggs-official-items';
	datasets: WorldIntelligenceCatalogDatasetSummaryV1[];
}

export interface WorldIntelligenceCatalogDatasetV1 extends WorldIntelligenceCatalogDatasetSummaryV1 {
	rows: Array<Record<string, unknown>>;
	history: WorldIntelligenceCatalogDatasetSummaryV1[];
}

export interface WorldIntelligencePublicMetricV1 {
	label: string;
	value: number;
	rank?: number;
	unit?: string;
	source?: 'gge-highscore' | 'gge-player-event';
	listType?: number;
	leagueId?: number;
	eventId?: number;
	metricId?: number;
	observedAt: string;
	validUntil?: string;
}

export interface WorldIntelligencePlayerObservationV1 {
	worldId: string;
	playerId: number;
	name: string;
	allianceId?: number;
	allianceName?: string;
	level?: number;
	legendLevel?: number;
	might?: number;
	glory?: number;
	weeklyLoot?: number;
	honor?: number;
	publicProfile?: {
		achievementPoints?: number;
		highestGlory?: number;
		allianceRank?: number;
		titlePrefixId?: number;
		titleSuffixId?: number;
		titleId?: number;
		bestRank?: number;
		ruined?: boolean;
	};
	publicMetrics?: Record<string, WorldIntelligencePublicMetricV1>;
	source: 'account' | 'alliance' | 'event-ranking' | 'leaderboard';
	observedAt: string;
}

export interface WorldIntelligenceAllianceObservationV1 {
	worldId: string;
	allianceId: number;
	name: string;
	memberCount?: number;
	totalMight?: number;
	publicMetrics?: Record<string, WorldIntelligencePublicMetricV1>;
	source: 'alliance' | 'event-ranking' | 'leaderboard';
	observedAt: string;
}

export interface WorldIntelligenceHoldingObservationV1 {
	worldId: string;
	allianceId: number;
	playerId: number;
	castleId: number;
	kingdomId: number;
	x: number;
	y: number;
	slotType: number;
	observedAt: string;
}

export interface WorldIntelligenceSearchResultV1 {
	type: 'player' | 'alliance';
	worldId: string;
	id: number;
	name: string;
	allianceId?: number;
	allianceName?: string;
	level?: number;
	legendLevel?: number;
	might?: number;
	glory?: number;
	weeklyLoot?: number;
	honor?: number;
	publicProfile?: WorldIntelligencePlayerObservationV1['publicProfile'];
	publicMetrics?: Record<string, WorldIntelligencePublicMetricV1>;
	memberCount?: number;
	lastObservedAt: string;
}

export interface WorldIntelligenceSearchResponseV1 {
	worldId: string;
	query: string;
	results: WorldIntelligenceSearchResultV1[];
}

export interface WorldIntelligencePlayerProfileV1 {
	current: WorldIntelligencePlayerObservationV1;
	history: WorldIntelligencePlayerObservationV1[];
}

export interface WorldIntelligenceAllianceProfileV1 {
	current: WorldIntelligenceAllianceObservationV1;
	history: WorldIntelligenceAllianceObservationV1[];
	members: WorldIntelligencePlayerObservationV1[];
	holdings: WorldIntelligenceHoldingObservationV1[];
}

export interface WorldIntelligenceRankingEntryV1 {
	rank: number;
	type: 'player' | 'alliance';
	worldId: string;
	id: number;
	name: string;
	allianceId?: number;
	allianceName?: string;
	level?: number;
	legendLevel?: number;
	might?: number;
	glory?: number;
	weeklyLoot?: number;
	honor?: number;
	memberCount?: number;
	value: number;
	lastObservedAt: string;
}

export interface WorldIntelligenceRankingResponseV1 {
	worldId: string;
	type: 'players' | 'alliances';
	metric: string;
	entries: WorldIntelligenceRankingEntryV1[];
}

export interface WorldIntelligenceRankingMetricV1 {
	metric: string;
	label: string;
	unit?: string;
	source: 'public-ranking' | 'public-profile' | 'gge-highscore' | 'gge-player-event';
	populatedRows: number;
	latestObservedAt?: string;
}

export interface WorldIntelligenceRankingMetricCatalogV1 {
	worldId: string;
	type: 'players' | 'alliances';
	metrics: WorldIntelligenceRankingMetricV1[];
}

export interface WorldIntelligenceEventScoreObservationV1 {
	worldId: string;
	occurrenceId: string;
	eventId: number;
	eventKey: string;
	eventName: string;
	listType: number;
	leagueId: number;
	boardKey?: string;
	playerId: number;
	playerName: string;
	allianceId?: number;
	allianceName?: string;
	rank: number;
	score?: number;
	scoreKnown: boolean;
	scoreUnit: string;
	runStartedOn: string;
	eventEndsAt: string;
	source: 'gge-highscore';
	observedAt: string;
}

export interface WorldIntelligenceEventRunV1 {
	occurrenceId: string;
	worldId: string;
	eventId: number;
	eventKey: string;
	eventName: string;
	scoreUnit: string;
	runStartedOn: string;
	eventEndsAt: string;
	firstObservedAt: string;
	lastObservedAt: string;
	participants: number;
	scoreRows: number;
}

export interface WorldIntelligenceEventRunListV1 {
	worldId: string;
	eventKey?: string;
	runs: WorldIntelligenceEventRunV1[];
}

export interface WorldIntelligenceEventRunRankingV1 {
	run: WorldIntelligenceEventRunV1;
	entries: WorldIntelligenceEventScoreObservationV1[];
}

export interface WorldIntelligencePlayerEventScoreHistoryV1 {
	worldId: string;
	playerId: number;
	eventKey?: string;
	occurrenceId?: string;
	history: WorldIntelligenceEventScoreObservationV1[];
}

export interface WorldIntelligenceWorldCoverageV1 {
	worldId: string;
	players: number;
	alliances: number;
	holdings: number;
	eventRuns: number;
	eventScores: number;
	observationCount: number;
	firstObservedAt?: string;
	lastObservedAt?: string;
}

export interface WorldIntelligenceCoverageResponseV1 {
	worlds: WorldIntelligenceWorldCoverageV1[];
}

export interface BattleResearchPhasePredictionV2 {
	winner: 'attacker' | 'defender';
	attackerStarted: number;
	defenderStarted: number;
	attackerPower: number;
	defenderPower: number;
	attackerLost: number;
	defenderLost: number;
	attackerSurvivors: number;
	defenderSurvivors: number;
}

export interface BattleResearchWavePredictionV2 {
	wave: number;
	left: BattleResearchPhasePredictionV2;
	center: BattleResearchPhasePredictionV2;
	right: BattleResearchPhasePredictionV2;
}

export interface BattleResearchPredictionV2 {
	modelVersion: string;
	generatedAt: string;
	predictedResult: 'Victory' | 'Defeat';
	attackWinProbability: number;
	confidence: string;
	attackerSent: number;
	defenderObserved: number;
	expectedAttackerLost: number;
	expectedDefenderLost: number;
	expectedAttackerSurvivors: number;
	expectedDefenderSurvivors: number;
	unitStatCoverage: number;
	attackerMeleeBonusPercent?: number;
	attackerRangeBonusPercent?: number;
	wallReductionPercent?: number;
	gateReductionPercent?: number;
	moatReductionPercent?: number;
	waves: BattleResearchWavePredictionV2[];
	courtyard: BattleResearchPhasePredictionV2;
	considered: string[];
	recordedNotModeled: string[];
	assumptions: string[];
}

export interface BattleResearchTrialSummaryV2 {
	id: string;
	phase: string;
	movementID?: number;
	targetX: number;
	targetY: number;
	kingdomID: number;
	arrivesAt?: string;
	createdAt: string;
	updatedAt: string;
	prediction?: BattleResearchPredictionV2;
	actualResult?: string;
	actualAttackerLost?: number;
	actualDefenderLost?: number;
	uploadState: string;
	lastError?: string;
}

export interface BattleResearchStatusV2 {
	beta: true;
	enabled: boolean;
	consentVersion: number;
	requiredConsentVersion: number;
	state: 'disabled' | 'consent-update-required' | 'waiting-for-session' | 'observing';
	activeTrials: number;
	completedTrials: number;
	pendingUploads: number;
	lastMovementPollAt?: string;
	lastError?: string;
	calculator: {
		modelVersion: string;
		maturity: string;
		description: string;
		considered: string[];
		limitations: string[];
	};
	trials: BattleResearchTrialSummaryV2[];
}

export interface SceatSkillActivationV2 {
	id: number;
	remainingSec: number;
}

export interface LegendSkillStateV2 {
	activeIds: number[];
	skillPoints: number;
	resetRemainingSec: number;
	resetCount: number;
	sceatSkillIds: number[];
	sceatActivations: SceatSkillActivationV2[];
	observedAt?: string;
}

export interface PlayerStateV2 {
  id: number;
  name?: string;
  allianceId?: number;
  level?: number;
  legendLevel?: number;
	might?: number;
	glory?: number;
	gallantry?: number;
	resources: Record<string, number>;
	currencies: Record<string, number>;
	vip: { points?: number; level?: number; remainingSec?: number; upgrade?: number };
	achievements: {
		points: number;
		completed: Record<string, boolean>;
		progress: Record<string, number[]>;
		observedAt?: string;
	};
	legendSkills: LegendSkillStateV2;
}

export interface ResourceBalanceV2 {
	amount: number;
	productionPerHour?: number;
	capacity?: number;
}

export interface CastleUnitsV2 {
	stationed: Record<string, number>;
	traveling: Record<string, number>;
	hospital: Record<string, number>;
	specialHospital: Record<string, number>;
	total: Record<string, number>;
}

export interface DefenseToolSlotV2 {
	definitionId: number;
	amount: number;
}

export interface DefenseWallSectionV2 {
	toolSlots: DefenseToolSlotV2[];
	unitPercent: number;
	unitTypePercent: number;
}

export interface DefenseWallStateV2 {
	left: DefenseWallSectionV2;
	middle: DefenseWallSectionV2;
	right: DefenseWallSectionV2;
	unitCount?: number;
	stationedUnits?: number;
	defense?: number;
	observedAt?: string;
}

export interface DefenseKeepStateV2 {
	primaryToolSlots: DefenseToolSlotV2[];
	secondaryToolSlots: DefenseToolSlotV2[];
	mauct?: number;
	unitTypePercent: number;
	unitCount?: number;
	uyl?: number;
	auyl?: number;
	observedAt?: string;
}

export interface DefenseMoatStateV2 {
	leftToolSlots: DefenseToolSlotV2[];
	middleToolSlots: DefenseToolSlotV2[];
	rightToolSlots: DefenseToolSlotV2[];
	defense?: number;
	observedAt?: string;
}

export interface CastleDefenseStateV2 {
	wall: DefenseWallStateV2;
	keep: DefenseKeepStateV2;
	moat: DefenseMoatStateV2;
	castellanId?: number;
	gateDefense?: number;
	meleeDefense?: number;
	rangedDefense?: number;
	hdwl?: number;
	rangedUnitIds: number[];
	meleeUnitIds: number[];
	inventory: Record<string, number>;
	inventoryObservedAt?: string;
	observedAt?: string;
	openGateUntil?: string;
}

export interface CastleBuildingV2 {
	instanceId: number;
	definitionId: number;
	gridX?: number;
	gridY?: number;
	rotation?: number;
	level?: number;
	layer?: 'BG' | 'BD' | 'T' | 'G' | 'D' | 'FP';
	placed: boolean;
}

export interface CastleLayoutV2 {
	ground: Record<string, CastleBuildingV2>;
	objects: Record<string, CastleBuildingV2>;
	fixed: Record<string, CastleBuildingV2>;
	observedAt?: string;
}

export interface BuildingConstructionQueueSlotV2 {
	index: number;
	wireValue: number;
	status: 'available' | 'locked' | 'occupied' | 'unknown';
	buildingId?: number;
}

export interface BuildingConstructionQueueV2 {
	slotCount: number;
	slots: BuildingConstructionQueueSlotV2[];
	observedAt?: string;
}

export interface ConstructionSlotV2 {
	definitionId: number;
	slot: number;
	remainingSec?: number;
	level?: number;
}

export interface QueueItemV2 {
	definition: {
		collection: string;
		id: number;
	};
	amount?: number;
	startedAt?: string;
	completesAt?: string;
}

export interface ProductionQueueV2 {
	lineId: number;
	active?: QueueItemV2;
	queued: QueueItemV2[];
	capacity: number;
	observedAt: string;
}

export interface CraftingQueueItemV2 {
	recipeId: number;
	/** Per-job output boost percentage captured when this recipe was queued. */
	batchValue?: number;
	remainingSec?: number;
	runtimeSec?: number;
}

export interface CraftingBuildingV2 {
	kingdomId: number;
	castleId: number;
	instanceId: number;
	definitionId: number;
	queueTypeId: number;
	slotCount?: number;
	activeSlotRentals: number[];
	queueSlotRentals: number[];
	active: CraftingQueueItemV2[];
	queued: CraftingQueueItemV2[];
	observedAt: string;
}

export interface CraftingStateV2 {
	buildings: Record<string, CraftingBuildingV2>;
	enabledRecipeIds: number[];
	enabledRecipeGroupIds: number[];
	outputBoostByQueueType: Record<string, number>;
}

export interface CastleStateV2 {
	id: number;
	kingdomId: number;
	slotType?: number;
	name?: string;
	x: number;
	y: number;
	focused: boolean;
	resources: Record<string, ResourceBalanceV2>;
	units: CastleUnitsV2;
	unitsObservedAt?: string;
	defense: CastleDefenseStateV2;
	buildings: Record<string, CastleBuildingV2>;
	layout: CastleLayoutV2;
	buildingQueue: BuildingConstructionQueueV2;
	constructionSlots: Record<string, ConstructionSlotV2[]>;
	constructionSlotsObservedAt?: string;
	production: Record<string, ProductionQueueV2>;
	queueableProduction: Record<string, Array<{ collection: string; id: number }>>;
	queueableObservedAt?: string;
	crafting: CraftingStateV2;
}

export interface CommanderStateV2 {
	id: number;
	name?: string;
	visiblePosition?: number;
	available: boolean;
	equipment: Record<string, number>;
	gems: Record<string, number>;
}

export interface CastellanStateV2 {
	id: number;
	castleId?: number;
	name?: string;
	equipment: Record<string, number>;
	gems: Record<string, number>;
}

export interface EquipmentInstanceV2 {
	id: number;
	definitionId: number;
	slot: number;
	typeId?: number;
	rarityId?: number;
	setId?: number;
	level?: number;
	wearerId?: number;
	wearerKind?: string;
	effects: EquipmentEffectV2[];
}

export interface EquipmentEffectV2 {
	wireId: number;
	definitionId: number;
	rollPercent?: number;
	values: number[];
}

export interface GemInstanceV2 {
	id: number;
	definitionId: number;
	typeId?: number;
	compatibleWearerId?: number;
	combatMode?: 'pvp' | 'pve' | 'any';
	setId?: number;
	slot?: number;
	level?: number;
	equipmentInstanceId?: number;
	wearerId?: number;
	wearerKind?: string;
	effects: EquipmentEffectV2[];
}

export interface InventoryStateV2 {
	constructionItems: Record<string, number>;
	constructionItemsObservedAt?: string;
	constructionOffers: Record<string, number>;
	constructionOffersObservedAt?: string;
	constructionOffersCastleId?: number;
	constructionOffersKingdomId: number;
	equipment: Record<string, EquipmentInstanceV2>;
	gems: Record<string, GemInstanceV2>;
	gemStacks: Record<string, number>;
	items: Record<string, Record<string, number>>;
}

export interface SubscriptionStateV2 {
	typeId: number;
	remainingSec?: number;
	gracePeriodSec?: number;
}

export interface MarketStateV2 {
	castles: Record<string, {
		castleId: number;
		kingdomId: number;
		totalBarrows: number;
		availableBarrows: number;
		resources: Record<string, number>;
		areaEffects: Array<{ effectId: number; values: number[]; source?: string }>;
	}>;
	boosters?: Record<string, {
		id: number;
		level?: number;
		bonusPercent?: number;
		remainingSec?: number;
		continuousPurchaseCount?: number;
		expiresAt?: string;
		permanent?: boolean;
	}>;
	feast?: {
		id?: number;
		remainingSec?: number;
		expiresAt?: string;
		observedAt?: string;
	};
	caravanLevel?: number;
	caravanLevelLoaded: boolean;
	observedAt?: string;
	boostersObservedAt?: string;
}

export interface KingdomTransportStateV2 {
	unlocks: Record<string, {
		kingdomId: number;
		unlocked: boolean;
		created: boolean;
		stage?: number;
	}>;
	pending: Array<{
		kingdomId: number;
		remainingSec?: number;
		goods: Array<{ resourceId: number; amount: number }>;
	}>;
	pendingUnits: Array<{
		kingdomId: number;
		remainingSec?: number;
		units: Array<{ unitId: number; amount: number }>;
	}>;
	observedAt?: string;
}

export interface BeriStateV2 {
	availableTroops: number;
	troopsByUnit: Record<string, number>;
	parsedSourceId?: number;
	observedAt?: string;
	consumedAt?: string;
	campOpenRequestedAt?: string;
	targetX?: number;
	targetY?: number;
	targetTypeId?: number;
	targetObservedAt?: string;
	targetConsumedAt?: string;
}

export interface EquipmentPriorityV2 {
	effectId: number;
	tier: 1 | 2;
	position: number;
}

export interface EquipmentOptimizeRequest {
	leaderKind: 'commander' | 'castellan';
	leaderId: number;
	combatMode: 'pvp' | 'pve';
	priorities: EquipmentPriorityV2[];
}

export interface EquipmentEffectTotalV2 {
	definitionId: number;
	value: number;
	cap?: number;
	capped: boolean;
}

export interface EquipmentLoadoutV2 {
	equipment: Record<string, number>;
	gems: Record<string, number>;
	effects: EquipmentEffectTotalV2[];
	score: number;
}

export interface EquipmentOptimizeResponse {
	leaderKind: 'commander' | 'castellan';
	leaderId: number;
	stateRevision: number;
	current: EquipmentLoadoutV2;
	proposed: EquipmentLoadoutV2;
	candidates: {
		equipmentBySlot: Record<string, number>;
		gems: number;
	};
}

export interface BuildingCostV2 {
	field: string;
	key: string;
	scope: 'castle_resource' | 'player_resource' | 'currency' | 'unknown';
	definitionId?: number;
	amount: number;
	premium: boolean;
}

export interface BuildingDefinitionV2 {
	id: number;
	internalName?: string;
	type?: string;
	group?: string;
	shopCategory?: string;
	groundType?: string;
	level?: number;
	width?: number;
	height?: number;
	rotateType?: number;
	upgradeDefinitionId?: number;
	downgradeDefinitionId?: number;
	requiredLevel?: number;
	requiredLegendLevel?: number;
	earlyUnlockRequiredLevel?: number;
	maximumCount?: number;
	kingdomIds?: number[];
	eventIds?: number[];
	areaTypeIds?: number[];
	mapIds?: number[];
	durationSec?: number;
	lowLevelDurationSec?: number;
	forcedPosition?: string;
	storeable?: boolean;
	movable?: boolean;
	destructable?: boolean;
	battleGround?: boolean;
	district?: boolean;
	constructionItemGroupIds?: number[];
	unlockIds?: number[];
	questObjectives?: BuildingQuestObjectiveV2[];
	costs: BuildingCostV2[];
	values: Record<string, number>;
	localizationKeys?: string[];
	displayName: string;
}

export interface BuildingQuestObjectiveV2 {
	questId: number;
	requiredCount: number;
	requiredLevel?: number;
	requiredLegendLevel?: number;
	requiredQuestId?: number;
	orRequiredQuestId?: number;
	eventId?: number;
	shownKingdomId: number;
	triggerKingdomId: number;
	xp?: number;
	hidden: boolean;
	sortPriority?: number;
}

export interface BuildingCatalogQuery {
	ids?: number[];
	kingdomId?: number;
	eventId?: number;
	areaTypeId?: number;
	mapId?: number;
	maxLevel?: number;
	rootOnly?: boolean;
	query?: string;
	offset?: number;
	limit?: number;
}

export interface BuildingCatalogResponse {
	metadata: GameDataMetadata;
	total: number;
	offset: number;
	limit: number;
	items: BuildingDefinitionV2[];
}

export interface BuildingObjectiveV2 {
	metric: string;
	weight: number;
}

export interface BuildingPreviewConstraintsV2 {
	allowPremium?: boolean;
	allowUnaffordable?: boolean;
	minimumValues?: Record<string, number>;
	resourceReserves?: Record<string, number>;
}

export interface BuildingPreviewRequest {
	expectedRevision?: number;
	castleId?: number;
	profile?: 'progression' | 'balanced' | 'berimond' | 'storm' | 'custom';
	eventId?: number;
	mapId?: number;
	objectives?: BuildingObjectiveV2[];
	activeQuestIds?: number[];
	constraints?: BuildingPreviewConstraintsV2;
	candidateDefinitionIds?: number[];
	includeConstruct?: boolean;
	includeUpgrades?: boolean;
	includeBlocked?: boolean;
	maxCandidates?: number;
}

export interface BuildingLayoutIssueV2 {
	code: string;
	message: string;
	severity: 'error' | 'warning';
	buildingIds?: number[];
}

export interface BuildingLayoutAnalysisV2 {
	valid: boolean;
	bounds?: { minX: number; minY: number; maxX: number; maxY: number };
	groundCells: number;
	occupiedCells: number;
	freeCells: number;
	placedObjects: number;
	storedObjects: number;
	fixedObjects: number;
	issues: BuildingLayoutIssueV2[];
}

export interface BuildingCostStatusV2 {
	field: string;
	key: string;
	scope: BuildingCostV2['scope'];
	definitionId?: number;
	required: number;
	available: number;
	reserve: number;
	shortfall: number;
	known: boolean;
	sufficient: boolean;
	premium: boolean;
}

export interface BuildingCandidateV2 {
	kind: 'construct' | 'upgrade';
	buildingId?: number;
	fromDefinitionId?: number;
	definition: BuildingDefinitionV2;
	placement?: { gridX: number; gridY: number; rotation: number; width: number; height: number };
	durationSec: number;
	costs: BuildingCostStatusV2[];
	affordable: boolean;
	eligible: boolean;
	requiresLiveValidation: boolean;
	score: number;
	deltaValues: Record<string, number>;
	projectedValues: Record<string, number>;
	blockers: Array<{ code: string; message: string }>;
}

export interface BuildingPreviewResponse {
	revision: number;
	catalogVersion?: string;
	castleId: number;
	profile: string;
	eventId?: number;
	mapId?: number;
	objectives: BuildingObjectiveV2[];
	constraints: BuildingPreviewConstraintsV2;
	currentValues: Record<string, number>;
	layout: BuildingLayoutAnalysisV2;
	buildingQueue: BuildingConstructionQueueV2;
	candidates: BuildingCandidateV2[];
	recommended?: BuildingCandidateV2;
	rejectedCount: number;
	warnings: string[];
}

export type BuildingTargetCaptureMode = 'functional' | 'layout' | 'exact';
export type BuildingTargetStoredMode = BuildingTargetCaptureMode | 'buildings' | 'full';

export interface BuildingTargetCaptureRequest {
	expectedRevision?: number;
	castleId: number;
	mode: BuildingTargetCaptureMode;
}

export interface BuildingTargetCaptureResponse {
	version: 1;
	revision: number;
	catalogVersion?: string;
	capturedAt: string;
	castleId: number;
	kingdomId: number;
	mode: BuildingTargetStoredMode;
	exact: boolean;
	ground: Array<{
		definitionId: number;
		gridX: number;
		gridY: number;
		direction: number;
	}>;
	buildings: Array<{
		targetId: string;
		definitionId: number;
		placement?: { gridX: number; gridY: number; rotation: number };
	}>;
	fixed?: Array<{
		targetId: string;
		definitionId: number;
		slot?: { gridX: number; gridY: number; rotation: number };
	}>;
	summary: {
		groundCount: number;
		buildingCount: number;
		decorationCount: number;
		fixedCount: number;
	};
}

export interface BuildingBlueprintDiffRequest {
	expectedRevision?: number;
	target: BuildingTargetCaptureResponse;
	policy: {
		allowPremium: boolean;
		resourceReserves?: Record<string, number>;
	};
}

export interface BuildingBlueprintDiffResponse {
	revision: number;
	catalogVersion?: string;
	target: BuildingTargetCaptureResponse;
	missingGround: BuildingTargetCaptureResponse['ground'];
	compilable: boolean;
	satisfied: boolean;
	targetCount: number;
	satisfiedCount: number;
	plannedCount: number;
	waitingCount: number;
	blockedCount: number;
	actionCount: number;
	normal: {
		issues: Array<{ severity: string; code: string; message: string; targetId?: string }>;
	};
	fixed: {
		issues: Array<{ severity: string; code: string; message: string; targetId?: string }>;
	};
}

export interface ExpansionDefinitionV2 {
	id: number;
	spaceIds: number[];
	level: number;
	costs: BuildingCostV2[];
	sceatSkillLocked?: number;
	effectLocked: boolean;
}

export interface ExpansionPreviewRequest {
	expectedRevision?: number;
	castleId: number;
	payment?: 'resources' | 'premium';
	resourceReserves?: Record<string, number>;
	allowPremium?: boolean;
	allowTimeSkips?: boolean;
	x?: number;
	y?: number;
	direction?: number;
}

export interface ExpansionCostStatusV2 extends BuildingCostStatusV2 {
	capacity?: number;
	capacityKnown: boolean;
	capacityShortfall: number;
	capacitySufficient: boolean;
	storageMetric?: string;
}

export interface ExpansionStorageItemCandidateV2 {
	constructionItemId: number;
	groupId: number;
	level: number;
	displayName: string;
	buildingInstanceId?: number;
	buildingDefinitionId?: number;
	inventoryCount: number;
	capacityGain: Record<string, number>;
	eligible: boolean;
	closesCapacityGap: boolean;
	blockers: Array<{ code: string; message: string }>;
}

export interface ExpansionTransportCandidateV2 {
	sourceCastleId: number;
	sourceKingdomId: number;
	targetCastleId: number;
	targetKingdomId: number;
	goods: Array<{ resourceId: number; key: string; amount: number }>;
	coverage: number;
	remainingSurplus: number;
	complete: boolean;
	eligible: boolean;
	blockers: Array<{ code: string; message: string }>;
}

export interface ExpansionActionV2 {
	kind: 'refresh_castle' | 'upgrade_storage' | 'construct_storage' | 'equip_storage_item' | 'skip_storage_build' | 'refresh_transport' | 'skip_transport' | 'transport_resources' | 'expand';
	intent: string;
	arguments: Record<string, unknown>;
	reason: string;
}

export interface ExpansionPreviewResponse {
	revision: number;
	catalogVersion?: string;
	castleId: number;
	kingdomId: number;
	payment: 'resources' | 'premium';
	currentExpansionLevel: number;
	nextExpansion?: ExpansionDefinitionV2;
	costs: ExpansionCostStatusV2[];
	storageBuildingCandidates: BuildingCandidateV2[];
	storageItemCandidates: ExpansionStorageItemCandidateV2[];
	pendingStorageBuild?: {
		buildingInstanceId: number;
		currentDefinitionId: number;
		targetDefinitionId: number;
		progressSec: number;
		estimatedRemainingSec: number;
		capacityGain: Record<string, number>;
		closesCapacityGap: boolean;
	};
	transportCandidates: ExpansionTransportCandidateV2[];
	pendingTransport?: {
		kingdomId: number;
		remainingSec?: number;
		goods: Array<{ resourceId: number; amount: number }>;
	};
	timeSkipOptions: Array<{ currencyId: number; timeSkipId: string; minutes: number; balance: number }>;
	ready: boolean;
	recommendedAction?: ExpansionActionV2;
	blockers: Array<{ code: string; message: string }>;
	warnings: string[];
}

export interface AllianceMemberV2 {
	playerId: number;
	name?: string;
	rankId?: number;
	level?: number;
	legendLevel?: number;
	might?: number;
	returnProtectionSec?: number;
}

export interface AllianceHoldingV2 {
	castleId: number;
	playerId: number;
	kingdomId: number;
	x: number;
	y: number;
	slotType: number;
}

export interface AllianceStateV2 {
	id: number;
	name?: string;
	members: AllianceMemberV2[];
	holdings: AllianceHoldingV2[];
	observedAt?: string;
}

export interface AllianceTargetOptionV2 {
	externalId: string;
	name: string;
	rank: number;
	playerCount: number;
}

export interface AllianceTargetSelectionV2 {
	externalId: string;
	allianceId: number;
	name: string;
}

export interface AllianceTargetCastleV2 {
	castleId?: number;
	name: string;
	typeName?: string;
	typeId?: number;
	x: number;
	y: number;
}

export interface AllianceTargetOwnCastleV2 {
	name: string;
	x: number;
	y: number;
}

export interface AllianceTargetV2 {
	playerId: number;
	name: string;
	level?: number;
	legendLevel?: number;
	might: number;
	underBird: boolean;
	rptSeconds: number;
	targetCastle: AllianceTargetCastleV2;
	closestOwnCastle: AllianceTargetOwnCastleV2;
	distance: number;
	spyReport?: {
		capturedAtUnixMillis: number;
		status: string;
		totalTroops: number;
	};
}

export interface AllianceTargetAttackPresetSlotV2 {
	itemId: number | null;
	quantity: number;
}

export interface AllianceTargetAttackPresetLaneV2 {
	troops: AllianceTargetAttackPresetSlotV2[];
	tools: AllianceTargetAttackPresetSlotV2[];
}

export interface AllianceTargetAttackPresetWaveV2 {
	L: AllianceTargetAttackPresetLaneV2;
	M: AllianceTargetAttackPresetLaneV2;
	R: AllianceTargetAttackPresetLaneV2;
}

export interface AllianceTargetAttackPresetCourtyardSupportV2 {
	troops: AllianceTargetAttackPresetSlotV2[];
	tools: AllianceTargetAttackPresetSlotV2[];
}

export interface AllianceTargetAttackPreviewRequest {
	sourceCastleId: number;
	kingdomId: number;
	targetX: number;
	targetY: number;
	targetCastleId?: number;
	targetTypeId: number;
	targetLevel: number;
	targetLegendLevel?: number;
	preset: {
		id: string;
		name: string;
		waves: AllianceTargetAttackPresetWaveV2[];
		courtyardSupport: AllianceTargetAttackPresetCourtyardSupportV2;
	};
}

export interface AllianceTargetAttackPreviewV2 {
	commanderId: number;
	capacity: {
		left: number;
		front: number;
		right: number;
	};
	supportCapacity: number;
	maximumWaves: number;
	presetWaves: number;
	appliedWaves: number;
	totalTroops: number;
	totalTools: number;
	requirements: Array<{
		kind: 'troop' | 'tool';
		itemId: number;
		required: number;
		available: number;
	}>;
	ready: boolean;
}

export interface AllianceTargetQueryV2 {
	allianceId?: string;
	server?: string;
	refresh?: boolean;
	search?: string;
	status?: 'all' | 'under-bird' | 'attackable';
	sort?: 'player' | 'might' | 'rpt' | 'target' | 'closestCastle' | 'distance';
	direction?: 'asc' | 'desc';
	page?: number;
	includeAlliances?: boolean;
}

export interface AllianceTargetViewV2 {
	server: string;
	alliances: AllianceTargetOptionV2[];
	selectedAlliance?: AllianceTargetSelectionV2;
	targets: AllianceTargetV2[];
	totalTargets: number;
	page: number;
	pageSize: number;
	pageCount: number;
	canInspect: boolean;
	spies: {
		canLaunch: boolean;
		available: number;
		sourceCastleId?: number;
	};
}

export interface MapObservationV2 {
	kingdomId: number;
	x: number;
	y: number;
	typeId: number;
	name?: string;
	level?: number;
	ownerId?: number;
	objectId?: number;
	towerVictoryCount?: number;
	towerCooldownRemaining?: number;
	eventCampId?: number;
	eventCampVictoryCount?: number;
	eventCampCooldownRemaining?: number;
	eventCampBaseWallBonus?: number;
	eventCampBaseGateBonus?: number;
	eventCampBaseMoatBonus?: number;
	stormIsleId?: number;
	stormKind?: 'fort' | 'island';
	stormResource?: 'wood' | 'stone' | 'aquamarine';
	stormSize?: 'large' | 'small';
	stormFixedLoot?: number;
	stormVictoryCount?: number;
	stormCooldownRemaining?: number;
	stormReadyAt?: string;
	stormExpiresAt?: string;
	observedAt: string;
}

export interface TowerCooldownStateV2 {
	kingdomId: number;
	x: number;
	y: number;
	reportId?: number;
	lastSuccessfulBattleAt: string;
	cooldownRemaining?: number;
	cooldownObservedAt?: string;
	pendingCooldownRefresh?: boolean;
}

export interface TowerQueueEntryV2 {
	kingdomId: number;
	targetX: number;
	targetY: number;
	mapObservedAt: string;
	queuedAt: string;
}

export interface TowerQueueStateV2 {
	entriesByCastle: Record<string, TowerQueueEntryV2[]>;
	lastScannedAt: Record<string, string>;
}

export interface StormMapBoundsV2 {
	x1: number;
	y1: number;
	x2: number;
	y2: number;
}

export interface StormMapStateV2 {
	serverUrl?: string;
	playerId?: number;
	sourceCastleId?: number;
	coveredBounds: StormMapBoundsV2;
	nextBounds: StormMapBoundsV2;
	lastAttemptAt?: string;
	lastCompletedAt?: string;
	windowCount?: number;
	targets: Record<string, MapObservationV2>;
}

export interface StormIslandReturnStateV2 {
	kingdomId: number;
	sourceCastleId: number;
	targetX: number;
	targetY: number;
	islandObjectId: number;
	reportId?: number;
	status: 'awaiting_report' | 'ready';
	leaveBehind: number;
	survivors: Record<string, number>;
	launchedAt: string;
	reportedAt?: string;
}

export interface StormStateV2 {
	lastScannedAt: Record<string, string>;
	map: StormMapStateV2;
	islandReturns: Record<string, StormIslandReturnStateV2>;
	lunaShopTableId?: number;
	lunaShopProductId?: number;
	lunaShopObservedAt?: string;
}

export interface MovementStateV2 {
	id: number;
	typeId?: number;
	direction: number;
	ownerPlayerId?: number;
	targetPlayerId?: number;
	sourceTypeId?: number;
	sourceCastleId?: number;
	targetCastleId?: number;
	targetTypeId?: number;
	commanderId?: number;
	kingdomId: number;
	sourceX?: number;
	sourceY?: number;
	targetX: number;
	targetY: number;
	travelSeconds?: number;
	waitSeconds?: number;
	progressSeconds?: number;
	spyCount?: number;
	startedAt?: string;
	arrivesAt?: string;
	returnsAt?: string;
	observedAt?: string;
	units: Record<string, number>;
	marketBarrows?: number;
	marketGoods?: Array<{ resourceId: number; amount: number }>;
}

export interface ProtocolObservationV2 {
	opcode: string;
	count: number;
	inboundCount: number;
	outboundCount: number;
	lastDirection: string;
	lastCode?: number;
	lastError?: string;
	lastSeenAt: string;
	lastRevision: number;
	lastSuccessfulInboundAt?: string;
	lastSuccessfulInboundRevision?: number;
}

export interface CommandContextStateV2 {
	productionSessionKey?: number;
	productionObservedAt?: string;
}

export interface AllianceHelpRequestStateV2 {
	hospitalProductionIds: number[];
	recruitmentCastleIds: number[];
	pendingOtherListIds: number[];
	observedAt?: string;
	othersObservedAt?: string;
	othersObservedGeneration?: number;
	lastHelpAllAt?: string;
	lastHelpAllGeneration?: number;
}

export interface AutomationStateV2 {
	id: string;
	enabled: boolean;
	status: string;
	detail?: string;
	nextCheckAt?: string;
	lastRunAt?: string;
	lastOperationId?: string;
	lastError?: string;
	metrics?: Record<string, number>;
	updatedAt: string;
}

export interface AutoStormTroopCapPreviewRequest {
	settings: unknown;
}

export interface AutoStormTroopCapPreviewV2 {
	available: boolean;
	maximumTroops: number;
	troopsPerAttack: number;
	minimumTroops: number;
	historyHours: number;
	attacksInHistory: number;
	measuredAttacksInHistory: number;
	troopsSentInHistory: number;
	averageTroopsPerHour: number;
	bufferedTroops: number;
	detail?: string;
}

export interface AttackLaunchRatesV2 {
	observedAt: string;
	windowMinutes: number;
	launchesByFeature: Record<string, number>;
}

export interface MovementSnapshotV2 {
	version: number;
	observedAt?: string;
}

export interface StationingOperationV2 {
	id: string;
	purpose: string;
	phase?: 'target-ready' | 'dispatch-ready' | 'away' | 'waiting';
	sourceCastleId: number;
	targetCastleId: number;
	movementId?: number;
	units: Record<string, number>;
	delayHours?: number;
	waitSeconds?: number;
	travelSeconds?: number;
	allianceObservedAt?: string;
	unitsObservedAt?: string;
	dispatchedAt?: string;
	safeAfter?: string;
	expectedReturnAt?: string;
	nextAttemptAt?: string;
	statusDetail?: string;
	createdAt: string;
	updatedAt: string;
}

export interface ScheduledOperationV2 {
	id: string;
	version: number;
	intent: string;
	actor: string;
	arguments: Record<string, unknown>;
	worldId?: string;
	playerId?: number;
	executeAt: string;
	createdAt: string;
	updatedAt: string;
	status: string;
	lastOperationId?: string;
	lastError?: string;
}

export type CRACommanderSelectionStrategy = 'first_available' | 'lowest_id' | 'highest_id';

export interface CRACommanderSelection {
	candidates?: number[];
	count?: number;
	strategy?: CRACommanderSelectionStrategy;
}

export interface RiftLaunchV2 {
	id: string;
	displayName?: string;
	savedAtUnix: number;
	body: Record<string, unknown>;
	commanderID?: number;
	sourceX?: number;
	sourceY?: number;
	targetX?: number;
	targetY?: number;
	kingdomID?: number;
	attackValid?: number;
	waveCount?: number;
	useTravelFeather: boolean;
	oneWayTTSeconds?: number;
	lastSuccessAtUnix?: number;
}

export interface RiftStateV2 {
	launches: Record<string, RiftLaunchV2>;
	deletedLaunchIds?: Record<string, number>;
	pendingLaunchId?: string;
	maidenRun?: RiftMaidenRunStateV2;
}

export interface RiftMaidenRunStateV2 {
	id: string;
	status: 'running' | 'completed' | 'cancelled' | string;
	requestedAttacks: number;
	attacksLaunched: number;
	unitId: number;
	horseTravelBoostId: number;
	commanderIds: number[];
	launchIds?: number[];
	sourceCastleId: number;
	sourceX: number;
	sourceY: number;
	kingdomId: number;
	targetX: number;
	targetY: number;
	startedAt: string;
	updatedAt: string;
}

export interface ReportNoticeV2 {
	messageId: number;
	typeId: number;
	battleKey?: string;
	reportId?: number;
	ageSec?: number;
	status: string;
	observedAt: string;
}

export interface ReportStateV2 {
	notices: Record<string, ReportNoticeV2>;
	spyCaptures: Record<string, { messageId: number; payload: Record<string, unknown>; capturedAt: string }>;
	battleCaptures: Record<string, {
		messageId: number;
		reportId?: number;
		battleKey?: string;
		summary?: Record<string, unknown>;
		waves?: Record<string, unknown>;
		details?: Record<string, unknown>;
		capturedAt: string;
	}>;
	activeBattleReport?: number;
}

export interface ScalableEventScoreV2 {
	eventId: number;
	eventType?: string;
	name?: string;
	localizationKey: string;
	difficultyId?: number;
	difficultyTypeId?: number;
	difficultyTypeName?: string;
	playerScore: number;
	allianceScore: number;
	playerRank?: number;
	allianceRank?: number;
	remainingSec?: number;
	leagueId?: number;
	allianceLeagueId?: number;
	rewardSetId?: number;
	rewardPagesReached?: number;
	rewardPagesTotal?: number;
	nextRewardScore?: number;
	advisorActive?: boolean;
	advisorCurrencyId?: number;
	advisorFree?: boolean;
	observedAt: string;
}

export interface EventCombatTotalsV2 {
	launches: number;
	battles: number;
	victories: number;
	defeats: number;
	troopLosses: number;
	toolsUsed: number;
	loot: number;
}

export interface EventActivityStateV2 {
	eventId: number;
	occurrenceEndsAt?: string;
	observedFrom: string;
	invasion: EventCombatTotalsV2;
	camp: EventCombatTotalsV2;
	advisor: EventCombatTotalsV2;
	khan: EventCombatTotalsV2;
	khanDefense: EventCombatTotalsV2;
	advisorObservedAt?: string;
	fortifyCurrencies?: string[];
	fortifyCurrenciesObservedAt?: string;
}

export interface EventRankingEntryV2 {
	alliance?: string;
	allianceId?: number;
	memberCount?: number;
	famePoints?: number;
	rank: number;
	score: number;
}

export interface EventRankingStateV2 {
	eventId: number;
	scope?: string;
	leagueId: number;
	listType: number;
	totalAlliances: number;
	ownAllianceId?: number;
	searchValue?: string;
	firstRank?: number;
	globalFlag?: number;
	entries: EventRankingEntryV2[];
	pending?: boolean;
	observedAt?: string;
}

export interface EventScoreStateV2 {
	activeEventId?: number;
	byEvent: Record<string, ScalableEventScoreV2>;
	activityByEvent?: Record<string, EventActivityStateV2>;
	rankingByEvent?: Record<string, EventRankingStateV2>;
}

export interface AdvisorRunStateV2 {
	eventId: number;
	eventEndsAt?: string;
	sourceCastleId: number;
	kingdomId: number;
	targetTypeId: number;
	targetX: number;
	targetY: number;
	commanderId: number;
	movementId: number;
	requestedAttacks: number;
	currentAttack: number;
	launchState: number;
	status: 'running' | 'completed' | 'cancelled' | string;
	startedAt: string;
	lastAttackAt: string;
	updatedAt: string;
}

export interface AdvisorSummaryStateV2 {
	advisorType?: number;
	count?: number;
	gains: Record<string, number>;
	costs: Record<string, number>;
	unitsLost?: number;
	toolsLost?: number;
	wins?: number;
	defeats?: number;
	pendingAttacks?: number;
	observedAt?: string;
}

export interface AdvisorStateV2 {
	run?: AdvisorRunStateV2;
	summary: AdvisorSummaryStateV2;
}

export interface InvasionStateV2 {
	lastScannedAt: Record<string, string>;
	fortifiedTargets: Record<string, string>;
	fortifyCurrencies: string[];
	fortifyResourceCount: number;
	fortifyRubyCount: number;
}

export interface NomadCampTargetStateV2 {
	sourceCastleId: number;
	eventId: number;
	difficultyId: number;
	kingdomId: number;
	typeId: number;
	x: number;
	y: number;
	eventCampId: number;
	victoryCount: number;
	defenseScore: number;
	eventEndsAt?: string;
	lockedAt: string;
}

export interface NomadCampCooldownStateV2 {
	kingdomId: number;
	x: number;
	y: number;
	reportId?: number;
	lastSuccessfulBattleAt: string;
	cooldownRemaining?: number;
	cooldownObservedAt?: string;
	pendingCooldownRefresh?: boolean;
}

export interface NomadCampStateV2 {
	lastScannedAt: Record<string, string>;
	lockedTarget?: NomadCampTargetStateV2;
	cooldowns: Record<string, NomadCampCooldownStateV2>;
}

export interface KhanLaunchStateV2 {
	commanderId: number;
	movementId: number;
	arrivesAt?: string;
}

export interface KhanTauntStateV2 {
	movementId: number;
	observedAt: string;
	impactAt: string;
}

export interface KhanProtectionStateV2 {
	active: boolean;
	castleId?: number;
	offensiveWallUnits?: number;
	offensiveUnitThreshold?: number;
	triggeredAt?: string;
	gateOpenUntil?: string;
	reason?: string;
}

export interface KhanStateV2 {
	runId?: string;
	eventEndsAt?: string;
	sourceCastleId?: number;
	mainCastleId?: number;
	kingdomId?: number;
	targetX?: number;
	targetY?: number;
	attacksLaunched: number;
	victoriesConfirmed: number;
	cooldownsSkipped: number;
	launches: KhanLaunchStateV2[];
	taunts: Record<string, KhanTauntStateV2>;
	tauntsObserved: number;
	tauntsResolved: number;
	lastTauntResolvedAt?: string;
	lastReportId?: number;
	lastAttackLaunchedAt?: string;
	lastCooldownSkippedAt?: string;
	safetyError?: string;
	protection: KhanProtectionStateV2;
}

export interface DailyAttackStateV2 {
	count: number;
	serverThreshold: number;
	growthRate: number;
	observedAt?: string;
}

export interface GameStateV2 {
  schemaVersion: number;
  revision: number;
  updatedAt: string;
  catalogVersion?: string;
  languageVersion?: string;
  session: SessionStateV2;
  account: AccountBindingStateV2;
  player: PlayerStateV2;
	castles: Record<string, CastleStateV2>;
	commanders: Record<string, CommanderStateV2>;
	castellans: Record<string, CastellanStateV2>;
	movements: Record<string, MovementStateV2>;
	movementSnapshot: MovementSnapshotV2;
	stationing: Record<string, StationingOperationV2>;
	scheduled: Record<string, ScheduledOperationV2>;
	rift: RiftStateV2;
  inventory: InventoryStateV2;
	subscriptions: Record<string, SubscriptionStateV2>;
	market: MarketStateV2;
	kingdomTransport: KingdomTransportStateV2;
	beri: BeriStateV2;
  alliance: AllianceStateV2;
  alliances: Record<string, AllianceStateV2>;
	allianceHelpRequests: AllianceHelpRequestStateV2;
  map: Record<string, Record<string, MapObservationV2>>;
	towerCooldowns: Record<string, TowerCooldownStateV2>;
	towerQueue: TowerQueueStateV2;
	invasion: InvasionStateV2;
	storm: StormStateV2;
	nomadCamps: NomadCampStateV2;
	khan: KhanStateV2;
	dailyAttacks: DailyAttackStateV2;
	eventScores: EventScoreStateV2;
	advisor: AdvisorStateV2;
	commandContext: CommandContextStateV2;
	automations: Record<string, AutomationStateV2>;
	reports: ReportStateV2;
	observations: Record<string, ProtocolObservationV2>;
}

export interface GameDataMetadata {
  itemVersion: string;
  language?: string;
  languageVersion?: string;
  sourceUrl: string;
  digestSha256: string;
  fetchedAt: string;
  loadedAt: string;
}

export interface AutoBuyerPriceV1 {
  field: string;
  jsonKey?: string;
  name: string;
  scope: 'playerResource' | 'castleResource' | 'currency';
  resourceId?: number;
  currencyId?: number;
  amount: number;
  premium: boolean;
}

export interface AutoBuyerShopV1 {
  id: string;
  name: string;
  kind: 'merchant' | 'event' | string;
  tableId: number;
  requiresEvent: boolean;
  resetTracking: string;
  packageCount: number;
  unsupportedCount?: number;
}

export interface AutoBuyerPackageV1 {
  shopId: string;
  shopName: string;
  shopKind: string;
  tableId: number;
  requiresEvent: boolean;
  packageId: number;
  packageType?: string;
  name: string;
  detail?: string;
  stock: number;
  maxBuyPerClick?: number;
  minLevel?: number;
  maxLevel?: number;
  minLegendLevel?: number;
  maxLegendLevel?: number;
  price: AutoBuyerPriceV1;
}

export interface AutoBuyerSpecialistV1 {
  id: number;
  key: string;
  name: string;
  durationSec: number;
  baseRubyCost: number;
  bonusPercent?: number;
}

export interface AutoBuyerFeastV1 {
  id: number;
  name: string;
  type?: string;
  durationSec: number;
  productionBoostPercent: number;
  minLevel?: number;
  maxLevel?: number;
  price: AutoBuyerPriceV1;
}

export interface AutoBuyerProjectionV1 {
  metadata: GameDataMetadata;
  shops: AutoBuyerShopV1[];
  packages: AutoBuyerPackageV1[];
  specialists: AutoBuyerSpecialistV1[];
  feasts: AutoBuyerFeastV1[];
  timedOffers: { supported: boolean; reason?: string };
}

export interface LanguageMetadata {
  language: string;
  version: string;
  branch?: string;
  sourceUrl: string;
  fetchedAt: string;
  loadedAt: string;
}

export interface CatalogSummary {
  name: string;
  kind: string;
  count: number;
  primaryKey?: string;
  fields?: string[];
}

export interface CatalogManifest {
  metadata: GameDataMetadata;
  language?: LanguageMetadata;
  catalogs: CatalogSummary[];
}

export interface CatalogResponse<T extends Record<string, unknown> = Record<string, unknown>> {
  metadata: GameDataMetadata;
  catalog: CatalogSummary;
  items: T[];
}

export type IntentStatus = 'planning' | 'planned' | 'queued' | 'running' | 'paused' | 'succeeded' | 'cancelled' | 'failed' | 'partially_succeeded' | 'indeterminate' | 'reconciling';

export type IntentEffectPhase = 'accepted' | 'planned' | 'dispatching' | 'sent' | 'awaiting_response' | 'observed' | 'reconciliation_required' | 'completed';

export interface IntentResourceKey {
	account?: string;
	scope: 'application' | 'session' | 'account' | 'kingdom' | 'castle';
	kingdomId?: number;
	castleId?: number;
	capability: string;
	resourceKind: string;
	resourceId?: string;
}

export interface IntentStep {
  name?: string;
  action?: string;
	resolver?: string;
	resolverArguments?: Record<string, unknown>;
  opcode?: string;
	payload?: Record<string, unknown> | unknown[];
  awaitOpcode?: string;
  timeoutMillis?: number;
	successCodes?: number[];
	captureResponse?: boolean;
	responseBarrier?: 'wire' | 'wire-then-committed' | 'committed';
	resumePolicy?: 'once' | 'rebuild';
}

export interface IntentAdmission {
	class: 'attack-launch' | string;
	module: string;
	affinity?: string;
	notBefore?: string;
	deadline?: string;
}

export interface IntentPlan {
  intent: string;
  effect: 'read' | 'write' | 'launch' | 'external';
  stateRevision: number;
  catalogVersion?: string;
  claims?: string[];
  resources?: IntentResourceKey[];
  steps: IntentStep[];
  summary?: string;
	admission?: IntentAdmission;
}

export interface IntentReceipt {
  streamSequence?: number;
  streamGap?: boolean;
  id: string;
  intent: string;
  actor: string;
  priority: number;
  status: IntentStatus;
  phase?: IntentEffectPhase;
  attempt?: number;
  plan?: IntentPlan;
	exchanges?: IntentCommandExchange[];
  error?: string;
  submittedAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface IntentCommandExchange {
	step?: string;
	command: {
		namespace?: string;
		opcode: string;
		sequence?: string;
		route?: string;
		payload: unknown;
		bare?: boolean;
		omitNamespace?: boolean;
	};
	response?: {
		direction: string;
		transport: string;
		namespace?: string;
		opcode: string;
		route?: string;
		responseCode?: number;
		responseText?: string;
		payload?: unknown;
		payloadText?: string;
		receivedAt: string;
	};
}

export interface IntentDefinition {
	name: string;
	description: string;
	effect: 'read' | 'write' | 'launch' | 'external';
	argumentsExample?: Record<string, unknown>;
	attackModule?: {
		id: string;
		label: string;
		description?: string;
		defaultWeight: number;
	};
}

export interface SubmitIntentOptions {
  id?: string;
  actor?: string;
  /** Explicit 1–100 override. Omit to derive priority from the actor. */
  priority?: number;
  expectedRevision?: number;
  dryRun?: boolean;
}

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
  payload?: T;
}

export interface SessionStateV2 {
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

export interface BrowserCandidate {
  id: string;
  name: string;
  executablePath: string;
}

export interface BrowserInventory {
  selected: Pick<BrowserCandidate, 'id' | 'name'> | null;
  available: BrowserCandidate[];
  selectionIntent: 'session.select_browser';
}

export interface ConfigurationSnapshot {
	schemaVersion: number;
	revision: number;
	updatedAt: string;
	sections: Record<string, unknown>;
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

export interface CastleBuildingV2 {
	instanceId: number;
	definitionId: number;
	gridX?: number;
	gridY?: number;
	rotation?: number;
	level?: number;
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
	buildings: Record<string, CastleBuildingV2>;
	constructionSlots: Record<string, ConstructionSlotV2[]>;
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
	caravanLevel?: number;
	caravanLevelLoaded: boolean;
	observedAt?: string;
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
	observedAt?: string;
}

export interface BeriStateV2 {
	availableTroops: number;
	troopsByUnit: Record<string, number>;
	parsedSourceId?: number;
	observedAt?: string;
	consumedAt?: string;
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
	current: EquipmentLoadoutV2;
	proposed: EquipmentLoadoutV2;
	candidates: {
		equipmentBySlot: Record<string, number>;
		gems: number;
	};
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
	allianceId: number;
	name: string;
	rank: number;
	might: number;
	playerCount: number;
}

export interface AllianceTargetCastleV2 {
	castleId?: number;
	name: string;
	typeName?: string;
	x: number;
	y: number;
	type?: number;
}

export interface AllianceTargetV2 {
	playerId: number;
	name: string;
	might: number;
	underBird: boolean;
	rptSeconds: number;
	birdUntil?: string;
	updatedAt?: string;
	targetCastle: AllianceTargetCastleV2;
	closestOwnCastle: AllianceTargetCastleV2;
	distance: number;
}

export interface AllianceTargetViewV2 {
	server: string;
	alliances: AllianceTargetOptionV2[];
	selectedAlliance?: AllianceTargetOptionV2;
	targets: AllianceTargetV2[];
	spies: {
		capacity: number;
		active: number;
		available: number;
		buildingRowsLoaded: boolean;
		sourceCastle: AllianceTargetCastleV2;
		taverns: Array<{ level: number; capacity: number }>;
	};
	fetchedAt: string;
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
	observedAt: string;
}

export interface MovementStateV2 {
	id: number;
	typeId?: number;
	direction: number;
	ownerPlayerId?: number;
	targetPlayerId?: number;
	sourceCastleId?: number;
	targetCastleId?: number;
	commanderId?: number;
	kingdomId: number;
	sourceX?: number;
	sourceY?: number;
	targetX: number;
	targetY: number;
	travelSeconds?: number;
	progressSeconds?: number;
	spyCount?: number;
	arrivesAt?: string;
	returnsAt?: string;
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
}

export interface CommandContextStateV2 {
	productionSessionKey?: number;
	productionObservedAt?: string;
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

export interface MovementSnapshotV2 {
	version: number;
	observedAt?: string;
}

export interface StationingOperationV2 {
	id: string;
	purpose: string;
	sourceCastleId: number;
	targetCastleId: number;
	movementId?: number;
	units: Record<string, number>;
	safeAfter?: string;
	createdAt: string;
	updatedAt: string;
}

export interface ScheduledOperationV2 {
	id: string;
	intent: string;
	actor: string;
	arguments: Record<string, unknown>;
	executeAt: string;
	createdAt: string;
	status: string;
	lastOperationId?: string;
	lastError?: string;
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
	pendingLaunchId?: string;
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

export interface GameStateV2 {
  schemaVersion: number;
  revision: number;
  updatedAt: string;
  catalogVersion?: string;
  languageVersion?: string;
  session: SessionStateV2;
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
  map: Record<string, Record<string, MapObservationV2>>;
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

export type IntentStatus = 'planning' | 'planned' | 'running' | 'succeeded' | 'cancelled' | 'failed';

export interface IntentStep {
  name?: string;
  action?: string;
  opcode?: string;
	payload?: Record<string, unknown> | unknown[];
  awaitOpcode?: string;
  timeoutMillis?: number;
  successCodes?: number[];
}

export interface IntentPlan {
  intent: string;
  effect: 'read' | 'write' | 'launch' | 'external';
  stateRevision: number;
  claims?: string[];
  steps: IntentStep[];
  summary?: string;
}

export interface IntentReceipt {
  id: string;
  intent: string;
  actor: string;
  status: IntentStatus;
  plan?: IntentPlan;
  error?: string;
  submittedAt: string;
  startedAt?: string;
  completedAt?: string;
}

export interface IntentDefinition {
	name: string;
	description: string;
	effect: 'read' | 'write' | 'launch' | 'external';
}

export interface SubmitIntentOptions {
  id?: string;
  actor?: string;
  expectedRevision?: number;
  dryRun?: boolean;
}

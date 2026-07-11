export type APIConnectionStatus = 'Disconnected' | 'Connecting' | 'Connected';

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
	queues: Record<string, unknown[]>;
}

export interface CommanderStateV2 {
	id: number;
	name?: string;
	available: boolean;
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
	arrivesAt?: string;
	returnsAt?: string;
	units: Record<string, number>;
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
	movements: Record<string, MovementStateV2>;
  inventory: Record<string, unknown>;
  alliance: Record<string, unknown>;
  map: Record<string, unknown>;
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

export type IntentStatus = 'planning' | 'planned' | 'running' | 'succeeded' | 'failed';

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

export interface SubmitIntentOptions {
  actor?: string;
  expectedRevision?: number;
  dryRun?: boolean;
}

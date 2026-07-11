import { queueConfigurationUpdate } from './Configuration';

export const AUTO_SCEAT_RES_SETTINGS_STORAGE_KEY = 'autoSceatResSettings';
export const AUTO_SCEAT_RES_SETTINGS_CHANGED_EVENT = 'autoSceatResSettingsChanged';
export const DEFAULT_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC = 300;
export const MIN_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC = 30;
export const MAX_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC = 86400;

export interface AutoSceatRecipeStep {
  recipeID: number;
  repeat: number;
}

export interface AutoSceatBuildingPlan {
  enabled: boolean;
  steps: AutoSceatRecipeStep[];
  cursor: number;
  autoRentActiveSlot: boolean;
  autoRentQueueSlots: number;
}

export interface AutoSceatCastlePlan {
  buildings: Record<string, AutoSceatBuildingPlan>;
}

export interface AutoSceatResClientSettings {
  checkIntervalSec: number;
  minimumShipmentSize: number;
  sourceReservePercent: number;
  overflowThresholdPercent: number;
  autoKingdomTransport: boolean;
  useKingdomTimeSkips: boolean;
  allowedTimeSkips: string[];
  timeSkipReserve: Record<string, number>;
  useStormBuffer: boolean;
  allowRubyRecipes: boolean;
  useRubyOverflowSkip: boolean;
  minimumCoinReserve: number;
  minimumRubyReserve: number;
  castles: Record<string, AutoSceatCastlePlan>;
}

export interface AutoSceatResourceMeta {
  key: string;
  name: string;
  jsonKey?: string;
  assetName?: string;
  iconUrl?: string;
}

export interface AutoSceatRecipeOutput {
  rewardID: number;
  key: string;
  name: string;
  amount: number;
  iconUrl?: string;
}

export interface AutoSceatRecipeCatalogEntry {
  recipeID: number;
  queueTypeID: number;
  recipeGroupID: number;
  researchGroupID?: number;
  level: number;
  type: string;
  durationSec: number;
  skipCostRubies: number;
  requiredBuildingWIDs?: number[];
  costs: Record<string, number>;
  output: AutoSceatRecipeOutput;
}

export interface AutoSceatBuildingState {
  queueTypeID: number;
  name: string;
  oid: number;
  wid: number;
  activeCapacity: number;
  queueCapacity: number;
  activeRecipes: number[];
  queuedRecipes: number[];
  availableRecipeIDs: number[];
}

export interface AutoSceatMarketState {
  loaded: boolean;
  baseBarrows: number;
  buildItemBarrows: number;
  otherBarrows: number;
  totalBarrows: number;
  availableBarrows: number;
  busyBarrows: number;
  caravanLevel: number;
  caravanBoostPercent: number;
  areaCapacityBoostPercent: number;
  capacityBonus: number;
  capacityPerBarrow: number;
  availableShipmentCapacity: number;
}

export interface AutoSceatStorageNode {
  castleID: number;
  name: string;
  role: string;
  kingdomID: number;
  canCraft: boolean;
  stormBuffer: boolean;
  resources: Record<string, number>;
  storage: Record<string, number>;
  market?: AutoSceatMarketState;
  buildings: AutoSceatBuildingState[];
}

export interface AutoSceatResCatalog {
  recipes: AutoSceatRecipeCatalogEntry[];
  resources: Record<string, AutoSceatResourceMeta>;
  nodes: AutoSceatStorageNode[];
  researchLoaded: boolean;
}

const VALID_TIME_SKIPS = new Set(['MS1', 'MS2', 'MS3', 'MS4', 'MS5', 'MS6', 'MS7']);

export function defaultAutoSceatResSettings(): AutoSceatResClientSettings {
  return {
    checkIntervalSec: DEFAULT_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC,
    minimumShipmentSize: 10_000,
    sourceReservePercent: 10,
    overflowThresholdPercent: 90,
    autoKingdomTransport: false,
    useKingdomTimeSkips: false,
    allowedTimeSkips: ['MS5'],
    timeSkipReserve: {},
    useStormBuffer: true,
    allowRubyRecipes: false,
    useRubyOverflowSkip: false,
    minimumCoinReserve: 0,
    minimumRubyReserve: 0,
    castles: {},
  };
}

function finiteInteger(value: unknown, fallback: number, min: number, max: number): number {
  const number = Number(value);
  if (!Number.isFinite(number)) return fallback;
  return Math.min(max, Math.max(min, Math.round(number)));
}

function normalizeStep(raw: unknown): AutoSceatRecipeStep | null {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return null;
  const step = raw as Record<string, unknown>;
  const recipeID = finiteInteger(step.recipeID, 0, 0, Number.MAX_SAFE_INTEGER);
  if (recipeID <= 0) return null;
  return {
    recipeID,
    repeat: finiteInteger(step.repeat, 1, 1, 100),
  };
}

function normalizeBuilding(raw: unknown): AutoSceatBuildingPlan {
  const value = raw && typeof raw === 'object' && !Array.isArray(raw)
    ? raw as Record<string, unknown>
    : {};
  const steps = Array.isArray(value.steps)
    ? value.steps.map(normalizeStep).filter((step): step is AutoSceatRecipeStep => step != null)
    : [];
  const cycleLength = steps.reduce((total, step) => total + step.repeat, 0);
  const rawCursor = finiteInteger(value.cursor, 0, 0, Number.MAX_SAFE_INTEGER);
  return {
    enabled: value.enabled === true,
    steps,
    cursor: cycleLength > 0 ? rawCursor % cycleLength : 0,
    autoRentActiveSlot: value.autoRentActiveSlot === true,
    autoRentQueueSlots: finiteInteger(value.autoRentQueueSlots, 0, 0, 3),
  };
}

export function normalizeAutoSceatResSettings(raw: unknown): AutoSceatResClientSettings {
  const defaults = defaultAutoSceatResSettings();
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return defaults;
  const value = raw as Record<string, unknown>;
  const castles: Record<string, AutoSceatCastlePlan> = {};
  if (value.castles && typeof value.castles === 'object' && !Array.isArray(value.castles)) {
    Object.entries(value.castles as Record<string, unknown>).forEach(([castleID, rawCastle]) => {
      if (!Number.isFinite(Number(castleID)) || Number(castleID) <= 0) return;
      const castle = rawCastle && typeof rawCastle === 'object' && !Array.isArray(rawCastle)
        ? rawCastle as Record<string, unknown>
        : {};
      const buildings: Record<string, AutoSceatBuildingPlan> = {};
      if (castle.buildings && typeof castle.buildings === 'object' && !Array.isArray(castle.buildings)) {
        Object.entries(castle.buildings as Record<string, unknown>).forEach(([queueID, building]) => {
          const id = Number(queueID);
          if (!Number.isInteger(id) || id < 1 || id > 4) return;
          buildings[String(id)] = normalizeBuilding(building);
        });
      }
      castles[String(Math.floor(Number(castleID)))] = { buildings };
    });
  }

  const allowedTimeSkips = Array.isArray(value.allowedTimeSkips)
    ? [...new Set(value.allowedTimeSkips
        .map((entry) => String(entry).trim().toUpperCase())
        .filter((entry) => VALID_TIME_SKIPS.has(entry)))]
    : defaults.allowedTimeSkips;
  const timeSkipReserve: Record<string, number> = {};
  if (value.timeSkipReserve && typeof value.timeSkipReserve === 'object' && !Array.isArray(value.timeSkipReserve)) {
    Object.entries(value.timeSkipReserve as Record<string, unknown>).forEach(([id, reserve]) => {
      const normalizedID = id.toUpperCase();
      if (!VALID_TIME_SKIPS.has(normalizedID)) return;
      timeSkipReserve[normalizedID] = finiteInteger(reserve, 0, 0, Number.MAX_SAFE_INTEGER);
    });
  }

  return {
    checkIntervalSec: finiteInteger(
      value.checkIntervalSec,
      defaults.checkIntervalSec,
      MIN_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC,
      MAX_AUTO_SCEAT_RES_CHECK_INTERVAL_SEC,
    ),
    minimumShipmentSize: finiteInteger(value.minimumShipmentSize, defaults.minimumShipmentSize, 0, Number.MAX_SAFE_INTEGER),
    sourceReservePercent: finiteInteger(value.sourceReservePercent, defaults.sourceReservePercent, 0, 95),
    overflowThresholdPercent: finiteInteger(value.overflowThresholdPercent, defaults.overflowThresholdPercent, 50, 100),
    autoKingdomTransport: value.autoKingdomTransport === true,
    useKingdomTimeSkips: value.useKingdomTimeSkips === true,
    allowedTimeSkips: allowedTimeSkips.length > 0 ? allowedTimeSkips : defaults.allowedTimeSkips,
    timeSkipReserve,
    useStormBuffer: value.useStormBuffer !== false,
    allowRubyRecipes: value.allowRubyRecipes === true,
    useRubyOverflowSkip: value.useRubyOverflowSkip === true,
    minimumCoinReserve: Math.max(0, Number.isFinite(Number(value.minimumCoinReserve)) ? Number(value.minimumCoinReserve) : 0),
    minimumRubyReserve: Math.max(0, Number.isFinite(Number(value.minimumRubyReserve)) ? Number(value.minimumRubyReserve) : 0),
    castles,
  };
}

export function loadAutoSceatResSettingsFromStorage(): AutoSceatResClientSettings {
  try {
    const raw = localStorage.getItem(AUTO_SCEAT_RES_SETTINGS_STORAGE_KEY);
    return raw ? normalizeAutoSceatResSettings(JSON.parse(raw)) : defaultAutoSceatResSettings();
  } catch {
    return defaultAutoSceatResSettings();
  }
}

export function applyAutoSceatResSettingsToLocalStorage(settings: AutoSceatResClientSettings): void {
  const normalized = normalizeAutoSceatResSettings(settings);
  localStorage.setItem(AUTO_SCEAT_RES_SETTINGS_STORAGE_KEY, JSON.stringify(normalized));
  window.dispatchEvent(new CustomEvent(AUTO_SCEAT_RES_SETTINGS_CHANGED_EVENT, { detail: normalized }));
}

export function persistAutoSceatResSettings(settings: AutoSceatResClientSettings): boolean {
  const normalized = normalizeAutoSceatResSettings(settings);
  applyAutoSceatResSettingsToLocalStorage(normalized);
  return queueConfigurationUpdate('automation.autoSceatResources', normalized);
}

export function emptyAutoSceatResCatalog(): AutoSceatResCatalog {
  return { recipes: [], resources: {}, nodes: [], researchLoaded: false };
}

export function normalizeAutoSceatResCatalog(raw: unknown): AutoSceatResCatalog {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return emptyAutoSceatResCatalog();
  const value = raw as Record<string, unknown>;
  const recipes = Array.isArray(value.recipes)
    ? value.recipes.filter((recipe): recipe is AutoSceatRecipeCatalogEntry => !!recipe && typeof recipe === 'object')
    : [];
  const resources = value.resources && typeof value.resources === 'object' && !Array.isArray(value.resources)
    ? value.resources as Record<string, AutoSceatResourceMeta>
    : {};
  const nodes = Array.isArray(value.nodes)
    ? value.nodes
        .filter((node): node is Record<string, unknown> => !!node && typeof node === 'object' && !Array.isArray(node))
        .map((node): AutoSceatStorageNode => ({
          castleID: finiteInteger(node.castleID, 0, 0, Number.MAX_SAFE_INTEGER),
          name: typeof node.name === 'string' ? node.name : 'Castle',
          role: typeof node.role === 'string' ? node.role : 'Castle',
          kingdomID: finiteInteger(node.kingdomID, 0, 0, Number.MAX_SAFE_INTEGER),
          canCraft: node.canCraft === true,
          stormBuffer: node.stormBuffer === true,
          resources: node.resources && typeof node.resources === 'object' && !Array.isArray(node.resources)
            ? node.resources as Record<string, number>
            : {},
          storage: node.storage && typeof node.storage === 'object' && !Array.isArray(node.storage)
            ? node.storage as Record<string, number>
            : {},
          market: node.market && typeof node.market === 'object' && !Array.isArray(node.market)
            ? {
                loaded: (node.market as Record<string, unknown>).loaded === true,
                baseBarrows: finiteInteger((node.market as Record<string, unknown>).baseBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                buildItemBarrows: finiteInteger((node.market as Record<string, unknown>).buildItemBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                otherBarrows: finiteInteger((node.market as Record<string, unknown>).otherBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                totalBarrows: finiteInteger((node.market as Record<string, unknown>).totalBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                availableBarrows: finiteInteger((node.market as Record<string, unknown>).availableBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                busyBarrows: finiteInteger((node.market as Record<string, unknown>).busyBarrows, 0, 0, Number.MAX_SAFE_INTEGER),
                caravanLevel: finiteInteger((node.market as Record<string, unknown>).caravanLevel, 0, 0, Number.MAX_SAFE_INTEGER),
                caravanBoostPercent: Number((node.market as Record<string, unknown>).caravanBoostPercent) || 0,
                areaCapacityBoostPercent: Number((node.market as Record<string, unknown>).areaCapacityBoostPercent) || 0,
                capacityBonus: Number((node.market as Record<string, unknown>).capacityBonus) || 0,
                capacityPerBarrow: finiteInteger((node.market as Record<string, unknown>).capacityPerBarrow, 0, 0, Number.MAX_SAFE_INTEGER),
                availableShipmentCapacity: finiteInteger((node.market as Record<string, unknown>).availableShipmentCapacity, 0, 0, Number.MAX_SAFE_INTEGER),
              }
            : undefined,
          buildings: Array.isArray(node.buildings)
            ? node.buildings
                .filter((building): building is Record<string, unknown> => !!building && typeof building === 'object' && !Array.isArray(building))
                .map((building): AutoSceatBuildingState => ({
                  queueTypeID: finiteInteger(building.queueTypeID, 0, 0, 4),
                  name: typeof building.name === 'string' ? building.name : 'Crafting Building',
                  oid: finiteInteger(building.oid, 0, 0, Number.MAX_SAFE_INTEGER),
                  wid: finiteInteger(building.wid, 0, 0, Number.MAX_SAFE_INTEGER),
                  activeCapacity: finiteInteger(building.activeCapacity, 1, 1, 2),
                  queueCapacity: finiteInteger(building.queueCapacity, 1, 1, 4),
                  activeRecipes: Array.isArray(building.activeRecipes)
                    ? building.activeRecipes.map(Number).filter(Number.isFinite)
                    : [],
                  queuedRecipes: Array.isArray(building.queuedRecipes)
                    ? building.queuedRecipes.map(Number).filter(Number.isFinite)
                    : [],
                  availableRecipeIDs: Array.isArray(building.availableRecipeIDs)
                    ? building.availableRecipeIDs.map(Number).filter(Number.isFinite)
                    : [],
                }))
            : [],
        }))
        .filter((node) => node.castleID > 0)
    : [];
  return { recipes, resources, nodes, researchLoaded: value.researchLoaded === true };
}

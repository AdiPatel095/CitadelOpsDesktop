import type { BuildingTargetCaptureResponse } from '../api/Contracts';
import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_STORM_SECTION = 'automation.autoStorm';
export const AUTO_STORM_BLUEPRINTS_SECTION = 'automation.autoStormBlueprints';
export const AUTO_STORM_MAP_REFRESH_INTERVAL_SEC = 2 * 60 * 60;
export const AUTO_STORM_LUNA_PACKAGE_IDS: readonly number[] = [
  3116, 3117, 3118, 3119, 3120, 3122, 3123, 3124, 3125,
  245, 246, 247, 248,
  2795, 2796, 2797, 2798,
];

export type AutoStormResource = 'wood' | 'stone' | 'aquamarine';
export type AutoStormIslandSize = 'large' | 'small';
export const AUTO_STORM_TARGET_PRIORITIES = [
  'fort:80',
  'fort:70',
  'fort:60',
  'fort:50',
  'fort:40',
  'island:large',
  'island:small',
] as const;
export type AutoStormTargetPriority = typeof AUTO_STORM_TARGET_PRIORITIES[number];

export interface AutoStormDefenseUnit {
  unitId: number;
  amount: number;
}

export interface AutoStormShopPurchase {
  packageId: number;
  targetPurchases: number;
  unlimited: boolean;
  priority: number;
}

export interface AutoStormClientStateV1 {
  version: 1;
  target?: BuildingTargetCaptureResponse;
  decorationPresetCastleId: number;
  decorationPresetId: string;
  build: {
    allowPremium: boolean;
    allowDemolition: boolean;
    allowResourceTransport: boolean;
    allowTimeSkips: boolean;
    resourceReserves: Record<string, number>;
    sourceResourceReserves: Record<string, number>;
    timeSkipReserve: Record<string, number>;
  };
  harbor: {
    enabled: boolean;
    targetLevel: number;
  };
  forts: {
    enabled: boolean;
    levels: number[];
    minimumWins: number;
    presetId: string;
  };
  islands: {
    enabled: boolean;
    resources: AutoStormResource[];
    sizes: AutoStormIslandSize[];
    presetId: string;
    defenseUnits: AutoStormDefenseUnit[];
  };
  troopImport: {
    enabled: boolean;
    donorCastleIds: number[];
  };
  aquamarine: {
    reserve: number;
    shopTableId: number;
    purchases: AutoStormShopPurchase[];
  };
  targetPriority: AutoStormTargetPriority[];
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  horseTravelBoostId: HorseTravelBoostID;
}

const FORT_LEVELS = [40, 50, 60, 70, 80] as const;
const ISLAND_RESOURCES: AutoStormResource[] = ['wood', 'stone', 'aquamarine'];
const ISLAND_SIZES: AutoStormIslandSize[] = ['large', 'small'];
const AUTO_STORM_LUNA_PACKAGE_ID_SET = new Set(AUTO_STORM_LUNA_PACKAGE_IDS);

export function defaultAutoStormClientState(): AutoStormClientStateV1 {
  return {
    version: 1,
    decorationPresetCastleId: 0,
    decorationPresetId: '',
    build: {
      allowPremium: false,
      allowDemolition: false,
      allowResourceTransport: true,
      allowTimeSkips: false,
      resourceReserves: {},
      sourceResourceReserves: {},
      timeSkipReserve: {},
    },
    harbor: { enabled: false, targetLevel: 1 },
    forts: { enabled: false, levels: [...FORT_LEVELS], minimumWins: 0, presetId: '' },
    islands: {
      enabled: false,
      resources: [...ISLAND_RESOURCES],
      sizes: [...ISLAND_SIZES],
      presetId: '',
      defenseUnits: [],
    },
    troopImport: { enabled: false, donorCastleIds: [] },
    aquamarine: { reserve: 0, shopTableId: 0, purchases: [] },
    targetPriority: [...AUTO_STORM_TARGET_PRIORITIES],
    checkIntervalSec: 30,
    mapRefreshIntervalSec: AUTO_STORM_MAP_REFRESH_INTERVAL_SEC,
    dailyAttackLimit: 0,
    horseTravelBoostId: -1,
  };
}

export function parseAutoStormClientState(value: unknown): AutoStormClientStateV1 {
  const fallback = defaultAutoStormClientState();
  if (!isRecord(value)) return fallback;

  const build = isRecord(value.build) ? value.build : {};
  const harbor = isRecord(value.harbor) ? value.harbor : {};
  const forts = isRecord(value.forts) ? value.forts : {};
  const islands = isRecord(value.islands) ? value.islands : {};
  const troopImport = isRecord(value.troopImport) ? value.troopImport : {};
  const aquamarine = isRecord(value.aquamarine) ? value.aquamarine : {};
  const target = parseTarget(value.target);

  return {
    version: 1,
    ...(target ? { target } : {}),
    decorationPresetCastleId: positiveInteger(value.decorationPresetCastleId),
    decorationPresetId: stringValue(value.decorationPresetId),
    build: {
      allowPremium: build.allowPremium === true,
      allowDemolition: build.allowDemolition === true,
      allowResourceTransport: build.allowResourceTransport !== false,
      allowTimeSkips: build.allowTimeSkips === true,
      resourceReserves: parseNumberMap(build.resourceReserves),
      sourceResourceReserves: parseNumberMap(build.sourceResourceReserves),
      timeSkipReserve: parseNumberMap(build.timeSkipReserve),
    },
    harbor: {
      enabled: harbor.enabled === true,
      targetLevel: clampAutoStormInteger(harbor.targetLevel, 1, 3, fallback.harbor.targetLevel),
    },
    forts: {
      enabled: forts.enabled === true,
      levels: parseChoiceArray(forts.levels, FORT_LEVELS),
      minimumWins: clampAutoStormInteger(forts.minimumWins, 0, Number.MAX_SAFE_INTEGER, 0),
      presetId: stringValue(forts.presetId),
    },
    islands: {
      enabled: islands.enabled === true,
      resources: parseChoiceArray(islands.resources, ISLAND_RESOURCES),
      sizes: parseChoiceArray(islands.sizes, ISLAND_SIZES),
      presetId: stringValue(islands.presetId),
      defenseUnits: parseDefenseUnits(islands.defenseUnits),
    },
    troopImport: {
      enabled: troopImport.enabled === true,
      donorCastleIds: parsePositiveIntegerArray(troopImport.donorCastleIds),
    },
    aquamarine: {
      reserve: clampAutoStormInteger(aquamarine.reserve, 0, Number.MAX_SAFE_INTEGER, 0),
      shopTableId: positiveInteger(aquamarine.shopTableId),
      purchases: parseShopPurchases(aquamarine.purchases),
    },
    targetPriority: parseTargetPriority(value.targetPriority, value.combatOrder),
    checkIntervalSec: clampAutoStormInteger(value.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: AUTO_STORM_MAP_REFRESH_INTERVAL_SEC,
    dailyAttackLimit: clampAutoStormInteger(value.dailyAttackLimit, 0, Number.MAX_SAFE_INTEGER, 0),
    horseTravelBoostId: parseHorseTravelBoostID(value.horseTravelBoostId),
  };
}

export function clampAutoStormInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function parseTarget(value: unknown): BuildingTargetCaptureResponse | undefined {
  if (!isRecord(value)
    || value.version !== 1
    || !positiveInteger(value.castleId)
    || !Number.isFinite(Number(value.kingdomId))
    || !['functional', 'layout', 'exact', 'buildings', 'full'].includes(String(value.mode))
    || !Array.isArray(value.ground)
    || !Array.isArray(value.buildings)
    || !isRecord(value.summary)) {
    return undefined;
  }
  return value as unknown as BuildingTargetCaptureResponse;
}

export interface AutoStormBlueprint {
  id: string;
  name: string;
  createdAt: string;
  updatedAt: string;
  target: BuildingTargetCaptureResponse;
}

export interface AutoStormBlueprintDocument {
  version: 1;
  activeId: string;
  blueprints: Record<string, AutoStormBlueprint>;
}

export function parseAutoStormBlueprintDocument(value: unknown): AutoStormBlueprintDocument {
  const fallback: AutoStormBlueprintDocument = { version: 1, activeId: '', blueprints: {} };
  if (!isRecord(value) || !isRecord(value.blueprints)) return fallback;
  const blueprints: Record<string, AutoStormBlueprint> = {};
  for (const [key, raw] of Object.entries(value.blueprints)) {
    if (!isRecord(raw)) continue;
    const target = parseTarget(raw.target);
    const id = stringValue(raw.id) || key.trim();
    if (!target || !id) continue;
    blueprints[id] = {
      id,
      name: stringValue(raw.name) || blueprintModeLabel(target.mode),
      createdAt: stringValue(raw.createdAt),
      updatedAt: stringValue(raw.updatedAt),
      target,
    };
  }
  const activeId = stringValue(value.activeId);
  return {
    version: 1,
    activeId: blueprints[activeId] ? activeId : '',
    blueprints,
  };
}

function blueprintModeLabel(mode: BuildingTargetCaptureResponse['mode']): string {
  if (mode === 'functional') return 'Functional target';
  if (mode === 'layout' || mode === 'buildings') return 'Layout target';
  return 'Exact clone';
}

function parseDefenseUnits(value: unknown): AutoStormDefenseUnit[] {
  if (!Array.isArray(value)) return [];
  const byID = new Map<number, number>();
  for (const row of value) {
    if (!isRecord(row)) continue;
    const unitId = positiveInteger(row.unitId);
    const amount = positiveInteger(row.amount);
    if (unitId > 0 && amount > 0) byID.set(unitId, amount);
  }
  return Array.from(byID, ([unitId, amount]) => ({ unitId, amount })).slice(0, 8);
}

function parseShopPurchases(value: unknown): AutoStormShopPurchase[] {
  if (!Array.isArray(value)) return [];
  const result: AutoStormShopPurchase[] = [];
  for (const row of value) {
    if (!isRecord(row)) continue;
    const packageId = positiveInteger(row.packageId);
    if (packageId <= 0 || !AUTO_STORM_LUNA_PACKAGE_ID_SET.has(packageId)) continue;
    result.push({
      packageId,
      targetPurchases: clampAutoStormInteger(row.targetPurchases, 1, Number.MAX_SAFE_INTEGER, 1),
      unlimited: row.unlimited === true,
      priority: clampAutoStormInteger(row.priority, 0, 1_000_000, result.length + 1),
    });
  }
  return result;
}

function parseNumberMap(value: unknown): Record<string, number> {
  if (!isRecord(value)) return {};
  const result: Record<string, number> = {};
  for (const [key, raw] of Object.entries(value)) {
    const amount = clampAutoStormInteger(raw, 0, Number.MAX_SAFE_INTEGER, 0);
    if (amount > 0) result[key.trim()] = amount;
  }
  return result;
}

function parsePositiveIntegerArray(value: unknown): number[] {
  if (!Array.isArray(value)) return [];
  const result: number[] = [];
  const seen = new Set<number>();
  for (const raw of value) {
    const parsed = positiveInteger(raw);
    if (parsed <= 0 || seen.has(parsed)) continue;
    seen.add(parsed);
    result.push(parsed);
  }
  return result;
}

function parseTargetPriority(value: unknown, legacyCombatOrder: unknown): AutoStormTargetPriority[] {
  const fallback: AutoStormTargetPriority[] = legacyCombatOrder === 'islands_first'
    ? ['island:large', 'island:small', 'fort:80', 'fort:70', 'fort:60', 'fort:50', 'fort:40']
    : [...AUTO_STORM_TARGET_PRIORITIES];
  if (!Array.isArray(value)) return fallback;

  const allowed = new Set<string>(AUTO_STORM_TARGET_PRIORITIES);
  const result: AutoStormTargetPriority[] = [];
  const seen = new Set<string>();
  const append = (candidate: unknown) => {
    if (typeof candidate !== 'string' || !allowed.has(candidate) || seen.has(candidate)) return;
    seen.add(candidate);
    result.push(candidate as AutoStormTargetPriority);
  };
  value.forEach(append);
  fallback.forEach(append);
  return result;
}

function parseChoiceArray<T extends string | number>(value: unknown, allowed: readonly T[]): T[] {
  if (!Array.isArray(value)) return [...allowed];
  const selected = new Set(value);
  return allowed.filter((choice) => selected.has(choice));
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

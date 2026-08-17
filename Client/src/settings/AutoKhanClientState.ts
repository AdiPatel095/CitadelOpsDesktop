import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_KHAN_SECTION = 'automation.autoKhan';

export interface AutoKhanClientStateV1 {
  version: 1;
  sourceCastleId: number;
  attackPresetId: string;
  defensePresetId: string;
  minimumRemainingSec: number;
  checkIntervalSec: number;
  defenseRefreshIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  attackLaunchesEnabled: boolean;
  triggerRage: boolean;
  skipCooldowns: boolean;
  timeSkipReserve: Record<string, number>;
  openGateProtection: boolean;
  offensiveUnitThreshold: number;
  horseTravelBoostId: HorseTravelBoostID;
  nomadPointThreshold: number;
  replenishDefenseTools: boolean;
}

export function defaultAutoKhanClientState(): AutoKhanClientStateV1 {
  return {
    version: 1,
    sourceCastleId: 0,
    attackPresetId: '',
    defensePresetId: '',
    minimumRemainingSec: 300,
    checkIntervalSec: 30,
    defenseRefreshIntervalSec: 30,
    mapRefreshIntervalSec: 30,
    dailyAttackLimit: 0,
    attackLaunchesEnabled: true,
    triggerRage: true,
    skipCooldowns: true,
    timeSkipReserve: {},
    openGateProtection: true,
    offensiveUnitThreshold: 1000,
    horseTravelBoostId: -1,
    nomadPointThreshold: 0,
    replenishDefenseTools: false,
  };
}

export function parseAutoKhanClientState(value: unknown): AutoKhanClientStateV1 {
  const fallback = defaultAutoKhanClientState();
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fallback;
  const raw = value as Record<string, unknown>;
  const rawReserve = raw.timeSkipReserve && typeof raw.timeSkipReserve === 'object' && !Array.isArray(raw.timeSkipReserve)
    ? raw.timeSkipReserve as Record<string, unknown>
    : {};
  return {
    version: 1,
    sourceCastleId: positiveInteger(raw.sourceCastleId),
    attackPresetId: typeof raw.attackPresetId === 'string' ? raw.attackPresetId.trim() : '',
    defensePresetId: typeof raw.defensePresetId === 'string' ? raw.defensePresetId.trim() : '',
    minimumRemainingSec: clampAutoKhanInteger(raw.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    checkIntervalSec: clampAutoKhanInteger(raw.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    defenseRefreshIntervalSec: clampAutoKhanInteger(
      raw.defenseRefreshIntervalSec,
      30,
      3600,
      fallback.defenseRefreshIntervalSec,
    ),
    mapRefreshIntervalSec: clampAutoKhanInteger(raw.mapRefreshIntervalSec, 30, 3600, fallback.mapRefreshIntervalSec),
    dailyAttackLimit: clampAutoKhanInteger(raw.dailyAttackLimit, 0, Number.MAX_SAFE_INTEGER, 0),
    attackLaunchesEnabled: raw.attackLaunchesEnabled !== false,
    triggerRage: raw.triggerRage !== false,
    skipCooldowns: raw.skipCooldowns !== false,
    timeSkipReserve: Object.fromEntries(
      ['MS1', 'MS2', 'MS3', 'MS4', 'MS5', 'MS6', 'MS7'].map((key) => [
        key,
        clampAutoKhanInteger(rawReserve[key], 0, Number.MAX_SAFE_INTEGER, 0),
      ]),
    ),
    openGateProtection: raw.openGateProtection !== false,
    offensiveUnitThreshold: clampAutoKhanInteger(
      raw.offensiveUnitThreshold,
      1,
      Number.MAX_SAFE_INTEGER,
      fallback.offensiveUnitThreshold,
    ),
    horseTravelBoostId: parseHorseTravelBoostID(raw.horseTravelBoostId),
    nomadPointThreshold: clampAutoKhanInteger(raw.nomadPointThreshold, 0, Number.MAX_SAFE_INTEGER, 0),
    replenishDefenseTools: raw.replenishDefenseTools === true,
  };
}

export function clampAutoKhanInteger(
  value: unknown,
  minimum: number,
  maximum: number,
  fallback: number,
): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

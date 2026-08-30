import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_NOMAD_SECTION = 'automation.autoNomad';

export interface AutoNomadRBCTestState {
  enabled: boolean;
  runId: string;
  targetX: number;
  targetY: number;
}

export interface AutoNomadClientStateV5 {
  version: 5;
  sourceCastleId: number;
  nomadPresetId: string;
  samuraiPresetId: string;
  nomadDifficultyId: number;
  samuraiDifficultyId: number;
  scoreTarget: number;
  minimumRemainingSec: number;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  skipCooldowns: boolean;
  timeSkipReserve: Record<string, number>;
  rbcTest: AutoNomadRBCTestState;
  horseTravelBoostId: HorseTravelBoostID;
}

export function defaultAutoNomadClientState(): AutoNomadClientStateV5 {
  return {
    version: 5,
    sourceCastleId: 0,
    nomadPresetId: '',
    samuraiPresetId: '',
    nomadDifficultyId: 0,
    samuraiDifficultyId: 0,
    scoreTarget: 0,
    minimumRemainingSec: 1800,
    checkIntervalSec: 30,
    mapRefreshIntervalSec: 300,
    dailyAttackLimit: 0,
    skipCooldowns: false,
    timeSkipReserve: {},
    rbcTest: { enabled: false, runId: '', targetX: 0, targetY: 0 },
    horseTravelBoostId: -1,
  };
}

export function parseAutoNomadClientState(value: unknown): AutoNomadClientStateV5 {
  const fallback = defaultAutoNomadClientState();
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fallback;
  const raw = value as Record<string, unknown>;
  const legacyPresetId = trimmedString(raw.presetId);
  const rawReserve = raw.timeSkipReserve && typeof raw.timeSkipReserve === 'object' && !Array.isArray(raw.timeSkipReserve)
    ? raw.timeSkipReserve as Record<string, unknown>
    : {};
  const rbcTest = raw.rbcTest && typeof raw.rbcTest === 'object' && !Array.isArray(raw.rbcTest)
    ? raw.rbcTest as Record<string, unknown>
    : {};
  return {
    version: 5,
    sourceCastleId: positiveInteger(raw.sourceCastleId),
    nomadPresetId: trimmedString(raw.nomadPresetId) || legacyPresetId,
    samuraiPresetId: trimmedString(raw.samuraiPresetId) || legacyPresetId,
    nomadDifficultyId: positiveInteger(raw.nomadDifficultyId),
    samuraiDifficultyId: positiveInteger(raw.samuraiDifficultyId),
    scoreTarget: positiveInteger(raw.scoreTarget),
    minimumRemainingSec: clampAutoNomadInteger(raw.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    checkIntervalSec: clampAutoNomadInteger(raw.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampAutoNomadInteger(raw.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
    dailyAttackLimit: clampAutoNomadInteger(raw.dailyAttackLimit, 0, Number.MAX_SAFE_INTEGER, 0),
    skipCooldowns: raw.skipCooldowns === true,
    timeSkipReserve: Object.fromEntries(
      ['MS1', 'MS2', 'MS3', 'MS4', 'MS5', 'MS6', 'MS7'].map((key) => [
        key,
        clampAutoNomadInteger(rawReserve[key], 0, Number.MAX_SAFE_INTEGER, 0),
      ]),
    ),
    rbcTest: {
      enabled: rbcTest.enabled === true,
      runId: typeof rbcTest.runId === 'string' ? rbcTest.runId.trim() : '',
      targetX: clampAutoNomadInteger(rbcTest.targetX, 0, 2000, 0),
      targetY: clampAutoNomadInteger(rbcTest.targetY, 0, 2000, 0),
    },
    horseTravelBoostId: parseHorseTravelBoostID(raw.horseTravelBoostId),
  };
}

export function clampAutoNomadInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

function trimmedString(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

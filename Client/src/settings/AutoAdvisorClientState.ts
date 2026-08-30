import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_ADVISOR_SECTION = 'automation.autoAdvisor';
export const AUTO_ADVISOR_MAX_ATTACKS = 9999;
export const AUTO_ADVISOR_SKIP_KEYS = ['MS1', 'MS2', 'MS3', 'MS4', 'MS5', 'MS6', 'MS7'] as const;

export interface AutoAdvisorClientStateV1 {
  version: 1;
  sourceCastleId: number;
  presetId: string;
  nomadDifficultyId: number;
  samuraiDifficultyId: number;
  maxAttackCount: number;
  minimumRemainingSec: number;
  coinCostPerAttack: number;
  minimumCoinReserve: number;
  rubyCostPerAttack: number;
  minimumRubyReserve: number;
  minimumFeatherReserve: number;
  timeSkipReserve: Record<string, number>;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  horseTravelBoostId: HorseTravelBoostID;
}

export function defaultAutoAdvisorClientState(): AutoAdvisorClientStateV1 {
  return {
    version: 1,
    sourceCastleId: 0,
    presetId: '',
    nomadDifficultyId: 0,
    samuraiDifficultyId: 0,
    maxAttackCount: AUTO_ADVISOR_MAX_ATTACKS,
    minimumRemainingSec: 1800,
    coinCostPerAttack: 500,
    minimumCoinReserve: 0,
    rubyCostPerAttack: 0,
    minimumRubyReserve: 0,
    minimumFeatherReserve: 0,
    timeSkipReserve: {},
    checkIntervalSec: 30,
    mapRefreshIntervalSec: 300,
    horseTravelBoostId: -1,
  };
}

export function parseAutoAdvisorClientState(value: unknown): AutoAdvisorClientStateV1 {
  const fallback = defaultAutoAdvisorClientState();
  if (!isRecord(value)) return fallback;
  const reserve = isRecord(value.timeSkipReserve) ? value.timeSkipReserve : {};
  return {
    version: 1,
    sourceCastleId: clampAutoAdvisorInteger(value.sourceCastleId, 0, Number.MAX_SAFE_INTEGER, 0),
    presetId: typeof value.presetId === 'string' ? value.presetId.trim() : '',
    nomadDifficultyId: validDifficulty(value.nomadDifficultyId, 301),
    samuraiDifficultyId: validDifficulty(value.samuraiDifficultyId, 201),
    maxAttackCount: clampAutoAdvisorInteger(value.maxAttackCount, 1, AUTO_ADVISOR_MAX_ATTACKS, fallback.maxAttackCount),
    minimumRemainingSec: clampAutoAdvisorInteger(value.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    coinCostPerAttack: clampAutoAdvisorInteger(value.coinCostPerAttack, 1, Number.MAX_SAFE_INTEGER, fallback.coinCostPerAttack),
    minimumCoinReserve: clampAutoAdvisorInteger(value.minimumCoinReserve, 0, Number.MAX_SAFE_INTEGER, 0),
    rubyCostPerAttack: clampAutoAdvisorInteger(value.rubyCostPerAttack, 0, Number.MAX_SAFE_INTEGER, 0),
    minimumRubyReserve: clampAutoAdvisorInteger(value.minimumRubyReserve, 0, Number.MAX_SAFE_INTEGER, 0),
    minimumFeatherReserve: clampAutoAdvisorInteger(value.minimumFeatherReserve, 0, Number.MAX_SAFE_INTEGER, 0),
    timeSkipReserve: Object.fromEntries(AUTO_ADVISOR_SKIP_KEYS.map((key) => [
      key,
      clampAutoAdvisorInteger(reserve[key], 0, Number.MAX_SAFE_INTEGER, 0),
    ])),
    checkIntervalSec: clampAutoAdvisorInteger(value.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampAutoAdvisorInteger(value.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
    horseTravelBoostId: parseHorseTravelBoostID(value.horseTravelBoostId),
  };
}

export function clampAutoAdvisorInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function validDifficulty(value: unknown, firstDifficultyId: number): number {
  const parsed = clampAutoAdvisorInteger(value, 0, Number.MAX_SAFE_INTEGER, 0);
  return parsed >= firstDifficultyId && parsed <= firstDifficultyId + 10 ? parsed : 0;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

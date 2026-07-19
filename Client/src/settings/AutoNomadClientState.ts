import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_NOMAD_SECTION = 'automation.autoNomad';

export interface AutoNomadRBCTestState {
  enabled: boolean;
  runId: string;
  targetX: number;
  targetY: number;
}

export interface AutoNomadClientStateV4 {
  version: 4;
  sourceCastleId: number;
  presetId: string;
  nomadDifficultyId: number;
  samuraiDifficultyId: number;
  scoreTarget: number;
  minimumRemainingSec: number;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  skipCooldowns: boolean;
  timeSkipReserve: Record<string, number>;
  rbcTest: AutoNomadRBCTestState;
  horseTravelBoostId: HorseTravelBoostID;
}

export function defaultAutoNomadClientState(): AutoNomadClientStateV4 {
  return {
    version: 4,
    sourceCastleId: 0,
    presetId: '',
    nomadDifficultyId: 0,
    samuraiDifficultyId: 0,
    scoreTarget: 0,
    minimumRemainingSec: 1800,
    checkIntervalSec: 30,
    mapRefreshIntervalSec: 300,
    skipCooldowns: false,
    timeSkipReserve: {},
    rbcTest: { enabled: false, runId: '', targetX: 0, targetY: 0 },
    horseTravelBoostId: -1,
  };
}

export function parseAutoNomadClientState(value: unknown): AutoNomadClientStateV4 {
  const fallback = defaultAutoNomadClientState();
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fallback;
  const raw = value as Record<string, unknown>;
  const rawReserve = raw.timeSkipReserve && typeof raw.timeSkipReserve === 'object' && !Array.isArray(raw.timeSkipReserve)
    ? raw.timeSkipReserve as Record<string, unknown>
    : {};
  const rbcTest = raw.rbcTest && typeof raw.rbcTest === 'object' && !Array.isArray(raw.rbcTest)
    ? raw.rbcTest as Record<string, unknown>
    : {};
  return {
    version: 4,
    sourceCastleId: positiveInteger(raw.sourceCastleId),
    presetId: typeof raw.presetId === 'string' ? raw.presetId.trim() : '',
    nomadDifficultyId: validDifficulty(raw.nomadDifficultyId, 301),
    samuraiDifficultyId: validDifficulty(raw.samuraiDifficultyId, 201),
    scoreTarget: positiveInteger(raw.scoreTarget),
    minimumRemainingSec: clampAutoNomadInteger(raw.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    checkIntervalSec: clampAutoNomadInteger(raw.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampAutoNomadInteger(raw.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
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

export const AUTO_NOMAD_DIFFICULTY_NAMES = [
  'Easy',
  'Easy+',
  'Intermediate',
  'Intermediate+',
  'Hard',
  'Hard+',
  'Expert',
  'Expert+',
  'Master',
  'Master+',
  'Archmaster',
] as const;

export function autoNomadDifficultyOptions(
  firstDifficultyId: number,
  firstUnlockAchievementId: number,
  completedAchievements: Record<string, boolean>,
): Array<{ value: string; label: string }> {
  return AUTO_NOMAD_DIFFICULTY_NAMES.flatMap((name, index) => {
    if (index < 4) return [{ value: String(firstDifficultyId + index), label: name }];
    if (index >= 10 || !completedAchievements[String(firstUnlockAchievementId + index - 4)]) return [];
    return [{ value: String(firstDifficultyId + index), label: name }];
  });
}

export function autoNomadDifficultyName(difficultyId: number, firstDifficultyId: number): string {
  return AUTO_NOMAD_DIFFICULTY_NAMES[difficultyId - firstDifficultyId] ?? 'Unknown';
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

function validDifficulty(value: unknown, firstDifficultyId: number): number {
  const parsed = positiveInteger(value);
  return parsed >= firstDifficultyId && parsed <= firstDifficultyId + 10 ? parsed : 0;
}

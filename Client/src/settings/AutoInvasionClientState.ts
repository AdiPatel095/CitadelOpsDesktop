import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_INVASION_SECTION = 'automation.autoInvasion';

export interface AutoInvasionClientStateV1 {
  version: 1;
  sourceCastleId: number;
  presetId: string;
  foreignLordsDifficultyId: number;
  bloodcrowDifficultyId: number;
  scoreTarget: number;
  minimumRemainingSec: number;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
	fortifyCurrency: AutoInvasionFortifyCurrency;
  horseTravelBoostId: HorseTravelBoostID;
}

export type AutoInvasionFortifyCurrency = '' | 'GTO' | 'STO' | 'KM' | 'C2';

export function defaultAutoInvasionClientState(): AutoInvasionClientStateV1 {
  return {
    version: 1,
    sourceCastleId: 0,
    presetId: '',
    foreignLordsDifficultyId: 0,
    bloodcrowDifficultyId: 0,
    scoreTarget: 0,
    minimumRemainingSec: 1800,
    checkIntervalSec: 30,
    mapRefreshIntervalSec: 300,
		fortifyCurrency: '',
    horseTravelBoostId: -1,
  };
}

export function parseAutoInvasionClientState(value: unknown): AutoInvasionClientStateV1 {
  const fallback = defaultAutoInvasionClientState();
  if (!value || typeof value !== 'object' || Array.isArray(value)) return fallback;
  const raw = value as Record<string, unknown>;
  return {
    version: 1,
    sourceCastleId: positiveInteger(raw.sourceCastleId),
    presetId: typeof raw.presetId === 'string' ? raw.presetId.trim() : '',
    foreignLordsDifficultyId: validEventDifficulty(raw.foreignLordsDifficultyId, 1),
    bloodcrowDifficultyId: validEventDifficulty(raw.bloodcrowDifficultyId, 101),
    scoreTarget: positiveInteger(raw.scoreTarget),
    minimumRemainingSec: clampInteger(raw.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    checkIntervalSec: clampInteger(raw.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampInteger(raw.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
		fortifyCurrency: validFortifyCurrency(raw.fortifyCurrency),
    horseTravelBoostId: parseHorseTravelBoostID(raw.horseTravelBoostId),
  };
}

function validFortifyCurrency(value: unknown): AutoInvasionFortifyCurrency {
	return value === 'GTO' || value === 'STO' || value === 'KM' || value === 'C2' ? value : '';
}

export const AUTO_INVASION_DIFFICULTY_NAMES = [
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

export function autoInvasionDifficultyOptions(
  firstDifficultyId: number,
  completedAchievements: Record<string, boolean>,
): Array<{ value: string; label: string }> {
  const firstUnlockAchievementId = firstDifficultyId === 1 ? 1084 : 1090;
  return AUTO_INVASION_DIFFICULTY_NAMES.flatMap((name, index) => {
    if (index < 4) return [{ value: String(firstDifficultyId + index), label: name }];
    if (index >= 10 || !completedAchievements[String(firstUnlockAchievementId + index - 4)]) return [];
    return [{ value: String(firstDifficultyId + index), label: name }];
  });
}

export function autoInvasionDifficultyName(difficultyId: number, firstDifficultyId: number): string {
  return AUTO_INVASION_DIFFICULTY_NAMES[difficultyId - firstDifficultyId] ?? 'Unknown';
}

export function clampAutoInvasionInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  return clampInteger(value, minimum, maximum, fallback);
}

function clampInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

function validEventDifficulty(value: unknown, firstDifficultyId: number): number {
  const parsed = positiveInteger(value);
  return parsed >= firstDifficultyId && parsed <= firstDifficultyId + 10 ? parsed : 0;
}

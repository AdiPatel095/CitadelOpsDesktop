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
	dailyAttackLimit: number;
	fortifyCurrency: AutoInvasionFortifyCurrency;
  horseTravelBoostId: HorseTravelBoostID;
}

export type AutoInvasionFortifyCurrency = '' | 'GTO' | 'STO' | 'MEDALS' | 'C2';

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
		dailyAttackLimit: 0,
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
    foreignLordsDifficultyId: positiveInteger(raw.foreignLordsDifficultyId),
    bloodcrowDifficultyId: positiveInteger(raw.bloodcrowDifficultyId),
    scoreTarget: positiveInteger(raw.scoreTarget),
    minimumRemainingSec: clampInteger(raw.minimumRemainingSec, 0, 86400, fallback.minimumRemainingSec),
    checkIntervalSec: clampInteger(raw.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampInteger(raw.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
		dailyAttackLimit: clampInteger(raw.dailyAttackLimit, 0, Number.MAX_SAFE_INTEGER, 0),
		fortifyCurrency: validFortifyCurrency(raw.fortifyCurrency),
    horseTravelBoostId: parseHorseTravelBoostID(raw.horseTravelBoostId),
  };
}

function validFortifyCurrency(value: unknown): AutoInvasionFortifyCurrency {
	if (value === 'KM' || value === 'ST' || value === 'MEDALS') return 'MEDALS';
	return value === 'GTO' || value === 'STO' || value === 'C2' ? value : '';
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

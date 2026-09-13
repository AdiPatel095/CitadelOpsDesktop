import { queueConfigurationUpdate } from './Configuration';
import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export interface AutoTowerCastleSettings {
	enabled: boolean;
	radius: number;
	unitId: number;
  maidenOnly: boolean;
}

export const AUTO_TOWER_MAXIMUM_DAILY_TIME_SKIPS = 9998;

export interface AutoTowerClientStateV4 {
  version: 4;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  horseTravelBoostId: HorseTravelBoostID;
  useAdvisor: boolean;
  autoActivateAdvisor: boolean;
  maximumDailyTimeSkips: number;
  castles: Record<string, AutoTowerCastleSettings>;
}

export const defaultAutoTowerCastleSettings = (): AutoTowerCastleSettings => ({
	enabled: false,
	radius: 10,
  unitId: 0,
  maidenOnly: false,
});

export const defaultAutoTowerClientState = (): AutoTowerClientStateV4 => ({
	version: 4,
	checkIntervalSec: 30,
	mapRefreshIntervalSec: 1800,
  dailyAttackLimit: 0,
  horseTravelBoostId: -1,
  useAdvisor: false,
  autoActivateAdvisor: false,
  maximumDailyTimeSkips: 0,
  castles: {},
});

export function parseAutoTowerClientState(raw: unknown): AutoTowerClientStateV4 {
  const fallback = defaultAutoTowerClientState();
  if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) return fallback;
  const document = raw as Record<string, unknown>;
  const castles: Record<string, AutoTowerCastleSettings> = {};
  if (document.castles && typeof document.castles === 'object' && !Array.isArray(document.castles)) {
    for (const [castleId, value] of Object.entries(document.castles as Record<string, unknown>)) {
      if (!/^\d+$/.test(castleId) || value == null || typeof value !== 'object' || Array.isArray(value)) continue;
      const candidate = value as Partial<AutoTowerCastleSettings>;
      castles[castleId] = {
        enabled: candidate.enabled === true,
			radius: clampRadius(candidate.radius),
			unitId: positiveInteger(candidate.unitId),
        maidenOnly: candidate.maidenOnly === true,
      };
    }
  }
  return {
    version: 4,
    checkIntervalSec: clampInterval(document.checkIntervalSec, fallback.checkIntervalSec),
		mapRefreshIntervalSec: clampMapRefreshInterval(document.mapRefreshIntervalSec),
    dailyAttackLimit: positiveInteger(document.dailyAttackLimit),
    horseTravelBoostId: parseHorseTravelBoostID(document.horseTravelBoostId),
    useAdvisor: document.useAdvisor === true,
    autoActivateAdvisor: document.autoActivateAdvisor === true,
    maximumDailyTimeSkips: clampMaximumDailyTimeSkips(document.maximumDailyTimeSkips),
    castles,
  };
}

export function persistAutoTowerClientState(state: AutoTowerClientStateV4) {
  return queueConfigurationUpdate('automation.autoTowers', state);
}

export function clampMaximumDailyTimeSkips(value: unknown): number {
  return Math.min(AUTO_TOWER_MAXIMUM_DAILY_TIME_SKIPS, positiveInteger(value));
}

export function clampRadius(value: unknown): number {
  const numeric = positiveInteger(value);
  return Math.min(50, Math.max(1, numeric || 10));
}

export function clampMapRefreshInterval(value: unknown): number {
	const numeric = positiveInteger(value);
	if (numeric === 0) return 1800;
	return Math.min(3600, Math.max(1800, numeric));
}

function clampInterval(value: unknown, fallback: number): number {
  const numeric = positiveInteger(value);
  return Math.min(3600, Math.max(30, numeric || fallback));
}

function positiveInteger(value: unknown): number {
  const numeric = Number(value);
  return Number.isFinite(numeric) && numeric > 0 ? Math.trunc(numeric) : 0;
}

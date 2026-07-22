import { queueConfigurationUpdate } from './Configuration';
import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export interface AutoTowerCastleSettings {
	enabled: boolean;
	radius: number;
	unitId: number;
  maidenOnly: boolean;
}

export interface AutoTowerClientStateV2 {
  version: 2;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  horseTravelBoostId: HorseTravelBoostID;
  castles: Record<string, AutoTowerCastleSettings>;
}

export const defaultAutoTowerCastleSettings = (): AutoTowerCastleSettings => ({
	enabled: false,
	radius: 10,
  unitId: 0,
  maidenOnly: false,
});

export const defaultAutoTowerClientState = (): AutoTowerClientStateV2 => ({
	version: 2,
	checkIntervalSec: 30,
	mapRefreshIntervalSec: 1800,
  dailyAttackLimit: 0,
  horseTravelBoostId: -1,
  castles: {},
});

export function parseAutoTowerClientState(raw: unknown): AutoTowerClientStateV2 {
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
    version: 2,
    checkIntervalSec: clampInterval(document.checkIntervalSec, fallback.checkIntervalSec),
		mapRefreshIntervalSec: clampMapRefreshInterval(document.mapRefreshIntervalSec),
    dailyAttackLimit: positiveInteger(document.dailyAttackLimit),
    horseTravelBoostId: parseHorseTravelBoostID(document.horseTravelBoostId),
    castles,
  };
}

export function persistAutoTowerClientState(state: AutoTowerClientStateV2): void {
  queueConfigurationUpdate('automation.autoTowers', state);
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

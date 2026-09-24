import { queueConfigurationUpdate } from './Configuration';
import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export const AUTO_FORTRESS_SECTION = 'automation.autoFortress';
export const AUTO_FORTRESS_DIREWOLF_ID = 277;

export interface AutoFortressKingdomSettings {
  enabled: boolean;
}

export interface AutoFortressClientStateV1 {
  version: 1;
  checkIntervalSec: number;
  mapRefreshIntervalSec: number;
  dailyAttackLimit: number;
  horseTravelBoostId: HorseTravelBoostID;
  minimumCommanderSpeedBonus: 100;
  direwolfPurchaseLimit: number;
  minimumTabletReserve: number;
  useTimeSkips: boolean;
  timeSkipReserve: Record<string, number>;
  kingdoms: Record<string, AutoFortressKingdomSettings>;
}

export const defaultAutoFortressClientState = (): AutoFortressClientStateV1 => ({
  version: 1,
  checkIntervalSec: 5,
  mapRefreshIntervalSec: 1800,
  dailyAttackLimit: 0,
  horseTravelBoostId: -1,
  minimumCommanderSpeedBonus: 100,
  direwolfPurchaseLimit: 0,
  minimumTabletReserve: 0,
  useTimeSkips: false,
  timeSkipReserve: {},
  kingdoms: {
    '1': { enabled: false },
    '2': { enabled: false },
    '3': { enabled: false },
  },
});

export function parseAutoFortressClientState(raw: unknown): AutoFortressClientStateV1 {
  const fallback = defaultAutoFortressClientState();
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return fallback;
  const document = raw as Record<string, unknown>;
  const rawKingdoms = isRecord(document.kingdoms) ? document.kingdoms : {};
  return {
    version: 1,
    checkIntervalSec: clampInteger(document.checkIntervalSec, 1, 3600, fallback.checkIntervalSec),
    mapRefreshIntervalSec: clampInteger(document.mapRefreshIntervalSec, 60, 3600, fallback.mapRefreshIntervalSec),
    dailyAttackLimit: clampInteger(document.dailyAttackLimit, 0, 100_000, 0),
    horseTravelBoostId: parseHorseTravelBoostID(document.horseTravelBoostId ?? fallback.horseTravelBoostId),
    minimumCommanderSpeedBonus: 100,
    direwolfPurchaseLimit: clampHundreds(document.direwolfPurchaseLimit),
    minimumTabletReserve: clampInteger(document.minimumTabletReserve, 0, Number.MAX_SAFE_INTEGER, 0),
    useTimeSkips: document.useTimeSkips === true,
    timeSkipReserve: parseTimeSkipReserve(document.timeSkipReserve),
    kingdoms: Object.fromEntries(['1', '2', '3'].map((kingdomID) => [
      kingdomID,
      { enabled: isRecord(rawKingdoms[kingdomID]) && rawKingdoms[kingdomID].enabled === true },
    ])),
  };
}

export function persistAutoFortressClientState(state: AutoFortressClientStateV1) {
  return queueConfigurationUpdate(AUTO_FORTRESS_SECTION, state);
}

export function clampDirewolfPurchaseLimit(value: unknown): number {
  return clampHundreds(value);
}

function clampHundreds(value: unknown): number {
  const numeric = Number(value);
  if (!Number.isFinite(numeric) || numeric <= 0) return 0;
  return Math.min(100_000, Math.max(100, Math.round(numeric / 100) * 100));
}

function clampInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(numeric)));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value);
}

function parseTimeSkipReserve(value: unknown): Record<string, number> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(Object.entries(value).flatMap(([rawKey, rawAmount]) => {
    const key = rawKey.trim().toUpperCase();
    if (!/^MS\d+$/.test(key)) return [];
    return [[key, clampInteger(rawAmount, 0, Number.MAX_SAFE_INTEGER, 0)]];
  }));
}

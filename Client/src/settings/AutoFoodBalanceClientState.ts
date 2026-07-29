import { parseHorseTravelBoostID, type HorseTravelBoostID } from './HorseTravelBoost';

export interface AutoFoodBalanceSettings {
  checkIntervalSec: number;
  stateRefreshIntervalSec: number;
  logisticsRefreshIntervalSec: number;
  safetyHours: number;
  sourceSafetyHours: number;
  minimumShipmentSize: number;
  minimumSourceReserve: number;
  minimumCoinReserve: number;
  autoKingdomTransport: boolean;
  useKingdomTimeSkips: boolean;
  allowedTimeSkips: string[];
  timeSkipReserve: Record<string, number>;
  horseTravelBoostId: HorseTravelBoostID;
}

export const AUTO_FOOD_BALANCE_TIME_SKIPS = [
  { id: 'MS1', label: '1m' },
  { id: 'MS2', label: '5m' },
  { id: 'MS3', label: '10m' },
  { id: 'MS4', label: '30m' },
  { id: 'MS5', label: '1h' },
  { id: 'MS6', label: '5h' },
  { id: 'MS7', label: '24h' },
] as const;

const AUTO_FOOD_BALANCE_TIME_SKIP_IDS = new Set<string>(
  AUTO_FOOD_BALANCE_TIME_SKIPS.map((skip) => skip.id),
);

export const DEFAULT_AUTO_FOOD_BALANCE_SETTINGS: AutoFoodBalanceSettings = {
  checkIntervalSec: 60,
  stateRefreshIntervalSec: 900,
  logisticsRefreshIntervalSec: 300,
  safetyHours: 8,
  sourceSafetyHours: 24,
  minimumShipmentSize: 1_000,
  minimumSourceReserve: 1_000,
  minimumCoinReserve: 0,
  autoKingdomTransport: true,
  useKingdomTimeSkips: false,
  allowedTimeSkips: [],
  timeSkipReserve: {},
  horseTravelBoostId: -1,
};

export function parseAutoFoodBalanceSettings(payload: unknown): AutoFoodBalanceSettings {
  const value = isRecord(payload) ? payload : {};
  const allowedTimeSkips = Array.isArray(value.allowedTimeSkips)
    ? [...new Set(value.allowedTimeSkips
        .map((entry) => String(entry).trim().toUpperCase())
        .filter((entry) => AUTO_FOOD_BALANCE_TIME_SKIP_IDS.has(entry)))]
    : DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.allowedTimeSkips;
  const timeSkipReserve: Record<string, number> = {};
  if (isRecord(value.timeSkipReserve)) {
    Object.entries(value.timeSkipReserve).forEach(([rawID, reserve]) => {
      const id = rawID.trim().toUpperCase();
      if (!AUTO_FOOD_BALANCE_TIME_SKIP_IDS.has(id)) return;
      timeSkipReserve[id] = integer(reserve, 0, 0, Number.MAX_SAFE_INTEGER);
    });
  }
  return {
    checkIntervalSec: integer(value.checkIntervalSec, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.checkIntervalSec, 30, 3600),
    stateRefreshIntervalSec: integer(value.stateRefreshIntervalSec, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.stateRefreshIntervalSec, 60, 86400),
    logisticsRefreshIntervalSec: integer(value.logisticsRefreshIntervalSec, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.logisticsRefreshIntervalSec, 30, 3600),
    safetyHours: number(value.safetyHours, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.safetyHours, 1, 168),
    sourceSafetyHours: number(value.sourceSafetyHours, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.sourceSafetyHours, 1, 336),
    minimumShipmentSize: integer(value.minimumShipmentSize, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.minimumShipmentSize, 1, Number.MAX_SAFE_INTEGER),
    minimumSourceReserve: number(value.minimumSourceReserve, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.minimumSourceReserve, 0, Number.MAX_SAFE_INTEGER),
    minimumCoinReserve: number(value.minimumCoinReserve, DEFAULT_AUTO_FOOD_BALANCE_SETTINGS.minimumCoinReserve, 0, Number.MAX_SAFE_INTEGER),
    autoKingdomTransport: value.autoKingdomTransport !== false,
    useKingdomTimeSkips: value.useKingdomTimeSkips === true,
    allowedTimeSkips,
    timeSkipReserve,
    horseTravelBoostId: parseHorseTravelBoostID(value.horseTravelBoostId),
  };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

function integer(value: unknown, fallback: number, minimum: number, maximum: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.round(parsed)));
}

function number(value: unknown, fallback: number, minimum: number, maximum: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, parsed));
}

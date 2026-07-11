import { queueConfigurationUpdate } from './Configuration';

export interface AutoStationTroopReserve {
  id: number;
  amount: number;
}

export interface AutoStationClientStateV1 {
  version: 1;
  leadTimeSec: number;
  recallWhenClear: boolean;
  minRPTDays: number;
  settings: Record<string, AutoStationTroopReserve[]>;
}

export const DEFAULT_AUTO_STATION_STATE: AutoStationClientStateV1 = {
  version: 1,
  leadTimeSec: 60,
  recallWhenClear: true,
  minRPTDays: 3,
  settings: {},
};

function clampInteger(value: unknown, fallback: number, min: number, max: number): number {
  const parsed = typeof value === 'number' ? value : Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(max, Math.max(min, Math.round(parsed)));
}

export function parseAutoStationClientState(raw: unknown): AutoStationClientStateV1 {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return { ...DEFAULT_AUTO_STATION_STATE, settings: {} };
  }
  const source = raw as Record<string, unknown>;
  const settings: Record<string, AutoStationTroopReserve[]> = {};
  if (source.settings && typeof source.settings === 'object' && !Array.isArray(source.settings)) {
    Object.entries(source.settings as Record<string, unknown>).forEach(([castleID, value]) => {
      if (!Array.isArray(value) || !/^\d+$/.test(castleID)) return;
      const byUnit = new Map<number, number>();
      value.forEach((entry) => {
        if (!entry || typeof entry !== 'object' || Array.isArray(entry)) return;
        const item = entry as Record<string, unknown>;
        const id = clampInteger(item.id, 0, 0, Number.MAX_SAFE_INTEGER);
        const amount = clampInteger(item.amount, 0, 0, Number.MAX_SAFE_INTEGER);
        if (id > 0) byUnit.set(id, amount);
      });
      settings[castleID] = Array.from(byUnit, ([id, amount]) => ({ id, amount }));
    });
  }
  return {
    version: 1,
    leadTimeSec: clampInteger(source.leadTimeSec, 60, 60, 3600),
    recallWhenClear: source.recallWhenClear !== false,
    minRPTDays: clampInteger(source.minRPTDays, 3, 0, 30),
    settings,
  };
}

export function persistAutoStationClientState(state: AutoStationClientStateV1): void {
  const normalized = parseAutoStationClientState(state);
  queueConfigurationUpdate('automation.autoStation', normalized);
}

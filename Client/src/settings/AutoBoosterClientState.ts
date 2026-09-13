import { queueConfigurationUpdate } from './Configuration';

export const AUTO_BOOSTER_SECTION = 'automation.autoBooster';
export const AUTO_BOOSTER_GLOBAL_EFFECT_ID = 2;
export const AUTO_BOOSTER_RUBY_COST = 2500 as const;

export interface AutoBoosterClientStateV1 {
  version: 1;
  checkIntervalSec: number;
  rubyCostCeiling: 2500;
  minimumRubyReserve: number;
}

export function defaultAutoBoosterClientState(): AutoBoosterClientStateV1 {
  return {
    version: 1,
    checkIntervalSec: 60,
    rubyCostCeiling: AUTO_BOOSTER_RUBY_COST,
    minimumRubyReserve: 0,
  };
}

export function parseAutoBoosterClientState(value: unknown): AutoBoosterClientStateV1 {
  const fallback = defaultAutoBoosterClientState();
  if (!isRecord(value)) return fallback;
  return {
    version: 1,
    checkIntervalSec: clampInteger(value.checkIntervalSec, 30, 3600, fallback.checkIntervalSec),
    rubyCostCeiling: AUTO_BOOSTER_RUBY_COST,
    minimumRubyReserve: clampInteger(value.minimumRubyReserve, 0, Number.MAX_SAFE_INTEGER, 0),
  };
}

export function persistAutoBoosterClientState(state: AutoBoosterClientStateV1) {
  return queueConfigurationUpdate(AUTO_BOOSTER_SECTION, state);
}

function clampInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const numeric = Number(value);
  if (!Number.isFinite(numeric)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(numeric)));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value);
}

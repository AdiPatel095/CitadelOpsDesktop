import type { AttackLaunchRatesV2 } from '../../api/Contracts';

export type CountView =
  | { kind: 'unknown' }
  | { kind: 'count'; count: number; window: 'hour' | 'day' | 'since'; since?: string };

export function rateView(rates: AttackLaunchRatesV2 | null | undefined, featureId: string, offline: boolean): CountView {
  if (offline || rates?.launchesByFeature == null) return { kind: 'unknown' };
  const start = rates.windowStartedAt;
  const partial = start !== undefined && Date.parse(start) > Date.parse(rates.observedAt) - 60 * 60 * 1000;
  return { kind: 'count', count: rates.launchesByFeature[featureId] ?? 0,
    window: partial ? 'since' : 'hour', ...(partial ? { since: start } : {}) };
}

export function dailyView(rates: AttackLaunchRatesV2 | null | undefined, featureId: string, offline: boolean): CountView {
  const session = rates?.dailySession;
  if (offline || session?.launchesByFeature == null) return { kind: 'unknown' };
  return { kind: 'count', count: session.launchesByFeature[featureId] ?? 0,
    window: session.window ?? 'day', since: session.startedAt };
}

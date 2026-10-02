import { timedRemainingParameters } from '../../i18n/automationDuration';

export type TimedRunState = { kind: 'idle' } | { kind: 'active'; duration: string };

export function timedRunState(expiresAt: number | undefined, now: number, locale: string): TimedRunState {
  if (expiresAt !== undefined && expiresAt > now) {
    return { kind: 'active', duration: timedRemainingParameters(expiresAt, now, locale).duration };
  }
  return { kind: 'idle' };
}

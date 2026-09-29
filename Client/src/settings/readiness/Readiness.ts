import type { MessageKey, MessageParameters } from '../../i18n/messages';

/**
 * Readiness model (CIT-14 plan §2.5). Evaluators are pure and never send game
 * actions; the runtime stays authoritative.
 * - `valid`: satisfied by saved settings and current observations;
 * - `blocked`: the user must change something;
 * - `pending`: decided by the runtime at Start or while running;
 * - `unavailable`: the observation needed to decide has not arrived.
 */
export type CheckState = 'valid' | 'blocked' | 'pending' | 'unavailable';

/** Where an in-context fix lives. */
export type ReadinessFix = 'settings' | 'assignment' | 'connection' | 'presets';

export interface ReadinessCheck {
  id: string;
  state: CheckState;
  messageKey: MessageKey;
  params?: MessageParameters;
  fix?: ReadinessFix;
  /** Module slot this check belongs to, when it is slot-specific. */
  slot?: string;
}

export interface ReadinessReport {
  featureId: string;
  checks: ReadinessCheck[];
  overall: CheckState;
}

const severity: Record<CheckState, number> = { valid: 0, pending: 1, unavailable: 2, blocked: 3 };

/** Worst state wins: blocked > unavailable > pending > valid. */
export function aggregateReadiness(checks: readonly ReadinessCheck[]): CheckState {
  let overall: CheckState = 'valid';
  for (const check of checks) {
    if (severity[check.state] > severity[overall]) overall = check.state;
  }
  return overall;
}

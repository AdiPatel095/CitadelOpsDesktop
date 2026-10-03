import type { CastleStateV2, SessionStateV2 } from '../../api/Contracts';
import type { MessageKey } from '../../i18n/messages';

/**
 * Whether a castle's stationed-unit counts may be treated as current (CIT-15 D1).
 *
 * The dashboard projection zeroes `unitsObservedAt`, which Go serializes as
 * "0001-01-01T00:00:00Z", so no per-castle unit time reaches the client today.
 * Freshness therefore follows the runtime's own session rule: counts are
 * current once this connection has its baseline (`baselineGeneration ===
 * generation`, reconciled after `changedAt`). A real per-castle timestamp is
 * still honoured if a runtime supplies one.
 */

export type ObservationUnavailableReason = 'disconnected' | 'checkpoint' | 'awaiting-baseline' | 'stale-before-connection';

export type ObservationFreshness =
  /** `castle`: a real per-castle time; `session`: the baseline of this connection. */
  | { state: 'observed'; scope: 'castle' | 'session'; observedAt?: string; since?: string }
  | { state: 'unavailable'; reason: ObservationUnavailableReason; observedAt?: string; since?: string };

/** Hosted runtime presence (structural subset of `config/Deployment` on hosted; desktop passes nothing). */
export interface ObservationPresence {
  mode: 'live' | 'checkpoint';
  checkpointObservedAt?: string;
}

export interface ObservationContext {
  session: SessionStateV2 | null;
  /** Game connection usable: Connected && loggedIn && socketReady (AuthContext.gameLoggedIn). */
  connected: boolean;
  hostedPresence?: ObservationPresence;
}

/** A usable timestamp, or undefined for missing, unparsable or zero-time ("0001-01-01…", year <= 1) values. */
export function observationTimestamp(value: string | undefined | null): string | undefined {
  if (typeof value !== 'string' || !value.trim()) return undefined;
  const parsed = Date.parse(value);
  if (!Number.isFinite(parsed)) return undefined;
  if (new Date(parsed).getUTCFullYear() <= 1) return undefined;
  return value;
}

export function unitObservationFreshness(input: ObservationContext & {
  castle: Pick<CastleStateV2, 'unitsObservedAt'> | null;
}): ObservationFreshness {
  const { session, hostedPresence } = input;
  if (hostedPresence?.mode === 'checkpoint') {
    return { state: 'unavailable', reason: 'checkpoint', observedAt: observationTimestamp(hostedPresence.checkpointObservedAt) };
  }
  if (!input.connected) return { state: 'unavailable', reason: 'disconnected' };
  if (!session || session.generation === 0 || session.baselineGeneration !== session.generation) {
    return { state: 'unavailable', reason: 'awaiting-baseline' };
  }
  const since = observationTimestamp(session.changedAt);
  const observedAt = observationTimestamp(input.castle?.unitsObservedAt);
  if (observedAt) {
    if (since && Date.parse(observedAt) < Date.parse(since)) {
      return { state: 'unavailable', reason: 'stale-before-connection', observedAt, since };
    }
    return { state: 'observed', scope: 'castle', observedAt };
  }
  return since ? { state: 'observed', scope: 'session', since } : { state: 'observed', scope: 'session' };
}

const message = (key: MessageKey): MessageKey => key;

/** Why unit counts cannot be used right now. */
export function observationUnavailableMessage(reason: ObservationUnavailableReason): MessageKey {
  if (reason === 'disconnected') return message('ui.components.staleSessionBanner.disconnected.last.known.data.166a8c99');
  if (reason === 'checkpoint') return message('ui.settings.requirements.observationFreshness.this.is.a.saved.checkpoint.troop.counts.48b43424');
  if (reason === 'awaiting-baseline') return message('ui.settings.requirements.observationFreshness.waiting.for.the.game.connection.to.finish.c661a838');
  return message('ui.settings.requirements.observationFreshness.troop.counts.are.older.than.the.current.a894b573');
}

export interface StockObservationNote {
  messageKey: MessageKey;
  params?: { observedAt: number };
  /** Why the counts are last-known, when they are. */
  reasonKey?: MessageKey;
}

/**
 * The one-line caption for stationed counts shown outside a readiness report (the troop picker, CIT-20):
 * "as of <time>" when the game reports a real per-castle time, "last known" with the reason when the counts
 * cannot be treated as current, and nothing when they are simply current on this connection.
 */
export function stockObservationNote(freshness: ObservationFreshness): StockObservationNote | null {
  if (freshness.state === 'unavailable') {
    return { messageKey: message('observedAt.lastKnown'), reasonKey: observationUnavailableMessage(freshness.reason) };
  }
  if (freshness.scope === 'castle' && freshness.observedAt) {
    return { messageKey: message('observedAt.castleUnits'), params: { observedAt: Date.parse(freshness.observedAt) } };
  }
  return null;
}

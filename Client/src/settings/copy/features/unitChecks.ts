import type { MessageKey } from '../../../i18n/messages';
import type { CastleCandidate, CastleCopyContext, CastleCopyReason } from '../castleCopy';
import { unitObservationFreshness } from '../../requirements/observationFreshness';

/** Shared destination checks for the troop-based descriptors (Towers, Station, Bird). Nothing here substitutes a unit. */

const message = (key: MessageKey): MessageKey => key;

export function troopName(context: CastleCopyContext, id: number): string {
  return context.troops[id]?.name?.trim() || `#${id}`;
}

export function toolName(context: CastleCopyContext, id: number): string {
  return context.tools[id]?.name?.trim() || `#${id}`;
}

export interface TroopDemand {
  id: number;
  /** Stationed count the destination needs to cover this entry. */
  amount: number;
}

/**
 * Reasons a destination cannot take these troop entries as they are:
 * - the game does not know the unit: incompatible, and the entry is dropped (never substituted);
 * - the destination holds fewer than the amount: unavailable (includable; the game checks stock at launch);
 * - counts are not current on this connection (D1): one `unknown` reason with the connection fix.
 */
export function troopEntryReasons(
  fieldId: string,
  demands: readonly TroopDemand[],
  destination: CastleCandidate,
  context: CastleCopyContext,
): CastleCopyReason[] {
  const reasons: CastleCopyReason[] = [];
  if (!destination.castle) {
    reasons.push({ id: 'not-in-world', state: 'incompatible', messageKey: message('castleCopy.reason.notInWorld'), params: { castle: destination.name } });
    return reasons;
  }
  if (!context.metadataReady) {
    reasons.push({ id: 'metadata', state: 'unknown', messageKey: message('castleCopy.reason.metadataLoading') });
    return reasons;
  }
  const unknown = demands.filter((demand) => !context.troops[demand.id]);
  for (const demand of unknown) {
    reasons.push({
      id: `unknown-item:${demand.id}`, state: 'incompatible', messageKey: message('castleCopy.reason.unknownItem'),
      params: { kind: 'troop', id: demand.id }, fieldId, dropEntryIds: [demand.id],
    });
  }
  const known = demands.filter((demand) => context.troops[demand.id]);
  if (known.length === 0) return reasons;
  const freshness = unitObservationFreshness({ castle: destination.castle, ...context.observation });
  if (freshness.state === 'unavailable') {
    reasons.push({ id: 'unobserved', state: 'unknown', messageKey: message('castleCopy.reason.unobserved'), params: { castle: destination.name }, fix: 'connection' });
    return reasons;
  }
  const stationed = destination.castle.units?.stationed ?? {};
  for (const demand of known) {
    const have = Math.max(0, Number(stationed[String(demand.id)]) || 0);
    if (have < demand.amount) {
      reasons.push({
        id: `stock:${demand.id}`, state: 'unavailable', messageKey: message('castleCopy.reason.stockShort'),
        params: { castle: destination.name, stationed: have, required: demand.amount, unit: troopName(context, demand.id) },
      });
    }
  }
  return reasons;
}

import type { EventInventoryStateV2, ScalableEventScoreV2, WorldIntelligenceEventScoreObservationV1 } from '../../api/Contracts';
import { completedEventScoreFinals } from '../../worldIntelligence/components/WorldEventFinals';
import { canonicalEventWorldID } from './FeatureEventWorld';

// These are account event leaderboards, not automation-attributed totals.
// Towers and Rift report analytics have no corresponding collected board.
export const featureEventIds: Partial<Record<string, readonly number[]>> = {
  autoInvasion: [71, 103],
  autoNomad: [72, 80],
  autoKhan: [72],
  autoAdvisor: [71, 72, 80, 88, 103],
  autoStorm: [102],
  autoBeriWorld: [3],
};

// Expired cached scores must never masquerade as a currently running event.
export function isFeatureEventRunning(
  event: ScalableEventScoreV2 | undefined,
  inventory: EventInventoryStateV2 | undefined,
  now: number,
  sessionChangedAt?: string,
): event is ScalableEventScoreV2 {
  if (!event || !Number.isFinite(event.remainingSec) || (event.remainingSec ?? 0) <= 0) return false;
  const observed = Date.parse(event.observedAt);
  const session = Date.parse(sessionChangedAt ?? '');
  if (!Number.isFinite(observed) || observed > now || (Number.isFinite(session) && observed < session)) return false;
  if (observed + (event.remainingSec ?? 0) * 1000 <= now) return false;
  const inventoryObserved = Date.parse(inventory?.observedAt ?? '');
  if (Number.isFinite(inventoryObserved) && inventoryObserved >= observed && inventoryObserved <= now) {
    const availability = inventory?.activeByEvent?.[String(event.eventId)];
    return availability?.eventId === event.eventId && Date.parse(availability.endsAt) > now;
  }
  return true;
}

export function featureEventFinals(
  entries: readonly WorldIntelligenceEventScoreObservationV1[],
  worldId: string,
  playerId: number,
  now: number,
): WorldIntelligenceEventScoreObservationV1[] {
  const world = canonicalEventWorldID(worldId);
  if (!world || !Number.isSafeInteger(playerId) || playerId <= 0) return [];
  return completedEventScoreFinals(entries.filter((entry) => (
    entry.playerId === playerId && typeof entry.worldId === 'string' && canonicalEventWorldID(entry.worldId) === world
    && Number.isFinite(Date.parse(entry.observedAt)) && Date.parse(entry.observedAt) <= now
    && typeof entry.score === 'number' && entry.score >= 0
  )), now);
}

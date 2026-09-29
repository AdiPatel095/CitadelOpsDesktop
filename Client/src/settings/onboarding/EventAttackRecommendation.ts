import type { CastleStateV2 } from '../../api/Contracts';
import type { InlineAttackSetup } from '../../attackPresets/AppCreatedPresets';
import type { AttackSetupLane, AttackSetupSlot, AttackSetupWave } from '../../components/AttackSetupModal';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import type { ReadinessFix } from '../readiness/Readiness';
import { observationUnavailableMessage, unitObservationFreshness, type ObservationContext } from '../requirements/observationFreshness';
import { isAttackUnit } from '../UnitRole';
import { EVENT_ATTACK_STARTER_RECIPE, pendingStarterReviews, type EventAttackStarterRecipe } from './StarterRecipes';

export type EventAttackRequirementId =
  | 'source-castle'
  | 'units-not-observed'
  | 'no-stationed-troops'
  | 'metadata-unavailable';

export interface EventAttackRequirement {
  id: EventAttackRequirementId;
  messageKey: MessageKey;
  fix?: ReadinessFix;
}

export interface EventAttackRecommendationInput {
  sourceCastle: CastleStateV2 | null;
  /** Session, connection and hosted presence used to decide whether unit counts are current (D1). */
  observation: ObservationContext;
  troops: Record<number, MetadataItem>;
  tools: Record<number, MetadataItem>;
  metadataReady: boolean;
  eventId: number;
  recipe?: EventAttackStarterRecipe;
}

export interface EventAttackRecommendation {
  setup: InlineAttackSetup | null;
  requirements: EventAttackRequirement[];
  notes: MessageKey[];
  /** Recipe entries consumed by this recommendation that still await review. */
  pendingReviews: string[];
  resolvedFor: { sourceCastleId: number; eventId: number };
}

/** Per-lane slot counts enforced by AttackSetupModal and the runtime (2/6/2 troops, 2/3/2 tools). */
const LANE_SLOTS = {
  L: { troops: 2, tools: 2 },
  M: { troops: 6, tools: 3 },
  R: { troops: 2, tools: 2 },
} as const;
const LANE_ORDER = ['L', 'M', 'R'] as const;
type LaneKey = typeof LANE_ORDER[number];

/**
 * Troop slot fill order for `most-numerous-stationed-troop-types-center-first`:
 * one type per lane first (center front, left flank, right flank), then the
 * remaining center, left and right slots. The runtime fills each lane
 * first-fit up to its capacity, so every lane's first slot is the one sent first.
 */
export const CENTER_FIRST_SLOT_ORDER: ReadonlyArray<readonly [LaneKey, number]> = (() => {
  const order: Array<readonly [LaneKey, number]> = [['M', 0], ['L', 0], ['R', 0]];
  for (const laneKey of ['M', 'L', 'R'] as const) {
    for (let slot = 1; slot < LANE_SLOTS[laneKey].troops; slot += 1) order.push([laneKey, slot]);
  }
  return order;
})();
const COURTYARD_TROOP_SLOTS = 8;
const COURTYARD_TOOL_SLOTS = 3;

const message = (key: MessageKey): MessageKey => key;

/**
 * Pure starting configuration resolved against the selected source castle.
 * Never returns a setup while any requirement is open, never chooses tools,
 * commanders or consumables, and uses no numbers beyond the reviewed recipe
 * and the castle's observed stationed counts.
 */
export function recommendEventAttackSetup(input: EventAttackRecommendationInput): EventAttackRecommendation {
  const recipe = input.recipe ?? EVENT_ATTACK_STARTER_RECIPE;
  const requirements: EventAttackRequirement[] = [];
  const resolvedFor = { sourceCastleId: input.sourceCastle?.id ?? 0, eventId: input.eventId };
  const pendingReviews = pendingStarterReviews(recipe);
  if (!input.metadataReady) {
    requirements.push({ id: 'metadata-unavailable', messageKey: message('ui.settings.onboarding.eventAttackRecommendation.troop.data.is.still.loading.so.a.0fa88000') });
  }
  if (!input.sourceCastle) {
    requirements.push({ id: 'source-castle', messageKey: message('ui.settings.onboarding.eventAttackRecommendation.choose.a.source.castle.first.the.starting.5edacd0b'), fix: 'settings' });
  }
  const freshness = unitObservationFreshness({ castle: input.sourceCastle, ...input.observation });
  const unitsCurrent = input.sourceCastle != null && freshness.state === 'observed';
  if (input.sourceCastle && freshness.state === 'unavailable') {
    requirements.push({ id: 'units-not-observed', messageKey: observationUnavailableMessage(freshness.reason), fix: 'connection' });
  }
  const ranked = unitsCurrent && input.sourceCastle && input.metadataReady
    ? rankStationedTroops(input.sourceCastle, input.troops, input.tools)
    : [];
  if (unitsCurrent && input.metadataReady && ranked.length === 0) {
    requirements.push({ id: 'no-stationed-troops', messageKey: message('ui.settings.onboarding.eventAttackRecommendation.no.attack.troops.are.stationed.in.this.270d8b35'), fix: 'settings' });
  }
  if (requirements.length > 0) {
    return { setup: null, requirements, notes: [], pendingReviews, resolvedFor };
  }

  const waveCount = Math.max(1, Math.min(30, Math.trunc(recipe.waveCount.value)));
  const waves: AttackSetupWave[] = [];
  let next = 0;
  for (let index = 0; index < waveCount; index += 1) {
    const wave = {} as AttackSetupWave;
    for (const laneKey of LANE_ORDER) {
      const lane: AttackSetupLane = {
        troops: Array.from({ length: LANE_SLOTS[laneKey].troops }, (): AttackSetupSlot => ({ itemId: null, quantity: 0 })),
        tools: Array.from({ length: LANE_SLOTS[laneKey].tools }, () => ({ itemId: null, quantity: 0 })),
      };
      wave[laneKey] = lane;
    }
    // Each stationed type is used once, with its full count, in center-first slot order.
    for (const [laneKey, slot] of CENTER_FIRST_SLOT_ORDER) {
      const entry = ranked[next];
      if (!entry) break;
      wave[laneKey].troops[slot] = { itemId: entry.id, quantity: entry.quantity };
      next += 1;
    }
    waves.push(wave);
  }
  const setup: InlineAttackSetup = {
    targetType: recipe.targetType.value,
    useTroopFamilies: recipe.useTroopFamilies.value,
    waves,
    courtyardSupport: {
      troops: Array.from({ length: COURTYARD_TROOP_SLOTS }, () => ({ itemId: null, quantity: 0 })),
      tools: Array.from({ length: COURTYARD_TOOL_SLOTS }, () => ({ itemId: null, quantity: 0 })),
    },
  };
  const notes: MessageKey[] = [
    message('ui.settings.onboarding.eventAttackRecommendation.each.lane.s.first.troop.type.is.51372e83'),
    message('ui.settings.onboarding.eventAttackRecommendation.no.tools.or.courtyard.support.are.added.143d3a76'),
  ];
  if (pendingReviews.length > 0) notes.push(message('ui.settings.onboarding.eventAttackRecommendation.these.starter.values.are.pending.product.review.215357af'));
  return { setup, requirements, notes, pendingReviews, resolvedFor };
}

/**
 * Stationed attack troop types by quantity (descending), then unit id (ascending).
 * Tools and defensive units (the troop picker's role rule, `unitCombatRole`) are excluded.
 */
export function rankStationedTroops(
  castle: CastleStateV2,
  troops: Record<number, MetadataItem>,
  tools: Record<number, MetadataItem>,
): Array<{ id: number; quantity: number }> {
  const ranked: Array<{ id: number; quantity: number }> = [];
  for (const [rawId, rawCount] of Object.entries(castle.units?.stationed ?? {})) {
    const id = Number(rawId);
    const quantity = Math.trunc(Number(rawCount));
    if (!Number.isInteger(id) || id <= 0 || !Number.isFinite(quantity) || quantity <= 0) continue;
    if (tools[id] || !troops[id] || !isAttackUnit(troops[id])) continue;
    ranked.push({ id, quantity });
  }
  return ranked.sort((left, right) => right.quantity - left.quantity || left.id - right.id);
}

import type { CastleStateV2 } from '../../api/Contracts';
import type { InlineAttackSetup } from '../../attackPresets/AppCreatedPresets';
import type { AttackSetupLane, AttackSetupSlot, AttackSetupWave } from '../../components/AttackSetupModal';
import type { MetadataItem } from '../../context/MetadataContext';
import type { MessageKey } from '../../i18n/messages';
import type { ReadinessFix } from '../readiness/Readiness';
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
  } else if (!input.sourceCastle.unitsObservedAt) {
    requirements.push({ id: 'units-not-observed', messageKey: message('ui.settings.onboarding.eventAttackRecommendation.troops.in.this.castle.have.not.been.6c663496'), fix: 'connection' });
  }
  const ranked = input.sourceCastle?.unitsObservedAt && input.metadataReady
    ? rankStationedTroops(input.sourceCastle, input.troops, input.tools)
    : [];
  if (input.sourceCastle?.unitsObservedAt && input.metadataReady && ranked.length === 0) {
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
      const troops: AttackSetupSlot[] = [];
      for (let slot = 0; slot < LANE_SLOTS[laneKey].troops; slot += 1) {
        const entry = ranked[next];
        if (entry) {
          troops.push({ itemId: entry.id, quantity: entry.quantity });
          next += 1;
        } else {
          troops.push({ itemId: null, quantity: 0 });
        }
      }
      const lane: AttackSetupLane = {
        troops,
        tools: Array.from({ length: LANE_SLOTS[laneKey].tools }, () => ({ itemId: null, quantity: 0 })),
      };
      wave[laneKey] = lane;
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
    message('ui.settings.onboarding.eventAttackRecommendation.each.slot.uses.one.stationed.troop.type.7a7aa312'),
    message('ui.settings.onboarding.eventAttackRecommendation.no.tools.or.courtyard.support.are.added.143d3a76'),
  ];
  if (pendingReviews.length > 0) notes.push(message('ui.settings.onboarding.eventAttackRecommendation.these.starter.values.are.pending.product.review.215357af'));
  return { setup, requirements, notes, pendingReviews, resolvedFor };
}

/** Stationed troop types by quantity (descending), then unit id (ascending). Tools are excluded. */
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
    if (tools[id] || !troops[id]) continue;
    ranked.push({ id, quantity });
  }
  return ranked.sort((left, right) => right.quantity - left.quantity || left.id - right.id);
}

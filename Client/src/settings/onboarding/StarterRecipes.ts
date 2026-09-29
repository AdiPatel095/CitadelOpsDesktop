/**
 * Starter values are product-reviewable configuration, not code constants.
 * `existing-default` entries cite the current default they repeat;
 * `account-data` entries hold a derivation rule instead of a number;
 * `proposed` entries are new structural choices awaiting product review.
 * No entry is approved by CIT-15: Maya flips `review.status` to `accepted`
 * (with evidence) during product acceptance.
 */
export type ReviewableValueSource = 'existing-default' | 'official-catalog' | 'account-data' | 'proposed';

export interface ReviewableValue<T> {
  value: T;
  unit?: string;
  source: ReviewableValueSource;
  /** Reviewer-facing explanation. Not displayed in the product UI. */
  rationale: string;
  /** Source reference for `existing-default` and `official-catalog` values. */
  citation?: string;
  review: {
    owner: 'Maya';
    status: 'pending' | 'accepted';
    evidence?: string;
  };
}

export type EventAttackLaneFillRule = 'most-numerous-stationed-troop-types';
export type EventAttackToolsRule = 'none';
export type EventAttackCourtyardRule = 'none';

export interface EventAttackStarterRecipe {
  waveCount: ReviewableValue<number>;
  laneFill: ReviewableValue<EventAttackLaneFillRule>;
  tools: ReviewableValue<EventAttackToolsRule>;
  courtyardSupport: ReviewableValue<EventAttackCourtyardRule>;
  targetType: ReviewableValue<'pve' | 'pvp'>;
  useTroopFamilies: ReviewableValue<boolean>;
}

export const EVENT_ATTACK_STARTER_RECIPE: EventAttackStarterRecipe = {
  waveCount: {
    value: 1,
    unit: 'waves',
    source: 'proposed',
    rationale: 'Smallest valid composition (the runtime requires 1 to 30 waves); users add waves in the editor.',
    review: { owner: 'Maya', status: 'pending' },
  },
  laneFill: {
    value: 'most-numerous-stationed-troop-types',
    source: 'account-data',
    rationale: 'Rank the source castle stationed attack troop types (defensive units excluded by the troop picker role rule: best attack value at least best defence value) by quantity (descending, then unit id ascending), assign them once each to the troop slots in lane order left (2), center (6), right (2), each slot holding that type\'s full stationed count. The runtime limits every lane to its capacity at launch. No stationed troops means no setup and a requirement instead.',
    review: { owner: 'Maya', status: 'pending' },
  },
  tools: {
    value: 'none',
    source: 'proposed',
    rationale: 'No siege tools are chosen for the user; tool eligibility is decided by the runtime and consumables need explicit permission.',
    review: { owner: 'Maya', status: 'pending' },
  },
  courtyardSupport: {
    value: 'none',
    source: 'proposed',
    rationale: 'Courtyard support spends extra troops and one-use Sceat tools; it stays empty until the user adds it.',
    review: { owner: 'Maya', status: 'pending' },
  },
  targetType: {
    value: 'pve',
    source: 'existing-default',
    rationale: 'Event camps and towers are PvE targets; this repeats the current preset default.',
    citation: 'attackPresets/AttackPresetTypes.ts parseAttackPreset (targetType falls back to pve); views/AttackPresetsView.tsx handleSave',
    review: { owner: 'Maya', status: 'pending' },
  },
  useTroopFamilies: {
    value: false,
    source: 'existing-default',
    rationale: 'Repeats the current preset default: exact unit ids, no family substitution.',
    citation: 'attackPresets/AttackPresetTypes.ts parseAttackPreset (useTroopFamilies is true only when stored as true)',
    review: { owner: 'Maya', status: 'pending' },
  },
};

/** Recipe entry names whose values still await product review. */
export function pendingStarterReviews(recipe: EventAttackStarterRecipe = EVENT_ATTACK_STARTER_RECIPE): string[] {
  return Object.entries(recipe)
    .filter(([, entry]) => (entry as ReviewableValue<unknown>).review.status !== 'accepted')
    .map(([name]) => name);
}

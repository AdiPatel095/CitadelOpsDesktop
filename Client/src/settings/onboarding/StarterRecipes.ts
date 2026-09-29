/**
 * Starter values are product-reviewable configuration, not code constants.
 * `existing-default` entries cite the current default they repeat;
 * `account-data` entries hold a derivation rule instead of a number;
 * `proposed` entries are new structural choices awaiting product review.
 * Maya decides each entry during product acceptance; the recorded decision
 * (with evidence) is then applied here as `review.status: 'accepted'`.
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

export type EventAttackLaneFillRule = 'most-numerous-stationed-troop-types-center-first';
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

/** Maya's CIT-15 product acceptance record for the accepted entries. */
const EVIDENCE_CIT15_ACCEPTANCE = 'Maya product acceptance 2026-09-29, Desktop 723d12d / Hosted 2178a0e, Product/Simpler automation setup.md § CIT-15 product acceptance';

export const EVENT_ATTACK_STARTER_RECIPE: EventAttackStarterRecipe = {
  waveCount: {
    value: 1,
    unit: 'waves',
    source: 'proposed',
    rationale: 'Smallest valid composition (the runtime requires 1 to 30 waves); users add waves in the editor.',
    review: { owner: 'Maya', status: 'accepted', evidence: EVIDENCE_CIT15_ACCEPTANCE },
  },
  laneFill: {
    value: 'most-numerous-stationed-troop-types-center-first',
    source: 'account-data',
    rationale: 'Rank the source castle stationed attack troop types (tools and defensive units excluded by unitCombatRole) by quantity (descending, then unit id ascending). The first type takes the center front first slot, the second the left flank first slot, the third the right flank first slot; the following types fill the remaining slots in the order center front, left flank, right flank. Each slot holds that type\'s full stationed count. The runtime fills each lane first-fit up to its capacity, so a lane\'s first type is sent first. No stationed attack troops means no setup and a requirement instead. Adjusted by Maya (CIT-15 product acceptance); stays pending until Sophie re-checks the preview.',
    review: { owner: 'Maya', status: 'pending' },
  },
  tools: {
    value: 'none',
    source: 'proposed',
    rationale: 'No siege tools are chosen for the user; tool eligibility is decided by the runtime and consumables need explicit permission.',
    review: { owner: 'Maya', status: 'accepted', evidence: EVIDENCE_CIT15_ACCEPTANCE },
  },
  courtyardSupport: {
    value: 'none',
    source: 'proposed',
    rationale: 'Courtyard support spends extra troops and one-use Sceat tools; it stays empty until the user adds it.',
    review: { owner: 'Maya', status: 'accepted', evidence: EVIDENCE_CIT15_ACCEPTANCE },
  },
  targetType: {
    value: 'pve',
    source: 'existing-default',
    rationale: 'Event camps and towers are PvE targets; this repeats the current preset default.',
    citation: 'attackPresets/AttackPresetTypes.ts parseAttackPreset (targetType falls back to pve); views/AttackPresetsView.tsx handleSave',
    review: { owner: 'Maya', status: 'accepted', evidence: EVIDENCE_CIT15_ACCEPTANCE },
  },
  useTroopFamilies: {
    value: false,
    source: 'existing-default',
    rationale: 'Repeats the current preset default: exact unit ids, no family substitution.',
    citation: 'attackPresets/AttackPresetTypes.ts parseAttackPreset (useTroopFamilies is true only when stored as true)',
    review: { owner: 'Maya', status: 'accepted', evidence: EVIDENCE_CIT15_ACCEPTANCE },
  },
};

/** Recipe entry names whose values still await product review. */
export function pendingStarterReviews(recipe: EventAttackStarterRecipe = EVENT_ATTACK_STARTER_RECIPE): string[] {
  return Object.entries(recipe)
    .filter(([, entry]) => (entry as ReviewableValue<unknown>).review.status !== 'accepted')
    .map(([name]) => name);
}

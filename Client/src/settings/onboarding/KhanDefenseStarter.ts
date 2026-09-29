import type { CastleStateV2 } from '../../api/Contracts';
import { inlineDefenseFromPreset, type InlineDefenseSetup } from '../../defensePresets/AppCreatedDefensePresets';
import { defensePresetDraftFromCastle } from '../../defensePresets/DefensePresetTypes';
import type { MessageKey } from '../../i18n/messages';
import {
  observationTimestamp,
  observationUnavailableMessage,
  unitObservationFreshness,
  type ObservationContext,
} from '../requirements/observationFreshness';
import { KHAN_DEFENSE_STARTER_RECIPE, pendingStarterReviews } from './StarterRecipes';

/**
 * Khan main-castle defense starting configuration (CIT-16): the main castle's
 * currently observed defense, captured exactly like Defense Presets' "Capture
 * current". Pure; never applied without an explicit preview. Defense
 * observation times are real per-castle times (not zeroed by the projection).
 */
export interface KhanDefenseStarter {
  setup: InlineDefenseSetup | null;
  /** Why no starter is offered; null when `setup` is set. */
  reason: MessageKey | null;
  pendingReviews: string[];
  observedAt?: string;
}

const message = (key: MessageKey): MessageKey => key;

export function khanDefenseStarter(input: { mainCastle: CastleStateV2 | null; observation: ObservationContext }): KhanDefenseStarter {
  const pendingReviews = pendingStarterReviews(KHAN_DEFENSE_STARTER_RECIPE);
  const castle = input.mainCastle;
  if (!castle) {
    return { setup: null, reason: message('ui.settings.onboarding.khanDefenseStarter.the.main.castle.is.not.observed.in.c57ce3ce'), pendingReviews };
  }
  const freshness = unitObservationFreshness({ castle, ...input.observation });
  if (freshness.state === 'unavailable' && freshness.reason !== 'stale-before-connection') {
    return { setup: null, reason: observationUnavailableMessage(freshness.reason), pendingReviews };
  }
  const observedAt = observationTimestamp(castle.defense?.observedAt);
  const inventoryObservedAt = observationTimestamp(castle.defense?.inventoryObservedAt);
  if (!observedAt || !inventoryObservedAt) {
    return { setup: null, reason: message('ui.settings.onboarding.khanDefenseStarter.the.main.castle.defense.has.not.been.344c0b6b'), pendingReviews };
  }
  const since = observationTimestamp(input.observation.session?.changedAt);
  if (since && Date.parse(observedAt) < Date.parse(since)) {
    return { setup: null, reason: message('ui.settings.onboarding.khanDefenseStarter.the.main.castle.defense.was.read.before.d8973319'), pendingReviews, observedAt };
  }
  return { setup: inlineDefenseFromPreset(defensePresetDraftFromCastle(castle)), reason: null, pendingReviews, observedAt };
}

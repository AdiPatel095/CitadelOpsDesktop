import type { MessageKey } from '../../i18n/messages';
import { SETTINGS_PLACEMENT, type SettingsFeatureId } from '../disclosure/placement';

/**
 * Goal-led first setup (CIT-19). One goal per catalog feature, so "Guide me through this" works on every row and the
 * checklist is always about one feature. Eight goals are curated (Maya's list, in her order) and shown first; the
 * rest are listed under "Show all automations" with the feature name as the title and its catalog description as the
 * outcome. Outcomes describe what the feature does and never promise a result. Goals are only an entry: the catalog
 * rows, switches and editors stay reachable without one.
 */
export interface AutomationGoal {
  id: string;
  featureId: SettingsFeatureId;
  titleKey: MessageKey;
  outcomeKey: MessageKey;
  /** Curated goals are offered first; the others sit under "Show all automations". */
  curated: boolean;
}

const goal = (featureId: SettingsFeatureId, curated: boolean): AutomationGoal => ({
  id: featureId,
  featureId,
  titleKey: `goal.${featureId}.title` as MessageKey,
  outcomeKey: `goal.${featureId}.outcome` as MessageKey,
  curated,
});

/** Curated goals, in the order they are offered. */
export const CURATED_GOAL_FEATURES: readonly SettingsFeatureId[] = [
  'autoTowers',
  'autoNomad',
  'autoInvasion',
  'autoKhan',
  'autoFoodBalance',
  'autoStation',
  'autoRecruit',
  'autoHospital',
];

/** The remaining goals, in catalog order. */
export const MORE_GOAL_FEATURES: readonly SettingsFeatureId[] = [
  'autoTool',
  'autoBird',
  'autoTCI',
  'autoSceatRes',
  'autoBooster',
  'autoBuyer',
  'autoFortress',
  'autoEquipmentCleanup',
  'autoAdvisor',
  'autoBeriWorld',
  'autoStorm',
];

export const AUTOMATION_GOALS: readonly AutomationGoal[] = [
  ...CURATED_GOAL_FEATURES.map((featureId) => goal(featureId, true)),
  ...MORE_GOAL_FEATURES.map((featureId) => goal(featureId, false)),
];

export function goalById(goalId: string | null | undefined): AutomationGoal | undefined {
  return AUTOMATION_GOALS.find((entry) => entry.id === goalId);
}

export function goalForFeature(featureId: string): AutomationGoal | undefined {
  return AUTOMATION_GOALS.find((entry) => entry.featureId === featureId);
}

/** Every catalog feature has exactly one goal (asserted by tests against the placement table). */
export const GOAL_FEATURE_IDS: readonly SettingsFeatureId[] = Object.keys(SETTINGS_PLACEMENT) as SettingsFeatureId[];

/** The configuration section that holds each feature's saved settings; "Required choices saved" reads only this. */
export const GOAL_SAVED_SECTION: Readonly<Record<SettingsFeatureId, string>> = {
  autoNomad: 'automation.autoNomad',
  autoInvasion: 'automation.autoInvasion',
  autoKhan: 'automation.autoKhan',
  autoBeriWorld: 'automation.autoBeriWorld',
  autoTowers: 'automation.autoTowers',
  autoFortress: 'automation.autoFortress',
  autoStorm: 'automation.autoStorm',
  autoFoodBalance: 'automation.autoFoodBalance',
  autoStation: 'automation.autoStation',
  autoBird: 'automation.autoBird',
  autoRecruit: 'automation.recruitTroops',
  autoTool: 'automation.autoTool',
  autoHospital: 'automation.autoHospital',
  autoTCI: 'automation.constructionItems',
  autoSceatRes: 'automation.autoSceatResources',
  autoBooster: 'automation.autoBooster',
  autoBuyer: 'automation.autoBuyer',
  autoAdvisor: 'automation.autoAdvisor',
  autoEquipmentCleanup: 'automation.autoEquipmentCleanup',
};

/** A saved section counts as present only when it holds at least one value; an empty object is nothing saved. */
export const isSavedSection = (value: unknown): boolean => (
  typeof value === 'object' && value !== null && !Array.isArray(value) && Object.keys(value).length > 0
);

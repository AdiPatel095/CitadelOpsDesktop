import type { SettingsFeatureId } from './disclosure/placement';

/** Change this together with the Automation page's group and feature display order. */
export const AUTOMATION_FEATURE_ORDER = [
  'autoTowers',
  'autoFortress',
  'autoInvasion',
  'autoNomad',
  'autoAdvisor',
  'autoKhan',
  'autoBeriWorld',
  'autoStorm',
  'autoRecruit',
  'autoTool',
  'autoSceatRes',
  'autoTCI',
  'autoFoodBalance',
  'autoBooster',
  'autoBuyer',
  'autoEquipmentCleanup',
  'autoHospital',
  'autoStation',
  'autoBird',
] as const satisfies readonly SettingsFeatureId[];

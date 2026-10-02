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

export const AUTOMATION_FEATURE_NAMES: Record<SettingsFeatureId, string> = {
  "autoEquipmentCleanup": "Auto Equipment Cleanup",
  "autoRecruit": "Auto Recruit",
  "autoTool": "Auto Tool",
  "autoHospital": "Auto Hospital",
  "autoStation": "Auto Station",
  "autoBird": "Auto Bird",
  "autoTCI": "Auto TCI",
  "autoSceatRes": "Auto Sceat Resources",
  "autoFoodBalance": "Auto Food Balance",
  "autoBooster": "Auto Booster",
  "autoBuyer": "Auto Buyer",
  "autoTowers": "Auto Towers",
  "autoFortress": "Auto Fortress",
  "autoInvasion": "Auto Invasion",
  "autoNomad": "Auto Nomad / Samurai",
  "autoAdvisor": "Auto Advisor",
  "autoKhan": "Auto Khan",
  "autoBeriWorld": "Auto Beri World",
  "autoStorm": "Auto Storm"
};

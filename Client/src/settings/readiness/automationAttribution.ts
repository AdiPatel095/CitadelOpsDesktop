import type { IntentReceipt, IntentStatus } from '../../api/Contracts';

/**
 * Which operation receipts belong to which automation (CIT-20).
 *
 * The Automation coordinator submits every automated operation with the actor
 * `automation:<policy actor id>` (`Server/Automation/Coordinator.go`, where
 * `policyActorID` is the policy's `ActorID()` or its own `ID()`). Lane
 * policies share their feature's actor (autoKhan:cooldown, autoStormShop and
 * autoBeriWorldTools report as `automation:autoKhan`, `automation:autoStorm`,
 * `automation:autoBeriWorld`; Sceat logistics as `automation:autoSceatRes`), so
 * the feature id is the actor id for every feature on the Automation page.
 * Operations started by the player or another module carry other actors
 * (`ui`, `scheduler:*`, `report-manager`) and never count.
 */
export const AUTOMATION_ACTOR_PREFIX = 'automation:';

const ATTRIBUTABLE_FEATURES = new Set([
  'autoNomad', 'autoInvasion', 'autoKhan', 'autoBeriWorld', 'autoTowers', 'autoFortress', 'autoStorm',
  'autoFoodBalance', 'autoStation', 'autoBird', 'autoRecruit', 'autoTool', 'autoHospital', 'autoTCI',
  'autoSceatRes', 'autoBooster', 'autoBuyer', 'autoAdvisor', 'autoEquipmentCleanup',
]);

/** The receipt actor of a feature's automated operations, or null when the feature is not known to produce attributable receipts. */
export function automationActor(featureId: string): string | null {
  return ATTRIBUTABLE_FEATURES.has(featureId) ? `${AUTOMATION_ACTOR_PREFIX}${featureId}` : null;
}

export const ACTIVE_STATUSES: ReadonlySet<IntentStatus> = new Set<IntentStatus>([
  'planning', 'planned', 'queued', 'running', 'paused', 'reconciling',
]);

/** Still running or waiting to run. A dry-run plan (`planned`, phase `completed`) is finished, not active. */
export function isActiveReceipt(receipt: IntentReceipt): boolean {
  return ACTIVE_STATUSES.has(receipt.status) && !(receipt.status === 'planned' && receipt.phase === 'completed');
}

/**
 * A game action, as opposed to a read (map scans, refreshes) or a
 * configuration follow-up (`config.update`), neither of which is something
 * the player would call the automation's first action.
 */
export function isGameAction(receipt: IntentReceipt): boolean {
  const effect = receipt.plan?.effect;
  if (!effect || effect === 'read') return false;
  return !receipt.intent.startsWith('config.');
}

export function attributedReceipts(featureId: string, operations: Record<string, IntentReceipt>): IntentReceipt[] {
  const actor = automationActor(featureId);
  if (!actor) return [];
  return Object.values(operations).filter((receipt) => receipt.actor === actor);
}

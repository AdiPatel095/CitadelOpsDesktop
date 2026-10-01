/**
 * The only file of the harness that names product paths (CIT-22). The desktop and hosted copies of every other harness
 * module are byte-identical; this file is the one that differs, because the two repositories lay the product out differently.
 */
export type * from '../../src/api/Contracts';
export type { AllianceTargetViewV2, PlayerHistoryRetentionV1 } from '../../src/api/Contracts';
export { stableDigest } from '../../src/settings/DraftRecovery';

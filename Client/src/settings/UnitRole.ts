import type { MetadataItem } from '../context/MetadataContext';

export type UnitCombatRole = 'attack' | 'defense';

/**
 * Combat role used by the troop picker and the recommended starting setup:
 * a unit is an attack unit when its best attack value (melee or ranged) is at
 * least its best defence value.
 */
export function unitCombatRole(item: MetadataItem): UnitCombatRole {
  const attack = Math.max(metadataNumber(item.meleeAttack), metadataNumber(item.rangeAttack));
  const defense = Math.max(metadataNumber(item.meleeDefence), metadataNumber(item.rangeDefence));
  return attack >= defense ? 'attack' : 'defense';
}

export function isAttackUnit(item: MetadataItem): boolean {
  return unitCombatRole(item) === 'attack';
}

function metadataNumber(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) ? parsed : 0;
}

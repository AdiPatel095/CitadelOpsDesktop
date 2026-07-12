import type { CastleStateV2, CraftingBuildingV2 } from '../api/Contracts';
import type { MetadataItem } from '../context/MetadataContext';

export type CastleQueueStripId =
  | 'recruitment'
  | 'tool'
  | 'refinery'
  | 'toolsmith'
  | 'dragon-hoard'
  | 'dragon-breath-forge';

export function visibleCastleQueueIds(
  castle: CastleStateV2,
  definitions: Record<number, MetadataItem>,
): Set<CastleQueueStripId> {
  const result = new Set<CastleQueueStripId>();
	if (castle.production['0']) result.add('recruitment');
	if (castle.production['1']) result.add('tool');
  for (const building of Object.values(castle.buildings)) {
    const identity = buildingIdentity(definitions[building.definitionId]);
    if (identity.includes('barrack')) result.add('recruitment');
    if (identity.includes('workshop') || identity.includes('dworkshop')) result.add('tool');
    if (identity.includes('refinery')) result.add('refinery');
    if (identity.includes('toolsmith')) result.add('toolsmith');
    if (identity.includes('dragonhoard') || identity.includes('dragon hoard')) result.add('dragon-hoard');
    if (identity.includes('dragonbreathforge') || identity.includes('dragon forge')) result.add('dragon-breath-forge');
  }
  for (const building of Object.values(castle.crafting.buildings)) {
    const identity = buildingIdentity(definitions[building.definitionId]);
    if (identity.includes('refinery')) result.add('refinery');
    if (identity.includes('toolsmith')) result.add('toolsmith');
    if (identity.includes('dragonhoard') || identity.includes('dragon hoard')) result.add('dragon-hoard');
    if (identity.includes('dragonbreathforge') || identity.includes('dragon forge')) result.add('dragon-breath-forge');
  }
  return result;
}

export function craftingBuildingForStrip(
  castle: CastleStateV2,
  strip: CastleQueueStripId,
  definitions: Record<number, MetadataItem>,
): CraftingBuildingV2 | undefined {
  return Object.values(castle.crafting.buildings).find((building) => {
    const identity = buildingIdentity(definitions[building.definitionId]);
    if (strip === 'refinery') return identity.includes('refinery');
    if (strip === 'toolsmith') return identity.includes('toolsmith');
    if (strip === 'dragon-hoard') return identity.includes('dragonhoard') || identity.includes('dragon hoard');
    if (strip === 'dragon-breath-forge') return identity.includes('dragonbreathforge') || identity.includes('dragon forge');
    return false;
  });
}

function buildingIdentity(definition: MetadataItem | undefined): string {
  if (!definition) return '';
  return [definition.internalName, definition.type, definition.JSONKey, definition.comment2, definition.name]
    .filter((value): value is string => typeof value === 'string')
    .join(' ')
    .toLowerCase();
}

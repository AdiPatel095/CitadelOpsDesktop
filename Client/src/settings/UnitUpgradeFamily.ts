import type { MetadataItem } from '../context/MetadataContext';

export interface UnitUpgradeFamily {
  ids: number[];
  minId: number;
  maxId: number;
}

const MAX_UNIT_FAMILY_SIZE = 128;

function positiveUnitID(value: unknown): number | null {
  const parsed = Number(value);
  if (!Number.isFinite(parsed) || parsed <= 0 || !Number.isInteger(parsed)) return null;
  return parsed;
}

function linkedUnitID(
  troops: Record<number, MetadataItem>,
  unitID: number,
  field: 'upgradeWodID' | 'downgradeWodID',
): number | null {
  const linkedID = positiveUnitID(troops[unitID]?.[field]);
  return linkedID != null && troops[linkedID] ? linkedID : null;
}

/** Returns the selected unit's complete official research upgrade chain. */
export function unitUpgradeFamily(
  selectedID: number,
  troops: Record<number, MetadataItem>,
): UnitUpgradeFamily | null {
  const anchorID = positiveUnitID(selectedID);
  if (anchorID == null) return null;

  const seen = new Set<number>([anchorID]);
  const lower = [anchorID];
  let currentID = anchorID;
  while (seen.size < MAX_UNIT_FAMILY_SIZE) {
    const nextID = linkedUnitID(troops, currentID, 'downgradeWodID');
    if (nextID == null || seen.has(nextID)) break;
    seen.add(nextID);
    lower.push(nextID);
    currentID = nextID;
  }
  lower.reverse();

  const ids = [...lower];
  currentID = anchorID;
  while (seen.size < MAX_UNIT_FAMILY_SIZE) {
    const nextID = linkedUnitID(troops, currentID, 'upgradeWodID');
    if (nextID == null || seen.has(nextID)) break;
    seen.add(nextID);
    ids.push(nextID);
    currentID = nextID;
  }

  return {
    ids,
    minId: Math.min(...ids),
    maxId: Math.max(...ids),
  };
}

/** Collapses multiple live tiers of one unit family to its highest available tier. */
export function highestUnitIDsByFamily(
  unitIDs: number[],
  troops: Record<number, MetadataItem>,
): number[] {
  const highestByFamily = new Map<string, { id: number; tier: number }>();
  unitIDs.forEach((unitID) => {
    const family = unitUpgradeFamily(unitID, troops);
    const key = family?.ids.join(',') ?? String(unitID);
    const tier = family?.ids.indexOf(unitID) ?? 0;
    const current = highestByFamily.get(key);
    if (!current || tier > current.tier || (tier === current.tier && unitID > current.id)) {
      highestByFamily.set(key, { id: unitID, tier });
    }
  });
  return Array.from(highestByFamily.values(), ({ id }) => id).sort((left, right) => left - right);
}

export function highestAvailableUnitIDInFamily(
  selectedID: number,
  availableIDs: number[] | undefined,
  troops: Record<number, MetadataItem>,
): number | null {
  if (availableIDs === undefined) return positiveUnitID(selectedID);
  const family = unitUpgradeFamily(selectedID, troops);
  const familyIDs = new Set(family?.ids ?? [selectedID]);
  return highestUnitIDsByFamily(
    availableIDs.filter((unitID) => familyIDs.has(unitID)),
    troops,
  )[0] ?? null;
}

/**
 * Preserves the old "available at every castle" picker rule while treating
 * different researched tiers of the same unit as equivalent choices.
 */
export function unitIDsAvailableByFamilyAcrossCastles(
  castleUnitIDs: number[][],
  troops: Record<number, MetadataItem>,
): number[] {
  if (castleUnitIDs.length === 0) return [];
  const familyKey = (unitID: number) => unitUpgradeFamily(unitID, troops)?.ids.join(',') ?? String(unitID);
  let sharedFamilies = new Set(castleUnitIDs[0].map(familyKey));
  for (const unitIDs of castleUnitIDs.slice(1)) {
    const castleFamilies = new Set(unitIDs.map(familyKey));
    sharedFamilies = new Set(Array.from(sharedFamilies).filter((key) => castleFamilies.has(key)));
  }

  const allowed = new Set<number>();
  castleUnitIDs.forEach((unitIDs) => unitIDs.forEach((unitID) => {
    if (sharedFamilies.has(familyKey(unitID))) allowed.add(unitID);
  }));
  return highestUnitIDsByFamily(Array.from(allowed), troops);
}

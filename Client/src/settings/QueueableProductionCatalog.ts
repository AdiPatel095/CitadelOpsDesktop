export interface QueueableProductionCatalogEntry {
  buildingRowsLoaded: boolean;
  recruitUnitIds: number[];
  toolIds: number[];
}

export type QueueableProductionCatalog = Record<string, QueueableProductionCatalogEntry>;
export type QueueableProductionField = 'recruitUnitIds' | 'toolIds';
export type QueueableProductionMergeMode = 'union' | 'intersection';

interface QueueableProductionWireEntry {
  castleID?: unknown;
  buildingRowsLoaded?: unknown;
  recruitUnitIds?: unknown;
  toolIds?: unknown;
}

function normalizeIDList(raw: unknown): number[] {
  if (!Array.isArray(raw)) return [];
  const ids = raw
    .map((value) => Number(value))
    .filter((value) => Number.isFinite(value) && value > 0)
    .map((value) => Math.floor(value));
  return Array.from(new Set(ids)).sort((a, b) => a - b);
}

export function normalizeQueueableProductionCatalog(raw: unknown): QueueableProductionCatalog {
  if (!Array.isArray(raw)) return {};

  const catalog: QueueableProductionCatalog = {};
  raw.forEach((entryRaw) => {
    if (!entryRaw || typeof entryRaw !== 'object') return;
    const entry = entryRaw as QueueableProductionWireEntry;
    const castleID = Number(entry.castleID);
    if (!Number.isFinite(castleID) || castleID <= 0) return;
    catalog[String(Math.floor(castleID))] = {
      buildingRowsLoaded: entry.buildingRowsLoaded === true,
      recruitUnitIds: normalizeIDList(entry.recruitUnitIds),
      toolIds: normalizeIDList(entry.toolIds),
    };
  });

  return catalog;
}

export function queueableIDsForCastle(
  catalog: QueueableProductionCatalog,
  castleID: number | string,
  field: QueueableProductionField,
): number[] {
  return [...(catalog[String(castleID)]?.[field] ?? [])];
}

export function queueableBuildingRowsLoaded(
  catalog: QueueableProductionCatalog,
  castleID: number | string,
): boolean {
  return catalog[String(castleID)]?.buildingRowsLoaded === true;
}

export function queueableCastleEligible(
  catalog: QueueableProductionCatalog,
  castleID: number | string,
  field: QueueableProductionField,
): boolean {
  const entry = catalog[String(castleID)];
  if (!entry || !entry.buildingRowsLoaded) return true;
  return entry[field].length > 0;
}

export function queueableIDsForCastles(
  catalog: QueueableProductionCatalog,
  castleIDs: Array<number | string>,
  field: QueueableProductionField,
  mode: QueueableProductionMergeMode = 'union',
): number[] {
  const ids = castleIDs.map((castleID) => String(castleID));
  if (ids.length === 0) return [];

  if (mode === 'intersection') {
    let intersection: Set<number> | null = null;
    ids.forEach((castleID) => {
      const castleIDsSet = new Set(queueableIDsForCastle(catalog, castleID, field));
      if (intersection == null) {
        intersection = castleIDsSet;
        return;
      }
      intersection = new Set(Array.from(intersection).filter((id) => castleIDsSet.has(id)));
    });
    return Array.from(intersection ?? []).sort((a, b) => a - b);
  }

  const union = new Set<number>();
  ids.forEach((castleID) => {
    queueableIDsForCastle(catalog, castleID, field).forEach((id) => union.add(id));
  });
  return Array.from(union).sort((a, b) => a - b);
}

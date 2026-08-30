import type { GameStateV2 } from '../api/Contracts';
import type { MetadataItem } from '../context/MetadataContext';

export interface QueueableProductionCatalogEntry {
  buildingRowsLoaded: boolean;
  recruitUnitIds: number[];
  toolIds: number[];
}

export type QueueableProductionCatalog = Record<string, QueueableProductionCatalogEntry>;
export type QueueableProductionField = 'recruitUnitIds' | 'toolIds';
export type QueueableProductionMergeMode = 'union' | 'intersection';

export function buildQueueableProductionCatalog(
  state: GameStateV2 | null,
  buildings: Record<number, MetadataItem>,
  troops: Record<number, MetadataItem>,
  tools: Record<number, MetadataItem>,
): QueueableProductionCatalog {
  if (!state) return {};
  const catalog: QueueableProductionCatalog = {};
  for (const castle of Object.values(state.castles)) {
    const recruitUnitIds = new Set<number>();
    const toolIds = new Set<number>();
		if (castle.queueableObservedAt) {
			for (const definition of castle.queueableProduction['0'] ?? []) {
				if (definition.collection === 'units' && troops[definition.id]) recruitUnitIds.add(definition.id);
			}
			for (const definition of castle.queueableProduction['1'] ?? []) {
				if (definition.collection === 'tools' && tools[definition.id]) toolIds.add(definition.id);
			}
			catalog[String(castle.id)] = {
				buildingRowsLoaded: true,
				recruitUnitIds: Array.from(recruitUnitIds).sort((left, right) => left - right),
				toolIds: Array.from(toolIds).sort((left, right) => left - right),
			};
			continue;
		}
    const buildingRows = Object.values(castle.buildings);
    for (const building of buildingRows) {
      const definition = buildings[building.definitionId];
      for (const id of officialIDList(definition?.unlockIDs)) {
        if (troops[id]) recruitUnitIds.add(id);
        if (tools[id]) toolIds.add(id);
      }
    }
    catalog[String(castle.id)] = {
      buildingRowsLoaded: buildingRows.length > 0,
      recruitUnitIds: Array.from(recruitUnitIds).sort((left, right) => left - right),
      toolIds: Array.from(toolIds).sort((left, right) => left - right),
    };
  }
  return catalog;
}

function normalizeIDList(raw: unknown): number[] {
  if (!Array.isArray(raw)) return [];
  const ids = raw
    .map((value) => Number(value))
    .filter((value) => Number.isFinite(value) && value > 0)
    .map((value) => Math.floor(value));
  return Array.from(new Set(ids)).sort((a, b) => a - b);
}

function officialIDList(raw: unknown): number[] {
  if (Array.isArray(raw)) return normalizeIDList(raw);
  if (typeof raw === 'string') {
    return normalizeIDList(raw.split(/[;,\s]+/).filter(Boolean));
  }
  if (raw == null) return [];
  return normalizeIDList([raw]);
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

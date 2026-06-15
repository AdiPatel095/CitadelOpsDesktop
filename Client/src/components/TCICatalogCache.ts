import { FrontendWebsocket } from '../Websocket';

/** One wire CID tier in a TCI design group (same name/effect line, different in-game level). */
export interface CatalogGroupTier {
  wireCid: number;
  level: number;
}

export interface ConstructionItemCatalogEntry {
  id: number;
  groupIds: number[];
  /** All tiers in this group, sorted by level, with each wire constructionItemID. */
  groupTiers: CatalogGroupTier[];
  minLevel: number;
  maxLevel: number;
  label: string;
  internal: string;
  /** Human level range, e.g. "1-4" (same as min–max for display). */
  level: string;
  category: string;
  /** Resolved effect line (same as in-game / General’s Camp style overviews). */
  effects: string;
}

/** Find the catalog group row for a base id or any tier wire id in the group. */
export function getCatalogEntryForWireId(
  catalog: ConstructionItemCatalogEntry[],
  wireId: number
): ConstructionItemCatalogEntry | undefined {
  for (const e of catalog) {
    if (e.id === wireId) {
      return e;
    }
    if (e.groupIds?.includes(wireId)) {
      return e;
    }
  }
  return undefined;
}

/** One line per wire CID tier, for picker lists (matches grouped level rows on reference overviews). */
export function formatGroupTiersLine(entry: ConstructionItemCatalogEntry): string {
  if (entry.groupTiers?.length) {
    return entry.groupTiers.map((t) => `L${t.level} #${t.wireCid}`).join(' · ');
  }
  return `Levels ${entry.level} · #${entry.id}`;
}

export function levelRangeLabel(entry: ConstructionItemCatalogEntry): string {
  if (entry.minLevel === entry.maxLevel) {
    return `Level ${entry.minLevel}`;
  }
  return `Levels ${entry.minLevel}–${entry.maxLevel}`;
}

let cache: ConstructionItemCatalogEntry[] | null = null;
let inFlight: Promise<ConstructionItemCatalogEntry[]> | null = null;

/** Loads the construction items catalog from the backend (cached). */
export function fetchConstructionItemsCatalog(): Promise<ConstructionItemCatalogEntry[]> {
  if (cache) {
    return Promise.resolve(cache);
  }
  if (inFlight) {
    return inFlight;
  }
  inFlight = new Promise((resolve, reject) => {
    const handler = (msg: any) => {
      if (msg.type !== 'constructionItemsCatalog') {
        return;
      }
      FrontendWebsocket.removeMessageListener(handler);
      inFlight = null;
      const payload = msg.payload;
      if (!Array.isArray(payload)) {
        cache = [];
        resolve(cache);
        return;
      }
      cache = (payload as ConstructionItemCatalogEntry[]).map(normalizeCatalogEntry);
      resolve(cache);
    };
    FrontendWebsocket.addMessageListener(handler);
    FrontendWebsocket.sendMessage({ type: 'getConstructionItemsCatalog' });
  });
  return inFlight;
}

function normalizeCatalogEntry(
  e: ConstructionItemCatalogEntry & { groupTiers?: CatalogGroupTier[]; minLevel?: number; maxLevel?: number }
): ConstructionItemCatalogEntry {
  const groupIds = e.groupIds?.length ? e.groupIds : [e.id];
  const minLevel = e.minLevel ?? 1;
  const maxLevel = e.maxLevel ?? minLevel;
  let groupTiers = e.groupTiers;
  if (!groupTiers || groupTiers.length === 0) {
    groupTiers = groupIds.map((wireCid) => ({ wireCid, level: minLevel }));
  }
  return {
    ...e,
    groupIds,
    groupTiers,
    minLevel,
    maxLevel,
  };
}

export function getCachedConstructionCatalog(): ConstructionItemCatalogEntry[] | null {
  return cache;
}

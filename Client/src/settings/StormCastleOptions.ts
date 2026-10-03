/**
 * Official Storm starter castles (the `prebuiltcastles` catalog) the unlock can open. Shared by the
 * Storm editor and the readiness checks, so both read the same "currently offered" set (CIT-20).
 */
export interface StormCastleOption {
  id: number;
  name: string;
  minLevel: number;
  costWood: number;
  costStone: number;
  costFood: number;
  costCoins: number;
  costPremium: number;
}

export function parseStormCastleOptions(rows: Record<string, unknown>[], playerLevel?: number): StormCastleOption[] {
  const availableLevel = playerLevel && playerLevel > 0 ? playerLevel : Number.MAX_SAFE_INTEGER;
  const options: StormCastleOption[] = [];
  for (const row of rows) {
    const spaces = String(row.spaceIDs ?? '')
      .split(',')
      .map((value) => Number(value.trim()))
      .filter(Number.isFinite);
    const id = positiveInteger(row.preBuiltCastleID);
    const minLevel = positiveInteger(row.minLevel);
    if (!spaces.includes(4) || id <= 0 || minLevel > availableLevel) continue;
    options.push({
      id,
      name: stringValue(row.comment2),
      minLevel,
      costWood: positiveInteger(row.costWood),
      costStone: positiveInteger(row.costStone),
      costFood: positiveInteger(row.costFood),
      costCoins: positiveInteger(row.costC1),
      costPremium: positiveInteger(row.costC2),
    });
  }
  return options.sort((left, right) => left.id - right.id);
}

export function preferredStormCastleOption(options: StormCastleOption[]): StormCastleOption | undefined {
  return options.find((option) => option.costPremium === 0) ?? options[0];
}

function positiveInteger(value: unknown): number {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed > 0 ? Math.trunc(parsed) : 0;
}

function stringValue(value: unknown): string {
  return typeof value === 'string' ? value.trim() : '';
}

type CatalogLoader = <T extends Record<string, unknown>>(name: string) => Promise<{ items: T[] }>;

export interface StormUnlockOffer {
  loaded: boolean;
  offeredIds: number[];
}

const OFFER_CACHE_MS = 60_000;
const offerCache = new Map<string, { at: number; rows: Promise<Record<string, unknown>[]> }>();

/**
 * The official starter castles currently offered, from the cached `prebuiltcastles` catalog (CIT-20). The Automation
 * row and the Start check read this same loader, so the unlock check cannot differ between them. A failed load is
 * `{ loaded: false }`: nothing is claimed about the offer.
 */
export async function loadStormUnlockOffer(getCatalog: CatalogLoader, playerLevel: number | undefined, now: number = Date.now()): Promise<StormUnlockOffer> {
  const cached = offerCache.get('prebuiltcastles');
  let rows = cached && now - cached.at < OFFER_CACHE_MS ? cached.rows : undefined;
  if (!rows) {
    rows = getCatalog<Record<string, unknown>>('prebuiltcastles').then((response) => response.items);
    offerCache.set('prebuiltcastles', { at: now, rows });
    rows.catch(() => { if (offerCache.get('prebuiltcastles')?.rows === rows) offerCache.delete('prebuiltcastles'); });
  }
  try {
    return { loaded: true, offeredIds: parseStormCastleOptions(await rows, playerLevel).map((option) => option.id) };
  } catch {
    return { loaded: false, offeredIds: [] };
  }
}

export function resetStormOfferCacheForTests(): void {
  offerCache.clear();
}

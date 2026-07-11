import { CitadelAPI } from '../api/CitadelClient';

/** One wire CID tier in a TCI design group (same name/effect line, different in-game level). */
export interface CatalogGroupTier {
  wireCid: number;
  level: number;
  effects?: string;
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

function splitEffectLines(effects: string): string[] {
  return effects
    .split(' • ')
    .map((part) => part.trim())
    .filter(Boolean);
}

function splitEffectLabelValue(line: string): { label: string; value: string } | null {
  const index = line.indexOf(':');
  if (index < 0) {
    return null;
  }
  const label = line.slice(0, index).trim();
  const value = line.slice(index + 1).trim();
  if (!label || !value) {
    return null;
  }
  return { label, value };
}

export function formatEffectUpgradeLine(entry: ConstructionItemCatalogEntry): string {
  const tiers = (entry.groupTiers ?? [])
    .filter((tier) => tier.effects?.trim())
    .slice()
    .sort((a, b) => a.level - b.level || a.wireCid - b.wireCid);

  if (tiers.length <= 1) {
    return tiers[0]?.effects?.trim() || entry.effects || '';
  }

  const tierLines = tiers.map((tier) => ({
    level: tier.level,
    effects: tier.effects?.trim() ?? '',
    lines: splitEffectLines(tier.effects ?? ''),
  }));
  const lineCount = tierLines[0].lines.length;
  const sameShape = lineCount > 0 && tierLines.every((tier) => tier.lines.length === lineCount);

  if (sameShape) {
    const collapsed: string[] = [];
    for (let i = 0; i < lineCount; i += 1) {
      const parsed = tierLines.map((tier) => splitEffectLabelValue(tier.lines[i]));
      const firstLabel = parsed[0]?.label;
      if (!firstLabel || parsed.some((part) => !part || part.label !== firstLabel)) {
        return tierLines.map((tier) => `L${tier.level} ${tier.effects}`).join(' → ');
      }
      collapsed.push(
        `${firstLabel}: ${parsed
          .map((part, index) => `L${tierLines[index].level} ${part?.value ?? ''}`)
          .join(' → ')}`
      );
    }
    return collapsed.join(' • ');
  }

  return tierLines.map((tier) => `L${tier.level} ${tier.effects}`).join(' → ');
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
  inFlight = loadOfficialConstructionItems()
    .then((items) => {
      cache = items;
      return items;
    })
    .finally(() => {
      inFlight = null;
    });
  return inFlight;
}

async function loadOfficialConstructionItems(): Promise<ConstructionItemCatalogEntry[]> {
  const response = await CitadelAPI.getCatalog<Record<string, unknown>>('constructionItems');
  const localizationNames = Array.from(new Set(response.items
    .map((row) => typeof row.name === 'string' ? row.name : '')
    .filter(Boolean)));
  const localizationKeys = localizationNames.flatMap((name) => [`${name}_name`, name]).slice(0, 5000);
  const translations = await CitadelAPI.localize(localizationKeys);
  const groups = new Map<number, CatalogGroupTier[]>();
  const records = new Map<number, Record<string, unknown>>();
  for (const row of response.items) {
    const id = positiveInteger(row.constructionItemID);
    if (id === 0) continue;
    const groupID = positiveInteger(row.constructionItemGroupID) || id;
    const tier: CatalogGroupTier = {
      wireCid: id,
      level: positiveInteger(row.level) || 1,
      effects: formatOfficialEffects(row),
    };
    groups.set(groupID, [...(groups.get(groupID) ?? []), tier]);
    if (!records.has(groupID)) records.set(groupID, row);
  }
  return Array.from(groups, ([groupID, unsortedTiers]) => {
    const groupTiers = unsortedTiers.sort((left, right) => left.level - right.level || left.wireCid - right.wireCid);
    const row = records.get(groupID) ?? {};
    const internal = typeof row.name === 'string' ? row.name : `constructionItem${groupID}`;
    const label = translations[`${internal}_name`] ?? translations[internal] ?? humanize(internal);
    const minLevel = groupTiers[0]?.level ?? 1;
    const maxLevel = groupTiers[groupTiers.length - 1]?.level ?? minLevel;
    return {
      id: groupTiers[0]?.wireCid ?? groupID,
      groupIds: groupTiers.map((tier) => tier.wireCid),
      groupTiers,
      minLevel,
      maxLevel,
      label,
      internal,
      level: minLevel === maxLevel ? String(minLevel) : `${minLevel}-${maxLevel}`,
      category: typeof row.comment1 === 'string' ? row.comment1 : `Slot ${row.slotTypeID ?? ''}`.trim(),
      effects: groupTiers[0]?.effects ?? '',
    };
  }).sort((left, right) => left.category.localeCompare(right.category) || left.label.localeCompare(right.label));
}

const constructionMetadataFields = new Set([
  'constructionItemID', 'constructionItemGroupID', 'constructionItemEffectGroupID',
  'name', 'comment1', 'comment2', 'level', 'rarenessID', 'slotTypeID',
  'removalCostC1', 'lockRemoval', 'isPremium', 'effects',
]);

function formatOfficialEffects(row: Record<string, unknown>): string {
  const effects: string[] = [];
  for (const [key, value] of Object.entries(row)) {
    if (constructionMetadataFields.has(key) || key.startsWith('add') || key.startsWith('cost')) continue;
    const amount = Number(value);
    if (!Number.isFinite(amount) || amount === 0) continue;
    effects.push(`${humanize(key)}: ${amount.toLocaleString()}`);
  }
  return effects.join(' • ');
}

function positiveInteger(value: unknown): number {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? Math.floor(number) : 0;
}

function humanize(value: string): string {
  return value
    .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
    .replace(/[_-]+/g, ' ')
    .replace(/^./, (character) => character.toUpperCase());
}

export function getCachedConstructionCatalog(): ConstructionItemCatalogEntry[] | null {
  return cache;
}

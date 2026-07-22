import { CitadelAPI } from '../api/CitadelClient';

/** One wire CID tier in a TCI design group (same name/effect line, different in-game level). */
export interface CatalogGroupTier {
  wireCid: number;
  level: number;
  effects?: string;
  durationSeconds: number;
  rarity: number;
  removalCost: number;
  premium: boolean;
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
  imageUrl: string;
  buildingName: string;
  durationSecondsMin: number;
  durationSecondsMax: number;
  premium: boolean;
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
    return entry.groupTiers
      .map((tier) => `L${tier.level} #${tier.wireCid} · ${formatDuration(tier.durationSeconds)}`)
      .join(' → ');
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

export function formatDuration(totalSeconds: number): string {
  const seconds = Math.max(0, Math.floor(totalSeconds));
  if (seconds === 0) return 'No duration';
  const units: string[] = [];
  const days = Math.floor(seconds / 86_400);
  const hours = Math.floor((seconds % 86_400) / 3_600);
  const minutes = Math.floor((seconds % 3_600) / 60);
  const remainingSeconds = seconds % 60;
  if (days > 0) units.push(`${days}d`);
  if (hours > 0) units.push(`${hours}h`);
  if (minutes > 0) units.push(`${minutes}m`);
  if (units.length === 0 || (days === 0 && hours === 0 && remainingSeconds > 0)) {
    units.push(`${remainingSeconds}s`);
  }
  return units.slice(0, 2).join(' ');
}

export function durationRangeLabel(entry: ConstructionItemCatalogEntry): string {
  if (entry.durationSecondsMin === entry.durationSecondsMax) {
    return formatDuration(entry.durationSecondsMin);
  }
  return `${formatDuration(entry.durationSecondsMin)} → ${formatDuration(entry.durationSecondsMax)}`;
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
  const [response, effectsResponse, buildingAssetRows] = await Promise.all([
    CitadelAPI.getCatalog<Record<string, unknown>>('constructionItems'),
    CitadelAPI.getCatalog<Record<string, unknown>>('effects'),
    CitadelAPI.getCatalog<Record<string, unknown>>('construction-item-building-icons')
      .then((assetResponse) => assetResponse.items)
      .catch(() => []),
  ]);
  const effectNames = effectNamesByID(effectsResponse.items);
  const buildingAssets = constructionItemBuildingAssets(buildingAssetRows);
  const localizationKeys = Array.from(new Set(response.items.flatMap((row) => {
    const internal = typeof row.name === 'string' ? row.name.trim() : '';
    return internal ? tciDisplayNameLocalizationKeys(internal) : [];
  }).concat(effectLocalizationKeys(response.items, effectNames)))).slice(0, 5000);
  const translations = await CitadelAPI.localize(localizationKeys);
  const groups = new Map<string, Record<string, unknown>[]>();
  for (const row of response.items) {
    const id = positiveInteger(row.constructionItemID);
    if (id === 0 || !isSelectableConstructionItem(row)) continue;
    const key = constructionItemGroupKey(row);
    groups.set(key, [...(groups.get(key) ?? []), row]);
  }
  return Array.from(groups, ([, unsortedRows]) => {
    const rows = sortConstructionItemRows(unsortedRows);
    const row = rows[0] ?? {};
    const groupTiers = rows.map((tierRow) => ({
      wireCid: positiveInteger(tierRow.constructionItemID),
      level: positiveInteger(tierRow.level) || 1,
      effects: formatOfficialEffects(tierRow, effectNames, translations),
      durationSeconds: positiveInteger(tierRow.duration),
      rarity: positiveInteger(tierRow.rarenessID),
      removalCost: positiveInteger(tierRow.removalCostC1),
      premium: booleanValue(tierRow.isPremium),
    })).sort((left, right) => left.level - right.level || left.wireCid - right.wireCid);
    const internal = typeof row.name === 'string' ? row.name : `constructionItem${groupTiers[0]?.wireCid ?? ''}`;
    const label = tciDisplayName(row, internal, translations);
    const constructionItemGroupID = positiveInteger(row.constructionItemGroupID);
    const buildingAsset = buildingAssets.get(constructionItemBuildingAssetKey(constructionItemGroupID, internal));
    const minLevel = groupTiers[0]?.level ?? 1;
    const maxLevel = groupTiers[groupTiers.length - 1]?.level ?? minLevel;
    const durations = groupTiers.map((tier) => tier.durationSeconds).filter((duration) => duration > 0);
    const durationSecondsMin = durations.length > 0 ? Math.min(...durations) : 0;
    const durationSecondsMax = durations.length > 0 ? Math.max(...durations) : 0;
    return {
      id: groupTiers[0]?.wireCid ?? positiveInteger(row.constructionItemGroupID),
      groupIds: groupTiers.map((tier) => tier.wireCid),
      groupTiers,
      minLevel,
      maxLevel,
      label,
      internal,
      level: minLevel === maxLevel ? String(minLevel) : `${minLevel}-${maxLevel}`,
      category: typeof row.comment1 === 'string' ? row.comment1 : `Slot ${row.slotTypeID ?? ''}`.trim(),
      effects: groupTiers[0]?.effects ?? '',
      imageUrl: buildingAsset?.url ?? '',
      buildingName: buildingAsset?.buildingName ?? '',
      durationSecondsMin,
      durationSecondsMax,
      premium: groupTiers.some((tier) => tier.premium),
    };
  }).sort((left, right) => left.label.localeCompare(right.label) || left.effects.localeCompare(right.effects) || left.id - right.id);
}

function isSelectableConstructionItem(row: Record<string, unknown>): boolean {
  const id = positiveInteger(row.constructionItemID);
  if (positiveInteger(row.duration) === 0 || (id >= 1000 && id <= 9999)) return false;
  const comment1 = String(row.comment1 ?? '').trim().toLowerCase();
  const comment2 = String(row.comment2 ?? '').trim().toLowerCase();
  return comment1 !== 'appearance' && !comment1.includes('testing') && !comment2.includes('testing');
}

function constructionItemBuildingAssets(rows: Record<string, unknown>[]): Map<string, { buildingName: string; url: string }> {
  const assets = new Map<string, { buildingName: string; url: string }>();
  for (const row of rows) {
    const constructionItemGroupID = positiveInteger(row.constructionItemGroupId);
    const constructionItemName = typeof row.constructionItemName === 'string' ? row.constructionItemName.trim() : '';
    const buildingName = typeof row.buildingName === 'string' ? row.buildingName.trim() : '';
    const url = typeof row.url === 'string' ? row.url.trim() : '';
    if (constructionItemGroupID > 0 && constructionItemName && url) {
      assets.set(constructionItemBuildingAssetKey(constructionItemGroupID, constructionItemName), { buildingName, url });
    }
  }
  return assets;
}

function constructionItemBuildingAssetKey(groupID: number, internal: string): string {
  return `${groupID}|${internal.trim().toLowerCase()}`;
}

function constructionItemGroupKey(row: Record<string, unknown>): string {
  const name = typeof row.name === 'string' ? row.name.trim() : '';
  const effectIDs = Array.from(new Set(wireEffectIDs(row))).sort((left, right) => left - right).join(',');
  const legacyFields = legacyEffectFields.filter((field) => hasValue(row[field])).sort().join(',');
  const isAppearance = positiveInteger(row.slotTypeID) === 0 && hasValue(row.decoPoints);
  const isTemporary = positiveInteger(row.duration) > 0;
  const slotType = String(row.slotTypeID ?? '').trim();
  const groupID = String(row.constructionItemGroupID ?? '').trim();
  return [name, effectIDs, legacyFields, isAppearance ? 'appearance' : 'normal', isTemporary ? 'temporary' : 'permanent', slotType, groupID].join('|');
}

function sortConstructionItemRows(rows: Record<string, unknown>[]): Record<string, unknown>[] {
  return rows.slice().sort((left, right) => {
    if (positiveInteger(left.slotTypeID) === 1 && positiveInteger(right.slotTypeID) === 1) {
      return positiveInteger(left.level) - positiveInteger(right.level);
    }
    const rarenessDifference = positiveInteger(left.rarenessID) - positiveInteger(right.rarenessID);
    return rarenessDifference || totalEffectValue(left) - totalEffectValue(right);
  });
}

function totalEffectValue(row: Record<string, unknown>): number {
  const wireEffects = typeof row.effects === 'string' ? row.effects : '';
  return wireEffects.split(',').reduce((total, entry) => {
    const [, rawValue = ''] = entry.trim().split('&', 2);
    return total + (effectAmount(rawValue) ?? 0);
  }, 0);
}

function hasValue(value: unknown): boolean {
  return value !== undefined && value !== null && String(value).trim() !== '';
}

function booleanValue(value: unknown): boolean {
  if (typeof value === 'boolean') return value;
  if (typeof value === 'number') return value !== 0;
  const normalized = String(value ?? '').trim().toLowerCase();
  return normalized === '1' || normalized === 'true' || normalized === 'yes';
}

function tciDisplayNameLocalizationKeys(internal: string): string[] {
  const name = internal.toLowerCase();
  const keys: string[] = [];
  for (const prefix of ['appearance', 'primary', 'secondary']) {
    keys.push(`ci_${prefix}_${name}`, `ci_${prefix}_${name}_premium`);
  }
  keys.push(`ci_${name}`, `ci_${name}_premium`, `${name}_name`, name);
  return keys;
}

function tciDisplayName(
  row: Record<string, unknown>,
  internal: string,
  translations: Record<string, string>,
): string {
  for (const key of tciDisplayNameLocalizationKeys(internal)) {
    const translation = translations[key];
    if (translation?.trim()) return translation;
  }
  const displayName = typeof row._display_name === 'string' ? row._display_name.trim() : '';
  return humanize(displayName || internal);
}

function effectNamesByID(rows: Record<string, unknown>[]): Map<number, string> {
  const names = new Map<number, string>();
  for (const row of rows) {
    const id = positiveInteger(row.effectID);
    const name = typeof row.name === 'string' ? row.name.trim() : '';
    if (id && name) names.set(id, name);
  }
  return names;
}

function effectLocalizationKeys(
  constructionItems: Record<string, unknown>[],
  effectNames: Map<number, string>,
): string[] {
  const keys = new Set<string>();
  for (const row of constructionItems) {
    for (const id of wireEffectIDs(row)) {
      const internal = effectNames.get(id);
      if (internal) effectLocalizationKeysForInternal(internal, keys);
    }
  }
  for (const internal of legacyEffectFields) effectLocalizationKeysForInternal(internal, keys);
  return Array.from(keys);
}

function effectLocalizationKeysForInternal(internal: string, keys: Set<string>): void {
  const name = internal.toLowerCase();
  keys.add(`ci_effect_${name}_tt`);
  keys.add(`effect_name_${name}`);
  keys.add(`ci_effect_${name}`);
  keys.add(`subscription_effect_description_${name}`);
}

function wireEffectIDs(row: Record<string, unknown>): number[] {
  const wireEffects = typeof row.effects === 'string' ? row.effects : '';
  return wireEffects.split(',').flatMap((entry) => {
    const id = positiveInteger(entry.trim().split('&', 1)[0]);
    return id ? [id] : [];
  });
}

function formatOfficialEffects(
  row: Record<string, unknown>,
  effectNames: Map<number, string>,
  translations: Record<string, string>,
): string {
  const effects: string[] = [];
  const wireEffects = typeof row.effects === 'string' ? row.effects : '';
  for (const entry of wireEffects.split(',')) {
    const [rawID, rawValue = ''] = entry.trim().split('&', 2);
    const id = positiveInteger(rawID);
    const amount = effectAmount(rawValue);
    if (!id || amount === null) continue;
    const internal = effectNames.get(id) ?? `Effect #${id}`;
    addEffectLine(effects, formatEffectLine(internal, amount, translations));
  }
  for (const internal of legacyEffectFields) {
    const amount = numericValue(row[internal]);
    if (amount === null || amount === 0) continue;
    addEffectLine(effects, formatEffectLine(internal, amount, translations));
  }
  return effects.join(' • ');
}

function effectAmount(rawValue: string): number | null {
  if (!rawValue.trim()) return null;
  const value = rawValue.includes('+') ? rawValue.split('+', 2)[1] : rawValue;
  return numericValue(value);
}

function numericValue(value: unknown): number | null {
  const amount = Number(value);
  return Number.isFinite(amount) ? amount : null;
}

function formatEffectLine(internal: string, amount: number, translations: Record<string, string>): string {
  const name = internal.toLowerCase();
  const template = [
    `ci_effect_${name}_tt`,
    `effect_name_${name}`,
    `ci_effect_${name}`,
    `subscription_effect_description_${name}`,
  ].map((key) => translations[key]).find((value) => value?.trim()) ?? humanize(internal);
  const formattedAmount = Math.abs(amount).toLocaleString();
  if (template.includes('{0}')) {
    const value = amount < 0 ? `-${formattedAmount}` : formattedAmount;
    return template.replaceAll('{0}', value);
  }
  return `${template}${template.includes(':') ? ' ' : ': '}${amount.toLocaleString()}`;
}

function addEffectLine(effects: string[], line: string): void {
  if (!effects.some((existing) => existing.toLowerCase() === line.toLowerCase())) effects.push(line);
}

const legacyEffectFields = [
  'unitWallCount', 'recruitSpeedBoost', 'woodStorage', 'stoneStorage', 'ReduceResearchResourceCosts',
  'Stoneproduction', 'Woodproduction', 'Foodproduction', 'foodStorage', 'unboostedFoodProduction',
  'defensiveToolsSpeedBoost', 'defensiveToolsCostsReduction', 'meadStorage', 'recruitCostReduction',
  'honeyStorage', 'hospitalCapacity', 'healSpeed', 'marketCarriages', 'XPBoostBuildBuildings',
  'stackSize', 'glassStorage', 'Glassproduction', 'ironStorage', 'Ironproduction', 'coalStorage',
  'Coalproduction', 'oilStorage', 'Oilproduction', 'offensiveToolsCostsReduction', 'feastCostsReduction',
  'Meadreduction', 'surviveBoost', 'unboostedStoneProduction', 'unboostedWoodProduction',
  'offensiveToolsSpeedBoost', 'espionageTravelBoost', 'decoPoints',
];

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

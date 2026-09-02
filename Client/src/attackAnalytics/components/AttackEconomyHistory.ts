export interface AttackEconomyAggregate {
  viewKey: string;
  bucketStart: string;
  bucketSeconds: number;
  reportCount: number;
  victories: number;
  defeats: number;
  troopsSent: number;
  troopLosses: number;
  toolsUsed: number;
  gallantryPoints: number;
  lootTotal: number;
  resources: Record<string, number>;
  revision: number;
}

export interface AttackEconomyResourceSummary {
  gallantryPoints: number;
  loot: Record<string, number>;
}

interface AttackEconomyPage {
  aggregates?: AttackEconomyAggregate[];
  sourceBucketSeconds?: number;
  nextBefore?: string;
}

type RuntimeFetcher = (path: string, init?: RequestInit) => Promise<Response>;

const pageSize = 5_000;

export async function fetchAttackEconomyAggregates(
  fetcher: RuntimeFetcher,
  options: {
    view: string;
    since?: string;
    signal?: AbortSignal;
  },
): Promise<AttackEconomyAggregate[]> {
  const aggregates: AttackEconomyAggregate[] = [];
  const seenCursors = new Set<string>();
  let before = '';

  do {
    const parameters = new URLSearchParams({ view: options.view, limit: String(pageSize) });
    if (options.since) parameters.set('since', options.since);
    if (before) parameters.set('before', before);
    const response = await fetcher(`/api/v2/analytics/resource-aggregates?${parameters.toString()}`, {
      cache: 'no-store',
      signal: options.signal,
    });
    if (!response.ok) throw new Error(`Feature resource history returned HTTP ${response.status}`);
    const payload = await response.json() as AttackEconomyPage;
    aggregates.push(...validAggregates(payload.aggregates ?? []));
    const nextBefore = typeof payload.nextBefore === 'string' ? payload.nextBefore.trim() : '';
    if (!nextBefore) break;
    if (seenCursors.has(nextBefore)) throw new Error('Feature resource history returned a repeated cursor');
    seenCursors.add(nextBefore);
    before = nextBefore;
  } while (before);

  return aggregates.sort((left, right) => aggregateTimestamp(left) - aggregateTimestamp(right));
}

export function attackEconomyRangeSince(rangeSeconds: number | null, nowMs = Date.now()): string | undefined {
  if (rangeSeconds == null) return undefined;
  return new Date(nowMs - rangeSeconds * 1000).toISOString();
}

export function aggregateTimestamp(aggregate: AttackEconomyAggregate): number {
  const parsed = Date.parse(aggregate.bucketStart);
  return Number.isFinite(parsed) ? Math.floor(parsed / 1000) : 0;
}

export function aggregateEndTimestamp(aggregate: AttackEconomyAggregate): number {
  return aggregateTimestamp(aggregate) + Math.max(1, Math.floor(Number(aggregate.bucketSeconds) || 0));
}

// Keep the presentation inventory data-driven: report parsers can discover a
// resource before the bundled game metadata knows its friendly name.
export function summarizeAttackEconomyResources(
  aggregates: AttackEconomyAggregate[],
): AttackEconomyResourceSummary {
  const result: AttackEconomyResourceSummary = { gallantryPoints: 0, loot: {} };
  for (const aggregate of aggregates) {
    result.gallantryPoints += finitePositive(aggregate.gallantryPoints);
    for (const [key, rawAmount] of Object.entries(aggregate.resources ?? {})) {
      const amount = finitePositive(rawAmount);
      if (amount <= 0) continue;
      result.loot[key] = (result.loot[key] ?? 0) + amount;
    }
  }
  return result;
}

export function attackEconomyResourceFallbackLabel(rawKey: string): string {
  const key = typeof rawKey === 'string' ? rawKey.trim() : '';
  if (!key) return 'Resource';
  if (/^[A-Z0-9]{1,4}$/.test(key)) return key;
  const words = key.replace(/[_:.-]+/g, ' ').trim().split(/\s+/).filter(Boolean);
  if (words.length === 0) return 'Resource';
  return words
    .map((word) => `${word.charAt(0).toLocaleUpperCase()}${word.slice(1).toLocaleLowerCase()}`)
    .join(' ');
}

function validAggregates(values: AttackEconomyAggregate[]): AttackEconomyAggregate[] {
  return values.filter((value) => value
    && typeof value === 'object'
    && typeof value.viewKey === 'string'
    && aggregateTimestamp(value) > 0
    && Number.isFinite(Number(value.bucketSeconds))
    && Number(value.bucketSeconds) > 0);
}

function finitePositive(value: unknown): number {
  const number = Number(value);
  return Number.isFinite(number) && number > 0 ? number : 0;
}

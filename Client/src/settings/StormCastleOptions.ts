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

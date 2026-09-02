export interface WalletHistorySample {
  coins?: number;
  rubies?: number;
  currencies?: Record<string, number>;
}

export interface WalletMetricIDs {
  resourceIDs: string[];
  currencyIDs: string[];
}

// My Stats stores every observed player resource and currency in the sample's
// currencies map. Build the visible metric inventory from both live state and
// retained samples so a balance does not disappear from the page merely
// because its current value reached zero or the live payload stopped listing it.
export function collectWalletMetricIDs(
  liveResources: Record<string, number> | null | undefined,
  liveCurrencies: Record<string, number> | null | undefined,
  retainedSamples: Array<WalletHistorySample | null | undefined>,
): WalletMetricIDs {
  const resourceIDs = new Set<string>();
  const currencyIDs = new Set<string>();

  collectLiveIDs(resourceIDs, liveResources);
  collectLiveIDs(currencyIDs, liveCurrencies);
  for (const sample of retainedSamples) {
    for (const [key, rawAmount] of Object.entries(sample?.currencies ?? {})) {
      if (!isVisibleBalance(rawAmount)) continue;
      const match = /^(resource|currency):([1-9]\d*)$/.exec(key);
      if (!match) continue;
      (match[1] === 'resource' ? resourceIDs : currencyIDs).add(match[2]);
    }
  }

  return {
    resourceIDs: [...resourceIDs].sort(numericIDOrder),
    currencyIDs: [...currencyIDs].sort(numericIDOrder),
  };
}

export function retainedWalletBalance(
  sample: WalletHistorySample | null | undefined,
  key: string,
): number | undefined {
  if (!sample) return undefined;
  const candidates = key === 'coins'
    ? [sample.currencies?.['resource:1'], sample.currencies?.coins, sample.coins]
    : key === 'rubies'
      ? [sample.currencies?.['resource:2'], sample.currencies?.rubies, sample.rubies]
      : [sample.currencies?.[key]];
  for (const candidate of candidates) {
    const amount = Number(candidate);
    if (Number.isFinite(amount)) return amount;
  }
  return undefined;
}

function collectLiveIDs(target: Set<string>, values: Record<string, number> | null | undefined) {
  for (const [rawID, rawAmount] of Object.entries(values ?? {})) {
    if (!/^[1-9]\d*$/.test(rawID) || !isVisibleBalance(rawAmount)) continue;
    target.add(rawID);
  }
}

function isVisibleBalance(value: unknown): boolean {
  const amount = Number(value);
  return Number.isFinite(amount) && amount !== 0;
}

function numericIDOrder(left: string, right: string): number {
  return Number(left) - Number(right);
}

export const AUTO_BUYER_SECTION = 'automation.autoBuyer';
export const AUTO_BUYER_MINIMUM_SPECIALIST_DAYS = 14;

export interface AutoBuyerPackageRuleV1 {
  enabled: boolean;
  shopId: string;
  packageId: number;
  targetPurchasesPerReset: number;
  minimumBalanceReserve: number;
  maximumRubySpendPerReset: number;
}

export interface AutoBuyerSpecialistRuleV1 {
  enabled: boolean;
  id: number;
  minimumDays: number;
  maximumRubyCostPerPurchase: number;
}

export interface AutoBuyerFeastSettingsV1 {
  enabled: boolean;
  feastId: number;
  minimumRemainingHours: number;
  sourceCastleId: number;
  minimumFoodReserve: number;
  allowRubies: boolean;
  maximumRubyCostPerPurchase: number;
}

export interface AutoBuyerClientStateV1 {
  version: 1;
  checkIntervalSec: number;
  historyRefreshSec: number;
  sourceCastleId: number;
  minimumRubyReserve: number;
  allowRubyPackages: boolean;
  packages: AutoBuyerPackageRuleV1[];
  specialists: AutoBuyerSpecialistRuleV1[];
  feast: AutoBuyerFeastSettingsV1;
}

export function defaultAutoBuyerClientState(): AutoBuyerClientStateV1 {
	return {
		version: 1,
		checkIntervalSec: 1800,
		historyRefreshSec: 3600,
    sourceCastleId: 0,
    minimumRubyReserve: 0,
    allowRubyPackages: false,
    packages: [],
    specialists: [],
    feast: {
      enabled: false,
      feastId: 0,
      minimumRemainingHours: 12,
      sourceCastleId: 0,
      minimumFoodReserve: 0,
      allowRubies: false,
      maximumRubyCostPerPurchase: 0,
    },
  };
}

export function parseAutoBuyerClientState(value: unknown): AutoBuyerClientStateV1 {
  const fallback = defaultAutoBuyerClientState();
  if (!isRecord(value)) return fallback;
  const feast = isRecord(value.feast) ? value.feast : {};
  const seenPackages = new Set<string>();
  const packages = Array.isArray(value.packages) ? value.packages.flatMap((candidate) => {
    if (!isRecord(candidate)) return [];
    const shopId = typeof candidate.shopId === 'string' ? candidate.shopId.trim() : '';
    const packageId = clampAutoBuyerInteger(candidate.packageId, 1, Number.MAX_SAFE_INTEGER, 0);
    const key = `${shopId}:${packageId}`;
    if (!shopId || packageId <= 0 || seenPackages.has(key)) return [];
    seenPackages.add(key);
    return [{
      enabled: candidate.enabled === true,
      shopId,
      packageId,
      targetPurchasesPerReset: clampAutoBuyerInteger(candidate.targetPurchasesPerReset, 1, Number.MAX_SAFE_INTEGER, 1),
      minimumBalanceReserve: clampAutoBuyerInteger(candidate.minimumBalanceReserve, 0, Number.MAX_SAFE_INTEGER, 0),
      maximumRubySpendPerReset: clampAutoBuyerInteger(candidate.maximumRubySpendPerReset, 0, Number.MAX_SAFE_INTEGER, 0),
    } satisfies AutoBuyerPackageRuleV1];
  }) : [];
  const seenSpecialists = new Set<number>();
  const specialists = Array.isArray(value.specialists) ? value.specialists.flatMap((candidate) => {
    if (!isRecord(candidate)) return [];
    const id = clampAutoBuyerInteger(candidate.id, 0, Number.MAX_SAFE_INTEGER, -1);
    if (id < 0 || seenSpecialists.has(id)) return [];
    seenSpecialists.add(id);
    return [{
      enabled: candidate.enabled === true,
      id,
      minimumDays: clampAutoBuyerInteger(
        candidate.minimumDays,
        AUTO_BUYER_MINIMUM_SPECIALIST_DAYS,
        365,
        AUTO_BUYER_MINIMUM_SPECIALIST_DAYS,
      ),
      maximumRubyCostPerPurchase: clampAutoBuyerInteger(candidate.maximumRubyCostPerPurchase, 0, Number.MAX_SAFE_INTEGER, 0),
    } satisfies AutoBuyerSpecialistRuleV1];
  }) : [];

  return {
    version: 1,
		checkIntervalSec: clampAutoBuyerInteger(value.checkIntervalSec, 1800, 3600, fallback.checkIntervalSec),
		historyRefreshSec: clampAutoBuyerInteger(value.historyRefreshSec, 3600, 3600, fallback.historyRefreshSec),
    sourceCastleId: clampAutoBuyerInteger(value.sourceCastleId, 0, Number.MAX_SAFE_INTEGER, 0),
    minimumRubyReserve: clampAutoBuyerInteger(value.minimumRubyReserve, 0, Number.MAX_SAFE_INTEGER, 0),
    allowRubyPackages: value.allowRubyPackages === true,
    packages,
    specialists,
    feast: {
      enabled: feast.enabled === true,
      feastId: clampAutoBuyerInteger(feast.feastId, 0, Number.MAX_SAFE_INTEGER, 0),
      minimumRemainingHours: clampAutoBuyerInteger(feast.minimumRemainingHours, 1, 24 * 30, fallback.feast.minimumRemainingHours),
      sourceCastleId: clampAutoBuyerInteger(feast.sourceCastleId, 0, Number.MAX_SAFE_INTEGER, 0),
      minimumFoodReserve: clampAutoBuyerInteger(feast.minimumFoodReserve, 0, Number.MAX_SAFE_INTEGER, 0),
      allowRubies: feast.allowRubies === true,
      maximumRubyCostPerPurchase: clampAutoBuyerInteger(feast.maximumRubyCostPerPurchase, 0, Number.MAX_SAFE_INTEGER, 0),
    },
  };
}

export function clampAutoBuyerInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

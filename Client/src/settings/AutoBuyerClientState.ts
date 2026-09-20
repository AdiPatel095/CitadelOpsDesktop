import type { AutoBuyerProjectionV1 } from '../api/Contracts';

export const AUTO_BUYER_SECTION = 'automation.autoBuyer';
export const AUTO_BUYER_MINIMUM_SPECIALIST_DAYS = 14;
export const AUTO_BUYER_MAXIMUM_SPECIALIST_DAYS = 365;
export const AUTO_BUYER_RUBY_FRESHNESS_MS = 60_000;

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

export interface AutoBuyerSpecialistEvidenceStatusInput {
  outcome: string;
  attemptedAt: string;
  updatedAt: string;
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
      feastId: 8,
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

// Catalog drift in an unchanged saved goal must not prevent editing feast
// upkeep. New/changed spending goals and their shared limits still validate.
export function autoBuyerOtherGoalsValid(
  draft: AutoBuyerClientStateV1,
  saved: AutoBuyerClientStateV1,
  catalog: AutoBuyerProjectionV1,
): boolean {
  const packageLimitsUnchanged = draft.sourceCastleId === saved.sourceCastleId
    && draft.allowRubyPackages === saved.allowRubyPackages
    && draft.minimumRubyReserve === saved.minimumRubyReserve;
  for (const rule of draft.packages.filter((candidate) => candidate.enabled)) {
    const previous = saved.packages.find((candidate) => candidate.shopId === rule.shopId && candidate.packageId === rule.packageId);
    if (packageLimitsUnchanged && previous && JSON.stringify(rule) === JSON.stringify(previous)) continue;
    const product = catalog.packages.find((candidate) => candidate.shopId === rule.shopId && candidate.packageId === rule.packageId);
    if (draft.sourceCastleId <= 0 || !product || rule.targetPurchasesPerReset < 1 || rule.targetPurchasesPerReset > product.stock) return false;
    if (product.price.premium && (!draft.allowRubyPackages || rule.maximumRubySpendPerReset < product.price.amount)) return false;
  }
  for (const rule of draft.specialists.filter((candidate) => candidate.enabled)) {
		const previous = saved.specialists.find((candidate) => candidate.id === rule.id);
		if (catalog.specialistUpkeep?.supported !== true) {
			if (draft.minimumRubyReserve === saved.minimumRubyReserve && previous && JSON.stringify(rule) === JSON.stringify(previous)) continue;
			return false;
		}
		if (draft.minimumRubyReserve === saved.minimumRubyReserve && previous && JSON.stringify(rule) === JSON.stringify(previous)) continue;
		const specialist = catalog.specialists.find((candidate) => candidate.id === rule.id);
		const safeMaximum = specialist?.validatedMaximumRubyCost ?? 0;
		if (!specialist || safeMaximum <= 0 || specialistMinimumDaysError(rule.minimumDays) || specialistRubyCeilingError(rule.maximumRubyCostPerPurchase, safeMaximum)) return false;
  }
  return true;
}

export function specialistMinimumDaysError(value: number): string {
  if (!Number.isSafeInteger(value) || value < AUTO_BUYER_MINIMUM_SPECIALIST_DAYS || value > AUTO_BUYER_MAXIMUM_SPECIALIST_DAYS) {
    return `Enter a whole number from ${AUTO_BUYER_MINIMUM_SPECIALIST_DAYS} to ${AUTO_BUYER_MAXIMUM_SPECIALIST_DAYS} days.`;
  }
  return '';
}

export function specialistRubyCeilingError(value: number, safeMaximum: number): string {
  if (!Number.isSafeInteger(value) || value < 0) return 'Enter a whole, non-negative ruby ceiling.';
  if (safeMaximum <= 0) return 'A validated ruby maximum is unavailable. Disable this specialist until pricing is supported.';
  if (value < safeMaximum) return `Set at least ${safeMaximum.toLocaleString()} rubies or disable this specialist.`;
  return '';
}

export function autoBuyerSpecialistRuntimeStatus(
  rule: AutoBuyerSpecialistRuleV1 | undefined,
  booster: { expiresAt?: string; permanent?: boolean } | undefined,
  latest: AutoBuyerSpecialistEvidenceStatusInput | null,
  safeMaximum: number,
  minimumRubyReserve: number,
  runtime: AutoBuyerProjectionV1['specialistRuntime'],
  timerFreshnessSec: number,
  now = Date.now(),
): string {
  if (!rule?.enabled) return 'Disabled';
  const latestOutcome = latest?.outcome.trim().toLowerCase() ?? '';
  if (latestOutcome === 'purchasing') return 'Purchasing';
  if (latestOutcome === 'verifying') return 'Verifying';
  if (latestOutcome === 'pending' || latestOutcome.includes('unresolved') || latestOutcome.includes('unconfirmed') || latestOutcome.includes('unverified')) {
    return 'Unresolved';
  }
  if (!runtime?.timersObservedAt) return 'Waiting for timer data';
  if (!runtime.timersCurrentSession) return 'Waiting for current-session timer data';
  const timerObservedAt = Date.parse(runtime.timersObservedAt);
  const timerFreshnessMs = timerFreshnessSec * 1000;
  if (!Number.isFinite(timerObservedAt) || timerObservedAt > now || !Number.isFinite(timerFreshnessMs) || timerFreshnessMs <= 0 || now - timerObservedAt >= timerFreshnessMs) {
    return 'Waiting for fresh timer data';
  }
  if (booster?.permanent) return 'Covered permanently';
  const expiry = booster?.expiresAt ? Date.parse(booster.expiresAt) : Number.NaN;
  if (Number.isFinite(expiry) && expiry > now + rule.minimumDays * 24 * 60 * 60 * 1000) return 'Covered';
  const latestAt = latest ? Date.parse(latest.updatedAt || latest.attemptedAt) : Number.NaN;
  if (latest && Number.isFinite(latestAt) && latestAt >= timerObservedAt) {
    if (latestOutcome === 'rejected') return 'Rejected';
    if (latestOutcome === 'not-sent') return 'Not sent · Waiting';
  }
  const inactive = !Number.isFinite(expiry) || expiry <= now;
  const prefix = inactive ? 'Inactive · ' : '';
  if (safeMaximum <= 0) return `${prefix}Waiting for validated price`;
  if (!Number.isSafeInteger(rule.maximumRubyCostPerPurchase) || rule.maximumRubyCostPerPurchase < safeMaximum) {
    return `${prefix}Waiting: ceiling below validated maximum`;
  }
  if (runtime.rubyBalance === undefined || !runtime.rubyObservedAt) return `${prefix}Waiting for ruby balance data`;
  if (!runtime.rubyCurrentSession) return `${prefix}Waiting for current-session ruby balance`;
  const rubyObservedAt = Date.parse(runtime.rubyObservedAt);
  if (!Number.isFinite(rubyObservedAt) || rubyObservedAt > now || now - rubyObservedAt > AUTO_BUYER_RUBY_FRESHNESS_MS) {
    return `${prefix}Waiting for fresh ruby balance`;
  }
  if (runtime.rubyBalance - minimumRubyReserve < safeMaximum) return `${prefix}Waiting for ruby reserve`;
  return inactive ? 'Inactive · Ready to activate' : 'Ready to renew';
}

export function clampAutoBuyerInteger(value: unknown, minimum: number, maximum: number, fallback: number): number {
  const parsed = Number(value);
  if (!Number.isFinite(parsed)) return fallback;
  return Math.min(maximum, Math.max(minimum, Math.trunc(parsed)));
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

import type { AutoStormTroopCapPreviewV2 } from '../api/Contracts';

export const AUTO_STORM_BASELINE_TROOPS = 5_000;

export interface AutoStormTroopCapPresentation {
  available: boolean;
  maximumTroops: number | null;
  baselineTroops: number;
  largestPresetTroops: number | null;
  enabledPresetCount: number | null;
  averagePresetTroops: number | null;
  resetSessionAvailable: boolean;
  resetSessionStartedAt?: string;
  attacksSinceReset: number | null;
  averageAttacksPerHour: number | null;
  rateBasedTroops: number | null;
  capBasis: 'baseline' | 'reset_rate' | 'reserve' | null;
  detail: string;
}

function finiteNonNegative(value: unknown): number | null {
  const parsed = Number(value);
  return Number.isFinite(parsed) && parsed >= 0 ? parsed : null;
}

function finiteInteger(value: unknown): number | null {
  const parsed = finiteNonNegative(value);
  return parsed == null ? null : Math.trunc(parsed);
}

export function presentAutoStormTroopCap(
  preview: AutoStormTroopCapPreviewV2 | null | undefined,
): AutoStormTroopCapPresentation {
  const maximumTroops = finiteInteger(preview?.maximumTroops);
  const baselineTroops = finiteInteger(preview?.baselineTroops) ?? AUTO_STORM_BASELINE_TROOPS;
  const enabledPresetCount = finiteInteger(preview?.enabledPresetCount);
  const capBasis = preview?.capBasis === 'baseline'
    || preview?.capBasis === 'reset_rate'
    || preview?.capBasis === 'reserve'
    ? preview.capBasis
    : null;
  const resetSessionStartedAt = typeof preview?.resetSessionStartedAt === 'string'
    && Number.isFinite(Date.parse(preview.resetSessionStartedAt))
    ? preview.resetSessionStartedAt
    : undefined;

  return {
    available: preview?.available === true && maximumTroops != null,
    maximumTroops,
    baselineTroops,
    largestPresetTroops: finiteInteger(preview?.troopsPerAttack),
    enabledPresetCount,
    averagePresetTroops: enabledPresetCount != null && enabledPresetCount > 0
      ? finiteNonNegative(preview?.averagePresetTroops)
      : null,
    resetSessionAvailable: preview?.resetSessionAvailable === true,
    ...(resetSessionStartedAt ? { resetSessionStartedAt } : {}),
    attacksSinceReset: finiteInteger(preview?.attacksSinceReset),
    averageAttacksPerHour: finiteNonNegative(preview?.averageAttacksPerHour),
    rateBasedTroops: finiteInteger(preview?.rateBasedTroops),
    capBasis,
    detail: typeof preview?.detail === 'string' ? preview.detail.trim() : '',
  };
}

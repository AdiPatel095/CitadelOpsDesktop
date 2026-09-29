import type { MessageKey, MessageParameters } from '../../i18n/messages';

/**
 * Collapsed-section summaries (CIT-17). Pure builders over the modal's draft
 * (never configuration directly), so a summary reflects unsaved edits. Each
 * line is a message key plus parameters; the section renders them joined.
 * Player language only: no "runtime" (guarded by tests/settings-summaries).
 */
export interface SettingsSummaryLine {
  messageKey: MessageKey;
  params?: MessageParameters;
}

const line = (messageKey: MessageKey, params?: MessageParameters): SettingsSummaryLine => (
  params ? { messageKey, params } : { messageKey }
);

export type DurationUnit = 'second' | 'minute' | 'hour' | 'day';

/** A stored second count in the largest familiar unit that divides it exactly (stored semantics unchanged). */
export function durationParts(seconds: number): { count: number; unit: DurationUnit } {
  const value = Math.max(0, Math.round(Number.isFinite(seconds) ? seconds : 0));
  if (value > 0 && value % 86_400 === 0) return { count: value / 86_400, unit: 'day' };
  if (value > 0 && value % 3_600 === 0) return { count: value / 3_600, unit: 'hour' };
  if (value > 0 && value % 60 === 0) return { count: value / 60, unit: 'minute' };
  return { count: value, unit: 'second' };
}

export function checkIntervalLine(seconds: number): SettingsSummaryLine {
  return line('settingsSummary.checkEvery', durationParts(seconds));
}

export function travelBoostKind(horseTravelBoostId: number): 'coins' | 'rubies' | 'feather' {
  if (horseTravelBoostId === 1007) return 'coins';
  if (horseTravelBoostId === 1008 || horseTravelBoostId === 1009) return 'rubies';
  return 'feather';
}

/** Paid travel is a consumable choice: always summarized. */
export function travelLine(horseTravelBoostId: number): SettingsSummaryLine {
  return line('settingsSummary.travel', { boost: travelBoostKind(horseTravelBoostId) });
}

export function reservedSkipCount(reserve: Readonly<Record<string, number>> | undefined): number {
  return Object.values(reserve ?? {}).reduce((total, amount) => total + (Number.isFinite(amount) && amount > 0 ? Math.trunc(amount) : 0), 0);
}

/** Time-skip use and how many skips stay reserved. */
export function timeSkipLines(enabled: boolean, reserve?: Readonly<Record<string, number>>): SettingsSummaryLine[] {
  if (!enabled) return [line('settingsSummary.timeSkips', { enabled: 'off' })];
  return [
    line('settingsSummary.timeSkips', { enabled: 'on' }),
    line('settingsSummary.skipReserve', { count: reservedSkipCount(reserve) }),
  ];
}

export function toggleLine(messageKey: MessageKey, enabled: boolean): SettingsSummaryLine {
  return line(messageKey, { enabled: enabled ? 'on' : 'off' });
}

export function countLine(messageKey: MessageKey, count: number, params: MessageParameters = {}): SettingsSummaryLine {
  return line(messageKey, { ...params, count });
}

/** Number of listed fields whose value differs from the feature default. */
export function countCustomValues<T extends object>(current: T, defaults: T, fields: readonly (keyof T)[]): number {
  return fields.filter((field) => JSON.stringify(current[field]) !== JSON.stringify(defaults[field])).length;
}

// ——— Per-feature summaries ———

export function foodTimingSummary(settings: { checkIntervalSec: number; minimumShipmentSize: number; minimumStormShipmentSize: number }): SettingsSummaryLine[] {
  return [
    checkIntervalLine(settings.checkIntervalSec),
    line('settingsSummary.foodShipments', { kingdom: settings.minimumShipmentSize, storm: settings.minimumStormShipmentSize }),
  ];
}

export function stationFiltersSummary(state: { minRPTDays: number; openGateFallback: boolean }): SettingsSummaryLine[] {
  return [
    line('settingsSummary.stationProtection', { days: state.minRPTDays }),
    toggleLine('settingsSummary.openGateFallback', state.openGateFallback),
  ];
}

export function boosterEvidenceSummary(hasPurchase: boolean): SettingsSummaryLine[] {
  return [line('settingsSummary.boosterEvidence', { state: hasPurchase ? 'recorded' : 'none' })];
}

export function towerAdvisorSummary(settings: { useAdvisor: boolean; autoActivateAdvisor: boolean; maximumDailyTimeSkips: number }): SettingsSummaryLine[] {
  if (!settings.useAdvisor) return [line('settingsSummary.towerAdvisor', { advisor: 'off', activate: 'off', skips: 0 })];
  return [line('settingsSummary.towerAdvisor', {
    advisor: 'on',
    activate: settings.autoActivateAdvisor ? 'on' : 'off',
    skips: settings.maximumDailyTimeSkips,
  })];
}

export function towerScanSummary(mapRefreshIntervalSec: number, enabledCastles: readonly { radius: number; maidenOnly: boolean }[]): SettingsSummaryLine[] {
  const lines = [line('settingsSummary.mapScanEvery', durationParts(mapRefreshIntervalSec))];
  if (enabledCastles.length > 0) {
    const radii = enabledCastles.map((castle) => castle.radius);
    const min = Math.min(...radii);
    const max = Math.max(...radii);
    lines.push(line('settingsSummary.towerRadius', { range: min === max ? 'single' : 'range', min, max }));
  }
  lines.push(line('settingsSummary.maidenOnly', { count: enabledCastles.filter((castle) => castle.maidenOnly).length }));
  return lines;
}

export function birdTimingSummary(settings: { minDelay: number; maxDelay: number; minSend: number }): SettingsSummaryLine[] {
  return [line('settingsSummary.birdTiming', settings)];
}

/** Camp cooldown skipping (Nomad, Khan): spends inventory time skips, keeping the listed reserve. */
export function cooldownSkipLines(enabled: boolean, reserve?: Readonly<Record<string, number>>): SettingsSummaryLine[] {
  if (!enabled) return [line('settingsSummary.cooldownSkips', { enabled: 'off' })];
  return [
    line('settingsSummary.cooldownSkips', { enabled: 'on' }),
    line('settingsSummary.skipReserve', { count: reservedSkipCount(reserve) }),
  ];
}

export function rbcTrialLine(trial: { enabled: boolean; targetX: number; targetY: number }): SettingsSummaryLine {
  return line('settingsSummary.rbcTrial', { enabled: trial.enabled ? 'on' : 'off', x: String(trial.targetX), y: String(trial.targetY) });
}

export function khanStopLimitsSummary(draft: { maxRageChain: number; requireActiveRageBooster: boolean; nomadPointThreshold: number }): SettingsSummaryLine[] {
  return [
    line('settingsSummary.khanRageChain', { count: draft.maxRageChain }),
    toggleLine('settingsSummary.khanRageBooster', draft.requireActiveRageBooster),
    line('settingsSummary.khanNomadPoints', { points: draft.nomadPointThreshold }),
  ];
}

export function beriBuildOptionsSummary(build: { allowPremium: boolean; allowDemolition: boolean; allowTimeSkips: boolean; timeSkipReserve: Readonly<Record<string, number>> }): SettingsSummaryLine[] {
  return [
    toggleLine('settingsSummary.buildPremium', build.allowPremium),
    toggleLine('settingsSummary.buildDemolition', build.allowDemolition),
    ...timeSkipLines(build.allowTimeSkips, build.timeSkipReserve),
  ];
}

export function beriAttackOptionsSummary(settings: { attackCheckIntervalSec: number; toolMinimums: Readonly<Record<string, number>>; useTroopTransportTimeSkips: boolean }): SettingsSummaryLine[] {
  return [
    checkIntervalLine(settings.attackCheckIntervalSec),
    line('settingsSummary.armorerMinimums', { count: Object.values(settings.toolMinimums).filter((amount) => amount > 0).length }),
    toggleLine('settingsSummary.transportSkips', settings.useTroopTransportTimeSkips),
  ];
}

export function advisorRunSizingLine(draft: { maxAttackCount: number; minimumRemainingSec: number }): SettingsSummaryLine {
  return line('settingsSummary.advisorRunSizing', { max: draft.maxAttackCount, minutes: Math.round(draft.minimumRemainingSec / 60) });
}

export function mapRefreshLine(seconds: number): SettingsSummaryLine {
  return line('settingsSummary.mapRefreshEvery', durationParts(seconds));
}

export function stormImportTuningSummary(troopImport: { enabled: boolean; minimumTroops: number }): SettingsSummaryLine[] {
  return [line('settingsSummary.stormImportTuning', { enabled: troopImport.enabled ? 'on' : 'off', troops: troopImport.minimumTroops })];
}

export function stormPriorityLine(activeTargetTypes: number): SettingsSummaryLine {
  return line('settingsSummary.stormPriority', { count: activeTargetTypes });
}

export function stormConstructionSummary(
  draft: {
    build: { allowResourceTransport: boolean; allowTimeSkips: boolean; allowPremium: boolean; allowDemolition: boolean; timeSkipReserve: Readonly<Record<string, number>> };
    harbor: { enabled: boolean; targetLevel: number };
    decorationPresetId: string;
  },
  buildActive: boolean,
): SettingsSummaryLine[] {
  return [
    toggleLine('settingsSummary.stormBuildTarget', buildActive),
    toggleLine('settingsSummary.buildPremium', draft.build.allowPremium),
    toggleLine('settingsSummary.buildDemolition', draft.build.allowDemolition),
    toggleLine('settingsSummary.resourceTransport', draft.build.allowResourceTransport),
    ...timeSkipLines(draft.build.allowTimeSkips, draft.build.timeSkipReserve),
    line('settingsSummary.harbor', { enabled: draft.harbor.enabled ? 'on' : 'off', level: draft.harbor.targetLevel }),
    toggleLine('settingsSummary.decoration', draft.decorationPresetId !== ''),
  ];
}

export function tciPresetsSummary(count: number, appliedName: string): SettingsSummaryLine[] {
  return [line('settingsSummary.savedPresets', { count, applied: appliedName ? 'yes' : 'no', name: appliedName })];
}

export function sceatTimingSummary(settings: { checkIntervalSec: number; minimumShipmentSize: number; overflowThresholdPercent: number }): SettingsSummaryLine[] {
  return [
    checkIntervalLine(settings.checkIntervalSec),
    line('settingsSummary.sceatShipments', { minimum: settings.minimumShipmentSize, overflow: settings.overflowThresholdPercent }),
  ];
}

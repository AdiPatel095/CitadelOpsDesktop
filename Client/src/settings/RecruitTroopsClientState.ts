import { queueConfigurationUpdate } from './Configuration';

export const RECRUIT_TROOPS_SETTINGS_STORAGE_KEY = 'recruitTroopsSettings';
export const RECRUIT_TROOPS_SETTINGS_CHANGED_EVENT = 'recruitTroopsSettingsChanged';
export const DEFAULT_RECRUIT_CHECK_INTERVAL_SEC = 300;
export const MIN_RECRUIT_CHECK_INTERVAL_SEC = 30;
export const MAX_RECRUIT_CHECK_INTERVAL_SEC = 86400;
export const RECRUIT_CHECK_INTERVAL_SEC_PER_MIN = 60;
export const MIN_RECRUIT_CHECK_INTERVAL_MIN = Math.ceil(
  MIN_RECRUIT_CHECK_INTERVAL_SEC / RECRUIT_CHECK_INTERVAL_SEC_PER_MIN,
);
export const MAX_RECRUIT_CHECK_INTERVAL_MIN = Math.floor(
  MAX_RECRUIT_CHECK_INTERVAL_SEC / RECRUIT_CHECK_INTERVAL_SEC_PER_MIN,
);
export const DEFAULT_RECRUIT_CHECK_INTERVAL_MIN = Math.round(
  DEFAULT_RECRUIT_CHECK_INTERVAL_SEC / RECRUIT_CHECK_INTERVAL_SEC_PER_MIN,
);

export type RecruitTroopsMode = 'global' | 'perCastle';

export interface RecruitTroopsItem {
  id: number;
  amount?: number;
}

export interface RecruitTroopsCastleSettings {
  enabled: boolean;
  items: RecruitTroopsItem[];
}

export interface RecruitTroopsClientSettingsV1 {
  version: 1;
  mode: RecruitTroopsMode;
  checkIntervalSec: number;
  globalItems: RecruitTroopsItem[];
  castles: Record<string, RecruitTroopsCastleSettings>;
}

export function recruitCastleScheduleID(castleID: number | string): string {
  return `autoRecruit:${castleID}`;
}

export function clampRecruitCheckIntervalSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_RECRUIT_CHECK_INTERVAL_SEC;
  return Math.min(
    MAX_RECRUIT_CHECK_INTERVAL_SEC,
    Math.max(MIN_RECRUIT_CHECK_INTERVAL_SEC, Math.round(value)),
  );
}

export function recruitCheckIntervalSecToMinutes(value: number): number {
  return Math.min(
    MAX_RECRUIT_CHECK_INTERVAL_MIN,
    Math.max(
      MIN_RECRUIT_CHECK_INTERVAL_MIN,
      Math.round(clampRecruitCheckIntervalSec(value) / RECRUIT_CHECK_INTERVAL_SEC_PER_MIN),
    ),
  );
}

export function recruitCheckIntervalMinutesToSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_RECRUIT_CHECK_INTERVAL_SEC;
  return clampRecruitCheckIntervalSec(Math.round(value) * RECRUIT_CHECK_INTERVAL_SEC_PER_MIN);
}

function normalizeItem(raw: unknown): RecruitTroopsItem | null {
  if (!raw || typeof raw !== 'object') return null;
  const item = raw as Partial<RecruitTroopsItem>;
  const id = Number(item.id);
  const amount = item.amount == null ? 0 : Number(item.amount);
  if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(amount) || amount < 0) return null;
  return {
    id: Math.floor(id),
    amount: Math.floor(amount),
  };
}

function normalizeItems(raw: unknown): RecruitTroopsItem[] {
  if (Array.isArray(raw)) {
    return raw.map(normalizeItem).filter((item): item is RecruitTroopsItem => item != null);
  }

  if (raw && typeof raw === 'object') {
    return Object.entries(raw as Record<string, unknown>)
      .map(([unitID, amount]) => {
        const id = Number(unitID);
        const count = Number(amount);
        if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(count) || count < 0) return null;
        return { id: Math.floor(id), amount: Math.floor(count) };
      })
      .filter((item): item is RecruitTroopsItem => item != null);
  }

  return [];
}

function targetsMapToCastles(raw: unknown, enabledRaw?: unknown): Record<string, RecruitTroopsCastleSettings> {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};

  const enabledMap =
    enabledRaw && typeof enabledRaw === 'object' && !Array.isArray(enabledRaw)
      ? (enabledRaw as Record<string, unknown>)
      : {};

  const castles: Record<string, RecruitTroopsCastleSettings> = {};
  Object.entries(raw as Record<string, unknown>).forEach(([castleID, targets]) => {
    const items = normalizeItems(targets);
    castles[castleID] = {
      enabled: typeof enabledMap[castleID] === 'boolean' ? !!enabledMap[castleID] : items.length > 0,
      items,
    };
  });

  Object.entries(enabledMap).forEach(([castleID, enabled]) => {
    if (castles[castleID]) return;
    castles[castleID] = {
      enabled: !!enabled,
      items: [],
    };
  });

  return castles;
}

function defaultRecruitTroopsSettings(): RecruitTroopsClientSettingsV1 {
  return {
    version: 1,
    mode: 'global',
    checkIntervalSec: DEFAULT_RECRUIT_CHECK_INTERVAL_SEC,
    globalItems: [],
    castles: {},
  };
}

export function normalizeRecruitTroopsSettings(raw: unknown): RecruitTroopsClientSettingsV1 {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return defaultRecruitTroopsSettings();
  }

  const payload = raw as Record<string, unknown>;
  const hasModernShape =
    'mode' in payload ||
    'checkIntervalSec' in payload ||
    'globalItems' in payload ||
    'globalTargets' in payload ||
    'enabledCastles' in payload ||
    'castles' in payload ||
    'targets' in payload;

  if (!hasModernShape) {
    return {
      version: 1,
      mode: 'perCastle',
      checkIntervalSec: DEFAULT_RECRUIT_CHECK_INTERVAL_SEC,
      globalItems: [],
      castles: targetsMapToCastles(payload),
    };
  }

  const rawMode = payload.mode;
  const mode: RecruitTroopsMode = rawMode === 'perCastle' ? 'perCastle' : 'global';
  const targets = payload.targets;
  const castlesFromExplicitShape =
    payload.castles && typeof payload.castles === 'object' && !Array.isArray(payload.castles)
      ? Object.fromEntries(
          Object.entries(payload.castles as Record<string, unknown>).map(([castleID, value]) => {
            const castleValue = value && typeof value === 'object' ? (value as Record<string, unknown>) : {};
            return [
              castleID,
              {
                enabled: typeof castleValue.enabled === 'boolean' ? castleValue.enabled : normalizeItems(castleValue.items).length > 0,
                items: normalizeItems(castleValue.items),
              },
            ];
          }),
        )
      : {};

  const castles = {
    ...targetsMapToCastles(targets, payload.enabledCastles),
    ...castlesFromExplicitShape,
  };

  return {
    version: 1,
    mode,
    checkIntervalSec: clampRecruitCheckIntervalSec(Number(payload.checkIntervalSec)),
    globalItems: normalizeItems(payload.globalItems ?? payload.globalTargets),
    castles,
  };
}

export function loadRecruitTroopsSettingsFromStorage(): RecruitTroopsClientSettingsV1 {
  try {
    const raw = localStorage.getItem(RECRUIT_TROOPS_SETTINGS_STORAGE_KEY);
    if (!raw) return defaultRecruitTroopsSettings();
    return normalizeRecruitTroopsSettings(JSON.parse(raw));
  } catch {
    return defaultRecruitTroopsSettings();
  }
}

export function notifyRecruitTroopsSettingsChanged(settings: RecruitTroopsClientSettingsV1): void {
  window.dispatchEvent(new CustomEvent(RECRUIT_TROOPS_SETTINGS_CHANGED_EVENT, { detail: settings }));
}

export function applyRecruitTroopsSettingsToLocalStorage(settings: RecruitTroopsClientSettingsV1): void {
  const normalized = normalizeRecruitTroopsSettings(settings);
  localStorage.setItem(RECRUIT_TROOPS_SETTINGS_STORAGE_KEY, JSON.stringify(normalized));
  notifyRecruitTroopsSettingsChanged(normalized);
}

export function persistRecruitTroopsSettings(settings: RecruitTroopsClientSettingsV1): boolean {
  const normalized = normalizeRecruitTroopsSettings(settings);
  applyRecruitTroopsSettingsToLocalStorage(normalized);
  return queueConfigurationUpdate('automation.recruitTroops', normalized);
}

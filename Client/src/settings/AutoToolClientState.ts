import { queueConfigurationUpdate } from './Configuration';

export const AUTO_TOOL_SETTINGS_STORAGE_KEY = 'autoToolSettings';
export const AUTO_TOOL_SETTINGS_CHANGED_EVENT = 'autoToolSettingsChanged';
export const DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC = 300;
export const MIN_AUTO_TOOL_CHECK_INTERVAL_SEC = 30;
export const MAX_AUTO_TOOL_CHECK_INTERVAL_SEC = 86400;
export const AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN = 60;
export const MIN_AUTO_TOOL_CHECK_INTERVAL_MIN = Math.ceil(
  MIN_AUTO_TOOL_CHECK_INTERVAL_SEC / AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN,
);
export const MAX_AUTO_TOOL_CHECK_INTERVAL_MIN = Math.floor(
  MAX_AUTO_TOOL_CHECK_INTERVAL_SEC / AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN,
);
export const DEFAULT_AUTO_TOOL_CHECK_INTERVAL_MIN = Math.round(
  DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC / AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN,
);

export type AutoToolMode = 'global' | 'perCastle';

export interface AutoToolItem {
  id: number;
  amount?: number;
}

export interface AutoToolCastleSettings {
  enabled: boolean;
  items: AutoToolItem[];
}

export interface AutoToolClientSettingsV1 {
  version: 1;
  mode: AutoToolMode;
  checkIntervalSec: number;
  globalItems: AutoToolItem[];
  castles: Record<string, AutoToolCastleSettings>;
}

export function autoToolCastleScheduleID(castleID: number | string): string {
  return `autoTool:${castleID}`;
}

export function clampAutoToolCheckIntervalSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC;
  return Math.min(
    MAX_AUTO_TOOL_CHECK_INTERVAL_SEC,
    Math.max(MIN_AUTO_TOOL_CHECK_INTERVAL_SEC, Math.round(value)),
  );
}

export function autoToolCheckIntervalSecToMinutes(value: number): number {
  return Math.min(
    MAX_AUTO_TOOL_CHECK_INTERVAL_MIN,
    Math.max(
      MIN_AUTO_TOOL_CHECK_INTERVAL_MIN,
      Math.round(clampAutoToolCheckIntervalSec(value) / AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN),
    ),
  );
}

export function autoToolCheckIntervalMinutesToSec(value: number): number {
  if (!Number.isFinite(value)) return DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC;
  return clampAutoToolCheckIntervalSec(Math.round(value) * AUTO_TOOL_CHECK_INTERVAL_SEC_PER_MIN);
}

function normalizeItem(raw: unknown): AutoToolItem | null {
  if (!raw || typeof raw !== 'object') return null;
  const item = raw as Partial<AutoToolItem>;
  const id = Number(item.id);
  const amount = item.amount == null ? 0 : Number(item.amount);
  if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(amount) || amount < 0) return null;
  return {
    id: Math.floor(id),
    amount: Math.floor(amount),
  };
}

function normalizeItems(raw: unknown): AutoToolItem[] {
  if (Array.isArray(raw)) {
    return raw.map(normalizeItem).filter((item): item is AutoToolItem => item != null);
  }

  if (raw && typeof raw === 'object') {
    return Object.entries(raw as Record<string, unknown>)
      .map(([toolID, amount]) => {
        const id = Number(toolID);
        const count = Number(amount);
        if (!Number.isFinite(id) || id <= 0 || !Number.isFinite(count) || count < 0) return null;
        return { id: Math.floor(id), amount: Math.floor(count) };
      })
      .filter((item): item is AutoToolItem => item != null);
  }

  return [];
}

function targetsMapToCastles(raw: unknown, enabledRaw?: unknown): Record<string, AutoToolCastleSettings> {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) return {};

  const enabledMap =
    enabledRaw && typeof enabledRaw === 'object' && !Array.isArray(enabledRaw)
      ? (enabledRaw as Record<string, unknown>)
      : {};

  const castles: Record<string, AutoToolCastleSettings> = {};
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

function defaultAutoToolSettings(): AutoToolClientSettingsV1 {
  return {
    version: 1,
    mode: 'global',
    checkIntervalSec: DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC,
    globalItems: [],
    castles: {},
  };
}

export function normalizeAutoToolSettings(raw: unknown): AutoToolClientSettingsV1 {
  if (!raw || typeof raw !== 'object' || Array.isArray(raw)) {
    return defaultAutoToolSettings();
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
      checkIntervalSec: DEFAULT_AUTO_TOOL_CHECK_INTERVAL_SEC,
      globalItems: [],
      castles: targetsMapToCastles(payload),
    };
  }

  const rawMode = payload.mode;
  const mode: AutoToolMode = rawMode === 'perCastle' ? 'perCastle' : 'global';
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
    checkIntervalSec: clampAutoToolCheckIntervalSec(Number(payload.checkIntervalSec)),
    globalItems: normalizeItems(payload.globalItems ?? payload.globalTargets),
    castles,
  };
}

export function loadAutoToolSettingsFromStorage(): AutoToolClientSettingsV1 {
  try {
    const raw = localStorage.getItem(AUTO_TOOL_SETTINGS_STORAGE_KEY);
    if (!raw) return defaultAutoToolSettings();
    return normalizeAutoToolSettings(JSON.parse(raw));
  } catch {
    return defaultAutoToolSettings();
  }
}

export function notifyAutoToolSettingsChanged(settings: AutoToolClientSettingsV1): void {
  window.dispatchEvent(new CustomEvent(AUTO_TOOL_SETTINGS_CHANGED_EVENT, { detail: settings }));
}

export function applyAutoToolSettingsToLocalStorage(settings: AutoToolClientSettingsV1): void {
  const normalized = normalizeAutoToolSettings(settings);
  localStorage.setItem(AUTO_TOOL_SETTINGS_STORAGE_KEY, JSON.stringify(normalized));
  notifyAutoToolSettingsChanged(normalized);
}

export function persistAutoToolSettings(settings: AutoToolClientSettingsV1): boolean {
  const normalized = normalizeAutoToolSettings(settings);
  applyAutoToolSettingsToLocalStorage(normalized);
  return queueConfigurationUpdate('automation.autoTool', normalized);
}

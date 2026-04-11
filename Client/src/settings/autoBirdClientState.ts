import { FrontendWebsocket } from '../websocket';
import {
  loadPresetsFile,
  parsePresetsPayload,
  savePresetsFile,
  type PresetsFileV1,
} from './autobirdPresets';

export const AUTO_BIRD_IGNORE_STORAGE_KEY = 'autobirdSettings';

export interface AutoBirdStoredSettings {
  settings: Record<string, { id: number; amount: number }[]>;
  minDelay: number;
  maxDelay: number;
  minSend: number;
}

export interface AutoBirdClientStateV1 {
  version: 1;
  ignoreSettings: AutoBirdStoredSettings;
  presets: PresetsFileV1;
}

const defaultIgnore = (): AutoBirdStoredSettings => ({
  settings: {},
  minDelay: 6,
  maxDelay: 12,
  minSend: 0,
});

export function loadAutoBirdSettingsFromStorage(): AutoBirdStoredSettings {
  try {
    const raw = localStorage.getItem(AUTO_BIRD_IGNORE_STORAGE_KEY);
    if (!raw) return defaultIgnore();
    const parsed = JSON.parse(raw) as Partial<AutoBirdStoredSettings>;
    return {
      ...defaultIgnore(),
      ...parsed,
      settings: parsed.settings && typeof parsed.settings === 'object' ? parsed.settings : {},
    };
  } catch {
    return defaultIgnore();
  }
}

/** Normalizes server payload into a full v1 client document. */
export function parseAutoBirdClientState(raw: unknown): AutoBirdClientStateV1 {
  if (raw == null || typeof raw !== 'object') {
    return {
      version: 1,
      ignoreSettings: loadAutoBirdSettingsFromStorage(),
      presets: loadPresetsFile(),
    };
  }
  const o = raw as Record<string, unknown>;
  const ignoreRaw = o.ignoreSettings;
  let ignoreSettings = defaultIgnore();
  if (ignoreRaw && typeof ignoreRaw === 'object') {
    const ig = ignoreRaw as Partial<AutoBirdStoredSettings>;
    ignoreSettings = {
      ...defaultIgnore(),
      ...ig,
      settings: ig.settings && typeof ig.settings === 'object' ? ig.settings : {},
    };
  }
  const presets = parsePresetsPayload(o.presets);
  return { version: 1, ignoreSettings, presets };
}

/** Mirrors server state into localStorage (for offline toggle + troop picker). */
export function applyAutoBirdClientStateToLocalStorage(state: AutoBirdClientStateV1): void {
  localStorage.setItem(AUTO_BIRD_IGNORE_STORAGE_KEY, JSON.stringify(state.ignoreSettings));
  savePresetsFile(state.presets);
}

export function buildAutoBirdClientState(
  ignoreSettings: AutoBirdStoredSettings,
  presets: PresetsFileV1
): AutoBirdClientStateV1 {
  return { version: 1, ignoreSettings, presets };
}

/** Persists to localStorage and the Go data dir (same folder as DecorationPresets.json). */
export function persistAutoBirdClientState(state: AutoBirdClientStateV1): void {
  applyAutoBirdClientStateToLocalStorage(state);
  if (FrontendWebsocket.getStatus() === 'Connected') {
    FrontendWebsocket.sendMessage({ type: 'saveAutoBirdClientState', payload: state });
  }
}

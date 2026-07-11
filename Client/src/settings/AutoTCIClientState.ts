import { queueConfigurationUpdate } from './Configuration';
import { clampLevelCeiling, normalizeLevelRange } from '../components/TCIPickerModal';
import {
  loadPresetsFile,
  parsePresetsPayload,
  savePresetsFile,
  type PresetsFileV1,
} from './AutoTCIPresets';

export const AUTO_TCI_SETTINGS_STORAGE_KEY = 'autoTCISettings';

export type AutoTCIStoredItem = { id: number; amount: number; minLevel?: number };
export type AutoTCIStoredSettings = Record<string, AutoTCIStoredItem[]>;

export interface AutoTCIClientStateV1 {
  version: 1;
  targets: AutoTCIStoredSettings;
  presets: PresetsFileV1;
}

function normalizeTargets(raw: unknown): AutoTCIStoredSettings {
  if (raw == null || typeof raw !== 'object' || Array.isArray(raw)) {
    return {};
  }
  const out: AutoTCIStoredSettings = {};
  for (const [castleId, itemsRaw] of Object.entries(raw as Record<string, unknown>)) {
    if (Array.isArray(itemsRaw)) {
      out[castleId] = itemsRaw
        .filter((x) => x && typeof x === 'object')
        .map((item) => {
          const row = item as { id?: number; amount?: number; minLevel?: number };
          const range = normalizeLevelRange(
            typeof row.minLevel === 'number' ? row.minLevel : 1,
            typeof row.amount === 'number' ? row.amount : 1,
          );
          const stored: AutoTCIStoredItem = {
            id: typeof row.id === 'number' ? row.id : 0,
            amount: range.ceiling,
          };
          if (range.floor > 1) {
            stored.minLevel = range.floor;
          }
          return stored;
        })
        .filter((x) => x.id > 0);
      continue;
    }
    if (itemsRaw != null && typeof itemsRaw === 'object' && !Array.isArray(itemsRaw)) {
      out[castleId] = Object.entries(itemsRaw as Record<string, number>).map(([idStr, amount]) => ({
        id: parseInt(idStr, 10),
        amount: clampLevelCeiling(typeof amount === 'number' ? amount : 1),
        minLevel: 1,
      }));
    }
  }
  return out;
}

export function loadAutoTCISettingsFromStorage(): AutoTCIStoredSettings {
  try {
    const raw = localStorage.getItem(AUTO_TCI_SETTINGS_STORAGE_KEY);
    if (!raw) return {};
    return normalizeTargets(JSON.parse(raw));
  } catch {
    return {};
  }
}

/** Normalizes server payload into a full v1 client document. */
export function parseAutoTCIClientState(raw: unknown): AutoTCIClientStateV1 {
  if (raw == null || typeof raw !== 'object') {
    return {
      version: 1,
      targets: loadAutoTCISettingsFromStorage(),
      presets: loadPresetsFile(),
    };
  }
  const o = raw as Record<string, unknown>;
  const targets = normalizeTargets(o.targets);
  const presets = parsePresetsPayload(o.presets);
  return { version: 1, targets, presets };
}

/** Mirrors server state into localStorage (for offline toggle). */
export function applyAutoTCIClientStateToLocalStorage(state: AutoTCIClientStateV1): void {
  localStorage.setItem(AUTO_TCI_SETTINGS_STORAGE_KEY, JSON.stringify(state.targets));
  savePresetsFile(state.presets);
}

/** @deprecated Use applyAutoTCIClientStateToLocalStorage after parsing wire payload. */
export function applyAutoTCISettingsToLocalStorage(
  payload: Record<string, Record<string, number>>,
): void {
  applyAutoTCIClientStateToLocalStorage({
    version: 1,
    targets: normalizeTargets(payload),
    presets: loadPresetsFile(),
  });
}

export function buildAutoTCIClientState(
  targets: AutoTCIStoredSettings,
  presets: PresetsFileV1,
): AutoTCIClientStateV1 {
  return { version: 1, targets, presets };
}

/** Persists to localStorage and the Go data dir (same folder as AutoBird.json). */
export function persistAutoTCIClientState(state: AutoTCIClientStateV1): void {
  applyAutoTCIClientStateToLocalStorage(state);
  queueConfigurationUpdate('automation.constructionItems', state);
}

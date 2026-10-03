import type { GameStateV2 } from '../api/Contracts';
import { normalizeStormKeys, parseStormLegacyKey, stormLegacyKeyFor } from './stormRole';
import { queueConfigurationUpdate } from './Configuration';
import {
  emptyPresetsFile,
  parsePresetsPayload,
  type PresetsFileV1,
} from './AutoBirdPresets';

export interface AutoBirdStoredSettings {
  settings: Record<string, { id: number; amount: number }[]>;
  minDelay: number;
  maxDelay: number;
  minSend: number;
  minRPTDays: number;
}

export interface AutoBirdClientStateV2 {
  version: 2;
  stormLegacyKey?: string;
  activePresetId: string | null;
  ignoreSettings: AutoBirdStoredSettings;
  presets: PresetsFileV1;
}

export const defaultAutoBirdSettings = (): AutoBirdStoredSettings => ({
  settings: {},
  minDelay: 6,
  maxDelay: 12,
  minSend: 0,
  minRPTDays: 3,
});

export function parseAutoBirdClientState(raw: unknown): AutoBirdClientStateV2 {
  if (raw == null || typeof raw !== 'object') {
    return {
      version: 2,
      activePresetId: null,
      ignoreSettings: defaultAutoBirdSettings(),
      presets: emptyPresetsFile(),
    };
  }
  const o = raw as Record<string, unknown>;
  const ignoreRaw = o.ignoreSettings;
  let ignoreSettings = defaultAutoBirdSettings();
  if (ignoreRaw && typeof ignoreRaw === 'object') {
    const ig = ignoreRaw as Partial<AutoBirdStoredSettings>;
    ignoreSettings = {
      ...defaultAutoBirdSettings(),
      ...ig,
      settings: ig.settings && typeof ig.settings === 'object' ? ig.settings : {},
    };
  }
  const presets = parsePresetsPayload(o.presets);
  const activePresetId = typeof o.activePresetId === 'string' && o.activePresetId.trim()
    ? o.activePresetId.trim()
    : null;
  return { version: 2, activePresetId, ignoreSettings, presets, ...(parseStormLegacyKey(o.stormLegacyKey) ? { stormLegacyKey: parseStormLegacyKey(o.stormLegacyKey) } : {}) };
}

export function buildAutoBirdClientState(
  ignoreSettings: AutoBirdStoredSettings,
  presets: PresetsFileV1,
  activePresetId: string | null = null,
): AutoBirdClientStateV2 {
  return {
    version: 2,
    activePresetId: typeof activePresetId === 'string' && activePresetId.trim()
      ? activePresetId.trim()
      : null,
    ignoreSettings,
    presets,
  };
}

/**
 * Returns a complete Auto Bird section with a different runtime preset.
 * Other features can use this instead of copying the preset's troop reserves.
 */
export function activateAutoBirdPreset(raw: unknown, presetId: string | null): AutoBirdClientStateV2 {
  const state = parseAutoBirdClientState(raw);
  const normalizedID = typeof presetId === 'string' && presetId.trim() ? presetId.trim() : null;
  if (normalizedID && !state.presets.presets.some((preset) => preset.id === normalizedID)) {
    throw new Error(`Auto Bird preset ${normalizedID} does not exist.`);
  }
  return { ...state, activePresetId: normalizedID };
}

export function persistAutoBirdClientState(state: AutoBirdClientStateV2) {
  return queueConfigurationUpdate('automation.autoBird', state);
}


/** Save compatibility mirrors every reserve map for the current owned Storm castle. */
export function normalizeAutoBirdStormSettings(saved: AutoBirdClientStateV2, state: GameStateV2 | null): AutoBirdClientStateV2 {
  const { stormLegacyKey: previous, ...section } = saved;
  const options = { stormLegacyKey: previous, dualWrite: true };
  const ignoreSettings = { ...saved.ignoreSettings, settings: normalizeStormKeys(saved.ignoreSettings.settings, state, options) };
  const presets = { ...saved.presets, presets: saved.presets.presets.map((preset) => ({ ...preset, settings: normalizeStormKeys(preset.settings, state, options) })) };
  const hasStormEntry = [ignoreSettings.settings, ...presets.presets.map((preset) => preset.settings)].some((settings) => Object.hasOwn(settings, 'storm'));
  const stormLegacyKey = hasStormEntry ? stormLegacyKeyFor(state) : undefined;
  return { ...section, ...(stormLegacyKey ? { stormLegacyKey } : {}), ignoreSettings, presets };
}

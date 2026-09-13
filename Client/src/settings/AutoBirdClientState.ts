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
  return { version: 2, activePresetId, ignoreSettings, presets };
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

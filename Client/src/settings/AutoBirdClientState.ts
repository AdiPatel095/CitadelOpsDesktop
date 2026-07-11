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

export interface AutoBirdClientStateV1 {
  version: 1;
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

export function parseAutoBirdClientState(raw: unknown): AutoBirdClientStateV1 {
  if (raw == null || typeof raw !== 'object') {
    return {
      version: 1,
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
  return { version: 1, ignoreSettings, presets };
}

export function buildAutoBirdClientState(
  ignoreSettings: AutoBirdStoredSettings,
  presets: PresetsFileV1
): AutoBirdClientStateV1 {
  return { version: 1, ignoreSettings, presets };
}

export function persistAutoBirdClientState(state: AutoBirdClientStateV1): void {
  queueConfigurationUpdate('automation.autoBird', state);
}

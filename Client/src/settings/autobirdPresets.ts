const PRESETS_FILE_KEY = 'autobirdPresetsV1';

export interface AutoBirdPreset {
  id: string;
  name: string;
  settings: Record<string, { id: number; amount: number }[]>;
  minDelay: number;
  maxDelay: number;
  minSend: number;
}

export interface PresetsFileV1 {
  version: 1;
  lastSelectedPresetId: string | null;
  presets: AutoBirdPreset[];
}

function emptyFile(): PresetsFileV1 {
  return { version: 1, lastSelectedPresetId: null, presets: [] };
}

/** Normalizes server or local JSON into PresetsFileV1. */
export function parsePresetsPayload(raw: unknown): PresetsFileV1 {
  try {
    if (raw == null || typeof raw !== 'object') return emptyFile();
    const p = raw as Partial<PresetsFileV1>;
    if (p.version !== 1 || !Array.isArray(p.presets)) return emptyFile();
    return {
      version: 1,
      lastSelectedPresetId: typeof p.lastSelectedPresetId === 'string' ? p.lastSelectedPresetId : null,
      presets: p.presets.filter((x) => x && typeof x.id === 'string' && typeof x.name === 'string'),
    };
  } catch {
    return emptyFile();
  }
}

export function loadPresetsFile(): PresetsFileV1 {
  try {
    const raw = localStorage.getItem(PRESETS_FILE_KEY);
    if (!raw) return emptyFile();
    return parsePresetsPayload(JSON.parse(raw));
  } catch {
    return emptyFile();
  }
}

export function savePresetsFile(file: PresetsFileV1): void {
  localStorage.setItem(PRESETS_FILE_KEY, JSON.stringify(file));
}

export function snapshotFromForm(
  settings: Record<string, { id: number; amount: number }[]>,
  minDelay: number,
  maxDelay: number,
  minSend: number
): Pick<AutoBirdPreset, 'settings' | 'minDelay' | 'maxDelay' | 'minSend'> {
  return {
    settings: JSON.parse(JSON.stringify(settings)) as Record<string, { id: number; amount: number }[]>,
    minDelay,
    maxDelay,
    minSend,
  };
}

export function applyPresetToStoredShape(preset: AutoBirdPreset): {
  settings: Record<string, { id: number; amount: number }[]>;
  minDelay: number;
  maxDelay: number;
  minSend: number;
} {
  return {
    settings: JSON.parse(JSON.stringify(preset.settings)) as Record<string, { id: number; amount: number }[]>,
    minDelay: preset.minDelay,
    maxDelay: preset.maxDelay,
    minSend: preset.minSend,
  };
}

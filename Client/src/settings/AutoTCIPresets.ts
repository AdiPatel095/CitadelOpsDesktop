export interface AutoTCIPreset {
  id: string;
  name: string;
  settings: Record<string, { id: number; amount: number }[]>;
}

export interface PresetsFileV1 {
  version: 1;
  lastSelectedPresetId: string | null;
  presets: AutoTCIPreset[];
}

export function emptyPresetsFile(): PresetsFileV1 {
  return { version: 1, lastSelectedPresetId: null, presets: [] };
}

/** Normalizes server or local JSON into PresetsFileV1. */
export function parsePresetsPayload(raw: unknown): PresetsFileV1 {
  try {
    if (raw == null || typeof raw !== 'object') return emptyPresetsFile();
    const p = raw as Partial<PresetsFileV1>;
    if (p.version !== 1 || !Array.isArray(p.presets)) return emptyPresetsFile();
    return {
      version: 1,
      lastSelectedPresetId: typeof p.lastSelectedPresetId === 'string' ? p.lastSelectedPresetId : null,
      presets: p.presets.filter((x) => x && typeof x.id === 'string' && typeof x.name === 'string'),
    };
  } catch {
    return emptyPresetsFile();
  }
}

export function snapshotFromForm(
  settings: Record<string, { id: number; amount: number }[]>,
): Pick<AutoTCIPreset, 'settings'> {
  return {
    settings: JSON.parse(JSON.stringify(settings)) as Record<string, { id: number; amount: number }[]>,
  };
}

export function applyPresetToStoredShape(preset: AutoTCIPreset): {
  settings: Record<string, { id: number; amount: number }[]>;
} {
  return {
    settings: JSON.parse(JSON.stringify(preset.settings)) as Record<string, { id: number; amount: number }[]>,
  };
}

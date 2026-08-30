interface IdentifiedPreset {
  id: string;
}

// Preset editors understand only valid current-schema entries. Preserve any
// unparsed sibling verbatim so editing one preset cannot silently delete
// legacy or forward-version data from the same document.
export function buildPresetDocumentUpdate<T extends IdentifiedPreset>(
  rawDocument: unknown,
  currentPresets: readonly T[],
  nextPresets: readonly T[],
): Record<string, unknown> & { version: 1; presets: unknown[] } {
  const source = isRecord(rawDocument) ? rawDocument : {};
  if (source.version != null && source.version !== 1) {
    throw new Error(`Preset document version ${String(source.version)} is not supported by this client.`);
  }
  const rawPresets = Array.isArray(source.presets) ? source.presets : [];
  const currentByID = new Map(currentPresets.map((preset) => [preset.id, preset]));
  const remaining = new Map(nextPresets.map((preset) => [preset.id, preset]));
  const presets: unknown[] = [];

  for (const rawPreset of rawPresets) {
    const id = isRecord(rawPreset) && typeof rawPreset.id === 'string' ? rawPreset.id : '';
    const replacement = id ? remaining.get(id) : undefined;
    if (replacement) {
      // Parsed presets are normalized views of the raw document. Retain the
      // exact raw sibling when the caller did not replace that parsed object,
      // preserving unknown fields and forward-compatible nested data.
      presets.push(replacement === currentByID.get(id) ? rawPreset : replacement);
      remaining.delete(id);
      continue;
    }
    if (id && currentByID.has(id)) continue;
    presets.push(rawPreset);
  }
  presets.push(...remaining.values());
  return { ...source, version: 1, presets };
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return value != null && typeof value === 'object' && !Array.isArray(value);
}

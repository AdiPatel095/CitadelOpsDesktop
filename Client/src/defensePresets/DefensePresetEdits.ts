import { withoutRecordMarker } from '../presets/AppCreatedRecords';
import { cloneDefensePresetDraft, type AppDefensePreset, type DefensePresetDraft } from './DefensePresetTypes';

/**
 * Defense Presets page edits (CIT-16), the defense twin of `AttackPresetEdits`:
 * - saving the editor rebuilds the record from the draft, so an app-created
 *   preset edited or renamed here is promoted to a normal preset (no `app`);
 * - duplicating copies only: the copy is a normal preset and the original,
 *   including its `app` marker, is left untouched.
 */

/** Record written by the preset editor. It never carries `app`: editing an app-created preset promotes it. */
export function editedDefensePreset(
  existing: AppDefensePreset | undefined,
  draft: DefensePresetDraft,
  options: { id: string; now: string },
): AppDefensePreset {
  return {
    ...cloneDefensePresetDraft(draft),
    id: existing?.id ?? options.id,
    createdAt: existing?.createdAt ?? options.now,
    updatedAt: options.now,
  };
}

/** Appends a marker-free copy with a unique name; every existing record, including the source, is kept as is. */
export function duplicateDefensePreset(
  presets: readonly AppDefensePreset[],
  source: AppDefensePreset,
  options: { id: string; now: string },
): { duplicate: AppDefensePreset; presets: AppDefensePreset[] } {
  const duplicate: AppDefensePreset = {
    ...withoutRecordMarker({ ...source, ...cloneDefensePresetDraft(source) }),
    id: options.id,
    name: uniqueDefenseCopyName(source.name, presets),
    createdAt: options.now,
    updatedAt: options.now,
  };
  return { duplicate, presets: [...presets, duplicate] };
}

export function uniqueDefenseCopyName(name: string, presets: readonly { name: string }[]): string {
  const existing = new Set(presets.map((preset) => preset.name.toLowerCase()));
  let candidate = `${name} copy`;
  let suffix = 2;
  while (existing.has(candidate.toLowerCase())) candidate = `${name} copy ${suffix++}`;
  return candidate;
}

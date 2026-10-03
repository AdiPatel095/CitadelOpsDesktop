import type { AttackSetupDraft } from '../components/AttackSetupModal';
import { cloneInlineSetup, withoutMarker } from './AppCreatedPresets';
import type { AppAttackPreset, AttackPresetTargetType } from './AttackPresetTypes';

/**
 * Attack Presets page edits (CIT-15). Pure so the ownership rules are testable:
 * - saving the editor rebuilds the record from the draft, so an app-created
 *   preset edited or renamed here is promoted to a normal preset (no `app`);
 * - duplicating copies only: the copy is a normal preset and the original,
 *   including its `app` marker, is left untouched.
 */

/** Record written by the preset editor. It never carries `app`: editing an app-created preset promotes it. */
export function editedAttackPreset(
  existing: AppAttackPreset | undefined,
  draft: AttackSetupDraft,
  options: { id: string; targetType?: AttackPresetTargetType; now: string },
): AppAttackPreset {
  const composition = cloneInlineSetup({
    targetType: options.targetType ?? existing?.targetType ?? 'pve',
    useTroopFamilies: Boolean(draft.useTroopFamilies),
    waves: draft.waves,
    courtyardSupport: draft.courtyardSupport,
  });
  return {
    id: existing?.id ?? options.id,
    name: draft.name.trim(),
    ...composition,
    createdAt: existing?.createdAt ?? options.now,
    updatedAt: options.now,
  };
}

/** Appends a marker-free copy with a unique name; every existing record, including the source, is kept as is. */
export function duplicateAttackPreset(
  presets: readonly AppAttackPreset[],
  source: AppAttackPreset,
  options: { id: string; now: string },
): { duplicate: AppAttackPreset; presets: AppAttackPreset[] } {
  const duplicate: AppAttackPreset = {
    ...withoutMarker(source),
    ...cloneInlineSetup(source),
    id: options.id,
    name: uniqueCopyName(source.name, presets),
    createdAt: options.now,
    updatedAt: options.now,
  };
  return { duplicate, presets: [...presets, duplicate] };
}

export function uniqueCopyName(name: string, presets: readonly AppAttackPreset[]): string {
  const existing = new Set(presets.map((preset) => preset.name.toLowerCase()));
  let candidate = `${name} copy`;
  let suffix = 2;
  while (existing.has(candidate.toLowerCase())) candidate = `${name} copy ${suffix++}`;
  return candidate;
}

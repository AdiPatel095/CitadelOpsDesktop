import {
  isRecordOwnedBy,
  promoteRecords,
  recordOwner,
  removeUnreferencedRecords,
  uniqueRecordName,
  upsertOwnedRecord,
  type AppCreatedPresetMarker,
  type PresetDocumentValue,
  type RecordReference,
} from '../presets/AppCreatedRecords';
import {
  cloneDefensePresetDraft,
  normalizeDefensePresetSlots,
  summarizeDefensePreset,
  type AppDefensePreset,
  type DefensePresetDocument,
  type DefensePresetDraft,
  type DefensePresetSummary,
} from './DefensePresetTypes';
import { interpolate, messages } from '../i18n/messages';

/**
 * App-created defense presets (CIT-16): the `defense.presets` adapter over the
 * generic core in `presets/AppCreatedRecords.ts`. Khan's inline main-castle
 * defense is stored as an ordinary defense preset record with an `app` marker;
 * the runtime keeps resolving `defensePresetId` unchanged.
 */

/** A defense composition without the preset name. */
export type InlineDefenseSetup = Omit<DefensePresetDraft, 'name'>;

export type DefenseSetupRef =
  | { source: 'none' }
  | { source: 'preset'; presetId: string; missing: boolean; appCreatedBy?: AppCreatedPresetMarker }
  | { source: 'inline'; presetId: string; setup: InlineDefenseSetup; missing: boolean };

export interface DefenseSetupRefSummary {
  name: string;
  summary: DefensePresetSummary | null;
  missing: boolean;
  badge: 'app' | null;
}

export type DefensePresetDocumentValue = PresetDocumentValue;

export function inlineDefenseFromPreset(preset: DefensePresetDraft): InlineDefenseSetup {
  const draft = cloneDefensePresetDraft(preset);
  return {
    wall: draft.wall,
    moat: draft.moat,
    ...(draft.keep ? { keep: draft.keep } : {}),
    ...(draft.sourceCastleId != null ? { sourceCastleId: draft.sourceCastleId } : {}),
    ...(draft.sourceCastleName ? { sourceCastleName: draft.sourceCastleName } : {}),
  };
}

export function cloneInlineDefense(setup: InlineDefenseSetup): InlineDefenseSetup {
  return inlineDefenseFromPreset({ name: '', ...setup });
}

export function inlineDefensesEqual(left: InlineDefenseSetup, right: InlineDefenseSetup): boolean {
  return JSON.stringify(comparableDefense(left)) === JSON.stringify(comparableDefense(right));
}

function comparableDefense(setup: InlineDefenseSetup): unknown {
  const normalized = normalizeDefensePresetSlots({ name: '', ...setup });
  const slots = (list: { definitionId: number; amount: number }[]) => list.map((slot) => [slot.definitionId, slot.amount]);
  const wall = (section: DefensePresetDraft['wall']['left']) => [slots(section.toolSlots), section.unitPercent, section.unitTypePercent];
  return [
    wall(normalized.wall.left),
    wall(normalized.wall.middle),
    wall(normalized.wall.right),
    slots(normalized.moat.leftToolSlots),
    slots(normalized.moat.middleToolSlots),
    slots(normalized.moat.rightToolSlots),
    normalized.keep
      ? [normalized.keep.mauct, normalized.keep.unitTypePercent, slots(normalized.keep.primaryToolSlots ?? []), slots(normalized.keep.secondaryToolSlots ?? [])]
      : null,
  ];
}

/** Resolves a module slot's stored defense preset id against the defense presets document. */
export function defenseSetupRef(
  presetId: string,
  document: DefensePresetDocument,
  section: string,
  slot: string,
): DefenseSetupRef {
  const id = presetId.trim();
  if (!id) return { source: 'none' };
  const preset = document.presets.find((candidate) => candidate.id === id);
  if (!preset) return { source: 'preset', presetId: id, missing: true };
  if (isRecordOwnedBy(preset, section, slot)) {
    return { source: 'inline', presetId: id, setup: inlineDefenseFromPreset(preset), missing: false };
  }
  const owner = recordOwner(preset);
  return owner
    ? { source: 'preset', presetId: id, missing: false, appCreatedBy: owner }
    : { source: 'preset', presetId: id, missing: false };
}

export function summarizeDefenseSetupRef(ref: DefenseSetupRef, document: DefensePresetDocument): DefenseSetupRefSummary {
  if (ref.source === 'none') return { name: '', summary: null, missing: false, badge: null };
  const preset = ref.presetId ? document.presets.find((candidate) => candidate.id === ref.presetId) : undefined;
  if (ref.source === 'inline') {
    return {
      name: preset?.name ?? '',
      summary: summarizeDefensePreset({ name: '', ...ref.setup }),
      missing: ref.presetId !== '' && preset == null,
      badge: 'app',
    };
  }
  if (!preset) return { name: '', summary: null, missing: true, badge: null };
  return { name: preset.name, summary: summarizeDefensePreset(preset), missing: false, badge: preset.app ? 'app' : null };
}

/** True when the ref can be saved as a module reference: an existing record, or any inline defense. */
export function defenseSetupRefUsable(ref: DefenseSetupRef, document: DefensePresetDocument): boolean {
  if (ref.source === 'none') return false;
  if (ref.source === 'inline') return true;
  return !ref.missing && document.presets.some((preset) => preset.id === ref.presetId);
}

/** Generated title with a numeric suffix on collision. */
export function appCreatedDefensePresetName(
  existing: readonly { name: string }[],
  moduleLabel: string,
  slotLabel: string,
  formatName: (moduleLabel: string, slotLabel: string) => string = defaultAppCreatedName,
): string {
  return uniqueRecordName(existing, formatName(moduleLabel, slotLabel).trim() || defaultAppCreatedName(moduleLabel, slotLabel));
}

function defaultAppCreatedName(moduleLabel: string, slotLabel: string): string {
  return interpolate(messages['attackPresets.appCreatedName'], { module: moduleLabel, slot: slotLabel });
}

export function upsertOwnedAppCreatedDefensePreset(
  rawDocument: unknown,
  current: readonly AppDefensePreset[],
  section: string,
  slot: string,
  currentId: string,
  setup: InlineDefenseSetup,
  name: string,
  now: string = new Date().toISOString(),
): { document: DefensePresetDocumentValue; presetId: string; changed: boolean } {
  return upsertOwnedRecord(rawDocument, current, section, slot, currentId, {
    unchanged: (existing) => inlineDefensesEqual(inlineDefenseFromPreset(existing), setup),
    update: (existing) => ({ ...existing, ...normalizedSetup(setup), updatedAt: now }),
    create: (id) => ({ id, name, ...normalizedSetup(setup), createdAt: now, updatedAt: now, app: { section, slot } }),
  });
}

function normalizedSetup(setup: InlineDefenseSetup): InlineDefenseSetup {
  return inlineDefenseFromPreset(normalizeDefensePresetSlots({ name: '', ...setup }));
}

export function promoteAppCreatedDefensePresets(
  rawDocument: unknown,
  current: readonly AppDefensePreset[],
  ids: readonly string[],
): { document: DefensePresetDocumentValue; promoted: string[] } {
  return promoteRecords(rawDocument, current, ids);
}

export function removeUnreferencedAppCreatedDefense(
  rawDocument: unknown,
  current: readonly AppDefensePreset[],
  section: string,
  references: readonly RecordReference[],
): { document: DefensePresetDocumentValue; removed: string[]; promoted: string[] } {
  return removeUnreferencedRecords(rawDocument, current, section, references);
}

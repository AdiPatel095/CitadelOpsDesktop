import type {
  AttackSetupCourtyardSupport,
  AttackSetupWave,
} from '../components/AttackSetupModal';
import {
  isRecordOwnedBy,
  newAppCreatedRecordId,
  promoteRecords,
  recordOwner,
  removeUnreferencedRecords,
  uniqueRecordName,
  upsertOwnedRecord,
  withoutRecordMarker,
  type PresetDocumentValue,
} from '../presets/AppCreatedRecords';
import {
  summarizeAttackPreset,
  type AppAttackPreset,
  type AppCreatedPresetMarker,
  type AttackPresetDocument,
  type AttackPresetSummary,
  type AttackPresetTargetType,
} from './AttackPresetTypes';
import type { PresetReference } from './AttackPresetReferences';
import { interpolate, messages } from '../i18n/messages';

/**
 * App-created attack presets (CIT-15): the `attacks.presets` adapter over the
 * generic core in `presets/AppCreatedRecords.ts`. A module's inline setup is an
 * ordinary record carrying an `app` marker; the marker, not the id, defines
 * ownership, and any other reuse promotes the record to a normal user preset.
 */

export type AppCreatedPresetOwner = AppCreatedPresetMarker;

export interface InlineAttackSetup {
  targetType: AttackPresetTargetType;
  useTroopFamilies: boolean;
  waves: AttackSetupWave[];
  courtyardSupport: AttackSetupCourtyardSupport;
}

export type AttackSetupRef =
  | { source: 'none' }
  | { source: 'preset'; presetId: string; missing: boolean; appCreatedBy?: AppCreatedPresetOwner }
  | { source: 'inline'; presetId: string; setup: InlineAttackSetup; missing: boolean };

export interface AttackSetupRefSummary {
  name: string;
  summary: AttackPresetSummary | null;
  missing: boolean;
  badge: 'app' | null;
}

/** Raw `attacks.presets` document as written to configuration. */
export type AttackPresetDocumentValue = PresetDocumentValue;

export function isAppCreatedPreset(preset: AppAttackPreset): boolean {
  return preset.app != null;
}

export function appCreatedPresetOwner(preset: AppAttackPreset): AppCreatedPresetOwner | null {
  return recordOwner(preset);
}

export function isOwnedBy(preset: AppAttackPreset, section: string, slot: string): boolean {
  return isRecordOwnedBy(preset, section, slot);
}

/** Not derivable from the owner: a promoted record must never collide with the owner's next record. */
export function newAppCreatedPresetId(section: string, slot: string): string {
  return newAppCreatedRecordId(section, slot);
}

/**
 * Generated title with a numeric suffix on collision. `formatName` receives the
 * localized module and slot labels and returns the localized title pattern.
 */
export function appCreatedPresetName(
  existing: readonly { name: string }[],
  moduleLabel: string,
  slotLabel: string,
  formatName: (moduleLabel: string, slotLabel: string) => string = defaultAppCreatedName,
): string {
  return uniqueRecordName(existing, formatName(moduleLabel, slotLabel).trim() || defaultAppCreatedName(moduleLabel, slotLabel));
}

/** English catalog fallback; UI callers pass the localized pattern. */
function defaultAppCreatedName(moduleLabel: string, slotLabel: string): string {
  return interpolate(messages['attackPresets.appCreatedName'], { module: moduleLabel, slot: slotLabel });
}

export function inlineSetupFromPreset(preset: AppAttackPreset): InlineAttackSetup {
  return cloneInlineSetup({
    targetType: preset.targetType,
    useTroopFamilies: preset.useTroopFamilies,
    waves: preset.waves,
    courtyardSupport: preset.courtyardSupport,
  });
}

export function cloneInlineSetup(setup: InlineAttackSetup): InlineAttackSetup {
  return {
    targetType: setup.targetType,
    useTroopFamilies: setup.useTroopFamilies,
    waves: setup.waves.map((wave) => ({
      L: { troops: wave.L.troops.map((slot) => ({ ...slot })), tools: wave.L.tools.map((slot) => ({ ...slot })) },
      M: { troops: wave.M.troops.map((slot) => ({ ...slot })), tools: wave.M.tools.map((slot) => ({ ...slot })) },
      R: { troops: wave.R.troops.map((slot) => ({ ...slot })), tools: wave.R.tools.map((slot) => ({ ...slot })) },
    })),
    courtyardSupport: {
      troops: setup.courtyardSupport.troops.map((slot) => ({ ...slot })),
      tools: setup.courtyardSupport.tools.map((slot) => ({ ...slot })),
    },
  };
}

export function inlineSetupsEqual(left: InlineAttackSetup, right: InlineAttackSetup): boolean {
  return JSON.stringify(comparableSetup(left)) === JSON.stringify(comparableSetup(right));
}

function comparableSetup(setup: InlineAttackSetup): unknown {
  return [
    setup.targetType,
    setup.useTroopFamilies,
    setup.waves.map((wave) => [wave.L, wave.M, wave.R].map((lane) => [
      lane.troops.map((slot) => [slot.itemId, slot.quantity]),
      lane.tools.map((slot) => [slot.itemId, slot.quantity]),
    ])),
    setup.courtyardSupport.troops.map((slot) => [slot.itemId, slot.quantity]),
    setup.courtyardSupport.tools.map((slot) => [slot.itemId, slot.quantity]),
  ];
}

export function inlineSetupTroopCount(setup: InlineAttackSetup): number {
  return summarizeAttackPreset({ name: '', ...setup }).troops;
}

/** Resolves a module slot's stored preset id against the presets document. */
export function attackSetupRef(
  presetId: string,
  document: AttackPresetDocument,
  section: string,
  slot: string,
): AttackSetupRef {
  const id = presetId.trim();
  if (!id) return { source: 'none' };
  const preset = document.presets.find((candidate) => candidate.id === id);
  if (!preset) return { source: 'preset', presetId: id, missing: true };
  if (isOwnedBy(preset, section, slot)) {
    return { source: 'inline', presetId: id, setup: inlineSetupFromPreset(preset), missing: false };
  }
  const owner = appCreatedPresetOwner(preset);
  return owner
    ? { source: 'preset', presetId: id, missing: false, appCreatedBy: owner }
    : { source: 'preset', presetId: id, missing: false };
}

export function summarizeAttackSetupRef(ref: AttackSetupRef, document: AttackPresetDocument): AttackSetupRefSummary {
  if (ref.source === 'none') return { name: '', summary: null, missing: false, badge: null };
  const preset = ref.presetId ? document.presets.find((candidate) => candidate.id === ref.presetId) : undefined;
  if (ref.source === 'inline') {
    return {
      name: preset?.name ?? '',
      summary: summarizeAttackPreset({ name: '', ...ref.setup }),
      missing: ref.presetId !== '' && preset == null,
      badge: 'app',
    };
  }
  if (!preset) return { name: '', summary: null, missing: true, badge: null };
  return {
    name: preset.name,
    summary: summarizeAttackPreset(preset),
    missing: false,
    badge: preset.app ? 'app' : null,
  };
}

/** True when the ref can be saved as a module reference: an existing record, or an inline setup with troops. */
export function attackSetupRefUsable(ref: AttackSetupRef, document: AttackPresetDocument): boolean {
  if (ref.source === 'none') return false;
  if (ref.source === 'inline') return inlineSetupTroopCount(ref.setup) > 0;
  return !ref.missing && document.presets.some((preset) => preset.id === ref.presetId);
}

/**
 * Upserts this slot's app-created record. The current record is reused only
 * when its id matches AND its marker names this slot; otherwise a new record is
 * created. An unchanged composition leaves the document untouched.
 */
export function upsertOwnedAppCreatedPreset(
  rawDocument: unknown,
  current: readonly AppAttackPreset[],
  section: string,
  slot: string,
  currentId: string,
  setup: InlineAttackSetup,
  name: string,
  now: string = new Date().toISOString(),
): { document: AttackPresetDocumentValue; presetId: string; changed: boolean } {
  return upsertOwnedRecord(rawDocument, current, section, slot, currentId, {
    unchanged: (existing) => inlineSetupsEqual(inlineSetupFromPreset(existing), setup),
    update: (existing) => ({ ...existing, ...cloneInlineSetup(setup), updatedAt: now }),
    create: (id) => ({ id, name, ...cloneInlineSetup(setup), createdAt: now, updatedAt: now, app: { section, slot } }),
  });
}

/** Clears the `app` marker of the named app-created records; everything else is kept verbatim. */
export function promoteAppCreatedPresets(
  rawDocument: unknown,
  current: readonly AppAttackPreset[],
  ids: readonly string[],
): { document: AttackPresetDocumentValue; promoted: string[] } {
  return promoteRecords(rawDocument, current, ids);
}

/**
 * Cleanup for one module section: deletes this section's app-created records
 * that no reference points to, and promotes this section's records that another
 * slot references. Never touches user presets or other sections' records.
 */
export function removeUnreferencedAppCreated(
  rawDocument: unknown,
  current: readonly AppAttackPreset[],
  section: string,
  references: readonly PresetReference[],
): { document: AttackPresetDocumentValue; removed: string[]; promoted: string[] } {
  return removeUnreferencedRecords(rawDocument, current, section, references);
}

export function withoutMarker(preset: AppAttackPreset): AppAttackPreset {
  return withoutRecordMarker(preset);
}

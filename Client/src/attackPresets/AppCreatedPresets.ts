import type {
  AttackSetupCourtyardSupport,
  AttackSetupWave,
} from '../components/AttackSetupModal';
import { buildPresetDocumentUpdate } from '../configuration/PresetDocumentUpdate';
import {
  summarizeAttackPreset,
  type AppAttackPreset,
  type AppCreatedPresetMarker,
  type AttackPresetDocument,
  type AttackPresetSummary,
  type AttackPresetTargetType,
} from './AttackPresetTypes';
import type { AttackPresetReference } from './AttackPresetReferences';
import { interpolate, messages } from '../i18n/messages';

/**
 * App-created attack presets (CIT-15). A module's inline setup is persisted as
 * an ordinary `attacks.presets` record carrying an `app` marker, so the
 * unchanged runtime keeps resolving compositions by preset id. The marker, not
 * the id, defines ownership: only the owning module slot edits the record, and
 * only while it is the sole referrer. Any other reuse promotes the record to a
 * normal user preset by clearing the marker.
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
export type AttackPresetDocumentValue = Record<string, unknown> & { version: 1; presets: unknown[] };

export function isAppCreatedPreset(preset: AppAttackPreset): boolean {
  return preset.app != null;
}

export function appCreatedPresetOwner(preset: AppAttackPreset): AppCreatedPresetOwner | null {
  return preset.app ? { section: preset.app.section, slot: preset.app.slot } : null;
}

export function isOwnedBy(preset: AppAttackPreset, section: string, slot: string): boolean {
  return preset.app?.section === section && preset.app.slot === slot;
}

/** Not derivable from the owner: a promoted record must never collide with the owner's next record. */
export function newAppCreatedPresetId(section: string, slot: string): string {
  const bytes = new Uint8Array(4);
  globalThis.crypto.getRandomValues(bytes);
  const suffix = Array.from(bytes, (byte) => byte.toString(16).padStart(2, '0')).join('');
  return `app:${section}:${slot}:${suffix}`;
}

/**
 * Generated title with a numeric suffix on collision. `formatName` receives the
 * localized module and slot labels and returns the localized title pattern.
 */
export function appCreatedPresetName(
  existing: readonly AppAttackPreset[],
  moduleLabel: string,
  slotLabel: string,
  formatName: (moduleLabel: string, slotLabel: string) => string = defaultAppCreatedName,
): string {
  const base = formatName(moduleLabel, slotLabel).trim() || defaultAppCreatedName(moduleLabel, slotLabel);
  const taken = new Set(existing.map((preset) => preset.name.trim().toLowerCase()));
  let candidate = base;
  let suffix = 2;
  while (taken.has(candidate.toLowerCase())) candidate = `${base} ${suffix++}`;
  return candidate;
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
  const existing = currentId
    ? current.find((preset) => preset.id === currentId && isOwnedBy(preset, section, slot))
    : undefined;
  if (existing && inlineSetupsEqual(inlineSetupFromPreset(existing), setup)) {
    return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, current), presetId: existing.id, changed: false };
  }
  const composition = cloneInlineSetup(setup);
  const next: AppAttackPreset = existing
    ? { ...existing, ...composition, updatedAt: now }
    : {
      id: newAppCreatedPresetId(section, slot),
      name,
      ...composition,
      createdAt: now,
      updatedAt: now,
      app: { section, slot },
    };
  const presets = existing
    ? current.map((preset) => preset === existing ? next : preset)
    : [...current, next];
  return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, presets), presetId: next.id, changed: true };
}

/** Clears the `app` marker of the named app-created records; everything else is kept verbatim. */
export function promoteAppCreatedPresets(
  rawDocument: unknown,
  current: readonly AppAttackPreset[],
  ids: readonly string[],
): { document: AttackPresetDocumentValue; promoted: string[] } {
  const targets = new Set(ids);
  const promoted: string[] = [];
  const presets = current.map((preset) => {
    if (!targets.has(preset.id) || !preset.app) return preset;
    promoted.push(preset.id);
    return withoutMarker(preset);
  });
  return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, presets), promoted };
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
  references: readonly AttackPresetReference[],
): { document: AttackPresetDocumentValue; removed: string[]; promoted: string[] } {
  const removed: string[] = [];
  const promoted: string[] = [];
  const presets: AppAttackPreset[] = [];
  for (const preset of current) {
    if (preset.app?.section !== section) {
      presets.push(preset);
      continue;
    }
    const referrers = references.filter((reference) => reference.presetId === preset.id);
    if (referrers.length === 0) {
      removed.push(preset.id);
      continue;
    }
    const marker = preset.app;
    if (referrers.some((reference) => reference.section !== marker.section || reference.slot !== marker.slot)) {
      promoted.push(preset.id);
      presets.push(withoutMarker(preset));
      continue;
    }
    presets.push(preset);
  }
  return { document: buildPresetDocumentUpdate(rawDocument ?? {}, current, presets), removed, promoted };
}

export function withoutMarker(preset: AppAttackPreset): AppAttackPreset {
  const copy = { ...preset };
  delete copy.app;
  return copy;
}

import type { ConfigurationSnapshot } from '../api/Contracts';
import {
  appCreatedPresetName,
  promoteAppCreatedPresets,
  removeUnreferencedAppCreated,
  upsertOwnedAppCreatedPreset,
  type AttackPresetDocumentValue,
  type AttackSetupRef,
  type InlineAttackSetup,
} from '../attackPresets/AppCreatedPresets';
import {
  attackPresetReferences,
  defensePresetReferences,
  type PresetDocumentKey,
  type PresetReference,
} from '../attackPresets/AttackPresetReferences';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  type AppAttackPreset,
} from '../attackPresets/AttackPresetTypes';
import { buildPresetDocumentUpdate } from '../configuration/PresetDocumentUpdate';
import {
  appCreatedDefensePresetName,
  promoteAppCreatedDefensePresets,
  removeUnreferencedAppCreatedDefense,
  upsertOwnedAppCreatedDefensePreset,
  type DefenseSetupRef,
  type InlineDefenseSetup,
} from '../defensePresets/AppCreatedDefensePresets';
import {
  DEFENSE_PRESETS_SECTION,
  normalizeDefensePresetSlots,
  parseDefensePresetDocument,
  type AppDefensePreset,
} from '../defensePresets/DefensePresetTypes';
import { isRecordOwnedBy, type AppCreatedRecord, type PresetDocumentValue } from '../presets/AppCreatedRecords';

/** The subset of `useConfigurationDraftSession` the ordered save needs. */
export interface AppCreatedPresetDraftSession {
  sections: Record<string, unknown> | undefined;
  save: (value: unknown) => Promise<ConfigurationSnapshot>;
  saveSection: (section: string, value: unknown) => Promise<ConfigurationSnapshot>;
}

interface SlotLabels {
  slot: string;
  /** Localized labels used only when a new record needs a generated title. */
  moduleLabel: string;
  slotLabel: string;
}

/** An attack slot (default document) or, since CIT-16, a defense slot. */
export type AppCreatedPresetSlotInput =
  | (SlotLabels & { document?: 'attacks.presets'; ref: AttackSetupRef })
  | (SlotLabels & { document: 'defense.presets'; ref: DefenseSetupRef });

export type AppCreatedPresetSaveWarning = 'cleanup-pending';

export interface SaveModuleWithAppCreatedPresetsOptions<TSectionValue> {
  draftSession: AppCreatedPresetDraftSession;
  section: string;
  slots: readonly AppCreatedPresetSlotInput[];
  buildSectionValue: (idsBySlot: Record<string, string>) => TSectionValue;
  /** Localized `attackPresets.appCreatedName` pattern. */
  formatPresetName?: (moduleLabel: string, slotLabel: string) => string;
  /** Receives warnings even when the call throws (rollback failure). */
  warnings?: AppCreatedPresetSaveWarning[];
  now?: () => string;
}

export interface SaveModuleWithAppCreatedPresetsResult {
  idsBySlot: Record<string, string>;
  warnings: AppCreatedPresetSaveWarning[];
}

type InlineSetup = InlineAttackSetup | InlineDefenseSetup;
type SetupRef = AttackSetupRef | DefenseSetupRef;

/** Document-specific operations of the ordered save. */
interface PresetDocumentAdapter<TRecord extends AppCreatedRecord = AppCreatedRecord> {
  key: PresetDocumentKey;
  parse: (raw: unknown) => TRecord[];
  references: (sections: Record<string, unknown> | undefined) => PresetReference[];
  promote: (raw: unknown, current: readonly TRecord[], ids: readonly string[]) => { document: PresetDocumentValue; promoted: string[] };
  upsert: (raw: unknown, current: readonly TRecord[], section: string, slot: string, currentId: string, setup: InlineSetup, name: string, now: string)
    => { document: PresetDocumentValue; presetId: string; changed: boolean };
  removeUnreferenced: (raw: unknown, current: readonly TRecord[], section: string, references: readonly PresetReference[])
    => { document: PresetDocumentValue; removed: string[]; promoted: string[] };
  name: (current: readonly TRecord[], moduleLabel: string, slotLabel: string, format?: (moduleLabel: string, slotLabel: string) => string) => string;
}

// Adapters take the generic record type; each casts to its own document's record type.
const ATTACK_ADAPTER: PresetDocumentAdapter<AppCreatedRecord> = {
  key: ATTACK_PRESETS_SECTION,
  parse: (raw) => parseAttackPresetDocument(raw).presets,
  references: attackPresetReferences,
  promote: (raw, current, ids) => promoteAppCreatedPresets(raw, current as readonly AppAttackPreset[], ids),
  upsert: (raw, current, section, slot, currentId, setup, name, now) =>
    upsertOwnedAppCreatedPreset(raw, current as readonly AppAttackPreset[], section, slot, currentId, setup as InlineAttackSetup, name, now),
  removeUnreferenced: (raw, current, section, references) =>
    removeUnreferencedAppCreated(raw, current as readonly AppAttackPreset[], section, references),
  name: appCreatedPresetName,
};

const DEFENSE_ADAPTER: PresetDocumentAdapter<AppCreatedRecord> = {
  key: DEFENSE_PRESETS_SECTION,
  parse: (raw) => parseDefensePresetDocument(raw).presets,
  references: defensePresetReferences,
  promote: (raw, current, ids) => promoteAppCreatedDefensePresets(raw, current as readonly AppDefensePreset[], ids),
  upsert: (raw, current, section, slot, currentId, setup, name, now) =>
    upsertOwnedAppCreatedDefensePreset(raw, current as readonly AppDefensePreset[], section, slot, currentId, setup as InlineDefenseSetup, name, now),
  removeUnreferenced: (raw, current, section, references) =>
    removeUnreferencedAppCreatedDefense(raw, current as readonly AppDefensePreset[], section, references),
  name: appCreatedDefensePresetName,
};

function slotDocument(input: AppCreatedPresetSlotInput): PresetDocumentKey {
  return input.document ?? ATTACK_PRESETS_SECTION;
}

/**
 * Ordered module save (no multi-section atomic write exists):
 * 1. presets writes — (a) `attacks.presets`, then (b) `defense.presets`: upsert this
 *    module's app-created records and promote other slots' app-created records this
 *    module now references (each skipped when unchanged);
 * 2. module write — the section value with the resolved ids;
 * 3. if (1b) or 2 fails, restore the written preset documents in reverse order
 *    (defense, then attack); a failed restore leaves only an orphan record
 *    (`cleanup-pending`) and the original error is rethrown;
 * 4. cleanup per document — remove this section's app-created records nothing
 *    references, promote those another slot references; failure is a warning only.
 * Records always exist before they are referenced, so no dangling reference is possible.
 */
export async function saveModuleWithAppCreatedPresets<TSectionValue>(
  options: SaveModuleWithAppCreatedPresetsOptions<TSectionValue>,
): Promise<SaveModuleWithAppCreatedPresetsResult> {
  const { draftSession, section, slots, buildSectionValue, formatPresetName } = options;
  const warnings = options.warnings ?? [];
  const now = options.now ?? (() => new Date().toISOString());
  const sections = draftSession.sections ?? {};
  const timestamp = now();
  const documents = [ATTACK_ADAPTER, DEFENSE_ADAPTER]
    .filter((adapter) => adapter.key === ATTACK_PRESETS_SECTION || slots.some((input) => slotDocument(input) === adapter.key));
  const plans = documents.map((adapter) => ({
    adapter,
    baselineRaw: sections[adapter.key],
    plan: planPresetsWrite(adapter, sections[adapter.key], sections, section,
      slots.filter((input) => slotDocument(input) === adapter.key), formatPresetName, timestamp),
  }));

  const written: typeof plans = [];
  const rollback = async () => {
    for (const entry of [...written].reverse()) {
      try {
        await draftSession.saveSection(entry.adapter.key, entry.baselineRaw ?? emptyPresetDocument());
      } catch {
        warnings.push('cleanup-pending');
      }
    }
  };

  const idsBySlot: Record<string, string> = {};
  for (const entry of plans) {
    Object.assign(idsBySlot, entry.plan.idsBySlot);
    if (!entry.plan.changed) continue;
    try {
      await draftSession.saveSection(entry.adapter.key, entry.plan.document);
    } catch (error) {
      await rollback();
      throw error;
    }
    written.push(entry);
  }

  let saved: ConfigurationSnapshot;
  try {
    saved = await draftSession.save(buildSectionValue(idsBySlot));
  } catch (error) {
    await rollback();
    throw error;
  }

  let latestSections = saved?.sections ?? {};
  for (const { adapter } of plans) {
    const latestRaw = latestSections[adapter.key];
    const latest = adapter.parse(latestRaw);
    const cleanup = adapter.removeUnreferenced(
      latestRaw,
      latest,
      section,
      adapter.references(latestSections),
    );
    if (cleanup.removed.length === 0 && cleanup.promoted.length === 0) continue;
    try {
      const snapshot = await draftSession.saveSection(adapter.key, cleanup.document);
      latestSections = snapshot?.sections ?? latestSections;
    } catch {
      warnings.push('cleanup-pending');
    }
  }
  return { idsBySlot, warnings };
}

interface PresetsWritePlan {
  document: PresetDocumentValue;
  idsBySlot: Record<string, string>;
  changed: boolean;
}

function planPresetsWrite(
  adapter: PresetDocumentAdapter<AppCreatedRecord>,
  baselineRaw: unknown,
  sections: Record<string, unknown>,
  section: string,
  slots: readonly AppCreatedPresetSlotInput[],
  formatPresetName: ((moduleLabel: string, slotLabel: string) => string) | undefined,
  now: string,
): PresetsWritePlan {
  let raw: unknown = baselineRaw ?? emptyPresetDocument();
  let current: AppCreatedRecord[] = adapter.parse(raw);
  let document = buildPresetDocumentUpdate(raw, current, current);
  let changed = false;
  const reparse = (next: PresetDocumentValue) => {
    raw = next;
    document = next;
    current = adapter.parse(next);
  };

  // References that stay in place after this save: every other section as saved.
  const otherSectionReferences = adapter.references(sections).filter((reference) => reference.section !== section);
  const selectedId = (ref: SetupRef) => ref.source === 'none' ? '' : ref.presetId;

  // (b) Promote other slots' app-created records that this module now references.
  const promote = slots
    .filter((input) => input.ref.source === 'preset')
    .map((input) => current.find((record) => record.id === selectedId(input.ref)))
    .filter((record): record is AppCreatedRecord => record?.app != null)
    .filter((record) => !slots.some((input) => input.ref.source === 'preset' && record.id === selectedId(input.ref) && isRecordOwnedBy(record, section, input.slot)))
    .map((record) => record.id);
  if (promote.length > 0) {
    const result = adapter.promote(raw, current, promote);
    if (result.promoted.length > 0) {
      reparse(result.document);
      changed = true;
    }
  }

  // (a) Upsert this module's own records for inline slots.
  const idsBySlot: Record<string, string> = {};
  for (const input of slots) {
    const ref = input.ref;
    if (ref.source !== 'inline') {
      idsBySlot[input.slot] = selectedId(ref);
      continue;
    }
    let currentId = ref.presetId;
    const owned = currentId ? current.find((record) => record.id === currentId && isRecordOwnedBy(record, section, input.slot)) : undefined;
    const otherReferrer = otherSectionReferences.some((reference) => reference.presetId === currentId)
      || slots.some((other) => other.slot !== input.slot && selectedId(other.ref) === currentId);
    if (owned && otherReferrer) {
      // Never edit a record under two referrers: the other referrer keeps it as a user preset.
      reparse(adapter.promote(raw, current, [owned.id]).document);
      changed = true;
      currentId = '';
    }
    const name = adapter.name(current, input.moduleLabel, input.slotLabel, formatPresetName);
    const result = adapter.upsert(raw, current, section, input.slot, owned ? currentId : '', ref.setup, name, now);
    if (result.changed) {
      reparse(result.document);
      changed = true;
    }
    idsBySlot[input.slot] = result.presetId;
  }
  return { document, idsBySlot, changed };
}

export function emptyPresetDocument(): AttackPresetDocumentValue {
  return { version: 1, presets: [] };
}

/**
 * Explicit "Save as preset": creates a normal user preset (fresh id, no marker)
 * from an inline setup. The app-created record stays until the module save's
 * cleanup removes it, so no module reference ever dangles.
 */
export async function saveInlineSetupAsUserPreset(
  draftSession: Pick<AppCreatedPresetDraftSession, 'sections' | 'saveSection'>,
  setup: InlineAttackSetup,
  name: string,
  now: string = new Date().toISOString(),
): Promise<string> {
  const trimmed = name.trim();
  const raw = draftSession.sections?.[ATTACK_PRESETS_SECTION] ?? emptyPresetDocument();
  const current = parseAttackPresetDocument(raw).presets;
  const validation = validateUserPresetName(trimmed, current);
  if (validation) throw new Error(validation);
  const preset: AppAttackPreset = {
    id: globalThis.crypto.randomUUID(),
    name: trimmed,
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
    createdAt: now,
    updatedAt: now,
  };
  await draftSession.saveSection(ATTACK_PRESETS_SECTION, buildPresetDocumentUpdate(raw, current, [...current, preset]));
  return preset.id;
}

/** Returns a machine reason when the name cannot be used; the UI maps it to a message. */
export function validateUserPresetName(name: string, presets: readonly { name: string }[]): 'empty' | 'duplicate' | null {
  const trimmed = name.trim();
  if (!trimmed) return 'empty';
  const lower = trimmed.toLowerCase();
  return presets.some((preset) => preset.name.trim().toLowerCase() === lower) ? 'duplicate' : null;
}

/**
 * Explicit "Save as preset" for an inline defense (CIT-16): creates a normal user
 * defense preset (fresh id, no marker). The app-created record stays until the
 * module save's cleanup removes it, so no module reference ever dangles.
 */
export async function saveInlineDefenseAsUserPreset(
  draftSession: Pick<AppCreatedPresetDraftSession, 'sections' | 'saveSection'>,
  setup: InlineDefenseSetup,
  name: string,
  now: string = new Date().toISOString(),
): Promise<string> {
  const trimmed = name.trim();
  const raw = draftSession.sections?.[DEFENSE_PRESETS_SECTION] ?? emptyPresetDocument();
  const current = parseDefensePresetDocument(raw).presets;
  const validation = validateUserPresetName(trimmed, current);
  if (validation) throw new Error(validation);
  const preset: AppDefensePreset = {
    ...normalizeDefensePresetSlots({ name: trimmed, ...setup }),
    id: globalThis.crypto.randomUUID(),
    createdAt: now,
    updatedAt: now,
  };
  await draftSession.saveSection(DEFENSE_PRESETS_SECTION, buildPresetDocumentUpdate(raw, current, [...current, preset]));
  return preset.id;
}

import type { ConfigurationSnapshot } from '../api/Contracts';
import {
  appCreatedPresetName,
  isOwnedBy,
  promoteAppCreatedPresets,
  removeUnreferencedAppCreated,
  upsertOwnedAppCreatedPreset,
  type AttackPresetDocumentValue,
  type AttackSetupRef,
  type InlineAttackSetup,
} from '../attackPresets/AppCreatedPresets';
import { attackPresetReferences } from '../attackPresets/AttackPresetReferences';
import {
  ATTACK_PRESETS_SECTION,
  parseAttackPresetDocument,
  type AppAttackPreset,
} from '../attackPresets/AttackPresetTypes';
import { buildPresetDocumentUpdate } from '../configuration/PresetDocumentUpdate';

/** The subset of `useConfigurationDraftSession` the ordered save needs. */
export interface AppCreatedPresetDraftSession {
  sections: Record<string, unknown> | undefined;
  save: (value: unknown) => Promise<ConfigurationSnapshot>;
  saveSection: (section: string, value: unknown) => Promise<ConfigurationSnapshot>;
}

export interface AppCreatedPresetSlotInput {
  slot: string;
  ref: AttackSetupRef;
  /** Localized labels used only when a new record needs a generated title. */
  moduleLabel: string;
  slotLabel: string;
}

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

/**
 * Ordered module save (no multi-section atomic write exists):
 * 1. presets write — upsert this module's app-created records and promote other
 *    slots' app-created records this module now references (skipped when unchanged);
 * 2. module write — the section value with the resolved ids;
 * 3. if 2 fails, restore the presets baseline; a failed restore leaves only an
 *    orphan record (`cleanup-pending`) and the original error is rethrown;
 * 4. cleanup — remove this section's app-created records nothing references,
 *    promote those another slot references; failure is a warning only.
 * Records always exist before they are referenced, so no dangling reference is possible.
 */
export async function saveModuleWithAppCreatedPresets<TSectionValue>(
  options: SaveModuleWithAppCreatedPresetsOptions<TSectionValue>,
): Promise<SaveModuleWithAppCreatedPresetsResult> {
  const { draftSession, section, slots, buildSectionValue, formatPresetName } = options;
  const warnings = options.warnings ?? [];
  const now = options.now ?? (() => new Date().toISOString());
  const sections = draftSession.sections ?? {};
  const baselineRaw = sections[ATTACK_PRESETS_SECTION];
  const plan = planPresetsWrite(baselineRaw, sections, section, slots, formatPresetName, now());

  let presetsWritten = false;
  if (plan.changed) {
    await draftSession.saveSection(ATTACK_PRESETS_SECTION, plan.document);
    presetsWritten = true;
  }

  let saved: ConfigurationSnapshot;
  try {
    saved = await draftSession.save(buildSectionValue(plan.idsBySlot));
  } catch (error) {
    if (presetsWritten) {
      try {
        await draftSession.saveSection(ATTACK_PRESETS_SECTION, baselineRaw ?? emptyPresetDocument());
      } catch {
        warnings.push('cleanup-pending');
      }
    }
    throw error;
  }

  const latestSections = saved?.sections ?? {};
  const latestRaw = latestSections[ATTACK_PRESETS_SECTION];
  const latest = parseAttackPresetDocument(latestRaw);
  const cleanup = removeUnreferencedAppCreated(
    latestRaw,
    latest.presets,
    section,
    attackPresetReferences(latestSections),
  );
  if (cleanup.removed.length > 0 || cleanup.promoted.length > 0) {
    try {
      await draftSession.saveSection(ATTACK_PRESETS_SECTION, cleanup.document);
    } catch {
      warnings.push('cleanup-pending');
    }
  }
  return { idsBySlot: plan.idsBySlot, warnings };
}

interface PresetsWritePlan {
  document: AttackPresetDocumentValue;
  idsBySlot: Record<string, string>;
  changed: boolean;
}

function planPresetsWrite(
  baselineRaw: unknown,
  sections: Record<string, unknown>,
  section: string,
  slots: readonly AppCreatedPresetSlotInput[],
  formatPresetName: ((moduleLabel: string, slotLabel: string) => string) | undefined,
  now: string,
): PresetsWritePlan {
  let raw: unknown = baselineRaw ?? emptyPresetDocument();
  let current: AppAttackPreset[] = parseAttackPresetDocument(raw).presets;
  let document = buildPresetDocumentUpdate(raw, current, current);
  let changed = false;
  const reparse = (next: AttackPresetDocumentValue) => {
    raw = next;
    document = next;
    current = parseAttackPresetDocument(next).presets;
  };

  // References that stay in place after this save: every other section as saved.
  const otherSectionReferences = attackPresetReferences(sections).filter((reference) => reference.section !== section);
  const selectedId = (input: AppCreatedPresetSlotInput) => input.ref.source === 'none' ? '' : input.ref.presetId;

  // (b) Promote other slots' app-created records that this module now references.
  const promote = slots
    .filter((input) => input.ref.source === 'preset')
    .map((input) => current.find((preset) => preset.id === selectedId(input)))
    .filter((preset): preset is AppAttackPreset => preset?.app != null)
    .filter((preset) => !slots.some((input) => input.ref.source === 'preset' && preset.id === selectedId(input) && isOwnedBy(preset, section, input.slot)))
    .map((preset) => preset.id);
  if (promote.length > 0) {
    const result = promoteAppCreatedPresets(raw, current, promote);
    if (result.promoted.length > 0) {
      reparse(result.document);
      changed = true;
    }
  }

  // (a) Upsert this module's own records for inline slots.
  const idsBySlot: Record<string, string> = {};
  for (const input of slots) {
    if (input.ref.source !== 'inline') {
      idsBySlot[input.slot] = selectedId(input);
      continue;
    }
    const ref = input.ref;
    let currentId = ref.presetId;
    const owned = currentId ? current.find((preset) => preset.id === currentId && isOwnedBy(preset, section, input.slot)) : undefined;
    const otherReferrer = otherSectionReferences.some((reference) => reference.presetId === currentId)
      || slots.some((other) => other.slot !== input.slot && selectedId(other) === currentId);
    if (owned && otherReferrer) {
      // Never edit a record under two referrers: the other referrer keeps it as a user preset.
      reparse(promoteAppCreatedPresets(raw, current, [owned.id]).document);
      changed = true;
      currentId = '';
    }
    const name = appCreatedPresetName(current, input.moduleLabel, input.slotLabel, formatPresetName);
    const result = upsertOwnedAppCreatedPreset(raw, current, section, input.slot, owned ? currentId : '', ref.setup, name, now);
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
export function validateUserPresetName(name: string, presets: readonly AppAttackPreset[]): 'empty' | 'duplicate' | null {
  const trimmed = name.trim();
  if (!trimmed) return 'empty';
  const lower = trimmed.toLowerCase();
  return presets.some((preset) => preset.name.trim().toLowerCase() === lower) ? 'duplicate' : null;
}

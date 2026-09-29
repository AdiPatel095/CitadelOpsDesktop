import type { MessageKey } from '../i18n/messages';

/** The preset documents module fields reference by id. */
export type PresetDocumentKey = 'attacks.presets' | 'defense.presets';

/** One module field that stores an attack or defense preset id in configuration. */
export interface PresetReference {
  document: PresetDocumentKey;
  section: string;
  slot: string;
  presetId: string;
  moduleLabelKey: MessageKey;
  slotLabelKey: MessageKey;
}

/** Attack-document references (kept name for CIT-15 callers). */
export type AttackPresetReference = PresetReference;

export interface PresetSlotDefinition {
  document: PresetDocumentKey;
  section: string;
  slot: string;
  /** Field path inside the module section. */
  path: readonly string[];
  /** Legacy field read when `path` is empty (Nomad `presetId`). */
  legacyPath?: readonly string[];
  moduleLabelKey: MessageKey;
  slotLabelKey: MessageKey;
}

const message = (key: MessageKey): MessageKey => key;

export type AttackPresetSlotDefinition = PresetSlotDefinition;

/**
 * Every module field that stores an attack or defense preset id. Cleanup and
 * delete guards trust this list: a missing field could let cleanup delete a
 * referenced record. `tests/preset-references.test.mjs` fails on any
 * unregistered `*PresetId` field in `settings/*ClientState.ts`.
 */
export const PRESET_SLOTS: readonly PresetSlotDefinition[] = [
  {
    document: 'attacks.presets', section: 'automation.autoNomad', slot: 'nomad', path: ['nomadPresetId'], legacyPath: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoNomad'), slotLabelKey: message('attackPresets.slot.nomad'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoNomad', slot: 'samurai', path: ['samuraiPresetId'], legacyPath: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoNomad'), slotLabelKey: message('attackPresets.slot.samurai'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoInvasion', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoInvasion'), slotLabelKey: message('attackPresets.slot.attack'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoBeriWorld', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoBeriWorld'), slotLabelKey: message('attackPresets.slot.towerAttack'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoKhan', slot: 'attack', path: ['attackPresetId'],
    moduleLabelKey: message('attackPresets.module.autoKhan'), slotLabelKey: message('attackPresets.slot.attack'),
  },
  {
    document: 'defense.presets', section: 'automation.autoKhan', slot: 'defense', path: ['defensePresetId'],
    moduleLabelKey: message('attackPresets.module.autoKhan'), slotLabelKey: message('attackPresets.slot.defense'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoStorm', slot: 'forts', path: ['forts', 'presetId'],
    moduleLabelKey: message('attackPresets.module.autoStorm'), slotLabelKey: message('attackPresets.slot.forts'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoStorm', slot: 'islands', path: ['islands', 'presetId'],
    moduleLabelKey: message('attackPresets.module.autoStorm'), slotLabelKey: message('attackPresets.slot.islands'),
  },
  {
    document: 'attacks.presets', section: 'automation.autoAdvisor', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoAdvisor'), slotLabelKey: message('attackPresets.slot.attack'),
  },
];

export const ATTACK_PRESET_SLOTS: readonly PresetSlotDefinition[] = PRESET_SLOTS.filter((definition) => definition.document === 'attacks.presets');
export const DEFENSE_PRESET_SLOTS: readonly PresetSlotDefinition[] = PRESET_SLOTS.filter((definition) => definition.document === 'defense.presets');

/** Every non-empty attack preset reference in the given configuration sections. */
export function attackPresetReferences(sections: Record<string, unknown> | undefined): PresetReference[] {
  return presetReferences(sections, 'attacks.presets');
}

/** Every non-empty defense preset reference in the given configuration sections. */
export function defensePresetReferences(sections: Record<string, unknown> | undefined): PresetReference[] {
  return presetReferences(sections, 'defense.presets');
}

/** Every non-empty preset reference of one document in the given configuration sections. */
export function presetReferences(sections: Record<string, unknown> | undefined, document: PresetDocumentKey): PresetReference[] {
  const references: PresetReference[] = [];
  for (const definition of PRESET_SLOTS) {
    if (definition.document !== document) continue;
    const value = sections?.[definition.section];
    const presetId = stringAt(value, definition.path) || (definition.legacyPath ? stringAt(value, definition.legacyPath) : '');
    if (!presetId) continue;
    references.push({
      document: definition.document,
      section: definition.section,
      slot: definition.slot,
      presetId,
      moduleLabelKey: definition.moduleLabelKey,
      slotLabelKey: definition.slotLabelKey,
    });
  }
  return references;
}

export function attackPresetReferrers(
  references: readonly PresetReference[],
  presetId: string,
): PresetReference[] {
  return references.filter((reference) => reference.presetId === presetId);
}

export const presetReferrers = attackPresetReferrers;

export function attackPresetSlotDefinition(section: string, slot: string): PresetSlotDefinition | undefined {
  return presetSlotDefinition('attacks.presets', section, slot);
}

export function presetSlotDefinition(document: PresetDocumentKey, section: string, slot: string): PresetSlotDefinition | undefined {
  return PRESET_SLOTS.find((definition) => definition.document === document && definition.section === section && definition.slot === slot);
}

function stringAt(value: unknown, path: readonly string[]): string {
  let current: unknown = value;
  for (const key of path) {
    if (current == null || typeof current !== 'object' || Array.isArray(current)) return '';
    current = (current as Record<string, unknown>)[key];
  }
  return typeof current === 'string' ? current.trim() : '';
}

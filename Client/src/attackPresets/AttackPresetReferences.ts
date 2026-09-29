import type { MessageKey } from '../i18n/messages';

/** One module field that stores an attack preset id in configuration. */
export interface AttackPresetReference {
  section: string;
  slot: string;
  presetId: string;
  moduleLabelKey: MessageKey;
  slotLabelKey: MessageKey;
}

export interface AttackPresetSlotDefinition {
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

/**
 * Every module field that stores an attack preset id. Cleanup and delete guards
 * trust this list: a missing field could let cleanup delete a referenced record.
 * `tests/attack-preset-references.test.mjs` fails on any unregistered
 * `*PresetId` field in `settings/*ClientState.ts`. CIT-16 extends it.
 */
export const ATTACK_PRESET_SLOTS: readonly AttackPresetSlotDefinition[] = [
  {
    section: 'automation.autoNomad', slot: 'nomad', path: ['nomadPresetId'], legacyPath: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoNomad'), slotLabelKey: message('attackPresets.slot.nomad'),
  },
  {
    section: 'automation.autoNomad', slot: 'samurai', path: ['samuraiPresetId'], legacyPath: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoNomad'), slotLabelKey: message('attackPresets.slot.samurai'),
  },
  {
    section: 'automation.autoInvasion', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoInvasion'), slotLabelKey: message('attackPresets.slot.attack'),
  },
  {
    section: 'automation.autoBeriWorld', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoBeriWorld'), slotLabelKey: message('attackPresets.slot.towerAttack'),
  },
  {
    section: 'automation.autoKhan', slot: 'attack', path: ['attackPresetId'],
    moduleLabelKey: message('attackPresets.module.autoKhan'), slotLabelKey: message('attackPresets.slot.attack'),
  },
  {
    section: 'automation.autoStorm', slot: 'forts', path: ['forts', 'presetId'],
    moduleLabelKey: message('attackPresets.module.autoStorm'), slotLabelKey: message('attackPresets.slot.forts'),
  },
  {
    section: 'automation.autoStorm', slot: 'islands', path: ['islands', 'presetId'],
    moduleLabelKey: message('attackPresets.module.autoStorm'), slotLabelKey: message('attackPresets.slot.islands'),
  },
  {
    section: 'automation.autoAdvisor', slot: 'attack', path: ['presetId'],
    moduleLabelKey: message('attackPresets.module.autoAdvisor'), slotLabelKey: message('attackPresets.slot.attack'),
  },
];

/** Every non-empty attack preset reference in the given configuration sections. */
export function attackPresetReferences(sections: Record<string, unknown> | undefined): AttackPresetReference[] {
  const references: AttackPresetReference[] = [];
  for (const definition of ATTACK_PRESET_SLOTS) {
    const value = sections?.[definition.section];
    const presetId = stringAt(value, definition.path) || (definition.legacyPath ? stringAt(value, definition.legacyPath) : '');
    if (!presetId) continue;
    references.push({
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
  references: readonly AttackPresetReference[],
  presetId: string,
): AttackPresetReference[] {
  return references.filter((reference) => reference.presetId === presetId);
}

export function attackPresetSlotDefinition(section: string, slot: string): AttackPresetSlotDefinition | undefined {
  return ATTACK_PRESET_SLOTS.find((definition) => definition.section === section && definition.slot === slot);
}

function stringAt(value: unknown, path: readonly string[]): string {
  let current: unknown = value;
  for (const key of path) {
    if (current == null || typeof current !== 'object' || Array.isArray(current)) return '';
    current = (current as Record<string, unknown>)[key];
  }
  return typeof current === 'string' ? current.trim() : '';
}

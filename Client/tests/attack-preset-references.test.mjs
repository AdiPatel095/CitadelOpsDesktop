import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readdir, readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const registry = await vite.ssrLoadModule('/src/attackPresets/AttackPresetReferences.ts');

after(async () => {
  await vite.close();
});

test('registry returns every attack preset slot for fixture sections', () => {
  const references = registry.attackPresetReferences({
    'automation.autoNomad': { nomadPresetId: 'n', samuraiPresetId: ' s ' },
    'automation.autoInvasion': { presetId: 'i' },
    'automation.autoBeriWorld': { presetId: 'b' },
    'automation.autoKhan': { attackPresetId: 'k', defensePresetId: 'defense-not-attack' },
    'automation.autoStorm': { forts: { presetId: 'f' }, islands: { presetId: 'is' }, decorationPresetId: 'decoration' },
    'automation.autoAdvisor': { presetId: 'a' },
  });
  assert.deepEqual(references.map(({ section, slot, presetId }) => `${section}:${slot}=${presetId}`), [
    'automation.autoNomad:nomad=n',
    'automation.autoNomad:samurai=s',
    'automation.autoInvasion:attack=i',
    'automation.autoBeriWorld:attack=b',
    'automation.autoKhan:attack=k',
    'automation.autoStorm:forts=f',
    'automation.autoStorm:islands=is',
    'automation.autoAdvisor:attack=a',
  ]);
  for (const reference of references) {
    assert.ok(reference.moduleLabelKey.startsWith('attackPresets.module.'));
    assert.ok(reference.slotLabelKey.startsWith('attackPresets.slot.'));
  }
});

test('registry reads the Nomad legacy presetId exactly like the Nomad parser', () => {
  const legacy = registry.attackPresetReferences({ 'automation.autoNomad': { presetId: 'legacy' } });
  assert.deepEqual(legacy.map(({ slot, presetId }) => `${slot}=${presetId}`), ['nomad=legacy', 'samurai=legacy']);
  const mixed = registry.attackPresetReferences({ 'automation.autoNomad': { presetId: 'legacy', samuraiPresetId: 'new' } });
  assert.deepEqual(mixed.map(({ slot, presetId }) => `${slot}=${presetId}`), ['nomad=legacy', 'samurai=new']);
  assert.deepEqual(registry.attackPresetReferences({}), []);
  assert.deepEqual(registry.attackPresetReferences(undefined), []);
  assert.deepEqual(registry.attackPresetReferences({ 'automation.autoStorm': { forts: 'bad' } }), []);
});

// Fields that store ids of other preset families. Anything else must be registered.
const NON_ATTACK_PRESET_FIELDS = {
  'AutoBirdClientState.ts': new Set(['activePresetId', 'presetId']), // Auto Bird presets inside automation.autoBird
  'AutoKhanClientState.ts': new Set(['defensePresetId']), // defense presets
  'AutoStormClientState.ts': new Set(['decorationPresetId']), // decoration presets
};

// Client-state files whose module section has no exported *_SECTION constant.
const SECTION_OVERRIDES = {
  'AutoBeriWorldClientState.ts': 'automation.autoBeriWorld', // written by AutoBeriWorldSettingsModal
};

test('every *PresetId/presetId field in settings/*ClientState.ts maps to a registry entry', async () => {
  const settingsDirectory = new URL('../src/settings/', import.meta.url);
  const files = (await readdir(settingsDirectory)).filter((name) => name.endsWith('ClientState.ts'));
  assert.ok(files.length > 10);
  const unregistered = [];
  for (const file of files) {
    const source = await readFile(new URL(file, settingsDirectory), 'utf8');
    // Property names only (declarations, object keys, property reads), not local variables.
    const fields = new Set(Array.from(
      source.matchAll(/(?:\.|\b)([A-Za-z]*PresetId|presetId)\b(?=\s*\??:)|\.([A-Za-z]*PresetId|presetId)\b/g),
      (match) => match[1] ?? match[2],
    ));
    if (fields.size === 0) continue;
    const section = SECTION_OVERRIDES[file] ?? source.match(/export const [A-Z_]+_SECTION = '([^']+)'/)?.[1];
    for (const field of fields) {
      if (NON_ATTACK_PRESET_FIELDS[file]?.has(field)) continue;
      const registered = registry.ATTACK_PRESET_SLOTS.some((definition) => definition.section === section
        && (definition.path.at(-1) === field || definition.legacyPath?.at(-1) === field));
      if (!registered) unregistered.push(`${file}: ${field} (section ${section ?? 'unknown'})`);
    }
  }
  assert.deepEqual(unregistered, [], `Register these attack preset id fields in ATTACK_PRESET_SLOTS or document them as non-attack fields: ${unregistered.join(', ')}`);
});

test('the Beri World section constant used by the registry matches the modal write path', async () => {
  const modal = await readFile(new URL('../src/settings/components/AutoBeriWorldSettingsModal.tsx', import.meta.url), 'utf8');
  assert.match(modal, /'automation\.autoBeriWorld'/);
  assert.ok(registry.ATTACK_PRESET_SLOTS.some((definition) => definition.section === 'automation.autoBeriWorld'));
});

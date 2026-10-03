import assert from 'node:assert/strict';
import { after, test } from 'node:test';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';

const clientRoot = fileURLToPath(new URL('..', import.meta.url));
const vite = await createServer({
  root: clientRoot,
  appType: 'custom',
  logLevel: 'silent',
  server: { middlewareMode: true },
});
const edits = await vite.ssrLoadModule('/src/attackPresets/AttackPresetEdits.ts');
const types = await vite.ssrLoadModule('/src/attackPresets/AttackPresetTypes.ts');
const { buildPresetDocumentUpdate } = await vite.ssrLoadModule('/src/configuration/PresetDocumentUpdate.ts');

after(async () => {
  await vite.close();
});

const lane = (troops) => ({ troops: troops.map(([itemId, quantity]) => ({ itemId, quantity })), tools: [] });
const raw = {
  version: 1,
  presets: [
    {
      id: 'app:automation.autoNomad:nomad:0a0b0c0d',
      name: 'Auto Nomad – Nomad (auto)',
      targetType: 'pve',
      waves: [{ L: lane([[10, 5]]), M: lane([]), R: lane([]) }],
      createdAt: '2026-09-01T00:00:00.000Z',
      updatedAt: '2026-09-01T00:00:00.000Z',
      app: { section: 'automation.autoNomad', slot: 'nomad' },
    },
  ],
};
const NOW = '2026-09-29T12:00:00.000Z';

test('duplicating an app-created preset copies only: the original keeps its marker', () => {
  const current = types.parseAttackPresetDocument(raw).presets;
  const [original] = current;
  const { duplicate, presets } = edits.duplicateAttackPreset(current, original, { id: 'copy-1', now: NOW });
  assert.equal(presets[0], original, 'the original record object is untouched');
  assert.deepEqual(presets[0].app, { section: 'automation.autoNomad', slot: 'nomad' });
  assert.equal(Object.hasOwn(duplicate, 'app'), false, 'the copy is a normal preset');
  assert.equal(duplicate.id, 'copy-1');
  assert.equal(duplicate.name, 'Auto Nomad – Nomad (auto) copy');
  assert.deepEqual(duplicate.waves, original.waves);
  assert.notEqual(duplicate.waves, original.waves, 'deep copy');
  const document = buildPresetDocumentUpdate(raw, current, presets);
  assert.equal(document.presets[0], raw.presets[0], 'the original raw record, marker included, is written back verbatim');
  assert.equal(document.presets.length, 2);
});

test('uniqueCopyName suffixes existing copies', () => {
  assert.equal(edits.uniqueCopyName('A', [{ name: 'A copy' }, { name: 'a copy 2' }]), 'A copy 3');
});

test('saving the editor rebuilds the record without the marker, promoting an edited app-created preset', () => {
  const [original] = types.parseAttackPresetDocument(raw).presets;
  const draft = { name: '  Renamed  ', useTroopFamilies: true, waves: [{ L: lane([[11, 7]]), M: lane([]), R: lane([]) }], courtyardSupport: original.courtyardSupport };
  const edited = edits.editedAttackPreset(original, draft, { id: 'unused', now: NOW });
  assert.equal(Object.hasOwn(edited, 'app'), false);
  assert.equal(edited.id, original.id, 'the module keeps referencing the same id');
  assert.equal(edited.name, 'Renamed');
  assert.equal(edited.targetType, 'pve');
  assert.equal(edited.useTroopFamilies, true);
  assert.equal(edited.createdAt, original.createdAt);
  assert.equal(edited.updatedAt, NOW);
  const created = edits.editedAttackPreset(undefined, draft, { id: 'new-id', targetType: 'pvp', now: NOW });
  assert.equal(created.id, 'new-id');
  assert.equal(created.targetType, 'pvp');
  assert.equal(created.createdAt, NOW);
});

test('the Attack Presets view saves and duplicates through these helpers without a duplicate confirm', async () => {
  const view = await readFile(new URL('../src/views/AttackPresetsView.tsx', import.meta.url), 'utf8');
  assert.match(view, /editedAttackPreset\(/);
  assert.match(view, /duplicateAttackPreset\(/);
  const duplicateHandler = view.slice(view.indexOf('const handleDuplicate'), view.indexOf('const handleDelete'));
  assert.doesNotMatch(duplicateHandler, /confirmPromotion|withoutMarker/);
});
